package db

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// TestGetDatabaseStats tests the GetDatabaseStats function
func TestGetDatabaseStats(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Get stats from empty database (should have schema tables)
	stats, err := db.GetDatabaseStats()
	if err != nil {
		t.Fatalf("GetDatabaseStats failed: %v", err)
	}

	if stats.TotalSizeMB <= 0 {
		t.Error("Expected non-zero total size for database")
	}

	if len(stats.Tables) == 0 {
		t.Error("Expected at least one table in stats")
	}

	// Add some test data
	event := map[string]interface{}{
		"classifier":      "vehicle",
		"start_time":      float64(time.Now().Unix()),
		"end_time":        float64(time.Now().Unix() + 5),
		"delta_time_msec": 5000,
		"max_speed_mps":   15.0,
		"min_speed_mps":   10.0,
		"speed_change":    5.0,
		"max_magnitude":   100,
		"avg_magnitude":   80,
		"total_frames":    50,
		"frames_per_mps":  3.33,
		"length_m":        4.5,
	}
	eventJSON, _ := json.Marshal(event)
	err = db.RecordRadarObject(string(eventJSON))
	if err != nil {
		t.Fatalf("RecordRadarObject failed: %v", err)
	}

	// Get stats again with data
	stats, err = db.GetDatabaseStats()
	if err != nil {
		t.Fatalf("GetDatabaseStats failed after adding data: %v", err)
	}

	// Verify tables are present and sorted by size
	foundRadarObjects := false
	var prevSize float64 = math.MaxFloat64 // Start with max value for descending sort check
	for _, table := range stats.Tables {
		if table.Name == "radar_objects" {
			foundRadarObjects = true
			if table.RowCount != 1 {
				t.Errorf("Expected 1 row in radar_objects, got %d", table.RowCount)
			}
		}
		// Verify tables are sorted descending by size
		if table.SizeMB > prevSize {
			t.Errorf("Tables not sorted by size descending: %s (%.2f MB) after %.2f MB",
				table.Name, table.SizeMB, prevSize)
		}
		prevSize = table.SizeMB
	}

	if !foundRadarObjects {
		t.Error("Expected radar_objects table in stats")
	}
}

// TestGetDatabaseStats_EmptyDB tests GetDatabaseStats with a fresh database
func TestGetDatabaseStats_EmptyDB(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	stats, err := db.GetDatabaseStats()
	if err != nil {
		t.Fatalf("GetDatabaseStats failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected non-nil stats")
	}

	// Should have migration tables at minimum
	if len(stats.Tables) == 0 {
		t.Error("Expected at least migration tables in empty database")
	}
}

// TestFindDuplicateBgSnapshots_NoDuplicates tests when there are no duplicates
func TestFindDuplicateBgSnapshots_NoDuplicates(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	sensorID := "test-sensor-1"

	// Insert unique snapshots
	snap1 := &l3grid.BgSnapshot{
		SensorID:       sensorID,
		TakenUnixNanos: time.Now().UnixNano(),
		Rings:          16,
		AzimuthBins:    360,
		ParamsJSON:     "{}",
		GridBlob:       []byte("unique-data-1"),
		SnapshotReason: "test",
	}

	snap2 := &l3grid.BgSnapshot{
		SensorID:       sensorID,
		TakenUnixNanos: time.Now().UnixNano() + 1000,
		Rings:          16,
		AzimuthBins:    360,
		ParamsJSON:     "{}",
		GridBlob:       []byte("unique-data-2"),
		SnapshotReason: "test",
	}

	_, err := db.InsertBgSnapshot(snap1)
	if err != nil {
		t.Fatalf("Failed to insert snapshot 1: %v", err)
	}

	_, err = db.InsertBgSnapshot(snap2)
	if err != nil {
		t.Fatalf("Failed to insert snapshot 2: %v", err)
	}

	// Find duplicates
	duplicates, err := db.FindDuplicateBgSnapshots(sensorID)
	if err != nil {
		t.Fatalf("FindDuplicateBgSnapshots failed: %v", err)
	}

	if len(duplicates) != 0 {
		t.Errorf("Expected no duplicates, got %d groups", len(duplicates))
	}
}

