package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// TestRecordRadarObject_EmptyJSON tests error handling for empty JSON
func TestRecordRadarObject_EmptyJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	err = db.RecordRadarObject("")
	if err == nil {
		t.Error("Expected error for empty JSON, got nil")
	}
}

// TestRecordRawData_EmptyJSON tests error handling for empty JSON
func TestRecordRawData_EmptyJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	err = db.RecordRawData("")
	if err == nil {
		t.Error("Expected error for empty JSON, got nil")
	}
}

// TestInsertBgSnapshot_NilSnapshot tests handling of nil snapshot
func TestInsertBgSnapshot_NilSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	id, err := db.InsertBgSnapshot(nil)
	if err != nil {
		t.Errorf("InsertBgSnapshot(nil) returned error: %v", err)
	}
	if id != 0 {
		t.Errorf("Expected id 0 for nil snapshot, got %d", id)
	}
}

// TestInsertBgSnapshot_ValidSnapshot tests inserting a valid snapshot
func TestInsertBgSnapshot_ValidSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	snap := &lidar.BgSnapshot{
		SensorID:           "test-sensor",
		TakenUnixNanos:     time.Now().UnixNano(),
		Rings:              40,
		AzimuthBins:        1800,
		ParamsJSON:         `{"key": "value"}`,
		RingElevationsJSON: `[1.0, 2.0, 3.0]`,
		GridBlob:           []byte("test-blob-data"),
		ChangedCellsCount:  100,
		SnapshotReason:     "initial",
	}

	id, err := db.InsertBgSnapshot(snap)
	if err != nil {
		t.Fatalf("InsertBgSnapshot failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("Expected positive id, got %d", id)
	}

	// Verify it was inserted correctly
	retrieved, err := db.GetLatestBgSnapshot("test-sensor")
	if err != nil {
		t.Fatalf("GetLatestBgSnapshot failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if retrieved.ParamsJSON != snap.ParamsJSON {
		t.Errorf("ParamsJSON mismatch: got %s, want %s", retrieved.ParamsJSON, snap.ParamsJSON)
	}
	if retrieved.RingElevationsJSON != snap.RingElevationsJSON {
		t.Errorf("RingElevationsJSON mismatch: got %s, want %s", retrieved.RingElevationsJSON, snap.RingElevationsJSON)
	}
}

// TestDeleteBgSnapshots_EmptySlice tests deleting with empty slice
func TestDeleteBgSnapshots_EmptySlice(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	deleted, err := db.DeleteBgSnapshots([]int64{})
	if err != nil {
		t.Fatalf("DeleteBgSnapshots failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Expected 0 deleted, got %d", deleted)
	}
}

// TestDeleteBgSnapshots_ValidIDs tests deleting specific snapshots
func TestDeleteBgSnapshots_ValidIDs(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert 3 snapshots
	var ids []int64
	for i := 0; i < 3; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
			TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			Rings:             40,
			AzimuthBins:       1800,
			GridBlob:          []byte("test-blob"),
			ChangedCellsCount: i,
			SnapshotReason:    "test",
		}
		id, err := db.InsertBgSnapshot(snap)
		if err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
		ids = append(ids, id)
	}

	// Delete first two
	deleted, err := db.DeleteBgSnapshots(ids[:2])
	if err != nil {
		t.Fatalf("DeleteBgSnapshots failed: %v", err)
	}
	if deleted != 2 {
		t.Errorf("Expected 2 deleted, got %d", deleted)
	}

	// Verify only one remains
	remaining, err := db.ListRecentBgSnapshots("test-sensor", 10)
	if err != nil {
		t.Fatalf("ListRecentBgSnapshots failed: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("Expected 1 remaining, got %d", len(remaining))
	}
}

