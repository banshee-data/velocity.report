package l5tracks

// The tracked near-edge update, S2.2 of docs/plans/lidar-near-edge-tracked-state-plan.md.
//
// Under TrackerConfig.NearEdgeTracking the solid body stops keeping a filter of
// its own. The near-edge state machine (stepNearEdge) runs on the tracked
// filter instead, so the fix, the re-reference, the faceless coast and the lapse
// that the shadow measured are what the tracked state does, and the difference
// between the shadow arm and the tracked arm is association and identity alone.
// The solid body keeps its beliefs (extents, orientation, face hysteresis) and
// reads the tracked state.
//
// Association compares like with like (invariant 4). A body-centre track is
// gated against a measurement of its centre: the pair's near-edge faces along
// their normals, with the tracked noise, and the medoid across any direction no
// face constrains, with the noise loosened by the believed half-extent that way
// (A2). That is always two degrees of freedom, so the chi-square gate and
// GatingDistanceSquared are unchanged. The loose term is association evidence
// only; the update never uses it (invariant 2). A medoid-referenced track, in
// its initialisation window or after a lapse, is gated on the medoid as today.
// The pair's measurement is kept and reused by the update, so each associated
// cluster is measured once.

import "math"

// nearEdgePairKey names one track and cluster pairing within a frame.
type nearEdgePairKey struct {
	track      *TrackedObject
	clusterIdx int
}

// syncFromTrack makes the solid body's dynamic state a copy of the tracked
// filter's.
func (sb *solidBodyTrack) syncFromTrack(track *TrackedObject) {
	sb.state = [4]float32{track.X, track.Y, track.VX, track.VY}
	sb.p = track.P
}

// writeToTrack makes the tracked filter's state the solid body's.
func (sb *solidBodyTrack) writeToTrack(track *TrackedObject) {
	track.X, track.Y, track.VX, track.VY = sb.state[0], sb.state[1], sb.state[2], sb.state[3]
	track.P = sb.p
}

// nearEdgePriorFor is the class dimension prior the state machine reads for a
// track this frame.
func nearEdgePriorFor(track *TrackedObject) classDimensionPrior {
	effective, _ := solidBodyClass(track).Effective()
	return dimensionPriorFor(effective)
}

// measureTrackedPair is the pair's near-edge measurement for a tracked body:
// made with the solid body's beliefs at the tracked prediction, with the
// orientation of the track's previous frame, since this frame's heading
// decision follows the update. observations counts this frame.
func (t *Tracker) measureTrackedPair(track *TrackedObject, cluster WorldCluster) nearEdgeFrame {
	sb := &track.solidBody
	sb.syncFromTrack(track)
	return t.measureNearEdgeFrame(sb, cluster, nearEdgePriorFor(track), track.ObservationCount+1)
}

// stepTrackedNearEdge takes the state machine's step on the tracked filter.
// It returns true when the step decided the tracked state (a fix, a lapse, or
// a faceless frame the body coasts through) and false when the state is still
// referenced to the medoid, so the tracked filter's usual update applies. The
// step is kept for completeTrackedSolidBody.
func (t *Tracker) stepTrackedNearEdge(track *TrackedObject, cluster WorldCluster) bool {
	sb := &track.solidBody
	frame, ok := t.nearEdgePairs[nearEdgePairKey{track, sb.pairClusterIdx}]
	if !ok || !sb.hasPair {
		frame = t.measureTrackedPair(track, cluster)
	}
	sb.hasPair = false
	sb.syncFromTrack(track)
	m, outcome := t.stepNearEdge(sb, cluster, nearEdgePriorFor(track), frame)
	sb.pending = nearEdgePending{valid: true, frame: frame, m: m, outcome: outcome}
	if outcome == nearEdgeMedoid {
		return false
	}
	sb.writeToTrack(track)
	return true
}

