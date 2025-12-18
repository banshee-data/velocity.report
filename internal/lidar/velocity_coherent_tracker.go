package lidar

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// =============================================================================
// VelocityCoherentTracker - Orchestrates all 5 phases of VC algorithm
// =============================================================================

// VelocityCoherentTrackerConfig holds all configuration for the VC tracker.
type VelocityCoherentTrackerConfig struct {
	// Phase 1: Velocity estimation
	VelocityEstimation VelocityEstimationConfig

	// Phase 2: 6D Clustering
	Clustering Clustering6DConfig

	// Phase 3: Long-tail management
	PreTail  PreTailConfig
	PostTail PostTailConfig

	// Phase 4: Sparse continuation
	SparseContinuation SparseTrackConfig

	// Phase 5: Fragment merging
	Merge MergeConfig

	// Standard tracking parameters
	MaxTracks     int // Maximum concurrent tracks
	MaxMisses     int // Consecutive misses before post-tail
	HitsToConfirm int // Hits needed for confirmation

	// Sensor boundary for entry/exit detection
	SensorBoundary SensorBoundary
}

// DefaultVelocityCoherentTrackerConfig returns sensible defaults.
func DefaultVelocityCoherentTrackerConfig() VelocityCoherentTrackerConfig {
	return VelocityCoherentTrackerConfig{
		VelocityEstimation: DefaultVelocityEstimationConfig(),
		Clustering:         DefaultClustering6DConfig(),
		PreTail:            DefaultPreTailConfig(),
		PostTail:           DefaultPostTailConfig(),
		SparseContinuation: DefaultSparseTrackConfig(),
		Merge:              DefaultMergeConfig(),
		MaxTracks:          100,
		MaxMisses:          3,
		HitsToConfirm:      3,
		SensorBoundary:     DefaultSensorBoundary(),
	}
}

// VelocityCoherentTracker orchestrates velocity-coherent foreground extraction.
// It combines all 5 phases: velocity estimation, 6D clustering, long-tail management,
// sparse continuation, and fragment merging.
type VelocityCoherentTracker struct {
	Config VelocityCoherentTrackerConfig

	// Components
	velocityEstimator *VelocityEstimator
	longTailManager   *LongTailManager

	// Track state
	tracks      map[string]*VelocityCoherentTrack
	nextTrackID int64

	// Timing
	lastUpdateNanos int64

	// Fragment detection for merging
	completedTracks []*VelocityCoherentTrack

	mu sync.RWMutex
}

// NewVelocityCoherentTracker creates a new VC tracker with the given configuration.
func NewVelocityCoherentTracker(config VelocityCoherentTrackerConfig) *VelocityCoherentTracker {
	return &VelocityCoherentTracker{
		Config:            config,
		velocityEstimator: NewVelocityEstimator(config.VelocityEstimation, 10),
		longTailManager:   NewLongTailManager(config.PreTail, config.PostTail),
		tracks:            make(map[string]*VelocityCoherentTrack),
		nextTrackID:       1,
		completedTracks:   make([]*VelocityCoherentTrack, 0),
	}
}

