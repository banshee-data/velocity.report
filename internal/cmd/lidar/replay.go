//go:build pcap
// +build pcap

package lidar

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/analysis"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// ReplayEvalMain parses pcap-replay flags, replays the capture through the
// perception pipeline, and optionally analyses or A/B-compares the result.
// args is the argument slice after the command word. It returns the exit code.
func ReplayEvalMain(args []string) int {
	fs := flag.NewFlagSet("velocity-lidar-pcap-replay", flag.ContinueOnError)
	pcapFile := fs.String("pcap", "", "Input PCAP/PCAPNG file (required)")
	outDir := fs.String("output", "", "Output VRLOG directory (required)")
	configPath := fs.String("config", config.DefaultConfigPath, "Path to JSON tuning config (falls back to the embedded defaults)")
	sensor := fs.String("sensor-id", "pcap-replay", "Sensor identifier stamped into the recording")
	udpPort := fs.Int("port", 0, "UDP port for LiDAR data (0 = auto-detect from the capture)")
	startSeconds := fs.Float64("start-seconds", 0, "Start replay at this capture offset in seconds")
	durationSeconds := fs.Float64("duration-seconds", 0, "Replay duration in seconds (0 = remaining capture)")
	warmupSeconds := fs.Float64("warmup-seconds", 0, "Process this prefix before start-seconds without recording it; retain background and tracker state")
	requireSettled := fs.Bool("require-settled", false, "Fail unless the background is settled before the first scored frame")
	includePoints := fs.Bool("include-points", false, "Record the point cloud (much larger output; needed only for the visualiser)")
	progress := fs.Int("progress-frames", 200, "Log progress every N frames (0 = off)")
	analyse := fs.Bool("analyse", true, "Generate analysis.json in the output directory")
	compareTo := fs.String("compare-to", "", "Path to a baseline VRLOG; writes an A/B comparison against it")
	compareOut := fs.String("compare-output", "", "Where to write the comparison JSON (default: <output>/comparison.json)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: velocity lidar pcap-replay --pcap FILE --output DIR [options]\n\n")
		fmt.Fprintf(os.Stderr,
			`Replay a PCAP through the full L1-L6 perception pipeline and record a VRLOG.
No server, no database, no listening port.

Use this to measure a change to clustering, tracking or classification. Replaying
an existing VRLOG cannot do that: a VRLOG stores the decisions the pipeline
already made, so it shows what the old code concluded, not what the new code
would conclude.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  # Record a run and analyse it
  velocity lidar pcap-replay --pcap capture.pcap --output ./runs/after

  # A/B two tuning configs over the same capture
  velocity lidar pcap-replay --pcap capture.pcap --output ./runs/before \
      --config config/before.json
  velocity lidar pcap-replay --pcap capture.pcap --output ./runs/after \
      --config config/after.json --compare-to ./runs/before
`)
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *pcapFile == "" {
		fmt.Fprintln(os.Stderr, "error: --pcap is required")
		fs.Usage()
		return 2
	}
	if *outDir == "" {
		fmt.Fprintln(os.Stderr, "error: --output is required")
		fs.Usage()
		return 2
	}

	port := resolveUDPPort(*udpPort, *pcapFile)
	if port < 0 {
		return 1
	}

	result, err := replayeval.Run(replayeval.Config{
		PCAPFile:        *pcapFile,
		OutDir:          *outDir,
		TuningFile:      *configPath,
		SensorID:        *sensor,
		UDPPort:         port,
		StartSeconds:    *startSeconds,
		DurationSeconds: *durationSeconds,
		WarmupSeconds:   *warmupSeconds,
		RequireSettled:  *requireSettled,
		IncludePoints:   *includePoints,
		ProgressEvery:   *progress,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pcap-replay: %v\n", err)
		return 1
	}

	fmt.Printf("recorded %d frames (%d with no tracks) in %s\n",
		result.FramesRecorded, result.FramesEmpty, result.Elapsed.Round(time.Millisecond))
	fmt.Printf("vrlog: %s\n", result.VRLOGPath)

	if !*analyse && *compareTo == "" {
		return 0
	}

	report, reportPath, err := analysis.GenerateReport(result.VRLOGPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pcap-replay: analyse: %v\n", err)
		return 1
	}
	fmt.Printf("analysis: %s\n", reportPath)
	printReplaySummary(report)

	if *compareTo == "" {
		return 0
	}

	out := *compareOut
	if out == "" {
		out = result.VRLOGPath + "/comparison.json"
	}
	if _, err := analysis.CompareReports(*compareTo, result.VRLOGPath, out); err != nil {
		fmt.Fprintf(os.Stderr, "pcap-replay: compare: %v\n", err)
		return 1
	}
	fmt.Printf("comparison: %s\n", out)
	return 0
}

// printReplaySummary prints the handful of figures a tracker change is judged
// on, so the common case needs no separate tool.
func printReplaySummary(r *analysis.AnalysisReport) {
	ts := r.TrackSummary
	fmt.Printf("\ntracks: %d total, %d confirmed, fragmentation %.3f\n",
		ts.TotalTracks, ts.ConfirmedTracks, ts.FragmentationRatio)

	if a := ts.Alignment; a != nil && a.CourseAlignmentP50Deg != nil {
		d := a.CourseAlignmentP50Deg
		fmt.Printf("course alignment over %d tracks: median %.1f, avg %.1f, max %.1f deg\n",
			a.CourseAlignmentTracks, derefFloat(d.P50), d.Avg, d.Max)
	}
	if hl := ts.HeadingLock; hl != nil {
		// Acceptance leads, and course error is printed next to it. The two
		// heading paths name their accepted frames differently, so acceptance
		// is what compares between them; and a course error measured over half
		// as many frames is not the same claim as one measured over all of them.
		fmt.Printf("heading acceptance %.1f%% (%d accepted, %d held of %d frames)\n",
			100*hl.AcceptanceRatio, hl.AcceptedFrames, hl.HeldFrames,
			hl.AcceptedFrames+hl.HeldFrames)
		printAbstentionReasons(hl.SourceFrames, hl.HeldFrames)
		fmt.Printf("legacy lifetime heading lock: %d of %d sustained -> %d never recovered (%.0f%%), %d relocked, %d released\n",
			hl.SustainedLockTracks, hl.Tracks, hl.NeverRecoveredTracks, 100*hl.TrappedRatio,
			hl.RelockedTracks, hl.ReleasedTracks)
		fmt.Printf("longest run %d frames, %d forced releases\n",
			hl.LongestLockRunFrames, hl.ForcedReleases)
		fmt.Printf("terminal measurement episodes: %d unrecovered, %d recovered, %d censored (%d assessed)\n",
			hl.TerminalUnrecovered, hl.TerminalRecovered, hl.TerminalCensored, hl.TerminalAssessed)
	}
}

// printAbstentionReasons breaks the held frames down by why the estimator
// declined. A near-square observation, one that fits no interpretation, and a
// tie between two need different fixes, so a single held total cannot be acted
// on. Only the reasons actually present are printed.
func printAbstentionReasons(sources map[string]int, held int) {
	if held == 0 {
		return
	}
	reasons := []struct{ key, label string }{
		{"locked", "guard-locked"},
		{"axis_square", "no distinguishable axis"},
		{"axis_low_support", "too little of the object visible"},
		{"axis_no_fit", "fits neither interpretation"},
		{"ambiguous", "interpretations tied"},
		{"insufficient", "geometry missing or invalid"},
	}
	parts, accounted := make([]string, 0, len(reasons)), 0
	for _, r := range reasons {
		if n := sources[r.key]; n > 0 {
			accounted += n
			parts = append(parts, fmt.Sprintf("%s %d (%.0f%%)", r.label, n, 100*float64(n)/float64(held)))
		}
	}
	if len(parts) > 0 {
		fmt.Printf("  held because: %s\n", strings.Join(parts, ", "))
	}
	// The reasons partition the held frames. A shortfall means a source was
	// added without being named here, and the breakdown would quietly mislead.
	if accounted != held {
		fmt.Printf("  WARNING: %d held frames unaccounted for; a heading source is missing from this breakdown\n",
			held-accounted)
	}
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
