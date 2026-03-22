package sqlite

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// AnalysisRun represents a complete analysis session with parameters.
// All LIDAR parameters are stored in ParamsJSON for full reproducibility.
type AnalysisRun struct {
	RunID            string          `json:"run_id"`
	CreatedAt        time.Time       `json:"created_at"`
	SourceType       string          `json:"source_type"` // "pcap" or "live"
	SourcePath       string          `json:"source_path,omitempty"`
	SensorID         string          `json:"sensor_id"`
	ParamsJSON       json.RawMessage `json:"params_json"`
	DurationSecs     float64         `json:"duration_secs"`
	TotalFrames      int             `json:"total_frames"`
	TotalClusters    int             `json:"total_clusters"`
	TotalTracks      int             `json:"total_tracks"`
	ConfirmedTracks  int             `json:"confirmed_tracks"`
	ProcessingTimeMs int64           `json:"processing_time_ms"`
	Status           string          `json:"status"` // "running", "completed", "failed"
	ErrorMessage     string          `json:"error_message,omitempty"`
	ParentRunID      string          `json:"parent_run_id,omitempty"`
	Notes            string          `json:"notes,omitempty"`
	VRLogPath        string          `json:"vrlog_path,omitempty"` // Path to VRLOG recording for replay

	// Derived fields (not persisted in DB, computed on retrieval)
	ReplayCaseName string          `json:"replay_case_name,omitempty"` // Derived from SourcePath filename
	LabelRollup    *RunLabelRollup `json:"label_rollup,omitempty"`     // Derived from run-track labels
}

// RunLabelRollup summarises the current human labelling state for a run.
// Counts are mutually exclusive and always sum to Total.
type RunLabelRollup struct {
	Total      int `json:"total"`
	Classified int `json:"classified"`
	TaggedOnly int `json:"tagged_only"`
	Unlabelled int `json:"unlabelled"`
}

// LabelledCount returns tracks with any human-applied label state.
func (r *RunLabelRollup) LabelledCount() int {
	if r == nil {
		return 0
	}
	return r.Classified + r.TaggedOnly
}

// PopulateReplayCaseName sets ReplayCaseName from SourcePath by extracting the base
// filename without extension. E.g. "/data/kirk1.pcap" → "kirk1".
func (r *AnalysisRun) PopulateReplayCaseName() {
	if r.SourcePath != "" {
		base := filepath.Base(r.SourcePath)
		r.ReplayCaseName = strings.TrimSuffix(base, filepath.Ext(base))
	} else {
		r.ReplayCaseName = ""
	}
}

const (
	normalisedLabelSourceExpr  = "TRIM(REPLACE(REPLACE(REPLACE(COALESCE(label_source, ''), CHAR(9), ' '), CHAR(10), ' '), CHAR(13), ' '))"
	normalisedUserLabelExpr    = "TRIM(REPLACE(REPLACE(REPLACE(COALESCE(user_label, ''), CHAR(9), ' '), CHAR(10), ' '), CHAR(13), ' '))"
	normalisedQualityExpr      = "TRIM(REPLACE(REPLACE(REPLACE(COALESCE(quality_label, ''), CHAR(9), ' '), CHAR(10), ' '), CHAR(13), ' '))"
	normalisedLinkedIDsExpr    = "TRIM(REPLACE(REPLACE(REPLACE(COALESCE(linked_track_ids, ''), CHAR(9), ' '), CHAR(10), ' '), CHAR(13), ' '))"
	manualLabelSourcePredicate = "(" + normalisedLabelSourceExpr + " = '' OR " + normalisedLabelSourceExpr + " = 'human_manual')"
	manualClassPredicate       = manualLabelSourcePredicate + " AND " + normalisedUserLabelExpr + " != '' AND " + normalisedUserLabelExpr + " NOT IN ('split', 'merge')"
	manualTagPredicate         = manualLabelSourcePredicate + " AND ((" + normalisedQualityExpr + " != '') OR (" + normalisedUserLabelExpr + " IN ('split', 'merge')) OR is_split_candidate = 1 OR is_merge_candidate = 1 OR (" + normalisedLinkedIDsExpr + " != '' AND " + normalisedLinkedIDsExpr + " != '[]'))"
)

func isMissingRunTracksTableErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table: lidar_run_tracks")
}

func normaliseRunTrackString(value string) string {
	return strings.TrimSpace(value)
}

func normaliseRunTrackQualityLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parts := strings.Split(value, ",")
	normalised := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		normalised = append(normalised, part)
	}
	return strings.Join(normalised, ",")
}

func normaliseRunTrackLinkedIDs(linkedIDs []string) []string {
	if len(linkedIDs) == 0 {
		return nil
	}

	normalised := make([]string, 0, len(linkedIDs))
	for _, linkedID := range linkedIDs {
		linkedID = strings.TrimSpace(linkedID)
		if linkedID == "" {
			continue
		}
		normalised = append(normalised, linkedID)
	}
	if len(normalised) == 0 {
		return nil
	}
	return normalised
}

// RunParams captures all configurable parameters for reproducibility.
// This is the structure serialized into AnalysisRun.ParamsJSON.
type RunParams struct {
	Version        string                     `json:"version"`
	Timestamp      time.Time                  `json:"timestamp"`
	Background     BackgroundParamsExport     `json:"background"`
	Clustering     ClusteringParamsExport     `json:"clustering"`
	Tracking       TrackingParamsExport       `json:"tracking"`
	Classification ClassificationParamsExport `json:"classification,omitempty"`
}

// BackgroundParamsExport is the JSON-serializable background params.
type BackgroundParamsExport struct {
	BackgroundUpdateFraction       float32 `json:"background_update_fraction"`
	ClosenessSensitivityMultiplier float32 `json:"closeness_sensitivity_multiplier"`
	SafetyMarginMeters             float32 `json:"safety_margin_meters"`
	NeighborConfirmationCount      int     `json:"neighbor_confirmation_count"`
	NoiseRelativeFraction          float32 `json:"noise_relative_fraction"`
	SeedFromFirstObservation       bool    `json:"seed_from_first_observation"`
	FreezeDurationNanos            int64   `json:"freeze_duration_nanos"`
}

// ClusteringParamsExport is the JSON-serializable clustering params.
type ClusteringParamsExport struct {
	Eps      float64 `json:"eps"`
	MinPts   int     `json:"min_pts"`
	CellSize float64 `json:"cell_size,omitempty"`
}

// TrackingParamsExport is the JSON-serializable tracking params.
type TrackingParamsExport struct {
	MaxTracks               int           `json:"max_tracks"`
	MaxMisses               int           `json:"max_misses"`
	HitsToConfirm           int           `json:"hits_to_confirm"`
	GatingDistanceSquared   float32       `json:"gating_distance_squared"`
	ProcessNoisePos         float32       `json:"process_noise_pos"`
	ProcessNoiseVel         float32       `json:"process_noise_vel"`
	MeasurementNoise        float32       `json:"measurement_noise"`
	DeletedTrackGracePeriod time.Duration `json:"deleted_track_grace_period_nanos"`
}

// ClassificationParamsExport is the JSON-serializable classification params.
type ClassificationParamsExport struct {
	ModelType  string                 `json:"model_type"`
	Thresholds map[string]interface{} `json:"thresholds,omitempty"`
}

// DefaultRunParams returns run parameters loaded from the canonical tuning
// defaults file (config/tuning.defaults.json). Panics if the file cannot
// be found — intended for tests and tools.
func DefaultRunParams() RunParams {
	cfg := config.MustLoadDefaultConfig()
	return RunParamsFromTuning(cfg)
}

