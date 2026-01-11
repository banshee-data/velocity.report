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
	// DefaultDeletedTrackGracePeriod is how long to keep deleted tracks before cleanup
	DefaultDeletedTrackGracePeriod = 5 * time.Second
)

// TrackerConfig holds configuration parameters for the tracker.
type TrackerConfig struct {
	MaxTracks               int           // Maximum number of concurrent tracks
	MaxMisses               int           // Consecutive misses before deletion
	HitsToConfirm           int           // Consecutive hits needed for confirmation
	GatingDistanceSquared   float32       // Squared gating distance for association (meters²)
	ProcessNoisePos         float32       // Process noise for position (σ²)
	ProcessNoiseVel         float32       // Process noise for velocity (σ²)
	MeasurementNoise        float32       // Measurement noise (σ²)
	DeletedTrackGracePeriod time.Duration // How long to keep deleted tracks before cleanup
}

// DefaultTrackerConfig returns default tracker configuration.
func DefaultTrackerConfig() TrackerConfig {
	return TrackerConfig{
		MaxTracks:               100,
		MaxMisses:               3,
		HitsToConfirm:           3,
		GatingDistanceSquared:   25.0, // 5.0 meters squared
		ProcessNoisePos:         0.1,
		ProcessNoiseVel:         0.5,
		MeasurementNoise:        0.2,
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

	// Classification (Phase 3.4)
	ObjectClass         string  // Classification result: "pedestrian", "car", "bird", "other"
	ObjectConfidence    float32 // Classification confidence [0, 1]
	ClassificationModel string  // Model version used for classification
}

// Tracker manages multi-object tracking with explicit lifecycle states.
type Tracker struct {
	Tracks      map[string]*TrackedObject
	NextTrackID int64
	Config      TrackerConfig

	// Last update timestamp for dt computation
	LastUpdateNanos int64

	mu sync.RWMutex
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
	associations := t.associate(clusters)

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

	// Step 4: Handle unmatched tracks
	for trackID, track := range t.Tracks {
		if !matchedTracks[trackID] && track.State != TrackDeleted {
			track.Misses++
			track.Hits = 0

			if track.Misses >= t.Config.MaxMisses {
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

// associate performs cluster-to-track association using gating and nearest neighbor.
// Returns a map from cluster index to track ID (empty string for unassociated).
func (t *Tracker) associate(clusters []WorldCluster) []string {
	associations := make([]string, len(clusters))

	// Build list of active track IDs
	activeTrackIDs := make([]string, 0, len(t.Tracks))
	for id, track := range t.Tracks {
		if track.State != TrackDeleted {
			activeTrackIDs = append(activeTrackIDs, id)
		}
	}

	// For each cluster, find best matching track within gating distance
	trackUsed := make(map[string]bool)

	for ci, cluster := range clusters {
		bestTrackID := ""
		bestDist2 := t.Config.GatingDistanceSquared

		for _, trackID := range activeTrackIDs {
			if trackUsed[trackID] {
				continue // Track already matched
			}

			track := t.Tracks[trackID]
			dist2 := t.mahalanobisDistanceSquared(track, cluster)

			if dist2 < bestDist2 {
				bestDist2 = dist2
				bestTrackID = trackID
			}
		}

		if bestTrackID != "" {
			associations[ci] = bestTrackID
			trackUsed[bestTrackID] = true
		}
	}

	return associations
}

// mahalanobisDistanceSquared computes the squared Mahalanobis distance for gating.
// Uses only position (x, y) for distance computation.
func (t *Tracker) mahalanobisDistanceSquared(track *TrackedObject, cluster WorldCluster) float32 {
	// Innovation: difference between measurement and prediction
	dx := cluster.CentroidX - track.X
	dy := cluster.CentroidY - track.Y

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
	track.History = append(track.History, TrackPoint{
		X:         track.X,
		Y:         track.Y,
		Timestamp: nowNanos,
	})

	// Store speed history for percentile computation
	track.speedHistory = append(track.speedHistory, speed)
	if len(track.speedHistory) > MaxSpeedHistoryLength {
		track.speedHistory = track.speedHistory[1:]
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
func (t *Tracker) GetActiveTracks() []*TrackedObject {
	t.mu.RLock()
	defer t.mu.RUnlock()

	active := make([]*TrackedObject, 0, len(t.Tracks))
	for _, track := range t.Tracks {
		if track.State != TrackDeleted {
			active = append(active, track)
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
