package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/storage/configasset"
)

type analysisRunRowScanner interface {
	Scan(dest ...any) error
}

type analysisRunRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func (s *AnalysisRunStore) runRecordSelectColumns() ([]string, analysisRunRecordCapabilities, error) {
	caps, err := s.runRecordCapabilities()
	if err != nil {
		return nil, analysisRunRecordCapabilities{}, err
	}

	columns := []string{
		"run_id",
		"created_at",
		"source_type",
		"source_path",
		"sensor_id",
		"duration_secs",
		"total_frames",
		"total_clusters",
		"total_tracks",
		"confirmed_tracks",
		"processing_time_ms",
		"status",
		"error_message",
		"parent_run_id",
		"notes",
		"vrlog_path",
	}
	if caps.ParamsJSON {
		columns = append(columns, "params_json")
	}
	if caps.RunConfigID {
		columns = append(columns, "run_config_id")
	}
	if caps.RequestedParamSetID {
		columns = append(columns, "requested_param_set_id")
	}
	if caps.ReplayCaseID {
		columns = append(columns, "replay_case_id")
	}
	if caps.CompletedAt {
		columns = append(columns, "completed_at")
	}
	if caps.FrameStartNs {
		columns = append(columns, "frame_start_ns")
	}
	if caps.FrameEndNs {
		columns = append(columns, "frame_end_ns")
	}
	if caps.StatisticsJSON {
		columns = append(columns, "statistics_json")
	}

	return columns, caps, nil
}

func scanAnalysisRunRecord(scanner analysisRunRowScanner, caps analysisRunRecordCapabilities) (*AnalysisRun, error) {
	var (
		run            AnalysisRun
		createdAt      int64
		sourcePath     sql.NullString
		errorMessage   sql.NullString
		parentRunID    sql.NullString
		notes          sql.NullString
		vrlogPath      sql.NullString
		paramsJSON     sql.NullString
		statisticsJSON sql.NullString
	)

	dests := []any{
		&run.RunID,
		&createdAt,
		&run.SourceType,
		&sourcePath,
		&run.SensorID,
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
	}

	if caps.ParamsJSON {
		dests = append(dests, &paramsJSON)
	}

	var (
		runConfigID         sql.NullString
		requestedParamSetID sql.NullString
		replayCaseID        sql.NullString
		completedAt         sql.NullInt64
		frameStartNs        sql.NullInt64
		frameEndNs          sql.NullInt64
	)
	if caps.RunConfigID {
		dests = append(dests, &runConfigID)
	}
	if caps.RequestedParamSetID {
		dests = append(dests, &requestedParamSetID)
	}
	if caps.ReplayCaseID {
		dests = append(dests, &replayCaseID)
	}
	if caps.CompletedAt {
		dests = append(dests, &completedAt)
	}
	if caps.FrameStartNs {
		dests = append(dests, &frameStartNs)
	}
	if caps.FrameEndNs {
		dests = append(dests, &frameEndNs)
	}
	if caps.StatisticsJSON {
		dests = append(dests, &statisticsJSON)
	}

	if err := scanner.Scan(dests...); err != nil {
		return nil, err
	}

	run.CreatedAt = time.Unix(0, createdAt)
	if paramsJSON.Valid {
		run.ParamsJSON = json.RawMessage(paramsJSON.String)
	}
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
	if runConfigID.Valid {
		run.RunConfigID = runConfigID.String
	}
	if requestedParamSetID.Valid {
		run.RequestedParamSetID = requestedParamSetID.String
	}
	if replayCaseID.Valid {
		run.ReplayCaseID = replayCaseID.String
	}
	if completedAt.Valid {
		value := time.Unix(0, completedAt.Int64)
		run.CompletedAt = &value
	}
	if frameStartNs.Valid {
		value := frameStartNs.Int64
		run.FrameStartNs = &value
	}
	if frameEndNs.Valid {
		value := frameEndNs.Int64
		run.FrameEndNs = &value
	}
	if statisticsJSON.Valid && strings.TrimSpace(statisticsJSON.String) != "" {
		run.StatisticsJSON = json.RawMessage(statisticsJSON.String)
	}

	run.PopulateReplayCaseName()
	return &run, nil
}

func collectAnalysisRunRecords(rows analysisRunRows, caps analysisRunRecordCapabilities) ([]*AnalysisRun, error) {
	var runs []*AnalysisRun
	for rows.Next() {
		run, err := scanAnalysisRunRecord(rows, caps)
		if err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runs: %w", err)
	}
	return runs, nil
}

func finalizeAnalysisRunRecords(runs []*AnalysisRun, hydrate func(*AnalysisRun), populate func([]*AnalysisRun) error) ([]*AnalysisRun, error) {
	for _, run := range runs {
		hydrate(run)
	}
	if err := populate(runs); err != nil {
		return nil, err
	}
	return runs, nil
}

