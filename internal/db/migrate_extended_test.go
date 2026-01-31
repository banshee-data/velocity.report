package db

import (
	"io/fs"
	"path/filepath"
	"testing"
)

// TestMigrateUp tests applying all migrations
func TestMigrateUp_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply migrations
	err = db.MigrateUp(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Verify version is set
	version, dirty, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version == 0 {
		t.Error("Expected non-zero version after migration")
	}
	if dirty {
		t.Error("Database should not be dirty after successful migration")
	}
}

// TestMigrateUp_AlreadyUpToDate tests MigrateUp when already at latest
func TestMigrateUp_AlreadyUpToDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply migrations twice
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("First MigrateUp failed: %v", err)
	}

	// Second call should succeed (no change)
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("Second MigrateUp failed: %v", err)
	}
}

// TestMigrateDown tests rolling back one migration
func TestMigrateDown_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply all migrations first
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	versionBefore, _, _ := db.MigrateVersion(migrationsFS)

	// Roll back one
	if err := db.MigrateDown(migrationsFS); err != nil {
		t.Fatalf("MigrateDown failed: %v", err)
	}

	versionAfter, _, _ := db.MigrateVersion(migrationsFS)

	if versionAfter >= versionBefore {
		t.Errorf("Expected version to decrease: before=%d, after=%d", versionBefore, versionAfter)
	}
}

// TestMigrateVersion_FreshDatabase tests MigrateVersion with fresh database
func TestMigrateVersion_FreshDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	version, dirty, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version != 0 {
		t.Errorf("Expected version 0 for fresh database, got %d", version)
	}
	if dirty {
		t.Error("Fresh database should not be dirty")
	}
}

// TestMigrateForce tests forcing migration version
func TestMigrateForce_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// First apply some migrations
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Force to a specific version
	if err := db.MigrateForce(migrationsFS, 1); err != nil {
		t.Fatalf("MigrateForce failed: %v", err)
	}

	version, _, _ := db.MigrateVersion(migrationsFS)
	if version != 1 {
		t.Errorf("Expected version 1 after force, got %d", version)
	}
}

// TestMigrateTo tests migrating to a specific version
func TestMigrateTo_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Migrate to version 1
	if err := db.MigrateTo(migrationsFS, 1); err != nil {
		t.Fatalf("MigrateTo(1) failed: %v", err)
	}

	version, _, _ := db.MigrateVersion(migrationsFS)
	if version != 1 {
		t.Errorf("Expected version 1, got %d", version)
	}
}

// TestMigrateTo_AlreadyAtVersion tests MigrateTo when already at target
func TestMigrateTo_AlreadyAtVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Migrate to version 1 twice
	if err := db.MigrateTo(migrationsFS, 1); err != nil {
		t.Fatalf("First MigrateTo(1) failed: %v", err)
	}

	// Second call should succeed (no change)
	if err := db.MigrateTo(migrationsFS, 1); err != nil {
		t.Fatalf("Second MigrateTo(1) failed: %v", err)
	}
}

// TestBaselineAtVersion tests baselining a database
func TestBaselineAtVersion_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Baseline at version 5
	if err := db.BaselineAtVersion(5); err != nil {
		t.Fatalf("BaselineAtVersion failed: %v", err)
	}

	// Verify via direct query
	var version int
	var dirty int
	if err := db.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatalf("Failed to query schema_migrations: %v", err)
	}

	if version != 5 {
		t.Errorf("Expected version 5, got %d", version)
	}
	if dirty != 0 {
		t.Errorf("Expected dirty=0, got %d", dirty)
	}
}

// TestBaselineAtVersion_AlreadyHasMigrations tests baselining when migrations exist
func TestBaselineAtVersion_AlreadyHasMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Baseline once
	if err := db.BaselineAtVersion(5); err != nil {
		t.Fatalf("First BaselineAtVersion failed: %v", err)
	}

	// Second baseline should fail
	err = db.BaselineAtVersion(10)
	if err == nil {
		t.Error("Expected error when baselining twice")
	}
}