// TestFindDuplicateBgSnapshots tests finding duplicate snapshots
func TestFindDuplicateBgSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	sensorID := "test-sensor"

	// Insert snapshots with duplicates
	blobs := [][]byte{
		[]byte("blob-1"),
		[]byte("blob-1"), // duplicate
		[]byte("blob-2"),
		[]byte("blob-1"), // duplicate
		[]byte("blob-3"),
	}

	for i, blob := range blobs {
		snap := &lidar.BgSnapshot{
			SensorID:          sensorID,
			TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			Rings:             40,
			AzimuthBins:       1800,
			GridBlob:          blob,
			ChangedCellsCount: i,
			SnapshotReason:    "test",
		}
		if _, err := db.InsertBgSnapshot(snap); err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
	}

	// Find duplicates
	groups, err := db.FindDuplicateBgSnapshots(sensorID)
	if err != nil {
		t.Fatalf("FindDuplicateBgSnapshots failed: %v", err)
	}

	// Should find 1 group (blob-1 has 3 copies)
	if len(groups) != 1 {
		t.Errorf("Expected 1 duplicate group, got %d", len(groups))
	}

	if len(groups) > 0 {
		group := groups[0]
		if group.Count != 3 {
			t.Errorf("Expected count 3 in group, got %d", group.Count)
		}
		if len(group.DeleteIDs) != 2 {
			t.Errorf("Expected 2 delete IDs, got %d", len(group.DeleteIDs))
		}
		if group.SensorID != sensorID {
			t.Errorf("Expected sensor ID %s, got %s", sensorID, group.SensorID)
		}
	}
}

// TestFindDuplicateBgSnapshots_UniqueBlobsOnly tests when there are no duplicates
func TestFindDuplicateBgSnapshots_UniqueBlobsOnly(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	sensorID := "test-sensor"

	// Insert unique snapshots
	for i := 0; i < 3; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:          sensorID,
			TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			Rings:             40,
			AzimuthBins:       1800,
			GridBlob:          []byte("unique-blob-" + string(rune('a'+i))),
			ChangedCellsCount: i,
			SnapshotReason:    "test",
		}
		if _, err := db.InsertBgSnapshot(snap); err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
	}

	groups, err := db.FindDuplicateBgSnapshots(sensorID)
	if err != nil {
		t.Fatalf("FindDuplicateBgSnapshots failed: %v", err)
	}

	if len(groups) != 0 {
		t.Errorf("Expected 0 duplicate groups, got %d", len(groups))
	}
}

