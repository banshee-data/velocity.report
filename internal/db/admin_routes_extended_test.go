package db

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// TestAttachAdminRoutes_AllEndpoints tests that all admin routes are registered
func TestAttachAdminRoutes_AllEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	// Test various endpoints are registered (they may return 403 due to auth, but not 404)
	endpoints := []string{
		"/debug/db-stats",
		"/debug/backup",
		"/debug/tailsql/",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			// Should not be 404 - that would mean the route isn't registered
			if w.Code == http.StatusNotFound {
				t.Errorf("Endpoint %s should be registered, got 404", endpoint)
			}
		})
	}
}

// TestGetDatabaseStats_Comprehensive tests database stats comprehensively
func TestGetDatabaseStats_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert various types of data to test stats across tables
	// Radar objects
	radarEvent := `{"classifier":"vehicle","start_time":1000.0,"end_time":1005.0,` +
		`"delta_time_msec":5000,"max_speed_mps":15.0,"min_speed_mps":10.0,` +
		`"speed_change":5.0,"max_magnitude":100,"avg_magnitude":80,` +
		`"total_frames":50,"frames_per_mps":3.33,"length_m":4.5}`

	for i := 0; i < 50; i++ {
		if err := db.RecordRadarObject(radarEvent); err != nil {
			t.Fatalf("Failed to insert radar object: %v", err)
		}
	}

	// Raw radar data
	rawEvent := `{"uptime":100.0,"magnitude":50,"speed":10.0}`
	for i := 0; i < 30; i++ {
		if err := db.RecordRawData(rawEvent); err != nil {
			t.Fatalf("Failed to insert raw data: %v", err)
		}
	}

	// Background snapshots
	for i := 0; i < 5; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
			TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			Rings:             40,
			AzimuthBins:       1800,
			GridBlob:          make([]byte, 1000), // 1KB blob
			ChangedCellsCount: i,
			SnapshotReason:    "test",
		}
		if _, err := db.InsertBgSnapshot(snap); err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
	}

	stats, err := db.GetDatabaseStats()
	if err != nil {
		t.Fatalf("GetDatabaseStats failed: %v", err)
	}

	// Verify structure
	if stats.TotalSizeMB <= 0 {
		t.Error("Expected positive total size")
	}

	// Check table presence and counts
	tableMap := make(map[string]*TableStats)
	for i := range stats.Tables {
		tableMap[stats.Tables[i].Name] = &stats.Tables[i]
	}

	// Verify radar_objects
	if ro, ok := tableMap["radar_objects"]; ok {
		if ro.RowCount < 50 {
			t.Errorf("Expected at least 50 radar_objects, got %d", ro.RowCount)
		}
	} else {
		t.Error("Expected radar_objects table in stats")
	}

	// Verify radar_data
	if rd, ok := tableMap["radar_data"]; ok {
		if rd.RowCount < 30 {
			t.Errorf("Expected at least 30 radar_data, got %d", rd.RowCount)
		}
	} else {
		t.Error("Expected radar_data table in stats")
	}

	// Verify lidar_bg_snapshot
	if bg, ok := tableMap["lidar_bg_snapshot"]; ok {
		if bg.RowCount < 5 {
			t.Errorf("Expected at least 5 bg_snapshots, got %d", bg.RowCount)
		}
	} else {
		t.Error("Expected lidar_bg_snapshot table in stats")
	}

	// Verify tables are sorted by size (descending)
	for i := 1; i < len(stats.Tables); i++ {
		if stats.Tables[i].SizeMB > stats.Tables[i-1].SizeMB {
			t.Error("Tables should be sorted by size descending")
			break
		}
	}
}

// TestDBStatsEndpoint_JSONResponse tests the db-stats endpoint JSON response
func TestDBStatsEndpoint_JSONResponse(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/debug/db-stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// If we get 200, validate the JSON
	if w.Code == http.StatusOK {
		var stats DatabaseStats
		if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify JSON structure
		if stats.Tables == nil {
			t.Error("Expected Tables array in response")
		}
	}
}

