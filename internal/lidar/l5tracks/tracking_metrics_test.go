package l5tracks

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetRecentlyDeletedTracks tests retrieval of recently deleted tracks.
func TestGetRecentlyDeletedTracks(t *testing.T) {
	t.Parallel()

	t.Run("returns empty slice when no deleted tracks", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{
			DeletedTrackGracePeriod: time.Second,
		})

		deleted := tracker.GetRecentlyDeletedTracks(time.Now().UnixNano())
		assert.Empty(t, deleted)
	})

	t.Run("returns empty slice with default grace period", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{
			DeletedTrackGracePeriod: 0, // Uses default
		})

		deleted := tracker.GetRecentlyDeletedTracks(time.Now().UnixNano())
		assert.Empty(t, deleted)
	})

	t.Run("returns deleted tracks within grace period", func(t *testing.T) {
		t.Parallel()
		gracePeriod := time.Second
		tracker := NewTracker(TrackerConfig{
			DeletedTrackGracePeriod: gracePeriod,
		})

		now := time.Now().UnixNano()
		deletedAt := now - int64(500*time.Millisecond) // 500ms ago

		track := &TrackedObject{
			TrackID:       "track-deleted-1",
			State:         TrackDeleted,
			LastUnixNanos: deletedAt,
			History: []TrackPoint{
				{X: 1.0, Y: 2.0, Timestamp: time.Now().UnixNano()},
				{X: 1.5, Y: 2.5, Timestamp: time.Now().UnixNano()},
			},
		}
		tracker.Tracks["track-deleted-1"] = track

		deleted := tracker.GetRecentlyDeletedTracks(now)
		require.Len(t, deleted, 1)
		assert.Equal(t, "track-deleted-1", deleted[0].TrackID)
		assert.Equal(t, TrackDeleted, deleted[0].State)
	})

	t.Run("excludes tracks past grace period", func(t *testing.T) {
		t.Parallel()
		gracePeriod := time.Second
		tracker := NewTracker(TrackerConfig{
			DeletedTrackGracePeriod: gracePeriod,
		})

		now := time.Now().UnixNano()
		deletedAt := now - int64(2*time.Second) // 2 seconds ago (past grace period)

		track := &TrackedObject{
			TrackID:       "track-old-deleted",
			State:         TrackDeleted,
			LastUnixNanos: deletedAt,
		}
		tracker.Tracks["track-old-deleted"] = track

		deleted := tracker.GetRecentlyDeletedTracks(now)
		assert.Empty(t, deleted)
	})

	t.Run("excludes non-deleted tracks", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{
			DeletedTrackGracePeriod: time.Second,
		})

		now := time.Now().UnixNano()

		tracker.Tracks["track-active"] = &TrackedObject{
			TrackID:       "track-active",
			State:         TrackConfirmed,
			LastUnixNanos: now - int64(100*time.Millisecond),
		}
		tracker.Tracks["track-tentative"] = &TrackedObject{
			TrackID:       "track-tentative",
			State:         TrackTentative,
			LastUnixNanos: now - int64(100*time.Millisecond),
		}

		deleted := tracker.GetRecentlyDeletedTracks(now)
		assert.Empty(t, deleted)
	})

	t.Run("returns deep copy of history", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{
			DeletedTrackGracePeriod: time.Second,
		})

		now := time.Now().UnixNano()
		originalHistory := []TrackPoint{
			{X: 1.0, Y: 2.0, Timestamp: time.Now().UnixNano()},
			{X: 3.0, Y: 4.0, Timestamp: time.Now().UnixNano()},
		}

		track := &TrackedObject{
			TrackID:       "track-copy-test",
			State:         TrackDeleted,
			LastUnixNanos: now - int64(100*time.Millisecond),
			History:       originalHistory,
		}
		tracker.Tracks["track-copy-test"] = track

		deleted := tracker.GetRecentlyDeletedTracks(now)
		require.Len(t, deleted, 1)

		// Modify the returned history
		deleted[0].History[0].X = 999.0

		// Original should be unchanged
		assert.Equal(t, float32(1.0), track.History[0].X)
	})

	t.Run("handles future timestamp gracefully", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{
			DeletedTrackGracePeriod: time.Second,
		})

		now := time.Now().UnixNano()
		futureTime := now + int64(10*time.Second) // Track says deleted in the future

		track := &TrackedObject{
			TrackID:       "track-future",
			State:         TrackDeleted,
			LastUnixNanos: futureTime,
		}
		tracker.Tracks["track-future"] = track

		// With negative elapsed time, should not be included
		deleted := tracker.GetRecentlyDeletedTracks(now)
		assert.Empty(t, deleted)
	})
}

// TestGetLastAssociations tests retrieval of cluster-to-track associations.
func TestGetLastAssociations(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when no associations", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{})

		associations := tracker.GetLastAssociations()
		assert.Nil(t, associations)
	})

	t.Run("returns copy of associations", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{})
		tracker.lastAssociations = []string{"track-1", "track-2", "", "track-3"}

		associations := tracker.GetLastAssociations()
		require.Len(t, associations, 4)
		assert.Equal(t, "track-1", associations[0])
		assert.Equal(t, "track-2", associations[1])
		assert.Empty(t, associations[2])
		assert.Equal(t, "track-3", associations[3])

		// Modify returned slice
		associations[0] = "modified"

		// Original should be unchanged
		assert.Equal(t, "track-1", tracker.lastAssociations[0])
	})
}

