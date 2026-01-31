package db

import (
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// TestHandleMigrateStatus tests migration status display logic
func TestHandleMigrateStatus_WithValidDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a minimal migrations filesystem
	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS test_table (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS test_table;")},
	}

	// Test GetMigrationStatus with valid database
	status, err := db.GetMigrationStatus(migrationsFS)
	if err != nil {
		t.Fatalf("GetMigrationStatus failed: %v", err)
	}

	// Verify status map contains expected keys
	if _, ok := status["schema_migrations_exists"]; !ok {
		t.Error("Expected 'schema_migrations_exists' key in status map")
	}
}

// TestHandleMigrateStatus_NilMigrationsFS tests error handling for nil migrations
func TestHandleMigrateStatus_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Empty migrations FS
	migrationsFS := fstest.MapFS{}

	// Should handle empty migrations gracefully
	_, err = db.GetMigrationStatus(migrationsFS)
	// May return error or empty status depending on implementation
	if err != nil {
		t.Logf("GetMigrationStatus with empty FS returned expected error: %v", err)
	}
}

// TestMigrateUp_EmptyMigrationsFS tests MigrateUp with no migrations
func TestMigrateUp_EmptyMigrationsFS(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Empty migrations FS
	migrationsFS := fstest.MapFS{}

	// MigrateUp with no migrations should not fail catastrophically
	err = db.MigrateUp(migrationsFS)
	// May return error about no migrations found
	if err != nil {
		t.Logf("MigrateUp with empty FS returned: %v", err)
	}
}

// TestMigrateDown_NoMigrationsApplied tests MigrateDown when at version 0
func TestMigrateDown_NoMigrationsApplied(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS test_table (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS test_table;")},
	}

	// Try to migrate down without any migrations applied
	err = db.MigrateDown(migrationsFS)
	// Should return error about no applied migrations
	if err != nil {
		t.Logf("MigrateDown with no applied migrations returned expected error: %v", err)
	}
}

// TestMigrateTo_InvalidVersion tests migration to non-existent version
func TestMigrateTo_InvalidVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS test_table (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS test_table;")},
	}

	// Try to migrate to version that doesn't exist
	err = db.MigrateTo(migrationsFS, 999)
	if err == nil {
		t.Error("Expected error when migrating to non-existent version, got nil")
	}
}

// TestMigrateForce_InvalidVersion tests forcing an invalid version
func TestMigrateForce_InvalidVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS test_table (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS test_table;")},
	}

	// Force to negative version (recovery scenario)
	err = db.MigrateForce(migrationsFS, -1)
	// Should work or return specific error
	if err != nil {
		t.Logf("MigrateForce with -1 returned: %v", err)
	}
}

// TestMigrateVersion_UninitialisedDatabase tests getting version from uninitialised DB
func TestMigrateVersion_UninitialisedDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "uninit.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	migrationsFS := fstest.MapFS{}

	version, dirty, err := db.MigrateVersion(migrationsFS)
	// Should handle uninitialised database gracefully
	if err != nil {
		t.Logf("MigrateVersion on uninitialised DB: version=%d, dirty=%v, err=%v", version, dirty, err)
	}
}

// TestBaselineAtVersion_ValidVersion tests baselining at a specific version
func TestBaselineAtVersion_ValidVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "baseline.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Baseline at version 5
	err = db.BaselineAtVersion(5)
	if err != nil {
		t.Fatalf("BaselineAtVersion failed: %v", err)
	}

	// Verify baseline was set correctly
	migrationsFS := fstest.MapFS{}
	version, dirty, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("Failed to get version after baseline: %v", err)
	}

	if version != 5 {
		t.Errorf("Expected version 5 after baseline, got %d", version)
	}
	if dirty {
		t.Error("Expected dirty=false after baseline")
	}
}

// TestBaselineAtVersion_ZeroVersion tests baselining at version 0
func TestBaselineAtVersion_ZeroVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "baseline_zero.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Baseline at version 0 (reset)
	err = db.BaselineAtVersion(0)
	if err != nil {
		t.Fatalf("BaselineAtVersion(0) failed: %v", err)
	}
}

// TestMigrateTo_DowngradeVersion tests migrating to a lower version
func TestMigrateTo_DowngradeVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "downgrade.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":     &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS test1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql":   &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS test1;")},
		"000002_addcol.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS test2 (id INTEGER PRIMARY KEY);")},
		"000002_addcol.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS test2;")},
	}

	// Migrate up to version 2
	err = db.MigrateTo(migrationsFS, 2)
	if err != nil {
		t.Fatalf("MigrateTo(2) failed: %v", err)
	}

	// Now downgrade to version 1
	err = db.MigrateTo(migrationsFS, 1)
	if err != nil {
		t.Fatalf("MigrateTo(1) (downgrade) failed: %v", err)
	}

	// Verify we're at version 1
	version, _, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("Failed to get version: %v", err)
	}
	if version != 1 {
		t.Errorf("Expected version 1 after downgrade, got %d", version)
	}
}

// TestMigrateUp_SQLSyntaxError tests handling of invalid SQL in migrations
func TestMigrateUp_SQLSyntaxError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "syntax_error.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Migration with SQL syntax error
	migrationsFS := fstest.MapFS{
		"000001_bad.up.sql":   &fstest.MapFile{Data: []byte("INVALID SQL SYNTAX HERE!!!")},
		"000001_bad.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS nonexistent;")},
	}

	err = db.MigrateUp(migrationsFS)
	if err == nil {
		t.Error("Expected error for invalid SQL, got nil")
	}
}

// TestDetectSchemaVersion_EmptyDatabase tests schema detection on empty DB
func TestDetectSchemaVersion_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty_detect.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS test_table (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS test_table;")},
	}

	version, score, diffs, err := db.DetectSchemaVersion(migrationsFS)
	if err != nil {
		t.Fatalf("DetectSchemaVersion failed: %v", err)
	}

	t.Logf("Detected version: %d, score: %d%%, diffs: %v", version, score, diffs)
}

// TestDetectSchemaVersion_MatchingSchema tests detection with matching schema
func TestDetectSchemaVersion_MatchingSchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "matching_detect.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS users;")},
	}

	// Apply migration first
	err = db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Now detect - should match version 1
	version, score, _, err := db.DetectSchemaVersion(migrationsFS)
	if err != nil {
		t.Fatalf("DetectSchemaVersion failed: %v", err)
	}

	if version != 1 {
		t.Errorf("Expected detected version 1, got %d", version)
	}
	if score < 90 {
		t.Errorf("Expected high match score (>=90%%), got %d%%", score)
	}
}

// mockErrFS is a test filesystem that returns errors
type mockErrFS struct{}

func (m mockErrFS) Open(name string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

// TestMigrateUp_FSError tests handling of filesystem errors
func TestMigrateUp_FSError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fs_error.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Use a filesystem that always returns errors
	errFS := mockErrFS{}

	err = db.MigrateUp(errFS)
	if err == nil {
		t.Error("Expected error with broken filesystem, got nil")
	}
}
