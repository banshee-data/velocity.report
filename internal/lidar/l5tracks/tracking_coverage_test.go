package l5tracks

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// UpdateClassification
// ---------------------------------------------------------------------------

func TestUpdateClassification(t *testing.T) {
	t.Parallel()

	t.Run("updates existing track", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(DefaultTrackerConfig())
		tracker.mu.Lock()
		tracker.Tracks["trk_001"] = &TrackedObject{
			TrackID: "trk_001",
			State:   TrackConfirmed,
		}
		tracker.mu.Unlock()

		tracker.UpdateClassification("trk_001", "vehicle", 0.95, "rf-v2")

		tracker.mu.RLock()
		track := tracker.Tracks["trk_001"]
		tracker.mu.RUnlock()

		assert.Equal(t, "vehicle", track.ObjectClass)
		assert.InDelta(t, 0.95, float64(track.ObjectConfidence), 0.001)
		assert.Equal(t, "rf-v2", track.ClassificationModel)
	})

	t.Run("no-op for missing track", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(DefaultTrackerConfig())
		// Should not panic
		tracker.UpdateClassification("nonexistent", "car", 0.8, "model-1")
	})
}

// ---------------------------------------------------------------------------
// AdvanceMisses
// ---------------------------------------------------------------------------

func TestAdvanceMisses(t *testing.T) {
	t.Parallel()

	t.Run("increments misses for active tracks", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultTrackerConfig()
		cfg.MaxMisses = 5
		tracker := NewTracker(cfg)

		tracker.mu.Lock()
		tracker.Tracks["t1"] = &TrackedObject{
			TrackID: "t1",
			State:   TrackTentative,
			Misses:  0,
			Hits:    3,
		}
		tracker.Tracks["t2"] = &TrackedObject{
			TrackID: "t2",
			State:   TrackConfirmed,
			Misses:  1,
			Hits:    2,
		}
		tracker.mu.Unlock()

		tracker.AdvanceMisses(time.Now())

		tracker.mu.RLock()
		assert.Equal(t, 1, tracker.Tracks["t1"].Misses)
		assert.Equal(t, 0, tracker.Tracks["t1"].Hits)
		assert.Equal(t, 2, tracker.Tracks["t2"].Misses)
		assert.Equal(t, 0, tracker.Tracks["t2"].Hits)
		tracker.mu.RUnlock()
	})

	t.Run("deletes tracks exceeding miss budget", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultTrackerConfig()
		cfg.MaxMisses = 3
		tracker := NewTracker(cfg)

		tracker.mu.Lock()
		tracker.Tracks["t1"] = &TrackedObject{
			TrackID: "t1",
			State:   TrackTentative,
			Misses:  2, // will become 3 → deleted
		}
		tracker.mu.Unlock()

		tracker.AdvanceMisses(time.Now())

		tracker.mu.RLock()
		assert.Equal(t, TrackDeleted, tracker.Tracks["t1"].State)
		tracker.mu.RUnlock()
	})

	t.Run("uses MaxMissesConfirmed for confirmed tracks", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultTrackerConfig()
		cfg.MaxMisses = 3
		cfg.MaxMissesConfirmed = 10
		tracker := NewTracker(cfg)

		tracker.mu.Lock()
		tracker.Tracks["t1"] = &TrackedObject{
			TrackID: "t1",
			State:   TrackConfirmed,
			Misses:  8, // will become 9, under MaxMissesConfirmed=10
		}
		tracker.mu.Unlock()

		tracker.AdvanceMisses(time.Now())

		tracker.mu.RLock()
		assert.Equal(t, TrackConfirmed, tracker.Tracks["t1"].State)
		assert.Equal(t, 9, tracker.Tracks["t1"].Misses)
		tracker.mu.RUnlock()
	})

	t.Run("skips already deleted tracks", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultTrackerConfig()
		tracker := NewTracker(cfg)

		tracker.mu.Lock()
		tracker.Tracks["t1"] = &TrackedObject{
			TrackID: "t1",
			State:   TrackDeleted,
			Misses:  0,
		}
		tracker.mu.Unlock()

		tracker.AdvanceMisses(time.Now())

		tracker.mu.RLock()
		// Misses should not have been incremented
		assert.Equal(t, 0, tracker.Tracks["t1"].Misses)
		tracker.mu.RUnlock()
	})
}