// Update processes a new frame of world points through all VC phases.
// This is the main entry point for the VC tracking pipeline.
func (t *VelocityCoherentTracker) Update(worldPoints []WorldPoint, timestamp time.Time, sensorID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	nowNanos := timestamp.UnixNano()

	// Compute dt
	var dt float64
	if t.lastUpdateNanos > 0 {
		dt = float64(nowNanos-t.lastUpdateNanos) / 1e9
	} else {
		dt = 0.1 // Default 100ms
	}
	t.lastUpdateNanos = nowNanos

	// Phase 1: Velocity Estimation
	vcPoints := t.velocityEstimator.EstimateVelocities(worldPoints, timestamp, sensorID)
	if len(vcPoints) == 0 {
		t.handleMissingFrame(nowNanos, dt)
		return
	}

	// Phase 2: 6D Clustering
	clusters := DBSCAN6D(vcPoints, t.Config.Clustering)
	if len(clusters) == 0 {
		t.handleMissingFrame(nowNanos, dt)
		return
	}

	// Phase 3: Long-tail management - update predictions for post-tail tracks
	t.longTailManager.UpdatePredictions(t.tracks, timestamp)

	// Phase 3: Long-tail management - try to recover post-tail tracks
	recoveredAssociations := make(map[int64]string) // clusterID -> trackID
	for _, cluster := range clusters {
		if assoc := t.longTailManager.TryRecoverTrack(cluster); assoc != nil {
			recoveredAssociations[cluster.ClusterID] = assoc.TrackID
		}
	}

	// Associate clusters to tracks
	associations := t.associateClusters(clusters)

	// Merge recovered associations
	for clusterID, trackID := range recoveredAssociations {
		if _, exists := associations[clusterID]; !exists {
			associations[clusterID] = trackID
		}
	}

	// Update matched tracks and create new tracks
	matchedTracks := make(map[string]bool)
	for _, cluster := range clusters {
		trackID, matched := associations[cluster.ClusterID]
		if matched {
			// Update existing track
			track := t.tracks[trackID]
			t.updateTrack(track, cluster, nowNanos)
			matchedTracks[trackID] = true

			// Handle track recovered from post-tail
			if track.State == TrackPostTail {
				track.State = TrackConfirmVC
				track.Misses = 0
			}

			// Phase 4: Sparse continuation - validate sparse tracks
			if cluster.PointCount < 12 {
				track.SparseFrameCount++
				if cluster.PointCount < track.MinPointsObserved || track.MinPointsObserved == 0 {
					track.MinPointsObserved = cluster.PointCount
				}

				// Check if sparse track is still valid
				if track.State == TrackConfirmVC {
					valid, _ := IsSparseTrackValid(cluster, track, t.Config.SparseContinuation)
					if !valid {
						track.Misses++
					}
				}
			}

			// Promote tentative → confirmed
			if track.State == TrackTentVC && track.Hits >= t.Config.HitsToConfirm {
				track.State = TrackConfirmVC
			}
		} else {
			// Create new track
			t.createTrack(cluster, nowNanos, sensorID)
		}
	}

	// Handle unmatched tracks
	for trackID, track := range t.tracks {
		if matchedTracks[trackID] {
			continue
		}

		track.Misses++

		switch track.State {
		case TrackTentVC:
			// Tentative tracks are immediately deleted on miss
			track.State = TrackDeletedVC
		case TrackConfirmVC:
			// Confirmed tracks move to post-tail after MaxMisses
			if track.Misses >= t.Config.MaxMisses {
				track.State = TrackPostTail
			}
		case TrackPostTail:
			// Post-tail tracks are deleted after MaxPredictionFrames
			framesSinceLast := int((nowNanos - track.LastUnixNanos) / 100_000_000)
			if framesSinceLast >= t.Config.PostTail.MaxPredictionFrames {
				track.State = TrackDeletedVC
				t.completedTracks = append(t.completedTracks, track)
			}
		}
	}

	// Cleanup deleted tracks
	t.cleanupDeletedTracks()

	// Phase 5: Fragment merging (periodic, not every frame)
	if len(t.completedTracks) >= 2 {
		t.attemptFragmentMerging()
	}
}

// handleMissingFrame processes a frame with no clusters (all tracks miss).
func (t *VelocityCoherentTracker) handleMissingFrame(nowNanos int64, dt float64) {
	for _, track := range t.tracks {
		track.Misses++

		switch track.State {
		case TrackTentVC:
			track.State = TrackDeletedVC
		case TrackConfirmVC:
			if track.Misses >= t.Config.MaxMisses {
				track.State = TrackPostTail
			}
		case TrackPostTail:
			framesSinceLast := int((nowNanos - track.LastUnixNanos) / 100_000_000)
			if framesSinceLast >= t.Config.PostTail.MaxPredictionFrames {
				track.State = TrackDeletedVC
				t.completedTracks = append(t.completedTracks, track)
			}
		}
	}

	t.cleanupDeletedTracks()
}

// associateClusters matches clusters to existing tracks using gating.
func (t *VelocityCoherentTracker) associateClusters(clusters []VelocityCoherentCluster) map[int64]string {
	associations := make(map[int64]string)

	if len(t.tracks) == 0 {
		return associations
	}

	// Simple greedy association based on position + velocity distance
	for _, cluster := range clusters {
		bestTrackID := ""
		bestScore := float64(math.MaxFloat64)

		for trackID, track := range t.tracks {
			if track.State == TrackDeletedVC {
				continue
			}

			// Compute weighted distance (position + velocity)
			posDist := math.Sqrt(
				math.Pow(float64(cluster.CentroidX)-float64(track.X), 2) +
					math.Pow(float64(cluster.CentroidY)-float64(track.Y), 2),
			)

			velDist := math.Sqrt(
				math.Pow(cluster.VelocityX-float64(track.VX), 2) +
					math.Pow(cluster.VelocityY-float64(track.VY), 2),
			)

			// Use adaptive tolerances based on track state
			velTol, spatialTol := AdaptiveTolerances(cluster.PointCount)
			if velTol == 0 || spatialTol == 0 {
				velTol, spatialTol = 2.0, 1.0 // Fallback
			}

			// Gating check
			if posDist > spatialTol*5 || velDist > velTol*3 {
				continue
			}

			// Combined score
			score := posDist + velDist*2.0 // Weight velocity more

			if score < bestScore {
				bestScore = score
				bestTrackID = trackID
			}
		}

		if bestTrackID != "" {
			associations[cluster.ClusterID] = bestTrackID
		}
	}

	return associations
}

