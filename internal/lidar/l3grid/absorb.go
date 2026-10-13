package l3grid

import (
	"math"
	"time"
)

// AbsorbReturns makes an object's returns background: L3's half of the
// pipeline's dwell policy. L3 cannot tell a parked car from a skip, so an
// object that arrives after the background has settled can stay foreground
// for as long as the background is old (the beam-wise review, Section 5.1);
// L5 can tell, and the pipeline passes the returns of a track that has stood
// still long enough. Nothing else calls it.
//
// Each cell hit by at least one of the returns is reseeded:
//
//   - AverageRangeMeters becomes the mean of the returns' ranges, so the
//     object's next returns sit inside the cell's closeness window;
//   - RangeSpreadMeters keeps the spread the cell has learned, widened to
//     half the returns' span when they disagree. The spread is the beam's
//     measured noise, and the arrival never updated it, because a cell
//     learns only from returns it accepts;
//   - TimesSeenCount is kept: absorption tells the cell what it now sees,
//     not that it has seen it often. An empty cell is seeded at one, as the
//     first-observation path seeds it, so its window takes the warm-up ramp;
//   - the freeze is lifted and RecentForegroundCount cleared, as a thaw
//     does, so the next return is judged against the new average at once;
//   - a locked baseline in force is moved to the object, at the cell's
//     spread, as if the cell had learned the object; one not yet in force is
//     left to lock from the average when the count reaches the threshold.
//
// The lock is moved rather than kept on the surface behind the object, though
// keeping it would make the object's departure background at once. The lock's
// slow update follows every accepted return while no foreground is recent, and
// its spread is the running mean of |return - lock|: a cell accepting an object
// 7 m in front of its lock would widen that window to the whole range within
// a minute or two (about 3.6 to 21 m after 50 s, 0 to 21 m after 100 s, for a
// car at 8 m before a road at 15 m), and anything passing in front of the
// object would be background. The cost of moving it is the departure: the
// surface behind returns as a ghost that clears as any learned object's does,
// in seconds where the surface flanks it on the ring (neighbour confirmation)
// and by the drain where it does not, which is the departure half (E2) the
// review pairs with this policy.
//
// A return that maps to no cell (an invalid channel) or has no range is
// ignored. Returns are folded into their cells in the order given and each
// cell is written independently, so the result does not depend on map order.
// It returns the number of cells absorbed.
func (bm *BackgroundManager) AbsorbReturns(points []PointPolar, now time.Time) int {
	if bm == nil || bm.Grid == nil || len(points) == 0 {
		return 0
	}
	g := bm.Grid
	rings, azBins := g.Rings, g.AzimuthBins
	if rings <= 0 || azBins <= 0 || len(g.Cells) != rings*azBins {
		return 0
	}

	type fold struct {
		cell          int
		sum, min, max float64
		n             int
	}
	var folds []fold
	slot := make(map[int]int, len(points))
	for _, p := range points {
		if !(p.Distance > 0) || math.IsInf(p.Distance, 0) {
			continue
		}
		cell, ok := returnCell(rings, azBins, p)
		if !ok {
			continue
		}
		i, seen := slot[cell]
		if !seen {
			i = len(folds)
			slot[cell] = i
			folds = append(folds, fold{cell: cell, min: p.Distance, max: p.Distance})
		}
		f := &folds[i]
		f.sum += p.Distance
		f.n++
		f.min = math.Min(f.min, p.Distance)
		f.max = math.Max(f.max, p.Distance)
	}
	if len(folds) == 0 {
		return 0
	}

	nowNanos := now.UnixNano()
	g.mu.Lock()
	defer g.mu.Unlock()
	lockedThreshold := g.Params.LockedBaselineThreshold
	if lockedThreshold == 0 {
		lockedThreshold = DefaultLockedBaselineThreshold
	}
	for _, f := range folds {
		c := &g.Cells[f.cell]
		halfSpan := float32((f.max - f.min) / 2)
		if c.TimesSeenCount == 0 {
			c.TimesSeenCount = 1
			c.RangeSpreadMeters = 0
			g.nonzeroCellCount++
		}
		c.AverageRangeMeters = float32(f.sum / float64(f.n))
		if halfSpan > c.RangeSpreadMeters {
			c.RangeSpreadMeters = halfSpan
		}
		if c.LockedAtCount >= lockedThreshold {
			c.LockedBaseline = c.AverageRangeMeters
			c.LockedSpread = c.RangeSpreadMeters
		}
		c.FrozenUntilUnixNanos = 0
		c.RecentForegroundCount = 0
		c.LastUpdateUnixNanos = nowNanos
	}
	g.ChangesSinceSnapshot += len(folds)
	return len(folds)
}

// returnCell is the cell a return falls in: ring from its 1-based channel,
// azimuth bin from its azimuth normalised into [0, 360). It is the mapping
// ProcessFramePolarWithMaskAt applies inline on the per-point path, so a cell
// absorbed here is the one that path classifies the next return of the same
// beam in; TestAbsorbedReturnIsBackgroundOnTheMaskPath holds the two together.
func returnCell(rings, azBins int, p PointPolar) (int, bool) {
	ring := p.Channel - 1
	if ring < 0 || ring >= rings {
		return 0, false
	}
	az := math.Mod(p.Azimuth, 360.0)
	if az < 0 {
		az += 360.0
	}
	azBin := int((az / 360.0) * float64(azBins))
	if azBin >= azBins {
		azBin = azBins - 1
	}
	return ring*azBins + azBin, true
}
