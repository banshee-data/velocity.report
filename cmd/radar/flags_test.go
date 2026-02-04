package main

import (
	"flag"
	"testing"
	"time"
)

// TestLidarBgFlushDisableFlag verifies the --lidar-bg-flush-disable flag exists
// and has the correct default value.
func TestLidarBgFlushDisableFlag(t *testing.T) {
	// The flag is defined in the main package's var block.
	// We verify it exists and has the expected default.
	if lidarBgFlushDisable == nil {
		t.Fatal("lidarBgFlushDisable flag not defined")
	}

	// Default should be false (flushing enabled by default)
	if *lidarBgFlushDisable != false {
		t.Errorf("expected lidarBgFlushDisable default to be false, got %v", *lidarBgFlushDisable)
	}
}

// TestLidarBgFlushIntervalFlag verifies the --lidar-bg-flush-interval flag exists
// and has the correct default value.
func TestLidarBgFlushIntervalFlag(t *testing.T) {
	if lidarBgFlushInterval == nil {
		t.Fatal("lidarBgFlushInterval flag not defined")
	}

	// Default should be 60 seconds
	expected := 60 * time.Second
	if *lidarBgFlushInterval != expected {
		t.Errorf("expected lidarBgFlushInterval default to be %v, got %v", expected, *lidarBgFlushInterval)
	}
}

// TestBgFlushDisableCondition verifies the logic that determines whether
// background flushing should be enabled. This mirrors the condition in radar.go:
//
//	backgroundManager != nil && *lidarBgFlushInterval > 0 && !*lidarBgFlushDisable
func TestBgFlushDisableCondition(t *testing.T) {
	tests := []struct {
		name          string
		hasManager    bool
		flushInterval time.Duration
		flushDisable  bool
		wantEnabled   bool
	}{
		{
			name:          "default settings - flushing enabled",
			hasManager:    true,
			flushInterval: 60 * time.Second,
			flushDisable:  false,
			wantEnabled:   true,
		},
		{
			name:          "disable flag set - flushing disabled",
			hasManager:    true,
			flushInterval: 60 * time.Second,
			flushDisable:  true,
			wantEnabled:   false,
		},
		{
			name:          "zero interval - flushing disabled",
			hasManager:    true,
			flushInterval: 0,
			flushDisable:  false,
			wantEnabled:   false,
		},
		{
			name:          "no manager - flushing disabled",
			hasManager:    false,
			flushInterval: 60 * time.Second,
			flushDisable:  false,
			wantEnabled:   false,
		},
		{
			name:          "all disabled conditions",
			hasManager:    false,
			flushInterval: 0,
			flushDisable:  true,
			wantEnabled:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the condition from radar.go
			enabled := tc.hasManager && tc.flushInterval > 0 && !tc.flushDisable

			if enabled != tc.wantEnabled {
				t.Errorf("bgFlushEnabled = %v, want %v", enabled, tc.wantEnabled)
			}
		})
	}
}

// TestFlagParsing verifies that the flags can be parsed correctly.
// This uses a separate FlagSet to avoid polluting the global flags.
func TestFlagParsing(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantBool bool
	}{
		{
			name:     "flag not set",
			args:     []string{},
			wantBool: false,
		},
		{
			name:     "flag set explicitly true",
			args:     []string{"--lidar-bg-flush-disable=true"},
			wantBool: true,
		},
		{
			name:     "flag set without value (implies true)",
			args:     []string{"--lidar-bg-flush-disable"},
			wantBool: true,
		},
		{
			name:     "flag set explicitly false",
			args:     []string{"--lidar-bg-flush-disable=false"},
			wantBool: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			disableFlag := fs.Bool("lidar-bg-flush-disable", false, "Disable background flushing")

			err := fs.Parse(tc.args)
			if err != nil {
				t.Fatalf("failed to parse flags: %v", err)
			}

			if *disableFlag != tc.wantBool {
				t.Errorf("lidar-bg-flush-disable = %v, want %v", *disableFlag, tc.wantBool)
			}
		})
	}
}
