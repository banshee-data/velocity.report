package lidar

import (
	"math"
	"testing"
	"time"
)

func TestNewTracker(t *testing.T) {
	config := DefaultTrackerConfig()
	tracker := NewTracker(config)

	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}
	if tracker.Tracks == nil {
		t.Error("expected non-nil tracks map")
	}
	if tracker.NextTrackID != 1 {
		t.Errorf("expected NextTrackID=1, got %d", tracker.NextTrackID)
	}
}

func TestDefaultTrackerConfig(t *testing.T) {
	config := DefaultTrackerConfig()

	// Structural: all fields are within valid operating ranges.
	if config.MaxTracks < 1 {
		t.Errorf("MaxTracks must be >= 1, got %d", config.MaxTracks)
	}
	if config.MaxMisses < 1 {
		t.Errorf("MaxMisses must be >= 1, got %d", config.MaxMisses)
	}
	if config.HitsToConfirm < 1 {
		t.Errorf("HitsToConfirm must be >= 1, got %d", config.HitsToConfirm)
	}
	if config.GatingDistanceSquared <= 0 {
		t.Errorf("GatingDistanceSquared must be positive, got %v", config.GatingDistanceSquared)
	}
	if config.ProcessNoisePos <= 0 {
		t.Errorf("ProcessNoisePos must be positive, got %v", config.ProcessNoisePos)
	}
	if config.ProcessNoiseVel <= 0 {
		t.Errorf("ProcessNoiseVel must be positive, got %v", config.ProcessNoiseVel)
	}
	if config.MeasurementNoise <= 0 {
		t.Errorf("MeasurementNoise must be positive, got %v", config.MeasurementNoise)
	}
}

func TestTracker_InitTrack(t *testing.T) {
	tracker := NewTracker(DefaultTrackerConfig())
	now := time.Now()

	cluster := WorldCluster{
		CentroidX:         5.0,
		CentroidY:         10.0,
		CentroidZ:         1.0,
		BoundingBoxLength: 2.0,
		BoundingBoxWidth:  1.5,
		BoundingBoxHeight: 1.2,
		SensorID:          "test-sensor",
	}

	// Add cluster to trigger track creation
	tracker.Update([]WorldCluster{cluster}, now)

	total, tentative, confirmed, deleted := tracker.GetTrackCount()

	if total != 1 {
		t.Errorf("expected 1 total track, got %d", total)
	}
	if tentative != 1 {
		t.Errorf("expected 1 tentative track, got %d", tentative)
	}
	if confirmed != 0 {
		t.Errorf("expected 0 confirmed tracks, got %d", confirmed)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted tracks, got %d", deleted)
	}

	// Verify track properties
	tracks := tracker.GetActiveTracks()
	if len(tracks) != 1 {
		t.Fatalf("expected 1 active track, got %d", len(tracks))
	}

	track := tracks[0]
	if track.State != TrackTentative {
		t.Errorf("expected TrackTentative state, got %v", track.State)
	}
	if track.X != 5.0 {
		t.Errorf("expected X=5.0, got %v", track.X)
	}
	if track.Y != 10.0 {
		t.Errorf("expected Y=10.0, got %v", track.Y)
	}
	if track.SensorID != "test-sensor" {
		t.Errorf("expected SensorID=test-sensor, got %s", track.SensorID)
	}
}

func TestTracker_Lifecycle_TentativeToConfirmed(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 3
	tracker := NewTracker(config)

	now := time.Now()
	cluster := WorldCluster{CentroidX: 5.0, CentroidY: 10.0, SensorID: "test"}

	// Frame 1: Create tentative track
	tracker.Update([]WorldCluster{cluster}, now)
	tracks := tracker.GetActiveTracks()
	if len(tracks) != 1 || tracks[0].State != TrackTentative {
		t.Errorf("frame 1: expected 1 tentative track")
	}

	// Frame 2: Hit (still tentative)
	now = now.Add(100 * time.Millisecond)
	cluster.CentroidX = 5.1
	tracker.Update([]WorldCluster{cluster}, now)
	tracks = tracker.GetActiveTracks()
	if tracks[0].Hits != 2 {
		t.Errorf("frame 2: expected 2 hits, got %d", tracks[0].Hits)
	}
	if tracks[0].State != TrackTentative {
		t.Errorf("frame 2: expected tentative state")
	}

	// Frame 3: Hit (confirmed after 3 hits)
	now = now.Add(100 * time.Millisecond)
	cluster.CentroidX = 5.2
	tracker.Update([]WorldCluster{cluster}, now)
	tracks = tracker.GetActiveTracks()
	if tracks[0].Hits != 3 {
		t.Errorf("frame 3: expected 3 hits, got %d", tracks[0].Hits)
	}
	if tracks[0].State != TrackConfirmed {
		t.Errorf("frame 3: expected confirmed state, got %v", tracks[0].State)
	}
}

