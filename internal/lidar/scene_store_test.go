package lidar

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// setupTestSceneDB creates a test database with the lidar_scenes table.
func setupTestSceneDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Create lidar_analysis_runs table (referenced by FK)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_analysis_runs (
			run_id TEXT PRIMARY KEY,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_analysis_runs table: %v", err)
	}

	// Create lidar_scenes table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_scenes (
			scene_id TEXT PRIMARY KEY,
			sensor_id TEXT NOT NULL,
			pcap_file TEXT NOT NULL,
			pcap_start_secs REAL,
			pcap_duration_secs REAL,
			description TEXT,
			reference_run_id TEXT,
			optimal_params_json TEXT,
			created_at_ns INTEGER NOT NULL,
			updated_at_ns INTEGER,
			FOREIGN KEY (reference_run_id) REFERENCES lidar_analysis_runs(run_id) ON DELETE SET NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create lidar_scenes table: %v", err)
	}

	return db
}

func TestSceneStore_InsertAndGet(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()

	store := NewSceneStore(db)

	// Create a scene
	startSecs := 10.5
	durationSecs := 30.0
	scene := &Scene{
		SensorID:         "sensor-001",
		PCAPFile:         "test.pcap",
		PCAPStartSecs:    &startSecs,
		PCAPDurationSecs: &durationSecs,
		Description:      "Test scene",
	}

	err := store.InsertScene(scene)
	if err != nil {
		t.Fatalf("InsertScene failed: %v", err)
	}

	// Scene ID should be auto-generated
	if scene.SceneID == "" {
		t.Error("expected scene_id to be generated")
	}

	// Verify created_at_ns was set
	if scene.CreatedAtNs == 0 {
		t.Error("expected created_at_ns to be set")
	}

	// Retrieve the scene
	retrieved, err := store.GetScene(scene.SceneID)
	if err != nil {
		t.Fatalf("GetScene failed: %v", err)
	}

	// Verify fields
	if retrieved.SensorID != scene.SensorID {
		t.Errorf("sensor_id mismatch: got %s, want %s", retrieved.SensorID, scene.SensorID)
	}
	if retrieved.PCAPFile != scene.PCAPFile {
		t.Errorf("pcap_file mismatch: got %s, want %s", retrieved.PCAPFile, scene.PCAPFile)
	}
	if retrieved.PCAPStartSecs == nil || *retrieved.PCAPStartSecs != startSecs {
		t.Errorf("pcap_start_secs mismatch: got %v, want %f", retrieved.PCAPStartSecs, startSecs)
	}
	if retrieved.PCAPDurationSecs == nil || *retrieved.PCAPDurationSecs != durationSecs {
		t.Errorf("pcap_duration_secs mismatch: got %v, want %f", retrieved.PCAPDurationSecs, durationSecs)
	}
	if retrieved.Description != scene.Description {
		t.Errorf("description mismatch: got %s, want %s", retrieved.Description, scene.Description)
	}
}

