package l3grid

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
)

const refineAzBins = 360

func newRefineManager(t *testing.T, rings int, refine bool) *BackgroundManager {
	t.Helper()
	params := BackgroundConfigFromActiveTuning(config.MustLoadDefaultConfig()).ToBackgroundParams()
	params.DisableRegionOverrides = true
	params.ForegroundRefine = nil
	bm := NewBackgroundManagerDI("refine", rings, refineAzBins, params, nil)
	if bm == nil {
		t.Fatal("NewBackgroundManagerDI returned nil")
	}
	bm.SetReplayMode(true)
	if refine {
		p := refineParams()
		params.ForegroundRefine = &p
		if err := bm.SetParams(params); err != nil {
			t.Fatal(err)
		}
	}
	return bm
}

// refineParams is the parameter set foreground-refine.json runs.
func refineParams() ForegroundRefineParams {
	cfg, err := config.LoadTuningConfig("../../../config/experiments/foreground-refine.json")
	if err != nil {
		panic(err)
	}
	return *ForegroundRefineParamsFromTuning(cfg.L3.ActiveCommon().ForegroundRefine)
}

var refineEpoch = time.Date(2026, time.October, 31, 0, 0, 0, 0, time.UTC)

// refineWall is one return per cell of the block at the given range.
func refineBlock(rings []int, firstBin, lastBin int, r float64) []PointPolar {
	var pts []PointPolar
	for _, ch := range rings {
		for bin := firstBin; bin <= lastBin; bin++ {
			pts = append(pts, fgPoint(refineAzBins, ch, bin, r))
		}
	}
	return pts
}

