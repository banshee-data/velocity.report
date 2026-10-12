package l3grid

import (
	"math"
	"testing"
)

// Gap B8 in data/maths/paper-implementation-gap-analysis.md. When settling
// completes, cells are grouped into regions by variance tercile and each
// region fixes its own NoiseRelativeFraction, NeighbourConfirmationCount and
// update fraction from the config in force at that moment. effectiveCellParams
// then serves those values to the classifier in place of the globals, so:
//
//   - the noise fraction a settled cell runs at is 3x, 4x or 8x the configured
//     one, not the configured one, and
//   - a runtime change to any of the three globals never reaches a settled
//     cell, which is why the campaign's tracker-side sweeps of noise_relative,
//     neighbour_confirmation_count and background_update_fraction moved nothing
//     while closeness_multiplier, which has no override, moved labelled recall
//     from 4 to 13 of 16.
//
// These tests pin the shipped behaviour. DisableRegionOverrides is the
// default-off switch that lets the two be measured against each other.

// settledThreeZoneGrid is a grid whose rings fall into the three variance
// terciles, with regions identified against the given global noise fraction.
func settledThreeZoneGrid(t *testing.T, noiseRel float32) *BackgroundGrid {
	t.Helper()
	const rings, azBins = 6, 8
	grid := makeTestGrid(rings, azBins)
	grid.Params.NoiseRelativeFraction = noiseRel
	grid.Params.NeighbourConfirmationCount = 3
	grid.Params.BackgroundUpdateFraction = 0.02
	rm := grid.RegionMgr
	for ring := 0; ring < rings; ring++ {
		spread := float32(0.1) // stable
		if ring >= 4 {
			spread = 1.5 // volatile
		} else if ring >= 2 {
			spread = 0.5 // variable
		}
		for az := 0; az < azBins; az++ {
			idx := grid.Idx(ring, az)
			grid.Cells[idx].TimesSeenCount = 10
			grid.Cells[idx].AverageRangeMeters = 10
			grid.Cells[idx].RangeSpreadMeters = spread
			rm.SettlingMetrics.VariancePerCell[idx] = float64(spread)
		}
	}
	rm.SettlingMetrics.FramesSampled = 20
	if err := rm.IdentifyRegions(grid, 50); err != nil {
		t.Fatalf("IdentifyRegions: %v", err)
	}
	return grid
}

