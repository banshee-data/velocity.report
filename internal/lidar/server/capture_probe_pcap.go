//go:build pcap
// +build pcap

package server

import (
	"github.com/banshee-data/velocity.report/internal/lidar/capindex"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
)

// detectCapturePort is the per-capture port sniffer, indirected for tests.
var detectCapturePort = network.DetectUDPPort

// probeCaptureExtent obtains one capture's packet-time extent.
//
// The configured port is tried first, because on a live deployment the captures
// were written by this very sensor and it is right. When nothing matches, the
// port carried by the capture itself is used instead: an index that could only
// read captures recorded on the port this process happens to listen on would be
// useless for anything copied in from another site or another sensor, and
// `velocity lidar pcap-split` has always sniffed the port this way.
func probeCaptureExtent(absPath string, udpPort int) (capindex.Extent, error) {
	if udpPort > 0 {
		result, err := countPCAPPackets(absPath, udpPort)
		if err == nil && result.Count > 0 {
			return capindex.Extent{
				FirstPacketNs: result.FirstTimestampNs,
				LastPacketNs:  result.LastTimestampNs,
				PacketCount:   result.Count,
				UDPPort:       udpPort,
			}, nil
		}
	}

	detected, err := detectCapturePort(absPath)
	if err != nil {
		return capindex.Extent{}, err
	}
	if detected == udpPort {
		// Sniffing found the same port that just matched nothing, so the
		// capture genuinely holds no LiDAR data. Reporting an empty extent
		// lets the indexer say that rather than repeating the read.
		return capindex.Extent{UDPPort: udpPort}, nil
	}

	result, err := countPCAPPackets(absPath, detected)
	if err != nil {
		return capindex.Extent{}, err
	}
	return capindex.Extent{
		FirstPacketNs: result.FirstTimestampNs,
		LastPacketNs:  result.LastTimestampNs,
		PacketCount:   result.Count,
		UDPPort:       detected,
	}, nil
}
