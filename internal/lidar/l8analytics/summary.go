package l8analytics

import (
	"encoding/json"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

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
func ComputeRunStatistics(tracks []*l5tracks.TrackedObject) *RunStatistics {
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
			className = "dynamic"
		}
		stats.ClassCounts[className]++
		stats.ClassConfidenceAvg[className] += track.ObjectConfidence

		// Lifecycle counts
		switch track.State {
		case l5tracks.TrackTentative:
			tentativeCount++
		case l5tracks.TrackConfirmed:
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
	unknownCount := stats.ClassCounts["dynamic"]
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

// ToJSON serialises RunStatistics to JSON for database storage.
func (rs *RunStatistics) ToJSON() (string, error) {
	data, err := json.Marshal(rs)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ParseRunStatistics deserialises RunStatistics from JSON.
func ParseRunStatistics(jsonStr string) (*RunStatistics, error) {
	var stats RunStatistics
	if err := json.Unmarshal([]byte(jsonStr), &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}