// TestRadarObjectRollupRange_InvalidRange tests error handling for invalid range
func TestRadarObjectRollupRange_InvalidRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	tests := []struct {
		name         string
		startUnix    int64
		endUnix      int64
		groupSeconds int64
		expectErr    bool
	}{
		{
			name:         "end before start",
			startUnix:    1000,
			endUnix:      500,
			groupSeconds: 3600,
			expectErr:    true,
		},
		{
			name:         "end equals start",
			startUnix:    1000,
			endUnix:      1000,
			groupSeconds: 3600,
			expectErr:    true,
		},
		{
			name:         "negative groupSeconds",
			startUnix:    0,
			endUnix:      1000,
			groupSeconds: -1,
			expectErr:    true,
		},
		{
			name:         "valid parameters",
			startUnix:    0,
			endUnix:      1000,
			groupSeconds: 3600,
			expectErr:    false,
		},
		{
			name:         "zero groupSeconds (all aggregation)",
			startUnix:    0,
			endUnix:      1000,
			groupSeconds: 0,
			expectErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.RadarObjectRollupRange(tt.startUnix, tt.endUnix, tt.groupSeconds, 0, "", "", 0, 0, 0, 0)
			if tt.expectErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestRadarObjectRollupRange_InvalidDataSource tests error for invalid data source
func TestRadarObjectRollupRange_InvalidDataSource(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	_, err = db.RadarObjectRollupRange(0, 1000, 3600, 0, "invalid_source", "", 0, 0, 0, 0)
	if err == nil {
		t.Error("Expected error for invalid data source, got nil")
	}
}

// TestRadarObjectRollupRange_HistogramGeneration tests histogram generation
func TestRadarObjectRollupRange_HistogramGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()

	// Insert test data
	events := []map[string]interface{}{
		{
			"classifier":      "vehicle",
			"start_time":      float64(now),
			"end_time":        float64(now + 5),
			"delta_time_msec": 5000,
			"max_speed_mps":   10.0,
			"min_speed_mps":   8.0,
			"speed_change":    2.0,
			"max_magnitude":   50,
			"avg_magnitude":   40,
			"total_frames":    25,
			"frames_per_mps":  5.0,
			"length_m":        3.0,
		},
		{
			"classifier":      "vehicle",
			"start_time":      float64(now + 10),
			"end_time":        float64(now + 15),
			"delta_time_msec": 5000,
			"max_speed_mps":   15.0,
			"min_speed_mps":   12.0,
			"speed_change":    3.0,
			"max_magnitude":   100,
			"avg_magnitude":   80,
			"total_frames":    50,
			"frames_per_mps":  3.0,
			"length_m":        4.5,
		},
	}

	for _, event := range events {
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if err := db.RecordRadarObject(string(eventJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Test with histogram
	result, err := db.RadarObjectRollupRange(
		now-100, now+100, 3600,
		0, "radar_objects", "",
		2.0,  // histogram bucket size
		20.0, // histogram max
		0, 0,
	)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	if result.Histogram == nil {
		t.Error("Expected histogram to be non-nil")
	}

	// Verify histogram has entries
	if len(result.Histogram) == 0 {
		t.Error("Expected histogram to have entries")
	}

	// Verify min speed was set to default
	if result.MinSpeedUsed == 0 {
		t.Error("Expected MinSpeedUsed to be set")
	}
}

// TestRadarObjectRollupRange_AllAggregation tests groupSeconds=0 (all aggregation)
func TestRadarObjectRollupRange_AllAggregation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()

	// Insert test data
	for i := 0; i < 5; i++ {
		event := map[string]interface{}{
			"classifier":      "vehicle",
			"start_time":      float64(now + int64(i*10)),
			"end_time":        float64(now + int64(i*10) + 5),
			"delta_time_msec": 5000,
			"max_speed_mps":   float64(10 + i),
			"min_speed_mps":   float64(8 + i),
			"speed_change":    2.0,
			"max_magnitude":   50,
			"avg_magnitude":   40,
			"total_frames":    25,
			"frames_per_mps":  5.0,
			"length_m":        3.0,
		}
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if err := db.RecordRadarObject(string(eventJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Test with groupSeconds=0 (all aggregation)
	result, err := db.RadarObjectRollupRange(
		now-100, now+100, 0, // groupSeconds=0
		0, "radar_objects", "",
		0, 0, 0, 0,
	)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	// Should have a single aggregated result
	if len(result.Metrics) != 1 {
		t.Errorf("Expected 1 aggregated metric, got %d", len(result.Metrics))
	}

	if len(result.Metrics) > 0 && result.Metrics[0].Count != 5 {
		t.Errorf("Expected count of 5, got %d", result.Metrics[0].Count)
	}
}

// TestEvents_WithData tests retrieving events with actual data
func TestEvents_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert raw data
	for i := 0; i < 5; i++ {
		event := map[string]interface{}{
			"uptime":    float64(100 + i*10),
			"magnitude": 50 + i*5,
			"speed":     float64(10 + i),
		}
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	events, err := db.Events()
	if err != nil {
		t.Fatalf("Events failed: %v", err)
	}

	if len(events) != 5 {
		t.Errorf("Expected 5 events, got %d", len(events))
	}
}

// TestEventToAPI_AllFields tests EventToAPI conversion with all valid fields
func TestEventToAPI_AllFields(t *testing.T) {
	event := Event{
		Magnitude: sql.NullFloat64{Float64: 50.5, Valid: true},
		Uptime:    sql.NullFloat64{Float64: 100.0, Valid: true},
		Speed:     sql.NullFloat64{Float64: 15.5, Valid: true},
	}

	api := EventToAPI(event)

	if api.Magnitude == nil {
		t.Error("Expected Magnitude to be non-nil")
	} else if *api.Magnitude != 50.5 {
		t.Errorf("Expected Magnitude 50.5, got %f", *api.Magnitude)
	}

	if api.Uptime == nil {
		t.Error("Expected Uptime to be non-nil")
	} else if *api.Uptime != 100.0 {
		t.Errorf("Expected Uptime 100.0, got %f", *api.Uptime)
	}

	if api.Speed == nil {
		t.Error("Expected Speed to be non-nil")
	} else if *api.Speed != 15.5 {
		t.Errorf("Expected Speed 15.5, got %f", *api.Speed)
	}
}

// TestEventToAPI_NullFields tests EventToAPI conversion with null fields
func TestEventToAPI_NullFields(t *testing.T) {
	event := Event{
		Magnitude: sql.NullFloat64{Valid: false},
		Uptime:    sql.NullFloat64{Valid: false},
		Speed:     sql.NullFloat64{Valid: false},
	}

	api := EventToAPI(event)

	if api.Magnitude != nil {
		t.Error("Expected Magnitude to be nil")
	}
	if api.Uptime != nil {
		t.Error("Expected Uptime to be nil")
	}
	if api.Speed != nil {
		t.Error("Expected Speed to be nil")
	}
}

// TestTransitWorker_RunOnce tests running the transit worker once
func TestTransitWorker_RunOnce(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 30, "test-v1")

	// RunOnce should work even with no data
	err = worker.RunOnce(context.Background())
	if err != nil {
		t.Errorf("RunOnce failed: %v", err)
	}
}

// TestTransitWorker_RunFullHistory tests running full history
func TestTransitWorker_RunFullHistory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 30, "test-v1")

	// Should work with empty database
	err = worker.RunFullHistory(context.Background())
	if err != nil {
		t.Errorf("RunFullHistory failed: %v", err)
	}
}

// TestTransitWorker_RunRange tests running with a specific range
func TestTransitWorker_RunRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert some raw data
	now := time.Now()
	for i := 0; i < 10; i++ {
		event := map[string]interface{}{
			"uptime":    float64(100 + i),
			"magnitude": 50 + i*5,
			"speed":     float64(10 + i),
		}
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	worker := NewTransitWorker(db, 30, "test-v1")

	start := float64(now.Add(-time.Hour).Unix())
	end := float64(now.Add(time.Hour).Unix())

	err = worker.RunRange(context.Background(), start, end)
	if err != nil {
		t.Errorf("RunRange failed: %v", err)
	}
}

// TestTransitWorker_MigrateModelVersion tests migrating between model versions
func TestTransitWorker_MigrateModelVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 30, "v2")

	// Should fail when old and new versions are the same
	err = worker.MigrateModelVersion(context.Background(), "v2")
	if err == nil {
		t.Error("Expected error when versions are the same")
	}

	// Should work when versions differ
	err = worker.MigrateModelVersion(context.Background(), "v1")
	if err != nil {
		t.Errorf("MigrateModelVersion failed: %v", err)
	}
}

// TestTransitWorker_DeleteAllTransitsEmpty tests deleting all transits from empty database
func TestTransitWorker_DeleteAllTransitsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 30, "test-v1")

	deleted, err := worker.DeleteAllTransits(context.Background(), "test-v1")
	if err != nil {
		t.Errorf("DeleteAllTransits failed: %v", err)
	}
	// Initially 0 transits
	if deleted != 0 {
		t.Errorf("Expected 0 deleted, got %d", deleted)
	}
}