// TestFindDuplicateBgSnapshots_WithDuplicates tests when duplicates exist
func TestFindDuplicateBgSnapshots_WithDuplicates(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	sensorID := "test-sensor-2"

	// Insert snapshots with duplicate blob data
	duplicateBlob := []byte("duplicate-data")

	snap1 := &l3grid.BgSnapshot{
		SensorID:       sensorID,
		TakenUnixNanos: time.Now().UnixNano(),
		Rings:          16,
		AzimuthBins:    360,
		ParamsJSON:     "{}",
		GridBlob:       duplicateBlob,
		SnapshotReason: "test",
	}

	snap2 := &l3grid.BgSnapshot{
		SensorID:       sensorID,
		TakenUnixNanos: time.Now().UnixNano() + 1000,
		Rings:          16,
		AzimuthBins:    360,
		ParamsJSON:     "{}",
		GridBlob:       duplicateBlob,
		SnapshotReason: "test",
	}

	snap3 := &l3grid.BgSnapshot{
		SensorID:       sensorID,
		TakenUnixNanos: time.Now().UnixNano() + 2000,
		Rings:          16,
		AzimuthBins:    360,
		ParamsJSON:     "{}",
		GridBlob:       duplicateBlob,
		SnapshotReason: "test",
	}

	id1, err := db.InsertBgSnapshot(snap1)
	if err != nil {
		t.Fatalf("Failed to insert snapshot 1: %v", err)
	}

	id2, err := db.InsertBgSnapshot(snap2)
	if err != nil {
		t.Fatalf("Failed to insert snapshot 2: %v", err)
	}

	id3, err := db.InsertBgSnapshot(snap3)
	if err != nil {
		t.Fatalf("Failed to insert snapshot 3: %v", err)
	}

	// Find duplicates
	duplicates, err := db.FindDuplicateBgSnapshots(sensorID)
	if err != nil {
		t.Fatalf("FindDuplicateBgSnapshots failed: %v", err)
	}

	if len(duplicates) != 1 {
		t.Fatalf("Expected 1 duplicate group, got %d", len(duplicates))
	}

	group := duplicates[0]

	if group.Count != 3 {
		t.Errorf("Expected count of 3, got %d", group.Count)
	}

	if group.SensorID != sensorID {
		t.Errorf("Expected sensor ID %s, got %s", sensorID, group.SensorID)
	}

	if group.KeepID != id1 {
		t.Errorf("Expected to keep oldest snapshot %d, got %d", id1, group.KeepID)
	}

	if len(group.DeleteIDs) != 2 {
		t.Errorf("Expected 2 IDs to delete, got %d", len(group.DeleteIDs))
	}

	if group.DeleteIDs[0] != id2 || group.DeleteIDs[1] != id3 {
		t.Errorf("Expected delete IDs [%d, %d], got %v", id2, id3, group.DeleteIDs)
	}

	if group.BlobBytes != len(duplicateBlob) {
		t.Errorf("Expected blob size %d, got %d", len(duplicateBlob), group.BlobBytes)
	}
}

// TestFindDuplicateBgSnapshots_DifferentSensors tests isolation between sensors
func TestFindDuplicateBgSnapshots_DifferentSensors(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	duplicateBlob := []byte("duplicate-data")

	// Insert same blob for different sensors
	snap1 := &l3grid.BgSnapshot{
		SensorID:       "sensor-a",
		TakenUnixNanos: time.Now().UnixNano(),
		Rings:          16,
		AzimuthBins:    360,
		ParamsJSON:     "{}",
		GridBlob:       duplicateBlob,
		SnapshotReason: "test",
	}

	snap2 := &l3grid.BgSnapshot{
		SensorID:       "sensor-b",
		TakenUnixNanos: time.Now().UnixNano() + 1000,
		Rings:          16,
		AzimuthBins:    360,
		ParamsJSON:     "{}",
		GridBlob:       duplicateBlob,
		SnapshotReason: "test",
	}

	_, err := db.InsertBgSnapshot(snap1)
	if err != nil {
		t.Fatalf("Failed to insert snapshot 1: %v", err)
	}

	_, err = db.InsertBgSnapshot(snap2)
	if err != nil {
		t.Fatalf("Failed to insert snapshot 2: %v", err)
	}

	// Find duplicates for sensor-a (should find none)
	duplicatesA, err := db.FindDuplicateBgSnapshots("sensor-a")
	if err != nil {
		t.Fatalf("FindDuplicateBgSnapshots failed for sensor-a: %v", err)
	}

	if len(duplicatesA) != 0 {
		t.Errorf("Expected no duplicates for sensor-a, got %d groups", len(duplicatesA))
	}

	// Find duplicates for sensor-b (should find none)
	duplicatesB, err := db.FindDuplicateBgSnapshots("sensor-b")
	if err != nil {
		t.Fatalf("FindDuplicateBgSnapshots failed for sensor-b: %v", err)
	}

	if len(duplicatesB) != 0 {
		t.Errorf("Expected no duplicates for sensor-b, got %d groups", len(duplicatesB))
	}
}

