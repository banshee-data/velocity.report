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
		log.Fatalf("Cannot load migrations: %v — check the binary was built correctly with embedded migrations", err)
	}

	// Open database connection without running schema initialisation
	// (migrations will manage the schema)
	database, err := OpenDB(dbPath)
	if err != nil {
		log.Fatalf("Cannot open database at %s: %v — check path exists and directory is writable", dbPath, err)
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
			log.Fatal("Missing version number. Usage: velocity-report migrate version <N>")
		}
		handleMigrateVersion(database, migrationsFS, args[1])

	case "force":
		if len(args) < 2 {
			log.Fatal("Missing version number. Usage: velocity-report migrate force <N>")
		}
		handleMigrateForce(database, migrationsFS, args[1])

	case "baseline":
		if len(args) < 2 {
			log.Fatal("Missing version number. Usage: velocity-report migrate baseline <N>")
		}
		handleMigrateBaseline(database, args[1])

	case "detect":
		handleMigrateDetect(database, migrationsFS)

	case "help":
		PrintMigrateHelp()

	default:
		fmt.Printf("Unknown migrate action %q — see available commands below.\n\n", action)
		PrintMigrateHelp()
		os.Exit(1)
	}
}

// handleMigrateUp applies all pending migrations
func handleMigrateUp(database *DB, migrationsFS fs.FS) {
	log.Printf("Applying pending migrations...")
	if err := database.MigrateUp(migrationsFS); err != nil {
		log.Fatalf("Migration failed: %v\nTry: run 'velocity-report migrate status' to inspect the current state.", err)
	}
	log.Println("✓ Migrations applied — schema is current")

	// Show current version
	version, dirty, _ := database.MigrateVersion(migrationsFS)
	log.Printf("Now at version %d (dirty: %v)", version, dirty)
}

// handleMigrateDown rolls back one migration
func handleMigrateDown(database *DB, migrationsFS fs.FS) {
	log.Printf("Rolling back one migration...")
	if err := database.MigrateDown(migrationsFS); err != nil {
		log.Fatalf("Rollback failed: %v\nTry: run 'velocity-report migrate status' to check if the database is in a dirty state.", err)
	}
	log.Println("✓ Rolled back one migration")

	// Show current version
	version, dirty, _ := database.MigrateVersion(migrationsFS)
	log.Printf("Now at version %d (dirty: %v)", version, dirty)
}

// handleMigrateStatus displays the current migration status
func handleMigrateStatus(database *DB, migrationsFS fs.FS) {
	version, dirty, err := database.MigrateVersion(migrationsFS)
	if err != nil {
		log.Fatalf("Cannot read migration status: %v — check the database is accessible and not locked by another process", err)
	}

	status, err := database.GetMigrationStatus(migrationsFS)
	if err != nil {
		log.Fatalf("Cannot read migration status: %v — check the database is accessible and not locked by another process", err)
	}

	fmt.Println("=== Migration Status ===")
	fmt.Printf("Current version: %d\n", version)
	fmt.Printf("Dirty: %v\n", dirty)
	fmt.Printf("Schema migrations table exists: %v\n", status["schema_migrations_exists"])

	if dirty {
		fmt.Println("\n⚠️  Database is in a dirty state.")
		fmt.Println("A migration stopped partway through. To recover:")
		fmt.Println("  1. Check the database for partially-applied changes")
		fmt.Println("  2. Fix anything that looks wrong")
		fmt.Println("  3. Run: velocity-report migrate force <version>")
	}
}

// handleMigrateVersion migrates to a specific version
func handleMigrateVersion(database *DB, migrationsFS fs.FS, versionStr string) {
	var targetVersion uint
	if _, err := fmt.Sscanf(versionStr, "%d", &targetVersion); err != nil {
		log.Fatalf("Not a valid version number: %s — provide a numeric version, e.g. 'velocity-report migrate version 6'", versionStr)
	}

	log.Printf("Migrating to version %d...", targetVersion)
	if err := database.MigrateTo(migrationsFS, targetVersion); err != nil {
		log.Fatalf("Migration to version %d failed: %v\nTry: run 'velocity-report migrate status' to check current state.", targetVersion, err)
	}
	log.Printf("✓ Now at version %d", targetVersion)
}

// handleMigrateForce forces the migration version (recovery only)
func handleMigrateForce(database *DB, migrationsFS fs.FS, versionStr string) {
	var forceVersion int
	if _, err := fmt.Sscanf(versionStr, "%d", &forceVersion); err != nil {
		log.Fatalf("Not a valid version number: %s — e.g. 'velocity-report migrate force 5'", versionStr)
	}

	fmt.Printf("⚠️  This will force the migration version to %d.\n", forceVersion)
	fmt.Println("Only use this to recover from a dirty migration state — not as a shortcut.")
	fmt.Print("Continue? [y/N]: ")

	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		log.Println("Aborted.")
		os.Exit(0)
	}

	if err := database.MigrateForce(migrationsFS, forceVersion); err != nil {
		log.Fatalf("Force failed: %v — check the database is writable and the version number is valid", err)
	}
	log.Printf("✓ Version forced to %d", forceVersion)
}

