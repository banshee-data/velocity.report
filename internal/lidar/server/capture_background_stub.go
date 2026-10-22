//go:build !pcap
// +build !pcap

package server

import (
	"context"
	"fmt"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/pcapsplit"
)

// periodBackground is unavailable without libpcap: building the background
// means reading the captures.
func periodBackground(_ context.Context, _ []string, _ []capseq.Segment, _ int,
	_ *cfgpkg.TuningConfig, _, _ float64) (*pcapsplit.Background, error) {
	return nil, fmt.Errorf("settled background unavailable: rebuild with -tags=pcap")
}
