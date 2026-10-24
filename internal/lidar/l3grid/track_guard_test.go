package l3grid

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
)

// The track guard's tests run one ring of 360 one-degree bins at elevation
// 0, so a return at azimuth a and range r lies at (r sin a, r cos a, 0). A
// road at 15 m settles in bins 0-9; a car at 13.5 m covers bins 3-6.

const (
	guardAzBins     = 360
	guardRoadMetres = 15.0
	guardCarMetres  = 13.5
)

var guardCarBins = []int{3, 4, 5, 6}

// newGuardManager is the shipped defaults' L3 with region overrides off and
// replay mode on, the given guard (nil is off), and the road settled over
// 100 frames.
func newGuardManager(t *testing.T, guard *TrackGuardParams) *BackgroundManager {
	t.Helper()
	params := BackgroundConfigFromActiveTuning(config.MustLoadDefaultConfig()).ToBackgroundParams()
	params.DisableRegionOverrides = true
	params.TrackGuard = guard
	bm := NewBackgroundManagerDI("track-guard", 1, guardAzBins, params, nil)
	if bm == nil {
		t.Fatal("NewBackgroundManagerDI returned nil")
	}
	bm.SetReplayMode(true)
	rng := rand.New(rand.NewSource(1))
	for f := 0; f < 100; f++ {
		pts := make([]PointPolar, 0, 10)
		for b := 0; b < 10; b++ {
			pts = append(pts, guardPoint(b, guardRoadMetres+rng.NormFloat64()*0.01))
		}
		if _, err := bm.ProcessFramePolarWithMaskAt(pts, guardFrameTime(f)); err != nil {
			t.Fatal(err)
		}
	}
	return bm
}

func guardFrameTime(frame int) time.Time {
	return maskReviewEpoch.Add(time.Duration(frame) * maskReviewFramePeriod)
}

func guardPoint(bin int, rangeMetres float64) PointPolar {
	return PointPolar{Channel: 1, Azimuth: float64(bin) + 0.5, Distance: rangeMetres}
}

func guardCarFrame() []PointPolar {
	pts := make([]PointPolar, 0, len(guardCarBins))
	for _, b := range guardCarBins {
		pts = append(pts, guardPoint(b, guardCarMetres))
	}
	return pts
}

// guardCarBox is a 3 m by 2 m box about the car, heading +X, so that it
// spans y in [12.45, 14.45]: the car and not the road behind it.
func guardCarBox() GuardBox {
	az := 5.0 * math.Pi / 180
	return GuardBox{X: guardCarMetres * math.Sin(az), Y: guardCarMetres * math.Cos(az), Z: 0.5,
		Length: 3, Width: 2, Height: 2}
}

func guardCells(bm *BackgroundManager) []BackgroundCell {
	return append([]BackgroundCell(nil), bm.Grid.Cells...)
}

func holdOnly(cooldown time.Duration) *TrackGuardParams {
	return &TrackGuardParams{HoldTracks: true, HoldCooldownNanos: cooldown.Nanoseconds()}
}

func TestTrackGuardHoldLeavesCellsUnchangedAndClassifiesAsUsual(t *testing.T) {
	control := newGuardManager(t, nil)
	guarded := newGuardManager(t, holdOnly(0))
	before := guardCells(guarded)

	for f := 100; f < 120; f++ {
		at := guardFrameTime(f)
		guarded.SetTrackHolds([]GuardBox{guardCarBox()}, at)
		want, err := control.ProcessFramePolarWithMaskAt(guardCarFrame(), at)
		if err != nil {
			t.Fatal(err)
		}
		want = append([]bool(nil), want...)
		got, err := guarded.ProcessFramePolarWithMaskAt(guardCarFrame(), at)
		if err != nil {
			t.Fatal(err)
		}
		if f == 100 {
			// Same model, same returns: the held frame is classed as the
			// unguarded one is.
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("frame %d return %d: held mask %v, unguarded %v", f, i, got[i], want[i])
				}
			}
		}
		held := guarded.TrackHeldMask()
		if len(held) != len(guardCarBins) {
			t.Fatalf("frame %d: held mask has %d entries, want %d", f, len(held), len(guardCarBins))
		}
		for i, h := range held {
			if !h {
				t.Fatalf("frame %d: return %d inside the box was not held", f, i)
			}
		}
	}

	after := guardCells(guarded)
	changed := guardCells(control)
	for _, b := range guardCarBins {
		if after[b] != before[b] {
			t.Errorf("held cell %d changed: before %+v after %+v", b, before[b], after[b])
		}
		if changed[b] == before[b] {
			t.Errorf("cell %d unchanged without the guard too, so the test shows nothing", b)
		}
	}
}

