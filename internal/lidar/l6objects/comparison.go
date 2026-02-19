package l6objects

// ComputeTemporalIoU calculates the temporal intersection-over-union for two
// time ranges. IoU = intersection / union of [startA, endA] and [startB, endB].
// All values are in nanoseconds. Returns a value in [0, 1] where 1 means
// perfect temporal alignment. Returns 0 if the ranges do not overlap.
func ComputeTemporalIoU(startA, endA, startB, endB int64) float64 {
	// Calculate intersection: max(starts) to min(ends)
	intersectionStart := startA
	if startB > intersectionStart {
		intersectionStart = startB
	}

	intersectionEnd := endA
	if endB < intersectionEnd {
		intersectionEnd = endB
	}

	// If no overlap, IoU is 0
	if intersectionStart >= intersectionEnd {
		return 0.0
	}

	intersection := float64(intersectionEnd - intersectionStart)

	// Calculate union: min(starts) to max(ends)
	unionStart := startA
	if startB < unionStart {
		unionStart = startB
	}

	unionEnd := endA
	if endB > unionEnd {
		unionEnd = endB
	}

	union := float64(unionEnd - unionStart)

	if union <= 0 {
		return 0.0
	}

	return intersection / union
}

// RunComparison shows differences between two analysis runs.
type RunComparison struct {
	Run1ID          string         `json:"run1_id"`
	Run2ID          string         `json:"run2_id"`
	ParamDiff       map[string]any `json:"param_diff,omitempty"`
	TracksOnlyRun1  []string       `json:"tracks_only_run1,omitempty"`
	TracksOnlyRun2  []string       `json:"tracks_only_run2,omitempty"`
	SplitCandidates []TrackSplit   `json:"split_candidates,omitempty"`
	MergeCandidates []TrackMerge   `json:"merge_candidates,omitempty"`
	MatchedTracks   []TrackMatch   `json:"matched_tracks,omitempty"`
}

// TrackSplit represents a suspected track split between runs.
type TrackSplit struct {
	OriginalTrack string   `json:"original_track"`
	SplitTracks   []string `json:"split_tracks"`
	SplitX        float32  `json:"split_x"`
	SplitY        float32  `json:"split_y"`
	Confidence    float32  `json:"confidence"`
}

// TrackMerge represents a suspected track merge between runs.
type TrackMerge struct {
	MergedTrack  string   `json:"merged_track"`
	SourceTracks []string `json:"source_tracks"`
	MergeX       float32  `json:"merge_x"`
	MergeY       float32  `json:"merge_y"`
	Confidence   float32  `json:"confidence"`
}

// TrackMatch represents a matched track between two runs.
type TrackMatch struct {
	Track1ID   string  `json:"track1_id"`
	Track2ID   string  `json:"track2_id"`
	OverlapPct float32 `json:"overlap_pct"`
}