// RunParamsFromTuning builds RunParams from a loaded TuningConfig.
// Use this in production code where the TuningConfig is already loaded.
func RunParamsFromTuning(cfg *config.TuningConfig) RunParams {
	return RunParams{
		Version:   "1.0",
		Timestamp: time.Now(),
		Background: BackgroundParamsExport{
			BackgroundUpdateFraction:       float32(cfg.GetBackgroundUpdateFraction()),
			ClosenessSensitivityMultiplier: float32(cfg.GetClosenessMultiplier()),
			SafetyMarginMeters:             float32(cfg.GetSafetyMarginMetres()),
			NeighborConfirmationCount:      cfg.GetNeighbourConfirmationCount(),
			NoiseRelativeFraction:          float32(cfg.GetNoiseRelative()),
			SeedFromFirstObservation:       cfg.GetSeedFromFirst(),
			FreezeDurationNanos:            5e9,
		},
		Clustering: ClusteringParamsExport{
			Eps:      cfg.GetForegroundDBSCANEps(),
			MinPts:   cfg.GetForegroundMinClusterPoints(),
			CellSize: cfg.GetForegroundDBSCANEps(),
		},
		Tracking: TrackingParamsExport{
			MaxTracks:               cfg.GetMaxTracks(),
			MaxMisses:               cfg.GetMaxMisses(),
			HitsToConfirm:           cfg.GetHitsToConfirm(),
			GatingDistanceSquared:   float32(cfg.GetGatingDistanceSquared()),
			ProcessNoisePos:         float32(cfg.GetProcessNoisePos()),
			ProcessNoiseVel:         float32(cfg.GetProcessNoiseVel()),
			MeasurementNoise:        float32(cfg.GetMeasurementNoise()),
			DeletedTrackGracePeriod: cfg.GetDeletedTrackGracePeriod(),
		},
		Classification: ClassificationParamsExport{
			ModelType: "rule_based",
		},
	}
}

// FromBackgroundParams creates export params from BackgroundParams.
func FromBackgroundParams(p BackgroundParams) BackgroundParamsExport {
	return BackgroundParamsExport{
		BackgroundUpdateFraction:       p.BackgroundUpdateFraction,
		ClosenessSensitivityMultiplier: p.ClosenessSensitivityMultiplier,
		SafetyMarginMeters:             p.SafetyMarginMetres,
		NeighborConfirmationCount:      p.NeighbourConfirmationCount,
		NoiseRelativeFraction:          p.NoiseRelativeFraction,
		SeedFromFirstObservation:       p.SeedFromFirstObservation,
		FreezeDurationNanos:            p.FreezeDurationNanos,
	}
}

// FromDBSCANParams creates export params from DBSCANParams.
func FromDBSCANParams(p DBSCANParams) ClusteringParamsExport {
	return ClusteringParamsExport{
		Eps:      p.Eps,
		MinPts:   p.MinPts,
		CellSize: p.Eps,
	}
}

// FromTrackerConfig creates export params from TrackerConfig.
func FromTrackerConfig(c TrackerConfig) TrackingParamsExport {
	return TrackingParamsExport{
		MaxTracks:               c.MaxTracks,
		MaxMisses:               c.MaxMisses,
		HitsToConfirm:           c.HitsToConfirm,
		GatingDistanceSquared:   c.GatingDistanceSquared,
		ProcessNoisePos:         c.ProcessNoisePos,
		ProcessNoiseVel:         c.ProcessNoiseVel,
		MeasurementNoise:        c.MeasurementNoise,
		DeletedTrackGracePeriod: c.DeletedTrackGracePeriod,
	}
}

// ToJSON serializes RunParams to JSON.
func (p *RunParams) ToJSON() (json.RawMessage, error) {
	return json.Marshal(p)
}

