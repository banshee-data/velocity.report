// Package visualiser provides gRPC streaming of LiDAR perception data.
package visualiser

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Helper function to safely cast interface{} to *FrameBundle in tests
func toFrameBundle(t *testing.T, i interface{}) *FrameBundle {
	t.Helper()
	bundle, ok := i.(*FrameBundle)
	if !ok || bundle == nil {
		t.Fatal("expected non-nil *FrameBundle")
	}
	return bundle
}

func TestNewFrameAdapter(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	if adapter == nil {
		t.Fatal("expected non-nil FrameAdapter")
	}
	if adapter.sensorID != "hesai-01" {
		t.Errorf("expected sensorID=hesai-01, got %s", adapter.sensorID)
	}
	if adapter.frameID != 0 {
		t.Errorf("expected frameID=0, got %d", adapter.frameID)
	}
}

func TestFrameAdapter_AdaptFrame_BasicFrame(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	frame := &lidar.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []lidar.Point{},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil))

	if bundle == nil {
		t.Fatal("expected non-nil FrameBundle")
	}
	if bundle.FrameID != 1 {
		t.Errorf("expected FrameID=1, got %d", bundle.FrameID)
	}
	if bundle.SensorID != "hesai-01" {
		t.Errorf("expected SensorID=hesai-01, got %s", bundle.SensorID)
	}
	if bundle.TimestampNanos != now.UnixNano() {
		t.Errorf("expected TimestampNanos=%d, got %d", now.UnixNano(), bundle.TimestampNanos)
	}
}

func TestFrameAdapter_AdaptFrame_FrameIDIncrement(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	frame := &lidar.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points:         []lidar.Point{},
	}

	bundle1 := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil))
	bundle2 := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil))
	bundle3 := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil))

	if bundle1.FrameID != 1 {
		t.Errorf("expected FrameID=1, got %d", bundle1.FrameID)
	}
	if bundle2.FrameID != 2 {
		t.Errorf("expected FrameID=2, got %d", bundle2.FrameID)
	}
	if bundle3.FrameID != 3 {
		t.Errorf("expected FrameID=3, got %d", bundle3.FrameID)
	}
}

func TestFrameAdapter_AdaptFrame_WithPointCloud(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	frame := &lidar.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []lidar.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
			{X: 3.0, Y: 4.0, Z: 1.0, Intensity: 150},
			{X: 5.0, Y: 6.0, Z: 1.5, Intensity: 200},
		},
	}

	mask := []bool{true, false, true} // foreground, background, foreground

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, mask, nil, nil))

	if bundle.PointCloud == nil {
		t.Fatal("expected non-nil PointCloud")
	}

	pc := bundle.PointCloud
	if pc.PointCount != 3 {
		t.Errorf("expected PointCount=3, got %d", pc.PointCount)
	}
	if len(pc.X) != 3 {
		t.Errorf("expected X length=3, got %d", len(pc.X))
	}
	if pc.X[0] != 1.0 {
		t.Errorf("expected X[0]=1.0, got %f", pc.X[0])
	}
	if pc.Y[1] != 4.0 {
		t.Errorf("expected Y[1]=4.0, got %f", pc.Y[1])
	}
	if pc.Intensity[2] != 200 {
		t.Errorf("expected Intensity[2]=200, got %d", pc.Intensity[2])
	}
	// Check classification
	if pc.Classification[0] != 1 { // foreground
		t.Errorf("expected Classification[0]=1, got %d", pc.Classification[0])
	}
	if pc.Classification[1] != 0 { // background
		t.Errorf("expected Classification[1]=0, got %d", pc.Classification[1])
	}
	if pc.Classification[2] != 1 { // foreground
		t.Errorf("expected Classification[2]=1, got %d", pc.Classification[2])
	}
}

