package l5tracks

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TrackPoint represents a single point in a track's history.
type TrackPoint struct {
	X         float32
	Y         float32
	Timestamp int64 // Unix nanos
}

// TrackedObject represents a single tracked object in the tracker.
type TrackedObject struct {
	// Identity + shared measurement fields (persisted to both lidar_tracks
	// and lidar_run_tracks)
	TrackID string
	TrackMeasurement

	// Lifecycle counters
	Hits   int // Consecutive successful associations
	Misses int // Consecutive missed associations

	// Kalman state (world frame): [x, y, vx, vy]
	X  float32 // Position X
	Y  float32 // Position Y
	VX float32 // Velocity X
	VY float32 // Velocity Y

	// Kalman covariance (4x4, row-major)
	P [16]float32

	// History of positions
	History []TrackPoint

	// Speed history for jitter/variance analysis and classification features
	speedHistory []float32

	// OBB heading (smoothed via exponential moving average)
	OBBHeadingRad float32       // Smoothed heading from oriented bounding box
	HeadingSource HeadingSource // Source of the current heading (for debug rendering)

	// Latest per-frame OBB dimensions (instantaneous, for real-time rendering)
	OBBLength float32 // Latest frame bounding box length (metres)
	OBBWidth  float32 // Latest frame bounding box width (metres)
	OBBHeight float32 // Latest frame bounding box height (metres)

	// Latest Z from the associated cluster OBB (ground-level, used for rendering)
	LatestZ float32

	// Track quality metrics
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

	// Heading Jitter Metrics
	// Measures frame-to-frame OBB heading instability (spinning bounding boxes).
	HeadingJitterSumSq float64 // Running sum of squared heading deltas (radians²)
	HeadingJitterCount int     // Number of heading delta samples

	// Speed Jitter Metrics
	// Measures frame-to-frame Kalman speed instability (m/s).
	SpeedJitterSumSq float64 // Running sum of squared speed deltas ((m/s)²)
	SpeedJitterCount int     // Number of speed delta samples
	PrevSpeedMps     float32 // Previous frame speed for delta computation

	// Merge/split coherence (task 3.3)
	// When a cluster is significantly larger than the track's historical
	// OBB, it may be a merge of two objects. When a confirmed track's
	// cluster suddenly shrinks while a new track appears nearby, it is
	// likely a split. These flags are advisory — used by quality metrics
	// and labelling rather than hard rejection.
	MergeCandidate bool   // true when current cluster area ≫ historical average
	SplitCandidate bool   // true when current cluster area ≪ historical average while nearby new track appears
	LinkedTrackID  string // if non-empty, the track this one was split from or merged with
}

// Tracker manages multi-object tracking with explicit lifecycle states.
type Tracker struct {
	Tracks      map[string]*TrackedObject
	NextTrackID int64
	Config      TrackerConfig

	// Last update timestamp for dt computation
	LastUpdateNanos int64

	// Fragmentation counters (reset via ResetFragmentation)
	TracksCreated   int
	TracksConfirmed int

	// Scene-level foreground capture accumulators.
	// Updated via RecordFrameStats() from the tracking pipeline.
	TotalForegroundPoints int64 // Running total of foreground points entering DBSCAN
	ClusteredPoints       int64 // Running total of points assigned to DBSCAN clusters

	// Empty box accumulators — updated in Update() per frame.
	EmptyBoxFrames int64 // Running sum of unmatched active tracks across frames
	TotalBoxFrames int64 // Running sum of active tracks across frames

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

// UpdateConfig applies the given function to the tracker's configuration
// under the tracker lock. This is the safe way to mutate Config fields
// from outside the tracking goroutine (e.g. HTTP tuning handlers).
func (t *Tracker) UpdateConfig(fn func(*TrackerConfig)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fn(&t.Config)
}

// GetConfig returns a snapshot of the tracker's current configuration
// under a read lock. Safe to call from any goroutine.
func (t *Tracker) GetConfig() TrackerConfig {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Config
}

// Reset clears all tracks and resets the tracker to its initial state.
// This is used between sweep permutations to ensure each combination
// starts with a clean tracker (no residual Kalman filter state).
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	clearedTracks := len(t.Tracks)
	t.Tracks = make(map[string]*TrackedObject)
	t.NextTrackID = 1
	t.LastUpdateNanos = 0
	t.lastAssociations = nil
	t.TotalForegroundPoints = 0
	t.ClusteredPoints = 0
	t.EmptyBoxFrames = 0
	t.TotalBoxFrames = 0
	diagf("Tracker reset: cleared_tracks=%d", clearedTracks)
}

