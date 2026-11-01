package l3grid

import "math"

// DecisionCode says why the per-point classifier gave a return its class. It
// exists for offline diagnosis of foreground extraction: a frame's mask says
// what L3 decided, the trace says which rule decided it.
type DecisionCode uint8

const (
	// DecisionInvalidChannel is a return outside the grid's rings: foreground.
	DecisionInvalidChannel DecisionCode = iota
	// DecisionFrozenMatches is a return in a frozen cell that sits inside the
	// cell's own closeness window: foreground only because the cell is frozen.
	DecisionFrozenMatches
	// DecisionFrozenDiverges is a return in a frozen cell outside its window.
	DecisionFrozenDiverges
	// DecisionBackgroundLocked is background by the locked baseline alone.
	DecisionBackgroundLocked
	// DecisionBackgroundBase is background inside the base window: the global
	// noise model with no warm-up ramp and no region override.
	DecisionBackgroundBase
	// DecisionBackgroundWidened is background only because the warm-up ramp
	// or a region override widened the window past the base window.
	DecisionBackgroundWidened
	// DecisionBackgroundNeighbour is background by neighbour confirmation alone.
	DecisionBackgroundNeighbour
	// DecisionBackgroundDeadlock is background by the deadlock breaker.
	DecisionBackgroundDeadlock
	// DecisionBackgroundSeed seeded an empty cell: background by default.
	DecisionBackgroundSeed
	// DecisionForeground is foreground by divergence from the cell.
	DecisionForeground
	// DecisionForegroundFroze is foreground and froze the cell.
	DecisionForegroundFroze
	// DecisionBackgroundFar is background by being beyond the learned surface.
	DecisionBackgroundFar
	// DecisionForegroundGrown is a weak return that foreground refinement
	// brought in: connected to foreground, or in a novel patch.
	DecisionForegroundGrown
)

// DecisionRecord is one return's trace: the rule, and the cell state and
// window the rule read.
type DecisionRecord struct {
	Code       DecisionCode
	Held       bool // under the track guard: the cell learned nothing
	Average    float32
	Spread     float32
	Window     float32
	BaseWindow float32
	Locked     float32
	Seen       uint32
}

// SetDecisionTrace turns the per-return trace on or off. Off, the zero value,
// the classifier does no extra work.
func (bm *BackgroundManager) SetDecisionTrace(on bool) {
	if bm == nil {
		return
	}
	bm.traceOn.Store(on)
}

// DecisionTrace is the trace of the frame last classified, one record per
// return, in return order. It is reused by the next frame.
func (bm *BackgroundManager) DecisionTrace() []DecisionRecord {
	if bm == nil {
		return nil
	}
	return bm.trace
}

func (bm *BackgroundManager) prepareTrace(n int) []DecisionRecord {
	if !bm.traceOn.Load() {
		return nil
	}
	if cap(bm.trace) < n {
		bm.trace = make([]DecisionRecord, n)
	}
	bm.trace = bm.trace[:n]
	return bm.trace
}

func traceCell(cell *BackgroundCell, code DecisionCode, window, baseWindow float64) DecisionRecord {
	return DecisionRecord{Code: code, Average: cell.AverageRangeMeters, Spread: cell.RangeSpreadMeters,
		Window: float32(window), BaseWindow: float32(baseWindow), Locked: cell.LockedBaseline, Seen: cell.TimesSeenCount}
}

// traceBaseWindow is the window with no ramp and no region override.
func traceBaseWindow(multiplier, spread, noiseRel, distance, safety float64) float64 {
	return ForegroundClosenessWindowMetres(multiplier, spread, noiseRel, distance, safety, 1)
}

// traceDiff is a return's distance from its cell's average.
func traceDiff(cell *BackgroundCell, distance float64) float64 {
	return math.Abs(float64(cell.AverageRangeMeters) - distance)
}
