package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// InsertRun creates a new analysis run.
func (s *AnalysisRunStore) InsertRun(run *AnalysisRun) error {
	caps, err := s.runRecordCapabilities()
	if err != nil {
		return err
	}

	columns := []string{
		"run_id", "created_at", "source_type", "source_path", "sensor_id",
		"duration_secs", "total_frames", "total_clusters",
		"total_tracks", "confirmed_tracks", "processing_time_ms",
		"status", "error_message", "parent_run_id", "notes", "vrlog_path",
	}
	args := []any{
		run.RunID,
		run.CreatedAt.UnixNano(),
		run.SourceType,
		nullString(run.SourcePath),
		run.SensorID,
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
		nullString(run.VRLogPath),
	}

	if caps.ParamsJSON {
		columns = append(columns, "params_json")
		args = append(args, string(run.ParamsJSON))
	}

	if caps.RunConfigID {
		columns = append(columns, "run_config_id")
		args = append(args, nullString(run.RunConfigID))
	}
	if caps.RequestedParamSetID {
		columns = append(columns, "requested_param_set_id")
		args = append(args, nullString(run.RequestedParamSetID))
	}
	if caps.ReplayCaseID {
		columns = append(columns, "replay_case_id")
		args = append(args, nullString(run.ReplayCaseID))
	}
	if caps.CompletedAt {
		columns = append(columns, "completed_at")
		args = append(args, nullableTimeUnixNano(run.CompletedAt))
	}
	if caps.FrameStartNs {
		columns = append(columns, "frame_start_ns")
		args = append(args, nullInt64(run.FrameStartNs))
	}
	if caps.FrameEndNs {
		columns = append(columns, "frame_end_ns")
		args = append(args, nullInt64(run.FrameEndNs))
	}

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`
		INSERT INTO lidar_run_records (
			%s
		) VALUES (%s)
	`, strings.Join(columns, ", "), strings.Join(placeholders, ", "))

	// Retry on SQLITE_BUSY errors
	return retryOnBusy(func() error {
		_, err := s.db.Exec(query, args...)
		if err != nil {
			return fmt.Errorf("insert analysis run: %w", err)
		}
		return nil
	})
}

// UpdateRunStatus updates the status of an analysis run.
func (s *AnalysisRunStore) UpdateRunStatus(runID, status, errorMsg string) error {
	caps, err := s.runRecordCapabilities()
	if err != nil {
		return err
	}

	setClauses := []string{"status = ?", "error_message = ?"}
	args := []any{status, nullString(errorMsg)}
	if caps.CompletedAt && status != "running" {
		setClauses = append(setClauses, "completed_at = ?")
		args = append(args, time.Now().UnixNano())
	}
	args = append(args, runID)

	query := fmt.Sprintf(
		`UPDATE lidar_run_records SET %s WHERE run_id = ?`,
		strings.Join(setClauses, ", "),
	)
	return retryOnBusy(func() error {
		_, err := s.db.Exec(query, args...)
		if err != nil {
			return fmt.Errorf("update run status: %w", err)
		}
		return nil
	})
}

// UpdateRunVRLogPath updates the vrlog_path of an analysis run.
func (s *AnalysisRunStore) UpdateRunVRLogPath(runID, vrlogPath string) error {
	query := `UPDATE lidar_run_records SET vrlog_path = ? WHERE run_id = ?`
	return retryOnBusy(func() error {
		_, err := s.db.Exec(query, nullString(vrlogPath), runID)
		if err != nil {
			return fmt.Errorf("update run vrlog path: %w", err)
		}
		return nil
	})
}

// CompleteRun marks a run as completed with final statistics.
func (s *AnalysisRunStore) CompleteRun(runID string, stats *AnalysisStats) error {
	caps, err := s.runRecordCapabilities()
	if err != nil {
		return err
	}

	setClauses := []string{
		"duration_secs = ?",
		"total_frames = ?",
		"total_clusters = ?",
		"total_tracks = ?",
		"confirmed_tracks = ?",
		"processing_time_ms = ?",
		"status = 'completed'",
	}
	args := []any{
		stats.DurationSecs,
		stats.TotalFrames,
		stats.TotalClusters,
		stats.TotalTracks,
		stats.ConfirmedTracks,
		stats.ProcessingTimeMs,
	}

	if caps.CompletedAt {
		completedAt := stats.CompletedAt
		if completedAt.IsZero() {
			completedAt = time.Now()
		}
		setClauses = append(setClauses, "completed_at = ?")
		args = append(args, completedAt.UnixNano())
	}
	if caps.FrameStartNs {
		setClauses = append(setClauses, "frame_start_ns = ?")
		args = append(args, nullableInt64Value(stats.FrameStartNs))
	}
	if caps.FrameEndNs {
		setClauses = append(setClauses, "frame_end_ns = ?")
		args = append(args, nullableInt64Value(stats.FrameEndNs))
	}
	args = append(args, runID)

	query := fmt.Sprintf(`
		UPDATE lidar_run_records SET
			%s
		WHERE run_id = ?
	`, strings.Join(setClauses, ",\n\t\t\t"))

	// Retry on SQLITE_BUSY errors
	return retryOnBusy(func() error {
		_, err := s.db.Exec(query, args...)
		if err != nil {
			return fmt.Errorf("complete run: %w", err)
		}
		return nil
	})
}