func TestTracker_Lifecycle_ConfirmedToDeleted(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 2      // Need 2 hits to confirm
	config.MaxMisses = 2          // Quick deletion for tentative tracks
	config.MaxMissesConfirmed = 2 // Quick deletion for confirmed tracks too
	tracker := NewTracker(config)

	now := time.Now()
	cluster := WorldCluster{CentroidX: 5.0, CentroidY: 10.0, SensorID: "test"}

	// Frame 1: Create tentative track
	tracker.Update([]WorldCluster{cluster}, now)
	tracks := tracker.GetActiveTracks()
	if tracks[0].State != TrackTentative {
		t.Fatalf("frame 1: expected tentative track")
	}

	// Frame 2: Confirm track (2nd hit)
	now = now.Add(100 * time.Millisecond)
	tracker.Update([]WorldCluster{cluster}, now)
	tracks = tracker.GetActiveTracks()
	if tracks[0].State != TrackConfirmed {
		t.Fatalf("frame 2: expected confirmed track, got %v", tracks[0].State)
	}

	// Frame 3: Miss (cluster not present)
	now = now.Add(100 * time.Millisecond)
	tracker.Update([]WorldCluster{}, now)
	tracks = tracker.GetActiveTracks()
	if tracks[0].Misses != 1 {
		t.Errorf("frame 3: expected 1 miss, got %d", tracks[0].Misses)
	}
	if tracks[0].State != TrackConfirmed {
		t.Errorf("frame 3: expected confirmed state")
	}

	// Frame 4: Miss (deleted after MaxMisses)
	now = now.Add(100 * time.Millisecond)
	tracker.Update([]WorldCluster{}, now)

	// Track should now be deleted
	total, _, confirmed, deleted := tracker.GetTrackCount()
	if confirmed != 0 {
		t.Errorf("frame 4: expected 0 confirmed, got %d", confirmed)
	}
	if deleted != 1 {
		t.Errorf("frame 4: expected 1 deleted, got %d", deleted)
	}
	if total != 1 {
		t.Errorf("frame 4: expected 1 total (still in map until cleanup), got %d", total)
	}
}

func TestTracker_Association(t *testing.T) {
	tracker := NewTracker(DefaultTrackerConfig())
	now := time.Now()

	// Create two separate tracks
	clusters := []WorldCluster{
		{CentroidX: 0.0, CentroidY: 0.0, SensorID: "test"},
		{CentroidX: 20.0, CentroidY: 0.0, SensorID: "test"},
	}
	tracker.Update(clusters, now)

	if len(tracker.GetActiveTracks()) != 2 {
		t.Fatalf("expected 2 tracks created")
	}

	// Update with slightly moved clusters - should associate to existing tracks
	now = now.Add(100 * time.Millisecond)
	clusters[0].CentroidX = 0.5  // Moved slightly
	clusters[1].CentroidX = 20.5 // Moved slightly
	tracker.Update(clusters, now)

	// Should still have 2 tracks (not 4)
	total, _, _, _ := tracker.GetTrackCount()
	if total != 2 {
		t.Errorf("expected 2 tracks after association, got %d", total)
	}

	// Verify hits increased
	tracks := tracker.GetActiveTracks()
	for _, track := range tracks {
		if track.Hits != 2 {
			t.Errorf("expected 2 hits on track, got %d", track.Hits)
		}
	}
}

