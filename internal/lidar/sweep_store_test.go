package lidar

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestSweepDB creates a test database with the lidar_sweeps table.
func setupTestSweepDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Create lidar_sweeps table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_sweeps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sweep_id TEXT NOT NULL UNIQUE,
			sensor_id TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT 'sweep',
			status TEXT NOT NULL DEFAULT 'running',
			request TEXT NOT NULL,
			results TEXT,
			charts TEXT,
			recommendation TEXT,
			round_results TEXT,
			error TEXT,
			started_at DATETIME NOT NULL,
			completed_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			objective_name TEXT,
			objective_version TEXT,
			transform_pipeline_name TEXT,
			transform_pipeline_version TEXT,
			score_components_json TEXT,
			recommendation_explanation_json TEXT,
			label_provenance_summary_json TEXT
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_sweeps table: %v", err)
	}

	return db
}

func TestSweepStore_InsertAndGet(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Create a sweep record
	startedAt := time.Now().UTC()
	request := json.RawMessage(`{"param":"noise_relative","start":0.01,"end":0.2}`)
	record := SweepRecord{
		SweepID:   "sweep-001",
		SensorID:  "sensor-001",
		Mode:      "manual",
		Status:    "running",
		Request:   request,
		StartedAt: startedAt,
	}

	err := store.InsertSweep(record)
	if err != nil {
		t.Fatalf("InsertSweep failed: %v", err)
	}

	// Retrieve the sweep
	retrieved, err := store.GetSweep("sweep-001")
	if err != nil {
		t.Fatalf("GetSweep failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected sweep to be found")
	}

	// Verify fields
	if retrieved.SweepID != record.SweepID {
		t.Errorf("sweep_id mismatch: got %s, want %s", retrieved.SweepID, record.SweepID)
	}
	if retrieved.SensorID != record.SensorID {
		t.Errorf("sensor_id mismatch: got %s, want %s", retrieved.SensorID, record.SensorID)
	}
	if retrieved.Mode != record.Mode {
		t.Errorf("mode mismatch: got %s, want %s", retrieved.Mode, record.Mode)
	}
	if retrieved.Status != record.Status {
		t.Errorf("status mismatch: got %s, want %s", retrieved.Status, record.Status)
	}
	if string(retrieved.Request) != string(request) {
		t.Errorf("request mismatch: got %s, want %s", string(retrieved.Request), string(request))
	}
	// Check time with some tolerance (1 second)
	if retrieved.StartedAt.Sub(startedAt).Abs() > time.Second {
		t.Errorf("started_at mismatch: got %v, want %v", retrieved.StartedAt, startedAt)
	}
	if retrieved.CompletedAt != nil {
		t.Error("expected completed_at to be nil")
	}
}

