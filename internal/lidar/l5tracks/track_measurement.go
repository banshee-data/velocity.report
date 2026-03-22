package l5tracks

// TrackMeasurement contains the measurement fields shared between
// TrackedObject (live L5 tracks) and RunTrack (L8 analysis snapshots).
//
// Two database tables — lidar_tracks (transient, live) and lidar_run_tracks
// (permanent, per-analysis-run) — store these same 15 columns. The
// duplication in the schema is intentional (different lifecycles, different
// primary keys, different FK relationships). The duplication in Go was not.
// This struct eliminates the duplicate field declarations, scan loops,
// column lists, and INSERT argument builders.
type TrackMeasurement struct {
	SensorID             string     `json:"sensor_id"`
	TrackState           TrackState `json:"track_state"`
	StartUnixNanos       int64      `json:"start_unix_nanos"`
	EndUnixNanos         int64      `json:"end_unix_nanos,omitempty"`
	ObservationCount     int        `json:"observation_count"`
	AvgSpeedMps          float32    `json:"avg_speed_mps"`
	MaxSpeedMps          float32    `json:"max_speed_mps"`
	BoundingBoxLengthAvg float32    `json:"bounding_box_length_avg"`
	BoundingBoxWidthAvg  float32    `json:"bounding_box_width_avg"`
	BoundingBoxHeightAvg float32    `json:"bounding_box_height_avg"`
	HeightP95Max         float32    `json:"height_p95_max"`
	IntensityMeanAvg     float32    `json:"intensity_mean_avg"`
	ObjectClass          string     `json:"object_class,omitempty"`
	ObjectConfidence     float32    `json:"object_confidence,omitempty"`
	ClassificationModel  string     `json:"classification_model,omitempty"`
}
