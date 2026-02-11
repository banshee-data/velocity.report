package lidar

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SweepRecord represents a persisted sweep or auto-tune run.
type SweepRecord struct {
	ID             int64           `json:"id"`
	SweepID        string          `json:"sweep_id"`
	SensorID       string          `json:"sensor_id"`
	Mode           string          `json:"mode"`
	Status         string          `json:"status"`
	Request        json.RawMessage `json:"request"`
	Results        json.RawMessage `json:"results,omitempty"`
	Charts         json.RawMessage `json:"charts,omitempty"`
	Recommendation json.RawMessage `json:"recommendation,omitempty"`
	RoundResults   json.RawMessage `json:"round_results,omitempty"`
	Error          string          `json:"error,omitempty"`
	StartedAt      time.Time       `json:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
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
			sweep_id, sensor_id, mode, status, request, started_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`
	err := retryOnBusy(func() error {
		_, err := s.db.Exec(query,
			record.SweepID,
			record.SensorID,
			record.Mode,
			record.Status,
			string(record.Request),
			record.StartedAt.UTC().Format(time.RFC3339),
		)
		return err
	})
	if err != nil {
		return fmt.Errorf("inserting sweep %s: %w", record.SweepID, err)
	}
	return nil
}

// UpdateSweepResults updates a sweep record with results on completion or error.
func (s *SweepStore) UpdateSweepResults(sweepID, status string, results, recommendation, roundResults json.RawMessage, completedAt *time.Time, errMsg string) error {
	query := `
		UPDATE lidar_sweeps
		SET status = ?, results = ?, recommendation = ?, round_results = ?, error = ?, completed_at = ?
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
		       recommendation, round_results, error, started_at, completed_at
		FROM lidar_sweeps
		WHERE sweep_id = ?
	`
	var rec SweepRecord
	var request, results, charts, recommendation, roundResults, errMsg sql.NullString
	var startedAt, completedAt sql.NullString

	err := s.db.QueryRow(query, sweepID).Scan(
		&rec.ID, &rec.SweepID, &rec.SensorID, &rec.Mode, &rec.Status,
		&request, &results, &charts,
		&recommendation, &roundResults, &errMsg,
		&startedAt, &completedAt,
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
func (s *SweepStore) SaveSweepStart(sweepID, sensorID, mode string, request json.RawMessage, startedAt time.Time) error {
	return s.InsertSweep(SweepRecord{
		SweepID:   sweepID,
		SensorID:  sensorID,
		Mode:      mode,
		Status:    "running",
		Request:   request,
		StartedAt: startedAt,
	})
}

// SaveSweepComplete implements sweep.SweepPersister for the Runner/AutoTuner integration.
func (s *SweepStore) SaveSweepComplete(sweepID, status string, results, recommendation, roundResults json.RawMessage, completedAt time.Time, errMsg string) error {
	return s.UpdateSweepResults(sweepID, status, results, recommendation, roundResults, &completedAt, errMsg)
}

// nullJSON returns a sql.NullString for a JSON value, treating nil or empty as NULL.
func nullJSON(data json.RawMessage) *string {
	if len(data) == 0 {
		return nil
	}
	s := string(data)
	return &s
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
