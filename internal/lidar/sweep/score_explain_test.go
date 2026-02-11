package sweep

import (
	"testing"
)

func TestTopContributors(t *testing.T) {
	sc := ScoreComponents{
		DetectionRate:     0.8,
		Fragmentation:     -0.3,
		FalsePositives:    -0.5,
		VelocityCoverage:  0.2,
		QualityPremium:    0.1,
		TruncationRate:    -0.05,
		VelocityNoiseRate: -0.02,
		StoppedRecovery:   0.01,
	}

	top := topContributors(sc, 3)
	if len(top) != 3 {
		t.Fatalf("expected 3 contributors, got %d", len(top))
	}
	// detection_rate (0.8), false_positives (0.5), fragmentation (0.3)
	if top[0] != "detection_rate" {
		t.Errorf("expected detection_rate first, got %s", top[0])
	}
	if top[1] != "false_positives" {
		t.Errorf("expected false_positives second, got %s", top[1])
	}
	if top[2] != "fragmentation" {
		t.Errorf("expected fragmentation third, got %s", top[2])
	}
}

func TestTopContributors_RequestMoreThanAvailable(t *testing.T) {
	sc := ScoreComponents{DetectionRate: 1.0}
	top := topContributors(sc, 100)
	if len(top) != 8 {
		t.Errorf("expected 8 contributors (all), got %d", len(top))
	}
}

func TestDeltaComponents(t *testing.T) {
	current := ScoreComponents{
		DetectionRate:  0.8,
		Fragmentation:  -0.3,
		CompositeScore: 0.5,
	}
	previous := ScoreComponents{
		DetectionRate:  0.6,
		Fragmentation:  -0.4,
		CompositeScore: 0.2,
	}

	delta := DeltaComponents(current, previous)
	const epsilon = 1e-9
	if abs(delta.DetectionRate-0.2) > epsilon {
		t.Errorf("expected detection_rate delta 0.2, got %f", delta.DetectionRate)
	}
	if abs(delta.Fragmentation-0.1) > epsilon {
		t.Errorf("expected fragmentation delta 0.1, got %f", delta.Fragmentation)
	}
	if abs(delta.CompositeScore-0.3) > epsilon {
		t.Errorf("expected composite_score delta 0.3, got %f", delta.CompositeScore)
	}
}

func TestBuildExplanation(t *testing.T) {
	components := ScoreComponents{
		DetectionRate:  0.8,
		FalsePositives: -0.5,
		Fragmentation:  -0.3,
		CompositeScore: 0.5,
	}

	t.Run("without previous", func(t *testing.T) {
		exp := BuildExplanation(components, nil, 0.85)
		if exp.LabelCoverageConfidence != 0.85 {
			t.Errorf("expected label_coverage_confidence 0.85, got %f", exp.LabelCoverageConfidence)
		}
		if len(exp.TopContributors) != 3 {
			t.Errorf("expected 3 top contributors, got %d", len(exp.TopContributors))
		}
		if exp.DeltaVsPrevious != nil {
			t.Error("expected DeltaVsPrevious to be nil")
		}
	})

	t.Run("with previous", func(t *testing.T) {
		previous := ScoreComponents{
			DetectionRate:  0.6,
			CompositeScore: 0.2,
		}
		exp := BuildExplanation(components, &previous, 0.90)
		if exp.DeltaVsPrevious == nil {
			t.Fatal("expected DeltaVsPrevious to be non-nil")
		}
		if exp.DeltaVsPrevious.CompositeScore != 0.3 {
			t.Errorf("expected composite delta 0.3, got %f", exp.DeltaVsPrevious.CompositeScore)
		}
	})
}

func TestAbs(t *testing.T) {
	if abs(-5.0) != 5.0 {
		t.Error("abs(-5.0) should be 5.0")
	}
	if abs(5.0) != 5.0 {
		t.Error("abs(5.0) should be 5.0")
	}
	if abs(0.0) != 0.0 {
		t.Error("abs(0.0) should be 0.0")
	}
}
