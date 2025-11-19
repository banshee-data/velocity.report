package db

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// MigrateUp runs all pending migrations up to the latest version.
// Returns nil if no migrations were needed (already at latest version).
func (db *DB) MigrateUp(migrationsFS fs.FS) error {
	m, err := db.newMigrate(migrationsFS)
	if err != nil {
		return err
	}
	// Note: We cannot call m.Close() when using WithInstance() because the sqlite driver's
	// Close() method closes the underlying sql.DB connection, which we manage separately.
	// The source driver (iofs) doesn't hold resources that need explicit cleanup.

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration up failed: %w", err)
	}

	return nil
}

// MigrateDown rolls back the most recent migration.
func (db *DB) MigrateDown(migrationsFS fs.FS) error {
	m, err := db.newMigrate(migrationsFS)
	if err != nil {
		return err
	}
	// Note: We cannot call m.Close() when using WithInstance() because the sqlite driver's
	// Close() method closes the underlying sql.DB connection, which we manage separately.

	if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration down failed: %w", err)
	}

	return nil
}

// MigrateVersion returns the current migration version and dirty state.
// Returns 0, false, nil if no migrations have been applied yet.
func (db *DB) MigrateVersion(migrationsFS fs.FS) (version uint, dirty bool, err error) {
	m, err := db.newMigrate(migrationsFS)
	if err != nil {
		return 0, false, err
	}
	// Note: We cannot call m.Close() when using WithInstance() because the sqlite driver's
	// Close() method closes the underlying sql.DB connection, which we manage separately.

	version, dirty, err = m.Version()
	if err != nil && errors.Is(err, migrate.ErrNilVersion) {
		// No migrations applied yet
		return 0, false, nil
	}

	return version, dirty, err
}

// MigrateForce forces the migration version to a specific value.
// This should only be used to recover from a dirty migration state.
func (db *DB) MigrateForce(migrationsFS fs.FS, version int) error {
	m, err := db.newMigrate(migrationsFS)
	if err != nil {
		return err
	}
	// Note: We cannot call m.Close() when using WithInstance() because the sqlite driver's
	// Close() method closes the underlying sql.DB connection, which we manage separately.

	if err := m.Force(version); err != nil {
		return fmt.Errorf("force migration to version %d failed: %w", version, err)
	}

	return nil
}

// MigrateTo migrates to a specific version.
// Use this to migrate up or down to a specific version.
func (db *DB) MigrateTo(migrationsFS fs.FS, version uint) error {
	m, err := db.newMigrate(migrationsFS)
	if err != nil {
		return err
	}
	// Note: We cannot call m.Close() when using WithInstance() because the sqlite driver's
	// Close() method closes the underlying sql.DB connection, which we manage separately.

	if err := m.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration to version %d failed: %w", version, err)
	}

	return nil
}

// newMigrate creates a new migrate instance configured for this database.
// Note: The returned migrate instance should NOT be closed when using WithInstance(),
// because the sqlite driver's Close() method closes the underlying sql.DB connection,
// which is managed separately by the DB struct. The migrate instance and its drivers
// will be garbage collected when no longer referenced.
func (db *DB) newMigrate(migrationsFS fs.FS) (*migrate.Migrate, error) {
	// Create iofs source driver from the provided filesystem
	sourceDriver, err := iofs.New(migrationsFS, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to create iofs source driver: %w", err)
	}

	// Create sqlite driver instance
	driver, err := sqlite.WithInstance(db.DB, &sqlite.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create sqlite driver: %w", err)
	}

	// Create migrate instance
	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", driver)
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
func (db *DB) GetMigrationStatus(migrationsFS fs.FS) (map[string]interface{}, error) {
	version, dirty, err := db.MigrateVersion(migrationsFS)
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

// GetLatestMigrationVersion returns the latest available migration version
// by scanning the migrations filesystem.
func GetLatestMigrationVersion(migrationsFS fs.FS) (uint, error) {
	// Read the migrations directory
	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return 0, fmt.Errorf("failed to read migrations filesystem: %w", err)
	}

	if len(entries) == 0 {
		return 0, fmt.Errorf("no migration files found")
	}

	// Parse version numbers from filenames
	var maxVersion uint
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		// Migration files follow format: 000001_name.up.sql
		if strings.HasSuffix(filename, ".up.sql") {
			var version uint
			if _, err := fmt.Sscanf(filename, "%d_", &version); err == nil {
				if version > maxVersion {
					maxVersion = version
				}
			}
		}
	}

	if maxVersion == 0 {
		return 0, fmt.Errorf("could not determine latest migration version")
	}

	return maxVersion, nil
}

