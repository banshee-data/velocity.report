// Package visualiser provides gRPC streaming of LiDAR perception data.
package visualiser

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/debug"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
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

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []l2frames.Point{},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil, nil))

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

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points:         []l2frames.Point{},
	}

	bundle1 := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil, nil))
	bundle2 := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil, nil))
	bundle3 := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil, nil))

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

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []l2frames.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
			{X: 3.0, Y: 4.0, Z: 1.0, Intensity: 150},
			{X: 5.0, Y: 6.0, Z: 1.5, Intensity: 200},
		},
	}

	mask := []bool{true, false, true} // foreground, background, foreground

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, mask, nil, nil, nil))

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

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []l2frames.Point{},
	}

	clusters := []l4perception.WorldCluster{
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

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, clusters, nil, nil))

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

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []l2frames.Point{},
	}

	// Create a tracker with a track
	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	// Add a cluster to create a track
	cluster := l4perception.WorldCluster{
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
	tracker.Update([]l4perception.WorldCluster{cluster}, now)

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, tracker, nil))

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
		input    l5tracks.TrackState
		expected TrackState
	}{
		{l5tracks.TrackTentative, TrackStateTentative},
		{l5tracks.TrackConfirmed, TrackStateConfirmed},
		{l5tracks.TrackDeleted, TrackStateDeleted},
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

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []l2frames.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
			{X: 3.0, Y: 4.0, Z: 1.0, Intensity: 150},
		},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil, nil))

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

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []l2frames.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
			{X: 3.0, Y: 4.0, Z: 1.0, Intensity: 150},
			{X: 5.0, Y: 6.0, Z: 1.5, Intensity: 200},
		},
	}

	// Mask shorter than points
	mask := []bool{true}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, mask, nil, nil, nil))

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

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points:         []l2frames.Point{},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, []l4perception.WorldCluster{}, nil, nil))

	// Empty clusters slice should result in nil Clusters
	if bundle.Clusters != nil {
		t.Error("expected nil Clusters for empty input")
	}
}

func TestFrameAdapter_AdaptTracks_WithHistory(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []l2frames.Point{},
	}

	// Create a tracker and update it multiple times to build history
	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	cluster := l4perception.WorldCluster{
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
		tracker.Update([]l4perception.WorldCluster{cluster}, now.Add(time.Duration(i)*100*time.Millisecond))
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, tracker, nil))

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

// TestFrameAdapter_AdaptTracks_HistoryLengthConsistency tests that trail point
// allocation matches the number of history points copied. This is a regression
// test for a race condition where History could grow during iteration.
func TestFrameAdapter_AdaptTracks_HistoryLengthConsistency(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []l2frames.Point{},
	}

	// Create a tracker and build a significant history
	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	cluster := l4perception.WorldCluster{
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

	// Update many times to build a longer history (similar to 180 points case)
	for i := 0; i < 200; i++ {
		cluster.CentroidX = 5.0 + float32(i)*0.1
		cluster.CentroidY = 10.0 + float32(i)*0.1
		tracker.Update([]l4perception.WorldCluster{cluster}, now.Add(time.Duration(i)*10*time.Millisecond))
	}

	// This should not panic even with a long history
	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, tracker, nil))

	if bundle.Tracks == nil {
		t.Fatal("expected non-nil Tracks")
	}

	// Verify trails are present and have consistent length
	if len(bundle.Tracks.Trails) == 0 {
		t.Error("expected at least one trail")
	}

	for _, trail := range bundle.Tracks.Trails {
		// The number of points allocated should equal the actual points
		// (i.e., no panic from index out of range)
		if len(trail.Points) == 0 {
			t.Error("trail should have points")
		}
		// Verify all points are valid (non-zero timestamps for real history)
		for i, pt := range trail.Points {
			if pt.TimestampNanos == 0 && i > 0 {
				// First point might be zero in some edge cases
				t.Errorf("trail point %d has zero timestamp", i)
			}
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
	// Create points that cluster within voxels — several points within
	// the same voxel cell should be reduced to a single representative.
	// With ratio 0.5, leafSize = 0.04/0.5 = 0.08m.
	// Place groups of points within 0.08m cubes.
	pc := &PointCloudFrame{
		X:              []float32{0.01, 0.02, 0.03, 0.04, 0.05, 0.50, 0.51, 0.52, 0.53, 0.54},
		Y:              []float32{0.01, 0.02, 0.03, 0.04, 0.05, 0.50, 0.51, 0.52, 0.53, 0.54},
		Z:              []float32{0.01, 0.02, 0.03, 0.04, 0.05, 0.50, 0.51, 0.52, 0.53, 0.54},
		Intensity:      []uint8{100, 110, 120, 130, 140, 150, 160, 170, 180, 190},
		Classification: []uint8{1, 0, 1, 0, 1, 0, 1, 0, 1, 0},
		PointCount:     10,
	}

	pc.ApplyDecimation(DecimationVoxel, 0.5)

	// Voxel decimation should reduce points since multiple points share voxels
	if pc.PointCount >= 10 {
		t.Errorf("Expected reduced point count with DecimationVoxel, got %d", pc.PointCount)
	}
	if pc.DecimationMode != DecimationVoxel {
		t.Error("DecimationMode should be DecimationVoxel")
	}
}

// =============================================================================
// Tests for adaptForegroundOnly (SplitStreaming mode)
// =============================================================================

func TestFrameAdapter_SplitStreaming_ForegroundOnly(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	adapter.SplitStreaming = true

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []l2frames.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100}, // foreground
			{X: 3.0, Y: 4.0, Z: 1.0, Intensity: 150}, // background
			{X: 5.0, Y: 6.0, Z: 1.5, Intensity: 200}, // foreground
			{X: 7.0, Y: 8.0, Z: 2.0, Intensity: 250}, // background
		},
	}

	mask := []bool{true, false, true, false} // 2 foreground, 2 background

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, mask, nil, nil, nil))

	if bundle.PointCloud == nil {
		t.Fatal("expected non-nil PointCloud")
	}

	pc := bundle.PointCloud
	// SplitStreaming should only include foreground points
	if pc.PointCount != 2 {
		t.Errorf("expected PointCount=2 (foreground only), got %d", pc.PointCount)
	}
	if len(pc.X) != 2 {
		t.Errorf("expected X length=2, got %d", len(pc.X))
	}

	// Verify only foreground points were kept
	if pc.X[0] != 1.0 || pc.X[1] != 5.0 {
		t.Errorf("expected foreground X values [1.0, 5.0], got [%f, %f]", pc.X[0], pc.X[1])
	}
	if pc.Y[0] != 2.0 || pc.Y[1] != 6.0 {
		t.Errorf("expected foreground Y values [2.0, 6.0], got [%f, %f]", pc.Y[0], pc.Y[1])
	}

	// All kept points should be classified as foreground
	for i := 0; i < pc.PointCount; i++ {
		if pc.Classification[i] != 1 {
			t.Errorf("expected Classification[%d]=1, got %d", i, pc.Classification[i])
		}
	}

	// Verify decimation mode is set correctly
	if pc.DecimationMode != DecimationForegroundOnly {
		t.Errorf("expected DecimationMode=DecimationForegroundOnly, got %d", pc.DecimationMode)
	}

	// Verify frame type is set for split streaming
	if bundle.FrameType != FrameTypeForeground {
		t.Errorf("expected FrameType=FrameTypeForeground, got %d", bundle.FrameType)
	}
}

