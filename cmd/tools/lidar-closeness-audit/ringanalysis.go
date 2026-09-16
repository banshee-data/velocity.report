package main

// Per-ring breakdown of where the high-spread cells are.
//
// The closeness audit shows a p90 spread tail of 4-10 m at 50-100 m, which is
// large enough to widen a cell's own acceptance window past a vehicle. Three
// explanations fit the aggregate equally well, and they call for different
// responses:
//
//   - Sky and above-horizon rings, whose beams never strike a surface at a
//     consistent range. Nothing to fix: those cells cannot report traffic
//     anyway.
//   - Vegetation, which genuinely moves. The spread is honest, and a wide
//     window there is doing its job.
//   - A feedback loop, where a cell that repeatedly sees traffic learns a wide
//     spread from the traffic itself, and the wide spread then makes it less
//     able to report the next vehicle. That one is a defect.
//
// Ring elevation separates the first from the rest, and RecentForegroundCount —
// how many consecutive frames the cell has just been reporting foreground —
// separates the third: a cell whose spread came from traffic is a cell that has
// been seeing traffic.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// highSpreadMetres is the spread above which a cell's own learned variance,
// rather than the range-proportional model, dominates its acceptance window.
// At the shipped multiplier of 3 this contributes over 3 m on its own.
const highSpreadMetres = 1.0

// ringAccumulator collects per-ring cell statistics across every snapshot.
type ringAccumulator struct {
	elevationDeg []float64 // by ring index; NaN where unknown
	rings        []ringCells
	azimuthBins  int
	// allRanges is every settled cell's range, so the near-field share of the
	// population is visible rather than implied.
	allRanges []float64
	high      []highSpreadCell
}

// highSpreadCell retains just enough about one high-spread cell to cross
// tabulate where they sit, which the per-ring medians cannot show: a ring
// aggregates every range, and most cells in every ring are near-field.
type highSpreadCell struct {
	rangeMetres  float64
	elevationDeg float64
	foreground   bool
}

type ringCells struct {
	ranges     []float64
	spreads    []float64
	foreground []int // RecentForegroundCount
	highSpread int
	// highSpreadWithForeground counts the high-spread cells that are also
	// actively reporting foreground: the feedback-loop signature.
	highSpreadWithForeground int
	locked                   int
}

func newRingAccumulator(rings, azimuthBins int) *ringAccumulator {
	a := &ringAccumulator{
		elevationDeg: make([]float64, rings),
		rings:        make([]ringCells, rings),
		azimuthBins:  azimuthBins,
	}
	for i := range a.elevationDeg {
		a.elevationDeg[i] = math.NaN()
	}
	return a
}

// setElevations records per-ring elevations from a snapshot, if it carries them.
// Snapshots for one sensor agree, so the first one to supply them wins.
func (a *ringAccumulator) setElevations(elevationsJSON string) {
	if elevationsJSON == "" || !math.IsNaN(a.elevationDeg[0]) {
		return
	}
	var elevations []float64
	if err := json.Unmarshal([]byte(elevationsJSON), &elevations); err != nil {
		return
	}
	if len(elevations) != len(a.elevationDeg) {
		return
	}
	copy(a.elevationDeg, elevations)
}

// observe records one settled cell. cellIndex is its position in the grid's
// flat cell slice, which BackgroundGrid.Idx builds as ring*azimuthBins+azBin.
func (a *ringAccumulator) observe(cellIndex int, c l3grid.BackgroundCell, lockedThreshold uint32) {
	// cellIndex is checked before the division, not after: Go truncates toward
	// zero, so any index in (-azimuthBins, 0) divides to ring 0 and would pass
	// a check on the quotient.
	if a.azimuthBins <= 0 || cellIndex < 0 {
		return
	}
	ring := cellIndex / a.azimuthBins
	if ring >= len(a.rings) {
		return
	}

	r := &a.rings[ring]
	spread := float64(c.RangeSpreadMeters)
	r.ranges = append(r.ranges, float64(c.AverageRangeMeters))
	r.spreads = append(r.spreads, spread)
	r.foreground = append(r.foreground, int(c.RecentForegroundCount))

	a.allRanges = append(a.allRanges, float64(c.AverageRangeMeters))
	if spread > highSpreadMetres {
		r.highSpread++
		if c.RecentForegroundCount > 0 {
			r.highSpreadWithForeground++
		}
		a.high = append(a.high, highSpreadCell{
			rangeMetres:  float64(c.AverageRangeMeters),
			elevationDeg: a.elevationDeg[ring],
			foreground:   c.RecentForegroundCount > 0,
		})
	}
	if c.LockedBaseline > 0 && c.LockedAtCount >= lockedThreshold {
		r.locked++
	}
}

