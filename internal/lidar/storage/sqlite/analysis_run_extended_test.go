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

// setupAnalysisRunTestDB creates a test database with proper schema from schema.sql.
// This avoids hardcoded CREATE TABLE statements that can get out of sync with migrations.
func setupAnalysisRunTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Apply essential PRAGMAs
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			t.Fatalf("Failed to execute %q: %v", pragma, err)
		}
	}

	// Read and execute schema.sql from the db package
	schemaPath := filepath.Join("..", "..", "..", "db", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to read schema.sql: %v", err)
	}

	if _, err := db.Exec(string(schemaSQL)); err != nil {
		db.Close()
		t.Fatalf("Failed to execute schema.sql: %v", err)
	}

	// Baseline at latest migration version
	// NOTE: Update this when new migrations are added to internal/db/migrations/
	latestMigrationVersion := 15
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (?, false)`, latestMigrationVersion); err != nil {
		db.Close()
		t.Fatalf("Failed to baseline migrations: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

// insertTestAnalysisRun inserts a parent analysis run for foreign key compliance.
func insertTestAnalysisRun(t *testing.T, db *sql.DB, runID, sensorID string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO lidar_analysis_runs (
			run_id, created_at, source_type, source_path, sensor_id, params_json, status
		) VALUES (?, ?, 'pcap', '/test.pcap', ?, '{}', 'completed')
	`, runID, time.Now().UnixNano(), sensorID)
	if err != nil {
		t.Fatalf("Failed to insert test analysis run: %v", err)
	}
}

// TestGetRun tests retrieving an analysis run by ID.
func TestGetRun(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Create and insert a run
	params := DefaultRunParams()
	paramsJSON, _ := params.ToJSON()

	run := &AnalysisRun{
		RunID:           "run-get-test",
		CreatedAt:       time.Now(),
		SourceType:      "pcap",
		SourcePath:      "/path/to/test.pcap",
		SensorID:        "sensor-1",
		ParamsJSON:      paramsJSON,
		DurationSecs:    120.5,
		TotalFrames:     1000,
		TotalClusters:   500,
		TotalTracks:     50,
		ConfirmedTracks: 45,
		Status:          "completed",
		Notes:           "Test run",
	}

	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	// Retrieve the run
	retrieved, err := store.GetRun("run-get-test")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if retrieved.RunID != run.RunID {
		t.Errorf("RunID mismatch: got %s, want %s", retrieved.RunID, run.RunID)
	}
	if retrieved.SourceType != run.SourceType {
		t.Errorf("SourceType mismatch: got %s, want %s", retrieved.SourceType, run.SourceType)
	}
	if retrieved.SourcePath != run.SourcePath {
		t.Errorf("SourcePath mismatch: got %s, want %s", retrieved.SourcePath, run.SourcePath)
	}
	if retrieved.TotalFrames != run.TotalFrames {
		t.Errorf("TotalFrames mismatch: got %d, want %d", retrieved.TotalFrames, run.TotalFrames)
	}
	if retrieved.Notes != run.Notes {
		t.Errorf("Notes mismatch: got %s, want %s", retrieved.Notes, run.Notes)
	}
}

// TestGetRun_NotFound tests error handling for non-existent run.
func TestGetRun_NotFound(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	_, err := store.GetRun("non-existent-run")
	if err == nil {
		t.Error("Expected error for non-existent run")
	}
}

// TestListRuns tests listing analysis runs.
func TestListRuns(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	params := DefaultRunParams()
	paramsJSON, _ := params.ToJSON()

	// Insert multiple runs with different timestamps
	for i := 0; i < 5; i++ {
		run := &AnalysisRun{
			RunID:      "run-list-" + string(rune('a'+i)),
			CreatedAt:  time.Now().Add(time.Duration(i) * time.Hour),
			SourceType: "pcap",
			SensorID:   "sensor-1",
			ParamsJSON: paramsJSON,
			Status:     "completed",
		}
		if err := store.InsertRun(run); err != nil {
			t.Fatalf("InsertRun failed: %v", err)
		}
	}

	// List runs with limit
	runs, err := store.ListRuns(3)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}

	if len(runs) != 3 {
		t.Errorf("Expected 3 runs, got %d", len(runs))
	}

	// Verify descending order by created_at
	if len(runs) >= 2 {
		if runs[0].CreatedAt.Before(runs[1].CreatedAt) {
			t.Error("Runs should be in descending order by created_at")
		}
	}
}

