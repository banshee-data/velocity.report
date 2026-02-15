package sweep

import (
	"math"
	"strings"
	"testing"
)

func TestNarrowBounds(t *testing.T) {
	topK := []ScoredResult{
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"noise_relative": 0.02}}, Score: 0.9},
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"noise_relative": 0.04}}, Score: 0.85},
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"noise_relative": 0.03}}, Score: 0.8},
	}

	start, end := narrowBounds(topK, "noise_relative", 5)

	// Min=0.02, Max=0.04, range=0.02, step=0.02/4=0.005
	// Margin = 1 step = 0.005
	// Expected: start=0.02-0.005=0.015, end=0.04+0.005=0.045
	expectedStart := 0.015
	expectedEnd := 0.045

	if math.Abs(start-expectedStart) > 0.0001 {
		t.Errorf("expected start ~%v, got %v", expectedStart, start)
	}
	if math.Abs(end-expectedEnd) > 0.0001 {
		t.Errorf("expected end ~%v, got %v", expectedEnd, end)
	}
}

func TestNarrowBoundsWithMargin(t *testing.T) {
	topK := []ScoredResult{
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"closeness_multiplier": 4.0}}, Score: 0.9},
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"closeness_multiplier": 8.0}}, Score: 0.85},
	}

	start, end := narrowBounds(topK, "closeness_multiplier", 5)

	// Min=4.0, Max=8.0, range=4.0, step=4.0/4=1.0
	// Margin = 1 step = 1.0
	// Expected: start=4.0-1.0=3.0, end=8.0+1.0=9.0
	expectedStart := 3.0
	expectedEnd := 9.0

	if math.Abs(start-expectedStart) > 0.0001 {
		t.Errorf("expected start ~%v, got %v", expectedStart, start)
	}
	if math.Abs(end-expectedEnd) > 0.0001 {
		t.Errorf("expected end ~%v, got %v", expectedEnd, end)
	}
}

func TestNarrowBoundsSingleResult(t *testing.T) {
	topK := []ScoredResult{
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"noise_relative": 0.05}}, Score: 0.9},
	}

	start, end := narrowBounds(topK, "noise_relative", 5)

	// Single value: should add a small margin around it
	// Min=Max=0.05, margin = 0.05 * 0.1 = 0.005
	// Expected: start=0.05-0.005=0.045, end=0.05+0.005=0.055
	expectedStart := 0.045
	expectedEnd := 0.055

	if math.Abs(start-expectedStart) > 0.0001 {
		t.Errorf("expected start ~%v, got %v", expectedStart, start)
	}
	if math.Abs(end-expectedEnd) > 0.0001 {
		t.Errorf("expected end ~%v, got %v", expectedEnd, end)
	}
}

func TestNarrowBoundsEmpty(t *testing.T) {
	topK := []ScoredResult{}
	start, end := narrowBounds(topK, "noise_relative", 5)

	if start != 0 || end != 0 {
		t.Errorf("expected (0, 0) for empty topK, got (%v, %v)", start, end)
	}
}

func TestNarrowBoundsIntType(t *testing.T) {
	topK := []ScoredResult{
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"neighbour_count": int(3)}}, Score: 0.9},
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"neighbour_count": int(5)}}, Score: 0.85},
	}

	start, end := narrowBounds(topK, "neighbour_count", 5)

	// Min=3, Max=5, range=2, step=2/4=0.5
	// Margin = 1 step = 0.5
	// Expected: start=3-0.5=2.5, end=5+0.5=5.5
	expectedStart := 2.5
	expectedEnd := 5.5

	if math.Abs(start-expectedStart) > 0.0001 {
		t.Errorf("expected start ~%v, got %v", expectedStart, start)
	}
	if math.Abs(end-expectedEnd) > 0.0001 {
		t.Errorf("expected end ~%v, got %v", expectedEnd, end)
	}
}

