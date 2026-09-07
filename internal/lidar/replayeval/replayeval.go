// Package replayeval replays a captured PCAP offline through the full L1-L6
// perception pipeline and records the result as a VRLOG, without a server,
// a database, or a listening port.
//
// It exists because a VRLOG cannot answer the question a tracker change poses.
// A VRLOG stores decisions the pipeline already made, so replaying one shows
// what the old code concluded, not what the new code would conclude. Measuring
// a change to L4, L5 or L6 means re-running perception over the packets. Until
// now the only route to that was the live server's PCAP replay endpoint, which
// binds ports and shares state with whatever else the server is doing.
//
// The output is a VRLOG directory, so everything downstream already works:
// analysis.GenerateReport for metrics, analysis.CompareReports for A/B, and the
// macOS visualiser for looking at it.
package replayeval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
	"github.com/banshee-data/velocity.report/internal/version"
)

// sha256Sum is a small helper so the provenance block reads in one line.
func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

// schemaVersionOrUnknown reports the tuning config's parameter schema version,
// or "unknown" when the loaded config does not carry one.
func schemaVersionOrUnknown(cfg *config.TuningConfig) string {
	if cfg == nil {
		return "unknown"
	}
	return fmt.Sprintf("v%d", cfg.Version)
}

// Config holds the parameters for an offline perception replay.
type Config struct {
	// PCAPFile is the capture to replay. Required.
	PCAPFile string
	// OutDir is the directory the VRLOG is written into. Required.
	OutDir string
	// TuningFile is the tuning config to run with. Empty uses the default
	// path, falling back to the binary-embedded defaults. Pointing two runs at
	// two different files is how an A/B comparison is set up.
	TuningFile string
	// SensorID is stamped into the recording. Defaults to "pcap-replay".
	SensorID string
	// UDPPort filters packets in the capture. Callers should resolve 0 to a
	// detected port before calling.
	UDPPort int
	// StartSeconds and DurationSeconds window the replay. A zero duration
	// means the whole capture.
	StartSeconds    float64
	DurationSeconds float64
	// WarmupSeconds processes this much of the same capture before StartSeconds.
	// It does not record that prefix. Background and tracker state carry over.
	WarmupSeconds float64
	// RequireSettled fails if the grid is not settled at the scoring boundary.
	// This checks grid convergence, not whether the sensor was physically static.
	RequireSettled bool
	// IncludePoints records the point cloud in the VRLOG. This dominates the
	// output size, so it is off unless the recording is meant for the
	// visualiser rather than for metrics.
	IncludePoints bool
	// ProgressEvery logs a line every N frames. Zero disables progress logs.
	ProgressEvery int
}

// Result summarises a completed replay.
type Result struct {
	VRLOGPath      string
	FramesRead     int
	FramesEmpty    int
	FramesRecorded int
	WarmupFrames   int
	Elapsed        time.Duration
	TuningFile     string
	SensorID       string
	SourcePCAP     string
}

// recordingPublisher writes each adapted FrameBundle straight to a recorder.
// It stands in for the gRPC publisher, which is the only reason the pipeline
// normally needs a server to produce a VRLOG.
type recordingPublisher struct {
	rec              *recorder.Recorder
	mu               sync.Mutex
	recorded         int
	writeErr         error
	dropPoints       bool
	emptyFrames      int
	recordAfterNanos int64
	warmupFrames     int
}

func (p *recordingPublisher) Publish(frame interface{}) {
	bundle, ok := frame.(*l9endpoints.FrameBundle)
	if !ok || bundle == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if bundle.TimestampNanos < p.recordAfterNanos {
		p.warmupFrames++
		return
	}
	if p.writeErr != nil {
		return
	}
	if p.dropPoints {
		// Metrics runs do not need the cloud, and it is the bulk of the file.
		bundle.PointCloud = nil
	}
	if bundle.Tracks == nil || len(bundle.Tracks.Tracks) == 0 {
		p.emptyFrames++
	}
	if err := p.rec.Record(bundle); err != nil {
		p.writeErr = err
	}
	p.recorded++
}

