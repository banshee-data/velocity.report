//go:build pcap
// +build pcap

// Package main provides an algorithm comparison tool for LIDAR foreground extraction.
// It processes PCAP files through multiple extraction algorithms and compares their results.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/network"
	"github.com/banshee-data/velocity.report/internal/lidar/parse"
	_ "modernc.org/sqlite"
)

// Config holds configuration for the algorithm comparison.
type Config struct {
	PCAPFile   string
	OutputDir  string
	SensorID   string
	UDPPort    int
	Verbose    bool
	MergeMode  string
	OutputJSON string
}

// ComparisonResult holds the results of algorithm comparison.
type ComparisonResult struct {
	PCAPFile          string                  `json:"pcap_file"`
	Duration          time.Duration           `json:"duration_ns"`
	DurationSecs      float64                 `json:"duration_secs"`
	TotalFrames       int                     `json:"total_frames"`
	TotalPoints       int64                   `json:"total_points"`
	MergeMode         string                  `json:"merge_mode"`
	ProcessingTimeMs  int64                   `json:"processing_time_ms"`
	PerAlgorithm      map[string]AlgoStats    `json:"per_algorithm"`
	AgreementStats    AgreementStats          `json:"agreement_stats"`
	RecentComparisons []lidar.FrameComparison `json:"recent_comparisons,omitempty"`
}

// AlgoStats holds per-algorithm statistics.
type AlgoStats struct {
	Name              string  `json:"name"`
	TotalForeground   int64   `json:"total_foreground"`
	TotalBackground   int64   `json:"total_background"`
	ForegroundRatio   float64 `json:"foreground_ratio"`
	AvgProcessingUs   float64 `json:"avg_processing_us"`
	TotalProcessingUs int64   `json:"total_processing_us"`
}

// AgreementStats holds agreement statistics between algorithms.
type AgreementStats struct {
	TotalComparisons int     `json:"total_comparisons"`
	AvgAgreementPct  float64 `json:"avg_agreement_pct"`
	MinAgreementPct  float64 `json:"min_agreement_pct"`
	MaxAgreementPct  float64 `json:"max_agreement_pct"`
}

func main() {
	cfg := parseFlags()

	if cfg.PCAPFile == "" {
		log.Fatal("PCAP file is required")
	}

	if _, err := os.Stat(cfg.PCAPFile); os.IsNotExist(err) {
		log.Fatalf("PCAP file not found: %s", cfg.PCAPFile)
	}

	// Create output directory if needed
	if cfg.OutputDir != "" {
		if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}

	// Run comparison
	result, err := runComparison(cfg)
	if err != nil {
		log.Fatalf("Comparison failed: %v", err)
	}

	// Output results
	printResults(result)

	// Export JSON if requested
	if cfg.OutputJSON != "" {
		outputPath := cfg.OutputJSON
		if cfg.OutputDir != "" {
			outputPath = filepath.Join(cfg.OutputDir, cfg.OutputJSON)
		}
		if err := exportJSON(result, outputPath); err != nil {
			log.Printf("Warning: failed to export JSON: %v", err)
		} else {
			log.Printf("Results exported to: %s", outputPath)
		}
	}
}

func parseFlags() Config {
	cfg := Config{}

	flag.StringVar(&cfg.PCAPFile, "pcap", "", "Path to PCAP file to analyze")
	flag.StringVar(&cfg.OutputDir, "output", "", "Output directory for results")
	flag.StringVar(&cfg.SensorID, "sensor", "lidar-01", "Sensor ID")
	flag.IntVar(&cfg.UDPPort, "port", 2368, "UDP port to filter")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enable verbose logging")
	flag.StringVar(&cfg.MergeMode, "merge", "union", "Merge mode: union, intersection, primary")
	flag.StringVar(&cfg.OutputJSON, "json", "", "Output JSON filename (e.g., results.json)")

	flag.Parse()

	return cfg
}

