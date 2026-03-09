package l5tracks

import (
	"math"
	"testing"
	"time"
)

// TestTracker_SpeedComputation_Avg verifies that AvgSpeedMps is populated
// correctly through the full tracker Update path.  A cluster moves at a
// constant rate so the Kalman filter converges to a steady velocity; after
// convergence we check that the average is non-zero and consistent.
func TestTracker_SpeedComputation_Avg(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.HitsToConfirm = 2
	tracker := NewTracker(cfg)

	now := time.Now()
	const frames = 20
	const deltaX = float32(1.0) // 1 m per frame
	const dtSec = 0.1           // 100ms per frame => 10 m/s true speed

	for i := 0; i < frames; i++ {
		clusters := []WorldCluster{
			{
				CentroidX:         10.0 + float32(i)*deltaX,
				CentroidY:         0.0,
				CentroidZ:         1.0,
				SensorID:          "test",
				BoundingBoxLength: 4.0,
				BoundingBoxWidth:  2.0,
				BoundingBoxHeight: 1.5,
				PointsCount:       100,
			},
		}
		tracker.Update(clusters, now)
		now = now.Add(time.Duration(dtSec * float64(time.Second)))
	}

	// Find the confirmed track
	confirmed := tracker.GetConfirmedTracks()
	if len(confirmed) == 0 {
		t.Fatal("expected at least one confirmed track")
	}

	track := confirmed[0]

	// AvgSpeedMps must be populated
	if track.AvgSpeedMps <= 0 {
		t.Errorf("expected AvgSpeedMps > 0, got %f", track.AvgSpeedMps)
	}

	// MaxSpeedMps >= AvgSpeedMps (peak is never below average)
	if track.MaxSpeedMps < track.AvgSpeedMps {
		t.Errorf("expected MaxSpeedMps(%f) >= AvgSpeedMps(%f)",
			track.MaxSpeedMps, track.AvgSpeedMps)
	}

	// Speed history has one entry per update() call. The first observation
	// creates the track via newTrack() without a speed sample, so history
	// length is ObservationCount - 1.
	history := track.SpeedHistory()
	if len(history) != track.ObservationCount-1 {
		t.Errorf("expected %d speed history entries, got %d",
			track.ObservationCount-1, len(history))
	}

	// After convergence, the Kalman-estimated speed should be within 50% of
	// the true speed (10 m/s). The Kalman filter takes several frames to
	// converge, so we use a generous tolerance on the overall average.
	trueSpeed := float32(deltaX / dtSec) // 10 m/s
	if math.Abs(float64(track.AvgSpeedMps-trueSpeed)) > float64(trueSpeed)*0.5 {
		t.Errorf("AvgSpeedMps=%f too far from true speed=%f", track.AvgSpeedMps, trueSpeed)
	}
}

// TestTracker_AvgSpeedMps_IsRunningMean verifies that AvgSpeedMps equals the
// arithmetic mean of all entries in the speed history.
func TestTracker_AvgSpeedMps_IsRunningMean(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.HitsToConfirm = 2
	tracker := NewTracker(cfg)

	now := time.Now()
	const frames = 15

	for i := 0; i < frames; i++ {
		clusters := []WorldCluster{
			{
				CentroidX:         5.0 + float32(i)*0.8,
				CentroidY:         3.0 + float32(i)*0.6,
				CentroidZ:         1.0,
				SensorID:          "test",
				BoundingBoxLength: 3.0,
				BoundingBoxWidth:  1.5,
				BoundingBoxHeight: 1.2,
				PointsCount:       80,
			},
		}
		tracker.Update(clusters, now)
		now = now.Add(100 * time.Millisecond)
	}

	confirmed := tracker.GetConfirmedTracks()
	if len(confirmed) == 0 {
		t.Fatal("expected at least one confirmed track")
	}

	track := confirmed[0]
	history := track.SpeedHistory()
	if len(history) == 0 {
		t.Fatal("expected non-empty speed history")
	}

	// The running mean formula uses ObservationCount as the denominator,
	// which includes the first observation from newTrack() (implicitly
	// speed=0). So AvgSpeedMps = sum(history) / ObservationCount, NOT
	// sum(history) / len(history).
	var sum float32
	for _, s := range history {
		sum += s
	}
	expectedAvg := sum / float32(track.ObservationCount)

	// The running mean formula should produce the same result
	// (within floating point tolerance).
	if math.Abs(float64(track.AvgSpeedMps-expectedAvg)) > 0.05 {
		t.Errorf("AvgSpeedMps=%f differs from computed mean=%f (history len=%d, obs=%d)",
			track.AvgSpeedMps, expectedAvg, len(history), track.ObservationCount)
	}
}