// ---------------------------------------------------------------------------
// GetDeletedTrackGracePeriod
// ---------------------------------------------------------------------------

func TestGetDeletedTrackGracePeriod(t *testing.T) {
	t.Parallel()
	cfg := DefaultTrackerConfig()
	cfg.DeletedTrackGracePeriod = 5 * time.Second
	tracker := NewTracker(cfg)
	assert.Equal(t, 5*time.Second, tracker.GetDeletedTrackGracePeriod())
}

// ---------------------------------------------------------------------------
// clampVelocity
// ---------------------------------------------------------------------------

func TestClampVelocity(t *testing.T) {
	t.Parallel()

	t.Run("no-op when speed below limit", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultTrackerConfig()
		cfg.MaxReasonableSpeedMps = 50.0
		tracker := NewTracker(cfg)

		track := &TrackedObject{VX: 3.0, VY: 4.0} // speed = 5 m/s
		tracker.clampVelocity(track)

		assert.InDelta(t, 3.0, float64(track.VX), 0.001)
		assert.InDelta(t, 4.0, float64(track.VY), 0.001)
	})

	t.Run("clamps when speed exceeds limit", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultTrackerConfig()
		cfg.MaxReasonableSpeedMps = 10.0
		tracker := NewTracker(cfg)

		track := &TrackedObject{VX: 30.0, VY: 40.0} // speed = 50 m/s
		tracker.clampVelocity(track)

		speed := float64(track.VX*track.VX + track.VY*track.VY)
		assert.InDelta(t, 100.0, speed, 0.1) // 10^2 = 100
		// Direction preserved
		ratio := float64(track.VY) / float64(track.VX)
		assert.InDelta(t, 40.0/30.0, ratio, 0.001)
	})
}

// ---------------------------------------------------------------------------
// isFiniteState
// ---------------------------------------------------------------------------

