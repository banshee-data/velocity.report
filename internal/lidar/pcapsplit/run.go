//go:build pcap
// +build pcap

package pcapsplit

import (
	"fmt"
	"log"
	"os"
	"time"
)

// Run performs a full two-pass split: it analyses the capture (pass 1), builds
// the motion/static timeline and segments, writes each segment's PCAP (pass 2),
// and emits the summary plus optional JSON/CSV metadata. It is the orchestration
// shared by the standalone pcap-split tool and `velocity lidar pcap-split`.
func Run(cfg SplitConfig) error {
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	start := time.Now()

	// Pass 1: classify frames.
	if cfg.Verbose {
		log.Printf("pass 1: analysing %s (port %d) ...", cfg.PCAPFile, cfg.UDPPort)
	}
	analysis, err := Analyse(cfg)
	if err != nil {
		return err
	}

	periods := BuildTimeline(analysis.Samples, cfg.TimelineConfig())
	segments := BuildSegments(periods, cfg.OutputPrefix)
	AssignFrameStates(analysis.Frames, segments)

	// Pass 2: write segment files (fills in PacketCount), unless this is a
	// non-destructive preview used to calibrate real captures.
	if !cfg.DryRun {
		if cfg.Verbose {
			log.Printf("pass 2: writing %d segment(s) ...", len(segments))
		}
		if err := WriteSegments(cfg, segments); err != nil {
			return err
		}
	}

	report := Report{
		InputFile:        cfg.PCAPFile,
		ProcessingTimeMs: time.Since(start).Milliseconds(),
		TotalPackets:     analysis.TotalPackets,
		TotalFrames:      analysis.TotalFrames,
		TotalDurationSec: analysis.LastTime.Sub(analysis.FirstTime).Seconds(),
		Config:           cfg,
		Segments:         segments,
	}

	// Always write a human-readable summary; print it too.
	if err := WriteSummary(cfg.MetadataPath("summary.txt"), report); err != nil {
		return err
	}
	fmt.Print(FormatSummary(report))

	if cfg.ExportJSON {
		if err := WriteSegmentsJSON(cfg.MetadataPath("segments.json"), report); err != nil {
			return err
		}
	}
	if cfg.ExportMetrics {
		if err := WriteFrameMetricsCSV(cfg.MetadataPath("frame_metrics.csv"), analysis.Frames); err != nil {
			return err
		}
	}
	return nil
}
