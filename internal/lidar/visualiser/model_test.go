// Package visualiser provides gRPC streaming of LiDAR perception data.
package visualiser

import (
	"testing"
	"time"
)

func TestNewFrameBundle(t *testing.T) {
	frameID := uint64(42)
	sensorID := "hesai-01"
	ts := time.Now()

	bundle := NewFrameBundle(frameID, sensorID, ts)

	if bundle == nil {
		t.Fatal("expected non-nil FrameBundle")
	}
	if bundle.FrameID != frameID {
		t.Errorf("expected FrameID=%d, got %d", frameID, bundle.FrameID)
	}
	if bundle.SensorID != sensorID {
		t.Errorf("expected SensorID=%s, got %s", sensorID, bundle.SensorID)
	}
	if bundle.TimestampNanos != ts.UnixNano() {
		t.Errorf("expected TimestampNanos=%d, got %d", ts.UnixNano(), bundle.TimestampNanos)
	}
	if bundle.CoordinateFrame.FrameID != "site/"+sensorID {
		t.Errorf("expected CoordinateFrame.FrameID=site/%s, got %s", sensorID, bundle.CoordinateFrame.FrameID)
	}
	if bundle.CoordinateFrame.ReferenceFrame != "ENU" {
		t.Errorf("expected CoordinateFrame.ReferenceFrame=ENU, got %s", bundle.CoordinateFrame.ReferenceFrame)
	}
}

func TestFrameBundle_DefaultValues(t *testing.T) {
	bundle := NewFrameBundle(1, "test", time.Now())

	// Check that optional fields are nil by default
	if bundle.PointCloud != nil {
		t.Error("expected nil PointCloud by default")
	}
	if bundle.Clusters != nil {
		t.Error("expected nil Clusters by default")
	}
	if bundle.Tracks != nil {
		t.Error("expected nil Tracks by default")
	}
	if bundle.Debug != nil {
		t.Error("expected nil Debug by default")
	}
	if bundle.PlaybackInfo != nil {
		t.Error("expected nil PlaybackInfo by default")
	}
}

func TestDecimationMode_Constants(t *testing.T) {
	// Test that constants have expected values
	if DecimationNone != 0 {
		t.Errorf("expected DecimationNone=0, got %d", DecimationNone)
	}
	if DecimationUniform != 1 {
		t.Errorf("expected DecimationUniform=1, got %d", DecimationUniform)
	}
	if DecimationVoxel != 2 {
		t.Errorf("expected DecimationVoxel=2, got %d", DecimationVoxel)
	}
	if DecimationForegroundOnly != 3 {
		t.Errorf("expected DecimationForegroundOnly=3, got %d", DecimationForegroundOnly)
	}
}

func TestClusteringMethod_Constants(t *testing.T) {
	if ClusteringDBSCAN != 0 {
		t.Errorf("expected ClusteringDBSCAN=0, got %d", ClusteringDBSCAN)
	}
	if ClusteringConnectedComponents != 1 {
		t.Errorf("expected ClusteringConnectedComponents=1, got %d", ClusteringConnectedComponents)
	}
}

func TestTrackState_Constants(t *testing.T) {
	if TrackStateUnknown != 0 {
		t.Errorf("expected TrackStateUnknown=0, got %d", TrackStateUnknown)
	}
	if TrackStateTentative != 1 {
		t.Errorf("expected TrackStateTentative=1, got %d", TrackStateTentative)
	}
	if TrackStateConfirmed != 2 {
		t.Errorf("expected TrackStateConfirmed=2, got %d", TrackStateConfirmed)
	}
	if TrackStateDeleted != 3 {
		t.Errorf("expected TrackStateDeleted=3, got %d", TrackStateDeleted)
	}
}

func TestOcclusionState_Constants(t *testing.T) {
	if OcclusionNone != 0 {
		t.Errorf("expected OcclusionNone=0, got %d", OcclusionNone)
	}
	if OcclusionPartial != 1 {
		t.Errorf("expected OcclusionPartial=1, got %d", OcclusionPartial)
	}
	if OcclusionFull != 2 {
		t.Errorf("expected OcclusionFull=2, got %d", OcclusionFull)
	}
}

func TestMotionModel_Constants(t *testing.T) {
	if MotionModelCV != 0 {
		t.Errorf("expected MotionModelCV=0, got %d", MotionModelCV)
	}
	if MotionModelCA != 1 {
		t.Errorf("expected MotionModelCA=1, got %d", MotionModelCA)
	}
}

