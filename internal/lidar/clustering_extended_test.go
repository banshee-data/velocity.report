package lidar

import (
	"math"
	"testing"
	"time"
)

// TestTransformPointsToWorld tests the convenience function for transforming Point (Cartesian) to world frame
func TestTransformPointsToWorld(t *testing.T) {
	// Test with nil/empty slice
	result := TransformPointsToWorld(nil, nil)
	if result != nil {
		t.Errorf("TransformPointsToWorld(nil, nil) = %v, want nil", result)
	}

	emptyPoints := []Point{}
	result = TransformPointsToWorld(emptyPoints, nil)
	if result != nil {
		t.Errorf("TransformPointsToWorld(empty, nil) = %v, want nil", result)
	}

	// Test with identity transform (nil pose)
	points := []Point{
		{X: 1.0, Y: 2.0, Z: 3.0, Intensity: 100, Distance: 3.74, Azimuth: 0, Elevation: 0},
		{X: -5.0, Y: 10.0, Z: 0.5, Intensity: 150, Distance: 11.2, Azimuth: 90, Elevation: 5},
		{X: 0.0, Y: 0.0, Z: 0.0, Intensity: 50, Distance: 0, Azimuth: 180, Elevation: -10},
	}

	worldPoints := TransformPointsToWorld(points, nil)

	if len(worldPoints) != len(points) {
		t.Fatalf("len(worldPoints) = %d, want %d", len(worldPoints), len(points))
	}

	// With identity transform, world coordinates should match sensor coordinates
	for i, wp := range worldPoints {
		if !floatEquals(wp.X, points[i].X, 0.0001) {
			t.Errorf("Point %d X = %f, want %f", i, wp.X, points[i].X)
		}
		if !floatEquals(wp.Y, points[i].Y, 0.0001) {
			t.Errorf("Point %d Y = %f, want %f", i, wp.Y, points[i].Y)
		}
		if !floatEquals(wp.Z, points[i].Z, 0.0001) {
			t.Errorf("Point %d Z = %f, want %f", i, wp.Z, points[i].Z)
		}
		// Verify intensity is preserved (only metadata in WorldPoint)
		if wp.Intensity != points[i].Intensity {
			t.Errorf("Point %d Intensity = %d, want %d", i, wp.Intensity, points[i].Intensity)
		}
	}
}

// TestTransformPointsToWorld_WithTranslation tests transformation with translation
func TestTransformPointsToWorld_WithTranslation(t *testing.T) {
	// Create a pose with translation (10, 20, 5) in world frame
	pose := &Pose{
		PoseID:   1,
		SensorID: "sensor-test",
		T: [16]float64{
			1, 0, 0, 10, // Row 0: no rotation, +10 in X
			0, 1, 0, 20, // Row 1: no rotation, +20 in Y
			0, 0, 1, 5, // Row 2: no rotation, +5 in Z
			0, 0, 0, 1, // Row 3: homogeneous coordinate
		},
	}

	points := []Point{
		{X: 0.0, Y: 0.0, Z: 0.0},    // Origin should map to (10, 20, 5)
		{X: 1.0, Y: 2.0, Z: 3.0},    // Should map to (11, 22, 8)
		{X: -5.0, Y: -10.0, Z: 1.0}, // Should map to (5, 10, 6)
	}

	worldPoints := TransformPointsToWorld(points, pose)

	expected := [][3]float64{
		{10.0, 20.0, 5.0},
		{11.0, 22.0, 8.0},
		{5.0, 10.0, 6.0},
	}

	for i, wp := range worldPoints {
		if !floatEquals(wp.X, expected[i][0], 0.0001) {
			t.Errorf("Point %d X = %f, want %f", i, wp.X, expected[i][0])
		}
		if !floatEquals(wp.Y, expected[i][1], 0.0001) {
			t.Errorf("Point %d Y = %f, want %f", i, wp.Y, expected[i][1])
		}
		if !floatEquals(wp.Z, expected[i][2], 0.0001) {
			t.Errorf("Point %d Z = %f, want %f", i, wp.Z, expected[i][2])
		}
	}
}

// TestTransformPointsToWorld_WithRotation tests transformation with rotation
func TestTransformPointsToWorld_WithRotation(t *testing.T) {
	// Create a pose with 90-degree rotation around Z-axis (yaw)
	// Rotation matrix for 90° around Z:
	// [ cos(90)  -sin(90)  0 ]   [ 0  -1  0 ]
	// [ sin(90)   cos(90)  0 ] = [ 1   0  0 ]
	// [   0         0      1 ]   [ 0   0  1 ]
	pose := &Pose{
		PoseID:   2,
		SensorID: "sensor-rotated",
		T: [16]float64{
			0, -1, 0, 0, // Row 0
			1, 0, 0, 0, // Row 1
			0, 0, 1, 0, // Row 2
			0, 0, 0, 1, // Row 3
		},
	}

	points := []Point{
		{X: 1.0, Y: 0.0, Z: 0.0}, // Should map to (0, 1, 0)
		{X: 0.0, Y: 1.0, Z: 0.0}, // Should map to (-1, 0, 0)
		{X: 1.0, Y: 1.0, Z: 0.0}, // Should map to (-1, 1, 0)
	}

	worldPoints := TransformPointsToWorld(points, pose)

	expected := [][3]float64{
		{0.0, 1.0, 0.0},
		{-1.0, 0.0, 0.0},
		{-1.0, 1.0, 0.0},
	}

	for i, wp := range worldPoints {
		if !floatEquals(wp.X, expected[i][0], 0.0001) {
			t.Errorf("Point %d X = %f, want %f", i, wp.X, expected[i][0])
		}
		if !floatEquals(wp.Y, expected[i][1], 0.0001) {
			t.Errorf("Point %d Y = %f, want %f", i, wp.Y, expected[i][1])
		}
		if !floatEquals(wp.Z, expected[i][2], 0.0001) {
			t.Errorf("Point %d Z = %f, want %f", i, wp.Z, expected[i][2])
		}
	}
}

