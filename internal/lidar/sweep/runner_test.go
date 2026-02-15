package sweep

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestComputeCombinationsMulti(t *testing.T) {
	r := newQuietRunner(nil)
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
	r := newQuietRunner(nil)
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
	r := newQuietRunner(nil)
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
	r := newQuietRunner(nil)
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
	r := newQuietRunner(nil)
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
	r := newQuietRunner(nil)
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
	r := newQuietRunner(nil)
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
	r := newQuietRunner(nil)
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
	r := newQuietRunner(nil)
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

// --- Generic param tests ---

func TestCartesianProductSingle(t *testing.T) {
	params := []SweepParam{
		{Name: "noise_relative", Values: []interface{}{0.01, 0.02, 0.03}},
	}
	combos, err := cartesianProduct(params)
	if err != nil {
		t.Fatalf("cartesianProduct failed: %v", err)
	}
	if len(combos) != 3 {
		t.Fatalf("expected 3 combos, got %d", len(combos))
	}
	for i, v := range []float64{0.01, 0.02, 0.03} {
		if combos[i]["noise_relative"] != v {
			t.Errorf("combo[%d]: expected noise_relative=%v, got %v", i, v, combos[i]["noise_relative"])
		}
	}
}

func TestCartesianProductMulti(t *testing.T) {
	params := []SweepParam{
		{Name: "a", Values: []interface{}{1, 2}},
		{Name: "b", Values: []interface{}{"x", "y", "z"}},
	}
	combos, err := cartesianProduct(params)
	if err != nil {
		t.Fatalf("cartesianProduct failed: %v", err)
	}
	if len(combos) != 6 {
		t.Fatalf("expected 6 combos, got %d", len(combos))
	}
	// First combo should be a=1, b="x"
	if combos[0]["a"] != 1 || combos[0]["b"] != "x" {
		t.Errorf("combo[0]: expected a=1 b=x, got %v", combos[0])
	}
	// Last combo should be a=2, b="z"
	if combos[5]["a"] != 2 || combos[5]["b"] != "z" {
		t.Errorf("combo[5]: expected a=2 b=z, got %v", combos[5])
	}
}

func TestCartesianProductEmpty(t *testing.T) {
	combos, err := cartesianProduct(nil)
	if err != nil {
		t.Fatalf("cartesianProduct failed: %v", err)
	}
	if combos != nil {
		t.Errorf("expected nil for empty params, got %v", combos)
	}
}

func TestCartesianProductExcessiveCombinations(t *testing.T) {
	// Test that excessive combinations are rejected before memory allocation (DoS protection)
	params := []SweepParam{
		{Name: "param1", Values: []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}, // 10 values
		{Name: "param2", Values: []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}, // 10 values
		{Name: "param3", Values: []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}, // 10 values
		{Name: "param4", Values: []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}, // 10 values
		// Total = 10^4 = 10,000 combinations (at the limit)
	}

	// This should succeed (exactly at maxCombos limit)
	combos, err := cartesianProduct(params)
	if err != nil {
		t.Fatalf("expected success at limit, got error: %v", err)
	}
	if len(combos) != 10000 {
		t.Errorf("expected 10000 combos, got %d", len(combos))
	}

	// Now add one more value to exceed the limit
	params = append(params, SweepParam{Name: "param5", Values: []interface{}{1, 2}})
	// Total would be 10^4 * 2 = 20,000 combinations (exceeds limit)

	_, err = cartesianProduct(params)
	if err == nil {
		t.Fatal("expected error for excessive combinations, got success")
	}
	if !strings.Contains(err.Error(), "exceed safe limit") {
		t.Errorf("expected 'exceed safe limit' error, got: %v", err)
	}
}

func TestExpandSweepParamFloat64Range(t *testing.T) {
	sp := SweepParam{Name: "noise_relative", Type: "float64", Start: 0.01, End: 0.03, Step: 0.01}
	if err := expandSweepParam(&sp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sp.Values) != 3 {
		t.Fatalf("expected 3 values, got %d", len(sp.Values))
	}
}

func TestExpandSweepParamIntRange(t *testing.T) {
	sp := SweepParam{Name: "hits_to_confirm", Type: "int", Start: 1, End: 5, Step: 2}
	if err := expandSweepParam(&sp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sp.Values) != 3 {
		t.Fatalf("expected 3 values (1,3,5), got %d", len(sp.Values))
	}
	expected := []int{1, 3, 5}
	for i, v := range expected {
		if sp.Values[i] != v {
			t.Errorf("value[%d]: expected %d, got %v", i, v, sp.Values[i])
		}
	}
}

func TestExpandSweepParamInt64Range(t *testing.T) {
	sp := SweepParam{Name: "warmup_duration_nanos", Type: "int64", Start: 1000000, End: 3000000, Step: 1000000}
	if err := expandSweepParam(&sp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sp.Values) != 3 {
		t.Fatalf("expected 3 values, got %d", len(sp.Values))
	}
}

func TestExpandSweepParamBool(t *testing.T) {
	sp := SweepParam{Name: "seed_from_first", Type: "bool"}
	if err := expandSweepParam(&sp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sp.Values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(sp.Values))
	}
	if sp.Values[0] != true || sp.Values[1] != false {
		t.Errorf("expected [true, false], got %v", sp.Values)
	}
}

