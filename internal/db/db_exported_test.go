package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestApplyPragmasSetsRuntimeSettings(t *testing.T) {
	// ApplyPragmas is the exported wrapper the migrate tooling uses to bring a
	// bare *sql.DB up to the same runtime configuration NewDB applies.
	path := filepath.Join(t.TempDir(), "pragmas.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer conn.Close()

	if err := ApplyPragmas(conn); err != nil {
		t.Fatalf("ApplyPragmas: %v", err)
	}

	var journalMode string
	if err := conn.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("reading journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var foreignKeys int
	if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("reading foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1 (ON)", foreignKeys)
	}
}

func TestLatestMigrationVersionMatchesEmbeddedMigrations(t *testing.T) {
	// The convenience wrapper resolves the embedded migrations filesystem and
	// reports its highest version. A fresh database is baselined at exactly
	// this number, so a mismatch would break bootstrap.
	got, err := LatestMigrationVersion()
	if err != nil {
		t.Fatalf("LatestMigrationVersion: %v", err)
	}
	if got == 0 {
		t.Fatal("LatestMigrationVersion = 0, want the highest embedded migration")
	}

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS: %v", err)
	}
	want, err := GetLatestMigrationVersion(migrationsFS)
	if err != nil {
		t.Fatalf("GetLatestMigrationVersion: %v", err)
	}
	if got != want {
		t.Errorf("LatestMigrationVersion() = %d, want %d", got, want)
	}
}

func TestLatestMigrationVersionMatchesFreshDatabaseBaseline(t *testing.T) {
	latest, err := LatestMigrationVersion()
	if err != nil {
		t.Fatalf("LatestMigrationVersion: %v", err)
	}

	database, cleanup := NewTestDB(t)
	defer cleanup()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS: %v", err)
	}
	current, dirty, err := database.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion: %v", err)
	}
	if dirty {
		t.Error("fresh database reports a dirty migration state")
	}
	if current != latest {
		t.Errorf("fresh database is at version %d, want the latest migration %d", current, latest)
	}
}
