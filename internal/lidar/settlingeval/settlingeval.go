// Package settlingeval evaluates LiDAR background-grid settling convergence by
// replaying a captured PCAP offline through a local BackgroundManager at full
// speed. It backs both the standalone settling-eval tool and the
// `velocity lidar settling-eval` subcommand.
package settlingeval

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// Config holds the parameters for an offline settling evaluation. A zero
// DurationSeconds preserves the historical full-capture behaviour.
type Config struct {
	PCAPFile        string
	TuningFile      string
	SensorID        string
	UDPPort         int
	StartSeconds    float64
	DurationSeconds float64
}

// Run replays a PCAP file offline through a local BackgroundManager and
// evaluates settling convergence on every frame. No server is required.
func Run(cfg Config) (*l3grid.SettlingReport, error) {
	start := time.Now()
	if cfg.DurationSeconds == 0 {
		cfg.DurationSeconds = -1
	}

	// --- Load tuning configuration ---
	// Load tuning from the path if present, else the binary-embedded defaults,
	// exactly as the live pipeline and the other pcap-* tools do.
	if cfg.TuningFile == "" {
		cfg.TuningFile = config.DefaultConfigPath
	}
	tuningCfg, err := config.LoadTuningConfigOrEmbedded(cfg.TuningFile, radarassets.TuningDefaults)
	if err != nil {
		return nil, fmt.Errorf("load tuning config %s: %w", cfg.TuningFile, err)
	}
	log.Printf("loaded tuning config (config=%s)", cfg.TuningFile)

	bgConfig := backgroundConfigFromTuningConfig(tuningCfg)
	// For offline evaluation disable warmup gating so we can observe the
	// full settling curve from frame 0. Set high settling period and
	// warmup to not truncate the observation window.
	bgConfig.WarmupMinFrames = 0
	bgConfig.WarmupDuration = 0
	bgConfig.SettlingPeriod = 24 * time.Hour // effectively infinite for offline
	if err := bgConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid background config: %w", err)
	}
	params := bgConfig.ToBackgroundParams()

	// --- Create parser ---
	parserCfg, err := parse.LoadPandar40PConfig()
	if err != nil {
		return nil, fmt.Errorf("load parser config: %w", err)
	}
	parser := parse.NewPandar40PParser(*parserCfg)
	elevations := parse.ElevationsFromConfig(parserCfg)

	// --- Create BackgroundManager (DI, no global registration) ---
	const rings = 40
	const azBins = 1800
	bgMgr := l3grid.NewBackgroundManagerDI(cfg.SensorID, rings, azBins, params, nil)
	if bgMgr == nil {
		return nil, fmt.Errorf("failed to create BackgroundManager")
	}
	if err := bgMgr.SetRingElevations(elevations); err != nil {
		return nil, fmt.Errorf("set ring elevations: %w", err)
	}
	bgMgr.SetSourcePath(cfg.PCAPFile)

	// --- Convergence tracking state ---
	thresholds := settlingThresholdsFromTuning(tuningCfg)
	var (
		mu               sync.Mutex
		history          []l3grid.SettlingMetrics
		frameCount       int
		recommendedFrame = -1
	)

	// --- Frame callback: update background grid and evaluate settling ---
	frameCallback := func(frame *l2frames.LiDARFrame) {
		if frame == nil || len(frame.Points) == 0 {
			return
		}

		// Use the frame-owned polar representation directly.
		bgMgr.ProcessFramePolar(frame.PolarPoints)

		mu.Lock()
		frameCount++
		fn := frameCount
		mu.Unlock()

		// Evaluate settling metrics.
		metrics := bgMgr.EvaluateSettling(fn)
		converged := metrics.IsConverged(thresholds)

		mu.Lock()
		history = append(history, metrics)
		if converged && recommendedFrame < 0 {
			recommendedFrame = fn
			log.Printf("✓ convergence detected at frame %d", fn)
		}
		mu.Unlock()

		// Log progress every 50 frames.
		if fn%50 == 0 {
			log.Printf("frame=%d coverage=%.3f spread_delta=%.6f region_stability=%.3f confidence=%.1f converged=%v",
				fn, metrics.CoverageRate, metrics.SpreadDeltaRate,
				metrics.RegionStability, metrics.MeanConfidence, converged)
		}
	}

	// --- Create FrameBuilder ---
	fb := l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{
		SensorID:        cfg.SensorID,
		FrameCallback:   frameCallback,
		FrameChCapacity: 32,
	})
	closedFB := false
	defer func() {
		if !closedFB {
			fb.Close()
		}
	}()

	// --- Replay PCAP at full speed ---
	log.Printf("replaying %s (port %d) ...", cfg.PCAPFile, cfg.UDPPort)
	err = network.ReadPCAPFile(
		context.Background(),
		cfg.PCAPFile,
		cfg.UDPPort,
		parser,
		fb,
		nil, // no packet stats
		nil, // no packet forwarder
		cfg.StartSeconds,
		cfg.DurationSeconds,
		0,   // packetOffset
		0,   // totalPackets (unknown)
		nil, // onProgress
	)
	if err != nil {
		return nil, fmt.Errorf("pcap replay: %w", err)
	}

	// Drain the FrameBuilder's callback channel so all frames are processed.
	fb.Close()
	closedFB = true

	mu.Lock()
	defer mu.Unlock()

	wallDur := time.Since(start)
	log.Printf("replay complete: %d frames in %v", frameCount, wallDur.Round(time.Millisecond))

	// Build the thresholds struct for the report.
	reportThresholds := l3grid.SettlingThresholds{
		MinCoverage:        thresholds.MinCoverage,
		MaxSpreadDelta:     thresholds.MaxSpreadDelta,
		MinRegionStability: thresholds.MinRegionStability,
		MinConfidence:      thresholds.MinConfidence,
	}

	rationale := l3grid.BuildRationale(history, recommendedFrame, reportThresholds)

	return &l3grid.SettlingReport{
		PCAPFile:            cfg.PCAPFile,
		TuningFile:          cfg.TuningFile,
		SensorID:            cfg.SensorID,
		TotalSamples:        len(history),
		TotalFrames:         frameCount,
		MetricsHistory:      history,
		RecommendedFrame:    recommendedFrame,
		RecommendedDuration: l3grid.FormatRecommendedDuration(recommendedFrame),
		Thresholds:          reportThresholds,
		Rationale:           rationale,
		WallDuration:        wallDur.Round(time.Millisecond).String(),
	}, nil
}

func settlingThresholdsFromTuning(tuningCfg *config.TuningConfig) l3grid.SettlingThresholds {
	return l3grid.SettlingThresholds{
		MinCoverage:        tuningCfg.GetSettlingMinCoverage(),
		MaxSpreadDelta:     tuningCfg.GetSettlingMaxSpreadDelta(),
		MinRegionStability: tuningCfg.GetSettlingMinRegionStability(),
		MinConfidence:      tuningCfg.GetSettlingMinConfidence(),
	}
}

func backgroundConfigFromTuningConfig(tuningCfg *config.TuningConfig) *l3grid.BackgroundConfig {
	return l3grid.BackgroundConfigFromActiveTuning(tuningCfg)
}
