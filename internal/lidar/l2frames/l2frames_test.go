package l2frames_test

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// TestAliasesCompile verifies that type aliases resolve correctly.
func TestAliasesCompile(t *testing.T) {
	// PointPolar alias.
	var pp l2frames.PointPolar
	pp.Distance = 10.0
	if pp.Distance != 10.0 {
		t.Fatalf("PointPolar alias broken")
	}

	// SphericalToCartesian alias.
	x, y, z := l2frames.SphericalToCartesian(1.0, 0.0, 0.0)
	if x == 0 && y == 0 && z == 0 {
		t.Fatal("SphericalToCartesian returned zero for non-zero input")
	}
}
