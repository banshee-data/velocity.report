package l4perception

import (
	"math"
	"testing"
	"time"
)

func TestEstimateOBBFromCluster_EmptyPoints(t *testing.T) {
	obb := EstimateOBBFromCluster(nil)

	if obb.CenterX != 0 || obb.CenterY != 0 || obb.CenterZ != 0 {
		t.Errorf("Expected zero OBB for empty input, got center=(%.2f, %.2f, %.2f)",
			obb.CenterX, obb.CenterY, obb.CenterZ)
	}
}

func TestEstimateOBBFromCluster_SinglePoint(t *testing.T) {
	points := []WorldPoint{
		{X: 5.0, Y: 10.0, Z: 1.5, Timestamp: time.Now()},
	}

	obb := EstimateOBBFromCluster(points)

	// Center should be at the point
	if obb.CenterX != 5.0 || obb.CenterY != 10.0 || obb.CenterZ != 1.5 {
		t.Errorf("Expected center=(5, 10, 1.5), got (%.2f, %.2f, %.2f)",
			obb.CenterX, obb.CenterY, obb.CenterZ)
	}

	// Extents should be zero (no spread)
	if obb.Length != 0 || obb.Width != 0 || obb.Height != 0 {
		t.Errorf("Expected zero extents for single point, got (%.2f, %.2f, %.2f)",
			obb.Length, obb.Width, obb.Height)
	}
}

func TestEstimateOBBFromCluster_AlignedWithXAxis(t *testing.T) {
	// Points forming a line along X-axis
	points := []WorldPoint{
		{X: 0.0, Y: 5.0, Z: 1.0, Timestamp: time.Now()},
		{X: 1.0, Y: 5.0, Z: 1.0, Timestamp: time.Now()},
		{X: 2.0, Y: 5.0, Z: 1.0, Timestamp: time.Now()},
		{X: 3.0, Y: 5.0, Z: 1.0, Timestamp: time.Now()},
	}

	obb := EstimateOBBFromCluster(points)

	// Center should be midpoint
	expectedCX := float32(1.5)
	expectedCY := float32(5.0)
	if math.Abs(float64(obb.CenterX-expectedCX)) > 0.01 {
		t.Errorf("Expected CenterX≈%.2f, got %.2f", expectedCX, obb.CenterX)
	}
	if math.Abs(float64(obb.CenterY-expectedCY)) > 0.01 {
		t.Errorf("Expected CenterY≈%.2f, got %.2f", expectedCY, obb.CenterY)
	}

	// Length should be extent along X (3.0 units)
	if math.Abs(float64(obb.Length-3.0)) > 0.01 {
		t.Errorf("Expected Length≈3.0, got %.2f", obb.Length)
	}

	// Width should be near zero (collinear points)
	if obb.Width > 0.1 {
		t.Errorf("Expected Width≈0 for collinear points, got %.2f", obb.Width)
	}

	// Heading should be 0 (aligned with X-axis)
	if math.Abs(float64(obb.HeadingRad)) > 0.1 {
		t.Errorf("Expected HeadingRad≈0, got %.2f", obb.HeadingRad)
	}
}

func TestEstimateOBBFromCluster_AlignedWith45Degrees(t *testing.T) {
	// Points forming a line at 45° angle
	points := []WorldPoint{
		{X: 0.0, Y: 0.0, Z: 1.0, Timestamp: time.Now()},
		{X: 1.0, Y: 1.0, Z: 1.0, Timestamp: time.Now()},
		{X: 2.0, Y: 2.0, Z: 1.0, Timestamp: time.Now()},
		{X: 3.0, Y: 3.0, Z: 1.0, Timestamp: time.Now()},
	}

	obb := EstimateOBBFromCluster(points)

	// Heading should be approximately π/4 (45 degrees)
	expected45Deg := math.Pi / 4
	if math.Abs(float64(obb.HeadingRad)-expected45Deg) > 0.1 {
		t.Errorf("Expected HeadingRad≈%.2f (45°), got %.2f", expected45Deg, obb.HeadingRad)
	}

	// Length should be sqrt(2) * 3 ≈ 4.24 (diagonal distance)
	expectedLength := float32(math.Sqrt(2) * 3)
	if math.Abs(float64(obb.Length-expectedLength)) > 0.2 {
		t.Errorf("Expected Length≈%.2f, got %.2f", expectedLength, obb.Length)
	}
}

