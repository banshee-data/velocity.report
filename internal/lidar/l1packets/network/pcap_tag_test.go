//go:build pcap
// +build pcap

package network

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

const samplePCAPPath = "../testdata/lidar_20Hz.pcapng"

type replayTestParser struct {
	parseCalls int
	motorSpeed uint16
	lastTS     time.Time
	parseErr   error
}

func (p *replayTestParser) ParsePacket(_ []byte) ([]l4perception.PointPolar, error) {
	p.parseCalls++
	if p.parseErr != nil {
		return nil, p.parseErr
	}
	return []l4perception.PointPolar{
		{
			Channel:         1,
			Azimuth:         90,
			Elevation:       0,
			Distance:        5.0,
			Intensity:       80,
			Timestamp:       time.Now().UnixNano(),
			BlockID:         0,
			RawBlockAzimuth: 9000,
		},
	}, nil
}

func (p *replayTestParser) GetLastMotorSpeed() uint16 {
	return p.motorSpeed
}

func (p *replayTestParser) SetPacketTime(ts time.Time) {
	p.lastTS = ts
}

func TestCountPCAPPackets_WithTag(t *testing.T) {
	result, err := CountPCAPPackets(samplePCAPPath, 2369)
	if err != nil {
		t.Fatalf("CountPCAPPackets failed: %v", err)
	}
	if result.Count == 0 {
		t.Fatal("expected non-zero packet count for sample PCAP on port 2369")
	}
	if result.FirstTimestampNs <= 0 {
		t.Fatalf("expected first timestamp > 0, got %d", result.FirstTimestampNs)
	}
	if result.LastTimestampNs < result.FirstTimestampNs {
		t.Fatalf("last timestamp (%d) should be >= first (%d)", result.LastTimestampNs, result.FirstTimestampNs)
	}

	none, err := CountPCAPPackets(samplePCAPPath, 2368)
	if err != nil {
		t.Fatalf("CountPCAPPackets on non-matching port failed: %v", err)
	}
	if none.Count != 0 {
		t.Fatalf("expected zero packets on unmatched port, got %d", none.Count)
	}
}

func TestCountPCAPPackets_Errors(t *testing.T) {
	_, err := CountPCAPPackets("does-not-exist.pcapng", 2369)
	if err == nil {
		t.Fatal("expected error for missing PCAP file")
	}

	_, err = CountPCAPPackets(samplePCAPPath, -1)
	if err == nil {
		t.Fatal("expected BPF error for invalid UDP port")
	}
}

func TestReadPCAPFile_WithTag_Success(t *testing.T) {
	parser := &replayTestParser{motorSpeed: 600}
	frameBuilder := &MockFrameBuilder{}
	stats := &MockFullPacketStats{}

	countResult, err := CountPCAPPackets(samplePCAPPath, 2369)
	if err != nil {
		t.Fatalf("CountPCAPPackets failed: %v", err)
	}

	progressCalls := 0
	err = ReadPCAPFile(
		context.Background(),
		samplePCAPPath,
		2369,
		parser,
		frameBuilder,
		stats,
		nil,
		0,
		-1,
		0,
		countResult.Count,
		func(_, _ uint64) { progressCalls++ },
	)
	if err != nil {
		t.Fatalf("ReadPCAPFile failed: %v", err)
	}

	if parser.parseCalls == 0 {
		t.Fatal("expected parser to be called")
	}
	if frameBuilder.addCalled == 0 {
		t.Fatal("expected frame builder to receive points")
	}
	if frameBuilder.speedCalled == 0 {
		t.Fatal("expected frame builder motor speed updates")
	}
	if stats.GetPacketCount() == 0 {
		t.Fatal("expected packet stats to be updated")
	}
	if progressCalls == 0 {
		t.Fatal("expected progress callback to be called")
	}
	if parser.lastTS.IsZero() {
		t.Fatal("expected parser SetPacketTime to receive capture timestamps")
	}
}

