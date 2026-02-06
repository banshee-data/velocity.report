package lidar

import (
	"math"
	"testing"
	"time"
)

func TestExtractClusterFeatures_Basic(t *testing.T) {
	cluster := WorldCluster{
		PointsCount:       10,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		HeightP95:         1.4,
		IntensityMean:     128,
	}

	points := make([]WorldPoint, 10)
	now := time.Now()
	for i := 0; i < 10; i++ {
		points[i] = WorldPoint{
			X:         float64(i) * 0.4,
			Y:         float64(i%3) * 0.6,
			Z:         0.5 + float64(i)*0.1,
			Intensity: uint8(120 + i),
			Timestamp: now,
			SensorID:  "test",
		}
	}

	f := ExtractClusterFeatures(cluster, points)

	if f.PointCount != 10 {
		t.Errorf("PointCount = %d, want 10", f.PointCount)
	}
	if f.BBoxLength != 4.0 {
		t.Errorf("BBoxLength = %v, want 4.0", f.BBoxLength)
	}
	// Elongation = 4.0 / 2.0 = 2.0
	if f.Elongation != 2.0 {
		t.Errorf("Elongation = %v, want 2.0", f.Elongation)
	}
	// Compactness = 10 / (4.0 * 2.0 * 1.5) = 10 / 12 ≈ 0.833
	if math.Abs(float64(f.Compactness)-0.8333) > 0.01 {
		t.Errorf("Compactness = %v, want ~0.833", f.Compactness)
	}
	if f.IntensityStd <= 0 {
		t.Errorf("IntensityStd should be > 0, got %v", f.IntensityStd)
	}
	if f.VerticalSpread <= 0 {
		t.Errorf("VerticalSpread should be > 0, got %v", f.VerticalSpread)
	}
}

func TestExtractClusterFeatures_WithOBB(t *testing.T) {
	obb := &OrientedBoundingBox{
		Length: 5.0,
		Width:  1.8,
	}
	cluster := WorldCluster{
		PointsCount:       5,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		OBB:               obb,
	}

	f := ExtractClusterFeatures(cluster, nil)

	// Elongation should use OBB dimensions: 5.0 / 1.8 ≈ 2.778
	expectedElong := float32(5.0 / 1.8)
	if math.Abs(float64(f.Elongation-expectedElong)) > 0.01 {
		t.Errorf("Elongation = %v, want %v (from OBB)", f.Elongation, expectedElong)
	}
}

func TestExtractClusterFeatures_SinglePoint(t *testing.T) {
	cluster := WorldCluster{
		PointsCount:       1,
		BoundingBoxLength: 0,
		BoundingBoxWidth:  0,
		BoundingBoxHeight: 0,
	}
	points := []WorldPoint{{X: 1, Y: 2, Z: 3, Intensity: 50}}

	f := ExtractClusterFeatures(cluster, points)

	// With a single point, std-dev should be 0
	if f.IntensityStd != 0 {
		t.Errorf("IntensityStd = %v, want 0 for single point", f.IntensityStd)
	}
	if f.VerticalSpread != 0 {
		t.Errorf("VerticalSpread = %v, want 0 for single point", f.VerticalSpread)
	}
}

