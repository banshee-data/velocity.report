package l2frames_test

import (
	"math"
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

func TestApplyPose_Identity(t *testing.T) {
	identity := [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	wx, wy, wz := l2frames.ApplyPose(1.0, 2.0, 3.0, identity)
	if wx != 1.0 || wy != 2.0 || wz != 3.0 {
		t.Errorf("identity pose failed: got (%.2f, %.2f, %.2f), want (1.00, 2.00, 3.00)", wx, wy, wz)
	}
}

func TestApplyPose_Translation(t *testing.T) {
	// Pure translation by (10, 20, 30).
	T := [16]float64{1, 0, 0, 10, 0, 1, 0, 20, 0, 0, 1, 30, 0, 0, 0, 1}
	wx, wy, wz := l2frames.ApplyPose(1.0, 2.0, 3.0, T)
	if wx != 11.0 || wy != 22.0 || wz != 33.0 {
		t.Errorf("translation pose failed: got (%.2f, %.2f, %.2f), want (11.00, 22.00, 33.00)", wx, wy, wz)
	}
}

func TestApplyPose_Scaling(t *testing.T) {
	// Scale by 2 in all axes.
	T := [16]float64{2, 0, 0, 0, 0, 2, 0, 0, 0, 0, 2, 0, 0, 0, 0, 1}
	wx, wy, wz := l2frames.ApplyPose(1.0, 2.0, 3.0, T)
	if wx != 2.0 || wy != 4.0 || wz != 6.0 {
		t.Errorf("scaling pose failed: got (%.2f, %.2f, %.2f), want (2.00, 4.00, 6.00)", wx, wy, wz)
	}
}

func TestApplyPose_90DegRotationZ(t *testing.T) {
	// 90-degree rotation around Z: (1,0,0) -> (0,1,0).
	T := [16]float64{0, -1, 0, 0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	wx, wy, wz := l2frames.ApplyPose(1.0, 0.0, 0.0, T)
	const eps = 1e-9
	if math.Abs(wx-0.0) > eps || math.Abs(wy-1.0) > eps || math.Abs(wz-0.0) > eps {
		t.Errorf("90-deg Z rotation failed: got (%.6f, %.6f, %.6f), want (0.00, 1.00, 0.00)", wx, wy, wz)
	}
}
