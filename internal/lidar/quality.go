package lidar

import (
	"encoding/json"
	"sort"
)

// Phase 3: Track Quality Analysis & Training Data Curation
// This module provides quality metrics for curating high-quality training datasets.
// Focus: Identifying tracks suitable for ML training and exporting them for labeling.

// RunStatistics holds aggregate statistics for an analysis run.
type RunStatistics struct {
	// Track Quality Distribution
	AvgTrackLength    float32 `json:"avg_track_length_meters"`
	MedianTrackLength float32 `json:"median_track_length_meters"`
	AvgTrackDuration  float32 `json:"avg_track_duration_secs"`
	AvgOcclusionCount float32 `json:"avg_occlusion_count"`

	// Classification Distribution
	ClassCounts        map[string]int     `json:"class_counts"`
	ClassConfidenceAvg map[string]float32 `json:"class_confidence_avg"`
	UnknownRatio       float32            `json:"unknown_ratio"`

	// Noise & Coverage
	AvgNoiseRatio      float32 `json:"avg_noise_ratio"`
	AvgSpatialCoverage float32 `json:"avg_spatial_coverage"`

	// Track Lifecycle
	TentativeRatio          float32 `json:"tentative_ratio"`
	ConfirmedRatio          float32 `json:"confirmed_ratio"`
	AvgObservationsPerTrack int     `json:"avg_observations_per_track"`
}

// ComputeRunStatistics calculates aggregate statistics from a set of tracks.
func ComputeRunStatistics(tracks []*TrackedObject) *RunStatistics {
	if len(tracks) == 0 {
		return &RunStatistics{}
	}

	stats := &RunStatistics{
		ClassCounts:        make(map[string]int),
		ClassConfidenceAvg: make(map[string]float32),
	}

	// Collect metrics for distribution calculations
	var trackLengths []float32
	var trackDurations []float32
	var totalOcclusions int
	var totalNoiseRatio float32
	var totalSpatialCoverage float32
	var totalObservations int
	var tentativeCount, confirmedCount int

	for _, track := range tracks {
		// Track quality metrics
		trackLengths = append(trackLengths, track.TrackLengthMeters)
		trackDurations = append(trackDurations, track.TrackDurationSecs)
		totalOcclusions += track.OcclusionCount
		totalNoiseRatio += track.NoisePointRatio
		totalSpatialCoverage += track.SpatialCoverage
		totalObservations += track.ObservationCount

		// Classification distribution
		className := track.ObjectClass
		if className == "" {
			className = "other"
		}
		stats.ClassCounts[className]++
		stats.ClassConfidenceAvg[className] += track.ObjectConfidence

		// Lifecycle counts
		switch track.State {
		case TrackTentative:
			tentativeCount++
		case TrackConfirmed:
			confirmedCount++
		}
	}

	n := float32(len(tracks))

	// Track quality averages
	stats.AvgOcclusionCount = float32(totalOcclusions) / n
	stats.AvgNoiseRatio = totalNoiseRatio / n
	stats.AvgSpatialCoverage = totalSpatialCoverage / n

	// Track length distribution
	if len(trackLengths) > 0 {
		var sum float32
		for _, l := range trackLengths {
			sum += l
		}
		stats.AvgTrackLength = sum / float32(len(trackLengths))

		sort.Slice(trackLengths, func(i, j int) bool { return trackLengths[i] < trackLengths[j] })
		stats.MedianTrackLength = trackLengths[len(trackLengths)/2]
	}

	// Track duration average
	if len(trackDurations) > 0 {
		var sum float32
		for _, d := range trackDurations {
			sum += d
		}
		stats.AvgTrackDuration = sum / float32(len(trackDurations))
	}

	// Classification confidence averages
	for class, count := range stats.ClassCounts {
		if count > 0 {
			stats.ClassConfidenceAvg[class] /= float32(count)
		}
	}

	// Unknown ratio
	unknownCount := stats.ClassCounts["other"]
	stats.UnknownRatio = float32(unknownCount) / n

	// Lifecycle ratios
	stats.TentativeRatio = float32(tentativeCount) / n
	stats.ConfirmedRatio = float32(confirmedCount) / n

	// Average observations per track
	if len(tracks) > 0 {
		stats.AvgObservationsPerTrack = totalObservations / len(tracks)
	}

	return stats
}