func TestClusterSet_Creation(t *testing.T) {
	cs := &ClusterSet{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		Clusters:       make([]Cluster, 0),
		Method:         ClusteringDBSCAN,
	}

	if cs.FrameID != 1 {
		t.Errorf("expected FrameID=1, got %d", cs.FrameID)
	}
	if len(cs.Clusters) != 0 {
		t.Errorf("expected empty Clusters, got %d", len(cs.Clusters))
	}
}

func TestTrackSet_Creation(t *testing.T) {
	ts := &TrackSet{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		Tracks:         make([]Track, 0),
		Trails:         make([]TrackTrail, 0),
	}

	if ts.FrameID != 1 {
		t.Errorf("expected FrameID=1, got %d", ts.FrameID)
	}
	if len(ts.Tracks) != 0 {
		t.Errorf("expected empty Tracks, got %d", len(ts.Tracks))
	}
	if len(ts.Trails) != 0 {
		t.Errorf("expected empty Trails, got %d", len(ts.Trails))
	}
}

func TestPointCloudFrame_Creation(t *testing.T) {
	pcf := &PointCloudFrame{
		FrameID:         1,
		TimestampNanos:  time.Now().UnixNano(),
		SensorID:        "hesai-01",
		X:               []float32{1.0, 2.0, 3.0},
		Y:               []float32{4.0, 5.0, 6.0},
		Z:               []float32{7.0, 8.0, 9.0},
		Intensity:       []uint8{100, 150, 200},
		Classification:  []uint8{0, 1, 0},
		DecimationMode:  DecimationNone,
		DecimationRatio: 1.0,
		PointCount:      3,
	}

	if pcf.FrameID != 1 {
		t.Errorf("expected FrameID=1, got %d", pcf.FrameID)
	}
	if pcf.PointCount != 3 {
		t.Errorf("expected PointCount=3, got %d", pcf.PointCount)
	}
	if len(pcf.X) != 3 {
		t.Errorf("expected X length=3, got %d", len(pcf.X))
	}
}

func TestOrientedBoundingBox_7DOF(t *testing.T) {
	// Test 7-DOF bounding box structure
	obb := &OrientedBoundingBox{
		CenterX:    1.0,
		CenterY:    2.0,
		CenterZ:    3.0,
		Length:     4.0,
		Width:      5.0,
		Height:     6.0,
		HeadingRad: 1.57, // ~90 degrees
	}

	if obb.CenterX != 1.0 {
		t.Errorf("expected CenterX=1.0, got %f", obb.CenterX)
	}
	if obb.CenterY != 2.0 {
		t.Errorf("expected CenterY=2.0, got %f", obb.CenterY)
	}
	if obb.CenterZ != 3.0 {
		t.Errorf("expected CenterZ=3.0, got %f", obb.CenterZ)
	}
	if obb.Length != 4.0 {
		t.Errorf("expected Length=4.0, got %f", obb.Length)
	}
	if obb.Width != 5.0 {
		t.Errorf("expected Width=5.0, got %f", obb.Width)
	}
	if obb.Height != 6.0 {
		t.Errorf("expected Height=6.0, got %f", obb.Height)
	}
}

func TestCluster_WithOBB(t *testing.T) {
	c := Cluster{
		ClusterID:      1,
		SensorID:       "hesai-01",
		TimestampNanos: time.Now().UnixNano(),
		CentroidX:      5.0,
		CentroidY:      10.0,
		CentroidZ:      1.5,
		AABBLength:     2.0,
		AABBWidth:      1.5,
		AABBHeight:     1.0,
		OBB: &OrientedBoundingBox{
			CenterX:    5.0,
			CenterY:    10.0,
			CenterZ:    1.5,
			Length:     2.0,
			Width:      1.5,
			Height:     1.0,
			HeadingRad: 0.5,
		},
		PointsCount:   100,
		HeightP95:     1.8,
		IntensityMean: 75.5,
		SamplePoints:  []float32{1.0, 2.0, 3.0, 4.0, 5.0, 6.0},
	}

	if c.ClusterID != 1 {
		t.Errorf("expected ClusterID=1, got %d", c.ClusterID)
	}
	if c.OBB == nil {
		t.Error("expected non-nil OBB")
	}
	if c.OBB.HeadingRad != 0.5 {
		t.Errorf("expected OBB.HeadingRad=0.5, got %f", c.OBB.HeadingRad)
	}
}