// handleMigrateBaseline sets the baseline version without running migrations
func handleMigrateBaseline(database *DB, versionStr string) {
	var baselineVersion uint
	if _, err := fmt.Sscanf(versionStr, "%d", &baselineVersion); err != nil {
		log.Fatalf("Not a valid version number: %s — e.g. 'velocity-report migrate baseline 4'", versionStr)
	}

	log.Printf("Setting baseline to version %d...", baselineVersion)
	if err := database.BaselineAtVersion(baselineVersion); err != nil {
		log.Fatalf("Baseline failed: %v — try 'velocity-report migrate detect' to find the correct version", err)
	}
	log.Printf("✓ Baselined at version %d", baselineVersion)
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
		log.Fatalf("Cannot check schema_migrations table: %v — the database may be corrupted or locked", err)
	}

	if schemaMigrationsExists {
		// Database has schema_migrations - show current version
		version, dirty, err := database.MigrateVersion(migrationsFS)
		if err != nil {
			log.Fatalf("Unable to read migration version: %v — check that the database file is accessible and not locked", err)
		}

		latestVersion, err := GetLatestMigrationVersion(migrationsFS)
		if err != nil {
			log.Fatalf("Unable to determine latest migration version: %v — check that the binary includes embedded migration files", err)
		}

		fmt.Println("=== Schema Migration Status ===")
		fmt.Printf("Current version: %d\n", version)
		fmt.Printf("Latest available: %d\n", latestVersion)
		fmt.Printf("Dirty state: %v\n", dirty)
		fmt.Println()

		if version < latestVersion {
			fmt.Printf("⚠️  %d version(s) behind. Run 'velocity-report migrate up' to catch up.\n", latestVersion-version)
		} else if version == latestVersion && !dirty {
			fmt.Println("✓ Schema is up to date.")
		} else if dirty {
			fmt.Println("⚠️  Database is in a dirty state and needs recovery.")
		}
	} else {
		// Legacy database - run schema detection
		fmt.Println("No schema_migrations table — running automatic detection against known versions...")
		fmt.Println()

		detectedVersion, matchScore, differences, err := database.DetectSchemaVersion(migrationsFS)
		if err != nil {
			log.Fatalf("Schema detection failed: %v — check the database file is readable", err)
		}

		latestVersion, err := GetLatestMigrationVersion(migrationsFS)
		if err != nil {
			log.Fatalf("Unable to determine latest migration version: %v — check that the binary includes embedded migration files", err)
		}

		fmt.Println("=== Schema Detection Results ===")
		fmt.Printf("Closest match: version %d\n", detectedVersion)
		fmt.Printf("Similarity: %d%%\n", matchScore)
		fmt.Printf("Latest available: %d\n", latestVersion)
		fmt.Println()

		if matchScore == 100 {
			fmt.Println("✓ Exact match.")
			fmt.Println()
			fmt.Println("To bring this database under migration control:")
			fmt.Printf("  1. velocity-report migrate baseline %d\n", detectedVersion)
			if detectedVersion < latestVersion {
				fmt.Println("  2. velocity-report migrate up")
			}
		} else {
			fmt.Printf("⚠️  No exact match (best: %d%%).\n", matchScore)
			fmt.Println()
			fmt.Println("Differences from nearest version:")
			for _, diff := range differences {
				fmt.Printf("  %s\n", diff)
			}
			fmt.Println()
			fmt.Println("Options:")
			fmt.Printf("  1. Baseline at nearest version: velocity-report migrate baseline %d\n", detectedVersion)
			fmt.Println("  2. Inspect the schema manually before deciding")
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
	fmt.Println("  down            Roll back one migration")
	fmt.Println("  status          Show current migration status and version")
	fmt.Println("  detect          Detect schema version (for databases without schema_migrations)")
	fmt.Println("  version <N>     Migrate to a specific version N")
	fmt.Println("  force <N>       Force version to N (recovery only — not a shortcut)")
	fmt.Println("  baseline <N>    Mark version as N without running migrations")
	fmt.Println("  help            Show this help")
	fmt.Println()
	fmt.Println("Schema Detection:")
	fmt.Println("  The 'detect' command examines databases that pre-date the migration system:")
	fmt.Println("  - Compares the current schema against every known migration point")
	fmt.Println("  - Reports similarity score and any differences")
	fmt.Println("  - Suggests a baseline version for legacy databases")
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
	fmt.Println("Upgrading a Legacy Database (typical workflow):")
	fmt.Println("  1. velocity-report migrate detect        # Find current schema version")
	fmt.Println("  2. velocity-report migrate baseline <N>  # Set version from detect results")
	fmt.Println("  3. velocity-report migrate up            # Apply remaining migrations")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --db-path <path>    Path to database file (default: sensor_data.db)")
	fmt.Println()
	fmt.Println("Further reading:")
	fmt.Println("  - internal/db/migrations/README.md")
	fmt.Println("  - docs/database-migrations.md")
}
