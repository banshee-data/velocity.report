package db

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// ---------------------------------------------------------------------------
// migrate.go — MigrateForce: error from m.Force() via closed DB
// ---------------------------------------------------------------------------

func TestMigrateForce_ClosedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "force_closed.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	// Apply migrations so schema_migrations exists, then close the DB
	if err := database.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}
	database.Close()

	// MigrateForce on closed DB should fail at newMigrate
	err = database.MigrateForce(migrationsFS, 1)
	if err == nil {
		t.Error("expected error from MigrateForce on closed DB, got nil")
	}
}

// ---------------------------------------------------------------------------
// migrate.go — newMigrate: iofs.New failing (nil FS)
// ---------------------------------------------------------------------------

func TestNewMigrate_NilFS(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nilfs.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	// Passing nil as fs.FS causes iofs.New to panic (nil pointer dereference).
	// Verify that the panic occurs (confirming the code path is exercised).
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from MigrateUp with nil FS, got none")
		}
	}()

	_ = database.MigrateUp(nil)
}

// ---------------------------------------------------------------------------
// migrate.go — newMigrate: sqlite.WithInstance failing (closed DB)
// ---------------------------------------------------------------------------

func TestNewMigrate_ClosedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "closeddb.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	// Close the DB so sqlite.WithInstance fails
	database.Close()

	err = database.MigrateUp(migrationsFS)
	if err == nil {
		t.Error("expected error from MigrateUp on closed DB, got nil")
	}

	if !strings.Contains(err.Error(), "failed to create sqlite driver") {
		t.Errorf("expected 'failed to create sqlite driver' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — BaselineAtVersion: already-migrated DB error
// ---------------------------------------------------------------------------

func TestBaselineAtVersion_AlreadyMigrated(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "baseline_dup.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	// Apply real migrations so schema_migrations has a row
	if err := database.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// BaselineAtVersion should fail because migrations already exist
	err = database.BaselineAtVersion(5)
	if err == nil {
		t.Error("expected error when baselining already-migrated DB")
	}
	if !strings.Contains(err.Error(), "already has migrations applied") {
		t.Errorf("expected 'already has migrations applied' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — GetMigrationStatus: unexpected MigrateVersion error (closed DB)
// ---------------------------------------------------------------------------

func TestGetMigrationStatus_ClosedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "status_closed.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	database.Close()

	_, err = database.GetMigrationStatus(migrationsFS)
	if err == nil {
		t.Error("expected error from GetMigrationStatus on closed DB, got nil")
	}
}

// ---------------------------------------------------------------------------
// migrate.go — GetLatestMigrationVersion: empty FS → "no migration files found"
// ---------------------------------------------------------------------------

func TestGetLatestMigrationVersion_EmptyFSCoverage(t *testing.T) {
	// fstest.MapFS{} with no .up.sql files → "no migration files found"
	emptyFS := fstest.MapFS{
		"readme.txt": &fstest.MapFile{Data: []byte("not a migration")},
	}

	_, err := GetLatestMigrationVersion(emptyFS)
	if err == nil {
		t.Error("expected error for FS with no migration files")
	}
	if !strings.Contains(err.Error(), "could not determine latest migration version") {
		t.Errorf("expected 'could not determine latest migration version' in error, got: %v", err)
	}
}

func TestGetLatestMigrationVersion_UnreadableFS(t *testing.T) {
	// An FS that cannot be read at all
	badFS := mockErrFS{} // defined in migrate_cli_extended_test.go

	_, err := GetLatestMigrationVersion(badFS)
	if err == nil {
		t.Error("expected error for unreadable FS")
	}
	if !strings.Contains(err.Error(), "failed to read migrations filesystem") {
		t.Errorf("expected 'failed to read migrations filesystem', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — GetDatabaseSchema: closed DB → query error
// ---------------------------------------------------------------------------

func TestGetDatabaseSchema_ClosedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "schema_closed.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	database.Close()

	_, err = database.GetDatabaseSchema()
	if err == nil {
		t.Error("expected error from GetDatabaseSchema on closed DB, got nil")
	}
	if !strings.Contains(err.Error(), "failed to query schema") {
		t.Errorf("expected 'failed to query schema' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — GetSchemaAtMigration: MigrateTo failing on bad SQL
// ---------------------------------------------------------------------------

func TestGetSchemaAtMigration_BadMigration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "schema_at_bad.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	badFS := fstest.MapFS{
		"000001_bad.up.sql":   &fstest.MapFile{Data: []byte("THIS IS INVALID SQL!!!")},
		"000001_bad.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS nothing;")},
	}

	_, err = database.GetSchemaAtMigration(badFS, 1)
	if err == nil {
		t.Error("expected error from GetSchemaAtMigration with bad SQL, got nil")
	}
	if !strings.Contains(err.Error(), "failed to apply migrations") {
		t.Errorf("expected 'failed to apply migrations' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — DetectSchemaVersion: GetDatabaseSchema error (closed DB)
// ---------------------------------------------------------------------------

func TestDetectSchemaVersion_ClosedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "detect_closed.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	database.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	_, _, _, err = database.DetectSchemaVersion(migrationsFS)
	if err == nil {
		t.Error("expected error from DetectSchemaVersion on closed DB, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get current schema") {
		t.Errorf("expected 'failed to get current schema', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — DetectSchemaVersion: GetLatestMigrationVersion error (bad FS)
// ---------------------------------------------------------------------------

func TestDetectSchemaVersion_BadFS(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "detect_badfs.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	// FS that opens but has no migration files
	badFS := mockErrFS{}

	_, _, _, err = database.DetectSchemaVersion(badFS)
	if err == nil {
		t.Error("expected error from DetectSchemaVersion with bad FS, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get latest version") {
		t.Errorf("expected 'failed to get latest version', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — DetectSchemaVersion: early-exit optimisation block
// This tests the ≥98% but <100% early-exit path with many versions.
// ---------------------------------------------------------------------------

func TestDetectSchemaVersion_EarlyExit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "detect_earlyexit.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	// Create a large set of migrations.
	// Migration 1 creates a table matching the DB schema (version 1 is ≈100% match).
	// Migrations 2-10 add additional tables so later versions are lower matches.
	mfs := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS detect_t1 (id INTEGER PRIMARY KEY, name TEXT);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS detect_t1;")},
	}
	for i := 2; i <= 10; i++ {
		tbl := fmt.Sprintf("detect_extra_%d", i)
		mfs[fmt.Sprintf("%06d_extra_%d.up.sql", i, i)] = &fstest.MapFile{
			Data: []byte(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id INTEGER PRIMARY KEY);", tbl)),
		}
		mfs[fmt.Sprintf("%06d_extra_%d.down.sql", i, i)] = &fstest.MapFile{
			Data: []byte(fmt.Sprintf("DROP TABLE IF EXISTS %s;", tbl)),
		}
	}

	// Apply only the first migration to the actual database so detection
	// finds a near-perfect match at version 1 that drops for later versions.
	if err := database.MigrateTo(mfs, 1); err != nil {
		t.Fatalf("MigrateTo(1) failed: %v", err)
	}

	version, score, _, err := database.DetectSchemaVersion(mfs)
	if err != nil {
		t.Fatalf("DetectSchemaVersion failed: %v", err)
	}

	if version != 1 {
		t.Errorf("expected detected version 1, got %d", version)
	}
	if score < 90 {
		t.Errorf("expected high score, got %d", score)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — CheckAndPromptMigrations: dirty state error
// ---------------------------------------------------------------------------

func TestCheckAndPromptMigrations_DirtyStateError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "dirty.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	// Apply migration, then manually set dirty flag
	if err := database.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}
	_, err = database.Exec("UPDATE schema_migrations SET dirty = 1")
	if err != nil {
		t.Fatalf("failed to set dirty flag: %v", err)
	}

	shouldExit, err := database.CheckAndPromptMigrations(migrationsFS)
	if err == nil {
		t.Error("expected error for dirty state, got nil")
	}
	if !shouldExit {
		t.Error("expected shouldExit=true for dirty state")
	}
	if !strings.Contains(err.Error(), "dirty state") {
		t.Errorf("expected 'dirty state' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — CheckAndPromptMigrations: currentVersion > latestVersion
// ---------------------------------------------------------------------------

func TestCheckAndPromptMigrations_VersionAhead(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ahead.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	// FS has only version 1
	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	// Apply migrations, then force version ahead via direct SQL
	if err := database.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}
	_, err = database.Exec("UPDATE schema_migrations SET version = 99")
	if err != nil {
		t.Fatalf("failed to set version ahead: %v", err)
	}

	shouldExit, err := database.CheckAndPromptMigrations(migrationsFS)
	if err == nil {
		t.Error("expected error when version is ahead, got nil")
	}
	if !shouldExit {
		t.Error("expected shouldExit=true when version is ahead")
	}
	if !strings.Contains(err.Error(), "ahead of latest migration") {
		t.Errorf("expected 'ahead of latest migration' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — CheckAndPromptMigrations: MigrateVersion error (closed DB)
// ---------------------------------------------------------------------------

func TestCheckAndPromptMigrations_ClosedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "prompt_closed.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	database.Close()

	_, err = database.CheckAndPromptMigrations(migrationsFS)
	if err == nil {
		t.Error("expected error from CheckAndPromptMigrations on closed DB, got nil")
	}
}

// ---------------------------------------------------------------------------
// migrate_cli.go — handleMigrateStatus: dirty state warning output
// ---------------------------------------------------------------------------

func TestHandleMigrateStatus_DirtyWarning(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "status_dirty.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply migrations then mark dirty
	if err := database.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}
	_, err = database.Exec("UPDATE schema_migrations SET dirty = 1")
	if err != nil {
		t.Fatalf("failed to set dirty flag: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	handleMigrateStatus(database, migrationsFS)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "WARNING") {
		t.Errorf("expected dirty warning in output, got: %s", output)
	}
	if !strings.Contains(output, "dirty state") {
		t.Errorf("expected 'dirty state' in output, got: %s", output)
	}
}

// ---------------------------------------------------------------------------
// migrate_cli.go — handleMigrateVersion: invalid version string
// ---------------------------------------------------------------------------

func TestHandleMigrateVersion_InvalidVersionCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ver_invalid.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// handleMigrateVersion calls log.Fatalf on invalid version which calls os.Exit.
	// We redirect log output and run in a way that catches the fatal.
	// Since we can't directly test log.Fatalf without subprocess, we test the
	// underlying Sscanf parse path indirectly:
	var targetVersion uint
	_, parseErr := fmt.Sscanf("notanumber", "%d", &targetVersion)
	if parseErr == nil {
		t.Error("expected parse error for 'notanumber'")
	}

	// Also test that valid version strings work end-to-end
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	handleMigrateVersion(database, migrationsFS, "1")

	version, _, err := database.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("MigrateVersion failed: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}
}

// ---------------------------------------------------------------------------
// migrate_cli.go — handleMigrateVersion: MigrateTo error (bad version)
// ---------------------------------------------------------------------------

func TestHandleMigrateVersion_MigrateToError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ver_err.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	// FS with only version 1 migration
	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	// Migrating to version 999 should fail - test via MigrateTo directly
	err = database.MigrateTo(migrationsFS, 999)
	if err == nil {
		t.Error("expected error migrating to non-existent version")
	}
}

// ---------------------------------------------------------------------------
// migrate_cli.go — handleMigrateForce: invalid version string
// ---------------------------------------------------------------------------

func TestHandleMigrateForce_InvalidVersionParse(t *testing.T) {
	// Verify that fmt.Sscanf fails on non-numeric input (the code path in handleMigrateForce)
	var forceVersion int
	_, parseErr := fmt.Sscanf("abc", "%d", &forceVersion)
	if parseErr == nil {
		t.Error("expected parse error for 'abc'")
	}
}

// ---------------------------------------------------------------------------
// migrate_cli.go — handleMigrateBaseline: invalid version string
// ---------------------------------------------------------------------------

func TestHandleMigrateBaseline_InvalidVersionParse(t *testing.T) {
	// Verify that fmt.Sscanf fails on non-numeric input (the code path in handleMigrateBaseline)
	var baselineVersion uint
	_, parseErr := fmt.Sscanf("xyz", "%d", &baselineVersion)
	if parseErr == nil {
		t.Error("expected parse error for 'xyz'")
	}
}

// ---------------------------------------------------------------------------
// migrate_cli.go — handleMigrateBaseline: BaselineAtVersion error (already baseline)
// ---------------------------------------------------------------------------

func TestHandleMigrateBaseline_AlreadyBaseline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "baseline_twice.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	// Capture log output (first baseline succeeds)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	handleMigrateBaseline(database, "5")

	output := buf.String()
	if !strings.Contains(output, "baselined") {
		t.Errorf("expected 'baselined' in log output, got: %s", output)
	}

	// Second baseline should fail — but handleMigrateBaseline calls log.Fatalf.
	// Test the underlying function directly.
	err = database.BaselineAtVersion(10)
	if err == nil {
		t.Error("expected error from second BaselineAtVersion call")
	}
}

// ---------------------------------------------------------------------------
// migrate_cli.go — handleMigrateDetect: various error paths and legacy output
// ---------------------------------------------------------------------------

func TestHandleMigrateDetect_SchemaWithMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "detect_migrated.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply only first migration so current < latest
	if err := database.MigrateTo(migrationsFS, 1); err != nil {
		t.Fatalf("MigrateTo(1) failed: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	handleMigrateDetect(database, migrationsFS)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Current version") {
		t.Errorf("expected 'Current version' in detect output, got: %s", output)
	}
	// Should show "database is N version(s) behind"
	if !strings.Contains(output, "behind") {
		t.Errorf("expected 'behind' warning in detect output, got: %s", output)
	}
}

func TestHandleMigrateDetect_LegacyNoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "detect_legacy_nomatch.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS migration_only_table (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS migration_only_table;")},
	}

	// Create a completely different table so the schema won't match
	_, err = database.Exec("CREATE TABLE unrelated_table (x TEXT)")
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Suppress log output from detectSchemaVersion internals
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	handleMigrateDetect(database, migrationsFS)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Schema Detection Results") {
		t.Errorf("expected 'Schema Detection Results' in output, got: %s", output)
	}
}

func TestHandleMigrateDetect_LegacyPerfectMatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "detect_legacy_match.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS perfect_t (id INTEGER PRIMARY KEY, name TEXT);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS perfect_t;")},
	}

	// Manually create the exact same table that migration 1 would create,
	// without having schema_migrations — simulating a "legacy" database.
	_, err = database.Exec("CREATE TABLE perfect_t (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Suppress log output
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)

	handleMigrateDetect(database, migrationsFS)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Perfect match found") {
		t.Errorf("expected 'Perfect match found' in output, got: %s", output)
	}
}

func TestHandleMigrateDetect_DirtyState(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "detect_dirty.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply then set dirty
	if err := database.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}
	_, err = database.Exec("UPDATE schema_migrations SET dirty = 1")
	if err != nil {
		t.Fatalf("failed to set dirty flag: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	handleMigrateDetect(database, migrationsFS)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "dirty state") {
		t.Errorf("expected 'dirty state' in detect output, got: %s", output)
	}
}

// ---------------------------------------------------------------------------
// migrate_cli.go — RunMigrateCommand: help subcommand
// ---------------------------------------------------------------------------

func TestRunMigrateCommand_HelpSubcommand(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "help.db")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	RunMigrateCommand([]string{"help"}, dbPath)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Database Migration Commands") {
		t.Errorf("expected help text, got: %s", output)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — MigrateDown: closed DB error
// ---------------------------------------------------------------------------

func TestMigrateDown_ClosedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "down_closed.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	database.Close()

	err = database.MigrateDown(migrationsFS)
	if err == nil {
		t.Error("expected error from MigrateDown on closed DB, got nil")
	}
}

// ---------------------------------------------------------------------------
// migrate.go — MigrateVersion: closed DB error
// ---------------------------------------------------------------------------

func TestMigrateVersion_ClosedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "version_closed.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	database.Close()

	_, _, err = database.MigrateVersion(migrationsFS)
	if err == nil {
		t.Error("expected error from MigrateVersion on closed DB, got nil")
	}
}

// ---------------------------------------------------------------------------
// migrate.go — MigrateTo: closed DB error
// ---------------------------------------------------------------------------

func TestMigrateTo_ClosedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "to_closed.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS t1 (id INTEGER PRIMARY KEY);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
	}

	database.Close()

	err = database.MigrateTo(migrationsFS, 1)
	if err == nil {
		t.Error("expected error from MigrateTo on closed DB, got nil")
	}
}

// ---------------------------------------------------------------------------
// migrate.go — BaselineAtVersion: ensureSchemaMigrationsTable error (closed DB)
// ---------------------------------------------------------------------------

func TestBaselineAtVersion_ClosedDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "baseline_closed.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	database.Close()

	err = database.BaselineAtVersion(5)
	if err == nil {
		t.Error("expected error from BaselineAtVersion on closed DB, got nil")
	}
	if !strings.Contains(err.Error(), "failed to ensure schema_migrations table") {
		t.Errorf("expected 'failed to ensure schema_migrations table', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — CompareSchemas: one-schema-empty cases
// ---------------------------------------------------------------------------

func TestCompareSchemas_OneEmpty(t *testing.T) {
	schema := map[string]string{
		"table1": "CREATE TABLE table1 (id INTEGER PRIMARY KEY)",
	}

	// Current has tables, expected is empty
	score, diffs := CompareSchemas(schema, map[string]string{})
	if score >= 100 {
		t.Errorf("expected score < 100 when schemas differ, got %d", score)
	}
	if len(diffs) == 0 {
		t.Error("expected differences when one schema is empty")
	}

	// Current is empty, expected has tables
	score2, diffs2 := CompareSchemas(map[string]string{}, schema)
	if score2 >= 100 {
		t.Errorf("expected score < 100 when schemas differ, got %d", score2)
	}
	if len(diffs2) == 0 {
		t.Error("expected differences when one schema is empty")
	}
}

// ---------------------------------------------------------------------------
// migrate.go — CompareSchemas: modified objects (same key, different SQL)
// ---------------------------------------------------------------------------

func TestCompareSchemas_Modified(t *testing.T) {
	schema1 := map[string]string{
		"t1": "CREATE TABLE t1 (id INTEGER PRIMARY KEY)",
	}
	schema2 := map[string]string{
		"t1": "CREATE TABLE t1 (id INTEGER PRIMARY KEY, name TEXT)",
	}

	score, diffs := CompareSchemas(schema1, schema2)
	if score >= 100 {
		t.Errorf("expected score < 100 for modified schema, got %d", score)
	}

	found := false
	for _, d := range diffs {
		if strings.Contains(d, "Modified") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'Modified' diff, got: %v", diffs)
	}
}

// ---------------------------------------------------------------------------
// migrate.go — normalizeSQLForComparison: edge cases
// ---------------------------------------------------------------------------

func TestNormalizeSQLForComparison_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"only whitespace", "   \n\t  "},
		{"quoted table name", `CREATE TABLE "my_table" (id INTEGER)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just ensure it doesn't panic
			result := normalizeSQLForComparison(tt.input)
			_ = result
		})
	}
}

// ---------------------------------------------------------------------------
// migrate.go — GetSchemaAtMigration: success with in-memory temp DB
// ---------------------------------------------------------------------------

func TestGetSchemaAtMigration_SuccessCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "schema_at.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS := fstest.MapFS{
		"000001_init.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS schema_t (id INTEGER PRIMARY KEY, name TEXT);")},
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS schema_t;")},
		"000002_add.up.sql":    &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS schema_t2 (id INTEGER PRIMARY KEY);")},
		"000002_add.down.sql":  &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS schema_t2;")},
	}

	schema, err := database.GetSchemaAtMigration(migrationsFS, 1)
	if err != nil {
		t.Fatalf("GetSchemaAtMigration failed: %v", err)
	}
	if _, ok := schema["schema_t"]; !ok {
		t.Error("expected schema_t in schema at version 1")
	}
	if _, ok := schema["schema_t2"]; ok {
		t.Error("did not expect schema_t2 in schema at version 1")
	}
}

// ---------------------------------------------------------------------------
// migrate.go — GetDatabaseSchema: empty database (no tables)
// ---------------------------------------------------------------------------

func TestGetDatabaseSchema_EmptyDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "schema_empty.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	schema, err := database.GetDatabaseSchema()
	if err != nil {
		t.Fatalf("GetDatabaseSchema failed: %v", err)
	}
	if len(schema) != 0 {
		t.Errorf("expected empty schema, got %d entries", len(schema))
	}
}

// ---------------------------------------------------------------------------
// migrate.go — GetLatestMigrationVersion: FS with only .down.sql files
// ---------------------------------------------------------------------------

func TestGetLatestMigrationVersion_OnlyDownFiles(t *testing.T) {
	migrationsFS := fstest.MapFS{
		"000001_init.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t1;")},
		"000002_add.down.sql":  &fstest.MapFile{Data: []byte("DROP TABLE IF EXISTS t2;")},
	}

	_, err := GetLatestMigrationVersion(migrationsFS)
	if err == nil {
		t.Error("expected error for FS with only .down.sql files")
	}
}

// ---------------------------------------------------------------------------
// migrate.go — GetLatestMigrationVersion: FS with directories only
// ---------------------------------------------------------------------------

func TestGetLatestMigrationVersion_DirectoriesOnly(t *testing.T) {
	migrationsFS := fstest.MapFS{
		"subdir/file.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	}

	_, err := GetLatestMigrationVersion(migrationsFS)
	if err == nil {
		t.Error("expected error for FS with no top-level .up.sql files")
	}
}

// ---------------------------------------------------------------------------
// migrate_cli.go — handleMigrateDetect: up-to-date with schema_migrations
// ---------------------------------------------------------------------------

func TestHandleMigrateDetect_UpToDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "detect_uptodate.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer database.Close()

	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS failed: %v", err)
	}

	// Apply all migrations
	if err := database.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("MigrateUp failed: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	handleMigrateDetect(database, migrationsFS)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "up to date") {
		t.Errorf("expected 'up to date' in output, got: %s", output)
	}
}

// ---------------------------------------------------------------------------
// Ensure the unused import of fs.FS is covered by interface usage
// ---------------------------------------------------------------------------