// TestListRuns_Empty tests listing runs when none exist.
func TestListRuns_Empty(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	runs, err := store.ListRuns(10)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}

	if len(runs) != 0 {
		t.Errorf("Expected 0 runs, got %d", len(runs))
	}
}

// TestInsertAndGetRunTracks tests inserting and retrieving run tracks.
func TestInsertAndGetRunTracks(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Insert parent run to satisfy foreign key constraint
	insertTestAnalysisRun(t, db, "run-1", "sensor-1")

	// Insert tracks
	tracks := []*RunTrack{
		{
			RunID:            "run-1",
			TrackID:          "track-1",
			SensorID:         "sensor-1",
			TrackState:       "confirmed",
			StartUnixNanos:   1000,
			EndUnixNanos:     2000,
			ObservationCount: 10,
			AvgSpeedMps:      5.0,
			PeakSpeedMps:     8.0,
			P50SpeedMps:      5.0,
			P85SpeedMps:      6.5,
			P95SpeedMps:      7.5,
			ObjectClass:      "car",
			ObjectConfidence: 0.85,
			LinkedTrackIDs:   []string{"track-2"},
		},
		{
			RunID:            "run-1",
			TrackID:          "track-2",
			SensorID:         "sensor-1",
			TrackState:       "confirmed",
			StartUnixNanos:   1500,
			EndUnixNanos:     2500,
			ObservationCount: 8,
			AvgSpeedMps:      4.0,
		},
	}

	for _, track := range tracks {
		if err := store.InsertRunTrack(track); err != nil {
			t.Fatalf("InsertRunTrack failed: %v", err)
		}
	}

	// Retrieve tracks
	retrieved, err := store.GetRunTracks("run-1")
	if err != nil {
		t.Fatalf("GetRunTracks failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Fatalf("Expected 2 tracks, got %d", len(retrieved))
	}

	// Verify order (by start_unix_nanos)
	if retrieved[0].StartUnixNanos > retrieved[1].StartUnixNanos {
		t.Error("Tracks should be ordered by start_unix_nanos")
	}

	// Verify linked track IDs were persisted
	if len(retrieved[0].LinkedTrackIDs) != 1 || retrieved[0].LinkedTrackIDs[0] != "track-2" {
		t.Errorf("LinkedTrackIDs not properly persisted: %v", retrieved[0].LinkedTrackIDs)
	}
}

// TestGetRunTracks_Empty tests retrieving tracks for a run with no tracks.
func TestGetRunTracks_Empty(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	tracks, err := store.GetRunTracks("non-existent-run")
	if err != nil {
		t.Fatalf("GetRunTracks failed: %v", err)
	}

	if len(tracks) != 0 {
		t.Errorf("Expected 0 tracks, got %d", len(tracks))
	}
}

// TestUpdateTrackLabel tests updating a track's user label.
func TestUpdateTrackLabel(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Insert parent run to satisfy foreign key constraint
	insertTestAnalysisRun(t, db, "run-label", "sensor-1")

	// Insert a track
	track := &RunTrack{
		RunID:          "run-label",
		TrackID:        "track-label",
		SensorID:       "sensor-1",
		TrackState:     "confirmed",
		StartUnixNanos: 1000,
	}
	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("InsertRunTrack failed: %v", err)
	}

	// Update label
	if err := store.UpdateTrackLabel("run-label", "track-label", "car", "", 0.95, "user-123", "human_manual"); err != nil {
		t.Fatalf("UpdateTrackLabel failed: %v", err)
	}

	// Retrieve and verify
	tracks, err := store.GetRunTracks("run-label")
	if err != nil {
		t.Fatalf("GetRunTracks failed: %v", err)
	}

	if len(tracks) != 1 {
		t.Fatalf("Expected 1 track, got %d", len(tracks))
	}

	if tracks[0].UserLabel != "car" {
		t.Errorf("UserLabel mismatch: got %s, want car", tracks[0].UserLabel)
	}
	if tracks[0].LabelConfidence != 0.95 {
		t.Errorf("LabelConfidence mismatch: got %f, want 0.95", tracks[0].LabelConfidence)
	}
	if tracks[0].LabelerID != "user-123" {
		t.Errorf("LabelerID mismatch: got %s, want user-123", tracks[0].LabelerID)
	}
	if tracks[0].LabeledAt == 0 {
		t.Error("LabeledAt should be set")
	}
}

