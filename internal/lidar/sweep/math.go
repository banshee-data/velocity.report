// Package sweep provides utilities for parameter sweep operations on LiDAR
// background models. It includes parsing, range generation, statistics
// calculation, and output formatting functions.
package sweep

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseCSVFloat64s parses a comma-separated list of float64 values.
// Returns nil, nil for empty input strings.
func ParseCSVFloat64s(s string) ([]float64, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float '%s': %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// ParseCSVInts parses a comma-separated list of int values.
// Returns nil, nil for empty input strings.
func ParseCSVInts(s string) ([]int, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid int '%s': %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// ToFloat64Slice converts a JSON-decoded value (typically []interface{})
// into a fixed-length []float64 slice. Unknown values become 0.
func ToFloat64Slice(v interface{}, length int) []float64 {
	out := make([]float64, length)
	if v == nil {
		return out
	}
	switch vv := v.(type) {
	case []interface{}:
		for i := 0; i < len(out) && i < len(vv); i++ {
			switch val := vv[i].(type) {
			case float64:
				out[i] = val
			case int:
				out[i] = float64(val)
			case int64:
				out[i] = float64(val)
			default:
				out[i] = 0
			}
		}
	case []float64:
		for i := 0; i < len(out) && i < len(vv); i++ {
			out[i] = vv[i]
		}
	}
	return out
}

// ToInt64Slice converts a JSON-decoded value (typically []interface{})
// into a fixed-length []int64 slice. Unknown values become 0.
func ToInt64Slice(v interface{}, length int) []int64 {
	out := make([]int64, length)
	if v == nil {
		return out
	}
	switch vv := v.(type) {
	case []interface{}:
		for i := 0; i < len(out) && i < len(vv); i++ {
			switch val := vv[i].(type) {
			case float64:
				out[i] = int64(val)
			case int:
				out[i] = int64(val)
			case int64:
				out[i] = val
			default:
				out[i] = 0
			}
		}
	case []int64:
		for i := 0; i < len(out) && i < len(vv); i++ {
			out[i] = vv[i]
		}
	}
	return out
}

// MeanStddev calculates the mean and sample standard deviation of a slice.
// Returns (0, 0) for empty slices.
func MeanStddev(xs []float64) (mean float64, stddev float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range xs {
		sum += v
	}
	mean = sum / float64(len(xs))

	var sdSum float64
	for _, v := range xs {
		d := v - mean
		sdSum += d * d
	}
	if len(xs) > 1 {
		stddev = math.Sqrt(sdSum / float64(len(xs)-1))
	} else {
		stddev = 0
	}
	return mean, stddev
}

// toFloat64FromMap converts a single JSON-decoded value to float64.
func toFloat64FromMap(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

// toIntFromMap converts a single JSON-decoded value to int.
func toIntFromMap(v interface{}) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case float32:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	default:
		return 0
	}
}
