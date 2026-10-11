package l3grid

import (
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
)

// These tests pin three behaviours of ProcessFramePolarWithMaskAt that the
// beam-wise background subtraction review
// (data/maths/proposals/20261011-beamwise-background-subtraction-review.md,
// Sections 5.1, 5.3 and 8) measured with throwaway harnesses. They are
// characterisation tests: each asserts what the shipped defaults do today and
// logs the review's measured frame counts beside its own, so a behaviour
// change (a dwell policy for stationary objects, or any edit to the spread
// update) has to come here and say what it changed.
//
// Common setup, as in the review's Section 8: one ring, parameters from
// config/tuning.defaults.json, region overrides off, replay mode on (so
// warmup does not suppress the mask), no store, frames every 100 ms from a
// fixed epoch, Gaussian range noise from a seeded source.

const (
	maskReviewFramePeriod = 100 * time.Millisecond
	maskReviewRoadMetres  = 15.0
	maskReviewCarMetres   = 8.0
	maskReviewNoiseMetres = 0.010
)

var maskReviewEpoch = time.Date(2026, time.October, 11, 0, 0, 0, 0, time.UTC)

func newMaskReviewManager(t *testing.T, azBins int) *BackgroundManager {
	t.Helper()
	cfg := config.MustLoadDefaultConfig()
	params := BackgroundConfigFromActiveTuning(cfg).ToBackgroundParams()
	params.DisableRegionOverrides = true
	bm := NewBackgroundManagerDI("mask-review", 1, azBins, params, nil)
	if bm == nil {
		t.Fatal("NewBackgroundManagerDI returned nil")
	}
	bm.SetReplayMode(true)
	return bm
}

func maskReviewFrameTime(frame int) time.Time {
	return maskReviewEpoch.Add(time.Duration(frame) * maskReviewFramePeriod)
}

// maskReviewPoint is a return on ring 1 at the centre of azimuth bin b of a
// ring with azBins bins.
func maskReviewPoint(bin, azBins int, rangeMetres float64) PointPolar {
	return PointPolar{
		Channel:  1,
		Azimuth:  (float64(bin) + 0.5) * 360.0 / float64(azBins),
		Distance: rangeMetres,
	}
}

// parkedCarResult is what runParkedCarDeparture measured about the ghost the
// car leaves behind. Frames are counted from the departure frame.
type parkedCarResult struct {
	// lastForeground[i] is the last frame on which car cell i was
	// foreground, or -1 if it was background from the departure frame on.
	lastForeground []int
	// continuousGhost is the number of frames from departure during which
	// every car cell was foreground on every frame.
	continuousGhost int
}

// clearFrames reports the frame on which the first car cell, the median car
// cell and the last car cell were clear for good: the review's "first cell
// clear / half / all" columns.
func (r parkedCarResult) clearFrames() (first, half, all int) {
	sorted := append([]int(nil), r.lastForeground...)
	sort.Ints(sorted)
	return sorted[0] + 1, sorted[(len(sorted)-1)/2] + 1, sorted[len(sorted)-1] + 1
}

// runParkedCarDeparture drives the review's Simulation A on a 60-bin ring.
// Bins below roadCells return the road on every frame; the remaining car
// cells return the car for the first dwell frames and the road from then on.
// The run continues for maxAfter frames past departure.
func runParkedCarDeparture(t *testing.T, roadCells, dwell, maxAfter int) parkedCarResult {
	t.Helper()
	const azBins = 60
	bm := newMaskReviewManager(t, azBins)
	rng := rand.New(rand.NewSource(1))
	points := make([]PointPolar, azBins)
	res := parkedCarResult{lastForeground: make([]int, azBins-roadCells), continuousGhost: -1}
	for i := range res.lastForeground {
		res.lastForeground[i] = -1
	}

	for frame := 0; frame < dwell+maxAfter; frame++ {
		carPresent := frame < dwell
		for b := 0; b < azBins; b++ {
			r := maskReviewRoadMetres
			if b >= roadCells && carPresent {
				r = maskReviewCarMetres
			}
			points[b] = maskReviewPoint(b, azBins, r+rng.NormFloat64()*maskReviewNoiseMetres)
		}
		mask, err := bm.ProcessFramePolarWithMaskAt(points, maskReviewFrameTime(frame))
		if err != nil {
			t.Fatalf("frame %d: ProcessFramePolarWithMaskAt: %v", frame, err)
		}
		if frame == dwell-1 {
			// The car was there from frame 0, so by the end of the dwell it
			// is the settled background and must not be reported.
			for b := roadCells; b < azBins; b++ {
				if mask[b] {
					t.Fatalf("frame %d: car cell %d is foreground at the end of the dwell", frame, b)
				}
			}
		}
		if carPresent {
			continue
		}
		after := frame - dwell
		allForeground := true
		for b := roadCells; b < azBins; b++ {
			if mask[b] {
				res.lastForeground[b-roadCells] = after
			} else {
				allForeground = false
			}
		}
		if !allForeground && res.continuousGhost < 0 {
			res.continuousGhost = after
		}
	}
	if res.continuousGhost < 0 {
		res.continuousGhost = maxAfter
	}
	return res
}