func TestFrameAdapter_SplitStreaming_AllForeground(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	adapter.SplitStreaming = true

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []l2frames.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
			{X: 3.0, Y: 4.0, Z: 1.0, Intensity: 150},
			{X: 5.0, Y: 6.0, Z: 1.5, Intensity: 200},
		},
	}

	mask := []bool{true, true, true} // All foreground

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, mask, nil, nil, nil))

	pc := bundle.PointCloud
	if pc.PointCount != 3 {
		t.Errorf("expected PointCount=3 (all foreground), got %d", pc.PointCount)
	}
}

func TestFrameAdapter_SplitStreaming_NoForeground(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	adapter.SplitStreaming = true

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []l2frames.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
			{X: 3.0, Y: 4.0, Z: 1.0, Intensity: 150},
		},
	}

	mask := []bool{false, false} // All background

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, mask, nil, nil, nil))

	pc := bundle.PointCloud
	if pc.PointCount != 0 {
		t.Errorf("expected PointCount=0 (no foreground), got %d", pc.PointCount)
	}
}

func TestFrameAdapter_SplitStreaming_EmptyMask(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	adapter.SplitStreaming = true

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []l2frames.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
		},
	}

	// Empty mask with SplitStreaming - should fall back to normal path
	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, []bool{}, nil, nil, nil))

	pc := bundle.PointCloud
	// Empty mask returns all points via normal path
	if pc.PointCount != 1 {
		t.Errorf("expected PointCount=1 with empty mask, got %d", pc.PointCount)
	}
	// FrameType should not be set to Foreground when mask is empty
	if bundle.FrameType == FrameTypeForeground {
		t.Error("expected FrameType != FrameTypeForeground with empty mask")
	}
}

func TestFrameAdapter_SplitStreaming_PartialMask(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	adapter.SplitStreaming = true

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []l2frames.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
			{X: 3.0, Y: 4.0, Z: 1.0, Intensity: 150},
			{X: 5.0, Y: 6.0, Z: 1.5, Intensity: 200},
		},
	}

	// Mask shorter than points - points beyond mask treated as background
	mask := []bool{true}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, mask, nil, nil, nil))

	pc := bundle.PointCloud
	// Only first point should be included (foreground)
	if pc.PointCount != 1 {
		t.Errorf("expected PointCount=1 with partial mask, got %d", pc.PointCount)
	}
	if pc.X[0] != 1.0 {
		t.Errorf("expected X[0]=1.0, got %f", pc.X[0])
	}
}

// =============================================================================
// Tests for adaptClusters (direct cluster conversion)
// =============================================================================

