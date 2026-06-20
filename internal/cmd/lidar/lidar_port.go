//go:build pcap
// +build pcap

package lidar

import (
	"fmt"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
)

// resolveUDPPort returns port unchanged when it is non-zero; otherwise it
// auto-detects the sensor's UDP port from the capture (the port carrying the
// most packets) and reports the choice on stderr. It returns -1 if detection
// was attempted and failed, so callers can exit with an error.
func resolveUDPPort(port int, pcapFile string) int {
	if port != 0 || pcapFile == "" {
		return port
	}
	detected, err := network.DetectUDPPort(pcapFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not auto-detect UDP port (pass --port): %v\n", err)
		return -1
	}
	fmt.Fprintf(os.Stderr, "auto-detected UDP port %d\n", detected)
	return detected
}
