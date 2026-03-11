package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// runPCAPEval replays a PCAP file offline through a local BackgroundManager
// and evaluates settling convergence on every frame. No server is required.
func runPCAPEval(pcapFile, tuningFile, sensorID string, udpPort int) (*l3grid.SettlingReport, error) {
	start := time.Now()

	// --- Load tuning configuration ---
	var tuningCfg *config.TuningConfig
	var err error
	if tuningFile != "" {
		tuningCfg, err = config.LoadTuningConfig(tuningFile)
		if err != nil {
			return nil, fmt.Errorf("load tuning config %s: %w", tuningFile, err)
		}
		log.Printf("loaded tuning config from %s", tuningFile)
	} else {
		tuningCfg = config.MustLoadDefaultConfig()
		tuningFile = "config/tuning.defaults.json"
		log.Printf("using default tuning config")
	}

	bgConfig := l3grid.BackgroundConfigFromTuning(tuningCfg)
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
	bgMgr := l3grid.NewBackgroundManagerDI(sensorID, rings, azBins, params, nil)
	if bgMgr == nil {
		return nil, fmt.Errorf("failed to create BackgroundManager")
	}
	if err := bgMgr.SetRingElevations(elevations); err != nil {
		return nil, fmt.Errorf("set ring elevations: %w", err)
	}
	bgMgr.SetSourcePath(pcapFile)

	// --- Convergence tracking state ---
	thresholds := l3grid.DefaultSettlingThresholds()
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
		SensorID:        sensorID,
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
	log.Printf("replaying %s (port %d) ...", pcapFile, udpPort)
	err = network.ReadPCAPFile(
		context.Background(),
		pcapFile,
		udpPort,
		parser,
		fb,
		nil, // no packet stats
		nil, // no packet forwarder
		0,   // startSeconds
		-1,  // durationSeconds (full file)
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
		PCAPFile:            pcapFile,
		TuningFile:          tuningFile,
		SensorID:            sensorID,
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