func TestSweepStore_UpdateSweepResults(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Insert initial sweep
	startedAt := time.Now().UTC()
	request := json.RawMessage(`{"param":"noise_relative"}`)
	record := SweepRecord{
		SweepID:   "sweep-002",
		SensorID:  "sensor-001",
		Mode:      "manual",
		Status:    "running",
		Request:   request,
		StartedAt: startedAt,
	}
	if err := store.InsertSweep(record); err != nil {
		t.Fatalf("InsertSweep failed: %v", err)
	}

	// Update with results
	completedAt := time.Now().UTC()
	results := json.RawMessage(`{"best_value":0.05,"score":95.5}`)
	recommendation := json.RawMessage(`{"param":"noise_relative","value":0.05}`)
	roundResults := json.RawMessage(`[{"value":0.01,"score":80},{"value":0.05,"score":95.5}]`)

	err := store.UpdateSweepResults("sweep-002", "completed", results, recommendation, roundResults, &completedAt, "", nil, nil, nil, "", "")
	if err != nil {
		t.Fatalf("UpdateSweepResults failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.GetSweep("sweep-002")
	if err != nil {
		t.Fatalf("GetSweep failed: %v", err)
	}

	if retrieved.Status != "completed" {
		t.Errorf("status not updated: got %s, want completed", retrieved.Status)
	}
	if string(retrieved.Results) != string(results) {
		t.Errorf("results not updated: got %s, want %s", string(retrieved.Results), string(results))
	}
	if string(retrieved.Recommendation) != string(recommendation) {
		t.Errorf("recommendation not updated: got %s, want %s", string(retrieved.Recommendation), string(recommendation))
	}
	if string(retrieved.RoundResults) != string(roundResults) {
		t.Errorf("round_results not updated: got %s, want %s", string(retrieved.RoundResults), string(roundResults))
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	} else if retrieved.CompletedAt.Sub(completedAt).Abs() > time.Second {
		t.Errorf("completed_at mismatch: got %v, want %v", retrieved.CompletedAt, completedAt)
	}
	if retrieved.Error != "" {
		t.Errorf("expected no error, got %s", retrieved.Error)
	}
}

func TestSweepStore_UpdateSweepResults_WithError(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Insert initial sweep
	startedAt := time.Now().UTC()
	request := json.RawMessage(`{"param":"noise_relative"}`)
	record := SweepRecord{
		SweepID:   "sweep-003",
		SensorID:  "sensor-001",
		Mode:      "auto",
		Status:    "running",
		Request:   request,
		StartedAt: startedAt,
	}
	if err := store.InsertSweep(record); err != nil {
		t.Fatalf("InsertSweep failed: %v", err)
	}

	// Update with error
	completedAt := time.Now().UTC()
	errMsg := "sensor communication timeout"

	err := store.UpdateSweepResults("sweep-003", "failed", nil, nil, nil, &completedAt, errMsg, nil, nil, nil, "", "")
	if err != nil {
		t.Fatalf("UpdateSweepResults failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.GetSweep("sweep-003")
	if err != nil {
		t.Fatalf("GetSweep failed: %v", err)
	}

	if retrieved.Status != "failed" {
		t.Errorf("status not updated: got %s, want failed", retrieved.Status)
	}
	if retrieved.Error != errMsg {
		t.Errorf("error not updated: got %s, want %s", retrieved.Error, errMsg)
	}
	if retrieved.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestSweepStore_UpdateSweepCharts(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Insert initial sweep
	startedAt := time.Now().UTC()
	request := json.RawMessage(`{"param":"noise_relative"}`)
	record := SweepRecord{
		SweepID:   "sweep-004",
		SensorID:  "sensor-001",
		Mode:      "manual",
		Status:    "completed",
		Request:   request,
		StartedAt: startedAt,
	}
	if err := store.InsertSweep(record); err != nil {
		t.Fatalf("InsertSweep failed: %v", err)
	}

	// Update charts
	charts := json.RawMessage(`[{"id":"chart1","type":"line","title":"Test Chart"}]`)
	err := store.UpdateSweepCharts("sweep-004", charts)
	if err != nil {
		t.Fatalf("UpdateSweepCharts failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.GetSweep("sweep-004")
	if err != nil {
		t.Fatalf("GetSweep failed: %v", err)
	}

	if string(retrieved.Charts) != string(charts) {
		t.Errorf("charts not updated: got %s, want %s", string(retrieved.Charts), string(charts))
	}
}

func TestSweepStore_ListSweeps(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Insert multiple sweeps
	baseTime := time.Now().UTC()
	sweeps := []SweepRecord{
		{
			SweepID:   "sweep-101",
			SensorID:  "sensor-001",
			Mode:      "manual",
			Status:    "completed",
			Request:   json.RawMessage(`{}`),
			StartedAt: baseTime.Add(-3 * time.Hour),
		},
		{
			SweepID:   "sweep-102",
			SensorID:  "sensor-001",
			Mode:      "auto",
			Status:    "completed",
			Request:   json.RawMessage(`{}`),
			StartedAt: baseTime.Add(-1 * time.Hour),
		},
		{
			SweepID:   "sweep-103",
			SensorID:  "sensor-002",
			Mode:      "manual",
			Status:    "running",
			Request:   json.RawMessage(`{}`),
			StartedAt: baseTime,
		},
	}

	for _, sweep := range sweeps {
		if err := store.InsertSweep(sweep); err != nil {
			t.Fatalf("InsertSweep failed: %v", err)
		}
	}

	// List sweeps for sensor-001
	sensor001Sweeps, err := store.ListSweeps("sensor-001", 10)
	if err != nil {
		t.Fatalf("ListSweeps failed: %v", err)
	}
	if len(sensor001Sweeps) != 2 {
		t.Errorf("expected 2 sweeps for sensor-001, got %d", len(sensor001Sweeps))
	}

	// Verify ordering (newest first)
	if sensor001Sweeps[0].SweepID != "sweep-102" {
		t.Errorf("expected newest sweep first, got %s", sensor001Sweeps[0].SweepID)
	}
	if sensor001Sweeps[1].SweepID != "sweep-101" {
		t.Errorf("expected oldest sweep second, got %s", sensor001Sweeps[1].SweepID)
	}

	// Verify limit
	limitedSweeps, err := store.ListSweeps("sensor-001", 1)
	if err != nil {
		t.Fatalf("ListSweeps with limit failed: %v", err)
	}
	if len(limitedSweeps) != 1 {
		t.Errorf("expected 1 sweep with limit=1, got %d", len(limitedSweeps))
	}
}

func TestSweepStore_ListSweeps_LimitBounds(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Insert a sweep
	record := SweepRecord{
		SweepID:   "sweep-201",
		SensorID:  "sensor-001",
		Mode:      "manual",
		Status:    "completed",
		Request:   json.RawMessage(`{}`),
		StartedAt: time.Now().UTC(),
	}
	if err := store.InsertSweep(record); err != nil {
		t.Fatalf("InsertSweep failed: %v", err)
	}

	// Test limit=0 defaults to 20
	sweeps, err := store.ListSweeps("sensor-001", 0)
	if err != nil {
		t.Fatalf("ListSweeps(limit=0) failed: %v", err)
	}
	if len(sweeps) != 1 {
		t.Errorf("expected 1 sweep, got %d", len(sweeps))
	}

	// Test limit>100 caps to 100
	sweeps, err = store.ListSweeps("sensor-001", 200)
	if err != nil {
		t.Fatalf("ListSweeps(limit=200) failed: %v", err)
	}
	if len(sweeps) != 1 {
		t.Errorf("expected 1 sweep, got %d", len(sweeps))
	}
}

func TestSweepStore_DeleteSweep(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Insert a sweep
	record := SweepRecord{
		SweepID:   "sweep-301",
		SensorID:  "sensor-001",
		Mode:      "manual",
		Status:    "completed",
		Request:   json.RawMessage(`{}`),
		StartedAt: time.Now().UTC(),
	}
	if err := store.InsertSweep(record); err != nil {
		t.Fatalf("InsertSweep failed: %v", err)
	}

	// Delete the sweep
	if err := store.DeleteSweep("sweep-301"); err != nil {
		t.Fatalf("DeleteSweep failed: %v", err)
	}

	// Verify it's gone
	retrieved, err := store.GetSweep("sweep-301")
	if err != nil {
		t.Fatalf("GetSweep failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected sweep to be deleted")
	}
}

func TestSweepStore_GetSweep_NotFound(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Get non-existent sweep
	retrieved, err := store.GetSweep("non-existent")
	if err != nil {
		t.Fatalf("GetSweep failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil for non-existent sweep")
	}
}

func TestSweepStore_NullableFields(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Insert sweep with minimal fields
	record := SweepRecord{
		SweepID:   "sweep-401",
		SensorID:  "sensor-001",
		Mode:      "manual",
		Status:    "running",
		Request:   json.RawMessage(`{}`),
		StartedAt: time.Now().UTC(),
	}
	if err := store.InsertSweep(record); err != nil {
		t.Fatalf("InsertSweep failed: %v", err)
	}

	// Retrieve and verify nullable fields are nil/empty
	retrieved, err := store.GetSweep("sweep-401")
	if err != nil {
		t.Fatalf("GetSweep failed: %v", err)
	}

	if retrieved.Results != nil {
		t.Error("expected results to be nil")
	}
	if retrieved.Charts != nil {
		t.Error("expected charts to be nil")
	}
	if retrieved.Recommendation != nil {
		t.Error("expected recommendation to be nil")
	}
	if retrieved.RoundResults != nil {
		t.Error("expected round_results to be nil")
	}
	if retrieved.Error != "" {
		t.Error("expected error to be empty")
	}
	if retrieved.CompletedAt != nil {
		t.Error("expected completed_at to be nil")
	}
}

func TestSweepStore_GetSweep_InvalidTimeFormat(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// SQLite DATETIME type with invalid input will store as TEXT without conversion
	// We need to store a string that SQLite won't auto-convert but is not RFC3339
	// Use a string type column to bypass SQLite's DATETIME type affinity
	// Recreate the table with started_at as TEXT to avoid SQLite DROP COLUMN issues
	_, err := db.Exec(`DROP TABLE lidar_sweeps`)
	if err != nil {
		t.Fatalf("failed to drop lidar_sweeps table: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE lidar_sweeps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sweep_id TEXT NOT NULL UNIQUE,
			sensor_id TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT 'sweep',
			status TEXT NOT NULL DEFAULT 'running',
			request TEXT NOT NULL,
			results TEXT,
			charts TEXT,
			recommendation TEXT,
			round_results TEXT,
			error TEXT,
			started_at TEXT NOT NULL,
			completed_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			objective_name TEXT,
			objective_version TEXT,
			transform_pipeline_name TEXT,
			transform_pipeline_version TEXT,
			score_components_json TEXT,
			recommendation_explanation_json TEXT,
			label_provenance_summary_json TEXT
		)
	`)
	if err != nil {
		t.Fatalf("failed to recreate lidar_sweeps table with TEXT started_at: %v", err)
	}

	// Insert with an invalid RFC3339 format that SQLite won't convert
	_, err = db.Exec(`
		INSERT INTO lidar_sweeps (sweep_id, sensor_id, mode, status, request, started_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "sweep-bad-start", "sensor-001", "manual", "running", `{}`, "2024/01/01 12:00:00")
	if err != nil {
		t.Fatalf("failed to insert test record: %v", err)
	}

	// GetSweep should return an error for invalid started_at
	_, err = store.GetSweep("sweep-bad-start")
	if err == nil {
		t.Error("expected error when parsing invalid started_at, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "parsing started_at") {
		t.Errorf("expected parsing error, got: %v", err)
	}
}

func TestSweepStore_GetSweep_InvalidCompletedAtFormat(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Recreate the table with completed_at as TEXT to avoid SQLite DROP COLUMN issues
	_, err := db.Exec(`DROP TABLE lidar_sweeps`)
	if err != nil {
		t.Fatalf("failed to drop lidar_sweeps table: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE lidar_sweeps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sweep_id TEXT NOT NULL UNIQUE,
			sensor_id TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT 'sweep',
			status TEXT NOT NULL DEFAULT 'running',
			request TEXT NOT NULL,
			results TEXT,
			charts TEXT,
			recommendation TEXT,
			round_results TEXT,
			error TEXT,
			started_at DATETIME NOT NULL,
			completed_at TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			objective_name TEXT,
			objective_version TEXT,
			transform_pipeline_name TEXT,
			transform_pipeline_version TEXT,
			score_components_json TEXT,
			recommendation_explanation_json TEXT,
			label_provenance_summary_json TEXT
		)
	`)
	if err != nil {
		t.Fatalf("failed to recreate lidar_sweeps table with TEXT completed_at: %v", err)
	}

	// Insert with valid started_at but invalid completed_at format
	_, err = db.Exec(`
		INSERT INTO lidar_sweeps (sweep_id, sensor_id, mode, status, request, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "sweep-bad-complete", "sensor-001", "manual", "completed", `{}`,
		time.Now().UTC().Format(time.RFC3339), "01/02/2024 15:04:05")
	if err != nil {
		t.Fatalf("failed to insert test record: %v", err)
	}

	// GetSweep should return an error for invalid completed_at
	_, err = store.GetSweep("sweep-bad-complete")
	if err == nil {
		t.Error("expected error when parsing invalid completed_at, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "parsing completed_at") {
		t.Errorf("expected parsing error, got: %v", err)
	}
}

func TestSweepStore_ListSweeps_InvalidTimeFormat(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Insert a valid sweep first
	validRecord := SweepRecord{
		SweepID:   "sweep-valid",
		SensorID:  "sensor-001",
		Mode:      "manual",
		Status:    "completed",
		Request:   json.RawMessage(`{}`),
		StartedAt: time.Now().UTC(),
	}
	if err := store.InsertSweep(validRecord); err != nil {
		t.Fatalf("InsertSweep failed: %v", err)
	}

	// Alter the table to use TEXT for started_at to prevent SQLite conversion
	// We'll create a new table with the modified schema
	_, err := db.Exec(`
		CREATE TABLE lidar_sweeps_temp (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sweep_id TEXT NOT NULL UNIQUE,
			sensor_id TEXT NOT NULL,
			mode TEXT NOT NULL DEFAULT 'sweep',
			status TEXT NOT NULL DEFAULT 'running',
			request TEXT NOT NULL,
			results TEXT,
			charts TEXT,
			recommendation TEXT,
			round_results TEXT,
			error TEXT,
			started_at TEXT NOT NULL,
			completed_at TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			objective_name TEXT,
			objective_version TEXT,
			transform_pipeline_name TEXT,
			transform_pipeline_version TEXT,
			score_components_json TEXT,
			recommendation_explanation_json TEXT,
			label_provenance_summary_json TEXT
		)
	`)
	if err != nil {
		t.Fatalf("failed to create temp table: %v", err)
	}

	// Copy existing data
	_, err = db.Exec(`INSERT INTO lidar_sweeps_temp SELECT * FROM lidar_sweeps`)
	if err != nil {
		t.Fatalf("failed to copy data: %v", err)
	}

	// Drop old table and rename
	_, err = db.Exec(`DROP TABLE lidar_sweeps`)
	if err != nil {
		t.Fatalf("failed to drop old table: %v", err)
	}
	_, err = db.Exec(`ALTER TABLE lidar_sweeps_temp RENAME TO lidar_sweeps`)
	if err != nil {
		t.Fatalf("failed to rename table: %v", err)
	}

	// Insert a sweep with invalid started_at format
	_, err = db.Exec(`
		INSERT INTO lidar_sweeps (sweep_id, sensor_id, mode, status, request, started_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "sweep-bad-list", "sensor-001", "manual", "running", `{}`, "2024/01/01 10:00:00")
	if err != nil {
		t.Fatalf("failed to insert test record: %v", err)
	}

	// ListSweeps should return an error when encountering invalid time
	_, err = store.ListSweeps("sensor-001", 10)
	if err == nil {
		t.Error("expected error when parsing invalid started_at in list, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "parsing started_at") {
		t.Errorf("expected parsing error, got: %v", err)
	}
}

func TestSweepStore_SaveSweepStart(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Test SaveSweepStart interface method
	startedAt := time.Now().UTC()
	request := json.RawMessage(`{"param":"closeness_multiplier"}`)
	err := store.SaveSweepStart("sweep-501", "sensor-001", "auto", request, startedAt, "acceptance", "v1")
	if err != nil {
		t.Fatalf("SaveSweepStart failed: %v", err)
	}

	// Verify
	retrieved, err := store.GetSweep("sweep-501")
	if err != nil {
		t.Fatalf("GetSweep failed: %v", err)
	}
	if retrieved.Status != "running" {
		t.Errorf("expected status running, got %s", retrieved.Status)
	}
	if retrieved.Mode != "auto" {
		t.Errorf("expected mode auto, got %s", retrieved.Mode)
	}
}

func TestSweepStore_SaveSweepComplete(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Insert initial sweep
	startedAt := time.Now().UTC()
	record := SweepRecord{
		SweepID:   "sweep-502",
		SensorID:  "sensor-001",
		Mode:      "auto",
		Status:    "running",
		Request:   json.RawMessage(`{}`),
		StartedAt: startedAt,
	}
	if err := store.InsertSweep(record); err != nil {
		t.Fatalf("InsertSweep failed: %v", err)
	}

	// Test SaveSweepComplete interface method
	completedAt := time.Now().UTC()
	results := json.RawMessage(`{"optimal_value":1.5}`)
	recommendation := json.RawMessage(`{"param":"closeness_multiplier","value":1.5}`)
	roundResults := json.RawMessage(`[{"round":1,"value":1.5}]`)

	err := store.SaveSweepComplete("sweep-502", "completed", results, recommendation, roundResults, completedAt, "", nil, nil, nil, "", "")
	if err != nil {
		t.Fatalf("SaveSweepComplete failed: %v", err)
	}

	// Verify
	retrieved, err := store.GetSweep("sweep-502")
	if err != nil {
		t.Fatalf("GetSweep failed: %v", err)
	}
	if retrieved.Status != "completed" {
		t.Errorf("expected status completed, got %s", retrieved.Status)
	}
	if string(retrieved.Results) != string(results) {
		t.Errorf("results mismatch")
	}
}
