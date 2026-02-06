package lidar

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// TrackState represents the lifecycle state of a track.
type TrackState string

const (
	TrackTentative TrackState = "tentative" // New track, needs confirmation
	TrackConfirmed TrackState = "confirmed" // Stable track with sufficient history
	TrackDeleted   TrackState = "deleted"   // Track marked for removal
)

// Constants for tracker configuration
const (
	// MinDeterminantThreshold is the minimum determinant for covariance matrix inversion
	MinDeterminantThreshold = 1e-6
	// SingularDistanceRejection is the distance returned when covariance is singular
	SingularDistanceRejection = 1e9
	// MaxSpeedHistoryLength is the maximum number of speed samples kept for percentile computation
	MaxSpeedHistoryLength = 100
	// MaxTrackHistoryLength is the maximum number of position samples kept for trail rendering.
	// At 10 Hz this gives ~20 seconds of trail. Capping prevents unbounded memory growth
	// and reduces serialisation cost in the visualiser adapter.
	MaxTrackHistoryLength = 200
	// DefaultDeletedTrackGracePeriod is how long to keep deleted tracks before cleanup
	DefaultDeletedTrackGracePeriod = 5 * time.Second
	// MaxReasonableSpeedMps is the maximum reasonable speed for any tracked object (m/s)
	// Used to reject spurious associations that would imply impossible velocities
	MaxReasonableSpeedMps = 30.0 // ~108 km/h, ~67 mph
	// MaxPositionJumpMeters is the maximum allowed position jump between consecutive observations
	// Observations beyond this distance are rejected as likely false associations
	MaxPositionJumpMeters = 5.0
)

// TrackerConfig holds configuration parameters for the tracker.
type TrackerConfig struct {
	MaxTracks               int           // Maximum number of concurrent tracks
	MaxMisses               int           // Consecutive misses before tentative track deletion
	MaxMissesConfirmed      int           // Consecutive misses before confirmed track deletion (coasting)
	HitsToConfirm           int           // Consecutive hits needed for confirmation
	GatingDistanceSquared   float32       // Squared gating distance for association (meters²)
	ProcessNoisePos         float32       // Process noise for position (σ²)
	ProcessNoiseVel         float32       // Process noise for velocity (σ²)
	MeasurementNoise        float32       // Measurement noise (σ²)
	OcclusionCovInflation   float32       // Extra covariance inflation per occluded frame
	DeletedTrackGracePeriod time.Duration // How long to keep deleted tracks before cleanup
}

// DefaultTrackerConfig returns default tracker configuration.
func DefaultTrackerConfig() TrackerConfig {
	return TrackerConfig{
		MaxTracks:               100,
		MaxMisses:               3,
		MaxMissesConfirmed:      15,   // Confirmed tracks coast through occlusion (~1.5s at 10Hz)
		HitsToConfirm:           3,    // Require 3 consecutive hits for confirmation
		GatingDistanceSquared:   36.0, // 6.0 metres squared — wider gate for re-association
		ProcessNoisePos:         0.1,
		ProcessNoiseVel:         0.5,
		MeasurementNoise:        0.2,
		OcclusionCovInflation:   0.5, // Widen gating gate during occlusion
		DeletedTrackGracePeriod: DefaultDeletedTrackGracePeriod,
	}
}

// TrackPoint represents a single point in a track's history.
type TrackPoint struct {
	X         float32
	Y         float32
	Timestamp int64 // Unix nanos
}