func TestFrameAdapter_AdaptFrame_WithClusters(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	frame := &lidar.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []lidar.Point{},
	}

	clusters := []lidar.WorldCluster{
		{
			ClusterID:         1,
			SensorID:          "hesai-01",
			TSUnixNanos:       now.UnixNano(),
			CentroidX:         5.0,
			CentroidY:         10.0,
			CentroidZ:         1.0,
			BoundingBoxLength: 4.5,
			BoundingBoxWidth:  2.0,
			BoundingBoxHeight: 1.5,
			PointsCount:       100,
			HeightP95:         1.8,
			IntensityMean:     75.0,
		},
		{
			ClusterID:         2,
			SensorID:          "hesai-01",
			TSUnixNanos:       now.UnixNano(),
			CentroidX:         15.0,
			CentroidY:         20.0,
			CentroidZ:         0.5,
			BoundingBoxLength: 1.0,
			BoundingBoxWidth:  1.0,
			BoundingBoxHeight: 1.8,
			PointsCount:       25,
			HeightP95:         1.7,
			IntensityMean:     50.0,
		},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, clusters, nil))

	if bundle.Clusters == nil {
		t.Fatal("expected non-nil Clusters")
	}

	cs := bundle.Clusters
	if len(cs.Clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(cs.Clusters))
	}
	if cs.Method != ClusteringDBSCAN {
		t.Errorf("expected Method=ClusteringDBSCAN, got %d", cs.Method)
	}

	// Check first cluster
	c1 := cs.Clusters[0]
	if c1.ClusterID != 1 {
		t.Errorf("expected ClusterID=1, got %d", c1.ClusterID)
	}
	if c1.CentroidX != 5.0 {
		t.Errorf("expected CentroidX=5.0, got %f", c1.CentroidX)
	}
	if c1.AABBLength != 4.5 {
		t.Errorf("expected AABBLength=4.5, got %f", c1.AABBLength)
	}
	if c1.PointsCount != 100 {
		t.Errorf("expected PointsCount=100, got %d", c1.PointsCount)
	}
}

func TestFrameAdapter_AdaptFrame_WithTracker(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	frame := &lidar.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []lidar.Point{},
	}

	// Create a tracker with a track
	trackerCfg := lidar.DefaultTrackerConfig()
	tracker := lidar.NewTracker(trackerCfg)

	// Add a cluster to create a track
	cluster := lidar.WorldCluster{
		ClusterID:         1,
		SensorID:          "hesai-01",
		CentroidX:         5.0,
		CentroidY:         10.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.5,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       100,
		HeightP95:         1.8,
		IntensityMean:     75.0,
	}
	tracker.Update([]lidar.WorldCluster{cluster}, now)

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, tracker))

	if bundle.Tracks == nil {
		t.Fatal("expected non-nil Tracks")
	}

	ts := bundle.Tracks
	if len(ts.Tracks) != 1 {
		t.Errorf("expected 1 track, got %d", len(ts.Tracks))
	}

	track := ts.Tracks[0]
	if track.TrackID == "" {
		t.Error("expected non-empty TrackID")
	}
	if track.State != TrackStateTentative {
		t.Errorf("expected State=TrackStateTentative, got %d", track.State)
	}
	if track.X != 5.0 {
		t.Errorf("expected X=5.0, got %f", track.X)
	}
	if track.Y != 10.0 {
		t.Errorf("expected Y=10.0, got %f", track.Y)
	}
}

