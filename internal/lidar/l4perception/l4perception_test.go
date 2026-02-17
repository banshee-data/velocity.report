package l4perception_test

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// TestAliasesCompile verifies that type aliases resolve correctly.
func TestAliasesCompile(t *testing.T) {
	// WorldPoint alias should be usable.
	var wp l4perception.WorldPoint
	wp.X = 1.0
	wp.Y = 2.0
	wp.Z = 3.0
	if wp.X != 1.0 {
		t.Fatalf("WorldPoint alias broken")
	}

	// DBSCANParams alias.
	p := l4perception.DefaultDBSCANParams()
	if p.Eps <= 0 {
		t.Fatalf("DefaultDBSCANParams returned invalid Eps: %f", p.Eps)
	}

	// HeightBandFilter alias.
	f := l4perception.DefaultHeightBandFilter()
	if f == nil {
		t.Fatal("DefaultHeightBandFilter returned nil")
	}
}