// ParseRunParams deserializes RunParams from JSON.
func ParseRunParams(data json.RawMessage) (*RunParams, error) {
	var p RunParams
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// RunTrack represents a track within a specific analysis run.
// This extends TrackedObject with run-specific fields like user labels.
type RunTrack struct {
	RunID   string `json:"run_id"`
	TrackID string `json:"track_id"`

	// Track fields (from TrackedObject)
	SensorID             string  `json:"sensor_id"`
	TrackState           string  `json:"track_state"`
	StartUnixNanos       int64   `json:"start_unix_nanos"`
	EndUnixNanos         int64   `json:"end_unix_nanos,omitempty"`
	ObservationCount     int     `json:"observation_count"`
	AvgSpeedMps          float32 `json:"avg_speed_mps"`
	MaxSpeedMps          float32 `json:"max_speed_mps"`
	BoundingBoxLengthAvg float32 `json:"bounding_box_length_avg"`
	BoundingBoxWidthAvg  float32 `json:"bounding_box_width_avg"`
	BoundingBoxHeightAvg float32 `json:"bounding_box_height_avg"`
	HeightP95Max         float32 `json:"height_p95_max"`
	IntensityMeanAvg     float32 `json:"intensity_mean_avg"`
	ObjectClass          string  `json:"object_class,omitempty"`
	ObjectConfidence     float32 `json:"object_confidence,omitempty"`
	ClassificationModel  string  `json:"classification_model,omitempty"`

	// User labels (for ML training)
	UserLabel       string  `json:"user_label,omitempty"`
	LabelConfidence float32 `json:"label_confidence,omitempty"`
	LabelerID       string  `json:"labeler_id,omitempty"`
	LabeledAt       int64   `json:"labeled_at,omitempty"`
	QualityLabel    string  `json:"quality_label,omitempty"`
	LabelSource     string  `json:"label_source,omitempty"` // human_manual, carried_over, auto_suggested

	// Track quality flags
	IsSplitCandidate bool     `json:"is_split_candidate,omitempty"`
	IsMergeCandidate bool     `json:"is_merge_candidate,omitempty"`
	LinkedTrackIDs   []string `json:"linked_track_ids,omitempty"`
}

// RunTrackFromTrackedObject creates a RunTrack from a TrackedObject.
func RunTrackFromTrackedObject(runID string, t *TrackedObject) *RunTrack {
	return &RunTrack{
		RunID:                runID,
		TrackID:              t.TrackID,
		SensorID:             t.SensorID,
		TrackState:           string(t.State),
		StartUnixNanos:       t.FirstUnixNanos,
		EndUnixNanos:         t.LastUnixNanos,
		ObservationCount:     t.ObservationCount,
		AvgSpeedMps:          t.AvgSpeedMps,
		MaxSpeedMps:          t.MaxSpeedMps,
		BoundingBoxLengthAvg: t.BoundingBoxLengthAvg,
		BoundingBoxWidthAvg:  t.BoundingBoxWidthAvg,
		BoundingBoxHeightAvg: t.BoundingBoxHeightAvg,
		HeightP95Max:         t.HeightP95Max,
		IntensityMeanAvg:     t.IntensityMeanAvg,
		ObjectClass:          t.ObjectClass,
		ObjectConfidence:     t.ObjectConfidence,
		ClassificationModel:  t.ClassificationModel,
	}
}

// AnalysisStats holds statistics for a completed analysis run.
type AnalysisStats struct {
	DurationSecs     float64
	TotalFrames      int
	TotalClusters    int
	TotalTracks      int
	ConfirmedTracks  int
	ProcessingTimeMs int64
}

// RunComparison shows differences between two analysis runs.
// Canonical type is in l8analytics.
type RunComparison = l8analytics.RunComparison

// TrackSplit represents a suspected track split between runs.
type TrackSplit = l8analytics.TrackSplit

// TrackMerge represents a suspected track merge between runs.
type TrackMerge = l8analytics.TrackMerge

// TrackMatch represents a matched track between two runs.
type TrackMatch = l8analytics.TrackMatch

// AnalysisRunStore provides persistence for analysis runs.
type AnalysisRunStore struct {
	db DBClient
}

// NewAnalysisRunStore creates a new AnalysisRunStore.
func NewAnalysisRunStore(db DBClient) *AnalysisRunStore {
	return &AnalysisRunStore{db: db}
}

// isSQLiteBusy checks if an error is a SQLite SQLITE_BUSY error.
func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "database is locked") || strings.Contains(errStr, "SQLITE_BUSY")
}

// retryOnBusy retries a database operation with exponential backoff on SQLITE_BUSY errors.
// This handles SQLite's single-writer limitation when multiple goroutines try to write.
func retryOnBusy(operation func() error) error {
	const maxRetries = 5
	const baseDelay = 10 * time.Millisecond

	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = operation()
		if err == nil {
			return nil
		}

		if !isSQLiteBusy(err) {
			return err
		}

		if attempt < maxRetries-1 {
			delay := baseDelay * (1 << uint(attempt)) // Exponential backoff: 10ms, 20ms, 40ms, 80ms, 160ms
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", maxRetries, err)
}