// createTrack creates a new track from a cluster.
func (t *VelocityCoherentTracker) createTrack(cluster VelocityCoherentCluster, nowNanos int64, sensorID string) {
	if len(t.tracks) >= t.Config.MaxTracks {
		return // At capacity
	}

	trackID := fmt.Sprintf("vc-%s-%d", sensorID, t.nextTrackID)
	t.nextTrackID++

	speed := float32(math.Sqrt(cluster.VelocityX*cluster.VelocityX + cluster.VelocityY*cluster.VelocityY))

	track := &VelocityCoherentTrack{
		TrackID:              trackID,
		SensorID:             sensorID,
		State:                TrackTentVC,
		Hits:                 1,
		Misses:               0,
		FirstUnixNanos:       nowNanos,
		LastUnixNanos:        nowNanos,
		X:                    float32(cluster.CentroidX),
		Y:                    float32(cluster.CentroidY),
		VX:                   float32(cluster.VelocityX),
		VY:                   float32(cluster.VelocityY),
		VelocityConfidence:   cluster.VelocityConfidence,
		VelocityConsistency:  1.0,
		MinPointsObserved:    cluster.PointCount,
		SparseFrameCount:     0,
		ObservationCount:     1,
		BoundingBoxLengthAvg: cluster.BoundingBoxLength,
		BoundingBoxWidthAvg:  cluster.BoundingBoxWidth,
		BoundingBoxHeightAvg: cluster.BoundingBoxHeight,
		HeightP95Max:         cluster.HeightP95,
		IntensityMeanAvg:     cluster.IntensityMean,
		AvgSpeedMps:          speed,
		PeakSpeedMps:         speed,
		History: []TrackPoint{{
			X:         float32(cluster.CentroidX),
			Y:         float32(cluster.CentroidY),
			Timestamp: nowNanos,
		}},
	}

	// Check if this might be a pre-tail detection (sparse entry)
	if cluster.PointCount < 12 && cluster.PointCount >= t.Config.SparseContinuation.MinPointsAbsolute {
		track.State = TrackPreTail
		track.SparseFrameCount = 1
	}

	t.tracks[trackID] = track
}

// updateTrack updates an existing track with a new cluster observation.
func (t *VelocityCoherentTracker) updateTrack(track *VelocityCoherentTrack, cluster VelocityCoherentCluster, nowNanos int64) {
	track.Hits++
	track.Misses = 0
	track.LastUnixNanos = nowNanos
	track.ObservationCount++

	// Update position with exponential smoothing
	alpha := float32(0.3)
	track.X = alpha*float32(cluster.CentroidX) + (1-alpha)*track.X
	track.Y = alpha*float32(cluster.CentroidY) + (1-alpha)*track.Y

	// Update velocity with exponential smoothing
	track.VX = alpha*float32(cluster.VelocityX) + (1-alpha)*track.VX
	track.VY = alpha*float32(cluster.VelocityY) + (1-alpha)*track.VY

	// Update velocity confidence
	track.VelocityConfidence = alpha*cluster.VelocityConfidence + (1-alpha)*track.VelocityConfidence

	// Update velocity consistency
	velDiff := math.Sqrt(
		math.Pow(cluster.VelocityX-float64(track.VX), 2) +
			math.Pow(cluster.VelocityY-float64(track.VY), 2),
	)
	consistency := float32(math.Max(0, 1.0-velDiff/5.0))
	track.VelocityConsistency = alpha*consistency + (1-alpha)*track.VelocityConsistency

	// Update aggregated features
	count := float32(track.ObservationCount)
	track.BoundingBoxLengthAvg = (track.BoundingBoxLengthAvg*(count-1) + cluster.BoundingBoxLength) / count
	track.BoundingBoxWidthAvg = (track.BoundingBoxWidthAvg*(count-1) + cluster.BoundingBoxWidth) / count
	track.BoundingBoxHeightAvg = (track.BoundingBoxHeightAvg*(count-1) + cluster.BoundingBoxHeight) / count
	track.IntensityMeanAvg = (track.IntensityMeanAvg*(count-1) + cluster.IntensityMean) / count

	if cluster.HeightP95 > track.HeightP95Max {
		track.HeightP95Max = cluster.HeightP95
	}

	// Update speed statistics
	speed := float32(math.Sqrt(cluster.VelocityX*cluster.VelocityX + cluster.VelocityY*cluster.VelocityY))
	track.AvgSpeedMps = (track.AvgSpeedMps*(count-1) + speed) / count
	if speed > track.PeakSpeedMps {
		track.PeakSpeedMps = speed
	}

	// Append to history
	track.History = append(track.History, TrackPoint{
		X:         float32(cluster.CentroidX),
		Y:         float32(cluster.CentroidY),
		Timestamp: nowNanos,
	})

	// Limit history length
	const maxHistory = 100
	if len(track.History) > maxHistory {
		track.History = track.History[len(track.History)-maxHistory:]
	}
}

