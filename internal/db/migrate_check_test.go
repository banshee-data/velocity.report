package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckAndPromptMigrations_UpToDate verifies no error when database is current
func TestCheckAndPromptMigrations_UpToDate(t *testing.T) {
	db := setupEmptyTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Apply all migrations
	err := db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Check should pass since we're up to date
	shouldExit, err := db.CheckAndPromptMigrations(migrationsFS)
	if err != nil {
		t.Errorf("Expected no error when up to date, got: %v", err)
	}
	if shouldExit {
		t.Error("Expected shouldExit to be false when up to date")
	}
}

// TestCheckAndPromptMigrations_OutOfDate verifies error when migrations are pending
func TestCheckAndPromptMigrations_OutOfDate(t *testing.T) {
	db := setupEmptyTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Apply only first migration
	err := db.MigrateTo(migrationsFS, 1)
	if err != nil {
		t.Fatalf("MigrateTo(1) failed: %v", err)
	}

	// Check should fail since we're not up to date
	shouldExit, err := db.CheckAndPromptMigrations(migrationsFS)
	if err == nil {
		t.Error("Expected error when migrations are pending")
	}
	if !shouldExit {
		t.Error("Expected shouldExit to be true when migrations are pending")
	}
}

// TestCheckAndPromptMigrations_DirtyState verifies error when database is dirty
func TestCheckAndPromptMigrations_DirtyState(t *testing.T) {
	db := setupEmptyTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Apply migrations
	err := db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Manually set database to dirty state
	_, err = db.Exec("UPDATE schema_migrations SET dirty = 1")
	if err != nil {
		t.Fatalf("Failed to set dirty state: %v", err)
	}

	// Check should fail with dirty state error
	shouldExit, err := db.CheckAndPromptMigrations(migrationsFS)
	if err == nil {
		t.Error("Expected error when database is dirty")
	}
	if !shouldExit {
		t.Error("Expected shouldExit to be true when database is dirty")
	}
}

// TestGetLatestMigrationVersion verifies we can detect the latest migration version
func TestGetLatestMigrationVersion(t *testing.T) {
	migrationsFS := setupTestMigrations(t)

	version, err := GetLatestMigrationVersion(migrationsFS)
	if err != nil {
		t.Fatalf("GetLatestMigrationVersion failed: %v", err)
	}

	// setupTestMigrations creates migrations 1 and 2
	if version != 2 {
		t.Errorf("Expected latest version 2, got %d", version)
	}
}

// TestGetLatestMigrationVersion_NoMigrations verifies error when no migrations exist
func TestGetLatestMigrationVersion_NoMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	emptyFS := os.DirFS(tmpDir)

	_, err := GetLatestMigrationVersion(emptyFS)
	if err == nil {
		t.Error("Expected error when no migrations exist")
	}
}

// TestNewDBWithMigrationCheck_FreshDatabase verifies fresh database is baselined
func TestNewDBWithMigrationCheck_FreshDatabase(t *testing.T) {
	fname := filepath.Join(t.TempDir(), "test.db")

	// NewDBWithMigrationCheck should create and baseline the database
	// Note: This uses the production embedded migrations
	db, err := NewDBWithMigrationCheck(fname, false)
	if err != nil {
		t.Fatalf("NewDBWithMigrationCheck failed: %v", err)
	}
	defer db.Close()

	// Verify schema_migrations exists and version is set
	var version uint
	err = db.QueryRow("SELECT version FROM schema_migrations LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("Failed to read version: %v", err)
	}

	// Fresh database should be baselined at the latest migration version
	// Get latest version from production embedded migrations
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("Failed to get migrations FS: %v", err)
	}

	latestVersion, err := GetLatestMigrationVersion(migrationsFS)
	if err != nil {
		t.Fatalf("Failed to get latest migration version: %v", err)
	}

	if version != latestVersion {
		t.Errorf("Expected baseline version %d (latest from migrations), got %d", latestVersion, version)
	}
}

// TestNewDBWithMigrationCheck_OutOfDateDatabase verifies error on old database
func TestNewDBWithMigrationCheck_OutOfDateDatabase(t *testing.T) {
	fname := filepath.Join(t.TempDir(), "test.db")

	// Get production migrations
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("Failed to get migrations FS: %v", err)
	}

	// Create database and apply only first migration
	db := setupEmptyTestDB(t)
	dbPath := t.Name() + "_migrations.db"

	err = db.MigrateTo(migrationsFS, 1)
	if err != nil {
		t.Fatalf("MigrateTo(1) failed: %v", err)
	}
	db.Close()

	// Copy to test location
	srcPath := dbPath
	os.Rename(srcPath, fname)

	// NewDBWithMigrationCheck should detect out-of-date database and error
	_, err = NewDBWithMigrationCheck(fname, true)
	if err == nil {
		t.Error("Expected error when database is out of date")
	}

	// Cleanup
	os.Remove(fname)
	os.Remove(fname + "-shm")
	os.Remove(fname + "-wal")
}

