package l3grid_test

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// TestAliasesCompile verifies that type aliases resolve correctly.
func TestAliasesCompile(t *testing.T) {
	// BackgroundParams alias should be usable as the parent type.
	var p l3grid.BackgroundParams
	p.NeighborConfirmationCount = 5
	if p.NeighborConfirmationCount != 5 {
		t.Fatalf("BackgroundParams alias broken: got %d", p.NeighborConfirmationCount)
	}

	// FrameMetrics alias.
	var m l3grid.FrameMetrics
	m.TotalPoints = 100
	if m.TotalPoints != 100 {
		t.Fatalf("FrameMetrics alias broken: got %d", m.TotalPoints)
	}
}
