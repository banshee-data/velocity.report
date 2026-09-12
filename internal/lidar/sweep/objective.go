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
	// Course is a proxy, opt-in and subordinate to labelled acceptance gates.
	CourseAlignment float64 `json:"course_alignment"`
	// A site/window-specific count band replaces an unbounded default reward.
	ActiveTrackBand *TrackCountBand `json:"active_track_band,omitempty"`

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
		ActiveTracks: 0,
	}
}

// ScoreResult computes a scalar score for a ComboResult using the given weights.
// Log-scale is used for NonzeroCells and explicitly requested legacy ActiveTracks.
// Course is a proxy requiring eligible samples; a count band requires a labelled
// site/window reference. Neither term should reward missing evidence or extra IDs.
// Note: minimisation weights (e.g. Misalignment, EmptyBoxes) should be negative.
func ScoreResult(result ComboResult, weights ObjectiveWeights) float64 {
	score := 0.0
	if weights.CourseAlignment != 0 {
		if weights.CourseAlignment > 0 || math.IsNaN(weights.CourseAlignment) || math.IsInf(weights.CourseAlignment, 0) || result.CourseAlignmentP50Mean == nil || result.CourseAlignmentSnapshots == 0 ||
			math.IsNaN(*result.CourseAlignmentP50Mean) || math.IsInf(*result.CourseAlignmentP50Mean, 0) ||
			*result.CourseAlignmentP50Mean < 0 || *result.CourseAlignmentP50Mean > 90 {
			return -math.MaxFloat64
		}
		score += weights.CourseAlignment * *result.CourseAlignmentP50Mean
	}
	if band := weights.ActiveTrackBand; band != nil {
		if !band.valid() || result.TrackMetricsSnapshots == 0 || result.ActiveTracksMean < 0 || math.IsNaN(result.ActiveTracksMean) || math.IsInf(result.ActiveTracksMean, 0) {
			return -math.MaxFloat64
		}
		distance := math.Max(0, math.Max(band.Min-result.ActiveTracksMean, result.ActiveTracksMean-band.Max))
		score -= band.Penalty * distance
	}

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

	// Explicit legacy reward only; defaults and supplied bands do not reward IDs.
	if weights.ActiveTrackBand == nil && result.ActiveTracksMean > 0 {
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

// TrackCountBand is supplied from a declared site/window reference population.
// It is not inferred from whichever candidate happens to create the most tracks.
type TrackCountBand struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Penalty float64 `json:"penalty"`
}

func (b TrackCountBand) valid() bool {
	return b.Min >= 0 && b.Max >= b.Min && b.Penalty > 0 &&
		!math.IsInf(b.Max, 0) && !math.IsInf(b.Penalty, 0)
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
	Score      float64          `json:"score"`
	Components *ScoreComponents `json:"score_components,omitempty"`
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
