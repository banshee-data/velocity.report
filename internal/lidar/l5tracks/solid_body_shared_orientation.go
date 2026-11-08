package l5tracks

// SolidBodyOptions.SharedOrientation: one orientation belief for the track.
//
// Without it a track carries two heading estimates that never meet. The
// solid body's OrientationBelief is the tracked OBB heading (orientationFrom-
// Track), or the course (courseOrientation), or its own axis filter under
// RectangleHeading (axisOrientation), with a variance from heading jitter
// that nothing calibrates; and since the face-state review HeadingFaceNoise
// adds that variance to every face. The published box's two-state filter
// (box_model.go) is observed by the rectangle-fit axis with the fit's sigma
// and by the course, under the yaw-rate bound |ω| ≤ κ·v + ω₀; it is the
// estimator the box rotation runs scored (geom0005 to geom0008).
//
// Under the option the solid body's orientation is that filter's posterior:
// psi its state and VarianceRad2 its P_psi-psi, after this frame's axis and
// course corrections. The tracker runs the box's heading half before the
// solid body's update (advanceBoxHeading, step 3c) and its placement after
// (updateBox, step 3d), so the solid body reads this frame's posterior and
// the box still reads this frame's solid body (BoxExtentFromSolidBody). On a
// coast the box coasts first and the solid body then takes its prediction,
// whose variance has grown by the filter's process noise.
//
// The direction. The box filter is observed modulo 90° and is a heading only
// once the course, or the track's displacement, has labelled it: which axis
// is the length, and which end the front (BoxEstimator.label). A labelled
// box's heading has been resolved against a direction cue, which is the case
// OrientationBelief represents with AmbiguousModeWeight zero, as
// trackOrientation does for a velocity or displacement heading. An unlabelled
// box has four candidate headings, which the two-mode representation cannot
// state; that frame takes the orientation the solid body would have without
// the option, rather than call a quarter-turn ambiguity a half-turn one.
//
// The calibration. Each frame's axis observation leaves its normalised
// innovation r²/S on the solid body's measurement (HeadingNIS), S being the
// filter's own innovation variance, so a replay can read whether the shared
// variance is the variance the errors have.

// sharedOrientation is the box's heading posterior as the solid body's
// orientation belief, and false when the option is off, the body is the
// tracked filter, or the box has not this frame resolved a heading.
func (t *Tracker) sharedOrientation(track *TrackedObject) (OrientationBelief, bool) {
	if !t.Config.SolidBody.SharedOrientation || track.solidBody.tracked {
		return OrientationBelief{}, false
	}
	b := track.box
	if b == nil || !b.seeded || !b.labelled || b.lastNanos != t.LastUpdateNanos {
		return OrientationBelief{}, false
	}
	return OrientationBelief{
		PsiRad:       float32(b.psi),
		VarianceRad2: float32(b.p[0][0]),
		Provenance:   ProvenanceObserved,
	}, true
}

// headingInnovation copies this frame's axis innovation from the box onto
// a solid body measurement, under SharedOrientation.
func headingInnovation(track *TrackedObject, m *SolidBodyMeasurement) {
	if b := track.box; b != nil && b.axisInnovation.known {
		m.HeadingNIS, m.HeadingNISGated, m.HeadingNISKnown = float32(b.axisInnovation.nis), b.axisInnovation.gated, true
	}
}