// cleanupDeletedTracks removes deleted tracks from the active map.
func (t *VelocityCoherentTracker) cleanupDeletedTracks() {
	for trackID, track := range t.tracks {
		if track.State == TrackDeletedVC {
			delete(t.tracks, trackID)
		}
	}
}

// attemptFragmentMerging tries to merge track fragments.
func (t *VelocityCoherentTracker) attemptFragmentMerging() {
	if len(t.completedTracks) < 2 {
		return
	}

	// Create a fragment merger
	merger := NewFragmentMerger(t.Config.Merge, t.Config.SensorBoundary)

	// Detect fragments from completed tracks
	fragments := merger.DetectFragments(t.completedTracks)
	if len(fragments) < 2 {
		return
	}

	// Find merge candidates
	candidates := merger.FindMergeCandidates(fragments)

	// Apply merges (highest confidence first)
	merged := make(map[string]bool)
	for _, candidate := range candidates {
		if merged[candidate.Earlier.Track.TrackID] || merged[candidate.Later.Track.TrackID] {
			continue
		}

		// Perform merge
		mergedTrack := merger.MergeFragments(
			candidate.Earlier,
			candidate.Later,
			candidate.GapSeconds,
		)

		// Mark as merged
		merged[candidate.Earlier.Track.TrackID] = true
		merged[candidate.Later.Track.TrackID] = true

		// Replace in completed tracks
		newCompleted := make([]*VelocityCoherentTrack, 0, len(t.completedTracks))
		for _, track := range t.completedTracks {
			if track.TrackID != candidate.Earlier.Track.TrackID &&
				track.TrackID != candidate.Later.Track.TrackID {
				newCompleted = append(newCompleted, track)
			}
		}
		newCompleted = append(newCompleted, mergedTrack)
		t.completedTracks = newCompleted
	}
}

// GetActiveTracks returns a copy of currently active tracks.
func (t *VelocityCoherentTracker) GetActiveTracks() []*VelocityCoherentTrack {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*VelocityCoherentTrack, 0, len(t.tracks))
	for _, track := range t.tracks {
		if track.State != TrackDeletedVC {
			result = append(result, track)
		}
	}
	return result
}

// GetConfirmedTracks returns confirmed tracks only.
func (t *VelocityCoherentTracker) GetConfirmedTracks() []*VelocityCoherentTrack {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*VelocityCoherentTrack, 0)
	for _, track := range t.tracks {
		if track.State == TrackConfirmVC {
			result = append(result, track)
		}
	}
	return result
}

// GetCompletedTracks returns tracks that have completed their lifecycle.
func (t *VelocityCoherentTracker) GetCompletedTracks() []*VelocityCoherentTrack {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*VelocityCoherentTrack, len(t.completedTracks))
	copy(result, t.completedTracks)
	return result
}

// ClearCompletedTracks clears the completed tracks buffer.
func (t *VelocityCoherentTracker) ClearCompletedTracks() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.completedTracks = t.completedTracks[:0]
}

// GetTrackByID returns a track by its ID.
func (t *VelocityCoherentTracker) GetTrackByID(trackID string) *VelocityCoherentTrack {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.tracks[trackID]
}

// TrackCount returns the number of active tracks.
func (t *VelocityCoherentTracker) TrackCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.tracks)
}

// Reset clears all tracks and resets the tracker state.
func (t *VelocityCoherentTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.tracks = make(map[string]*VelocityCoherentTrack)
	t.nextTrackID = 1
	t.lastUpdateNanos = 0
	t.completedTracks = t.completedTracks[:0]
	t.velocityEstimator = NewVelocityEstimator(t.Config.VelocityEstimation, 10)
	t.longTailManager = NewLongTailManager(t.Config.PreTail, t.Config.PostTail)
}

// UpdateConfig updates the tracker configuration. Thread-safe.
func (t *VelocityCoherentTracker) UpdateConfig(config VelocityCoherentTrackerConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Config = config
	// Note: We don't reset the estimator/manager to preserve state
}

// GetConfig returns a copy of the current configuration.
func (t *VelocityCoherentTracker) GetConfig() VelocityCoherentTrackerConfig {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Config
}
