package l5tracks

import (
	"math"
	"testing"
	"time"
)

// A car occluded mid-passage keeps the length of the whole passage. The
// distance covered while it was hidden is measured from its last observed
// point when it is reacquired; measuring from the last coasted point counted
// only the final frame of the gap. A coast that never ends in reacquisition
// adds nothing.
func TestTrackLengthSpansAnOcclusion(t *testing.T) {
	const speed, period, frames = 8.0, 100 * time.Millisecond, 30
	step := speed * period.Seconds()
	for _, c := range []struct {
		name                 string
		hiddenFrom, hidden   int // first hidden frame, and how many
		observed             int
		wantCoasted, wantGap int // coasted trail points, and the gap they bridge
	}{
		{"no gap", 0, 0, 30, 0, 0},
		{"five-frame gap", 15, 5, 25, 5, 5},
		{"ten-frame gap", 10, 10, 20, 10, 10},
		// Hidden from frame 24 to the end: six coasted points, no reacquisition.
		{"trailing coast", 24, 6, 24, 6, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			body := uncCar(-12, 15, speed)
			body.hidden = func(f int) bool { return f >= c.hiddenFrom && f < c.hiddenFrom+c.hidden }
			scene := uncScene{frames: frames, period: period, seed: 5, bodies: []uncBody{body}}
			tk := runUncScene(t, DefaultTrackerConfig(), scene.render(), 0, false)
			if len(tk.Tracks) != 1 {
				t.Fatalf("%d tracks, want the car's one", len(tk.Tracks))
			}
			var track *TrackedObject
			for _, tr := range tk.Tracks {
				track = tr
			}
			if track.ObservationCount != c.observed {
				t.Fatalf("%d observations, want %d", track.ObservationCount, c.observed)
			}
			coasted := 0
			for _, p := range track.History {
				if !p.Support.IsObserved() {
					coasted++
				}
			}
			if coasted != c.wantCoasted {
				t.Fatalf("%d coasted trail points, want %d", coasted, c.wantCoasted)
			}

			// The car moves in a straight line without noise, so the true
			// distance between its first and last observation is exact.
			want := float64(c.observed-1+c.wantGap) * step
			got := float64(track.TrackLengthMeters)
			if math.Abs(got-want) > 0.1 {
				t.Errorf("length %.2f m, want %.2f m (the car's travel between its first and last observation)", got, want)
			}

			// The recomputation over the trail follows the same rule.
			recomputed := *track
			recomputed.ComputeQualityMetrics()
			if d := math.Abs(float64(recomputed.TrackLengthMeters - track.TrackLengthMeters)); d > 1e-3 {
				t.Errorf("ComputeQualityMetrics length %.3f m, running total %.3f m", recomputed.TrackLengthMeters, track.TrackLengthMeters)
			}
		})
	}
}

func TestComputeQualityMetricsMeasuresBetweenObservedPoints(t *testing.T) {
	// Observed at x = 0, 1 and 6; coasted at 2, 3 and 7. The coasted points
	// at 2 and 3 strayed off the path, so walking the trail through them
	// overstates the distance, and the one at 7 was never confirmed.
	track := &TrackedObject{History: []TrackPoint{
		{X: 0, Support: SupportObserved},
		{X: 1, Support: SupportObserved},
		{X: 2, Y: 3, Support: SupportCoasted},
		{X: 3, Y: 3, Support: SupportOccludedInferred},
		{X: 6, Support: SupportObserved},
		{X: 7, Support: SupportMissedUnknown},
	}}
	track.ComputeQualityMetrics()
	if track.TrackLengthMeters != 6 {
		t.Errorf("length %v, want 6: 1 m observed, then the 5 m chord across the gap", track.TrackLengthMeters)
	}
}

func TestLengthAnchor(t *testing.T) {
	if _, ok := lengthAnchor(nil); ok {
		t.Error("an empty trail has an anchor")
	}
	trail := []TrackPoint{
		{X: 1, Support: SupportObserved},
		{X: 2, Support: SupportCoasted},
		{X: 3, Support: SupportOutOfFOV},
	}
	if p, ok := lengthAnchor(trail); !ok || p.X != 1 {
		t.Errorf("anchor %+v, want the last observed point", p)
	}
	// A restored trail carries no support record and is measured as before.
	if p, ok := lengthAnchor([]TrackPoint{{X: 4}, {X: 5}}); !ok || p.X != 5 {
		t.Errorf("anchor %+v, want the newest unrecorded point", p)
	}
	// The cap trimmed every observed point: the earliest retained point is
	// the nearest stand-in for the one lost.
	if p, ok := lengthAnchor(trail[1:]); !ok || p.X != 2 {
		t.Errorf("anchor %+v, want the earliest retained point", p)
	}
}