// TestMask_DepartureGhost_UnflankedLastsDwell: a car face at 8 m learned as
// the background of all 60 cells from frame 0, parked for 1,000 frames, then
// replaced by the road at 15 m. No cell on the ring has the road as its
// background, so neighbour confirmation never vouches for the road return.
// The ghost lasts the dwell (TimesSeenCount drains by one per rejected
// return) plus a freeze-cycling tail: below a count of 100 each thaw rejects
// one return, decrements once and freezes for 5 s, until the warmup ramp has
// widened the window enough that the 7 m deviation is within three times it
// and the deadlock breaker relearns. The review measured first cell clear /
// half / all at 3,104 / 3,104 / 3,109 frames after departure.
func TestMask_DepartureGhost_UnflankedLastsDwell(t *testing.T) {
	const (
		dwell    = 1000
		maxAfter = 4500
		lowBand  = 2500
		highBand = 4000
	)
	res := runParkedCarDeparture(t, 0, dwell, maxAfter)
	first, half, all := res.clearFrames()
	t.Logf("unflanked ghost after a %d-frame dwell: every car cell foreground for %d frames; first cell clear %d, half %d, all %d (review: 3,104 / 3,104 / 3,109)",
		dwell, res.continuousGhost, first, half, all)

	if res.continuousGhost < lowBand {
		t.Errorf("a car cell was background %d frames after departure; the unflanked ghost should last at least %d frames", res.continuousGhost, lowBand)
	}
	if all < lowBand || all > highBand {
		t.Errorf("all car cells clear %d frames after departure, want within [%d, %d]", all, lowBand, highBand)
	}
}

// TestMask_DepartureGhost_FlankedErodesInSeconds: the same car, but bins 0
// to 19 return the road throughout. A car cell at the edge of the car is
// accepted by same-ring neighbour confirmation on its first road return,
// its accepted 7 m deviation inflates its spread by alpha*delta, and within
// a few frames it vouches for the next cell in. The ring wraps, so erosion
// runs in from both edges. The review measured first cell clear / half /
// all at 0 / 34 / 56 frames after departure, against 3,104 / 3,104 / 3,109
// without the road cells.
func TestMask_DepartureGhost_FlankedErodesInSeconds(t *testing.T) {
	const (
		roadCells = 20
		dwell     = 1000
		maxAfter  = 500
		within    = 100
	)
	res := runParkedCarDeparture(t, roadCells, dwell, maxAfter)
	first, half, all := res.clearFrames()
	t.Logf("flanked ghost after a %d-frame dwell: every car cell foreground for %d frames; first cell clear %d, half %d, all %d (review: 0 / 34 / 56)",
		dwell, res.continuousGhost, first, half, all)

	if all > within {
		t.Errorf("all %d car cells clear %d frames after departure, want within %d", 60-roadCells, all, within)
	}
}

