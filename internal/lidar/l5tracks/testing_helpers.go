package l5tracks

// SetSpeedHistory sets the speed history on a TrackedObject.
// This is exported to allow test code in other packages to set up
// test fixtures without accessing the unexported speedHistory field.
//
// NOTE: This function is intended for testing purposes only and should
// not be used in production code. The speedHistory field is intentionally
// unexported to maintain encapsulation. Production code should use the
// SpeedHistory() method to read the speed history.
func (track *TrackedObject) SetSpeedHistory(speeds []float32) {
	track.speedHistory = speeds
}
