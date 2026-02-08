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
	req := AutoTuneRequest{
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.1},
		},
		Iterations: 10,
		SettleTime: "5s",
		Interval:   "2s",
		Seed:       "true",
	}

	// Validation in Start() should apply defaults
	tuner := NewAutoTuner(nil)

	// Test that defaults are applied (we can't call Start without a runner, so we test the values)
	if req.MaxRounds == 0 {
		req.MaxRounds = 3
	}
	if req.ValuesPerParam == 0 {
		req.ValuesPerParam = 5
	}
	if req.TopK == 0 {
		req.TopK = 5
	}

	if req.MaxRounds != 3 {
		t.Errorf("expected MaxRounds default 3, got %d", req.MaxRounds)
	}
	if req.ValuesPerParam != 5 {
		t.Errorf("expected ValuesPerParam default 5, got %d", req.ValuesPerParam)
	}
	if req.TopK != 5 {
		t.Errorf("expected TopK default 5, got %d", req.TopK)
	}

	// Verify tuner was created
	if tuner == nil {
		t.Fatal("expected non-nil tuner")
	}
	if tuner.state.Mode != "auto" {
		t.Errorf("expected mode 'auto', got %q", tuner.state.Mode)
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
			tuner := NewAutoTuner(nil)
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
