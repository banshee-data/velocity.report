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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/debug"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
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

// offlineFrameBuilderConfig removes host wall-clock time from replay frame
// boundaries. Live builders need periodic cleanup when packets stop arriving;
// an offline reader has an explicit EOF and Close flushes its pending frames.
// Leaving the live timeout armed made an observation-heavy run finalise partial
// rotations that a faster repeat completed on azimuth wrap.
func offlineFrameBuilderConfig(sensorID string, callback func(*l2frames.LiDARFrame)) l2frames.FrameBuilderConfig {
	return l2frames.FrameBuilderConfig{
		SensorID:        sensorID,
		FrameCallback:   callback,
		FrameChCapacity: 32,
		BufferTimeout:   time.Duration(math.MaxInt64),
		CleanupInterval: time.Duration(math.MaxInt64),
	}
}

// Config holds the parameters for an offline perception replay.
type Config struct {
	// PCAPFile is the capture to replay. It remains available for callers with
	// one file; PCAPFiles is the ordered multi-file form.
	PCAPFile string
	// PCAPFiles is an ordered capture sequence. It is mutually exclusive with
	// PCAPFile. Each join is graded before replay and a broken join is refused.
	PCAPFiles []string
	// PCAPSHA256s optionally supplies the already-verified digests for
	// PCAPFiles, in the same order. A corpus runner may have just made an
	// immutable source manifest from these bytes; accepting that proof avoids
	// a second full disk read before the replay itself.
	PCAPSHA256s []string
	// CaptureSequence optionally supplies the already-validated ordered packet
	// sequence for PCAPFiles. It is useful to a two-pass corpus runner: packet
	// counting reads every capture, so the repeat must not do that exact work
	// again. Run still validates that it names this exact ordered file list.
	CaptureSequence *capseq.Sequence
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
	// IncludeDebug records predictions, innovations, associations, and raw
	// cluster boxes alongside track estimates. Disabled by default.
	IncludeDebug bool
	// ProgressEvery logs a line every N frames. Zero disables progress logs.
	ProgressEvery int

	// ObservationDBPath enables immutable L4 observation storage. It is an
	// explicit offline output, never the server's live database. The replay case
	// and calibration are required because names and an identity transform must
	// not be guessed from a sensor label.
	ObservationDBPath          string
	ReplayCaseID               string
	ObservationCalibration     l4bobserve.Calibration
	ObservationMaxSamplePoints int
	// ProfileEvidence includes exact frame-evidence persistence timings in the
	// returned Result. It is diagnostic-only and does not alter recorded data.
	ProfileEvidence bool
	// MeasurementSourceMode selects a replay-only position model. Empty uses
	// the shipped OBB-centre D2; medoid_v0 exists solely to establish the
	// historical reference arm for an acceptance comparison.
	MeasurementSourceMode l5tracks.MeasurementSource
	// UseSurfaceGround enables P11's settled-background, surface-relative
	// clipping. It remains opt-in while multi-site evidence is collected.
	UseSurfaceGround     bool
	SurfaceGroundFloor   float64
	SurfaceGroundCeiling float64
	// SurfaceGroundRegionMetres is the per-region ground-plane cell size; 0
	// uses l3grid.DefaultRegionSizeMetres. See TrackingPipelineConfig.
	SurfaceGroundRegionMetres float64
}

// Result summarises a completed replay.
type Result struct {
	VRLOGPath           string
	FramesRead          int
	FramesEmpty         int
	FramesRecorded      int
	WarmupFrames        int
	Elapsed             time.Duration
	TuningFile          string
	SensorID            string
	SourcePCAP          string
	SourcePCAPs         []string
	ObservationSourceID string
	EvidencePersistence *observationsqlite.FrameEvidenceStats
	// GroundSurfaceFit is the P11 ground-plane fit reached during this
	// replay, when Config.UseSurfaceGround was set and the background
	// settled in time to fit one. Nil otherwise.
	GroundSurfaceFit *l3grid.RegionalGroundSurface
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

// strictObservationSink turns the live pipeline's non-fatal persistence hook
// into a replay invariant. A server may continue publishing when an optional
// diagnostic store is briefly unavailable; an offline evidence run must never
// report a successful result after losing or revising a frozen observation.
type strictObservationSink struct {
	sink pipeline.DetectionObservationSink
	mu   sync.Mutex
	err  error
}

// strictFrameEvidenceSink makes the offline frame transaction a replay
// invariant. The pipeline logs optional persistence errors for live operation;
// this wrapper retains the first one so an evidence replay cannot succeed with
// a missing or partial frame.
type strictFrameEvidenceSink struct {
	sink pipeline.FrameEvidenceSink
	mu   sync.Mutex
	err  error
}

func (s *strictFrameEvidenceSink) InsertFrame(observations []l4bobserve.DetectionObservation, estimates []observationsqlite.FrameStateEstimate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	if err := s.sink.InsertFrame(observations, estimates); err != nil {
		s.err = err
		return err
	}
	return nil
}

func (s *strictFrameEvidenceSink) Err() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *strictFrameEvidenceSink) RecordFrameEvidenceFailure(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
}

