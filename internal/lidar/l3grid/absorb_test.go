package l3grid

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// The absorb tests reuse the review's one-ring setup from
// foreground_stationary_object_test.go: shipped parameters, region overrides
// off, replay mode, frames every 100 ms, 1 cm Gaussian range noise. A road at
// 15 m settles for 1,000 frames, then a car face at 8 m arrives in bins
// roadCells and up and stays. TestMask_Arrival_NeverAbsorbedWithoutRegionOverrides
// pins that L3 alone never absorbs it.

const (
	absorbAzBins     = 60
	absorbRoadCells  = 20
	absorbRoadFrames = 1000
	// absorbArrival is how long the car is foreground before the dwell
	// policy would act on it in these tests: 30 s at 10 Hz.
	absorbArrival = 300
)

// absorbScene drives the ring frame by frame and remembers the last frame's
// returns, which are what the pipeline would hand AbsorbReturns.
type absorbScene struct {
	t      *testing.T
	bm     *BackgroundManager
	rng    *rand.Rand
	frame  int
	points []PointPolar
}

func newAbsorbScene(t *testing.T) *absorbScene {
	t.Helper()
	return &absorbScene{
		t: t, bm: newMaskReviewManager(t, absorbAzBins), rng: rand.New(rand.NewSource(1)),
		points: make([]PointPolar, absorbAzBins),
	}
}

// step plays one frame in which every car cell returns carRange (the road
// cells always return the road) and returns its mask.
func (s *absorbScene) step(carRange float64) []bool {
	s.t.Helper()
	for b := 0; b < absorbAzBins; b++ {
		r := maskReviewRoadMetres
		if b >= absorbRoadCells {
			r = carRange
		}
		s.points[b] = maskReviewPoint(b, absorbAzBins, r+s.rng.NormFloat64()*maskReviewNoiseMetres)
	}
	mask, err := s.bm.ProcessFramePolarWithMaskAt(s.points, maskReviewFrameTime(s.frame))
	if err != nil {
		s.t.Fatalf("frame %d: %v", s.frame, err)
	}
	s.frame++
	return append([]bool(nil), mask...)
}

// arrive settles the road, then plays the car's arrival, and fails unless the
// car is still foreground in every car cell at the end of it: the state the
// dwell policy exists for.
func (s *absorbScene) arrive() {
	s.t.Helper()
	for i := 0; i < absorbRoadFrames; i++ {
		s.step(maskReviewRoadMetres)
	}
	var mask []bool
	for i := 0; i < absorbArrival; i++ {
		mask = s.step(maskReviewCarMetres)
	}
	for b := absorbRoadCells; b < absorbAzBins; b++ {
		if !mask[b] {
			s.t.Fatalf("premise: car cell %d is background %d frames after arrival; L3 alone was expected never to absorb it", b, absorbArrival)
		}
	}
}

// absorbCar hands AbsorbReturns the last frame's car returns, as the pipeline
// does with the returns inside an absorbed track's box.
func (s *absorbScene) absorbCar() int {
	s.t.Helper()
	car := append([]PointPolar(nil), s.points[absorbRoadCells:]...)
	return s.bm.AbsorbReturns(car, maskReviewFrameTime(s.frame-1))
}

