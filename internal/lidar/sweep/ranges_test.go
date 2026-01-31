package sweep

import (
	"reflect"
	"testing"
)

func TestParseRangeSpec(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expected  RangeSpec
		expectErr bool
	}{
		{"valid_range", "1.0:5.0:0.5", RangeSpec{Min: 1.0, Max: 5.0, Step: 0.5}, false},
		{"integer_range", "0:10:1", RangeSpec{Min: 0, Max: 10, Step: 1}, false},
		{"with_spaces", " 1.0 : 5.0 : 0.5 ", RangeSpec{Min: 1.0, Max: 5.0, Step: 0.5}, false},
		{"negative_values", "-5.0:5.0:1.0", RangeSpec{Min: -5.0, Max: 5.0, Step: 1.0}, false},
		{"small_step", "0.001:0.005:0.001", RangeSpec{Min: 0.001, Max: 0.005, Step: 0.001}, false},
		{"missing_parts", "1.0:5.0", RangeSpec{}, true},
		{"too_many_parts", "1.0:5.0:0.5:2.0", RangeSpec{}, true},
		{"invalid_min", "abc:5.0:0.5", RangeSpec{}, true},
		{"invalid_max", "1.0:abc:0.5", RangeSpec{}, true},
		{"invalid_step", "1.0:5.0:abc", RangeSpec{}, true},
		{"zero_step", "1.0:5.0:0", RangeSpec{}, true},
		{"negative_step", "1.0:5.0:-0.5", RangeSpec{}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseRangeSpec(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected error for input %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if result != tc.expected {
				t.Errorf("Expected %+v, got %+v", tc.expected, result)
			}
		})
	}
}

func TestParseIntRangeSpec(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expected  IntRangeSpec
		expectErr bool
	}{
		{"valid_range", "1:10:2", IntRangeSpec{Min: 1, Max: 10, Step: 2}, false},
		{"with_spaces", " 1 : 10 : 2 ", IntRangeSpec{Min: 1, Max: 10, Step: 2}, false},
		{"negative_values", "-10:10:5", IntRangeSpec{Min: -10, Max: 10, Step: 5}, false},
		{"single_step", "0:100:1", IntRangeSpec{Min: 0, Max: 100, Step: 1}, false},
		{"missing_parts", "1:10", IntRangeSpec{}, true},
		{"too_many_parts", "1:10:2:5", IntRangeSpec{}, true},
		{"float_value", "1.5:10:2", IntRangeSpec{}, true},
		{"invalid_min", "abc:10:2", IntRangeSpec{}, true},
		{"invalid_max", "1:abc:2", IntRangeSpec{}, true},
		{"invalid_step", "1:10:abc", IntRangeSpec{}, true},
		{"zero_step", "1:10:0", IntRangeSpec{}, true},
		{"negative_step", "1:10:-2", IntRangeSpec{}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseIntRangeSpec(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected error for input %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if result != tc.expected {
				t.Errorf("Expected %+v, got %+v", tc.expected, result)
			}
		})
	}
}

func TestGenerateRange(t *testing.T) {
	testCases := []struct {
		name     string
		min      float64
		max      float64
		step     float64
		expected []float64
	}{
		{"simple_range", 1.0, 3.0, 1.0, []float64{1.0, 2.0, 3.0}},
		{"fractional_step", 0.0, 1.0, 0.5, []float64{0.0, 0.5, 1.0}},
		{"single_value", 5.0, 5.0, 1.0, []float64{5.0}},
		{"negative_range", -3.0, -1.0, 1.0, []float64{-3.0, -2.0, -1.0}},
		{"min_greater_than_max", 5.0, 1.0, 1.0, nil},
		{"zero_step", 1.0, 5.0, 0, nil},
		{"negative_step", 1.0, 5.0, -1.0, nil},
		{"small_step", 0.0, 0.003, 0.001, []float64{0.0, 0.001, 0.002, 0.003}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateRange(tc.min, tc.max, tc.step)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestGenerateIntRange(t *testing.T) {
	testCases := []struct {
		name     string
		min      int
		max      int
		step     int
		expected []int
	}{
		{"simple_range", 1, 5, 1, []int{1, 2, 3, 4, 5}},
		{"step_2", 0, 10, 2, []int{0, 2, 4, 6, 8, 10}},
		{"step_3", 0, 10, 3, []int{0, 3, 6, 9}},
		{"single_value", 5, 5, 1, []int{5}},
		{"negative_range", -5, -1, 1, []int{-5, -4, -3, -2, -1}},
		{"min_greater_than_max", 10, 1, 1, nil},
		{"zero_step", 1, 5, 0, nil},
		{"negative_step", 1, 5, -1, nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateIntRange(tc.min, tc.max, tc.step)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestParseParamList(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expected  []float64
		expectErr bool
	}{
		{"empty_string", "", nil, false},
		{"csv_values", "1.0,2.0,3.0", []float64{1.0, 2.0, 3.0}, false},
		{"range_spec", "1:3:1", []float64{1.0, 2.0, 3.0}, false},
		{"range_fractional", "0:1:0.5", []float64{0.0, 0.5, 1.0}, false},
		{"single_value", "5.0", []float64{5.0}, false},
		{"invalid_csv", "1.0,abc,3.0", nil, true},
		{"invalid_range", "1:3", nil, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseParamList(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected error for input %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestParseIntParamList(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expected  []int
		expectErr bool
	}{
		{"empty_string", "", nil, false},
		{"csv_values", "1,2,3", []int{1, 2, 3}, false},
		{"range_spec", "1:5:2", []int{1, 3, 5}, false},
		{"single_value", "5", []int{5}, false},
		{"invalid_csv", "1,abc,3", nil, true},
		{"invalid_range", "1:3", nil, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseIntParamList(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected error for input %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestExpandRanges(t *testing.T) {
	testCases := []struct {
		name      string
		specs     []string
		expected  [][]float64
		expectErr bool
	}{
		{"empty", []string{}, nil, false},
		{"single_spec", []string{"1,2"}, [][]float64{{1}, {2}}, false},
		{"two_specs", []string{"1,2", "3,4"}, [][]float64{
			{1, 3}, {1, 4}, {2, 3}, {2, 4},
		}, false},
		{"range_and_csv", []string{"1:2:1", "10,20"}, [][]float64{
			{1, 10}, {1, 20}, {2, 10}, {2, 20},
		}, false},
		{"empty_spec_defaults", []string{""}, [][]float64{{0}}, false},
		{"invalid_spec", []string{"abc"}, nil, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ExpandRanges(tc.specs...)
			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected error for specs %v, got nil", tc.specs)
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestExpandRanges_CartesianProduct(t *testing.T) {
	// Test 3x3x3 = 27 combinations
	result, err := ExpandRanges("1:3:1", "10:30:10", "100:300:100")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(result) != 27 {
		t.Errorf("Expected 27 combinations, got %d", len(result))
	}

	// Verify first and last combinations
	first := result[0]
	expected := []float64{1, 10, 100}
	if !reflect.DeepEqual(first, expected) {
		t.Errorf("First combination: expected %v, got %v", expected, first)
	}

	last := result[26]
	expected = []float64{3, 30, 300}
	if !reflect.DeepEqual(last, expected) {
		t.Errorf("Last combination: expected %v, got %v", expected, last)
	}
}
