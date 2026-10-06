package l3grid

// Closeness thresholds decide whether an observation matches the learned
// background range for its cell. Extracted from the classification loop so the
// noise model in them can be measured against the sensor's own specification
// (gap HW1 in data/maths/paper-implementation-gap-analysis.md) rather than
// re-derived in a test that could drift from the code it claims to check.

// closenessFloorMetres keeps the threshold non-zero for a cell whose measured
// spread has collapsed, so a perfectly still cell does not reject every
// observation on floating-point noise alone.
const closenessFloorMetres = 0.01

// ClosenessThresholdMetres is how far an observation may sit from a cell's
// learned average range and still count as background.
//
// Exported so the closeness audit tool measures this expression rather than a
// copy of it: the point of extracting it was to remove that drift risk.
//
//	threshold = multiplier * (spread + noiseRelative*observedRange + floor) + safety
//
// The spread term is measured: it carries whatever run-to-run variation the
// cell has actually shown. The noiseRelative term is a model, and it is a
// *relative* one — a fixed fraction of range — which is the part gap HW1
// questions, because the Pandar40P's specified range accuracy is flat (±2 cm to
// 30 m, ±3 cm beyond), not proportional. See specRangeAccuracyMetres.
//
// safety is the operator's absolute margin; the neighbour-confirmation caller
// passes 0 for it, since that path is looking for agreement between adjacent
// cells rather than gating a classification decision.
func ClosenessThresholdMetres(multiplier, spread, noiseRelative, observedRange, safety float64) float64 {
	return multiplier*(spread+noiseRelative*observedRange+closenessFloorMetres) + safety
}

// WarmupSettledCount is the observation count at which a cell is treated as
// having learned its own variance. Below it, the foreground window is widened
// on a linear ramp: a new cell has no trustworthy spread yet, and calling its
// noise foreground produces "initialisation trails" where static wall points
// are reported as movement.
const WarmupSettledCount = 100

// WarmupMultiplier is the foreground window's tolerance factor for a cell that
// has been observed timesSeen times: 4x at zero, decaying linearly to 1x at
// WarmupSettledCount and staying there.
func WarmupMultiplier(timesSeen uint32) float64 {
	if timesSeen >= WarmupSettledCount {
		return 1.0
	}
	return 1.0 + 3.0*float64(WarmupSettledCount-timesSeen)/float64(WarmupSettledCount)
}

// ForegroundClosenessWindowMetres is the closeness window the per-point
// foreground decision uses. It is ClosenessThresholdMetres with the warmup
// ramp applied to the scaled term — the operator's safety margin sits outside
// the ramp, since it is an absolute allowance rather than a confidence one.
func ForegroundClosenessWindowMetres(multiplier, spread, noiseRelative, observedRange, safety, warmup float64) float64 {
	return multiplier*(spread+noiseRelative*observedRange+closenessFloorMetres)*warmup + safety
}

// lockedWindowFloorMetres keeps a locked cell's acceptance window usable when
// its locked spread has collapsed to nearly nothing.
const lockedWindowFloorMetres = 0.1

// LockedBaselineWindowMetres is the second, independent acceptance window: how
// far an observation may sit from a cell's *locked* baseline and still count as
// background. It exists to survive EMA drift during a transit, when the running
// average is being pulled toward the passing object.
//
// Note that the noise term here is not scaled by the closeness multiplier, so
// this window is narrower than ForegroundClosenessWindowMetres at range despite
// the larger spread multiplier. The foreground decision ORs the two, so the bar
// a real return must clear is whichever is wider.
func LockedBaselineWindowMetres(lockedMultiplier, lockedSpread, noiseRelative, observedRange, safety float64) float64 {
	w := lockedMultiplier*lockedSpread + noiseRelative*observedRange + safety
	if w < lockedWindowFloorMetres {
		return lockedWindowFloorMetres
	}
	return w
}
