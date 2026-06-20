//go:build pcap
// +build pcap

package network

import (
	"fmt"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// detectPortScanLimit caps how many UDP packets DetectUDPPort inspects; the
// sensor's data port dominates the first few thousand packets.
const detectPortScanLimit = 4000

// DetectUDPPort inspects the start of a PCAP and returns the UDP destination
// port carrying the most packets — the port the sensor logged on. It is used
// when the operator does not pass an explicit --port.
func DetectUDPPort(pcapFile string) (int, error) {
	handle, err := pcap.OpenOffline(pcapFile)
	if err != nil {
		return 0, fmt.Errorf("open pcap: %w", err)
	}
	defer handle.Close()
	if err := handle.SetBPFFilter("udp"); err != nil {
		return 0, fmt.Errorf("set bpf filter: %w", err)
	}

	counts := make(map[int]int)
	source := gopacket.NewPacketSource(handle, handle.LinkType())
	scanned := 0
	for packet := range source.Packets() {
		udp, ok := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
		if !ok {
			continue
		}
		counts[int(udp.DstPort)]++
		if scanned++; scanned >= detectPortScanLimit {
			break
		}
	}
	if len(counts) == 0 {
		return 0, fmt.Errorf("no UDP packets found in %s", pcapFile)
	}

	bestPort, bestCount := 0, -1
	for port, n := range counts {
		// Deterministic tie-break: higher count wins, then the lower port.
		if n > bestCount || (n == bestCount && port < bestPort) {
			bestPort, bestCount = port, n
		}
	}
	return bestPort, nil
}
