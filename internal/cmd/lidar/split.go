//go:build pcap
// +build pcap

package lidar

import (
	"flag"
	"fmt"
	"math"
	"os"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/pcapsplit"
)

// SplitMain parses pcap-split flags and runs the segmentation. args is the
// argument slice after the `pcap-split` command word. It returns the process
// exit code, and is also the entry point for the standalone cmd/tools/pcap-split
// wrapper so both surfaces share one flag set and one engine.
func SplitMain(args []string) int {
	cfg := pcapsplit.DefaultSplitConfig()

	fs := flag.NewFlagSet("velocity-lidar-pcap-split", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultConfigPath, "Path to JSON tuning config (falls back to the embedded defaults)")
	fs.StringVar(&cfg.PCAPFile, "pcap", "", "Input PCAP/PCAPNG file (required)")
	fs.StringVar(&cfg.OutputDir, "output", ".", "Output directory for segments and metadata")
	fs.StringVar(&cfg.OutputPrefix, "prefix", cfg.OutputPrefix, "Output filename prefix")
	fs.Float64Var(&cfg.SettlingSec, "settling-sec", cfg.SettlingSec, "Sustained stability (s) required to declare static")
	fs.Float64Var(&cfg.MotionTriggerSec, "motion-trigger-sec", cfg.MotionTriggerSec, "Sustained motion (s) required to declare motion")
	fs.Float64Var(&cfg.MaxMotionGapSec, "max-motion-gap-sec", cfg.MaxMotionGapSec, "Bridge static gaps shorter than this into motion (0 = off)")
	fs.Float64Var(&cfg.MinSegmentSec, "min-segment-sec", cfg.MinSegmentSec, "Merge segments shorter than this into a neighbour (0 = off)")
	settled := fs.Uint("settled-threshold", 0, "Override the config's settled cell count threshold (0 = use config)")
	fs.StringVar(&cfg.SensorID, "sensor-id", "", "Sensor identifier (default: from config l1.sensor)")
	fs.IntVar(&cfg.UDPPort, "port", 0, "UDP port for LiDAR data (0 = auto-detect from the capture)")
	fs.BoolVar(&cfg.ExportMetrics, "export-metrics", false, "Write per-frame metrics to frame_metrics.csv")
	fs.BoolVar(&cfg.ExportJSON, "export-json", false, "Write segment metadata to segments.json")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "Analyse and report segments without writing PCAP files")
	fs.Float64Var(&cfg.ProgressSecs, "progress", 20, "Seconds between progress updates during the PCAP read (0 = off)")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Verbose logging")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: velocity lidar pcap-split --pcap FILE [options]\n\n")
		fmt.Fprintf(os.Stderr, "Split a LiDAR PCAP into non-overlapping motion and static segments.\n\nOptions:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  velocity lidar pcap-split --pcap capture.pcapng --output ./segments\n")
		fmt.Fprintf(os.Stderr, "  velocity lidar pcap-split --pcap capture.pcapng --settling-sec 30 --export-json --export-metrics\n")
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
	if cfg.SensorID == "" {
		cfg.SensorID = tuningCfg.GetSensor()
	}
	cfg.SettledThreshold = uint32(tuningCfg.GetSettledThreshold())
	if *settled != 0 {
		if uint64(*settled) > math.MaxUint32 {
			fmt.Fprintf(os.Stderr, "error: --settled-threshold %d exceeds the maximum (%d)\n", *settled, uint32(math.MaxUint32))
			return 2
		}
		cfg.SettledThreshold = uint32(*settled)
	}

	if cfg.PCAPFile == "" {
		fmt.Fprintln(os.Stderr, "error: --pcap is required")
		fs.Usage()
		return 2
	}
	cfg.UDPPort = resolveUDPPort(cfg.UDPPort, cfg.PCAPFile)
	if cfg.UDPPort < 0 {
		return 1
	}
	if err := pcapsplit.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "pcap-split: %v\n", err)
		return 1
	}
	return 0
}
