//go:build pcap
// +build pcap

// Command lidar-bench measures the LiDAR L1–L6 tracking pipeline over a PCAP and
// compares the result against a committed baseline. It is the end-to-end
// perf-regression gate used by `make test-perf` and nightly CI, and is a thin
// wrapper over the shared engine. It is intentionally not part of the
// `velocity lidar` operational surface.
package main

import (
	"os"

	"github.com/banshee-data/velocity.report/internal/cmd/lidar"
)

func main() {
	os.Exit(lidar.BenchMain(os.Args[1:]))
}