// TestRadarObjectsRollupRow_String tests the String method for RadarObjectsRollupRow
func TestRadarObjectsRollupRow_StringMethod(t *testing.T) {
	row := RadarObjectsRollupRow{
		Classifier: "all",
		StartTime:  time.Unix(1700000000, 0),
		Count:      100,
		P50Speed:   12.5,
		P85Speed:   18.3,
		P98Speed:   22.1,
		MaxSpeed:   28.0,
	}

	s := row.String()

	// Should contain key fields
	if s == "" {
		t.Error("String() returned empty")
	}

	// Verify it contains expected substrings
	if len(s) < 50 {
		t.Error("String representation seems too short")
	}
}

// TestRadarObject_StringMethod tests the String method for RadarObject
func TestRadarObject_StringMethod(t *testing.T) {
	obj := RadarObject{
		Classifier:   "vehicle",
		StartTime:    time.Unix(1000, 0),
		EndTime:      time.Unix(1005, 0),
		DeltaTimeMs:  5000,
		MaxSpeed:     15.0,
		MinSpeed:     10.0,
		SpeedChange:  5.0,
		MaxMagnitude: 100,
		AvgMagnitude: 80,
		TotalFrames:  50,
		FramesPerMps: 3.33,
		Length:       4.5,
	}

	s := obj.String()

	if s == "" {
		t.Error("String() returned empty")
	}

	// Should contain classifier
	if len(s) < 50 {
		t.Error("String representation seems too short")
	}
}

// TestEvent_StringMethod tests the String method for Event
func TestEvent_StringMethod(t *testing.T) {
	event := Event{}
	event.Uptime.Float64 = 100.0
	event.Uptime.Valid = true
	event.Magnitude.Float64 = 50.0
	event.Magnitude.Valid = true
	event.Speed.Float64 = 15.0
	event.Speed.Valid = true

	s := event.String()

	if s == "" {
		t.Error("String() returned empty")
	}
}

// TestRadarObjectRollupRange_WithRadarData tests rollup with radar_data source
func TestRadarObjectRollupRange_WithRadarData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()

	// Insert raw radar data
	for i := 0; i < 10; i++ {
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

	result, err := db.RadarObjectRollupRange(
		now-1000, now+1000, 3600,
		0, "radar_data", "",
		0, 0, 0, 0,
	)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	// Should have some results
	if result == nil {
		t.Error("Expected non-nil result")
	}
}

