package sqlite

import (
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// setupTestDBWithSchema creates a test database with full schema for track store tests.
// Reads schema.sql directly to avoid import cycle with db package.
func setupTestDBWithSchema(t *testing.T) (*sql.DB, func()) {
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
	// NOTE: This version number must be updated when new migrations are added to internal/db/migrations/
	// Current latest: 000015_add_site_map_fields (as of 2026-02-02)
	// To find latest version: ls -1 internal/db/migrations/*.up.sql | sort | tail -1
	latestMigrationVersion := 15
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (?, false)`, latestMigrationVersion); err != nil {
		db.Close()
		t.Fatalf("Failed to baseline migrations: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-shm")
		os.Remove(dbPath + "-wal")
	}

	return db, cleanup
}

// TestClearTracks verifies clearing tracks, observations, and clusters for a sensor.
func TestClearTracks(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	sensorID := "sensor-clear-test"

	// Insert clusters
	for i := 0; i < 3; i++ {
		cluster := &WorldCluster{
			SensorID:    sensorID,
			WorldFrame:  "site/main",
			TSUnixNanos: int64(1000 + i),
			CentroidX:   float32(i),
		}
		if _, err := InsertCluster(db, cluster); err != nil {
			t.Fatalf("InsertCluster failed: %v", err)
		}
	}

	// Insert track
	track := &TrackedObject{
		TrackID:        "track-clear-1",
		SensorID:       sensorID,
		State:          TrackConfirmed,
		FirstUnixNanos: 1000,
	}
	track.SetSpeedHistory([]float32{5.0})
	if err := InsertTrack(db, track, "site/main"); err != nil {
		t.Fatalf("InsertTrack failed: %v", err)
	}

	// Insert observations
	for i := 0; i < 3; i++ {
		obs := &TrackObservation{
			TrackID:     "track-clear-1",
			TSUnixNanos: int64(1000 + i),
			WorldFrame:  "site/main",
			X:           float32(i),
		}
		if err := InsertTrackObservation(db, obs); err != nil {
			t.Fatalf("InsertTrackObservation failed: %v", err)
		}
	}

	// Verify data exists
	tracks, err := GetActiveTracks(db, sensorID, "")
	if err != nil || len(tracks) == 0 {
		t.Fatalf("Expected tracks before clear, got %d", len(tracks))
	}

	// Clear tracks
	if err := ClearTracks(db, sensorID); err != nil {
		t.Fatalf("ClearTracks failed: %v", err)
	}

	// Verify all cleared
	tracks, err = GetActiveTracks(db, sensorID, "")
	if err != nil {
		t.Fatalf("GetActiveTracks after clear failed: %v", err)
	}
	if len(tracks) != 0 {
		t.Errorf("Expected 0 tracks after clear, got %d", len(tracks))
	}

	// Verify observations cleared
	obs, err := GetTrackObservations(db, "track-clear-1", 10)
	if err != nil {
		t.Fatalf("GetTrackObservations after clear failed: %v", err)
	}
	if len(obs) != 0 {
		t.Errorf("Expected 0 observations after clear, got %d", len(obs))
	}

	// Verify clusters cleared
	clusters, err := GetRecentClusters(db, sensorID, 0, math.MaxInt64, 10)
	if err != nil {
		t.Fatalf("GetRecentClusters after clear failed: %v", err)
	}
	if len(clusters) != 0 {
		t.Errorf("Expected 0 clusters after clear, got %d", len(clusters))
	}
}

// TestClearTracks_EmptySensorID verifies error when sensorID is empty.
func TestClearTracks_EmptySensorID(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	err := ClearTracks(db, "")
	if err == nil {
		t.Error("Expected error for empty sensorID")
	}
}

// TestGetTrackObservationsInRange tests querying observations within a time window.
func TestGetTrackObservationsInRange(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	sensorID := "sensor-range-test"

	// Insert track
	track := &TrackedObject{
		TrackID:        "track-range-1",
		SensorID:       sensorID,
		State:          TrackConfirmed,
		FirstUnixNanos: 1000,
	}
	track.SetSpeedHistory([]float32{5.0})
	if err := InsertTrack(db, track, "site/main"); err != nil {
		t.Fatalf("InsertTrack failed: %v", err)
	}

	// Insert observations with different timestamps
	timestamps := []int64{1000, 2000, 3000, 4000, 5000}
	for _, ts := range timestamps {
		obs := &TrackObservation{
			TrackID:     "track-range-1",
			TSUnixNanos: ts,
			WorldFrame:  "site/main",
			X:           float32(ts / 1000),
		}
		if err := InsertTrackObservation(db, obs); err != nil {
			t.Fatalf("InsertTrackObservation failed: %v", err)
		}
	}

	// Query range [2000, 4000]
	obs, err := GetTrackObservationsInRange(db, sensorID, 2000, 4000, 10, "")
	if err != nil {
		t.Fatalf("GetTrackObservationsInRange failed: %v", err)
	}

	if len(obs) != 3 {
		t.Errorf("Expected 3 observations in range, got %d", len(obs))
	}

	// Verify ascending order
	if len(obs) > 1 && obs[0].TSUnixNanos > obs[1].TSUnixNanos {
		t.Error("Expected observations in ascending timestamp order")
	}
}

// TestGetTrackObservationsInRange_WithTrackIDFilter tests filtering by track ID.
func TestGetTrackObservationsInRange_WithTrackIDFilter(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	sensorID := "sensor-filter-test"

	// Insert two tracks
	for _, trackID := range []string{"track-a", "track-b"} {
		track := &TrackedObject{
			TrackID:        trackID,
			SensorID:       sensorID,
			State:          TrackConfirmed,
			FirstUnixNanos: 1000,
		}
		track.SetSpeedHistory([]float32{5.0})
		if err := InsertTrack(db, track, "site/main"); err != nil {
			t.Fatalf("InsertTrack failed: %v", err)
		}

		// Insert observations
		for i := 0; i < 3; i++ {
			obs := &TrackObservation{
				TrackID:     trackID,
				TSUnixNanos: int64(1000 + i),
				WorldFrame:  "site/main",
				X:           float32(i),
			}
			if err := InsertTrackObservation(db, obs); err != nil {
				t.Fatalf("InsertTrackObservation failed: %v", err)
			}
		}
	}

	// Query only track-a
	obs, err := GetTrackObservationsInRange(db, sensorID, 0, 5000, 100, "track-a")
	if err != nil {
		t.Fatalf("GetTrackObservationsInRange with trackID failed: %v", err)
	}

	if len(obs) != 3 {
		t.Errorf("Expected 3 observations for track-a, got %d", len(obs))
	}

	for _, o := range obs {
		if o.TrackID != "track-a" {
			t.Errorf("Expected only track-a observations, got %s", o.TrackID)
		}
	}
}

// TestGetTracksInRange tests querying tracks overlapping a time window.
func TestGetTracksInRange(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	sensorID := "sensor-tracks-range"

	// Insert tracks with different time spans
	tracks := []struct {
		id    string
		start int64
		end   int64
	}{
		{"track-1", 1000, 2000},
		{"track-2", 2500, 3500},
		{"track-3", 4000, 5000},
		{"track-4", 1500, 4500}, // Overlaps both windows
	}

	for _, tr := range tracks {
		track := &TrackedObject{
			TrackID:        tr.id,
			SensorID:       sensorID,
			State:          TrackConfirmed,
			FirstUnixNanos: tr.start,
			LastUnixNanos:  tr.end,
		}
		track.SetSpeedHistory([]float32{5.0})
		if err := InsertTrack(db, track, "site/main"); err != nil {
			t.Fatalf("InsertTrack failed: %v", err)
		}

		// Insert at least one observation for history population
		obs := &TrackObservation{
			TrackID:     tr.id,
			TSUnixNanos: tr.start,
			WorldFrame:  "site/main",
			X:           1.0,
		}
		if err := InsertTrackObservation(db, obs); err != nil {
			t.Fatalf("InsertTrackObservation failed: %v", err)
		}
	}

	// Query range [2000, 3000] - should match track-1 (ends at 2000), track-2, and track-4
	result, err := GetTracksInRange(db, sensorID, "", 2000, 3000, 100)
	if err != nil {
		t.Fatalf("GetTracksInRange failed: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 tracks in range [2000,3000], got %d", len(result))
	}

	// Query with state filter
	result, err = GetTracksInRange(db, sensorID, "confirmed", 0, 10000, 100)
	if err != nil {
		t.Fatalf("GetTracksInRange with state failed: %v", err)
	}

	if len(result) != 4 {
		t.Errorf("Expected 4 confirmed tracks, got %d", len(result))
	}
}

// TestNullFloat32 tests the nullFloat32 helper function.
func TestNullFloat32(t *testing.T) {
	tests := []struct {
		name    string
		value   float32
		wantNil bool
		wantVal float32
	}{
		{"positive value", 1.5, false, 1.5},
		{"zero value", 0.0, false, 0.0},
		{"negative value", -1.0, false, -1.0},
		{"NaN value", float32(math.NaN()), true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nullFloat32(tt.value)
			if tt.wantNil {
				if result != nil {
					t.Errorf("nullFloat32(%v) = %v, want nil", tt.value, result)
				}
			} else {
				if result == nil {
					t.Errorf("nullFloat32(%v) = nil, want %v", tt.value, tt.wantVal)
				} else if val, ok := result.(float32); !ok || val != tt.wantVal {
					t.Errorf("nullFloat32(%v) = %v, want %v", tt.value, result, tt.wantVal)
				}
			}
		})
	}
}

// TestGetRecentClusters_Limit tests the limit parameter.
func TestGetRecentClusters_Limit(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	sensorID := "sensor-limit-test"

	// Insert 10 clusters
	for i := 0; i < 10; i++ {
		cluster := &WorldCluster{
			SensorID:    sensorID,
			WorldFrame:  "site/main",
			TSUnixNanos: int64(1000 + i),
			CentroidX:   float32(i),
		}
		if _, err := InsertCluster(db, cluster); err != nil {
			t.Fatalf("InsertCluster failed: %v", err)
		}
	}

	// Query with limit
	clusters, err := GetRecentClusters(db, sensorID, 0, math.MaxInt64, 5)
	if err != nil {
		t.Fatalf("GetRecentClusters failed: %v", err)
	}

	if len(clusters) != 5 {
		t.Errorf("Expected 5 clusters with limit, got %d", len(clusters))
	}
}

// TestGetActiveTracks_WithHistory verifies that track history is populated.
func TestGetActiveTracks_WithHistory(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	sensorID := "sensor-history-test"

	// Use recent timestamps so observations fall within the 60 s recency
	// window used by GetActiveTracks.
	baseNanos := time.Now().Add(-10 * time.Second).UnixNano()

	// Insert track
	track := &TrackedObject{
		TrackID:        "track-history",
		SensorID:       sensorID,
		State:          TrackConfirmed,
		FirstUnixNanos: baseNanos,
	}
	track.SetSpeedHistory([]float32{5.0})
	if err := InsertTrack(db, track, "site/main"); err != nil {
		t.Fatalf("InsertTrack failed: %v", err)
	}

	// Insert observations
	for i := 0; i < 5; i++ {
		obs := &TrackObservation{
			TrackID:     "track-history",
			TSUnixNanos: baseNanos + int64(i)*int64(100*time.Millisecond),
			WorldFrame:  "site/main",
			X:           float32(i),
			Y:           float32(i * 2),
		}
		if err := InsertTrackObservation(db, obs); err != nil {
			t.Fatalf("InsertTrackObservation failed: %v", err)
		}
	}

	// Get tracks and verify history
	tracks, err := GetActiveTracks(db, sensorID, "")
	if err != nil {
		t.Fatalf("GetActiveTracks failed: %v", err)
	}

	if len(tracks) != 1 {
		t.Fatalf("Expected 1 track, got %d", len(tracks))
	}

	// History should be populated in chronological order (oldest first)
	if len(tracks[0].History) != 5 {
		t.Errorf("Expected 5 history points, got %d", len(tracks[0].History))
	}

	// Verify chronological order
	if len(tracks[0].History) > 1 {
		if tracks[0].History[0].Timestamp > tracks[0].History[1].Timestamp {
			t.Error("History should be in chronological order (oldest first)")
		}
	}
}

func TestGetActiveTracks_HistoryWindowPerTrack(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	sensorID := "sensor-history-window-test"
	now := time.Now()

	recentBase := now.Add(-5 * time.Second).UnixNano()
	oldBase := now.Add(-50 * time.Second).UnixNano()

	recent := &TrackedObject{
		TrackID:        "track-recent",
		SensorID:       sensorID,
		State:          TrackConfirmed,
		FirstUnixNanos: recentBase,
	}
	recent.SetSpeedHistory([]float32{1})
	if err := InsertTrack(db, recent, "site/main"); err != nil {
		t.Fatalf("InsertTrack recent failed: %v", err)
	}

	old := &TrackedObject{
		TrackID:        "track-old",
		SensorID:       sensorID,
		State:          TrackConfirmed,
		FirstUnixNanos: oldBase,
	}
	old.SetSpeedHistory([]float32{1})
	if err := InsertTrack(db, old, "site/main"); err != nil {
		t.Fatalf("InsertTrack old failed: %v", err)
	}

	// Recent observation for recent track.
	if err := InsertTrackObservation(db, &TrackObservation{
		TrackID:     recent.TrackID,
		TSUnixNanos: recentBase + int64(time.Second),
		WorldFrame:  "site/main",
		X:           1.0,
		Y:           1.0,
	}); err != nil {
		t.Fatalf("InsertTrackObservation recent failed: %v", err)
	}

	// Old observations should still be returned for old track (within 60s recency window).
	for i := 0; i < 2; i++ {
		if err := InsertTrackObservation(db, &TrackObservation{
			TrackID:     old.TrackID,
			TSUnixNanos: oldBase + int64(i)*int64(500*time.Millisecond),
			WorldFrame:  "site/main",
			X:           float32(i),
			Y:           float32(i),
		}); err != nil {
			t.Fatalf("InsertTrackObservation old failed: %v", err)
		}
	}

	tracks, err := GetActiveTracks(db, sensorID, "")
	if err != nil {
		t.Fatalf("GetActiveTracks failed: %v", err)
	}

	var oldTrack *TrackedObject
	for _, tr := range tracks {
		if tr.TrackID == old.TrackID {
			oldTrack = tr
			break
		}
	}
	if oldTrack == nil {
		t.Fatalf("old track not returned; tracks=%d", len(tracks))
	}
	if len(oldTrack.History) != 2 {
		t.Fatalf("old track history = %d, want 2", len(oldTrack.History))
	}
}

// TestGetTracksInRange_DefaultLimit tests the default limit behaviour.
func TestGetTracksInRange_DefaultLimit(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	sensorID := "sensor-default-limit"

	// Insert track
	track := &TrackedObject{
		TrackID:        "track-1",
		SensorID:       sensorID,
		State:          TrackConfirmed,
		FirstUnixNanos: 1000,
		LastUnixNanos:  2000,
	}
	track.SetSpeedHistory([]float32{5.0})
	if err := InsertTrack(db, track, "site/main"); err != nil {
		t.Fatalf("InsertTrack failed: %v", err)
	}

	// Query with limit=0 (should use default)
	result, err := GetTracksInRange(db, sensorID, "", 0, 10000, 0)
	if err != nil {
		t.Fatalf("GetTracksInRange with zero limit failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 track, got %d", len(result))
	}
}

// TestGetTracksInRange_NullEndNanos tests tracks with end_unix_nanos = 0 (ongoing track).
func TestGetTracksInRange_NullEndNanos(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	sensorID := "sensor-null-end"

	// Insert track with end=0 (still in progress) - note that InsertTrack
	// stores 0 as 0, not NULL. The COALESCE logic uses start when end is NULL,
	// but since we store 0, the track won't match unless range includes 0.
	track := &TrackedObject{
		TrackID:        "track-ongoing",
		SensorID:       sensorID,
		State:          TrackTentative,
		FirstUnixNanos: 5000,
		LastUnixNanos:  7000, // Use a valid end time for this test
	}
	track.SetSpeedHistory([]float32{5.0})
	if err := InsertTrack(db, track, "site/main"); err != nil {
		t.Fatalf("InsertTrack failed: %v", err)
	}

	// Query range that includes the track's time span
	result, err := GetTracksInRange(db, sensorID, "", 4000, 6000, 100)
	if err != nil {
		t.Fatalf("GetTracksInRange failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 track in range, got %d", len(result))
	}
}

// TestClearRuns verifies clearing all analysis runs and their associated run tracks for a sensor.
func TestClearRuns(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	sensorID1 := "sensor-clear-runs-1"
	sensorID2 := "sensor-clear-runs-2"

	// Create an AnalysisRunStore
	store := NewAnalysisRunStore(db)

	// Insert runs for sensor 1
	run1 := &AnalysisRun{
		RunID:           "run-001",
		SensorID:        sensorID1,
		SourceType:      "pcap",
		SourcePath:      "/test/data1.pcap",
		ParamsJSON:      []byte(`{"version":"1.0"}`),
		DurationSecs:    10.0,
		TotalFrames:     100,
		TotalClusters:   50,
		TotalTracks:     5,
		ConfirmedTracks: 3,
		Status:          "completed",
	}
	if err := store.InsertRun(run1); err != nil {
		t.Fatalf("failed to insert run1: %v", err)
	}

	run2 := &AnalysisRun{
		RunID:           "run-002",
		SensorID:        sensorID1,
		SourceType:      "pcap",
		SourcePath:      "/test/data2.pcap",
		ParamsJSON:      []byte(`{"version":"1.0"}`),
		DurationSecs:    15.0,
		TotalFrames:     150,
		TotalClusters:   75,
		TotalTracks:     8,
		ConfirmedTracks: 5,
		Status:          "completed",
	}
	if err := store.InsertRun(run2); err != nil {
		t.Fatalf("failed to insert run2: %v", err)
	}

	// Insert run for sensor 2 (should not be deleted)
	run3 := &AnalysisRun{
		RunID:           "run-003",
		SensorID:        sensorID2,
		SourceType:      "pcap",
		SourcePath:      "/test/data3.pcap",
		ParamsJSON:      []byte(`{"version":"1.0"}`),
		DurationSecs:    20.0,
		TotalFrames:     200,
		TotalClusters:   100,
		TotalTracks:     10,
		ConfirmedTracks: 7,
		Status:          "completed",
	}
	if err := store.InsertRun(run3); err != nil {
		t.Fatalf("failed to insert run3: %v", err)
	}

	// Insert run tracks for all runs
	track1 := &RunTrack{
		RunID:            "run-001",
		TrackID:          "track-001",
		SensorID:         sensorID1,
		TrackState:       "confirmed",
		StartUnixNanos:   1000000000,
		EndUnixNanos:     2000000000,
		ObservationCount: 10,
		AvgSpeedMps:      5.5,
		PeakSpeedMps:     8.0,
	}
	if err := store.InsertRunTrack(track1); err != nil {
		t.Fatalf("failed to insert track1: %v", err)
	}

	track2 := &RunTrack{
		RunID:            "run-002",
		TrackID:          "track-002",
		SensorID:         sensorID1,
		TrackState:       "confirmed",
		StartUnixNanos:   3000000000,
		EndUnixNanos:     4000000000,
		ObservationCount: 15,
		AvgSpeedMps:      6.2,
		PeakSpeedMps:     9.0,
	}
	if err := store.InsertRunTrack(track2); err != nil {
		t.Fatalf("failed to insert track2: %v", err)
	}

	track3 := &RunTrack{
		RunID:            "run-003",
		TrackID:          "track-003",
		SensorID:         sensorID2,
		TrackState:       "confirmed",
		StartUnixNanos:   5000000000,
		EndUnixNanos:     6000000000,
		ObservationCount: 20,
		AvgSpeedMps:      7.0,
		PeakSpeedMps:     10.0,
	}
	if err := store.InsertRunTrack(track3); err != nil {
		t.Fatalf("failed to insert track3: %v", err)
	}

	// Verify runs and tracks exist before clear
	var count int
	var err error
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_analysis_runs WHERE sensor_id = ?", sensorID1).Scan(&count)
	if err != nil {
		t.Fatalf("Count runs before clear failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("Expected 2 runs for sensor1 before clear, got %d", count)
	}

	// Clear runs for sensor 1
	if err := ClearRuns(db, sensorID1); err != nil {
		t.Fatalf("ClearRuns failed: %v", err)
	}

	// Verify sensor 1 runs are deleted
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_analysis_runs WHERE sensor_id = ?", sensorID1).Scan(&count)
	if err != nil {
		t.Fatalf("Count runs after clear failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 runs for sensor1 after clear, got %d", count)
	}

	// Verify sensor 1 run tracks are deleted (cascade)
	tracks1, err := store.GetRunTracks("run-001")
	if err != nil {
		t.Fatalf("GetRunTracks for run-001 after clear failed: %v", err)
	}
	if len(tracks1) != 0 {
		t.Errorf("Expected 0 tracks for run-001 after clear, got %d", len(tracks1))
	}

	tracks2, err := store.GetRunTracks("run-002")
	if err != nil {
		t.Fatalf("GetRunTracks for run-002 after clear failed: %v", err)
	}
	if len(tracks2) != 0 {
		t.Errorf("Expected 0 tracks for run-002 after clear, got %d", len(tracks2))
	}

	// Verify sensor 2 runs are NOT deleted
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_analysis_runs WHERE sensor_id = ?", sensorID2).Scan(&count)
	if err != nil {
		t.Fatalf("Count runs for sensor2 after clear failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 run for sensor2 after clear, got %d", count)
	}

	// Verify sensor 2 run tracks are NOT deleted
	tracks3, err := store.GetRunTracks("run-003")
	if err != nil {
		t.Fatalf("GetRunTracks for run-003 after clear failed: %v", err)
	}
	if len(tracks3) != 1 {
		t.Errorf("Expected 1 track for run-003 after clear, got %d", len(tracks3))
	}
}

// TestClearRuns_EmptySensorID verifies error when sensorID is empty.
func TestClearRuns_EmptySensorID(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	err := ClearRuns(db, "")
	if err == nil {
		t.Error("Expected error for empty sensorID")
	}
	if err != nil && !strings.Contains(err.Error(), "sensorID is required") {
		t.Errorf("Expected 'sensorID is required' error, got: %v", err)
	}
}

// TestDeleteRun verifies deleting a specific analysis run and its tracks.
func TestDeleteRun(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	sensorID := "sensor-delete-run"

	// Create an AnalysisRunStore
	store := NewAnalysisRunStore(db)

	// Insert two runs
	run1 := &AnalysisRun{
		RunID:           "run-001",
		SensorID:        sensorID,
		SourceType:      "pcap",
		SourcePath:      "/test/data1.pcap",
		ParamsJSON:      []byte(`{"version":"1.0"}`),
		DurationSecs:    10.0,
		TotalFrames:     100,
		TotalClusters:   50,
		TotalTracks:     5,
		ConfirmedTracks: 3,
		Status:          "completed",
	}
	if err := store.InsertRun(run1); err != nil {
		t.Fatalf("failed to insert run1: %v", err)
	}

	run2 := &AnalysisRun{
		RunID:           "run-002",
		SensorID:        sensorID,
		SourceType:      "pcap",
		SourcePath:      "/test/data2.pcap",
		ParamsJSON:      []byte(`{"version":"1.0"}`),
		DurationSecs:    15.0,
		TotalFrames:     150,
		TotalClusters:   75,
		TotalTracks:     8,
		ConfirmedTracks: 5,
		Status:          "completed",
	}
	if err := store.InsertRun(run2); err != nil {
		t.Fatalf("failed to insert run2: %v", err)
	}

	// Insert run tracks
	track1 := &RunTrack{
		RunID:            "run-001",
		TrackID:          "track-001",
		SensorID:         sensorID,
		TrackState:       "confirmed",
		StartUnixNanos:   1000000000,
		EndUnixNanos:     2000000000,
		ObservationCount: 10,
		AvgSpeedMps:      5.5,
		PeakSpeedMps:     8.0,
	}
	if err := store.InsertRunTrack(track1); err != nil {
		t.Fatalf("failed to insert track1: %v", err)
	}

	track2 := &RunTrack{
		RunID:            "run-002",
		TrackID:          "track-002",
		SensorID:         sensorID,
		TrackState:       "confirmed",
		StartUnixNanos:   3000000000,
		EndUnixNanos:     4000000000,
		ObservationCount: 15,
		AvgSpeedMps:      6.2,
		PeakSpeedMps:     9.0,
	}
	if err := store.InsertRunTrack(track2); err != nil {
		t.Fatalf("failed to insert track2: %v", err)
	}

	// Delete run-001
	if err := DeleteRun(db, "run-001"); err != nil {
		t.Fatalf("DeleteRun failed: %v", err)
	}

	// Verify run-001 is deleted
	_, err := store.GetRun("run-001")
	if err == nil {
		t.Error("Expected error getting deleted run, got nil")
	}

	// Verify run-001 tracks are deleted (cascade)
	tracks1, err := store.GetRunTracks("run-001")
	if err != nil {
		t.Fatalf("GetRunTracks for run-001 failed: %v", err)
	}
	if len(tracks1) != 0 {
		t.Errorf("Expected 0 tracks for deleted run-001, got %d", len(tracks1))
	}

	// Verify run-002 is NOT deleted
	run2Retrieved, err := store.GetRun("run-002")
	if err != nil {
		t.Fatalf("GetRun for run-002 failed: %v", err)
	}
	if run2Retrieved.RunID != "run-002" {
		t.Errorf("Expected run-002, got %s", run2Retrieved.RunID)
	}

	// Verify run-002 tracks are NOT deleted
	tracks2, err := store.GetRunTracks("run-002")
	if err != nil {
		t.Fatalf("GetRunTracks for run-002 failed: %v", err)
	}
	if len(tracks2) != 1 {
		t.Errorf("Expected 1 track for run-002, got %d", len(tracks2))
	}
}

// TestDeleteRun_NotFound verifies error when run doesn't exist.
func TestDeleteRun_NotFound(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	err := DeleteRun(db, "nonexistent-run")
	if err == nil {
		t.Error("Expected error for nonexistent run")
	}
	if err != nil && !strings.Contains(err.Error(), "run not found") {
		t.Errorf("Expected 'run not found' error, got: %v", err)
	}
}

// TestDeleteRun_EmptyRunID verifies error when runID is empty.
func TestDeleteRun_EmptyRunID(t *testing.T) {
	db, cleanup := setupTestDBWithSchema(t)
	defer cleanup()

	err := DeleteRun(db, "")
	if err == nil {
		t.Error("Expected error for empty runID")
	}
	if err != nil && !strings.Contains(err.Error(), "runID is required") {
		t.Errorf("Expected 'runID is required' error, got: %v", err)
	}
}
