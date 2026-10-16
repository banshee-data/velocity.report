package l5tracks

import (
	"encoding/json"
	"math"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"
)

// stationaryRun drives a single body along +X at speed (m/s) from x = 5 at
// 10 Hz from capture time 1 s, for the given number of frames, and calls
// observe after each frame with the frame time.
func stationaryRun(t *testing.T, speed float32, frames int, observe func(tk *Tracker, at time.Time)) *Tracker {
	t.Helper()
	tk := NewTracker(DefaultTrackerConfig())
	for i := 0; i < frames; i++ {
		secs := 1 + 0.1*float64(i)
		x := 5 + speed*0.1*float32(i)
		at := tdAt(secs)
		tk.Update([]WorldCluster{ccCluster(x, 2, 4.5, 1.8)}, at)
		if observe != nil {
			observe(tk, at)
		}
	}
	return tk
}

// A body that never moves is in one stationary run from its birth, and the
// confirmed-track snapshot the pipeline reads answers as the live track does.
func TestStationaryFor_StillBodyCountsFromBirth(t *testing.T) {
	const frames = 250
	tk := stationaryRun(t, 0, frames, nil)
	at := tdAt(1 + 0.1*float64(frames-1))
	live := soleActive(t, tk)
	want := at.Sub(tdAt(1))
	if got := live.StationaryFor(at.UnixNano()); got != want {
		t.Errorf("live track stationary for %v, want %v since birth", got, want)
	}
	confirmed := tk.GetConfirmedTracks()
	if len(confirmed) != 1 {
		t.Fatalf("%d confirmed tracks, want 1", len(confirmed))
	}
	if got := confirmed[0].StationaryFor(at.UnixNano()); got != want {
		t.Errorf("snapshot stationary for %v, want %v", got, want)
	}
	if got := live.StationaryFor(tdAt(0.5).UnixNano()); got != 0 {
		t.Errorf("stationary for %v at a time before the run began, want 0", got)
	}
}

// A body moving above StationarySpeedMps is never in a run once the filter
// has its velocity.
func TestStationaryFor_MovingBodyIsNotStationary(t *testing.T) {
	stationaryRun(t, 2, 100, func(tk *Tracker, at time.Time) {
		for _, track := range tk.Tracks {
			if track.ObservationCount >= 10 && track.StationaryFor(at.UnixNano()) > 0 {
				t.Fatalf("a body at 2 m/s has stood still for %v at speed %.2f m/s", track.StationaryFor(at.UnixNano()), track.Speed())
			}
		}
	})
}

// A creep under StationarySpeedMps is not a dwell: the run restarts each time
// the body passes StationaryDriftMetres from where it began, so it never
// approaches the dwell policy's 20 s.
func TestStationaryFor_CreepRestartsTheRun(t *testing.T) {
	const creep = 0.3
	longest := time.Duration(0)
	slowest := float32(1)
	stationaryRun(t, creep, 300, func(tk *Tracker, at time.Time) {
		for _, track := range tk.Tracks {
			if track.ObservationCount < 50 {
				continue
			}
			longest = max(longest, track.StationaryFor(at.UnixNano()))
			slowest = min(slowest, track.Speed())
		}
	})
	if slowest > StationarySpeedMps {
		t.Fatalf("premise: the creep's filtered speed reached %.2f m/s, above %.1f, so speed alone would end its runs", slowest, StationarySpeedMps)
	}
	// One metre at 0.3 m/s is 3.3 s; allow the filter's lag behind the body.
	if limit := 5 * time.Second; longest > limit {
		t.Errorf("a %.1f m/s creep stayed in one run for %v, want under %v", creep, longest, limit)
	}
}

// stationaryPath tracks a single body through positions (pos(i), 2) at 10 Hz
// from capture time 1 s under the given drift rule, and calls observe after
// each frame with the sole live track and the frame time.
func stationaryPath(t *testing.T, rule StationaryRule, frames int, pos func(i int) float32,
	observe func(track *TrackedObject, at time.Time)) {
	t.Helper()
	cfg := DefaultTrackerConfig()
	cfg.Stationary = rule
	tk := NewTracker(cfg)
	for i := 0; i < frames; i++ {
		at := tdAt(1 + 0.1*float64(i))
		tk.Update([]WorldCluster{ccCluster(pos(i), 2, 4.5, 1.8)}, at)
		observe(soleActive(t, tk), at)
	}
}

var stationaryRules = []struct {
	name string
	rule StationaryRule
}{
	{"first estimate, 1 m", StationaryRule{}},
	{"first estimate, 2 m", StationaryRule{DriftMetres: 2}},
	{"running median, 1 m", StationaryRule{Anchor: StationaryAnchorMedian}},
}