func TestTrackGuardHoldLeavesABackgroundLikeReturnUnlearned(t *testing.T) {
	// A return just in front of the road, inside the cell's window, is
	// background either way; held, the cell does not learn it.
	guarded := newGuardManager(t, holdOnly(0))
	before := guardCells(guarded)
	az := 5.0 * math.Pi / 180
	near := guardRoadMetres - 0.05
	box := GuardBox{X: near * math.Sin(az), Y: near * math.Cos(az), Z: 0.5, Length: 0.5, Width: 0.05, Height: 2}
	at := guardFrameTime(100)
	guarded.SetTrackHolds([]GuardBox{box}, at)
	mask, err := guarded.ProcessFramePolarWithMaskAt([]PointPolar{guardPoint(5, near)}, at)
	if err != nil {
		t.Fatal(err)
	}
	if mask[0] {
		t.Fatal("a return inside the cell's window was classed foreground")
	}
	if held := guarded.TrackHeldMask(); len(held) != 1 || !held[0] {
		t.Fatalf("held mask %v, want [true]", held)
	}
	if after := guardCells(guarded); after[5] != before[5] {
		t.Errorf("held cell learned the return: before %+v after %+v", before[5], after[5])
	}
}

func TestTrackGuardCooldownKeepsCellsUnchangedAfterTheBoxLeaves(t *testing.T) {
	guarded := newGuardManager(t, holdOnly(time.Second))
	before := guardCells(guarded)
	at := guardFrameTime(100)
	guarded.SetTrackHolds([]GuardBox{guardCarBox()}, at)
	if _, err := guarded.ProcessFramePolarWithMaskAt(guardCarFrame(), at); err != nil {
		t.Fatal(err)
	}
	// The box is gone; for the next second the cells stay as they were.
	for f := 101; f < 110; f++ {
		at := guardFrameTime(f)
		guarded.SetTrackHolds(nil, at)
		if _, err := guarded.ProcessFramePolarWithMaskAt(guardCarFrame(), at); err != nil {
			t.Fatal(err)
		}
		if held := guarded.TrackHeldMask(); held != nil {
			t.Fatalf("frame %d: no box, but held mask %v", f, held)
		}
		after := guardCells(guarded)
		for _, b := range guardCarBins {
			if after[b] != before[b] {
				t.Fatalf("frame %d: cell %d changed within its cool-down", f, b)
			}
		}
	}
	// Past the cool-down they follow their returns again.
	at = guardFrameTime(111)
	if _, err := guarded.ProcessFramePolarWithMaskAt(guardCarFrame(), at); err != nil {
		t.Fatal(err)
	}
	after := guardCells(guarded)
	for _, b := range guardCarBins {
		if after[b] == before[b] {
			t.Errorf("cell %d still unchanged after its cool-down", b)
		}
	}
}