// recordTrackedNearEdgeResidual records what entered the tracked filter on a
// frame the state machine decided, as the medoid update records its own, and
// returns the measurement the frame's bookkeeping names. predicted is the
// track before the step.
//
// A fix's measurement is the reconstructed centre, with the step's NIS over
// its rank; along a direction no face constrained that centre is the
// prediction, so the innovation there is zero. A lapse's is the medoid it
// returned to, with the medoid's NIS against the prediction. A faceless frame
// updated nothing, so it records no residual and keeps the last source: the
// estimate sinks write a row for a frame the filter was updated on.
func (t *Tracker) recordTrackedNearEdgeResidual(track *TrackedObject, predicted trackedPrediction, cluster WorldCluster, medoid PositionMeasurement) PositionMeasurement {
	pending := track.solidBody.pending
	measurement := medoid
	var nis float32
	switch pending.outcome {
	case nearEdgeFix:
		measurement.X, measurement.Y = pending.frame.edges.CentreX, pending.frame.edges.CentreY
		measurement.Source = MeasurementNearEdgeCandidateV1
		nis = pending.m.NIS
	case nearEdgeLapse:
		nis = positionNIS(predicted, medoid, t.Config.MeasurementNoise)
	default:
		measurement.Source = track.LastMeasurementSource
		return measurement
	}
	track.LastResidual = FilterResidual{
		Valid: true, PredictedX: predicted.x, PredictedY: predicted.y, Measurement: measurement,
		InnovationX: measurement.X - predicted.x, InnovationY: measurement.Y - predicted.y, NIS: nis,
		GeometryCovariance: covarianceForCluster(cluster, t.Config.MeasurementNoise),
	}
	return measurement
}

// trackedPrediction is the tracked position and covariance before a step.
type trackedPrediction struct {
	x, y float32
	p    [16]float32
}

// positionNIS is a position measurement's normalised innovation squared
// against a prediction, with isotropic noise r, and zero when the innovation
// covariance is not positive definite.
func positionNIS(predicted trackedPrediction, m PositionMeasurement, r float32) float32 {
	s00, s01 := float64(predicted.p[0*4+0]+r), float64(predicted.p[0*4+1])
	s10, s11 := float64(predicted.p[1*4+0]), float64(predicted.p[1*4+1]+r)
	det := s00*s11 - s01*s10
	if !(det > 0) {
		return 0
	}
	dx, dy := float64(m.X-predicted.x), float64(m.Y-predicted.y)
	return float32((dx*(s11*dx-s01*dy) + dy*(s00*dy-s10*dx)) / det)
}

// completeTrackedSolidBody finishes a tracked body's frame once the tracked
// update and the frame's merge flags are done: the body reads the tracked
// state and this frame's heading, then admits extents, support, lifecycle
// and the reading as the shadow does.
func (t *Tracker) completeTrackedSolidBody(track *TrackedObject, cluster WorldCluster) {
	sb := &track.solidBody
	pending := sb.pending
	sb.pending = nearEdgePending{}
	sb.syncFromTrack(track)
	sb.orientation = orientationFromTrack(track, sb.orientation)
	class := solidBodyClass(track)
	m, measured := pending.m, pending.valid && pending.outcome != nearEdgeCoast
	if !pending.valid {
		m.FallbackReason = "no_tracked_step"
	}
	if pending.outcome == nearEdgeMedoid {
		// The tracked filter's own update: applied unless its innovation
		// covariance was singular, which is when it records no residual.
		measured = track.LastResidual.Valid
		if measured {
			m.Source, m.Rank, m.NIS = track.LastMeasurementSource, 2, track.LastResidual.NIS
		}
	}
	t.completeSolidBodyFrame(track, class, cluster, pending.frame, m, measured)
}

// attachNearEdgePair tells a tracked body which cluster association gave it,
// so the update reuses the measurement the gate made for that pair.
func (t *Tracker) attachNearEdgePair(track *TrackedObject, clusterIdx int) {
	if !track.solidBody.tracked {
		return
	}
	track.solidBody.pairClusterIdx = clusterIdx
	track.solidBody.hasPair = true
}