func TestExpandSweepParamStringRequiresValues(t *testing.T) {
	sp := SweepParam{Name: "buffer_timeout", Type: "string"}
	err := expandSweepParam(&sp)
	if err == nil {
		t.Error("expected error for string param without explicit values")
	}
}

func TestExpandSweepParamExplicitValues(t *testing.T) {
	sp := SweepParam{Name: "noise_relative", Type: "float64", Values: []interface{}{"0.01", "0.05"}}
	if err := expandSweepParam(&sp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Values should be coerced from string to float64
	if sp.Values[0] != 0.01 {
		t.Errorf("expected 0.01, got %v (type %T)", sp.Values[0], sp.Values[0])
	}
	if sp.Values[1] != 0.05 {
		t.Errorf("expected 0.05, got %v (type %T)", sp.Values[1], sp.Values[1])
	}
}

func TestCoerceValueFloat64(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected interface{}
	}{
		{42.0, 42.0},
		{"3.14", 3.14},
		{true, 1.0},
		{false, 0.0},
	}
	for _, tc := range tests {
		got, err := coerceValue(tc.input, "float64")
		if err != nil {
			t.Errorf("coerceValue(%v, float64) unexpected error: %v", tc.input, err)
		}
		if got != tc.expected {
			t.Errorf("coerceValue(%v, float64) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestCoerceValueInt(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected interface{}
	}{
		{42.0, 42},
		{"7", 7},
	}
	for _, tc := range tests {
		got, err := coerceValue(tc.input, "int")
		if err != nil {
			t.Errorf("coerceValue(%v, int) unexpected error: %v", tc.input, err)
		}
		if got != tc.expected {
			t.Errorf("coerceValue(%v, int) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestCoerceValueBool(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected interface{}
	}{
		{true, true},
		{false, false},
		{"true", true},
		{"false", false},
		{1.0, true},
		{0.0, false},
	}
	for _, tc := range tests {
		got, err := coerceValue(tc.input, "bool")
		if err != nil {
			t.Errorf("coerceValue(%v, bool) unexpected error: %v", tc.input, err)
		}
		if got != tc.expected {
			t.Errorf("coerceValue(%v, bool) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestCoerceValueString(t *testing.T) {
	got, err := coerceValue("  hello  ", "string")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("coerceValue('  hello  ', string) = %v, want 'hello'", got)
	}
	got, err = coerceValue(42.0, "string")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "42" {
		t.Errorf("coerceValue(42.0, string) = %v, want '42'", got)
	}
}

func TestCoerceValueErrors(t *testing.T) {
	// Invalid string→float64 should error
	_, err := coerceValue("not_a_number", "float64")
	if err == nil {
		t.Error("expected error for invalid float64 string")
	}

	// Invalid string→int should error
	_, err = coerceValue("abc", "int")
	if err == nil {
		t.Error("expected error for invalid int string")
	}

	// Unsupported type coercion should error
	_, err = coerceValue(struct{}{}, "float64")
	if err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected float64
		ok       bool
	}{
		{42.0, 42.0, true},
		{int(7), 7.0, true},
		{int64(100), 100.0, true},
		{"nope", 0, false},
		{nil, 0, false},
	}
	for _, tc := range tests {
		got, ok := toFloat64(tc.input)
		if ok != tc.ok || got != tc.expected {
			t.Errorf("toFloat64(%v) = (%v, %v), want (%v, %v)", tc.input, got, ok, tc.expected, tc.ok)
		}
	}
}

func TestToInt(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected int
		ok       bool
	}{
		{int(7), 7, true},
		{42.0, 42, true},
		{int64(100), 100, true},
		{"nope", 0, false},
		{nil, 0, false},
	}
	for _, tc := range tests {
		got, ok := toInt(tc.input)
		if ok != tc.ok || got != tc.expected {
			t.Errorf("toInt(%v) = (%v, %v), want (%v, %v)", tc.input, got, ok, tc.expected, tc.ok)
		}
	}
}

func TestStartGenericRejectsExcessiveCombinations(t *testing.T) {
	r := newQuietRunner(nil)
	// 100 * 100 = 10000 > 1000
	vals100 := make([]interface{}, 100)
	for i := range vals100 {
		vals100[i] = float64(i)
	}
	err := r.StartWithRequest(nil, SweepRequest{
		Iterations: 1,
		Params: []SweepParam{
			{Name: "noise_relative", Type: "float64", Values: vals100},
			{Name: "closeness_multiplier", Type: "float64", Values: vals100},
		},
	})
	if err == nil {
		t.Error("expected error for excessive generic combinations, got nil")
	}
}

func TestStartGenericRejectsEmptyParam(t *testing.T) {
	r := newQuietRunner(nil)
	err := r.StartWithRequest(nil, SweepRequest{
		Iterations: 1,
		Params: []SweepParam{
			{Name: "buffer_timeout", Type: "string"}, // no values, string requires explicit
		},
	})
	if err == nil {
		t.Error("expected error for string param without values, got nil")
	}
}

func TestCartesianProductOrder(t *testing.T) {
	params := []SweepParam{
		{Name: "a", Values: []interface{}{1, 2}},
		{Name: "b", Values: []interface{}{10, 20}},
	}
	combos, err := cartesianProduct(params)
	if err != nil {
		t.Fatalf("cartesianProduct failed: %v", err)
	}
	expected := []map[string]interface{}{
		{"a": 1, "b": 10},
		{"a": 1, "b": 20},
		{"a": 2, "b": 10},
		{"a": 2, "b": 20},
	}
	if len(combos) != len(expected) {
		t.Fatalf("expected %d combos, got %d", len(expected), len(combos))
	}
	for i, exp := range expected {
		if !reflect.DeepEqual(combos[i], exp) {
			t.Errorf("combo[%d]: expected %v, got %v", i, exp, combos[i])
		}
	}
}

func TestComputeComboResult(t *testing.T) {
	r := newQuietRunner(nil)
	buckets := []string{"1", "2", "4"}

	results := []SampleResult{
		{
			AcceptanceRates:        []float64{0.80, 0.90, 0.70},
			NonzeroCells:           100,
			OverallAcceptPct:       0.80,
			ActiveTracks:           3,
			MeanAlignmentDeg:       10.0,
			MisalignmentRatio:      0.1,
			ForegroundCaptureRatio: 0.8,
			UnboundedPointRatio:    0.2,
			EmptyBoxRatio:          0.1,
		},
		{
			AcceptanceRates:        []float64{0.90, 0.80, 0.60},
			NonzeroCells:           120,
			OverallAcceptPct:       0.76,
			ActiveTracks:           5,
			MeanAlignmentDeg:       20.0,
			MisalignmentRatio:      0.3,
			ForegroundCaptureRatio: 0.6,
			UnboundedPointRatio:    0.4,
			EmptyBoxRatio:          0.3,
		},
	}

	combo := r.computeComboResult(0.04, 8.0, 7, results, buckets)

	// Legacy fields
	if combo.Noise != 0.04 {
		t.Errorf("expected noise 0.04, got %f", combo.Noise)
	}
	if combo.Closeness != 8.0 {
		t.Errorf("expected closeness 8.0, got %f", combo.Closeness)
	}
	if combo.Neighbour != 7 {
		t.Errorf("expected neighbour 7, got %d", combo.Neighbour)
	}

	// Overall acceptance: mean of 0.80 and 0.76 = 0.78
	if math.Abs(combo.OverallAcceptMean-0.78) > 0.001 {
		t.Errorf("expected overall_accept_mean ~0.78, got %f", combo.OverallAcceptMean)
	}
	if combo.OverallAcceptStddev <= 0 {
		t.Error("expected nonzero overall_accept_stddev")
	}

	// Nonzero cells: mean of 100 and 120 = 110
	if math.Abs(combo.NonzeroCellsMean-110) > 0.001 {
		t.Errorf("expected nonzero_cells_mean ~110, got %f", combo.NonzeroCellsMean)
	}

	// Per-bucket means: (0.80+0.90)/2=0.85, (0.90+0.80)/2=0.85, (0.70+0.60)/2=0.65
	expectedBucketMeans := []float64{0.85, 0.85, 0.65}
	for i, exp := range expectedBucketMeans {
		if math.Abs(combo.BucketMeans[i]-exp) > 0.001 {
			t.Errorf("bucket[%d] mean: expected %f, got %f", i, exp, combo.BucketMeans[i])
		}
	}

	// Active tracks: mean of 3 and 5 = 4
	if math.Abs(combo.ActiveTracksMean-4.0) > 0.001 {
		t.Errorf("expected active_tracks_mean ~4, got %f", combo.ActiveTracksMean)
	}

	// Alignment: mean of 10 and 20 = 15
	if math.Abs(combo.AlignmentDegMean-15.0) > 0.001 {
		t.Errorf("expected alignment_deg_mean ~15, got %f", combo.AlignmentDegMean)
	}

	// Misalignment ratio: mean of 0.1 and 0.3 = 0.2
	if math.Abs(combo.MisalignmentRatioMean-0.2) > 0.001 {
		t.Errorf("expected misalignment_ratio_mean ~0.2, got %f", combo.MisalignmentRatioMean)
	}

	// Foreground capture: mean of 0.8 and 0.6 = 0.7
	if math.Abs(combo.ForegroundCaptureMean-0.7) > 0.001 {
		t.Errorf("expected foreground_capture_mean ~0.7, got %f", combo.ForegroundCaptureMean)
	}

	// Unbounded point: mean of 0.2 and 0.4 = 0.3
	if math.Abs(combo.UnboundedPointMean-0.3) > 0.001 {
		t.Errorf("expected unbounded_point_mean ~0.3, got %f", combo.UnboundedPointMean)
	}

	// Empty box ratio: mean of 0.1 and 0.3 = 0.2
	if math.Abs(combo.EmptyBoxRatioMean-0.2) > 0.001 {
		t.Errorf("expected empty_box_ratio_mean ~0.2, got %f", combo.EmptyBoxRatioMean)
	}
}

func TestComputeComboResultEmpty(t *testing.T) {
	r := newQuietRunner(nil)
	combo := r.computeComboResult(0.04, 8.0, 7, nil, []string{"1"})

	if combo.OverallAcceptMean != 0 {
		t.Errorf("expected 0 for empty results, got %f", combo.OverallAcceptMean)
	}
	if combo.ActiveTracksMean != 0 {
		t.Errorf("expected 0 for empty results, got %f", combo.ActiveTracksMean)
	}
}