// TestUpdateTrackQualityFlags tests updating split/merge flags.
func TestUpdateTrackQualityFlags(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Insert parent run to satisfy foreign key constraint
	insertTestAnalysisRun(t, db, "run-flags", "sensor-1")

	// Insert a track
	track := &RunTrack{
		RunID:          "run-flags",
		TrackID:        "track-flags",
		SensorID:       "sensor-1",
		TrackState:     "confirmed",
		StartUnixNanos: 1000,
	}
	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("InsertRunTrack failed: %v", err)
	}

	// Update quality flags
	linkedIDs := []string{"track-a", "track-b"}
	if err := store.UpdateTrackQualityFlags("run-flags", "track-flags", true, false, linkedIDs); err != nil {
		t.Fatalf("UpdateTrackQualityFlags failed: %v", err)
	}

	// Retrieve and verify
	tracks, err := store.GetRunTracks("run-flags")
	if err != nil {
		t.Fatalf("GetRunTracks failed: %v", err)
	}

	if len(tracks) != 1 {
		t.Fatalf("Expected 1 track, got %d", len(tracks))
	}

	if !tracks[0].IsSplitCandidate {
		t.Error("IsSplitCandidate should be true")
	}
	if tracks[0].IsMergeCandidate {
		t.Error("IsMergeCandidate should be false")
	}
	if len(tracks[0].LinkedTrackIDs) != 2 {
		t.Errorf("LinkedTrackIDs length mismatch: got %d, want 2", len(tracks[0].LinkedTrackIDs))
	}
}

// TestGetLabelingProgress tests getting labeling statistics.
func TestGetLabelingProgress(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Insert parent run to satisfy foreign key constraint
	insertTestAnalysisRun(t, db, "run-progress", "s1")

	// Insert tracks with various label states
	tracks := []*RunTrack{
		{RunID: "run-progress", TrackID: "track-1", SensorID: "s1", TrackState: "confirmed", StartUnixNanos: 1000},
		{RunID: "run-progress", TrackID: "track-2", SensorID: "s1", TrackState: "confirmed", StartUnixNanos: 2000},
		{RunID: "run-progress", TrackID: "track-3", SensorID: "s1", TrackState: "confirmed", StartUnixNanos: 3000},
	}

	for _, tr := range tracks {
		if err := store.InsertRunTrack(tr); err != nil {
			t.Fatalf("InsertRunTrack failed: %v", err)
		}
	}

	// Label some tracks
	store.UpdateTrackLabel("run-progress", "track-1", "car", "", 0.9, "user-1", "human_manual")
	store.UpdateTrackLabel("run-progress", "track-2", "pedestrian", "", 0.8, "user-1", "human_manual")

	// Get progress
	total, labeled, byClass, err := store.GetLabelingProgress("run-progress")
	if err != nil {
		t.Fatalf("GetLabelingProgress failed: %v", err)
	}

	if total != 3 {
		t.Errorf("Total mismatch: got %d, want 3", total)
	}
	if labeled != 2 {
		t.Errorf("Labeled mismatch: got %d, want 2", labeled)
	}
	if byClass["car"] != 1 {
		t.Errorf("Car count mismatch: got %d, want 1", byClass["car"])
	}
	if byClass["pedestrian"] != 1 {
		t.Errorf("Pedestrian count mismatch: got %d, want 1", byClass["pedestrian"])
	}
}

