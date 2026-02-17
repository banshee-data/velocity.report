package l4perception

import (
	"testing"
	"time"
)

func TestVoxelGrid_Empty(t *testing.T) {
	result := VoxelGrid(nil, 0.1)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestVoxelGrid_ZeroLeafSize(t *testing.T) {
	points := []WorldPoint{{X: 1, Y: 2, Z: 3}}
	result := VoxelGrid(points, 0)
	if len(result) != 1 {
		t.Errorf("expected passthrough for zero leaf size, got %d points", len(result))
	}
}

func TestVoxelGrid_SinglePoint(t *testing.T) {
	points := []WorldPoint{
		{X: 1.0, Y: 2.0, Z: 3.0, Intensity: 100, SensorID: "s1"},
	}
	result := VoxelGrid(points, 1.0)
	if len(result) != 1 {
		t.Fatalf("expected 1 point, got %d", len(result))
	}
	if result[0].X != 1.0 || result[0].Y != 2.0 || result[0].Z != 3.0 {
		t.Errorf("point not preserved: %v", result[0])
	}
	if result[0].Intensity != 100 {
		t.Errorf("intensity not preserved: %d", result[0].Intensity)
	}
}

func TestVoxelGrid_TwoPointsSameVoxel(t *testing.T) {
	// Two points within the same 1m voxel → output should be 1 point.
	points := []WorldPoint{
		{X: 0.1, Y: 0.1, Z: 0.1, Intensity: 50},
		{X: 0.2, Y: 0.2, Z: 0.2, Intensity: 60},
	}
	result := VoxelGrid(points, 1.0)
	if len(result) != 1 {
		t.Fatalf("expected 1 point (same voxel), got %d", len(result))
	}
	// The point closest to the voxel centroid (0.15, 0.15, 0.15) should be kept.
	// Both are equidistant but implementation picks the one with smaller squared distance.
}

func TestVoxelGrid_DistinctVoxels(t *testing.T) {
	// Three points in three different voxels → all kept.
	points := []WorldPoint{
		{X: 0.5, Y: 0.5, Z: 0.5},
		{X: 1.5, Y: 0.5, Z: 0.5},
		{X: 0.5, Y: 1.5, Z: 0.5},
	}
	result := VoxelGrid(points, 1.0)
	if len(result) != 3 {
		t.Errorf("expected 3 points (distinct voxels), got %d", len(result))
	}
}

func TestVoxelGrid_Reduction(t *testing.T) {
	// Generate a dense cluster of 100 points in a small area.
	now := time.Now()
	points := make([]WorldPoint, 100)
	for i := 0; i < 100; i++ {
		// Points in a 1m × 1m × 1m cube
		x := float64(i%10) * 0.1
		y := float64(i/10) * 0.1
		points[i] = WorldPoint{
			X:         x,
			Y:         y,
			Z:         0.5,
			Intensity: uint8(i),
			Timestamp: now,
			SensorID:  "test",
		}
	}

	// With 0.5m leaf size, the 1m×1m area → ~4 voxels
	result := VoxelGrid(points, 0.5)
	if len(result) >= len(points) {
		t.Errorf("expected reduction, got %d from %d", len(result), len(points))
	}
	if len(result) == 0 {
		t.Error("expected non-zero output")
	}
	// Should be approximately 4 voxels (2×2 grid × 1 Z layer)
	if len(result) < 2 || len(result) > 8 {
		t.Errorf("expected ~4 output points for 0.5m voxels on 1m² area, got %d", len(result))
	}
}

func TestVoxelGrid_NegativeCoordinates(t *testing.T) {
	// Points spanning negative and positive coordinates.
	points := []WorldPoint{
		{X: -0.5, Y: -0.5, Z: 0.0},
		{X: 0.5, Y: 0.5, Z: 0.0},
	}
	result := VoxelGrid(points, 1.0)
	// These are in different voxels (-1,0 vs 0,0) so both should be kept.
	if len(result) != 2 {
		t.Errorf("expected 2 points (different voxels across origin), got %d", len(result))
	}
}

func TestVoxelGrid_PreservesClosestToCentroid(t *testing.T) {
	// Three points in same voxel: only the one closest to centroid survives.
	points := []WorldPoint{
		{X: 0.0, Y: 0.0, Z: 0.0, Intensity: 10},    // corner
		{X: 0.45, Y: 0.45, Z: 0.45, Intensity: 20}, // near centre
		{X: 0.9, Y: 0.9, Z: 0.9, Intensity: 30},    // far corner
	}
	result := VoxelGrid(points, 1.0)
	if len(result) != 1 {
		t.Fatalf("expected 1 point, got %d", len(result))
	}
	// Centroid = (0.45, 0.45, 0.45) → point at (0.45, 0.45, 0.45) is closest.
	if result[0].Intensity != 20 {
		t.Errorf("expected point closest to centroid (intensity=20), got intensity=%d", result[0].Intensity)
	}
}

func TestVoxelGrid_LargeLeafCollapsesAll(t *testing.T) {
	// Leaf size larger than all point spread → single output.
	points := []WorldPoint{
		{X: 0.1, Y: 0.1, Z: 0.1},
		{X: 0.2, Y: 0.2, Z: 0.2},
		{X: 0.3, Y: 0.3, Z: 0.3},
	}
	result := VoxelGrid(points, 100.0) // 100m leaf
	if len(result) != 1 {
		t.Errorf("expected 1 point with huge leaf size, got %d", len(result))
	}
}

func TestVoxelGrid_MetadataPreserved(t *testing.T) {
	now := time.Now()
	points := []WorldPoint{
		{X: 0.5, Y: 0.5, Z: 0.5, Intensity: 42, Timestamp: now, SensorID: "lidar-01"},
	}
	result := VoxelGrid(points, 1.0)
	if len(result) != 1 {
		t.Fatalf("expected 1 point, got %d", len(result))
	}
	if result[0].Intensity != 42 {
		t.Errorf("intensity not preserved: %d", result[0].Intensity)
	}
	if result[0].SensorID != "lidar-01" {
		t.Errorf("sensorID not preserved: %s", result[0].SensorID)
	}
	if !result[0].Timestamp.Equal(now) {
		t.Errorf("timestamp not preserved")
	}
}

func TestVoxelGrid_3DSeparation(t *testing.T) {
	// Two points at same XY but different Z → different voxels.
	points := []WorldPoint{
		{X: 0.5, Y: 0.5, Z: 0.5},
		{X: 0.5, Y: 0.5, Z: 1.5},
	}
	result := VoxelGrid(points, 1.0)
	if len(result) != 2 {
		t.Errorf("expected 2 points (different Z voxels), got %d", len(result))
	}
}