// A still body whose estimate wanders ±0.7 m, slowly enough to stay under
// StationarySpeedMps, starting at one extreme: from its first estimate the
// other extreme is 1.4 m away, so the shipped 1 m rule restarts the run on
// the first swing, and keeps restarting. A 2 m tolerance, or 1 m from the
// run's running median, holds one run from birth.
func TestStationaryFor_WanderHoldsUnderTheWiderRules(t *testing.T) {
	const frames = 500
	wander := func(i int) float32 { return 10 + 0.7*float32(math.Cos(2*math.Pi*float64(i)/200)) }
	for _, c := range stationaryRules {
		t.Run(c.name, func(t *testing.T) {
			var firstRestart, longest time.Duration
			lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
			birth := tdAt(1)
			stationaryPath(t, c.rule, frames, wander, func(track *TrackedObject, at time.Time) {
				if track.Speed() > StationarySpeedMps {
					t.Fatalf("premise: speed %.2f m/s over %.1f at %v", track.Speed(), StationarySpeedMps, at.Sub(birth))
				}
				lo, hi = min(lo, track.X), max(hi, track.X)
				held := track.StationaryFor(at.UnixNano())
				longest = max(longest, held)
				if firstRestart == 0 && held < at.Sub(birth) {
					firstRestart = at.Sub(birth)
				}
			})
			if lo > 9.35 || hi < 10.65 {
				t.Fatalf("premise: the estimate spans [%.2f, %.2f], not the ±0.7 m wander", lo, hi)
			}
			if c.rule == (StationaryRule{}) {
				if firstRestart == 0 || firstRestart > 10*time.Second || longest >= 20*time.Second {
					t.Errorf("first restart at %v, longest run %v; want a restart on the first swing and no run near 20 s",
						firstRestart, longest)
				}
				return
			}
			if want := tdAt(1 + 0.1*(frames-1)).Sub(birth); firstRestart != 0 || longest != want {
				t.Errorf("first restart at %v, longest run %v; want one run of %v from birth", firstRestart, longest, want)
			}
		})
	}
}

// A creep of 3 m over 50 s, at 0.06 m/s, restarts the run under every rule
// before 40 s. The median trails a steady creep by half the run, so 1 m from
// it restarts after about 2 m, as 2 m from the first estimate does. A median
// over only the last 200 frames would trail by at most 0.6 m here and hold
// one run throughout, which is why StationaryMedianWindowFrames is 400.
func TestStationaryFor_CreepRestartsUnderEveryRule(t *testing.T) {
	const frames = 500
	creep := func(i int) float32 { return 10 + 3*float32(i)/frames }
	for _, c := range stationaryRules {
		t.Run(c.name, func(t *testing.T) {
			var longest time.Duration
			restarts := 0
			var since int64
			stationaryPath(t, c.rule, frames, creep, func(track *TrackedObject, at time.Time) {
				if track.Speed() > StationarySpeedMps {
					t.Fatalf("premise: speed %.2f m/s over %.1f", track.Speed(), StationarySpeedMps)
				}
				if since != 0 && track.stationaryRun && track.stationarySinceNanos != since {
					restarts++
				}
				since = track.stationarySinceNanos
				longest = max(longest, track.StationaryFor(at.UnixNano()))
			})
			if restarts == 0 || longest >= 40*time.Second {
				t.Errorf("%d restarts, longest run %v; want the creep to restart its run short of 40 s", restarts, longest)
			}
		})
	}
}

// The shipped rule keeps no window: a run from the first estimate costs the
// track nothing it did not cost before the median existed.
func TestStationaryFor_ShippedRuleKeepsNoWindow(t *testing.T) {
	tk := stationaryRun(t, 0, 50, nil)
	if w := soleActive(t, tk).stationaryWindow; w != nil {
		t.Errorf("the shipped rule allocated a median window: %+v", w)
	}
}

