// Package sweep provides utilities for LiDAR parameter sweep operations.
// This package contains functions extracted from cmd/sweep/main.go to enable
// unit testing and reuse across the codebase.
package sweep

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RangeSpec defines a floating-point parameter range for sweeping.
type RangeSpec struct {
	Min  float64
	Max  float64
	Step float64
}

// IntRangeSpec defines an integer parameter range for sweeping.
type IntRangeSpec struct {
	Min  int
	Max  int
	Step int
}

// ParseRangeSpec parses a "min:max:step" string into a RangeSpec.
// Returns an error if the format is invalid or values cannot be parsed.
func ParseRangeSpec(s string) (RangeSpec, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return RangeSpec{}, fmt.Errorf("invalid range format %q: expected min:max:step", s)
	}

	min, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return RangeSpec{}, fmt.Errorf("invalid min value %q: %w", parts[0], err)
	}

	max, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return RangeSpec{}, fmt.Errorf("invalid max value %q: %w", parts[1], err)
	}

	step, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if err != nil {
		return RangeSpec{}, fmt.Errorf("invalid step value %q: %w", parts[2], err)
	}

	if step <= 0 {
		return RangeSpec{}, fmt.Errorf("step must be positive, got %f", step)
	}

	return RangeSpec{Min: min, Max: max, Step: step}, nil
}

// ParseIntRangeSpec parses a "min:max:step" string into an IntRangeSpec.
// Returns an error if the format is invalid or values cannot be parsed.
func ParseIntRangeSpec(s string) (IntRangeSpec, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return IntRangeSpec{}, fmt.Errorf("invalid range format %q: expected min:max:step", s)
	}

	min, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return IntRangeSpec{}, fmt.Errorf("invalid min value %q: %w", parts[0], err)
	}

	max, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return IntRangeSpec{}, fmt.Errorf("invalid max value %q: %w", parts[1], err)
	}

	step, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		return IntRangeSpec{}, fmt.Errorf("invalid step value %q: %w", parts[2], err)
	}

	if step <= 0 {
		return IntRangeSpec{}, fmt.Errorf("step must be positive, got %d", step)
	}

	return IntRangeSpec{Min: min, Max: max, Step: step}, nil
}

// GenerateRange generates a slice of float64 values from min to max (inclusive)
// stepping by step. Returns an empty slice if min > max.
// Limits the number of generated values to prevent excessive memory allocation.
func GenerateRange(min, max, step float64) []float64 {
	if step <= 0 {
		return nil
	}
	if min > max {
		return nil
	}

	// Calculate expected count and enforce limit to prevent DoS
	const maxValues = 10000
	expectedCount := int((max-min)/step) + 1
	if expectedCount > maxValues || expectedCount < 0 {
		// Return empty slice rather than panic - callers should validate ranges
		return nil
	}

	var result []float64
	for v := min; v <= max+step/1000; v += step {
		// Additional safety check during loop
		if len(result) >= maxValues {
			break
		}
		// Round to avoid floating point accumulation errors
		// Use math.Round for proper handling of negative values
		rounded := math.Round(v*1000) / 1000
		if rounded <= max {
			result = append(result, rounded)
		}
	}
	return result
}

// GenerateIntRange generates a slice of int values from min to max (inclusive)
// stepping by step. Returns an empty slice if min > max.
// Limits the number of generated values to prevent excessive memory allocation.
func GenerateIntRange(min, max, step int) []int {
	if step <= 0 {
		return nil
	}
	if min > max {
		return nil
	}

	// Calculate expected count and enforce limit to prevent DoS
	const maxValues = 10000
	expectedCount := (max-min)/step + 1
	if expectedCount > maxValues || expectedCount < 0 {
		// Return empty slice rather than panic - callers should validate ranges
		return nil
	}

	var result []int
	for v := min; v <= max; v += step {
		// Additional safety check during loop
		if len(result) >= maxValues {
			break
		}
		result = append(result, v)
	}
	return result
}

// ParseParamList parses a comma-separated list of floats or a range specification.
// If the string contains a colon, it is treated as "min:max:step" range spec.
// Otherwise, it is parsed as comma-separated values.
func ParseParamList(s string) ([]float64, error) {
	if s == "" {
		return nil, nil
	}

	// Check if it's a range specification
	if strings.Contains(s, ":") {
		spec, err := ParseRangeSpec(s)
		if err != nil {
			return nil, err
		}
		return GenerateRange(spec.Min, spec.Max, spec.Step), nil
	}

	// Parse as comma-separated values
	return ParseCSVFloat64s(s)
}

// ParseIntParamList parses a comma-separated list of integers or a range specification.
// If the string contains a colon, it is treated as "min:max:step" range spec.
// Otherwise, it is parsed as comma-separated values.
func ParseIntParamList(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}

	// Check if it's a range specification
	if strings.Contains(s, ":") {
		spec, err := ParseIntRangeSpec(s)
		if err != nil {
			return nil, err
		}
		return GenerateIntRange(spec.Min, spec.Max, spec.Step), nil
	}

	// Parse as comma-separated values
	return ParseCSVInts(s)
}

// ExpandRanges generates the full cartesian product of multiple range specifications.
// Each spec string can be either "min:max:step" or comma-separated values.
// Returns a slice of slices, where each inner slice represents one combination.
func ExpandRanges(specs ...string) ([][]float64, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	// Parse all specs into value slices
	values := make([][]float64, len(specs))
	for i, spec := range specs {
		v, err := ParseParamList(spec)
		if err != nil {
			return nil, fmt.Errorf("parsing spec %d (%q): %w", i, spec, err)
		}
		if len(v) == 0 {
			v = []float64{0} // Default to single zero value
		}
		values[i] = v
	}

	// Calculate total combinations and validate before allocation to prevent DoS
	const maxCombos = 10000
	total := int64(1)
	for _, v := range values {
		if len(v) <= 0 {
			continue
		}
		total *= int64(len(v))
		// Check for overflow or excessive combinations
		if total > maxCombos || total < 0 {
			return nil, fmt.Errorf("parameter combinations would exceed safe limit of %d", maxCombos)
		}
	}

	if total == 0 {
		return nil, nil
	}

	// Generate cartesian product - safe to allocate now
	result := make([][]float64, total)
	for i := range result {
		result[i] = make([]float64, len(specs))
	}

	repeat := int64(1)
	for dim := len(specs) - 1; dim >= 0; dim-- {
		dimValues := values[dim]
		cycle := int64(len(dimValues))
		for i := int64(0); i < total; i++ {
			result[i][dim] = dimValues[(i/repeat)%cycle]
		}
		repeat *= cycle
	}

	return result, nil
}