// TrackedObject represents a single tracked object in the tracker.
type TrackedObject struct {
	// Identity
	TrackID  string
	SensorID string
	State    TrackState

	// Lifecycle counters
	Hits   int // Consecutive successful associations
	Misses int // Consecutive missed associations

	// Timestamps
	FirstUnixNanos int64
	LastUnixNanos  int64

	// Kalman state (world frame): [x, y, vx, vy]
	X  float32 // Position X
	Y  float32 // Position Y
	VX float32 // Velocity X
	VY float32 // Velocity Y

	// Kalman covariance (4x4, row-major)
	P [16]float32

	// Aggregated features
	ObservationCount     int
	BoundingBoxLengthAvg float32
	BoundingBoxWidthAvg  float32
	BoundingBoxHeightAvg float32
	HeightP95Max         float32
	IntensityMeanAvg     float32
	AvgSpeedMps          float32
	PeakSpeedMps         float32

	// History of positions
	History []TrackPoint

	// Speed history for percentile computation
	speedHistory []float32

	// OBB heading (smoothed via exponential moving average)
	OBBHeadingRad float32 // Smoothed heading from oriented bounding box

	// Latest Z from the associated cluster OBB (ground-level, used for rendering)
	LatestZ float32

	// Classification (Phase 3.4)
	ObjectClass         string  // Classification result: "pedestrian", "car", "bird", "other"
	ObjectConfidence    float32 // Classification confidence [0, 1]
	ClassificationModel string  // Model version used for classification

	// Phase 1: Track Quality Metrics
	TrackLengthMeters  float32 // Total distance traveled (meters)
	TrackDurationSecs  float32 // Total lifetime (seconds)
	OcclusionCount     int     // Number of missed frames (gaps)
	MaxOcclusionFrames int     // Longest gap in observations
	SpatialCoverage    float32 // % of bounding box covered by observations
	NoisePointRatio    float32 // Ratio of noise points to cluster points

	// Velocity-Trail Alignment Metrics
	// Measures how well the Kalman velocity vector aligns with the actual
	// direction of travel computed from recent trail positions. A perfectly
	// aligned track has AlignmentMeanRad ≈ 0.
	AlignmentSampleCount int     // Number of alignment samples taken
	AlignmentSumRad      float32 // Running sum of angular differences (radians)
	AlignmentMeanRad     float32 // Running mean angular difference (radians, [0, π])
	AlignmentMisaligned  int     // Count of samples where angular diff > π/4 (45°)
}

// TrackingMetrics holds aggregate tracking quality metrics across all active tracks.
// Used by the sweep tool to evaluate parameter configurations.
type TrackingMetrics struct {
	// Number of active tracks contributing to metrics
	ActiveTracks int `json:"active_tracks"`
	// Total alignment samples across all tracks
	TotalAlignmentSamples int `json:"total_alignment_samples"`
	// Mean angular difference between velocity vector and displacement direction (radians)
	MeanAlignmentRad float32 `json:"mean_alignment_rad"`
	// Mean angular difference in degrees (convenience)
	MeanAlignmentDeg float32 `json:"mean_alignment_deg"`
	// Total misaligned samples (angular diff > 45°) across all tracks
	TotalMisaligned int `json:"total_misaligned"`
	// Misalignment ratio: misaligned / total samples [0, 1]
	MisalignmentRatio float32 `json:"misalignment_ratio"`
	// Per-track alignment breakdown
	PerTrack []TrackAlignmentMetrics `json:"per_track,omitempty"`
}

// TrackAlignmentMetrics holds velocity alignment metrics for a single track.
type TrackAlignmentMetrics struct {
	TrackID          string  `json:"track_id"`
	State            string  `json:"state"`
	SampleCount      int     `json:"sample_count"`
	MeanAlignmentRad float32 `json:"mean_alignment_rad"`
	MeanAlignmentDeg float32 `json:"mean_alignment_deg"`
	MisalignedCount  int     `json:"misaligned_count"`
	MisalignmentRate float32 `json:"misalignment_rate"`
	SpeedMps         float32 `json:"speed_mps"`
}

// Tracker manages multi-object tracking with explicit lifecycle states.
type Tracker struct {
	Tracks      map[string]*TrackedObject
	NextTrackID int64
	Config      TrackerConfig

	// Last update timestamp for dt computation
	LastUpdateNanos int64

	// lastAssociations stores the result of the most recent associate() call.
	// It is a slice indexed by cluster index; each element is the trackID
	// the cluster was associated with, or "" if unassociated.
	// Protected by mu — read via GetLastAssociations().
	lastAssociations []string

	// DebugCollector captures algorithm internals for visualisation (optional)
	DebugCollector DebugCollector

	mu sync.RWMutex
}

