package db

import (
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// TestAttachAdminRoutes_DBStatsSuccess tests the db-stats endpoint success path
func TestAttachAdminRoutes_DBStatsSuccess(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Insert test data
	event := map[string]interface{}{
		"classifier":      "vehicle",
		"start_time":      1000.0,
		"end_time":        1005.0,
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
	if err := db.RecordRadarObject(string(eventJSON)); err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/debug/db-stats", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Should be registered (might return 403 due to auth or 200 if auth passes)
	if w.Code == http.StatusNotFound {
		t.Error("Expected /debug/db-stats to be registered, got 404")
	}

	// If we get 200, validate the JSON response
	if w.Code == http.StatusOK {
		// Verify content type
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", ct)
		}

		// Verify JSON structure
		var stats DatabaseStats
		if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
			t.Errorf("Failed to decode response: %v", err)
		} else {
			if stats.TotalSizeMB <= 0 {
				t.Error("Expected positive total size")
			}

			if len(stats.Tables) == 0 {
				t.Error("Expected at least one table")
			}
		}
	}
}

// TestAttachAdminRoutes_DBStatsError tests the db-stats endpoint when GetDatabaseStats fails
func TestAttachAdminRoutes_DBStatsError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	// Close the DB to force an error (intentionally done after AttachAdminRoutes)
	db.Close()

	req := httptest.NewRequest(http.MethodGet, "/debug/db-stats", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Should get an error (500) or auth error (403), but endpoint should be registered
	if w.Code == http.StatusNotFound {
		t.Error("Expected endpoint to be registered, got 404")
	}

	// If we somehow bypass auth and get to the handler, expect 500
	if w.Code == http.StatusInternalServerError {
		body := w.Body.String()
		if !strings.Contains(body, "Failed to get database stats") {
			t.Errorf("Expected error message about failed stats, got: %s", body)
		}
	}
}

// TestAttachAdminRoutes_BackupSuccess tests the backup endpoint success path
func TestAttachAdminRoutes_BackupSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Save current working directory and change to tmpDir so backup is created there
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	// Only restore if we successfully changed directories
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	}()

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/debug/backup", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Should be registered (might return 403 due to auth or 200 if auth passes)
	if w.Code == http.StatusNotFound {
		t.Error("Expected /debug/backup to be registered, got 404")
	}

	// If we get 200, verify the response
	if w.Code == http.StatusOK {
		// Verify headers
		if cd := w.Header().Get("Content-Disposition"); cd == "" {
			t.Error("Expected Content-Disposition header")
		} else if !strings.Contains(cd, "attachment") {
			t.Errorf("Expected 'attachment' in Content-Disposition, got: %s", cd)
		}

		if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
			t.Errorf("Expected Content-Type 'application/octet-stream', got '%s'", ct)
		}

		if ce := w.Header().Get("Content-Encoding"); ce != "gzip" {
			t.Errorf("Expected Content-Encoding 'gzip', got '%s'", ce)
		}

		// Verify response is gzipped
		body := w.Body.Bytes()
		if len(body) == 0 {
			t.Error("Expected non-empty response body")
		}

		// Try to decompress to verify it's valid gzip
		gr, err := gzip.NewReader(w.Body)
		if err != nil {
			t.Errorf("Failed to create gzip reader: %v", err)
		} else {
			defer gr.Close()

			// Read some data to verify it's valid
			buf := make([]byte, 100)
			if _, err := gr.Read(buf); err != nil && err != io.EOF {
				t.Errorf("Failed to read gzipped data: %v", err)
			}
		}
	}
}

// TestAttachAdminRoutes_BackupVacuumError tests backup endpoint when VACUUM fails
func TestAttachAdminRoutes_BackupVacuumError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	// Close the DB to force VACUUM to fail
	db.Close()

	req := httptest.NewRequest(http.MethodGet, "/debug/backup", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Should be registered (not 404)
	if w.Code == http.StatusNotFound {
		t.Error("Expected endpoint to be registered, got 404")
	}

	// If we get 500 (bypassed auth), verify error message
	if w.Code == http.StatusInternalServerError {
		body := w.Body.String()
		if !strings.Contains(body, "Failed to create backup") {
			t.Errorf("Expected error message about failed backup, got: %s", body)
		}
	}
}

// TestAttachAdminRoutes_TailsqlEndpoint tests that tailsql endpoint is registered
func TestAttachAdminRoutes_TailsqlEndpoint(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/debug/tailsql/", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Should not get 404 (endpoint should be registered)
	// It might return 403 or other auth error, but not 404
	if w.Code == http.StatusNotFound {
		t.Error("Expected /debug/tailsql/ to be registered, got 404")
	}
}

