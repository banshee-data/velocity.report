package l5tracks

// SetSpeedHistory populates the speedWindow on a TrackedObject from a slice
// of speed values. This is exported to allow test code in other packages to
// set up test fixtures without accessing the unexported speeds field.
//
// NOTE: This function is intended for testing purposes only and should
// not be used in production code. The speeds field is intentionally
// unexported to maintain encapsulation. Production code should use the
// SpeedHistory() method to read the speed history.
func (track *TrackedObject) SetSpeedHistory(speeds []float32) {
	if speeds == nil {
		track.speeds = nil
		return
	}
	sw := newSpeedWindow(len(speeds))
	for _, s := range speeds {
		sw.Add(s)
	}
	track.speeds = sw
}
