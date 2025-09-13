package main

import (
	"fmt"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/parse"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
) // TestHesaiParser_WithRealPCAP tests the parser with actual PCAP data using gopacket
func TestHesaiParser_WithRealPCAP(t *testing.T) {
	// Path to the PCAP file (absolute path)
	pcapPath := "./lidar_20Hz.pcapng"

	// Create a real Hesai parser with proper calibration
	config := createTestHesaiParserConfig()
	parser := parse.NewPandar40PParser(config)
	parser.SetTimestampMode(parse.TimestampModeSystemTime)
	parser.SetDebug(false)

	// Extract real PCAP data using gopacket
	packets, err := extractPCAPDataUsingGopacket(pcapPath, parser)
	if err != nil {
		t.Fatalf("Failed to extract PCAP data: %v", err)
	}

	if len(packets) == 0 {
		t.Fatal("No packets extracted from PCAP")
	}

	t.Logf("SUCCESS: Extracted %d packets from PCAP using gopacket!", len(packets))
}

// extractPCAPDataUsingGopacket extracts PCAP data using gopacket instead of tshark
func extractPCAPDataUsingGopacket(pcapPath string, parser *parse.Pandar40PParser) ([]lidar.Point, error) {
	handle, err := pcap.OpenOffline(pcapPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PCAP file: %v", err)
	}
	defer handle.Close()

	var allPoints []lidar.Point
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	lidarPackets := 0

	for packet := range packetSource.Packets() {
		udpLayer := packet.Layer(layers.LayerTypeUDP)
		if udpLayer == nil {
			continue
		}

		udp := udpLayer.(*layers.UDP)

		// Check for LiDAR data on port 2369 (Hesai LiDAR data port)
		if udp.DstPort != 2369 && udp.SrcPort != 2369 {
			continue
		}
		lidarPackets++

		lidarData := udp.Payload
		if len(lidarData) == 0 {
			continue
		}

		// Use the production parser to parse the LiDAR data
		points, err := parser.ParsePacket(lidarData)
		if err == nil && len(points) > 0 {
			allPoints = append(allPoints, points...)
		}
	}

	return allPoints, nil
}

// createTestHesaiParserConfig creates a test configuration for the Hesai parser
func createTestHesaiParserConfig() parse.Pandar40PConfig {
	config := parse.Pandar40PConfig{}

	for i := 0; i < 40; i++ {
		config.AngleCorrections[i] = parse.AngleCorrection{
			Channel:   i + 1,
			Elevation: float64(i-20) * 0.8,
			Azimuth:   0.0,
		}
		config.FiretimeCorrections[i] = parse.FiretimeCorrection{
			Channel:  i + 1,
			FireTime: float64(i) * 0.5,
		}
	}

	return config
}