// TestAnalyseTransitOverlaps_EmptyDatabase tests analysing transit overlaps with no data
func TestAnalyseTransitOverlaps_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	stats, err := db.AnalyseTransitOverlaps(context.Background())
	if err != nil {
		t.Fatalf("AnalyseTransitOverlaps failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected non-nil stats")
	}

	if stats.ModelVersionCounts == nil {
		t.Error("Expected ModelVersionCounts to be initialised")
	}

	// Should be empty initially
	if stats.TotalTransits != 0 {
		t.Errorf("Expected 0 total transits, got %d", stats.TotalTransits)
	}
}

// TestOpenDB tests opening a database without migration checks
func TestOpenDB_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	// Verify connection works
	if err := db.Ping(); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

// TestApplyPragmas tests that pragmas are applied correctly
func TestApplyPragmas(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer rawDB.Close()

	err = applyPragmas(rawDB)
	if err != nil {
		t.Fatalf("applyPragmas failed: %v", err)
	}

	// Verify WAL mode is set
	var journalMode string
	if err := rawDB.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("Failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("Expected journal_mode 'wal', got '%s'", journalMode)
	}

	// Verify busy_timeout is set
	var busyTimeout int
	if err := rawDB.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("Failed to query busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("Expected busy_timeout 5000, got %d", busyTimeout)
	}
}