func (s *strictObservationSink) Insert(observation l4bobserve.DetectionObservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	if err := s.sink.Insert(observation); err != nil {
		s.err = err
		return err
	}
	return nil
}

func (s *strictObservationSink) Err() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
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
	return run(cfg, defaultRuntime())
}

// captureFiles resolves the backwards-compatible single-file form into the
// ordered sequence used by the evaluator. Keeping this small check separate
// makes it impossible for an accidental CLI merge to silently replay the same
// first file twice.
func captureFiles(cfg Config) ([]string, error) {
	if cfg.PCAPFile != "" && len(cfg.PCAPFiles) != 0 {
		return nil, fmt.Errorf("PCAPFile and PCAPFiles are mutually exclusive")
	}
	files := cfg.PCAPFiles
	if len(files) == 0 && cfg.PCAPFile != "" {
		files = []string{cfg.PCAPFile}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("PCAPFile or PCAPFiles is required")
	}
	for _, file := range files {
		if strings.TrimSpace(file) == "" {
			return nil, fmt.Errorf("capture path is required")
		}
	}
	return append([]string(nil), files...), nil
}

// captureSequence probes capture-time bounds and refuses a list whose supplied
// order does not match packet order. Source identity deliberately binds the
// supplied order, so replay must not quietly sort a mistaken list for it.
func captureSequence(files []string, udpPort int) (*capseq.Sequence, error) {
	segments := make([]capseq.Segment, 0, len(files))
	for _, file := range files {
		packets, err := network.CountPCAPPackets(file, udpPort)
		if err != nil {
			return nil, fmt.Errorf("inspect capture %s: %w", file, err)
		}
		if packets.Count == 0 {
			return nil, fmt.Errorf("capture %s has no packets on UDP port %d", file, udpPort)
		}
		segments = append(segments, capseq.Segment{
			Path: file, FirstPacket: time.Unix(0, packets.FirstTimestampNs),
			LastPacket: time.Unix(0, packets.LastTimestampNs), PacketCount: packets.Count,
		})
	}
	sequence, err := capseq.Build(segments, capseq.DefaultTolerances())
	if err != nil {
		return nil, fmt.Errorf("build capture sequence: %w", err)
	}
	for i, segment := range sequence.Segments {
		if segment.Path != files[i] {
			return nil, fmt.Errorf("capture files are not in packet-time order: %q precedes %q", segment.Path, files[i])
		}
	}
	if !sequence.Continuous() {
		broken := sequence.BrokenSeams()[0]
		return nil, fmt.Errorf("capture sequence has an unusable join %s → %s (%s, gap %s)",
			broken.Before, broken.After, broken.Grade, broken.Gap)
	}
	return sequence, nil
}

// PrepareCaptureSequence validates packet-time order and joins once for an
// offline caller that will replay the same captures more than once.
func PrepareCaptureSequence(files []string, udpPort int) (*capseq.Sequence, error) {
	return captureSequence(files, udpPort)
}

func configuredCaptureSequence(cfg Config, files []string) (*capseq.Sequence, error) {
	if cfg.CaptureSequence == nil {
		return captureSequence(files, cfg.UDPPort)
	}
	sequence := cfg.CaptureSequence
	if len(sequence.Segments) != len(files) {
		return nil, fmt.Errorf("CaptureSequence has %d segments for %d capture files", len(sequence.Segments), len(files))
	}
	for i, segment := range sequence.Segments {
		if segment.Path != files[i] {
			return nil, fmt.Errorf("CaptureSequence path %q at position %d does not match capture %q", segment.Path, i, files[i])
		}
	}
	if !sequence.Continuous() {
		return nil, fmt.Errorf("CaptureSequence is not continuous")
	}
	return sequence, nil
}

