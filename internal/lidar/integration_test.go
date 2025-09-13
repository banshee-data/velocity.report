package lidar_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/parse"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// TestHesaiLiDAR_PCAPIntegration is the comprehensive integration test that:
// 1. Parses real PCAP data using gopacket
// 2. Uses the production Hesai parser to extract points
// 3. Builds complete frames using the frame builder
// 4. Validates the entire pipeline works end-to-end
func TestHesaiLiDAR_PCAPIntegration(t *testing.T) {
	// Path to the PCAP file (in testdata directory)
	pcapPath := filepath.Join("testdata", "lidar_20Hz.pcapng")

	// Step 1: Set up Hesai parser with realistic configuration
	config := createTestHesaiParserConfig()
	parser := parse.NewPandar40PParser(config)
	parser.SetTimestampMode(parse.TimestampModeSystemTime)
	parser.SetDebug(false)

	// Step 2: Extract real PCAP data using gopacket (replaces tshark)
	t.Log("Extracting LiDAR data from PCAP using gopacket...")
	allPoints, err := extractPCAPDataUsingGopacket(pcapPath, parser)
	if err != nil {
		t.Fatalf("Failed to extract PCAP data: %v", err)
	}

	if len(allPoints) == 0 {
		t.Fatal("No LiDAR points extracted from PCAP")
	}

	t.Logf("Successfully extracted %d LiDAR points from PCAP using gopacket", len(allPoints))

	// Step 3: Analyze extracted data for frame building validation
	minAzimuth, maxAzimuth := 360.0, 0.0
	startTime, endTime := allPoints[0].Timestamp, allPoints[0].Timestamp

	for _, point := range allPoints {
		if point.Azimuth < minAzimuth {
			minAzimuth = point.Azimuth
		}
		if point.Azimuth > maxAzimuth {
			maxAzimuth = point.Azimuth
		}
		if point.Timestamp.Before(startTime) {
			startTime = point.Timestamp
		}
		if point.Timestamp.After(endTime) {
			endTime = point.Timestamp
		}
	}

	totalDuration := endTime.Sub(startTime)
	t.Logf("PCAP data analysis: %.1f° - %.1f° azimuth range, %v total duration",
		minAzimuth, maxAzimuth, totalDuration)

	// Step 4: Set up frame builder to process the extracted points
	var completedFrames []*lidar.LiDARFrame

	frameConfig := lidar.FrameBuilderConfig{
		SensorID: "hesai-pandar40p-integration-test",
		FrameCallback: func(frame *lidar.LiDARFrame) {
			completedFrames = append(completedFrames, frame)
			t.Logf("Frame completed: %s, %d points, %.1f° - %.1f° azimuth, %v duration",
				frame.FrameID, frame.PointCount, frame.MinAzimuth, frame.MaxAzimuth,
				frame.EndTimestamp.Sub(frame.StartTimestamp))
		},
		EnableTimeBased:       false,                  // Start with traditional azimuth-based detection
		ExpectedFrameDuration: 17 * time.Millisecond,  // Based on PCAP analysis: 16.5ms total
		MinFramePoints:        1000,                   // Lower threshold for real data
		BufferTimeout:         100 * time.Millisecond, // Shorter timeout
		CleanupInterval:       50 * time.Millisecond,  // Faster cleanup
	}

	frameBuilder := lidar.NewFrameBuilder(frameConfig)

	// Step 5: Process all extracted points through the frame builder
	t.Log("Processing points through frame builder...")
	frameBuilder.AddPoints(allPoints)

	// Step 6: Wait for frame processing to complete
	time.Sleep(500 * time.Millisecond)

	// Step 7: Validate the integration results
	if len(completedFrames) == 0 {
		t.Fatal("No frames were built from PCAP data")
	}

	t.Logf("Successfully built %d complete frames from PCAP data", len(completedFrames))

	// Validate frame quality
	for i, frame := range completedFrames {
		if frame.PointCount == 0 {
			t.Errorf("Frame %d has no points", i)
			continue
		}

		azimuthSpan := frame.MaxAzimuth - frame.MinAzimuth
		if azimuthSpan < 0 {
			azimuthSpan += 360
		}

		frameDuration := frame.EndTimestamp.Sub(frame.StartTimestamp)
		t.Logf("Frame %d: %d points, %.1f° coverage, %v duration",
			i, frame.PointCount, azimuthSpan, frameDuration)
	}

	// Step 8: Final integration validation
	totalFramePoints := 0
	for _, frame := range completedFrames {
		totalFramePoints += frame.PointCount
	}

	pointsUsedRatio := float64(totalFramePoints) / float64(len(allPoints))
	t.Logf("Integration efficiency: %d/%d points used in frames (%.1f%%)",
		totalFramePoints, len(allPoints), pointsUsedRatio*100)

	t.Log("INTEGRATION TEST PASSED: PCAP parsing + Frame building working correctly!")
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

	for packet := range packetSource.Packets() {
		udpLayer := packet.Layer(layers.LayerTypeUDP)
		if udpLayer == nil {
			continue
		}

		udp := udpLayer.(*layers.UDP)

		if udp.DstPort != 2369 && udp.SrcPort != 2369 {
			continue
		}

		lidarData := udp.Payload
		if len(lidarData) == 0 {
			continue
		}

		parsedPoints, err := parser.ParsePacket(lidarData)
		if err == nil && len(parsedPoints) > 0 {
			// Convert parsed points to lidar.Point type
			for _, point := range parsedPoints {
				lidarPoint := lidar.Point{
					X:         point.X,
					Y:         point.Y,
					Z:         point.Z,
					Intensity: point.Intensity,
					Azimuth:   point.Azimuth,
					Elevation: point.Elevation,
					Distance:  point.Distance,
					Timestamp: point.Timestamp,
					Channel:   point.Channel,
				}
				allPoints = append(allPoints, lidarPoint)
			}
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
