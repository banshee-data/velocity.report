package db

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// MigrateUp runs all pending migrations up to the latest version.
// Returns nil if no migrations were needed (already at latest version).
func (db *DB) MigrateUp(migrationsDir string) error {
	m, err := db.newMigrate(migrationsDir)
	if err != nil {
		return err
	}
	// Note: We don't close m here because it would close the underlying DB connection.
	// The migrate instance will be garbage collected when no longer needed.

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration up failed: %w", err)
	}

	return nil
}

// MigrateDown rolls back the most recent migration.
func (db *DB) MigrateDown(migrationsDir string) error {
	m, err := db.newMigrate(migrationsDir)
	if err != nil {
		return err
	}
	// Note: We don't close m here because it would close the underlying DB connection.

	if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration down failed: %w", err)
	}

	return nil
}

// MigrateVersion returns the current migration version and dirty state.
// Returns 0, false, nil if no migrations have been applied yet.
func (db *DB) MigrateVersion(migrationsDir string) (version uint, dirty bool, err error) {
	m, err := db.newMigrate(migrationsDir)
	if err != nil {
		return 0, false, err
	}
	// Note: We don't close m here because it would close the underlying DB connection.

	version, dirty, err = m.Version()
	if err != nil && errors.Is(err, migrate.ErrNilVersion) {
		// No migrations applied yet
		return 0, false, nil
	}

	return version, dirty, err
}

// MigrateForce forces the migration version to a specific value.
// This should only be used to recover from a dirty migration state.
func (db *DB) MigrateForce(migrationsDir string, version int) error {
	m, err := db.newMigrate(migrationsDir)
	if err != nil {
		return err
	}
	// Note: We don't close m here because it would close the underlying DB connection.

	if err := m.Force(version); err != nil {
		return fmt.Errorf("force migration to version %d failed: %w", version, err)
	}

	return nil
}

// MigrateTo migrates to a specific version.
// Use this to migrate up or down to a specific version.
func (db *DB) MigrateTo(migrationsDir string, version uint) error {
	m, err := db.newMigrate(migrationsDir)
	if err != nil {
		return err
	}
	// Note: We don't close m here because it would close the underlying DB connection.

	if err := m.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration to version %d failed: %w", version, err)
	}

	return nil
}

// newMigrate creates a new migrate instance configured for this database.
// The caller is responsible for closing the returned migrate instance.
func (db *DB) newMigrate(migrationsDir string) (*migrate.Migrate, error) {
	// Get absolute path to migrations directory
	absPath, err := filepath.Abs(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for migrations: %w", err)
	}

	// Create sqlite driver instance
	driver, err := sqlite.WithInstance(db.DB, &sqlite.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create sqlite driver: %w", err)
	}

	// Create migrate instance
	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", absPath),
		"sqlite",
		driver,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	// Set up logger
	m.Log = &migrateLogger{}

	return m, nil
}

// migrateLogger implements migrate.Logger interface
type migrateLogger struct{}

func (l *migrateLogger) Printf(format string, v ...interface{}) {
	log.Printf("[migrate] "+format, v...)
}

func (l *migrateLogger) Verbose() bool {
	return false
}

// ensureSchemaMigrationsTable ensures the schema_migrations table exists.
// This is called automatically by golang-migrate but can be used for baselining.
func (db *DB) ensureSchemaMigrationsTable() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER NOT NULL,
			dirty INTEGER NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS version_unique ON schema_migrations (version);
	`)
	return err
}

// BaselineAtVersion creates a schema_migrations entry at the specified version
// without running any migrations. This is useful for existing databases that
// already have the schema from that version applied.
func (db *DB) BaselineAtVersion(version uint) error {
	if err := db.ensureSchemaMigrationsTable(); err != nil {
		return fmt.Errorf("failed to ensure schema_migrations table: %w", err)
	}

	// Check if any version already exists
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check existing migrations: %w", err)
	}

	if count > 0 {
		return fmt.Errorf("database already has migrations applied, cannot baseline")
	}

	// Insert the baseline version
	_, err = db.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (?, 0)", version)
	if err != nil {
		return fmt.Errorf("failed to insert baseline version: %w", err)
	}

	log.Printf("Database baselined at version %d", version)
	return nil
}

// GetMigrationStatus returns a summary of the migration status including
// current version, dirty state, and whether migrations are needed.
func (db *DB) GetMigrationStatus(migrationsDir string) (map[string]interface{}, error) {
	version, dirty, err := db.MigrateVersion(migrationsDir)
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return nil, fmt.Errorf("failed to get migration version: %w", err)
	}

	status := map[string]interface{}{
		"current_version": version,
		"dirty":           dirty,
	}

	// Check if migrations table exists
	var tableExists bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0 
		FROM sqlite_master 
		WHERE type='table' AND name='schema_migrations'
	`).Scan(&tableExists)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to check schema_migrations table: %w", err)
	}

	status["schema_migrations_exists"] = tableExists

	return status, nil
}