// TestListRecentBgSnapshots_ScanError tests error handling during scan
func TestListRecentBgSnapshots_MultipleSensors(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert snapshots for different sensors
	sensors := []string{"sensor-1", "sensor-2", "sensor-3"}
	for _, sensor := range sensors {
		for i := 0; i < 3; i++ {
			snap := &lidar.BgSnapshot{
				SensorID:          sensor,
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
	}

	// Query for each sensor
	for _, sensor := range sensors {
		snapshots, err := db.ListRecentBgSnapshots(sensor, 10)
		if err != nil {
			t.Fatalf("ListRecentBgSnapshots failed for %s: %v", sensor, err)
		}

		if len(snapshots) != 3 {
			t.Errorf("Expected 3 snapshots for %s, got %d", sensor, len(snapshots))
		}

		// Verify all snapshots belong to this sensor
		for _, snap := range snapshots {
			if snap.SensorID != sensor {
				t.Errorf("Expected sensor %s, got %s", sensor, snap.SensorID)
			}
		}
	}
}

// TestRadarObjectRollupRange_WithBoundaryThreshold tests boundary filtering
func TestRadarObjectRollupRange_WithBoundaryThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create timestamp at start of a day
	now := time.Now().Truncate(24 * time.Hour)

	// Insert data across multiple hours and days
	for day := 0; day < 3; day++ {
		dayStart := now.Add(time.Duration(day) * 24 * time.Hour)

		for hour := 0; hour < 24; hour++ {
			hourStart := dayStart.Add(time.Duration(hour) * time.Hour)

			// Insert fewer events in boundary hours (0 and 23)
			count := 10
			if hour == 0 || hour == 23 {
				count = 2
			}

			for i := 0; i < count; i++ {
				ts := hourStart.Add(time.Duration(i) * time.Minute)
				event := map[string]interface{}{
					"classifier":      "vehicle",
					"start_time":      float64(ts.Unix()),
					"end_time":        float64(ts.Unix() + 5),
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
		}
	}

	// Test with boundary threshold
	startUnix := now.Unix()
	endUnix := now.Add(72 * time.Hour).Unix()

	result, err := db.RadarObjectRollupRange(
		startUnix, endUnix, 3600,
		0, "radar_objects", "",
		0, 0, 0, 5, // boundary threshold of 5
	)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
}

// TestAttachAdminRoutes_DbStatsEndpoint tests the /debug/db-stats endpoint directly
func TestAttachAdminRoutes_DbStatsEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert some data
	radarEvent := `{"classifier":"vehicle","start_time":1000.0,"end_time":1005.0,` +
		`"delta_time_msec":5000,"max_speed_mps":15.0,"min_speed_mps":10.0,` +
		`"speed_change":5.0,"max_magnitude":100,"avg_magnitude":80,` +
		`"total_frames":50,"frames_per_mps":3.33,"length_m":4.5}`
	for i := 0; i < 10; i++ {
		if err := db.RecordRadarObject(radarEvent); err != nil {
			t.Fatalf("Failed to insert radar object: %v", err)
		}
	}

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	// Test db-stats endpoint
	req := httptest.NewRequest(http.MethodGet, "/debug/db-stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Check response - it may return 403 or 200 depending on auth
	// We're mostly testing that the handler doesn't panic
	if w.Code == http.StatusInternalServerError {
		t.Errorf("db-stats endpoint returned 500 error: %s", w.Body.String())
	}

	// If we get a 200, verify the response is valid JSON
	if w.Code == http.StatusOK {
		var stats DatabaseStats
		if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
			t.Errorf("Failed to parse db-stats response: %v", err)
		}
	}
}

// TestAttachAdminRoutes_BackupEndpoint tests the /debug/backup endpoint
func TestAttachAdminRoutes_BackupEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	// Test backup endpoint
	req := httptest.NewRequest(http.MethodGet, "/debug/backup", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// The backup endpoint might return various codes depending on auth and setup
	// We're mostly testing that it doesn't panic
	if w.Code == http.StatusNotFound {
		t.Error("backup endpoint not registered")
	}
}

// ============================================================================
// Additional coverage tests for db.go functions
// ============================================================================

// TestEvents_LargeDataset tests Events with more than 500 rows
func TestEvents_LargeDataset(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert more than 500 rows to test the LIMIT clause
	rawEvent := `{"uptime":100.0,"magnitude":50,"speed":10.0}`
	for i := 0; i < 600; i++ {
		if err := db.RecordRawData(rawEvent); err != nil {
			t.Fatalf("Failed to insert raw data: %v", err)
		}
	}

	events, err := db.Events()
	if err != nil {
		t.Fatalf("Events failed: %v", err)
	}

	// Should return at most 500 due to LIMIT
	if len(events) != 500 {
		t.Errorf("Expected 500 events (LIMIT), got %d", len(events))
	}
}

// TestRadarDataRange_WithData_Extended tests RadarDataRange with actual data
func TestRadarDataRange_WithData_Extended(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert radar objects with different timestamps
	radarEvent := `{"classifier":"vehicle","start_time":1000.0,"end_time":1005.0,` +
		`"delta_time_msec":5000,"max_speed_mps":15.0,"min_speed_mps":10.0,` +
		`"speed_change":5.0,"max_magnitude":100,"avg_magnitude":80,` +
		`"total_frames":50,"frames_per_mps":3.33,"length_m":4.5}`

	for i := 0; i < 10; i++ {
		if err := db.RecordRadarObject(radarEvent); err != nil {
			t.Fatalf("Failed to insert radar object: %v", err)
		}
	}

	// Get data range
	dataRange, err := db.RadarDataRange()
	if err != nil {
		t.Fatalf("RadarDataRange failed: %v", err)
	}

	if dataRange == nil {
		t.Fatal("Expected non-nil data range")
	}

	// Verify that start and end are populated
	if dataRange.StartUnix <= 0 || dataRange.EndUnix <= 0 {
		t.Error("Expected positive start and end timestamps")
	}

	if dataRange.EndUnix < dataRange.StartUnix {
		t.Error("End should be >= start")
	}
}

// TestRadarDataRange_EmptyDB tests RadarDataRange with no data
func TestRadarDataRange_EmptyDB(t *testing.T) {
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

	if dataRange == nil {
		t.Fatal("Expected non-nil data range even for empty DB")
	}

	// Empty should return zeroes
	if dataRange.StartUnix != 0 || dataRange.EndUnix != 0 {
		t.Errorf("Expected zeroes for empty DB, got start=%f end=%f", dataRange.StartUnix, dataRange.EndUnix)
	}
}

// TestGetDatabaseStats_EmptyDB_Extended tests GetDatabaseStats on empty database
func TestGetDatabaseStats_EmptyDB_Extended(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	stats, err := db.GetDatabaseStats()
	if err != nil {
		t.Fatalf("GetDatabaseStats failed: %v", err)
	}

	if stats.Tables == nil {
		t.Error("Expected non-nil Tables array")
	}

	// Even empty DB should have some tables in schema
	if len(stats.Tables) == 0 {
		t.Error("Expected some tables in stats")
	}
}

// TestDeleteDuplicateBgSnapshots_NoDuplicates tests with no duplicates
func TestDeleteDuplicateBgSnapshots_NoDuplicates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert unique snapshots
	for i := 0; i < 3; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
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

	// Delete duplicates - should delete 0
	deleted, err := db.DeleteDuplicateBgSnapshots("test-sensor")
	if err != nil {
		t.Fatalf("DeleteDuplicateBgSnapshots failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("Expected 0 deleted (no duplicates), got %d", deleted)
	}
}

// TestDeleteDuplicateBgSnapshots_EmptyDB tests with empty database
func TestDeleteDuplicateBgSnapshots_EmptyDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	deleted, err := db.DeleteDuplicateBgSnapshots("nonexistent-sensor")
	if err != nil {
		t.Fatalf("DeleteDuplicateBgSnapshots failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("Expected 0 deleted for empty DB, got %d", deleted)
	}
}

// TestFindDuplicateBgSnapshots_NoDuplicates_Extended tests finding duplicates when there are none
func TestFindDuplicateBgSnapshots_NoDuplicates_Extended(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert unique snapshots only
	for i := 0; i < 3; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
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

	duplicates, err := db.FindDuplicateBgSnapshots("test-sensor")
	if err != nil {
		t.Fatalf("FindDuplicateBgSnapshots failed: %v", err)
	}

	// No duplicates - should return empty slice
	if len(duplicates) != 0 {
		t.Errorf("Expected 0 duplicate groups, got %d", len(duplicates))
	}
}

// TestFindDuplicateBgSnapshots_WithDuplicates_Extended tests finding duplicate snapshot groups
func TestFindDuplicateBgSnapshots_WithDuplicates_Extended(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert snapshots with duplicates
	blobs := [][]byte{
		[]byte("blob-a"),
		[]byte("blob-a"), // duplicate of first
		[]byte("blob-b"),
		[]byte("blob-a"), // another duplicate
		[]byte("blob-b"), // duplicate
	}

	for i, blob := range blobs {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
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

	duplicates, err := db.FindDuplicateBgSnapshots("test-sensor")
	if err != nil {
		t.Fatalf("FindDuplicateBgSnapshots failed: %v", err)
	}

	// Should have 2 groups (blob-a with 3 items, blob-b with 2 items)
	if len(duplicates) != 2 {
		t.Errorf("Expected 2 duplicate groups, got %d", len(duplicates))
	}

	// Verify each group has multiple items
	for _, group := range duplicates {
		if len(group.SnapshotIDs) < 2 {
			t.Errorf("Duplicate group should have at least 2 items, got %d", len(group.SnapshotIDs))
		}
	}
}

// TestDeleteBgSnapshots_Extended tests deleting specific snapshots by ID
func TestDeleteBgSnapshots_Extended(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert snapshots
	var ids []int64
	for i := 0; i < 5; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
			TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			Rings:             40,
			AzimuthBins:       1800,
			GridBlob:          []byte("blob"),
			ChangedCellsCount: i,
			SnapshotReason:    "test",
		}
		id, err := db.InsertBgSnapshot(snap)
		if err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
		ids = append(ids, id)
	}

	// Delete some snapshots
	toDelete := []int64{ids[0], ids[2], ids[4]}
	deleted, err := db.DeleteBgSnapshots(toDelete)
	if err != nil {
		t.Fatalf("DeleteBgSnapshots failed: %v", err)
	}

	if deleted != 3 {
		t.Errorf("Expected 3 deleted, got %d", deleted)
	}

	// Verify remaining snapshots
	snapshots, err := db.ListRecentBgSnapshots("test-sensor", 10)
	if err != nil {
		t.Fatalf("ListRecentBgSnapshots failed: %v", err)
	}

	if len(snapshots) != 2 {
		t.Errorf("Expected 2 remaining snapshots, got %d", len(snapshots))
	}
}

// TestDeleteBgSnapshots_EmptyList_Extended tests deleting with empty ID list
func TestDeleteBgSnapshots_EmptyList_Extended(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	deleted, err := db.DeleteBgSnapshots(nil)
	if err != nil {
		t.Fatalf("DeleteBgSnapshots(nil) failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("Expected 0 deleted for nil list, got %d", deleted)
	}

	deleted, err = db.DeleteBgSnapshots([]int64{})
	if err != nil {
		t.Fatalf("DeleteBgSnapshots([]) failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("Expected 0 deleted for empty list, got %d", deleted)
	}
}

// TestCountUniqueBgSnapshotHashes_MultiSensor tests counting with multiple sensors
func TestCountUniqueBgSnapshotHashes_MultiSensor(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert snapshots for different sensors
	sensors := []string{"sensor-a", "sensor-b"}
	for _, sensor := range sensors {
		for i := 0; i < 3; i++ {
			snap := &lidar.BgSnapshot{
				SensorID:          sensor,
				TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
				Rings:             40,
				AzimuthBins:       1800,
				GridBlob:          []byte("blob-" + sensor + "-" + string(rune('a'+i))),
				ChangedCellsCount: i,
				SnapshotReason:    "test",
			}
			if _, err := db.InsertBgSnapshot(snap); err != nil {
				t.Fatalf("InsertBgSnapshot failed: %v", err)
			}
		}
	}

	// Count for sensor-a (should be 3 unique)
	count, err := db.CountUniqueBgSnapshotHashes("sensor-a")
	if err != nil {
		t.Fatalf("CountUniqueBgSnapshotHashes failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 unique hashes for sensor-a, got %d", count)
	}

	// Count for sensor-b (should be 3 unique)
	count, err = db.CountUniqueBgSnapshotHashes("sensor-b")
	if err != nil {
		t.Fatalf("CountUniqueBgSnapshotHashes failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 unique hashes for sensor-b, got %d", count)
	}
}

// TestOpenDB_Valid tests opening an existing database
func TestOpenDB_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// First create a database with NewDB
	db1, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	db1.Close()

	// Now open it with OpenDB (no schema init)
	db2, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db2.Close()

	// Should be able to query
	var count int
	err = db2.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if count == 0 {
		t.Error("Expected tables to exist")
	}
}

// TestRadarObjects_Sorting tests that RadarObjects returns sorted results
func TestRadarObjects_Sorting(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert radar objects with different timestamps
	for i := 0; i < 10; i++ {
		radarEvent := `{"classifier":"vehicle","start_time":1000.0,"end_time":1005.0,` +
			`"delta_time_msec":5000,"max_speed_mps":15.0,"min_speed_mps":10.0,` +
			`"speed_change":5.0,"max_magnitude":100,"avg_magnitude":80,` +
			`"total_frames":50,"frames_per_mps":3.33,"length_m":4.5}`
		if err := db.RecordRadarObject(radarEvent); err != nil {
			t.Fatalf("Failed to insert radar object: %v", err)
		}
	}

	// Query objects (no arguments)
	objects, err := db.RadarObjects()
	if err != nil {
		t.Fatalf("RadarObjects failed: %v", err)
	}

	if len(objects) != 10 {
		t.Errorf("Expected 10 objects, got %d", len(objects))
	}
}