func TestSceneStore_ListScenes(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()

	store := NewSceneStore(db)

	// Insert multiple scenes
	scenes := []*Scene{
		{SensorID: "sensor-001", PCAPFile: "scene1.pcap", Description: "Scene 1"},
		{SensorID: "sensor-001", PCAPFile: "scene2.pcap", Description: "Scene 2"},
		{SensorID: "sensor-002", PCAPFile: "scene3.pcap", Description: "Scene 3"},
	}

	for _, scene := range scenes {
		if err := store.InsertScene(scene); err != nil {
			t.Fatalf("InsertScene failed: %v", err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(1 * time.Millisecond)
	}

	// List all scenes
	allScenes, err := store.ListScenes("")
	if err != nil {
		t.Fatalf("ListScenes failed: %v", err)
	}
	if len(allScenes) != 3 {
		t.Errorf("expected 3 scenes, got %d", len(allScenes))
	}

	// List scenes for sensor-001
	sensor001Scenes, err := store.ListScenes("sensor-001")
	if err != nil {
		t.Fatalf("ListScenes(sensor-001) failed: %v", err)
	}
	if len(sensor001Scenes) != 2 {
		t.Errorf("expected 2 scenes for sensor-001, got %d", len(sensor001Scenes))
	}

	// Verify ordering (newest first)
	if sensor001Scenes[0].Description != "Scene 2" {
		t.Errorf("expected newest scene first, got %s", sensor001Scenes[0].Description)
	}
}

func TestSceneStore_UpdateScene(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()

	store := NewSceneStore(db)

	// Insert a scene
	scene := &Scene{
		SensorID:    "sensor-001",
		PCAPFile:    "test.pcap",
		Description: "Original description",
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("InsertScene failed: %v", err)
	}

	// Update the scene
	scene.Description = "Updated description"
	scene.ReferenceRunID = "run-123"
	paramsJSON := json.RawMessage(`{"eps": 1.0}`)
	scene.OptimalParamsJSON = paramsJSON

	if err := store.UpdateScene(scene); err != nil {
		t.Fatalf("UpdateScene failed: %v", err)
	}

	// Verify updated_at_ns was set
	if scene.UpdatedAtNs == nil || *scene.UpdatedAtNs == 0 {
		t.Error("expected updated_at_ns to be set")
	}

	// Retrieve and verify
	retrieved, err := store.GetScene(scene.SceneID)
	if err != nil {
		t.Fatalf("GetScene failed: %v", err)
	}

	if retrieved.Description != "Updated description" {
		t.Errorf("description not updated: got %s", retrieved.Description)
	}
	if retrieved.ReferenceRunID != "run-123" {
		t.Errorf("reference_run_id not updated: got %s", retrieved.ReferenceRunID)
	}
	if string(retrieved.OptimalParamsJSON) != string(paramsJSON) {
		t.Errorf("optimal_params_json not updated: got %s", string(retrieved.OptimalParamsJSON))
	}
	if retrieved.UpdatedAtNs == nil {
		t.Error("updated_at_ns should be set")
	}
}

func TestSceneStore_DeleteScene(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()

	store := NewSceneStore(db)

	// Insert a scene
	scene := &Scene{
		SensorID: "sensor-001",
		PCAPFile: "test.pcap",
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("InsertScene failed: %v", err)
	}

	// Delete the scene
	if err := store.DeleteScene(scene.SceneID); err != nil {
		t.Fatalf("DeleteScene failed: %v", err)
	}

	// Verify it's gone
	_, err := store.GetScene(scene.SceneID)
	if err == nil {
		t.Error("expected error when getting deleted scene")
	}

	// Delete non-existent scene should fail
	err = store.DeleteScene("non-existent")
	if err == nil {
		t.Error("expected error when deleting non-existent scene")
	}
}

func TestSceneStore_SetReferenceRun(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()

	store := NewSceneStore(db)

	// Insert a run
	_, err := db.Exec(`INSERT INTO lidar_analysis_runs (run_id) VALUES (?)`, "run-123")
	if err != nil {
		t.Fatalf("failed to insert test run: %v", err)
	}

	// Insert a scene
	scene := &Scene{
		SensorID: "sensor-001",
		PCAPFile: "test.pcap",
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("InsertScene failed: %v", err)
	}

	// Set reference run
	if err := store.SetReferenceRun(scene.SceneID, "run-123"); err != nil {
		t.Fatalf("SetReferenceRun failed: %v", err)
	}

	// Verify
	retrieved, err := store.GetScene(scene.SceneID)
	if err != nil {
		t.Fatalf("GetScene failed: %v", err)
	}
	if retrieved.ReferenceRunID != "run-123" {
		t.Errorf("reference_run_id not set: got %s", retrieved.ReferenceRunID)
	}
	if retrieved.UpdatedAtNs == nil {
		t.Error("updated_at_ns should be set")
	}

	// Set to empty string (clear)
	if err := store.SetReferenceRun(scene.SceneID, ""); err != nil {
		t.Fatalf("SetReferenceRun(empty) failed: %v", err)
	}

	retrieved, err = store.GetScene(scene.SceneID)
	if err != nil {
		t.Fatalf("GetScene failed: %v", err)
	}
	if retrieved.ReferenceRunID != "" {
		t.Errorf("reference_run_id should be cleared: got %s", retrieved.ReferenceRunID)
	}
}

func TestSceneStore_SetOptimalParams(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()

	store := NewSceneStore(db)

	// Insert a scene
	scene := &Scene{
		SensorID: "sensor-001",
		PCAPFile: "test.pcap",
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("InsertScene failed: %v", err)
	}

	// Set optimal params
	paramsJSON := json.RawMessage(`{"eps": 1.5, "min_pts": 3}`)
	if err := store.SetOptimalParams(scene.SceneID, paramsJSON); err != nil {
		t.Fatalf("SetOptimalParams failed: %v", err)
	}

	// Verify
	retrieved, err := store.GetScene(scene.SceneID)
	if err != nil {
		t.Fatalf("GetScene failed: %v", err)
	}
	if string(retrieved.OptimalParamsJSON) != string(paramsJSON) {
		t.Errorf("optimal_params_json not set: got %s", string(retrieved.OptimalParamsJSON))
	}
	if retrieved.UpdatedAtNs == nil {
		t.Error("updated_at_ns should be set")
	}
}

func TestSceneStore_NullableFields(t *testing.T) {
	db := setupTestSceneDB(t)
	defer db.Close()

	store := NewSceneStore(db)

	// Insert scene with minimal fields (no optional fields)
	scene := &Scene{
		SensorID: "sensor-001",
		PCAPFile: "test.pcap",
	}
	if err := store.InsertScene(scene); err != nil {
		t.Fatalf("InsertScene failed: %v", err)
	}

	// Retrieve and verify nullable fields are nil
	retrieved, err := store.GetScene(scene.SceneID)
	if err != nil {
		t.Fatalf("GetScene failed: %v", err)
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
	if len(retrieved.OptimalParamsJSON) > 0 {
		t.Error("expected optimal_params_json to be nil")
	}
	if retrieved.UpdatedAtNs != nil {
		t.Error("expected updated_at_ns to be nil")
	}
}