func rawSHA256(digest string) (string, error) {
	const prefix = "sha256:"
	if !strings.HasPrefix(digest, prefix) || len(digest) != len(prefix)+64 {
		return "", fmt.Errorf("invalid capture digest %q", digest)
	}
	return strings.TrimPrefix(digest, prefix), nil
}

func captureSHA256s(cfg Config, runtime replayRuntime, files []string) ([]string, []string, error) {
	if len(cfg.PCAPSHA256s) != 0 && len(cfg.PCAPSHA256s) != len(files) {
		return nil, nil, fmt.Errorf("PCAPSHA256s has %d digests for %d capture files", len(cfg.PCAPSHA256s), len(files))
	}
	digests := make([]string, len(files))
	raw := make([]string, len(files))
	for i, file := range files {
		var err error
		if len(cfg.PCAPSHA256s) != 0 {
			digests[i] = cfg.PCAPSHA256s[i]
		} else {
			digests[i], err = runtime.hashFile(file)
			if err != nil {
				return nil, nil, fmt.Errorf("hash capture %s: %w", file, err)
			}
		}
		raw[i], err = rawSHA256(digests[i])
		if err != nil {
			return nil, nil, err
		}
	}
	return digests, raw, nil
}

func run(cfg Config, runtime replayRuntime) (*Result, error) {
	start := time.Now()

	pcapFiles, err := captureFiles(cfg)
	if err != nil {
		return nil, err
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
	if cfg.MeasurementSourceMode != "" && cfg.MeasurementSourceMode != l5tracks.MeasurementMedoidV0 && cfg.MeasurementSourceMode != l5tracks.MeasurementOBBCentreV1 {
		return nil, fmt.Errorf("unsupported measurement source mode %q", cfg.MeasurementSourceMode)
	}
	if cfg.TuningFile == "" {
		cfg.TuningFile = config.DefaultConfigPath
	}

	tuningCfg, err := config.LoadTuningConfigOrEmbedded(cfg.TuningFile, radarassets.TuningDefaults)
	if err != nil {
		return nil, fmt.Errorf("load tuning config %s: %w", cfg.TuningFile, err)
	}
	sequence, err := configuredCaptureSequence(cfg, pcapFiles)
	if err != nil {
		return nil, err
	}
	firstTimestampNs := sequence.Start.UnixNano()
	lastTimestampNs := sequence.End.UnixNano()
	scoreStart := firstTimestampNs + int64(cfg.StartSeconds*1e9)
	if scoreStart < firstTimestampNs || scoreStart > lastTimestampNs {
		return nil, fmt.Errorf("scoring window starts outside capture")
	}
	pcapHashes, rawHashes, err := captureSHA256s(cfg, runtime, pcapFiles)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("create out dir %s: %w", cfg.OutDir, err)
	}

	// --- L1: parser ---
	parserCfg, err := runtime.loadParser()
	if err != nil {
		return nil, fmt.Errorf("load parser config: %w", err)
	}
	parser := parse.NewPandar40PParser(*parserCfg)
	elevations := parse.ElevationsFromConfig(parserCfg)

	// --- L3: background model ---
	bgMgr, err := runtime.background(cfg.SensorID, tuningCfg, elevations)
	if err != nil {
		return nil, err
	}
	bgMgr.SetSourcePath(strings.Join(pcapFiles, "\n"))

	// --- L5, L6 ---
	trackerConfig := l5tracks.TrackerConfigFromTuning(tuningCfg.L5.CvKfV1)
	trackerConfig.MeasurementSourceMode = cfg.MeasurementSourceMode
	tracker := l5tracks.NewTracker(trackerConfig)
	classifier := l6objects.NewTrackClassifierWithMinObservations(
		tuningCfg.GetMinObservationsForClassification())

	// --- Recorder + publisher ---
	rec, err := runtime.newRecorder(cfg.OutDir, cfg.SensorID)
	if err != nil {
		return nil, fmt.Errorf("create recorder: %w", err)
	}
	// Close also on provenance failures before frame processing begins. Recorder
	// Close is idempotent; the explicit close below still reports finalisation errors.
	defer rec.Close()
	pub := &recordingPublisher{rec: rec, dropPoints: !cfg.IncludePoints, recordAfterNanos: scoreStart}
	adapter := l9endpoints.NewFrameAdapter(cfg.SensorID)

	// Provenance. Without it a recording cannot say which code and which
	// parameters produced it, which is the one thing an A/B corpus has to be
	// able to answer. The live path takes these from the run-config store in
	// the database; there is no database here, so they are derived from the
	// tuning file actually loaded and from the build stamp.
	paramsJSON, err := runtime.marshal(tuningCfg)
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
	rec.SetProvenance("pcap", filepath.Base(pcapFiles[0]), paramsHash, 0)

	var frameEvidenceSink *strictFrameEvidenceSink
	var frameEvidenceStore *observationsqlite.FrameEvidenceStore
	var observationSourceID, observationCalibrationID string
	maxSamplePoints := tuningCfg.L4.ActiveCommon().MaxSamplePoints
	if cfg.ObservationDBPath != "" {
		if strings.TrimSpace(cfg.ReplayCaseID) == "" {
			return nil, fmt.Errorf("ReplayCaseID is required when ObservationDBPath is set")
		}
		if cfg.ObservationCalibration.SensorID != cfg.SensorID {
			return nil, fmt.Errorf("observation calibration sensor %q does not match replay sensor %q", cfg.ObservationCalibration.SensorID, cfg.SensorID)
		}
		if cfg.ObservationMaxSamplePoints <= 0 || cfg.ObservationMaxSamplePoints > 1024 {
			return nil, fmt.Errorf("ObservationMaxSamplePoints must be between 1 and 1024 when ObservationDBPath is set")
		}
		maxSamplePoints = cfg.ObservationMaxSamplePoints
		observationSourceID, err = l4bobserve.SourceID(l4bobserve.CaptureSource{
			ReplayCaseID: cfg.ReplayCaseID, CapturePaths: pcapFiles, CaptureSHA256s: rawHashes,
			ExtractorID: "l4.dbscan_xy/v1/" + paramsHash,
		})
		if err != nil {
			return nil, fmt.Errorf("derive observation source identity: %w", err)
		}
		observationCalibrationID, err = l4bobserve.CalibrationID(cfg.ObservationCalibration)
		if err != nil {
			return nil, fmt.Errorf("derive observation calibration identity: %w", err)
		}
		database, err := db.NewDB(cfg.ObservationDBPath)
		if err != nil {
			return nil, fmt.Errorf("open observation database: %w", err)
		}
		defer database.Close()
		// The offline database owns one transaction per completed frame. It
		// persists both pre-association L4 evidence and the L5 records derived
		// from it; a failed frame therefore cannot leave a partial corpus.
		frameEvidenceStore = observationsqlite.NewFrameEvidenceStore(database)
		defer frameEvidenceStore.Close()
		frameEvidenceSink = &strictFrameEvidenceSink{sink: frameEvidenceStore}
	}

	// --- Pipeline ---
	// The frame-rate throttle is left off. It exists to stop a real-time
	// replay flooding a live gRPC client, and dropping frames here would make
	// two runs of the same capture disagree for reasons unrelated to the
	// change under test. Determinism matters more than throughput offline.
	disablePersistence := &atomic.Bool{}
	disablePersistence.Store(true)
	stateObservationModelID := string(l5tracks.MeasurementOBBCentreV1)
	if cfg.MeasurementSourceMode == l5tracks.MeasurementMedoidV0 {
		stateObservationModelID = string(l5tracks.MeasurementMedoidV0)
	}

	pipeCfg := &pipeline.TrackingPipelineConfig{
		BackgroundManager:         bgMgr,
		Tracker:                   tracker,
		Classifier:                classifier,
		SensorID:                  cfg.SensorID,
		VisualiserPublisher:       pub,
		VisualiserAdapter:         adapter,
		DisableTrackPersistence:   disablePersistence,
		HeightBandFloor:           tuningCfg.GetHeightBandFloor(),
		HeightBandCeiling:         tuningCfg.GetHeightBandCeiling(),
		RemoveGround:              tuningCfg.GetRemoveGround(),
		UseSurfaceGround:          cfg.UseSurfaceGround,
		SurfaceGroundFloor:        cfg.SurfaceGroundFloor,
		SurfaceGroundCeiling:      cfg.SurfaceGroundCeiling,
		SurfaceGroundRegionMetres: cfg.SurfaceGroundRegionMetres,
		MaxSamplePoints:           maxSamplePoints,
		ObservationSourceID:       observationSourceID,
		ObservationCalibrationID:  observationCalibrationID,
		StateEstimatorID:          "cv_kf_v1",
		StateObservationModelID:   stateObservationModelID,
		StateParameterHash:        paramsHash,
	}
	if frameEvidenceSink != nil {
		pipeCfg.FrameEvidenceSink = frameEvidenceSink
	}
	var groundSurfaceFit atomic.Pointer[l3grid.RegionalGroundSurface]
	if cfg.UseSurfaceGround {
		pipeCfg.GroundSurfaceFit = &groundSurfaceFit
	}
	if cfg.IncludeDebug {
		collector := debug.NewDebugCollector()
		collector.SetEnabled(true)
		tracker.DebugCollector = collector
		pipeCfg.DebugCollector = collector
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
			tracker.BeginTrackingBaseline()
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

	fb := l2frames.NewFrameBuilder(offlineFrameBuilderConfig(cfg.SensorID, frameCallback))

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

	log.Printf("replaying %d capture file(s) (port %d) through the perception pipeline", len(pcapFiles), cfg.UDPPort)
	processingDuration := cfg.DurationSeconds
	if processingDuration > 0 {
		processingDuration += cfg.WarmupSeconds
	}
	steps, err := sequence.Plan(cfg.StartSeconds-cfg.WarmupSeconds, processingDuration)
	if err != nil {
		return nil, fmt.Errorf("plan replay window: %w", err)
	}
	_, replayErr := network.ReadPCAPSequence(context.Background(), steps, network.SequenceReplayConfig{
		UDPPort: cfg.UDPPort, Parser: parser, FrameBuilder: fb,
	})

	// Drain before closing the recorder, or the tail of the capture is lost.
	fb.Close()

	if cerr := runtime.closeRecorder(rec); cerr != nil && replayErr == nil {
		replayErr = fmt.Errorf("close recording: %w", cerr)
	}
	if frameEvidenceErr := frameEvidenceSink.Err(); frameEvidenceErr != nil && replayErr == nil {
		replayErr = fmt.Errorf("store frame evidence: %w", frameEvidenceErr)
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
	parserJSON, err := runtime.marshal(parserCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal calibration: %w", err)
	}
	manifest := map[string]interface{}{
		// source_sha256/source_basename are retained for single-file consumers;
		// the plural fields carry the complete ordered multi-file provenance.
		"schema_version": 2, "source_sha256": pcapHashes[0], "source_basename": filepath.Base(pcapFiles[0]),
		"source_sha256s": pcapHashes, "source_paths": pcapFiles,
		"source_first_ns": firstTimestampNs, "source_last_ns": lastTimestampNs,
		"processing_start_seconds": cfg.StartSeconds - cfg.WarmupSeconds,
		"scoring_start_seconds":    cfg.StartSeconds, "scoring_start_ns": scoreStart,
		"scoring_duration_seconds": cfg.DurationSeconds, "warmup_seconds": cfg.WarmupSeconds,
		"tracker_boundary_policy": "retain", "settled_at_boundary": settledAtBoundary,
		"require_settled": cfg.RequireSettled, "sensor_id": cfg.SensorID, "udp_port": cfg.UDPPort,
		"include_points": cfg.IncludePoints, "include_debug": cfg.IncludeDebug, "params_sha256": paramsHash,
		"calibration_sha256": "sha256:" + hex.EncodeToString(sha256Sum(parserJSON)),
		"build_version":      version.Version, "build_git_sha": version.GitSHA,
		"build_stamped":    version.GitSHA != "" && version.GitSHA != "unknown" && version.GitSHA != "dev",
		"frames_processed": frameCount, "frames_recorded": pub.recorded, "warmup_frames": pub.warmupFrames,
		"warmup_static_verified":  false,
		"measurement_source_mode": stateObservationModelID,
	}
	if observationSourceID != "" {
		manifest["observation_source_id"] = observationSourceID
		manifest["observation_calibration_id"] = observationCalibrationID
		manifest["observation_max_sample_points"] = maxSamplePoints
	}
	manifestJSON, err := runtime.marshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal replay manifest: %w", err)
	}
	if err := runtime.writeFile(filepath.Join(cfg.OutDir, "replay_manifest.json"), append(manifestJSON, '\n'), 0644); err != nil {
		return nil, fmt.Errorf("write replay manifest: %w", err)
	}

	// Phase 0 scoring-window aggregates include empty frames and ended tracks.
	// Debug VRLOGs also carry individual innovations, but not these banded NIS
	// and association summaries. Keep the population declaration beside the run.
	if err := runtime.writeBaseline(cfg.OutDir, tracker.GetWindowBaseline()); err != nil {
		return nil, err
	}

	return &Result{
		VRLOGPath:           filepath.Clean(cfg.OutDir),
		FramesRead:          frameCount,
		FramesEmpty:         pub.emptyFrames,
		FramesRecorded:      pub.recorded,
		WarmupFrames:        pub.warmupFrames,
		Elapsed:             time.Since(start),
		TuningFile:          cfg.TuningFile,
		SensorID:            cfg.SensorID,
		SourcePCAP:          pcapFiles[0],
		SourcePCAPs:         append([]string(nil), pcapFiles...),
		ObservationSourceID: observationSourceID,
		EvidencePersistence: func() *observationsqlite.FrameEvidenceStats {
			if !cfg.ProfileEvidence || frameEvidenceStore == nil {
				return nil
			}
			stats := frameEvidenceStore.Stats()
			return &stats
		}(),
		GroundSurfaceFit: groundSurfaceFit.Load(),
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
	SchemaVersion         int                               `json:"schema_version"`
	Population            string                            `json:"population"`
	NISSelection          string                            `json:"nis_selection"`
	AssociationPopulation string                            `json:"association_population"`
	Residuals             []l5tracks.ResidualBandSummary    `json:"residual_bands"`
	Association           []l5tracks.AssociationBandSummary `json:"association_bands"`
}

// baselineMetricPrecision is the published precision of the Phase 0
// filter-consistency report. The tracker uses float32 state and may visit
// otherwise identical accepted observations in a different scheduling order;
// retaining binary float64 accumulator tails would turn that harmless detail
// into a false failed repeat. Six decimal places is sub-micrometre precision,
// matches the tuning-document precision, and still leaves the baseline files
// compared byte-for-byte.
const baselineMetricPrecision = 1e6

func roundBaselineMetric(v float64) float64 {
	return math.Round(v*baselineMetricPrecision) / baselineMetricPrecision
}

func canonicalTrackingBaseline(m l5tracks.TrackingMetrics) l5tracks.TrackingMetrics {
	canonical := m
	canonical.Residuals = append([]l5tracks.ResidualBandSummary(nil), m.Residuals...)
	for i := range canonical.Residuals {
		r := &canonical.Residuals[i]
		r.LateralRMSMetres = roundBaselineMetric(r.LateralRMSMetres)
		r.LongitudinalRMSMetres = roundBaselineMetric(r.LongitudinalRMSMetres)
		r.LateralBiasMetres = roundBaselineMetric(r.LateralBiasMetres)
		r.LongitudinalBias = roundBaselineMetric(r.LongitudinalBias)
		r.MeanNIS = roundBaselineMetric(r.MeanNIS)
		r.NISExceedanceRatio = roundBaselineMetric(r.NISExceedanceRatio)
	}
	canonical.Association = append([]l5tracks.AssociationBandSummary(nil), m.Association...)
	for i := range canonical.Association {
		canonical.Association[i].Rate = roundBaselineMetric(canonical.Association[i].Rate)
	}
	return canonical
}

// writeTrackingBaseline records the residual and association bands beside the
// recording.
//
// It includes the scoring window only, retaining contributions after tracks
// die. Schema 1 pooled surviving tracks' lifetimes, including warm-up, and is
// not population-compatible with this baseline.
func writeTrackingBaseline(outDir string, m l5tracks.TrackingMetrics) error {
	m = canonicalTrackingBaseline(m)
	b, err := json.MarshalIndent(TrackingBaseline{
		SchemaVersion:         2,
		Population:            "scoring_window_including_terminated_tracks",
		NISSelection:          "accepted_associations_only",
		AssociationPopulation: "preexisting_active_tracks_including_empty_frames_and_terminal_misses",
		Residuals:             m.Residuals,
		Association:           m.Association,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tracking baseline: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "tracking_baseline.json"), append(b, '\n'), 0644); err != nil {
		return fmt.Errorf("write tracking baseline: %w", err)
	}
	return nil
}
