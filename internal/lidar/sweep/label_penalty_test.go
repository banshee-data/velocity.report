package sweep

import (
	"math"
	"testing"
)

func TestLabelCoveragePenalty_FullCoverage(t *testing.T) {
	penalty := LabelCoveragePenalty(1.0, 0.9)
	if penalty != 1.0 {
		t.Errorf("expected penalty=1.0 for full coverage, got %f", penalty)
	}
}

func TestLabelCoveragePenalty_AboveThreshold(t *testing.T) {
	penalty := LabelCoveragePenalty(0.95, 0.9)
	if penalty != 1.0 {
		t.Errorf("expected penalty=1.0 for coverage above threshold, got %f", penalty)
	}
}

func TestLabelCoveragePenalty_AtThreshold(t *testing.T) {
	penalty := LabelCoveragePenalty(0.9, 0.9)
	if penalty != 1.0 {
		t.Errorf("expected penalty=1.0 for coverage at threshold, got %f", penalty)
	}
}

func TestLabelCoveragePenalty_BelowThreshold(t *testing.T) {
	penalty := LabelCoveragePenalty(0.45, 0.9)
	expected := 0.5 // 0.45 / 0.9
	if math.Abs(penalty-expected) > 0.001 {
		t.Errorf("expected penalty≈%f, got %f", expected, penalty)
	}
}

func TestLabelCoveragePenalty_HalfCoverage(t *testing.T) {
	penalty := LabelCoveragePenalty(0.5, 1.0)
	if penalty != 0.5 {
		t.Errorf("expected penalty=0.5 for half coverage, got %f", penalty)
	}
}

func TestLabelCoveragePenalty_ZeroCoverage(t *testing.T) {
	penalty := LabelCoveragePenalty(0.0, 0.9)
	if penalty != 0.0 {
		t.Errorf("expected penalty=0.0 for zero coverage, got %f", penalty)
	}
}

func TestLabelCoveragePenalty_NegativeCoverage(t *testing.T) {
	// Edge case: negative coverage should clamp to 0
	penalty := LabelCoveragePenalty(-0.1, 0.9)
	if penalty != 0.0 {
		t.Errorf("expected penalty=0.0 for negative coverage, got %f", penalty)
	}
}

func TestLabelCoveragePenalty_ZeroThreshold(t *testing.T) {
	// Edge case: zero threshold should return 1.0 (no penalty)
	penalty := LabelCoveragePenalty(0.5, 0.0)
	if penalty != 1.0 {
		t.Errorf("expected penalty=1.0 for zero threshold, got %f", penalty)
	}
}

func TestLabelCoveragePenalty_NegativeThreshold(t *testing.T) {
	// Edge case: negative threshold should return 1.0 (no penalty)
	penalty := LabelCoveragePenalty(0.5, -0.9)
	if penalty != 1.0 {
		t.Errorf("expected penalty=1.0 for negative threshold, got %f", penalty)
	}
}

func TestLabelCoveragePenalty_VeryLowThreshold(t *testing.T) {
	// Coverage exceeds a very low threshold
	penalty := LabelCoveragePenalty(0.5, 0.1)
	if penalty != 1.0 {
		t.Errorf("expected penalty=1.0 when coverage exceeds low threshold, got %f", penalty)
	}
}

func TestLabelCoveragePenalty_LinearScaling(t *testing.T) {
	// Verify linear scaling behaviour
	threshold := 0.8
	testCases := []struct {
		coverage float64
		expected float64
	}{
		{0.8, 1.0},  // at threshold
		{0.4, 0.5},  // half threshold
		{0.2, 0.25}, // quarter threshold
		{0.6, 0.75}, // 3/4 threshold
		{0.0, 0.0},  // zero coverage
		{0.16, 0.2}, // 20% of threshold
		{0.64, 0.8}, // 80% of threshold
		{0.88, 1.0}, // above threshold (clamped)
		{1.0, 1.0},  // full coverage
	}

	for _, tc := range testCases {
		penalty := LabelCoveragePenalty(tc.coverage, threshold)
		if math.Abs(penalty-tc.expected) > 0.001 {
			t.Errorf("coverage=%f, threshold=%f: expected penalty≈%f, got %f",
				tc.coverage, threshold, tc.expected, penalty)
		}
	}
}

func TestApplyLabelCoveragePenalty_FullCoverage(t *testing.T) {
	score := 0.85
	adjusted := ApplyLabelCoveragePenalty(score, 1.0, 0.9)
	if adjusted != score {
		t.Errorf("expected no penalty with full coverage: %f, got %f", score, adjusted)
	}
}