func TestAbsorbReturns_ArrivedObjectBecomesBackground(t *testing.T) {
	s := newAbsorbScene(t)
	s.arrive()
	before := append([]BackgroundCell(nil), s.bm.Grid.Cells...)
	carReturns := append([]PointPolar(nil), s.points[absorbRoadCells:]...)

	if got, want := s.absorbCar(), absorbAzBins-absorbRoadCells; got != want {
		t.Fatalf("AbsorbReturns absorbed %d cells, want %d", got, want)
	}
	threshold := s.bm.Grid.Params.LockedBaselineThreshold
	for b := 0; b < absorbAzBins; b++ {
		c, was := s.bm.Grid.Cells[b], before[b]
		if b < absorbRoadCells {
			if c != was {
				t.Errorf("road cell %d changed: %+v, was %+v", b, c, was)
			}
			continue
		}
		want := float32(carReturns[b-absorbRoadCells].Distance)
		if c.AverageRangeMeters != want {
			t.Errorf("cell %d average %.4f, want the return %.4f", b, c.AverageRangeMeters, want)
		}
		if c.TimesSeenCount != was.TimesSeenCount {
			t.Errorf("cell %d count %d, want it kept at %d", b, c.TimesSeenCount, was.TimesSeenCount)
		}
		if c.RangeSpreadMeters != was.RangeSpreadMeters {
			t.Errorf("cell %d spread %.4f, want the learned %.4f kept (one return has no span)", b, c.RangeSpreadMeters, was.RangeSpreadMeters)
		}
		if c.FrozenUntilUnixNanos != 0 || c.RecentForegroundCount != 0 {
			t.Errorf("cell %d frozen until %d with %d recent foreground, want thawed and cleared",
				b, c.FrozenUntilUnixNanos, c.RecentForegroundCount)
		}
		if was.LockedAtCount < threshold {
			t.Fatalf("premise: cell %d had no lock in force after %d road frames", b, absorbRoadFrames)
		}
		if c.LockedBaseline != want || c.LockedSpread != c.RangeSpreadMeters || c.LockedAtCount != was.LockedAtCount {
			t.Errorf("cell %d lock %.4f ± %.4f at %d, want moved to %.4f ± %.4f at %d",
				b, c.LockedBaseline, c.LockedSpread, c.LockedAtCount, want, c.RangeSpreadMeters, was.LockedAtCount)
		}
	}

	for i := 0; i < 100; i++ {
		mask := s.step(maskReviewCarMetres)
		for b := range mask {
			if mask[b] {
				t.Fatalf("%d frames after absorbing: cell %d is foreground", i, b)
			}
		}
	}
}

// Moving the lock to the object, rather than keeping it on the road, is what
// keeps a road user passing in front of the absorbed object visible: kept on
// the road, the lock's slow update would widen its window over the whole
// range within a couple of minutes. This checks the passer at the worst point
// of that drift, 100 s after absorption, and at once.
func TestAbsorbReturns_PasserInFrontOfAbsorbedObjectStaysForeground(t *testing.T) {
	const passer = 5.0
	s := newAbsorbScene(t)
	s.arrive()
	s.absorbCar()
	for _, after := range []int{1, 1000} {
		for i := 0; i < after; i++ {
			s.step(maskReviewCarMetres)
		}
		mask := s.step(passer)
		for b := absorbRoadCells; b < absorbAzBins; b++ {
			if !mask[b] {
				c := s.bm.Grid.Cells[b]
				t.Fatalf("%d frames after absorbing: a return at %.0f m in front of the car is background in cell %d (average %.2f, lock %.2f ± %.2f)",
					after, passer, b, c.AverageRangeMeters, c.LockedBaseline, c.LockedSpread)
			}
		}
		// Let the passer's freeze, if any, lapse before the next check.
		for i := 0; i < 60; i++ {
			s.step(maskReviewCarMetres)
		}
	}
}

// The cost of moving the lock: when the absorbed car leaves, the road behind
// it is a ghost that clears as any learned object's does. Here the road flanks
// the car on both sides of the ring, as it does for a real parked car, and
// neighbour confirmation erodes the ghost from the edges.
func TestAbsorbReturns_FlankedDepartureClearsByNeighbours(t *testing.T) {
	const parked, maxAfter = 100, 3000
	s := newAbsorbScene(t)
	s.arrive()
	s.absorbCar()
	for i := 0; i < parked; i++ {
		s.step(maskReviewCarMetres)
	}
	lastForeground := make([]int, absorbAzBins)
	for b := range lastForeground {
		lastForeground[b] = -1
	}
	for f := 0; f < maxAfter; f++ {
		mask := s.step(maskReviewRoadMetres)
		for b := range mask {
			if mask[b] {
				lastForeground[b] = f
			}
		}
	}
	allClear := -1
	for b := absorbRoadCells; b < absorbAzBins; b++ {
		if lastForeground[b] == maxAfter-1 {
			t.Fatalf("car cell %d is still foreground %d frames after the car left", b, maxAfter)
		}
		allClear = max(allClear, lastForeground[b]+1)
	}
	for b := 0; b < absorbRoadCells; b++ {
		if lastForeground[b] >= 0 {
			t.Errorf("road cell %d was foreground after the car left", b)
		}
	}
	t.Logf("every car cell clear %d frames after the car left (review, car learned from frame 0, 20 road cells: 56)", allClear)
	// Seconds, not the dwell: a minute is the ceiling this asserts.
	if allClear > 600 {
		t.Errorf("the departure ghost lasted %d frames; a flanked ghost should clear within 600", allClear)
	}
}

