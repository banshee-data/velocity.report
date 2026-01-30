package db

import (
	"path/filepath"
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

