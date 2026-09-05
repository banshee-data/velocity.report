//go:build pcap
// +build pcap

package lidar

import (
	"flag"
	"fmt"
	"os"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/lidarbench"
)

// BenchMain parses lidar-bench flags and runs the pipeline performance
// benchmark. args is the argument slice after the command word. It is the entry
// point for the standalone cmd/tools/lidar-bench tool (used by `make test-perf`
// and nightly CI) and is intentionally NOT registered in the `velocity lidar`
// operational namespace.
func BenchMain(args []string) int {
	var cfg lidarbench.Config

	fs := flag.NewFlagSet("lidar-bench", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultConfigPath, "Path to JSON tuning config (falls back to the embedded defaults)")
	fs.StringVar(&cfg.PCAPFile, "pcap", "", "Path to PCAP file (required)")
	fs.Float64Var(&cfg.StartSeconds, "start-seconds", 0, "Start replay at this capture offset in seconds")
	fs.Float64Var(&cfg.DurationSeconds, "duration-seconds", -1, "Replay duration in seconds (0 or -1 = remaining capture)")
	fs.StringVar(&cfg.OutputDir, "output", ".", "Output directory for benchmark JSON")
	fs.StringVar(&cfg.SensorID, "sensor-id", "", "Sensor ID (default: from config l1.sensor)")
	fs.IntVar(&cfg.UDPPort, "port", 0, "UDP port for LiDAR data (0 = auto-detect from the capture)")
	fs.StringVar(&cfg.BenchmarkOutput, "benchmark-output", "", "Output file for benchmark JSON (default: {pcap}_benchmark.json)")
	fs.StringVar(&cfg.CompareBaseline, "compare-baseline", "", "Compare against a baseline benchmark file")
	fs.Float64Var(&cfg.RegressionThreshold, "regression-threshold", 0.10, "Threshold for flagging regressions (default: 0.10 = 10%)")
	profileName := fs.String("profile", "", "Reduce pipeline depth to l3-only or detect by disabling layers (default: whatever the config runs)")
	maxOverBudgetPct := fs.Float64("max-frames-over-budget-pct", 1.0,
		"Share of frames allowed to exceed pipeline.frame_budget_ms before the run fails")
	fs.Float64Var(&cfg.WorkTolerance, "work-tolerance", lidarbench.DefaultWorkTolerance,
		"Fraction a work counter may drift from the baseline before the runs are treated as different workloads")
	fs.IntVar(&cfg.Repeats, "repeat", 1, "Run the benchmark N times and report the median run by wall clock")
	fs.BoolVar(&cfg.Quiet, "quiet", false, "Suppress non-essential output to keep measurements clean")
	fs.BoolVar(&cfg.Quiet, "q", false, "Suppress non-essential output (alias for -quiet)")
	fs.Float64Var(&cfg.ProgressSecs, "progress", 10, "Seconds between progress updates during the PCAP read (0 = off)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lidar-bench -pcap FILE [options]\n\n")
		fmt.Fprintf(os.Stderr, "Measure the LiDAR L1–L6 tracking pipeline over a PCAP and compare against a\n")
		fmt.Fprintf(os.Stderr, "baseline. This is the perf-regression gate used by `make test-perf` and CI.\n\nOptions:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  lidar-bench -pcap capture.pcapng -benchmark-output base.json\n")
		fmt.Fprintf(os.Stderr, "  lidar-bench -pcap capture.pcapng -compare-baseline base.json -quiet\n")
		fmt.Fprintf(os.Stderr, "  lidar-bench -pcap capture.pcapng -profile l3-only -repeat 5 -benchmark-output base.json\n")
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	tuningCfg, err := config.LoadTuningConfigOrEmbedded(*configPath, radarassets.TuningDefaults)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load tuning config %s: %v\n", *configPath, err)
		return 1
	}
	cfg.Tuning = tuningCfg
	// The CLI always enforces the budget: this is what the perf gate runs, on
	// an uninstrumented binary that can actually meet it.
	cfg.MaxFramesOverBudgetPct = maxOverBudgetPct
	if *profileName != "" {
		profile, perr := config.ParseProfile(*profileName)
		if perr != nil {
			fmt.Fprintf(os.Stderr, "-profile: %v\n", perr)
			return 2
		}
		// Apply to the config rather than carrying a parallel switch, so the
		// fingerprint, the pipeline gates and the recorded profile all read
		// the same configuration.
		if perr := tuningCfg.ApplyProfile(profile); perr != nil {
			fmt.Fprintf(os.Stderr, "-profile: %v\n", perr)
			return 2
		}
	}
	if cfg.Repeats < 1 {
		fmt.Fprintln(os.Stderr, "-repeat must be at least 1")
		return 2
	}
	if cfg.SensorID == "" {
		cfg.SensorID = tuningCfg.GetSensor()
	}
	if cfg.PCAPFile == "" {
		fmt.Fprintln(os.Stderr, "error: -pcap is required")
		fs.Usage()
		return 2
	}
	if cfg.Quiet {
		cfg.ProgressSecs = 0
	}
	cfg.UDPPort = resolveUDPPort(cfg.UDPPort, cfg.PCAPFile)
	if cfg.UDPPort < 0 {
		return 1
	}
	return lidarbench.Run(cfg)
}
