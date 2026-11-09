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
// variance is the variance the errors have. Its parts go with it, so the
// same innovations can be read against another AxisSigmaScale
// (HeadingNISAtScale) without changing the tuning key or the filter.
//
// The published box. Alone the option is a shadow: the box's heading half
// and placement half are Observe's code split in two, with the same inputs,
// so the box, and through L3's track guard the tracks, are what they are
// without it. Beside BoxExtentFromSolidBody it is not, because the box then
// draws the solid body's extent, and the solid body is what the option
// changes. Two things reach the box that way. The extents themselves, now
// found along the box's heading; that is the extent state the two options
// together describe. And which frames take them: the box refuses an extent
// measured along an axis more than the span window from its heading, and
// on a shared frame the body's axis is the box's heading, so the test cannot
// refuse. That second effect is gated on its own
// (BoxModelConfig.BodyExtentSharedAxis): without it the test reads the
// orientation the body would hold without the option (own), so the shared
// heading does not also widen box_extent_solid_body's admission.
// shared-orientation-duplicates-2026-11 measures both on kirk0.

// ownOrientation is the orientation the body would hold this frame without
// SharedOrientation: solidBodyOrientation with last frame's own orientation
// as its previous belief, which is what orientation is when the option is
// off. solidBodyOrientation reads the previous belief from sb.orientation,
// so it is swapped in for the call and restored; nothing else is written.
func (t *Tracker) ownOrientation(track *TrackedObject, sb *solidBodyTrack) OrientationBelief {
	held := sb.orientation
	sb.orientation = sb.own
	own := t.solidBodyOrientation(track, sb)
	sb.orientation = held
	return own
}

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
		a := b.axisInnovation
		m.HeadingNIS, m.HeadingNISGated, m.HeadingNISKnown = float32(a.nis), a.gated, true
		m.HeadingInnovationSq, m.HeadingPriorVariance, m.HeadingFitVariance = float32(a.r2), float32(a.prior), float32(a.fit)
	}
}

// HeadingNISAtScale is this frame's heading innovation normalised as if the
// box model's AxisSigmaScale were scale: r² / (P + scale²·fit), on the
// innovation and predicted variance the filter actually had. It is a reading
// only. A filter run at that scale would carry another P and other
// innovations, so this is not that filter's NIS: it reads how far the
// observation variance alone is from the errors, with the filter's
// prediction as it ran. False when the frame has no heading innovation.
func (m SolidBodyMeasurement) HeadingNISAtScale(scale float64) (float64, bool) {
	if !m.HeadingNISKnown {
		return 0, false
	}
	s := float64(m.HeadingPriorVariance) + scale*scale*float64(m.HeadingFitVariance)
	if !(s > 0) {
		return 0, false
	}
	return float64(m.HeadingInnovationSq) / s, true
}
