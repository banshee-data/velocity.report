package lidar

import (
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "lidar-track-store-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create tables
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
			intensity_mean REAL,
			noise_points_count INTEGER DEFAULT 0,
			cluster_density REAL,
			aspect_ratio REAL
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
			classification_model TEXT,
			track_length_meters REAL,
			track_duration_secs REAL,
			occlusion_count INTEGER DEFAULT 0,
			max_occlusion_frames INTEGER DEFAULT 0,
			spatial_coverage REAL,
			noise_point_ratio REAL
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
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create tables: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

func TestInsertCluster(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cluster := &WorldCluster{
		SensorID:          "sensor-001",
		WorldFrame:        "site/main",
		TSUnixNanos:       1234567890000000000,
		CentroidX:         10.5,
		CentroidY:         20.5,
		CentroidZ:         1.0,
		BoundingBoxLength: 4.5,
		BoundingBoxWidth:  2.0,
		BoundingBoxHeight: 1.5,
		PointsCount:       100,
		HeightP95:         1.4,
		IntensityMean:     128.5,
	}

	id, err := InsertCluster(db, cluster)
	if err != nil {
		t.Fatalf("InsertCluster failed: %v", err)
	}

	if id <= 0 {
		t.Errorf("Expected positive cluster ID, got %d", id)
	}

	// Verify the cluster was inserted (use max int64 as far-future timestamp)
	clusters, err := GetRecentClusters(db, "sensor-001", 0, math.MaxInt64, 10)
	if err != nil {
		t.Fatalf("GetRecentClusters failed: %v", err)
	}

	if len(clusters) != 1 {
		t.Fatalf("Expected 1 cluster, got %d", len(clusters))
	}

	c := clusters[0]
	if c.SensorID != "sensor-001" {
		t.Errorf("Expected sensor_id 'sensor-001', got '%s'", c.SensorID)
	}
	if c.CentroidX != 10.5 {
		t.Errorf("Expected centroid_x 10.5, got %f", c.CentroidX)
	}
}

func TestInsertAndGetTrack(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	track := &TrackedObject{
		TrackID:              "track-001",
		SensorID:             "sensor-001",
		State:                TrackConfirmed,
		FirstUnixNanos:       1234567890000000000,
		LastUnixNanos:        1234567900000000000,
		ObservationCount:     10,
		AvgSpeedMps:          8.5,
		PeakSpeedMps:         12.0,
		BoundingBoxLengthAvg: 4.0,
		BoundingBoxWidthAvg:  2.0,
		BoundingBoxHeightAvg: 1.5,
		HeightP95Max:         1.4,
		IntensityMeanAvg:     100.0,
		ObjectClass:          "car",
		ObjectConfidence:     0.85,
		ClassificationModel:  "rule-based-v1.0",
		speedHistory:         []float32{7, 8, 9, 8, 9, 10, 8, 9, 8, 9},
	}

	err := InsertTrack(db, track, "site/main")
	if err != nil {
		t.Fatalf("InsertTrack failed: %v", err)
	}

	// Get active tracks
	tracks, err := GetActiveTracks(db, "sensor-001", "confirmed")
	if err != nil {
		t.Fatalf("GetActiveTracks failed: %v", err)
	}

	if len(tracks) != 1 {
		t.Fatalf("Expected 1 track, got %d", len(tracks))
	}

	retrieved := tracks[0]
	if retrieved.TrackID != "track-001" {
		t.Errorf("Expected track_id 'track-001', got '%s'", retrieved.TrackID)
	}
	if retrieved.State != TrackConfirmed {
		t.Errorf("Expected state 'confirmed', got '%s'", retrieved.State)
	}
	if retrieved.ObjectClass != "car" {
		t.Errorf("Expected object_class 'car', got '%s'", retrieved.ObjectClass)
	}
}

func TestUpdateTrack(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert initial track
	track := &TrackedObject{
		TrackID:          "track-update",
		SensorID:         "sensor-001",
		State:            TrackTentative,
		FirstUnixNanos:   1234567890000000000,
		ObservationCount: 3,
		AvgSpeedMps:      5.0,
		speedHistory:     []float32{4, 5, 6},
	}

	err := InsertTrack(db, track, "site/main")
	if err != nil {
		t.Fatalf("InsertTrack failed: %v", err)
	}

	// Update track
	track.State = TrackConfirmed
	track.ObservationCount = 10
	track.AvgSpeedMps = 8.0
	track.ObjectClass = "pedestrian"
	track.ObjectConfidence = 0.75
	track.speedHistory = []float32{6, 7, 8, 9, 8, 7, 8, 9, 8, 7}

	err = UpdateTrack(db, track, "site/main")
	if err != nil {
		t.Fatalf("UpdateTrack failed: %v", err)
	}

	// Verify update
	tracks, err := GetActiveTracks(db, "sensor-001", "")
	if err != nil {
		t.Fatalf("GetActiveTracks failed: %v", err)
	}

	if len(tracks) != 1 {
		t.Fatalf("Expected 1 track, got %d", len(tracks))
	}

	updated := tracks[0]
	if updated.State != TrackConfirmed {
		t.Errorf("Expected state 'confirmed', got '%s'", updated.State)
	}
	if updated.ObservationCount != 10 {
		t.Errorf("Expected observation_count 10, got %d", updated.ObservationCount)
	}
	if updated.ObjectClass != "pedestrian" {
		t.Errorf("Expected object_class 'pedestrian', got '%s'", updated.ObjectClass)
	}
}

func TestInsertAndGetTrackObservations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert a track first
	track := &TrackedObject{
		TrackID:        "track-obs-test",
		SensorID:       "sensor-001",
		State:          TrackConfirmed,
		FirstUnixNanos: 1234567890000000000,
		speedHistory:   []float32{5.0},
	}

	err := InsertTrack(db, track, "site/main")
	if err != nil {
		t.Fatalf("InsertTrack failed: %v", err)
	}

	// Insert observations
	observations := []*TrackObservation{
		{
			TrackID:           "track-obs-test",
			TSUnixNanos:       1234567890000000000,
			WorldFrame:        "site/main",
			X:                 10.0,
			Y:                 20.0,
			Z:                 1.0,
			VelocityX:         5.0,
			VelocityY:         0.0,
			SpeedMps:          5.0,
			HeadingRad:        0.0,
			BoundingBoxLength: 4.0,
			BoundingBoxWidth:  2.0,
			BoundingBoxHeight: 1.5,
			HeightP95:         1.4,
			IntensityMean:     100.0,
		},
		{
			TrackID:           "track-obs-test",
			TSUnixNanos:       1234567891000000000,
			WorldFrame:        "site/main",
			X:                 15.0,
			Y:                 20.0,
			Z:                 1.0,
			VelocityX:         5.0,
			VelocityY:         0.0,
			SpeedMps:          5.0,
			HeadingRad:        0.0,
			BoundingBoxLength: 4.0,
			BoundingBoxWidth:  2.0,
			BoundingBoxHeight: 1.5,
			HeightP95:         1.4,
			IntensityMean:     105.0,
		},
	}

	for _, obs := range observations {
		if err := InsertTrackObservation(db, obs); err != nil {
			t.Fatalf("InsertTrackObservation failed: %v", err)
		}
	}

	// Get observations
	retrieved, err := GetTrackObservations(db, "track-obs-test", 10)
	if err != nil {
		t.Fatalf("GetTrackObservations failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Fatalf("Expected 2 observations, got %d", len(retrieved))
	}

	// Should be in descending order by timestamp
	if retrieved[0].TSUnixNanos < retrieved[1].TSUnixNanos {
		t.Error("Expected observations in descending timestamp order")
	}

	if retrieved[0].X != 15.0 {
		t.Errorf("Expected X=15.0 for most recent observation, got %f", retrieved[0].X)
	}
}

func TestGetActiveTracksFilterByState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert tracks with different states
	tracks := []*TrackedObject{
		{TrackID: "track-1", SensorID: "sensor-001", State: TrackTentative, FirstUnixNanos: 1, speedHistory: []float32{}},
		{TrackID: "track-2", SensorID: "sensor-001", State: TrackConfirmed, FirstUnixNanos: 2, speedHistory: []float32{}},
		{TrackID: "track-3", SensorID: "sensor-001", State: TrackDeleted, FirstUnixNanos: 3, LastUnixNanos: 4, speedHistory: []float32{}},
	}

	for _, track := range tracks {
		if err := InsertTrack(db, track, "site/main"); err != nil {
			t.Fatalf("InsertTrack failed: %v", err)
		}
	}

	// Get only confirmed tracks
	confirmed, err := GetActiveTracks(db, "sensor-001", "confirmed")
	if err != nil {
		t.Fatalf("GetActiveTracks(confirmed) failed: %v", err)
	}
	if len(confirmed) != 1 {
		t.Errorf("Expected 1 confirmed track, got %d", len(confirmed))
	}

	// Get all non-deleted tracks
	active, err := GetActiveTracks(db, "sensor-001", "")
	if err != nil {
		t.Fatalf("GetActiveTracks('') failed: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("Expected 2 active (non-deleted) tracks, got %d", len(active))
	}
}