// ToJSON serializes RunStatistics to JSON for database storage.
func (rs *RunStatistics) ToJSON() (string, error) {
	data, err := json.Marshal(rs)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ParseRunStatistics deserializes RunStatistics from JSON.
func ParseRunStatistics(jsonStr string) (*RunStatistics, error) {
	var stats RunStatistics
	if err := json.Unmarshal([]byte(jsonStr), &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// TrackQualityMetrics provides per-track quality assessment.
type TrackQualityMetrics struct {
	TrackID            string  `json:"track_id"`
	TrackLengthMeters  float32 `json:"track_length_meters"`
	TrackDurationSecs  float32 `json:"track_duration_secs"`
	OcclusionCount     int     `json:"occlusion_count"`
	MaxOcclusionFrames int     `json:"max_occlusion_frames"`
	SpatialCoverage    float32 `json:"spatial_coverage"`
	NoisePointRatio    float32 `json:"noise_point_ratio"`
	QualityScore       float32 `json:"quality_score"` // Composite quality metric (0-1)
}

// ComputeTrackQualityMetrics extracts quality metrics from a TrackedObject.
func ComputeTrackQualityMetrics(track *TrackedObject) *TrackQualityMetrics {
	metrics := &TrackQualityMetrics{
		TrackID:            track.TrackID,
		TrackLengthMeters:  track.TrackLengthMeters,
		TrackDurationSecs:  track.TrackDurationSecs,
		OcclusionCount:     track.OcclusionCount,
		MaxOcclusionFrames: track.MaxOcclusionFrames,
		SpatialCoverage:    track.SpatialCoverage,
		NoisePointRatio:    track.NoisePointRatio,
	}

	// Compute composite quality score (0=poor, 1=excellent)
	// Factors: spatial coverage (higher is better), noise ratio (lower is better),
	//          occlusion count (lower is better), track length (longer is better)
	var score float32 = 0

	// Spatial coverage contribution (0-0.3)
	score += metrics.SpatialCoverage * 0.3

	// Noise ratio contribution (0-0.3, inverted)
	noiseScore := (1.0 - metrics.NoisePointRatio) * 0.3
	if noiseScore < 0 {
		noiseScore = 0
	}
	score += noiseScore

	// Occlusion contribution (0-0.2, inverted and clamped)
	occlusionScore := float32(1.0) - (float32(metrics.OcclusionCount) / 10.0)
	if occlusionScore < 0 {
		occlusionScore = 0
	}
	score += occlusionScore * 0.2

	// Track length contribution (0-0.2, normalized to 50m max)
	lengthScore := metrics.TrackLengthMeters / 50.0
	if lengthScore > 1.0 {
		lengthScore = 1.0
	}
	score += lengthScore * 0.2

	metrics.QualityScore = score
	return metrics
}

// NoiseCoverageMetrics quantifies "unknown" classification coverage.
// Phase 3: Scaffolding for coverage analysis.
type NoiseCoverageMetrics struct {
	TotalTracks         int                `json:"total_tracks"`
	TracksWithHighNoise int                `json:"tracks_with_high_noise"`       // noise_ratio > 0.3
	TracksUnknownClass  int                `json:"tracks_unknown_class"`         // object_class == "other"
	TracksLowConfidence int                `json:"tracks_low_confidence"`        // object_confidence < 0.6
	UnknownRatioBySpeed map[string]float32 `json:"unknown_ratio_by_speed"`       // "slow"/"medium"/"fast"
	UnknownRatioBySize  map[string]float32 `json:"unknown_ratio_by_size"`        // "small"/"medium"/"large"
	NoiseRatioHistogram []int              `json:"noise_ratio_histogram_counts"` // Counts for bins [0-0.1, 0.1-0.2, ...]
}

// ComputeNoiseCoverageMetrics calculates coverage metrics for a set of tracks.
// Phase 3: Placeholder implementation.
func ComputeNoiseCoverageMetrics(tracks []*TrackedObject) *NoiseCoverageMetrics {
	metrics := &NoiseCoverageMetrics{
		TotalTracks:         len(tracks),
		UnknownRatioBySpeed: make(map[string]float32),
		UnknownRatioBySize:  make(map[string]float32),
		NoiseRatioHistogram: make([]int, 10), // 10 bins: [0-0.1, 0.1-0.2, ..., 0.9-1.0]
	}

	// TODO Phase 3: Implement full noise coverage analysis
	// For now, just count high-noise and unknown tracks
	for _, track := range tracks {
		if track.NoisePointRatio > 0.3 {
			metrics.TracksWithHighNoise++
		}
		if track.ObjectClass == "" || track.ObjectClass == "other" {
			metrics.TracksUnknownClass++
		}
		if track.ObjectConfidence < 0.6 {
			metrics.TracksLowConfidence++
		}

		// Populate histogram
		bin := int(track.NoisePointRatio * 10)
		if bin >= 10 {
			bin = 9
		}
		if bin >= 0 && bin < 10 {
			metrics.NoiseRatioHistogram[bin]++
		}
	}

	return metrics
}

// TrackTrainingFilter defines criteria for selecting high-quality tracks for ML training.
type TrackTrainingFilter struct {
	MinQualityScore   float32      // Minimum composite quality score (0-1)
	MinDuration       float32      // Minimum track duration (seconds)
	MinLength         float32      // Minimum track length (meters)
	MaxOcclusionRatio float32      // Maximum occlusion ratio (occlusions / observations)
	MinObservations   int          // Minimum observation count
	RequireClass      bool         // Only include tracks with assigned class
	AllowedStates     []TrackState // Allowed track states (e.g., only confirmed)
}

// DefaultTrackTrainingFilter returns sensible defaults for high-quality training tracks.
func DefaultTrackTrainingFilter() *TrackTrainingFilter {
	return &TrackTrainingFilter{
		MinQualityScore:   0.6,                          // Good quality or better
		MinDuration:       2.0,                          // At least 2 seconds
		MinLength:         5.0,                          // At least 5 meters traveled
		MaxOcclusionRatio: 0.3,                          // Max 30% occlusions
		MinObservations:   20,                           // At least 20 frames (2s @ 10Hz)
		RequireClass:      false,                        // Include unlabeled tracks for annotation
		AllowedStates:     []TrackState{TrackConfirmed}, // Only confirmed tracks
	}
}

// FilterTracksForTraining selects tracks that meet training data quality criteria.
func FilterTracksForTraining(tracks []*TrackedObject, filter *TrackTrainingFilter) []*TrackedObject {
	filtered := make([]*TrackedObject, 0)

	for _, track := range tracks {
		// Compute quality metrics
		qualityMetrics := ComputeTrackQualityMetrics(track)

		// Check quality score
		if qualityMetrics.QualityScore < filter.MinQualityScore {
			continue
		}

		// Check duration
		if track.TrackDurationSecs < filter.MinDuration {
			continue
		}

		// Check length
		if track.TrackLengthMeters < filter.MinLength {
			continue
		}

		// Check occlusion ratio
		if track.ObservationCount > 0 {
			occlusionRatio := float32(track.OcclusionCount) / float32(track.ObservationCount)
			if occlusionRatio > filter.MaxOcclusionRatio {
				continue
			}
		}

		// Check observation count
		if track.ObservationCount < filter.MinObservations {
			continue
		}

		// Check classification requirement
		if filter.RequireClass && (track.ObjectClass == "" || track.ObjectClass == "other") {
			continue
		}

		// Check state
		if len(filter.AllowedStates) > 0 {
			stateAllowed := false
			for _, allowedState := range filter.AllowedStates {
				if track.State == allowedState {
					stateAllowed = true
					break
				}
			}
			if !stateAllowed {
				continue
			}
		}

		filtered = append(filtered, track)
	}

	return filtered
}

// TrainingDatasetSummary provides statistics about a curated training dataset.
type TrainingDatasetSummary struct {
	TotalTracks       int            `json:"total_tracks"`
	TotalFrames       int            `json:"total_frames"`
	TotalPoints       int            `json:"total_points"`
	ClassDistribution map[string]int `json:"class_distribution"`
	AvgQualityScore   float32        `json:"avg_quality_score"`
	AvgDuration       float32        `json:"avg_duration_secs"`
	AvgLength         float32        `json:"avg_length_meters"`
}

// SummarizeTrainingDataset generates statistics for a curated training dataset.
func SummarizeTrainingDataset(tracks []*TrackedObject) *TrainingDatasetSummary {
	if len(tracks) == 0 {
		return &TrainingDatasetSummary{ClassDistribution: make(map[string]int)}
	}

	summary := &TrainingDatasetSummary{
		TotalTracks:       len(tracks),
		ClassDistribution: make(map[string]int),
	}

	var totalQuality float32
	var totalDuration float32
	var totalLength float32

	for _, track := range tracks {
		summary.TotalFrames += track.ObservationCount
		// TODO: Add point count when point cloud storage is integrated

		className := track.ObjectClass
		if className == "" {
			className = "unlabeled"
		}
		summary.ClassDistribution[className]++

		qualityMetrics := ComputeTrackQualityMetrics(track)
		totalQuality += qualityMetrics.QualityScore
		totalDuration += track.TrackDurationSecs
		totalLength += track.TrackLengthMeters
	}

	n := float32(len(tracks))
	summary.AvgQualityScore = totalQuality / n
	summary.AvgDuration = totalDuration / n
	summary.AvgLength = totalLength / n

	return summary
}
