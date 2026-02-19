package l5tracks_test

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// TestAliasesCompile verifies that type aliases resolve correctly.
func TestAliasesCompile(t *testing.T) {
	// TrackerConfig alias.
	cfg := l5tracks.DefaultTrackerConfig()
	if cfg.GatingDistanceSquared <= 0 {
		t.Fatalf("DefaultTrackerConfig returned invalid gating distance: %f", cfg.GatingDistanceSquared)
	}

	// NewTracker alias.
	tracker := l5tracks.NewTracker(cfg)
	if tracker == nil {
		t.Fatal("NewTracker returned nil")
	}

	// HungarianAssign alias.
	cost := [][]float32{{1, 2}, {2, 1}}
	assignment := l5tracks.HungarianAssign(cost)
	if len(assignment) != 2 {
		t.Fatalf("HungarianAssign returned wrong length: %d", len(assignment))
	}
}
