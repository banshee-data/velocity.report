//go:build !pcap
// +build !pcap

package server

import (
	"context"
	"fmt"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// sessionMotionPass is unavailable without libpcap. Classifying a capture means
// reading it, and reading one means libpcap.
func sessionMotionPass(_ context.Context, _ []string, _ int,
	_ *cfgpkg.TuningConfig, _ func(current, total int64, detail string)) ([]sqlite.MotionPeriod, error) {
	return nil, fmt.Errorf("motion pass unavailable: rebuild with -tags=pcap")
}