// GetRun retrieves an analysis run by ID.
func (s *AnalysisRunStore) GetRun(runID string) (*AnalysisRun, error) {
	columns, caps, err := s.runRecordSelectColumns()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM lidar_run_records
		WHERE run_id = ?
	`, strings.Join(columns, ", "))

	run, err := scanAnalysisRunRecord(s.db.QueryRow(query, runID), caps)
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	s.hydrateRunConfigAssets(run)
	labelRollup, err := s.GetRunLabelRollup(runID)
	if err != nil {
		return nil, err
	}
	run.LabelRollup = labelRollup

	return run, nil
}

// ListRuns retrieves recent analysis runs.
func (s *AnalysisRunStore) ListRuns(limit int) ([]*AnalysisRun, error) {
	columns, caps, err := s.runRecordSelectColumns()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM lidar_run_records
		ORDER BY created_at DESC
		LIMIT ?
	`, strings.Join(columns, ", "))

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	runs, err := collectAnalysisRunRecords(rows, caps)
	if err != nil {
		return nil, err
	}

	return finalizeAnalysisRunRecords(runs, s.hydrateRunConfigAssets, s.populateRunLabelRollups)
}

func (s *AnalysisRunStore) hydrateRunConfigAssets(run *AnalysisRun) {
	if run == nil || strings.TrimSpace(run.RunConfigID) == "" {
		return
	}

	configStore := configasset.NewStore(s.db)
	runConfig, err := configStore.GetRunConfig(run.RunConfigID)
	if err != nil {
		if err == sql.ErrNoRows || isMissingConfigAssetSchemaErr(err) {
			return
		}
		return
	}

	run.ParamSetID = runConfig.ParamSetID
	run.ConfigHash = runConfig.ConfigHash
	run.ParamsHash = runConfig.ParamsHash
	run.SchemaVersion = runConfig.ParamSchemaVersion
	run.ParamSetType = runConfig.ParamSetType
	run.BuildVersion = runConfig.BuildVersion
	run.BuildGitSHA = runConfig.BuildGitSHA
	if len(runConfig.ComposedJSON) > 0 {
		run.ExecutionConfig = append(json.RawMessage(nil), runConfig.ComposedJSON...)
	}
}

// GetRunTracks retrieves all tracks for an analysis run.
func (s *AnalysisRunStore) GetRunTracks(runID string) ([]*RunTrack, error) {
	query := `
		SELECT run_id, track_id, ` + trackMeasurementColumns + `,
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

	return collectRunTracks(rows)
}

func collectRunTracks(rows analysisRunRows) ([]*RunTrack, error) {
	var tracks []*RunTrack
	for rows.Next() {
		var track RunTrack
		var labeledAt sql.NullInt64
		var userLabel, labelerID, qualityLabel, labelSource, linkedJSON sql.NullString
		var labelConf sql.NullFloat64

		measDests, applyMeas := scanTrackMeasurementDests(&track.TrackMeasurement)

		dests := []any{&track.RunID, &track.TrackID}
		dests = append(dests, measDests...)
		dests = append(dests,
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

		err := rows.Scan(dests...)
		if err != nil {
			return nil, fmt.Errorf("scan run track: %w", err)
		}
		applyMeas()

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
		SELECT run_id, track_id, ` + trackMeasurementColumns + `,
			user_label, label_confidence, labeler_id, labeled_at, quality_label,
			label_source,
			is_split_candidate, is_merge_candidate, linked_track_ids
		FROM lidar_run_tracks
		WHERE run_id = ? AND track_id = ?
	`

	var track RunTrack
	var labeledAt sql.NullInt64
	var userLabel, labelerID, qualityLabel, labelSource, linkedJSON sql.NullString
	var labelConf sql.NullFloat64

	measDests, applyMeas := scanTrackMeasurementDests(&track.TrackMeasurement)

	dests := []any{&track.RunID, &track.TrackID}
	dests = append(dests, measDests...)
	dests = append(dests,
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

	err := s.db.QueryRow(query, runID, trackID).Scan(dests...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("track %s not found in run %s", trackID, runID)
		}
		return nil, fmt.Errorf("query run track: %w", err)
	}
	applyMeas()

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

	return assignRunLabelRollups(rows, byID)
}

func assignRunLabelRollups(rows analysisRunRows, byID map[string]*AnalysisRun) error {
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
		SELECT run_id, track_id, ` + trackMeasurementColumns + `,
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

	return collectUnlabeledTracks(rows)
}

func collectUnlabeledTracks(rows analysisRunRows) ([]*RunTrack, error) {
	var tracks []*RunTrack
	for rows.Next() {
		var track RunTrack
		var labeledAt sql.NullInt64
		var userLabel, labelerID, qualityLabel, labelSource, linkedJSON sql.NullString
		var labelConf sql.NullFloat64

		measDests, applyMeas := scanTrackMeasurementDests(&track.TrackMeasurement)

		dests := []any{&track.RunID, &track.TrackID}
		dests = append(dests, measDests...)
		dests = append(dests,
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

		err := rows.Scan(dests...)
		if err != nil {
			return nil, fmt.Errorf("scan unlabeled track: %w", err)
		}
		applyMeas()

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
