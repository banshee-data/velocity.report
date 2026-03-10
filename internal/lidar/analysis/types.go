package analysis

// ---------------------------------------------------------------------------
// Report types
// ---------------------------------------------------------------------------

// AnalysisReport is the top-level analysis output for a single .vrlog.
type AnalysisReport struct {
	Version     string `json:"version"`
	GeneratedAt string `json:"generated_at"`
	ToolVersion string `json:"tool_version"`
	Source      string `json:"source"`

	Recording                  RecordingMeta         `json:"recording"`
	FrameSummary               FrameSummary          `json:"frame_summary"`
	TrackSummary               TrackSummary          `json:"track_summary"`
	Tracks                     []TrackDetail         `json:"tracks"`
	SpeedHistogram             SpeedHistogram        `json:"speed_histogram"`
	ClassificationDistribution map[string]ClassStats `json:"classification_distribution"`
}

// RecordingMeta is §2 in the spec.
type RecordingMeta struct {
	SensorID        string  `json:"sensor_id"`
	TotalFrames     uint64  `json:"total_frames"`
	StartNs         int64   `json:"start_ns"`
	EndNs           int64   `json:"end_ns"`
	DurationSecs    float64 `json:"duration_secs"`
	CoordinateFrame string  `json:"coordinate_frame"`
}

// FrameSummary is §3 in the spec.
type FrameSummary struct {
	TotalFrames                 int        `json:"total_frames"`
	FramesWithTracks            int        `json:"frames_with_tracks"`
	FramesWithClusters          int        `json:"frames_with_clusters"`
	AvgPointsPerFrame           float64    `json:"avg_points_per_frame"`
	AvgForegroundPointsPerFrame float64    `json:"avg_foreground_points_per_frame"`
	ForegroundPct               float64    `json:"foreground_pct"`
	AvgClustersPerFrame         float64    `json:"avg_clusters_per_frame"`
	AvgTracksPerFrame           float64    `json:"avg_tracks_per_frame"`
	FrameIntervalMs             *DistStats `json:"frame_interval_ms,omitempty"`
}

// TrackSummary is §4 in the spec.
type TrackSummary struct {
	TotalTracks        int               `json:"total_tracks"`
	ConfirmedTracks    int               `json:"confirmed_tracks"`
	TentativeTracks    int               `json:"tentative_tracks"`
	DeletedTracks      int               `json:"deleted_tracks"`
	FragmentationRatio float64           `json:"fragmentation_ratio"`
	ObservationCount   *DistStats        `json:"observation_count,omitempty"`
	TrackDurationSecs  *DistStats        `json:"track_duration_secs,omitempty"`
	TrackLengthMetres  *DistStats        `json:"track_length_metres,omitempty"`
	Occlusion          *OcclusionSummary `json:"occlusion"`
}

// OcclusionSummary captures aggregate occlusion metrics.
type OcclusionSummary struct {
	MeanOcclusionCount float64 `json:"mean_occlusion_count"`
	MaxOcclusionCount  int     `json:"max_occlusion_count"`
	TotalOcclusions    int     `json:"total_occlusions"`
}

// TrackDetail is §5 in the spec — one entry per track.
type TrackDetail struct {
	TrackID         string  `json:"track_id"`
	State           string  `json:"state"`
	ObjectClass     string  `json:"object_class"`
	ClassConfidence float32 `json:"class_confidence"`

	ObservationCount int     `json:"observation_count"`
	Hits             int     `json:"hits"`
	Misses           int     `json:"misses"`
	FirstSeenNs      int64   `json:"first_seen_ns"`
	LastSeenNs       int64   `json:"last_seen_ns"`
	DurationSecs     float64 `json:"duration_secs"`

	AvgSpeedMps  float32   `json:"avg_speed_mps"`
	PeakSpeedMps float32   `json:"peak_speed_mps"`
	SpeedSamples []float32 `json:"speed_samples,omitempty"`

	StartX            float32 `json:"start_x"`
	StartY            float32 `json:"start_y"`
	EndX              float32 `json:"end_x"`
	EndY              float32 `json:"end_y"`
	TrackLengthMetres float32 `json:"track_length_metres"`

	AvgBBox      BBoxDims `json:"avg_bbox"`
	HeightP95Max float32  `json:"height_p95_max"`

	OcclusionCount int     `json:"occlusion_count"`
	MotionModel    string  `json:"motion_model"`
	Confidence     float32 `json:"confidence"`
}