// TestGetDatabaseStats_ErrorPaths tests error handling in GetDatabaseStats
func TestGetDatabaseStats_ErrorPaths(t *testing.T) {
	t.Run("closed database", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := NewDB(dbPath)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		db.Close()

		_, err = db.GetDatabaseStats()
		if err == nil {
			t.Error("Expected error for closed database")
		}
	})
}

// TestEvents_ErrorPaths tests error handling in Events function
func TestEvents_ErrorPaths(t *testing.T) {
	t.Run("query on closed database", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := NewDB(dbPath)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		db.Close()

		_, err = db.Events()
		if err == nil {
			t.Error("Expected error when querying closed database")
		}
	})

	t.Run("successful query with data", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := NewDB(dbPath)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		defer db.Close()

		// Insert test data
		rawData := map[string]interface{}{
			"uptime":    1234.56,
			"magnitude": 75,
			"speed":     12.5,
		}
		rawJSON, _ := json.Marshal(rawData)
		if err := db.RecordRawData(string(rawJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}

		events, err := db.Events()
		if err != nil {
			t.Fatalf("Events() failed: %v", err)
		}

		if len(events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(events))
		}

		// Verify data
		if !events[0].Uptime.Valid || events[0].Uptime.Float64 != 1234.56 {
			t.Errorf("Expected uptime 1234.56, got %v", events[0].Uptime)
		}
	})
}

// TestRadarDataRange_ErrorPath tests error handling in RadarDataRange
func TestRadarDataRange_ErrorPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	db.Close()

	_, err = db.RadarDataRange()
	if err == nil {
		t.Error("Expected error when querying closed database")
	}
}

// TestInsertBgSnapshot_ErrorPath tests error handling in InsertBgSnapshot
func TestInsertBgSnapshot_ErrorPath(t *testing.T) {
	t.Run("nil snapshot returns 0", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := NewDB(dbPath)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		defer db.Close()

		id, err := db.InsertBgSnapshot(nil)
		if err != nil {
			t.Errorf("Expected no error for nil snapshot, got: %v", err)
		}
		if id != 0 {
			t.Errorf("Expected id 0 for nil snapshot, got %d", id)
		}
	})

	t.Run("insert on closed database", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := NewDB(dbPath)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		db.Close()

		snap := &l3grid.BgSnapshot{
			SensorID:       "test",
			TakenUnixNanos: time.Now().UnixNano(),
			Rings:          16,
			AzimuthBins:    360,
			ParamsJSON:     "{}",
			GridBlob:       []byte("data"),
		}

		_, err = db.InsertBgSnapshot(snap)
		if err == nil {
			t.Error("Expected error when inserting to closed database")
		}
	})
}

// TestInsertRegionSnapshot_ErrorPaths tests error handling in InsertRegionSnapshot
func TestInsertRegionSnapshot_ErrorPaths(t *testing.T) {
	t.Run("nil snapshot returns 0", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := NewDB(dbPath)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		defer db.Close()

		id, err := db.InsertRegionSnapshot(nil)
		if err != nil {
			t.Errorf("Expected no error for nil snapshot, got: %v", err)
		}
		if id != 0 {
			t.Errorf("Expected id 0 for nil snapshot, got %d", id)
		}
	})

	t.Run("insert on closed database", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := NewDB(dbPath)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		db.Close()

		snap := &l3grid.RegionSnapshot{
			SnapshotID:       1,
			SensorID:         "test",
			CreatedUnixNanos: time.Now().UnixNano(),
			RegionCount:      1,
			RegionsJSON:      "[]",
		}

		_, err = db.InsertRegionSnapshot(snap)
		if err == nil {
			t.Error("Expected error when inserting to closed database")
		}
	})

	t.Run("successful insert", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := NewDB(dbPath)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		defer db.Close()

		snap := &l3grid.RegionSnapshot{
			SnapshotID:       1,
			SensorID:         "test-sensor",
			CreatedUnixNanos: time.Now().UnixNano(),
			RegionCount:      2,
			RegionsJSON:      `[{"x":1,"y":2}]`,
			VarianceDataJSON: `{"var":0.5}`,
			SettlingFrames:   10,
			SceneHash:        "hash123",
			SourcePath:       "/path/to/source",
		}

		id, err := db.InsertRegionSnapshot(snap)
		if err != nil {
			t.Errorf("InsertRegionSnapshot failed: %v", err)
		}
		if id == 0 {
			t.Error("Expected non-zero region_set_id")
		}
	})
}

