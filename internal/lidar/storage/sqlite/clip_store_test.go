package sqlite

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupTestClipDB creates a test database with the lidar_clips table.
func setupTestClipDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Create lidar_runs table (referenced by FK)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_runs (
			run_id TEXT PRIMARY KEY,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_runs table: %v", err)
	}

	// Create lidar_clips table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_clips (
			clip_id TEXT PRIMARY KEY,
			sensor_id TEXT NOT NULL,
			pcap_start_secs REAL,
			pcap_duration_secs REAL,
			description TEXT,
			reference_run_id TEXT,
			created_at_ns INTEGER NOT NULL,
			updated_at_ns INTEGER,
			recommended_param_set_id TEXT,
			dataset_id TEXT,
			north_azimuth_deg REAL,
			storage TEXT NOT NULL DEFAULT 'sources',
			session_id TEXT,
			source_period_id TEXT,
			FOREIGN KEY (reference_run_id) REFERENCES lidar_runs(run_id) ON DELETE SET NULL
		);

	CREATE TABLE IF NOT EXISTS lidar_clip_captures (
		clip_id TEXT NOT NULL,
		ordinal INTEGER NOT NULL,
		capture_file_id TEXT,
		pcap_file TEXT NOT NULL,
		PRIMARY KEY (clip_id, ordinal)
	)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_clips table: %v", err)
	}

	// Create lidar_param_sets table (required for recommended param set normalisation)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_param_sets (
			param_set_id TEXT PRIMARY KEY,
			params_hash TEXT UNIQUE NOT NULL,
			schema_version TEXT NOT NULL,
			param_set_type TEXT NOT NULL,
			params_json TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_param_sets table: %v", err)
	}

	return db
}

func TestClipStore_InsertAndGet(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()

	store := NewClipStore(db)

	// Create a clip
	startSecs := 10.5
	durationSecs := 30.0
	clip := &Clip{
		SensorID: "sensor-001", Files: []ClipCapture{{PCAPFile: "test.pcap"}}, PCAPStartSecs: &startSecs,
		PCAPDurationSecs: &durationSecs,
		Description:      "Test clip",
	}

	err := store.InsertClip(clip)
	if err != nil {
		t.Fatalf("InsertClip failed: %v", err)
	}

	// Clip ID should be auto-generated
	if clip.ClipID == "" {
		t.Error("expected clip_id to be generated")
	}

	// Verify created_at_ns was set
	if clip.CreatedAtNs == 0 {
		t.Error("expected created_at_ns to be set")
	}

	// Retrieve the clip
	retrieved, err := store.GetClip(clip.ClipID)
	if err != nil {
		t.Fatalf("GetClip failed: %v", err)
	}

	// Verify fields
	if retrieved.SensorID != clip.SensorID {
		t.Errorf("sensor_id mismatch: got %s, want %s", retrieved.SensorID, clip.SensorID)
	}
	if retrieved.Files[0].PCAPFile != clip.Files[0].PCAPFile {
		t.Errorf("pcap_file mismatch: got %s, want %s", retrieved.Files[0].PCAPFile, clip.Files[0].PCAPFile)
	}
	if retrieved.PCAPStartSecs == nil || *retrieved.PCAPStartSecs != startSecs {
		t.Errorf("pcap_start_secs mismatch: got %v, want %f", retrieved.PCAPStartSecs, startSecs)
	}
	if retrieved.PCAPDurationSecs == nil || *retrieved.PCAPDurationSecs != durationSecs {
		t.Errorf("pcap_duration_secs mismatch: got %v, want %f", retrieved.PCAPDurationSecs, durationSecs)
	}
	if retrieved.Description != clip.Description {
		t.Errorf("description mismatch: got %s, want %s", retrieved.Description, clip.Description)
	}
}

