package l4perception

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
)

// testDBSCANParams returns a DBSCANParams suitable for unit tests,
// with generous filter thresholds that won't reject normal test clusters.
func testDBSCANParams(eps float64, minPts int) DBSCANParams {
	return DBSCANParams{
		Eps:                   eps,
		MinPts:                minPts,
		MaxClusterDiameter:    100.0,
		MinClusterDiameter:    0.0,
		MaxClusterAspectRatio: 100.0,
	}
}

// =============================================================================
// Tests: Polar → World Transform
// =============================================================================

func TestTransformToWorld_IdentityPose(t *testing.T) {
	// Point at (distance=10m, azimuth=0°, elevation=0°)
	// With identity pose, should stay at sensor coordinates
	polar := []PointPolar{
		{Distance: 10.0, Azimuth: 0.0, Elevation: 0.0, Intensity: 100, Timestamp: time.Now().UnixNano()},
	}

	world := TransformToWorld(polar, nil, "test-sensor") // nil pose = identity

	if len(world) != 1 {
		t.Fatalf("expected 1 world point, got %d", len(world))
	}

	// At azimuth=0, elevation=0: X=0, Y=distance, Z=0 (using sensor coordinate convention)
	if math.Abs(world[0].X) > 0.01 {
		t.Errorf("expected X≈0, got %v", world[0].X)
	}
	if math.Abs(world[0].Y-10.0) > 0.01 {
		t.Errorf("expected Y≈10.0, got %v", world[0].Y)
	}
	if math.Abs(world[0].Z) > 0.01 {
		t.Errorf("expected Z≈0, got %v", world[0].Z)
	}
	if world[0].Intensity != 100 {
		t.Errorf("expected Intensity=100, got %d", world[0].Intensity)
	}
	if world[0].SensorID != "test-sensor" {
		t.Errorf("expected SensorID=test-sensor, got %s", world[0].SensorID)
	}
}

func TestTransformToWorld_WithPose(t *testing.T) {
	// Point at (distance=10m, azimuth=0°, elevation=0°)
	polar := []PointPolar{
		{Distance: 10.0, Azimuth: 0.0, Elevation: 0.0},
	}

	// Pose that translates by (1, 2, 3)
	pose := &Pose{
		T: [16]float64{
			1, 0, 0, 1, // Row 0: identity rotation + X translation
			0, 1, 0, 2, // Row 1: identity rotation + Y translation
			0, 0, 1, 3, // Row 2: identity rotation + Z translation
			0, 0, 0, 1, // Row 3: homogeneous
		},
		SensorID: "test-sensor",
	}

	world := TransformToWorld(polar, pose, "test-sensor")

	// Original point: (0, 10, 0) -> translated: (1, 12, 3)
	if math.Abs(world[0].X-1.0) > 0.01 {
		t.Errorf("expected X≈1.0, got %v", world[0].X)
	}
	if math.Abs(world[0].Y-12.0) > 0.01 {
		t.Errorf("expected Y≈12.0, got %v", world[0].Y)
	}
	if math.Abs(world[0].Z-3.0) > 0.01 {
		t.Errorf("expected Z≈3.0, got %v", world[0].Z)
	}
}

func TestTransformToWorld_EmptyInput(t *testing.T) {
	result := TransformToWorld([]PointPolar{}, nil, "test")
	if result != nil {
		t.Errorf("expected nil result for empty input")
	}
}

func TestTransformToWorld_Azimuth90(t *testing.T) {
	// Point at azimuth=90° should be along X axis
	polar := []PointPolar{
		{Distance: 10.0, Azimuth: 90.0, Elevation: 0.0},
	}

	world := TransformToWorld(polar, nil, "test")

	// At azimuth=90, elevation=0: X=distance, Y=0, Z=0
	if math.Abs(world[0].X-10.0) > 0.01 {
		t.Errorf("expected X≈10.0, got %v", world[0].X)
	}
	if math.Abs(world[0].Y) > 0.01 {
		t.Errorf("expected Y≈0, got %v", world[0].Y)
	}
}

