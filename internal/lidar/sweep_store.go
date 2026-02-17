package lidar

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SweepRecord represents a persisted sweep or auto-tune run.
type SweepRecord struct {
	ID                        int64           `json:"id"`
	SweepID                   string          `json:"sweep_id"`
	SensorID                  string          `json:"sensor_id"`
	Mode                      string          `json:"mode"`
	Status                    string          `json:"status"`
	Request                   json.RawMessage `json:"request"`
	Results                   json.RawMessage `json:"results,omitempty"`
	Charts                    json.RawMessage `json:"charts,omitempty"`
	Recommendation            json.RawMessage `json:"recommendation,omitempty"`
	RoundResults              json.RawMessage `json:"round_results,omitempty"`
	Error                     string          `json:"error,omitempty"`
	StartedAt                 time.Time       `json:"started_at"`
	CompletedAt               *time.Time      `json:"completed_at,omitempty"`
	ObjectiveName             string          `json:"objective_name,omitempty"`
	ObjectiveVersion          string          `json:"objective_version,omitempty"`
	TransformPipelineName     string          `json:"transform_pipeline_name,omitempty"`
	TransformPipelineVersion  string          `json:"transform_pipeline_version,omitempty"`
	ScoreComponents           json.RawMessage `json:"score_components,omitempty"`
	RecommendationExplanation json.RawMessage `json:"recommendation_explanation,omitempty"`
	LabelProvenanceSummary    json.RawMessage `json:"label_provenance_summary,omitempty"`
}

// SweepStore provides persistence for sweep and auto-tune results.
type SweepStore struct {
	db *sql.DB
}

// NewSweepStore creates a new SweepStore.
func NewSweepStore(db *sql.DB) *SweepStore {
	return &SweepStore{db: db}
}

