package lidar

import (
	"database/sql"
	"encoding/json"
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
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
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

	err := store.UpdateSweepResults("sweep-002", "completed", results, recommendation, roundResults, &completedAt, "")
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

	err := store.UpdateSweepResults("sweep-003", "failed", nil, nil, nil, &completedAt, errMsg)
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

	if len(retrieved.Results) > 0 {
		t.Error("expected results to be nil")
	}
	if len(retrieved.Charts) > 0 {
		t.Error("expected charts to be nil")
	}
	if len(retrieved.Recommendation) > 0 {
		t.Error("expected recommendation to be nil")
	}
	if len(retrieved.RoundResults) > 0 {
		t.Error("expected round_results to be nil")
	}
	if retrieved.Error != "" {
		t.Error("expected error to be empty")
	}
	if retrieved.CompletedAt != nil {
		t.Error("expected completed_at to be nil")
	}
}

func TestSweepStore_SaveSweepStart(t *testing.T) {
	db := setupTestSweepDB(t)
	defer db.Close()

	store := NewSweepStore(db)

	// Test SaveSweepStart interface method
	startedAt := time.Now().UTC()
	request := json.RawMessage(`{"param":"closeness_multiplier"}`)
	err := store.SaveSweepStart("sweep-501", "sensor-001", "auto", request, startedAt)
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

	err := store.SaveSweepComplete("sweep-502", "completed", results, recommendation, roundResults, completedAt, "")
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