// TestGetMigrationStatus tests getting migration status
func TestGetMigrationStatus_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply migrations
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	status, err := db.GetMigrationStatus(migrationsFS)
	if err != nil {
		t.Fatalf("GetMigrationStatus failed: %v", err)
	}

	if status == nil {
		t.Fatal("Expected non-nil status")
	}

	if _, ok := status["current_version"]; !ok {
		t.Error("Expected current_version in status")
	}
	if _, ok := status["dirty"]; !ok {
		t.Error("Expected dirty in status")
	}
	if _, ok := status["schema_migrations_exists"]; !ok {
		t.Error("Expected schema_migrations_exists in status")
	}
}

// TestGetLatestMigrationVersion tests getting the latest migration version
func TestGetLatestMigrationVersion_Success(t *testing.T) {
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	version, err := GetLatestMigrationVersion(migrationsFS)
	if err != nil {
		t.Fatalf("GetLatestMigrationVersion failed: %v", err)
	}

	if version == 0 {
		t.Error("Expected non-zero latest version")
	}
}

// TestGetDatabaseSchema tests retrieving the database schema
func TestGetDatabaseSchema_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply migrations to have a schema
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	schema, err := db.GetDatabaseSchema()
	if err != nil {
		t.Fatalf("GetDatabaseSchema failed: %v", err)
	}

	if len(schema) == 0 {
		t.Error("Expected non-empty schema")
	}

	// Should have radar_objects table
	if _, ok := schema["radar_objects"]; !ok {
		t.Error("Expected radar_objects table in schema")
	}
}

// TestGetSchemaAtMigration tests getting schema at a specific migration
func TestGetSchemaAtMigration_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	schema, err := db.GetSchemaAtMigration(migrationsFS, 1)
	if err != nil {
		t.Fatalf("GetSchemaAtMigration failed: %v", err)
	}

	if len(schema) == 0 {
		t.Error("Expected non-empty schema at version 1")
	}
}

// TestCompareSchemas tests comparing two schemas
func TestCompareSchemas_Identical(t *testing.T) {
	schema := map[string]string{
		"table1": "CREATE TABLE table1 (id INTEGER PRIMARY KEY)",
		"table2": "CREATE TABLE table2 (name TEXT)",
	}

	score, diffs := CompareSchemas(schema, schema)

	if score != 100 {
		t.Errorf("Expected score 100 for identical schemas, got %d", score)
	}
	if len(diffs) != 0 {
		t.Errorf("Expected no differences, got %v", diffs)
	}
}

// TestCompareSchemas_Different tests comparing different schemas
func TestCompareSchemas_Different(t *testing.T) {
	schema1 := map[string]string{
		"table1": "CREATE TABLE table1 (id INTEGER PRIMARY KEY)",
		"table2": "CREATE TABLE table2 (name TEXT)",
	}
	schema2 := map[string]string{
		"table1": "CREATE TABLE table1 (id INTEGER PRIMARY KEY)",
		"table3": "CREATE TABLE table3 (value REAL)",
	}

	score, diffs := CompareSchemas(schema1, schema2)

	if score >= 100 {
		t.Errorf("Expected score less than 100 for different schemas, got %d", score)
	}
	if len(diffs) == 0 {
		t.Error("Expected differences for different schemas")
	}
}

// TestCompareSchemas_Empty tests comparing empty schemas
func TestCompareSchemas_Empty(t *testing.T) {
	score, diffs := CompareSchemas(map[string]string{}, map[string]string{})

	if score != 100 {
		t.Errorf("Expected score 100 for empty schemas, got %d", score)
	}
	if len(diffs) != 0 {
		t.Errorf("Expected no differences, got %v", diffs)
	}
}

// TestDetectSchemaVersion tests detecting schema version
func TestDetectSchemaVersion_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply migrations to version 1
	if err := db.MigrateTo(migrationsFS, 1); err != nil {
		t.Fatalf("MigrateTo failed: %v", err)
	}

	// Now detect
	version, score, _, err := db.DetectSchemaVersion(migrationsFS)
	if err != nil {
		t.Fatalf("DetectSchemaVersion failed: %v", err)
	}

	if version != 1 {
		t.Errorf("Expected detected version 1, got %d", version)
	}
	if score < 90 {
		t.Errorf("Expected high match score, got %d", score)
	}
}

