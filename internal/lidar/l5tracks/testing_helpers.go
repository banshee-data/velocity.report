package l5tracks

// SetSpeedHistory sets the speed history on a TrackedObject.
// This is exported to allow test code in other packages to set up
// test fixtures without accessing the unexported speedHistory field.
func (track *TrackedObject) SetSpeedHistory(speeds []float32) {
track.speedHistory = speeds
}
