// Package visualiser provides gRPC streaming of LiDAR perception data.
package visualiser

import (
	"testing"
)

func TestForwardMode_String(t *testing.T) {
	tests := []struct {
		mode     ForwardMode
		expected string
	}{
		{ForwardModeNone, "none"},
		{ForwardModeLidarView, "lidarview"},
		{ForwardModeGRPC, "grpc"},
		{ForwardModeBoth, "both"},
		{ForwardMode(99), "unknown"},
	}

	for _, tc := range tests {
		got := tc.mode.String()
		if got != tc.expected {
			t.Errorf("ForwardMode(%d).String() = %s, want %s", tc.mode, got, tc.expected)
		}
	}
}

func TestParseForwardMode(t *testing.T) {
	tests := []struct {
		input    string
		expected ForwardMode
	}{
		{"none", ForwardModeNone},
		{"lidarview", ForwardModeLidarView},
		{"grpc", ForwardModeGRPC},
		{"both", ForwardModeBoth},
		{"invalid", ForwardModeLidarView}, // defaults to lidarview
		{"", ForwardModeLidarView},        // defaults to lidarview
	}

	for _, tc := range tests {
		got := ParseForwardMode(tc.input)
		if got != tc.expected {
			t.Errorf("ParseForwardMode(%s) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

func TestForwardMode_Constants(t *testing.T) {
	// Ensure constants have expected values
	if ForwardModeNone != 0 {
		t.Errorf("expected ForwardModeNone=0, got %d", ForwardModeNone)
	}
	if ForwardModeLidarView != 1 {
		t.Errorf("expected ForwardModeLidarView=1, got %d", ForwardModeLidarView)
	}
	if ForwardModeGRPC != 2 {
		t.Errorf("expected ForwardModeGRPC=2, got %d", ForwardModeGRPC)
	}
	if ForwardModeBoth != 3 {
		t.Errorf("expected ForwardModeBoth=3, got %d", ForwardModeBoth)
	}
}

func TestDefaultForwardConfig(t *testing.T) {
	cfg := DefaultForwardConfig()

	if cfg.Mode != ForwardModeLidarView {
		t.Errorf("expected Mode=ForwardModeLidarView, got %d", cfg.Mode)
	}
	if cfg.LidarViewAddr != "127.0.0.1:2370" {
		t.Errorf("expected LidarViewAddr=127.0.0.1:2370, got %s", cfg.LidarViewAddr)
	}
	if cfg.GRPCAddr != "localhost:50051" {
		t.Errorf("expected GRPCAddr=localhost:50051, got %s", cfg.GRPCAddr)
	}
	if cfg.EnableDebugOverlays {
		t.Error("expected EnableDebugOverlays=false")
	}
	if cfg.RecordingEnabled {
		t.Error("expected RecordingEnabled=false")
	}
	if cfg.RecordingPath != "" {
		t.Errorf("expected empty RecordingPath, got %s", cfg.RecordingPath)
	}
}

func TestForwardConfig_LidarViewEnabled(t *testing.T) {
	tests := []struct {
		mode     ForwardMode
		expected bool
	}{
		{ForwardModeNone, false},
		{ForwardModeLidarView, true},
		{ForwardModeGRPC, false},
		{ForwardModeBoth, true},
	}

	for _, tc := range tests {
		cfg := ForwardConfig{Mode: tc.mode}
		got := cfg.LidarViewEnabled()
		if got != tc.expected {
			t.Errorf("ForwardConfig{Mode: %d}.LidarViewEnabled() = %v, want %v", tc.mode, got, tc.expected)
		}
	}
}

func TestForwardConfig_GRPCEnabled(t *testing.T) {
	tests := []struct {
		mode     ForwardMode
		expected bool
	}{
		{ForwardModeNone, false},
		{ForwardModeLidarView, false},
		{ForwardModeGRPC, true},
		{ForwardModeBoth, true},
	}

	for _, tc := range tests {
		cfg := ForwardConfig{Mode: tc.mode}
		got := cfg.GRPCEnabled()
		if got != tc.expected {
			t.Errorf("ForwardConfig{Mode: %d}.GRPCEnabled() = %v, want %v", tc.mode, got, tc.expected)
		}
	}
}

func TestForwardConfig_CustomValues(t *testing.T) {
	cfg := ForwardConfig{
		Mode:                ForwardModeBoth,
		LidarViewAddr:       "192.168.1.100:2370",
		GRPCAddr:            "0.0.0.0:50051",
		EnableDebugOverlays: true,
		RecordingEnabled:    true,
		RecordingPath:       "/data/recordings/session1",
	}

	if cfg.Mode != ForwardModeBoth {
		t.Errorf("expected Mode=ForwardModeBoth, got %d", cfg.Mode)
	}
	if !cfg.LidarViewEnabled() {
		t.Error("expected LidarViewEnabled=true")
	}
	if !cfg.GRPCEnabled() {
		t.Error("expected GRPCEnabled=true")
	}
	if !cfg.EnableDebugOverlays {
		t.Error("expected EnableDebugOverlays=true")
	}
	if !cfg.RecordingEnabled {
		t.Error("expected RecordingEnabled=true")
	}
	if cfg.RecordingPath != "/data/recordings/session1" {
		t.Errorf("expected RecordingPath=/data/recordings/session1, got %s", cfg.RecordingPath)
	}
}

func TestForwardMode_Roundtrip(t *testing.T) {
	// Test that parsing a stringified mode returns the same mode
	modes := []ForwardMode{
		ForwardModeNone,
		ForwardModeLidarView,
		ForwardModeGRPC,
		ForwardModeBoth,
	}

	for _, mode := range modes {
		str := mode.String()
		parsed := ParseForwardMode(str)
		if parsed != mode {
			t.Errorf("Roundtrip failed: %d -> %s -> %d", mode, str, parsed)
		}
	}
}
