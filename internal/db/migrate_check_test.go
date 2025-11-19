package db

import (
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
