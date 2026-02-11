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

	// Scene-level weights (opt-in; zero by default)
	ForegroundCapture float64 `json:"foreground_capture"` // Positive = maximise capture ratio
	EmptyBoxes        float64 `json:"empty_boxes"`        // Negative = minimise empty box ratio
	Fragmentation     float64 `json:"fragmentation"`      // Negative = minimise fragmentation ratio
	HeadingJitter     float64 `json:"heading_jitter"`     // Negative = minimise heading jitter
	SpeedJitter       float64 `json:"speed_jitter"`       // Negative = minimise speed jitter
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
// Log-scale is used for NonzeroCells and ActiveTracks; all other terms are linear.
// Note: minimisation weights (e.g. Misalignment, EmptyBoxes) should be negative.
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

	// Foreground capture ratio (0-1, higher is better)
	score += weights.ForegroundCapture * result.ForegroundCaptureMean

	// Empty box ratio (0-1, lower is better, so weight is typically negative)
	score += weights.EmptyBoxes * result.EmptyBoxRatioMean

	// Fragmentation ratio (0-1, lower is better, so weight is typically negative)
	score += weights.Fragmentation * result.FragmentationRatioMean

	// Heading jitter degrees (lower is better, so weight is typically negative)
	score += weights.HeadingJitter * result.HeadingJitterDegMean

	// Speed jitter m/s (lower is better, so weight is typically negative)
	score += weights.SpeedJitter * result.SpeedJitterMpsMean

	return score
}

// AcceptanceCriteria defines hard thresholds that a ComboResult must satisfy
// to be considered viable. A nil pointer means no constraint for that metric.
type AcceptanceCriteria struct {
	MaxFragmentationRatio  *float64 `json:"max_fragmentation_ratio,omitempty"`
	MaxUnboundedPointRatio *float64 `json:"max_unbounded_point_ratio,omitempty"`
	MaxEmptyBoxRatio       *float64 `json:"max_empty_box_ratio,omitempty"`
}

// CheckAcceptance returns true if the result satisfies all acceptance criteria.
// A nil criteria pointer means all results are accepted.
func CheckAcceptance(result ComboResult, criteria *AcceptanceCriteria) bool {
	if criteria == nil {
		return true
	}
	if criteria.MaxFragmentationRatio != nil && result.FragmentationRatioMean > *criteria.MaxFragmentationRatio {
		return false
	}
	if criteria.MaxUnboundedPointRatio != nil && result.UnboundedPointMean > *criteria.MaxUnboundedPointRatio {
		return false
	}
	if criteria.MaxEmptyBoxRatio != nil && result.EmptyBoxRatioMean > *criteria.MaxEmptyBoxRatio {
		return false
	}
	return true
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

// RankResultsWithCriteria scores and ranks results, applying acceptance criteria.
// Combos that fail criteria receive score = -MaxFloat64 and sort to the bottom.
func RankResultsWithCriteria(results []ComboResult, weights ObjectiveWeights, criteria *AcceptanceCriteria) []ScoredResult {
	scored := make([]ScoredResult, len(results))
	for i, r := range results {
		if CheckAcceptance(r, criteria) {
			scored[i] = ScoredResult{
				ComboResult: r,
				Score:       ScoreResult(r, weights),
			}
		} else {
			scored[i] = ScoredResult{
				ComboResult: r,
				Score:       -math.MaxFloat64,
			}
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored
}