func TestAdaptClusters_Direct(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	adapter.frameID = 42 // Set a known frame ID
	now := time.Now()

	clusters := []l4perception.WorldCluster{
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
			CentroidX:         20.0,
			CentroidY:         30.0,
			CentroidZ:         0.5,
			BoundingBoxLength: 2.0,
			BoundingBoxWidth:  1.5,
			BoundingBoxHeight: 0.8,
			PointsCount:       50,
			HeightP95:         0.7,
			IntensityMean:     50.0,
		},
	}

	cs := adapter.adaptClusters(clusters, now)

	if cs == nil {
		t.Fatal("expected non-nil ClusterSet")
	}
	if cs.FrameID != 42 {
		t.Errorf("expected FrameID=42, got %d", cs.FrameID)
	}
	if cs.TimestampNanos != now.UnixNano() {
		t.Errorf("expected TimestampNanos=%d, got %d", now.UnixNano(), cs.TimestampNanos)
	}
	if cs.Method != ClusteringDBSCAN {
		t.Errorf("expected Method=ClusteringDBSCAN, got %d", cs.Method)
	}
	if len(cs.Clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(cs.Clusters))
	}

	// Verify first cluster
	c1 := cs.Clusters[0]
	if c1.ClusterID != 1 {
		t.Errorf("expected ClusterID=1, got %d", c1.ClusterID)
	}
	if c1.CentroidX != 5.0 {
		t.Errorf("expected CentroidX=5.0, got %f", c1.CentroidX)
	}
	if c1.CentroidY != 10.0 {
		t.Errorf("expected CentroidY=10.0, got %f", c1.CentroidY)
	}
	if c1.AABBLength != 4.5 {
		t.Errorf("expected AABBLength=4.5, got %f", c1.AABBLength)
	}
	if c1.PointsCount != 100 {
		t.Errorf("expected PointsCount=100, got %d", c1.PointsCount)
	}
	if c1.HeightP95 != 1.8 {
		t.Errorf("expected HeightP95=1.8, got %f", c1.HeightP95)
	}
	if c1.IntensityMean != 75.0 {
		t.Errorf("expected IntensityMean=75.0, got %f", c1.IntensityMean)
	}

	// Verify second cluster
	c2 := cs.Clusters[1]
	if c2.ClusterID != 2 {
		t.Errorf("expected ClusterID=2, got %d", c2.ClusterID)
	}
	if c2.CentroidX != 20.0 {
		t.Errorf("expected CentroidX=20.0, got %f", c2.CentroidX)
	}
}

func TestAdaptClusters_Empty(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	cs := adapter.adaptClusters([]l4perception.WorldCluster{}, now)

	if cs == nil {
		t.Fatal("expected non-nil ClusterSet")
	}
	if len(cs.Clusters) != 0 {
		t.Errorf("expected 0 clusters, got %d", len(cs.Clusters))
	}
}

// =============================================================================
// Tests for adaptDebugFrame
// =============================================================================

func TestAdaptDebugFrame_Full(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	df := &debug.DebugFrame{
		FrameID: 123,
		AssociationCandidates: []debug.AssociationRecord{
			{ClusterID: 1, TrackID: "track-001", MahalanobisDistSquared: 2.5, Accepted: true},
			{ClusterID: 2, TrackID: "track-001", MahalanobisDistSquared: 10.0, Accepted: false},
		},
		GatingRegions: []debug.GatingRegion{
			{TrackID: "track-001", CenterX: 5.0, CenterY: 10.0, SemiMajorM: 3.0, SemiMinorM: 2.0, RotationRad: 0.5},
		},
		Innovations: []debug.KalmanInnovation{
			{TrackID: "track-001", PredictedX: 4.9, PredictedY: 9.8, MeasuredX: 5.0, MeasuredY: 10.0, ResidualMag: 0.22},
		},
		StatePredictions: []debug.StatePrediction{
			{TrackID: "track-001", X: 4.9, Y: 9.8, VX: 1.0, VY: 2.0},
		},
	}

	overlay := adapter.adaptDebugFrame(df, now)

	if overlay == nil {
		t.Fatal("expected non-nil DebugOverlaySet")
	}
	if overlay.FrameID != 123 {
		t.Errorf("expected FrameID=123, got %d", overlay.FrameID)
	}
	if overlay.TimestampNanos != now.UnixNano() {
		t.Errorf("expected TimestampNanos=%d, got %d", now.UnixNano(), overlay.TimestampNanos)
	}

	// Check AssociationCandidates
	if len(overlay.AssociationCandidates) != 2 {
		t.Errorf("expected 2 association candidates, got %d", len(overlay.AssociationCandidates))
	}
	ac := overlay.AssociationCandidates[0]
	if ac.ClusterID != 1 {
		t.Errorf("expected ClusterID=1, got %d", ac.ClusterID)
	}
	if ac.TrackID != "track-001" {
		t.Errorf("expected TrackID=track-001, got %s", ac.TrackID)
	}
	if ac.Distance != 2.5 {
		t.Errorf("expected Distance=2.5, got %f", ac.Distance)
	}
	if !ac.Accepted {
		t.Error("expected Accepted=true")
	}
	if overlay.AssociationCandidates[1].Accepted {
		t.Error("expected second candidate Accepted=false")
	}

	// Check GatingEllipses
	if len(overlay.GatingEllipses) != 1 {
		t.Errorf("expected 1 gating ellipse, got %d", len(overlay.GatingEllipses))
	}
	ge := overlay.GatingEllipses[0]
	if ge.TrackID != "track-001" {
		t.Errorf("expected TrackID=track-001, got %s", ge.TrackID)
	}
	if ge.CenterX != 5.0 {
		t.Errorf("expected CenterX=5.0, got %f", ge.CenterX)
	}
	if ge.SemiMajor != 3.0 {
		t.Errorf("expected SemiMajor=3.0, got %f", ge.SemiMajor)
	}
	if ge.SemiMinor != 2.0 {
		t.Errorf("expected SemiMinor=2.0, got %f", ge.SemiMinor)
	}
	if ge.RotationRad != 0.5 {
		t.Errorf("expected RotationRad=0.5, got %f", ge.RotationRad)
	}

	// Check Residuals
	if len(overlay.Residuals) != 1 {
		t.Errorf("expected 1 residual, got %d", len(overlay.Residuals))
	}
	res := overlay.Residuals[0]
	if res.TrackID != "track-001" {
		t.Errorf("expected TrackID=track-001, got %s", res.TrackID)
	}
	if res.PredictedX != 4.9 {
		t.Errorf("expected PredictedX=4.9, got %f", res.PredictedX)
	}
	if res.MeasuredX != 5.0 {
		t.Errorf("expected MeasuredX=5.0, got %f", res.MeasuredX)
	}
	if res.ResidualMagnitude != 0.22 {
		t.Errorf("expected ResidualMagnitude=0.22, got %f", res.ResidualMagnitude)
	}

	// Check Predictions
	if len(overlay.Predictions) != 1 {
		t.Errorf("expected 1 prediction, got %d", len(overlay.Predictions))
	}
	pred := overlay.Predictions[0]
	if pred.TrackID != "track-001" {
		t.Errorf("expected TrackID=track-001, got %s", pred.TrackID)
	}
	if pred.X != 4.9 {
		t.Errorf("expected X=4.9, got %f", pred.X)
	}
	if pred.VX != 1.0 {
		t.Errorf("expected VX=1.0, got %f", pred.VX)
	}
}

