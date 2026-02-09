package lidar

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Phase 3.7: Analysis Run Infrastructure
// This file implements the analysis run system that allows:
// - Versioned parameter configurations stored as JSON
// - Run comparison to detect parameter impacts
// - Track split/merge detection between runs

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

// DefaultRunParams returns default run parameters.
func DefaultRunParams() RunParams {
	return RunParams{
		Version:   "1.0",
		Timestamp: time.Now(),
		Background: BackgroundParamsExport{
			BackgroundUpdateFraction:       0.02,
			ClosenessSensitivityMultiplier: 3.0,
			SafetyMarginMeters:             0.5,
			NeighborConfirmationCount:      3,
			NoiseRelativeFraction:          0.01,
			SeedFromFirstObservation:       true,
			FreezeDurationNanos:            5e9,
		},
		Clustering: ClusteringParamsExport{
			Eps:      DefaultDBSCANEps,
			MinPts:   DefaultDBSCANMinPts,
			CellSize: DefaultDBSCANEps,
		},
		Tracking: TrackingParamsExport{
			MaxTracks:               100,
			MaxMisses:               3,
			HitsToConfirm:           5, // Require 5 consecutive hits for confirmation (matches DefaultTrackerConfig)
			GatingDistanceSquared:   25.0,
			ProcessNoisePos:         0.1,
			ProcessNoiseVel:         0.5,
			MeasurementNoise:        0.2,
			DeletedTrackGracePeriod: DefaultDeletedTrackGracePeriod,
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
		SafetyMarginMeters:             p.SafetyMarginMeters,
		NeighborConfirmationCount:      p.NeighborConfirmationCount,
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
	PeakSpeedMps         float32 `json:"peak_speed_mps"`
	P50SpeedMps          float32 `json:"p50_speed_mps,omitempty"`
	P85SpeedMps          float32 `json:"p85_speed_mps,omitempty"`
	P95SpeedMps          float32 `json:"p95_speed_mps,omitempty"`
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

	// Track quality flags
	IsSplitCandidate bool     `json:"is_split_candidate,omitempty"`
	IsMergeCandidate bool     `json:"is_merge_candidate,omitempty"`
	LinkedTrackIDs   []string `json:"linked_track_ids,omitempty"`
}

// RunTrackFromTrackedObject creates a RunTrack from a TrackedObject.
func RunTrackFromTrackedObject(runID string, t *TrackedObject) *RunTrack {
	p50, p85, p95 := ComputeSpeedPercentiles(t.speedHistory)
	return &RunTrack{
		RunID:                runID,
		TrackID:              t.TrackID,
		SensorID:             t.SensorID,
		TrackState:           string(t.State),
		StartUnixNanos:       t.FirstUnixNanos,
		EndUnixNanos:         t.LastUnixNanos,
		ObservationCount:     t.ObservationCount,
		AvgSpeedMps:          t.AvgSpeedMps,
		PeakSpeedMps:         t.PeakSpeedMps,
		P50SpeedMps:          p50,
		P85SpeedMps:          p85,
		P95SpeedMps:          p95,
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

// AnalysisRunStore provides persistence for analysis runs.
type AnalysisRunStore struct {
	db *sql.DB
}

// NewAnalysisRunStore creates a new AnalysisRunStore.
func NewAnalysisRunStore(db *sql.DB) *AnalysisRunStore {
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

// InsertRun creates a new analysis run.
func (s *AnalysisRunStore) InsertRun(run *AnalysisRun) error {
	query := `
		INSERT INTO lidar_analysis_runs (
			run_id, created_at, source_type, source_path, sensor_id,
			params_json, duration_secs, total_frames, total_clusters,
			total_tracks, confirmed_tracks, processing_time_ms,
			status, error_message, parent_run_id, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Retry on SQLITE_BUSY errors
	return retryOnBusy(func() error {
		_, err := s.db.Exec(query,
			run.RunID,
			run.CreatedAt.UnixNano(),
			run.SourceType,
			nullString(run.SourcePath),
			run.SensorID,
			string(run.ParamsJSON),
			run.DurationSecs,
			run.TotalFrames,
			run.TotalClusters,
			run.TotalTracks,
			run.ConfirmedTracks,
			run.ProcessingTimeMs,
			run.Status,
			nullString(run.ErrorMessage),
			nullString(run.ParentRunID),
			nullString(run.Notes),
		)
		if err != nil {
			return fmt.Errorf("insert analysis run: %w", err)
		}
		return nil
	})
}

// UpdateRunStatus updates the status of an analysis run.
func (s *AnalysisRunStore) UpdateRunStatus(runID, status, errorMsg string) error {
	query := `UPDATE lidar_analysis_runs SET status = ?, error_message = ? WHERE run_id = ?`
	return retryOnBusy(func() error {
		_, err := s.db.Exec(query, status, nullString(errorMsg), runID)
		if err != nil {
			return fmt.Errorf("update run status: %w", err)
		}
		return nil
	})
}

// CompleteRun marks a run as completed with final statistics.
func (s *AnalysisRunStore) CompleteRun(runID string, stats *AnalysisStats) error {
	query := `
		UPDATE lidar_analysis_runs SET
			duration_secs = ?,
			total_frames = ?,
			total_clusters = ?,
			total_tracks = ?,
			confirmed_tracks = ?,
			processing_time_ms = ?,
			status = 'completed'
		WHERE run_id = ?
	`

	// Retry on SQLITE_BUSY errors
	return retryOnBusy(func() error {
		_, err := s.db.Exec(query,
			stats.DurationSecs,
			stats.TotalFrames,
			stats.TotalClusters,
			stats.TotalTracks,
			stats.ConfirmedTracks,
			stats.ProcessingTimeMs,
			runID,
		)
		if err != nil {
			return fmt.Errorf("complete run: %w", err)
		}
		return nil
	})
}

// GetRun retrieves an analysis run by ID.
func (s *AnalysisRunStore) GetRun(runID string) (*AnalysisRun, error) {
	query := `
		SELECT run_id, created_at, source_type, source_path, sensor_id,
			params_json, duration_secs, total_frames, total_clusters,
			total_tracks, confirmed_tracks, processing_time_ms,
			status, error_message, parent_run_id, notes
		FROM lidar_analysis_runs
		WHERE run_id = ?
	`

	var run AnalysisRun
	var createdAt int64
	var sourcePath, errorMessage, parentRunID, notes sql.NullString
	var paramsJSON string

	err := s.db.QueryRow(query, runID).Scan(
		&run.RunID,
		&createdAt,
		&run.SourceType,
		&sourcePath,
		&run.SensorID,
		&paramsJSON,
		&run.DurationSecs,
		&run.TotalFrames,
		&run.TotalClusters,
		&run.TotalTracks,
		&run.ConfirmedTracks,
		&run.ProcessingTimeMs,
		&run.Status,
		&errorMessage,
		&parentRunID,
		&notes,
	)
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}

	run.CreatedAt = time.Unix(0, createdAt)
	run.ParamsJSON = json.RawMessage(paramsJSON)
	if sourcePath.Valid {
		run.SourcePath = sourcePath.String
	}
	if errorMessage.Valid {
		run.ErrorMessage = errorMessage.String
	}
	if parentRunID.Valid {
		run.ParentRunID = parentRunID.String
	}
	if notes.Valid {
		run.Notes = notes.String
	}

	return &run, nil
}

// ListRuns retrieves recent analysis runs.
func (s *AnalysisRunStore) ListRuns(limit int) ([]*AnalysisRun, error) {
	query := `
		SELECT run_id, created_at, source_type, source_path, sensor_id,
			params_json, duration_secs, total_frames, total_clusters,
			total_tracks, confirmed_tracks, processing_time_ms,
			status, error_message, parent_run_id, notes
		FROM lidar_analysis_runs
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var runs []*AnalysisRun
	for rows.Next() {
		var run AnalysisRun
		var createdAt int64
		var sourcePath, errorMessage, parentRunID, notes sql.NullString
		var paramsJSON string

		err := rows.Scan(
			&run.RunID,
			&createdAt,
			&run.SourceType,
			&sourcePath,
			&run.SensorID,
			&paramsJSON,
			&run.DurationSecs,
			&run.TotalFrames,
			&run.TotalClusters,
			&run.TotalTracks,
			&run.ConfirmedTracks,
			&run.ProcessingTimeMs,
			&run.Status,
			&errorMessage,
			&parentRunID,
			&notes,
		)
		if err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}

		run.CreatedAt = time.Unix(0, createdAt)
		run.ParamsJSON = json.RawMessage(paramsJSON)
		if sourcePath.Valid {
			run.SourcePath = sourcePath.String
		}
		if errorMessage.Valid {
			run.ErrorMessage = errorMessage.String
		}
		if parentRunID.Valid {
			run.ParentRunID = parentRunID.String
		}
		if notes.Valid {
			run.Notes = notes.String
		}

		runs = append(runs, &run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}

	return runs, nil
}

// InsertRunTrack inserts a track for an analysis run.
// Uses retry logic to handle SQLITE_BUSY errors from concurrent writes.
func (s *AnalysisRunStore) InsertRunTrack(track *RunTrack) error {
	linkedJSON := "[]"
	if len(track.LinkedTrackIDs) > 0 {
		if b, err := json.Marshal(track.LinkedTrackIDs); err == nil {
			linkedJSON = string(b)
		}
	}

	query := `
		INSERT INTO lidar_run_tracks (
			run_id, track_id, sensor_id, track_state,
			start_unix_nanos, end_unix_nanos, observation_count,
			avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg,
			object_class, object_confidence, classification_model,
			user_label, label_confidence, labeler_id, labeled_at, quality_label,
			is_split_candidate, is_merge_candidate, linked_track_ids
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	var endNanos interface{}
	if track.EndUnixNanos > 0 {
		endNanos = track.EndUnixNanos
	}

	var labeledAt interface{}
	if track.LabeledAt > 0 {
		labeledAt = track.LabeledAt
	}

	// Retry on SQLITE_BUSY errors
	return retryOnBusy(func() error {
		_, err := s.db.Exec(query,
			track.RunID,
			track.TrackID,
			track.SensorID,
			track.TrackState,
			track.StartUnixNanos,
			endNanos,
			track.ObservationCount,
			track.AvgSpeedMps,
			track.PeakSpeedMps,
			track.P50SpeedMps,
			track.P85SpeedMps,
			track.P95SpeedMps,
			track.BoundingBoxLengthAvg,
			track.BoundingBoxWidthAvg,
			track.BoundingBoxHeightAvg,
			track.HeightP95Max,
			track.IntensityMeanAvg,
			nullString(track.ObjectClass),
			nullFloat32(track.ObjectConfidence),
			nullString(track.ClassificationModel),
			nullString(track.UserLabel),
			nullFloat32(track.LabelConfidence),
			nullString(track.LabelerID),
			labeledAt,
			nullString(track.QualityLabel),
			track.IsSplitCandidate,
			track.IsMergeCandidate,
			linkedJSON,
		)
		if err != nil {
			return fmt.Errorf("insert run track: %w", err)
		}
		return nil
	})
}

// GetRunTracks retrieves all tracks for an analysis run.
func (s *AnalysisRunStore) GetRunTracks(runID string) ([]*RunTrack, error) {
	query := `
		SELECT run_id, track_id, sensor_id, track_state,
			start_unix_nanos, end_unix_nanos, observation_count,
			avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg,
			object_class, object_confidence, classification_model,
			user_label, label_confidence, labeler_id, labeled_at, quality_label,
			is_split_candidate, is_merge_candidate, linked_track_ids
		FROM lidar_run_tracks
		WHERE run_id = ?
		ORDER BY start_unix_nanos
	`

	rows, err := s.db.Query(query, runID)
	if err != nil {
		return nil, fmt.Errorf("query run tracks: %w", err)
	}
	defer rows.Close()

	var tracks []*RunTrack
	for rows.Next() {
		var track RunTrack
		var endNanos, labeledAt sql.NullInt64
		var objectClass, classModel, userLabel, labelerID, qualityLabel, linkedJSON sql.NullString
		var objConf, labelConf sql.NullFloat64

		err := rows.Scan(
			&track.RunID,
			&track.TrackID,
			&track.SensorID,
			&track.TrackState,
			&track.StartUnixNanos,
			&endNanos,
			&track.ObservationCount,
			&track.AvgSpeedMps,
			&track.PeakSpeedMps,
			&track.P50SpeedMps,
			&track.P85SpeedMps,
			&track.P95SpeedMps,
			&track.BoundingBoxLengthAvg,
			&track.BoundingBoxWidthAvg,
			&track.BoundingBoxHeightAvg,
			&track.HeightP95Max,
			&track.IntensityMeanAvg,
			&objectClass,
			&objConf,
			&classModel,
			&userLabel,
			&labelConf,
			&labelerID,
			&labeledAt,
			&qualityLabel,
			&track.IsSplitCandidate,
			&track.IsMergeCandidate,
			&linkedJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scan run track: %w", err)
		}

		if endNanos.Valid {
			track.EndUnixNanos = endNanos.Int64
		}
		if objectClass.Valid {
			track.ObjectClass = objectClass.String
		}
		if objConf.Valid {
			track.ObjectConfidence = float32(objConf.Float64)
		}
		if classModel.Valid {
			track.ClassificationModel = classModel.String
		}
		if userLabel.Valid {
			track.UserLabel = userLabel.String
		}
		if labelConf.Valid {
			track.LabelConfidence = float32(labelConf.Float64)
		}
		if labelerID.Valid {
			track.LabelerID = labelerID.String
		}
		if labeledAt.Valid {
			track.LabeledAt = labeledAt.Int64
		}
		if qualityLabel.Valid {
			track.QualityLabel = qualityLabel.String
		}
		if linkedJSON.Valid && linkedJSON.String != "" && linkedJSON.String != "[]" {
			json.Unmarshal([]byte(linkedJSON.String), &track.LinkedTrackIDs)
		}

		tracks = append(tracks, &track)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run tracks: %w", err)
	}

	return tracks, nil
}

// UpdateTrackLabel updates the user label and quality label for a track.
// Both userLabel and qualityLabel can be empty strings, which will be stored as NULL in the database.
// This function does NOT validate enum values - it accepts any string and stores it as-is.
// Validation of label enum values should be performed by the caller (e.g., API handlers)
// using ValidateUserLabel() and ValidateQualityLabel() from the api package.
func (s *AnalysisRunStore) UpdateTrackLabel(runID, trackID, userLabel, qualityLabel string, confidence float32, labelerID string) error {
	query := `
		UPDATE lidar_run_tracks SET
			user_label = ?,
			label_confidence = ?,
			labeler_id = ?,
			labeled_at = ?,
			quality_label = ?
		WHERE run_id = ? AND track_id = ?
	`

	_, err := s.db.Exec(query,
		nullString(userLabel),
		confidence,
		nullString(labelerID),
		time.Now().UnixNano(),
		nullString(qualityLabel),
		runID,
		trackID,
	)
	if err != nil {
		return fmt.Errorf("update track label: %w", err)
	}

	return nil
}

// UpdateTrackQualityFlags updates the split/merge flags for a track.
func (s *AnalysisRunStore) UpdateTrackQualityFlags(runID, trackID string, isSplit, isMerge bool, linkedIDs []string) error {
	linkedJSON := "[]"
	if len(linkedIDs) > 0 {
		if b, err := json.Marshal(linkedIDs); err == nil {
			linkedJSON = string(b)
		}
	}

	query := `
		UPDATE lidar_run_tracks SET
			is_split_candidate = ?,
			is_merge_candidate = ?,
			linked_track_ids = ?
		WHERE run_id = ? AND track_id = ?
	`

	_, err := s.db.Exec(query, isSplit, isMerge, linkedJSON, runID, trackID)
	if err != nil {
		return fmt.Errorf("update track quality flags: %w", err)
	}

	return nil
}

// GetLabelingProgress returns labeling statistics for a run.
func (s *AnalysisRunStore) GetLabelingProgress(runID string) (total, labeled int, byClass map[string]int, err error) {
	byClass = make(map[string]int)

	// Get total and labeled counts
	query := `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN user_label IS NOT NULL AND user_label != '' THEN 1 ELSE 0 END) as labeled
		FROM lidar_run_tracks
		WHERE run_id = ?
	`

	err = s.db.QueryRow(query, runID).Scan(&total, &labeled)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("get labeling counts: %w", err)
	}

	// Get counts by user label
	query = `
		SELECT user_label, COUNT(*) as count
		FROM lidar_run_tracks
		WHERE run_id = ? AND user_label IS NOT NULL AND user_label != ''
		GROUP BY user_label
	`

	rows, err := s.db.Query(query, runID)
	if err != nil {
		return total, labeled, nil, fmt.Errorf("get label counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var label string
		var count int
		if err := rows.Scan(&label, &count); err != nil {
			return total, labeled, nil, fmt.Errorf("scan label count: %w", err)
		}
		byClass[label] = count
	}

	return total, labeled, byClass, nil
}

// GetUnlabeledTracks returns tracks that need labeling.
func (s *AnalysisRunStore) GetUnlabeledTracks(runID string, limit int) ([]*RunTrack, error) {
	query := `
		SELECT run_id, track_id, sensor_id, track_state,
			start_unix_nanos, end_unix_nanos, observation_count,
			avg_speed_mps, peak_speed_mps, p50_speed_mps, p85_speed_mps, p95_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg,
			object_class, object_confidence, classification_model,
			user_label, label_confidence, labeler_id, labeled_at, quality_label,
			is_split_candidate, is_merge_candidate, linked_track_ids
		FROM lidar_run_tracks
		WHERE run_id = ? AND (user_label IS NULL OR user_label = '')
		ORDER BY observation_count DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, runID, limit)
	if err != nil {
		return nil, fmt.Errorf("query unlabeled tracks: %w", err)
	}
	defer rows.Close()

	var tracks []*RunTrack
	for rows.Next() {
		var track RunTrack
		var endNanos, labeledAt sql.NullInt64
		var objectClass, classModel, userLabel, labelerID, qualityLabel, linkedJSON sql.NullString
		var objConf, labelConf sql.NullFloat64

		err := rows.Scan(
			&track.RunID,
			&track.TrackID,
			&track.SensorID,
			&track.TrackState,
			&track.StartUnixNanos,
			&endNanos,
			&track.ObservationCount,
			&track.AvgSpeedMps,
			&track.PeakSpeedMps,
			&track.P50SpeedMps,
			&track.P85SpeedMps,
			&track.P95SpeedMps,
			&track.BoundingBoxLengthAvg,
			&track.BoundingBoxWidthAvg,
			&track.BoundingBoxHeightAvg,
			&track.HeightP95Max,
			&track.IntensityMeanAvg,
			&objectClass,
			&objConf,
			&classModel,
			&userLabel,
			&labelConf,
			&labelerID,
			&labeledAt,
			&qualityLabel,
			&track.IsSplitCandidate,
			&track.IsMergeCandidate,
			&linkedJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scan unlabeled track: %w", err)
		}

		if endNanos.Valid {
			track.EndUnixNanos = endNanos.Int64
		}
		if objectClass.Valid {
			track.ObjectClass = objectClass.String
		}
		if objConf.Valid {
			track.ObjectConfidence = float32(objConf.Float64)
		}
		if classModel.Valid {
			track.ClassificationModel = classModel.String
		}
		if userLabel.Valid {
			track.UserLabel = userLabel.String
		}
		if labelConf.Valid {
			track.LabelConfidence = float32(labelConf.Float64)
		}
		if labelerID.Valid {
			track.LabelerID = labelerID.String
		}
		if labeledAt.Valid {
			track.LabeledAt = labeledAt.Int64
		}
		if qualityLabel.Valid {
			track.QualityLabel = qualityLabel.String
		}
		if linkedJSON.Valid && linkedJSON.String != "" && linkedJSON.String != "[]" {
			json.Unmarshal([]byte(linkedJSON.String), &track.LinkedTrackIDs)
		}

		tracks = append(tracks, &track)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unlabeled tracks: %w", err)
	}

	return tracks, nil
}

// CompareRuns compares two analysis runs by matching their tracks using temporal IoU
// and spatial proximity. It populates RunComparison with matched tracks, split candidates,
// merge candidates, and tracks unique to each run.
func CompareRuns(store *AnalysisRunStore, run1ID, run2ID string) (*RunComparison, error) {
	// Load tracks for both runs
	run1Tracks, err := store.GetRunTracks(run1ID)
	if err != nil {
		return nil, fmt.Errorf("load run1 tracks: %w", err)
	}

	run2Tracks, err := store.GetRunTracks(run2ID)
	if err != nil {
		return nil, fmt.Errorf("load run2 tracks: %w", err)
	}

	comparison := &RunComparison{
		Run1ID: run1ID,
		Run2ID: run2ID,
	}

	// If either run is empty, return early with empty results
	if len(run1Tracks) == 0 || len(run2Tracks) == 0 {
		for _, t := range run1Tracks {
			comparison.TracksOnlyRun1 = append(comparison.TracksOnlyRun1, t.TrackID)
		}
		for _, t := range run2Tracks {
			comparison.TracksOnlyRun2 = append(comparison.TracksOnlyRun2, t.TrackID)
		}
		return comparison, nil
	}

	// Build cost matrix using temporal IoU
	// IoU > 0.3 means potential match (from design doc)
	const iouThreshold = 0.3
	const forbiddenCost = 1e18

	costMatrix := make([][]float32, len(run1Tracks))
	iouMatrix := make([][]float64, len(run1Tracks))

	for i, t1 := range run1Tracks {
		costMatrix[i] = make([]float32, len(run2Tracks))
		iouMatrix[i] = make([]float64, len(run2Tracks))

		for j, t2 := range run2Tracks {
			iou := computeTemporalIoU(t1, t2)
			iouMatrix[i][j] = iou

			if iou > iouThreshold {
				// Valid match: cost = 1.0 - IoU (lower cost is better)
				costMatrix[i][j] = float32(1.0 - iou)
			} else {
				// Forbidden match
				costMatrix[i][j] = forbiddenCost
			}
		}
	}

	// Use Hungarian algorithm for optimal bipartite matching
	assignments := HungarianAssign(costMatrix)

	// Build sets for matched tracks
	run1Matched := make(map[string]bool)
	run2Matched := make(map[string]bool)

	// Track how many run2 tracks are matched to each run1 track (for split detection)
	run1ToRun2 := make(map[string][]string)
	// Track how many run1 tracks are matched to each run2 track (for merge detection)
	run2ToRun1 := make(map[string][]string)

	// Build maps for efficient lookup
	run1TrackMap := make(map[string]*RunTrack, len(run1Tracks))
	for _, track := range run1Tracks {
		run1TrackMap[track.TrackID] = track
	}
	run2TrackMap := make(map[string]*RunTrack, len(run2Tracks))
	for _, track := range run2Tracks {
		run2TrackMap[track.TrackID] = track
	}

	// Process assignments
	for i, j := range assignments {
		if j >= 0 && j < len(run2Tracks) {
			// Check if this is a valid match (not forbidden)
			if costMatrix[i][j] < forbiddenCost {
				t1 := run1Tracks[i]
				t2 := run2Tracks[j]

				// Record the match
				run1Matched[t1.TrackID] = true
				run2Matched[t2.TrackID] = true

				run1ToRun2[t1.TrackID] = append(run1ToRun2[t1.TrackID], t2.TrackID)
				run2ToRun1[t2.TrackID] = append(run2ToRun1[t2.TrackID], t1.TrackID)

				// Add to matched tracks list
				overlapPct := float32(iouMatrix[i][j] * 100.0)
				comparison.MatchedTracks = append(comparison.MatchedTracks, TrackMatch{
					Track1ID:   t1.TrackID,
					Track2ID:   t2.TrackID,
					OverlapPct: overlapPct,
				})
			}
		}
	}

	// Detect splits: one run1 track matched to multiple run2 tracks
	// NOTE: With the current Hungarian 1:1 matching algorithm, this will never trigger
	// because each run1 track can only be matched to at most one run2 track.
	// Future enhancement: Use a different matching strategy (e.g., IoU threshold without
	// uniqueness constraint) to detect when one reference track overlaps with multiple candidates.
	for t1ID, t2IDs := range run1ToRun2 {
		if len(t2IDs) > 1 {
			// Use map for O(1) lookup
			t1 := run1TrackMap[t1ID]

			split := TrackSplit{
				OriginalTrack: t1ID,
				SplitTracks:   t2IDs,
				Confidence:    0.8, // High confidence for multiple matches
			}

			// Use average position of original track as split location estimate
			if t1 != nil {
				// Position not stored in RunTrack, so leave at 0,0
				// In future, could load observation data for more accurate position
				split.SplitX = 0.0
				split.SplitY = 0.0
			}

			comparison.SplitCandidates = append(comparison.SplitCandidates, split)
		}
	}

	// Detect merges: multiple run1 tracks matched to one run2 track
	// NOTE: With the current Hungarian 1:1 matching algorithm, this will never trigger
	// because each run2 track can only be matched to at most one run1 track.
	// Future enhancement: Use a different matching strategy to detect when multiple
	// reference tracks overlap with the same candidate track.
	for t2ID, t1IDs := range run2ToRun1 {
		if len(t1IDs) > 1 {
			// Use map for O(1) lookup
			t2 := run2TrackMap[t2ID]

			merge := TrackMerge{
				MergedTrack:  t2ID,
				SourceTracks: t1IDs,
				Confidence:   0.8, // High confidence for multiple matches
			}

			// Use average position of merged track as merge location estimate
			if t2 != nil {
				// Position not stored in RunTrack, so leave at 0,0
				// In future, could load observation data for more accurate position
				merge.MergeX = 0.0
				merge.MergeY = 0.0
			}

			comparison.MergeCandidates = append(comparison.MergeCandidates, merge)
		}
	}

	// Collect tracks only in run1
	for _, t := range run1Tracks {
		if !run1Matched[t.TrackID] {
			comparison.TracksOnlyRun1 = append(comparison.TracksOnlyRun1, t.TrackID)
		}
	}

	// Collect tracks only in run2
	for _, t := range run2Tracks {
		if !run2Matched[t.TrackID] {
			comparison.TracksOnlyRun2 = append(comparison.TracksOnlyRun2, t.TrackID)
		}
	}

	// Compare parameters if both runs have param data
	run1, err := store.GetRun(run1ID)
	if err == nil && len(run1.ParamsJSON) > 0 {
		run2, err := store.GetRun(run2ID)
		if err == nil && len(run2.ParamsJSON) > 0 {
			params1, err1 := ParseRunParams(run1.ParamsJSON)
			params2, err2 := ParseRunParams(run2.ParamsJSON)

			if err1 == nil && err2 == nil {
				comparison.ParamDiff = compareParams(params1, params2)
			}
		}
	}

	return comparison, nil
}

// compareParams compares two RunParams and returns a map of differences.
func compareParams(p1, p2 *RunParams) map[string]any {
	diff := make(map[string]any)

	// Compare background params
	if p1.Background != p2.Background {
		bgDiff := make(map[string]any)
		if p1.Background.BackgroundUpdateFraction != p2.Background.BackgroundUpdateFraction {
			bgDiff["background_update_fraction"] = map[string]any{
				"run1": p1.Background.BackgroundUpdateFraction,
				"run2": p2.Background.BackgroundUpdateFraction,
			}
		}
		if p1.Background.ClosenessSensitivityMultiplier != p2.Background.ClosenessSensitivityMultiplier {
			bgDiff["closeness_sensitivity_multiplier"] = map[string]any{
				"run1": p1.Background.ClosenessSensitivityMultiplier,
				"run2": p2.Background.ClosenessSensitivityMultiplier,
			}
		}
		if p1.Background.SafetyMarginMeters != p2.Background.SafetyMarginMeters {
			bgDiff["safety_margin_meters"] = map[string]any{
				"run1": p1.Background.SafetyMarginMeters,
				"run2": p2.Background.SafetyMarginMeters,
			}
		}
		if p1.Background.NeighborConfirmationCount != p2.Background.NeighborConfirmationCount {
			bgDiff["neighbor_confirmation_count"] = map[string]any{
				"run1": p1.Background.NeighborConfirmationCount,
				"run2": p2.Background.NeighborConfirmationCount,
			}
		}
		if p1.Background.NoiseRelativeFraction != p2.Background.NoiseRelativeFraction {
			bgDiff["noise_relative_fraction"] = map[string]any{
				"run1": p1.Background.NoiseRelativeFraction,
				"run2": p2.Background.NoiseRelativeFraction,
			}
		}
		if p1.Background.SeedFromFirstObservation != p2.Background.SeedFromFirstObservation {
			bgDiff["seed_from_first_observation"] = map[string]any{
				"run1": p1.Background.SeedFromFirstObservation,
				"run2": p2.Background.SeedFromFirstObservation,
			}
		}
		if len(bgDiff) > 0 {
			diff["background"] = bgDiff
		}
	}

	// Compare clustering params
	if p1.Clustering != p2.Clustering {
		clDiff := make(map[string]any)
		if p1.Clustering.Eps != p2.Clustering.Eps {
			clDiff["eps"] = map[string]any{
				"run1": p1.Clustering.Eps,
				"run2": p2.Clustering.Eps,
			}
		}
		if p1.Clustering.MinPts != p2.Clustering.MinPts {
			clDiff["min_pts"] = map[string]any{
				"run1": p1.Clustering.MinPts,
				"run2": p2.Clustering.MinPts,
			}
		}
		if len(clDiff) > 0 {
			diff["clustering"] = clDiff
		}
	}

	// Compare tracking params
	if p1.Tracking != p2.Tracking {
		trDiff := make(map[string]any)
		if p1.Tracking.MaxTracks != p2.Tracking.MaxTracks {
			trDiff["max_tracks"] = map[string]any{
				"run1": p1.Tracking.MaxTracks,
				"run2": p2.Tracking.MaxTracks,
			}
		}
		if p1.Tracking.MaxMisses != p2.Tracking.MaxMisses {
			trDiff["max_misses"] = map[string]any{
				"run1": p1.Tracking.MaxMisses,
				"run2": p2.Tracking.MaxMisses,
			}
		}
		if p1.Tracking.HitsToConfirm != p2.Tracking.HitsToConfirm {
			trDiff["hits_to_confirm"] = map[string]any{
				"run1": p1.Tracking.HitsToConfirm,
				"run2": p2.Tracking.HitsToConfirm,
			}
		}
		if p1.Tracking.GatingDistanceSquared != p2.Tracking.GatingDistanceSquared {
			trDiff["gating_distance_squared"] = map[string]any{
				"run1": p1.Tracking.GatingDistanceSquared,
				"run2": p2.Tracking.GatingDistanceSquared,
			}
		}
		if len(trDiff) > 0 {
			diff["tracking"] = trDiff
		}
	}

	return diff
}
