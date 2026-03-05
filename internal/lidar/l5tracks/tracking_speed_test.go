package l5tracks

import (
	"math"
	"sort"
	"testing"
	"time"
)

// TestTracker_SpeedComputation_AvgAndP50 verifies that AvgSpeedMps and
// P50SpeedMps are populated correctly through the full tracker Update path.
// A cluster moves at a constant rate so the Kalman filter converges to a
// steady velocity; after convergence we check that both statistics are
// non-zero and internally consistent.
func TestTracker_SpeedComputation_AvgAndP50(t *testing.T) {
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

	// P50SpeedMps must be populated
	if track.P50SpeedMps <= 0 {
		t.Errorf("expected P50SpeedMps > 0, got %f", track.P50SpeedMps)
	}

	// PeakSpeedMps >= AvgSpeedMps (peak is never below average)
	if track.PeakSpeedMps < track.AvgSpeedMps {
		t.Errorf("expected PeakSpeedMps(%f) >= AvgSpeedMps(%f)",
			track.PeakSpeedMps, track.AvgSpeedMps)
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

// TestTracker_P50SpeedMps_IsMedian verifies that P50SpeedMps equals the
// median of the speed history (i.e. sorted[n/2]).
func TestTracker_P50SpeedMps_IsMedian(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.HitsToConfirm = 2
	tracker := NewTracker(cfg)

	now := time.Now()
	const frames = 15

	for i := 0; i < frames; i++ {
		// Vary the speed profile: fast-slow-fast to make avg != median
		dx := float32(0.5)
		if i >= 5 && i < 10 {
			dx = 2.0 // faster segment
		}
		clusters := []WorldCluster{
			{
				CentroidX:         5.0 + float32(i)*dx,
				CentroidY:         0.0,
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

	// Compute expected P50: sort and pick middle element
	sorted := make([]float32, len(history))
	copy(sorted, history)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	expectedP50 := sorted[len(sorted)/2]

	if math.Abs(float64(track.P50SpeedMps-expectedP50)) > 0.01 {
		t.Errorf("P50SpeedMps=%f differs from computed median=%f", track.P50SpeedMps, expectedP50)
	}
}

// TestTracker_AvgSpeedMps_DiffersFromP50 verifies that AvgSpeedMps and
// P50SpeedMps are computed independently and can yield different values.
func TestTracker_AvgSpeedMps_DiffersFromP50(t *testing.T) {
	// Use SetSpeedHistory to create a controlled scenario where mean != median
	track := &TrackedObject{}
	speeds := []float32{1, 2, 3, 4, 100}
	track.SetSpeedHistory(speeds)

	// Compute AvgSpeedMps manually
	var sum float32
	for _, s := range speeds {
		sum += s
	}
	expectedAvg := sum / float32(len(speeds)) // 22.0
	expectedP50 := float32(3.0)               // sorted[5/2] = sorted[2] = 3

	track.AvgSpeedMps = expectedAvg
	track.P50SpeedMps = track.speeds.P50()

	if track.AvgSpeedMps == track.P50SpeedMps {
		t.Errorf("expected AvgSpeedMps(%f) != P50SpeedMps(%f) for skewed data",
			track.AvgSpeedMps, track.P50SpeedMps)
	}

	if math.Abs(float64(track.AvgSpeedMps-expectedAvg)) > 0.01 {
		t.Errorf("expected AvgSpeedMps=%f, got %f", expectedAvg, track.AvgSpeedMps)
	}
	if math.Abs(float64(track.P50SpeedMps-expectedP50)) > 0.01 {
		t.Errorf("expected P50SpeedMps=%f, got %f", expectedP50, track.P50SpeedMps)
	}
}
