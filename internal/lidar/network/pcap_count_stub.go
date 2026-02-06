//go:build !pcap
// +build !pcap

package network

import "fmt"

// CountPCAPPackets is a stub that returns an error when pcap support is not compiled in.
func CountPCAPPackets(pcapFile string, udpPort int) (uint64, error) {
	return 0, fmt.Errorf("PCAP support not compiled in (requires pcap build tag)")
}
