//go:build pcap
// +build pcap

// Package lidar implements the `velocity lidar` namespace: offline LiDAR
// capture diagnostics that require libpcap. It is compiled into the velocity
// binary via the `pcap` build tag (always set for velocity); a !pcap stub
// keeps the default toolchain building.
package lidar

import (
	"fmt"
	"os"
)

const namespaceUsage = `velocity lidar — LiDAR capture diagnostics

Usage:
  velocity lidar <command> [flags]

Commands:
  pcap-split      Scan a PCAP for capture/motion stats and segment it into
                  non-overlapping motion and static periods
  settling-eval   Evaluate background settling convergence; recommend warmup frames
  pcap-replay     Replay a PCAP through the full perception pipeline and record
                  a VRLOG, with no server, database, or listening port. Use this
                  to measure a change to clustering, tracking, or classification

Run 'velocity lidar <command> -h' for command flags.`

// Main routes the `velocity lidar` namespace to its subcommands. args is the
// argument slice after the `lidar` namespace word. It returns the process exit
// code.
func Main(args []string) int {
	if len(args) == 0 {
		fmt.Println(namespaceUsage)
		return 0
	}
	switch args[0] {
	case "pcap-split":
		return SplitMain(args[1:])
	case "settling-eval":
		return SettlingEvalMain(args[1:])
	case "pcap-replay":
		return ReplayEvalMain(args[1:])
	case "help", "-h", "--help":
		fmt.Println(namespaceUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown lidar command: %q\n\n%s\n", args[0], namespaceUsage)
		return 2
	}
}