func runComparison(cfg Config) (*ComparisonResult, error) {
	log.Printf("Starting algorithm comparison on: %s", cfg.PCAPFile)

	// Create background manager for background subtraction
	bgParams := lidar.DefaultBackgroundParams()
	bgParams.SeedFromFirstObservation = true
	bgManager := lidar.NewBackgroundManager(&bgParams)

	// Create extractors
	bsExtractor := lidar.NewBackgroundSubtractorExtractor(bgManager, cfg.SensorID)
	vcExtractor := lidar.NewVelocityCoherentExtractor(
		lidar.DefaultVelocityCoherentConfig(),
		cfg.SensorID,
	)

	// Create evaluation harness
	harness := lidar.NewEvaluationHarness(
		lidar.EvaluationHarnessConfig{
			LogComparisons: true,
		},
		[]lidar.ForegroundExtractor{bsExtractor, vcExtractor},
	)

	// Create hybrid extractor for merged output
	mergeMode := lidar.MergeMode(cfg.MergeMode)
	hybrid := lidar.NewHybridExtractor(
		lidar.HybridExtractorConfig{
			MergeMode:               mergeMode,
			PrimaryExtractor:        "background_subtraction",
			EnableMetricsComparison: true,
		},
		[]lidar.ForegroundExtractor{bsExtractor, vcExtractor},
		cfg.SensorID,
	)

	// Create parser and frame builder
	parser := parse.NewPandar40PParser()
	frameBuilder := lidar.NewFrameBuilder(lidar.FrameBuilderConfig{
		MaxPoints:        100000,
		FrameTimeout:     500 * time.Millisecond,
		AzimuthThreshold: 5.0,
	})

	// Stats tracking
	stats := lidar.NewPacketStats()
	startTime := time.Now()
	totalPoints := int64(0)
	frameCount := 0

	// Set up frame callback
	frameBuilder.SetOnFrameComplete(func(frame *lidar.LiDARFrame) {
		if frame == nil || len(frame.Points) == 0 {
			return
		}

		frameCount++
		totalPoints += int64(len(frame.Points))

		// Convert to polar
		polar := make([]lidar.PointPolar, len(frame.Points))
		for i, p := range frame.Points {
			polar[i] = lidar.PointPolar{
				Channel:   p.Channel,
				Azimuth:   p.Azimuth,
				Elevation: p.Elevation,
				Distance:  p.Distance,
				Intensity: p.Intensity,
				Timestamp: p.Timestamp.UnixNano(),
			}
		}

		// Run comparison through harness
		harness.ProcessFrame(polar, frame.StartTimestamp)

		// Also run through hybrid for merged stats
		_, _, _ = hybrid.ProcessFrame(polar, frame.StartTimestamp)

		if cfg.Verbose && frameCount%100 == 0 {
			log.Printf("Processed %d frames, %d points", frameCount, totalPoints)
		}
	})

	// Process PCAP file
	ctx := context.Background()
	replayConfig := network.RealtimeReplayConfig{
		SpeedMultiplier: 0, // As fast as possible
	}

	if err := network.ReadPCAPFileRealtime(
		ctx,
		cfg.PCAPFile,
		cfg.UDPPort,
		parser,
		frameBuilder,
		stats,
		replayConfig,
	); err != nil {
		return nil, fmt.Errorf("PCAP replay failed: %w", err)
	}

	processingTime := time.Since(startTime)

	// Build results
	summary := harness.GetSummary()
	comparisons := harness.GetRecentComparisons(100) // Last 100 comparisons

	// Compute per-algorithm stats
	perAlgo := make(map[string]AlgoStats)
	harnessStats := harness.GetStats()
	for name, s := range harnessStats {
		total := s.TotalForeground + s.TotalBackground
		ratio := 0.0
		if total > 0 {
			ratio = float64(s.TotalForeground) / float64(total)
		}
		avgUs := 0.0
		if s.FrameCount > 0 {
			avgUs = float64(s.TotalProcessingUs) / float64(s.FrameCount)
		}
		perAlgo[name] = AlgoStats{
			Name:              name,
			TotalForeground:   s.TotalForeground,
			TotalBackground:   s.TotalBackground,
			ForegroundRatio:   ratio,
			AvgProcessingUs:   avgUs,
			TotalProcessingUs: s.TotalProcessingUs,
		}
	}

	// Compute agreement stats
	agreementStats := computeAgreementStats(comparisons)

	result := &ComparisonResult{
		PCAPFile:         cfg.PCAPFile,
		Duration:         processingTime,
		DurationSecs:     processingTime.Seconds(),
		TotalFrames:      frameCount,
		TotalPoints:      totalPoints,
		MergeMode:        cfg.MergeMode,
		ProcessingTimeMs: processingTime.Milliseconds(),
		PerAlgorithm:     perAlgo,
		AgreementStats:   agreementStats,
	}

	// Add recent comparisons if verbose
	if cfg.Verbose && len(comparisons) > 0 {
		result.RecentComparisons = make([]lidar.FrameComparison, len(comparisons))
		for i, c := range comparisons {
			result.RecentComparisons[i] = *c
		}
	}

	log.Printf("Summary: %+v", summary)

	return result, nil
}

func computeAgreementStats(comparisons []*lidar.FrameComparison) AgreementStats {
	if len(comparisons) == 0 {
		return AgreementStats{}
	}

	var total, min, max float64
	min = 100.0
	max = 0.0

	for _, c := range comparisons {
		total += c.AgreementPct
		if c.AgreementPct < min {
			min = c.AgreementPct
		}
		if c.AgreementPct > max {
			max = c.AgreementPct
		}
	}

	return AgreementStats{
		TotalComparisons: len(comparisons),
		AvgAgreementPct:  total / float64(len(comparisons)),
		MinAgreementPct:  min,
		MaxAgreementPct:  max,
	}
}

func printResults(result *ComparisonResult) {
	fmt.Println("\n=== Algorithm Comparison Results ===")
	fmt.Printf("PCAP File: %s\n", result.PCAPFile)
	fmt.Printf("Processing Time: %.2fs\n", result.DurationSecs)
	fmt.Printf("Total Frames: %d\n", result.TotalFrames)
	fmt.Printf("Total Points: %d\n", result.TotalPoints)
	fmt.Printf("Merge Mode: %s\n", result.MergeMode)

	fmt.Println("\n--- Per-Algorithm Statistics ---")
	for name, stats := range result.PerAlgorithm {
		fmt.Printf("\n%s:\n", name)
		fmt.Printf("  Foreground: %d (%.2f%%)\n", stats.TotalForeground, stats.ForegroundRatio*100)
		fmt.Printf("  Background: %d\n", stats.TotalBackground)
		fmt.Printf("  Avg Processing: %.2f µs\n", stats.AvgProcessingUs)
	}

	fmt.Println("\n--- Agreement Statistics ---")
	fmt.Printf("Comparisons: %d\n", result.AgreementStats.TotalComparisons)
	fmt.Printf("Average Agreement: %.2f%%\n", result.AgreementStats.AvgAgreementPct)
	fmt.Printf("Min Agreement: %.2f%%\n", result.AgreementStats.MinAgreementPct)
	fmt.Printf("Max Agreement: %.2f%%\n", result.AgreementStats.MaxAgreementPct)
}

func exportJSON(result *ComparisonResult, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
