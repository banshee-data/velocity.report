//go:build pcap
// +build pcap

// Command pcap-split segments a LiDAR PCAP into non-overlapping motion and
// static segments. It is a thin wrapper over the shared engine and is
// equivalent to `velocity lidar pcap-split`.
package main

import (
	"os"

	"github.com/banshee-data/velocity.report/internal/cmd/lidar"
)

func main() {
	os.Exit(lidar.SplitMain(os.Args[1:]))
}