func TestTracker_Predict(t *testing.T) {
	tracker := NewTracker(DefaultTrackerConfig())
	now := time.Now()

	// Create track at origin
	cluster := WorldCluster{CentroidX: 0.0, CentroidY: 0.0, SensorID: "test"}
	tracker.Update([]WorldCluster{cluster}, now)

	// Get track ID
	tracks := tracker.GetActiveTracks()
	trackID := tracks[0].TrackID

	// Set velocity directly on the internal track object (via GetTrack which
	// returns the live pointer). GetActiveTracks returns deep copies so we
	// cannot mutate through it.
	tracker.mu.Lock()
	internalTrack := tracker.Tracks[trackID]
	internalTrack.VX = 1.0 // 1 m/s in X direction
	internalTrack.VY = 0.0
	tracker.mu.Unlock()

	// Update with no clusters (miss) after 0.4 second (within MaxPredictDt)
	now = now.Add(400 * time.Millisecond)
	tracker.Update([]WorldCluster{}, now)

	// Track should have predicted to X≈0.4 (VX=1.0 * dt=0.4)
	updatedTrack := tracker.GetTrack(trackID)
	if math.Abs(float64(updatedTrack.X)-0.4) > 0.1 {
		t.Errorf("expected X≈0.4 after prediction, got %v", updatedTrack.X)
	}
}

func TestTracker_Predict_DtClamped(t *testing.T) {
	// Verify that large dt values are clamped to MaxPredictDt
	tracker := NewTracker(DefaultTrackerConfig())
	now := time.Now()

	cluster := WorldCluster{CentroidX: 0.0, CentroidY: 0.0, SensorID: "test"}
	tracker.Update([]WorldCluster{cluster}, now)

	tracks := tracker.GetActiveTracks()
	trackID := tracks[0].TrackID

	tracker.mu.Lock()
	internalTrack := tracker.Tracks[trackID]
	internalTrack.VX = 1.0
	internalTrack.VY = 0.0
	tracker.mu.Unlock()

	// Update with a 5-second gap — dt should be clamped to MaxPredictDt (0.5)
	now = now.Add(5 * time.Second)
	tracker.Update([]WorldCluster{}, now)

	updatedTrack := tracker.GetTrack(trackID)
	// With clamping, X should be ~0.5 (VX=1.0 * MaxPredictDt=0.5), not 5.0
	if float64(updatedTrack.X) > 1.0 {
		t.Errorf("dt should be clamped: expected X≤1.0, got %v", updatedTrack.X)
	}
}

func TestTracker_MaxTracks(t *testing.T) {
	config := DefaultTrackerConfig()
	config.MaxTracks = 3
	tracker := NewTracker(config)

	now := time.Now()

	// Try to create 5 tracks (only 3 should be created)
	clusters := []WorldCluster{
		{CentroidX: 0.0, CentroidY: 0.0, SensorID: "test"},
		{CentroidX: 10.0, CentroidY: 0.0, SensorID: "test"},
		{CentroidX: 20.0, CentroidY: 0.0, SensorID: "test"},
		{CentroidX: 30.0, CentroidY: 0.0, SensorID: "test"},
		{CentroidX: 40.0, CentroidY: 0.0, SensorID: "test"},
	}
	tracker.Update(clusters, now)

	total, _, _, _ := tracker.GetTrackCount()
	if total != 3 {
		t.Errorf("expected 3 tracks (MaxTracks limit), got %d", total)
	}
}

func TestTracker_VelocityEstimation(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 1
	tracker := NewTracker(config)

	now := time.Now()
	dt := 100 * time.Millisecond

	// Frame 1: Create track at (0, 0)
	cluster := WorldCluster{CentroidX: 0.0, CentroidY: 0.0, SensorID: "test"}
	tracker.Update([]WorldCluster{cluster}, now)

	// Frame 2: Move to (1, 0) - velocity should be ~10 m/s in X
	now = now.Add(dt)
	cluster.CentroidX = 1.0
	tracker.Update([]WorldCluster{cluster}, now)

	tracks := tracker.GetActiveTracks()
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track")
	}

	// Velocity estimate should be positive in X direction
	if tracks[0].VX <= 0 {
		t.Errorf("expected positive VX after moving in +X direction, got %v", tracks[0].VX)
	}
}

