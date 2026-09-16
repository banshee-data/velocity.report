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

// closenessThresholdMetres is how far an observation may sit from a cell's
// learned average range and still count as background.
//
//	threshold = multiplier * (spread + noiseRelative*observedRange + floor) + safety
//
// The spread term is measured: it carries whatever run-to-run variation the
// cell has actually shown. The noiseRelative term is a model, and it is a
// *relative* one — a fixed fraction of range — which is the part gap HW1
// questions, because the Pandar40P's specified range accuracy is flat (±2 cm to
// 30 m, ±3 cm beyond), not proportional. See specRangeAccuracyMetres.
//
// safety is the operator's absolute margin; the neighbour-confirmation variant
// of this test passes 0 for it, since that path is looking for agreement
// between adjacent cells rather than gating a classification decision.
func closenessThresholdMetres(multiplier, spread, noiseRelative, observedRange, safety float64) float64 {
	return multiplier*(spread+noiseRelative*observedRange+closenessFloorMetres) + safety
}
