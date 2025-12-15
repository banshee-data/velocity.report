package lidar

import (
	"testing"
	"time"
)

// =============================================================================
// Phase 3 & 4: Long-Tail Track Management and Sparse Continuation Tests
// =============================================================================

func TestDefaultPreTailConfig(t *testing.T) {
	config := DefaultPreTailConfig()

	if config.MaxPreTailFrames <= 0 {
		t.Error("MaxPreTailFrames should be positive")
	}
	if config.MinVelocityConfidence <= 0 {
		t.Error("MinVelocityConfidence should be positive")
	}
}

func TestDefaultPostTailConfig(t *testing.T) {
	config := DefaultPostTailConfig()

	if config.MaxPredictionFrames <= 0 {
		t.Error("MaxPredictionFrames should be positive")
	}
	if config.MaxUncertaintyRadius <= 0 {
		t.Error("MaxUncertaintyRadius should be positive")
	}
}

func TestNewLongTailManager(t *testing.T) {
	preTail := DefaultPreTailConfig()
	postTail := DefaultPostTailConfig()

	manager := NewLongTailManager(preTail, postTail)

	if manager == nil {
		t.Fatal("NewLongTailManager returned nil")
	}
	if manager.predictions == nil {
		t.Error("predictions map not initialized")
	}
}

func TestLongTailManager_UpdatePredictions(t *testing.T) {
	manager := NewLongTailManager(DefaultPreTailConfig(), DefaultPostTailConfig())

	now := time.Now()
	tracks := map[string]*VelocityCoherentTrack{
		"track1": {
			TrackID:       "track1",
			State:         TrackPostTail,
			X:             10.0,
			Y:             20.0,
			VX:            5.0,
			VY:            2.0,
			LastUnixNanos: now.Add(-200 * time.Millisecond).UnixNano(),
		},
		"track2": {
			TrackID:       "track2",
			State:         TrackConfirmVC, // Not in post-tail
			X:             30.0,
			Y:             40.0,
			LastUnixNanos: now.UnixNano(),
		},
	}

	predictions := manager.UpdatePredictions(tracks, now)

	// Should have prediction only for track1 (in post-tail)
	if len(predictions) != 1 {
		t.Errorf("Expected 1 prediction, got %d", len(predictions))
	}

	if len(predictions) > 0 && predictions[0].TrackID != "track1" {
		t.Errorf("Expected prediction for track1, got %s", predictions[0].TrackID)
	}
}

func TestLongTailManager_PredictionPosition(t *testing.T) {
	manager := NewLongTailManager(DefaultPreTailConfig(), DefaultPostTailConfig())

	now := time.Now()
	dt := 0.5 // 500ms

	tracks := map[string]*VelocityCoherentTrack{
		"track1": {
			TrackID:       "track1",
			State:         TrackPostTail,
			X:             10.0,
			Y:             20.0,
			VX:            5.0, // 5 m/s in X
			VY:            2.0, // 2 m/s in Y
			LastUnixNanos: now.Add(-time.Duration(dt*1e9) * time.Nanosecond).UnixNano(),
		},
	}

	predictions := manager.UpdatePredictions(tracks, now)

	if len(predictions) != 1 {
		t.Fatalf("Expected 1 prediction, got %d", len(predictions))
	}

	pred := predictions[0]

	// Expected position after 500ms: X = 10 + 5*0.5 = 12.5, Y = 20 + 2*0.5 = 21
	expectedX := float32(10.0 + 5.0*dt)
	expectedY := float32(20.0 + 2.0*dt)

	tolerance := float32(0.1)
	if pred.PredictedX < expectedX-tolerance || pred.PredictedX > expectedX+tolerance {
		t.Errorf("PredictedX = %v, want ~%v", pred.PredictedX, expectedX)
	}
	if pred.PredictedY < expectedY-tolerance || pred.PredictedY > expectedY+tolerance {
		t.Errorf("PredictedY = %v, want ~%v", pred.PredictedY, expectedY)
	}
}

func TestLongTailManager_TryRecoverTrack(t *testing.T) {
	manager := NewLongTailManager(DefaultPreTailConfig(), DefaultPostTailConfig())

	// Add a prediction
	manager.predictions["track1"] = &PredictedPosition{
		TrackID:           "track1",
		PredictedX:        10.0,
		PredictedY:        20.0,
		VelocityX:         5.0,
		VelocityY:         2.0,
		UncertaintyRadius: 2.0,
		FramesSinceLast:   5,
	}

	// Cluster near predicted position with matching velocity
	cluster := VelocityCoherentCluster{
		ClusterID:  1,
		CentroidX:  10.5,
		CentroidY:  20.3,
		VelocityX:  5.2,
		VelocityY:  1.9,
		PointCount: 5,
	}

	assoc := manager.TryRecoverTrack(cluster)

	if assoc == nil {
		t.Fatal("Expected track association, got nil")
	}
	if assoc.TrackID != "track1" {
		t.Errorf("TrackID = %s, want track1", assoc.TrackID)
	}
	if assoc.Type != AssociationRecovery {
		t.Errorf("Type = %v, want AssociationRecovery", assoc.Type)
	}
}

