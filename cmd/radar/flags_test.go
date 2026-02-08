package main

import (
	"testing"
	"time"
)

// TestBgFlushEnableCondition verifies the logic that determines whether
// background flushing should be enabled. This mirrors the condition in radar.go:
//
//	backgroundManager != nil && flushInterval > 0 && flushEnable
func TestBgFlushEnableCondition(t *testing.T) {
	tests := []struct {
		name          string
		hasManager    bool
		flushInterval time.Duration
		flushEnable   bool
		wantEnabled   bool
	}{
		{
			name:          "default settings - flushing disabled",
			hasManager:    true,
			flushInterval: 60 * time.Second,
			flushEnable:   false,
			wantEnabled:   false,
		},
		{
			name:          "enable flag set - flushing enabled",
			hasManager:    true,
			flushInterval: 60 * time.Second,
			flushEnable:   true,
			wantEnabled:   true,
		},
		{
			name:          "zero interval - flushing disabled",
			hasManager:    true,
			flushInterval: 0,
			flushEnable:   true,
			wantEnabled:   false,
		},
		{
			name:          "no manager - flushing disabled",
			hasManager:    false,
			flushInterval: 60 * time.Second,
			flushEnable:   true,
			wantEnabled:   false,
		},
		{
			name:          "all disabled conditions",
			hasManager:    false,
			flushInterval: 0,
			flushEnable:   false,
			wantEnabled:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the condition from radar.go
			enabled := tc.hasManager && tc.flushInterval > 0 && tc.flushEnable

			if enabled != tc.wantEnabled {
				t.Errorf("bgFlushEnabled = %v, want %v", enabled, tc.wantEnabled)
			}
		})
	}
}