// TestMask_Arrival_NeverAbsorbedWithoutRegionOverrides: the road at 15 m is
// the settled background of every cell, then a car face at 8 m arrives and
// stays. The car's return is judged against the window at its own range,
// 8 m, and its neighbours' backgrounds are the road, so nothing vouches for
// it. TimesSeenCount drains by one per rejected return; below 100 the
// freeze cycles, one decrement per 5 s thaw; at the confidence floor the
// warmup ramp is at its widest and stops there.
//
// Bound. With region overrides off the window at range r for a cell of
// spread s and count at the floor (WarmupMultiplier(3) = 3.91) is
//
//	w = 3*(s + 0.02*r + 0.01)*3.91 + 0.15
//
// from ForegroundClosenessWindowMetres with the shipped multiplier 3,
// noise_relative 0.02, closeness floor 0.01 m and safety margin 0.15 m.
// The freeze fires while delta > 3*w (freeze_threshold_multiplier 3), and
// the deadlock breaker only ever sees a cell that is not freezing, since a
// frozen cell does not accumulate RecentForegroundCount and a thaw resets
// it. So the car is absorbed only if
//
//	delta <= 3*(3*(s + 0.02*r + 0.01)*3.91 + 0.15) = 0.70*r + 35*s + 0.80
//
// which at r = 8 m with s near 0.01 m is 6.78 m, below the 7 m depth. The
// breaker never fires and the freeze cycles for ever. The review ran the
// car for 9,000 frames and saw no cell absorb it.
func TestMask_Arrival_NeverAbsorbedWithoutRegionOverrides(t *testing.T) {
	const (
		azBins     = 60
		roadFrames = 1000
		carFrames  = 9000
	)
	bm := newMaskReviewManager(t, azBins)
	rng := rand.New(rand.NewSource(1))
	points := make([]PointPolar, azBins)

	absorbed := 0
	firstAbsorbed := -1
	floorReached := -1
	floor := bm.Grid.Params.MinConfidenceFloor
	for frame := 0; frame < roadFrames+carFrames; frame++ {
		r := maskReviewRoadMetres
		if frame >= roadFrames {
			r = maskReviewCarMetres
		}
		for b := 0; b < azBins; b++ {
			points[b] = maskReviewPoint(b, azBins, r+rng.NormFloat64()*maskReviewNoiseMetres)
		}
		mask, err := bm.ProcessFramePolarWithMaskAt(points, maskReviewFrameTime(frame))
		if err != nil {
			t.Fatalf("frame %d: ProcessFramePolarWithMaskAt: %v", frame, err)
		}
		if frame == roadFrames-1 {
			for b := range mask {
				if mask[b] {
					t.Fatalf("frame %d: cell %d is foreground on the settled road", frame, b)
				}
			}
			continue
		}
		if frame < roadFrames {
			continue
		}
		for b := range mask {
			if !mask[b] {
				absorbed++
				if firstAbsorbed < 0 {
					firstAbsorbed = frame - roadFrames
				}
			}
		}
		if floorReached < 0 && bm.Grid.Cells[0].TimesSeenCount <= floor {
			floorReached = frame - roadFrames
		}
	}

	// The premise of the test, from the parameters actually loaded: three
	// times the widest window the car's return is ever judged against is
	// below the car's depth.
	p := bm.Grid.Params
	spread := 0.0
	for _, c := range bm.Grid.Cells {
		if float64(c.RangeSpreadMeters) > spread {
			spread = float64(c.RangeSpreadMeters)
		}
	}
	window := ForegroundClosenessWindowMetres(
		float64(p.ClosenessSensitivityMultiplier), spread,
		float64(p.NoiseRelativeFraction), maskReviewCarMetres,
		float64(p.SafetyMarginMetres), WarmupMultiplier(floor))
	bound := float64(p.FreezeThresholdMultiplier) * window
	depth := maskReviewRoadMetres - maskReviewCarMetres
	t.Logf("absorption bound at %.0f m with spread %.4f m: 3*w = %.2f m against a %.0f m depth; count reached the floor (%d) %d frames after arrival; %d of %d car returns absorbed (review: never)",
		maskReviewCarMetres, spread, bound, depth, floor, floorReached, absorbed, carFrames*azBins)
	if bound >= depth {
		t.Fatalf("premise: absorption bound %.2f m is not below the car's depth %.2f m, so the deadlock breaker could fire", bound, depth)
	}

	if absorbed != 0 {
		t.Errorf("%d car returns were classified background (first %d frames after arrival); with region overrides off the car should never be absorbed", absorbed, firstAbsorbed)
	}
	if floorReached < 0 {
		t.Errorf("TimesSeenCount never drained to the floor %d in %d frames", floor, carFrames)
	}
	if got := bm.Grid.Cells[0].TimesSeenCount; got != floor {
		t.Errorf("TimesSeenCount at the end = %d, want the floor %d", got, floor)
	}
	if last := maskReviewFrameTime(roadFrames + carFrames - 1).UnixNano(); bm.Grid.Cells[0].FrozenUntilUnixNanos <= last {
		t.Errorf("cell 0 is not frozen at the end of the run; the freeze should still be cycling")
	}
}