func TestTracker_GetConfirmedTracks(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 2
	tracker := NewTracker(config)

	now := time.Now()
	cluster1 := WorldCluster{CentroidX: 0.0, CentroidY: 0.0, SensorID: "test"}
	cluster2 := WorldCluster{CentroidX: 10.0, CentroidY: 0.0, SensorID: "test"}

	// Frame 1: Create 2 tentative tracks
	tracker.Update([]WorldCluster{cluster1, cluster2}, now)

	confirmed := tracker.GetConfirmedTracks()
	if len(confirmed) != 0 {
		t.Errorf("expected 0 confirmed tracks initially, got %d", len(confirmed))
	}

	// Frame 2: Confirm both
	now = now.Add(100 * time.Millisecond)
	tracker.Update([]WorldCluster{cluster1, cluster2}, now)

	confirmed = tracker.GetConfirmedTracks()
	if len(confirmed) != 2 {
		t.Errorf("expected 2 confirmed tracks, got %d", len(confirmed))
	}
}

func TestTrackedObject_SpeedAndHeading(t *testing.T) {
	track := &TrackedObject{
		VX: 3.0,
		VY: 4.0,
	}

	speed := track.Speed()
	if math.Abs(float64(speed)-5.0) > 0.01 {
		t.Errorf("expected speed=5.0 (3-4-5 triangle), got %v", speed)
	}

	heading := track.Heading()
	expected := float32(math.Atan2(4.0, 3.0))
	if math.Abs(float64(heading-expected)) > 0.01 {
		t.Errorf("expected heading≈%v, got %v", expected, heading)
	}
}

func TestTracker_GatingRejectsDistantClusters(t *testing.T) {
	config := DefaultTrackerConfig()
	config.GatingDistanceSquared = 4.0 // 2 meter gating
	config.HitsToConfirm = 1
	tracker := NewTracker(config)

	now := time.Now()

	// Create track at origin
	cluster := WorldCluster{CentroidX: 0.0, CentroidY: 0.0, SensorID: "test"}
	tracker.Update([]WorldCluster{cluster}, now)

	// Try to associate a cluster at (10, 0) - should be rejected
	now = now.Add(100 * time.Millisecond)
	farCluster := WorldCluster{CentroidX: 10.0, CentroidY: 0.0, SensorID: "test"}
	tracker.Update([]WorldCluster{farCluster}, now)

	// Should have 2 tracks now (original track with 1 miss, new track from far cluster)
	total, _, _, _ := tracker.GetTrackCount()
	if total != 2 {
		t.Errorf("expected 2 tracks (original + new), got %d", total)
	}
}

func TestTracker_CleanupDeletedTracks(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 1
	config.MaxMisses = 1
	tracker := NewTracker(config)

	now := time.Now()

	// Create and confirm track
	cluster := WorldCluster{CentroidX: 0.0, CentroidY: 0.0, SensorID: "test"}
	tracker.Update([]WorldCluster{cluster}, now)

	// Miss to delete
	now = now.Add(100 * time.Millisecond)
	tracker.Update([]WorldCluster{}, now)

	// Track should be deleted but still in map
	total, _, _, deleted := tracker.GetTrackCount()
	if deleted != 1 {
		t.Errorf("expected 1 deleted track, got %d", deleted)
	}
	if total != 1 {
		t.Errorf("expected 1 total track, got %d", total)
	}

	// After 6 seconds, track should be cleaned up
	now = now.Add(6 * time.Second)
	tracker.Update([]WorldCluster{}, now)

	total, _, _, _ = tracker.GetTrackCount()
	if total != 0 {
		t.Errorf("expected 0 tracks after cleanup, got %d", total)
	}
}

func TestTracker_ObservationAggregation(t *testing.T) {
	config := DefaultTrackerConfig()
	config.HitsToConfirm = 1
	tracker := NewTracker(config)

	now := time.Now()

	// Create track with initial features
	cluster := WorldCluster{
		CentroidX:         5.0,
		CentroidY:         10.0,
		BoundingBoxLength: 4.0,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		HeightP95:         1.4,
		IntensityMean:     100,
		SensorID:          "test",
	}
	tracker.Update([]WorldCluster{cluster}, now)

	// Update with different features
	now = now.Add(100 * time.Millisecond)
	cluster.BoundingBoxLength = 4.2
	cluster.BoundingBoxWidth = 2.2
	cluster.HeightP95 = 1.6 // Higher than before
	cluster.IntensityMean = 110
	tracker.Update([]WorldCluster{cluster}, now)

	tracks := tracker.GetActiveTracks()
	track := tracks[0]

	// Running averages should reflect both observations
	if track.ObservationCount != 2 {
		t.Errorf("expected 2 observations, got %d", track.ObservationCount)
	}

	// Average of 4.0 and 4.2 should be 4.1
	expectedLength := (4.0 + 4.2) / 2
	if math.Abs(float64(track.BoundingBoxLengthAvg)-expectedLength) > 0.01 {
		t.Errorf("expected BoundingBoxLengthAvg≈%v, got %v", expectedLength, track.BoundingBoxLengthAvg)
	}

	// Max height P95 should be 1.6
	if track.HeightP95Max != 1.6 {
		t.Errorf("expected HeightP95Max=1.6, got %v", track.HeightP95Max)
	}
}