// settle feeds the frame until the grid has learned it, returning the frame
// count it used.
func settle(t *testing.T, bm *BackgroundManager, pts []PointPolar, frames int) int {
	t.Helper()
	for i := 0; i < frames; i++ {
		if _, err := bm.ProcessFramePolarWithMaskAt(pts, refineEpoch.Add(time.Duration(i)*100*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	return frames
}

func maskAt(t *testing.T, bm *BackgroundManager, pts []PointPolar, frame int) []bool {
	t.Helper()
	mask, err := bm.ProcessFramePolarWithMaskAt(pts, refineEpoch.Add(time.Duration(frame)*100*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	return append([]bool(nil), mask...)
}

func countTrue(mask []bool) int {
	n := 0
	for _, m := range mask {
		if m {
			n++
		}
	}
	return n
}

// A vehicle that stood through settling leaves: the road behind it reads as
// foreground under a two-sided window, until the cell catches up. With
// refinement it is background from the first return and the cell relearns.
func TestForegroundRefine_RevealedSurfaceIsBackground(t *testing.T) {
	rings := []int{1, 2, 3}
	stood := refineBlock(rings, 10, 30, 12)
	road := refineBlock(rings, 10, 30, 30)

	plain := newRefineManager(t, 3, false)
	n := settle(t, plain, stood, 150)
	if got := countTrue(maskAt(t, plain, road, n)); got != len(road) {
		t.Fatalf("control: two-sided window kept %d of %d revealed returns foreground, want all", got, len(road))
	}

	bm := newRefineManager(t, 3, true)
	n = settle(t, bm, stood, 150)
	if got := countTrue(maskAt(t, bm, road, n)); got != 0 {
		t.Fatalf("revealed surface: %d returns foreground, want 0", got)
	}
	for i := 1; i < bm.Grid.Params.ForegroundRefine.FarRelearnReturns; i++ {
		maskAt(t, bm, road, n+i)
	}
	cell := bm.Grid.Cells[bm.Grid.Idx(1, 20)]
	if cell.AverageRangeMeters != 30 || cell.LockedBaseline != 0 || cell.TimesSeenCount != bm.Grid.Params.ForegroundRefine.FarRelearnSeen {
		t.Fatalf("cell did not relearn: %+v", cell)
	}
	if env := bm.refine.nearEnvelope[bm.Grid.Idx(1, 20)]; env != 30 {
		t.Fatalf("envelope stayed at %v after the surface moved to 30", env)
	}
	// The surface the cell now holds is an ordinary one: something in front of
	// it is foreground.
	if got := countTrue(maskAt(t, bm, refineBlock(rings, 10, 30, 12), n+50)); got == 0 {
		t.Fatal("an object in front of the relearned surface was not foreground")
	}
}

// Far returns at scattered ranges are a cell seeing through something, not a
// new surface: they stay background and the cell keeps what it learned.
func TestForegroundRefine_DisagreeingFarReturnsDoNotRelearn(t *testing.T) {
	bm := newRefineManager(t, 3, true)
	wall := refineBlock([]int{2}, 20, 22, 10)
	n := settle(t, bm, wall, 150)
	for i := 0; i < 40; i++ {
		far := refineBlock([]int{2}, 20, 22, 25+3*float64(i%5))
		if got := countTrue(maskAt(t, bm, far, n+i)); got != 0 {
			t.Fatalf("frame %d: %d far returns foreground", i, got)
		}
	}
	if got := bm.Grid.Cells[bm.Grid.Idx(1, 21)].AverageRangeMeters; got > 10.5 {
		t.Fatalf("scattered far returns moved the surface to %v", got)
	}
}

// grow's fixtures: weak and foreground returns set directly, so each rule is
// tested without a window.
func growFixture(pts []PointPolar) (*BackgroundGrid, *foregroundRefine, []bool) {
	g := makeCoverageGrid(5, 64)
	r := &foregroundRefine{}
	r.prepare(len(g.Cells), len(pts))
	for i := range pts {
		r.cell[i] = int32(g.Idx(pts[i].Channel-1, azBin(pts[i].Azimuth, 64)))
	}
	return g, r, make([]bool, len(pts))
}

func TestForegroundRefine_WeakJoinsConnectedForeground(t *testing.T) {
	pts := []PointPolar{
		fgPoint(64, 2, 10, 20), // foreground
		fgPoint(64, 3, 11, 20.2),
		fgPoint(64, 4, 12, 20.3), // reachable only through the one above
		fgPoint(64, 3, 30, 20),   // weak, far away in azimuth
		fgPoint(64, 2, 11, 28),   // weak, adjacent, at another range
	}
	g, r, mask := growFixture(pts)
	mask[0] = true
	for i := 1; i < len(pts); i++ {
		r.weak[i] = true
	}
	p := refineParams()
	if got := r.grow(g, &p, pts, mask); got != 2 {
		t.Fatalf("grew %d, want 2", got)
	}
	want := []bool{true, true, true, false, false}
	for i := range want {
		if mask[i] != want[i] {
			t.Fatalf("mask %v, want %v", mask, want)
		}
	}
}

func TestForegroundRefine_NovelPatchComesInAndFlickerDoesNot(t *testing.T) {
	var pts []PointPolar
	for bin := 10; bin < 16; bin++ {
		for ch := 2; ch <= 3; ch++ {
			pts = append(pts, fgPoint(64, ch, bin, 20))
		}
	}
	g, r, mask := growFixture(pts)
	p := refineParams()

	// Weak but known to its cells: flicker stays background.
	for i := range pts {
		r.weak[i] = true
	}
	if got := r.grow(g, &p, pts, mask); got != 0 {
		t.Fatalf("flicker grew %d", got)
	}

	// A patch with too few novel returns stays out.
	g, r, mask = growFixture(pts)
	for i := range pts {
		r.weak[i] = true
		r.novel[i] = i < p.NovelMinReturns-1
	}
	if got := r.grow(g, &p, pts, mask); got != 0 {
		t.Fatalf("under-supported patch grew %d", got)
	}

	// With enough, the whole patch comes in, weak returns included.
	g, r, mask = growFixture(pts)
	for i := range pts {
		r.weak[i] = true
		r.novel[i] = i < p.NovelMinReturns
	}
	if got := r.grow(g, &p, pts, mask); got != len(pts) {
		t.Fatalf("patch grew %d of %d", got, len(pts))
	}
}

// Through the classifier: a vehicle inside the window of a surface that is
// wide open comes in whole, where the plain window leaves it background.
func TestForegroundRefine_ObjectInsideTheWindowComesInWhole(t *testing.T) {
	rings := []int{1, 2, 3, 4, 5}
	road := refineBlock(rings, 10, 40, 30)
	vehicle := refineBlock(rings, 20, 30, 29)
	frame := append(append([]PointPolar(nil), refineBlock(rings, 10, 19, 30)...), vehicle...)
	frame = append(frame, refineBlock(rings, 31, 40, 30)...)
	const first, last = 50, 105 // the vehicle's returns in frame

	plain := newRefineManager(t, 5, false)
	n := settle(t, plain, road, 150)
	if got := countTrue(maskAt(t, plain, frame, n)[first:last]); got != 0 {
		t.Fatalf("control: the window should accept the vehicle, %d returns foreground", got)
	}

	bm := newRefineManager(t, 5, true)
	n = settle(t, bm, road, 150)
	mask := maskAt(t, bm, frame, n)
	if got := countTrue(mask[first:last]); got != last-first {
		t.Fatalf("vehicle: %d of %d returns foreground", got, last-first)
	}
	if got := countTrue(mask[:first]) + countTrue(mask[last:]); got != 0 {
		t.Fatalf("%d road returns foreground beside the vehicle", got)
	}
	// An object teaches the envelope nothing, so the next one is just as new.
	if env := bm.refine.nearEnvelope[bm.Grid.Idx(2, 25)]; env < 29.9 {
		t.Fatalf("the vehicle moved the envelope to %v", env)
	}
}

// A vehicle inside the window must not widen its cells for the next one.
func TestForegroundRefine_WeakReturnsBarelyWidenTheCell(t *testing.T) {
	rings := []int{1, 2, 3}
	surface := refineBlock(rings, 20, 30, 30)
	vehicle := refineBlock(rings[:1], 20, 21, 29)
	spreadAfter := func(refine bool) float32 {
		bm := newRefineManager(t, 3, refine)
		n := settle(t, bm, surface, 150)
		for i := 0; i < 5; i++ {
			maskAt(t, bm, vehicle, n+i)
		}
		return bm.Grid.Cells[bm.Grid.Idx(0, 20)].RangeSpreadMeters
	}
	plain, refined := spreadAfter(false), spreadAfter(true)
	if refined >= plain {
		t.Fatalf("spread %v with refinement, %v without", refined, plain)
	}
}

// Flicker at a range the cells keep returning teaches them that range, so it
// stops being novel.
func TestForegroundRefine_FlickerTeachesTheEnvelope(t *testing.T) {
	rings := []int{1, 2, 3, 4, 5}
	surface := refineBlock(rings, 20, 30, 30)
	flicker := refineBlock(rings, 20, 30, 29)
	bm := newRefineManager(t, 5, true)
	n := settle(t, bm, surface, 150)
	// Too few returns to come in as a patch, so they stay background and teach.
	few := flicker[:bm.Grid.Params.ForegroundRefine.NovelMinReturns-1]
	for i := 0; i < 10; i++ {
		if got := countTrue(maskAt(t, bm, few, n+i)); got != 0 {
			t.Fatalf("frame %d: %d flicker returns foreground", i, got)
		}
	}
	if env := bm.refine.nearEnvelope[bm.Grid.Idx(0, 20)]; env > 29.2 {
		t.Fatalf("envelope at %v after ten returns at 29", env)
	}
	if got := countTrue(maskAt(t, bm, flicker, n+10)[:len(few)]); got != 0 {
		t.Fatalf("%d returns the envelope covers came in", got)
	}
}

func TestForegroundRefine_OffLeavesTheMaskAlone(t *testing.T) {
	rings := []int{1, 2, 3}
	a, b := newRefineManager(t, 3, false), newRefineManager(t, 3, false)
	b.SetDecisionTrace(true)
	stood := refineBlock(rings, 10, 30, 12)
	road := refineBlock(rings, 10, 30, 30)
	n := settle(t, a, stood, 120)
	settle(t, b, stood, 120)
	ma, mb := maskAt(t, a, road, n), maskAt(t, b, road, n)
	for i := range ma {
		if ma[i] != mb[i] {
			t.Fatalf("return %d differs with the trace on", i)
		}
	}
}

// A replay on a reused manager runs as one on a new manager.
func TestForegroundRefine_ResetGridForgetsTheEnvelope(t *testing.T) {
	rings := []int{1, 2, 3, 4, 5}
	bm := newRefineManager(t, 5, true)
	settle(t, bm, refineBlock(rings, 20, 30, 40), 60)
	if err := bm.ResetGrid(); err != nil {
		t.Fatal(err)
	}
	c := bm.Grid.Idx(0, 20)
	if bm.refine.nearEnvelope[c] != 0 || bm.refine.farRun[c] != 0 || bm.refine.farRange[c] != 0 {
		t.Fatalf("state survived the reset: env %v run %v range %v", bm.refine.nearEnvelope[c], bm.refine.farRun[c], bm.refine.farRange[c])
	}
}

// The pass runs every frame on a Pi: once its buffers have grown it allocates
// nothing.
func TestForegroundRefine_GrowDoesNotAllocate(t *testing.T) {
	var pts []PointPolar
	for bin := 10; bin < 30; bin++ {
		for ch := 2; ch <= 4; ch++ {
			pts = append(pts, fgPoint(64, ch, bin, 20))
		}
	}
	g, r, mask := growFixture(pts)
	p := refineParams()
	frame := func() {
		r.prepare(len(g.Cells), len(pts))
		clear(mask)
		for i := range pts {
			r.cell[i] = int32(g.Idx(pts[i].Channel-1, azBin(pts[i].Azimuth, 64)))
			r.weak[i] = true
			r.novel[i] = i%2 == 0
		}
		mask[0] = true
		r.grow(g, &p, pts, mask)
		r.learnEnvelope(&p, pts, mask, nil, 0)
	}
	frame()
	if got := testing.AllocsPerRun(20, frame); got != 0 {
		t.Fatalf("%v allocations a frame", got)
	}
}

func TestForegroundRefineTuning_RoundTrips(t *testing.T) {
	p := refineParams()
	got := ForegroundRefineParamsFromTuning(ForegroundRefineTuningFromParams(&p))
	if *got != p {
		t.Fatalf("got %+v, want %+v", *got, p)
	}
	if ForegroundRefineParamsFromTuning(nil) != nil || ForegroundRefineTuningFromParams(nil) != nil {
		t.Fatal("an absent block must stay absent")
	}
	if clone := p.clone(); clone == &p || *clone != p {
		t.Fatal("clone must copy")
	}
	var none *ForegroundRefineParams
	if none.clone() != nil {
		t.Fatal("nil clone")
	}
}

func TestDecisionTrace_NamesTheRule(t *testing.T) {
	rings := []int{1, 2, 3, 4, 5}
	road := refineBlock(rings, 10, 40, 30)
	bm := newRefineManager(t, 5, true)
	n := settle(t, bm, road, 150)
	bm.SetDecisionTrace(true)
	frame := append(refineBlock(rings, 10, 12, 30), refineBlock(rings, 20, 30, 29)...)
	frame = append(frame, refineBlock(rings, 33, 35, 60)...)
	frame = append(frame, refineBlock(rings, 36, 38, 5)...)
	maskAt(t, bm, frame, n)
	trace := bm.DecisionTrace()
	if len(trace) != len(frame) {
		t.Fatalf("%d records for %d returns", len(trace), len(frame))
	}
	want := map[int]DecisionCode{
		0:            DecisionBackgroundBase,
		15:           DecisionForegroundGrown,
		15 + 55:      DecisionBackgroundFar,
		15 + 55 + 15: DecisionForeground,
	}
	for i, code := range want {
		if trace[i].Code != code {
			t.Errorf("return %d: rule %d, want %d", i, trace[i].Code, code)
		}
	}
	var nilManager *BackgroundManager
	if nilManager.DecisionTrace() != nil {
		t.Fatal("nil manager has a trace")
	}
	nilManager.SetDecisionTrace(true)
}
