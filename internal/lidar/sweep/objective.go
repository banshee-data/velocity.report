package sweep

import (
	"math"
	"sort"
)

// ObjectiveWeights defines weights for multi-objective scoring.
type ObjectiveWeights struct {
	Acceptance   float64 `json:"acceptance"`
	Misalignment float64 `json:"misalignment"`
	Alignment    float64 `json:"alignment"`
	NonzeroCells float64 `json:"nonzero_cells"`
	ActiveTracks float64 `json:"active_tracks"`
}

// DefaultObjectiveWeights returns default weights for multi-objective scoring.
func DefaultObjectiveWeights() ObjectiveWeights {
	return ObjectiveWeights{
		Acceptance:   1.0,
		Misalignment: -0.5,
		Alignment:    -0.01,
		NonzeroCells: 0.1,
		ActiveTracks: 0.3,
	}
}

// ScoreResult computes a scalar score for a ComboResult using the given weights.
// Formula: w1*accept_rate + w2*misalignment_ratio + w3*alignment_deg + w4*log(NonzeroCells) + w5*log(ActiveTracks)
// Note: w2 and w3 are typically negative (minimise), so the sign should be baked into the weight values.
func ScoreResult(result ComboResult, weights ObjectiveWeights) float64 {
	score := 0.0

	// Acceptance rate (0-1, higher is better)
	score += weights.Acceptance * result.OverallAcceptMean

	// Misalignment ratio (0-1, lower is better, so weight is typically negative)
	score += weights.Misalignment * result.MisalignmentRatioMean

	// Alignment degrees (lower is better, so weight is typically negative)
	score += weights.Alignment * result.AlignmentDegMean

	// Nonzero cells (log scale, more cells is better)
	if result.NonzeroCellsMean > 0 {
		score += weights.NonzeroCells * math.Log(result.NonzeroCellsMean)
	}

	// Active tracks (log scale, more tracks is better for detection)
	if result.ActiveTracksMean > 0 {
		score += weights.ActiveTracks * math.Log(result.ActiveTracksMean)
	}

	return score
}

// ScoredResult pairs a ComboResult with its objective score.
type ScoredResult struct {
	ComboResult
	Score float64 `json:"score"`
}

// RankResults sorts ComboResults by score (highest first) and returns the sorted slice.
func RankResults(results []ComboResult, weights ObjectiveWeights) []ScoredResult {
	scored := make([]ScoredResult, len(results))
	for i, r := range results {
		scored[i] = ScoredResult{
			ComboResult: r,
			Score:       ScoreResult(r, weights),
		}
	}

	// Sort by score descending (highest first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored
}
