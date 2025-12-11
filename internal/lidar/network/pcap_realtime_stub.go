//go:build !pcap
// +build !pcap

package network

import (
	"context"
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// RealtimeReplayConfig is a stub when pcap is not available.
type RealtimeReplayConfig struct {
	SpeedMultiplier         float64
	PacketForwarder         *PacketForwarder
	ForegroundForwarder     *ForegroundForwarder
	BackgroundManager       *lidar.BackgroundManager
}

// ReadPCAPFileRealtime is a stub that returns an error when pcap support is not compiled in.
func ReadPCAPFileRealtime(ctx context.Context, pcapFile string, udpPort int, parser Parser, frameBuilder FrameBuilder, stats PacketStatsInterface, config RealtimeReplayConfig) error {
	return fmt.Errorf("PCAP real-time replay support not compiled in (requires pcap build tag)")
}
