//go:build !pcap
// +build !pcap

package server

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/capindex"
)

// probeCaptureExtent is unavailable without libpcap: learning a capture's
// packet extent means reading it.
func probeCaptureExtent(_ string, _ int) (capindex.Extent, error) {
	return capindex.Extent{}, fmt.Errorf("capture probing unavailable: rebuild with -tags=pcap")
}
