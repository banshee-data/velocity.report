package l5tracks

import (
	"encoding/json"
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
