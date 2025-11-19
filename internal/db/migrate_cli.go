package db

import (
	"fmt"
	"io/fs"
	"log"
	"os"
)

// RunMigrateCommand handles the 'migrate' subcommand dispatching
func RunMigrateCommand(args []string, dbPath string) {
	if len(args) < 1 {
		PrintMigrateHelp()
		os.Exit(1)
	}

	action := args[0]

	// Get migrations filesystem (uses embedded FS in production, local files in dev)
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		log.Fatalf("Failed to get migrations filesystem: %v", err)
	}

	// Open database connection without running schema initialization
	// (migrations will manage the schema)
	database, err := OpenDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	switch action {
	case "up":
		handleMigrateUp(database, migrationsFS)

	case "down":
		handleMigrateDown(database, migrationsFS)

	case "status":
		handleMigrateStatus(database, migrationsFS)

	case "version":
		if len(args) < 2 {
			log.Fatal("Usage: velocity-report migrate version <version_number>")
		}
		handleMigrateVersion(database, migrationsFS, args[1])

	case "force":
		if len(args) < 2 {
			log.Fatal("Usage: velocity-report migrate force <version_number>")
		}
		handleMigrateForce(database, migrationsFS, args[1])

	case "baseline":
		if len(args) < 2 {
			log.Fatal("Usage: velocity-report migrate baseline <version_number>")
		}
		handleMigrateBaseline(database, args[1])

	case "detect":
		handleMigrateDetect(database, migrationsFS)

	case "help":
		PrintMigrateHelp()

	default:
		fmt.Printf("Unknown migrate action: %s\n\n", action)
		PrintMigrateHelp()
		os.Exit(1)
	}
}

// handleMigrateUp applies all pending migrations
func handleMigrateUp(database *DB, migrationsFS fs.FS) {
	log.Printf("Running migrations...")
	if err := database.MigrateUp(migrationsFS); err != nil {
		log.Fatalf("Migration up failed: %v", err)
	}
	log.Println("✓ All migrations applied successfully")

	// Show current version
	version, dirty, _ := database.MigrateVersion(migrationsFS)
	log.Printf("Current version: %d (dirty: %v)", version, dirty)
}

// handleMigrateDown rolls back one migration
func handleMigrateDown(database *DB, migrationsFS fs.FS) {
	log.Printf("Rolling back one migration...")
	if err := database.MigrateDown(migrationsFS); err != nil {
		log.Fatalf("Migration down failed: %v", err)
	}
	log.Println("✓ Migration rolled back successfully")

	// Show current version
	version, dirty, _ := database.MigrateVersion(migrationsFS)
	log.Printf("Current version: %d (dirty: %v)", version, dirty)
}

// handleMigrateStatus displays the current migration status
func handleMigrateStatus(database *DB, migrationsFS fs.FS) {
	version, dirty, err := database.MigrateVersion(migrationsFS)
	if err != nil {
		log.Fatalf("Failed to get migration status: %v", err)
	}

	status, err := database.GetMigrationStatus(migrationsFS)
	if err != nil {
		log.Fatalf("Failed to get migration status: %v", err)
	}

	fmt.Println("=== Migration Status ===")
	fmt.Printf("Current version: %d\n", version)
	fmt.Printf("Dirty: %v\n", dirty)
	fmt.Printf("Schema migrations table exists: %v\n", status["schema_migrations_exists"])

	if dirty {
		fmt.Println("\n⚠️  WARNING: Database is in a dirty state!")
		fmt.Println("A migration failed mid-execution. You may need to:")
		fmt.Println("  1. Inspect the database manually")
		fmt.Println("  2. Fix any issues")
		fmt.Println("  3. Run: velocity-report migrate force <version>")
	}
}

// handleMigrateVersion migrates to a specific version
func handleMigrateVersion(database *DB, migrationsFS fs.FS, versionStr string) {
	var targetVersion uint
	if _, err := fmt.Sscanf(versionStr, "%d", &targetVersion); err != nil {
		log.Fatalf("Invalid version number: %s", versionStr)
	}

	log.Printf("Migrating to version %d...", targetVersion)
	if err := database.MigrateTo(migrationsFS, targetVersion); err != nil {
		log.Fatalf("Migration to version %d failed: %v", targetVersion, err)
	}
	log.Printf("✓ Migrated to version %d successfully", targetVersion)
}

// handleMigrateForce forces the migration version (recovery only)
func handleMigrateForce(database *DB, migrationsFS fs.FS, versionStr string) {
	var forceVersion int
	if _, err := fmt.Sscanf(versionStr, "%d", &forceVersion); err != nil {
		log.Fatalf("Invalid version number: %s", versionStr)
	}

	fmt.Printf("⚠️  WARNING: Forcing migration version to %d\n", forceVersion)
	fmt.Println("This should only be used to recover from a dirty migration state.")
	fmt.Print("Continue? [y/N]: ")

	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		log.Println("Aborted")
		os.Exit(0)
	}

	if err := database.MigrateForce(migrationsFS, forceVersion); err != nil {
		log.Fatalf("Force migration failed: %v", err)
	}
	log.Printf("✓ Migration version forced to %d", forceVersion)
}

// handleMigrateBaseline sets the baseline version without running migrations
func handleMigrateBaseline(database *DB, versionStr string) {
	var baselineVersion uint
	if _, err := fmt.Sscanf(versionStr, "%d", &baselineVersion); err != nil {
		log.Fatalf("Invalid version number: %s", versionStr)
	}

	log.Printf("Baselining database at version %d...", baselineVersion)
	if err := database.BaselineAtVersion(baselineVersion); err != nil {
		log.Fatalf("Baseline failed: %v", err)
	}
	log.Printf("✓ Database baselined at version %d", baselineVersion)
}