func TestNarrowBoundsMissingParam(t *testing.T) {
	// When the parameter is missing from all topK results, should return (0, 0)
	topK := []ScoredResult{
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"other_param": 1.0}}, Score: 0.9},
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"other_param": 2.0}}, Score: 0.85},
	}

	start, end := narrowBounds(topK, "missing_param", 5)
	if start != 0 || end != 0 {
		t.Errorf("expected (0, 0) for missing param, got (%v, %v)", start, end)
	}
}

func TestNarrowBoundsUnsupportedType(t *testing.T) {
	// When the parameter has an unsupported type (string), should return (0, 0)
	topK := []ScoredResult{
		{ComboResult: ComboResult{ParamValues: map[string]interface{}{"mode": "fast"}}, Score: 0.9},
	}

	start, end := narrowBounds(topK, "mode", 5)
	if start != 0 || end != 0 {
		t.Errorf("expected (0, 0) for unsupported type, got (%v, %v)", start, end)
	}
}

func TestGenerateIntGrid(t *testing.T) {
	grid := generateIntGrid(1.0, 10.0, 5)

	// Should produce deduplicated, evenly-spaced integers
	if len(grid) == 0 {
		t.Fatal("expected non-empty grid")
	}

	// First and last should be 1 and 10
	if grid[0] != 1 {
		t.Errorf("expected first value 1, got %d", grid[0])
	}
	if grid[len(grid)-1] != 10 {
		t.Errorf("expected last value 10, got %d", grid[len(grid)-1])
	}

	// All values should be unique
	seen := make(map[int]bool)
	for _, v := range grid {
		if seen[v] {
			t.Errorf("duplicate value %d in grid", v)
		}
		seen[v] = true
	}
}

func TestGenerateIntGridNarrowRange(t *testing.T) {
	// Range 3-5 with n=5 should produce [3,4,5] (deduplicated)
	grid := generateIntGrid(3.0, 5.0, 5)

	if len(grid) != 3 {
		t.Fatalf("expected 3 deduplicated values for range 3-5, got %d: %v", len(grid), grid)
	}

	expected := []int{3, 4, 5}
	for i, v := range expected {
		if grid[i] != v {
			t.Errorf("grid[%d]: expected %d, got %d", i, v, grid[i])
		}
	}
}

func TestGenerateIntGridSingle(t *testing.T) {
	grid := generateIntGrid(2.0, 8.0, 1)
	if len(grid) != 1 {
		t.Fatalf("expected 1 value, got %d", len(grid))
	}
	if grid[0] != 5 {
		t.Errorf("expected midpoint 5, got %d", grid[0])
	}
}

func TestNilRunnerValidation(t *testing.T) {
	tuner := newQuietAutoTuner(nil)
	err := tuner.Start(nil, AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
		},
	})
	if err == nil {
		t.Fatal("expected error for nil runner, got nil")
	}
	if !strings.Contains(err.Error(), "runner is not configured") {
		t.Errorf("expected 'runner is not configured' error, got %q", err.Error())
	}
}

func TestValuesPerParamMinimum(t *testing.T) {
	// values_per_param must be at least 2 for meaningful grid generation
	tuner := newQuietAutoTuner(&Runner{})
	err := tuner.Start(nil, AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
		},
		ValuesPerParam: 1,
	})
	if err == nil {
		t.Fatal("expected error for values_per_param=1, got nil")
	}
	if !strings.Contains(err.Error(), "values_per_param must be at least 2") {
		t.Errorf("expected 'must be at least 2' error, got %q", err.Error())
	}
}

