package lidar

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// setupTransitTestDB creates an in-memory database for transit testing
func setupTransitTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create the lidar_transits table
	schema := `
		CREATE TABLE IF NOT EXISTS lidar_transits (
			transit_id INTEGER PRIMARY KEY AUTOINCREMENT,
			track_id TEXT NOT NULL UNIQUE,
			sensor_id TEXT NOT NULL,
			transit_start_unix DOUBLE NOT NULL,
			transit_end_unix DOUBLE NOT NULL,
			max_speed_mps REAL,
			min_speed_mps REAL,
			avg_speed_mps REAL,
			p50_speed_mps REAL,
			p85_speed_mps REAL,
			p95_speed_mps REAL,
			track_length_m REAL,
			observation_count INTEGER,
			object_class TEXT,
			classification_confidence REAL,
			quality_score REAL,
			bbox_length_avg REAL,
			bbox_width_avg REAL,
			bbox_height_avg REAL,
			created_at DOUBLE DEFAULT (UNIXEPOCH('subsec'))
		);

		CREATE INDEX IF NOT EXISTS idx_lidar_transits_time ON lidar_transits(transit_start_unix, transit_end_unix);
		CREATE INDEX IF NOT EXISTS idx_lidar_transits_sensor ON lidar_transits(sensor_id);
		CREATE INDEX IF NOT EXISTS idx_lidar_transits_class ON lidar_transits(object_class);
		CREATE INDEX IF NOT EXISTS idx_lidar_transits_speed ON lidar_transits(p85_speed_mps);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func TestInsertTransit(t *testing.T) {
	db := setupTransitTestDB(t)
	defer db.Close()

	store := NewTransitStore(db)

	transit := &LidarTransit{
		TrackID:                  "track-001",
		SensorID:                 "sensor-alpha",
		TransitStartUnix:         1609459200.0, // 2021-01-01 00:00:00 UTC
		TransitEndUnix:           1609459210.0, // 2021-01-01 00:00:10 UTC
		MaxSpeedMps:              15.5,
		MinSpeedMps:              8.0,
		AvgSpeedMps:              12.0,
		P50SpeedMps:              11.5,
		P85SpeedMps:              14.0,
		P95SpeedMps:              15.0,
		TrackLengthM:             120.0,
		ObservationCount:         100,
		ObjectClass:              "car",
		ClassificationConfidence: 0.95,
		QualityScore:             0.85,
		BboxLengthAvg:            4.5,
		BboxWidthAvg:             2.0,
		BboxHeightAvg:            1.5,
	}

	err := store.InsertTransit(transit)
	if err != nil {
		t.Fatalf("failed to insert transit: %v", err)
	}

	if transit.TransitID == 0 {
		t.Error("expected TransitID to be set after insert")
	}

	// Verify the transit was inserted
	transits, err := store.ListTransits("sensor-alpha", 0, 0, 0, 0, 10)
	if err != nil {
		t.Fatalf("failed to list transits: %v", err)
	}

	if len(transits) != 1 {
		t.Fatalf("expected 1 transit, got %d", len(transits))
	}

	inserted := transits[0]
	if inserted.TrackID != "track-001" {
		t.Errorf("expected TrackID 'track-001', got '%s'", inserted.TrackID)
	}
	if inserted.SensorID != "sensor-alpha" {
		t.Errorf("expected SensorID 'sensor-alpha', got '%s'", inserted.SensorID)
	}
	if inserted.P85SpeedMps != 14.0 {
		t.Errorf("expected P85SpeedMps 14.0, got %f", inserted.P85SpeedMps)
	}
}

func TestListTransitsWithFilters(t *testing.T) {
	db := setupTransitTestDB(t)
	defer db.Close()

	store := NewTransitStore(db)

	// Insert multiple transits
	transits := []*LidarTransit{
		{
			TrackID:          "track-001",
			SensorID:         "sensor-alpha",
			TransitStartUnix: 1609459200.0,
			TransitEndUnix:   1609459210.0,
			P85SpeedMps:      10.0,
			AvgSpeedMps:      9.5,
			ObjectClass:      "car",
			QualityScore:     0.8,
			ObservationCount: 50,
		},
		{
			TrackID:          "track-002",
			SensorID:         "sensor-alpha",
			TransitStartUnix: 1609459220.0,
			TransitEndUnix:   1609459230.0,
			P85SpeedMps:      15.0,
			AvgSpeedMps:      14.5,
			ObjectClass:      "pedestrian",
			QualityScore:     0.9,
			ObservationCount: 60,
		},
		{
			TrackID:          "track-003",
			SensorID:         "sensor-beta",
			TransitStartUnix: 1609459240.0,
			TransitEndUnix:   1609459250.0,
			P85SpeedMps:      20.0,
			AvgSpeedMps:      19.5,
			ObjectClass:      "car",
			QualityScore:     0.85,
			ObservationCount: 70,
		},
	}

	for _, tr := range transits {
		if err := store.InsertTransit(tr); err != nil {
			t.Fatalf("failed to insert transit %s: %v", tr.TrackID, err)
		}
	}

	// Test: Filter by sensor_id
	results, err := store.ListTransits("sensor-alpha", 0, 0, 0, 0, 10)
	if err != nil {
		t.Fatalf("failed to list transits by sensor: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 transits for sensor-alpha, got %d", len(results))
	}

	// Test: Filter by speed range
	results, err = store.ListTransits("", 0, 0, 12.0, 18.0, 10)
	if err != nil {
		t.Fatalf("failed to list transits by speed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 transit with speed 12-18 m/s, got %d", len(results))
	}
	if results[0].TrackID != "track-002" {
		t.Errorf("expected track-002, got %s", results[0].TrackID)
	}

	// Test: Filter by time range
	results, err = store.ListTransits("", 1609459215.0, 1609459235.0, 0, 0, 10)
	if err != nil {
		t.Fatalf("failed to list transits by time: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 transit in time range, got %d", len(results))
	}

	// Test: Limit
	results, err = store.ListTransits("", 0, 0, 0, 0, 2)
	if err != nil {
		t.Fatalf("failed to list transits with limit: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 transits with limit=2, got %d", len(results))
	}
}

func TestGetTransitSummary(t *testing.T) {
	db := setupTransitTestDB(t)
	defer db.Close()

	store := NewTransitStore(db)

	// Insert test transits
	transits := []*LidarTransit{
		{
			TrackID:          "track-001",
			SensorID:         "sensor-alpha",
			TransitStartUnix: 1609459200.0,
			TransitEndUnix:   1609459210.0,
			MaxSpeedMps:      12.0,
			AvgSpeedMps:      10.0,
			P50SpeedMps:      9.5,
			P85SpeedMps:      11.0,
			P95SpeedMps:      11.5,
			ObjectClass:      "car",
			QualityScore:     0.8,
			ObservationCount: 50,
		},
		{
			TrackID:          "track-002",
			SensorID:         "sensor-alpha",
			TransitStartUnix: 1609459220.0,
			TransitEndUnix:   1609459230.0,
			MaxSpeedMps:      16.0,
			AvgSpeedMps:      14.0,
			P50SpeedMps:      13.5,
			P85SpeedMps:      15.0,
			P95SpeedMps:      15.5,
			ObjectClass:      "car",
			QualityScore:     0.9,
			ObservationCount: 60,
		},
		{
			TrackID:          "track-003",
			SensorID:         "sensor-alpha",
			TransitStartUnix: 1609459240.0,
			TransitEndUnix:   1609459250.0,
			MaxSpeedMps:      3.0,
			AvgSpeedMps:      2.0,
			P50SpeedMps:      2.0,
			P85SpeedMps:      2.5,
			P95SpeedMps:      2.8,
			ObjectClass:      "pedestrian",
			QualityScore:     0.85,
			ObservationCount: 70,
		},
	}

	for _, tr := range transits {
		if err := store.InsertTransit(tr); err != nil {
			t.Fatalf("failed to insert transit %s: %v", tr.TrackID, err)
		}
	}

	// Get summary
	summary, err := store.GetTransitSummary("sensor-alpha", 0, 0)
	if err != nil {
		t.Fatalf("failed to get transit summary: %v", err)
	}

	if summary.TotalCount != 3 {
		t.Errorf("expected TotalCount 3, got %d", summary.TotalCount)
	}

	// Check class distribution
	if summary.ByClass["car"] != 2 {
		t.Errorf("expected 2 cars, got %d", summary.ByClass["car"])
	}
	if summary.ByClass["pedestrian"] != 1 {
		t.Errorf("expected 1 pedestrian, got %d", summary.ByClass["pedestrian"])
	}

	// Check speed statistics
	if summary.MaxSpeedMps != 16.0 {
		t.Errorf("expected MaxSpeedMps 16.0, got %f", summary.MaxSpeedMps)
	}

	// Check speed buckets
	// track-001: P85 = 11.0 m/s = 39.6 km/h → 30-40 bucket
	// track-002: P85 = 15.0 m/s = 54.0 km/h → 50+ bucket
	// track-003: P85 = 2.5 m/s = 9.0 km/h → 0-20 bucket
	expectedBuckets := map[string]int{
		"0-20":  1, // 2.5 m/s (9 km/h)
		"20-30": 0,
		"30-40": 1, // 11.0 m/s (39.6 km/h)
		"40-50": 0,
		"50+":   1, // 15.0 m/s (54 km/h)
	}

	for bucket, count := range expectedBuckets {
		if summary.SpeedBuckets[bucket] != count {
			t.Errorf("expected SpeedBucket[%s] = %d, got %d", bucket, count, summary.SpeedBuckets[bucket])
		}
	}
}

func TestShouldPromoteToTransit(t *testing.T) {
	tests := []struct {
		name     string
		track    *RunTrack
		expected bool
	}{
		{
			name: "good_vehicle with perfect quality",
			track: &RunTrack{
				UserLabel:        "good_vehicle",
				QualityLabel:     "perfect",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     3000000000,
				ObservationCount: 50,
			},
			expected: true,
		},
		{
			name: "good_pedestrian with good quality",
			track: &RunTrack{
				UserLabel:        "good_pedestrian",
				QualityLabel:     "good",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     3000000000,
				ObservationCount: 50,
			},
			expected: true,
		},
		{
			name: "good_vehicle with truncated quality",
			track: &RunTrack{
				UserLabel:        "good_vehicle",
				QualityLabel:     "truncated",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     3000000000,
				ObservationCount: 50,
			},
			expected: false,
		},
		{
			name: "good_vehicle with noisy_velocity quality",
			track: &RunTrack{
				UserLabel:        "good_vehicle",
				QualityLabel:     "noisy_velocity",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     3000000000,
				ObservationCount: 50,
			},
			expected: false,
		},
		{
			name: "noise label",
			track: &RunTrack{
				UserLabel:        "noise",
				QualityLabel:     "good",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     3000000000,
				ObservationCount: 50,
			},
			expected: false,
		},
		{
			name: "split label",
			track: &RunTrack{
				UserLabel:        "split",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     3000000000,
				ObservationCount: 50,
			},
			expected: false,
		},
		{
			name: "unlabelled with sufficient duration and observations",
			track: &RunTrack{
				UserLabel:        "",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     3500000000, // 2.5 seconds
				ObservationCount: 25,
			},
			expected: true,
		},
		{
			name: "unlabelled with insufficient duration",
			track: &RunTrack{
				UserLabel:        "",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     2500000000, // 1.5 seconds
				ObservationCount: 25,
			},
			expected: false,
		},
		{
			name: "unlabelled with insufficient observations",
			track: &RunTrack{
				UserLabel:        "",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     3500000000, // 2.5 seconds
				ObservationCount: 15,
			},
			expected: false,
		},
		{
			name: "good_vehicle without quality label",
			track: &RunTrack{
				UserLabel:        "good_vehicle",
				QualityLabel:     "",
				StartUnixNanos:   1000000000,
				EndUnixNanos:     3000000000,
				ObservationCount: 50,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldPromoteToTransit(tt.track)
			if result != tt.expected {
				t.Errorf("ShouldPromoteToTransit() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestTransitFromRunTrack(t *testing.T) {
	track := &RunTrack{
		RunID:                "run-001",
		TrackID:              "track-001",
		SensorID:             "sensor-alpha",
		StartUnixNanos:       1609459200000000000, // 2021-01-01 00:00:00 UTC
		EndUnixNanos:         1609459210000000000, // 2021-01-01 00:00:10 UTC
		ObservationCount:     100,
		AvgSpeedMps:          12.0,
		PeakSpeedMps:         15.0,
		P50SpeedMps:          11.5,
		P85SpeedMps:          14.0,
		P95SpeedMps:          14.5,
		BoundingBoxLengthAvg: 4.5,
		BoundingBoxWidthAvg:  2.0,
		BoundingBoxHeightAvg: 1.5,
		ObjectClass:          "car",
		ObjectConfidence:     0.95,
		UserLabel:            "good_vehicle",
		QualityLabel:         "perfect",
	}

	transit := TransitFromRunTrack(track)

	if transit.TrackID != "track-001" {
		t.Errorf("expected TrackID 'track-001', got '%s'", transit.TrackID)
	}

	if transit.SensorID != "sensor-alpha" {
		t.Errorf("expected SensorID 'sensor-alpha', got '%s'", transit.SensorID)
	}

	expectedStartUnix := 1609459200.0
	if transit.TransitStartUnix != expectedStartUnix {
		t.Errorf("expected TransitStartUnix %f, got %f", expectedStartUnix, transit.TransitStartUnix)
	}

	expectedEndUnix := 1609459210.0
	if transit.TransitEndUnix != expectedEndUnix {
		t.Errorf("expected TransitEndUnix %f, got %f", expectedEndUnix, transit.TransitEndUnix)
	}

	if transit.AvgSpeedMps != 12.0 {
		t.Errorf("expected AvgSpeedMps 12.0, got %f", transit.AvgSpeedMps)
	}

	if transit.P85SpeedMps != 14.0 {
		t.Errorf("expected P85SpeedMps 14.0, got %f", transit.P85SpeedMps)
	}

	if transit.ObjectClass != "car" {
		t.Errorf("expected ObjectClass 'car', got '%s'", transit.ObjectClass)
	}

	if transit.QualityScore != 1.0 {
		t.Errorf("expected QualityScore 1.0 for perfect quality, got %f", transit.QualityScore)
	}
}

func TestMain(m *testing.M) {
	// Run tests
	exitCode := m.Run()
	os.Exit(exitCode)
}