// NewTracker creates a new tracker with the specified configuration.
func NewTracker(config TrackerConfig) *Tracker {
	tracker := &Tracker{
		Tracks:      make(map[string]*TrackedObject),
		NextTrackID: 1,
		Config:      config,
	}
	diagf("Tracker created: max_tracks=%d hits_to_confirm=%d max_misses=%d max_misses_confirmed=%d",
		config.MaxTracks, config.HitsToConfirm, config.MaxMisses, config.MaxMissesConfirmed)
	return tracker
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
	// Clamp dt to MaxPredictDt so throttle-induced gaps (e.g. 250 ms at
	// 12 fps cap) don't create an inflated time step for association gating.
	// Predict() also clamps independently, but the raw dt flows into
	// associate() where it affects implied-speed plausibility checks
	// (task 7.1).
	if dt > t.Config.MaxPredictDt {
		dt = t.Config.MaxPredictDt
	}
	t.LastUpdateNanos = nowNanos

	if traceLogger != nil {
		activeBefore := 0
		for _, track := range t.Tracks {
			if track.TrackState != TrackDeleted {
				activeBefore++
			}
		}
		tracef("Update start: ts=%d clusters=%d active_tracks=%d dt=%.3f",
			nowNanos, len(clusters), activeBefore, dt)
	}

	// Step 1: Predict all active tracks to current time
	for _, track := range t.Tracks {
		if track.TrackState != TrackDeleted {
			t.predict(track, dt)
		}
	}

	// Step 2: Associate clusters to tracks using gating
	associations := t.associate(clusters, dt)
	t.lastAssociations = associations

	// Step 3: Update matched tracks
	matchedTracks := make(map[string]bool)
	newlyConfirmed := 0
	for clusterIdx, trackID := range associations {
		if trackID != "" {
			track := t.Tracks[trackID]
			t.update(track, clusters[clusterIdx], nowNanos)
			track.Hits++
			track.Misses = 0
			matchedTracks[trackID] = true

			// Promote tentative → confirmed
			if track.TrackState == TrackTentative && track.Hits >= t.Config.HitsToConfirm {
				track.TrackState = TrackConfirmed
				t.TracksConfirmed++
				newlyConfirmed++
				diagf("Track confirmed: track_id=%s hits=%d observations=%d cluster_id=%d",
					track.TrackID, track.Hits, track.ObservationCount, clusters[clusterIdx].ClusterID)
			}
		}
	}

	// Step 3b: Merge/split coherence detection (task 3.3).
	// After association, flag tracks whose associated cluster dimensions
	// deviate significantly from their historical averages. A cluster
	// much larger than the track's average suggests a merge; much smaller
	// suggests a split. These flags are advisory for quality metrics.
	mergeSizeRatio := float64(t.Config.MergeSizeRatio)
	splitSizeRatio := float64(t.Config.SplitSizeRatio)
	for clusterIdx, trackID := range associations {
		if trackID == "" {
			continue
		}
		track := t.Tracks[trackID]
		if track.ObservationCount < 3 {
			continue // need history to compare against
		}
		clusterArea := float64(clusters[clusterIdx].BoundingBoxLength) * float64(clusters[clusterIdx].BoundingBoxWidth)
		historicalArea := float64(track.BoundingBoxLengthAvg) * float64(track.BoundingBoxWidthAvg)
		if historicalArea < 0.01 {
			continue // avoid division by zero
		}
		ratio := clusterArea / historicalArea
		track.MergeCandidate = ratio > mergeSizeRatio
		track.SplitCandidate = ratio < splitSizeRatio
	}

	// Step 4: Handle unmatched tracks with occlusion-aware coasting.
	// Confirmed tracks are allowed more miss frames (MaxMissesConfirmed)
	// than tentative tracks (MaxMisses). During occlusion the Kalman
	// prediction step (already applied above) keeps the position estimate
	// coasting, and we inflate the covariance to widen the gating gate
	// so re-association is easier when the object reappears.
	deletedThisFrame := 0
	for trackID, track := range t.Tracks {
		if !matchedTracks[trackID] && track.TrackState != TrackDeleted {
			track.Misses++
			track.Hits = 0
			track.OcclusionCount++
			if track.Misses > track.MaxOcclusionFrames {
				track.MaxOcclusionFrames = track.Misses
			}

			// Inflate covariance during occlusion so the gating
			// ellipse grows and re-association becomes easier.
			// Capped at MaxCovarianceDiag to prevent unbounded growth
			// over long coasting periods (e.g. 15 frames × 0.5 = +7.5).
			if t.Config.OcclusionCovInflation > 0 {
				track.P[0*4+0] += t.Config.OcclusionCovInflation
				track.P[1*4+1] += t.Config.OcclusionCovInflation
				if track.P[0*4+0] > t.Config.MaxCovarianceDiag {
					track.P[0*4+0] = t.Config.MaxCovarianceDiag
				}
				if track.P[1*4+1] > t.Config.MaxCovarianceDiag {
					track.P[1*4+1] = t.Config.MaxCovarianceDiag
				}
			}

			// Append predicted (coasted) position to history
			distFromOrigin := track.X*track.X + track.Y*track.Y
			if distFromOrigin > 0.01 { // > 0.1m squared
				track.History = append(track.History, TrackPoint{
					X:         track.X,
					Y:         track.Y,
					Timestamp: nowNanos,
				})
				if len(track.History) > t.Config.MaxTrackHistoryLength {
					track.History = track.History[len(track.History)-t.Config.MaxTrackHistoryLength:]
				}
			}

			// Determine miss limit based on track maturity.
			maxMisses := t.Config.MaxMisses
			if track.TrackState == TrackConfirmed && t.Config.MaxMissesConfirmed > 0 {
				maxMisses = t.Config.MaxMissesConfirmed
			}
			if track.Misses >= maxMisses {
				prevState := track.TrackState
				track.TrackState = TrackDeleted
				track.EndUnixNanos = nowNanos
				deletedThisFrame++
				diagf("Track deleted after misses: track_id=%s previous_state=%s misses=%d max_misses=%d",
					track.TrackID, prevState, track.Misses, maxMisses)
			}
		}
	}

	// Step 4b: Update empty box accumulators.
	// Count active tracks not matched to any cluster this frame.
	activeCount := int64(0)
	for _, track := range t.Tracks {
		if track.TrackState != TrackDeleted {
			activeCount++
		}
	}
	matchedCount := int64(len(matchedTracks))
	t.TotalBoxFrames += activeCount
	t.EmptyBoxFrames += activeCount - matchedCount

	// Step 5: Initialise new tracks from unassociated clusters
	newTracks := 0
	for clusterIdx, trackID := range associations {
		if trackID == "" && len(t.Tracks) < t.Config.MaxTracks {
			t.initTrack(clusters[clusterIdx], nowNanos)
			newTracks++
		}
	}

	// Step 6: Cleanup deleted tracks (keep for grace period, then remove)
	t.cleanupDeletedTracks(nowNanos)

	if traceLogger != nil {
		activeAfter := 0
		for _, track := range t.Tracks {
			if track.TrackState != TrackDeleted {
				activeAfter++
			}
		}
		tracef("Update complete: ts=%d active_tracks=%d matched_tracks=%d new_tracks=%d confirmed_tracks=%d deleted_tracks=%d",
			nowNanos, activeAfter, len(matchedTracks), newTracks, newlyConfirmed, deletedThisFrame)
	}
}

