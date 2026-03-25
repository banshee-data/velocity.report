package sqlite

import (
	"database/sql"
	"testing"
	"time"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

// setupTestAnalysisDB creates a test database via the canonical internal/db bootstrap path.
func setupTestAnalysisDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	db, cleanup := dbpkg.NewTestDB(t)
	return db.DB, cleanup
}

// TestAnalysisRunStore_VRLogPath tests CRUD operations with vrlog_path.
func TestAnalysisRunStore_VRLogPath(t *testing.T) {
	db, cleanup := setupTestAnalysisDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Test 1: Insert run with vrlog_path
	run := &AnalysisRun{
		RunID:      "test-run-1",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SourcePath: "/path/to/test.pcap",
		SensorID:   "hesai-01",
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

	// Insert multiple runs
	runs := []*AnalysisRun{
		{
			RunID:      "run-with-vrlog",
			CreatedAt:  time.Now().Add(-2 * time.Hour),
			SourceType: "pcap",
			SensorID:   "hesai-01",
			Status:     "completed",
			VRLogPath:  "/path/to/run-with-vrlog",
		},
		{
			RunID:      "run-without-vrlog",
			CreatedAt:  time.Now().Add(-1 * time.Hour),
			SourceType: "pcap",
			SensorID:   "hesai-01",
			Status:     "completed",
		},
		{
			RunID:      "recent-run",
			CreatedAt:  time.Now(),
			SourceType: "pcap",
			SensorID:   "hesai-01",
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
