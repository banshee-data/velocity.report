package l8analytics

import "github.com/banshee-data/velocity.report/internal/lidar/l5tracks"

// TrackClassSummary contains summary statistics for a single object class.
type TrackClassSummary struct {
	Count       int     `json:"count"`
	AvgSpeedMps float32 `json:"avg_speed_mps"`
	MaxSpeedMps float32 `json:"max_speed_mps"`
	AvgDuration float64 `json:"avg_duration_seconds"`
}

// TrackOverallSummary contains overall summary statistics across all tracks.
type TrackOverallSummary struct {
	TotalTracks    int     `json:"total_tracks"`
	ConfirmedCount int     `json:"confirmed_count"`
	TentativeCount int     `json:"tentative_count"`
	DeletedCount   int     `json:"deleted_count"`
	AvgSpeedMps    float32 `json:"avg_speed_mps"`
}

// TrackSummaryResult holds the computed summary, ready for the transport
// layer to serialise.
type TrackSummaryResult struct {
	ByClass map[string]TrackClassSummary `json:"by_class"`
	ByState map[string]int               `json:"by_state"`
	Overall TrackOverallSummary          `json:"overall"`
}

// ComputeTrackSummary aggregates class-level and overall statistics from a
// slice of tracked objects. The computation is transport-independent.
func ComputeTrackSummary(tracks []*l5tracks.TrackedObject) *TrackSummaryResult {
	type accum struct {
		count         int
		totalSpeed    float32
		maxSpeed      float32
		totalDuration float64
	}

	byClass := make(map[string]*accum)
	byState := make(map[string]int)
	var totalSpeed float32
	var speedCount int

	for _, track := range tracks {
		byState[string(track.TrackState)]++

		class := track.ObjectClass
		if class == "" {
			class = "unclassified"
		}
		if _, ok := byClass[class]; !ok {
			byClass[class] = &accum{}
		}
		a := byClass[class]
		a.count++
		a.totalSpeed += track.AvgSpeedMps
		if track.MaxSpeedMps > a.maxSpeed {
			a.maxSpeed = track.MaxSpeedMps
		}
		if track.EndUnixNanos > 0 && track.StartUnixNanos > 0 {
			duration := float64(track.EndUnixNanos-track.StartUnixNanos) / 1e9
			a.totalDuration += duration
		}

		totalSpeed += track.AvgSpeedMps
		speedCount++
	}

	result := &TrackSummaryResult{
		ByClass: make(map[string]TrackClassSummary),
		ByState: byState,
	}

	for class, a := range byClass {
		var avgSpeed float32
		var avgDuration float64
		if a.count > 0 {
			avgSpeed = a.totalSpeed / float32(a.count)
			avgDuration = a.totalDuration / float64(a.count)
		}
		result.ByClass[class] = TrackClassSummary{
			Count:       a.count,
			AvgSpeedMps: avgSpeed,
			MaxSpeedMps: a.maxSpeed,
			AvgDuration: avgDuration,
		}
	}

	var overallAvgSpeed float32
	if speedCount > 0 {
		overallAvgSpeed = totalSpeed / float32(speedCount)
	}

	result.Overall = TrackOverallSummary{
		TotalTracks:    len(tracks),
		ConfirmedCount: byState["confirmed"],
		TentativeCount: byState["tentative"],
		DeletedCount:   byState["deleted"],
		AvgSpeedMps:    overallAvgSpeed,
	}

	return result
}

// ComputeLabellingProgress calculates the labelling progress percentage.
func ComputeLabellingProgress(total, labelled int) float64 {
	if total <= 0 {
		return 0.0
	}
	return float64(labelled) / float64(total) * 100.0
}
