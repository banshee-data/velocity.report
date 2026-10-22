//go:build pcap
// +build pcap

package server

import (
	"context"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/pcapsplit"
)

// periodBackground replays a window of a session's captures into a fresh L3
// model and returns its settled cells. startSecs is measured from the first
// packet of paths[0].
func periodBackground(ctx context.Context, paths []string, extents []capseq.Segment, udpPort int,
	tuning *cfgpkg.TuningConfig, startSecs, durationSecs float64) (*pcapsplit.Background, error) {
	cfg := pcapsplit.DefaultSplitConfig()
	cfg.PCAPFile = paths[0]
	if len(paths) > 1 {
		cfg.PCAPFiles = paths
	}
	cfg.Extents = extents
	cfg.UDPPort = udpPort
	cfg.Tuning = tuning
	if tuning != nil && cfg.SensorID == "" {
		cfg.SensorID = tuning.GetSensor()
	}
	cfg.StartSeconds = startSecs
	cfg.DurationSeconds = durationSecs
	return pcapsplit.SettledBackground(ctx, cfg)
}
