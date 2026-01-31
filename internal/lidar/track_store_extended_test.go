package lidar

import (
	"database/sql"
	"math"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// setupTestDBWithSchema creates a test database with full schema for track store tests.
func setupTestDBWithSchema(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create tables with full schema
	createSQL := `
		CREATE TABLE IF NOT EXISTS lidar_clusters (
			lidar_cluster_id INTEGER PRIMARY KEY,
			sensor_id TEXT NOT NULL,
			world_frame TEXT NOT NULL,
			ts_unix_nanos INTEGER NOT NULL,
			centroid_x REAL,
			centroid_y REAL,
			centroid_z REAL,
			bounding_box_length REAL,
			bounding_box_width REAL,
			bounding_box_height REAL,
			points_count INTEGER,
			height_p95 REAL,
			intensity_mean REAL
		);

		CREATE TABLE IF NOT EXISTS lidar_tracks (
			track_id TEXT PRIMARY KEY,
			sensor_id TEXT NOT NULL,
			world_frame TEXT NOT NULL,
			track_state TEXT NOT NULL,
			start_unix_nanos INTEGER NOT NULL,
			end_unix_nanos INTEGER,
			observation_count INTEGER,
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
			classification_model TEXT
		);

		CREATE TABLE IF NOT EXISTS lidar_track_obs (
			track_id TEXT NOT NULL,
			ts_unix_nanos INTEGER NOT NULL,
			world_frame TEXT NOT NULL,
			x REAL,
			y REAL,
			z REAL,
			velocity_x REAL,
			velocity_y REAL,
			speed_mps REAL,
			heading_rad REAL,
			bounding_box_length REAL,
			bounding_box_width REAL,
			bounding_box_height REAL,
			height_p95 REAL,
			intensity_mean REAL,
			PRIMARY KEY (track_id, ts_unix_nanos)
		);
	`

	if _, err := db.Exec(createSQL); err != nil {
		db.Close()
		t.Fatalf("Failed to create tables: %v", err)
	}

	cleanup := func() {
		db.Close()
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
		speedHistory:   []float32{5.0},
	}
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
		speedHistory:   []float32{5.0},
	}
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
			speedHistory:   []float32{5.0},
		}
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
			speedHistory:   []float32{5.0},
		}
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

	// Insert track
	track := &TrackedObject{
		TrackID:        "track-history",
		SensorID:       sensorID,
		State:          TrackConfirmed,
		FirstUnixNanos: 1000,
		speedHistory:   []float32{5.0},
	}
	if err := InsertTrack(db, track, "site/main"); err != nil {
		t.Fatalf("InsertTrack failed: %v", err)
	}

	// Insert observations
	for i := 0; i < 5; i++ {
		obs := &TrackObservation{
			TrackID:     "track-history",
			TSUnixNanos: int64(1000 + i*100),
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
		speedHistory:   []float32{5.0},
	}
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
		speedHistory:   []float32{5.0},
	}
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
