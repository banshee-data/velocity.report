package lidar

import (
	"testing"
	"time"
)

// =============================================================================
// Phase 2: Velocity-Coherent Clustering Tests
// =============================================================================

func TestDefaultClustering6DConfig(t *testing.T) {
	config := DefaultClustering6DConfig()

	if config.PositionEps <= 0 {
		t.Errorf("PositionEps should be positive, got %v", config.PositionEps)
	}
	if config.VelocityEps <= 0 {
		t.Errorf("VelocityEps should be positive, got %v", config.VelocityEps)
	}
	if config.MinPts != 3 {
		t.Errorf("MinPts should be 3 (reduced for velocity-coherent), got %d", config.MinPts)
	}
	if config.VelocityWeight <= config.PositionWeight {
		t.Errorf("VelocityWeight should be greater than PositionWeight for velocity-coherent clustering")
	}
}

func TestNewSpatialIndex6D(t *testing.T) {
	si := NewSpatialIndex6D(0.6, 1.0)

	if si == nil {
		t.Fatal("NewSpatialIndex6D returned nil")
	}
	if si.PositionCellSize != 0.6 {
		t.Errorf("PositionCellSize = %v, want 0.6", si.PositionCellSize)
	}
	if si.VelocityCellSize != 1.0 {
		t.Errorf("VelocityCellSize = %v, want 1.0", si.VelocityCellSize)
	}
}

