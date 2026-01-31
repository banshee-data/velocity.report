//go:build !pcap
// +build !pcap

package network

import (
	"context"
	"testing"
)

// TestReadPCAPFile_Stub tests the stub implementation returns an error
func TestReadPCAPFile_Stub(t *testing.T) {
	ctx := context.Background()

	err := ReadPCAPFile(ctx, "test.pcap", 2368, nil, nil, nil, nil, 0, -1)

	if err == nil {
		t.Error("Expected error from stub implementation")
	}

	expectedMsg := "PCAP support not enabled"
	if err != nil && err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Expected error message to start with '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestReadPCAPFile_Stub_WithParameters tests stub with various parameters
func TestReadPCAPFile_Stub_WithParameters(t *testing.T) {
	testCases := []struct {
		name     string
		pcapFile string
		udpPort  int
		startSec float64
		duration float64
	}{
		{"default parameters", "test.pcap", 2368, 0, -1},
		{"with start offset", "test.pcap", 2368, 10.0, -1},
		{"with duration", "test.pcap", 2368, 0, 30.0},
		{"with start and duration", "test.pcap", 2368, 5.0, 10.0},
		{"different port", "test.pcap", 2370, 0, -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			err := ReadPCAPFile(ctx, tc.pcapFile, tc.udpPort, nil, nil, nil, nil, tc.startSec, tc.duration)
			if err == nil {
				t.Error("Expected error from stub implementation")
			}
		})
	}
}
