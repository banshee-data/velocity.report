package sweep

import (
	"io"
	"log"
	"math"
)

const (
	// singleValueMarginRatio is the fraction of a single value to use as margin when narrowing
	// bounds around a single result (e.g., if value is 0.05, margin = 0.05 * 0.1 = 0.005).
	singleValueMarginRatio = 0.1

	// minMargin is the minimum absolute margin to add around a single value when narrowing bounds.
	minMargin = 0.001

	// defaultMarginSteps is the number of grid steps to add as margin on each side when narrowing bounds.
	defaultMarginSteps = 1.0

	// maxValuesPerParam limits the number of grid points generated for a single parameter
	// to avoid excessive memory allocation from untrusted input.
	maxValuesPerParam = 10000
)

// discardAutoLogger returns a logger that discards all output.
func discardAutoLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

// narrowBounds computes narrowed parameter bounds from the top K results.
// For each parameter, finds min/max across top K, adds a margin of 1 step.
func narrowBounds(topK []ScoredResult, paramName string, valuesPerParam int) (start, end float64) {
	if len(topK) == 0 {
		return 0, 0
	}

	minVal := math.Inf(1)
	maxVal := math.Inf(-1)

	for _, r := range topK {
		val, ok := r.ParamValues[paramName]
		if !ok {
			continue
		}

		// Convert to float64 for comparison
		var fval float64
		switch v := val.(type) {
		case float64:
			fval = v
		case int:
			fval = float64(v)
		case int64:
			fval = float64(v)
		default:
			continue
		}

		if fval < minVal {
			minVal = fval
		}
		if fval > maxVal {
			maxVal = fval
		}
	}

	// If no numeric values were found for this parameter, do not narrow bounds.
	if math.IsInf(minVal, 1) && math.IsInf(maxVal, -1) {
		return 0, 0
	}

	// If we only have one result, or all results have the same value
	if minVal == maxVal {
		// Add a small margin around the single value
		margin := math.Abs(minVal) * singleValueMarginRatio
		if margin < minMargin {
			margin = minMargin
		}
		return minVal - margin, maxVal + margin
	}

	// Calculate step size based on the range and number of values
	rangeSize := maxVal - minVal
	step := rangeSize / float64(valuesPerParam-1)

	// Add margin on each side (1 step by default)
	return minVal - step*defaultMarginSteps, maxVal + step*defaultMarginSteps
}

// generateGrid creates N evenly-spaced values between start and end (inclusive).
func generateGrid(start, end float64, n int) []float64 {
	if n <= 0 {
		return []float64{}
	}
	if n == 1 {
		// Return midpoint
		return []float64{(start + end) / 2.0}
	}

	// Enforce an upper bound to prevent excessive memory allocation from untrusted input.
	// Clamp to safe maximum before allocation to prevent DoS attacks.
	if n > maxValuesPerParam {
		n = maxValuesPerParam
	}
	// Additional safety check: ensure n is within safe bounds after clamping
	if n < 0 || n > maxValuesPerParam {
		return []float64{}
	}

	grid := make([]float64, n)
	step := (end - start) / float64(n-1)
	for i := 0; i < n; i++ {
		grid[i] = start + step*float64(i)
	}
	return grid
}

// boundsFromParams builds a bounds map from request parameter definitions.
func boundsFromParams(params []SweepParam) map[string][2]float64 {
	bounds := make(map[string][2]float64, len(params))
	for _, p := range params {
		bounds[p.Name] = [2]float64{p.Start, p.End}
	}
	return bounds
}

// checkpointRoundForSuspend computes the round to resume from when a running
// auto-tune is suspended. If the current round has already fully completed,
// resume from the next round; otherwise resume the current round.
func checkpointRoundForSuspend(state AutoTuneState) int {
	round := state.Round
	if round <= 0 {
		round = 1
	}
	if state.TotalCombos > 0 &&
		state.CompletedCombos >= state.TotalCombos &&
		round < state.TotalRounds {
		return round + 1
	}
	return round
}

// copyBounds creates a deep copy of a bounds map.
func copyBounds(bounds map[string][2]float64) map[string][2]float64 {
	result := make(map[string][2]float64)
	for k, v := range bounds {
		result[k] = v
	}
	return result
}

// copyParamValues creates a deep copy of a parameter values map.
func copyParamValues(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// applyAutoTuneDefaults applies default values for unset fields on an AutoTuneRequest.
// This is exported as a helper so tests can exercise the defaulting logic directly.
func applyAutoTuneDefaults(req AutoTuneRequest) AutoTuneRequest {
	if req.MaxRounds <= 0 {
		req.MaxRounds = 3
	}
	if req.ValuesPerParam <= 0 {
		req.ValuesPerParam = 5
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.Objective == "" {
		req.Objective = "acceptance"
	}
	return req
}

// generateIntGrid creates N evenly-spaced integer values between start and end (inclusive).
// Values are rounded to the nearest integer and deduplicated while preserving order.
// Both endpoints are always included (if they map to distinct integers).
func generateIntGrid(start, end float64, n int) []int {
	if n <= 0 {
		return []int{}
	}

	intStart := int(math.Round(start))
	intEnd := int(math.Round(end))

	if n == 1 {
		return []int{(intStart + intEnd) / 2}
	}

	// Enforce an upper bound to prevent excessive memory allocation.
	if n > maxValuesPerParam {
		n = maxValuesPerParam
	}
	// Additional safety check: ensure n is within safe bounds after clamping
	if n < 0 || n > maxValuesPerParam {
		return []int{}
	}

	// Generate float grid, round, and deduplicate
	floatGrid := generateGrid(start, end, n)
	seen := make(map[int]bool, len(floatGrid))
	result := make([]int, 0, len(floatGrid))
	for _, v := range floatGrid {
		iv := int(math.Round(v))
		if !seen[iv] {
			seen[iv] = true
			result = append(result, iv)
		}
	}

	// Ensure endpoints are included
	if !seen[intStart] {
		result = append([]int{intStart}, result...)
	}
	if !seen[intEnd] {
		result = append(result, intEnd)
	}

	return result
}
