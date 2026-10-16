package l3grid

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// Experiment l3bg0010: the two levers the region map report names.
// RegionOverridesByCellTercile serves each cell the overrides of its own
// variance tercile rather than its merged region's, and
// VolatileWindowCapMetres bounds a volatile override's acceptance windows in
// metres. Both are default off; region_override_runtime_test.go pins the
// shipped behaviour they are measured against.

// mergedIslandGrid is a settled 6x8 grid whose rings fall into the three
// terciles (0-1 stable, 2-3 variable, 4-5 volatile) with two one-cell islands
// that merging moves into a region of another tercile, and one cell never
// seen while the grid settled:
//
//   - (0,3) has a volatile spread inside the stable rings and merges into the
//     stable region;
//   - (4,5) has a stable spread inside the volatile rings and merges into the
//     volatile region;
//   - (1,7) is unseen, so it has neither a region nor a tercile.
//
// That leaves 15 stable, 16 variable and 16 volatile cells, so the tercile
// cut points fall on the spreads' own boundaries, and 5 connected
// components, which maxRegions 3 reduces by merging the two islands.
func mergedIslandGrid(t *testing.T) *BackgroundGrid {
	t.Helper()
	const rings, azBins = 6, 8
	grid := makeTestGrid(rings, azBins)
	grid.Params.NoiseRelativeFraction = 0.02
	grid.Params.NeighbourConfirmationCount = 3
	grid.Params.BackgroundUpdateFraction = 0.02
	rm := grid.RegionMgr
	for ring := 0; ring < rings; ring++ {
		for az := 0; az < azBins; az++ {
			spread := float32(0.1) // stable
			if ring >= 4 {
				spread = 1.5 // volatile
			} else if ring >= 2 {
				spread = 0.5 // variable
			}
			switch {
			case ring == 0 && az == 3:
				spread = 1.5
			case ring == 4 && az == 5:
				spread = 0.1
			}
			idx := grid.Idx(ring, az)
			if ring == 1 && az == 7 {
				continue // unseen
			}
			grid.Cells[idx].TimesSeenCount = 10
			grid.Cells[idx].AverageRangeMeters = 10
			grid.Cells[idx].RangeSpreadMeters = spread
			rm.SettlingMetrics.VariancePerCell[idx] = float64(spread)
		}
	}
	rm.SettlingMetrics.FramesSampled = 20
	if err := rm.IdentifyRegions(grid, 3); err != nil {
		t.Fatalf("IdentifyRegions: %v", err)
	}
	return grid
}

// regionParamsOfTercile returns the parameters of a region identified from
// the given tercile, the figures RegionOverridesByCellTercile must reproduce.
func regionParamsOfTercile(t *testing.T, rm *RegionManager, tercile VarianceTercile) RegionParams {
	t.Helper()
	for _, r := range rm.Regions {
		if r.Tercile == tercile {
			return r.Params
		}
	}
	t.Fatalf("no region of tercile %s", tercile)
	return RegionParams{}
}

type servedParams struct {
	noise      float64
	neighbours int
	alpha      float64
	volatile   bool
}

func served(g *BackgroundGrid, ring, az int) servedParams {
	noise, neighbours, alpha, volatile := g.effectiveCellParams(g.Idx(ring, az), 0.02, 3, 0.02)
	return servedParams{noise, neighbours, alpha, volatile}
}

func servedFrom(p RegionParams, volatile bool) servedParams {
	return servedParams{float64(p.NoiseRelativeFraction), p.NeighbourConfirmationCount, float64(p.SettleUpdateFraction), volatile}
}

func (s servedParams) equal(o servedParams) bool {
	return approx(s.noise, o.noise) && s.neighbours == o.neighbours && approx(s.alpha, o.alpha) && s.volatile == o.volatile
}

func TestMergedIslandGridHasTheTercilesItClaims(t *testing.T) {
	g := mergedIslandGrid(t)
	rm := g.RegionMgr
	for _, c := range []struct {
		ring, az     int
		own, region  VarianceTercile
		regionAssign bool
	}{
		{0, 3, TercileVolatile, TercileStable, true},
		{4, 5, TercileStable, TercileVolatile, true},
		{0, 0, TercileStable, TercileStable, true},
		{2, 0, TercileVariable, TercileVariable, true},
		{4, 0, TercileVolatile, TercileVolatile, true},
		{1, 7, TercileNone, TercileNone, false},
	} {
		idx := g.Idx(c.ring, c.az)
		if got := rm.GetCellTercile(idx); got != c.own {
			t.Errorf("cell (%d,%d): own tercile %s, want %s", c.ring, c.az, got, c.own)
		}
		regionID := rm.GetRegionForCell(idx)
		if !c.regionAssign {
			if regionID != -1 {
				t.Errorf("cell (%d,%d): region %d, want none", c.ring, c.az, regionID)
			}
			continue
		}
		if regionID < 0 || rm.Regions[regionID].Tercile != c.region {
			t.Fatalf("cell (%d,%d): region %d of tercile %v, want tercile %s; the fixture no longer merges as described",
				c.ring, c.az, regionID, rm.Regions[regionID].Tercile, c.region)
		}
	}
}

