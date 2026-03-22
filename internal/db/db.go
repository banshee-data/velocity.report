package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// DB is the canonical radar/core SQLite wrapper for the shared schema.
//
// The package-level SQL boundary in this repo is intentionally split across
// `internal/db` and `internal/lidar/storage/sqlite`. The only LiDAR crossover
// that remains here is the small background/region snapshot surface needed to
// satisfy l3grid.BgStore. Keeping that interface on DB avoids introducing a
// third SQL package just for two persistence methods.
//
// compile-time assertion: ensure DB implements l3grid.BgStore (InsertBgSnapshot)
var _ l3grid.BgStore = (*DB)(nil)

const (
	// nearPerpendicularThreshold is the minimum absolute cosine value for a valid sensor angle.
	// Angles near 90° (perpendicular to traffic) produce cosines near 0, leading to extremely large
	// corrected speeds. To keep corrections within a physically meaningful range and to reflect
	// practical mounting constraints, we require |cos(angle)| >= 0.1, which corresponds to angles
	// no closer than ~84.3° to perpendicular (i.e. we avoid speed corrections larger than ~10x).
	nearPerpendicularThreshold = 0.1

	// radiansPerDegree converts degrees to radians
	radiansPerDegree = math.Pi / 180.0
)

type DB struct {
	*sql.DB
}

//go:embed schema.sql
var schemaSQL string

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DevMode controls whether to use filesystem or embedded migrations.
// Set to true in development for hot-reloading, false in production.
var DevMode = false

var (
	// Cache schema.sql consistency checks so repeated NewDB() calls (common in tests)
	// don't replay all migrations for every fresh database.
	prodSchemaConsistencyOnce sync.Once
	prodSchemaConsistencyErr  error
	prodLatestVersion         uint
)

// getMigrationsFS returns the appropriate filesystem for migrations.
// In dev mode, uses the local filesystem for hot-reloading.
// In production, uses the embedded filesystem.
func getMigrationsFS() (fs.FS, error) {
	if DevMode {
		// Development: use local filesystem
		return os.DirFS("internal/db/migrations"), nil
	}
	// Production: use embedded filesystem
	// The embed directive includes "migrations/*.sql", so we need to extract just the migrations subdir
	subFS, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to create sub-filesystem for embedded migrations directory %q: %w", "migrations", err)
	}
	return subFS, nil
}

func getSchemaConsistencyResult(migrationsFS fs.FS) (uint, error) {
	if DevMode {
		// Dev mode uses filesystem-backed migrations for hot-reload behavior.
		// Re-check on each call so new/edited migration files are picked up
		// without restarting the process.
		return validateSchemaSQLConsistency(migrationsFS)
	}

	prodSchemaConsistencyOnce.Do(func() {
		prodLatestVersion, prodSchemaConsistencyErr = validateSchemaSQLConsistency(migrationsFS)
	})
	return prodLatestVersion, prodSchemaConsistencyErr
}

func validateSchemaSQLConsistency(migrationsFS fs.FS) (uint, error) {
	latestVersion, err := GetLatestMigrationVersion(migrationsFS)
	if err != nil {
		return 0, fmt.Errorf("failed to get latest migration version: %w", err)
	}

	// Validate consistency in an isolated temp database so we only pay this cost once.
	tmpDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return 0, fmt.Errorf("failed to create temp database for schema validation: %w", err)
	}
	defer tmpDB.Close()

	if _, err := tmpDB.Exec(schemaSQL); err != nil {
		return 0, fmt.Errorf("failed to initialize temp schema database: %w", err)
	}

	tmpWrapper := &DB{tmpDB}
	schemaFromSQL, err := tmpWrapper.GetDatabaseSchema()
	if err != nil {
		return 0, fmt.Errorf("failed to get schema from schema.sql: %w", err)
	}

	schemaFromMigrations, err := tmpWrapper.GetSchemaAtMigration(migrationsFS, latestVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to get schema at migration v%d: %w", latestVersion, err)
	}

	score, differences := CompareSchemas(schemaFromSQL, schemaFromMigrations)
	if score != 100 {
		log.Printf("⚠️  WARNING: schema.sql is out of sync with migrations!")
		log.Printf("   Schema from schema.sql differs from migration v%d (similarity: %d%%)", latestVersion, score)
		log.Printf("   Differences:")
		for _, diff := range differences {
			log.Printf("     %s", diff)
		}
		log.Printf("")
		log.Printf("   This indicates that schema.sql needs to be updated to match the latest migrations.")
		log.Printf("   Please run the schema consistency test or regenerate schema.sql from migrations.")
		log.Printf("")
		return 0, fmt.Errorf("schema.sql is out of sync with migration v%d (similarity: %d%%). Cannot baseline safely", latestVersion, score)
	}

	return latestVersion, nil
}

