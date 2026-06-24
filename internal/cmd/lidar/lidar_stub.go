//go:build !pcap
// +build !pcap

// Package lidar's non-pcap stub. The LiDAR PCAP tools require libpcap, compiled
// in via the `pcap` build tag (the velocity binary always sets it). This stub
// keeps `go build ./...`, `go test ./...`, and the IDE green without libpcap.
package lidar

import (
	"fmt"
	"os"
)

// Main is the non-pcap stub for the `velocity lidar` namespace.
func Main(args []string) int {
	fmt.Fprintln(os.Stderr, "velocity lidar: this build was compiled without pcap support; rebuild with -tags pcap")
	return 1
}