// The parameters a cell gets from its own tercile are exactly those
// identification gave the regions of that tercile, read from the regions, not
// restated.
func TestTercileParamsAreTheRegionsOwn(t *testing.T) {
	g := mergedIslandGrid(t)
	rm := g.RegionMgr
	for _, tercile := range []VarianceTercile{TercileStable, TercileVariable, TercileVolatile} {
		got := rm.GetTercileParams(tercile)
		if got == nil {
			t.Fatalf("no parameters for tercile %s", tercile)
		}
		if want := regionParamsOfTercile(t, rm, tercile); *got != want {
			t.Errorf("tercile %s: %+v, regions of that tercile run at %+v", tercile, *got, want)
		}
	}
	if rm.GetTercileParams(TercileNone) != nil {
		t.Error("TercileNone has parameters")
	}
	for _, r := range rm.Regions {
		if want := rm.GetTercileParams(r.Tercile); want == nil || r.Params != *want {
			t.Errorf("region %d (%s) runs at %+v, not its tercile's", r.ID, r.Tercile, r.Params)
		}
	}
}

func TestRegionOverridesByCellTercileServeTheCellsOwnTercile(t *testing.T) {
	g := mergedIslandGrid(t)
	rm := g.RegionMgr
	stable := servedFrom(regionParamsOfTercile(t, rm, TercileStable), false)
	variable := servedFrom(regionParamsOfTercile(t, rm, TercileVariable), false)
	volatile := servedFrom(regionParamsOfTercile(t, rm, TercileVolatile), true)
	globals := servedParams{0.02, 3, 0.02, false}

	type cell struct{ ring, az int }
	islandVolatile, islandStable := cell{0, 3}, cell{4, 5}
	plainStable, plainVariable, plainVolatile, unseen := cell{0, 0}, cell{2, 0}, cell{4, 0}, cell{1, 7}

	for _, arm := range []struct {
		name    string
		byCell  bool
		scope   RegionOverrideScope
		disable bool
		want    map[cell]servedParams
	}{
		{"region path (shipped)", false, RegionOverrideScopeAll, false, map[cell]servedParams{
			islandVolatile: stable, islandStable: volatile,
			plainStable: stable, plainVariable: variable, plainVolatile: volatile, unseen: globals}},
		{"cell tercile", true, RegionOverrideScopeAll, false, map[cell]servedParams{
			islandVolatile: volatile, islandStable: stable,
			plainStable: stable, plainVariable: variable, plainVolatile: volatile, unseen: globals}},
		{"cell tercile, volatile scope", true, RegionOverrideScopeVolatile, false, map[cell]servedParams{
			islandVolatile: volatile, islandStable: globals,
			plainStable: globals, plainVariable: globals, plainVolatile: volatile, unseen: globals}},
		{"cell tercile, quiet scope", true, RegionOverrideScopeQuiet, false, map[cell]servedParams{
			islandVolatile: globals, islandStable: stable,
			plainStable: stable, plainVariable: variable, plainVolatile: globals, unseen: globals}},
		{"region path, volatile scope", false, RegionOverrideScopeVolatile, false, map[cell]servedParams{
			islandVolatile: globals, islandStable: volatile,
			plainStable: globals, plainVariable: globals, plainVolatile: volatile, unseen: globals}},
		{"region path, quiet scope", false, RegionOverrideScopeQuiet, false, map[cell]servedParams{
			islandVolatile: stable, islandStable: globals,
			plainStable: stable, plainVariable: variable, plainVolatile: globals, unseen: globals}},
		{"cell tercile with overrides disabled", true, RegionOverrideScopeAll, true, map[cell]servedParams{
			islandVolatile: globals, islandStable: globals,
			plainStable: globals, plainVariable: globals, plainVolatile: globals, unseen: globals}},
	} {
		g.Params.RegionOverridesByCellTercile = arm.byCell
		g.Params.RegionOverrideScope = arm.scope
		g.Params.DisableRegionOverrides = arm.disable
		for c, want := range arm.want {
			if got := served(g, c.ring, c.az); !got.equal(want) {
				t.Errorf("%s, cell (%d,%d): served %+v, want %+v", arm.name, c.ring, c.az, got, want)
			}
		}
	}
}

