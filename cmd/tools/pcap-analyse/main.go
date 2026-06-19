//go:build pcap
// +build pcap

// Command pcap-analyse analyses a LiDAR PCAP through the full tracking
// pipeline. It is a thin wrapper over the shared engine and is equivalent to
// `velocity lidar pcap-analyse`.
package main

import (
	"os"

	"github.com/banshee-data/velocity.report/internal/cmd/lidar"
)

func main() {
	os.Exit(lidar.AnalyseMain(os.Args[1:]))
}