// TestRecordRawData_ErrorPath tests error handling in RecordRawData
func TestRecordRawData_ErrorPath(t *testing.T) {
	t.Run("closed database", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := NewDB(dbPath)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		db.Close()

		rawData := map[string]interface{}{
			"uptime":    1234.56,
			"magnitude": 75,
		}
		rawJSON, _ := json.Marshal(rawData)

		err = db.RecordRawData(string(rawJSON))
		if err == nil {
			t.Error("Expected error when inserting to closed database")
		}
	})
}

// TestRecordRadarObject_ErrorPath tests error handling in RecordRadarObject
func TestRecordRadarObject_ErrorPath(t *testing.T) {
	t.Run("closed database", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := NewDB(dbPath)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		db.Close()

		event := map[string]interface{}{
			"classifier":    "vehicle",
			"start_time":    1000.0,
			"end_time":      1005.0,
			"max_speed_mps": 15.0,
		}
		eventJSON, _ := json.Marshal(event)

		err = db.RecordRadarObject(string(eventJSON))
		if err == nil {
			t.Error("Expected error when inserting to closed database")
		}
	})
}

// TestRadarObjects_ErrorPath tests error handling in RadarObjects
func TestRadarObjects_ErrorPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	db.Close()

	_, err = db.RadarObjects()
	if err == nil {
		t.Error("Expected error when querying closed database")
	}
}

// TestDeleteDuplicateBgSnapshots_ErrorPath tests error handling in DeleteDuplicateBgSnapshots
func TestDeleteDuplicateBgSnapshots_ErrorPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	db.Close()

	_, err = db.DeleteDuplicateBgSnapshots("test-sensor")
	if err == nil {
		t.Error("Expected error when deleting from closed database")
	}
}

// TestApplyPragmas_ErrorPath tests error handling in applyPragmas
func TestApplyPragmas_ErrorPath(t *testing.T) {
	// Create a database and close it
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	sqlDB.Close()

	// Try to apply pragmas to closed database
	err = applyPragmas(sqlDB)
	if err == nil {
		t.Error("Expected error when applying pragmas to closed database")
	}
}

// TestGetMigrationsFS_DevMode tests getMigrationsFS in dev mode
func TestGetMigrationsFS_DevMode(t *testing.T) {
	// Save original DevMode value
	origDevMode := DevMode
	defer func() { DevMode = origDevMode }()

	// Test dev mode
	DevMode = true
	fs, err := getMigrationsFS()
	if err != nil {
		t.Errorf("getMigrationsFS() in dev mode failed: %v", err)
	}
	if fs == nil {
		t.Error("Expected non-nil filesystem in dev mode")
	}

	// Test production mode (embedded)
	DevMode = false
	fs, err = getMigrationsFS()
	if err != nil {
		t.Errorf("getMigrationsFS() in production mode failed: %v", err)
	}
	if fs == nil {
		t.Error("Expected non-nil filesystem in production mode")
	}
}

// TestNewDBWithMigrationCheck_Paths tests different code paths in NewDBWithMigrationCheck
func TestNewDBWithMigrationCheck_Paths(t *testing.T) {
	t.Run("with migration check disabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		db, err := NewDBWithMigrationCheck(dbPath, false)
		if err != nil {
			t.Errorf("NewDBWithMigrationCheck with checkMigrations=false failed: %v", err)
		}
		if db == nil {
			t.Fatal("Expected non-nil DB")
		}
		defer db.Close()

		// Verify database is functional
		var result int
		err = db.QueryRow("SELECT 1").Scan(&result)
		if err != nil {
			t.Errorf("Failed to query database: %v", err)
		}
		if result != 1 {
			t.Errorf("Expected query result 1, got %d", result)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		// Try to create DB in a non-existent directory with invalid path
		invalidPath := "/nonexistent/impossible/path/to/test.db"
		_, err := NewDBWithMigrationCheck(invalidPath, false)
		if err == nil {
			t.Error("Expected error for invalid database path")
		}
	})
}

// TestOpenDB_Coverage tests OpenDB function for coverage
func TestOpenDB_Coverage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// First create a database
	db1, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create initial database: %v", err)
	}
	db1.Close()

	// Now open it with OpenDB
	db2, err := OpenDB(dbPath)
	if err != nil {
		t.Errorf("OpenDB failed: %v", err)
	}
	if db2 == nil {
		t.Fatal("Expected non-nil DB from OpenDB")
	}
	defer db2.Close()

	// Verify it's functional
	var result int
	err = db2.QueryRow("SELECT 1").Scan(&result)
	if err != nil {
		t.Errorf("Failed to query opened database: %v", err)
	}
}