// InsertSweep creates a new sweep record when a sweep starts.
func (s *SweepStore) InsertSweep(record SweepRecord) error {
	query := `
		INSERT INTO lidar_sweeps (
			sweep_id, sensor_id, mode, status, request, started_at,
			objective_name, objective_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	err := retryOnBusy(func() error {
		_, err := s.db.Exec(query,
			record.SweepID,
			record.SensorID,
			record.Mode,
			record.Status,
			string(record.Request),
			record.StartedAt.UTC().Format(time.RFC3339),
			nullStr(record.ObjectiveName),
			nullStr(record.ObjectiveVersion),
		)
		return err
	})
	if err != nil {
		return fmt.Errorf("inserting sweep %s: %w", record.SweepID, err)
	}
	return nil
}

// UpdateSweepResults updates a sweep record with results on completion or error.
func (s *SweepStore) UpdateSweepResults(sweepID, status string, results, recommendation, roundResults json.RawMessage, completedAt *time.Time, errMsg string, scoreComponents, recommendationExplanation, labelProvenanceSummary json.RawMessage, transformPipelineName, transformPipelineVersion string) error {
	query := `
		UPDATE lidar_sweeps
		SET status = ?, results = ?, recommendation = ?, round_results = ?, error = ?, completed_at = ?,
		    score_components_json = ?, recommendation_explanation_json = ?, label_provenance_summary_json = ?,
		    transform_pipeline_name = ?, transform_pipeline_version = ?
		WHERE sweep_id = ?
	`
	var completedAtStr *string
	if completedAt != nil {
		s := completedAt.UTC().Format(time.RFC3339)
		completedAtStr = &s
	}
	err := retryOnBusy(func() error {
		_, err := s.db.Exec(query,
			status,
			nullJSON(results),
			nullJSON(recommendation),
			nullJSON(roundResults),
			nullStr(errMsg),
			completedAtStr,
			nullJSON(scoreComponents),
			nullJSON(recommendationExplanation),
			nullJSON(labelProvenanceSummary),
			nullStr(transformPipelineName),
			nullStr(transformPipelineVersion),
			sweepID,
		)
		return err
	})
	if err != nil {
		return fmt.Errorf("updating sweep results for %s: %w", sweepID, err)
	}
	return nil
}

// UpdateSweepCharts saves chart configuration for a sweep.
func (s *SweepStore) UpdateSweepCharts(sweepID string, charts json.RawMessage) error {
	query := `UPDATE lidar_sweeps SET charts = ? WHERE sweep_id = ?`
	err := retryOnBusy(func() error {
		_, err := s.db.Exec(query, string(charts), sweepID)
		return err
	})
	if err != nil {
		return fmt.Errorf("updating sweep charts for %s: %w", sweepID, err)
	}
	return nil
}

// GetSweep returns a single sweep record by ID.
func (s *SweepStore) GetSweep(sweepID string) (*SweepRecord, error) {
	query := `
		SELECT id, sweep_id, sensor_id, mode, status, request, results, charts,
		       recommendation, round_results, error, started_at, completed_at,
		       objective_name, objective_version, transform_pipeline_name, transform_pipeline_version,
		       score_components_json, recommendation_explanation_json, label_provenance_summary_json
		FROM lidar_sweeps
		WHERE sweep_id = ?
	`
	var rec SweepRecord
	var request, results, charts, recommendation, roundResults, errMsg sql.NullString
	var startedAt, completedAt sql.NullString
	var objectiveName, objectiveVersion, transformPipelineName, transformPipelineVersion sql.NullString
	var scoreComponents, recommendationExplanation, labelProvenanceSummary sql.NullString

	err := s.db.QueryRow(query, sweepID).Scan(
		&rec.ID, &rec.SweepID, &rec.SensorID, &rec.Mode, &rec.Status,
		&request, &results, &charts,
		&recommendation, &roundResults, &errMsg,
		&startedAt, &completedAt,
		&objectiveName, &objectiveVersion, &transformPipelineName, &transformPipelineVersion,
		&scoreComponents, &recommendationExplanation, &labelProvenanceSummary,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying sweep %s: %w", sweepID, err)
	}

	rec.Request = jsonOrNil(request)
	rec.Results = jsonOrNil(results)
	rec.Charts = jsonOrNil(charts)
	rec.Recommendation = jsonOrNil(recommendation)
	rec.RoundResults = jsonOrNil(roundResults)
	if errMsg.Valid {
		rec.Error = errMsg.String
	}
	if startedAt.Valid {
		t, err := time.Parse(time.RFC3339, startedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parsing started_at for sweep %s: %w", sweepID, err)
		}
		rec.StartedAt = t
	}
	if completedAt.Valid {
		t, err := time.Parse(time.RFC3339, completedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parsing completed_at for sweep %s: %w", sweepID, err)
		}
		rec.CompletedAt = &t
	}
	if objectiveName.Valid {
		rec.ObjectiveName = objectiveName.String
	}
	if objectiveVersion.Valid {
		rec.ObjectiveVersion = objectiveVersion.String
	}
	if transformPipelineName.Valid {
		rec.TransformPipelineName = transformPipelineName.String
	}
	if transformPipelineVersion.Valid {
		rec.TransformPipelineVersion = transformPipelineVersion.String
	}
	rec.ScoreComponents = jsonOrNil(scoreComponents)
	rec.RecommendationExplanation = jsonOrNil(recommendationExplanation)
	rec.LabelProvenanceSummary = jsonOrNil(labelProvenanceSummary)

	return &rec, nil
}

// SweepSummary is a lightweight version of SweepRecord for list views (omits large JSON blobs).
type SweepSummary struct {
	ID          int64      `json:"id"`
	SweepID     string     `json:"sweep_id"`
	SensorID    string     `json:"sensor_id"`
	Mode        string     `json:"mode"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ListSweeps returns recent sweeps for a sensor, ordered by most recent first.
// The results omit large JSON blobs (results, charts, etc.) for performance.
func (s *SweepStore) ListSweeps(sensorID string, limit int) ([]SweepSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT id, sweep_id, sensor_id, mode, status, error, started_at, completed_at
		FROM lidar_sweeps
		WHERE sensor_id = ?
		ORDER BY started_at DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, sensorID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing sweeps: %w", err)
	}
	defer rows.Close()

	var sweeps []SweepSummary
	for rows.Next() {
		var rec SweepSummary
		var errMsg sql.NullString
		var startedAt, completedAt sql.NullString

		if err := rows.Scan(&rec.ID, &rec.SweepID, &rec.SensorID, &rec.Mode, &rec.Status, &errMsg, &startedAt, &completedAt); err != nil {
			return nil, fmt.Errorf("scanning sweep row: %w", err)
		}

		if errMsg.Valid {
			rec.Error = errMsg.String
		}
		if startedAt.Valid {
			t, err := time.Parse(time.RFC3339, startedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parsing started_at for sweep row: %w", err)
			}
			rec.StartedAt = t
		}
		if completedAt.Valid {
			t, err := time.Parse(time.RFC3339, completedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parsing completed_at for sweep row: %w", err)
			}
			rec.CompletedAt = &t
		}

		sweeps = append(sweeps, rec)
	}

	return sweeps, rows.Err()
}

// DeleteSweep removes a sweep record.
func (s *SweepStore) DeleteSweep(sweepID string) error {
	return retryOnBusy(func() error {
		_, err := s.db.Exec(`DELETE FROM lidar_sweeps WHERE sweep_id = ?`, sweepID)
		return err
	})
}

// SaveSweepStart implements sweep.SweepPersister for the Runner/AutoTuner integration.
func (s *SweepStore) SaveSweepStart(sweepID, sensorID, mode string, request json.RawMessage, startedAt time.Time, objectiveName, objectiveVersion string) error {
	return s.InsertSweep(SweepRecord{
		SweepID:          sweepID,
		SensorID:         sensorID,
		Mode:             mode,
		Status:           "running",
		Request:          request,
		StartedAt:        startedAt,
		ObjectiveName:    objectiveName,
		ObjectiveVersion: objectiveVersion,
	})
}

