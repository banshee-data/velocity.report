package sweep

// LabelCoveragePenalty computes a penalty factor based on label coverage.
// When coverage is below the threshold, the score is reduced proportionally.
// Returns a multiplier in (0, 1] where 1.0 means no penalty.
//
// Formula: penalty = clamp(coverage / threshold, 0, 1)
// Where:
//   - coverage is the fraction of tracks that have been labelled (0-1)
//   - threshold is the minimum acceptable coverage (e.g. 0.9)
func LabelCoveragePenalty(coverage, threshold float64) float64 {
	if threshold <= 0 {
		return 1.0
	}

	penalty := coverage / threshold

	// Clamp to [0, 1]
	if penalty < 0 {
		penalty = 0
	}
	if penalty > 1.0 {
		penalty = 1.0
	}

	return penalty
}

// ApplyLabelCoveragePenalty applies a label-coverage penalty to a score.
// If coverage is below threshold, the score is reduced proportionally.
func ApplyLabelCoveragePenalty(score, coverage, threshold float64) float64 {
	penalty := LabelCoveragePenalty(coverage, threshold)
	return score * penalty
}