func TestAdaptDebugFrame_Nil(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	overlay := adapter.adaptDebugFrame(nil, now)

	if overlay != nil {
		t.Error("expected nil DebugOverlaySet for nil input")
	}
}

func TestAdaptDebugFrame_WrongType(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	// Pass a string instead of *debug.DebugFrame
	overlay := adapter.adaptDebugFrame("not a debug frame", now)

	if overlay != nil {
		t.Error("expected nil DebugOverlaySet for wrong type")
	}
}

func TestAdaptDebugFrame_Empty(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	df := &debug.DebugFrame{
		FrameID:               456,
		AssociationCandidates: []debug.AssociationRecord{},
		GatingRegions:         []debug.GatingRegion{},
		Innovations:           []debug.KalmanInnovation{},
		StatePredictions:      []debug.StatePrediction{},
	}

	overlay := adapter.adaptDebugFrame(df, now)

	if overlay == nil {
		t.Fatal("expected non-nil DebugOverlaySet")
	}
	if overlay.FrameID != 456 {
		t.Errorf("expected FrameID=456, got %d", overlay.FrameID)
	}
	if len(overlay.AssociationCandidates) != 0 {
		t.Errorf("expected 0 association candidates, got %d", len(overlay.AssociationCandidates))
	}
	if len(overlay.GatingEllipses) != 0 {
		t.Errorf("expected 0 gating ellipses, got %d", len(overlay.GatingEllipses))
	}
	if len(overlay.Residuals) != 0 {
		t.Errorf("expected 0 residuals, got %d", len(overlay.Residuals))
	}
	if len(overlay.Predictions) != 0 {
		t.Errorf("expected 0 predictions, got %d", len(overlay.Predictions))
	}
}

func TestFrameAdapter_AdaptFrame_WithDebugFrame(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []l2frames.Point{},
	}

	df := &debug.DebugFrame{
		FrameID: 99,
		AssociationCandidates: []debug.AssociationRecord{
			{ClusterID: 1, TrackID: "track-001", MahalanobisDistSquared: 1.0, Accepted: true},
		},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil, df))

	if bundle.Debug == nil {
		t.Fatal("expected non-nil Debug")
	}
	if bundle.Debug.FrameID != 99 {
		t.Errorf("expected Debug.FrameID=99, got %d", bundle.Debug.FrameID)
	}
	if len(bundle.Debug.AssociationCandidates) != 1 {
		t.Errorf("expected 1 association candidate, got %d", len(bundle.Debug.AssociationCandidates))
	}
}

// =============================================================================
// Tests for slice pool functions
// =============================================================================

func TestGetFloat32Slice_Normal(t *testing.T) {
	s := getFloat32Slice(100)
	if len(s) != 100 {
		t.Errorf("expected length=100, got %d", len(s))
	}
	if cap(s) < 100 {
		t.Errorf("expected capacity >= 100, got %d", cap(s))
	}

	// Return to pool
	putFloat32Slice(s)
}