func TestAdaptTrackState(t *testing.T) {
	tests := []struct {
		input    lidar.TrackState
		expected TrackState
	}{
		{lidar.TrackTentative, TrackStateTentative},
		{lidar.TrackConfirmed, TrackStateConfirmed},
		{lidar.TrackDeleted, TrackStateDeleted},
		{"unknown_state", TrackStateUnknown},
	}

	for _, tc := range tests {
		got := adaptTrackState(tc.input)
		if got != tc.expected {
			t.Errorf("adaptTrackState(%s) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

func TestFrameAdapter_AdaptPointCloud_EmptyMask(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	frame := &lidar.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []lidar.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
			{X: 3.0, Y: 4.0, Z: 1.0, Intensity: 150},
		},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil))

	pc := bundle.PointCloud
	if pc == nil {
		t.Fatal("expected non-nil PointCloud")
	}

	// With nil mask, all points should be classified as background (0)
	for i, c := range pc.Classification {
		if c != 0 {
			t.Errorf("expected Classification[%d]=0 with nil mask, got %d", i, c)
		}
	}
}

func TestFrameAdapter_AdaptPointCloud_PartialMask(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	frame := &lidar.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []lidar.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
			{X: 3.0, Y: 4.0, Z: 1.0, Intensity: 150},
			{X: 5.0, Y: 6.0, Z: 1.5, Intensity: 200},
		},
	}

	// Mask shorter than points
	mask := []bool{true}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, mask, nil, nil))

	pc := bundle.PointCloud
	if pc == nil {
		t.Fatal("expected non-nil PointCloud")
	}

	// First point should be foreground, rest background
	if pc.Classification[0] != 1 {
		t.Errorf("expected Classification[0]=1, got %d", pc.Classification[0])
	}
	if pc.Classification[1] != 0 {
		t.Errorf("expected Classification[1]=0, got %d", pc.Classification[1])
	}
	if pc.Classification[2] != 0 {
		t.Errorf("expected Classification[2]=0, got %d", pc.Classification[2])
	}
}

func TestFrameAdapter_AdaptClusters_Empty(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	frame := &lidar.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points:         []lidar.Point{},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, []lidar.WorldCluster{}, nil))

	// Empty clusters slice should result in nil Clusters
	if bundle.Clusters != nil {
		t.Error("expected nil Clusters for empty input")
	}
}

func TestFrameAdapter_AdaptTracks_WithHistory(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	frame := &lidar.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []lidar.Point{},
	}

	// Create a tracker and update it multiple times to build history
	trackerCfg := lidar.DefaultTrackerConfig()
	tracker := lidar.NewTracker(trackerCfg)

	cluster := lidar.WorldCluster{
		ClusterID:         1,
		SensorID:          "hesai-01",
		CentroidX:         5.0,
		CentroidY:         10.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.5,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       100,
		HeightP95:         1.8,
		IntensityMean:     75.0,
	}

	// Update multiple times to build history
	for i := 0; i < 5; i++ {
		cluster.CentroidX = 5.0 + float32(i)*0.5
		cluster.CentroidY = 10.0 + float32(i)*0.5
		tracker.Update([]lidar.WorldCluster{cluster}, now.Add(time.Duration(i)*100*time.Millisecond))
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, tracker))

	if bundle.Tracks == nil {
		t.Fatal("expected non-nil Tracks")
	}

	// Check that trails are populated
	if len(bundle.Tracks.Trails) == 0 {
		t.Error("expected at least one trail")
	}

	if len(bundle.Tracks.Trails) > 0 {
		trail := bundle.Tracks.Trails[0]
		if len(trail.Points) < 2 {
			t.Errorf("expected at least 2 trail points, got %d", len(trail.Points))
		}
	}
}

func TestApplyDecimation_RatioOne(t *testing.T) {
	pc := &PointCloudFrame{
		X:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Y:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Z:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Intensity:      []uint8{100, 110, 120, 130, 140, 150, 160, 170, 180, 190},
		Classification: []uint8{1, 0, 1, 0, 1, 0, 1, 0, 1, 0},
		PointCount:     10,
	}

	pc.ApplyDecimation(DecimationUniform, 1.0)

	// Ratio 1.0 should keep all points
	if len(pc.X) != 10 {
		t.Errorf("Expected 10 points with ratio=1.0, got %d", len(pc.X))
	}
	if pc.DecimationMode != DecimationUniform {
		t.Error("DecimationMode should be set even with ratio=1.0")
	}
	if pc.DecimationRatio != 1.0 {
		t.Error("DecimationRatio should be 1.0")
	}
}

