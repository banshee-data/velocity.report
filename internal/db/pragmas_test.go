package db

import (
	"os"
	"testing"
)

// TestPragmasApplied verifies that essential PRAGMAs are set on all databases
func TestPragmasApplied(t *testing.T) {
	testDB := t.TempDir() + "/test_pragmas.db"
	defer os.Remove(testDB)

	db, err := NewDB(testDB)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Verify journal_mode is WAL
	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("Failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("Expected journal_mode=wal, got %s", journalMode)
	}

	// Verify busy_timeout is 5000
	var busyTimeout int
	err = db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout)
	if err != nil {
		t.Fatalf("Failed to query busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("Expected busy_timeout=5000, got %d", busyTimeout)
	}

	// Verify synchronous is NORMAL (1)
	var synchronous int
	err = db.QueryRow("PRAGMA synchronous").Scan(&synchronous)
	if err != nil {
		t.Fatalf("Failed to query synchronous: %v", err)
	}
	if synchronous != 1 { // 1 = NORMAL
		t.Errorf("Expected synchronous=1 (NORMAL), got %d", synchronous)
	}

	// Verify temp_store is MEMORY (2)
	var tempStore int
	err = db.QueryRow("PRAGMA temp_store").Scan(&tempStore)
	if err != nil {
		t.Fatalf("Failed to query temp_store: %v", err)
	}
	if tempStore != 2 { // 2 = MEMORY
		t.Errorf("Expected temp_store=2 (MEMORY), got %d", tempStore)
	}
}

// TestPragmasAppliedToExistingDB verifies PRAGMAs are set when opening existing databases
func TestPragmasAppliedToExistingDB(t *testing.T) {
	testDB := t.TempDir() + "/test_pragmas_existing.db"
	defer os.Remove(testDB)

	// Create database
	db1, err := NewDB(testDB)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	db1.Close()

	// Reopen database - PRAGMAs should still be applied
	db2, err := NewDBWithMigrationCheck(testDB, false)
	if err != nil {
		t.Fatalf("Failed to reopen database: %v", err)
	}
	defer db2.Close()

	// Verify journal_mode is still WAL
	var journalMode string
	err = db2.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("Failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("Expected journal_mode=wal after reopening, got %s", journalMode)
	}
}