func TestTransformToWorld_WithElevation(t *testing.T) {
	// Point at elevation=45° should have Z component
	polar := []PointPolar{
		{Distance: 10.0, Azimuth: 0.0, Elevation: 45.0},
	}

	world := TransformToWorld(polar, nil, "test")

	// At azimuth=0, elevation=45: X=0, Y=r*cos(45), Z=r*sin(45)
	expectedY := 10.0 * math.Cos(45.0*math.Pi/180.0)
	expectedZ := 10.0 * math.Sin(45.0*math.Pi/180.0)

	if math.Abs(world[0].Y-expectedY) > 0.01 {
		t.Errorf("expected Y≈%v, got %v", expectedY, world[0].Y)
	}
	if math.Abs(world[0].Z-expectedZ) > 0.01 {
		t.Errorf("expected Z≈%v, got %v", expectedZ, world[0].Z)
	}
}

// =============================================================================
// Tests: DBSCAN Clustering
// =============================================================================

func TestSpatialIndex_Build(t *testing.T) {
	points := []WorldPoint{
		{X: 0.0, Y: 0.0, Z: 0.0},
		{X: 0.5, Y: 0.5, Z: 0.0},
		{X: 10.0, Y: 10.0, Z: 0.0},
	}

	si := NewSpatialIndex(1.0)
	si.Build(points)

	// Should have at least 2 different cells (points 0,1 close together; point 2 far)
	if len(si.Grid) < 2 {
		t.Errorf("expected at least 2 cells, got %d", len(si.Grid))
	}
}

func TestSpatialIndex_RegionQuery(t *testing.T) {
	points := []WorldPoint{
		{X: 0.0, Y: 0.0, Z: 0.0},
		{X: 0.3, Y: 0.3, Z: 0.0},
		{X: 10.0, Y: 10.0, Z: 0.0},
	}

	si := NewSpatialIndex(1.0)
	si.Build(points)

	// Query from point 0 with eps=0.5 should find points 0 and 1
	neighbors := si.RegionQuery(points, 0, 0.5)
	if len(neighbors) != 2 {
		t.Errorf("expected 2 neighbors, got %d", len(neighbors))
	}

	// Query from point 2 with eps=0.5 should find only point 2
	neighbors = si.RegionQuery(points, 2, 0.5)
	if len(neighbors) != 1 {
		t.Errorf("expected 1 neighbor, got %d", len(neighbors))
	}
}

func TestDBSCAN_TwoSeparateClusters(t *testing.T) {
	// Create two clusters: one near origin, one at (10, 0)
	points := []WorldPoint{}

	// Cluster 1 around origin
	for i := 0; i < 20; i++ {
		x := 0.3 * float64(i%5)
		y := 0.3 * float64(i/5)
		points = append(points, WorldPoint{X: x, Y: y, Z: 0, Intensity: 100, Timestamp: time.Now(), SensorID: "test"})
	}

	// Cluster 2 around (10, 0)
	for i := 0; i < 20; i++ {
		x := 10.0 + 0.3*float64(i%5)
		y := 0.3 * float64(i/5)
		points = append(points, WorldPoint{X: x, Y: y, Z: 0, Intensity: 100, Timestamp: time.Now(), SensorID: "test"})
	}

	params := testDBSCANParams(0.6, 5)
	clusters := DBSCAN(points, params)

	if len(clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(clusters))
	}

	// Verify centroids are approximately correct
	centroids := []float32{clusters[0].CentroidX, clusters[1].CentroidX}
	if centroids[0] > centroids[1] {
		centroids[0], centroids[1] = centroids[1], centroids[0]
	}

	if centroids[0] < -1.0 || centroids[0] > 2.0 {
		t.Errorf("expected first cluster centroid near 0, got %v", centroids[0])
	}
	if centroids[1] < 9.0 || centroids[1] > 12.0 {
		t.Errorf("expected second cluster centroid near 10, got %v", centroids[1])
	}
}

func TestDBSCAN_NoisePoints(t *testing.T) {
	// Points too sparse to form clusters
	points := []WorldPoint{
		{X: 0.0, Y: 0.0, Z: 0.0},
		{X: 10.0, Y: 0.0, Z: 0.0},
		{X: 20.0, Y: 0.0, Z: 0.0},
	}

	params := testDBSCANParams(0.6, 5)
	clusters := DBSCAN(points, params)

	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters for sparse points, got %d", len(clusters))
	}
}

func TestDBSCAN_EmptyInput(t *testing.T) {
	params := testDBSCANParams(0.6, 5)
	clusters := DBSCAN([]WorldPoint{}, params)

	if clusters != nil {
		t.Errorf("expected nil for empty input")
	}
}