func TestApplyDecimation_UniformHalf(t *testing.T) {
	pc := &PointCloudFrame{
		X:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Y:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Z:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Intensity:      []uint8{100, 110, 120, 130, 140, 150, 160, 170, 180, 190},
		Classification: []uint8{1, 0, 1, 0, 1, 0, 1, 0, 1, 0},
		PointCount:     10,
	}

	pc.ApplyDecimation(DecimationUniform, 0.5)

	// Ratio 0.5 should keep approximately half the points
	if pc.PointCount == 0 || pc.PointCount > 10 {
		t.Errorf("Expected reduced point count with ratio=0.5, got %d", pc.PointCount)
	}
	if pc.DecimationMode != DecimationUniform {
		t.Error("DecimationMode should be DecimationUniform")
	}
	if pc.DecimationRatio != 0.5 {
		t.Errorf("DecimationRatio should be 0.5, got %f", pc.DecimationRatio)
	}
}

func TestApplyDecimation_UniformSmallRatio(t *testing.T) {
	pc := &PointCloudFrame{
		X:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Y:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Z:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Intensity:      []uint8{100, 110, 120, 130, 140, 150, 160, 170, 180, 190},
		Classification: []uint8{1, 0, 1, 0, 1, 0, 1, 0, 1, 0},
		PointCount:     10,
	}

	pc.ApplyDecimation(DecimationUniform, 0.1)

	// Ratio 0.1 should keep at least 1 point
	if pc.PointCount < 1 {
		t.Errorf("Expected at least 1 point with ratio=0.1, got %d", pc.PointCount)
	}
	if pc.DecimationMode != DecimationUniform {
		t.Error("DecimationMode should be DecimationUniform")
	}
}

func TestApplyDecimation_ForegroundOnly(t *testing.T) {
	pc := &PointCloudFrame{
		X:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Y:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Z:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Intensity:      []uint8{100, 110, 120, 130, 140, 150, 160, 170, 180, 190},
		Classification: []uint8{1, 0, 1, 0, 1, 0, 1, 0, 1, 0}, // 5 foreground, 5 background
		PointCount:     10,
	}

	pc.ApplyDecimation(DecimationForegroundOnly, 0.5)

	// Should keep only foreground points (5 of them)
	if pc.PointCount != 5 {
		t.Errorf("Expected 5 foreground points, got %d", pc.PointCount)
	}
	if pc.DecimationMode != DecimationForegroundOnly {
		t.Error("DecimationMode should be DecimationForegroundOnly")
	}
	// All remaining points should be foreground
	for i, c := range pc.Classification {
		if c != 1 {
			t.Errorf("Expected all remaining points to be foreground, got classification=%d at index %d", c, i)
		}
	}
}

func TestApplyDecimation_ForegroundOnlyAllForeground(t *testing.T) {
	pc := &PointCloudFrame{
		X:              []float32{1, 2, 3, 4, 5},
		Y:              []float32{1, 2, 3, 4, 5},
		Z:              []float32{1, 2, 3, 4, 5},
		Intensity:      []uint8{100, 110, 120, 130, 140},
		Classification: []uint8{1, 1, 1, 1, 1}, // all foreground
		PointCount:     5,
	}

	pc.ApplyDecimation(DecimationForegroundOnly, 1.0)

	// Should keep all 5 foreground points
	if pc.PointCount != 5 {
		t.Errorf("Expected 5 points (all foreground), got %d", pc.PointCount)
	}
}

func TestApplyDecimation_ForegroundOnlyNoForeground(t *testing.T) {
	pc := &PointCloudFrame{
		X:              []float32{1, 2, 3, 4, 5},
		Y:              []float32{1, 2, 3, 4, 5},
		Z:              []float32{1, 2, 3, 4, 5},
		Intensity:      []uint8{100, 110, 120, 130, 140},
		Classification: []uint8{0, 0, 0, 0, 0}, // all background
		PointCount:     5,
	}

	pc.ApplyDecimation(DecimationForegroundOnly, 1.0)

	// Should keep 0 points (no foreground)
	if pc.PointCount != 0 {
		t.Errorf("Expected 0 points (no foreground), got %d", pc.PointCount)
	}
}

