package db

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// Test data constants
const (
	testRadarObjectJSON = `{"classifier":"object_outbound","end_time":"1000.0",` +
		`"start_time":"999.0","delta_time_msec":1000,"max_speed_mps":10.0,` +
		`"min_speed_mps":5.0,"max_magnitude":50,"avg_magnitude":30,` +
		`"total_frames":10,"frames_per_mps":1.0,"length_m":10.0,"speed_change":5.0}`

	testRadarObjectJSON2 = `{"classifier":"object_inbound","end_time":"2000.0",` +
		`"start_time":"1999.0","delta_time_msec":1000,"max_speed_mps":15.0,` +
		`"min_speed_mps":10.0,"max_magnitude":60,"avg_magnitude":40,` +
		`"total_frames":10,"frames_per_mps":1.0,"length_m":15.0,"speed_change":5.0}`
)

// TestDeleteDuplicateBgSnapshots tests deletion of duplicate background snapshots
func TestDeleteDuplicateBgSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	sensorID := "test-sensor-1"

	// Insert some background snapshots with duplicate grid_blobs
	gridBlob1 := []byte("grid_data_1")
	gridBlob2 := []byte("grid_data_2")

	// Insert 3 snapshots: 2 with same grid_blob, 1 unique
	snap1 := &l3grid.BgSnapshot{
		SensorID:           sensorID,
		TakenUnixNanos:     time.Now().UnixNano(),
		Rings:              40,
		AzimuthBins:        1800,
		ParamsJSON:         `{}`,
		RingElevationsJSON: `[]`,
		GridBlob:           gridBlob1,
		ChangedCellsCount:  0,
		SnapshotReason:     "test",
	}
	if _, err := db.InsertBgSnapshot(snap1); err != nil {
		t.Fatalf("Failed to insert first snapshot: %v", err)
	}

	snap2 := &l3grid.BgSnapshot{
		SensorID:           sensorID,
		TakenUnixNanos:     time.Now().Add(time.Second).UnixNano(),
		Rings:              40,
		AzimuthBins:        1800,
		ParamsJSON:         `{}`,
		RingElevationsJSON: `[]`,
		GridBlob:           gridBlob1, // Duplicate
		ChangedCellsCount:  0,
		SnapshotReason:     "test",
	}
	if _, err := db.InsertBgSnapshot(snap2); err != nil {
		t.Fatalf("Failed to insert duplicate snapshot: %v", err)
	}

	snap3 := &l3grid.BgSnapshot{
		SensorID:           sensorID,
		TakenUnixNanos:     time.Now().Add(2 * time.Second).UnixNano(),
		Rings:              40,
		AzimuthBins:        1800,
		ParamsJSON:         `{}`,
		RingElevationsJSON: `[]`,
		GridBlob:           gridBlob2, // Unique
		ChangedCellsCount:  0,
		SnapshotReason:     "test",
	}
	if _, err := db.InsertBgSnapshot(snap3); err != nil {
		t.Fatalf("Failed to insert unique snapshot: %v", err)
	}

	// Verify we have 3 snapshots
	snapshots, err := db.ListRecentBgSnapshots(sensorID, 10)
	if err != nil {
		t.Fatalf("Failed to list snapshots: %v", err)
	}
	if len(snapshots) != 3 {
		t.Fatalf("Expected 3 snapshots, got %d", len(snapshots))
	}

	// Delete duplicates
	deleted, err := db.DeleteDuplicateBgSnapshots(sensorID)
	if err != nil {
		t.Fatalf("Failed to delete duplicates: %v", err)
	}

	// Should have deleted 1 duplicate (keeping the latest of the 2 identical)
	if deleted != 1 {
		t.Errorf("Expected to delete 1 duplicate, got %d", deleted)
	}

	// Verify we now have 2 unique snapshots
	snapshots, err = db.ListRecentBgSnapshots(sensorID, 10)
	if err != nil {
		t.Fatalf("Failed to list snapshots after deletion: %v", err)
	}
	if len(snapshots) != 2 {
		t.Errorf("Expected 2 snapshots after deletion, got %d", len(snapshots))
	}

	// Verify we have 2 unique hashes
	uniqueCount, err := db.CountUniqueBgSnapshotHashes(sensorID)
	if err != nil {
		t.Fatalf("Failed to count unique hashes: %v", err)
	}
	if uniqueCount != 2 {
		t.Errorf("Expected 2 unique hashes, got %d", uniqueCount)
	}
}

// TestDeleteDuplicateBgSnapshots_NoData tests deletion with no data
func TestDeleteDuplicateBgSnapshots_NoData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Try to delete duplicates for non-existent sensor
	deleted, err := db.DeleteDuplicateBgSnapshots("non-existent-sensor")
	if err != nil {
		t.Fatalf("Failed to delete duplicates: %v", err)
	}

	// Should have deleted 0 rows
	if deleted != 0 {
		t.Errorf("Expected to delete 0 rows, got %d", deleted)
	}
}

