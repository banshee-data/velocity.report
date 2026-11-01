package l3grid

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/config"
)

// Foreground refinement (experiment l3bg0029, the tuning block
// l3.ema_baseline_v1.foreground_refine;
// docs/lidar/operations/l3-foreground-refinement-2026-10.md) gives the
// per-return decision evidence a window depth cannot. No depth separates a
// road user in front of a variable surface from that surface's own flicker,
// so the window stays as it is and three rules work around it:
//
//   - One-sided decision: a return beyond the surface a cell has learned
//     cannot be an object in front of that surface. It is background, and a
//     run of such returns agreeing on one range relearns the cell. This is
//     the road and the walls behind a vehicle that stood through settling and
//     then left, which a two-sided window paints foreground until the cell
//     catches up.
//   - Hysteresis: a return the window accepted but which lies in front of its
//     cell's surface is weak. Weak returns connected to foreground through
//     similar-range neighbours in the ring-by-azimuth grid are foreground: the
//     object's body comes in with the part of it the window rejected.
//   - Novelty: a weak patch with no foreground of its own comes in when enough
//     of it lies in front of its cells' near envelope, how close those cells'
//     background returns have recently come. A surface's flicker teaches its
//     cells that range within a few returns; a passing vehicle teaches them
//     nothing, because foreground never updates the envelope.

// ForegroundRefineParams configures foreground refinement. Nil in
// BackgroundParams is off, as every L3 before it ran.
type ForegroundRefineParams struct {
	// FarRelearnReturns is how many consecutive far returns agreeing on one
	// range relearn a cell.
	FarRelearnReturns int
	// FarAgreeMetres and FarAgreePerMetre set how close two far returns must
	// be to agree: base + per-metre * range.
	FarAgreeMetres   float64
	FarAgreePerMetre float64
	// FarRelearnSeen is the observation count a relearned cell restarts from.
	FarRelearnSeen uint32
	// WeakMarginMetres, WeakSpreads and WeakMarginPerMetre set how far in front
	// of its cell's surface a background return must lie to be weak:
	// margin + spreads * cell spread + per-metre * range.
	WeakMarginMetres   float64
	WeakSpreads        float64
	WeakMarginPerMetre float64
	// GrowRings and GrowBins are the neighbourhood foreground grows through, in
	// rings and azimuth bins either side.
	GrowRings, GrowBins int
	// GrowTauMetres and GrowTauPerMetre are the range similarity between
	// neighbours: base + per-metre * range.
	GrowTauMetres   float64
	GrowTauPerMetre float64
	// EnvelopeAttack and EnvelopeRelease are the share of the distance the near
	// envelope moves towards a background return in front of it and behind it.
	EnvelopeAttack  float32
	EnvelopeRelease float32
	// NovelMarginMetres and NovelMarginPerMetre set how far in front of the
	// envelope a weak return must lie to be new to its cell.
	NovelMarginMetres   float64
	NovelMarginPerMetre float64
	// NovelMinSeen is the observations a cell needs before its envelope counts.
	NovelMinSeen uint32
	// NovelMinReturns is how many novel returns bring a weak patch in.
	NovelMinReturns int
	// DeferWeakLearning teaches a cell a weak return after the grow pass, at
	// the cell's rate if it stayed background and at PromotedLearnShare of that
	// rate, unboosted, if it came in. A passing vehicle then leaves the cell's
	// average and spread alone, and one that stops is absorbed in about a
	// minute instead of never.
	DeferWeakLearning  bool
	PromotedLearnShare float64
}

// ForegroundRefineParamsFromTuning is the tuning block's refinement; nil without
// the block.
func ForegroundRefineParamsFromTuning(t *config.L3ForegroundRefine) *ForegroundRefineParams {
	if t == nil {
		return nil
	}
	return &ForegroundRefineParams{
		FarRelearnReturns: t.FarRelearnReturns, FarAgreeMetres: t.FarAgreeMetres, FarAgreePerMetre: t.FarAgreePerMetre,
		FarRelearnSeen:   uint32(t.FarRelearnSeen),
		WeakMarginMetres: t.WeakMarginMetres, WeakSpreads: t.WeakSpreads, WeakMarginPerMetre: t.WeakMarginPerMetre,
		GrowRings: t.GrowRings, GrowBins: t.GrowBins, GrowTauMetres: t.GrowTauMetres, GrowTauPerMetre: t.GrowTauPerMetre,
		EnvelopeAttack: float32(t.EnvelopeAttack), EnvelopeRelease: float32(t.EnvelopeRelease),
		NovelMarginMetres: t.NovelMarginMetres, NovelMarginPerMetre: t.NovelMarginPerMetre,
		NovelMinSeen: uint32(t.NovelMinSeen), NovelMinReturns: t.NovelMinReturns,
		DeferWeakLearning: t.DeferWeakLearning, PromotedLearnShare: t.PromotedLearnShare,
	}
}

