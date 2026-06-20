//go:build pcap
// +build pcap

package lidar

import (
	"flag"
	"fmt"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/pcapanalyse"
)

// AnalyseMain parses pcap-analyse flags into a pcapanalyse.Config and runs the
// shared analysis engine. args is the argument slice after the `pcap-analyse`
// command word (i.e. for `velocity lidar pcap-analyse -pcap f.pcap` it is
// ["-pcap", "f.pcap"]). It returns the process exit code.
//
// AnalyseMain is also the entry point for the standalone cmd/tools/pcap-analyse
// wrapper, so both surfaces share one flag set and one engine.
func AnalyseMain(args []string) int {
	var cfg pcapanalyse.Config

	// Per-applet flag set (not the global flag.CommandLine) so this composes
	// with the other velocity namespaces in the single multi-call binary.
	fs := flag.NewFlagSet("velocity-lidar-pcap-analyse", flag.ContinueOnError)

	fs.StringVar(&cfg.PCAPFile, "pcap", "", "Path to PCAP file (required)")
	fs.StringVar(&cfg.OutputDir, "output", ".", "Output directory for results")
	fs.StringVar(&cfg.SensorID, "sensor-id", "hesai-pandar40p", "Sensor ID")
	fs.IntVar(&cfg.UDPPort, "port", 0, "UDP port for LiDAR data (0 = auto-detect from the capture)")
	fs.StringVar(&cfg.DBPath, "db", "", "SQLite database path (optional, for persistence)")
	fs.BoolVar(&cfg.ExportCSV, "csv", true, "Export tracks to CSV")
	fs.BoolVar(&cfg.ExportJSON, "json", true, "Export full results to JSON")
	fs.BoolVar(&cfg.ExportTraining, "training", false, "Export training data (foreground blobs)")
	fs.BoolVar(&cfg.Verbose, "v", false, "Verbose output")
	fs.Float64Var(&cfg.FrameRate, "fps", 10.0, "Expected frame rate in Hz")
	fs.BoolVar(&cfg.Stats, "stats", false, "Display concise capture statistics (frame rate, RPM, duration)")
	fs.BoolVar(&cfg.Stats10s, "stats-10s", false, "Display per-10s frame rate buckets (grep-friendly)")
	fs.BoolVar(&cfg.Motion, "motion", false, "Report a motion/static timeline (sensor movement vs. stable periods)")
	fs.StringVar(&cfg.MotionJSONPath, "motion-json", "", "Write the motion/static timeline to this JSON file (implies -motion)")

	// Benchmark flags (short and long forms bind to the same variable).
	fs.BoolVar(&cfg.Benchmark, "benchmark", false, "Enable performance measurement mode")
	fs.BoolVar(&cfg.Benchmark, "bench", false, "Enable performance measurement mode (alias for -benchmark)")
	fs.StringVar(&cfg.BenchmarkOutput, "benchmark-output", "", "Output file for benchmark JSON (default: {pcap}_benchmark.json)")
	fs.BoolVar(&cfg.Quiet, "quiet", false, "Suppress verbose output to prevent logging from affecting measurements")
	fs.BoolVar(&cfg.Quiet, "q", false, "Suppress verbose output (alias for -quiet)")
	fs.StringVar(&cfg.CompareBaseline, "compare-baseline", "", "Compare against a baseline benchmark file")
	fs.Float64Var(&cfg.RegressionThreshold, "regression-threshold", 0.10, "Threshold for flagging regressions (default: 0.10 = 10%)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: velocity lidar pcap-analyse [options]\n\n")
		fmt.Fprintf(os.Stderr, "Analyse a LiDAR PCAP through the full L1–L6 tracking pipeline:\n")
		fmt.Fprintf(os.Stderr, "parse packets, build frames, extract foreground, cluster, track,\n")
		fmt.Fprintf(os.Stderr, "classify, and export tracks, capture statistics, an optional motion\n")
		fmt.Fprintf(os.Stderr, "timeline, and ML training data.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  velocity lidar pcap-analyse -pcap capture.pcapng -output ./results\n")
		fmt.Fprintf(os.Stderr, "  velocity lidar pcap-analyse -pcap capture.pcapng -motion -stats\n")
		fmt.Fprintf(os.Stderr, "  velocity lidar pcap-analyse -pcap capture.pcapng -benchmark -compare-baseline base.json\n")
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	cfg.UDPPort = resolveUDPPort(cfg.UDPPort, cfg.PCAPFile)
	if cfg.UDPPort < 0 {
		return 1
	}

	return pcapanalyse.Run(cfg)
}
