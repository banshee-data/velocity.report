package sweep

import (
	"fmt"
	"strconv"
	"strings"
)

// computeCombinations computes parameter combinations from legacy request format.
func (r *Runner) computeCombinations(req SweepRequest) ([]float64, []float64, []int) {
	var noiseCombos, closenessCombos []float64
	var neighbourCombos []int

	switch req.Mode {
	case "multi":
		noiseCombos = req.NoiseValues
		closenessCombos = req.ClosenessValues
		neighbourCombos = req.NeighbourValues

		if len(noiseCombos) == 0 {
			if req.NoiseStep > 0 {
				noiseCombos = GenerateRange(req.NoiseStart, req.NoiseEnd, req.NoiseStep)
			}
		}
		if len(closenessCombos) == 0 {
			if req.ClosenessStep > 0 {
				closenessCombos = GenerateRange(req.ClosenessStart, req.ClosenessEnd, req.ClosenessStep)
			}
		}
		if len(neighbourCombos) == 0 {
			if req.NeighbourStep > 0 {
				neighbourCombos = GenerateIntRange(req.NeighbourStart, req.NeighbourEnd, req.NeighbourStep)
			}
		}
	case "noise":
		noiseCombos = GenerateRange(req.NoiseStart, req.NoiseEnd, req.NoiseStep)
		closenessCombos = []float64{req.FixedCloseness}
		neighbourCombos = []int{req.FixedNeighbour}
	case "closeness":
		noiseCombos = []float64{req.FixedNoise}
		closenessCombos = GenerateRange(req.ClosenessStart, req.ClosenessEnd, req.ClosenessStep)
		neighbourCombos = []int{req.FixedNeighbour}
	case "neighbour":
		noiseCombos = []float64{req.FixedNoise}
		closenessCombos = []float64{req.FixedCloseness}
		neighbourCombos = GenerateIntRange(req.NeighbourStart, req.NeighbourEnd, req.NeighbourStep)
	}

	// Defaults if still empty
	if len(noiseCombos) == 0 {
		noiseCombos = []float64{0.005, 0.01, 0.02}
	}
	if len(closenessCombos) == 0 {
		closenessCombos = []float64{1.5, 2.0, 2.5}
	}
	if len(neighbourCombos) == 0 {
		neighbourCombos = []int{0, 1, 2}
	}

	return noiseCombos, closenessCombos, neighbourCombos
}

// expandSweepParam expands sweep param range into values.
func expandSweepParam(sp *SweepParam) error {
	if len(sp.Values) > 0 {
		// Already have explicit values — type-coerce them
		for i, v := range sp.Values {
			coerced, err := coerceValue(v, sp.Type)
			if err != nil {
				return fmt.Errorf("value[%d]: %w", i, err)
			}
			sp.Values[i] = coerced
		}
		return nil
	}

	// Generate values from Start/End/Step
	switch sp.Type {
	case "float64":
		if sp.Step <= 0 {
			return fmt.Errorf("step must be positive for float64 range")
		}
		for _, v := range GenerateRange(sp.Start, sp.End, sp.Step) {
			sp.Values = append(sp.Values, v)
		}
	case "int":
		if sp.Step <= 0 {
			return fmt.Errorf("step must be positive for int range")
		}
		for _, v := range GenerateIntRange(int(sp.Start), int(sp.End), int(sp.Step)) {
			sp.Values = append(sp.Values, v)
		}
	case "int64":
		if sp.Step <= 0 {
			return fmt.Errorf("step must be positive for int64 range")
		}
		for v := int64(sp.Start); v <= int64(sp.End); v += int64(sp.Step) {
			sp.Values = append(sp.Values, v)
		}
	case "bool":
		sp.Values = []interface{}{true, false}
	case "string":
		// No range generation for strings; values must be explicit
		return fmt.Errorf("string params require explicit values")
	default:
		return fmt.Errorf("unknown type %q", sp.Type)
	}
	return nil
}

// coerceValue converts a value to the appropriate Go type for the given param type.
// Returns an error if the conversion fails instead of silently defaulting to zero.
func coerceValue(v interface{}, typ string) (interface{}, error) {
	switch typ {
	case "float64":
		switch val := v.(type) {
		case float64:
			return val, nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
			if err != nil {
				return nil, fmt.Errorf("cannot parse %q as float64: %w", val, err)
			}
			return f, nil
		case bool:
			if val {
				return 1.0, nil
			}
			return 0.0, nil
		}
	case "int":
		switch val := v.(type) {
		case int:
			return val, nil
		case float64:
			return int(val), nil
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(val))
			if err != nil {
				return nil, fmt.Errorf("cannot parse %q as int: %w", val, err)
			}
			return n, nil
		}
	case "int64":
		switch val := v.(type) {
		case int64:
			return val, nil
		case float64:
			return int64(val), nil
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot parse %q as int64: %w", val, err)
			}
			return n, nil
		}
	case "bool":
		switch val := v.(type) {
		case bool:
			return val, nil
		case string:
			return strings.TrimSpace(strings.ToLower(val)) == "true", nil
		case float64:
			return val != 0, nil
		}
	case "string":
		switch val := v.(type) {
		case string:
			return strings.TrimSpace(val), nil
		default:
			return fmt.Sprintf("%v", val), nil
		}
	}
	return nil, fmt.Errorf("unsupported coercion: %T to %s", v, typ)
}

// cartesianProduct computes the Cartesian product of all SweepParam value lists.
// Returns a slice of maps, where each map represents one parameter combination.
// Returns an error if the total number of combinations exceeds safe limits to prevent DoS attacks.
func cartesianProduct(params []SweepParam) ([]map[string]interface{}, error) {
	if len(params) == 0 {
		return nil, nil
	}

	// Validate total combinations before allocating memory to prevent DoS attacks.
	// Using int64 to detect overflow during multiplication.
	const maxCombos = 10000 // Hard limit to prevent excessive memory allocation
	total := int64(1)
	for _, p := range params {
		if len(p.Values) <= 0 {
			continue
		}
		total *= int64(len(p.Values))
		// Check for overflow or excessive combinations during computation
		if total > maxCombos || total < 0 {
			return nil, fmt.Errorf("parameter combinations would exceed safe limit of %d (detected: %d parameters with potential for >%d combinations)", maxCombos, len(params), maxCombos)
		}
	}

	if total == 0 {
		return nil, nil
	}

	combos := make([]map[string]interface{}, total)
	for i := range combos {
		combos[i] = make(map[string]interface{}, len(params))
	}

	repeat := int64(1)
	for dim := len(params) - 1; dim >= 0; dim-- {
		vals := params[dim].Values
		name := params[dim].Name
		cycle := int64(len(vals))
		for i := int64(0); i < total; i++ {
			combos[i][name] = vals[(i/repeat)%cycle]
		}
		repeat *= cycle
	}

	return combos, nil
}

// toFloat64 converts an interface{} to float64.
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	}
	return 0, false
}

// toInt converts an interface{} to int.
func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case float64:
		return int(val), true
	case int64:
		return int(val), true
	}
	return 0, false
}