// The window's median is the per-axis median of the last capacity values
// pushed since its reset, duplicates and signed zeros included, however many
// were pushed before.
func TestStationaryWindow_MedianOfTheLastPushes(t *testing.T) {
	const capacity = 5
	w := newStationaryWindow(capacity)
	rng := rand.New(rand.NewSource(3))
	var xs, ys []float32
	pick := []float32{-1, 0, float32(math.Copysign(0, -1)), 0.5, 2, 2}
	for i := 0; i < 60; i++ {
		x := pick[rng.Intn(len(pick))]
		y := float32(rng.NormFloat64())
		w.push(x, y)
		xs, ys = append(xs, x), append(ys, y)
		if i == 30 {
			w.reset()
			xs, ys = xs[:0], ys[:0]
			continue
		}
		gotX, gotY := w.median()
		wantX, wantY := bruteMedian(xs, capacity), bruteMedian(ys, capacity)
		if gotX != wantX || gotY != wantY {
			t.Fatalf("push %d: median (%v, %v), want (%v, %v) over the last %d", i, gotX, gotY, wantX, wantY, capacity)
		}
		if n := min(len(xs), capacity); len(w.sortX) != n || len(w.sortY) != n || len(w.xs) != n {
			t.Fatalf("push %d: window holds %d/%d/%d values, want %d", i, len(w.xs), len(w.sortX), len(w.sortY), n)
		}
	}
}

func bruteMedian(values []float32, capacity int) float32 {
	last := append([]float32{}, values[max(0, len(values)-capacity):]...)
	sort.Slice(last, func(i, j int) bool { return last[i] < last[j] })
	return sortedMedian(last)
}

// The rule's zero value is the shipped rule, and Validate refuses what the
// clock would otherwise run as something else.
func TestStationaryRule_DefaultsAndValidate(t *testing.T) {
	var zero StationaryRule
	if zero.Drift() != StationaryDriftMetres || zero.AnchorMode() != StationaryAnchorFirst {
		t.Errorf("zero rule runs %g m from %q", zero.Drift(), zero.AnchorMode())
	}
	for _, ok := range []StationaryRule{{}, {DriftMetres: 2}, {Anchor: StationaryAnchorMedian}, {Anchor: StationaryAnchorFirst}} {
		if err := ok.Validate(); err != nil {
			t.Errorf("%+v refused: %v", ok, err)
		}
	}
	for _, bad := range []StationaryRule{{Anchor: "mean"}, {DriftMetres: -1}, {DriftMetres: float32(math.NaN())},
		{DriftMetres: float32(math.Inf(1))}} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

// EndAbsorbed ends a live track with its own reason, once; the deleted track
// is never associated again, and the scoring record counts it apart.
func TestEndAbsorbed_DeletesOnceWithItsReason(t *testing.T) {
	tk, track, now := confirmedTrack(t, DefaultTrackerConfig(), 10, 2, 4.5, 1.8)
	id := track.TrackID
	at := tdAt(now)
	if !tk.EndAbsorbed(id, at) {
		t.Fatal("EndAbsorbed refused a confirmed track")
	}
	if track.TrackState != TrackDeleted || track.ExpiryReason != ExpiryAbsorbed || track.Existence != ExistenceExpired {
		t.Fatalf("after EndAbsorbed: state %s reason %s existence %v", track.TrackState, track.ExpiryReason, track.Existence)
	}
	if track.EndUnixNanos != at.UnixNano() {
		t.Errorf("end %d, want the absorption time %d", track.EndUnixNanos, at.UnixNano())
	}
	if tk.EndAbsorbed(id, at) {
		t.Error("EndAbsorbed ended a deleted track a second time")
	}
	if tk.EndAbsorbed("trk_unknown", at) {
		t.Error("EndAbsorbed ended an unknown track")
	}
	if got := tk.ContinuityStats().ExpiredByReason.Absorbed; got != 1 {
		t.Errorf("absorbed expiries %d, want 1", got)
	}
	// The same body seen again starts a new track; the absorbed one stays
	// deleted.
	tk.Update([]WorldCluster{ccCluster(10, 2, 4.5, 1.8)}, tdAt(now+0.1))
	if track.TrackState != TrackDeleted {
		t.Error("the absorbed track was revived")
	}
	if fresh := soleActive(t, tk); fresh.TrackID == id {
		t.Error("the next cluster was associated with the absorbed track")
	}
	if ExpiryAbsorbed.String() != "absorbed" {
		t.Errorf("ExpiryAbsorbed token %q", ExpiryAbsorbed.String())
	}
}

// The absorbed count is omitted at zero, so a run without the dwell policy
// writes the expiry record it always wrote.
func TestExpiryCountsOmitAbsorbedAtZero(t *testing.T) {
	zero, err := json.Marshal(ExpiryCounts{Misses: 3})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(zero), "absorbed") {
		t.Errorf("zero absorbed expiries were written: %s", zero)
	}
	one, err := json.Marshal(ExpiryCounts{Absorbed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(one), `"absorbed":1`) {
		t.Errorf("an absorbed expiry was not written: %s", one)
	}
}
