//go:build pcap
// +build pcap

package network

import (
	"fmt"
	"log"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
)

// CountPCAPPackets counts the total number of UDP packets matching the given
// port in a PCAP file. This enables progress reporting and offset-based seeking.
func CountPCAPPackets(pcapFile string, udpPort int) (uint64, error) {
	handle, err := pcap.OpenOffline(pcapFile)
	if err != nil {
		return 0, fmt.Errorf("failed to open PCAP file %s for counting: %w", pcapFile, err)
	}
	defer handle.Close()

	filterStr := fmt.Sprintf("udp port %d", udpPort)
	if err := handle.SetBPFFilter(filterStr); err != nil {
		return 0, fmt.Errorf("failed to set BPF filter '%s': %w", filterStr, err)
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	var count uint64
	for packet := range packetSource.Packets() {
		if packet == nil {
			break
		}
		count++
	}

	log.Printf("PCAP packet count: %d packets matching filter '%s' in %s", count, filterStr, pcapFile)
	return count, nil
}