func TestLongTailManager_NoRecoveryOutsideRadius(t *testing.T) {
	manager := NewLongTailManager(DefaultPreTailConfig(), DefaultPostTailConfig())

	manager.predictions["track1"] = &PredictedPosition{
		TrackID:           "track1",
		PredictedX:        10.0,
		PredictedY:        20.0,
		VelocityX:         5.0,
		VelocityY:         2.0,
		UncertaintyRadius: 2.0,
	}

	// Cluster too far from predicted position
	cluster := VelocityCoherentCluster{
		ClusterID:  1,
		CentroidX:  20.0, // 10 meters away
		CentroidY:  20.0,
		VelocityX:  5.0,
		VelocityY:  2.0,
		PointCount: 5,
	}

	assoc := manager.TryRecoverTrack(cluster)

	if assoc != nil {
		t.Error("Expected nil association for cluster outside radius")
	}
}

// =============================================================================
// Phase 4: Sparse Continuation Tests
// =============================================================================

func TestDefaultSparseTrackConfig(t *testing.T) {
	config := DefaultSparseTrackConfig()

	if config.MinPointsAbsolute != 3 {
		t.Errorf("MinPointsAbsolute = %d, want 3", config.MinPointsAbsolute)
	}
	if config.MinVelocityConfidenceForSparse <= 0 {
		t.Error("MinVelocityConfidenceForSparse should be positive")
	}
}

func TestIsSparseTrackValid_Valid(t *testing.T) {
	config := DefaultSparseTrackConfig()

	cluster := VelocityCoherentCluster{
		PointCount:         4,
		VelocityConfidence: 0.8,
		VelocityX:          5.0,
		VelocityY:          2.0,
		BoundingBoxLength:  1.5,
		BoundingBoxWidth:   1.0,
	}

	track := &VelocityCoherentTrack{
		VX: 5.0,
		VY: 2.0,
	}

	valid, confidence := IsSparseTrackValid(cluster, track, config)

	if !valid {
		t.Error("Expected valid sparse track")
	}
	if confidence <= 0 {
		t.Error("Expected positive confidence")
	}
}

func TestIsSparseTrackValid_TooFewPoints(t *testing.T) {
	config := DefaultSparseTrackConfig()

	cluster := VelocityCoherentCluster{
		PointCount:         2, // Less than MinPointsAbsolute
		VelocityConfidence: 0.8,
		VelocityX:          5.0,
		VelocityY:          2.0,
	}

	track := &VelocityCoherentTrack{VX: 5.0, VY: 2.0}

	valid, _ := IsSparseTrackValid(cluster, track, config)

	if valid {
		t.Error("Expected invalid for too few points")
	}
}

func TestIsSparseTrackValid_LowConfidence(t *testing.T) {
	config := DefaultSparseTrackConfig()

	cluster := VelocityCoherentCluster{
		PointCount:         5,
		VelocityConfidence: 0.3, // Below threshold
		VelocityX:          5.0,
		VelocityY:          2.0,
	}

	track := &VelocityCoherentTrack{VX: 5.0, VY: 2.0}

	valid, _ := IsSparseTrackValid(cluster, track, config)

	if valid {
		t.Error("Expected invalid for low confidence")
	}
}

func TestIsSparseTrackValid_VelocityMismatch(t *testing.T) {
	config := DefaultSparseTrackConfig()

	cluster := VelocityCoherentCluster{
		PointCount:         5,
		VelocityConfidence: 0.8,
		VelocityX:          10.0, // Very different from track
		VelocityY:          10.0,
	}

	track := &VelocityCoherentTrack{VX: 5.0, VY: 2.0}

	valid, _ := IsSparseTrackValid(cluster, track, config)

	if valid {
		t.Error("Expected invalid for velocity mismatch")
	}
}

func TestAdaptiveTolerances(t *testing.T) {
	tests := []struct {
		pointCount  int
		wantVel     float64
		wantSpatial float64
	}{
		{15, 2.0, 1.0}, // >= 12 points
		{12, 2.0, 1.0}, // exactly 12
		{8, 1.5, 0.8},  // 6-11 range
		{6, 1.5, 0.8},  // exactly 6
		{4, 0.5, 0.5},  // 3-5 range
		{3, 0.5, 0.5},  // exactly 3
		{2, 0, 0},      // < 3
		{0, 0, 0},      // 0
	}

	for _, tt := range tests {
		velTol, spatialTol := AdaptiveTolerances(tt.pointCount)

		if velTol != tt.wantVel {
			t.Errorf("AdaptiveTolerances(%d) velTol = %v, want %v",
				tt.pointCount, velTol, tt.wantVel)
		}
		if spatialTol != tt.wantSpatial {
			t.Errorf("AdaptiveTolerances(%d) spatialTol = %v, want %v",
				tt.pointCount, spatialTol, tt.wantSpatial)
		}
	}
}
