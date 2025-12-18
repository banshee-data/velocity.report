package lidar

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestVCDatabase(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	// Create temp database
	f, err := os.CreateTemp("", "vc_track_store_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	dbPath := f.Name()
	f.Close()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		os.Remove(dbPath)
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create required tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS lidar_velocity_coherent_tracks (
			track_id TEXT PRIMARY KEY,
			sensor_id TEXT NOT NULL,
			world_frame TEXT NOT NULL,
			track_state TEXT NOT NULL,
			start_unix_nanos INTEGER NOT NULL,
			end_unix_nanos INTEGER NOT NULL,
			observation_count INTEGER DEFAULT 0,
			hits INTEGER DEFAULT 0,
			misses INTEGER DEFAULT 0,
			avg_speed_mps REAL,
			peak_speed_mps REAL,
			avg_velocity_confidence REAL,
			velocity_consistency_score REAL,
			bounding_box_length_avg REAL,
			bounding_box_width_avg REAL,
			bounding_box_height_avg REAL,
			height_p95_max REAL,
			intensity_mean_avg REAL,
			min_points_observed INTEGER,
			sparse_frame_count INTEGER,
			object_class TEXT,
			object_confidence REAL
		);

		CREATE TABLE IF NOT EXISTS lidar_velocity_coherent_track_obs (
			track_id TEXT NOT NULL,
			ts_unix_nanos INTEGER NOT NULL,
			world_frame TEXT NOT NULL,
			x REAL,
			y REAL,
			z REAL,
			velocity_x REAL,
			velocity_y REAL,
			velocity_z REAL,
			velocity_confidence REAL,
			speed_mps REAL,
			heading_rad REAL,
			bounding_box_length REAL,
			bounding_box_width REAL,
			bounding_box_height REAL,
			height_p95 REAL,
			intensity_mean REAL,
			points_count INTEGER,
			PRIMARY KEY (track_id, ts_unix_nanos)
		);
	`)
	if err != nil {
		db.Close()
		os.Remove(dbPath)
		t.Fatalf("Failed to create tables: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.Remove(dbPath)
	}

	return db, cleanup
}

func TestInsertVCTrack(t *testing.T) {
	db, cleanup := setupTestVCDatabase(t)
	defer cleanup()

	track := &VelocityCoherentTrack{
		TrackID:              "vc-track-001",
		SensorID:             "test-sensor",
		State:                TrackConfirmVC,
		FirstUnixNanos:       time.Now().UnixNano(),
		LastUnixNanos:        time.Now().UnixNano() + int64(time.Second),
		ObservationCount:     5,
		Hits:                 5,
		Misses:               0,
		AvgSpeedMps:          5.0,
		PeakSpeedMps:         6.5,
		VelocityConfidence:   0.95,
		VelocityConsistency:  0.9,
		BoundingBoxLengthAvg: 4.5,
		BoundingBoxWidthAvg:  1.8,
		BoundingBoxHeightAvg: 1.5,
		HeightP95Max:         1.6,
		IntensityMeanAvg:     50.0,
		MinPointsObserved:    10,
		SparseFrameCount:     0,
		ObjectClass:          "car",
		ObjectConfidence:     0.85,
	}

	err := InsertVCTrack(db, track, "site/test-sensor")
	if err != nil {
		t.Fatalf("InsertVCTrack failed: %v", err)
	}

	// Verify insertion
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_velocity_coherent_tracks WHERE track_id = ?", track.TrackID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query track: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 track, got %d", count)
	}
}

func TestInsertVCTrack_Update(t *testing.T) {
	db, cleanup := setupTestVCDatabase(t)
	defer cleanup()

	track := &VelocityCoherentTrack{
		TrackID:          "vc-track-002",
		SensorID:         "test-sensor",
		State:            TrackTentVC,
		FirstUnixNanos:   time.Now().UnixNano(),
		LastUnixNanos:    time.Now().UnixNano(),
		ObservationCount: 2,
		Hits:             2,
	}

	err := InsertVCTrack(db, track, "site/test-sensor")
	if err != nil {
		t.Fatalf("InsertVCTrack failed: %v", err)
	}

	// Update the track
	track.State = TrackConfirmVC
	track.ObservationCount = 5
	track.Hits = 5

	err = InsertVCTrack(db, track, "site/test-sensor")
	if err != nil {
		t.Fatalf("InsertVCTrack update failed: %v", err)
	}

	// Verify only one record exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_velocity_coherent_tracks WHERE track_id = ?", track.TrackID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query track: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 track after upsert, got %d", count)
	}

	// Verify the update
	var state string
	var obsCount int
	err = db.QueryRow("SELECT track_state, observation_count FROM lidar_velocity_coherent_tracks WHERE track_id = ?", track.TrackID).Scan(&state, &obsCount)
	if err != nil {
		t.Fatalf("Failed to query track state: %v", err)
	}
	if state != string(TrackConfirmVC) {
		t.Errorf("Expected state %s, got %s", TrackConfirmVC, state)
	}
	if obsCount != 5 {
		t.Errorf("Expected observation_count 5, got %d", obsCount)
	}
}

func TestInsertVCTrack_NilDB(t *testing.T) {
	track := &VelocityCoherentTrack{TrackID: "test"}
	err := InsertVCTrack(nil, track, "frame")
	if err != nil {
		t.Errorf("Expected nil error for nil db, got %v", err)
	}
}

func TestInsertVCTrack_NilTrack(t *testing.T) {
	db, cleanup := setupTestVCDatabase(t)
	defer cleanup()

	err := InsertVCTrack(db, nil, "frame")
	if err != nil {
		t.Errorf("Expected nil error for nil track, got %v", err)
	}
}

func TestInsertVCTrackObservation(t *testing.T) {
	db, cleanup := setupTestVCDatabase(t)
	defer cleanup()

	obs := &VCTrackObservation{
		TrackID:            "vc-track-001",
		TSUnixNanos:        time.Now().UnixNano(),
		WorldFrame:         "site/test-sensor",
		X:                  5.0,
		Y:                  0.0,
		Z:                  0.5,
		VelocityX:          5.0,
		VelocityY:          0.0,
		VelocityZ:          0.0,
		VelocityConfidence: 0.9,
		SpeedMps:           5.0,
		HeadingRad:         0.0,
		BoundingBoxLength:  4.5,
		BoundingBoxWidth:   1.8,
		BoundingBoxHeight:  1.5,
		HeightP95:          1.5,
		IntensityMean:      50.0,
		PointsCount:        20,
	}

	err := InsertVCTrackObservation(db, obs)
	if err != nil {
		t.Fatalf("InsertVCTrackObservation failed: %v", err)
	}

	// Verify insertion
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_velocity_coherent_track_obs WHERE track_id = ?", obs.TrackID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query observation: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 observation, got %d", count)
	}
}

func TestInsertVCTrackObservation_NilDB(t *testing.T) {
	obs := &VCTrackObservation{TrackID: "test"}
	err := InsertVCTrackObservation(nil, obs)
	if err != nil {
		t.Errorf("Expected nil error for nil db, got %v", err)
	}
}

func TestInsertVCTrackObservation_NilObs(t *testing.T) {
	db, cleanup := setupTestVCDatabase(t)
	defer cleanup()

	err := InsertVCTrackObservation(db, nil)
	if err != nil {
		t.Errorf("Expected nil error for nil observation, got %v", err)
	}
}

func TestInsertVCTrackObservation_DuplicateIgnored(t *testing.T) {
	db, cleanup := setupTestVCDatabase(t)
	defer cleanup()

	tsNanos := time.Now().UnixNano()
	obs := &VCTrackObservation{
		TrackID:     "vc-track-001",
		TSUnixNanos: tsNanos,
		WorldFrame:  "site/test-sensor",
		X:           5.0,
		Y:           0.0,
		Z:           0.5,
	}

	// Insert first
	err := InsertVCTrackObservation(db, obs)
	if err != nil {
		t.Fatalf("First insert failed: %v", err)
	}

	// Insert duplicate - should be ignored (ON CONFLICT DO NOTHING)
	obs.X = 10.0 // Change a value
	err = InsertVCTrackObservation(db, obs)
	if err != nil {
		t.Fatalf("Duplicate insert failed: %v", err)
	}

	// Verify only one record
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_velocity_coherent_track_obs WHERE track_id = ? AND ts_unix_nanos = ?",
		obs.TrackID, tsNanos).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 observation (duplicate ignored), got %d", count)
	}

	// Verify original value kept
	var x float32
	err = db.QueryRow("SELECT x FROM lidar_velocity_coherent_track_obs WHERE track_id = ? AND ts_unix_nanos = ?",
		obs.TrackID, tsNanos).Scan(&x)
	if err != nil {
		t.Fatalf("Failed to query x: %v", err)
	}
	if x != 5.0 {
		t.Errorf("Expected original x=5.0 preserved, got %f", x)
	}
}