func TestTrackGuardHoldsExpireAndFollowTheirVelocity(t *testing.T) {
	guarded := newGuardManager(t, holdOnly(0))
	given := guardFrameTime(100)

	// Moving at 5 m/s away from the sensor, the box is 1.5 m further out
	// 300 ms later: over the road at 15 m, off the car at 13.5 m.
	box := guardCarBox()
	box.VX, box.VY = 5*math.Sin(5*math.Pi/180), 5*math.Cos(5*math.Pi/180)
	guarded.SetTrackHolds([]GuardBox{box}, given)
	pts := []PointPolar{guardPoint(5, guardCarMetres), guardPoint(5, guardRoadMetres)}
	if _, err := guarded.ProcessFramePolarWithMaskAt(pts, given.Add(300*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	held := guarded.TrackHeldMask()
	if len(held) != 2 || held[0] || !held[1] {
		t.Fatalf("held mask %v, want [false true]: the box carried 1.5 m out", held)
	}

	// Older than holdMaxAgeNanos, the set guards nothing.
	guarded.SetTrackHolds([]GuardBox{guardCarBox()}, given)
	if _, err := guarded.ProcessFramePolarWithMaskAt(guardCarFrame(), given.Add(600*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if held := guarded.TrackHeldMask(); held != nil {
		t.Fatalf("a hold 600 ms old still held %v", held)
	}
	// Nor does a set from the future (a replay seeking back).
	guarded.SetTrackHolds([]GuardBox{guardCarBox()}, given.Add(time.Second))
	if _, err := guarded.ProcessFramePolarWithMaskAt(guardCarFrame(), given.Add(700*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if held := guarded.TrackHeldMask(); held != nil {
		t.Fatalf("a hold given after the frame held %v", held)
	}
}

func TestTrackGuardOffHoldsNothing(t *testing.T) {
	for name, guard := range map[string]*TrackGuardParams{
		"nil":           nil,
		"edge only":     {EdgeFollowCluster: true},
		"hold disabled": {HoldTracks: false, HoldCooldownNanos: int64(time.Second)},
	} {
		t.Run(name, func(t *testing.T) {
			control := newGuardManager(t, nil)
			bm := newGuardManager(t, guard)
			at := guardFrameTime(100)
			bm.SetTrackHolds([]GuardBox{guardCarBox()}, at)
			if _, err := bm.ProcessFramePolarWithMaskAt(guardCarFrame(), at); err != nil {
				t.Fatal(err)
			}
			if _, err := control.ProcessFramePolarWithMaskAt(guardCarFrame(), at); err != nil {
				t.Fatal(err)
			}
			if held := bm.TrackHeldMask(); held != nil {
				t.Fatalf("held mask %v with the hold off", held)
			}
			got, want := guardCells(bm), guardCells(control)
			for _, b := range guardCarBins {
				if got[b] != want[b] {
					t.Errorf("cell %d differs from the unguarded L3", b)
				}
			}
		})
	}
}

func TestTrackGuardResetGridEndsCooldowns(t *testing.T) {
	bm := newGuardManager(t, holdOnly(time.Minute))
	at := guardFrameTime(100)
	bm.SetTrackHolds([]GuardBox{guardCarBox()}, at)
	if _, err := bm.ProcessFramePolarWithMaskAt(guardCarFrame(), at); err != nil {
		t.Fatal(err)
	}
	cooling := 0
	for _, u := range bm.heldUntil {
		if u > 0 {
			cooling++
		}
	}
	if cooling != len(guardCarBins) {
		t.Fatalf("%d cells cooling down, want %d", cooling, len(guardCarBins))
	}
	if err := bm.ResetGrid(); err != nil {
		t.Fatal(err)
	}
	for i, u := range bm.heldUntil {
		if u != 0 {
			t.Fatalf("cell %d still cooling down after ResetGrid", i)
		}
	}
}

func TestFollowBoxesPublishesBackgroundInsideBoxesAsForeground(t *testing.T) {
	box := guardCarBox() // z in [-0.5, 1.5]
	points := []PointPolar{
		guardPoint(5, guardCarMetres),                             // inside, background: flipped
		guardPoint(4, guardCarMetres),                             // inside, already foreground
		{Channel: 1, Azimuth: 5.5, Distance: 13.5, Elevation: -5}, // under the base (z = -1.18)
		guardPoint(5, guardRoadMetres),                            // beyond the box
		guardPoint(40, guardCarMetres),                            // another azimuth
	}
	mask := []bool{false, true, false, false, false}
	if n := FollowBoxes(points, mask, []GuardBox{box}, 0.2); n != 1 {
		t.Fatalf("FollowBoxes changed %d returns, want 1", n)
	}
	want := []bool{true, true, false, false, false}
	for i := range want {
		if mask[i] != want[i] {
			t.Fatalf("mask %v, want %v", mask, want)
		}
	}
	// The margin reaches the road 0.55 m beyond the box's far side.
	if n := FollowBoxes(points, mask, []GuardBox{box}, 0.6); n != 1 || !mask[3] {
		t.Fatalf("a 0.6 m margin changed %d returns, mask %v", n, mask)
	}
	if n := FollowBoxes(points, mask[:2], []GuardBox{box}, 0); n != 0 {
		t.Fatal("FollowBoxes wrote a mask not indexed as its points")
	}
	if n := FollowBoxes(points, mask, nil, 0); n != 0 {
		t.Fatal("FollowBoxes changed returns with no boxes")
	}
}

func TestPublishedMaskCopiesTheHeldReturnsUnderEdgeFollow(t *testing.T) {
	off := newGuardManager(t, holdOnly(0))
	mask := []bool{false, false}
	if got := off.PublishedMask(mask, nil); &got[0] != &mask[0] {
		t.Fatal("without edge follow PublishedMask must return the mask itself")
	}

	bm := newGuardManager(t, &TrackGuardParams{HoldTracks: true, EdgeFollowCluster: true})
	at := guardFrameTime(100)
	bm.SetTrackHolds([]GuardBox{guardCarBox()}, at)
	pts := []PointPolar{guardPoint(5, guardCarMetres), guardPoint(8, guardRoadMetres)}
	l3, err := bm.ProcessFramePolarWithMaskAt(pts, at)
	if err != nil {
		t.Fatal(err)
	}
	l3 = append([]bool(nil), l3...)
	l3[0] = false // as if the cell's window had admitted the car
	buf := make([]bool, 0, 8)
	got := bm.PublishedMask(l3, buf)
	if &got[0] == &l3[0] {
		t.Fatal("PublishedMask must not write L3's mask")
	}
	if !got[0] || got[1] {
		t.Fatalf("published %v, want [true false]", got)
	}
	if l3[0] {
		t.Fatal("PublishedMask changed L3's mask")
	}
}

func TestTrackGuardParamsTuningRoundTrip(t *testing.T) {
	if TrackGuardParamsFromTuning(nil) != nil || TrackGuardTuningFromParams(nil) != nil {
		t.Fatal("a nil block must be a nil guard and back")
	}
	block := &config.L3TrackGuard{HoldTracks: true, HoldMarginMetres: 0.3, HoldCooldown: "1.5s",
		HoldMinSpeedMps: 1, EdgeFollowCluster: true, EdgeMarginMetres: 0.2}
	p := TrackGuardParamsFromTuning(block)
	if p.HoldCooldownNanos != int64(1500*time.Millisecond) || !p.HoldTracks || !p.EdgeFollowCluster ||
		p.HoldMarginMetres != 0.3 || p.EdgeMarginMetres != 0.2 || p.HoldMinSpeedMps != 1 {
		t.Fatalf("params %+v from %+v", p, block)
	}
	if back := TrackGuardTuningFromParams(p); *back != *block {
		t.Fatalf("round trip %+v, want %+v", back, block)
	}
	if bad := TrackGuardParamsFromTuning(&config.L3TrackGuard{HoldCooldown: "soon"}); bad.HoldCooldownNanos != 0 {
		t.Fatalf("an unparsable cool-down read as %d ns", bad.HoldCooldownNanos)
	}
	// The defaults carry the guard on, and ToBackgroundParams copies it.
	cfg := BackgroundConfigFromActiveTuning(config.MustLoadDefaultConfig())
	if cfg.TrackGuard == nil || !cfg.TrackGuard.HoldTracks || !cfg.TrackGuard.EdgeFollowCluster {
		t.Fatalf("defaults' guard %+v, want hold and edge follow on", cfg.TrackGuard)
	}
	params := cfg.ToBackgroundParams()
	if params.TrackGuard == cfg.TrackGuard || *params.TrackGuard != *cfg.TrackGuard {
		t.Fatal("ToBackgroundParams must copy the guard, not share it")
	}
}

func TestSnapshotRangesMatchesTheSnapshot(t *testing.T) {
	bm := newGuardManager(t, nil)
	if err := bm.SetRingElevations([]float64{0}); err != nil {
		t.Fatal(err)
	}
	snap, err := bm.GenerateBackgroundSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	ranges := bm.SnapshotRanges()
	if len(ranges) != guardAzBins {
		t.Fatalf("%d ranges, want one per cell (%d)", len(ranges), guardAzBins)
	}
	n := 0
	for i, r := range ranges {
		if r > 0 {
			n++
			if math.Abs(float64(r)-guardRoadMetres) > 0.1 {
				t.Errorf("cell %d range %g, want about %g", i, r, guardRoadMetres)
			}
		}
	}
	if n != len(snap.X) || n != 10 {
		t.Fatalf("%d settled ranges, snapshot has %d points, want 10", n, len(snap.X))
	}
	var nilBM *BackgroundManager
	if nilBM.SnapshotRanges() != nil {
		t.Fatal("a nil manager has no ranges")
	}
}