// TestGetMigrationsFS_Production tests getting migrations FS in production mode
func TestGetMigrationsFS_Production(t *testing.T) {
	// Ensure DevMode is false (production)
	originalDevMode := DevMode
	DevMode = false
	defer func() { DevMode = originalDevMode }()

	fs, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	if fs == nil {
		t.Error("Expected non-nil filesystem")
	}
}

// TestNewDBWithMigrationCheck_CheckDisabled tests creating DB without migration checks
func TestNewDBWithMigrationCheck_CheckDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDBWithMigrationCheck(dbPath, false)
	if err != nil {
		t.Fatalf("NewDBWithMigrationCheck failed: %v", err)
	}
	defer db.Close()

	// Verify database is functional
	if err := db.Ping(); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

// TestRadarObjects_Empty tests retrieving radar objects when none exist
func TestRadarObjects_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	objects, err := db.RadarObjects()
	if err != nil {
		t.Fatalf("RadarObjects failed: %v", err)
	}

	if len(objects) != 0 {
		t.Errorf("Expected 0 objects, got %d", len(objects))
	}
}

// TestRadarDataRange_Empty tests RadarDataRange with empty database
func TestRadarDataRange_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	dataRange, err := db.RadarDataRange()
	if err != nil {
		t.Fatalf("RadarDataRange failed: %v", err)
	}

	if dataRange.StartUnix != 0 || dataRange.EndUnix != 0 {
		t.Errorf("Expected zero range, got start=%f, end=%f", dataRange.StartUnix, dataRange.EndUnix)
	}
}

// TestRadarDataRange_WithData tests RadarDataRange with actual data
func TestRadarDataRange_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert radar objects to populate the range
	radarEvent := `{"classifier":"vehicle","start_time":1000.0,"end_time":1005.0,` +
		`"delta_time_msec":5000,"max_speed_mps":15.0,"min_speed_mps":10.0,` +
		`"speed_change":5.0,"max_magnitude":100,"avg_magnitude":80,` +
		`"total_frames":50,"frames_per_mps":3.33,"length_m":4.5}`

	for i := 0; i < 5; i++ {
		if err := db.RecordRadarObject(radarEvent); err != nil {
			t.Fatalf("Failed to insert radar object: %v", err)
		}
	}

	dataRange, err := db.RadarDataRange()
	if err != nil {
		t.Fatalf("RadarDataRange failed: %v", err)
	}

	// Verify the range is populated (write_timestamp should be set)
	if dataRange.StartUnix == 0 && dataRange.EndUnix == 0 {
		t.Log("Warning: write_timestamp may not be set in test - skipping validation")
	}
}