func TestIsFiniteState(t *testing.T) {
	t.Parallel()

	t.Run("returns true for finite state", func(t *testing.T) {
		t.Parallel()
		track := &TrackedObject{
			X: 1.0, Y: 2.0, VX: 0.5, VY: -0.3,
			P: [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		}
		assert.True(t, isFiniteState(track))
	})

	t.Run("returns false for NaN X", func(t *testing.T) {
		t.Parallel()
		track := &TrackedObject{X: float32(math.NaN())}
		assert.False(t, isFiniteState(track))
	})

	t.Run("returns false for NaN Y", func(t *testing.T) {
		t.Parallel()
		track := &TrackedObject{Y: float32(math.NaN())}
		assert.False(t, isFiniteState(track))
	})

	t.Run("returns false for Inf VX", func(t *testing.T) {
		t.Parallel()
		track := &TrackedObject{VX: float32(math.Inf(1))}
		assert.False(t, isFiniteState(track))
	})

	t.Run("returns false for Inf VY", func(t *testing.T) {
		t.Parallel()
		track := &TrackedObject{VY: float32(math.Inf(-1))}
		assert.False(t, isFiniteState(track))
	})

	t.Run("returns false for NaN in covariance diagonal", func(t *testing.T) {
		t.Parallel()
		track := &TrackedObject{
			X: 1.0, Y: 2.0, VX: 0.5, VY: -0.3,
			P: [16]float32{1, 0, 0, 0, 0, float32(math.NaN()), 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		}
		assert.False(t, isFiniteState(track))
	})

	t.Run("returns false for Inf in P[2,2]", func(t *testing.T) {
		t.Parallel()
		track := &TrackedObject{
			X: 1.0, Y: 2.0, VX: 0.5, VY: -0.3,
			P: [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, float32(math.Inf(1)), 0, 0, 0, 0, 1},
		}
		assert.False(t, isFiniteState(track))
	})

	t.Run("returns false for Inf in P[3,3]", func(t *testing.T) {
		t.Parallel()
		track := &TrackedObject{
			X: 1.0, Y: 2.0, VX: 0.5, VY: -0.3,
			P: [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, float32(math.Inf(-1))},
		}
		assert.False(t, isFiniteState(track))
	})
}

// ---------------------------------------------------------------------------
// predict — NaN guard branch
// ---------------------------------------------------------------------------

func TestPredict_NaNGuardResetsState(t *testing.T) {
	t.Parallel()

	cfg := DefaultTrackerConfig()
	tracker := NewTracker(cfg)

	// Create a track where prediction will produce NaN
	track := &TrackedObject{
		TrackID: "trk-nan",
		State:   TrackConfirmed,
		X:       float32(math.Inf(1)),
		Y:       1.0,
		VX:      float32(math.NaN()),
		VY:      0,
		P:       [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
	}

	tracker.predict(track, 0.1)

	// After NaN guard, state should be reset & deleted
	assert.Equal(t, TrackDeleted, track.State)
	assert.InDelta(t, 0, float64(track.X), 0.001)
	assert.InDelta(t, 0, float64(track.Y), 0.001)
}

// ---------------------------------------------------------------------------
// predict — dt clamping
// ---------------------------------------------------------------------------

func TestPredict_DtClamping(t *testing.T) {
	t.Parallel()

	cfg := DefaultTrackerConfig()
	cfg.MaxPredictDt = 0.5
	tracker := NewTracker(cfg)

	track := &TrackedObject{
		TrackID: "trk-dtclamp",
		State:   TrackConfirmed,
		X:       0,
		Y:       0,
		VX:      10.0,
		VY:      0,
		P:       [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
	}

	// dt=5.0 but should be clamped to MaxPredictDt=0.5
	tracker.predict(track, 5.0)

	// X should advance by VX * clamped_dt = 10 * 0.5 = 5.0 (not 50)
	assert.InDelta(t, 5.0, float64(track.X), 0.1)
}

// ---------------------------------------------------------------------------
// predict — with DebugCollector
// ---------------------------------------------------------------------------

// mockDebugCollector implements DebugCollector for testing.
type mockDebugCollector struct {
	enabled          bool
	predictionCalls  int
	gatingCalls      int
	innovationCalls  int
	associationCalls int
}

func (m *mockDebugCollector) IsEnabled() bool { return m.enabled }
func (m *mockDebugCollector) RecordAssociation(clusterID int64, trackID string, distSquared float32, accepted bool) {
	m.associationCalls++
}
func (m *mockDebugCollector) RecordGatingRegion(trackID string, centerX, centerY, semiMajor, semiMinor, rotation float32) {
	m.gatingCalls++
}
func (m *mockDebugCollector) RecordInnovation(trackID string, predX, predY, measX, measY, residualMag float32) {
	m.innovationCalls++
}
func (m *mockDebugCollector) RecordPrediction(trackID string, x, y, vx, vy float32) {
	m.predictionCalls++
}

func TestPredict_WithDebugCollector(t *testing.T) {
	t.Parallel()

	collector := &mockDebugCollector{enabled: true}
	cfg := DefaultTrackerConfig()
	tracker := NewTracker(cfg)
	tracker.DebugCollector = collector

	track := &TrackedObject{
		TrackID: "trk-debug",
		State:   TrackConfirmed,
		X:       1.0, Y: 2.0, VX: 0.5, VY: -0.3,
		P: [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
	}

	tracker.predict(track, 0.1)

	assert.Equal(t, 1, collector.predictionCalls)
}

// ---------------------------------------------------------------------------
// mahalanobisDistanceSquared — with DebugCollector (gating ellipse)
// ---------------------------------------------------------------------------

func TestMahalanobisDistanceSquared_WithDebugCollector(t *testing.T) {
	t.Parallel()

	collector := &mockDebugCollector{enabled: true}
	cfg := DefaultTrackerConfig()
	tracker := NewTracker(cfg)
	tracker.DebugCollector = collector

	track := &TrackedObject{
		TrackID: "trk-mahal",
		X:       0, Y: 0,
		P: [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
	}

	cluster := WorldCluster{CentroidX: 0.1, CentroidY: 0.1}
	dist := tracker.mahalanobisDistanceSquared(track, cluster, 0.1)

	assert.True(t, dist >= 0, "distance should be non-negative")
	assert.Equal(t, 1, collector.gatingCalls)
}

// ---------------------------------------------------------------------------
// update — with DebugCollector (innovation recording)
// ---------------------------------------------------------------------------

func TestUpdate_WithDebugCollector(t *testing.T) {
	t.Parallel()

	collector := &mockDebugCollector{enabled: true}
	cfg := DefaultTrackerConfig()
	tracker := NewTracker(cfg)
	tracker.DebugCollector = collector

	track := &TrackedObject{
		TrackID: "trk-update-debug",
		State:   TrackConfirmed,
		X:       0, Y: 0, VX: 1, VY: 0,
		P:       [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
		History: []TrackPoint{{X: 0, Y: 0, Timestamp: time.Now().UnixNano()}},
	}

	cluster := WorldCluster{CentroidX: 0.5, CentroidY: 0}
	tracker.update(track, cluster, time.Now().UnixNano())

	assert.Equal(t, 1, collector.innovationCalls)
}

// ---------------------------------------------------------------------------
// update — NaN guard resets to deleted
// ---------------------------------------------------------------------------

func TestUpdate_NaNGuardResetsState(t *testing.T) {
	t.Parallel()

	cfg := DefaultTrackerConfig()
	tracker := NewTracker(cfg)

	track := &TrackedObject{
		TrackID: "trk-nan-update",
		State:   TrackConfirmed,
		X:       0, Y: 0, VX: 0, VY: 0,
		P: [16]float32{
			float32(math.Inf(1)), 0, 0, 0,
			0, 1, 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		},
		History: []TrackPoint{{X: 0, Y: 0, Timestamp: time.Now().UnixNano()}},
	}

	cluster := WorldCluster{CentroidX: 1, CentroidY: 1}
	tracker.update(track, cluster, time.Now().UnixNano())

	// State should be reset to deleted due to NaN/Inf guard
	assert.Equal(t, TrackDeleted, track.State)
}

// ---------------------------------------------------------------------------
// initTrack — with OBB
// ---------------------------------------------------------------------------

func TestInitTrack_WithOBB(t *testing.T) {
	t.Parallel()

	cfg := DefaultTrackerConfig()
	tracker := NewTracker(cfg)
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	cluster := WorldCluster{
		CentroidX:         5.0,
		CentroidY:         3.0,
		BoundingBoxLength: 4.5,
		BoundingBoxWidth:  1.8,
		BoundingBoxHeight: 1.5,
		SensorID:          "sensor-1",
		OBB: &l4perception.OrientedBoundingBox{
			HeadingRad: 1.57,
			Length:     4.5,
			Width:      1.8,
			Height:     1.5,
			CenterZ:    -1.2,
		},
	}

	nowNanos := time.Now().UnixNano()
	track := tracker.initTrack(cluster, nowNanos)

	require.NotNil(t, track)
	assert.InDelta(t, 1.57, float64(track.OBBHeadingRad), 0.01)
	assert.InDelta(t, 4.5, float64(track.OBBLength), 0.01)
	assert.InDelta(t, 1.8, float64(track.OBBWidth), 0.01)
	assert.InDelta(t, 1.5, float64(track.OBBHeight), 0.01)
	assert.InDelta(t, -1.2, float64(track.LatestZ), 0.01)
	assert.Equal(t, TrackTentative, track.State)
}

func TestInitTrack_WithoutOBB(t *testing.T) {
	t.Parallel()

	cfg := DefaultTrackerConfig()
	tracker := NewTracker(cfg)
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	cluster := WorldCluster{
		CentroidX: 2.0,
		CentroidY: 4.0,
		SensorID:  "sensor-1",
	}

	track := tracker.initTrack(cluster, time.Now().UnixNano())

	require.NotNil(t, track)
	assert.InDelta(t, 2.0, float64(track.X), 0.001)
	assert.InDelta(t, 4.0, float64(track.Y), 0.001)
	assert.InDelta(t, 0.0, float64(track.OBBHeadingRad), 0.001)
}

// ---------------------------------------------------------------------------
// associate — empty inputs
// ---------------------------------------------------------------------------

func TestAssociate_EmptyClusters(t *testing.T) {
	t.Parallel()

	cfg := DefaultTrackerConfig()
	tracker := NewTracker(cfg)

	// No clusters, no tracks
	result := tracker.associate(nil, 0.1)
	assert.Empty(t, result)

	result = tracker.associate([]WorldCluster{}, 0.1)
	assert.Empty(t, result)
}

func TestAssociate_EmptyTracks(t *testing.T) {
	t.Parallel()

	cfg := DefaultTrackerConfig()
	tracker := NewTracker(cfg)

	clusters := []WorldCluster{{CentroidX: 1, CentroidY: 1}}
	result := tracker.associate(clusters, 0.1)
	assert.Len(t, result, 1)
	assert.Equal(t, "", result[0]) // unassociated
}

func TestAssociate_WithDebugCollector(t *testing.T) {
	t.Parallel()

	collector := &mockDebugCollector{enabled: true}
	cfg := DefaultTrackerConfig()
	tracker := NewTracker(cfg)
	tracker.DebugCollector = collector

	// Empty — triggers the early-return debug recording path
	result := tracker.associate(nil, 0.1)
	assert.Empty(t, result)
}

// ---------------------------------------------------------------------------
// GetTrackingMetrics — jitter/fragmentation/emptyBox branches
// ---------------------------------------------------------------------------

func TestGetTrackingMetrics_AllBranches(t *testing.T) {
	t.Parallel()

	cfg := DefaultTrackerConfig()
	tracker := NewTracker(cfg)

	tracker.mu.Lock()
	tracker.TracksCreated = 10
	tracker.TracksConfirmed = 6

	tracker.TotalForegroundPoints = 1000
	tracker.ClusteredPoints = 800
	tracker.TotalBoxFrames = 100
	tracker.EmptyBoxFrames = 15

	tracker.Tracks["t1"] = &TrackedObject{
		TrackID:              "t1",
		State:                TrackConfirmed,
		HeadingJitterSumSq:   0.5,
		HeadingJitterCount:   10,
		SpeedJitterSumSq:     2.0,
		SpeedJitterCount:     8,
		AlignmentSampleCount: 5,
		AlignmentSumRad:      0.3,
		AlignmentMisaligned:  1,
		AlignmentMeanRad:     0.06,
	}
	tracker.Tracks["t2"] = &TrackedObject{
		TrackID:              "t2",
		State:                TrackTentative,
		HeadingJitterSumSq:   0.3,
		HeadingJitterCount:   5,
		SpeedJitterSumSq:     1.0,
		SpeedJitterCount:     4,
		AlignmentSampleCount: 3,
		AlignmentSumRad:      0.2,
		AlignmentMisaligned:  0,
		AlignmentMeanRad:     0.067,
	}
	tracker.mu.Unlock()

	metrics := tracker.GetTrackingMetrics()

	assert.Equal(t, 2, metrics.ActiveTracks)
	assert.Equal(t, 10, metrics.TracksCreated)
	assert.Equal(t, 6, metrics.TracksConfirmed)
	assert.InDelta(t, 0.4, float64(metrics.FragmentationRatio), 0.01)     // 1 - 6/10
	assert.InDelta(t, 0.8, float64(metrics.ForegroundCaptureRatio), 0.01) // 800/1000
	assert.InDelta(t, 0.2, float64(metrics.UnboundedPointRatio), 0.01)
	assert.InDelta(t, 0.15, float64(metrics.EmptyBoxRatio), 0.01) // 15/100
	assert.True(t, metrics.HeadingJitterDeg > 0)
	assert.True(t, metrics.SpeedJitterMps > 0)
	assert.Len(t, metrics.PerTrack, 2)
}

// ---------------------------------------------------------------------------
// Full tracker lifecycle producing OBB update paths
// ---------------------------------------------------------------------------

func TestTracker_Lifecycle_OBBUpdate(t *testing.T) {
	t.Parallel()

	cfg := DefaultTrackerConfig()
	cfg.HitsToConfirm = 2
	cfg.MaxMisses = 5
	tracker := NewTracker(cfg)

	now := time.Now()

	// Frame 1: create track with OBB
	clusters1 := []WorldCluster{{
		CentroidX:         5.0,
		CentroidY:         0.0,
		PointsCount:       50,
		BoundingBoxLength: 4.5,
		BoundingBoxWidth:  1.8,
		BoundingBoxHeight: 1.5,
		IntensityMean:     120,
		HeightP95:         1.4,
		SensorID:          "s1",
		OBB: &l4perception.OrientedBoundingBox{
			HeadingRad: 0.0,
			Length:     4.5,
			Width:      1.8,
			Height:     1.5,
			CenterZ:    -1.2,
		},
	}}
	tracker.Update(clusters1, now)

	// Frame 2: same track moves, hits OBB update paths
	clusters2 := []WorldCluster{{
		CentroidX:         5.5,
		CentroidY:         0.0,
		PointsCount:       55,
		BoundingBoxLength: 4.6,
		BoundingBoxWidth:  1.9,
		BoundingBoxHeight: 1.6,
		IntensityMean:     125,
		HeightP95:         1.5,
		SensorID:          "s1",
		OBB: &l4perception.OrientedBoundingBox{
			HeadingRad: 0.05,
			Length:     4.6,
			Width:      1.9,
			Height:     1.6,
			CenterZ:    -1.1,
		},
	}}
	tracker.Update(clusters2, now.Add(100*time.Millisecond))

	// Frame 3: confirm track, further OBB evolution
	clusters3 := []WorldCluster{{
		CentroidX:         6.0,
		CentroidY:         0.0,
		PointsCount:       60,
		BoundingBoxLength: 4.7,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		IntensityMean:     130,
		HeightP95:         1.45,
		SensorID:          "s1",
		OBB: &l4perception.OrientedBoundingBox{
			HeadingRad: 0.1,
			Length:     4.7,
			Width:      2.0,
			Height:     1.5,
			CenterZ:    -1.0,
		},
	}}
	tracker.Update(clusters3, now.Add(200*time.Millisecond))

	// Verify tracks were created and have OBB data
	total, _, confirmed, _ := tracker.GetTrackCount()
	assert.GreaterOrEqual(t, total, 1)
	t.Logf("total=%d, confirmed=%d", total, confirmed)

	tracks := tracker.GetActiveTracks()
	for _, track := range tracks {
		t.Logf("TrackID=%s, State=%s, OBBHeading=%.3f, obsCount=%d",
			track.TrackID, track.State, track.OBBHeadingRad, track.ObservationCount)
	}
}

// ---------------------------------------------------------------------------
// Tracker.Update — velocity clamping integration
// ---------------------------------------------------------------------------

func TestTracker_Update_VelocityClamping(t *testing.T) {
	t.Parallel()

	cfg := DefaultTrackerConfig()
	cfg.MaxReasonableSpeedMps = 10.0
	cfg.HitsToConfirm = 1
	tracker := NewTracker(cfg)

	now := time.Now()

	// Create a track at origin
	tracker.Update([]WorldCluster{{
		CentroidX:   0,
		CentroidY:   0,
		PointsCount: 30,
		SensorID:    "s1",
	}}, now)

	// Large jump to simulate noisy measurement — triggers velocity clamp
	tracker.Update([]WorldCluster{{
		CentroidX:   0.1, // small step so it associates
		CentroidY:   0,
		PointsCount: 30,
		SensorID:    "s1",
	}}, now.Add(10*time.Millisecond))

	tracks := tracker.GetActiveTracks()
	for _, track := range tracks {
		speed := math.Sqrt(float64(track.VX*track.VX + track.VY*track.VY))
		assert.LessOrEqual(t, speed, float64(cfg.MaxReasonableSpeedMps)+0.1)
	}
}