// TestGetUnlabeledTracks tests getting tracks that need labeling.
func TestGetUnlabeledTracks(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Insert parent run to satisfy foreign key constraint
	insertTestAnalysisRun(t, db, "run-unlabeled", "s1")

	// Insert tracks with various label states
	tracks := []*RunTrack{
		{RunID: "run-unlabeled", TrackID: "track-1", SensorID: "s1", TrackState: "confirmed", StartUnixNanos: 1000, ObservationCount: 20},
		{RunID: "run-unlabeled", TrackID: "track-2", SensorID: "s1", TrackState: "confirmed", StartUnixNanos: 2000, ObservationCount: 10},
		{RunID: "run-unlabeled", TrackID: "track-3", SensorID: "s1", TrackState: "confirmed", StartUnixNanos: 3000, ObservationCount: 30},
	}

	for _, tr := range tracks {
		if err := store.InsertRunTrack(tr); err != nil {
			t.Fatalf("InsertRunTrack failed: %v", err)
		}
	}

	// Label one track
	store.UpdateTrackLabel("run-unlabeled", "track-2", "car", "", 0.9, "user-1", "human_manual")

	// Get unlabeled tracks
	unlabeled, err := store.GetUnlabeledTracks("run-unlabeled", 10)
	if err != nil {
		t.Fatalf("GetUnlabeledTracks failed: %v", err)
	}

	if len(unlabeled) != 2 {
		t.Errorf("Expected 2 unlabeled tracks, got %d", len(unlabeled))
	}

	// Should be ordered by observation_count DESC
	if len(unlabeled) >= 2 && unlabeled[0].ObservationCount < unlabeled[1].ObservationCount {
		t.Error("Unlabeled tracks should be ordered by observation_count DESC")
	}
}