// Without merging every cell's own tercile is its region's, so the two paths
// serve identical parameters to every cell.
func TestRegionOverridesByCellTercileMatchTheRegionPathWithoutMerging(t *testing.T) {
	g := settledThreeZoneGrid(t, 0.02)
	for idx := range g.Cells {
		regionNoise, regionNeighbours, regionAlpha, regionVolatile := g.effectiveCellParams(idx, 0.02, 3, 0.02)
		g.Params.RegionOverridesByCellTercile = true
		noise, neighbours, alpha, volatile := g.effectiveCellParams(idx, 0.02, 3, 0.02)
		g.Params.RegionOverridesByCellTercile = false
		if noise != regionNoise || neighbours != regionNeighbours || alpha != regionAlpha || volatile != regionVolatile {
			t.Errorf("cell %d: cell path %v %d %v %v, region path %v %d %v %v",
				idx, noise, neighbours, alpha, volatile, regionNoise, regionNeighbours, regionAlpha, regionVolatile)
		}
	}
}

// Like a region's, a tercile's parameters are fixed when regions are
// identified; a later change to the global does not reach them.
func TestTercileParamsAreFixedAtIdentification(t *testing.T) {
	g := mergedIslandGrid(t)
	g.Params.RegionOverridesByCellTercile = true
	before := served(g, 0, 3)
	g.Params.NoiseRelativeFraction = 0.005
	if after := served(g, 0, 3); !after.equal(before) {
		t.Errorf("a cell's own-tercile parameters followed a runtime change: %+v -> %+v", before, after)
	}
}

// A restored region manager records no cell terciles, so the option serves
// every cell the globals, while the region path keeps the restored regions'.
func TestRegionOverridesByCellTercileAfterRestore(t *testing.T) {
	g := mergedIslandGrid(t)
	snap := g.RegionMgr.ToSnapshot(g.SensorID, 1)
	if snap == nil {
		t.Fatal("no snapshot")
	}
	restored := NewRegionManager(g.Rings, g.AzimuthBins)
	if err := restored.RestoreFromSnapshot(snap, len(g.Cells)); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.CellTercile != nil || restored.TercileParams != nil {
		t.Fatal("a restore carried cell terciles or tercile parameters it did not identify")
	}
	stableRegion := servedFrom(regionParamsOfTercile(t, g.RegionMgr, TercileStable), false)
	g.RegionMgr = restored
	g.Params.RegionOverridesByCellTercile = true
	if got := served(g, 0, 0); !got.equal(servedParams{0.02, 3, 0.02, false}) {
		t.Errorf("restored grid, cell tercile option: served %+v, want the globals", got)
	}
	g.Params.RegionOverridesByCellTercile = false
	if got := served(g, 0, 0); !got.equal(stableRegion) {
		t.Errorf("restored grid, region path: served %+v, want the restored region's %+v", got, stableRegion)
	}
}

func TestRegionLeversOffByDefault(t *testing.T) {
	p := makeTestGrid(2, 4).Params
	if p.RegionOverridesByCellTercile || p.VolatileWindowCapMetres != 0 {
		t.Fatal("an l3bg0010 lever defaults on; the shipped behaviour must stay the default until it is measured")
	}
	bm := NewBackgroundManager(t.Name(), 2, 4, BackgroundParams{}, nil)
	if p := bm.GetParams(); p.RegionOverridesByCellTercile || p.VolatileWindowCapMetres != 0 {
		t.Fatal("a new manager starts with an l3bg0010 lever on")
	}
}