// DebugCollector interface for tracking algorithm instrumentation.
// Allows decoupling from the debug package to avoid circular dependencies.
type DebugCollector interface {
	IsEnabled() bool
	RecordAssociation(clusterID int64, trackID string, distSquared float32, accepted bool)
	RecordGatingRegion(trackID string, centerX, centerY, semiMajor, semiMinor, rotation float32)
	RecordInnovation(trackID string, predX, predY, measX, measY, residualMag float32)
	RecordPrediction(trackID string, x, y, vx, vy float32)
}

// NewTracker creates a new tracker with the specified configuration.
func NewTracker(config TrackerConfig) *Tracker {
	return &Tracker{
		Tracks:      make(map[string]*TrackedObject),
		NextTrackID: 1,
		Config:      config,
	}
}

// Update processes a new frame of clusters and updates tracks.
// This is the main entry point for the tracking pipeline.
func (t *Tracker) Update(clusters []WorldCluster, timestamp time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	nowNanos := timestamp.UnixNano()

	// Compute dt (time delta since last update)
	var dt float32
	if t.LastUpdateNanos > 0 {
		dt = float32(nowNanos-t.LastUpdateNanos) / 1e9 // Convert to seconds
	} else {
		dt = 0.1 // Default 100ms for first frame
	}
	t.LastUpdateNanos = nowNanos

	// Step 1: Predict all active tracks to current time
	for _, track := range t.Tracks {
		if track.State != TrackDeleted {
			t.predict(track, dt)
		}
	}

	// Step 2: Associate clusters to tracks using gating
	associations := t.associate(clusters, dt)
	t.lastAssociations = associations

	// Step 3: Update matched tracks
	matchedTracks := make(map[string]bool)
	for clusterIdx, trackID := range associations {
		if trackID != "" {
			track := t.Tracks[trackID]
			t.update(track, clusters[clusterIdx], nowNanos)
			track.Hits++
			track.Misses = 0
			matchedTracks[trackID] = true

			// Promote tentative → confirmed
			if track.State == TrackTentative && track.Hits >= t.Config.HitsToConfirm {
				track.State = TrackConfirmed
			}
		}
	}

	// Step 4: Handle unmatched tracks with occlusion-aware coasting.
	// Confirmed tracks are allowed more miss frames (MaxMissesConfirmed)
	// than tentative tracks (MaxMisses). During occlusion the Kalman
	// prediction step (already applied above) keeps the position estimate
	// coasting, and we inflate the covariance to widen the gating gate
	// so re-association is easier when the object reappears.
	for trackID, track := range t.Tracks {
		if !matchedTracks[trackID] && track.State != TrackDeleted {
			track.Misses++
			track.Hits = 0
			track.OcclusionCount++
			if track.Misses > track.MaxOcclusionFrames {
				track.MaxOcclusionFrames = track.Misses
			}

			// Inflate covariance during occlusion so the gating
			// ellipse grows and re-association becomes easier.
			if t.Config.OcclusionCovInflation > 0 {
				track.P[0*4+0] += t.Config.OcclusionCovInflation
				track.P[1*4+1] += t.Config.OcclusionCovInflation
			}

			// Append predicted (coasted) position to history
			distFromOrigin := track.X*track.X + track.Y*track.Y
			if distFromOrigin > 0.01 { // > 0.1m squared
				track.History = append(track.History, TrackPoint{
					X:         track.X,
					Y:         track.Y,
					Timestamp: nowNanos,
				})
				if len(track.History) > MaxTrackHistoryLength {
					track.History = track.History[len(track.History)-MaxTrackHistoryLength:]
				}
			}

			// Determine miss limit based on track maturity.
			maxMisses := t.Config.MaxMisses
			if track.State == TrackConfirmed && t.Config.MaxMissesConfirmed > 0 {
				maxMisses = t.Config.MaxMissesConfirmed
			}
			if track.Misses >= maxMisses {
				track.State = TrackDeleted
				track.LastUnixNanos = nowNanos
			}
		}
	}

	// Step 5: Initialise new tracks from unassociated clusters
	for clusterIdx, trackID := range associations {
		if trackID == "" && len(t.Tracks) < t.Config.MaxTracks {
			t.initTrack(clusters[clusterIdx], nowNanos)
		}
	}

	// Step 6: Cleanup deleted tracks (keep for grace period, then remove)
	t.cleanupDeletedTracks(nowNanos)
}

