//go:build pcap
// +build pcap

package network

import (
	"fmt"
	"log"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
)

// PCAPCountResult holds the result of counting packets in a PCAP file.
type PCAPCountResult struct {
	Count            uint64
	FirstTimestampNs int64
	LastTimestampNs  int64
}

// CountPCAPPackets counts the total number of UDP packets matching the given
// port in a PCAP file and captures the first/last packet timestamps.
// This enables progress reporting and timeline display.
func CountPCAPPackets(pcapFile string, udpPort int) (PCAPCountResult, error) {
	handle, err := pcap.OpenOffline(pcapFile)
	if err != nil {
		return PCAPCountResult{}, fmt.Errorf("failed to open PCAP file %s for counting: %w", pcapFile, err)
	}
	defer handle.Close()

	filterStr := fmt.Sprintf("udp port %d", udpPort)
	if err := handle.SetBPFFilter(filterStr); err != nil {
		return PCAPCountResult{}, fmt.Errorf("failed to set BPF filter '%s': %w", filterStr, err)
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	var result PCAPCountResult
	for packet := range packetSource.Packets() {
		if packet == nil {
			break
		}
		ts := packet.Metadata().Timestamp.UnixNano()
		if result.Count == 0 {
			result.FirstTimestampNs = ts
		}
		result.LastTimestampNs = ts
		result.Count++
	}

	log.Printf("PCAP packet count: %d packets matching filter '%s' in %s", result.Count, filterStr, pcapFile)
	return result, nil
}
