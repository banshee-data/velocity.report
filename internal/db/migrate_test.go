package db

import (
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// setupMigrationTestDB creates a test database without running migrations
func setupMigrationTestDB(t *testing.T) *DB {
	t.Helper()
	fname := t.Name() + ".db"
	_ = os.Remove(fname)

	// Open database directly without running schema.sql
	sqlDB, err := sql.Open("sqlite", fname)
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}

	return &DB{sqlDB}
}

// setupTestMigrations creates a temporary directory with test migration files
// and returns it as an fs.FS
func setupTestMigrations(t *testing.T) fs.FS {
	t.Helper()
	tmpDir := filepath.Join(t.TempDir(), "migrations")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("failed to create temp migrations dir: %v", err)
	}

	// Create test migration files
	migrations := map[string]string{
		"000001_create_test_table.up.sql": `
			CREATE TABLE IF NOT EXISTS test_table (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL
			);
		`,
		"000001_create_test_table.down.sql": `
			DROP TABLE IF EXISTS test_table;
		`,
		"000002_add_test_column.up.sql": `
			ALTER TABLE test_table ADD COLUMN description TEXT;
		`,
		"000002_add_test_column.down.sql": `
			-- SQLite doesn't support DROP COLUMN directly, so we need to recreate the table
			CREATE TABLE test_table_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL
			);
			INSERT INTO test_table_new (id, name) SELECT id, name FROM test_table;
			DROP TABLE test_table;
			ALTER TABLE test_table_new RENAME TO test_table;
		`,
	}

	for filename, content := range migrations {
		path := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write migration file %s: %v", filename, err)
		}
	}

	return os.DirFS(tmpDir)
}

func TestMigrateUp(t *testing.T) {
	db := setupMigrationTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Run migrations up
	err := db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Verify migration version
	version, dirty, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version != 2 {
		t.Errorf("expected version 2, got %d", version)
	}

	if dirty {
		t.Error("database should not be dirty after successful migration")
	}

	// Verify test_table exists and has correct schema
	var tableExists bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM sqlite_master
		WHERE type='table' AND name='test_table'
	`).Scan(&tableExists)
	if err != nil {
		t.Fatalf("failed to check test_table: %v", err)
	}

	if !tableExists {
		t.Error("test_table should exist after migration")
	}

	// Verify description column exists (from second migration)
	var hasDescription bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM pragma_table_info('test_table')
		WHERE name='description'
	`).Scan(&hasDescription)
	if err != nil {
		t.Fatalf("failed to check description column: %v", err)
	}

	if !hasDescription {
		t.Error("description column should exist after second migration")
	}
}

func TestMigrateUp_Idempotency(t *testing.T) {
	db := setupMigrationTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Run migrations up twice
	err := db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("first MigrateUp failed: %v", err)
	}

	err = db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("second MigrateUp failed: %v", err)
	}

	// Verify version is still correct
	version, _, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version != 2 {
		t.Errorf("expected version 2 after idempotent up, got %d", version)
	}
}

func TestMigrateDown(t *testing.T) {
	db := setupMigrationTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Run migrations up first
	err := db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Run one migration down
	err = db.MigrateDown(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateDown failed: %v", err)
	}

	// Verify version is now 1
	version, dirty, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version != 1 {
		t.Errorf("expected version 1 after down migration, got %d", version)
	}

	if dirty {
		t.Error("database should not be dirty after successful down migration")
	}

	// Verify description column no longer exists
	var hasDescription bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM pragma_table_info('test_table')
		WHERE name='description'
	`).Scan(&hasDescription)
	if err != nil {
		t.Fatalf("failed to check description column: %v", err)
	}

	if hasDescription {
		t.Error("description column should not exist after rolling back second migration")
	}

	// Verify test_table still exists
	var tableExists bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM sqlite_master
		WHERE type='table' AND name='test_table'
	`).Scan(&tableExists)
	if err != nil {
		t.Fatalf("failed to check test_table: %v", err)
	}

	if !tableExists {
		t.Error("test_table should still exist after rolling back only second migration")
	}
}

func TestMigrateVersion_NoMigrations(t *testing.T) {
	db := setupMigrationTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Check version before any migrations
	version, dirty, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version != 0 {
		t.Errorf("expected version 0 before migrations, got %d", version)
	}

	if dirty {
		t.Error("database should not be dirty before any migrations")
	}
}

func TestMigrateForce(t *testing.T) {
	db := setupMigrationTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Run migrations up
	err := db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Force version to 1
	err = db.MigrateForce(migrationsFS, 1)
	if err != nil {
		t.Fatalf("MigrateForce failed: %v", err)
	}

	// Verify version is now 1
	version, _, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version != 1 {
		t.Errorf("expected version 1 after force, got %d", version)
	}
}

