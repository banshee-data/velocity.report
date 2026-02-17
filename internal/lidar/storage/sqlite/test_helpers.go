package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// setupTrackingPipelineTestDB creates a test database with proper schema from schema.sql.
// This avoids hardcoded CREATE TABLE statements that can get out of sync with migrations.
func setupTrackingPipelineTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Apply essential PRAGMAs
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			t.Fatalf("Failed to execute %q: %v", pragma, err)
		}
	}

	// Read and execute schema.sql from the db package
	// From internal/lidar/storage/sqlite, we need to go up 4 levels to reach internal/db
	schemaPath := filepath.Join("..", "..", "..", "db", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to read schema.sql: %v", err)
	}

	if _, err := db.Exec(string(schemaSQL)); err != nil {
		db.Close()
		t.Fatalf("Failed to execute schema.sql: %v", err)
	}

	// Baseline at latest migration version
	// NOTE: Update this when new migrations are added to internal/db/migrations/
	latestMigrationVersion := 15
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (?, false)`, latestMigrationVersion); err != nil {
		db.Close()
		t.Fatalf("Failed to baseline migrations: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}
