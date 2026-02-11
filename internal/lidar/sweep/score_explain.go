package sweep

import (
	"sort"
)

// ScoreComponents holds the per-metric contributions to a composite score.
// Each field records the raw metric value multiplied by the corresponding weight.
type ScoreComponents struct {
	DetectionRate     float64            `json:"detection_rate"`
	Fragmentation     float64            `json:"fragmentation"`
	FalsePositives    float64            `json:"false_positives"`
	VelocityCoverage  float64            `json:"velocity_coverage"`
	QualityPremium    float64            `json:"quality_premium"`
	TruncationRate    float64            `json:"truncation_rate"`
	VelocityNoiseRate float64            `json:"velocity_noise_rate"`
	StoppedRecovery   float64            `json:"stopped_recovery"`
	CompositeScore    float64            `json:"composite_score"`
	WeightsUsed       GroundTruthWeights `json:"weights_used"`
}

// ScoreExplanation provides a full explanation of a sweep's best score.
type ScoreExplanation struct {
	Components              ScoreComponents  `json:"components"`
	TopContributors         []string         `json:"top_contributors"`
	DeltaVsPrevious         *ScoreComponents `json:"delta_vs_previous,omitempty"`
	LabelCoverageConfidence float64          `json:"label_coverage_confidence"`
}

// topContributors returns the top N metric names by absolute weighted contribution.
func topContributors(sc ScoreComponents, n int) []string {
	type entry struct {
		name string
		abs  float64
	}
	entries := []entry{
		{"detection_rate", abs(sc.DetectionRate)},
		{"fragmentation", abs(sc.Fragmentation)},
		{"false_positives", abs(sc.FalsePositives)},
		{"velocity_coverage", abs(sc.VelocityCoverage)},
		{"quality_premium", abs(sc.QualityPremium)},
		{"truncation_rate", abs(sc.TruncationRate)},
		{"velocity_noise_rate", abs(sc.VelocityNoiseRate)},
		{"stopped_recovery", abs(sc.StoppedRecovery)},
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].abs > entries[j].abs
	})
	if n > len(entries) {
		n = len(entries)
	}
	result := make([]string, n)
	for i := 0; i < n; i++ {
		result[i] = entries[i].name
	}
	return result
}

// abs returns the absolute value of a float64.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// DeltaComponents computes the element-wise difference (current - previous).
func DeltaComponents(current, previous ScoreComponents) ScoreComponents {
	return ScoreComponents{
		DetectionRate:     current.DetectionRate - previous.DetectionRate,
		Fragmentation:     current.Fragmentation - previous.Fragmentation,
		FalsePositives:    current.FalsePositives - previous.FalsePositives,
		VelocityCoverage:  current.VelocityCoverage - previous.VelocityCoverage,
		QualityPremium:    current.QualityPremium - previous.QualityPremium,
		TruncationRate:    current.TruncationRate - previous.TruncationRate,
		VelocityNoiseRate: current.VelocityNoiseRate - previous.VelocityNoiseRate,
		StoppedRecovery:   current.StoppedRecovery - previous.StoppedRecovery,
		CompositeScore:    current.CompositeScore - previous.CompositeScore,
		WeightsUsed:       current.WeightsUsed,
	}
}

// BuildExplanation creates a ScoreExplanation from a ScoreComponents and optional previous round.
func BuildExplanation(components ScoreComponents, previous *ScoreComponents, labelCoverage float64) ScoreExplanation {
	explanation := ScoreExplanation{
		Components:              components,
		TopContributors:         topContributors(components, 3),
		LabelCoverageConfidence: labelCoverage,
	}
	if previous != nil {
		delta := DeltaComponents(components, *previous)
		explanation.DeltaVsPrevious = &delta
	}
	return explanation
}
