package lidar

import (
	"encoding/json"
	"sort"
)

// Phase 3: Track Quality Analysis & Introspection (Scaffolding)
// This module provides quality metrics, statistics, and introspection capabilities.

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

// TrajectoryGeoJSON exports a track trajectory as GeoJSON for web rendering.
// Phase 3: Scaffolding for trajectory visualization.
type TrajectoryGeoJSON struct {
	Type     string                 `json:"type"`
	Geometry map[string]interface{} `json:"geometry"`
	Properties map[string]interface{} `json:"properties"`
}

// ExportTrajectoryGeoJSON converts a track to GeoJSON format.
// Phase 3: Placeholder - assumes coordinates are already in appropriate reference frame.
func ExportTrajectoryGeoJSON(track *TrackedObject) *TrajectoryGeoJSON {
	// Extract coordinates from history
	coordinates := make([][2]float32, len(track.History))
	for i, point := range track.History {
		coordinates[i] = [2]float32{point.X, point.Y}
	}

	geojson := &TrajectoryGeoJSON{
		Type: "Feature",
		Geometry: map[string]interface{}{
			"type":        "LineString",
			"coordinates": coordinates,
		},
		Properties: map[string]interface{}{
			"track_id":       track.TrackID,
			"object_class":   track.ObjectClass,
			"confidence":     track.ObjectConfidence,
			"avg_speed":      track.AvgSpeedMps,
			"duration":       track.TrackDurationSecs,
			"occlusion_count": track.OcclusionCount,
			"quality_score":  ComputeTrackQualityMetrics(track).QualityScore,
		},
	}

	return geojson
}
