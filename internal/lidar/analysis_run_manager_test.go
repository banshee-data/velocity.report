package lidar

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupAnalysisRunDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "analysis-run-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create required tables for analysis runs
	createSQL := `
CREATE TABLE IF NOT EXISTS lidar_analysis_runs (
run_id TEXT PRIMARY KEY,
created_at INTEGER NOT NULL,
source_type TEXT NOT NULL,
source_path TEXT,
sensor_id TEXT NOT NULL,
params_json TEXT NOT NULL,
duration_secs REAL,
total_frames INTEGER,
total_clusters INTEGER,
total_tracks INTEGER,
confirmed_tracks INTEGER,
processing_time_ms INTEGER,
status TEXT NOT NULL,
error_message TEXT,
parent_run_id TEXT,
notes TEXT
);

		CREATE TABLE IF NOT EXISTS lidar_run_tracks (
		run_id TEXT NOT NULL,
		track_id TEXT NOT NULL,
		sensor_id TEXT NOT NULL,
		track_state TEXT NOT NULL,
		start_unix_nanos INTEGER NOT NULL,
		end_unix_nanos INTEGER,
		observation_count INTEGER NOT NULL,
		avg_speed_mps REAL,
		peak_speed_mps REAL,
		p50_speed_mps REAL,
		p85_speed_mps REAL,
		p95_speed_mps REAL,
		bounding_box_length_avg REAL,
		bounding_box_width_avg REAL,
		bounding_box_height_avg REAL,
		height_p95_max REAL,
		intensity_mean_avg REAL,
		object_class TEXT,
		object_confidence REAL,
		classification_model TEXT,
		user_label TEXT,
		label_confidence REAL,
		labeler_id TEXT,
		labeled_at INTEGER,
		is_split_candidate INTEGER,
		is_merge_candidate INTEGER,
		linked_track_ids TEXT,
		PRIMARY KEY (run_id, track_id)
		);
`

	if _, err := db.Exec(createSQL); err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create tables: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

func TestNewAnalysisRunManager(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	if manager == nil {
		t.Fatal("NewAnalysisRunManager returned nil")
	}

	if manager.sensorID != "test-sensor" {
		t.Errorf("Expected sensorID 'test-sensor', got %s", manager.sensorID)
	}

	if manager.store == nil {
		t.Error("Expected store to be initialized")
	}

	if manager.tracksSeen == nil {
		t.Error("Expected tracksSeen map to be initialized")
	}

	if manager.currentRun != nil {
		t.Error("Expected currentRun to be nil initially")
	}
}

func TestAnalysisRunManagerRegistry(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	// Clear registry for test isolation
	armMu.Lock()
	armRegistry = make(map[string]*AnalysisRunManager)
	armMu.Unlock()

	manager := NewAnalysisRunManager(db, "sensor-1")
	RegisterAnalysisRunManager("sensor-1", manager)

	retrieved := GetAnalysisRunManager("sensor-1")
	if retrieved == nil {
		t.Fatal("GetAnalysisRunManager returned nil")
	}

	if retrieved != manager {
		t.Error("Retrieved manager is not the same instance")
	}

	// Test non-existent sensor
	notFound := GetAnalysisRunManager("non-existent")
	if notFound != nil {
		t.Error("Expected nil for non-existent sensor")
	}
}

func TestStartRun(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	runID, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	if runID == "" {
		t.Error("Expected non-empty run ID")
	}

	// Verify run is active
	if !manager.IsRunActive() {
		t.Error("Expected run to be active after StartRun")
	}

	// Verify current run ID matches
	currentID := manager.CurrentRunID()
	if currentID != runID {
		t.Errorf("CurrentRunID mismatch: got %s, want %s", currentID, runID)
	}

	// Verify run is in database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_analysis_runs WHERE run_id = ?", runID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 run in database, got %d", count)
	}

	// Verify params are stored
	var paramsJSON string
	err = db.QueryRow("SELECT params_json FROM lidar_analysis_runs WHERE run_id = ?", runID).Scan(&paramsJSON)
	if err != nil {
		t.Fatalf("Failed to query params: %v", err)
	}

	var storedParams RunParams
	if err := json.Unmarshal([]byte(paramsJSON), &storedParams); err != nil {
		t.Errorf("Failed to unmarshal stored params: %v", err)
	}
}

func TestRecordFrame(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	_, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Record frames
	for i := 0; i < 100; i++ {
		manager.RecordFrame()
	}

	// Verify internal counter
	manager.mu.RLock()
	frameCount := manager.totalFrames
	manager.mu.RUnlock()

	if frameCount != 100 {
		t.Errorf("Expected 100 frames, got %d", frameCount)
	}
}

func TestRecordClusters(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	_, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Record clusters
	manager.RecordClusters(5)
	manager.RecordClusters(3)
	manager.RecordClusters(7)

	// Verify internal counter
	manager.mu.RLock()
	clusterCount := manager.totalClusters
	manager.mu.RUnlock()

	if clusterCount != 15 {
		t.Errorf("Expected 15 clusters, got %d", clusterCount)
	}
}