func TestSetRegionLevers(t *testing.T) {
	var nilManager *BackgroundManager
	if err := nilManager.SetRegionOverridesByCellTercile(true); err == nil {
		t.Error("a nil manager accepted the cell tercile option")
	}
	if err := nilManager.SetVolatileWindowCapMetres(1); err == nil {
		t.Error("a nil manager accepted a window cap")
	}
	bm := NewBackgroundManager(t.Name(), 2, 4, BackgroundParams{}, nil)
	if err := bm.SetRegionOverridesByCellTercile(true); err != nil || !bm.Grid.Params.RegionOverridesByCellTercile {
		t.Fatalf("cell tercile option not set: %v", err)
	}
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1)} {
		if err := bm.SetVolatileWindowCapMetres(bad); err == nil {
			t.Errorf("cap %v accepted", bad)
		}
	}
	for _, v := range []float64{2, 1, 0} {
		if err := bm.SetVolatileWindowCapMetres(v); err != nil || bm.Grid.Params.VolatileWindowCapMetres != v {
			t.Errorf("cap %v: %v, grid reads %v", v, err, bm.Grid.Params.VolatileWindowCapMetres)
		}
	}
}

func TestCappedOverrideWindowMetres(t *testing.T) {
	for _, c := range []struct {
		window, global, capMetres, want float64
	}{
		{5.0, 1.0, 2.0, 2.0},  // capped at the cap
		{5.0, 3.0, 2.0, 3.0},  // never below the global window
		{1.5, 1.0, 2.0, 1.5},  // narrower than the bound: unchanged
		{0.8, 1.0, 2.0, 0.8},  // never widened, even below the global
		{5.0, 1.0, 0.0, 5.0},  // zero is off
		{5.0, 1.0, -1.0, 5.0}, // as is a negative cap
	} {
		if got := CappedOverrideWindowMetres(c.window, c.global, c.capMetres); !approx(got, c.want) {
			t.Errorf("CappedOverrideWindowMetres(%v, %v, %v) = %v, want %v", c.window, c.global, c.capMetres, got, c.want)
		}
	}
}

// probePath selects which of the classifier's three acceptance routes is the
// only one that can call a probe return background.
type probePath int

const (
	probeCloseness  probePath = iota // the cell's own EMA average
	probeLocked                      // the cell's locked baseline
	probeNeighbours                  // same-ring neighbour confirmation
)

func (p probePath) String() string {
	return [...]string{"closeness", "locked", "neighbours"}[p]
}

// windowProbe is a settled one-ring grid at the shipped L3 window parameters
// (config/tuning.defaults.json) whose cell at azimuth bin 0 has a region of
// regionTercile, its own tercile cellTercile, and its background at rangeM
// with spread metres of spread, reachable only through path.
type windowProbe struct {
	regionTercile VarianceTercile
	cellTercile   VarianceTercile
	byCell        bool
	path          probePath
	rangeM        float64
	spread        float64
	capMetres     float64
}

const (
	probeBins       = 32
	probeMultiplier = 3.0
	probeSafety     = 0.15
	probeNoise      = 0.02
	probeLockedMult = 4.0
)

func (w windowProbe) grid() *BackgroundGrid {
	cells := make([]BackgroundCell, probeBins)
	g := &BackgroundGrid{
		SensorID: "window-probe", Rings: 1, AzimuthBins: probeBins, Cells: cells,
		Params: BackgroundParams{
			BackgroundUpdateFraction:       0.02,
			ClosenessSensitivityMultiplier: probeMultiplier,
			SafetyMarginMetres:             probeSafety,
			NoiseRelativeFraction:          probeNoise,
			NeighbourConfirmationCount:     3,
			FreezeDurationNanos:            int64(5 * time.Second),
			FreezeThresholdMultiplier:      3,
			LockedBaselineThreshold:        50,
			LockedBaselineMultiplier:       probeLockedMult,
			RegionOverridesByCellTercile:   w.byCell,
			VolatileWindowCapMetres:        w.capMetres,
		},
		SettlingComplete: true,
	}
	g.Manager = &BackgroundManager{Grid: g}

	rm := NewRegionManager(1, probeBins)
	rm.TercileParams = make([]RegionParams, 3)
	for category := range rm.TercileParams {
		rm.TercileParams[category] = rm.assignRegionParams(category, g.Params)
	}
	all := make([]int, probeBins)
	rm.CellTercile = make([]VarianceTercile, probeBins)
	for i := range all {
		all[i] = i
		rm.CellToRegionID[i] = 0
		rm.CellTercile[i] = w.cellTercile
	}
	rm.Regions = []*Region{{
		ID: 0, CellList: all, CellCount: probeBins,
		Params:   rm.TercileParams[int(w.regionTercile)-1],
		Tercile:  w.regionTercile,
		Volatile: w.regionTercile == TercileVolatile,
	}}
	rm.IdentificationComplete = true
	g.RegionMgr = rm

	target := &g.Cells[0]
	*target = BackgroundCell{AverageRangeMeters: float32(w.rangeM + 100), RangeSpreadMeters: float32(w.spread), TimesSeenCount: 120}
	switch w.path {
	case probeCloseness:
		target.AverageRangeMeters = float32(w.rangeM)
	case probeLocked:
		target.LockedBaseline = float32(w.rangeM)
		target.LockedSpread = float32(w.spread)
		target.LockedAtCount = 120
	case probeNeighbours:
		// Five on each side: enough for the volatile override's count.
		for d := 1; d <= 5; d++ {
			for _, bin := range []int{d, probeBins - d} {
				g.Cells[bin] = BackgroundCell{AverageRangeMeters: float32(w.rangeM), RangeSpreadMeters: float32(w.spread), TimesSeenCount: 120}
			}
		}
	}
	return g
}