// SaveSweepComplete implements sweep.SweepPersister for the Runner/AutoTuner integration.
func (s *SweepStore) SaveSweepComplete(sweepID, status string, results, recommendation, roundResults json.RawMessage, completedAt time.Time, errMsg string, scoreComponents, recommendationExplanation, labelProvenanceSummary json.RawMessage, transformPipelineName, transformPipelineVersion string) error {
	return s.UpdateSweepResults(sweepID, status, results, recommendation, roundResults, &completedAt, errMsg, scoreComponents, recommendationExplanation, labelProvenanceSummary, transformPipelineName, transformPipelineVersion)
}

// SaveSweepCheckpoint persists a mid-run checkpoint so a suspended auto-tune can be resumed.
func (s *SweepStore) SaveSweepCheckpoint(sweepID string, round int, bounds, results, request json.RawMessage) error {
	query := `
		UPDATE lidar_sweeps
		SET status = 'suspended',
		    checkpoint_round = ?,
		    checkpoint_bounds = ?,
		    checkpoint_results = ?,
		    checkpoint_request = ?
		WHERE sweep_id = ?
	`
	err := retryOnBusy(func() error {
		_, err := s.db.Exec(query, round, nullJSON(bounds), nullJSON(results), nullJSON(request), sweepID)
		return err
	})
	if err != nil {
		return fmt.Errorf("saving checkpoint for sweep %s: %w", sweepID, err)
	}
	return nil
}

// LoadSweepCheckpoint loads a checkpoint for resuming a suspended auto-tune.
func (s *SweepStore) LoadSweepCheckpoint(sweepID string) (round int, bounds, results, request json.RawMessage, err error) {
	query := `
		SELECT checkpoint_round, checkpoint_bounds, checkpoint_results, checkpoint_request
		FROM lidar_sweeps
		WHERE sweep_id = ? AND status = 'suspended'
	`
	var (
		roundVal   sql.NullInt64
		boundsStr  sql.NullString
		resultsStr sql.NullString
		requestStr sql.NullString
	)
	row := s.db.QueryRow(query, sweepID)
	if err = row.Scan(&roundVal, &boundsStr, &resultsStr, &requestStr); err != nil {
		return 0, nil, nil, nil, fmt.Errorf("loading checkpoint for sweep %s: %w", sweepID, err)
	}
	if !roundVal.Valid {
		return 0, nil, nil, nil, fmt.Errorf("no checkpoint found for sweep %s", sweepID)
	}
	return int(roundVal.Int64), jsonOrNil(boundsStr), jsonOrNil(resultsStr), jsonOrNil(requestStr), nil
}

// nullJSON returns a sql.NullString for a JSON value, treating nil or empty as NULL.
func nullJSON(data json.RawMessage) *string {
	if len(data) == 0 {
		return nil
	}
	s := string(data)
	return &s
}

// SuspendedSweepInfo is a lightweight summary of a suspended sweep for the
// resume UI. It omits large JSON blobs to keep the response compact.
type SuspendedSweepInfo struct {
	SweepID         string    `json:"sweep_id"`
	SensorID        string    `json:"sensor_id"`
	CheckpointRound int       `json:"checkpoint_round"`
	StartedAt       time.Time `json:"started_at"`
}

// GetSuspendedSweep implements sweep.SweepPersister. It returns the most
// recent suspended sweep's ID and checkpoint round, or ("", 0, nil) when none
// exists.
func (s *SweepStore) GetSuspendedSweep() (string, int, error) {
	info, err := s.GetSuspendedSweepInfo()
	if err != nil {
		return "", 0, err
	}
	if info == nil {
		return "", 0, nil
	}
	return info.SweepID, info.CheckpointRound, nil
}

// GetSuspendedSweepInfo returns full suspended sweep info for the HTTP handler.
// Returns nil when no suspended sweep exists.
func (s *SweepStore) GetSuspendedSweepInfo() (*SuspendedSweepInfo, error) {
	query := `
		SELECT sweep_id, sensor_id, COALESCE(checkpoint_round, 0), started_at
		FROM lidar_sweeps
		WHERE status = 'suspended'
		ORDER BY started_at DESC
		LIMIT 1
	`
	var info SuspendedSweepInfo
	var startedAtStr string
	err := s.db.QueryRow(query).Scan(&info.SweepID, &info.SensorID, &info.CheckpointRound, &startedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying suspended sweeps: %w", err)
	}
	t, err := time.Parse(time.RFC3339, startedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parsing started_at for suspended sweep %s: %w", info.SweepID, err)
	}
	info.StartedAt = t
	return &info, nil
}

// nullStr returns nil for empty strings, pointer to string otherwise.
func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// jsonOrNil converts a sql.NullString to json.RawMessage, returning nil for NULL values.
func jsonOrNil(ns sql.NullString) json.RawMessage {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	return json.RawMessage(ns.String)
}