func TestRecordTrack(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	runID, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Create a test track
	track := &TrackedObject{
		TrackID:           "track-001",
		SensorID:          "test-sensor",
		State:             TrackConfirmed,
		FirstUnixNanos:    time.Now().UnixNano(),
		LastUnixNanos:     time.Now().Add(5 * time.Second).UnixNano(),
		ObservationCount:  50,
		AvgSpeedMps:       10.5,
		PeakSpeedMps:      15.2,
		TrackLengthMeters: 52.5,
		TrackDurationSecs: 5.0,
		OcclusionCount:    2,
		ObjectClass:       "vehicle",
		ObjectConfidence:  0.85,
	}

	// Record track - first time should return true
	isNew := manager.RecordTrack(track)
	if !isNew {
		t.Error("Expected RecordTrack to return true for new track")
	}

	// Record same track again - should return false
	isNew = manager.RecordTrack(track)
	if isNew {
		t.Error("Expected RecordTrack to return false for duplicate track")
	}

	// Verify track is in database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_run_tracks WHERE run_id = ? AND track_id = ?",
		runID, track.TrackID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 track in database, got %d", count)
	}

	// Verify internal tracking
	manager.mu.RLock()
	seen := manager.tracksSeen[track.TrackID]
	manager.mu.RUnlock()

	if !seen {
		t.Error("Expected track to be marked as seen")
	}
}

func TestCompleteRun(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	runID, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Record some activity
	for i := 0; i < 100; i++ {
		manager.RecordFrame()
	}
	manager.RecordClusters(50)

	// Record a few tracks
	for i := 0; i < 5; i++ {
		track := &TrackedObject{
			TrackID:           "track-" + string(rune('A'+i)),
			SensorID:          "test-sensor",
			State:             TrackConfirmed,
			FirstUnixNanos:    time.Now().UnixNano(),
			LastUnixNanos:     time.Now().Add(time.Second).UnixNano(),
			ObservationCount:  10,
			TrackLengthMeters: 10.0,
			TrackDurationSecs: 1.0,
		}
		manager.RecordTrack(track)
	}

	// Sleep briefly to ensure measurable duration
	time.Sleep(10 * time.Millisecond)

	// Complete the run
	err = manager.CompleteRun()
	if err != nil {
		t.Fatalf("CompleteRun failed: %v", err)
	}

	// Verify run is no longer active
	if manager.IsRunActive() {
		t.Error("Expected run to be inactive after CompleteRun")
	}

	if manager.CurrentRunID() != "" {
		t.Error("Expected CurrentRunID to be empty after CompleteRun")
	}

	// Verify database status
	var status string
	var totalFrames, totalClusters, totalTracks int
	var durationSecs float64

	err = db.QueryRow(`
SELECT status, total_frames, total_clusters, total_tracks, duration_secs 
FROM lidar_analysis_runs WHERE run_id = ?`, runID).Scan(
		&status, &totalFrames, &totalClusters, &totalTracks, &durationSecs)
	if err != nil {
		t.Fatalf("Failed to query completed run: %v", err)
	}

	if status != "completed" {
		t.Errorf("Expected status 'completed', got %s", status)
	}

	if totalFrames != 100 {
		t.Errorf("Expected 100 frames, got %d", totalFrames)
	}

	if totalClusters != 50 {
		t.Errorf("Expected 50 clusters, got %d", totalClusters)
	}

	if totalTracks != 5 {
		t.Errorf("Expected 5 tracks, got %d", totalTracks)
	}

	if durationSecs <= 0 {
		t.Errorf("Expected positive duration, got %f", durationSecs)
	}
}

func TestFailRun(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()

	runID, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Fail the run
	errMsg := "test error: file not found"
	err = manager.FailRun(errMsg)
	if err != nil {
		t.Fatalf("FailRun failed: %v", err)
	}

	// Verify run is no longer active
	if manager.IsRunActive() {
		t.Error("Expected run to be inactive after FailRun")
	}

	// Verify database status
	var status, storedErrMsg string
	err = db.QueryRow("SELECT status, error_message FROM lidar_analysis_runs WHERE run_id = ?", runID).Scan(&status, &storedErrMsg)
	if err != nil {
		t.Fatalf("Failed to query failed run: %v", err)
	}

	if status != "failed" {
		t.Errorf("Expected status 'failed', got %s", status)
	}

	if storedErrMsg != errMsg {
		t.Errorf("Expected error_message '%s', got '%s'", errMsg, storedErrMsg)
	}
}

func TestGetCurrentRunParams(t *testing.T) {
	db, cleanup := setupAnalysisRunDB(t)
	defer cleanup()

	manager := NewAnalysisRunManager(db, "test-sensor")
	params := DefaultRunParams()
	params.Background.BackgroundUpdateFraction = 0.123

	// Before starting run
	_, ok := manager.GetCurrentRunParams()
	if ok {
		t.Error("Expected GetCurrentRunParams to return false when no run is active")
	}

	// Start run
	_, err := manager.StartRun("/path/to/test.pcap", params)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Get params
	retrievedParams, ok := manager.GetCurrentRunParams()
	if !ok {
		t.Fatal("Expected GetCurrentRunParams to return true when run is active")
	}

	if retrievedParams.Background.BackgroundUpdateFraction != 0.123 {
		t.Errorf("Expected BackgroundUpdateFraction 0.123, got %f",
			retrievedParams.Background.BackgroundUpdateFraction)
	}
}