func TestGetFloat32Slice_Large(t *testing.T) {
	// Request a slice larger than the pool's pre-allocated size
	s := getFloat32Slice(200000)
	if len(s) != 200000 {
		t.Errorf("expected length=200000, got %d", len(s))
	}

	// Should still return without issue
	putFloat32Slice(s)
}

func TestGetFloat32Slice_Zero(t *testing.T) {
	s := getFloat32Slice(0)
	if len(s) != 0 {
		t.Errorf("expected length=0, got %d", len(s))
	}
}

func TestGetUint8Slice_Normal(t *testing.T) {
	s := getUint8Slice(100)
	if len(s) != 100 {
		t.Errorf("expected length=100, got %d", len(s))
	}
	if cap(s) < 100 {
		t.Errorf("expected capacity >= 100, got %d", cap(s))
	}

	// Return to pool
	putUint8Slice(s)
}

func TestGetUint8Slice_Large(t *testing.T) {
	// Request a slice larger than the pool's pre-allocated size
	s := getUint8Slice(200000)
	if len(s) != 200000 {
		t.Errorf("expected length=200000, got %d", len(s))
	}

	// Should still return without issue
	putUint8Slice(s)
}

func TestPutFloat32Slice_TooLarge(t *testing.T) {
	// Create a very large slice
	s := make([]float32, 200000)

	// This should not panic (slice is not pooled because it's too large)
	putFloat32Slice(s)
}

func TestPutUint8Slice_TooLarge(t *testing.T) {
	// Create a very large slice
	s := make([]uint8, 200000)

	// This should not panic (slice is not pooled because it's too large)
	putUint8Slice(s)
}

// =============================================================================
// Tests for PointCloudFrame.Release() and reference counting
// =============================================================================

func TestPointCloudFrame_Release_Basic(t *testing.T) {
	pc := &PointCloudFrame{
		X:              getFloat32Slice(10),
		Y:              getFloat32Slice(10),
		Z:              getFloat32Slice(10),
		Intensity:      getUint8Slice(10),
		Classification: getUint8Slice(10),
		PointCount:     10,
	}

	// Release should clear the slices
	pc.Release()

	if pc.X != nil {
		t.Error("expected X to be nil after Release")
	}
	if pc.Y != nil {
		t.Error("expected Y to be nil after Release")
	}
	if pc.Z != nil {
		t.Error("expected Z to be nil after Release")
	}
	if pc.Intensity != nil {
		t.Error("expected Intensity to be nil after Release")
	}
	if pc.Classification != nil {
		t.Error("expected Classification to be nil after Release")
	}
}

func TestPointCloudFrame_Release_Nil(t *testing.T) {
	var pc *PointCloudFrame = nil
	// Should not panic
	pc.Release()
}

func TestPointCloudFrame_Release_WithRetain(t *testing.T) {
	pc := &PointCloudFrame{
		X:              getFloat32Slice(10),
		Y:              getFloat32Slice(10),
		Z:              getFloat32Slice(10),
		Intensity:      getUint8Slice(10),
		Classification: getUint8Slice(10),
		PointCount:     10,
	}

	// Retain increments ref count (starting from 0)
	pc.Retain() // refCount = 1
	pc.Retain() // refCount = 2

	// First release - ref count goes to 1, slices should remain
	pc.Release()
	if pc.X == nil {
		t.Error("expected X to still exist after first Release (refCount should be 1)")
	}

	// Second release - ref count goes to 0, slices should be released
	pc.Release()
	if pc.X != nil {
		t.Error("expected X to be nil after second Release (refCount should be 0)")
	}
}

// =============================================================================
// Tests for AdaptFrame edge cases
// =============================================================================

func TestFrameAdapter_AdaptFrame_FrameWithNoPoints(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	// Frame with nil Points slice - should create bundle with nil PointCloud
	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points:         nil, // No points
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil, nil))

	if bundle.PointCloud != nil {
		t.Error("expected nil PointCloud for frame with no points")
	}
}

func TestFrameAdapter_AdaptFrame_EmptyFrame(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points:         []l2frames.Point{}, // Empty points
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, nil, nil))

	// Empty points should result in nil PointCloud
	if bundle.PointCloud != nil {
		t.Error("expected nil PointCloud for empty frame")
	}
}

func TestFrameAdapter_AdaptFrame_FullPipeline(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points: []l2frames.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
			{X: 3.0, Y: 4.0, Z: 1.0, Intensity: 150},
		},
	}

	mask := []bool{true, false}

	// Create tracker with a cluster
	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)
	cluster := l4perception.WorldCluster{
		ClusterID:         1,
		SensorID:          "hesai-01",
		CentroidX:         5.0,
		CentroidY:         10.0,
		BoundingBoxLength: 4.5,
		BoundingBoxWidth:  2.0,
	}
	tracker.Update([]l4perception.WorldCluster{cluster}, now)

	// Create debug frame
	df := &debug.DebugFrame{
		FrameID: 1,
		AssociationCandidates: []debug.AssociationRecord{
			{ClusterID: 1, TrackID: "track-001", MahalanobisDistSquared: 1.0, Accepted: true},
		},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, mask, []l4perception.WorldCluster{cluster}, tracker, df))

	// Verify all components are present
	if bundle.PointCloud == nil {
		t.Error("expected non-nil PointCloud")
	}
	if bundle.Tracks == nil {
		t.Error("expected non-nil Tracks")
	}
	if bundle.Debug == nil {
		t.Error("expected non-nil Debug")
	}
	// Clusters should be nil because the cluster was associated with a track
	// (adaptUnassociatedClusters filters them out)
}