func nullableInt64Value(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func nullableTimeUnixNano(value *time.Time) interface{} {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UnixNano()
}

// InsertRunTrack inserts a track for an analysis run.
// Uses retry logic to handle SQLITE_BUSY errors from concurrent writes.
func (s *AnalysisRunStore) InsertRunTrack(track *RunTrack) error {
	userLabel := normaliseRunTrackString(track.UserLabel)
	qualityLabel := normaliseRunTrackQualityLabel(track.QualityLabel)
	labelerID := normaliseRunTrackString(track.LabelerID)
	labelSource := normaliseRunTrackString(track.LabelSource)
	linkedIDs := normaliseRunTrackLinkedIDs(track.LinkedTrackIDs)
	linkedJSON := "[]"
	if len(linkedIDs) > 0 {
		if b, err := json.Marshal(linkedIDs); err == nil {
			linkedJSON = string(b)
		}
	}

	query := `
		INSERT INTO lidar_run_tracks (
			run_id, track_id, ` + trackMeasurementColumns + `,
			user_label, label_confidence, labeler_id, labeled_at, quality_label,
			label_source,
			is_split_candidate, is_merge_candidate, linked_track_ids
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	var labeledAt interface{}
	if track.LabeledAt > 0 {
		labeledAt = track.LabeledAt
	}

	args := []any{track.RunID, track.TrackID}
	args = append(args, trackMeasurementInsertArgs(&track.TrackMeasurement)...)
	args = append(args,
		nullString(userLabel),
		nullFloat32(track.LabelConfidence),
		nullString(labelerID),
		labeledAt,
		nullString(qualityLabel),
		nullString(labelSource),
		track.IsSplitCandidate,
		track.IsMergeCandidate,
		linkedJSON,
	)

	// Retry on SQLITE_BUSY errors
	return retryOnBusy(func() error {
		_, err := s.db.Exec(query, args...)
		if err != nil {
			return fmt.Errorf("insert run track: %w", err)
		}
		return nil
	})
}

// UpdateTrackLabel updates the user label and quality label for a track.
// Both userLabel and qualityLabel can be empty strings, which will be stored as NULL in the database.
// Values are trimmed and canonicalised before storage.
// This function does NOT validate enum values - it accepts any string and stores it as-is.
// Validation of label enum values should be performed by the caller (e.g., API handlers)
// using ValidateUserLabel() and ValidateQualityLabel() from the api package.
func (s *AnalysisRunStore) UpdateTrackLabel(runID, trackID, userLabel, qualityLabel string, confidence float32, labelerID, labelSource string) error {
	userLabel = normaliseRunTrackString(userLabel)
	qualityLabel = normaliseRunTrackQualityLabel(qualityLabel)
	labelerID = normaliseRunTrackString(labelerID)
	labelSource = normaliseRunTrackString(labelSource)

	query := `
		UPDATE lidar_run_tracks SET
			user_label = ?,
			label_confidence = ?,
			labeler_id = ?,
			labeled_at = ?,
			quality_label = ?,
			label_source = ?
		WHERE run_id = ? AND track_id = ?
	`

	return retryOnBusy(func() error {
		_, err := s.db.Exec(query,
			nullString(userLabel),
			confidence,
			nullString(labelerID),
			time.Now().UnixNano(),
			nullString(qualityLabel),
			nullString(labelSource),
			runID,
			trackID,
		)
		if err != nil {
			return fmt.Errorf("update track label: %w", err)
		}
		return nil
	})
}

// UpdateTrackQualityFlags updates the split/merge flags for a track.
func (s *AnalysisRunStore) UpdateTrackQualityFlags(runID, trackID string, isSplit, isMerge bool, linkedIDs []string) error {
	linkedIDs = normaliseRunTrackLinkedIDs(linkedIDs)
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

	return retryOnBusy(func() error {
		_, err := s.db.Exec(query, isSplit, isMerge, linkedJSON, runID, trackID)
		if err != nil {
			return fmt.Errorf("update track quality flags: %w", err)
		}
		return nil
	})
}