func TestAbsorbReturns_FoldsSeedsAndIgnores(t *testing.T) {
	bm := newMaskReviewManager(t, absorbAzBins)
	at := maskReviewFrameTime(0)
	g := bm.Grid
	nonzero := g.nonzeroCellCount

	// Two returns in bin 3, one in bin 4, and returns that map nowhere or
	// carry no range.
	pts := []PointPolar{
		maskReviewPoint(3, absorbAzBins, 8.0),
		maskReviewPoint(4, absorbAzBins, 9.0),
		maskReviewPoint(3, absorbAzBins, 8.4),
		{Channel: 0, Azimuth: 10, Distance: 5},
		{Channel: 2, Azimuth: 10, Distance: 5},
		maskReviewPoint(5, absorbAzBins, 0),
		maskReviewPoint(6, absorbAzBins, math.NaN()),
		maskReviewPoint(7, absorbAzBins, math.Inf(1)),
	}
	if got := bm.AbsorbReturns(pts, at); got != 2 {
		t.Fatalf("absorbed %d cells, want 2", got)
	}
	c3, c4 := g.Cells[3], g.Cells[4]
	if math.Abs(float64(c3.AverageRangeMeters)-8.2) > 1e-6 || math.Abs(float64(c3.RangeSpreadMeters)-0.2) > 1e-6 {
		t.Errorf("bin 3: average %.4f spread %.4f, want the mean 8.2 and half the span 0.2", c3.AverageRangeMeters, c3.RangeSpreadMeters)
	}
	if c3.TimesSeenCount != 1 || c4.TimesSeenCount != 1 || c4.AverageRangeMeters != 9 || c4.RangeSpreadMeters != 0 {
		t.Errorf("empty cells were not seeded as a first observation: bin 3 %+v, bin 4 %+v", c3, c4)
	}
	if c3.LockedAtCount != 0 || c3.LockedBaseline != 0 {
		t.Errorf("bin 3 took a lock with no count behind it: %+v", c3)
	}
	if c3.LastUpdateUnixNanos != at.UnixNano() {
		t.Errorf("bin 3 last update %d, want %d", c3.LastUpdateUnixNanos, at.UnixNano())
	}
	if got := g.nonzeroCellCount - nonzero; got != 2 {
		t.Errorf("nonzero cell count rose by %d, want 2", got)
	}
	for _, b := range []int{5, 6, 7} {
		if g.Cells[b].TimesSeenCount != 0 {
			t.Errorf("bin %d was absorbed from a return without a range", b)
		}
	}

	if got := bm.AbsorbReturns(nil, at); got != 0 {
		t.Errorf("no returns absorbed %d cells", got)
	}
	var none *BackgroundManager
	if got := none.AbsorbReturns(pts, at); got != 0 {
		t.Errorf("a nil manager absorbed %d cells", got)
	}
}

// AbsorbReturns maps a return to its cell with returnCell; the mask path does
// the same inline. Absorb a near return into a settled far background and
// play it back: if the two mappings disagreed, the mask path would judge it
// against an unabsorbed far cell and call it foreground.
func TestAbsorbedReturnIsBackgroundOnTheMaskPath(t *testing.T) {
	const rings, azBins = 4, 1800
	azimuths := []float64{0, 0.1, 0.2, 0.19999999, 179.9, 359.9, 359.99999999, 360, 360.1, 725.3, -0.1, -0.2, -359.95, -720.05}
	for ch := 1; ch <= rings; ch++ {
		for _, az := range azimuths {
			params := newMaskReviewManager(t, 1).Grid.Params
			bm := NewBackgroundManagerDI("absorb-mapping", rings, azBins, params, nil)
			bm.SetReplayMode(true)
			for i := range bm.Grid.Cells {
				c := &bm.Grid.Cells[i]
				c.AverageRangeMeters, c.TimesSeenCount = 20, 500
				c.LockedBaseline, c.LockedAtCount = 20, 500
			}
			bm.Grid.SettlingComplete = true
			p := PointPolar{Channel: ch, Azimuth: az, Distance: 5}
			if n := bm.AbsorbReturns([]PointPolar{p}, time.Unix(0, 0)); n != 1 {
				t.Fatalf("channel %d azimuth %v: absorbed %d cells, want 1", ch, az, n)
			}
			mask, err := bm.ProcessFramePolarWithMaskAt([]PointPolar{p}, time.Unix(0, int64(100*time.Millisecond)))
			if err != nil {
				t.Fatal(err)
			}
			if mask[0] {
				t.Errorf("channel %d azimuth %v: the absorbed return is foreground on the mask path", ch, az)
			}
		}
	}
}
