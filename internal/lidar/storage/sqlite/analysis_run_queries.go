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
			split := TrackSplit{
				OriginalTrack: t1ID,
				SplitTracks:   t2IDs,
				Confidence:    0.8, // High confidence for multiple matches
			}

			// Position fields (SplitX/SplitY) remain at zero value — position data
			// is not available from RunTrack; load observations for accurate location.

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
			merge := TrackMerge{
				MergedTrack:  t2ID,
				SourceTracks: t1IDs,
				Confidence:   0.8, // High confidence for multiple matches
			}

			// Position fields (MergeX/MergeY) remain at zero value — position data
			// is not available from RunTrack; load observations for accurate location.

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