func TestFrameAdapter_AdaptFrame_StatsLogging(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: time.Now(),
		Points: []l2frames.Point{
			{X: 1.0, Y: 2.0, Z: 0.5, Intensity: 100},
		},
	}

	// Call AdaptFrame multiple times to trigger stats logging at 100 frames
	for i := 0; i < 101; i++ {
		adapter.AdaptFrame(frame, nil, nil, nil, nil)
	}

	// Verify internal counters
	if adapter.frameCount.Load() != 101 {
		t.Errorf("expected frameCount=101, got %d", adapter.frameCount.Load())
	}
	if adapter.totalAdaptTimeNs.Load() <= 0 {
		t.Error("expected positive totalAdaptTimeNs")
	}
}

// =============================================================================
// Tests for adaptTracks additional paths
// =============================================================================

func TestFrameAdapter_AdaptTracks_WithCovariance(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []l2frames.Point{},
	}

	// Create tracker with multiple updates to populate covariance
	trackerCfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(trackerCfg)

	cluster := l4perception.WorldCluster{
		ClusterID:         1,
		SensorID:          "hesai-01",
		CentroidX:         5.0,
		CentroidY:         10.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.5,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       100,
	}

	// Multiple updates to build covariance matrix
	for i := 0; i < 10; i++ {
		cluster.CentroidX = 5.0 + float32(i)*0.5
		tracker.Update([]l4perception.WorldCluster{cluster}, now.Add(time.Duration(i)*100*time.Millisecond))
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, tracker, nil))

	if bundle.Tracks == nil {
		t.Fatal("expected non-nil Tracks")
	}
	if len(bundle.Tracks.Tracks) == 0 {
		t.Fatal("expected at least one track")
	}

	// Track should have covariance populated
	track := bundle.Tracks.Tracks[0]
	if track.Covariance4x4 == nil {
		t.Error("expected non-nil Covariance4x4")
	}
}

func TestFrameAdapter_AdaptTracks_DeletedTracksFade(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	// Create a tracker with short grace period for testing
	trackerCfg := l5tracks.DefaultTrackerConfig()
	trackerCfg.DeletedTrackGracePeriod = 5 * time.Second
	tracker := l5tracks.NewTracker(trackerCfg)

	cluster := l4perception.WorldCluster{
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

	// Build a confirmed track - needs multiple consecutive hits
	// HitsToConfirm is typically 3
	for i := 0; i < 5; i++ {
		cluster.CentroidX = 5.0 + float32(i)*0.5
		tracker.Update([]l4perception.WorldCluster{cluster}, now.Add(time.Duration(i)*100*time.Millisecond))
	}

	// Verify track exists
	activeTracks := tracker.GetActiveTracks()
	if len(activeTracks) == 0 {
		t.Fatal("expected at least one active track after updates")
	}

	// Now remove the cluster to trigger track deletion
	// Need enough misses to delete the track
	for i := 0; i < 10; i++ {
		tracker.Update([]l4perception.WorldCluster{}, now.Add(time.Duration(500+i*100)*time.Millisecond))
	}

	// Create frame with timestamp just after track deletion
	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now.Add(2 * time.Second), // 2 seconds after start
		Points:         []l2frames.Point{},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, tracker, nil))

	// The test exercises the deleted tracks fade-out code path
	// The deleted track may or may not appear depending on timing
	if bundle.Tracks != nil {
		// If tracks are present, verify the fade-out alpha calculation path was exercised
		for _, track := range bundle.Tracks.Tracks {
			if track.State == TrackStateDeleted {
				// Deleted track was included - verify alpha is in valid range [0, 1]
				if track.Alpha < 0 || track.Alpha > 1 {
					t.Errorf("expected Alpha in [0, 1], got %f", track.Alpha)
				}
			}
		}
	}
}