// predict applies the Kalman prediction step using constant velocity model.
func (t *Tracker) predict(track *TrackedObject, dt float32) {
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

	// Add process noise Q
	// Q = diag([σ_pos², σ_pos², σ_vel², σ_vel²])
	track.P[0*4+0] += t.Config.ProcessNoisePos
	track.P[1*4+1] += t.Config.ProcessNoisePos
	track.P[2*4+2] += t.Config.ProcessNoiseVel
	track.P[3*4+3] += t.Config.ProcessNoiseVel
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
		if track.State != TrackDeleted {
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
			if dist2 >= SingularDistanceRejection || dist2 >= float32(hungarianlnf) {
				costMatrix[ci][tj] = float32(hungarianlnf)
			} else {
				costMatrix[ci][tj] = dist2
			}
		}
	}

	// Solve optimal assignment.
	assign := hungarianAssign(costMatrix)

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
	if euclideanDist > MaxPositionJumpMeters {
		return SingularDistanceRejection
	}

	// Check if implied velocity would be unreasonable
	if dt > 0 {
		impliedSpeed := euclideanDist / dt
		if impliedSpeed > MaxReasonableSpeedMps {
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

// update applies the Kalman update step with a matched cluster measurement.
func (t *Tracker) update(track *TrackedObject, cluster WorldCluster, nowNanos int64) {
	// Measurement: z = [cluster.CentroidX, cluster.CentroidY]
	zX := cluster.CentroidX
	zY := cluster.CentroidY

	// Innovation
	yX := zX - track.X
	yY := zY - track.Y

	// Record innovation for debug visualisation
	if t.DebugCollector != nil && t.DebugCollector.IsEnabled() {
		residualMag := float32(math.Sqrt(float64(yX*yX + yY*yY)))
		t.DebugCollector.RecordInnovation(track.TrackID, track.X, track.Y, zX, zY, residualMag)
	}

	// Innovation covariance S = H * P * H^T + R
	S00 := track.P[0*4+0] + t.Config.MeasurementNoise
	S01 := track.P[0*4+1]
	S10 := track.P[1*4+0]
	S11 := track.P[1*4+1] + t.Config.MeasurementNoise

	// Compute S inverse
	det := S00*S11 - S01*S10
	if det < MinDeterminantThreshold {
		return // Cannot update with singular covariance
	}

	invS00 := S11 / det
	invS01 := -S01 / det
	invS10 := -S10 / det
	invS11 := S00 / det

	// Kalman gain K = P * H^T * S^-1
	// K is 4x2 matrix
	// K[i,0] = P[i,0]*invS00 + P[i,1]*invS10
	// K[i,1] = P[i,0]*invS01 + P[i,1]*invS11
	var K [8]float32
	for i := 0; i < 4; i++ {
		K[i*2+0] = track.P[i*4+0]*invS00 + track.P[i*4+1]*invS10
		K[i*2+1] = track.P[i*4+0]*invS01 + track.P[i*4+1]*invS11
	}

	// Update state: x' = x + K * y
	track.X += K[0*2+0]*yX + K[0*2+1]*yY
	track.Y += K[1*2+0]*yX + K[1*2+1]*yY
	track.VX += K[2*2+0]*yX + K[2*2+1]*yY
	track.VY += K[3*2+0]*yX + K[3*2+1]*yY

	// Update covariance: P' = (I - K*H) * P
	// K*H is 4x4, where (K*H)[i,j] = K[i,0]*H[0,j] + K[i,1]*H[1,j]
	// H[0,0]=1, H[0,1]=0, H[0,2]=0, H[0,3]=0
	// H[1,0]=0, H[1,1]=1, H[1,2]=0, H[1,3]=0
	// So (K*H)[i,j] = K[i,0] if j==0, K[i,1] if j==1, 0 otherwise
	var IminusKH [16]float32
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			identity := float32(0)
			if i == j {
				identity = 1
			}
			var kh float32
			if j == 0 {
				kh = K[i*2+0]
			} else if j == 1 {
				kh = K[i*2+1]
			}
			IminusKH[i*4+j] = identity - kh
		}
	}

	// P' = IminusKH * P
	var newP [16]float32
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += IminusKH[i*4+k] * track.P[k*4+j]
			}
			newP[i*4+j] = sum
		}
	}
	track.P = newP

	// Update timestamp
	track.LastUnixNanos = nowNanos

	// Update aggregated features
	track.ObservationCount++

	// Running average for bounding box
	n := float32(track.ObservationCount)
	track.BoundingBoxLengthAvg = ((n-1)*track.BoundingBoxLengthAvg + cluster.BoundingBoxLength) / n
	track.BoundingBoxWidthAvg = ((n-1)*track.BoundingBoxWidthAvg + cluster.BoundingBoxWidth) / n
	track.BoundingBoxHeightAvg = ((n-1)*track.BoundingBoxHeightAvg + cluster.BoundingBoxHeight) / n
	track.IntensityMeanAvg = ((n-1)*track.IntensityMeanAvg + cluster.IntensityMean) / n

	// Max height P95
	if cluster.HeightP95 > track.HeightP95Max {
		track.HeightP95Max = cluster.HeightP95
	}

	// Update speed statistics
	speed := float32(math.Sqrt(float64(track.VX*track.VX + track.VY*track.VY)))
	track.AvgSpeedMps = ((n-1)*track.AvgSpeedMps + speed) / n
	if speed > track.PeakSpeedMps {
		track.PeakSpeedMps = speed
	}

	// Append to history
	// Skip points too close to origin (noise/self-reflection)
	distFromOrigin := track.X*track.X + track.Y*track.Y
	if distFromOrigin > 0.01 { // > 0.1m squared
		track.History = append(track.History, TrackPoint{
			X:         track.X,
			Y:         track.Y,
			Timestamp: nowNanos,
		})
		if len(track.History) > MaxTrackHistoryLength {
			track.History = track.History[len(track.History)-MaxTrackHistoryLength:]
		}
	}

	// Store speed history for percentile computation
	track.speedHistory = append(track.speedHistory, speed)
	if len(track.speedHistory) > MaxSpeedHistoryLength {
		track.speedHistory = track.speedHistory[1:]
	}

	// Velocity-Trail Alignment: Compare Kalman velocity heading with
	// displacement heading from the last two trail positions.
	// Only compute when the track has sufficient history and speed.
	if len(track.History) >= 2 && speed > 0.5 { // Need ≥2 points and moving
		prev := track.History[len(track.History)-2]
		curr := track.History[len(track.History)-1]

		dx := curr.X - prev.X
		dy := curr.Y - prev.Y
		displacementDist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if displacementDist > 0.05 { // Minimum displacement to compute heading (5cm)
			displacementHeading := float32(math.Atan2(float64(dy), float64(dx)))
			velocityHeading := float32(math.Atan2(float64(track.VY), float64(track.VX)))

			// Angular difference, normalised to [0, π]
			angDiff := velocityHeading - displacementHeading
			for angDiff > math.Pi {
				angDiff -= 2 * math.Pi
			}
			for angDiff < -math.Pi {
				angDiff += 2 * math.Pi
			}
			absAngDiff := float32(math.Abs(float64(angDiff)))

			track.AlignmentSampleCount++
			track.AlignmentSumRad += absAngDiff
			track.AlignmentMeanRad = track.AlignmentSumRad / float32(track.AlignmentSampleCount)
			if absAngDiff > math.Pi/4 { // > 45° is misaligned
				track.AlignmentMisaligned++
			}
		}
	}

	// Update OBB heading with temporal smoothing.
	// Alpha = 0.15 provides strong smoothing to reduce PCA jitter while still
	// tracking genuine orientation changes. PCA heading can flip between
	// frames when the point cloud is sparse or the visible surface changes,
	// so heavier smoothing prevents the bounding box from spinning erratically.
	// Note: OBBHeadingRad is initialised in initTrack if cluster has OBB,
	// so subsequent updates here always apply smoothing.
	if cluster.OBB != nil {
		newOBBHeading := cluster.OBB.HeadingRad

		// Disambiguate PCA heading using velocity direction.
		// PCA gives the axis of maximum variance but has 180° ambiguity.
		// If the track has sufficient velocity, flip the PCA heading
		// to align with the direction of travel.
		speed := float32(math.Sqrt(float64(track.VX*track.VX + track.VY*track.VY)))
		if speed > 0.5 { // Only disambiguate when moving (>0.5 m/s)
			velHeading := float32(math.Atan2(float64(track.VY), float64(track.VX)))
			// Compute angular difference between PCA heading and velocity heading
			diff := newOBBHeading - velHeading
			// Normalise to [-π, π]
			for diff > math.Pi {
				diff -= 2 * math.Pi
			}
			for diff < -math.Pi {
				diff += 2 * math.Pi
			}
			// If PCA heading opposes velocity (diff > 90°), flip it by π
			if diff > math.Pi/2 || diff < -math.Pi/2 {
				newOBBHeading += math.Pi
				if newOBBHeading > math.Pi {
					newOBBHeading -= 2 * math.Pi
				}
			}
		}

		track.OBBHeadingRad = SmoothOBBHeading(track.OBBHeadingRad, newOBBHeading, 0.15)
		track.LatestZ = cluster.OBB.CenterZ
	}
}