func TestReadPCAPFile_WithTag_OffsetAndDuration(t *testing.T) {
	parser := &replayTestParser{motorSpeed: 600}
	stats := &MockFullPacketStats{}
	frameBuilder := &MockFrameBuilder{}

	err := ReadPCAPFile(
		context.Background(),
		samplePCAPPath,
		2369,
		parser,
		frameBuilder,
		stats,
		nil,
		0.01, // start offset
		0.02, // duration
		20,   // packet offset
		0,
		nil,
	)
	if err != nil {
		t.Fatalf("ReadPCAPFile with offsets failed: %v", err)
	}
	if parser.parseCalls == 0 {
		t.Fatal("expected parser calls with offset+duration replay")
	}
}

func TestReadPCAPFile_WithTag_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ReadPCAPFile(ctx, samplePCAPPath, 2369, nil, nil, nil, nil, 0, -1, 0, 0, nil)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancellation error, got %v", err)
	}
}

func TestReadPCAPFileRealtime_WithTag_Success(t *testing.T) {
	parser := &replayTestParser{motorSpeed: 600}
	stats := &MockFullPacketStats{}
	// Use a unique sensor ID to avoid cross-test registry pollution
	sensorID := "sensor-replay-" + t.Name()
	bgManager := l3grid.NewBackgroundManager(sensorID, 40, 360, l3grid.BackgroundParams{}, nil)

	foregroundForwarder := &ForegroundForwarder{
		channel: make(chan []l4perception.PointPolar, 8),
	}

	countResult, err := CountPCAPPackets(samplePCAPPath, 2369)
	if err != nil {
		t.Fatalf("CountPCAPPackets failed: %v", err)
	}

	progressCalls := 0
	frameCallbacks := 0

	err = ReadPCAPFileRealtime(
		context.Background(),
		samplePCAPPath,
		2369,
		parser,
		nil, // keep nil so BackgroundManager path in replay is exercised
		stats,
		RealtimeReplayConfig{
			SpeedMultiplier:     10.0, // Use high multiplier to avoid slow test
			DurationSeconds:     -1,
			SensorID:            sensorID,
			BackgroundManager:   bgManager,
			ForegroundForwarder: foregroundForwarder,
			WarmupPackets:       1,
			TotalPackets:        countResult.Count,
			OnProgress: func(_, _ uint64) {
				progressCalls++
			},
			OnFrameCallback: func(_ *l3grid.BackgroundManager, _ []l4perception.PointPolar) {
				frameCallbacks++
			},
		},
	)
	if err != nil {
		t.Fatalf("ReadPCAPFileRealtime failed: %v", err)
	}

	if parser.parseCalls == 0 {
		t.Fatal("expected parser calls")
	}
	if stats.GetPacketCount() == 0 {
		t.Fatal("expected packet stats to be updated")
	}
	if frameCallbacks == 0 {
		t.Fatal("expected frame callback to be called")
	}
	if progressCalls == 0 {
		t.Fatal("expected replay progress callbacks")
	}
}

func TestReadPCAPFileRealtime_WithTag_ErrorsAndCancel(t *testing.T) {
	err := ReadPCAPFileRealtime(context.Background(), "does-not-exist.pcapng", 2369, nil, nil, nil, RealtimeReplayConfig{})
	if err == nil {
		t.Fatal("expected error for missing PCAP file")
	}

	_, err = CountPCAPPackets(samplePCAPPath, 2369)
	if err != nil {
		t.Fatalf("count sample pcap: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = ReadPCAPFileRealtime(ctx, samplePCAPPath, 2369, nil, nil, nil, RealtimeReplayConfig{})
	if err == nil {
		t.Fatal("expected cancellation error")
	}

	err = ReadPCAPFileRealtime(context.Background(), samplePCAPPath, -1, nil, nil, nil, RealtimeReplayConfig{})
	if err == nil {
		t.Fatal("expected BPF filter error for invalid UDP port")
	}
}
