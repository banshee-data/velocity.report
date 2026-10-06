package l3grid

import (
	"os"
	"regexp"
	"testing"
	"time"
)

// The settling decision was made in two places — the background update path and
// the foreground extraction path — with two copies of the same logic. They
// drifted: convergence was added to one, and the live pipeline runs the other,
// so a feature that measurably worked was never reached and the grid took its
// full thirty seconds with nothing in the log to explain it.
//
// These guard the single decision that replaced them.

// Neither settling path may reimplement the decision. A second copy is how the
// convergence check came to be unreachable in the first place.
func TestSettlingDecisionHasOneImplementation(t *testing.T) {
	// The frames/duration test is the signature of the decision itself.
	pattern := regexp.MustCompile(`WarmupDurationNanos <= 0 \|\|`)

	for _, path := range []string{"foreground.go", "background_manager.go"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if pattern.Match(src) {
			t.Errorf("%s decides settling itself instead of calling settlingCompleteLocked; "+
				"a second copy is how convergence became unreachable in the live path", path)
		}
	}
}

// Each test gets its own sensor ID: managers are held in a package-level
// registry keyed by it, so a shared name leaks state between tests.
func newSettlingGrid(t *testing.T, params BackgroundParams) *BackgroundManager {
	t.Helper()
	bm := NewBackgroundManagerDI(t.Name(), 4, 8, params, nil)
	// A fixed minute already elapsed. Anchoring to time.Now() left the elapsed
	// time at zero whenever both calls landed in the same clock tick, which
	// made "the duration has passed" depend on scheduling.
	bm.StartTime = time.Now().Add(-time.Minute)
	return bm
}

// The frame minimum gates everything, convergence included.
func TestSettlingWaitsForTheFrameMinimum(t *testing.T) {
	bm := newSettlingGrid(t, BackgroundParams{
		SeedFromFirstObservation: true,
		WarmupMinFrames:          10,
		WarmupDurationNanos:      int64(time.Second), // long since elapsed
	})
	bm.Grid.mu.Lock()
	bm.Grid.WarmupFramesRemaining = 5
	done := bm.settlingCompleteLocked(time.Now().UnixNano())
	bm.Grid.mu.Unlock()

	if done {
		t.Error("settled with frames still outstanding")
	}
}

// With both met, settling completes — the behaviour that always worked.
func TestSettlingCompletesOnFramesAndDuration(t *testing.T) {
	bm := newSettlingGrid(t, BackgroundParams{
		SeedFromFirstObservation: true,
		WarmupMinFrames:          10,
		WarmupDurationNanos:      int64(time.Second),
	})
	bm.Grid.mu.Lock()
	bm.Grid.WarmupFramesRemaining = 0
	done := bm.settlingCompleteLocked(time.Now().UnixNano())
	bm.Grid.mu.Unlock()

	if !done {
		t.Error("did not settle with both the frame minimum and the duration met")
	}
}

// Without convergence configured the duration is the whole wait, unchanged.
func TestSettlingWaitsOutTheDurationWithoutConvergence(t *testing.T) {
	bm := newSettlingGrid(t, BackgroundParams{
		SeedFromFirstObservation: true,
		WarmupMinFrames:          10,
		WarmupDurationNanos:      int64(time.Hour),
	})
	bm.Grid.mu.Lock()
	bm.Grid.WarmupFramesRemaining = 0
	done := bm.settlingCompleteLocked(time.Now().UnixNano())
	bm.Grid.mu.Unlock()

	if done {
		t.Error("settled before the duration with no convergence criteria configured")
	}
}

// The plan is announced once, not once per frame.
func TestSettlingPlanIsAnnouncedOnce(t *testing.T) {
	bm := newSettlingGrid(t, BackgroundParams{
		SeedFromFirstObservation: true,
		WarmupMinFrames:          10,
		WarmupDurationNanos:      int64(time.Hour),
	})

	bm.Grid.mu.Lock()
	defer bm.Grid.mu.Unlock()

	bm.settlingCompleteLocked(time.Now().UnixNano())
	if !bm.Grid.settlingAnnounced {
		t.Fatal("the settling plan was never announced")
	}
	// A second call must not re-announce.
	bm.Grid.settlingAnnounced = false
	bm.settlingCompleteLocked(time.Now().UnixNano())
	if !bm.Grid.settlingAnnounced {
		t.Error("announcement flag not set on a later call")
	}
}