// TestFrameAdapter_AdaptTracks_DeletedTracksAlphaCalculation directly tests
// the alpha fade-out calculation for deleted tracks.
func TestFrameAdapter_AdaptTracks_DeletedTracksAlphaCalculation(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	// Create tracker with short grace period
	trackerCfg := l5tracks.DefaultTrackerConfig()
	trackerCfg.DeletedTrackGracePeriod = 5 * time.Second
	trackerCfg.HitsToConfirm = 3
	trackerCfg.MaxMisses = 3
	trackerCfg.MaxMissesConfirmed = 3 // Also set for confirmed tracks
	tracker := l5tracks.NewTracker(trackerCfg)

	baseTime := time.Now()

	cluster := l4perception.WorldCluster{
		ClusterID:         1,
		SensorID:          "hesai-01",
		CentroidX:         10.0,
		CentroidY:         20.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       50,
	}

	// Create a confirmed track with at least 3 observations
	for i := 0; i < 5; i++ {
		tracker.Update([]l4perception.WorldCluster{cluster}, baseTime.Add(time.Duration(i)*100*time.Millisecond))
	}

	// Force track deletion by removing the cluster (need MaxMissesConfirmed misses)
	for i := 0; i < 5; i++ {
		tracker.Update([]l4perception.WorldCluster{}, baseTime.Add(time.Duration(500+i*100)*time.Millisecond))
	}

	// Create frame at various times to test alpha calculation
	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: baseTime.Add(1500 * time.Millisecond), // Shortly after deletion
		Points:         []l2frames.Point{},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, tracker, nil))

	// This test exercises the deleted tracks code path
	// The track may or may not appear depending on exact timing
	if bundle.Tracks != nil {
		t.Logf("Track count: %d", len(bundle.Tracks.Tracks))
		for _, tr := range bundle.Tracks.Tracks {
			t.Logf("Track %s: State=%d Alpha=%f", tr.TrackID, tr.State, tr.Alpha)
		}
	}
}

// TestFrameAdapter_AdaptTracks_SkipsTentativeDeletedTracks verifies that
// tentative tracks that never confirmed are not included in fade-out.
func TestFrameAdapter_AdaptTracks_SkipsTentativeDeletedTracks(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	// Create tracker with high confirmation threshold
	trackerCfg := l5tracks.DefaultTrackerConfig()
	trackerCfg.DeletedTrackGracePeriod = 5 * time.Second
	trackerCfg.HitsToConfirm = 10 // High threshold - track won't confirm
	trackerCfg.MaxMisses = 2
	tracker := l5tracks.NewTracker(trackerCfg)

	baseTime := time.Now()

	cluster := l4perception.WorldCluster{
		ClusterID:         1,
		SensorID:          "hesai-01",
		CentroidX:         10.0,
		CentroidY:         20.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 2.0,
		BoundingBoxWidth:  1.5,
		BoundingBoxHeight: 0.8,
		PointsCount:       30,
	}

	// Create a tentative track with only 2 observations (< HitsToConfirm)
	for i := 0; i < 2; i++ {
		tracker.Update([]l4perception.WorldCluster{cluster}, baseTime.Add(time.Duration(i)*100*time.Millisecond))
	}

	// Delete the track
	for i := 0; i < 5; i++ {
		tracker.Update([]l4perception.WorldCluster{}, baseTime.Add(time.Duration(200+i*100)*time.Millisecond))
	}

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: baseTime.Add(1 * time.Second),
		Points:         []l2frames.Point{},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, tracker, nil))

	// Tentative tracks with low observation count should be skipped
	// (ObservationCount < 3)
	if bundle.Tracks != nil {
		for _, tr := range bundle.Tracks.Tracks {
			if tr.State == TrackStateDeleted && tr.ObservationCount < 3 {
				t.Errorf("tentative deleted track should be skipped: ObservationCount=%d", tr.ObservationCount)
			}
		}
	}
}

// TestFrameAdapter_AdaptTracks_DeletedTrackWithTrail tests that deleted tracks
// include their trail history.
func TestFrameAdapter_AdaptTracks_DeletedTrackWithTrail(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	trackerCfg := l5tracks.DefaultTrackerConfig()
	trackerCfg.DeletedTrackGracePeriod = 5 * time.Second
	trackerCfg.HitsToConfirm = 3
	trackerCfg.MaxMisses = 3
	trackerCfg.MaxMissesConfirmed = 3 // Also set for confirmed tracks
	tracker := l5tracks.NewTracker(trackerCfg)

	baseTime := time.Now()

	cluster := l4perception.WorldCluster{
		ClusterID:         1,
		SensorID:          "hesai-01",
		CentroidX:         10.0,
		CentroidY:         20.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       50,
	}

	// Create track with enough history points
	for i := 0; i < 10; i++ {
		cluster.CentroidX = 10.0 + float32(i)*0.5
		cluster.CentroidY = 20.0 + float32(i)*0.3
		tracker.Update([]l4perception.WorldCluster{cluster}, baseTime.Add(time.Duration(i)*100*time.Millisecond))
	}

	// Delete the track (need MaxMissesConfirmed misses)
	for i := 0; i < 5; i++ {
		tracker.Update([]l4perception.WorldCluster{}, baseTime.Add(time.Duration(1000+i*100)*time.Millisecond))
	}

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: baseTime.Add(2 * time.Second),
		Points:         []l2frames.Point{},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, tracker, nil))

	// Exercise the trail copying path for deleted tracks
	if bundle.Tracks != nil && len(bundle.Tracks.Trails) > 0 {
		for _, trail := range bundle.Tracks.Trails {
			if len(trail.Points) > 0 {
				// Verify trail points have valid data
				for i, pt := range trail.Points {
					if pt.X == 0 && pt.Y == 0 && i > 0 {
						t.Errorf("trail point %d has zero values", i)
					}
				}
			}
		}
	}
}

// =============================================================================
// Tests for OBB handling in clusters
// =============================================================================