// applyPragmas applies essential SQLite PRAGMAs for performance and concurrency.
// These settings are extracted from schema.sql and applied to all databases
// regardless of whether they were created from scratch or via migrations.
func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA busy_timeout = 30000", // 30s timeout for heavy PCAP replay load
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute %q: %w", pragma, err)
		}
	}

	return nil
}

// ApplyPragmas applies the canonical SQLite PRAGMA set used by both
// production code and shared test helpers.
func ApplyPragmas(db *sql.DB) error {
	return applyPragmas(db)
}

func NewDB(path string) (*DB, error) {
	return NewDBWithMigrationCheck(path, true)
}

// NewDBWithMigrationCheck opens a database and optionally checks for pending migrations.
// If checkMigrations is true and migrations are pending, returns an error prompting user to run migrations.
func NewDBWithMigrationCheck(path string, checkMigrations bool) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = db.Close()
		}
	}()

	dbWrapper := &DB{db}

	// Apply essential PRAGMAs for all databases, regardless of how they were created.
	// These settings are critical for performance and concurrency:
	// - WAL mode allows concurrent reads and writes
	// - busy_timeout prevents immediate "database is locked" errors
	// - NORMAL synchronous mode balances safety and performance
	// - MEMORY temp_store improves query performance
	if err := applyPragmas(db); err != nil {
		return nil, fmt.Errorf("failed to apply PRAGMAs: %w", err)
	}

	// Check if schema_migrations table exists
	var schemaMigrationsExists bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM sqlite_master
		WHERE type='table' AND name='schema_migrations'
	`).Scan(&schemaMigrationsExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check for schema_migrations table: %w", err)
	}

	// Get migrations filesystem
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		return nil, fmt.Errorf("failed to get migrations filesystem: %w", err)
	}

	// Case 1: Database with migration history - check if migrations are needed
	if schemaMigrationsExists {
		if checkMigrations {
			shouldExit, err := dbWrapper.CheckAndPromptMigrations(migrationsFS)
			if shouldExit {
				return nil, err
			}
		}
		closeOnError = false
		return dbWrapper, nil
	}

	// Case 2: Database without schema_migrations table
	// Check if this is a legacy database (has tables) or a fresh database
	var tableCount int
	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
	`).Scan(&tableCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count tables: %w", err)
	}

	isLegacyDB := (tableCount > 0)

	// Case 2a: Legacy database without migration history - detect and baseline
	if isLegacyDB && checkMigrations {
		log.Printf("⚠️  Database exists but has no schema_migrations table!")
		log.Printf("   Attempting to detect schema version...")

		detectedVersion, matchScore, differences, err := dbWrapper.DetectSchemaVersion(migrationsFS)
		if err != nil {
			return nil, fmt.Errorf("failed to detect schema version: %w", err)
		}

		log.Printf("   Schema detection results:")
		log.Printf("   - Best match: version %d (score: %d%%)", detectedVersion, matchScore)

		if matchScore == 100 {
			// Perfect match - baseline at this version
			log.Printf("   - Perfect match! Baselining at version %d", detectedVersion)
			if err := dbWrapper.BaselineAtVersion(detectedVersion); err != nil {
				return nil, fmt.Errorf("failed to baseline at version %d: %w", detectedVersion, err)
			}

			// Check if more migrations are needed
			latestVersion, err := GetLatestMigrationVersion(migrationsFS)
			if err != nil {
				return nil, fmt.Errorf("failed to get latest version: %w", err)
			}

			if detectedVersion < latestVersion {
				log.Printf("")
				log.Printf("   Database has been baselined at version %d", detectedVersion)
				log.Printf("   There are %d additional migrations available (up to version %d)",
					latestVersion-detectedVersion, latestVersion)
				log.Printf("")
				log.Printf("   To apply remaining migrations, run:")
				log.Printf("      velocity-report migrate up")
				log.Printf("")
				return nil, fmt.Errorf("database baselined at version %d, but migrations to version %d are available. Please run migrations", detectedVersion, latestVersion)
			}

			log.Printf("   Database is up to date!")
			closeOnError = false
			return dbWrapper, nil
		}

		// Not a perfect match - show differences and ask user
		log.Printf("   - No perfect match found (best: %d%%)", matchScore)
		log.Printf("")
		log.Printf("   Schema differences from version %d:", detectedVersion)
		for _, diff := range differences {
			log.Printf("     %s", diff)
		}
		log.Printf("")
		log.Printf("   The current schema does not exactly match any known migration version.")
		log.Printf("   Closest match is version %d with %d%% similarity.", detectedVersion, matchScore)
		log.Printf("")
		log.Printf("   Options:")
		log.Printf("   1. Baseline at version %d and apply remaining migrations:", detectedVersion)
		log.Printf("      velocity-report migrate baseline %d", detectedVersion)
		log.Printf("      velocity-report migrate up")
		log.Printf("")
		log.Printf("   2. Manually inspect the differences and adjust your schema")
		log.Printf("")
		return nil, fmt.Errorf("schema does not match any known version (best match: v%d at %d%%). Manual intervention required", detectedVersion, matchScore)
	}

	// Case 2b: Fresh database - initialize with schema.sql and baseline at latest version
	_, err = db.Exec(schemaSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	log.Println("ran database initialisation script")

	// Verify schema.sql consistency.
	// Production reuses a process-wide cached result; DevMode re-checks each call.
	latestVersion, err := getSchemaConsistencyResult(migrationsFS)
	if err != nil {
		return nil, err
	}

	// Schema is consistent - safe to baseline at latest version
	if err := dbWrapper.BaselineAtVersion(latestVersion); err != nil {
		return nil, fmt.Errorf("failed to baseline fresh database at version %d: %w", latestVersion, err)
	}

	// Verify baseline was successful
	currentVersion, _, err := dbWrapper.MigrateVersion(migrationsFS)
	if err != nil {
		return nil, fmt.Errorf("failed to verify baseline: %w", err)
	}
	if currentVersion != latestVersion {
		return nil, fmt.Errorf("baseline verification failed: expected version %d, got %d", latestVersion, currentVersion)
	}

	closeOnError = false
	return dbWrapper, nil
}

// OpenDB opens a database connection without running schema initialization.
// This is useful for migration commands that manage schema independently.
// Note: PRAGMAs are still applied for performance and concurrency.
func OpenDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// Apply PRAGMAs even for migration commands
	if err := applyPragmas(db); err != nil {
		return nil, fmt.Errorf("failed to apply PRAGMAs: %w", err)
	}

	return &DB{db}, nil
}

// LatestMigrationVersion returns the latest available migration version from
// the canonical migrations source for the current build mode.
func LatestMigrationVersion() (uint, error) {
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		return 0, err
	}
	return GetLatestMigrationVersion(migrationsFS)
}
