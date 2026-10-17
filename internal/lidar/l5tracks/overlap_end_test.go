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

func TestOverlapEndOnBodyNeedsThePredictionOnTheReturns(t *testing.T) {
	// The coasting track's drawn box lies over half on the live one's, but
	// its predicted position is a metre beyond the live body's returns: two
	// boxes overlapping, not one body.
	tracker := NewTracker(DefaultTrackerConfig())
	live := &TrackedObject{TrackID: "live", BoxX: 10, BoxY: 0, BoxLength: 4.5, BoxWidth: 1.8}
	live.TrackState = TrackConfirmed
	coast := &TrackedObject{TrackID: "coast", X: 12, Y: 0, BoxX: 11, BoxY: 0, BoxLength: 4.5, BoxWidth: 1.8}
	tracker.Tracks = map[string]*TrackedObject{"live": live, "coast": coast}
	body := []drawnRect{{id: "live", x: 9, y: 0, psi: 0, length: 1 + 2*overlapEndBodyMargin, wd: 1.8 + 2*overlapEndBodyMargin}}
	boxes := tracker.liveConfirmedRects(map[string]bool{"live": true})
	if _, hit := tracker.coastsOn(coast, boxes); !hit {
		t.Fatal("the drawn boxes overlap by more than half; the box test should hit")
	}
	tracker.Config.OverlapEndOnBody = true
	if _, hit := tracker.coastsOn(coast, body); hit {
		t.Fatal("a prediction a metre beyond the live returns was taken for the same body")
	}
	coast.X = 9.2
	if on, hit := tracker.coastsOn(coast, body); !hit || on != "live" {
		t.Fatal("a prediction on the live returns was not taken for the same body")
	}
}

// runBoth drives tracks on clusters 1 and 2, lead apart along x, keeps only
// cluster 2 from frame drop on, and returns the tracks that held 1 and 2.
func runBoth(t *testing.T, cfg TrackerConfig, lead float64, drop, end int) (*TrackedObject, *TrackedObject) {
	t.Helper()
	tracker := NewTracker(cfg)
	start := time.Unix(1_700_000_000, 0)
	var one, two string
	for i := 0; i <= end; i++ {
		x := 10 + 0.5*float64(i)
		clusters := []WorldCluster{pairCluster(2, x+lead, 0)}
		if i < drop {
			clusters = append([]WorldCluster{pairCluster(1, x, 0)}, clusters...)
		}
		tracker.Update(clusters, start.Add(time.Duration(i)*100*time.Millisecond))
		if i == drop-1 {
			for id, tr := range tracker.Tracks {
				if tr.TrackState != TrackConfirmed {
					continue
				}
				switch tr.LastClusterID {
				case 1:
					one = id
				case 2:
					two = id
				}
			}
			if one == "" || two == "" {
				t.Fatalf("setup: no confirmed track on each cluster (%q, %q)", one, two)
			}
		}
	}
	return tracker.Tracks[one], tracker.Tracks[two]
}

func TestOverlapEndOnBodyEndsADuplicateOnTheSameReturns(t *testing.T) {
	// Two tracks on the same returns, as an alternating pair is, lose one
	// cluster: whichever coasts lies on the other's returns and is ended.
	cfg := DefaultTrackerConfig()
	cfg.OverlapEndFrames, cfg.OverlapEndOnBody = 1, true
	a, b := runBoth(t, cfg, 0.05, 12, 13)
	ended, alive := 0, 0
	for _, tr := range []*TrackedObject{a, b} {
		switch {
		case tr.TrackState == TrackDeleted && tr.ExpiryReason == ExpiryOverlapped:
			ended++
		case tr.TrackState != TrackDeleted:
			alive++
		}
	}
	if ended != 1 || alive != 1 {
		t.Fatalf("want one ended as overlapped and one alive, got %d and %d", ended, alive)
	}
}

func TestOverlapEndKeepOlderEndsTheYoungerTrack(t *testing.T) {
	// Whichever of the two coasts, keeping the older ends the younger, and
	// the older holds the remaining cluster.
	cfg := DefaultTrackerConfig()
	cfg.OverlapEndFrames, cfg.OverlapEndOnBody, cfg.OverlapEndKeepOlder = 1, true, true
	a, b := runBoth(t, cfg, 0.05, 12, 14)
	older, younger := a, b
	if b.CreationSequence < a.CreationSequence {
		older, younger = b, a
	}
	if younger.TrackState != TrackDeleted || younger.ExpiryReason != ExpiryOverlapped {
		t.Fatalf("the younger track was not ended as overlapped: state %v reason %v", younger.TrackState, younger.ExpiryReason)
	}
	if older.TrackState == TrackDeleted || older.LastClusterID != 2 || older.Misses != 0 {
		t.Fatalf("the older track does not hold the cluster: state %v cluster %d misses %d", older.TrackState, older.LastClusterID, older.Misses)
	}
}