// TestDeleteBgSnapshots tests deleting snapshots by ID
func TestDeleteBgSnapshots(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	sensorID := "test-sensor-delete"

	// Insert test snapshots
	snap1 := &l3grid.BgSnapshot{
		SensorID:       sensorID,
		TakenUnixNanos: time.Now().UnixNano(),
		Rings:          16,
		AzimuthBins:    360,
		ParamsJSON:     "{}",
		GridBlob:       []byte("data-1"),
		SnapshotReason: "test",
	}

	snap2 := &l3grid.BgSnapshot{
		SensorID:       sensorID,
		TakenUnixNanos: time.Now().UnixNano() + 1000,
		Rings:          16,
		AzimuthBins:    360,
		ParamsJSON:     "{}",
		GridBlob:       []byte("data-2"),
		SnapshotReason: "test",
	}

	snap3 := &l3grid.BgSnapshot{
		SensorID:       sensorID,
		TakenUnixNanos: time.Now().UnixNano() + 2000,
		Rings:          16,
		AzimuthBins:    360,
		ParamsJSON:     "{}",
		GridBlob:       []byte("data-3"),
		SnapshotReason: "test",
	}

	id1, err := db.InsertBgSnapshot(snap1)
	if err != nil {
		t.Fatalf("Failed to insert snapshot 1: %v", err)
	}

	id2, err := db.InsertBgSnapshot(snap2)
	if err != nil {
		t.Fatalf("Failed to insert snapshot 2: %v", err)
	}

	id3, err := db.InsertBgSnapshot(snap3)
	if err != nil {
		t.Fatalf("Failed to insert snapshot 3: %v", err)
	}

	// Verify all 3 exist
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_bg_snapshot WHERE sensor_id = ?", sensorID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count snapshots: %v", err)
	}
	if count != 3 {
		t.Fatalf("Expected 3 snapshots, got %d", count)
	}

	// Delete two snapshots
	deleted, err := db.DeleteBgSnapshots([]int64{id2, id3})
	if err != nil {
		t.Fatalf("DeleteBgSnapshots failed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("Expected 2 rows deleted, got %d", deleted)
	}

	// Verify only one remains
	err = db.QueryRow("SELECT COUNT(*) FROM lidar_bg_snapshot WHERE sensor_id = ?", sensorID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count snapshots after delete: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 snapshot remaining, got %d", count)
	}

	// Verify the correct one remains
	var remainingID int64
	err = db.QueryRow("SELECT snapshot_id FROM lidar_bg_snapshot WHERE sensor_id = ?", sensorID).Scan(&remainingID)
	if err != nil {
		t.Fatalf("Failed to get remaining snapshot ID: %v", err)
	}
	if remainingID != id1 {
		t.Errorf("Expected remaining snapshot ID %d, got %d", id1, remainingID)
	}
}

// TestDeleteBgSnapshots_EmptyList tests deleting with empty ID list
func TestDeleteBgSnapshots_EmptyList(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	deleted, err := db.DeleteBgSnapshots([]int64{})
	if err != nil {
		t.Fatalf("DeleteBgSnapshots failed with empty list: %v", err)
	}

	if deleted != 0 {
		t.Errorf("Expected 0 rows deleted with empty list, got %d", deleted)
	}
}

// TestDeleteBgSnapshots_NonexistentIDs tests deleting non-existent IDs
func TestDeleteBgSnapshots_NonexistentIDs(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Use obviously invalid snapshot IDs (very large values that won't exist)
	const (
		nonexistentID1 int64 = 9999999999
		nonexistentID2 int64 = 9999999998
	)

	// Try to delete non-existent IDs
	deleted, err := db.DeleteBgSnapshots([]int64{nonexistentID1, nonexistentID2})
	if err != nil {
		t.Fatalf("DeleteBgSnapshots failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("Expected 0 rows deleted for non-existent IDs, got %d", deleted)
	}
}
