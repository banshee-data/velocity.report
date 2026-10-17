package sqlite

import (
	"database/sql"
	"math"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

// setupTestDB creates a test database via the canonical internal/db bootstrap path.
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	db, cleanup := dbpkg.NewTestDB(t)
	return db.DB, cleanup
}

func TestInsertCluster(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cluster := &WorldCluster{
		SensorID:          "sensor-001",
		FrameID:           "site/main",
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
		TrackID: "track-001", TrackMeasurement: TrackMeasurement{SensorID: "sensor-001",
			TrackState:           TrackConfirmed,
			StartUnixNanos:       1234567890000000000,
			EndUnixNanos:         1234567900000000000,
			ObservationCount:     10,
			AvgSpeedMps:          8.5,
			MaxSpeedMps:          12.0,
			BoundingBoxLengthAvg: 4.0,
			BoundingBoxWidthAvg:  2.0,
			BoundingBoxHeightAvg: 1.5,
			HeightP95Max:         1.4,
			IntensityMeanAvg:     100.0,
			ObjectClass:          "car",
			ObjectConfidence:     0.85,
			ClassificationModel:  "rule-based-v1.0"},
	}
	track.SetSpeedHistory([]float32{7, 8, 9, 8, 9, 10, 8, 9, 8, 9})

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
	if retrieved.TrackState != TrackConfirmed {
		t.Errorf("Expected state 'confirmed', got '%s'", retrieved.TrackState)
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
		TrackID: "track-update", TrackMeasurement: TrackMeasurement{SensorID: "sensor-001",
			TrackState:       TrackTentative,
			StartUnixNanos:   1234567890000000000,
			ObservationCount: 3,
			AvgSpeedMps:      5.0},
	}
	track.SetSpeedHistory([]float32{4, 5, 6})

	err := InsertTrack(db, track, "site/main")
	if err != nil {
		t.Fatalf("InsertTrack failed: %v", err)
	}

	// Update track
	track.TrackState = TrackConfirmed
	track.ObservationCount = 10
	track.AvgSpeedMps = 8.0
	track.EndUnixNanos = 1234567895000000000 // +5 seconds: exercises trackMeasurementUpdateArgs endNanos branch
	track.ObjectClass = "pedestrian"
	track.ObjectConfidence = 0.75
	track.SetSpeedHistory([]float32{6, 7, 8, 9, 8, 7, 8, 9, 8, 7})

	err = UpdateTrack(db, track)
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
	if updated.TrackState != TrackConfirmed {
		t.Errorf("Expected state 'confirmed', got '%s'", updated.TrackState)
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
		TrackID: "track-obs-test", TrackMeasurement: TrackMeasurement{SensorID: "sensor-001",
			TrackState:     TrackConfirmed,
			StartUnixNanos: 1234567890000000000},
	}
	track.SetSpeedHistory([]float32{5.0})

	err := InsertTrack(db, track, "site/main")
	if err != nil {
		t.Fatalf("InsertTrack failed: %v", err)
	}

	// Insert observations
	observations := []*TrackObservation{
		{
			TrackID:           "track-obs-test",
			TSUnixNanos:       1234567890000000000,
			FrameUnixNanos:    1234567889900000000,
			FrameID:           "site/main",
			MeasurementSource: "obb_centre_v1",
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
			FrameUnixNanos:    1234567890900000000,
			FrameID:           "site/main",
			MeasurementSource: "medoid_fallback_v1",
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
	if retrieved[0].FrameUnixNanos != 1234567890900000000 || retrieved[0].MeasurementSource != "medoid_fallback_v1" {
		t.Errorf("measurement provenance was not retained: %+v", retrieved[0])
	}
}

func TestGetActiveTracksFilterByState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert tracks with different states
	tracks := []*TrackedObject{
		{TrackID: "track-1", TrackMeasurement: TrackMeasurement{SensorID: "sensor-001", TrackState: TrackTentative, StartUnixNanos: 1}},
		{TrackID: "track-2", TrackMeasurement: TrackMeasurement{SensorID: "sensor-001", TrackState: TrackConfirmed, StartUnixNanos: 2}},
		{TrackID: "track-3", TrackMeasurement: TrackMeasurement{SensorID: "sensor-001", TrackState: TrackDeleted, StartUnixNanos: 3, EndUnixNanos: 4}},
	}

	for _, track := range tracks {
		track.SetSpeedHistory([]float32{})
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

func TestTrackObservationKeepsThePublishedBox(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	if _, err := db.Exec(`INSERT INTO lidar_tracks (track_id, sensor_id, frame_id, track_state,
		start_unix_nanos, end_unix_nanos, observation_count, avg_speed_mps, max_speed_mps,
		bounding_box_length_avg, bounding_box_width_avg, bounding_box_height_avg,
		height_p95_max, intensity_mean_avg)
		VALUES ('box-t1', 'box-sensor', 'world', 'confirmed', 1000, 3000, 2, 1, 2, 1, 1, 1, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	with := &TrackObservation{TrackID: "box-t1", TSUnixNanos: 1000, FrameID: "world", MeasurementSource: "medoid_v0",
		X: 10, Y: 2, BoundingBoxLength: 1.8, BoundingBoxWidth: 0.4,
		BoxX: 11.5, BoxY: 2.1, BoxHeadingRad: 0.3, BoxLength: 4.4, BoxWidth: 1.8}
	without := &TrackObservation{TrackID: "box-t1", TSUnixNanos: 2000, FrameID: "world", MeasurementSource: "medoid_v0",
		X: 11, Y: 2, BoundingBoxLength: 1.8, BoundingBoxWidth: 0.4}
	for _, o := range []*TrackObservation{with, without} {
		if err := InsertTrackObservation(db, o); err != nil {
			t.Fatal(err)
		}
	}
	var nulls int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_track_observations WHERE box_length IS NULL`).Scan(&nulls); err != nil || nulls != 1 {
		t.Fatalf("an observation without a box is stored as NULL: %d rows (%v)", nulls, err)
	}
	ranged, err := GetTrackObservationsInRange(db, "box-sensor", 0, 5000, 10, "")
	if err != nil || len(ranged) != 2 {
		t.Fatalf("range read: %d, %v", len(ranged), err)
	}
	if got := ranged[0]; !got.HasBox() || got.BoxX != 11.5 || got.BoxY != 2.1 || got.BoxHeadingRad != 0.3 || got.BoxLength != 4.4 || got.BoxWidth != 1.8 {
		t.Fatalf("the box did not round-trip: %+v", got)
	}
	if ranged[1].HasBox() {
		t.Fatalf("a box appeared on an observation without one: %+v", ranged[1])
	}
	latest, err := GetTrackObservations(db, "box-t1", 10)
	if err != nil || len(latest) != 2 || latest[0].HasBox() || !latest[1].HasBox() {
		t.Fatalf("per-track read (newest first): %+v, %v", latest, err)
	}
}