// initTrack creates a new track from an unassociated cluster.
// Track IDs are globally unique UUIDs to prevent collisions across tracker
// resets, server restarts, and long-running deployments.
func (t *Tracker) initTrack(cluster WorldCluster, nowNanos int64) *TrackedObject {
	trackID := fmt.Sprintf("trk_%s", uuid.NewString())
	t.NextTrackID++

	track := &TrackedObject{
		TrackID: trackID,
		TrackMeasurement: TrackMeasurement{
			SensorID:             cluster.SensorID,
			TrackState:           TrackTentative,
			StartUnixNanos:       nowNanos,
			EndUnixNanos:         nowNanos,
			ObservationCount:     1,
			BoundingBoxLengthAvg: cluster.BoundingBoxLength,
			BoundingBoxWidthAvg:  cluster.BoundingBoxWidth,
			BoundingBoxHeightAvg: cluster.BoundingBoxHeight,
			HeightP95Max:         cluster.HeightP95,
			IntensityMeanAvg:     cluster.IntensityMean,
		},
		Hits:   1,
		Misses: 0,

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

		TrackLengthMeters: 0,
		TrackDurationSecs: 0,

		History: []TrackPoint{{
			X:         cluster.CentroidX,
			Y:         cluster.CentroidY,
			Timestamp: nowNanos,
		}},

		speedHistory: make([]float32, 0, t.Config.MaxSpeedHistoryLength),
	}

	// Initialise OBB heading and per-frame dimensions from cluster if available
	if cluster.OBB != nil {
		track.OBBHeadingRad = cluster.OBB.HeadingRad
		track.OBBLength = cluster.OBB.Length
		track.OBBWidth = cluster.OBB.Width
		track.OBBHeight = cluster.OBB.Height
		track.LatestZ = cluster.OBB.CenterZ
	}

	t.Tracks[trackID] = track
	t.TracksCreated++
	diagf("Track initialised: track_id=%s cluster_id=%d sensor=%s points=%d",
		trackID, cluster.ClusterID, cluster.SensorID, cluster.PointsCount)
	return track
}