// initTrack creates a new track from an unassociated cluster.
func (t *Tracker) initTrack(cluster WorldCluster, nowNanos int64) *TrackedObject {
	trackID := fmt.Sprintf("track_%d", t.NextTrackID)
	t.NextTrackID++

	track := &TrackedObject{
		TrackID:  trackID,
		SensorID: cluster.SensorID,
		State:    TrackTentative,
		Hits:     1,
		Misses:   0,

		FirstUnixNanos: nowNanos,
		LastUnixNanos:  nowNanos,

		// Initialise position from cluster centroid
		X: cluster.CentroidX,
		Y: cluster.CentroidY,
		// Initialise velocity to zero
		VX: 0,
		VY: 0,

		// Initialise covariance with high uncertainty
		P: [16]float32{
			10, 0, 0, 0, // High position uncertainty
			0, 10, 0, 0,
			0, 0, 1, 0, // Lower velocity uncertainty
			0, 0, 0, 1,
		},

		// Initialise features
		ObservationCount:     1,
		BoundingBoxLengthAvg: cluster.BoundingBoxLength,
		BoundingBoxWidthAvg:  cluster.BoundingBoxWidth,
		BoundingBoxHeightAvg: cluster.BoundingBoxHeight,
		HeightP95Max:         cluster.HeightP95,
		IntensityMeanAvg:     cluster.IntensityMean,

		History: []TrackPoint{{
			X:         cluster.CentroidX,
			Y:         cluster.CentroidY,
			Timestamp: nowNanos,
		}},

		speedHistory: make([]float32, 0, MaxSpeedHistoryLength),
	}

	// Initialise OBB heading from cluster if available
	if cluster.OBB != nil {
		track.OBBHeadingRad = cluster.OBB.HeadingRad
		track.LatestZ = cluster.OBB.CenterZ
	}

	t.Tracks[trackID] = track
	return track
}

