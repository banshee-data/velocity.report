package l5tracks

import (
	"math"
	"time"
)

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
	// Heading jitter: RMS of frame-to-frame OBB heading changes (degrees)
	HeadingJitterDeg float32 `json:"heading_jitter_deg"`
	// Speed jitter: RMS of frame-to-frame Kalman speed changes (m/s)
	SpeedJitterMps float32 `json:"speed_jitter_mps"`
	// Track fragmentation: fraction of created tracks that never confirmed [0, 1]
	FragmentationRatio float32 `json:"fragmentation_ratio"`
	// Total tracks created and confirmed since last reset
	TracksCreated   int `json:"tracks_created"`
	TracksConfirmed int `json:"tracks_confirmed"`

	// Scene-level foreground capture metrics
	// ForegroundCaptureRatio is the fraction of foreground points assigned to
	// DBSCAN clusters (and hence to tracks). Higher is better. [0, 1]
	ForegroundCaptureRatio float32 `json:"foreground_capture_ratio"`
	// UnboundedPointRatio is 1 - ForegroundCaptureRatio: fraction of foreground
	// points that are DBSCAN noise and not captured by any bounding box. [0, 1]
	UnboundedPointRatio float32 `json:"unbounded_point_ratio"`
	// EmptyBoxRatio is the fraction of active-track-frames where the track had
	// no cluster association (coasting). Lower is better. [0, 1]
	EmptyBoxRatio float32 `json:"empty_box_ratio"`

	// Occlusion aggregate metrics across active tracks
	// MeanOcclusionCount is the mean number of occlusion gaps (>200ms) per track
	MeanOcclusionCount float32 `json:"mean_occlusion_count"`
	// MaxOcclusionFrames is the longest occlusion gap (in frames) across all active tracks
	MaxOcclusionFrames int `json:"max_occlusion_frames"`
	// TotalOcclusions is the sum of OcclusionCount across all active tracks
	TotalOcclusions int `json:"total_occlusions"`

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

// RecordFrameStats records per-frame foreground point statistics.
// Called from the tracking pipeline after DBSCAN clustering, before Update().
func (t *Tracker) RecordFrameStats(totalForegroundPoints, clusteredPoints int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TotalForegroundPoints += int64(totalForegroundPoints)
	t.ClusteredPoints += int64(clusteredPoints)
}

// UpdateClassification writes classification results back to a live track
// under the tracker lock. This ensures the in-memory track state is updated
// atomically, preventing data races when the visualiser or other goroutines
// read track fields concurrently (task 4.3).
func (t *Tracker) UpdateClassification(trackID, objectClass string, confidence float32, model string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if track, ok := t.Tracks[trackID]; ok {
		prevClass := track.ObjectClass
		track.ObjectClass = objectClass
		track.ObjectConfidence = confidence
		track.ClassificationModel = model
		if prevClass != objectClass {
			diagf("Track classification updated: track_id=%s class=%s->%s confidence=%.2f model=%s",
				trackID, prevClass, objectClass, confidence, model)
		}
	}
}

// GetDeletedTrackGracePeriod returns the configured deleted-track grace period.
func (t *Tracker) GetDeletedTrackGracePeriod() time.Duration {
	return t.Config.DeletedTrackGracePeriod
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
// Each returned TrackedObject is a shallow copy with deep-copied slices,
// making it safe for callers to read without holding the tracker lock.
// This prevents data races between the persistence pipeline (reading fields)
// and the tracker Update() goroutine (modifying them).
func (t *Tracker) GetConfirmedTracks() []*TrackedObject {
	t.mu.RLock()
	defer t.mu.RUnlock()

	confirmed := make([]*TrackedObject, 0)
	for _, track := range t.Tracks {
		if track.State == TrackConfirmed {
			// Shallow copy the struct to snapshot scalar fields
			copied := *track
			// Deep copy slices to avoid race with concurrent Update() appends
			if len(track.History) > 0 {
				copied.History = make([]TrackPoint, len(track.History))
				copy(copied.History, track.History)
			}
			if len(track.speedHistory) > 0 {
				copied.speedHistory = make([]float32, len(track.speedHistory))
				copy(copied.speedHistory, track.speedHistory)
			}
			confirmed = append(confirmed, &copied)
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

// SpeedHistory returns a copy of the track's speed history for classification
// and jitter/variance analysis. Speed percentiles are never computed per-track;
// they are aggregate-only (see speed-percentile-aggregation-alignment-plan.md).
func (track *TrackedObject) SpeedHistory() []float32 {
	if track.speedHistory == nil {
		return nil
	}
	result := make([]float32, len(track.speedHistory))
	copy(result, track.speedHistory)
	return result
}

// ComputeQualityMetrics calculates track quality metrics.
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
	var totalJitterSumSq float64
	var totalJitterCount int
	var totalSpeedJitterSumSq float64
	var totalSpeedJitterCount int

	for _, track := range t.Tracks {
		if track.State == TrackDeleted {
			continue
		}
		metrics.ActiveTracks++

		// Accumulate heading jitter across all active tracks
		totalJitterSumSq += track.HeadingJitterSumSq
		totalJitterCount += track.HeadingJitterCount

		// Accumulate speed jitter across all active tracks
		totalSpeedJitterSumSq += track.SpeedJitterSumSq
		totalSpeedJitterCount += track.SpeedJitterCount

		// Accumulate occlusion metrics across all active tracks
		metrics.TotalOcclusions += track.OcclusionCount
		if track.MaxOcclusionFrames > metrics.MaxOcclusionFrames {
			metrics.MaxOcclusionFrames = track.MaxOcclusionFrames
		}

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

	// Heading jitter: RMS of frame-to-frame heading changes (degrees)
	if totalJitterCount > 0 {
		rmsRad := math.Sqrt(totalJitterSumSq / float64(totalJitterCount))
		metrics.HeadingJitterDeg = float32(rmsRad * 180 / math.Pi)
	}

	// Speed jitter: RMS of frame-to-frame speed changes (m/s)
	if totalSpeedJitterCount > 0 {
		metrics.SpeedJitterMps = float32(math.Sqrt(totalSpeedJitterSumSq / float64(totalSpeedJitterCount)))
	}

	// Fragmentation: fraction of created tracks that never confirmed
	metrics.TracksCreated = t.TracksCreated
	metrics.TracksConfirmed = t.TracksConfirmed
	if t.TracksCreated > 0 {
		metrics.FragmentationRatio = 1.0 - float32(t.TracksConfirmed)/float32(t.TracksCreated)
	}

	// Foreground capture: fraction of foreground points assigned to clusters
	if t.TotalForegroundPoints > 0 {
		metrics.ForegroundCaptureRatio = float32(t.ClusteredPoints) / float32(t.TotalForegroundPoints)
		metrics.UnboundedPointRatio = 1.0 - metrics.ForegroundCaptureRatio
	}

	// Empty box: fraction of active-track-frames with no cluster association
	if t.TotalBoxFrames > 0 {
		metrics.EmptyBoxRatio = float32(t.EmptyBoxFrames) / float32(t.TotalBoxFrames)
	}

	// Mean occlusion count per active track
	if metrics.ActiveTracks > 0 {
		metrics.MeanOcclusionCount = float32(metrics.TotalOcclusions) / float32(metrics.ActiveTracks)
	}

	return metrics
}