// TestClipStore_FileCount checks that GetClip and ListClips both report how
// many captures a case covers without the caller having to load the file list
// itself — a legacy case with no lidar_clip_captures rows still counts as
// 1 via its pcap_file projection, and a multi-file case counts every one.
func TestClipStore_FileCount(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()

	store := NewClipStore(db)

	legacy := &Clip{SensorID: "sensor-001", Files: []ClipCapture{{PCAPFile: "solo.pcap"}}}
	if err := store.InsertClip(legacy); err != nil {
		t.Fatalf("InsertClip failed: %v", err)
	}

	multi := &Clip{SensorID: "sensor-001", Files: []ClipCapture{{PCAPFile: "part-0.pcap"}}}
	if err := store.InsertClip(multi); err != nil {
		t.Fatalf("InsertClip failed: %v", err)
	}
	files := []ClipCapture{
		{Ordinal: 0, PCAPFile: "part-0.pcap"},
		{Ordinal: 1, PCAPFile: "part-1.pcap"},
		{Ordinal: 2, PCAPFile: "part-2.pcap"},
	}
	if err := store.SetClipFiles(multi.ClipID, files); err != nil {
		t.Fatalf("SetClipFiles failed: %v", err)
	}

	gotLegacy, err := store.GetClip(legacy.ClipID)
	if err != nil {
		t.Fatalf("GetClip(legacy) failed: %v", err)
	}
	if gotLegacy.FileCount != 1 {
		t.Errorf("legacy case file_count: got %d, want 1", gotLegacy.FileCount)
	}

	gotMulti, err := store.GetClip(multi.ClipID)
	if err != nil {
		t.Fatalf("GetClip(multi) failed: %v", err)
	}
	if gotMulti.FileCount != 3 {
		t.Errorf("multi-file case file_count: got %d, want 3", gotMulti.FileCount)
	}

	listed, err := store.ListClips("")
	if err != nil {
		t.Fatalf("ListClips failed: %v", err)
	}
	counts := map[string]int{}
	for _, s := range listed {
		counts[s.ClipID] = s.FileCount
	}
	if counts[legacy.ClipID] != 1 {
		t.Errorf("ListClips legacy file_count: got %d, want 1", counts[legacy.ClipID])
	}
	if counts[multi.ClipID] != 3 {
		t.Errorf("ListClips multi-file file_count: got %d, want 3", counts[multi.ClipID])
	}
}