// handleMigrateDetect detects the schema version of a database
func handleMigrateDetect(database *DB, migrationsFS fs.FS) {
	log.Println("Detecting schema version...")
	log.Println()

	// Check if schema_migrations exists first
	var schemaMigrationsExists bool
	err := database.QueryRow(`
		SELECT COUNT(*) > 0
		FROM sqlite_master
		WHERE type='table' AND name='schema_migrations'
	`).Scan(&schemaMigrationsExists)

	if err != nil {
		log.Fatalf("Failed to check for schema_migrations table: %v", err)
	}

	if schemaMigrationsExists {
		// Database has schema_migrations - show current version
		version, dirty, err := database.MigrateVersion(migrationsFS)
		if err != nil {
			log.Fatalf("Failed to get migration version: %v", err)
		}

		latestVersion, err := GetLatestMigrationVersion(migrationsFS)
		if err != nil {
			log.Fatalf("Failed to get latest migration version: %v", err)
		}

		fmt.Println("=== Schema Migration Status ===")
		fmt.Printf("Current version: %d\n", version)
		fmt.Printf("Latest available: %d\n", latestVersion)
		fmt.Printf("Dirty state: %v\n", dirty)
		fmt.Println()

		if version < latestVersion {
			fmt.Printf("⚠️  Database is %d version(s) behind. Run 'velocity-report migrate up' to update.\n", latestVersion-version)
		} else if version == latestVersion && !dirty {
			fmt.Println("✓ Database is up to date!")
		} else if dirty {
			fmt.Println("⚠️  Database is in a dirty state. Recovery needed.")
		}
	} else {
		// Legacy database - run schema detection
		fmt.Println("No schema_migrations table found - running automatic detection...")
		fmt.Println()

		detectedVersion, matchScore, differences, err := database.DetectSchemaVersion(migrationsFS)
		if err != nil {
			log.Fatalf("Schema detection failed: %v", err)
		}

		latestVersion, err := GetLatestMigrationVersion(migrationsFS)
		if err != nil {
			log.Fatalf("Failed to get latest migration version: %v", err)
		}

		fmt.Println("=== Schema Detection Results ===")
		fmt.Printf("Best match: version %d\n", detectedVersion)
		fmt.Printf("Similarity: %d%%\n", matchScore)
		fmt.Printf("Latest available: %d\n", latestVersion)
		fmt.Println()

		if matchScore == 100 {
			fmt.Println("✓ Perfect match found!")
			fmt.Println()
			fmt.Println("To baseline and apply remaining migrations:")
			fmt.Printf("  1. velocity-report migrate baseline %d\n", detectedVersion)
			if detectedVersion < latestVersion {
				fmt.Println("  2. velocity-report migrate up")
			}
		} else {
			fmt.Printf("⚠️  No perfect match found (best: %d%%)\n", matchScore)
			fmt.Println()
			fmt.Println("Schema differences:")
			for _, diff := range differences {
				fmt.Printf("  %s\n", diff)
			}
			fmt.Println()
			fmt.Println("Options:")
			fmt.Printf("  1. Baseline at closest version: velocity-report migrate baseline %d\n", detectedVersion)
			fmt.Println("  2. Manually inspect and adjust schema before baselining")
		}
	}
}

// PrintMigrateHelp displays the help message for the migrate command
func PrintMigrateHelp() {
	fmt.Println("Database Migration Commands")
	fmt.Println()
	fmt.Println("Usage: velocity-report migrate <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  up              Apply all pending migrations")
	fmt.Println("  down            Rollback one migration")
	fmt.Println("  status          Show current migration status and version")
	fmt.Println("  detect          Detect schema version (for databases without schema_migrations)")
	fmt.Println("  version <N>     Migrate to specific version N")
	fmt.Println("  force <N>       Force migration version to N (recovery only)")
	fmt.Println("  baseline <N>    Set migration version to N without running migrations")
	fmt.Println("  help            Show this help message")
	fmt.Println()
	fmt.Println("Schema Detection:")
	fmt.Println("  The 'detect' command analyzes databases without schema_migrations table:")
	fmt.Println("  - Compares current schema against all known migration points")
	fmt.Println("  - Calculates similarity score and identifies differences")
	fmt.Println("  - Suggests baseline version for legacy database upgrades")
	fmt.Println("  - Automatically handles databases from pre-migration versions")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  velocity-report migrate up")
	fmt.Println("  velocity-report migrate down")
	fmt.Println("  velocity-report migrate status")
	fmt.Println("  velocity-report migrate detect")
	fmt.Println("  velocity-report migrate version 3")
	fmt.Println("  velocity-report migrate force 2")
	fmt.Println("  velocity-report migrate baseline 6")
	fmt.Println()
	fmt.Println("Legacy Database Upgrade (typical workflow):")
	fmt.Println("  1. velocity-report migrate detect        # Find current schema version")
	fmt.Println("  2. velocity-report migrate baseline <N>  # Set version based on detect results")
	fmt.Println("  3. velocity-report migrate up            # Apply remaining migrations")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --db-path <path>    Path to database file (default: sensor_data.db)")
	fmt.Println()
	fmt.Println("For more information, see:")
	fmt.Println("  - internal/db/migrations/README.md")
	fmt.Println("  - docs/database-migrations.md")
}
