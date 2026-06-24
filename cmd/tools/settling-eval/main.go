//go:build pcap
// +build pcap

// Command settling-eval evaluates LiDAR background-grid settling convergence by
// replaying a captured PCAP offline through a local BackgroundManager. It is a
// thin wrapper over the shared engine, equivalent to
// `velocity lidar settling-eval`.
package main

import (
	"os"

	"github.com/banshee-data/velocity.report/internal/cmd/lidar"
)

func main() {
	os.Exit(lidar.SettlingEvalMain(os.Args[1:]))
}