func TestDBSCAN_SingleDenseCluster(t *testing.T) {
	// One dense cluster
	points := make([]WorldPoint, 50)
	for i := range points {
		angle := float64(i) * 2 * math.Pi / 50
		points[i] = WorldPoint{
			X:         0.3 * math.Cos(angle),
			Y:         0.3 * math.Sin(angle),
			Z:         float64(i) * 0.01,
			Intensity: uint8(i + 50),
			Timestamp: time.Now(),
			SensorID:  "test",
		}
	}

	params := testDBSCANParams(0.6, 5)
	clusters := DBSCAN(points, params)

	if len(clusters) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(clusters))
	}

	if clusters[0].PointsCount != 50 {
		t.Errorf("expected 50 points in cluster, got %d", clusters[0].PointsCount)
	}

	// Verify centroid is near origin. With medoid computation (task 3.2) the
	// centroid is an actual cluster point, not the arithmetic mean, so
	// tolerance is relaxed to the ring's radius (0.3m).
	if math.Abs(float64(clusters[0].CentroidX)) > 0.35 {
		t.Errorf("expected centroid X near 0, got %v", clusters[0].CentroidX)
	}
	if math.Abs(float64(clusters[0].CentroidY)) > 0.35 {
		t.Errorf("expected centroid Y near 0, got %v", clusters[0].CentroidY)
	}
}

func TestComputeClusterMetrics(t *testing.T) {
	now := time.Now()
	points := []WorldPoint{
		{X: 0, Y: 0, Z: 0, Intensity: 100, Timestamp: now, SensorID: "test"},
		{X: 2, Y: 0, Z: 0, Intensity: 100, Timestamp: now, SensorID: "test"},
		{X: 1, Y: 2, Z: 1, Intensity: 100, Timestamp: now, SensorID: "test"},
		{X: 1, Y: 1, Z: 2, Intensity: 100, Timestamp: now, SensorID: "test"},
	}

	cluster := computeClusterMetrics(points, 1)

	// Centroid should be (1, 0.75, 0.75)
	if math.Abs(float64(cluster.CentroidX)-1.0) > 0.01 {
		t.Errorf("expected CentroidX=1.0, got %v", cluster.CentroidX)
	}

	// Bounding box dimensions come from OBB (PCA-based), not AABB.
	// For these 4 points, the OBB should encompass all points when rotated
	// by the heading. Height is axis-aligned so remains 2.0.
	if cluster.BoundingBoxLength <= 0 {
		t.Errorf("expected BoundingBoxLength > 0, got %v", cluster.BoundingBoxLength)
	}
	if cluster.BoundingBoxWidth <= 0 {
		t.Errorf("expected BoundingBoxWidth > 0, got %v", cluster.BoundingBoxWidth)
	}
	if cluster.BoundingBoxHeight != 2.0 {
		t.Errorf("expected BoundingBoxHeight=2.0, got %v", cluster.BoundingBoxHeight)
	}
	// OBB area should be <= AABB area (tighter fit) or at most equal
	obbArea := cluster.BoundingBoxLength * cluster.BoundingBoxWidth
	aabbArea := float32(2.0 * 2.0)
	if obbArea > aabbArea*1.01 { // small tolerance for floating point
		t.Errorf("OBB area (%v) should not exceed AABB area (%v)", obbArea, aabbArea)
	}

	if cluster.PointsCount != 4 {
		t.Errorf("expected PointsCount=4, got %d", cluster.PointsCount)
	}

	if cluster.IntensityMean != 100 {
		t.Errorf("expected IntensityMean=100, got %v", cluster.IntensityMean)
	}
}

func TestDefaultDBSCANParams(t *testing.T) {
	params := DefaultDBSCANParams()

	cfg := config.MustLoadDefaultConfig()
	if params.Eps != cfg.GetForegroundDBSCANEps() {
		t.Errorf("expected Eps=%v, got %v", cfg.GetForegroundDBSCANEps(), params.Eps)
	}
	if params.MinPts != cfg.GetForegroundMinClusterPoints() {
		t.Errorf("expected MinPts=%d, got %d", cfg.GetForegroundMinClusterPoints(), params.MinPts)
	}
}
