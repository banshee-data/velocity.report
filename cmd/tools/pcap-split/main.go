//go:build pcap
// +build pcap

// Command pcap-split segments a LiDAR PCAP capture into non-overlapping motion
// and static periods, writing one PCAP file per segment.
//
// It runs in two passes: pass 1 replays the capture through a BackgroundManager
// and classifies each frame as moving or stable; pass 2 re-reads the capture
// and copies each packet, unmodified, into its segment's output file based on
// the packet's capture timestamp. See internal/lidar/pcapsplit for the core.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/pcapsplit"
)

func main() {
	cfg := parseFlags()
	if cfg.PCAPFile == "" {
		fmt.Fprintln(os.Stderr, "error: --pcap is required")
		flag.Usage()
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		log.Fatalf("pcap-split: %v", err)
	}
}

func parseFlags() pcapsplit.SplitConfig {
	cfg := pcapsplit.DefaultSplitConfig()

	flag.StringVar(&cfg.PCAPFile, "pcap", "", "Input PCAP/PCAPNG file (required)")
	flag.StringVar(&cfg.OutputDir, "output", ".", "Output directory for segments and metadata")
	flag.StringVar(&cfg.OutputPrefix, "prefix", cfg.OutputPrefix, "Output filename prefix")
	flag.Float64Var(&cfg.SettlingSec, "settling-sec", cfg.SettlingSec, "Sustained stability (s) required to declare static")
	flag.Float64Var(&cfg.MotionTriggerSec, "motion-trigger-sec", cfg.MotionTriggerSec, "Sustained motion (s) required to declare motion")
	flag.Float64Var(&cfg.MaxMotionGapSec, "max-motion-gap-sec", cfg.MaxMotionGapSec, "Bridge static gaps shorter than this into motion (0 = off)")
	flag.Float64Var(&cfg.MinSegmentSec, "min-segment-sec", cfg.MinSegmentSec, "Merge segments shorter than this into a neighbour (0 = off)")
	settled := flag.Uint("settled-threshold", uint(cfg.SettledThreshold), "Settled cell count threshold (TimesSeenCount)")
	flag.StringVar(&cfg.SensorID, "sensor-id", cfg.SensorID, "Sensor identifier")
	flag.IntVar(&cfg.UDPPort, "port", cfg.UDPPort, "UDP port for LiDAR data")
	flag.BoolVar(&cfg.ExportMetrics, "export-metrics", false, "Write per-frame metrics to frame_metrics.csv")
	flag.BoolVar(&cfg.ExportJSON, "export-json", false, "Write segment metadata to segments.json")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Verbose logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s --pcap FILE [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Split a LiDAR PCAP into motion and static segments.\n\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s --pcap capture.pcapng --output ./segments\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --pcap capture.pcapng --settling-sec 30 --export-json --export-metrics\n", os.Args[0])
	}

	flag.Parse()
	cfg.SettledThreshold = uint32(*settled)
	return cfg
}

func run(cfg pcapsplit.SplitConfig) error {
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	start := time.Now()

	// Pass 1: classify frames.
	if cfg.Verbose {
		log.Printf("pass 1: analysing %s (port %d) ...", cfg.PCAPFile, cfg.UDPPort)
	}
	analysis, err := pcapsplit.Analyse(cfg)
	if err != nil {
		return err
	}

	periods := pcapsplit.BuildTimeline(analysis.Samples, cfg.TimelineConfig())
	segments := pcapsplit.BuildSegments(periods, cfg.OutputPrefix)
	pcapsplit.AssignFrameStates(analysis.Frames, segments)

	// Pass 2: write segment files (fills in PacketCount).
	if cfg.Verbose {
		log.Printf("pass 2: writing %d segment(s) ...", len(segments))
	}
	if err := pcapsplit.WriteSegments(cfg, segments); err != nil {
		return err
	}

	report := pcapsplit.Report{
		InputFile:        cfg.PCAPFile,
		ProcessingTimeMs: time.Since(start).Milliseconds(),
		TotalPackets:     analysis.TotalPackets,
		TotalFrames:      analysis.TotalFrames,
		TotalDurationSec: analysis.LastTime.Sub(analysis.FirstTime).Seconds(),
		Config:           cfg,
		Segments:         segments,
	}

	// Always write a human-readable summary; print it too.
	summaryPath := cfg.MetadataPath("summary.txt")
	if err := pcapsplit.WriteSummary(summaryPath, report); err != nil {
		return err
	}
	fmt.Print(pcapsplit.FormatSummary(report))

	if cfg.ExportJSON {
		if err := pcapsplit.WriteSegmentsJSON(cfg.MetadataPath("segments.json"), report); err != nil {
			return err
		}
	}
	if cfg.ExportMetrics {
		if err := pcapsplit.WriteFrameMetricsCSV(cfg.MetadataPath("frame_metrics.csv"), analysis.Frames); err != nil {
			return err
		}
	}
	return nil
}
