package l5tracks

import (
	"math"
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
	activeTrackIDs := make([]string, 0, len(t.Tracks))
	for id, track := range t.Tracks {
		if track.TrackState != TrackDeleted {
			activeTrackIDs = append(activeTrackIDs, id)
		}
	}

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
			dist2 := t.mahalanobisDistanceSquared(track, clusters[ci], dt)
			if dist2 >= SingularDistanceRejection || dist2 >= float32(hungarianlnf) || dist2 > t.Config.GatingDistanceSquared {
				costMatrix[ci][tj] = float32(hungarianlnf)
			} else {
				costMatrix[ci][tj] = dist2
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
	dx := cluster.CentroidX - track.X
	dy := cluster.CentroidY - track.Y

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