// TestTransformPointsToWorld_ComplexTransform tests with both rotation and translation
func TestTransformPointsToWorld_ComplexTransform(t *testing.T) {
	// 45-degree rotation around Z + translation (5, 10, 2)
	cos45 := math.Cos(math.Pi / 4)
	sin45 := math.Sin(math.Pi / 4)

	pose := &Pose{
		PoseID:   3,
		SensorID: "sensor-complex",
		T: [16]float64{
			cos45, -sin45, 0, 5, // Row 0
			sin45, cos45, 0, 10, // Row 1
			0, 0, 1, 2, // Row 2
			0, 0, 0, 1, // Row 3
		},
	}

	// Test point at (1, 0, 0) in sensor frame
	points := []Point{
		{X: 1.0, Y: 0.0, Z: 0.0},
	}

	worldPoints := TransformPointsToWorld(points, pose)

	// Expected: rotate (1,0,0) by 45° then add translation
	// (1,0,0) * [cos45, sin45] = (cos45, sin45) + (5, 10, 2)
	expectedX := cos45 + 5
	expectedY := sin45 + 10
	expectedZ := 0.0 + 2

	if !floatEquals(worldPoints[0].X, expectedX, 0.0001) {
		t.Errorf("X = %f, want %f", worldPoints[0].X, expectedX)
	}
	if !floatEquals(worldPoints[0].Y, expectedY, 0.0001) {
		t.Errorf("Y = %f, want %f", worldPoints[0].Y, expectedY)
	}
	if !floatEquals(worldPoints[0].Z, expectedZ, 0.0001) {
		t.Errorf("Z = %f, want %f", worldPoints[0].Z, expectedZ)
	}
}

// TestTransformPointsToWorld_MetadataPreservation tests that all metadata is preserved
func TestTransformPointsToWorld_MetadataPreservation(t *testing.T) {
	timestamp := time.Now()
	points := []Point{
		{
			X:         1.0,
			Y:         2.0,
			Z:         3.0,
			Intensity: 200,
			Timestamp: timestamp,
		},
	}

	worldPoints := TransformPointsToWorld(points, nil)

	if len(worldPoints) != 1 {
		t.Fatalf("Expected 1 world point, got %d", len(worldPoints))
	}

	wp := worldPoints[0]

	// Check metadata that WorldPoint actually has
	if wp.Intensity != 200 {
		t.Errorf("Intensity = %d, want 200", wp.Intensity)
	}
	if !wp.Timestamp.Equal(timestamp) {
		t.Errorf("Timestamp = %v, want %v", wp.Timestamp, timestamp)
	}
	// SensorID is set to empty string by TransformPointsToWorld
	if wp.SensorID != "" {
		t.Errorf("SensorID = %s, want empty string", wp.SensorID)
	}
}

// TestTransformPointsToWorld_LargeDataset tests with a larger dataset
func TestTransformPointsToWorld_LargeDataset(t *testing.T) {
	// Create a large dataset
	numPoints := 10000
	points := make([]Point, numPoints)
	for i := 0; i < numPoints; i++ {
		points[i] = Point{
			X:         float64(i) * 0.1,
			Y:         float64(i) * 0.2,
			Z:         float64(i) * 0.05,
			Intensity: uint8(i % 256),
		}
	}

	// Transform with simple translation
	pose := &Pose{
		T: [16]float64{
			1, 0, 0, 100,
			0, 1, 0, 200,
			0, 0, 1, 50,
			0, 0, 0, 1,
		},
	}

	worldPoints := TransformPointsToWorld(points, pose)

	if len(worldPoints) != numPoints {
		t.Fatalf("Expected %d world points, got %d", numPoints, len(worldPoints))
	}

	// Spot check a few points
	checkIndices := []int{0, 100, 1000, numPoints - 1}
	for _, i := range checkIndices {
		expectedX := points[i].X + 100
		expectedY := points[i].Y + 200
		expectedZ := points[i].Z + 50

		if !floatEquals(worldPoints[i].X, expectedX, 0.0001) {
			t.Errorf("Point %d X = %f, want %f", i, worldPoints[i].X, expectedX)
		}
		if !floatEquals(worldPoints[i].Y, expectedY, 0.0001) {
			t.Errorf("Point %d Y = %f, want %f", i, worldPoints[i].Y, expectedY)
		}
		if !floatEquals(worldPoints[i].Z, expectedZ, 0.0001) {
			t.Errorf("Point %d Z = %f, want %f", i, worldPoints[i].Z, expectedZ)
		}
	}
}

// floatEquals compares two float64 values with a tolerance
func floatEquals(a, b, tolerance float64) bool {
	return math.Abs(a-b) < tolerance
}
