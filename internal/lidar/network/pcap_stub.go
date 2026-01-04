//go:build !pcap
// +build !pcap

package network

import (
	"context"
	"fmt"
)

// ReadPCAPFile is a stub implementation when PCAP support is disabled
// Build with -tags=pcap to enable PCAP file reading
func ReadPCAPFile(ctx context.Context, pcapFile string, udpPort int, parser Parser, frameBuilder FrameBuilder, stats PacketStatsInterface, forwarder *PacketForwarder) error {
	return fmt.Errorf("PCAP support not enabled: rebuild with -tags=pcap to enable PCAP file reading")
}