// TestParseRunParams_InvalidJSON tests error handling for invalid JSON.
func TestParseRunParams_InvalidJSON(t *testing.T) {
	_, err := ParseRunParams([]byte("invalid json"))
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestParseRunParams_WithOptionalFields tests parsing with all optional fields.
func TestParseRunParams_WithOptionalFields(t *testing.T) {
	paramsJSON := `{
		"version": "1.0",
		"timestamp": "2025-01-15T12:00:00Z",
		"background": {
			"background_update_fraction": 0.05,
			"closeness_sensitivity_multiplier": 2.5,
			"safety_margin_meters": 0.2,
			"neighbor_confirmation_count": 3,
			"noise_relative_fraction": 0.01,
			"seed_from_first_observation": true,
			"freeze_duration_nanos": 5000000000
		},
		"clustering": {
			"eps": 0.75,
			"min_pts": 12
		},
		"tracking": {
			"max_tracks": 75,
			"max_misses": 4,
			"hits_to_confirm": 6,
			"gating_distance_squared": 30.0,
			"process_noise_pos": 0.12,
			"process_noise_vel": 0.55,
			"measurement_noise": 0.22,
			"deleted_track_grace_period": 8000000000
		},
		"classification": {
			"model_type": "ml_model",
			"model_path": "/path/to/model"
		}
	}`

	params, err := ParseRunParams([]byte(paramsJSON))
	if err != nil {
		t.Fatalf("ParseRunParams failed: %v", err)
	}

	if params.Version != "1.0" {
		t.Errorf("Version mismatch: got %s", params.Version)
	}
	if params.Background.BackgroundUpdateFraction != 0.05 {
		t.Errorf("BackgroundUpdateFraction mismatch: got %f", params.Background.BackgroundUpdateFraction)
	}
	if params.Clustering.Eps != 0.75 {
		t.Errorf("Eps mismatch: got %f", params.Clustering.Eps)
	}
	if params.Tracking.MaxTracks != 75 {
		t.Errorf("MaxTracks mismatch: got %d", params.Tracking.MaxTracks)
	}
	if params.Classification.ModelType != "ml_model" {
		t.Errorf("ModelType mismatch: got %s", params.Classification.ModelType)
	}
}

// TestAnalysisRunStore_UpdateRunStatus tests updating run status.
func TestAnalysisRunStore_UpdateRunStatus(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	params := DefaultRunParams()
	paramsJSON, _ := params.ToJSON()

	run := &AnalysisRun{
		RunID:      "run-status-test",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor-1",
		ParamsJSON: paramsJSON,
		Status:     "running",
	}

	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	// Update status
	if err := store.UpdateRunStatus("run-status-test", "failed", "Test error message"); err != nil {
		t.Fatalf("UpdateRunStatus failed: %v", err)
	}

	// Verify update
	retrieved, err := store.GetRun("run-status-test")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if retrieved.Status != "failed" {
		t.Errorf("Status mismatch: got %s, want failed", retrieved.Status)
	}
	if retrieved.ErrorMessage != "Test error message" {
		t.Errorf("ErrorMessage mismatch: got %s", retrieved.ErrorMessage)
	}
}

// TestAnalysisRunStore_CompleteRun tests completing a run with statistics.
func TestAnalysisRunStore_CompleteRun(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	params := DefaultRunParams()
	paramsJSON, _ := params.ToJSON()

	run := &AnalysisRun{
		RunID:      "run-complete-test",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor-1",
		ParamsJSON: paramsJSON,
		Status:     "running",
	}

	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	// Complete run
	stats := &AnalysisStats{
		DurationSecs:     300.5,
		TotalFrames:      3000,
		TotalClusters:    1500,
		TotalTracks:      150,
		ConfirmedTracks:  125,
		ProcessingTimeMs: 45000,
	}

	if err := store.CompleteRun("run-complete-test", stats); err != nil {
		t.Fatalf("CompleteRun failed: %v", err)
	}

	// Verify
	retrieved, err := store.GetRun("run-complete-test")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if retrieved.Status != "completed" {
		t.Errorf("Status should be 'completed', got %s", retrieved.Status)
	}
	if retrieved.TotalFrames != 3000 {
		t.Errorf("TotalFrames mismatch: got %d", retrieved.TotalFrames)
	}
	if retrieved.ConfirmedTracks != 125 {
		t.Errorf("ConfirmedTracks mismatch: got %d", retrieved.ConfirmedTracks)
	}
}

// TestRunTrackFromTrackedObject_EmptySpeedHistory tests conversion with empty speed history.
func TestRunTrackFromTrackedObject_EmptySpeedHistory(t *testing.T) {
	track := &TrackedObject{
		TrackID:          "track-empty",
		SensorID:         "sensor-1",
		State:            TrackConfirmed,
		FirstUnixNanos:   1000,
		LastUnixNanos:    2000,
		ObservationCount: 5,
		AvgSpeedMps:      5.0,
	}
	track.SetSpeedHistory([]float32{}) // Empty

	runTrack := RunTrackFromTrackedObject("run-1", track)

	if runTrack.P50SpeedMps != 0 {
		t.Errorf("P50SpeedMps should be 0 for empty history, got %f", runTrack.P50SpeedMps)
	}
}

// TestAnalysisRunStore_WithParentRun tests runs with parent run reference.
func TestAnalysisRunStore_WithParentRun(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	params := DefaultRunParams()
	paramsJSON, _ := params.ToJSON()

	// Insert parent run
	parentRun := &AnalysisRun{
		RunID:      "parent-run",
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SensorID:   "sensor-1",
		ParamsJSON: paramsJSON,
		Status:     "completed",
	}
	if err := store.InsertRun(parentRun); err != nil {
		t.Fatalf("InsertRun (parent) failed: %v", err)
	}

	// Insert child run with parent reference
	childRun := &AnalysisRun{
		RunID:       "child-run",
		CreatedAt:   time.Now(),
		SourceType:  "pcap",
		SensorID:    "sensor-1",
		ParamsJSON:  paramsJSON,
		Status:      "completed",
		ParentRunID: "parent-run",
	}
	if err := store.InsertRun(childRun); err != nil {
		t.Fatalf("InsertRun (child) failed: %v", err)
	}

	// Retrieve child and verify parent reference
	retrieved, err := store.GetRun("child-run")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if retrieved.ParentRunID != "parent-run" {
		t.Errorf("ParentRunID mismatch: got %s, want parent-run", retrieved.ParentRunID)
	}
}

// TestInsertRunTrack_EmptyLinkedIDs tests inserting a track with no linked IDs.
func TestInsertRunTrack_EmptyLinkedIDs(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Insert parent run to satisfy foreign key constraint
	insertTestAnalysisRun(t, db, "run-empty-linked", "sensor-1")

	track := &RunTrack{
		RunID:          "run-empty-linked",
		TrackID:        "track-1",
		SensorID:       "sensor-1",
		TrackState:     "confirmed",
		StartUnixNanos: 1000,
		LinkedTrackIDs: nil, // Empty
	}

	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("InsertRunTrack failed: %v", err)
	}

	// Retrieve and verify
	tracks, err := store.GetRunTracks("run-empty-linked")
	if err != nil {
		t.Fatalf("GetRunTracks failed: %v", err)
	}

	if len(tracks) != 1 {
		t.Fatalf("Expected 1 track, got %d", len(tracks))
	}

	if tracks[0].LinkedTrackIDs == nil {
		// nil is acceptable
	} else if len(tracks[0].LinkedTrackIDs) != 0 {
		t.Errorf("LinkedTrackIDs should be empty, got %v", tracks[0].LinkedTrackIDs)
	}
}