// BBoxDims captures averaged bounding box dimensions.
type BBoxDims struct {
	Length float32 `json:"length"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

// SpeedHistogram is §6 in the spec.
type SpeedHistogram struct {
	BinWidthMps float64        `json:"bin_width_mps"`
	Bins        []HistogramBin `json:"bins"`
	Percentiles *DistStats     `json:"percentiles"`
	TotalTracks int            `json:"total_tracks"`
}

// HistogramBin is a single bin in the speed histogram.
type HistogramBin struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
	Count int     `json:"count"`
}

// ClassStats is §7 in the spec — per-class aggregates.
type ClassStats struct {
	Count           int     `json:"count"`
	AvgSpeedMps     float64 `json:"avg_speed_mps"`
	AvgDurationSecs float64 `json:"avg_duration_secs"`
	AvgObservations float64 `json:"avg_observations"`
}

// DistStats captures min/max/avg/p50/p85/p98 for a distribution.
type DistStats struct {
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Avg     float64 `json:"avg"`
	P50     float64 `json:"p50"`
	P85     float64 `json:"p85"`
	P98     float64 `json:"p98"`
	Samples int     `json:"samples"`
}

// ---------------------------------------------------------------------------
// Comparison types (§8)
// ---------------------------------------------------------------------------

// ComparisonReport is the two-file comparison output.
type ComparisonReport struct {
	Version     string `json:"version"`
	GeneratedAt string `json:"generated_at"`
	RunA        string `json:"run_a"`
	RunB        string `json:"run_b"`

	FrameOverlap  FrameOverlap  `json:"frame_overlap"`
	TrackMatching TrackMatching `json:"track_matching"`
	SpeedDelta    SpeedDelta    `json:"speed_delta"`
	QualityDelta  QualityDelta  `json:"quality_delta"`
}

// FrameOverlap is §8.2.
type FrameOverlap struct {
	AFrames          int     `json:"a_frames"`
	BFrames          int     `json:"b_frames"`
	TemporalOverlapS float64 `json:"temporal_overlap_secs"`
	TemporalUnionS   float64 `json:"temporal_union_secs"`
	TemporalIoU      float64 `json:"temporal_iou"`
}

// TrackMatching is §8.3.
type TrackMatching struct {
	ATotalTracks int         `json:"a_total_tracks"`
	BTotalTracks int         `json:"b_total_tracks"`
	MatchedPairs int         `json:"matched_pairs"`
	AOnlyTracks  int         `json:"a_only_tracks"`
	BOnlyTracks  int         `json:"b_only_tracks"`
	Matches      []MatchPair `json:"matches"`
}

// MatchPair is a single matched track pair.
type MatchPair struct {
	ATrackID         string  `json:"a_track_id"`
	BTrackID         string  `json:"b_track_id"`
	TemporalIoU      float64 `json:"temporal_iou"`
	SpeedDeltaMps    float64 `json:"speed_delta_mps"`
	ObservationRatio float64 `json:"observation_ratio"`
	ClassMatch       bool    `json:"class_match"`
}

// SpeedDelta is §8.4.
type SpeedDelta struct {
	MeanAbsSpeedDeltaMps float64 `json:"mean_abs_speed_delta_mps"`
	MaxAbsSpeedDeltaMps  float64 `json:"max_abs_speed_delta_mps"`
	SpeedCorrelation     float64 `json:"speed_correlation"`
}

// QualityDelta is §8.5.
type QualityDelta struct {
	FragmentationRatio DeltaPair `json:"fragmentation_ratio"`
	MeanObservations   DeltaPair `json:"mean_observations"`
	MeanOcclusionCount DeltaPair `json:"mean_occlusion_count"`
}

// DeltaPair shows a and b values with their difference.
type DeltaPair struct {
	A     float64 `json:"a"`
	B     float64 `json:"b"`
	Delta float64 `json:"delta"`
}