// TestGetTrackingMetrics tests the aggregate tracking metrics.
func TestGetTrackingMetrics(t *testing.T) {
	t.Parallel()

	t.Run("empty tracker returns zero metrics", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{})

		metrics := tracker.GetTrackingMetrics()
		assert.Equal(t, 0, metrics.ActiveTracks)
		assert.Equal(t, 0, metrics.TotalAlignmentSamples)
		assert.Equal(t, float32(0), metrics.MeanAlignmentRad)
	})

	t.Run("counts only non-deleted tracks", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{})

		tracker.Tracks["active-1"] = &TrackedObject{
			TrackID: "active-1",
			State:   TrackConfirmed,
		}
		tracker.Tracks["active-2"] = &TrackedObject{
			TrackID: "active-2",
			State:   TrackConfirmed,
		}
		tracker.Tracks["deleted-1"] = &TrackedObject{
			TrackID: "deleted-1",
			State:   TrackDeleted,
		}

		metrics := tracker.GetTrackingMetrics()
		assert.Equal(t, 2, metrics.ActiveTracks)
	})

	t.Run("skips tracks without alignment samples", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{})

		tracker.Tracks["no-alignment"] = &TrackedObject{
			TrackID:              "no-alignment",
			State:                TrackConfirmed,
			AlignmentSampleCount: 0,
		}

		metrics := tracker.GetTrackingMetrics()
		assert.Equal(t, 1, metrics.ActiveTracks)
		assert.Empty(t, metrics.PerTrack)
	})

	t.Run("computes aggregate metrics correctly", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{})

		tracker.Tracks["track-1"] = &TrackedObject{
			TrackID:              "track-1",
			State:                TrackConfirmed,
			AlignmentSampleCount: 10,
			AlignmentSumRad:      1.0, // Sum for 10 samples
			AlignmentMeanRad:     0.1, // 1.0/10
			AlignmentMisaligned:  2,   // 2 out of 10
			VX:                   3.0,
			VY:                   4.0, // Speed = 5.0
		}
		tracker.Tracks["track-2"] = &TrackedObject{
			TrackID:              "track-2",
			State:                TrackConfirmed,
			AlignmentSampleCount: 20,
			AlignmentSumRad:      4.0, // Sum for 20 samples
			AlignmentMeanRad:     0.2,
			AlignmentMisaligned:  5,
			VX:                   6.0,
			VY:                   8.0, // Speed = 10.0
		}

		metrics := tracker.GetTrackingMetrics()

		assert.Equal(t, 2, metrics.ActiveTracks)
		assert.Equal(t, 30, metrics.TotalAlignmentSamples) // 10 + 20
		assert.Equal(t, 7, metrics.TotalMisaligned)        // 2 + 5

		// Mean alignment = (1.0 + 4.0) / 30 = 0.1667
		assert.InDelta(t, 0.1667, float64(metrics.MeanAlignmentRad), 0.01)

		// Misalignment ratio = 7 / 30 = 0.2333
		assert.InDelta(t, 0.2333, float64(metrics.MisalignmentRatio), 0.01)

		// Per-track metrics
		require.Len(t, metrics.PerTrack, 2)
	})

	t.Run("per-track metrics are correct", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{})

		tracker.Tracks["track-1"] = &TrackedObject{
			TrackID:              "track-1",
			State:                TrackConfirmed,
			AlignmentSampleCount: 100,
			AlignmentMeanRad:     0.05,
			AlignmentMisaligned:  10,
			VX:                   3.0,
			VY:                   4.0,
		}

		metrics := tracker.GetTrackingMetrics()
		require.Len(t, metrics.PerTrack, 1)

		perTrack := metrics.PerTrack[0]
		assert.Equal(t, "track-1", perTrack.TrackID)
		assert.Equal(t, "confirmed", perTrack.State)
		assert.Equal(t, 100, perTrack.SampleCount)
		assert.Equal(t, float32(0.05), perTrack.MeanAlignmentRad)
		assert.Equal(t, 10, perTrack.MisalignedCount)
		assert.InDelta(t, 0.1, float64(perTrack.MisalignmentRate), 0.001) // 10/100
		assert.InDelta(t, 5.0, float64(perTrack.SpeedMps), 0.01)
	})
}

// TestRecordFrameStatsAndSceneMetrics tests the scene-level foreground capture metrics.
func TestRecordFrameStatsAndSceneMetrics(t *testing.T) {
	t.Parallel()

	t.Run("zero frames returns zero ratios", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{})

		metrics := tracker.GetTrackingMetrics()
		assert.Equal(t, float32(0), metrics.ForegroundCaptureRatio)
		assert.Equal(t, float32(0), metrics.UnboundedPointRatio)
		assert.Equal(t, float32(0), metrics.EmptyBoxRatio)
	})

	t.Run("records foreground and clustered points", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{})

		// Simulate 2 frames: first with 100 foreground, 80 clustered; second with 50 foreground, 40 clustered
		tracker.RecordFrameStats(100, 80)
		tracker.RecordFrameStats(50, 40)

		metrics := tracker.GetTrackingMetrics()
		// Total: 150 foreground, 120 clustered
		// Capture ratio = 120/150 = 0.8
		assert.InDelta(t, 0.8, float64(metrics.ForegroundCaptureRatio), 0.01)
		// Unbounded = 1 - 0.8 = 0.2
		assert.InDelta(t, 0.2, float64(metrics.UnboundedPointRatio), 0.01)
	})

	t.Run("reset clears accumulators", func(t *testing.T) {
		t.Parallel()
		tracker := NewTracker(TrackerConfig{})

		tracker.RecordFrameStats(100, 80)
		tracker.Reset()

		metrics := tracker.GetTrackingMetrics()
		assert.Equal(t, float32(0), metrics.ForegroundCaptureRatio)
		assert.Equal(t, float32(0), metrics.UnboundedPointRatio)
		assert.Equal(t, float32(0), metrics.EmptyBoxRatio)
	})
}