func TestCopyParamValues(t *testing.T) {
	original := map[string]interface{}{
		"noise_relative": 0.04,
		"closeness":      8.0,
	}
	copied := copyParamValues(original)

	// Modifying copy should not affect original
	copied["noise_relative"] = 0.99
	if original["noise_relative"] != 0.04 {
		t.Errorf("original was modified: expected 0.04, got %v", original["noise_relative"])
	}

	// Nil input should return nil
	if copyParamValues(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestGenerateGrid(t *testing.T) {
	grid := generateGrid(0.0, 1.0, 5)

	if len(grid) != 5 {
		t.Fatalf("expected 5 values, got %d", len(grid))
	}

	expected := []float64{0.0, 0.25, 0.5, 0.75, 1.0}
	for i, v := range expected {
		if math.Abs(grid[i]-v) > 0.0001 {
			t.Errorf("grid[%d]: expected %v, got %v", i, v, grid[i])
		}
	}
}

func TestGenerateGridSingle(t *testing.T) {
	grid := generateGrid(2.0, 8.0, 1)

	if len(grid) != 1 {
		t.Fatalf("expected 1 value, got %d", len(grid))
	}

	// Should return midpoint
	expected := 5.0
	if math.Abs(grid[0]-expected) > 0.0001 {
		t.Errorf("expected midpoint %v, got %v", expected, grid[0])
	}
}

func TestGenerateGridZero(t *testing.T) {
	grid := generateGrid(0.0, 1.0, 0)

	if len(grid) != 0 {
		t.Errorf("expected empty grid for n=0, got %d values", len(grid))
	}
}

func TestGenerateGridNegative(t *testing.T) {
	grid := generateGrid(-1.0, 1.0, 3)

	if len(grid) != 3 {
		t.Fatalf("expected 3 values, got %d", len(grid))
	}

	expected := []float64{-1.0, 0.0, 1.0}
	for i, v := range expected {
		if math.Abs(grid[i]-v) > 0.0001 {
			t.Errorf("grid[%d]: expected %v, got %v", i, v, grid[i])
		}
	}
}

func TestAutoTuneRequestDefaults(t *testing.T) {
	// Test that applyAutoTuneDefaults fills in missing values
	req := applyAutoTuneDefaults(AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
		},
		Iterations: 10,
		SettleTime: "5s",
		Interval:   "2s",
		Seed:       "true",
	})

	if req.MaxRounds != 3 {
		t.Errorf("expected MaxRounds default 3, got %d", req.MaxRounds)
	}
	if req.ValuesPerParam != 5 {
		t.Errorf("expected ValuesPerParam default 5, got %d", req.ValuesPerParam)
	}
	if req.TopK != 5 {
		t.Errorf("expected TopK default 5, got %d", req.TopK)
	}
	if req.Objective != "acceptance" {
		t.Errorf("expected Objective default 'acceptance', got %q", req.Objective)
	}

	// Verify explicit values are preserved
	req2 := applyAutoTuneDefaults(AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
		},
		MaxRounds:      7,
		ValuesPerParam: 10,
		TopK:           3,
		Objective:      "weighted",
	})

	if req2.MaxRounds != 7 {
		t.Errorf("expected MaxRounds 7, got %d", req2.MaxRounds)
	}
	if req2.ValuesPerParam != 10 {
		t.Errorf("expected ValuesPerParam 10, got %d", req2.ValuesPerParam)
	}
	if req2.TopK != 3 {
		t.Errorf("expected TopK 3, got %d", req2.TopK)
	}
	if req2.Objective != "weighted" {
		t.Errorf("expected Objective 'weighted', got %q", req2.Objective)
	}
}

func TestAutoTuneValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     AutoTuneRequest
		wantErr string
	}{
		{
			name: "no params",
			req: AutoTuneRequest{
				MaxRounds:      3,
				ValuesPerParam: 5,
				TopK:           5,
			},
			wantErr: "no parameters specified",
		},
		{
			name: "start >= end",
			req: AutoTuneRequest{
				Params: []SweepParam{
					{Name: "noise_relative", Type: "float64", Start: 0.1, End: 0.01},
				},
			},
			wantErr: "start must be less than end",
		},
		{
			name: "non-numeric type",
			req: AutoTuneRequest{
				Params: []SweepParam{
					{Name: "mode", Type: "string", Start: 0.0, End: 1.0},
				},
			},
			wantErr: "only supports numeric types",
		},
		{
			name: "max_rounds too high",
			req: AutoTuneRequest{
				Params: []SweepParam{
					{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
				},
				MaxRounds: 20,
			},
			wantErr: "max_rounds must not exceed 10",
		},
		{
			name: "values_per_param too high",
			req: AutoTuneRequest{
				Params: []SweepParam{
					{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
				},
				ValuesPerParam: 30,
			},
			wantErr: "values_per_param must not exceed 20",
		},
		{
			name: "top_k too high",
			req: AutoTuneRequest{
				Params: []SweepParam{
					{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
				},
				TopK: 100,
			},
			wantErr: "top_k must not exceed 50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a non-nil runner so we reach parameter validation
			tuner := newQuietAutoTuner(&Runner{})
			err := tuner.Start(nil, tt.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// Phase 5: Test ground truth objective validation
func TestGroundTruthObjectiveValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     AutoTuneRequest
		wantErr string
	}{
		{
			name: "ground_truth without scene_id",
			req: AutoTuneRequest{
				Params: []SweepParam{
					{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
				},
				Objective: "ground_truth",
			},
			wantErr: "ground_truth objective requires scene_id to be set",
		},
		{
			name: "ground_truth without scorer",
			req: AutoTuneRequest{
				Params: []SweepParam{
					{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
				},
				Objective: "ground_truth",
				SceneID:   "test-scene",
			},
			wantErr: "ground_truth objective requires a ground truth scorer to be configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tuner := newQuietAutoTuner(&Runner{})
			err := tuner.Start(nil, tt.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestDefaultGroundTruthWeights(t *testing.T) {
	weights := DefaultGroundTruthWeights()

	// Verify default values from design doc
	if weights.DetectionRate != 1.0 {
		t.Errorf("expected DetectionRate 1.0, got %v", weights.DetectionRate)
	}
	if weights.Fragmentation != 5.0 {
		t.Errorf("expected Fragmentation 5.0, got %v", weights.Fragmentation)
	}
	if weights.FalsePositives != 2.0 {
		t.Errorf("expected FalsePositives 2.0, got %v", weights.FalsePositives)
	}
	if weights.VelocityCoverage != 0.5 {
		t.Errorf("expected VelocityCoverage 0.5, got %v", weights.VelocityCoverage)
	}
	if weights.QualityPremium != 0.3 {
		t.Errorf("expected QualityPremium 0.3, got %v", weights.QualityPremium)
	}
	if weights.TruncationRate != 0.4 {
		t.Errorf("expected TruncationRate 0.4, got %v", weights.TruncationRate)
	}
	if weights.VelocityNoiseRate != 0.4 {
		t.Errorf("expected VelocityNoiseRate 0.4, got %v", weights.VelocityNoiseRate)
	}
	if weights.StoppedRecovery != 0.2 {
		t.Errorf("expected StoppedRecovery 0.2, got %v", weights.StoppedRecovery)
	}
}

func TestAutoTuneRequestGroundTruthFields(t *testing.T) {
	// Test that ground truth fields are properly parsed from JSON
	req := AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
		},
		Objective: "ground_truth",
		SceneID:   "test-scene-123",
		GroundTruthWeights: &GroundTruthWeights{
			DetectionRate:  2.0,
			Fragmentation:  10.0,
			FalsePositives: 3.0,
		},
	}

	if req.SceneID != "test-scene-123" {
		t.Errorf("expected SceneID 'test-scene-123', got %q", req.SceneID)
	}
	if req.GroundTruthWeights == nil {
		t.Fatal("expected GroundTruthWeights to be set")
	}
	if req.GroundTruthWeights.DetectionRate != 2.0 {
		t.Errorf("expected DetectionRate 2.0, got %v", req.GroundTruthWeights.DetectionRate)
	}
}