// ForegroundRefineTuningFromParams is the tuning block for the refinement a
// manager runs, for reading the running configuration back; nil when it runs
// none.
func ForegroundRefineTuningFromParams(p *ForegroundRefineParams) *config.L3ForegroundRefine {
	if p == nil {
		return nil
	}
	return &config.L3ForegroundRefine{
		FarRelearnReturns: p.FarRelearnReturns, FarAgreeMetres: p.FarAgreeMetres, FarAgreePerMetre: p.FarAgreePerMetre,
		FarRelearnSeen:   int(p.FarRelearnSeen),
		WeakMarginMetres: p.WeakMarginMetres, WeakSpreads: p.WeakSpreads, WeakMarginPerMetre: p.WeakMarginPerMetre,
		GrowRings: p.GrowRings, GrowBins: p.GrowBins, GrowTauMetres: p.GrowTauMetres, GrowTauPerMetre: p.GrowTauPerMetre,
		EnvelopeAttack: float64(p.EnvelopeAttack), EnvelopeRelease: float64(p.EnvelopeRelease),
		NovelMarginMetres: p.NovelMarginMetres, NovelMarginPerMetre: p.NovelMarginPerMetre,
		NovelMinSeen: int(p.NovelMinSeen), NovelMinReturns: p.NovelMinReturns,
		DeferWeakLearning: p.DeferWeakLearning, PromotedLearnShare: p.PromotedLearnShare,
	}
}