// cleanupDeletedTracks removes tracks that have been deleted for a grace period.
func (t *Tracker) cleanupDeletedTracks(nowNanos int64) {
	gracePeriod := t.Config.DeletedTrackGracePeriod
	gracePeriodNanos := int64(gracePeriod)

	toRemove := make([]string, 0)
	for id, track := range t.Tracks {
		if track.TrackState == TrackDeleted {
			if nowNanos-track.EndUnixNanos > gracePeriodNanos {
				toRemove = append(toRemove, id)
			}
		}
	}

	for _, id := range toRemove {
		delete(t.Tracks, id)
	}
	if len(toRemove) > 0 {
		tracef("Removed deleted tracks: count=%d", len(toRemove))
	}
}

// AdvanceMisses increments the miss counter for every active track by one
// and deletes tracks that exceed their miss budget. This is called on
// throttled frames where the full Update() is skipped so that tracks are
// not artificially kept alive by the lack of cluster delivery (task 7.2).
func (t *Tracker) AdvanceMisses(timestamp time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	nowNanos := timestamp.UnixNano()
	deletedTracks := 0

	for _, track := range t.Tracks {
		if track.TrackState == TrackDeleted {
			continue
		}
		track.Misses++
		track.Hits = 0

		maxMisses := t.Config.MaxMisses
		if track.TrackState == TrackConfirmed && t.Config.MaxMissesConfirmed > 0 {
			maxMisses = t.Config.MaxMissesConfirmed
		}
		if track.Misses >= maxMisses {
			prevState := track.TrackState
			track.TrackState = TrackDeleted
			track.EndUnixNanos = nowNanos
			deletedTracks++
			diagf("Track deleted during AdvanceMisses: track_id=%s previous_state=%s misses=%d max_misses=%d",
				track.TrackID, prevState, track.Misses, maxMisses)
		}
	}
	tracef("AdvanceMisses complete: ts=%d deleted_tracks=%d", nowNanos, deletedTracks)
}
