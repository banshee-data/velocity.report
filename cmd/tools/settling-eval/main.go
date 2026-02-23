// Command settling-eval evaluates background grid settling convergence by
// replaying a captured PCAP file offline through a local BackgroundManager
// at full speed. It evaluates convergence on every frame and produces a JSON
// report with convergence metrics and a recommended WarmupMinFrames value.
//
// Usage:
//
//	go run -tags=pcap ./cmd/tools/settling-eval capture.pcap [--tuning config/tuning.defaults.json] [--output report.json]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func main() {
	output := flag.String("output", "", "output JSON path (default: stdout)")
	sensor := flag.String("sensor", "pcap-eval", "sensor ID")
	tuningFile := flag.String("tuning", "", "tuning config JSON path (default: config/tuning.defaults.json)")
	udpPort := flag.Int("port", 2368, "UDP port filter for PCAP packets")

	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: settling-eval [flags] <pcap-file>\n")
		flag.PrintDefaults()
		os.Exit(1)
	}
	pcapFile := flag.Arg(0)

	report, err := runPCAPEval(pcapFile, *tuningFile, *sensor, *udpPort)
	if err != nil {
		log.Fatalf("pcap eval: %v", err)
	}
	if err := l3grid.WriteReport(report, *output); err != nil {
		log.Fatalf("write report: %v", err)
	}
	if *output != "" {
		log.Printf("✓ report written to %s", *output)
	}
}