// TestCheckAndPromptMigrations_WhenUpToDate tests when migrations are up to date
func TestCheckAndPromptMigrations_WhenUpToDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply all migrations
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	shouldExit, err := db.CheckAndPromptMigrations(migrationsFS)
	if err != nil {
		t.Fatalf("CheckAndPromptMigrations failed: %v", err)
	}
	if shouldExit {
		t.Error("Expected shouldExit=false when up to date")
	}
}

// TestCheckAndPromptMigrations_WhenOutOfDate tests when migrations are needed
func TestCheckAndPromptMigrations_WhenOutOfDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply only first migration
	if err := db.MigrateTo(migrationsFS, 1); err != nil {
		t.Fatalf("MigrateTo failed: %v", err)
	}

	latestVersion, _ := GetLatestMigrationVersion(migrationsFS)

	// Only test if there are more migrations available
	if latestVersion > 1 {
		shouldExit, err := db.CheckAndPromptMigrations(migrationsFS)
		if err == nil {
			t.Error("Expected error when migrations are needed")
		}
		if !shouldExit {
			t.Error("Expected shouldExit=true when migrations needed")
		}
	}
}

// TestMigrateLogger tests the migrate logger
func TestMigrateLogger(t *testing.T) {
	logger := &migrateLogger{}

	// Should not panic
	logger.Printf("test message: %s", "value")

	if logger.Verbose() {
		t.Error("Expected Verbose() to return false")
	}
}

// TestNormalizeSQLForComparison tests SQL normalisation
func TestNormalizeSQLForComparison(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes trailing semicolon",
			input:    "CREATE TABLE foo (id INTEGER);",
			expected: "CREATE TABLE foo (id INTEGER)",
		},
		{
			name:     "normalises whitespace",
			input:    "CREATE   TABLE   foo   (id   INTEGER)",
			expected: "CREATE TABLE foo (id INTEGER)",
		},
		{
			name:     "removes space before comma",
			input:    "CREATE TABLE foo (id INTEGER , name TEXT)",
			expected: "CREATE TABLE foo (id INTEGER, name TEXT)",
		},
		{
			name:     "normalises table name parenthesis",
			input:    "CREATE TABLE foo( id INTEGER )",
			expected: "CREATE TABLE foo ( id INTEGER )",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeSQLForComparison(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestGetLatestMigrationVersion_EmptyFS tests with empty filesystem
func TestGetLatestMigrationVersion_EmptyFS(t *testing.T) {
	// Create empty filesystem
	emptyFS := emptyFS{}

	_, err := GetLatestMigrationVersion(emptyFS)
	if err == nil {
		t.Error("Expected error for empty filesystem")
	}
}

// emptyFS implements fs.FS with no files
type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

// TestDetectSchemaVersion_VersionTwo tests detecting schema at version 2
func TestDetectSchemaVersion_VersionTwo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply migrations up to version 2
	if err := db.MigrateTo(migrationsFS, 2); err != nil {
		t.Fatalf("MigrateTo(2) failed: %v", err)
	}

	version, score, _, err := db.DetectSchemaVersion(migrationsFS)
	if err != nil {
		t.Fatalf("DetectSchemaVersion failed: %v", err)
	}

	if version != 2 {
		t.Errorf("Expected detected version 2, got %d", version)
	}
	if score < 90 {
		t.Errorf("Expected high match score (>=90), got %d", score)
	}
}

// TestDetectSchemaVersion_LatestVersion tests detecting latest schema version
func TestDetectSchemaVersion_LatestVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply all migrations
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	version, score, diffs, err := db.DetectSchemaVersion(migrationsFS)
	if err != nil {
		t.Fatalf("DetectSchemaVersion failed: %v", err)
	}

	latestVersion, err := GetLatestMigrationVersion(migrationsFS)
	if err != nil {
		t.Fatalf("GetLatestMigrationVersion failed: %v", err)
	}

	if version != latestVersion {
		t.Errorf("Expected detected version %d, got %d", latestVersion, version)
	}
	if score != 100 {
		t.Errorf("Expected perfect match score 100, got %d", score)
	}
	if len(diffs) != 0 {
		t.Errorf("Expected no differences, got %v", diffs)
	}
}