func TestClipStore_ListClips(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()

	store := NewClipStore(db)

	// Insert multiple clips
	clips := []*Clip{
		{SensorID: "sensor-001", Files: []ClipCapture{{PCAPFile: "clip1.pcap"}}, Description: "Clip 1"},
		{SensorID: "sensor-001", Files: []ClipCapture{{PCAPFile: "clip2.pcap"}}, Description: "Clip 2"},
		{SensorID: "sensor-002", Files: []ClipCapture{{PCAPFile: "clip3.pcap"}}, Description: "Clip 3"},
	}

	for _, clip := range clips {
		if err := store.InsertClip(clip); err != nil {
			t.Fatalf("InsertClip failed: %v", err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
	}

	// List all clips
	allClips, err := store.ListClips("")
	if err != nil {
		t.Fatalf("ListClips failed: %v", err)
	}
	if len(allClips) != 3 {
		t.Errorf("expected 3 clips, got %d", len(allClips))
	}

	// List clips for sensor-001
	sensor001Clips, err := store.ListClips("sensor-001")
	if err != nil {
		t.Fatalf("ListClips(sensor-001) failed: %v", err)
	}
	if len(sensor001Clips) != 2 {
		t.Errorf("expected 2 clips for sensor-001, got %d", len(sensor001Clips))
	}

	// Verify ordering (newest first)
	if sensor001Clips[0].Description != "Clip 2" {
		t.Errorf("expected newest clip first, got %s", sensor001Clips[0].Description)
	}
}

func TestClipStore_UpdateClip(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()

	store := NewClipStore(db)

	// Insert a clip
	clip := &Clip{
		SensorID: "sensor-001", Files: []ClipCapture{{PCAPFile: "test.pcap"}}, Description: "Original description",
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("InsertClip failed: %v", err)
	}

	// Update the clip
	clip.Description = "Updated description"
	clip.ReferenceRunID = "run-123"
	clip.OptimalParamsJSON = json.RawMessage(`{"eps": 1.0}`)

	if err := store.UpdateClip(clip); err != nil {
		t.Fatalf("UpdateClip failed: %v", err)
	}

	// Verify updated_at_ns was set
	if clip.UpdatedAtNs == nil || *clip.UpdatedAtNs == 0 {
		t.Error("expected updated_at_ns to be set")
	}

	// Retrieve and verify
	retrieved, err := store.GetClip(clip.ClipID)
	if err != nil {
		t.Fatalf("GetClip failed: %v", err)
	}

	if retrieved.Description != "Updated description" {
		t.Errorf("description not updated: got %s", retrieved.Description)
	}
	if retrieved.ReferenceRunID != "run-123" {
		t.Errorf("reference_run_id not updated: got %s", retrieved.ReferenceRunID)
	}
	if retrieved.RecommendedParamSetID == "" {
		t.Error("expected recommended_param_set_id to be set after updating with optimal params")
	}
	if retrieved.UpdatedAtNs == nil {
		t.Error("updated_at_ns should be set")
	}
}

func TestClipStore_DeleteClip(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()

	store := NewClipStore(db)

	// Insert a clip
	clip := &Clip{
		SensorID: "sensor-001", Files: []ClipCapture{{PCAPFile: "test.pcap"}},
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("InsertClip failed: %v", err)
	}

	// Delete the clip
	if err := store.DeleteClip(clip.ClipID); err != nil {
		t.Fatalf("DeleteClip failed: %v", err)
	}

	// Verify it's gone
	_, err := store.GetClip(clip.ClipID)
	if err == nil {
		t.Error("expected error when getting deleted clip")
	}

	// Delete non-existent clip should fail
	err = store.DeleteClip("non-existent")
	if err == nil {
		t.Error("expected error when deleting non-existent clip")
	}
}

func TestClipStore_SetReferenceRun(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()

	store := NewClipStore(db)

	// Insert a run
	_, err := db.Exec(`INSERT INTO lidar_runs (run_id) VALUES (?)`, "run-123")
	if err != nil {
		t.Fatalf("failed to insert test run: %v", err)
	}

	// Insert a clip
	clip := &Clip{
		SensorID: "sensor-001", Files: []ClipCapture{{PCAPFile: "test.pcap"}},
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("InsertClip failed: %v", err)
	}

	// Set reference run
	if err := store.SetReferenceRun(clip.ClipID, "run-123"); err != nil {
		t.Fatalf("SetReferenceRun failed: %v", err)
	}

	// Verify
	retrieved, err := store.GetClip(clip.ClipID)
	if err != nil {
		t.Fatalf("GetClip failed: %v", err)
	}
	if retrieved.ReferenceRunID != "run-123" {
		t.Errorf("reference_run_id not set: got %s", retrieved.ReferenceRunID)
	}
	if retrieved.UpdatedAtNs == nil {
		t.Error("updated_at_ns should be set")
	}

	// Set to empty string (clear)
	if err := store.SetReferenceRun(clip.ClipID, ""); err != nil {
		t.Fatalf("SetReferenceRun(empty) failed: %v", err)
	}

	retrieved, err = store.GetClip(clip.ClipID)
	if err != nil {
		t.Fatalf("GetClip failed: %v", err)
	}
	if retrieved.ReferenceRunID != "" {
		t.Errorf("reference_run_id should be cleared: got %s", retrieved.ReferenceRunID)
	}
}

func TestClipStore_SetOptimalParams(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()

	store := NewClipStore(db)

	// Insert a clip
	clip := &Clip{
		SensorID: "sensor-001", Files: []ClipCapture{{PCAPFile: "test.pcap"}},
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("InsertClip failed: %v", err)
	}

	// Set optimal params
	paramsJSON := json.RawMessage(`{"eps": 1.5, "min_pts": 3}`)
	if err := store.SetOptimalParams(clip.ClipID, paramsJSON); err != nil {
		t.Fatalf("SetOptimalParams failed: %v", err)
	}

	// Verify
	retrieved, err := store.GetClip(clip.ClipID)
	if err != nil {
		t.Fatalf("GetClip failed: %v", err)
	}
	if retrieved.RecommendedParamSetID == "" {
		t.Error("expected recommended_param_set_id to be set after SetOptimalParams")
	}
	if retrieved.UpdatedAtNs == nil {
		t.Error("updated_at_ns should be set")
	}
}

func TestClipStore_NullableFields(t *testing.T) {
	db := setupTestClipDB(t)
	defer db.Close()

	store := NewClipStore(db)

	// Insert clip with minimal fields (no optional fields)
	clip := &Clip{
		SensorID: "sensor-001", Files: []ClipCapture{{PCAPFile: "test.pcap"}},
	}
	if err := store.InsertClip(clip); err != nil {
		t.Fatalf("InsertClip failed: %v", err)
	}

	// Retrieve and verify nullable fields are nil
	retrieved, err := store.GetClip(clip.ClipID)
	if err != nil {
		t.Fatalf("GetClip failed: %v", err)
	}

	if retrieved.PCAPStartSecs != nil {
		t.Error("expected pcap_start_secs to be nil")
	}
	if retrieved.PCAPDurationSecs != nil {
		t.Error("expected pcap_duration_secs to be nil")
	}
	if retrieved.Description != "" {
		t.Error("expected description to be empty")
	}
	if retrieved.ReferenceRunID != "" {
		t.Error("expected reference_run_id to be empty")
	}
	if retrieved.RecommendedParamSetID != "" {
		t.Error("expected recommended_param_set_id to be empty")
	}
	if retrieved.UpdatedAtNs != nil {
		t.Error("expected updated_at_ns to be nil")
	}
}
