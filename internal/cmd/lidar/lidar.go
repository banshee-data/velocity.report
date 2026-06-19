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
  pcap-analyse   Analyse a PCAP through the L1–L6 pipeline (stats, motion, benchmark)

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
	case "pcap-analyse":
		return AnalyseMain(args[1:])
	case "help", "-h", "--help":
		fmt.Println(namespaceUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown lidar command: %q\n\n%s\n", args[0], namespaceUsage)
		return 2
	}
}
