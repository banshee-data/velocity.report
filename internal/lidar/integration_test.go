//go:build pcap
// +build pcap

package lidar_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// TestHesaiLiDAR_PCAPIntegration is the comprehensive integration test that:
// 1. Parses real PCAP data using gopacket
// 2. Streams packets directly into the frame builder (no unbounded memory)
// 3. Validates the entire pipeline works end-to-end
//
// This test MUST stream data through the frame builder rather than collecting
// all points into a slice. The 99MB 20Hz PCAP produces ~26.7M points; with
// race instrumentation the accumulated slice would consume multi-GB and OOM
// CI runners. Streaming matches the production data flow.
func TestHesaiLiDAR_PCAPIntegration(t *testing.T) {
	pcapPath := filepath.Join("perf", "pcap", "lidar_20Hz.pcapng")

	// Step 1: Set up Hesai parser
	config := createTestHesaiParserConfig()
	parser := parse.NewPandar40PParser(config)
	parser.SetTimestampMode(parse.TimestampModeSystemTime)
	parser.SetDebug(false)

	// Step 2: Set up frame builder with callback
	var (
		completedFrames []*lidar.LiDARFrame
		completedMu     sync.Mutex
	)
	frameConfig := lidar.FrameBuilderConfig{
		SensorID: "hesai-pandar40p-integration-test",
		FrameCallback: func(frame *lidar.LiDARFrame) {
			completedMu.Lock()
			completedFrames = append(completedFrames, frame)
			completedMu.Unlock()
			t.Logf("Frame completed: %s, %d points, %.1f°-%.1f° azimuth, %v duration",
				frame.FrameID, frame.PointCount, frame.MinAzimuth, frame.MaxAzimuth,
				frame.EndTimestamp.Sub(frame.StartTimestamp))
		},
		EnableTimeBased:       false,
		ExpectedFrameDuration: 17 * time.Millisecond,
		MinFramePoints:        1000,
		BufferTimeout:         100 * time.Millisecond,
		CleanupInterval:       50 * time.Millisecond,
	}
	frameBuilder := lidar.NewFrameBuilder(frameConfig)

	// Step 3: Stream PCAP packets directly into the frame builder
	t.Log("Streaming PCAP packets into frame builder...")
	totalPoints, err := streamPCAPIntoFrameBuilder(pcapPath, parser, frameBuilder)
	if err != nil {
		t.Fatalf("Failed to stream PCAP data: %v", err)
	}

	if totalPoints == 0 {
		t.Fatal("No LiDAR points extracted from PCAP")
	}
	t.Logf("Streamed %d points from PCAP into frame builder", totalPoints)

	// Step 4: Wait for frame processing to complete
	time.Sleep(500 * time.Millisecond)

	// Step 5: Validate the integration results
	completedMu.Lock()
	framesCopy := append([]*lidar.LiDARFrame(nil), completedFrames...)
	completedMu.Unlock()

	if len(framesCopy) == 0 {
		t.Fatal("No frames were built from PCAP data")
	}

	t.Logf("Successfully built %d complete frames from PCAP data", len(framesCopy))

	totalFramePoints := 0
	for i, frame := range framesCopy {
		if frame.PointCount == 0 {
			t.Errorf("Frame %d has no points", i)
			continue
		}
		totalFramePoints += frame.PointCount

		azimuthSpan := frame.MaxAzimuth - frame.MinAzimuth
		if azimuthSpan < 0 {
			azimuthSpan += 360
		}
		frameDuration := frame.EndTimestamp.Sub(frame.StartTimestamp)
		t.Logf("Frame %d: %d points, %.1f° coverage, %v duration",
			i, frame.PointCount, azimuthSpan, frameDuration)
	}

	pointsUsedRatio := float64(totalFramePoints) / float64(totalPoints)
	t.Logf("Integration efficiency: %d/%d points used in frames (%.1f%%)",
		totalFramePoints, totalPoints, pointsUsedRatio*100)

	t.Log("INTEGRATION TEST PASSED: PCAP streaming + Frame building working correctly!")
}

// streamPCAPIntoFrameBuilder streams PCAP packets directly into the frame
// builder without accumulating points. This keeps memory bounded regardless
// of PCAP file size.
func streamPCAPIntoFrameBuilder(pcapPath string, parser *parse.Pandar40PParser, fb *lidar.FrameBuilder) (int, error) {
	handle, err := pcap.OpenOffline(pcapPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open PCAP file: %v", err)
	}
	defer handle.Close()

	totalPoints := 0
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
		if len(udp.Payload) == 0 {
			continue
		}

		parsedPoints, err := parser.ParsePacket(udp.Payload)
		if err != nil || len(parsedPoints) == 0 {
			continue
		}

		// Feed directly to frame builder — no intermediate storage
		fb.AddPointsPolar(parsedPoints)
		totalPoints += len(parsedPoints)
	}

	return totalPoints, nil
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