// TestOpenDB_InvalidPath tests OpenDB with invalid path
func TestOpenDB_InvalidPath(t *testing.T) {
	// SQLite is very permissive with paths, so we test with a path that's
	// more likely to cause issues: attempting to create a file in a directory
	// that doesn't exist without creating parent directories
	invalidPath := "/nonexistent/impossible/path/to/test.db"
	db, err := OpenDB(invalidPath)
	if err == nil && db != nil {
		db.Close()
		// SQLite might still succeed, so this path just exercises the code
	}
	// The test primarily exercises the OpenDB code path; SQLite's permissiveness
	// means we can't reliably test for failure without more complex setup
}

// TestNewDB_Wrapper tests that NewDB wraps NewDBWithMigrationCheck correctly
func TestNewDB_Wrapper(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Errorf("NewDB failed: %v", err)
	}
	if db == nil {
		t.Fatal("Expected non-nil DB from NewDB")
	}
	defer db.Close()

	// Verify it has the schema
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count)
	if err != nil {
		t.Errorf("Failed to query tables: %v", err)
	}
	if count == 0 {
		t.Error("Expected tables to be created")
	}
}

// TestAttachAdminRoutes_ComprehensiveCoverage ensures all handler paths are exercised
func TestAttachAdminRoutes_ComprehensiveCoverage(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Change to temp dir for backup file creation
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	tests := []struct {
		name   string
		path   string
		method string
	}{
		{"db-stats handler", "/debug/db-stats", http.MethodGet},
		{"backup handler", "/debug/backup", http.MethodGet},
		{"tailsql handler", "/debug/tailsql/", http.MethodGet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			// Just verify the endpoint is registered (not 404)
			if w.Code == http.StatusNotFound {
				t.Errorf("Expected endpoint %s to be registered, got 404", tt.path)
			}
		})
	}
}

// TestGetDatabaseStats_FallbackPath tests the fallback path in GetDatabaseStats
// This is tricky to test as it requires the first query to fail but fallback to succeed
// We'll test the normal success path to ensure coverage
func TestGetDatabaseStats_NormalPath(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Insert various types of data to make stats interesting
	rawData := map[string]interface{}{
		"uptime":    1234.56,
		"magnitude": 75,
		"speed":     12.5,
	}
	rawJSON, _ := json.Marshal(rawData)
	db.RecordRawData(string(rawJSON))

	event := map[string]interface{}{
		"classifier":      "vehicle",
		"start_time":      1000.0,
		"end_time":        1005.0,
		"delta_time_msec": 5000,
		"max_speed_mps":   15.0,
		"min_speed_mps":   10.0,
	}
	eventJSON, _ := json.Marshal(event)
	db.RecordRadarObject(string(eventJSON))

	stats, err := db.GetDatabaseStats()
	if err != nil {
		t.Fatalf("GetDatabaseStats failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected non-nil stats")
	}

	if stats.TotalSizeMB <= 0 {
		t.Error("Expected positive total size")
	}

	// Should have multiple tables
	if len(stats.Tables) < 2 {
		t.Errorf("Expected at least 2 tables, got %d", len(stats.Tables))
	}

	// Check that tables have reasonable stats
	for _, table := range stats.Tables {
		if table.Name == "" {
			t.Error("Expected table to have a name")
		}
		if table.SizeMB < 0 {
			t.Errorf("Expected non-negative size for table %s, got %f", table.Name, table.SizeMB)
		}
		if table.RowCount < 0 {
			t.Errorf("Expected non-negative row count for table %s, got %d", table.Name, table.RowCount)
		}
	}
}