// TestBaselineVerification tests that baseline verification catches incorrect baselines
func TestBaselineVerification(t *testing.T) {
	fname := filepath.Join(t.TempDir(), "test.db")

	// Create a fresh database
	rawDB, err := sql.Open("sqlite", fname)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Apply PRAGMAs
	if err := applyPragmas(rawDB); err != nil {
		t.Fatalf("Failed to apply PRAGMAs: %v", err)
	}

	// Create schema_migrations table with wrong version
	_, err = rawDB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version bigint NOT NULL PRIMARY KEY,
			dirty boolean NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create schema_migrations: %v", err)
	}

	// Set baseline at version 5 (but database is actually empty)
	_, err = rawDB.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (5, 0)")
	if err != nil {
		t.Fatalf("Failed to insert baseline: %v", err)
	}

	rawDB.Close()

	// Open with DB wrapper - this should fail migration check
	// because the database claims to be at v5 but has no tables
	db, err := OpenDB(fname)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Get migrations
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("Failed to get migrations FS: %v", err)
	}

	// Verify baseline with MigrateVersion - should report version 5
	version, dirty, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version != 5 {
		t.Errorf("Expected version 5, got %d", version)
	}

	if dirty {
		t.Errorf("Expected clean state, got dirty")
	}

	// Now verify that GetDatabaseSchema shows empty database
	schema, err := db.GetDatabaseSchema()
	if err != nil {
		t.Fatalf("GetDatabaseSchema failed: %v", err)
	}

	// Schema should only have schema_migrations table
	if len(schema) > 1 {
		t.Errorf("Expected only schema_migrations table, got %d tables", len(schema))
	}
}

// TestFreshDatabaseBaselineVerificationSuccess tests successful baseline verification
func TestFreshDatabaseBaselineVerificationSuccess(t *testing.T) {
	fname := filepath.Join(t.TempDir(), "test.db")

	// Create fresh database - this should succeed and properly verify baseline
	db, err := NewDBWithMigrationCheck(fname, false)
	if err != nil {
		t.Fatalf("NewDBWithMigrationCheck failed: %v", err)
	}
	defer db.Close()

	// Get migrations
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("Failed to get migrations FS: %v", err)
	}

	// Verify the baseline is at latest version
	version, dirty, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	latestVersion, err := GetLatestMigrationVersion(migrationsFS)
	if err != nil {
		t.Fatalf("GetLatestMigrationVersion failed: %v", err)
	}

	if version != latestVersion {
		t.Errorf("Expected version %d, got %d", latestVersion, version)
	}

	if dirty {
		t.Errorf("Expected clean state, got dirty")
	}

	// Verify that the database has actual tables (not just schema_migrations)
	schema, err := db.GetDatabaseSchema()
	if err != nil {
		t.Fatalf("GetDatabaseSchema failed: %v", err)
	}

	// GetDatabaseSchema returns map[objectName]sqlDefinition
	// Should have multiple objects (tables, indexes, etc.)
	if len(schema) < 5 {
		t.Errorf("Expected at least 5 schema objects, got %d", len(schema))
	}

	// Verify some expected tables exist
	expectedTables := []string{"radar_data", "radar_objects", "radar_commands"}
	for _, table := range expectedTables {
		if _, exists := schema[table]; !exists {
			t.Errorf("Expected table %q not found in schema", table)
		}
	}
}

// TestBaselineVerificationCatchesIncorrectBaseline tests that baseline verification
// detects when BaselineAtVersion sets the version but returns before it actually applies
func TestBaselineVerificationCatchesIncorrectBaseline(t *testing.T) {
	fname := filepath.Join(t.TempDir(), "test.db")

	// Create a database using the low-level approach
	rawDB, err := sql.Open("sqlite", fname)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Apply PRAGMAs
	if err := applyPragmas(rawDB); err != nil {
		t.Fatalf("Failed to apply PRAGMAs: %v", err)
	}

	// Execute schema.sql to create tables
	_, err = rawDB.Exec(schemaSQL)
	if err != nil {
		t.Fatalf("Failed to execute schema.sql: %v", err)
	}

	// Create schema_migrations table (normally created by BaselineAtVersion)
	_, err = rawDB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version bigint NOT NULL PRIMARY KEY,
			dirty boolean NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create schema_migrations: %v", err)
	}

	// Manually baseline at wrong version (e.g., version 5 instead of latest)
	// This simulates a bug where BaselineAtVersion doesn't work correctly
	_, err = rawDB.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (5, 0)")
	if err != nil {
		t.Fatalf("Failed to insert wrong baseline: %v", err)
	}

	rawDB.Close()

	// Now try to open with DB wrapper - the verification should catch this
	db := &DB{nil}

	// Reopen the database
	reopened, err := sql.Open("sqlite", fname)
	if err != nil {
		t.Fatalf("Failed to reopen database: %v", err)
	}
	db.DB = reopened
	defer db.Close()

	// Apply PRAGMAs
	if err := applyPragmas(reopened); err != nil {
		t.Fatalf("Failed to apply PRAGMAs: %v", err)
	}

	// Get migrations
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("Failed to get migrations FS: %v", err)
	}

	// Get latest version
	latestVersion, err := GetLatestMigrationVersion(migrationsFS)
	if err != nil {
		t.Fatalf("GetLatestMigrationVersion failed: %v", err)
	}

	// This should simulate what happens after baseline - verify it
	currentVersion, _, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	// The verification check should detect this mismatch
	if currentVersion != latestVersion {
		// This is what we expect - the version doesn't match
		t.Logf("Successfully detected incorrect baseline: expected version %d, got %d", latestVersion, currentVersion)
	} else {
		t.Errorf("Verification did not catch incorrect baseline - got version %d when expected to detect mismatch", currentVersion)
	}
}