func (p *ForegroundRefineParams) clone() *ForegroundRefineParams {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

// foregroundRefine is a manager's refinement state: per cell, the near envelope
// and the run of far returns; per frame, the reused buffers of the grow pass.
type foregroundRefine struct {
	nearEnvelope []float32
	farRun       []uint8
	farRange     []float32

	frame uint32
	stamp []uint32  // per cell: the frame that last wrote point
	point []int32   // per cell: its nearest foreground or weak return
	cell  []int32   // per return: its cell, or -1
	alpha []float32 // per weak return: its cell's update fraction
	weak  []bool
	novel []bool
	seen  []bool
	queue []int32
	patch []int32
}

func (r *foregroundRefine) prepare(cells, points int) {
	if len(r.nearEnvelope) != cells {
		r.nearEnvelope = make([]float32, cells)
		r.farRun = make([]uint8, cells)
		r.farRange = make([]float32, cells)
		r.stamp = make([]uint32, cells)
		r.point = make([]int32, cells)
		r.frame = 0
	}
	r.frame++
	if r.frame == 0 { // wrapped: every stamp is stale, start again
		clear(r.stamp)
		r.frame = 1
	}
	if cap(r.cell) < points {
		r.cell = make([]int32, points)
		r.alpha = make([]float32, points)
		r.weak = make([]bool, points)
		r.novel = make([]bool, points)
		r.seen = make([]bool, points)
	}
	r.cell = r.cell[:points]
	r.alpha = r.alpha[:points]
	for i := range r.cell {
		r.cell[i] = -1
	}
	r.weak = r.weak[:points]
	r.novel = r.novel[:points]
	r.seen = r.seen[:points]
	clear(r.weak)
	clear(r.novel)
	clear(r.seen)
}

// farReturn counts a return beyond its cell's surface towards relearning the
// cell, and relearns it when the run is long enough.
func (r *foregroundRefine) farReturn(p *ForegroundRefineParams, cell *BackgroundCell, cellIdx int, distance float64) {
	// The run must agree on one surface: a cell that sees through foliage to
	// several depths has no new surface to learn.
	if r.farRun[cellIdx] > 0 && math.Abs(float64(r.farRange[cellIdx])-distance) > p.FarAgreeMetres+p.FarAgreePerMetre*distance {
		r.farRun[cellIdx] = 0
	}
	if r.farRun[cellIdx] == 0 {
		r.farRange[cellIdx] = float32(distance)
	}
	if r.farRun[cellIdx] < math.MaxUint8 {
		r.farRun[cellIdx]++
	}
	if int(r.farRun[cellIdx]) < p.FarRelearnReturns {
		return
	}
	cell.AverageRangeMeters = float32(distance)
	cell.RangeSpreadMeters = 0
	cell.LockedBaseline = 0
	cell.LockedSpread = 0
	cell.LockedAtCount = 0
	cell.RecentForegroundCount = 0
	cell.FrozenUntilUnixNanos = 0
	cell.TimesSeenCount = p.FarRelearnSeen
	r.farRun[cellIdx] = 0
	r.nearEnvelope[cellIdx] = float32(distance)
}

// markWeak flags a background return in front of its cell's surface, and as
// novel when it is also in front of the cell's near envelope.
func (r *foregroundRefine) markWeak(p *ForegroundRefineParams, cell *BackgroundCell, cellIdx, i int, distance float64) {
	surface := float64(cell.AverageRangeMeters)
	if cell.LockedBaseline > 0 && float64(cell.LockedBaseline) < surface {
		surface = float64(cell.LockedBaseline)
	}
	if surface-distance <= p.WeakMarginMetres+p.WeakSpreads*float64(cell.RangeSpreadMeters)+p.WeakMarginPerMetre*distance {
		return
	}
	r.weak[i] = true
	if cell.TimesSeenCount < p.NovelMinSeen {
		return
	}
	if env := float64(r.nearEnvelope[cellIdx]); env != 0 && env-distance > p.NovelMarginMetres+p.NovelMarginPerMetre*distance {
		r.novel[i] = true
	}
}

// grow turns weak returns into foreground and returns how many it changed.
// mask is true for foreground; r.cell, r.weak and r.novel describe the frame.
func (r *foregroundRefine) grow(g *BackgroundGrid, p *ForegroundRefineParams, points []PointPolar, mask []bool) int {
	for i, c := range r.cell {
		if c < 0 || (!mask[i] && !r.weak[i]) {
			continue
		}
		if r.stamp[c] != r.frame || points[i].Distance < points[r.point[c]].Distance {
			r.stamp[c] = r.frame
			r.point[c] = int32(i)
		}
	}
	// neighbours calls visit for each weak return of the frame in the
	// neighbourhood of return i, at a similar range.
	neighbours := func(i int32, visit func(j int32)) {
		c := int(r.cell[i])
		ring, bin := c/g.AzimuthBins, c%g.AzimuthBins
		distance := points[i].Distance
		tau := p.GrowTauMetres + p.GrowTauPerMetre*distance
		for dr := -p.GrowRings; dr <= p.GrowRings; dr++ {
			if ring+dr < 0 || ring+dr >= g.Rings {
				continue
			}
			for db := -p.GrowBins; db <= p.GrowBins; db++ {
				k := g.Idx(ring+dr, (bin+db+g.AzimuthBins)%g.AzimuthBins)
				if r.stamp[k] != r.frame {
					continue
				}
				j := r.point[k]
				if !mask[j] && r.weak[j] && math.Abs(points[j].Distance-distance) <= tau {
					visit(j)
				}
			}
		}
	}

	grown := 0
	r.queue = r.queue[:0]
	for i, c := range r.cell {
		if c >= 0 && mask[i] {
			r.queue = append(r.queue, int32(i))
		}
	}
	for len(r.queue) > 0 {
		i := r.queue[len(r.queue)-1]
		r.queue = r.queue[:len(r.queue)-1]
		neighbours(i, func(j int32) {
			mask[j] = true
			grown++
			r.queue = append(r.queue, j)
		})
	}

	// A patch with no foreground of its own comes in when enough of it is new
	// to its cells.
	for start := range points {
		if !r.novel[start] || mask[start] || r.seen[start] {
			continue
		}
		r.patch = r.patch[:0]
		r.queue = append(r.queue[:0], int32(start))
		r.seen[start] = true
		novel := 0
		for len(r.queue) > 0 {
			i := r.queue[len(r.queue)-1]
			r.queue = r.queue[:len(r.queue)-1]
			r.patch = append(r.patch, i)
			if r.novel[i] {
				novel++
			}
			neighbours(i, func(j int32) {
				if !r.seen[j] {
					r.seen[j] = true
					r.queue = append(r.queue, j)
				}
			})
		}
		if novel >= p.NovelMinReturns {
			for _, i := range r.patch {
				mask[i] = true
				grown++
			}
		}
	}
	return grown
}

// learnWeak teaches the cells the frame's weak returns, once the grow pass has
// said which are objects.
func (r *foregroundRefine) learnWeak(g *BackgroundGrid, points []PointPolar, mask []bool, readOnly func(cell int) bool, share, reacqBoost float64, lockedThreshold uint32, nowNanos int64) {
	for i, c := range r.cell {
		if c < 0 || !r.weak[i] || readOnly(int(c)) {
			continue
		}
		cell := &g.Cells[c]
		if mask[i] {
			if share <= 0 {
				continue
			}
			cell.learnBackground(points[i].Distance, float64(r.alpha[i])*share, 1, lockedThreshold)
		} else {
			cell.learnBackground(points[i].Distance, float64(r.alpha[i]), reacqBoost, lockedThreshold)
		}
		cell.LastUpdateUnixNanos = nowNanos
	}
}

// learnEnvelope moves each cell's near envelope towards what stayed background
// this frame. Foreground teaches it nothing, and nor does a cell the track guard
// is holding. attack and release come from p.
func (r *foregroundRefine) learnEnvelope(p *ForegroundRefineParams, points []PointPolar, mask []bool, held func(cell int) bool) {
	for i, c := range r.cell {
		if c < 0 || mask[i] || held(int(c)) {
			continue
		}
		env := &r.nearEnvelope[c]
		distance := float32(points[i].Distance)
		switch {
		case *env == 0:
			*env = distance
		case distance < *env:
			*env += p.EnvelopeAttack * (distance - *env)
		default:
			*env += p.EnvelopeRelease * (distance - *env)
		}
	}
}
