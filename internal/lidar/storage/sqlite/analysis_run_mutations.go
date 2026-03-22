package sqlite

import (
	"encoding/json"
	"fmt"
	"time"
)

// InsertRun creates a new analysis run.
func (s *AnalysisRunStore) InsertRun(run *AnalysisRun) error {
	query := `
		INSERT INTO lidar_run_records (
			run_id, created_at, source_type, source_path, sensor_id,
			params_json, duration_secs, total_frames, total_clusters,
			total_tracks, confirmed_tracks, processing_time_ms,
			status, error_message, parent_run_id, notes, vrlog_path
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			nullString(run.VRLogPath),
		)
		if err != nil {
			return fmt.Errorf("insert analysis run: %w", err)
		}
		return nil
	})
}

// UpdateRunStatus updates the status of an analysis run.
func (s *AnalysisRunStore) UpdateRunStatus(runID, status, errorMsg string) error {
	query := `UPDATE lidar_run_records SET status = ?, error_message = ? WHERE run_id = ?`
	return retryOnBusy(func() error {
		_, err := s.db.Exec(query, status, nullString(errorMsg), runID)
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
	query := `
		UPDATE lidar_run_records SET
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
