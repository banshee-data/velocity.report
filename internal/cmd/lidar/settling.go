//go:build pcap
// +build pcap

package lidar

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/settlingeval"
)

// SettlingEvalMain parses settling-eval flags and runs offline settling
// convergence evaluation. args is the argument slice after the `settling-eval`
// command word; the single positional argument is the PCAP file. It returns the
// process exit code, and is also the entry point for the standalone
// cmd/tools/settling-eval wrapper.
func SettlingEvalMain(args []string) int {
	fs := flag.NewFlagSet("velocity-lidar-settling-eval", flag.ContinueOnError)
	output := fs.String("output", "", "output JSON path (default: stdout)")
	sensor := fs.String("sensor", "pcap-eval", "sensor ID")
	var configPath string
	fs.StringVar(&configPath, "config", config.DefaultConfigPath, "Path to JSON tuning config (falls back to the embedded defaults)")
	fs.StringVar(&configPath, "tuning", config.DefaultConfigPath, "deprecated alias for -config")
	udpPort := fs.Int("port", 0, "UDP port filter for PCAP packets (0 = auto-detect from the capture)")
	startSeconds := fs.Float64("start-seconds", 0, "Start replay at this capture offset in seconds")
	durationSeconds := fs.Float64("duration-seconds", -1, "Replay duration in seconds (-1 = remaining capture)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: velocity lidar settling-eval [flags] <pcap-file>\n\n")
		fmt.Fprintf(os.Stderr, "Replay a PCAP through a BackgroundManager and report settling convergence,\n")
		fmt.Fprintf(os.Stderr, "including a recommended WarmupMinFrames value.\n\nOptions:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "error: exactly one <pcap-file> argument is required")
		fs.Usage()
		return 2
	}
	pcapFile := fs.Arg(0)
	port := resolveUDPPort(*udpPort, pcapFile)
	if port < 0 {
		return 1
	}

	report, err := settlingeval.Run(settlingeval.Config{
		PCAPFile:        pcapFile,
		TuningFile:      configPath,
		SensorID:        *sensor,
		UDPPort:         port,
		StartSeconds:    *startSeconds,
		DurationSeconds: *durationSeconds,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "settling-eval: %v\n", err)
		return 1
	}
	if err := l3grid.WriteReport(report, *output); err != nil {
		fmt.Fprintf(os.Stderr, "settling-eval: write report: %v\n", err)
		return 1
	}
	if *output != "" {
		log.Printf("✓ report written to %s", *output)
	}
	return 0
}
