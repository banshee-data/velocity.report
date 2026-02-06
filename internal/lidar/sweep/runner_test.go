package sweep

import (
	"testing"
)

func TestComputeCombinationsMulti(t *testing.T) {
	r := NewRunner(nil)
	noise, closeness, neighbour := r.computeCombinations(SweepRequest{
		Mode:            "multi",
		NoiseValues:     []float64{0.01, 0.02},
		ClosenessValues: []float64{1.5},
		NeighbourValues: []int{0, 1},
	})
	if len(noise) != 2 {
		t.Errorf("expected 2 noise values, got %d", len(noise))
	}
	if len(closeness) != 1 {
		t.Errorf("expected 1 closeness value, got %d", len(closeness))
	}
	if len(neighbour) != 2 {
		t.Errorf("expected 2 neighbour values, got %d", len(neighbour))
	}
}

func TestComputeCombinationsNoise(t *testing.T) {
	r := NewRunner(nil)
	noise, closeness, neighbour := r.computeCombinations(SweepRequest{
		Mode:           "noise",
		NoiseStart:     0.01,
		NoiseEnd:       0.03,
		NoiseStep:      0.01,
		FixedCloseness: 2.0,
		FixedNeighbour: 1,
	})
	if len(noise) != 3 {
		t.Errorf("expected 3 noise values, got %d", len(noise))
	}
	if closeness[0] != 2.0 {
		t.Errorf("expected fixed closeness 2.0, got %f", closeness[0])
	}
	if neighbour[0] != 1 {
		t.Errorf("expected fixed neighbour 1, got %d", neighbour[0])
	}
}

func TestComputeCombinationsCloseness(t *testing.T) {
	r := NewRunner(nil)
	noise, closeness, neighbour := r.computeCombinations(SweepRequest{
		Mode:           "closeness",
		ClosenessStart: 1.5,
		ClosenessEnd:   2.5,
		ClosenessStep:  0.5,
		FixedNoise:     0.02,
		FixedNeighbour: 1,
	})
	if len(closeness) != 3 {
		t.Errorf("expected 3 closeness values, got %d", len(closeness))
	}
	if noise[0] != 0.02 {
		t.Errorf("expected fixed noise 0.02, got %f", noise[0])
	}
	if neighbour[0] != 1 {
		t.Errorf("expected fixed neighbour 1, got %d", neighbour[0])
	}
}

func TestComputeCombinationsNeighbour(t *testing.T) {
	r := NewRunner(nil)
	noise, closeness, neighbour := r.computeCombinations(SweepRequest{
		Mode:           "neighbour",
		NeighbourStart: 0,
		NeighbourEnd:   2,
		NeighbourStep:  1,
		FixedNoise:     0.02,
		FixedCloseness: 2.0,
	})
	if len(neighbour) != 3 {
		t.Errorf("expected 3 neighbour values, got %d", len(neighbour))
	}
	if noise[0] != 0.02 {
		t.Errorf("expected fixed noise 0.02, got %f", noise[0])
	}
	if closeness[0] != 2.0 {
		t.Errorf("expected fixed closeness 2.0, got %f", closeness[0])
	}
}

func TestNewRunnerState(t *testing.T) {
	r := NewRunner(nil)
	state := r.GetSweepState()
	if state.Status != SweepStatusIdle {
		t.Errorf("expected idle status, got %s", state.Status)
	}
	if state.TotalCombos != 0 {
		t.Errorf("expected 0 total combos, got %d", state.TotalCombos)
	}
	if state.CompletedCombos != 0 {
		t.Errorf("expected 0 completed combos, got %d", state.CompletedCombos)
	}
	if len(state.Results) != 0 {
		t.Errorf("expected empty results, got %d", len(state.Results))
	}
}

func TestComputeCombinationsDefaults(t *testing.T) {
	r := NewRunner(nil)
	noise, closeness, neighbour := r.computeCombinations(SweepRequest{
		Mode: "multi",
	})
	// Should get default values
	if len(noise) == 0 {
		t.Error("expected default noise values, got empty")
	}
	if len(closeness) == 0 {
		t.Error("expected default closeness values, got empty")
	}
	if len(neighbour) == 0 {
		t.Error("expected default neighbour values, got empty")
	}
}

func TestComputeCombinationsMultiWithRanges(t *testing.T) {
	r := NewRunner(nil)
	noise, closeness, neighbour := r.computeCombinations(SweepRequest{
		Mode:           "multi",
		NoiseStart:     0.01,
		NoiseEnd:       0.02,
		NoiseStep:      0.005,
		ClosenessStart: 1.5,
		ClosenessEnd:   2.0,
		ClosenessStep:  0.5,
		NeighbourStart: 0,
		NeighbourEnd:   1,
		NeighbourStep:  1,
	})
	if len(noise) != 3 {
		t.Errorf("expected 3 noise values from range, got %d", len(noise))
	}
	if len(closeness) != 2 {
		t.Errorf("expected 2 closeness values from range, got %d", len(closeness))
	}
	if len(neighbour) != 2 {
		t.Errorf("expected 2 neighbour values from range, got %d", len(neighbour))
	}
}

func TestStartRejectsExcessiveCombinations(t *testing.T) {
	r := NewRunner(nil)
	// Generate a request that would produce >1000 combinations
	err := r.StartWithRequest(nil, SweepRequest{
		Mode:       "noise",
		NoiseStart: 0.0001,
		NoiseEnd:   1.0,
		NoiseStep:  0.0001, // ~10000 values
		Iterations: 1,
	})
	if err == nil {
		t.Error("expected error for excessive combinations, got nil")
	}
}

func TestStartRejectsExcessiveIterations(t *testing.T) {
	r := NewRunner(nil)
	err := r.StartWithRequest(nil, SweepRequest{
		Mode:           "noise",
		NoiseStart:     0.01,
		NoiseEnd:       0.02,
		NoiseStep:      0.01,
		Iterations:     501,
		FixedCloseness: 2.0,
		FixedNeighbour: 1,
	})
	if err == nil {
		t.Error("expected error for excessive iterations, got nil")
	}
}