// cleanupDeletedTracks removes tracks that have been deleted for a grace period.
func (t *Tracker) cleanupDeletedTracks(nowNanos int64) {
	gracePeriod := t.Config.DeletedTrackGracePeriod
	if gracePeriod == 0 {
		gracePeriod = DefaultDeletedTrackGracePeriod
	}
	gracePeriodNanos := int64(gracePeriod)

	toRemove := make([]string, 0)
	for id, track := range t.Tracks {
		if track.State == TrackDeleted {
			if nowNanos-track.LastUnixNanos > gracePeriodNanos {
				toRemove = append(toRemove, id)
			}
		}
	}

	for _, id := range toRemove {
		delete(t.Tracks, id)
	}
}

// GetActiveTracks returns a slice of currently active (non-deleted) tracks.
// Each returned TrackedObject is a shallow copy with a deep-copied History slice,
// making it safe for callers to read History without holding the tracker lock.
// This prevents data races between the visualiser adapter (reading History) and
// the tracker Update() goroutine (appending to History).
func (t *Tracker) GetActiveTracks() []*TrackedObject {
	t.mu.RLock()
	defer t.mu.RUnlock()

	active := make([]*TrackedObject, 0, len(t.Tracks))
	for _, track := range t.Tracks {
		if track.State != TrackDeleted {
			// Shallow copy the struct to snapshot scalar fields
			copied := *track
			// Deep copy History to avoid race with concurrent Update() appends
			if len(track.History) > 0 {
				copied.History = make([]TrackPoint, len(track.History))
				copy(copied.History, track.History)
			}
			active = append(active, &copied)
		}
	}
	return active
}