func TestMigrateTo(t *testing.T) {
	db := setupMigrationTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Migrate to version 1 only
	err := db.MigrateTo(migrationsFS, 1)
	if err != nil {
		t.Fatalf("MigrateTo(1) failed: %v", err)
	}

	// Verify version is 1
	version, _, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}

	// Verify description column does not exist yet
	var hasDescription bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM pragma_table_info('test_table')
		WHERE name='description'
	`).Scan(&hasDescription)
	if err != nil {
		t.Fatalf("failed to check description column: %v", err)
	}

	if hasDescription {
		t.Error("description column should not exist at version 1")
	}

	// Now migrate to version 2
	err = db.MigrateTo(migrationsFS, 2)
	if err != nil {
		t.Fatalf("MigrateTo(2) failed: %v", err)
	}

	// Verify version is 2
	version, _, err = db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version != 2 {
		t.Errorf("expected version 2, got %d", version)
	}

	// Verify description column now exists
	err = db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM pragma_table_info('test_table')
		WHERE name='description'
	`).Scan(&hasDescription)
	if err != nil {
		t.Fatalf("failed to check description column: %v", err)
	}

	if !hasDescription {
		t.Error("description column should exist at version 2")
	}
}

func TestBaselineAtVersion(t *testing.T) {
	db := setupMigrationTestDB(t)
	defer cleanupTestDB(t, db)

	// Baseline at version 2
	err := db.BaselineAtVersion(2)
	if err != nil {
		t.Fatalf("BaselineAtVersion failed: %v", err)
	}

	// Verify schema_migrations table exists
	var tableExists bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM sqlite_master
		WHERE type='table' AND name='schema_migrations'
	`).Scan(&tableExists)
	if err != nil {
		t.Fatalf("failed to check schema_migrations table: %v", err)
	}

	if !tableExists {
		t.Error("schema_migrations table should exist after baseline")
	}

	// Verify version is set to 2
	var version int
	err = db.QueryRow("SELECT version FROM schema_migrations LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("failed to read version: %v", err)
	}

	if version != 2 {
		t.Errorf("expected baseline version 2, got %d", version)
	}

	// Try to baseline again (should fail)
	err = db.BaselineAtVersion(3)
	if err == nil {
		t.Error("expected error when baselining already-migrated database")
	}
}

func TestGetMigrationStatus(t *testing.T) {
	db := setupMigrationTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Get status before any migrations
	status, err := db.GetMigrationStatus(migrationsFS)
	if err != nil {
		t.Fatalf("GetMigrationStatus failed: %v", err)
	}

	if status["current_version"] != uint(0) {
		t.Errorf("expected version 0, got %v", status["current_version"])
	}

	if status["dirty"] != false {
		t.Error("expected dirty=false before migrations")
	}

	// Run migrations
	err = db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Get status after migrations
	status, err = db.GetMigrationStatus(migrationsFS)
	if err != nil {
		t.Fatalf("GetMigrationStatus failed: %v", err)
	}

	if status["current_version"] != uint(2) {
		t.Errorf("expected version 2, got %v", status["current_version"])
	}

	if status["schema_migrations_exists"] != true {
		t.Error("expected schema_migrations_exists=true after migrations")
	}
}

func TestMigrateUpDown_FullCycle(t *testing.T) {
	db := setupMigrationTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Full cycle: up -> down -> up
	err := db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("first MigrateUp failed: %v", err)
	}

	version, _, _ := db.MigrateVersion(migrationsFS)
	if version != 2 {
		t.Errorf("expected version 2 after up, got %d", version)
	}

	// Roll back both migrations
	err = db.MigrateDown(migrationsFS)
	if err != nil {
		t.Fatalf("first MigrateDown failed: %v", err)
	}

	err = db.MigrateDown(migrationsFS)
	if err != nil {
		t.Fatalf("second MigrateDown failed: %v", err)
	}

	version, _, _ = db.MigrateVersion(migrationsFS)
	if version != 0 {
		t.Errorf("expected version 0 after rolling back all, got %d", version)
	}

	// Verify test_table is gone
	var tableExists bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM sqlite_master
		WHERE type='table' AND name='test_table'
	`).Scan(&tableExists)
	if err != nil {
		t.Fatalf("failed to check test_table: %v", err)
	}

	if tableExists {
		t.Error("test_table should not exist after rolling back all migrations")
	}

	// Re-apply migrations
	err = db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("second MigrateUp failed: %v", err)
	}

	version, _, _ = db.MigrateVersion(migrationsFS)
	if version != 2 {
		t.Errorf("expected version 2 after re-applying, got %d", version)
	}

	// Verify test_table exists again
	err = db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM sqlite_master
		WHERE type='table' AND name='test_table'
	`).Scan(&tableExists)
	if err != nil {
		t.Fatalf("failed to check test_table: %v", err)
	}

	if !tableExists {
		t.Error("test_table should exist after re-applying migrations")
	}
}

func TestMigrate_NoChangeError(t *testing.T) {
	db := setupMigrationTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsFS := setupTestMigrations(t)

	// Apply all migrations
	err := db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Try to apply up again (should not error, handled gracefully)
	err = db.MigrateUp(migrationsFS)
	if err != nil {
		t.Errorf("second MigrateUp should not error: %v", err)
	}

	// Roll back all migrations
	err = db.MigrateDown(migrationsFS)
	if err != nil {
		t.Fatalf("first MigrateDown failed: %v", err)
	}

	err = db.MigrateDown(migrationsFS)
	if err != nil {
		t.Fatalf("second MigrateDown failed: %v", err)
	}

	// Try to roll back when at version 0 (should error - no migration to roll back)
	err = db.MigrateDown(migrationsFS)
	if err == nil {
		t.Error("MigrateDown at version 0 should error (no migration to roll back)")
	}
}