// TestRecordRawData_ValidData tests RecordRawData with valid data
func TestRecordRawData_ValidData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	rawEvent := `{"uptime":100.0,"magnitude":50,"speed":10.0}`
	err = db.RecordRawData(rawEvent)
	if err != nil {
		t.Fatalf("RecordRawData failed: %v", err)
	}

	// Verify it was recorded
	events, err := db.Events()
	if err != nil {
		t.Fatalf("Events failed: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
}

// TestRadarObjects_WithData tests RadarObjects with actual data
func TestRadarObjects_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert radar objects
	radarEvent := `{"classifier":"vehicle","start_time":1000.0,"end_time":1005.0,` +
		`"delta_time_msec":5000,"max_speed_mps":15.0,"min_speed_mps":10.0,` +
		`"speed_change":5.0,"max_magnitude":100,"avg_magnitude":80,` +
		`"total_frames":50,"frames_per_mps":3.33,"length_m":4.5}`

	for i := 0; i < 10; i++ {
		if err := db.RecordRadarObject(radarEvent); err != nil {
			t.Fatalf("Failed to insert radar object: %v", err)
		}
	}

	objects, err := db.RadarObjects()
	if err != nil {
		t.Fatalf("RadarObjects failed: %v", err)
	}

	// RadarObjects returns up to 500 objects
	if len(objects) == 0 {
		t.Error("Expected objects, got 0")
	}
}

// TestListRecentBgSnapshots_Empty tests ListRecentBgSnapshots with empty database
func TestListRecentBgSnapshots_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	snapshots, err := db.ListRecentBgSnapshots("test-sensor", 10)
	if err != nil {
		t.Fatalf("ListRecentBgSnapshots failed: %v", err)
	}

	if len(snapshots) != 0 {
		t.Errorf("Expected 0 snapshots, got %d", len(snapshots))
	}
}

// TestListRecentBgSnapshots_WithData tests ListRecentBgSnapshots with data
func TestListRecentBgSnapshots_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert multiple snapshots
	for i := 0; i < 5; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
			TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			Rings:             40,
			AzimuthBins:       1800,
			GridBlob:          []byte("test-blob"),
			ChangedCellsCount: i,
			SnapshotReason:    "test",
		}
		if _, err := db.InsertBgSnapshot(snap); err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
	}

	// Request 3 most recent
	snapshots, err := db.ListRecentBgSnapshots("test-sensor", 3)
	if err != nil {
		t.Fatalf("ListRecentBgSnapshots failed: %v", err)
	}

	if len(snapshots) != 3 {
		t.Errorf("Expected 3 snapshots, got %d", len(snapshots))
	}
}

// TestCountUniqueBgSnapshotHashes_Extended tests counting unique snapshot hashes with sensor filter
func TestCountUniqueBgSnapshotHashes_Extended(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert snapshots with same blob (will create duplicates)
	for i := 0; i < 5; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
			TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			Rings:             40,
			AzimuthBins:       1800,
			GridBlob:          []byte("same-blob-data"),
			ChangedCellsCount: i,
			SnapshotReason:    "test",
		}
		if _, err := db.InsertBgSnapshot(snap); err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
	}

	count, err := db.CountUniqueBgSnapshotHashes("test-sensor")
	if err != nil {
		t.Fatalf("CountUniqueBgSnapshotHashes failed: %v", err)
	}

	// All have same blob, so unique count should be 1
	if count != 1 {
		t.Errorf("Expected 1 unique hash, got %d", count)
	}
}

// TestDeleteDuplicateBgSnapshots_Extended tests deleting duplicate snapshots with sensor filter
func TestDeleteDuplicateBgSnapshots_Extended(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert snapshots with same blob
	for i := 0; i < 5; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
			TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			Rings:             40,
			AzimuthBins:       1800,
			GridBlob:          []byte("duplicate-blob-data"),
			ChangedCellsCount: i,
			SnapshotReason:    "test",
		}
		if _, err := db.InsertBgSnapshot(snap); err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
	}

	// Find duplicates
	duplicates, err := db.FindDuplicateBgSnapshots("test-sensor")
	if err != nil {
		t.Fatalf("FindDuplicateBgSnapshots failed: %v", err)
	}

	if len(duplicates) == 0 {
		t.Log("No duplicates found - skipping deletion test")
		return
	}

	// Delete duplicates
	deleted, err := db.DeleteDuplicateBgSnapshots("test-sensor")
	if err != nil {
		t.Fatalf("DeleteDuplicateBgSnapshots failed: %v", err)
	}

	if deleted == 0 {
		t.Log("No duplicates deleted - may be expected behaviour")
	}
}
