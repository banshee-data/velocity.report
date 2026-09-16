package l5tracks

import (
	"math"
	"sort"
)

// Internal numerical stability constants — not user-tunable.
const (
	// MinDeterminantThreshold is the minimum determinant for covariance matrix inversion
	MinDeterminantThreshold = 1e-6
	// SingularDistanceRejection is the distance returned when covariance is singular
	SingularDistanceRejection = 1e9
)

// isFiniteState returns true if every element of the Kalman state vector
// (X, Y, VX, VY) and the covariance matrix diagonal is finite (not NaN
// or ±Inf). Used as a post-predict/update guard against numerical
// instability from singular covariance inversions or degenerate inputs.
func isFiniteState(track *TrackedObject) bool {
	if math.IsNaN(float64(track.X)) || math.IsInf(float64(track.X), 0) {
		return false
	}
	if math.IsNaN(float64(track.Y)) || math.IsInf(float64(track.Y), 0) {
		return false
	}
	if math.IsNaN(float64(track.VX)) || math.IsInf(float64(track.VX), 0) {
		return false
	}
	if math.IsNaN(float64(track.VY)) || math.IsInf(float64(track.VY), 0) {
		return false
	}
	for i := 0; i < 4; i++ {
		v := float64(track.P[i*4+i])
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// clampVelocity scales VX/VY proportionally so the speed magnitude does not
// exceed MaxReasonableSpeedMps. This prevents teleport-like extrapolation
// from noisy Kalman updates or degenerate associations.
func (t *Tracker) clampVelocity(track *TrackedObject) {
	speed := float32(math.Sqrt(float64(track.VX*track.VX + track.VY*track.VY)))
	if speed > t.Config.MaxReasonableSpeedMps {
		scale := t.Config.MaxReasonableSpeedMps / speed
		track.VX *= scale
		track.VY *= scale
	}
}

// predict applies the Kalman prediction step using constant velocity model.
func (t *Tracker) predict(track *TrackedObject, dt float32) {
	// Clamp dt to prevent covariance explosion on frame gaps.
	// Large dt values (e.g. from throttled frames or PCAP catch-up) cause
	// F*P*F^T to grow quadratically, ballooning the gating ellipse.
	if dt > t.Config.MaxPredictDt {
		dt = t.Config.MaxPredictDt
	}

	// State transition matrix F for constant velocity model:
	// F = [1  0  dt  0 ]
	//     [0  1  0   dt]
	//     [0  0  1   0 ]
	//     [0  0  0   1 ]

	// Predict state: x' = F * x
	track.X += track.VX * dt
	track.Y += track.VY * dt
	// VX and VY remain unchanged in constant velocity model

	// Record prediction for debug visualisation
	if t.DebugCollector != nil && t.DebugCollector.IsEnabled() {
		t.DebugCollector.RecordPrediction(track.TrackID, track.X, track.Y, track.VX, track.VY)
	}

	// Predict covariance: P' = F * P * F^T + Q
	// For efficiency, we compute this directly

	// Extract current P (4x4 row-major)
	P := track.P

	// Compute F * P (state transition applied to covariance)
	// Row 0: P[0,j] + dt*P[2,j]
	// Row 1: P[1,j] + dt*P[3,j]
	// Row 2: P[2,j]
	// Row 3: P[3,j]
	var FP [16]float32
	for j := 0; j < 4; j++ {
		FP[0*4+j] = P[0*4+j] + dt*P[2*4+j]
		FP[1*4+j] = P[1*4+j] + dt*P[3*4+j]
		FP[2*4+j] = P[2*4+j]
		FP[3*4+j] = P[3*4+j]
	}

	// Compute F * P * F^T
	// Column i: FP[j,0] + dt*FP[j,2] for col 0, FP[j,1] + dt*FP[j,3] for col 1, etc.
	for i := 0; i < 4; i++ {
		track.P[i*4+0] = FP[i*4+0] + dt*FP[i*4+2]
		track.P[i*4+1] = FP[i*4+1] + dt*FP[i*4+3]
		track.P[i*4+2] = FP[i*4+2]
		track.P[i*4+3] = FP[i*4+3]
	}

	// Add process noise Q, scaled by dt for correct uncertainty growth
	// regardless of frame rate. Values in Config are dt-normalised.
	track.P[0*4+0] += t.Config.ProcessNoisePos * dt
	track.P[1*4+1] += t.Config.ProcessNoisePos * dt
	track.P[2*4+2] += t.Config.ProcessNoiseVel * dt
	track.P[3*4+3] += t.Config.ProcessNoiseVel * dt

	// Cap covariance diagonal elements to prevent unbounded gating ellipse
	// growth from accumulated prediction steps and occlusion inflation.
	for i := 0; i < 4; i++ {
		if track.P[i*4+i] > t.Config.MaxCovarianceDiag {
			track.P[i*4+i] = t.Config.MaxCovarianceDiag
		}
	}

	// Guard: reset state if prediction produced NaN/Inf (task 2.4).
	if !isFiniteState(track) {
		opsf("Predict produced non-finite state: track_id=%s deleting track", track.TrackID)
		track.X = 0
		track.Y = 0
		track.VX = 0
		track.VY = 0
		track.P = [16]float32{
			10, 0, 0, 0,
			0, 10, 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		}
		track.TrackState = TrackDeleted
		return
	}

	// Clamp velocity magnitude after prediction (task 2.3).
	t.clampVelocity(track)
}

// associate performs cluster-to-track association using the Hungarian
// (Kuhn–Munkres) algorithm for globally optimal assignment. This replaces the
// earlier greedy nearest-neighbour approach which could cause track splitting
// when two clusters competed for the same track.
//
// The cost matrix is built from squared Mahalanobis distances; entries
// exceeding the gating threshold are set to +Inf (forbidden).
// Returns a slice indexed by cluster index: each element is the trackID
// the cluster was associated with, or "" if unassociated.
func (t *Tracker) associate(clusters []WorldCluster, dt float32) []string {
	associations := make([]string, len(clusters))

	// Build ordered list of active tracks.
	//
	// The sort is not cosmetic. Go randomises map iteration order, so without
	// it the cost matrix columns are permuted on every frame, and Hungarian
	// assignment resolves ties differently from one frame to the next.
	//
	// Creation order makes tied costs independent of random public UUIDs.
	// Cross-run reproducibility still requires deterministic input/cluster order.
	activeTrackIDs := make([]string, 0, len(t.Tracks))
	for id, track := range t.Tracks {
		if track.TrackState != TrackDeleted {
			activeTrackIDs = append(activeTrackIDs, id)
		}
	}
	sort.Slice(activeTrackIDs, func(i, j int) bool {
		a, b := t.Tracks[activeTrackIDs[i]], t.Tracks[activeTrackIDs[j]]
		if a.CreationSequence != b.CreationSequence {
			return a.CreationSequence < b.CreationSequence
		}
		// Hand-built/legacy tracks may not have a sequence. Runtime-created tracks do.
		return activeTrackIDs[i] < activeTrackIDs[j]
	})

	nClusters := len(clusters)
	nTracks := len(activeTrackIDs)

	if nClusters == 0 || nTracks == 0 {
		// Record all candidates as unassociated for debug.
		if t.DebugCollector != nil && t.DebugCollector.IsEnabled() {
			for ci := range clusters {
				for _, trackID := range activeTrackIDs {
					track := t.Tracks[trackID]
					dist2 := t.mahalanobisDistanceSquared(track, clusters[ci], dt)
					t.DebugCollector.RecordAssociation(clusters[ci].ClusterID, trackID, dist2, false)
				}
			}
		}
		return associations
	}

	// Build cost matrix [nClusters × nTracks].
	costMatrix := make([][]float32, nClusters)
	for ci := range clusters {
		costMatrix[ci] = make([]float32, nTracks)
		for tj, trackID := range activeTrackIDs {
			track := t.Tracks[trackID]

			// Fragment guard: the gate is otherwise a 6 m radius on position
			// alone, with nothing to stop a scrap of a cluster capturing a
			// vehicle-sized track. On run baf20f02 a 0.11 m by 0.08 m cluster
			// was associated to a track carrying a 4.33 m car and became its
			// dimensions for the next 28 frames. Forbid the pairing here
			// rather than refusing it at update time, so the fragment stays
			// unassociated and can seed its own track.
			// With the soft cost disabled the fragment guard forbids the
			// pairing outright, which is the behaviour D1.5 shipped.
			if t.Config.AssociationExtentCostWeight <= 0 && t.isFragmentFor(track, clusters[ci]) {
				costMatrix[ci][tj] = float32(hungarianlnf)
				track.FragmentPairingsRejected++
				continue
			}

			dist2 := t.mahalanobisDistanceSquared(track, clusters[ci], dt)
			if dist2 >= SingularDistanceRejection || dist2 >= float32(hungarianlnf) || dist2 > t.Config.GatingDistanceSquared {
				costMatrix[ci][tj] = float32(hungarianlnf)
			} else {
				costMatrix[ci][tj] = dist2 + t.extentCompatibilityCost(track, clusters[ci])
			}
		}
	}

	// Solve optimal assignment.
	assign := HungarianAssign(costMatrix)

	// Populate associations and record debug info.
	for ci := range clusters {
		bestTrackIdx := -1
		if ci < len(assign) && assign[ci] >= 0 {
			bestTrackIdx = assign[ci]
		}

		if t.DebugCollector != nil && t.DebugCollector.IsEnabled() {
			for tj, trackID := range activeTrackIDs {
				accepted := (tj == bestTrackIdx)
				t.DebugCollector.RecordAssociation(clusters[ci].ClusterID, trackID, costMatrix[ci][tj], accepted)
			}
		}

		if bestTrackIdx >= 0 && bestTrackIdx < nTracks {
			associations[ci] = activeTrackIDs[bestTrackIdx]
		}
	}

	return associations
}

// mahalanobisDistanceSquared computes the squared Mahalanobis distance for gating.
// Uses only position (x, y) for distance computation.
// Also performs physical plausibility checks to reject spurious associations.
func (t *Tracker) mahalanobisDistanceSquared(track *TrackedObject, cluster WorldCluster, dt float32) float32 {
	// Innovation: difference between measurement and prediction
	measurement := t.measurementForCluster(cluster, t.LastUpdateNanos)
	dx := measurement.X - track.X
	dy := measurement.Y - track.Y

	// Physical plausibility check: reject if position jump is too large
	euclideanDist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if euclideanDist > t.Config.MaxPositionJumpMetres {
		return SingularDistanceRejection
	}

	// Check if implied velocity would be unreasonable
	if dt > 0 {
		impliedSpeed := euclideanDist / dt
		if impliedSpeed > t.Config.MaxReasonableSpeedMps {
			return SingularDistanceRejection
		}
	}

	// Innovation covariance S = H * P * H^T + R
	// H = [1 0 0 0; 0 1 0 0] (measurement extracts position only)
	// S = P[0:2, 0:2] + R
	S00 := track.P[0*4+0] + t.Config.MeasurementNoise
	S01 := track.P[0*4+1]
	S10 := track.P[1*4+0]
	S11 := track.P[1*4+1] + t.Config.MeasurementNoise

	// Compute determinant and inverse
	det := S00*S11 - S01*S10
	if det < MinDeterminantThreshold {
		return SingularDistanceRejection // Singular covariance, reject association
	}

	invS00 := S11 / det
	invS01 := -S01 / det
	invS10 := -S10 / det
	invS11 := S00 / det

	// Record gating ellipse for debug visualisation
	if t.DebugCollector != nil && t.DebugCollector.IsEnabled() {
		// Compute ellipse parameters from innovation covariance S
		// Eigenvalues of 2x2 symmetric matrix S:
		// λ = (S00 + S11 ± sqrt((S00-S11)² + 4*S01*S10)) / 2
		trace := S00 + S11
		discriminant := (S00-S11)*(S00-S11) + 4*S01*S10
		if discriminant < 0 {
			discriminant = 0
		}
		sqrtDisc := float32(math.Sqrt(float64(discriminant)))
		lambda1 := (trace + sqrtDisc) / 2.0
		lambda2 := (trace - sqrtDisc) / 2.0

		// Semi-axes are sqrt(eigenvalues) scaled by gating threshold
		// For chi-squared distribution with 2 DOF, gating threshold determines confidence
		gatingThreshold := float32(math.Sqrt(float64(t.Config.GatingDistanceSquared)))
		semiMajor := gatingThreshold * float32(math.Sqrt(float64(lambda1)))
		semiMinor := gatingThreshold * float32(math.Sqrt(float64(lambda2)))

		// Rotation angle from eigenvector of largest eigenvalue
		// For 2x2 matrix, eigenvector v1 of λ1: [S01, λ1-S00]
		// Rotation = atan2(v1_y, v1_x)
		var rotation float32
		if math.Abs(float64(S01)) > 1e-6 {
			rotation = float32(math.Atan2(float64(lambda1-S00), float64(S01)))
		} else {
			rotation = 0
		}

		t.DebugCollector.RecordGatingRegion(track.TrackID, track.X, track.Y, semiMajor, semiMinor, rotation)
	}

	// Mahalanobis distance squared: d² = [dx dy] * S^-1 * [dx dy]^T
	dist2 := dx*dx*invS00 + dx*dy*(invS01+invS10) + dy*dy*invS11

	return dist2
}

// FragmentGuardMinTrackExtentMetres is the size belief above which a track is
// treated as representing a metre-scale object, and so protected from being
// captured by a scrap of a cluster.
//
// The guard is deliberately confined to that population. A pedestrian or
// cyclist track legitimately carries an extent of well under a metre, and its
// clusters vary by a similar amount frame to frame, so applying a small-cluster
// rule there would reject ordinary observations. Vehicles do not shrink to a
// tenth of their length between frames.
// Association extent-compatibility scales. Bounded experiment values, not
// measured noise.
const (
	// associationExtentScale is the log-size disagreement counted as one unit.
	associationExtentScale = 0.5
	// associationShortfallDiscount is how much more cheaply a cluster smaller
	// than the belief is treated than a larger one. Occlusion produces the
	// first on every pass; only a merge or a wrong belief produces the second.
	associationShortfallDiscount = 0.4
	// associationExtentCostCap limits the term to a fraction of the gating
	// threshold, so position stays the dominant term in the assignment.
	associationExtentCostCap = 0.25
)

const FragmentGuardMinTrackExtentMetres = 2.0

// isFragmentFor reports whether a cluster is too small to be a plausible
// observation of the given track.
//
// This is a narrow absurdity check, not a shape gate. It only fires when a
// track already believes it is looking at something metre-scale and the cluster
// on offer is smaller than MinAssociableExtentMetres. Proper dimension
// consistency in the assignment cost is a separate, larger change.
//
// The track's belief is its running average extent rather than the latest
// frame's, because the latest frame is exactly the value a fragment would have
// corrupted.
func (t *Tracker) isFragmentFor(track *TrackedObject, cluster WorldCluster) bool {
	minExtent := t.Config.MinAssociableExtentMetres
	if minExtent <= 0 {
		return false
	}

	// Only guard tracks that have seen enough to have a belief worth trusting,
	// and that believe they are metre-scale.
	if track.ObservationCount < 3 {
		return false
	}
	belief := track.BoundingBoxLengthAvg
	if track.BoundingBoxWidthAvg > belief {
		belief = track.BoundingBoxWidthAvg
	}
	if belief < FragmentGuardMinTrackExtentMetres {
		return false
	}

	extent := cluster.BoundingBoxLength
	if cluster.BoundingBoxWidth > extent {
		extent = cluster.BoundingBoxWidth
	}
	// A cluster reporting no extent at all carries no evidence either way;
	// leave it to the distance gate rather than inventing a rejection.
	if extent <= 0 {
		return false
	}

	return extent < minExtent
}

// extentCompatibilityCost is the D2.2 shape term: a bounded penalty for
// pairing a track with a cluster whose size is hard to explain.
//
// It is deliberately not a rejection. A 0.11 m scrap may genuinely belong to
// the car it broke off, and refusing the pairing does not make the scrap go
// away — it leaves it free to seed a second track on the same vehicle, which
// is the split-car symptom this sprint started from. The hard fragment guard
// traded duplicate identities for protected dimensions. That trade is no
// longer necessary: extent beliefs are revised only by confidently assigned,
// well-supported observations, so dimensions are defended where they are
// formed rather than at the association gate.
//
// The term is asymmetric for the same reason the axis cost is. A cluster
// SMALLER than the track's belief is what occlusion produces on every pass, so
// it is charged lightly — enough that a complete cluster outbids a scrap for
// the same track, not enough to refuse the scrap when it is the only candidate.
// A cluster LARGER than the belief needs the belief to be wrong or the cluster
// to be a merge, and is charged accordingly.
//
// The result is capped below the gating threshold so position remains the
// dominant term and this can never, on its own, push a pairing out of the gate.
func (t *Tracker) extentCompatibilityCost(track *TrackedObject, cluster WorldCluster) float32 {
	weight := t.Config.AssociationExtentCostWeight
	if weight <= 0 {
		return 0
	}
	belief := track.believedLongExtent()
	extent := cluster.BoundingBoxLength
	if cluster.BoundingBoxWidth > extent {
		extent = cluster.BoundingBoxWidth
	}
	// No belief yet, or no extent reported: nothing to compare, and inventing
	// a penalty here would just be a tax on new tracks.
	if belief <= 0 || extent <= 0 {
		return 0
	}

	ratio := math.Log(float64(extent / belief))
	scaled := ratio / associationExtentScale
	if ratio < 0 {
		scaled *= associationShortfallDiscount
	}
	cost := float64(weight) * scaled * scaled

	limit := float64(t.Config.GatingDistanceSquared) * associationExtentCostCap
	if limit > 0 && cost > limit {
		cost = limit
	}
	return float32(cost)
}

// believedLongExtent is the track's best current view of its own longest
// dimension: the extent belief where the axis path has built one, and the
// running average otherwise. The running average is the value the fragment
// guard has always used, and it is the latest frame that a fragment would have
// corrupted, not the average.
func (t *TrackedObject) believedLongExtent() float32 {
	if e := t.lengthBelief.Estimate(); e > 0 {
		if w := t.widthBelief.Estimate(); w > e {
			return w
		}
		return e
	}
	belief := t.BoundingBoxLengthAvg
	if t.BoundingBoxWidthAvg > belief {
		belief = t.BoundingBoxWidthAvg
	}
	return belief
}