// GetDatabaseSchema extracts the current schema from the database.
// Returns a map of object names to their SQL definitions (normalized).
func (db *DB) GetDatabaseSchema() (map[string]string, error) {
	schema := make(map[string]string)

	rows, err := db.Query(`
		SELECT name, sql
		FROM sqlite_master
		WHERE type IN ('table', 'index', 'trigger', 'view')
		  AND name NOT LIKE 'sqlite_%'
		  AND name != 'schema_migrations'
		  AND name != 'version_unique'
		  AND sql IS NOT NULL
		ORDER BY type, name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query schema: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, sqlStr string
		if err := rows.Scan(&name, &sqlStr); err != nil {
			return nil, fmt.Errorf("failed to scan schema row: %w", err)
		}
		// Normalize SQL for consistent comparison
		schema[name] = normalizeSQLForComparison(sqlStr)
	}

	return schema, nil
}

// normalizeSQLForComparison normalizes SQL statements for consistent comparison
func normalizeSQLForComparison(sql string) string {
	// Remove extra whitespace
	sql = strings.TrimSpace(sql)

	// Replace multiple spaces/newlines with single space
	fields := strings.Fields(sql)
	sql = strings.Join(fields, " ")

	// Remove trailing semicolon
	sql = strings.TrimSuffix(sql, ";")

	// Normalize comma spacing - remove spaces before commas
	sql = strings.ReplaceAll(sql, " ,", ",")

	return sql
}

// GetSchemaAtMigration returns the schema that would result from applying migrations up to a specific version.
// This is done by creating a temporary database and applying migrations.
func (db *DB) GetSchemaAtMigration(migrationsFS fs.FS, targetVersion uint) (map[string]string, error) {
	// Create a temporary database
	tmpDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp database: %w", err)
	}
	defer tmpDB.Close()

	tmpWrapper := &DB{tmpDB}

	// Apply migrations up to target version
	if err := tmpWrapper.MigrateTo(migrationsFS, targetVersion); err != nil {
		return nil, fmt.Errorf("failed to apply migrations to version %d: %w", targetVersion, err)
	}

	// Get the schema
	return tmpWrapper.GetDatabaseSchema()
}

// CompareSchemas compares two schemas and returns a similarity score (0-100)
// and a list of differences.
func CompareSchemas(schema1, schema2 map[string]string) (score int, differences []string) {
	allKeys := make(map[string]bool)
	for k := range schema1 {
		allKeys[k] = true
	}
	for k := range schema2 {
		allKeys[k] = true
	}

	totalObjects := len(allKeys)
	if totalObjects == 0 {
		return 100, nil
	}

	matchingObjects := 0
	for key := range allKeys {
		sql1, exists1 := schema1[key]
		sql2, exists2 := schema2[key]

		if !exists1 {
			differences = append(differences, fmt.Sprintf("- Missing in current: %s", key))
		} else if !exists2 {
			differences = append(differences, fmt.Sprintf("+ Extra in current: %s", key))
		} else if sql1 == sql2 {
			matchingObjects++
		} else {
			differences = append(differences, fmt.Sprintf("~ Modified: %s", key))
			differences = append(differences, fmt.Sprintf("  Expected: %s", sql2))
			differences = append(differences, fmt.Sprintf("  Current:  %s", sql1))
		}
	}

	score = (matchingObjects * 100) / totalObjects
	return score, differences
}

// DetectSchemaVersion attempts to detect which migration version a database is at
// by comparing its schema to known schemas at each migration point.
// Returns the detected version, match score, and any differences.
func (db *DB) DetectSchemaVersion(migrationsFS fs.FS) (detectedVersion uint, matchScore int, differences []string, err error) {
	currentSchema, err := db.GetDatabaseSchema()
	if err != nil {
		return 0, 0, nil, fmt.Errorf("failed to get current schema: %w", err)
	}

	latestVersion, err := GetLatestMigrationVersion(migrationsFS)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("failed to get latest version: %w", err)
	}

	bestVersion := uint(0)
	bestScore := 0
	var bestDifferences []string

	// Check each migration version from latest to first
	for version := latestVersion; version >= 1; version-- {
		schemaAtVersion, err := db.GetSchemaAtMigration(migrationsFS, version)
		if err != nil {
			log.Printf("Warning: could not get schema at version %d: %v", version, err)
			continue
		}

		score, diffs := CompareSchemas(currentSchema, schemaAtVersion)

		if score > bestScore {
			bestScore = score
			bestVersion = version
			bestDifferences = diffs
		}

		// If we found a perfect match, no need to continue
		if score == 100 {
			break
		}
	}

	return bestVersion, bestScore, bestDifferences, nil
}

// CheckAndPromptMigrations checks if the database version differs from the latest
// available migration version. If they differ, it prompts the user to apply migrations.
// Returns true if migrations were needed but not applied (should exit), false otherwise.
func (db *DB) CheckAndPromptMigrations(migrationsFS fs.FS) (bool, error) {
	// Get current database version
	currentVersion, dirty, err := db.MigrateVersion(migrationsFS)
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return false, fmt.Errorf("failed to get current migration version: %w", err)
	}

	// Get latest available migration version
	latestVersion, err := GetLatestMigrationVersion(migrationsFS)
	if err != nil {
		return false, fmt.Errorf("failed to get latest migration version: %w", err)
	}

	// If versions match, no action needed
	if currentVersion == latestVersion && !dirty {
		return false, nil
	}

	// If database is dirty, report error
	if dirty {
		return true, fmt.Errorf("database is in a dirty state (version %d). Run 'velocity-report migrate status' to diagnose", currentVersion)
	}

	// If current version is ahead, that's an error
	if currentVersion > latestVersion {
		return true, fmt.Errorf("database version (%d) is ahead of latest migration (%d). This should not happen", currentVersion, latestVersion)
	}

	// Migrations are available but not applied
	log.Printf("⚠️  Database schema version mismatch detected!")
	log.Printf("   Current database version: %d", currentVersion)
	log.Printf("   Latest available version: %d", latestVersion)
	log.Printf("   Outstanding migrations: %d", latestVersion-currentVersion)
	log.Printf("")
	log.Printf("This database appears to be from a prior installation.")
	log.Printf("To apply the outstanding migrations, run:")
	log.Printf("   velocity-report migrate up")
	log.Printf("")
	log.Printf("To see migration status, run:")
	log.Printf("   velocity-report migrate status")
	log.Printf("")

	return true, fmt.Errorf("database schema is out of date (version %d, need %d). Please run migrations", currentVersion, latestVersion)
}
