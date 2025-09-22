// Package units provides shared constants and validation for speed units
package units

// Unit constants
const (
	MPS  = "mps"
	MPH  = "mph"
	KMPH = "kmph"
	KPH  = "kph"
)

// ValidUnits contains all valid unit values
var ValidUnits = []string{MPS, MPH, KMPH, KPH}

// IsValid checks if the given unit is in the list of valid units
func IsValid(unit string) bool {
	for _, validUnit := range ValidUnits {
		if unit == validUnit {
			return true
		}
	}
	return false
}

// GetValidUnitsString returns a comma-separated string of valid units for error messages
func GetValidUnitsString() string {
	return "mps, mph, kmph, kph"
}

// ConvertSpeed converts a speed from meters per second to the target units
// Database stores speeds in m/s (meters per second)
func ConvertSpeed(speedMPS float64, targetUnits string) float64 {
	switch targetUnits {
	case MPS:
		return speedMPS
	case MPH:
		return speedMPS * 2.2369362920544
	case KMPH, KPH:
		return speedMPS * 3.6
	default:
		return speedMPS
	}
}