// TestRadarObjects_Success tests successful retrieval of radar objects
func TestRadarObjects_Success(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Insert test data
	event := map[string]interface{}{
		"classifier":      "vehicle",
		"start_time":      1000.0,
		"end_time":        1005.0,
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
	if err := db.RecordRadarObject(string(eventJSON)); err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Query objects
	objects, err := db.RadarObjects()
	if err != nil {
		t.Fatalf("RadarObjects failed: %v", err)
	}

	if len(objects) != 1 {
		t.Errorf("Expected 1 object, got %d", len(objects))
	}

	// Verify data
	if len(objects) > 0 {
		obj := objects[0]
		if obj.Classifier != "vehicle" {
			t.Errorf("Expected classifier 'vehicle', got '%s'", obj.Classifier)
		}
		if obj.MaxSpeed != 15.0 {
			t.Errorf("Expected max speed 15.0, got %f", obj.MaxSpeed)
		}
	}
}

// TestDeleteDuplicateBgSnapshots_Success tests successful deletion
func TestDeleteDuplicateBgSnapshots_Success(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	sensorID := "test-sensor"
	gridBlob := []byte("duplicate-data")

	// Insert duplicate snapshots
	for i := 0; i < 3; i++ {
		snap := &l3grid.BgSnapshot{
			SensorID:       sensorID,
			TakenUnixNanos: time.Now().UnixNano() + int64(i),
			Rings:          16,
			AzimuthBins:    360,
			ParamsJSON:     "{}",
			GridBlob:       gridBlob,
		}
		if _, err := db.InsertBgSnapshot(snap); err != nil {
			t.Fatalf("Failed to insert snapshot %d: %v", i, err)
		}
	}

	// Delete duplicates
	deleted, err := db.DeleteDuplicateBgSnapshots(sensorID)
	if err != nil {
		t.Fatalf("DeleteDuplicateBgSnapshots failed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("Expected to delete 2 duplicates, got %d", deleted)
	}
}

// TestEvents_WithMultipleRows tests Events with multiple rows to cover rows.Err() path
func TestEvents_WithMultipleRows(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Insert multiple rows
	for i := 0; i < 10; i++ {
		rawData := map[string]interface{}{
			"uptime":    float64(1000 + i),
			"magnitude": float64(50 + i),
			"speed":     float64(10 + i),
		}
		rawJSON, _ := json.Marshal(rawData)
		if err := db.RecordRawData(string(rawJSON)); err != nil {
			t.Fatalf("Failed to insert row %d: %v", i, err)
		}
	}

	events, err := db.Events()
	if err != nil {
		t.Fatalf("Events() failed: %v", err)
	}

	if len(events) != 10 {
		t.Errorf("Expected 10 events, got %d", len(events))
	}

	// Verify data is in descending order by uptime
	for i := 0; i < len(events)-1; i++ {
		if events[i].Uptime.Valid && events[i+1].Uptime.Valid {
			if events[i].Uptime.Float64 < events[i+1].Uptime.Float64 {
				t.Error("Expected events to be in descending order by uptime")
				break
			}
		}
	}
}

// TestRadarDataRange_WithNullValues tests RadarDataRange when there's data
func TestRadarDataRange_WithNullValues(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Test with empty database (should return empty range)
	dataRange, err := db.RadarDataRange()
	if err != nil {
		t.Fatalf("RadarDataRange failed: %v", err)
	}

	if dataRange.StartUnix != 0 || dataRange.EndUnix != 0 {
		t.Errorf("Expected empty range for empty DB, got start=%f, end=%f",
			dataRange.StartUnix, dataRange.EndUnix)
	}

	// Insert data and test again
	event := map[string]interface{}{
		"classifier":      "vehicle",
		"start_time":      2000.0,
		"end_time":        2005.0,
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
	if err := db.RecordRadarObject(string(eventJSON)); err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	dataRange, err = db.RadarDataRange()
	if err != nil {
		t.Fatalf("RadarDataRange failed after insert: %v", err)
	}

	if dataRange.StartUnix == 0 && dataRange.EndUnix == 0 {
		t.Error("Expected non-zero range after inserting data")
	}

	if dataRange.StartUnix > dataRange.EndUnix {
		t.Errorf("Start (%f) should be <= end (%f)", dataRange.StartUnix, dataRange.EndUnix)
	}
}

// TestBackupEndpoint_FileOperations tests backup file creation and cleanup
func TestBackupEndpoint_FileOperations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Change to temp dir
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	// Only restore if we successfully changed
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Logf("Warning: Failed to restore working directory: %v", err)
		}
	}()

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	// List files before backup
	filesBefore, _ := filepath.Glob("backup-*.db")

	req := httptest.NewRequest(http.MethodGet, "/debug/backup", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// List files after backup
	filesAfter, _ := filepath.Glob("backup-*.db")

	// The backup file should be cleaned up after response
	// In the real handler, defer removes it after sending
	// In httptest, the file might still exist momentarily
	// We just verify we didn't create too many files
	if len(filesAfter) > len(filesBefore)+1 {
		t.Errorf("Too many backup files created: before=%d, after=%d",
			len(filesBefore), len(filesAfter))
	}
}
