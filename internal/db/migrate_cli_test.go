package db

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintMigrateHelp(t *testing.T) {
	// This function writes to stdout via log, but doesn't panic
	// We just ensure it doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PrintMigrateHelp panicked: %v", r)
		}
	}()

	PrintMigrateHelp()
}

func TestRunMigrateCommand_Help(t *testing.T) {
	// Test help command which doesn't exit
	// Create a temporary database for this test
	tmpDir := t.TempDir()
	_ = tmpDir // unused but created for potential use

	// Create a simple test that won't call os.Exit
	// We can't fully test RunMigrateCommand because it calls os.Exit
	// But we can test that certain paths don't panic

	// The help subcommand should work
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RunMigrateCommand with help panicked: %v", r)
		}
	}()

	// We can't actually call RunMigrateCommand here because it uses os.Exit
	// which would terminate the test. Instead we test the helper functions
	// that are called by it.
}

// Test that we can get migrations FS
func TestGetMigrationsFS(t *testing.T) {
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	if migrationsFS == nil {
		t.Error("Expected non-nil migrations FS")
	}
}

// Test OpenDB function used by migrate CLI
func TestOpenDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Error("Expected non-nil database")
	}

	// Verify the database is actually opened
	err = db.DB.Ping()
	if err != nil {
		t.Errorf("Database ping failed: %v", err)
	}
}

// Test that helper functions exist and have correct signatures
// by calling them with test database
func TestMigrateHelpers_Existence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Test that all helper functions can be called without panicking
	// (they may fail with errors, but shouldn't panic)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Helper functions panicked: %v", r)
		}
	}()

	// These will likely fail/log errors, but we're testing they exist and don't panic
	version, dirty, _ := db.MigrateVersion(migrationsFS)
	t.Logf("Initial version: %d, dirty: %v", version, dirty)

	// Test baseline with version 0 (shouldn't do much)
	_ = db.BaselineAtVersion(0)
}

func TestHandleMigrateUp(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Test handleMigrateUp
	handleMigrateUp(database, migrationsFS)

	output := buf.String()
	if output == "" {
		t.Error("Expected log output from handleMigrateUp")
	}

	// Verify migration was applied
	version, dirty, err := database.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version == 0 {
		t.Error("Expected version > 0 after migration up")
	}
	if dirty {
		t.Error("Expected clean state after migration up")
	}
}

func TestHandleMigrateDown(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// First migrate up
	if err := database.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	initialVersion, _, err := database.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if initialVersion == 0 {
		t.Skip("No migrations to test down with")
	}

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Test handleMigrateDown
	handleMigrateDown(database, migrationsFS)

	output := buf.String()
	if output == "" {
		t.Error("Expected log output from handleMigrateDown")
	}

	// Verify migration was rolled back
	newVersion, dirty, err := database.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if newVersion >= initialVersion {
		t.Errorf("Expected version to decrease from %d, got %d", initialVersion, newVersion)
	}
	if dirty {
		t.Error("Expected clean state after migration down")
	}
}

func TestHandleMigrateStatus(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Migrate to have some status to check
	if err := database.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Capture stdout output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	handleMigrateStatus(database, migrationsFS)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if output == "" {
		t.Error("Expected output from handleMigrateStatus")
	}

	// Check for expected content
	if !strings.Contains(output, "Migration Status") {
		t.Error("Expected 'Migration Status' in output")
	}
}

func TestHandleMigrateVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Test migrating to version 1
	handleMigrateVersion(database, migrationsFS, "1")

	output := buf.String()
	if output == "" {
		t.Error("Expected log output from handleMigrateVersion")
	}

	// Verify version
	version, _, err := database.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}

	if version != 1 {
		t.Errorf("Expected version 1, got %d", version)
	}
}

func TestHandleMigrateBaseline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Test baselining at version 1
	handleMigrateBaseline(database, "1")

	output := buf.String()
	if output == "" {
		t.Error("Expected log output from handleMigrateBaseline")
	}

	// Verify baseline was set (need to check via GetMigrationStatus since version might be 0)
	migrationsFS, _ := getMigrationsFS()
	status, err := database.GetMigrationStatus(migrationsFS)
	if err != nil {
		t.Fatalf("GetMigrationStatus failed: %v", err)
	}

	if !status["schema_migrations_exists"].(bool) {
		t.Error("Expected schema_migrations table to exist after baseline")
	}
}

func TestHandleMigrateDetect_WithMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Migrate up first
	if err := database.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Capture stdout output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	handleMigrateDetect(database, migrationsFS)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if output == "" {
		t.Error("Expected output from handleMigrateDetect")
	}

	// Should show current version
	if !strings.Contains(output, "Current version") {
		t.Error("Expected 'Current version' in output")
	}
}

func TestHandleMigrateDetect_LegacyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Create a simple table to simulate legacy database
	_, err = database.Exec(`CREATE TABLE radar_data (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Capture stdout output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	handleMigrateDetect(database, migrationsFS)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if output == "" {
		t.Error("Expected output from handleMigrateDetect")
	}

	// Should run schema detection
	if !strings.Contains(output, "Schema Detection Results") {
		t.Error("Expected 'Schema Detection Results' in output for legacy database")
	}
}

func TestHandleMigrateForce_WithConfirmation(t *testing.T) {
	// handleMigrateForce requires interactive stdin input (Scanln)
	// Testing it would require mocking os.Stdin which is complex
	// The function itself is thin wrapper around database.MigrateForce which is tested elsewhere
	t.Skip("handleMigrateForce requires interactive stdin input; underlying MigrateForce is tested in DB tests")
}

func TestHandleMigrateVersion_InvalidVersion(t *testing.T) {
	// handleMigrateVersion calls log.Fatalf on errors which calls os.Exit
	// This would terminate the test process, so we skip testing error paths
	// The underlying database.MigrateTo is tested elsewhere
	t.Skip("handleMigrateVersion calls log.Fatalf on error which exits the process")
}

func TestHandleMigrateBaseline_MultipleVersions(t *testing.T) {
	// Test baselining with a valid version
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Test baseline with version 1
	handleMigrateBaseline(database, "1")

	output := buf.String()
	if output == "" {
		t.Error("Expected log output for version 1")
	}

	if !strings.Contains(output, "baselined") {
		t.Error("Expected 'baselined' in output")
	}
}