func TestEstimateOBBFromCluster_Rectangle(t *testing.T) {
	// Points forming a rectangle: 4m long, 2m wide
	points := []WorldPoint{
		{X: 0.0, Y: 0.0, Z: 1.0, Timestamp: time.Now()},
		{X: 4.0, Y: 0.0, Z: 1.0, Timestamp: time.Now()},
		{X: 0.0, Y: 2.0, Z: 1.0, Timestamp: time.Now()},
		{X: 4.0, Y: 2.0, Z: 1.0, Timestamp: time.Now()},
		{X: 2.0, Y: 1.0, Z: 1.0, Timestamp: time.Now()}, // center point for better PCA
	}

	obb := EstimateOBBFromCluster(points)

	// Length should be the longer dimension (4m)
	// Width should be the shorter dimension (2m)
	// Note: PCA will pick the axis with most variation as "length"
	maxDim := math.Max(float64(obb.Length), float64(obb.Width))
	minDim := math.Min(float64(obb.Length), float64(obb.Width))

	if math.Abs(maxDim-4.0) > 0.5 {
		t.Errorf("Expected max dimension≈4.0, got %.2f", maxDim)
	}
	if math.Abs(minDim-2.0) > 0.5 {
		t.Errorf("Expected min dimension≈2.0, got %.2f", minDim)
	}
}

func TestEstimateOBBFromCluster_VerticalExtent(t *testing.T) {
	// Points with varying Z to test height calculation
	points := []WorldPoint{
		{X: 0.0, Y: 0.0, Z: 0.5, Timestamp: time.Now()},
		{X: 1.0, Y: 0.0, Z: 1.0, Timestamp: time.Now()},
		{X: 0.5, Y: 0.0, Z: 2.5, Timestamp: time.Now()},
		{X: 0.0, Y: 1.0, Z: 1.5, Timestamp: time.Now()},
	}

	obb := EstimateOBBFromCluster(points)

	// Height should be max Z - min Z = 2.5 - 0.5 = 2.0
	expectedHeight := float32(2.0)
	if math.Abs(float64(obb.Height-expectedHeight)) > 0.01 {
		t.Errorf("Expected Height=%.2f, got %.2f", expectedHeight, obb.Height)
	}

	// CenterZ should be min Z (ground plane) = 0.5
	expectedCZ := float32(0.5)
	if math.Abs(float64(obb.CenterZ-expectedCZ)) > 0.01 {
		t.Errorf("Expected CenterZ=%.2f, got %.2f", expectedCZ, obb.CenterZ)
	}
}

func TestSmoothOBBHeading_NoChange(t *testing.T) {
	prev := float32(0.5)
	new := float32(0.5)
	alpha := float32(0.3)

	smoothed := SmoothOBBHeading(prev, new, alpha)

	if smoothed != prev {
		t.Errorf("Expected unchanged heading, got %.4f", smoothed)
	}
}

func TestSmoothOBBHeading_SmallChange(t *testing.T) {
	prev := float32(0.0)
	new := float32(0.2)
	alpha := float32(0.3)

	smoothed := SmoothOBBHeading(prev, new, alpha)

	// Should be prev + alpha * (new - prev) = 0 + 0.3 * 0.2 = 0.06
	expected := float32(0.06)
	if math.Abs(float64(smoothed-expected)) > 0.001 {
		t.Errorf("Expected %.4f, got %.4f", expected, smoothed)
	}
}

func TestSmoothOBBHeading_Wraparound_PositiveToNegative(t *testing.T) {
	// Heading near +π wrapping to -π
	prev := float32(3.0)  // close to π
	new := float32(-3.0)  // close to -π
	alpha := float32(0.5) // 50% smoothing

	smoothed := SmoothOBBHeading(prev, new, alpha)

	// Shortest angular distance from 3.0 to -3.0 is actually small (both near π)
	// The algorithm should take the short path around the circle
	// Final result should be near ±π
	if math.Abs(float64(math.Abs(float64(smoothed))-math.Pi)) > 0.5 {
		t.Errorf("Expected result near ±π, got %.4f", smoothed)
	}
}

func TestSmoothOBBHeading_Wraparound_NegativeToPositive(t *testing.T) {
	// Heading near -π wrapping to +π
	prev := float32(-3.0)
	new := float32(3.0)
	alpha := float32(0.5)

	smoothed := SmoothOBBHeading(prev, new, alpha)

	// Should wrap correctly around ±π boundary
	if math.Abs(float64(math.Abs(float64(smoothed))-math.Pi)) > 0.5 {
		t.Errorf("Expected result near ±π, got %.4f", smoothed)
	}
}

func TestSmoothOBBHeading_AlphaZero(t *testing.T) {
	// Alpha = 0 means no smoothing (keep previous value)
	prev := float32(1.0)
	new := float32(2.0)
	alpha := float32(0.0)

	smoothed := SmoothOBBHeading(prev, new, alpha)

	if smoothed != prev {
		t.Errorf("Expected prev value %.2f with alpha=0, got %.2f", prev, smoothed)
	}
}