// reportRings prints the per-ring breakdown and the attribution summary.
func (a *ringAccumulator) report() {
	fmt.Printf("\nPer-ring breakdown. \"high spr\" is the share of cells whose learned\n")
	fmt.Printf("spread exceeds %.0f m; \"+fg\" is the share of those also reporting\n", highSpreadMetres)
	fmt.Printf("foreground right now, which is the feedback-loop signature.\n\n")
	fmt.Printf("  %5s %8s %10s %9s %9s %9s %8s %8s\n",
		"ring", "elev", "cells", "rng p50", "spr p50", "spr p90", "high spr", "+fg")

	for ring := range a.rings {
		r := &a.rings[ring]
		if len(r.spreads) == 0 {
			continue
		}
		sort.Float64s(r.ranges)
		sort.Float64s(r.spreads)

		highShare := 100 * float64(r.highSpread) / float64(len(r.spreads))
		fgShare := math.NaN()
		if r.highSpread > 0 {
			fgShare = 100 * float64(r.highSpreadWithForeground) / float64(r.highSpread)
		}

		fmt.Printf("  %5d %7.2f%s %10d %9.2f %9.3f %9.3f %7.1f%% %7.1f%%\n",
			ring, a.elevationDeg[ring], "d", len(r.spreads),
			percentile(r.ranges, 50), percentile(r.spreads, 50), percentile(r.spreads, 90),
			highShare, fgShare)
	}

	a.attribute()
}

// attribute groups the high-spread cells by the explanation their ring supports.
func (a *ringAccumulator) attribute() {
	var aboveHorizon, belowHorizon, unknownElevation int
	var aboveFG, belowFG int
	var totalHigh, totalCells int

	for ring := range a.rings {
		r := &a.rings[ring]
		totalCells += len(r.spreads)
		totalHigh += r.highSpread

		elevation := a.elevationDeg[ring]
		switch {
		case math.IsNaN(elevation):
			unknownElevation += r.highSpread
		case elevation > 0:
			aboveHorizon += r.highSpread
			aboveFG += r.highSpreadWithForeground
		default:
			belowHorizon += r.highSpread
			belowFG += r.highSpreadWithForeground
		}
	}

	if totalHigh == 0 {
		fmt.Printf("\nNo cells exceed the %.0f m spread threshold.\n", highSpreadMetres)
		return
	}

	share := func(n int) float64 { return 100 * float64(n) / float64(totalHigh) }

	fmt.Printf("\nAttribution of the %d high-spread cells (%.2f%% of %d settled):\n\n",
		totalHigh, 100*float64(totalHigh)/float64(totalCells), totalCells)
	fmt.Printf("  above horizon (elev > 0)   : %9d  %5.1f%%   of which reporting fg: %5.1f%%\n",
		aboveHorizon, share(aboveHorizon), pctOf(aboveFG, aboveHorizon))
	fmt.Printf("  below horizon (elev <= 0)  : %9d  %5.1f%%   of which reporting fg: %5.1f%%\n",
		belowHorizon, share(belowHorizon), pctOf(belowFG, belowHorizon))
	if unknownElevation > 0 {
		fmt.Printf("  elevation unknown          : %9d  %5.1f%%\n",
			unknownElevation, share(unknownElevation))
	}

	fmt.Printf("\nA high-spread cell above the horizon cannot be a traffic feedback loop:\n")
	fmt.Printf("its beam does not strike the roadway. One below the horizon that is also\n")
	fmt.Printf("reporting foreground is the case worth pursuing, since its spread and the\n")
	fmt.Printf("traffic it is failing to report have the same cause.\n")

	a.crossTabulate()
}

// crossTabulate places the high-spread cells in range, and shows what share of
// the whole population sits at each range. A ring's medians hide this: every
// ring contains both near-field clutter and whatever it sees at distance.
func (a *ringAccumulator) crossTabulate() {
	if len(a.high) == 0 {
		return
	}
	sort.Float64s(a.allRanges)

	fmt.Printf("\nWhere the high-spread cells sit in range, against where all cells sit:\n\n")
	fmt.Printf("  %-11s %12s %12s %12s %12s\n",
		"band", "all cells", "high spread", "high/band", "of which fg")

	for _, b := range bands {
		var all, high, highFG int
		for _, r := range a.allRanges {
			if r >= b.lo && r < b.hi {
				all++
			}
		}
		for _, c := range a.high {
			if c.rangeMetres >= b.lo && c.rangeMetres < b.hi {
				high++
				if c.foreground {
					highFG++
				}
			}
		}
		if all == 0 {
			continue
		}
		fmt.Printf("  %-11s %11.1f%% %11.1f%% %11.2f%% %11.1f%%\n",
			b.name,
			100*float64(all)/float64(len(a.allRanges)),
			100*float64(high)/float64(len(a.high)),
			100*float64(high)/float64(all),
			pctOf(highFG, high))
	}

	// The same split, restricted to below-horizon cells, since those are the
	// only ones whose beams can reach the roadway at all.
	fmt.Printf("\nBelow-horizon high-spread cells only, by range:\n\n")
	fmt.Printf("  %-11s %12s %12s\n", "band", "count", "of which fg")
	for _, b := range bands {
		var n, fg int
		for _, c := range a.high {
			if c.elevationDeg > 0 || math.IsNaN(c.elevationDeg) {
				continue
			}
			if c.rangeMetres >= b.lo && c.rangeMetres < b.hi {
				n++
				if c.foreground {
					fg++
				}
			}
		}
		if n == 0 {
			continue
		}
		fmt.Printf("  %-11s %12d %11.1f%%\n", b.name, n, pctOf(fg, n))
	}
}

func pctOf(n, total int) float64 {
	if total == 0 {
		return math.NaN()
	}
	return 100 * float64(n) / float64(total)
}
