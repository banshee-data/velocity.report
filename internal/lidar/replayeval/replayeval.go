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
	"fmt"
	"log"
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
)

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
	// IncludePoints records the point cloud in the VRLOG. This dominates the
	// output size, so it is off unless the recording is meant for the
	// visualiser rather than for metrics.
	IncludePoints bool
	// ProgressEvery logs a line every N frames. Zero disables progress logs.
	ProgressEvery int
}

// Result summarises a completed replay.
type Result struct {
	VRLOGPath   string
	FramesRead  int
	FramesEmpty int
	Elapsed     time.Duration
	TuningFile  string
	SensorID    string
	SourcePCAP  string
}

// recordingPublisher writes each adapted FrameBundle straight to a recorder.
// It stands in for the gRPC publisher, which is the only reason the pipeline
// normally needs a server to produce a VRLOG.
type recordingPublisher struct {
	rec         *recorder.Recorder
	mu          sync.Mutex
	recorded    int
	writeErr    error
	dropPoints  bool
	emptyFrames int
}

func (p *recordingPublisher) Publish(frame interface{}) {
	bundle, ok := frame.(*l9endpoints.FrameBundle)
	if !ok || bundle == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
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
	pub := &recordingPublisher{rec: rec, dropPoints: !cfg.IncludePoints}
	adapter := l9endpoints.NewFrameAdapter(cfg.SensorID)

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
	frameCallback := func(frame *l2frames.LiDARFrame) {
		if frame == nil {
			return
		}
		frameCount++
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
	replayErr := network.ReadPCAPFile(
		context.Background(),
		cfg.PCAPFile,
		cfg.UDPPort,
		parser,
		fb,
		nil, nil,
		cfg.StartSeconds,
		cfg.DurationSeconds,
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

	return &Result{
		VRLOGPath:   filepath.Clean(cfg.OutDir),
		FramesRead:  frameCount,
		FramesEmpty: pub.emptyFrames,
		Elapsed:     time.Since(start),
		TuningFile:  cfg.TuningFile,
		SensorID:    cfg.SensorID,
		SourcePCAP:  cfg.PCAPFile,
	}, nil
}