func TestTrack_FullFields(t *testing.T) {
	track := Track{
		TrackID:           "track_42",
		SensorID:          "hesai-01",
		State:             TrackStateConfirmed,
		Hits:              10,
		Misses:            0,
		ObservationCount:  10,
		FirstSeenNanos:    time.Now().Add(-time.Second).UnixNano(),
		LastSeenNanos:     time.Now().UnixNano(),
		X:                 5.0,
		Y:                 10.0,
		Z:                 0.0,
		VX:                2.0,
		VY:                1.0,
		VZ:                0.0,
		SpeedMps:          2.24,
		HeadingRad:        0.46,
		Covariance4x4:     make([]float32, 16),
		BBoxLengthAvg:     4.5,
		BBoxWidthAvg:      2.0,
		BBoxHeightAvg:     1.5,
		BBoxHeadingRad:    0.46,
		HeightP95Max:      1.8,
		IntensityMeanAvg:  100.0,
		AvgSpeedMps:       2.0,
		PeakSpeedMps:      3.0,
		ClassLabel:        "car",
		ClassConfidence:   0.95,
		TrackLengthMetres: 25.0,
		TrackDurationSecs: 10.0,
		OcclusionCount:    0,
		Confidence:        0.9,
		OcclusionState:    OcclusionNone,
		MotionModel:       MotionModelCV,
	}

	if track.TrackID != "track_42" {
		t.Errorf("expected TrackID=track_42, got %s", track.TrackID)
	}
	if track.State != TrackStateConfirmed {
		t.Errorf("expected State=TrackStateConfirmed, got %d", track.State)
	}
	if track.ClassLabel != "car" {
		t.Errorf("expected ClassLabel=car, got %s", track.ClassLabel)
	}
}

func TestTrackTrail_Creation(t *testing.T) {
	trail := TrackTrail{
		TrackID: "track_1",
		Points: []TrackPoint{
			{X: 0.0, Y: 0.0, TimestampNanos: 1000},
			{X: 1.0, Y: 1.0, TimestampNanos: 2000},
			{X: 2.0, Y: 2.0, TimestampNanos: 3000},
		},
	}

	if trail.TrackID != "track_1" {
		t.Errorf("expected TrackID=track_1, got %s", trail.TrackID)
	}
	if len(trail.Points) != 3 {
		t.Errorf("expected 3 points, got %d", len(trail.Points))
	}
}

func TestDebugOverlaySet_Creation(t *testing.T) {
	debug := &DebugOverlaySet{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		AssociationCandidates: []AssociationCandidate{
			{ClusterID: 1, TrackID: "track_1", Distance: 0.5, Accepted: true},
		},
		GatingEllipses: []GatingEllipse{
			{TrackID: "track_1", CenterX: 5.0, CenterY: 10.0, SemiMajor: 2.0, SemiMinor: 1.0, RotationRad: 0.0},
		},
		Residuals: []InnovationResidual{
			{TrackID: "track_1", PredictedX: 5.0, PredictedY: 10.0, MeasuredX: 5.1, MeasuredY: 10.1, ResidualMagnitude: 0.14},
		},
		Predictions: []StatePrediction{
			{TrackID: "track_1", X: 5.0, Y: 10.0, VX: 1.0, VY: 0.5},
		},
	}

	if debug.FrameID != 1 {
		t.Errorf("expected FrameID=1, got %d", debug.FrameID)
	}
	if len(debug.AssociationCandidates) != 1 {
		t.Errorf("expected 1 AssociationCandidate, got %d", len(debug.AssociationCandidates))
	}
	if len(debug.GatingEllipses) != 1 {
		t.Errorf("expected 1 GatingEllipse, got %d", len(debug.GatingEllipses))
	}
	if len(debug.Residuals) != 1 {
		t.Errorf("expected 1 Residual, got %d", len(debug.Residuals))
	}
	if len(debug.Predictions) != 1 {
		t.Errorf("expected 1 Prediction, got %d", len(debug.Predictions))
	}
}

func TestPlaybackInfo_Creation(t *testing.T) {
	pi := &PlaybackInfo{
		IsLive:       false,
		LogStartNs:   1000000000,
		LogEndNs:     2000000000,
		PlaybackRate: 1.0,
		Paused:       false,
	}

	if pi.IsLive {
		t.Error("expected IsLive=false")
	}
	if pi.LogStartNs != 1000000000 {
		t.Errorf("expected LogStartNs=1000000000, got %d", pi.LogStartNs)
	}
	if pi.PlaybackRate != 1.0 {
		t.Errorf("expected PlaybackRate=1.0, got %f", pi.PlaybackRate)
	}
}

func TestCoordinateFrameInfo_Creation(t *testing.T) {
	cfi := CoordinateFrameInfo{
		FrameID:        "site/hesai-01",
		ReferenceFrame: "ENU",
		OriginLat:      37.7749,
		OriginLon:      -122.4194,
		OriginAlt:      10.0,
		RotationDeg:    45.0,
	}

	if cfi.FrameID != "site/hesai-01" {
		t.Errorf("expected FrameID=site/hesai-01, got %s", cfi.FrameID)
	}
	if cfi.ReferenceFrame != "ENU" {
		t.Errorf("expected ReferenceFrame=ENU, got %s", cfi.ReferenceFrame)
	}
	if cfi.OriginLat != 37.7749 {
		t.Errorf("expected OriginLat=37.7749, got %f", cfi.OriginLat)
	}
}