func TestSmoothOBBHeading_AlphaOne(t *testing.T) {
	// Alpha = 1 means no smoothing (use new value)
	prev := float32(1.0)
	new := float32(2.0)
	alpha := float32(1.0)

	smoothed := SmoothOBBHeading(prev, new, alpha)

	if math.Abs(float64(smoothed-new)) > 0.001 {
		t.Errorf("Expected new value %.2f with alpha=1, got %.2f", new, smoothed)
	}
}

func TestSmoothOBBHeading_OutputRange(t *testing.T) {
	// Test that output is always in [-π, π]
	testCases := []struct {
		prev, new float32
	}{
		{0.0, 6.0},
		{6.0, 0.0},
		{-6.0, 6.0},
		{math.Pi * 2, 0.0},
		{-math.Pi * 2, 0.0},
	}

	alpha := float32(0.5)

	for _, tc := range testCases {
		smoothed := SmoothOBBHeading(tc.prev, tc.new, alpha)
		if smoothed < -math.Pi || smoothed > math.Pi {
			t.Errorf("Output %.4f outside range [-π, π] for prev=%.2f, new=%.2f",
				smoothed, tc.prev, tc.new)
		}
	}
}

func TestEstimateOBBFromCluster_NearSquareClusterPreservesNaturalAxes(t *testing.T) {
	// Verify that near-square clusters preserve PCA's natural axis assignment
	// without canonical-axis normalisation. This is important for pedestrians
	// whose bounding boxes are legitimately square or near-square.
	points := []WorldPoint{
		{X: 0, Y: 0, Z: 0, Timestamp: time.Now()},
		{X: 2.0, Y: 0, Z: 0, Timestamp: time.Now()},
		{X: 0, Y: 2.1, Z: 0, Timestamp: time.Now()},
		{X: 2.0, Y: 2.1, Z: 0, Timestamp: time.Now()},
		{X: 1.0, Y: 1.05, Z: 0, Timestamp: time.Now()},
	}

	obb := EstimateOBBFromCluster(points)

	// PCA should find Y as the principal axis (slightly more variance).
	// Without canonical-axis normalisation, Length follows the principal axis.
	// We verify the overall extents are correct (both ~2.0m) regardless of
	// which is labelled Length vs Width.
	maxDim := math.Max(float64(obb.Length), float64(obb.Width))
	minDim := math.Min(float64(obb.Length), float64(obb.Width))
	if math.Abs(maxDim-2.1) > 0.3 {
		t.Errorf("Expected max dimension ≈ 2.1, got %.2f", maxDim)
	}
	if math.Abs(minDim-2.0) > 0.3 {
		t.Errorf("Expected min dimension ≈ 2.0, got %.2f", minDim)
	}

	// Heading should be near 0 or ±π/2 — the PCA principal axis
	if math.Abs(float64(obb.HeadingRad)) > 0.3 &&
		math.Abs(math.Abs(float64(obb.HeadingRad))-math.Pi/2) > 0.3 {
		t.Errorf("Expected heading near 0 or ±π/2, got %.2f rad", obb.HeadingRad)
	}
}

func TestEstimateOBBFromCluster_RectangleDimensionsCorrect(t *testing.T) {
	// Verify that rectangles produce correct dimension values
	// regardless of whether Length > Width or vice versa.
	testCases := []struct {
		name   string
		points []WorldPoint
	}{
		{
			name: "rectangle longer in X",
			points: []WorldPoint{
				{X: 0, Y: 0, Z: 0, Timestamp: time.Now()},
				{X: 4, Y: 0, Z: 0, Timestamp: time.Now()},
				{X: 0, Y: 2, Z: 0, Timestamp: time.Now()},
				{X: 4, Y: 2, Z: 0, Timestamp: time.Now()},
				{X: 2, Y: 1, Z: 0, Timestamp: time.Now()},
			},
		},
		{
			name: "rectangle longer in Y",
			points: []WorldPoint{
				{X: 0, Y: 0, Z: 0, Timestamp: time.Now()},
				{X: 2, Y: 0, Z: 0, Timestamp: time.Now()},
				{X: 0, Y: 4, Z: 0, Timestamp: time.Now()},
				{X: 2, Y: 4, Z: 0, Timestamp: time.Now()},
				{X: 1, Y: 2, Z: 0, Timestamp: time.Now()},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			obb := EstimateOBBFromCluster(tc.points)
			maxDim := math.Max(float64(obb.Length), float64(obb.Width))
			minDim := math.Min(float64(obb.Length), float64(obb.Width))

			if math.Abs(maxDim-4.0) > 0.5 {
				t.Errorf("Expected max dimension ≈ 4.0, got %.2f", maxDim)
			}
			if math.Abs(minDim-2.0) > 0.5 {
				t.Errorf("Expected min dimension ≈ 2.0, got %.2f", minDim)
			}
		})
	}
}
