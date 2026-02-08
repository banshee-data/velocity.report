package sweep

import (
	"math"
	"testing"
)

func TestDefaultObjectiveWeights(t *testing.T) {
	weights := DefaultObjectiveWeights()
	if weights.Acceptance != 1.0 {
		t.Errorf("expected Acceptance=1.0, got %v", weights.Acceptance)
	}
	if weights.Misalignment != -0.5 {
		t.Errorf("expected Misalignment=-0.5, got %v", weights.Misalignment)
	}
	if weights.Alignment != -0.01 {
		t.Errorf("expected Alignment=-0.01, got %v", weights.Alignment)
	}
	if weights.NonzeroCells != 0.1 {
		t.Errorf("expected NonzeroCells=0.1, got %v", weights.NonzeroCells)
	}
}

func TestScoreResultFormula(t *testing.T) {
	result := ComboResult{
		OverallAcceptMean:     0.8,
		MisalignmentRatioMean: 0.2,
		AlignmentDegMean:      5.0,
		NonzeroCellsMean:      100.0,
	}

	weights := ObjectiveWeights{
		Acceptance:   1.0,
		Misalignment: -0.5,
		Alignment:    -0.01,
		NonzeroCells: 0.1,
	}

	score := ScoreResult(result, weights)

	// Expected: 1.0*0.8 + (-0.5)*0.2 + (-0.01)*5.0 + 0.1*log(100)
	// = 0.8 - 0.1 - 0.05 + 0.1*4.605 = 0.8 - 0.1 - 0.05 + 0.4605 = 1.1105
	expected := 0.8 - 0.1 - 0.05 + 0.1*math.Log(100.0)

	if math.Abs(score-expected) > 0.0001 {
		t.Errorf("expected score ~%v, got %v", expected, score)
	}
}

func TestScoreResultZeroWeights(t *testing.T) {
	result := ComboResult{
		OverallAcceptMean:     0.9,
		MisalignmentRatioMean: 0.1,
		AlignmentDegMean:      2.0,
		NonzeroCellsMean:      50.0,
	}

	weights := ObjectiveWeights{
		Acceptance:   0.0,
		Misalignment: 0.0,
		Alignment:    0.0,
		NonzeroCells: 0.0,
	}

	score := ScoreResult(result, weights)
	if score != 0.0 {
		t.Errorf("expected score 0.0 with zero weights, got %v", score)
	}
}

func TestScoreResultZeroNonzeroCells(t *testing.T) {
	result := ComboResult{
		OverallAcceptMean:     0.8,
		MisalignmentRatioMean: 0.2,
		AlignmentDegMean:      5.0,
		NonzeroCellsMean:      0.0, // Zero cells
	}

	weights := DefaultObjectiveWeights()
	score := ScoreResult(result, weights)

	// Should not panic, log(0) term should be skipped
	// Expected: 0.8 - 0.1 - 0.05 = 0.65
	expected := 0.8 - 0.5*0.2 - 0.01*5.0
	if math.Abs(score-expected) > 0.0001 {
		t.Errorf("expected score ~%v, got %v", expected, score)
	}
}

func TestRankResultsOrder(t *testing.T) {
	results := []ComboResult{
		{OverallAcceptMean: 0.5, MisalignmentRatioMean: 0.3, AlignmentDegMean: 10.0, NonzeroCellsMean: 50.0},
		{OverallAcceptMean: 0.9, MisalignmentRatioMean: 0.1, AlignmentDegMean: 2.0, NonzeroCellsMean: 100.0},
		{OverallAcceptMean: 0.7, MisalignmentRatioMean: 0.2, AlignmentDegMean: 5.0, NonzeroCellsMean: 80.0},
	}

	weights := DefaultObjectiveWeights()
	ranked := RankResults(results, weights)

	if len(ranked) != 3 {
		t.Fatalf("expected 3 results, got %d", len(ranked))
	}

	// Results should be sorted by score descending
	for i := 0; i < len(ranked)-1; i++ {
		if ranked[i].Score < ranked[i+1].Score {
			t.Errorf("results not sorted: score[%d]=%v < score[%d]=%v", i, ranked[i].Score, i+1, ranked[i+1].Score)
		}
	}

	// Best result should have highest acceptance (0.9)
	if ranked[0].OverallAcceptMean != 0.9 {
		t.Errorf("expected best result to have acceptance 0.9, got %v", ranked[0].OverallAcceptMean)
	}
}

func TestRankResultsEmpty(t *testing.T) {
	results := []ComboResult{}
	weights := DefaultObjectiveWeights()
	ranked := RankResults(results, weights)

	if len(ranked) != 0 {
		t.Errorf("expected empty result for empty input, got %d results", len(ranked))
	}
}

func TestRankResultsTiedScores(t *testing.T) {
	results := []ComboResult{
		{OverallAcceptMean: 0.8, MisalignmentRatioMean: 0.2, AlignmentDegMean: 5.0, NonzeroCellsMean: 100.0},
		{OverallAcceptMean: 0.8, MisalignmentRatioMean: 0.2, AlignmentDegMean: 5.0, NonzeroCellsMean: 100.0},
	}

	weights := DefaultObjectiveWeights()
	ranked := RankResults(results, weights)

	if len(ranked) != 2 {
		t.Fatalf("expected 2 results, got %d", len(ranked))
	}

	// Scores should be equal
	if math.Abs(ranked[0].Score-ranked[1].Score) > 0.0001 {
		t.Errorf("expected tied scores, got %v and %v", ranked[0].Score, ranked[1].Score)
	}
}