// M7: Reference counting tests

func TestPointCloudFrame_Retain_IncreasesRefCount(t *testing.T) {
	pc := &PointCloudFrame{
		X:          []float32{1.0, 2.0, 3.0},
		Y:          []float32{1.0, 2.0, 3.0},
		Z:          []float32{1.0, 2.0, 3.0},
		PointCount: 3,
	}

	if pc.RefCount() != 0 {
		t.Errorf("expected initial RefCount=0, got %d", pc.RefCount())
	}

	pc.Retain()
	if pc.RefCount() != 1 {
		t.Errorf("expected RefCount=1 after Retain, got %d", pc.RefCount())
	}

	pc.Retain()
	if pc.RefCount() != 2 {
		t.Errorf("expected RefCount=2 after second Retain, got %d", pc.RefCount())
	}
}

func TestPointCloudFrame_Retain_NilSafe(t *testing.T) {
	var pc *PointCloudFrame
	// Should not panic
	pc.Retain()

	if pc.RefCount() != 0 {
		t.Errorf("expected RefCount=0 for nil, got %d", pc.RefCount())
	}
}

func TestPointCloudFrame_RefCount_NilSafe(t *testing.T) {
	var pc *PointCloudFrame
	if pc.RefCount() != 0 {
		t.Errorf("expected RefCount=0 for nil, got %d", pc.RefCount())
	}
}

func TestPointCloudFrame_Release_DecreasesRefCount(t *testing.T) {
	pc := &PointCloudFrame{
		X:          []float32{1.0, 2.0, 3.0},
		Y:          []float32{1.0, 2.0, 3.0},
		Z:          []float32{1.0, 2.0, 3.0},
		PointCount: 3,
	}

	// Retain twice
	pc.Retain()
	pc.Retain()
	if pc.RefCount() != 2 {
		t.Errorf("expected RefCount=2, got %d", pc.RefCount())
	}

	// First release decreases count but doesn't nil slices
	pc.Release()
	if pc.RefCount() != 1 {
		t.Errorf("expected RefCount=1 after Release, got %d", pc.RefCount())
	}
	if pc.X == nil {
		t.Error("expected X to still be present after first Release")
	}

	// Second release decreases to 0 and nils slices
	pc.Release()
	if pc.RefCount() != 0 {
		t.Errorf("expected RefCount=0 after second Release, got %d", pc.RefCount())
	}
	if pc.X != nil {
		t.Error("expected X to be nil after final Release")
	}
}

func TestPointCloudFrame_Release_NilSafe(t *testing.T) {
	var pc *PointCloudFrame
	// Should not panic
	pc.Release()
}

func TestPointCloudFrame_Release_SingleUse(t *testing.T) {
	// When a frame is never Retain'd, Release should still work
	// (this is the single-client or adapter-only scenario)
	pc := &PointCloudFrame{
		X:          []float32{1.0, 2.0, 3.0},
		Y:          []float32{1.0, 2.0, 3.0},
		Z:          []float32{1.0, 2.0, 3.0},
		PointCount: 3,
	}

	// Release without Retain — refCount goes to -1, which is <= 0, so release
	pc.Release()
	if pc.X != nil {
		t.Error("expected X to be nil after Release on single-use frame")
	}
}

func TestPointCloudFrame_BroadcastScenario(t *testing.T) {
	// Simulate broadcast to 3 clients
	pc := &PointCloudFrame{
		X:          []float32{1.0, 2.0, 3.0},
		Y:          []float32{1.0, 2.0, 3.0},
		Z:          []float32{1.0, 2.0, 3.0},
		PointCount: 3,
	}

	// Publisher retains for each client
	numClients := 3
	for i := 0; i < numClients; i++ {
		pc.Retain()
	}
	if pc.RefCount() != 3 {
		t.Errorf("expected RefCount=3, got %d", pc.RefCount())
	}

	// First two clients release
	pc.Release()
	pc.Release()
	if pc.RefCount() != 1 {
		t.Errorf("expected RefCount=1, got %d", pc.RefCount())
	}
	// Data should still be available
	if pc.X == nil || len(pc.X) != 3 {
		t.Error("expected X to still be present while clients hold reference")
	}

	// Last client releases
	pc.Release()
	if pc.RefCount() != 0 {
		t.Errorf("expected RefCount=0, got %d", pc.RefCount())
	}
	// Data should be released to pool
	if pc.X != nil {
		t.Error("expected X to be nil after all clients release")
	}
}