// effectiveNoise is the noise fraction the classifier would use for a cell
// given the grid's current global parameters.
func effectiveNoise(g *BackgroundGrid, ring int) float64 {
	noise, _, _ := g.effectiveCellParams(g.Idx(ring, 0),
		float64(g.Params.NoiseRelativeFraction), g.Params.NeighbourConfirmationCount,
		float64(g.Params.BackgroundUpdateFraction))
	return noise
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestSettledCellsRunAtAMultipleOfTheConfiguredNoiseFraction(t *testing.T) {
	g := settledThreeZoneGrid(t, 0.02)
	for _, tc := range []struct {
		name string
		ring int
		want float64
	}{
		{"most stable third", 0, 0.02 * 4},
		{"middle third", 2, 0.02 * 3},
		{"most variable third", 4, 0.02 * 8},
	} {
		if got := effectiveNoise(g, tc.ring); !approx(got, tc.want) {
			t.Errorf("%s: effective noise fraction %.4f, want %.4f. This test pins the shipped region "+
				"multipliers; if they changed, update gap analysis B8 and HW1's audit with them", tc.name, got, tc.want)
		}
	}
}

func TestRuntimeNoiseChangeDoesNotReachSettledCells(t *testing.T) {
	g := settledThreeZoneGrid(t, 0.02)
	before := effectiveNoise(g, 0)

	// What SetNoiseRelativeFraction and the runtime tuning endpoint do.
	g.Params.NoiseRelativeFraction = 0.005

	if got := effectiveNoise(g, 0); !approx(got, before) {
		t.Fatalf("effective noise fraction followed the runtime change (%.4f -> %.4f). This test pins B8: "+
			"if the override now tracks the global, the campaign's void tracker-side sweeps can be rerun", before, got)
	}
}

func TestDisableRegionOverridesServesTheGlobals(t *testing.T) {
	g := settledThreeZoneGrid(t, 0.02)
	g.Params.DisableRegionOverrides = true
	for ring := 0; ring < 6; ring += 2 {
		if got := effectiveNoise(g, ring); !approx(got, 0.02) {
			t.Errorf("ring %d: effective noise fraction %.4f with overrides disabled, want the global 0.02", ring, got)
		}
	}

	g.Params.NoiseRelativeFraction = 0.005
	if got := effectiveNoise(g, 0); !approx(got, 0.005) {
		t.Errorf("effective noise fraction %.4f after a runtime change with overrides disabled, want 0.005", got)
	}

	_, neighbours, alpha := g.effectiveCellParams(g.Idx(4, 0), 0.02, 3, 0.02)
	if neighbours != 3 || !approx(alpha, 0.02) {
		t.Errorf("volatile region still overrides neighbours=%d alpha=%.4f with overrides disabled, want 3 and 0.02",
			neighbours, alpha)
	}
}

func TestDisableRegionOverridesOffByDefault(t *testing.T) {
	if makeTestGrid(2, 4).Params.DisableRegionOverrides {
		t.Fatal("DisableRegionOverrides defaults on; the shipped behaviour must stay the default until it is measured")
	}
}

func TestRegionOverrideScopeSplitsTheTerciles(t *testing.T) {
	g := settledThreeZoneGrid(t, 0.02)

	g.Params.RegionOverrideScope = RegionOverrideScopeVolatile
	for _, ring := range []int{0, 2} {
		noise, neighbours, alpha := g.effectiveCellParams(g.Idx(ring, 0), 0.02, 3, 0.02)
		if !approx(noise, 0.02) || neighbours != 3 || !approx(alpha, 0.02) {
			t.Errorf("volatile scope, ring %d (stable or variable): noise %.4f neighbours %d alpha %.4f, want the globals 0.02, 3, 0.02",
				ring, noise, neighbours, alpha)
		}
	}
	noise, neighbours, alpha := g.effectiveCellParams(g.Idx(4, 0), 0.02, 3, 0.02)
	if !approx(noise, 0.16) || neighbours != 5 || !approx(alpha, 0.01) {
		t.Errorf("volatile scope, ring 4 (volatile): noise %.4f neighbours %d alpha %.4f, want the overrides 0.16, 5, 0.01",
			noise, neighbours, alpha)
	}

	g.Params.RegionOverrideScope = RegionOverrideScopeQuiet
	if got := effectiveNoise(g, 0); !approx(got, 0.08) {
		t.Errorf("quiet scope, stable ring: effective noise %.4f, want the x4 override 0.08", got)
	}
	if got := effectiveNoise(g, 2); !approx(got, 0.06) {
		t.Errorf("quiet scope, variable ring: effective noise %.4f, want the x3 override 0.06", got)
	}
	noise, neighbours, alpha = g.effectiveCellParams(g.Idx(4, 0), 0.02, 3, 0.02)
	if !approx(noise, 0.02) || neighbours != 3 || !approx(alpha, 0.02) {
		t.Errorf("quiet scope, ring 4 (volatile): noise %.4f neighbours %d alpha %.4f, want the globals 0.02, 3, 0.02",
			noise, neighbours, alpha)
	}

	g.Params.RegionOverrideScope = RegionOverrideScopeVolatile
	g.Params.DisableRegionOverrides = true
	if got := effectiveNoise(g, 4); !approx(got, 0.02) {
		t.Errorf("volatile ring with overrides disabled and the volatile scope: effective noise %.4f, want the global 0.02 (disable wins)", got)
	}
}

func TestRegionOverrideScopeAfterRestore(t *testing.T) {
	g := settledThreeZoneGrid(t, 0.02)
	for _, r := range g.RegionMgr.Regions {
		r.Volatile = false // a restored region carries no tercile
	}
	g.Params.RegionOverrideScope = RegionOverrideScopeVolatile
	if got := effectiveNoise(g, 4); !approx(got, 0.02) {
		t.Errorf("volatile scope, restored volatile ring: effective noise %.4f, want the global 0.02", got)
	}
}

func TestRegionOverrideScopeAllByDefault(t *testing.T) {
	if makeTestGrid(2, 4).Params.RegionOverrideScope != RegionOverrideScopeAll {
		t.Fatal("RegionOverrideScope defaults to a partial scope; the shipped behaviour must stay the default until it is measured")
	}
}