// TestGetRunTracks_WithNullableFields tests retrieving tracks with nullable fields.
func TestGetRunTracks_WithNullableFields(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	// Insert parent run to satisfy foreign key constraint
	insertTestAnalysisRun(t, db, "run-nullable", "sensor-1")

	// Insert track with minimal fields (many will be null/default)
	track := &RunTrack{
		RunID:          "run-nullable",
		TrackID:        "track-nullable",
		SensorID:       "sensor-1",
		TrackState:     "tentative",
		StartUnixNanos: 1000,
		// All other fields left as zero/nil
	}

	if err := store.InsertRunTrack(track); err != nil {
		t.Fatalf("InsertRunTrack failed: %v", err)
	}

	// Retrieve and verify no errors
	tracks, err := store.GetRunTracks("run-nullable")
	if err != nil {
		t.Fatalf("GetRunTracks failed: %v", err)
	}

	if len(tracks) != 1 {
		t.Fatalf("Expected 1 track, got %d", len(tracks))
	}

	// Verify zero values are preserved
	if tracks[0].EndUnixNanos != 0 {
		t.Errorf("EndUnixNanos should be 0, got %d", tracks[0].EndUnixNanos)
	}
}

// TestListRuns_WithAllFields tests that all fields are properly retrieved.
func TestListRuns_WithAllFields(t *testing.T) {
	db, cleanup := setupAnalysisRunTestDB(t)
	defer cleanup()

	store := NewAnalysisRunStore(db)

	params := DefaultRunParams()
	paramsJSON, _ := params.ToJSON()

	run := &AnalysisRun{
		RunID:            "run-full",
		CreatedAt:        time.Now(),
		SourceType:       "live",
		SourcePath:       "/dev/ttyUSB0",
		SensorID:         "sensor-full",
		ParamsJSON:       paramsJSON,
		DurationSecs:     600.0,
		TotalFrames:      6000,
		TotalClusters:    3000,
		TotalTracks:      300,
		ConfirmedTracks:  250,
		ProcessingTimeMs: 120000,
		Status:           "completed",
		ErrorMessage:     "",
		ParentRunID:      "",
		Notes:            "Full test run with all fields",
	}

	if err := store.InsertRun(run); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	runs, err := store.ListRuns(1)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}

	if len(runs) != 1 {
		t.Fatalf("Expected 1 run, got %d", len(runs))
	}

	// Verify all fields
	r := runs[0]
	if r.SourceType != "live" {
		t.Errorf("SourceType mismatch: got %s", r.SourceType)
	}
	if r.Notes != "Full test run with all fields" {
		t.Errorf("Notes mismatch: got %s", r.Notes)
	}

	// Verify paramsJSON is valid
	var parsedParams RunParams
	if err := json.Unmarshal(r.ParamsJSON, &parsedParams); err != nil {
		t.Errorf("Failed to parse ParamsJSON: %v", err)
	}
}