// gateDistanceSquared is the pairing's squared distance for the gate and the
// cost: A2's face residual for a body-centre tracked body, and the medoid
// Mahalanobis distance otherwise.
func (t *Tracker) gateDistanceSquared(track *TrackedObject, clusters []WorldCluster, ci int, dt float32) float32 {
	sb := &track.solidBody
	if !sb.tracked || sb.reference != ReferenceBodyCentre {
		return t.mahalanobisDistanceSquared(track, clusters[ci], dt)
	}
	cluster := clusters[ci]
	measurement := t.measurementForCluster(cluster, t.LastUpdateNanos)
	if t.implausiblePairing(track, measurement, dt) {
		return SingularDistanceRejection
	}
	key := nearEdgePairKey{track, ci}
	frame, ok := t.nearEdgePairs[key]
	if !ok {
		frame = t.measureTrackedPair(track, cluster)
		if t.nearEdgePairs == nil {
			t.nearEdgePairs = map[nearEdgePairKey]nearEdgeFrame{}
		}
		t.nearEdgePairs[key] = frame
	}
	return t.faceResidualDistanceSquared(track, cluster, frame)
}

// faceResidualDistanceSquared is A2's squared distance between a body-centre
// track's prediction and the pair's measurement of the same point, in two
// orthonormal directions. A direction a usable face constrains carries the
// face's residual along its normal with the tracked noise R. A direction none
// does carries the medoid's residual projected onto it, with R plus the
// square of the believed half-extent that way, because the medoid can sit
// anywhere from the centre to the near surface along it. With no usable face
// both directions are the body axes. The residual covariance adds the
// prediction's position covariance projected onto the same directions.
func (t *Tracker) faceResidualDistanceSquared(track *TrackedObject, cluster WorldCluster, f nearEdgeFrame) float32 {
	sb := &track.solidBody
	length, width := f.length, f.width
	if f.fallback != "" {
		prior := nearEdgePriorFor(track)
		length = dimensionFromBelief(sb.lengthBelief, prior.lengthMetres, prior.sigmaMetres)
		width = dimensionFromBelief(sb.widthBelief, prior.widthMetres, prior.sigmaMetres)
	}
	halfLength, halfWidth := float64(length.Metres)/2, float64(width.Metres)/2
	axisX, axisY := math.Cos(float64(f.axis)), math.Sin(float64(f.axis))
	r := float64(t.Config.MeasurementNoise)
	x, y := float64(track.X), float64(track.Y)
	medoidX, medoidY := float64(cluster.CentroidX), float64(cluster.CentroidY)

	type direction struct{ hx, hy, residual, variance float64 }
	var dirs [2]direction
	n := 0
	var constrainsAxis, constrainsAcross bool
	for _, e := range evidenceBackedEdges(f.edges.Edges) {
		if (e.Face.IsLongitudinal() && constrainsAxis) || (!e.Face.IsLongitudinal() && constrainsAcross) || n == 2 {
			continue
		}
		hx, hy := float64(e.NormalX), float64(e.NormalY)
		dirs[n] = direction{hx, hy, float64(e.ImpliedCentreOffset()) - (hx*x + hy*y), r}
		n++
		if e.Face.IsLongitudinal() {
			constrainsAxis = true
		} else {
			constrainsAcross = true
		}
	}
	loose := func(hx, hy, half float64) direction {
		return direction{hx, hy, hx*(medoidX-x) + hy*(medoidY-y), r + half*half}
	}
	if !constrainsAxis {
		dirs[n] = loose(axisX, axisY, halfLength)
		n++
	}
	if !constrainsAcross && n < 2 {
		dirs[n] = loose(-axisY, axisX, halfWidth)
	}

	// S = H P Hᵀ + diag(variance), with H's rows the two directions over
	// position.
	p := func(i, j int) float64 { return float64(track.P[i*4+j]) }
	project := func(a, b direction) float64 {
		return a.hx*(p(0, 0)*b.hx+p(0, 1)*b.hy) + a.hy*(p(1, 0)*b.hx+p(1, 1)*b.hy)
	}
	s00 := project(dirs[0], dirs[0]) + dirs[0].variance
	s01 := project(dirs[0], dirs[1])
	s10 := project(dirs[1], dirs[0])
	s11 := project(dirs[1], dirs[1]) + dirs[1].variance
	det := s00*s11 - s01*s10
	if !(det >= MinDeterminantThreshold) || math.IsInf(det, 0) {
		return SingularDistanceRejection
	}
	a, b := dirs[0].residual, dirs[1].residual
	return float32((a*(s11*a-s01*b) + b*(s00*b-s10*a)) / det)
}