// Run replays the capture through the perception pipeline and writes a VRLOG.
//
// Nothing here touches a database or a socket. Track persistence is disabled
// explicitly rather than by leaving DB nil, so that a future pipeline change
// that starts assuming a DB fails loudly here instead of writing into the
// production store during an analysis run.
func Run(cfg Config) (*Result, error) {
	start := time.Now()

	if cfg.PCAPFile == "" {
		return nil, fmt.Errorf("PCAPFile is required")
	}
	if cfg.OutDir == "" {
		return nil, fmt.Errorf("OutDir is required")
	}
	for _, v := range []float64{cfg.StartSeconds, cfg.DurationSeconds, cfg.WarmupSeconds} {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) >= float64(math.MaxInt64)/1e9 {
			return nil, fmt.Errorf("replay windows must be finite representable seconds")
		}
	}
	if cfg.StartSeconds < 0 || cfg.WarmupSeconds < 0 || cfg.WarmupSeconds > cfg.StartSeconds || (cfg.DurationSeconds < 0 && cfg.DurationSeconds != -1) {
		return nil, fmt.Errorf("invalid replay window: require start >= warmup >= 0 and duration >= -1")
	}
	if cfg.DurationSeconds > 0 && cfg.DurationSeconds+cfg.WarmupSeconds >= float64(math.MaxInt64)/1e9 {
		return nil, fmt.Errorf("processing window exceeds representable duration")
	}
	if entries, err := os.ReadDir(cfg.OutDir); err == nil && len(entries) != 0 {
		return nil, fmt.Errorf("output directory must be empty: %s", cfg.OutDir)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect output directory: %w", err)
	}
	if cfg.SensorID == "" {
		cfg.SensorID = "pcap-replay"
	}
	if cfg.DurationSeconds == 0 {
		cfg.DurationSeconds = -1
	}
	if cfg.TuningFile == "" {
		cfg.TuningFile = config.DefaultConfigPath
	}

	tuningCfg, err := config.LoadTuningConfigOrEmbedded(cfg.TuningFile, radarassets.TuningDefaults)
	if err != nil {
		return nil, fmt.Errorf("load tuning config %s: %w", cfg.TuningFile, err)
	}
	packets, err := network.CountPCAPPackets(cfg.PCAPFile, cfg.UDPPort)
	if err != nil {
		return nil, fmt.Errorf("inspect capture: %w", err)
	}
	if packets.Count == 0 {
		return nil, fmt.Errorf("capture has no packets on UDP port %d", cfg.UDPPort)
	}
	scoreStart := packets.FirstTimestampNs + int64(cfg.StartSeconds*1e9)
	if scoreStart < packets.FirstTimestampNs || scoreStart > packets.LastTimestampNs {
		return nil, fmt.Errorf("scoring window starts outside capture")
	}
	pcapHash, err := fileSHA256(cfg.PCAPFile)
	if err != nil {
		return nil, fmt.Errorf("hash capture: %w", err)
	}

	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("create out dir %s: %w", cfg.OutDir, err)
	}

	// --- L1: parser ---
	parserCfg, err := parse.LoadPandar40PConfig()
	if err != nil {
		return nil, fmt.Errorf("load parser config: %w", err)
	}
	parser := parse.NewPandar40PParser(*parserCfg)
	elevations := parse.ElevationsFromConfig(parserCfg)

	// --- L3: background model ---
	bgConfig := l3grid.BackgroundConfigFromActiveTuning(tuningCfg)
	if err := bgConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid background config: %w", err)
	}
	const rings, azBins = 40, 1800
	bgMgr := l3grid.NewBackgroundManagerDI(cfg.SensorID, rings, azBins, bgConfig.ToBackgroundParams(), nil)
	if bgMgr == nil {
		return nil, fmt.Errorf("failed to create BackgroundManager")
	}
	if err := bgMgr.SetRingElevations(elevations); err != nil {
		return nil, fmt.Errorf("set ring elevations: %w", err)
	}
	bgMgr.SetSourcePath(cfg.PCAPFile)

	// --- L5, L6 ---
	tracker := l5tracks.NewTracker(l5tracks.TrackerConfigFromTuning(tuningCfg.L5.CvKfV1))
	classifier := l6objects.NewTrackClassifierWithMinObservations(
		tuningCfg.GetMinObservationsForClassification())

	// --- Recorder + publisher ---
	rec, err := recorder.NewRecorder(cfg.OutDir, cfg.SensorID)
	if err != nil {
		return nil, fmt.Errorf("create recorder: %w", err)
	}
	pub := &recordingPublisher{rec: rec, dropPoints: !cfg.IncludePoints, recordAfterNanos: scoreStart}
	adapter := l9endpoints.NewFrameAdapter(cfg.SensorID)

	// Provenance. Without it a recording cannot say which code and which
	// parameters produced it, which is the one thing an A/B corpus has to be
	// able to answer. The live path takes these from the run-config store in
	// the database; there is no database here, so they are derived from the
	// tuning file actually loaded and from the build stamp.
	paramsJSON, err := json.Marshal(tuningCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal tuning config for provenance: %w", err)
	}
	paramsHash := "sha256:" + hex.EncodeToString(sha256Sum(paramsJSON))

	rec.SetDeterministicConfig(
		"",         // no run-config row exists offline
		"",         // nor a param-set row
		paramsHash, // config and params are the same object here
		paramsHash,
		schemaVersionOrUnknown(tuningCfg),
		"replay", // distinguishes these from effective/requested run configs
		version.Version,
		version.GitSHA,
		paramsJSON,
	)
	rec.SetProvenance("pcap", filepath.Base(cfg.PCAPFile), paramsHash, 0)

	// --- Pipeline ---
	// The frame-rate throttle is left off. It exists to stop a real-time
	// replay flooding a live gRPC client, and dropping frames here would make
	// two runs of the same capture disagree for reasons unrelated to the
	// change under test. Determinism matters more than throughput offline.
	disablePersistence := &atomic.Bool{}
	disablePersistence.Store(true)

	pipeCfg := &pipeline.TrackingPipelineConfig{
		BackgroundManager:       bgMgr,
		Tracker:                 tracker,
		Classifier:              classifier,
		SensorID:                cfg.SensorID,
		VisualiserPublisher:     pub,
		VisualiserAdapter:       adapter,
		DisableTrackPersistence: disablePersistence,
		HeightBandFloor:         tuningCfg.GetHeightBandFloor(),
		HeightBandCeiling:       tuningCfg.GetHeightBandCeiling(),
		RemoveGround:            tuningCfg.GetRemoveGround(),
	}
	pipelineCallback := pipeCfg.NewFrameCallback()

	var frameCount int
	var boundaryChecked, settledAtBoundary bool
	frameCallback := func(frame *l2frames.LiDARFrame) {
		if frame == nil {
			return
		}
		frameCount++
		// Check before consuming the first scored frame: the scored observation
		// cannot itself be used to certify its own warm-up.
		if !boundaryChecked && frame.StartTimestamp.UnixNano() >= scoreStart {
			boundaryChecked = true
			settledAtBoundary = bgMgr.IsSettlingComplete()
			if cfg.RequireSettled && !settledAtBoundary {
				pub.mu.Lock()
				pub.writeErr = fmt.Errorf("background not settled at scoring boundary")
				pub.mu.Unlock()
			}
		}
		pipelineCallback(frame)
		if cfg.ProgressEvery > 0 && frameCount%cfg.ProgressEvery == 0 {
			log.Printf("frame=%d recorded=%d", frameCount, pub.recorded)
		}
	}

	fb := l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{
		SensorID:        cfg.SensorID,
		FrameCallback:   frameCallback,
		FrameChCapacity: 32,
	})

	// Back-pressure instead of frame dropping. In the default mode the
	// FrameBuilder discards a frame when the callback channel is full, which
	// is correct for a live sensor and fatal here: the PCAP reader outruns
	// clustering and tracking, so frames are lost at a rate that depends on
	// how busy the machine is. Two runs of the same capture then disagree for
	// reasons unrelated to whatever is being tested, and the absolute figures
	// describe a timing-dependent subset of the capture rather than the
	// capture. pcapsplit and the server's own analysis mode both set this for
	// the same reason.
	fb.SetBlockOnFrameChannel(true)

	log.Printf("replaying %s (port %d) through the perception pipeline", cfg.PCAPFile, cfg.UDPPort)
	processingDuration := cfg.DurationSeconds
	if processingDuration > 0 {
		processingDuration += cfg.WarmupSeconds
	}
	replayErr := network.ReadPCAPFile(
		context.Background(),
		cfg.PCAPFile,
		cfg.UDPPort,
		parser,
		fb,
		nil, nil,
		cfg.StartSeconds-cfg.WarmupSeconds,
		processingDuration,
		0, 0, nil,
	)

	// Drain before closing the recorder, or the tail of the capture is lost.
	fb.Close()

	if cerr := rec.Close(); cerr != nil && replayErr == nil {
		replayErr = fmt.Errorf("close recording: %w", cerr)
	}
	if replayErr != nil {
		return nil, fmt.Errorf("pcap replay: %w", replayErr)
	}
	if pub.writeErr != nil {
		return nil, fmt.Errorf("record frame: %w", pub.writeErr)
	}
	if pub.recorded == 0 {
		return nil, fmt.Errorf("replay produced no frames: check the UDP port filter (%d) and the capture window", cfg.UDPPort)
	}
	parserJSON, err := json.Marshal(parserCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal calibration: %w", err)
	}
	manifest := map[string]interface{}{
		"schema_version": 1, "source_sha256": pcapHash, "source_basename": filepath.Base(cfg.PCAPFile),
		"source_first_ns": packets.FirstTimestampNs, "source_last_ns": packets.LastTimestampNs,
		"processing_start_seconds": cfg.StartSeconds - cfg.WarmupSeconds,
		"scoring_start_seconds":    cfg.StartSeconds, "scoring_start_ns": scoreStart,
		"scoring_duration_seconds": cfg.DurationSeconds, "warmup_seconds": cfg.WarmupSeconds,
		"tracker_boundary_policy": "retain", "settled_at_boundary": settledAtBoundary,
		"require_settled": cfg.RequireSettled, "sensor_id": cfg.SensorID, "udp_port": cfg.UDPPort,
		"include_points": cfg.IncludePoints, "params_sha256": paramsHash,
		"calibration_sha256": "sha256:" + hex.EncodeToString(sha256Sum(parserJSON)),
		"build_version":      version.Version, "build_git_sha": version.GitSHA,
		"build_stamped":    version.GitSHA != "" && version.GitSHA != "unknown" && version.GitSHA != "dev",
		"frames_processed": frameCount, "frames_recorded": pub.recorded, "warmup_frames": pub.warmupFrames,
		"warmup_static_verified": false,
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal replay manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.OutDir, "replay_manifest.json"), append(manifestJSON, '\n'), 0644); err != nil {
		return nil, fmt.Errorf("write replay manifest: %w", err)
	}

	// Phase 0 baseline. These are the tracker's own residuals and association
	// rates, which a VRLOG cannot carry: it records the estimates the pipeline
	// published, not what they disagreed with the observations about. Written
	// beside the recording so a baseline can be compared run to run.
	if err := writeTrackingBaseline(cfg.OutDir, tracker.GetTrackingMetrics()); err != nil {
		return nil, err
	}

	return &Result{
		VRLOGPath:      filepath.Clean(cfg.OutDir),
		FramesRead:     frameCount,
		FramesEmpty:    pub.emptyFrames,
		FramesRecorded: pub.recorded,
		WarmupFrames:   pub.warmupFrames,
		Elapsed:        time.Since(start),
		TuningFile:     cfg.TuningFile,
		SensorID:       cfg.SensorID,
		SourcePCAP:     cfg.PCAPFile,
	}, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// TrackingBaseline is the Phase 0 filter-consistency baseline for one run.
type TrackingBaseline struct {
	SchemaVersion int                               `json:"schema_version"`
	Residuals     []l5tracks.ResidualBandSummary    `json:"residual_bands"`
	Association   []l5tracks.AssociationBandSummary `json:"association_bands"`
}

// writeTrackingBaseline records the residual and association bands beside the
// recording.
//
// It holds only live tracks: a deleted track's accumulators stop when it dies,
// and rolling them in would mix a track's whole life into a window it was only
// partly present for.
func writeTrackingBaseline(outDir string, m l5tracks.TrackingMetrics) error {
	b, err := json.MarshalIndent(TrackingBaseline{
		SchemaVersion: 1,
		Residuals:     m.Residuals,
		Association:   m.Association,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tracking baseline: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "tracking_baseline.json"), append(b, '\n'), 0644); err != nil {
		return fmt.Errorf("write tracking baseline: %w", err)
	}
	return nil
}
