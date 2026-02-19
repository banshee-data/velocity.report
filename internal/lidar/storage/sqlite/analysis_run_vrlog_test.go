package sqlite

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupTestAnalysisDB creates a temporary SQLite database for testing.
func setupTestAnalysisDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "analysis-run-test")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open database: %v", err)
	}

	// Apply essential PRAGMAs
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			os.RemoveAll(tmpDir)
			t.Fatalf("failed to execute %q: %v", pragma, err)
		}
	}

	// Create the analysis runs table
	schema := `
		CREATE TABLE IF NOT EXISTS lidar_analysis_runs (
			run_id TEXT PRIMARY KEY,
			created_at INTEGER NOT NULL,
			source_type TEXT NOT NULL,
			source_path TEXT,
			sensor_id TEXT NOT NULL,
			params_json TEXT NOT NULL,
			duration_secs REAL DEFAULT 0,
			total_frames INTEGER DEFAULT 0,
			total_clusters INTEGER DEFAULT 0,
			total_tracks INTEGER DEFAULT 0,
			confirmed_tracks INTEGER DEFAULT 0,
			processing_time_ms INTEGER DEFAULT 0,
			status TEXT NOT NULL,
			error_message TEXT,
			parent_run_id TEXT,
			notes TEXT,
			vrlog_path TEXT
		);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create table: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

// TestAnalysisRunStore_VRLogPath tests CRUD operations with vrlog_path.
func TestAnalysisRunStore_VRLogPath(t *testing.T) {
	db, cleanup := setupTestAnalysisDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Create test params
	params := DefaultRunParams()
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal params: %v", err)
	}

	// Test 1: Insert run with vrlog_path
	run := &AnalysisRun{
		RunID:      "test-run-1",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SourcePath: "/path/to/test.pcap",
		SensorID:   "hesai-01",
		ParamsJSON: paramsJSON,
		Status:     "running",
		VRLogPath:  "/var/lib/velocity-report/vrlog/test-run-1",
	}

	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	// Test 2: Get run and verify vrlog_path
	retrieved, err := store.GetRun("test-run-1")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if retrieved.VRLogPath != run.VRLogPath {
		t.Errorf("VRLogPath mismatch: got %q, want %q", retrieved.VRLogPath, run.VRLogPath)
	}

	// Test 3: Insert run without vrlog_path
	run2 := &AnalysisRun{
		RunID:      "test-run-2",
		CreatedAt:  time.Now(),
		SourceType: "live",
		SensorID:   "hesai-01",
		ParamsJSON: paramsJSON,
		Status:     "running",
	}

	if err := store.InsertRun(run2); err != nil {
		t.Fatalf("InsertRun without vrlog_path failed: %v", err)
	}

	retrieved2, err := store.GetRun("test-run-2")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if retrieved2.VRLogPath != "" {
		t.Errorf("expected empty VRLogPath, got %q", retrieved2.VRLogPath)
	}

	// Test 4: Update vrlog_path
	newPath := "/var/lib/velocity-report/vrlog/test-run-2"
	if err := store.UpdateRunVRLogPath("test-run-2", newPath); err != nil {
		t.Fatalf("UpdateRunVRLogPath failed: %v", err)
	}

	retrieved3, err := store.GetRun("test-run-2")
	if err != nil {
		t.Fatalf("GetRun after update failed: %v", err)
	}

	if retrieved3.VRLogPath != newPath {
		t.Errorf("VRLogPath after update mismatch: got %q, want %q", retrieved3.VRLogPath, newPath)
	}

	// Test 5: Clear vrlog_path by setting empty string
	if err := store.UpdateRunVRLogPath("test-run-2", ""); err != nil {
		t.Fatalf("UpdateRunVRLogPath with empty string failed: %v", err)
	}

	retrieved4, err := store.GetRun("test-run-2")
	if err != nil {
		t.Fatalf("GetRun after clear failed: %v", err)
	}

	if retrieved4.VRLogPath != "" {
		t.Errorf("expected empty VRLogPath after clear, got %q", retrieved4.VRLogPath)
	}
}

// TestAnalysisRunStore_ListRuns_VRLogPath tests that ListRuns includes vrlog_path.
func TestAnalysisRunStore_ListRuns_VRLogPath(t *testing.T) {
	db, cleanup := setupTestAnalysisDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	params := DefaultRunParams()
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal params: %v", err)
	}

	// Insert multiple runs
	runs := []*AnalysisRun{
		{
			RunID:      "run-with-vrlog",
			CreatedAt:  time.Now().Add(-2 * time.Hour),
			SourceType: "pcap",
			SensorID:   "hesai-01",
			ParamsJSON: paramsJSON,
			Status:     "completed",
			VRLogPath:  "/path/to/run-with-vrlog",
		},
		{
			RunID:      "run-without-vrlog",
			CreatedAt:  time.Now().Add(-1 * time.Hour),
			SourceType: "pcap",
			SensorID:   "hesai-01",
			ParamsJSON: paramsJSON,
			Status:     "completed",
		},
		{
			RunID:      "recent-run",
			CreatedAt:  time.Now(),
			SourceType: "pcap",
			SensorID:   "hesai-01",
			ParamsJSON: paramsJSON,
			Status:     "running",
			VRLogPath:  "/path/to/recent-run",
		},
	}

	for _, run := range runs {
		if err := store.InsertRun(run); err != nil {
			t.Fatalf("InsertRun failed: %v", err)
		}
	}

	// List runs
	listed, err := store.ListRuns(10)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}

	if len(listed) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(listed))
	}

	// Verify vrlog_path values (ordered by created_at DESC)
	expectedVRLogPaths := map[string]string{
		"run-with-vrlog":    "/path/to/run-with-vrlog",
		"run-without-vrlog": "",
		"recent-run":        "/path/to/recent-run",
	}

	for _, run := range listed {
		expected := expectedVRLogPaths[run.RunID]
		if run.VRLogPath != expected {
			t.Errorf("run %s: VRLogPath mismatch: got %q, want %q", run.RunID, run.VRLogPath, expected)
		}
	}
}

// TestAnalysisRunStore_UpdateRunVRLogPath_NonExistent tests updating a non-existent run.
func TestAnalysisRunStore_UpdateRunVRLogPath_NonExistent(t *testing.T) {
	db, cleanup := setupTestAnalysisDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Update non-existent run (should not error, just no rows affected)
	if err := store.UpdateRunVRLogPath("non-existent", "/some/path"); err != nil {
		t.Errorf("UpdateRunVRLogPath for non-existent run should not error: %v", err)
	}
}