// GetConfirmedTracks returns only confirmed tracks.
func (t *Tracker) GetConfirmedTracks() []*TrackedObject {
	t.mu.RLock()
	defer t.mu.RUnlock()

	confirmed := make([]*TrackedObject, 0)
	for _, track := range t.Tracks {
		if track.State == TrackConfirmed {
			confirmed = append(confirmed, track)
		}
	}
	return confirmed
}

// GetTrack returns a track by ID, or nil if not found.
func (t *Tracker) GetTrack(trackID string) *TrackedObject {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Tracks[trackID]
}

// GetTrackCount returns counts of tracks by state.
func (t *Tracker) GetTrackCount() (total, tentative, confirmed, deleted int) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, track := range t.Tracks {
		total++
		switch track.State {
		case TrackTentative:
			tentative++
		case TrackConfirmed:
			confirmed++
		case TrackDeleted:
			deleted++
		}
	}
	return
}

// GetAllTracks returns a slice of all tracks including deleted ones.
// This is useful for analysis and reporting after processing is complete.
func (t *Tracker) GetAllTracks() []*TrackedObject {
	t.mu.RLock()
	defer t.mu.RUnlock()

	all := make([]*TrackedObject, 0, len(t.Tracks))
	for _, track := range t.Tracks {
		all = append(all, track)
	}
	return all
}

// GetRecentlyDeletedTracks returns deleted tracks still within the grace period.
// Each returned TrackedObject is a shallow copy with a deep-copied History slice.
// Used by the visualiser adapter for fade-out rendering.
func (t *Tracker) GetRecentlyDeletedTracks(nowNanos int64) []*TrackedObject {
	t.mu.RLock()
	defer t.mu.RUnlock()

	gracePeriod := t.Config.DeletedTrackGracePeriod
	if gracePeriod == 0 {
		gracePeriod = DefaultDeletedTrackGracePeriod
	}
	gracePeriodNanos := int64(gracePeriod)

	deleted := make([]*TrackedObject, 0)
	for _, track := range t.Tracks {
		if track.State == TrackDeleted {
			elapsed := nowNanos - track.LastUnixNanos
			if elapsed >= 0 && elapsed < gracePeriodNanos {
				// Shallow copy + deep copy History
				copied := *track
				if len(track.History) > 0 {
					copied.History = make([]TrackPoint, len(track.History))
					copy(copied.History, track.History)
				}
				deleted = append(deleted, &copied)
			}
		}
	}
	return deleted
}

// GetLastAssociations returns a copy of the most recent cluster-to-track
// associations produced by Update(). The returned slice is indexed by
// cluster index; each element is the trackID the cluster was matched to,
// or "" if the cluster was unassociated (and spawned a new track).
func (t *Tracker) GetLastAssociations() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.lastAssociations == nil {
		return nil
	}
	out := make([]string, len(t.lastAssociations))
	copy(out, t.lastAssociations)
	return out
}

// Speed returns the current speed magnitude for a track.
func (track *TrackedObject) Speed() float32 {
	return float32(math.Sqrt(float64(track.VX*track.VX + track.VY*track.VY)))
}

// Heading returns the current heading in radians for a track.
func (track *TrackedObject) Heading() float32 {
	return float32(math.Atan2(float64(track.VY), float64(track.VX)))
}

// SpeedHistory returns a copy of the track's speed history for percentile computation.
func (track *TrackedObject) SpeedHistory() []float32 {
	if track.speedHistory == nil {
		return nil
	}
	result := make([]float32, len(track.speedHistory))
	copy(result, track.speedHistory)
	return result
}