func TestFrameAdapter_AdaptClusters_WithOBB(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")
	now := time.Now()

	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: now,
		Points:         []l2frames.Point{},
	}

	clusters := []l4perception.WorldCluster{
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
			OBB: &l4perception.OrientedBoundingBox{
				CenterX:    5.0,
				CenterY:    10.0,
				CenterZ:    1.0,
				Length:     4.5,
				Width:      2.0,
				Height:     1.5,
				HeadingRad: 0.785, // ~45 degrees
			},
		},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, clusters, nil, nil))

	if bundle.Clusters == nil {
		t.Fatal("expected non-nil Clusters")
	}
	if len(bundle.Clusters.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(bundle.Clusters.Clusters))
	}

	c := bundle.Clusters.Clusters[0]
	if c.OBB == nil {
		t.Fatal("expected non-nil OBB")
	}
	if c.OBB.HeadingRad != 0.785 {
		t.Errorf("expected OBB.HeadingRad=0.785, got %f", c.OBB.HeadingRad)
	}
	if c.OBB.Length != 4.5 {
		t.Errorf("expected OBB.Length=4.5, got %f", c.OBB.Length)
	}
}

// TestFrameAdapter_AdaptTracks_DeletedTrackProcessing verifies that deleted tracks
// with sufficient observation count are included in output with proper fade-out.
func TestFrameAdapter_AdaptTracks_DeletedTrackProcessing(t *testing.T) {
	adapter := NewFrameAdapter("hesai-01")

	// Create tracker with specific configuration
	trackerCfg := l5tracks.DefaultTrackerConfig()
	trackerCfg.DeletedTrackGracePeriod = 10 * time.Second // Long grace period
	trackerCfg.HitsToConfirm = 2
	trackerCfg.MaxMisses = 2          // Misses to delete a tentative track
	trackerCfg.MaxMissesConfirmed = 2 // Misses to delete a confirmed track
	tracker := l5tracks.NewTracker(trackerCfg)

	baseTime := time.Now()

	cluster := l4perception.WorldCluster{
		ClusterID:         1,
		SensorID:          "hesai-01",
		CentroidX:         10.0,
		CentroidY:         20.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       50,
	}

	// Step 1: Create a confirmed track (>= HitsToConfirm observations)
	for i := 0; i < 5; i++ {
		t.Logf("Update %d: Adding cluster", i)
		tracker.Update([]l4perception.WorldCluster{cluster}, baseTime.Add(time.Duration(i)*100*time.Millisecond))
	}

	// Verify track is active and confirmed
	activeTracks := tracker.GetActiveTracks()
	if len(activeTracks) == 0 {
		t.Fatal("expected at least one active track")
	}
	if activeTracks[0].ObservationCount < 3 {
		t.Fatalf("expected ObservationCount >= 3, got %d", activeTracks[0].ObservationCount)
	}
	t.Logf("Track confirmed: ObservationCount=%d", activeTracks[0].ObservationCount)

	// Step 2: Delete the track by providing empty clusters (misses)
	deletionTime := baseTime.Add(500 * time.Millisecond)
	for i := 0; i < 3; i++ {
		t.Logf("Update %d: No cluster (miss)", 5+i)
		tracker.Update([]l4perception.WorldCluster{}, deletionTime.Add(time.Duration(i)*100*time.Millisecond))
	}

	// The track should now be deleted
	activeTracks = tracker.GetActiveTracks()
	t.Logf("Active tracks after misses: %d", len(activeTracks))

	// Step 3: Query deleted tracks immediately (within grace period)
	queryTime := deletionTime.Add(400 * time.Millisecond)
	deletedTracks := tracker.GetRecentlyDeletedTracks(queryTime.UnixNano())
	t.Logf("Deleted tracks: %d", len(deletedTracks))

	if len(deletedTracks) == 0 {
		t.Log("Warning: No deleted tracks found - checking if track was merged or cleaned up")
		// Try with longer wait
		queryTime2 := deletionTime.Add(1 * time.Second)
		deletedTracks = tracker.GetRecentlyDeletedTracks(queryTime2.UnixNano())
		t.Logf("Deleted tracks (retry): %d", len(deletedTracks))
	}

	// Step 4: Run AdaptFrame with the query timestamp
	frame := &l2frames.LiDARFrame{
		SensorID:       "hesai-01",
		StartTimestamp: queryTime,
		Points:         []l2frames.Point{},
	}

	bundle := toFrameBundle(t, adapter.AdaptFrame(frame, nil, nil, tracker, nil))

	if bundle.Tracks == nil {
		t.Fatal("expected non-nil Tracks")
	}

	// Look for deleted track with alpha < 1.0
	foundDeletedTrack := false
	for _, track := range bundle.Tracks.Tracks {
		t.Logf("Track %s: State=%d Alpha=%f ObsCount=%d", track.TrackID, track.State, track.Alpha, track.ObservationCount)
		if track.State == TrackStateDeleted {
			foundDeletedTrack = true
			// Verify alpha is being calculated correctly
			if track.Alpha <= 0 || track.Alpha >= 1.0 {
				t.Errorf("deleted track alpha should be between 0 and 1, got %f", track.Alpha)
			}
		}
	}

	if !foundDeletedTrack {
		t.Error("expected to find a deleted track with fade-out alpha")
	}
}
