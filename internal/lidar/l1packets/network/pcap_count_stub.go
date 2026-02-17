//go:build !pcap
// +build !pcap

package network

import "fmt"

// PCAPCountResult holds the result of counting packets in a PCAP file.
type PCAPCountResult struct {
	Count            uint64
	FirstTimestampNs int64
	LastTimestampNs  int64
}

// CountPCAPPackets is a stub that returns an error when pcap support is not compiled in.
func CountPCAPPackets(pcapFile string, udpPort int) (PCAPCountResult, error) {
	return PCAPCountResult{}, fmt.Errorf("PCAP support not compiled in (requires pcap build tag)")
}