// TestMask_SkyCell_LearnsPassingVehicles: the review's Simulation B. On an
// 8-bin ring, bin 7 returns 5 m on every frame so the frame counter
// advances; bin 0 is a sky cell that returns nothing for 400 frames, then
// 200 vehicles of 8 frames each at a range drawn uniformly from 10 to 20 m,
// each followed by 40 empty frames. seed_from_first makes the first vehicle
// the cell's background; the count drains to the floor on the next, the
// deadlock breaker accepts it, and each accepted far return adds
// alpha*delta to the spread (spread inflation, Section 5.3) until the
// window covers the whole 10 to 20 m band. The review counted 5 of 80
// foreground flags over vehicles 1 to 10, 0 of 1,520 over vehicles 11 to
// 200, and a final mean of 14.67 m, spread 2.36 m, TimesSeenCount 1,590.
func TestMask_SkyCell_LearnsPassingVehicles(t *testing.T) {
	const (
		azBins        = 8
		skyBin        = 0
		groundBin     = 7
		groundMetres  = 5.0
		emptyFrames   = 400
		vehicles      = 200
		vehicleFrames = 8
		gapFrames     = 40
		vehicleNoise  = 0.020
	)
	bm := newMaskReviewManager(t, azBins)
	rng := rand.New(rand.NewSource(2))

	frame := 0
	points := make([]PointPolar, 0, 2)
	// step feeds one frame and reports whether the sky return, if any, was
	// foreground.
	step := func(skyRange float64, hasSky bool) bool {
		points = points[:0]
		points = append(points, maskReviewPoint(groundBin, azBins, groundMetres+rng.NormFloat64()*maskReviewNoiseMetres))
		if hasSky {
			points = append(points, maskReviewPoint(skyBin, azBins, skyRange))
		}
		mask, err := bm.ProcessFramePolarWithMaskAt(points, maskReviewFrameTime(frame))
		if err != nil {
			t.Fatalf("frame %d: ProcessFramePolarWithMaskAt: %v", frame, err)
		}
		frame++
		return hasSky && mask[1]
	}

	for i := 0; i < emptyFrames; i++ {
		step(0, false)
	}
	fgFirstTen, fgRest := 0, 0
	for v := 1; v <= vehicles; v++ {
		vehicleRange := 10 + 10*rng.Float64()
		for i := 0; i < vehicleFrames; i++ {
			if step(vehicleRange+rng.NormFloat64()*vehicleNoise, true) {
				if v <= 10 {
					fgFirstTen++
				} else {
					fgRest++
				}
			}
		}
		for i := 0; i < gapFrames; i++ {
			step(0, false)
		}
	}

	cell := bm.Grid.Cells[bm.Grid.Idx(0, skyBin)]
	const firstTenReturns = 10 * vehicleFrames
	const restReturns = (vehicles - 10) * vehicleFrames
	t.Logf("sky cell: %d of %d returns foreground over vehicles 1 to 10 (review: 5 of 80), %d of %d over vehicles 11 to 200 (review: 0 of 1,520); final mean %.2f m, spread %.2f m, TimesSeenCount %d (review: 14.67 m, 2.36 m, 1,590)",
		fgFirstTen, firstTenReturns, fgRest, restReturns, cell.AverageRangeMeters, cell.RangeSpreadMeters, cell.TimesSeenCount)

	if fgFirstTen*10 >= firstTenReturns {
		t.Errorf("%d of %d returns foreground over the first ten vehicles, want fewer than 10%%", fgFirstTen, firstTenReturns)
	}
	if fgRest != 0 {
		t.Errorf("%d of %d returns foreground over vehicles 11 to 200, want none: the cell should have learned the band by then", fgRest, restReturns)
	}
	if cell.RangeSpreadMeters <= 1.0 {
		t.Errorf("final spread %.2f m, want above 1 m: spread inflation should have widened the sky cell to the vehicle band", cell.RangeSpreadMeters)
	}
}