func TestExtractTrackFeatures_Basic(t *testing.T) {
	track := &TrackedObject{
		ObservationCount:     20,
		BoundingBoxLengthAvg: 4.5,
		BoundingBoxWidthAvg:  1.8,
		BoundingBoxHeightAvg: 1.5,
		HeightP95Max:         1.4,
		IntensityMeanAvg:     130,
		AvgSpeedMps:          8.5,
		PeakSpeedMps:         12.0,
		TrackDurationSecs:    10.0,
		TrackLengthMeters:    85.0,
		OcclusionCount:       2,
		speedHistory:         []float32{7.0, 8.0, 8.5, 9.0, 10.0, 11.0, 12.0, 8.0, 7.5, 9.5},
		History: []TrackPoint{
			{X: 0, Y: 0},
			{X: 1, Y: 0},
			{X: 2, Y: 0},
			{X: 3, Y: 1},
			{X: 4, Y: 2},
		},
	}

	f := ExtractTrackFeatures(track)

	if f.AvgSpeedMps != 8.5 {
		t.Errorf("AvgSpeedMps = %v, want 8.5", f.AvgSpeedMps)
	}
	if f.PeakSpeedMps != 12.0 {
		t.Errorf("PeakSpeedMps = %v, want 12.0", f.PeakSpeedMps)
	}
	// Occlusion ratio = 2 / (20 + 2) ≈ 0.0909
	expectedOccRatio := float32(2.0 / 22.0)
	if math.Abs(float64(f.OcclusionRatio-expectedOccRatio)) > 0.001 {
		t.Errorf("OcclusionRatio = %v, want %v", f.OcclusionRatio, expectedOccRatio)
	}
	if f.SpeedVariance <= 0 {
		t.Errorf("SpeedVariance should be > 0, got %v", f.SpeedVariance)
	}
	if f.HeadingVariance <= 0 {
		t.Errorf("HeadingVariance should be > 0, got %v", f.HeadingVariance)
	}
	if f.SpeedP50 <= 0 {
		t.Errorf("SpeedP50 should be > 0, got %v", f.SpeedP50)
	}
}

func TestTrackFeatures_ToVector(t *testing.T) {
	f := TrackFeatures{}
	f.PointCount = 10
	f.BBoxLength = 4.0
	f.AvgSpeedMps = 8.0

	vec := f.ToVector()
	names := SortedFeatureNames()

	if len(vec) != len(names) {
		t.Fatalf("vector length %d != feature names length %d", len(vec), len(names))
	}
	if vec[0] != 10.0 {
		t.Errorf("vec[0] (point_count) = %v, want 10", vec[0])
	}
	if vec[1] != 4.0 {
		t.Errorf("vec[1] (bbox_length) = %v, want 4.0", vec[1])
	}
}

func TestSortedFeatureNames_Length(t *testing.T) {
	names := SortedFeatureNames()
	if len(names) != 20 {
		t.Errorf("expected 20 feature names, got %d", len(names))
	}
}

func TestComputeVariance_Empty(t *testing.T) {
	v := computeVariance(nil)
	if v != 0 {
		t.Errorf("expected 0 for empty, got %v", v)
	}
}

func TestComputeVariance_Constant(t *testing.T) {
	v := computeVariance([]float32{5.0, 5.0, 5.0, 5.0})
	if v != 0 {
		t.Errorf("expected 0 for constant values, got %v", v)
	}
}

func TestComputeHeadingVariance_StraightLine(t *testing.T) {
	// Points moving in a straight line along X-axis → heading variance ≈ 0
	history := []TrackPoint{
		{X: 0, Y: 0},
		{X: 1, Y: 0},
		{X: 2, Y: 0},
		{X: 3, Y: 0},
		{X: 4, Y: 0},
	}
	v := computeHeadingVariance(history)
	if v > 0.001 {
		t.Errorf("expected ~0 heading variance for straight line, got %v", v)
	}
}

func TestComputeHeadingVariance_Turning(t *testing.T) {
	// Points that turn with varying rates → heading variance > 0
	// First segment: straight (heading 0)
	// Second segment: turn 45° (heading π/4)
	// Third segment: turn 60° more (heading π/4 + π/3)
	// Fourth segment: slight turn (heading π/4 + π/3 + π/6)
	history := []TrackPoint{
		{X: 0, Y: 0},
		{X: 1, Y: 0},     // heading 0
		{X: 2, Y: 1},     // heading π/4 (45°)
		{X: 2, Y: 2.732}, // heading ~π/2 + π/6 (longer vertical segment)
		{X: 1.5, Y: 3.5}, // shallow turn
	}
	v := computeHeadingVariance(history)
	if v <= 0 {
		t.Errorf("expected positive heading variance for turning path, got %v", v)
	}
}
