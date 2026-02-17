//go:build !pcap
// +build !pcap

package network

import (
	"context"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// TestReadPCAPFileRealtime_Stub tests the stub implementation returns an error
func TestReadPCAPFileRealtime_Stub(t *testing.T) {
	ctx := context.Background()
	config := RealtimeReplayConfig{}

	err := ReadPCAPFileRealtime(ctx, "test.pcap", 2368, nil, nil, nil, config)

	if err == nil {
		t.Error("Expected error from stub implementation")
	}

	expectedMsg := "PCAP real-time replay support not compiled in"
	if err != nil && err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Expected error message to start with '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestReadPCAPFileRealtime_Stub_WithConfig tests stub with various configurations
func TestReadPCAPFileRealtime_Stub_WithConfig(t *testing.T) {
	testCases := []struct {
		name   string
		config RealtimeReplayConfig
	}{
		{
			name:   "default config",
			config: RealtimeReplayConfig{},
		},
		{
			name: "with speed multiplier",
			config: RealtimeReplayConfig{
				SpeedMultiplier: 2.0,
			},
		},
		{
			name: "with start and duration",
			config: RealtimeReplayConfig{
				StartSeconds:    10.0,
				DurationSeconds: 30.0,
			},
		},
		{
			name: "with sensor ID",
			config: RealtimeReplayConfig{
				SensorID: "test-sensor",
			},
		},
		{
			name: "with warmup packets",
			config: RealtimeReplayConfig{
				WarmupPackets: 100,
			},
		},
		{
			name: "with debug range",
			config: RealtimeReplayConfig{
				DebugRingMin: 5,
				DebugRingMax: 10,
				DebugAzMin:   45.0,
				DebugAzMax:   90.0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			err := ReadPCAPFileRealtime(ctx, "test.pcap", 2368, nil, nil, nil, tc.config)
			if err == nil {
				t.Error("Expected error from stub implementation")
			}
		})
	}
}

// TestRealtimeReplayConfig_Fields tests that the stub config has the expected fields
func TestRealtimeReplayConfig_Fields(t *testing.T) {
	// This test ensures that the stub's RealtimeReplayConfig matches the real implementation
	config := RealtimeReplayConfig{
		SpeedMultiplier:     1.5,
		StartSeconds:        5.0,
		DurationSeconds:     30.0,
		SensorID:            "test-sensor",
		PacketForwarder:     nil,
		ForegroundForwarder: nil,
		BackgroundManager:   nil,
		WarmupPackets:       100,
		DebugRingMin:        1,
		DebugRingMax:        40,
		DebugAzMin:          0.0,
		DebugAzMax:          360.0,
		OnFrameCallback:     nil,
	}

	// Verify fields are accessible
	if config.SpeedMultiplier != 1.5 {
		t.Errorf("Expected SpeedMultiplier 1.5, got %f", config.SpeedMultiplier)
	}
	if config.SensorID != "test-sensor" {
		t.Errorf("Expected SensorID 'test-sensor', got '%s'", config.SensorID)
	}
	if config.WarmupPackets != 100 {
		t.Errorf("Expected WarmupPackets 100, got %d", config.WarmupPackets)
	}
}

// TestRealtimeReplayConfig_OnFrameCallback tests that the callback field is settable
func TestRealtimeReplayConfig_OnFrameCallback(t *testing.T) {
	callbackCalled := false

	config := RealtimeReplayConfig{
		OnFrameCallback: func(mgr *l3grid.BackgroundManager, points []l4perception.PointPolar) {
			callbackCalled = true
		},
	}

	// Verify callback is set
	if config.OnFrameCallback == nil {
		t.Error("Expected OnFrameCallback to be set")
	}

	// Call it to verify it works
	config.OnFrameCallback(nil, nil)
	if !callbackCalled {
		t.Error("Expected callback to be called")
	}
}
