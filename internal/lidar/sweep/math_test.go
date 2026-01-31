package sweep

import (
	"math"
	"testing"
)

func TestParseCSVFloat64s(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expected  []float64
		expectErr bool
	}{
		{"empty_string", "", nil, false},
		{"single_value", "1.5", []float64{1.5}, false},
		{"multiple_values", "1.0,2.5,3.0", []float64{1.0, 2.5, 3.0}, false},
		{"with_spaces", " 1.0 , 2.5 , 3.0 ", []float64{1.0, 2.5, 3.0}, false},
		{"integers", "1,2,3", []float64{1.0, 2.0, 3.0}, false},
		{"negative_values", "-1.5,-2.5", []float64{-1.5, -2.5}, false},
		{"scientific_notation", "1e-3,2e2", []float64{0.001, 200}, false},
		{"invalid_value", "1.0,abc,3.0", nil, true},
		{"empty_parts", "1.0,,3.0", []float64{1.0, 3.0}, false},
		{"trailing_comma", "1.0,2.0,", []float64{1.0, 2.0}, false},
		{"leading_comma", ",1.0,2.0", []float64{1.0, 2.0}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseCSVFloat64s(tc.input)
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
			if len(result) != len(tc.expected) {
				t.Errorf("Length mismatch: expected %d, got %d", len(tc.expected), len(result))
				return
			}
			for i, v := range result {
				if v != tc.expected[i] {
					t.Errorf("Value mismatch at index %d: expected %f, got %f", i, tc.expected[i], v)
				}
			}
		})
	}
}

func TestParseCSVInts(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expected  []int
		expectErr bool
	}{
		{"empty_string", "", nil, false},
		{"single_value", "5", []int{5}, false},
		{"multiple_values", "1,2,3", []int{1, 2, 3}, false},
		{"with_spaces", " 1 , 2 , 3 ", []int{1, 2, 3}, false},
		{"negative_values", "-1,-2,-3", []int{-1, -2, -3}, false},
		{"invalid_float", "1.5", nil, true},
		{"invalid_string", "abc", nil, true},
		{"mixed_valid_invalid", "1,abc,3", nil, true},
		{"empty_parts", "1,,3", []int{1, 3}, false},
		{"zero", "0", []int{0}, false},
		{"large_number", "1000000", []int{1000000}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseCSVInts(tc.input)
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
			if len(result) != len(tc.expected) {
				t.Errorf("Length mismatch: expected %d, got %d", len(tc.expected), len(result))
				return
			}
			for i, v := range result {
				if v != tc.expected[i] {
					t.Errorf("Value mismatch at index %d: expected %d, got %d", i, tc.expected[i], v)
				}
			}
		})
	}
}

func TestToFloat64Slice(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		length   int
		expected []float64
	}{
		{"nil_input", nil, 3, []float64{0, 0, 0}},
		{"empty_interface_slice", []interface{}{}, 3, []float64{0, 0, 0}},
		{"float64_values", []interface{}{1.5, 2.5, 3.5}, 3, []float64{1.5, 2.5, 3.5}},
		{"int_values", []interface{}{1, 2, 3}, 3, []float64{1, 2, 3}},
		{"int64_values", []interface{}{int64(1), int64(2), int64(3)}, 3, []float64{1, 2, 3}},
		{"mixed_types", []interface{}{1.5, 2, int64(3)}, 3, []float64{1.5, 2, 3}},
		{"shorter_input", []interface{}{1.0, 2.0}, 4, []float64{1.0, 2.0, 0, 0}},
		{"longer_input", []interface{}{1.0, 2.0, 3.0, 4.0}, 2, []float64{1.0, 2.0}},
		{"unknown_type", []interface{}{"string", true}, 2, []float64{0, 0}},
		{"native_float64_slice", []float64{1.5, 2.5}, 2, []float64{1.5, 2.5}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ToFloat64Slice(tc.input, tc.length)
			if len(result) != len(tc.expected) {
				t.Errorf("Length mismatch: expected %d, got %d", len(tc.expected), len(result))
				return
			}
			for i, v := range result {
				if v != tc.expected[i] {
					t.Errorf("Value mismatch at index %d: expected %f, got %f", i, tc.expected[i], v)
				}
			}
		})
	}
}

func TestToInt64Slice(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		length   int
		expected []int64
	}{
		{"nil_input", nil, 3, []int64{0, 0, 0}},
		{"empty_interface_slice", []interface{}{}, 3, []int64{0, 0, 0}},
		{"float64_values", []interface{}{1.0, 2.0, 3.0}, 3, []int64{1, 2, 3}},
		{"int_values", []interface{}{1, 2, 3}, 3, []int64{1, 2, 3}},
		{"int64_values", []interface{}{int64(1), int64(2), int64(3)}, 3, []int64{1, 2, 3}},
		{"mixed_types", []interface{}{1.0, 2, int64(3)}, 3, []int64{1, 2, 3}},
		{"shorter_input", []interface{}{int64(1), int64(2)}, 4, []int64{1, 2, 0, 0}},
		{"longer_input", []interface{}{int64(1), int64(2), int64(3), int64(4)}, 2, []int64{1, 2}},
		{"unknown_type", []interface{}{"string", true}, 2, []int64{0, 0}},
		{"native_int64_slice", []int64{100, 200}, 2, []int64{100, 200}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ToInt64Slice(tc.input, tc.length)
			if len(result) != len(tc.expected) {
				t.Errorf("Length mismatch: expected %d, got %d", len(tc.expected), len(result))
				return
			}
			for i, v := range result {
				if v != tc.expected[i] {
					t.Errorf("Value mismatch at index %d: expected %d, got %d", i, tc.expected[i], v)
				}
			}
		})
	}
}

func TestMeanStddev(t *testing.T) {
	testCases := []struct {
		name           string
		input          []float64
		expectedMean   float64
		expectedStddev float64
	}{
		{"empty_slice", []float64{}, 0, 0},
		{"single_value", []float64{5.0}, 5.0, 0},
		{"two_values", []float64{4.0, 6.0}, 5.0, math.Sqrt(2)},
		{"three_values", []float64{1.0, 2.0, 3.0}, 2.0, 1.0},
		{"identical_values", []float64{5.0, 5.0, 5.0}, 5.0, 0},
		{"negative_values", []float64{-1.0, -2.0, -3.0}, -2.0, 1.0},
		{"mixed_signs", []float64{-1.0, 0.0, 1.0}, 0.0, 1.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mean, stddev := MeanStddev(tc.input)

			if math.Abs(mean-tc.expectedMean) > 1e-9 {
				t.Errorf("Mean mismatch: expected %f, got %f", tc.expectedMean, mean)
			}
			if math.Abs(stddev-tc.expectedStddev) > 1e-9 {
				t.Errorf("Stddev mismatch: expected %f, got %f", tc.expectedStddev, stddev)
			}
		})
	}
}

func TestMeanStddev_LargeDataset(t *testing.T) {
	// Test with a larger dataset
	data := make([]float64, 1000)
	for i := range data {
		data[i] = float64(i)
	}

	mean, stddev := MeanStddev(data)

	expectedMean := 499.5
	if math.Abs(mean-expectedMean) > 1e-9 {
		t.Errorf("Mean mismatch: expected %f, got %f", expectedMean, mean)
	}

	// Standard deviation of 0..999 is sqrt((999-0+1)^2-1)/12) ≈ 288.675
	expectedStddev := 288.8194360957494 // Sample stddev
	if math.Abs(stddev-expectedStddev) > 0.01 {
		t.Errorf("Stddev mismatch: expected ~%f, got %f", expectedStddev, stddev)
	}
}
