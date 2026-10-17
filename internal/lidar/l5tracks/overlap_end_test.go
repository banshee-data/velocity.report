package l5tracks

import (
	"math"
	"testing"
	"time"
)

// pairCluster is a car-sized cluster at (x, y), its PCA box along x.
func pairCluster(id int64, x, y float64) WorldCluster {
	c := carCluster(id, x, y, 0)
	return c
}

// runPair drives two confirmed tracks on overlapping clusters moving
// together along x, drops the second cluster after frame drop, and returns
// the second track after frame end.
func runPair(t *testing.T, cfg TrackerConfig, lead, lateral float64, drop, end int) *TrackedObject {
	t.Helper()
	tracker := NewTracker(cfg)
	start := time.Unix(1_700_000_000, 0)
	var second string
	for i := 0; i <= end; i++ {
		x := 10 + 0.5*float64(i)
		clusters := []WorldCluster{pairCluster(1, x, 0)}
		if i < drop {
			clusters = append(clusters, pairCluster(2, x+lead, lateral))
		}
		tracker.Update(clusters, start.Add(time.Duration(i)*100*time.Millisecond))
		if i == drop-1 {
			for id, tr := range tracker.Tracks {
				if tr.TrackState == TrackConfirmed && tr.LastClusterID == 2 {
					second = id
				}
			}
			if second == "" {
				t.Fatal("no confirmed track on the second cluster")
			}
		}
	}
	return tracker.Tracks[second]
}

func TestOverlapEndEndsACoastingDuplicate(t *testing.T) {
	// The box model, as in the defaults, draws the car its 3.8 m floor; the
	// rear-face clusters alone would give the legacy OBB no width.
	cfg := DefaultTrackerConfig()
	if !cfg.BoxModel.Enabled {
		t.Fatal("the defaults have no box model")
	}
	// Off: the second track coasts on the first for its miss budget.
	if tr := runPair(t, cfg, 1.0, 0, 12, 15); tr == nil || tr.TrackState == TrackDeleted {
		t.Fatalf("without the option the coasting duplicate ended: %+v", tr)
	}
	cfg.OverlapEndFrames = 1
	tr := runPair(t, cfg, 1.0, 0, 12, 13)
	if tr == nil || tr.TrackState != TrackDeleted || tr.ExpiryReason != ExpiryOverlapped {
		t.Fatalf("the coasting duplicate was not ended as overlapped: %+v", tr)
	}
	if ExpiryOverlapped.String() != "overlapped" {
		t.Errorf("token %q", ExpiryOverlapped.String())
	}
}

func TestOverlapEndCountsTheCoastsFrames(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.OverlapEndFrames = 3
	if tr := runPair(t, cfg, 1.0, 0, 12, 13); tr == nil || tr.TrackState == TrackDeleted {
		t.Fatal("ended after two overlapping coasting frames, before its third")
	}
	if tr := runPair(t, cfg, 1.0, 0, 12, 14); tr == nil || tr.TrackState != TrackDeleted || tr.ExpiryReason != ExpiryOverlapped {
		t.Fatalf("not ended on its third overlapping coasting frame: %+v", tr)
	}
}

func TestOverlapEndSparesANeighbour(t *testing.T) {
	// A car in the next lane, 3 m across, coasting beside a live one: its
	// box does not lie on the live track's, so it keeps its miss budget.
	cfg := DefaultTrackerConfig()
	cfg.OverlapEndFrames = 1
	if tr := runPair(t, cfg, 0, 3.0, 12, 14); tr == nil || tr.TrackState == TrackDeleted {
		t.Fatalf("a neighbour coasting beside a live track was ended: %+v", tr)
	}
}

func TestRectOverlapShare(t *testing.T) {
	a := drawnRect{x: 0, y: 0, psi: 0, length: 4, wd: 2}
	for _, c := range []struct {
		b    drawnRect
		want float64
	}{
		{a, 1},
		{drawnRect{x: 2, length: 4, wd: 2}, 0.5},
		{drawnRect{x: 5, length: 4, wd: 2}, 0},
		{drawnRect{psi: math.Pi / 2, length: 4, wd: 2}, 0.5},
	} {
		if got := rectOverlapShare(a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("overlap with %+v = %.4f, want %.4f", c.b, got, c.want)
		}
	}
}
