package main

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// The ring index is derived from a cell's position in a flat slice, matching
// BackgroundGrid.Idx's ring*azimuthBins+azBin layout. Getting that arithmetic
// wrong would silently attribute every cell to the wrong elevation and so
// invert the conclusion the report draws, without failing anything.

func TestRingAccumulatorDerivesRingFromCellIndex(t *testing.T) {
	const rings, azimuthBins = 4, 10
	a := newRingAccumulator(rings, azimuthBins)
	a.setElevations(`[10, 5, -5, -10]`)

	// One cell in each ring, at a different azimuth within it, so a
	// transposed index would land in the wrong ring.
	for ring := 0; ring < rings; ring++ {
		idx := ring*azimuthBins + (ring + 2)
		a.observe(idx, l3grid.BackgroundCell{
			AverageRangeMeters: float32(10 * (ring + 1)),
			RangeSpreadMeters:  0.01,
			TimesSeenCount:     500,
		}, 50)
	}

	for ring := 0; ring < rings; ring++ {
		if got := len(a.rings[ring].spreads); got != 1 {
			t.Errorf("ring %d got %d cells, want 1", ring, got)
		}
		want := float64(10 * (ring + 1))
		if got := a.rings[ring].ranges[0]; got != want {
			t.Errorf("ring %d got range %v, want %v: cells are landing in the wrong ring", ring, got, want)
		}
	}

	// The last azimuth of one ring must not spill into the next.
	b := newRingAccumulator(rings, azimuthBins)
	b.observe(azimuthBins-1, l3grid.BackgroundCell{AverageRangeMeters: 5, TimesSeenCount: 500}, 50)
	if len(b.rings[0].spreads) != 1 || len(b.rings[1].spreads) != 0 {
		t.Errorf("the last azimuth of ring 0 was attributed to ring %d", 1)
	}
}

func TestRingAccumulatorIgnoresOutOfRangeIndices(t *testing.T) {
	a := newRingAccumulator(2, 10)
	// A grid that has grown between snapshots must not panic or corrupt the
	// tally; the cell is dropped instead.
	a.observe(1000, l3grid.BackgroundCell{AverageRangeMeters: 5, TimesSeenCount: 500}, 50)
	a.observe(-1, l3grid.BackgroundCell{AverageRangeMeters: 5, TimesSeenCount: 500}, 50)
	for ring := range a.rings {
		if len(a.rings[ring].spreads) != 0 {
			t.Errorf("ring %d recorded an out-of-range cell", ring)
		}
	}
	if len(a.allRanges) != 0 {
		t.Errorf("out-of-range cells reached the population tally")
	}
}

func TestRingAccumulatorSeparatesHighSpreadAndForeground(t *testing.T) {
	const rings, azimuthBins = 2, 4
	a := newRingAccumulator(rings, azimuthBins)
	a.setElevations(`[8, -8]`)

	add := func(ring int, azBin int, spread float32, fg uint16) {
		a.observe(ring*azimuthBins+azBin, l3grid.BackgroundCell{
			AverageRangeMeters:    50,
			RangeSpreadMeters:     spread,
			TimesSeenCount:        500,
			RecentForegroundCount: fg,
		}, 50)
	}

	// Ring 0 (above horizon): two high-spread, neither reporting foreground.
	add(0, 0, 5.0, 0)
	add(0, 1, 2.0, 0)
	add(0, 2, 0.01, 0) // not high spread
	// Ring 1 (below horizon): one high-spread reporting foreground.
	add(1, 0, 3.0, 7)
	add(1, 1, 0.02, 3) // reporting foreground but not high spread

	if got := a.rings[0].highSpread; got != 2 {
		t.Errorf("ring 0 high-spread count = %d, want 2", got)
	}
	if got := a.rings[0].highSpreadWithForeground; got != 0 {
		t.Errorf("ring 0 high-spread-with-foreground = %d, want 0", got)
	}
	if got := a.rings[1].highSpread; got != 1 {
		t.Errorf("ring 1 high-spread count = %d, want 1", got)
	}
	if got := a.rings[1].highSpreadWithForeground; got != 1 {
		t.Errorf("ring 1 high-spread-with-foreground = %d, want 1", got)
	}

	// A cell reporting foreground with a small spread must not be counted as
	// high spread: conflating the two is exactly the error that would
	// manufacture a feedback loop out of ordinary traffic.
	if len(a.high) != 3 {
		t.Errorf("retained %d high-spread cells, want 3", len(a.high))
	}
	for _, c := range a.high {
		if c.elevationDeg > 0 && c.foreground {
			t.Errorf("an above-horizon cell was recorded as reporting foreground")
		}
	}
}

func TestRingAccumulatorElevationsAreOptionalAndValidated(t *testing.T) {
	// A snapshot with a mismatched or missing elevation list must leave the
	// elevations unknown rather than silently misattributing rings.
	for _, elevationsJSON := range []string{"", "[1,2]", "not json", "[]"} {
		a := newRingAccumulator(4, 10)
		a.setElevations(elevationsJSON)
		if !math.IsNaN(a.elevationDeg[0]) {
			t.Errorf("elevations %q were accepted for a 4-ring grid", elevationsJSON)
		}
	}

	a := newRingAccumulator(4, 10)
	a.setElevations(`[10, 5, -5, -10]`)
	if a.elevationDeg[3] != -10 {
		t.Errorf("elevation not recorded: got %v", a.elevationDeg[3])
	}
	// The first snapshot to supply them wins, so a later one cannot shift the
	// attribution mid-run.
	a.setElevations(`[1, 2, 3, 4]`)
	if a.elevationDeg[3] != -10 {
		t.Errorf("a later snapshot overwrote the elevations: got %v", a.elevationDeg[3])
	}
}
