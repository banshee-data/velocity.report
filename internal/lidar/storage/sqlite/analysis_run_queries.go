package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// GetRun retrieves an analysis run by ID.
func (s *AnalysisRunStore) GetRun(runID string) (*AnalysisRun, error) {
	query := `
		SELECT run_id, created_at, source_type, source_path, sensor_id,
			params_json, duration_secs, total_frames, total_clusters,
			total_tracks, confirmed_tracks, processing_time_ms,
			status, error_message, parent_run_id, notes, vrlog_path
		FROM lidar_run_records
		WHERE run_id = ?
	`

	var run AnalysisRun
	var createdAt int64
	var sourcePath, errorMessage, parentRunID, notes, vrlogPath sql.NullString
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
		&vrlogPath,
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
	if vrlogPath.Valid {
		run.VRLogPath = vrlogPath.String
	}

	run.PopulateReplayCaseName()
	labelRollup, err := s.GetRunLabelRollup(runID)
	if err != nil {
		return nil, err
	}
	run.LabelRollup = labelRollup

	return &run, nil
}

// ListRuns retrieves recent analysis runs.
func (s *AnalysisRunStore) ListRuns(limit int) ([]*AnalysisRun, error) {
	query := `
		SELECT run_id, created_at, source_type, source_path, sensor_id,
			params_json, duration_secs, total_frames, total_clusters,
			total_tracks, confirmed_tracks, processing_time_ms,
			status, error_message, parent_run_id, notes, vrlog_path
		FROM lidar_run_records
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
		var sourcePath, errorMessage, parentRunID, notes, vrlogPath sql.NullString
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
			&vrlogPath,
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
		if vrlogPath.Valid {
			run.VRLogPath = vrlogPath.String
		}

		run.PopulateReplayCaseName()

		runs = append(runs, &run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}

	if err := s.populateRunLabelRollups(runs); err != nil {
		return nil, err
	}

	return runs, nil
}

// GetRunTracks retrieves all tracks for an analysis run.
func (s *AnalysisRunStore) GetRunTracks(runID string) ([]*RunTrack, error) {
	query := `
		SELECT run_id, track_id, sensor_id, track_state,
			start_unix_nanos, end_unix_nanos, observation_count,
			avg_speed_mps, max_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg,
			object_class, object_confidence, classification_model,
			user_label, label_confidence, labeler_id, labeled_at, quality_label,
			label_source,
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
		var objectClass, classModel, userLabel, labelerID, qualityLabel, labelSource, linkedJSON sql.NullString
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
			&track.MaxSpeedMps,
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
			&labelSource,
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
		if labelSource.Valid {
			track.LabelSource = labelSource.String
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

// GetRunTrack retrieves a single track for an analysis run.
func (s *AnalysisRunStore) GetRunTrack(runID, trackID string) (*RunTrack, error) {
	query := `
		SELECT run_id, track_id, sensor_id, track_state,
			start_unix_nanos, end_unix_nanos, observation_count,
			avg_speed_mps, max_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg,
			object_class, object_confidence, classification_model,
			user_label, label_confidence, labeler_id, labeled_at, quality_label,
			label_source,
			is_split_candidate, is_merge_candidate, linked_track_ids
		FROM lidar_run_tracks
		WHERE run_id = ? AND track_id = ?
	`

	var track RunTrack
	var endNanos, labeledAt sql.NullInt64
	var objectClass, classModel, userLabel, labelerID, qualityLabel, labelSource, linkedJSON sql.NullString
	var objConf, labelConf sql.NullFloat64

	err := s.db.QueryRow(query, runID, trackID).Scan(
		&track.RunID,
		&track.TrackID,
		&track.SensorID,
		&track.TrackState,
		&track.StartUnixNanos,
		&endNanos,
		&track.ObservationCount,
		&track.AvgSpeedMps,
		&track.MaxSpeedMps,
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
		&labelSource,
		&track.IsSplitCandidate,
		&track.IsMergeCandidate,
		&linkedJSON,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("track %s not found in run %s", trackID, runID)
		}
		return nil, fmt.Errorf("query run track: %w", err)
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
	if labelSource.Valid {
		track.LabelSource = labelSource.String
	}
	if linkedJSON.Valid && linkedJSON.String != "" && linkedJSON.String != "[]" {
		json.Unmarshal([]byte(linkedJSON.String), &track.LinkedTrackIDs)
	}

	return &track, nil
}

// GetLabelingProgress returns labeling statistics for a run.
func (s *AnalysisRunStore) GetLabelingProgress(runID string) (total, labeled int, byClass map[string]int, err error) {
	total, labeled, byClass, _, err = s.GetLabelingProgressWithRollup(runID)
	return total, labeled, byClass, err
}

// GetLabelingProgressWithRollup returns labeling statistics plus the current
// mutually-exclusive rollup for a run.
func (s *AnalysisRunStore) GetLabelingProgressWithRollup(runID string) (total, labeled int, byClass map[string]int, rollup *RunLabelRollup, err error) {
	byClass = make(map[string]int)

	rollup, err = s.GetRunLabelRollup(runID)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	if rollup == nil {
		return 0, 0, byClass, nil, nil
	}
	total = rollup.Total
	labeled = rollup.LabelledCount()

	// Get counts by user label
	query := `
		SELECT ` + normalisedUserLabelExpr + ` as label, COUNT(*) as count
		FROM lidar_run_tracks
		WHERE run_id = ? AND ` + manualClassPredicate + `
		GROUP BY ` + normalisedUserLabelExpr + `
	`

	rows, err := s.db.Query(query, runID)
	if err != nil {
		if isMissingRunTracksTableErr(err) {
			return total, labeled, byClass, rollup, nil
		}
		return total, labeled, nil, nil, fmt.Errorf("get label counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var label string
		var count int
		if err := rows.Scan(&label, &count); err != nil {
			return total, labeled, nil, nil, fmt.Errorf("scan label count: %w", err)
		}
		byClass[label] = count
	}

	return total, labeled, byClass, rollup, nil
}

// GetRunLabelRollup returns the current human labelling state for one run.
func (s *AnalysisRunStore) GetRunLabelRollup(runID string) (*RunLabelRollup, error) {
	query := `
		SELECT
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN ` + manualClassPredicate + ` THEN 1 ELSE 0 END), 0) as classified,
			COALESCE(SUM(CASE WHEN NOT (` + manualClassPredicate + `) AND ` + manualTagPredicate + ` THEN 1 ELSE 0 END), 0) as tagged_only
		FROM lidar_run_tracks
		WHERE run_id = ?
	`

	var rollup RunLabelRollup
	if err := s.db.QueryRow(query, runID).Scan(&rollup.Total, &rollup.Classified, &rollup.TaggedOnly); err != nil {
		if isMissingRunTracksTableErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get run label rollup: %w", err)
	}
	rollup.Unlabelled = rollup.Total - rollup.Classified - rollup.TaggedOnly
	if rollup.Unlabelled < 0 {
		rollup.Unlabelled = 0
	}
	return &rollup, nil
}

func (s *AnalysisRunStore) populateRunLabelRollups(runs []*AnalysisRun) error {
	if len(runs) == 0 {
		return nil
	}

	byID := make(map[string]*AnalysisRun, len(runs))
	args := make([]interface{}, 0, len(runs))
	placeholders := make([]string, 0, len(runs))
	for _, run := range runs {
		byID[run.RunID] = run
		args = append(args, run.RunID)
		placeholders = append(placeholders, "?")
	}

	query := `
		SELECT
			run_id,
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN ` + manualClassPredicate + ` THEN 1 ELSE 0 END), 0) as classified,
			COALESCE(SUM(CASE WHEN NOT (` + manualClassPredicate + `) AND ` + manualTagPredicate + ` THEN 1 ELSE 0 END), 0) as tagged_only
		FROM lidar_run_tracks
		WHERE run_id IN (` + strings.Join(placeholders, ",") + `)
		GROUP BY run_id
	`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		if isMissingRunTracksTableErr(err) {
			return nil
		}
		return fmt.Errorf("list run label rollups: %w", err)
	}
	defer rows.Close()

	for _, run := range runs {
		run.LabelRollup = &RunLabelRollup{}
	}

	for rows.Next() {
		var runID string
		var rollup RunLabelRollup
		if err := rows.Scan(&runID, &rollup.Total, &rollup.Classified, &rollup.TaggedOnly); err != nil {
			return fmt.Errorf("scan run label rollup: %w", err)
		}
		rollup.Unlabelled = rollup.Total - rollup.Classified - rollup.TaggedOnly
		if rollup.Unlabelled < 0 {
			rollup.Unlabelled = 0
		}
		if run := byID[runID]; run != nil {
			run.LabelRollup = &rollup
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate run label rollups: %w", err)
	}

	return nil
}

// GetUnlabeledTracks returns tracks that need labeling.
func (s *AnalysisRunStore) GetUnlabeledTracks(runID string, limit int) ([]*RunTrack, error) {
	query := `
		SELECT run_id, track_id, sensor_id, track_state,
			start_unix_nanos, end_unix_nanos, observation_count,
			avg_speed_mps, max_speed_mps,
			bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
			height_p95_max, intensity_mean_avg,
			object_class, object_confidence, classification_model,
			user_label, label_confidence, labeler_id, labeled_at, quality_label,
			label_source,
			is_split_candidate, is_merge_candidate, linked_track_ids
		FROM lidar_run_tracks
		WHERE run_id = ? AND (` + normalisedUserLabelExpr + ` = '')
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
		var objectClass, classModel, userLabel, labelerID, qualityLabel, labelSource, linkedJSON sql.NullString
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
			&track.MaxSpeedMps,
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
			&labelSource,
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
		if labelSource.Valid {
			track.LabelSource = labelSource.String
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