func TestSpatialIndex6D_Build(t *testing.T) {
	si := NewSpatialIndex6D(0.6, 1.0)

	points := []PointVelocity{
		{X: 0, Y: 0, Z: 0, VX: 5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.5, Y: 0, Z: 0, VX: 5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 10, Y: 10, Z: 0, VX: -5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
	}

	si.Build(points)

	if len(si.Grid) == 0 {
		t.Error("Grid should not be empty after build")
	}
}

func TestDBSCAN6D_Empty(t *testing.T) {
	config := DefaultClustering6DConfig()
	clusters := DBSCAN6D(nil, config)

	if clusters != nil {
		t.Errorf("Expected nil for empty input, got %v", clusters)
	}
}

func TestDBSCAN6D_SingleCluster(t *testing.T) {
	config := DefaultClustering6DConfig()

	// Create 5 points moving together (same velocity)
	points := []PointVelocity{
		{X: 0, Y: 0, Z: 0, VX: 5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.3, Y: 0.2, Z: 0, VX: 5.1, VY: 0.1, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.1, Y: 0.4, Z: 0, VX: 4.9, VY: -0.1, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.2, Y: 0.1, Z: 0, VX: 5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.4, Y: 0.3, Z: 0, VX: 5.2, VY: 0.1, VZ: 0, VelocityConfidence: 0.8},
	}

	clusters := DBSCAN6D(points, config)

	if len(clusters) != 1 {
		t.Errorf("Expected 1 cluster, got %d", len(clusters))
	}

	if len(clusters) > 0 && clusters[0].PointCount != 5 {
		t.Errorf("Expected cluster with 5 points, got %d", clusters[0].PointCount)
	}
}

func TestDBSCAN6D_TwoClusters(t *testing.T) {
	config := DefaultClustering6DConfig()

	// Cluster 1: Moving left at 5 m/s
	cluster1 := []PointVelocity{
		{X: 0, Y: 0, Z: 0, VX: 5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.3, Y: 0.2, Z: 0, VX: 5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.1, Y: 0.4, Z: 0, VX: 5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
	}

	// Cluster 2: Moving right at -5 m/s (opposite direction, far away)
	cluster2 := []PointVelocity{
		{X: 20, Y: 20, Z: 0, VX: -5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 20.3, Y: 20.2, Z: 0, VX: -5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 20.1, Y: 20.4, Z: 0, VX: -5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
	}

	points := append(cluster1, cluster2...)
	clusters := DBSCAN6D(points, config)

	if len(clusters) != 2 {
		t.Errorf("Expected 2 clusters, got %d", len(clusters))
	}
}

func TestDBSCAN6D_VelocitySeparation(t *testing.T) {
	config := DefaultClustering6DConfig()

	// Same position but different velocities should be separate clusters
	points := []PointVelocity{
		// Cluster 1: Moving fast right
		{X: 0, Y: 0, Z: 0, VX: 10, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.1, Y: 0.1, Z: 0, VX: 10, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.2, Y: 0, Z: 0, VX: 10, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		// Cluster 2: Same position, moving fast left
		{X: 0.3, Y: 0.1, Z: 0, VX: -10, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.4, Y: 0.2, Z: 0, VX: -10, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.2, Y: 0.2, Z: 0, VX: -10, VY: 0, VZ: 0, VelocityConfidence: 0.8},
	}

	clusters := DBSCAN6D(points, config)

	// With high velocity difference, should be 2 separate clusters
	if len(clusters) != 2 {
		t.Errorf("Expected 2 clusters (velocity separation), got %d", len(clusters))
	}
}

func TestDBSCAN6D_LowConfidenceFiltered(t *testing.T) {
	config := DefaultClustering6DConfig()
	config.MinVelocityConfidence = 0.5

	points := []PointVelocity{
		{X: 0, Y: 0, Z: 0, VX: 5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.3, Y: 0.2, Z: 0, VX: 5, VY: 0, VZ: 0, VelocityConfidence: 0.8},
		{X: 0.1, Y: 0.4, Z: 0, VX: 5, VY: 0, VZ: 0, VelocityConfidence: 0.1}, // Low confidence
	}

	clusters := DBSCAN6D(points, config)

	// Only 2 high-confidence points, which is below MinPts=3, so no cluster
	if len(clusters) != 0 {
		t.Errorf("Expected 0 clusters (not enough high-confidence points), got %d", len(clusters))
	}
}

func TestDistance6D(t *testing.T) {
	p1 := PointVelocity{X: 0, Y: 0, Z: 0, VX: 0, VY: 0, VZ: 0}
	p2 := PointVelocity{X: 3, Y: 4, Z: 0, VX: 0, VY: 0, VZ: 0}

	// Position only distance
	dist := Distance6D(p1, p2, 1.0, 0.0)
	if dist != 5.0 {
		t.Errorf("Expected position distance 5.0, got %v", dist)
	}

	// Velocity only distance
	p3 := PointVelocity{X: 0, Y: 0, Z: 0, VX: 3, VY: 4, VZ: 0}
	dist = Distance6D(p1, p3, 0.0, 1.0)
	if dist != 5.0 {
		t.Errorf("Expected velocity distance 5.0, got %v", dist)
	}

	// Combined distance
	dist = Distance6D(p1, p2, 1.0, 1.0)
	if dist != 5.0 {
		t.Errorf("Expected combined distance 5.0, got %v", dist)
	}
}

func TestVelocityCoherentCluster_Metrics(t *testing.T) {
	config := DefaultClustering6DConfig()
	config.PositionEps = 1.5 // Increase epsilon to include all points
	now := time.Now()

	// Points within epsilon distance of each other
	points := []PointVelocity{
		{X: 0, Y: 0, Z: 0, VX: 5, VY: 2, VZ: 0, VelocityConfidence: 0.8, TimestampNanos: now.UnixNano(), SensorID: "sensor1", Intensity: 100},
		{X: 0.5, Y: 0, Z: 0.5, VX: 5, VY: 2, VZ: 0, VelocityConfidence: 0.9, TimestampNanos: now.UnixNano(), SensorID: "sensor1", Intensity: 120},
		{X: 0, Y: 0.5, Z: 0.25, VX: 5, VY: 2, VZ: 0, VelocityConfidence: 0.7, TimestampNanos: now.UnixNano(), SensorID: "sensor1", Intensity: 110},
	}

	clusters := DBSCAN6D(points, config)

	if len(clusters) != 1 {
		t.Fatalf("Expected 1 cluster, got %d", len(clusters))
	}

	cluster := clusters[0]

	// Check centroid
	expectedCentroidX := (0.0 + 0.5 + 0.0) / 3.0
	tolerance := 0.001
	if cluster.CentroidX < expectedCentroidX-tolerance || cluster.CentroidX > expectedCentroidX+tolerance {
		t.Errorf("CentroidX = %v, want ~%v", cluster.CentroidX, expectedCentroidX)
	}

	// Check average velocity
	if cluster.VelocityX != 5.0 {
		t.Errorf("VelocityX = %v, want 5.0", cluster.VelocityX)
	}

	// Check bounding box
	expectedLength := float32(0.5)
	if cluster.BoundingBoxLength != expectedLength {
		t.Errorf("BoundingBoxLength = %v, want %v", cluster.BoundingBoxLength, expectedLength)
	}

	// Check point count
	if cluster.PointCount != 3 {
		t.Errorf("PointCount = %d, want 3", cluster.PointCount)
	}
}