// TestGetMigrationStatus_AtLatest tests getting migration status when at latest
func TestGetMigrationStatus_AtLatest(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply all migrations
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	status, err := db.GetMigrationStatus(migrationsFS)
	if err != nil {
		t.Fatalf("GetMigrationStatus failed: %v", err)
	}

	if status == nil {
		t.Fatal("Expected non-nil status")
	}

	currentVersion, ok := status["current_version"]
	if !ok || currentVersion == nil {
		t.Error("Expected current_version in status")
	}

	dirty, ok := status["dirty"].(bool)
	if !ok || dirty {
		t.Error("Expected clean (non-dirty) state")
	}
}

// TestGetMigrationStatus_Empty tests getting migration status on empty database
func TestGetMigrationStatus_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	status, err := db.GetMigrationStatus(migrationsFS)
	if err != nil {
		t.Fatalf("GetMigrationStatus failed: %v", err)
	}

	if status == nil {
		t.Fatal("Expected non-nil status")
	}
}

// TestMigrateUp_AlreadyAtLatest tests MigrateUp when already at latest
func TestMigrateUp_AlreadyAtLatest(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply all migrations twice to test the "already at latest" case
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("First MigrateUp failed: %v", err)
	}

	// Second call should succeed (no-op)
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Errorf("Second MigrateUp should succeed: %v", err)
	}
}

// TestMigrateTo_UpAndDown tests MigrateTo for going up and down versions
func TestMigrateTo_UpAndDown(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Migrate to version 3
	if err := db.MigrateTo(migrationsFS, 3); err != nil {
		t.Fatalf("MigrateTo(3) failed: %v", err)
	}

	// Verify version
	version, _, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}
	if version != 3 {
		t.Errorf("Expected version 3, got %d", version)
	}

	// Migrate down to version 2
	if err := db.MigrateTo(migrationsFS, 2); err != nil {
		t.Fatalf("MigrateTo(2) failed: %v", err)
	}

	// Verify version
	version, _, err = db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}
	if version != 2 {
		t.Errorf("Expected version 2, got %d", version)
	}
}

// TestMigrateForce_SetVersion tests MigrateForce to set specific version
func TestMigrateForce_SetVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply some migrations first
	if err := db.MigrateTo(migrationsFS, 2); err != nil {
		t.Fatalf("MigrateTo(2) failed: %v", err)
	}

	// Force version to 5
	if err := db.MigrateForce(migrationsFS, 5); err != nil {
		t.Fatalf("MigrateForce(5) failed: %v", err)
	}

	// Verify version
	version, _, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}
	if version != 5 {
		t.Errorf("Expected version 5, got %d", version)
	}
}

// TestMigrateDown_FromVersion3 tests MigrateDown from version 3
func TestMigrateDown_FromVersion3(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply migrations to version 3
	if err := db.MigrateTo(migrationsFS, 3); err != nil {
		t.Fatalf("MigrateTo(3) failed: %v", err)
	}

	// Roll back one migration
	if err := db.MigrateDown(migrationsFS); err != nil {
		t.Fatalf("MigrateDown failed: %v", err)
	}

	// Verify version is now 2
	version, _, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}
	if version != 2 {
		t.Errorf("Expected version 2 after rollback, got %d", version)
	}
}

// TestNewDBWithMigrationCheck_Disabled tests NewDBWithMigrationCheck with check disabled
func TestNewDBWithMigrationCheck_Disabled(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create DB with migration check disabled
	db, err := NewDBWithMigrationCheck(dbPath, true)
	if err != nil {
		t.Fatalf("NewDBWithMigrationCheck failed: %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Fatal("Expected non-nil database")
	}
}

// TestOpenDB_ExistingDB tests OpenDB with an existing database file
func TestOpenDB_ExistingDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create database first
	db1, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("First OpenDB failed: %v", err)
	}
	db1.Close()

	// Open again
	db2, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Second OpenDB failed: %v", err)
	}
	defer db2.Close()

	if db2 == nil {
		t.Fatal("Expected non-nil database")
	}
}