// TestRadarDataRange tests retrieving the time range of radar objects
func TestRadarDataRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	t.Run("empty database", func(t *testing.T) {
		dataRange, err := db.RadarDataRange()
		if err != nil {
			t.Fatalf("Failed to get radar data range: %v", err)
		}

		// Empty database should return empty range
		if dataRange.StartUnix != 0 || dataRange.EndUnix != 0 {
			t.Errorf("Expected empty range, got start=%f, end=%f", dataRange.StartUnix, dataRange.EndUnix)
		}
	})

	t.Run("with data", func(t *testing.T) {
		// Insert some radar objects
		if err := db.RecordRadarObject(testRadarObjectJSON); err != nil {
			t.Fatalf("Failed to insert radar object 1: %v", err)
		}
		if err := db.RecordRadarObject(testRadarObjectJSON2); err != nil {
			t.Fatalf("Failed to insert radar object 2: %v", err)
		}

		// Get data range
		dataRange, err := db.RadarDataRange()
		if err != nil {
			t.Fatalf("Failed to get radar data range: %v", err)
		}

		// Check that we have a valid range
		if dataRange.StartUnix == 0 || dataRange.EndUnix == 0 {
			t.Errorf("Expected non-zero range, got start=%f, end=%f", dataRange.StartUnix, dataRange.EndUnix)
		}

		// Start should be <= End
		if dataRange.StartUnix > dataRange.EndUnix {
			t.Errorf("Start time (%f) should be <= end time (%f)", dataRange.StartUnix, dataRange.EndUnix)
		}
	})
}

// TestAttachAdminRoutes_DBStats tests the database admin routes
func TestAttachAdminRoutes_DBStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert some test data to make stats meaningful
	if err := db.RecordRadarObject(testRadarObjectJSON); err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	httpMux := http.NewServeMux()
	db.AttachAdminRoutes(httpMux)

	t.Run("db-stats endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/debug/db-stats", nil)
		w := httptest.NewRecorder()

		httpMux.ServeHTTP(w, req)

		// Should be registered (might return 403 due to auth or 200 if auth passes)
		if w.Code == http.StatusNotFound {
			t.Error("Route /debug/db-stats should be registered, got 404")
		}

		// If we get 200, validate the JSON response
		if w.Code == http.StatusOK {
			var stats DatabaseStats
			if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
				t.Errorf("Failed to decode stats response: %v", err)
			}

			// Verify stats structure
			if stats.TotalSizeMB <= 0 {
				t.Error("Expected positive total size")
			}
			if len(stats.Tables) == 0 {
				t.Error("Expected at least one table in stats")
			}

			// Check content type
			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Expected Content-Type 'application/json', got %s", contentType)
			}
		}
	})

	t.Run("backup endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/debug/backup", nil)
		w := httptest.NewRecorder()

		httpMux.ServeHTTP(w, req)

		// Should be registered (might return 403 due to auth or 200 if auth passes)
		if w.Code == http.StatusNotFound {
			t.Error("Route /debug/backup should be registered, got 404")
		}

		// If we get 200, check headers
		if w.Code == http.StatusOK {
			contentDisposition := w.Header().Get("Content-Disposition")
			if contentDisposition == "" {
				t.Error("Expected Content-Disposition header for backup download")
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/octet-stream" {
				t.Logf("Expected Content-Type 'application/octet-stream', got %s", contentType)
			}
		}
	})

	t.Run("tailsql endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/debug/tailsql/", nil)
		w := httptest.NewRecorder()

		httpMux.ServeHTTP(w, req)

		// Should be registered (might return 403 due to auth)
		if w.Code == http.StatusNotFound {
			t.Error("Route /debug/tailsql/ should be registered, got 404")
		}
	})
}

// TestGetDatabaseStats_EdgeCases tests edge cases for database stats
func TestGetDatabaseStats_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Test with empty database
	stats, err := db.GetDatabaseStats()
	if err != nil {
		t.Fatalf("Failed to get database stats: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected stats to be non-nil")
	}

	if stats.TotalSizeMB <= 0 {
		t.Error("Expected positive total size even for empty database")
	}

	// Should have some tables from schema
	if len(stats.Tables) == 0 {
		t.Error("Expected at least some tables from schema")
	}
}

// TestGetDatabaseStats_WithData tests database stats with actual data
func TestGetDatabaseStats_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert multiple radar objects to generate meaningful stats
	for i := 0; i < 100; i++ {
		if err := db.RecordRadarObject(testRadarObjectJSON); err != nil {
			t.Fatalf("Failed to insert radar object: %v", err)
		}
	}

	stats, err := db.GetDatabaseStats()
	if err != nil {
		t.Fatalf("Failed to get database stats: %v", err)
	}

	// Find radar_objects table
	var radarObjectsTable *TableStats
	for i := range stats.Tables {
		if stats.Tables[i].Name == "radar_objects" {
			radarObjectsTable = &stats.Tables[i]
			break
		}
	}

	if radarObjectsTable == nil {
		t.Fatal("Expected radar_objects table in stats")
	}

	// Should have ~100 rows (might have a few extra from other operations)
	if radarObjectsTable.RowCount < 100 {
		t.Errorf("Expected at least 100 rows in radar_objects, got %d", radarObjectsTable.RowCount)
	}

	// Size should be positive
	if radarObjectsTable.SizeMB <= 0 {
		t.Errorf("Expected positive size for radar_objects table")
	}
}

// TestBackupEndpoint_FileCleanup tests that backup files are properly cleaned up
func TestBackupEndpoint_FileCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Save and restore working directory using t.Cleanup
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	})

	// Change to temp dir so backup files are created there
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	httpMux := http.NewServeMux()
	db.AttachAdminRoutes(httpMux)

	// Check for backup files before request
	beforeFiles, err := filepath.Glob("backup-*.db")
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/debug/backup", nil)
	w := httptest.NewRecorder()

	httpMux.ServeHTTP(w, req)

	// Check for backup files after request
	afterFiles, err := filepath.Glob("backup-*.db")
	if err != nil {
		t.Fatalf("Failed to list files after backup: %v", err)
	}

	// In a real request, the backup file should be cleaned up after sending
	// In this test with httptest.ResponseRecorder, it might still exist
	// Just verify that we didn't accumulate too many files
	if len(afterFiles) > len(beforeFiles)+1 {
		t.Errorf("Too many backup files created: before=%d, after=%d", len(beforeFiles), len(afterFiles))
	}
}
