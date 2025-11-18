package db

import (
	"os"
	"testing"
)

// TestGetDatabaseSchema verifies we can extract schema from a database
func TestGetDatabaseSchema(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	schema, err := db.GetDatabaseSchema()
	if err != nil {
		t.Fatalf("GetDatabaseSchema failed: %v", err)
	}

	// Should have some tables
	if len(schema) == 0 {
		t.Error("Expected schema to have some objects")
	}

	// Check for a known table
	found := false
	for name := range schema {
		if name == "radar_data" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find radar_data table in schema")
	}
}

// TestCompareSchemas verifies schema comparison works correctly
func TestCompareSchemas(t *testing.T) {
	schema1 := map[string]string{
		"table1": "CREATE TABLE table1 (id INT)",
		"table2": "CREATE TABLE table2 (name TEXT)",
	}

	schema2 := map[string]string{
		"table1": "CREATE TABLE table1 (id INT)",
		"table2": "CREATE TABLE table2 (name TEXT)",
	}

	score, diffs := CompareSchemas(schema1, schema2)
	if score != 100 {
		t.Errorf("Expected 100%% match, got %d%%", score)
	}
	if len(diffs) != 0 {
		t.Errorf("Expected no differences, got %d", len(diffs))
	}
}

// TestCompareSchemas_WithDifferences verifies schema comparison detects differences
func TestCompareSchemas_WithDifferences(t *testing.T) {
	schema1 := map[string]string{
		"table1": "CREATE TABLE table1 (id INT)",
		"table3": "CREATE TABLE table3 (extra TEXT)",
	}

	schema2 := map[string]string{
		"table1": "CREATE TABLE table1 (id INT)",
		"table2": "CREATE TABLE table2 (name TEXT)",
	}

	score, diffs := CompareSchemas(schema1, schema2)

	// Should be 33% match (1 out of 3 unique objects match)
	if score != 33 {
		t.Errorf("Expected 33%% match, got %d%%", score)
	}

	if len(diffs) == 0 {
		t.Error("Expected differences to be reported")
	}
}

// TestGetSchemaAtMigration verifies we can recreate schema at a specific migration
func TestGetSchemaAtMigration(t *testing.T) {
	db := setupEmptyTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsDir := setupTestMigrations(t)

	// Get schema at version 1
	schema, err := db.GetSchemaAtMigration(migrationsDir, 1)
	if err != nil {
		t.Fatalf("GetSchemaAtMigration failed: %v", err)
	}

	// Should have the test table from migration 1
	if _, exists := schema["test_table"]; !exists {
		t.Error("Expected test_table to exist at version 1")
	}

	// Should not have the column from migration 2
	if _, exists := schema["test_column_idx"]; exists {
		t.Error("Did not expect test_column_idx to exist at version 1")
	}
}

// TestDetectSchemaVersion verifies schema version detection works
func TestDetectSchemaVersion(t *testing.T) {
	// Create a database at version 1
	db := setupEmptyTestDB(t)
	defer cleanupTestDB(t, db)

	migrationsDir := setupTestMigrations(t)

	// Apply migration 1
	err := db.MigrateTo(migrationsDir, 1)
	if err != nil {
		t.Fatalf("MigrateTo(1) failed: %v", err)
	}

	// Remove schema_migrations table to simulate legacy database
	_, err = db.Exec("DROP TABLE schema_migrations")
	if err != nil {
		t.Fatalf("Failed to drop schema_migrations: %v", err)
	}

	// Detect version
	detectedVersion, matchScore, diffs, err := db.DetectSchemaVersion(migrationsDir)
	if err != nil {
		t.Fatalf("DetectSchemaVersion failed: %v", err)
	}

	if detectedVersion != 1 {
		t.Errorf("Expected version 1, got %d", detectedVersion)
	}

	if matchScore != 100 {
		t.Errorf("Expected 100%% match, got %d%%", matchScore)
		for _, diff := range diffs {
			t.Logf("Diff: %s", diff)
		}
	}

	if len(diffs) != 0 {
		t.Errorf("Expected no differences, got %d", len(diffs))
	}
}

// TestNewDBWithMigrationCheck_LegacyDatabase verifies handling of legacy databases
func TestNewDBWithMigrationCheck_LegacyDatabase(t *testing.T) {
	// Create a database at version 1 without schema_migrations
	tmpDB := setupEmptyTestDB(t)
	migrationsDir := setupTestMigrations(t)

	// Apply migration 1
	err := tmpDB.MigrateTo(migrationsDir, 1)
	if err != nil {
		t.Fatalf("MigrateTo(1) failed: %v", err)
	}

	// Remove schema_migrations table to simulate legacy database
	_, err = tmpDB.Exec("DROP TABLE schema_migrations")
	if err != nil {
		t.Fatalf("Failed to drop schema_migrations: %v", err)
	}

	// Get the database path
	dbPath := t.Name() + "_migrations.db"
	tmpDB.Close()

	// Try to open with migration check - should detect version and error
	_, err = NewDBWithMigrationCheck(dbPath, migrationsDir, true)

	// Should get an error about needing to run migrations
	if err == nil {
		t.Error("Expected error about needing migrations")
	} else {
		t.Logf("Got expected error: %v", err)
	}

	// Cleanup
	os.Remove(dbPath)
	os.Remove(dbPath + "-shm")
	os.Remove(dbPath + "-wal")
}

// TestNewDBWithMigrationCheck_LegacyDatabasePerfectMatch tests baselining when perfect match found
func TestNewDBWithMigrationCheck_LegacyDatabasePerfectMatch(t *testing.T) {
	// Create a database at the latest version without schema_migrations
	tmpDB := setupEmptyTestDB(t)
	migrationsDir := setupTestMigrations(t)

	// Apply all migrations
	err := tmpDB.MigrateUp(migrationsDir)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Remove schema_migrations table
	_, err = tmpDB.Exec("DROP TABLE schema_migrations")
	if err != nil {
		t.Fatalf("Failed to drop schema_migrations: %v", err)
	}

	dbPath := t.Name() + "_migrations.db"
	tmpDB.Close()

	// Try to open with migration check - should detect version 2 (latest) and baseline
	db, err := NewDBWithMigrationCheck(dbPath, migrationsDir, true)

	// Should succeed since we're at latest version
	if err != nil {
		t.Errorf("Expected success when at latest version, got: %v", err)
	}

	if db != nil {
		db.Close()
	}

	// Cleanup
	os.Remove(dbPath)
	os.Remove(dbPath + "-shm")
	os.Remove(dbPath + "-wal")
}