func TestApplyLabelCoveragePenalty_AboveThreshold(t *testing.T) {
	score := 0.85
	adjusted := ApplyLabelCoveragePenalty(score, 0.95, 0.9)
	if adjusted != score {
		t.Errorf("expected no penalty when above threshold: %f, got %f", score, adjusted)
	}
}

func TestApplyLabelCoveragePenalty_BelowThreshold(t *testing.T) {
	score := 0.80
	coverage := 0.45
	threshold := 0.9
	adjusted := ApplyLabelCoveragePenalty(score, coverage, threshold)

	penalty := coverage / threshold // 0.5
	expected := score * penalty     // 0.80 * 0.5 = 0.40

	if math.Abs(adjusted-expected) > 0.001 {
		t.Errorf("expected adjusted score≈%f, got %f", expected, adjusted)
	}
}

func TestApplyLabelCoveragePenalty_HalfCoverage(t *testing.T) {
	score := 0.90
	adjusted := ApplyLabelCoveragePenalty(score, 0.5, 1.0)
	expected := 0.45 // 0.90 * 0.5
	if math.Abs(adjusted-expected) > 0.001 {
		t.Errorf("expected adjusted score≈%f, got %f", expected, adjusted)
	}
}

func TestApplyLabelCoveragePenalty_ZeroCoverage(t *testing.T) {
	score := 0.90
	adjusted := ApplyLabelCoveragePenalty(score, 0.0, 0.9)
	if adjusted != 0.0 {
		t.Errorf("expected adjusted score=0.0 with zero coverage, got %f", adjusted)
	}
}

func TestApplyLabelCoveragePenalty_ZeroScore(t *testing.T) {
	adjusted := ApplyLabelCoveragePenalty(0.0, 0.5, 0.9)
	if adjusted != 0.0 {
		t.Errorf("expected adjusted score=0.0 with zero input score, got %f", adjusted)
	}
}

func TestApplyLabelCoveragePenalty_NegativeScore(t *testing.T) {
	// Scores can be negative in some contexts
	score := -0.50
	coverage := 0.45
	threshold := 0.9
	adjusted := ApplyLabelCoveragePenalty(score, coverage, threshold)

	penalty := coverage / threshold // 0.5
	expected := score * penalty     // -0.50 * 0.5 = -0.25

	if math.Abs(adjusted-expected) > 0.001 {
		t.Errorf("expected adjusted score≈%f, got %f", expected, adjusted)
	}
}

func TestApplyLabelCoveragePenalty_ExampleScenarios(t *testing.T) {
	testCases := []struct {
		name      string
		score     float64
		coverage  float64
		threshold float64
		expected  float64
	}{
		{
			name:      "high score, low coverage",
			score:     0.95,
			coverage:  0.30,
			threshold: 0.90,
			expected:  0.95 * (0.30 / 0.90), // ≈ 0.3167
		},
		{
			name:      "medium score, medium coverage",
			score:     0.70,
			coverage:  0.60,
			threshold: 0.90,
			expected:  0.70 * (0.60 / 0.90), // ≈ 0.4667
		},
		{
			name:      "low score, high coverage",
			score:     0.40,
			coverage:  0.95,
			threshold: 0.90,
			expected:  0.40, // no penalty when above threshold
		},
		{
			name:      "perfect score and coverage",
			score:     1.0,
			coverage:  1.0,
			threshold: 0.90,
			expected:  1.0,
		},
		{
			name:      "80% coverage, 90% threshold",
			score:     0.85,
			coverage:  0.80,
			threshold: 0.90,
			expected:  0.85 * (0.80 / 0.90), // ≈ 0.7556
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adjusted := ApplyLabelCoveragePenalty(tc.score, tc.coverage, tc.threshold)
			if math.Abs(adjusted-tc.expected) > 0.001 {
				t.Errorf("expected adjusted score≈%f, got %f", tc.expected, adjusted)
			}
		})
	}
}

func TestApplyLabelCoveragePenalty_ConsistentWithPenaltyFunction(t *testing.T) {
	// Verify that ApplyLabelCoveragePenalty uses LabelCoveragePenalty correctly
	score := 0.75
	coverage := 0.60
	threshold := 0.90

	penalty := LabelCoveragePenalty(coverage, threshold)
	expected := score * penalty
	adjusted := ApplyLabelCoveragePenalty(score, coverage, threshold)

	if math.Abs(adjusted-expected) > 0.0001 {
		t.Errorf("ApplyLabelCoveragePenalty inconsistent with LabelCoveragePenalty: expected %f, got %f",
			expected, adjusted)
	}
}