func TestApplyDecimation_NoneMode(t *testing.T) {
	pc := &PointCloudFrame{
		X:              []float32{1, 2, 3, 4, 5},
		Y:              []float32{1, 2, 3, 4, 5},
		Z:              []float32{1, 2, 3, 4, 5},
		Intensity:      []uint8{100, 110, 120, 130, 140},
		Classification: []uint8{1, 0, 1, 0, 1},
		PointCount:     5,
	}

	pc.ApplyDecimation(DecimationNone, 0.5)

	// DecimationNone should keep all points regardless of ratio
	if pc.PointCount != 5 {
		t.Errorf("Expected 5 points with DecimationNone, got %d", pc.PointCount)
	}
	if pc.DecimationMode != DecimationNone {
		t.Error("DecimationMode should remain DecimationNone")
	}
}

func TestApplyDecimation_InvalidRatioZero(t *testing.T) {
	pc := &PointCloudFrame{
		X:              []float32{1, 2, 3, 4, 5},
		Y:              []float32{1, 2, 3, 4, 5},
		Z:              []float32{1, 2, 3, 4, 5},
		Intensity:      []uint8{100, 110, 120, 130, 140},
		Classification: []uint8{1, 0, 1, 0, 1},
		PointCount:     5,
		DecimationMode: DecimationNone,
	}

	pc.ApplyDecimation(DecimationUniform, 0) // Invalid ratio

	// Invalid ratio should not change anything
	if pc.PointCount != 5 {
		t.Errorf("Expected 5 points with invalid ratio, got %d", pc.PointCount)
	}
}

func TestApplyDecimation_InvalidRatioNegative(t *testing.T) {
	pc := &PointCloudFrame{
		X:              []float32{1, 2, 3, 4, 5},
		Y:              []float32{1, 2, 3, 4, 5},
		Z:              []float32{1, 2, 3, 4, 5},
		Intensity:      []uint8{100, 110, 120, 130, 140},
		Classification: []uint8{1, 0, 1, 0, 1},
		PointCount:     5,
		DecimationMode: DecimationNone,
	}

	pc.ApplyDecimation(DecimationUniform, -0.5) // Invalid ratio

	// Invalid ratio should not change anything
	if pc.PointCount != 5 {
		t.Errorf("Expected 5 points with negative ratio, got %d", pc.PointCount)
	}
}

func TestApplyDecimation_InvalidRatioGreaterThanOne(t *testing.T) {
	pc := &PointCloudFrame{
		X:              []float32{1, 2, 3, 4, 5},
		Y:              []float32{1, 2, 3, 4, 5},
		Z:              []float32{1, 2, 3, 4, 5},
		Intensity:      []uint8{100, 110, 120, 130, 140},
		Classification: []uint8{1, 0, 1, 0, 1},
		PointCount:     5,
		DecimationMode: DecimationNone,
	}

	pc.ApplyDecimation(DecimationUniform, 1.5) // Invalid ratio

	// Invalid ratio should not change anything
	if pc.PointCount != 5 {
		t.Errorf("Expected 5 points with ratio > 1, got %d", pc.PointCount)
	}
}

func TestApplyDecimation_VoxelFallback(t *testing.T) {
	pc := &PointCloudFrame{
		X:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Y:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Z:              []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		Intensity:      []uint8{100, 110, 120, 130, 140, 150, 160, 170, 180, 190},
		Classification: []uint8{1, 0, 1, 0, 1, 0, 1, 0, 1, 0},
		PointCount:     10,
	}

	pc.ApplyDecimation(DecimationVoxel, 0.5)

	// Voxel decimation falls back to uniform, should reduce points
	if pc.PointCount == 10 {
		t.Error("Expected reduced point count with DecimationVoxel")
	}
	if pc.DecimationMode != DecimationVoxel {
		t.Error("DecimationMode should be DecimationVoxel")
	}
}
