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

	if config.MaxTracks != 100 {
		t.Errorf("expected MaxTracks=100, got %d", config.MaxTracks)
	}
	if config.MaxMisses != 3 {
		t.Errorf("expected MaxMisses=3, got %d", config.MaxMisses)
	}
	if config.HitsToConfirm != 3 {
		t.Errorf("expected HitsToConfirm=3, got %d", config.HitsToConfirm)
	}
	if config.GatingDistanceSquared != 25.0 {
		t.Errorf("expected GatingDistanceSquared=25.0, got %v", config.GatingDistanceSquared)
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
	config.HitsToConfirm = 2 // Need 2 hits to confirm
	config.MaxMisses = 2     // Quick deletion
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

	// Create track
	cluster := WorldCluster{CentroidX: 0.0, CentroidY: 0.0, SensorID: "test"}
	tracker.Update([]WorldCluster{cluster}, now)

	// Get track and manually set velocity
	tracks := tracker.GetActiveTracks()
	track := tracks[0]
	track.VX = 1.0 // 1 m/s in X direction
	track.VY = 0.0

	// Update with no clusters (miss) after 1 second
	now = now.Add(1 * time.Second)
	tracker.Update([]WorldCluster{}, now)

	// Track should have predicted to X=1.0
	updatedTrack := tracker.GetTrack(track.TrackID)
	if math.Abs(float64(updatedTrack.X)-1.0) > 0.1 {
		t.Errorf("expected X≈1.0 after prediction, got %v", updatedTrack.X)
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

// TestComputeQualityMetrics tests track quality metrics calculation
func TestComputeQualityMetrics(t *testing.T) {
	t.Run("empty history", func(t *testing.T) {
		track := &TrackedObject{
			TrackID: "track-1",
			History: []TrackPoint{},
		}
		
		track.ComputeQualityMetrics()
		
		// Should have zero metrics
		if track.TrackLengthMeters != 0 {
			t.Errorf("expected TrackLengthMeters=0, got %.2f", track.TrackLengthMeters)
		}
		if track.TrackDurationSecs != 0 {
			t.Errorf("expected TrackDurationSecs=0, got %.2f", track.TrackDurationSecs)
		}
		if track.OcclusionCount != 0 {
			t.Errorf("expected OcclusionCount=0, got %d", track.OcclusionCount)
		}
	})

	t.Run("single point", func(t *testing.T) {
		now := time.Now()
		track := &TrackedObject{
			TrackID: "track-1",
			History: []TrackPoint{
				{Timestamp: now.UnixNano(), X: 10.0, Y: 20.0},
			},
		}
		
		track.ComputeQualityMetrics()
		
		// Single point has zero length and duration
		if track.TrackLengthMeters != 0 {
			t.Errorf("expected TrackLengthMeters=0, got %.2f", track.TrackLengthMeters)
		}
		if track.TrackDurationSecs != 0 {
			t.Errorf("expected TrackDurationSecs=0, got %.2f", track.TrackDurationSecs)
		}
	})

	t.Run("straight line path", func(t *testing.T) {
		now := time.Now()
		track := &TrackedObject{
			TrackID: "track-1",
			History: []TrackPoint{
				{Timestamp: now.UnixNano(), X: 0.0, Y: 0.0},
				{Timestamp: now.Add(100 * time.Millisecond).UnixNano(), X: 3.0, Y: 0.0},
				{Timestamp: now.Add(200 * time.Millisecond).UnixNano(), X: 6.0, Y: 0.0},
				{Timestamp: now.Add(300 * time.Millisecond).UnixNano(), X: 9.0, Y: 0.0},
			},
			FirstUnixNanos: now.UnixNano(),
			LastUnixNanos:  now.Add(300 * time.Millisecond).UnixNano(),
			ObservationCount: 4,
		}
		
		track.ComputeQualityMetrics()
		
		// Length: 3 + 3 + 3 = 9 meters
		if math.Abs(float64(track.TrackLengthMeters)-9.0) > 0.01 {
			t.Errorf("expected TrackLengthMeters=9.0, got %.2f", track.TrackLengthMeters)
		}
		
		// Duration: 300ms = 0.3 seconds
		expectedDuration := 0.3
		if math.Abs(float64(track.TrackDurationSecs)-expectedDuration) > 0.01 {
			t.Errorf("expected TrackDurationSecs=%.1f, got %.2f", expectedDuration, track.TrackDurationSecs)
		}
		
		// No occlusions with 100ms gaps
		if track.OcclusionCount != 0 {
			t.Errorf("expected OcclusionCount=0, got %d", track.OcclusionCount)
		}
	})

	t.Run("path with pythagorean distance", func(t *testing.T) {
		now := time.Now()
		track := &TrackedObject{
			TrackID: "track-1",
			History: []TrackPoint{
				{Timestamp: now.UnixNano(), X: 0.0, Y: 0.0},
				{Timestamp: now.Add(100 * time.Millisecond).UnixNano(), X: 3.0, Y: 4.0}, // Distance: 5.0
			},
		}
		
		track.ComputeQualityMetrics()
		
		// Length: sqrt(3^2 + 4^2) = 5.0 meters
		expectedLength := 5.0
		if math.Abs(float64(track.TrackLengthMeters)-expectedLength) > 0.01 {
			t.Errorf("expected TrackLengthMeters=%.1f, got %.2f", expectedLength, track.TrackLengthMeters)
		}
	})

	t.Run("occlusion detection", func(t *testing.T) {
		now := time.Now()
		track := &TrackedObject{
			TrackID: "track-1",
			History: []TrackPoint{
				{Timestamp: now.UnixNano(), X: 0.0, Y: 0.0},
				{Timestamp: now.Add(100 * time.Millisecond).UnixNano(), X: 1.0, Y: 0.0},
				{Timestamp: now.Add(500 * time.Millisecond).UnixNano(), X: 2.0, Y: 0.0}, // Gap: 400ms > 200ms threshold
				{Timestamp: now.Add(600 * time.Millisecond).UnixNano(), X: 3.0, Y: 0.0},
			},
		}
		
		track.ComputeQualityMetrics()
		
		// Should detect 1 occlusion (gap of 400ms)
		if track.OcclusionCount != 1 {
			t.Errorf("expected OcclusionCount=1, got %d", track.OcclusionCount)
		}
		
		// Max occlusion: 400ms / 100ms per frame = 4 frames
		expectedMaxFrames := 4
		if track.MaxOcclusionFrames != expectedMaxFrames {
			t.Errorf("expected MaxOcclusionFrames=%d, got %d", expectedMaxFrames, track.MaxOcclusionFrames)
		}
	})

	t.Run("multiple occlusions", func(t *testing.T) {
		now := time.Now()
		track := &TrackedObject{
			TrackID: "track-1",
			History: []TrackPoint{
				{Timestamp: now.UnixNano(), X: 0.0, Y: 0.0},
				{Timestamp: now.Add(300 * time.Millisecond).UnixNano(), X: 1.0, Y: 0.0},  // Gap: 300ms
				{Timestamp: now.Add(400 * time.Millisecond).UnixNano(), X: 2.0, Y: 0.0},
				{Timestamp: now.Add(900 * time.Millisecond).UnixNano(), X: 3.0, Y: 0.0},  // Gap: 500ms (max)
				{Timestamp: now.Add(1000 * time.Millisecond).UnixNano(), X: 4.0, Y: 0.0},
			},
		}
		
		track.ComputeQualityMetrics()
		
		// Should detect 2 occlusions
		if track.OcclusionCount != 2 {
			t.Errorf("expected OcclusionCount=2, got %d", track.OcclusionCount)
		}
		
		// Max occlusion should be 500ms / 100ms = 5 frames
		expectedMaxFrames := 5
		if track.MaxOcclusionFrames != expectedMaxFrames {
			t.Errorf("expected MaxOcclusionFrames=%d, got %d", expectedMaxFrames, track.MaxOcclusionFrames)
		}
	})

	t.Run("spatial coverage calculation", func(t *testing.T) {
		now := time.Now()
		track := &TrackedObject{
			TrackID: "track-1",
			History: []TrackPoint{
				{Timestamp: now.UnixNano(), X: 0.0, Y: 0.0},
				{Timestamp: now.Add(100 * time.Millisecond).UnixNano(), X: 1.0, Y: 0.0},
				{Timestamp: now.Add(200 * time.Millisecond).UnixNano(), X: 2.0, Y: 0.0},
				{Timestamp: now.Add(300 * time.Millisecond).UnixNano(), X: 3.0, Y: 0.0},
			},
		}
		
		track.ComputeQualityMetrics()
		
		// Spatial coverage: observations per second at 10Hz
		// Duration: 0.3s, Observations: 4, Expected: 3 frames at 10Hz
		// Coverage: 4 / 3 = 1.33 (capped at 1.0)
		if track.SpatialCoverage < 0.0 || track.SpatialCoverage > 1.0 {
			t.Errorf("expected SpatialCoverage in [0,1], got %.2f", track.SpatialCoverage)
		}
	})

	t.Run("long track with good coverage", func(t *testing.T) {
		now := time.Now()
		track := &TrackedObject{
			TrackID: "track-1",
			History: make([]TrackPoint, 0, 50),
			FirstUnixNanos: now.UnixNano(),
			LastUnixNanos:  now.Add(4900 * time.Millisecond).UnixNano(),
			ObservationCount: 50,
		}
		
		// Create 50 observations over 5 seconds (10Hz)
		for i := 0; i < 50; i++ {
			point := TrackPoint{
				Timestamp: now.Add(time.Duration(i*100) * time.Millisecond).UnixNano(),
				X:         float32(i),
				Y:         0.0,
			}
			track.History = append(track.History, point)
		}
		
		track.ComputeQualityMetrics()
		
		// Length: 49 meters (each step is 1m)
		expectedLength := 49.0
		if math.Abs(float64(track.TrackLengthMeters)-expectedLength) > 0.1 {
			t.Errorf("expected TrackLengthMeters=%.1f, got %.2f", expectedLength, track.TrackLengthMeters)
		}
		
		// Duration: 4.9 seconds
		expectedDuration := 4.9
		if math.Abs(float64(track.TrackDurationSecs)-expectedDuration) > 0.1 {
			t.Errorf("expected TrackDurationSecs=%.1f, got %.2f", expectedDuration, track.TrackDurationSecs)
		}
		
		// Spatial coverage should be high (near 1.0)
		if track.SpatialCoverage < 0.9 {
			t.Errorf("expected high SpatialCoverage (>0.9), got %.2f", track.SpatialCoverage)
		}
	})
}