// foreground classifies one return diff metres beyond the background.
func (w windowProbe) foreground(t *testing.T, diff float64) bool {
	t.Helper()
	g := w.grid()
	mask, err := g.Manager.ProcessFramePolarWithMaskAt([]PointPolar{{Channel: 1, Azimuth: 0, Distance: w.rangeM + diff}}, time.Unix(1000, 0))
	if err != nil || len(mask) != 1 {
		t.Fatalf("classify: %v %v", mask, err)
	}
	return mask[0]
}

// window is the acceptance window of path at observed range p for a noise
// fraction, before any cap.
func (w windowProbe) window(p, noise float64) float64 {
	switch w.path {
	case probeLocked:
		return LockedBaselineWindowMetres(probeLockedMult, w.spread, noise, p, probeSafety)
	case probeNeighbours:
		// The neighbour's window is at the neighbour's range, not the return's.
		return ClosenessThresholdMetres(probeMultiplier, w.spread, noise, w.rangeM, 0)
	}
	return ForegroundClosenessWindowMetres(probeMultiplier, w.spread, noise, p, probeSafety, 1)
}

// boundary is the diff at which a return leaves the window f(p): the root of
// d = f(rangeM + d), found by bisection (every window grows with range by
// less than one metre a metre).
func (w windowProbe) boundary(f func(p float64) float64) float64 {
	lo, hi := 0.0, 200.0
	for i := 0; i < 100; i++ {
		mid := (lo + hi) / 2
		if mid <= f(w.rangeM+mid) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}

func (w windowProbe) assertBoundary(t *testing.T, label string, want float64) {
	t.Helper()
	const margin = 0.01
	if w.foreground(t, want*(1-margin)) {
		t.Errorf("%s: a return %.3f m out (just inside the %.3f m window) is foreground", label, want*(1-margin), want)
	}
	if !w.foreground(t, want*(1+margin)) {
		t.Errorf("%s: a return %.3f m out (just outside the %.3f m window) is background", label, want*(1+margin), want)
	}
}

func TestVolatileWindowCapBoundsEachWindowInMetres(t *testing.T) {
	const spread = 0.02
	tercileNoise := func(tercile VarianceTercile) float64 {
		return float64(windowProbe{regionTercile: tercile, cellTercile: tercile}.grid().RegionMgr.TercileParams[int(tercile)-1].NoiseRelativeFraction)
	}
	volatileNoise := tercileNoise(TercileVolatile)
	if volatileNoise <= probeNoise {
		t.Fatalf("volatile override noise %v is not wider than the global %v; the fixture is wrong", volatileNoise, probeNoise)
	}
	for _, path := range []probePath{probeCloseness, probeLocked, probeNeighbours} {
		for _, rangeM := range []float64{10, 20, 35, 50} {
			for _, capMetres := range []float64{1, 2} {
				w := windowProbe{regionTercile: TercileVolatile, cellTercile: TercileVolatile,
					path: path, rangeM: rangeM, spread: spread, capMetres: capMetres}
				label := func(s string) string {
					return fmt.Sprintf("%s window at %g m, cap %g m: %s", path, rangeM, capMetres, s)
				}
				global := w.boundary(func(p float64) float64 { return w.window(p, probeNoise) })
				uncapped := w.boundary(func(p float64) float64 { return w.window(p, volatileNoise) })
				capped := w.boundary(func(p float64) float64 {
					return CappedOverrideWindowMetres(w.window(p, volatileNoise), w.window(p, probeNoise), capMetres)
				})
				if capped < global-1e-9 {
					t.Fatalf("%s", label("the capped window is below the global"))
				}
				if capped >= uncapped {
					t.Fatalf("%s", label("the cap does not bind; the probe would show nothing"))
				}
				w.assertBoundary(t, label("volatile, capped"), capped)

				// The same return is background without the cap: the cap is
				// what moved it.
				off := w
				off.capMetres = 0
				if off.foreground(t, capped*1.01) {
					t.Errorf("%s", label("without the cap a return just outside the capped window is already foreground"))
				}
				off.assertBoundary(t, label("volatile, no cap"), uncapped)

				// Quiet terciles keep their override windows under the cap.
				for _, tercile := range []VarianceTercile{TercileStable, TercileVariable} {
					q := w
					q.regionTercile, q.cellTercile = tercile, tercile
					noise := tercileNoise(tercile)
					q.assertBoundary(t, label(tercile.String()+" uncapped"),
						q.boundary(func(p float64) float64 { return q.window(p, noise) }))
				}
			}
		}
	}
}

// Under the cell tercile option the cap follows the cell's own tercile, not
// its region's.
func TestVolatileWindowCapFollowsTheCellsOwnTercile(t *testing.T) {
	const rangeM, capMetres, spread = 20.0, 1.0, 0.02
	volatileInStable := windowProbe{regionTercile: TercileStable, cellTercile: TercileVolatile, byCell: true,
		path: probeCloseness, rangeM: rangeM, spread: spread, capMetres: capMetres}
	g := volatileInStable.grid()
	volatileNoise := float64(g.RegionMgr.TercileParams[int(TercileVolatile)-1].NoiseRelativeFraction)
	stableNoise := float64(g.RegionMgr.TercileParams[int(TercileStable)-1].NoiseRelativeFraction)
	volatileInStable.assertBoundary(t, "own volatile in a stable region", volatileInStable.boundary(func(p float64) float64 {
		return CappedOverrideWindowMetres(volatileInStable.window(p, volatileNoise), volatileInStable.window(p, probeNoise), capMetres)
	}))

	stableInVolatile := volatileInStable
	stableInVolatile.regionTercile, stableInVolatile.cellTercile = TercileVolatile, TercileStable
	stableInVolatile.assertBoundary(t, "own stable in a volatile region", stableInVolatile.boundary(func(p float64) float64 {
		return stableInVolatile.window(p, stableNoise)
	}))

	// The region path caps by the region's tercile instead.
	regionPath := stableInVolatile
	regionPath.byCell = false
	regionPath.assertBoundary(t, "region path, volatile region", regionPath.boundary(func(p float64) float64 {
		return CappedOverrideWindowMetres(regionPath.window(p, volatileNoise), regionPath.window(p, probeNoise), capMetres)
	}))
}

// ProcessFramePolar, the per-cell aggregate classifier, applies the same cap
// to its closeness window. (Its neighbour test looks at one bin either side,
// so a volatile cell, which needs five, never confirms there.)
func TestVolatileWindowCapInTheAggregateClassifier(t *testing.T) {
	const rangeM, spread = 20.0, 0.02
	for _, c := range []struct {
		capMetres float64
		tercile   VarianceTercile
	}{{0, TercileVolatile}, {1, TercileVolatile}, {2, TercileVolatile}, {1, TercileStable}} {
		w := windowProbe{regionTercile: c.tercile, cellTercile: c.tercile, path: probeCloseness,
			rangeM: rangeM, spread: spread, capMetres: c.capMetres}
		noise := float64(w.grid().RegionMgr.TercileParams[int(c.tercile)-1].NoiseRelativeFraction)
		window := func(p, n float64) float64 {
			return ClosenessThresholdMetres(probeMultiplier, spread, n, p, probeSafety)
		}
		want := w.boundary(func(p float64) float64 { return window(p, noise) })
		if c.capMetres > 0 && c.tercile == TercileVolatile {
			want = w.boundary(func(p float64) float64 {
				return CappedOverrideWindowMetres(window(p, noise), window(p, probeNoise), c.capMetres)
			})
		}
		for _, d := range []float64{want * 0.99, want * 1.01} {
			g := w.grid()
			g.Manager.ProcessFramePolar([]PointPolar{{Channel: 1, Azimuth: 0, Distance: rangeM + d}})
			// A background observation is counted as seen; a foreground one
			// lowers the count.
			background := g.Cells[0].TimesSeenCount > 120
			if wantBackground := d < want; background != wantBackground {
				t.Errorf("%s cell, cap %v m: a return %.3f m out (window %.3f m) background=%v, want %v",
					c.tercile, c.capMetres, d, want, background, wantBackground)
			}
		}
	}
}