// TestMahalanobisDistanceSquared tests the Mahalanobis distance computation.
func TestMahalanobisDistanceSquared(t *testing.T) {
	t.Parallel()

	t.Run("returns rejection for large position jump", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(DefaultTrackerConfig())

		track := &TrackedObject{
			X: 0,
			Y: 0,
			P: [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		}

		// Cluster very far away, exceeding MaxPositionJumpMeters
		cluster := WorldCluster{
			CentroidX: MaxPositionJumpMeters + 10, // way beyond limit
			CentroidY: 0,
		}

		dist := tracker.mahalanobisDistanceSquared(track, cluster, 0.1)
		if dist != SingularDistanceRejection {
			t.Errorf("expected SingularDistanceRejection, got %v", dist)
		}
	})

	t.Run("returns rejection for excessive implied speed", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(DefaultTrackerConfig())

		track := &TrackedObject{
			X: 0,
			Y: 0,
			P: [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		}

		// Cluster 20m away with dt=0.01s → implied speed = 2000 m/s (unreasonable)
		cluster := WorldCluster{
			CentroidX: 20,
			CentroidY: 0,
		}

		dist := tracker.mahalanobisDistanceSquared(track, cluster, 0.01)
		if dist != SingularDistanceRejection {
			t.Errorf("expected SingularDistanceRejection for excessive speed, got %v", dist)
		}
	})

	t.Run("returns rejection for singular covariance", func(t *testing.T) {
		t.Parallel()
		config := DefaultTrackerConfig()
		config.MeasurementNoise = 0 // Zero out measurement noise to allow singular
		tracker := NewTracker(config)

		track := &TrackedObject{
			X: 0,
			Y: 0,
			// Covariance with zero determinant after adding measurement noise
			// P[0,0] + MeasurementNoise = 0, P[1,1] + MeasurementNoise = 0
			// With MeasurementNoise=0, the det = 0 * 0 - 0 * 0 = 0
			P: [16]float32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		}

		cluster := WorldCluster{
			CentroidX: 1,
			CentroidY: 1,
		}

		dist := tracker.mahalanobisDistanceSquared(track, cluster, 0.1)
		if dist != SingularDistanceRejection {
			t.Errorf("expected SingularDistanceRejection for singular matrix, got %v", dist)
		}
	})

	t.Run("computes distance for valid inputs", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(DefaultTrackerConfig())

		track := &TrackedObject{
			X: 0,
			Y: 0,
			// Identity covariance
			P: [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		}

		// Cluster close by
		cluster := WorldCluster{
			CentroidX: 1,
			CentroidY: 1,
		}

		dist := tracker.mahalanobisDistanceSquared(track, cluster, 0.1)

		// Should return a valid distance, not rejection
		if dist == SingularDistanceRejection {
			t.Error("expected valid distance, got SingularDistanceRejection")
		}

		// Should be a positive value
		if dist <= 0 {
			t.Errorf("expected positive distance, got %v", dist)
		}
	})

	t.Run("handles zero dt without implied speed check", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(DefaultTrackerConfig())

		track := &TrackedObject{
			X: 0,
			Y: 0,
			P: [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		}

		// Cluster close by with dt=0
		cluster := WorldCluster{
			CentroidX: 2,
			CentroidY: 0,
		}

		// dt=0 should skip speed check and still compute distance
		dist := tracker.mahalanobisDistanceSquared(track, cluster, 0)

		if dist == SingularDistanceRejection {
			t.Error("expected valid distance with dt=0, got SingularDistanceRejection")
		}
	})
}