// ComputeQualityMetrics calculates track quality metrics for Phase 1.
// This should be called when a track is finalized (state changes to deleted or when exporting).
func (track *TrackedObject) ComputeQualityMetrics() {
	// Track length: Sum of Euclidean distances between consecutive positions
	track.TrackLengthMeters = 0
	if len(track.History) > 1 {
		for i := 1; i < len(track.History); i++ {
			dx := track.History[i].X - track.History[i-1].X
			dy := track.History[i].Y - track.History[i-1].Y
			track.TrackLengthMeters += float32(math.Sqrt(float64(dx*dx + dy*dy)))
		}
	}

	// Track duration: Total lifetime in seconds
	if track.LastUnixNanos > track.FirstUnixNanos {
		track.TrackDurationSecs = float32(track.LastUnixNanos-track.FirstUnixNanos) / 1e9
	}

	// Occlusion count: Count gaps in observations (>200ms = missed frame at ~10Hz)
	const occlusionThresholdNanos = 200_000_000 // 200ms
	track.OcclusionCount = 0
	track.MaxOcclusionFrames = 0

	if len(track.History) > 1 {
		for i := 1; i < len(track.History); i++ {
			gap := track.History[i].Timestamp - track.History[i-1].Timestamp
			if gap > occlusionThresholdNanos {
				track.OcclusionCount++
				// Estimate frames at 10Hz
				gapFrames := int(gap / 100_000_000) // 100ms per frame
				if gapFrames > track.MaxOcclusionFrames {
					track.MaxOcclusionFrames = gapFrames
				}
			}
		}
	}

	// Spatial coverage: Ratio of observed area to theoretical max
	// This is a simplified metric - more sophisticated versions could track
	// actual point cloud coverage within the bounding box
	if track.ObservationCount > 0 {
		// Estimate coverage as (observations / theoretical_max_observations)
		// At 10Hz, theoretical max = duration * 10
		theoreticalMax := track.TrackDurationSecs * 10
		if theoreticalMax > 0 {
			track.SpatialCoverage = float32(track.ObservationCount) / theoreticalMax
			// Clamp to [0, 1]
			if track.SpatialCoverage > 1.0 {
				track.SpatialCoverage = 1.0
			}
		}
	}

	// Note: NoisePointRatio is computed during clustering and passed via clusters
	// It will be aggregated when clusters are associated with tracks
}

// GetTrackingMetrics computes aggregate velocity-trail alignment metrics
// across all active and confirmed tracks. Used by the sweep tool to
// evaluate tracking parameter configurations.
func (t *Tracker) GetTrackingMetrics() TrackingMetrics {
	t.mu.RLock()
	defer t.mu.RUnlock()

	metrics := TrackingMetrics{}
	var totalSamples int
	var totalAngDiff float32
	var totalMisaligned int

	for _, track := range t.Tracks {
		if track.State == TrackDeleted {
			continue
		}
		metrics.ActiveTracks++

		if track.AlignmentSampleCount == 0 {
			continue
		}

		totalSamples += track.AlignmentSampleCount
		totalAngDiff += track.AlignmentSumRad
		totalMisaligned += track.AlignmentMisaligned

		var misalignmentRate float32
		if track.AlignmentSampleCount > 0 {
			misalignmentRate = float32(track.AlignmentMisaligned) / float32(track.AlignmentSampleCount)
		}

		metrics.PerTrack = append(metrics.PerTrack, TrackAlignmentMetrics{
			TrackID:          track.TrackID,
			State:            string(track.State),
			SampleCount:      track.AlignmentSampleCount,
			MeanAlignmentRad: track.AlignmentMeanRad,
			MeanAlignmentDeg: track.AlignmentMeanRad * 180 / math.Pi,
			MisalignedCount:  track.AlignmentMisaligned,
			MisalignmentRate: misalignmentRate,
			SpeedMps:         track.Speed(),
		})
	}

	metrics.TotalAlignmentSamples = totalSamples
	metrics.TotalMisaligned = totalMisaligned

	if totalSamples > 0 {
		metrics.MeanAlignmentRad = totalAngDiff / float32(totalSamples)
		metrics.MeanAlignmentDeg = metrics.MeanAlignmentRad * 180 / math.Pi
		metrics.MisalignmentRatio = float32(totalMisaligned) / float32(totalSamples)
	}

	return metrics
}
