package server

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// newWALTestDB creates a WAL-mode database with a small table and returns its
// path plus an open writer connection. If keepOpen is false the connection is
// checkpointed and closed; if true it is left open (the live-service case) and
// the caller must close it.
func newWALTestDB(t *testing.T, keepOpen bool) (string, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sensor_data.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT, val REAL)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO widgets (name, val) VALUES ('alpha', 1.5), ('beta', 2.0), (NULL, 3.0)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if keepOpen {
		return path, db
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return path, nil
}

func runSQLCapture(args []string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = runSQL(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRunSQLReadsRows(t *testing.T) {
	path, _ := newWALTestDB(t, false)
	code, out, errs := runSQLCapture([]string{"--db-path", path, "SELECT name, val FROM widgets ORDER BY val"})
	if code != 0 {
		t.Fatalf("exit %d, stderr=%q", code, errs)
	}
	want := "name\tval\nalpha\t1.5\nbeta\t2\nNULL\t3\n"
	if out != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", out, want)
	}
}

func TestRunSQLReadsLiveWAL(t *testing.T) {
	// The service holds the database open in WAL mode; the read-only inspector
	// must still read committed rows.
	path, writer := newWALTestDB(t, true)
	defer writer.Close()

	code, out, errs := runSQLCapture([]string{"--db-path", path, "SELECT count(*) FROM widgets"})
	if code != 0 {
		t.Fatalf("exit %d, stderr=%q", code, errs)
	}
	if got := strings.TrimSpace(out); got != "count(*)\n3" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunSQLRefusesWrites(t *testing.T) {
	path, _ := newWALTestDB(t, false)
	code, _, errs := runSQLCapture([]string{"--db-path", path, "INSERT INTO widgets (name) VALUES ('gamma')"})
	if code == 0 {
		t.Fatal("expected non-zero exit for a write against a read-only database")
	}
	if !strings.Contains(errs, "data sql:") {
		t.Fatalf("expected an error message, got %q", errs)
	}

	// Confirm nothing was written.
	verify, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer verify.Close()
	var n int
	if err := verify.QueryRow("SELECT count(*) FROM widgets").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("row count changed to %d; write was not refused", n)
	}
}

func TestRunSQLLimitTruncates(t *testing.T) {
	path, _ := newWALTestDB(t, false)
	code, out, errs := runSQLCapture([]string{"--db-path", path, "--limit", "1", "SELECT id FROM widgets ORDER BY id"})
	if code != 0 {
		t.Fatalf("exit %d, stderr=%q", code, errs)
	}
	// Header + exactly one data row.
	lines := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if lines != 2 {
		t.Fatalf("expected header + 1 row, got %d lines: %q", lines, out)
	}
	if !strings.Contains(errs, "truncated at --limit=1") {
		t.Fatalf("expected truncation notice, got %q", errs)
	}
}

func TestRunSQLMissingDatabase(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.db")
	code, _, errs := runSQLCapture([]string{"--db-path", missing, "SELECT 1"})
	if code != 1 {
		t.Fatalf("expected exit 1 for missing database, got %d (stderr=%q)", code, errs)
	}
}

func TestRunSQLRejectsWritableMode(t *testing.T) {
	path, _ := newWALTestDB(t, false)
	code, _, errs := runSQLCapture([]string{"--db-path", path, "--read-only=false", "SELECT 1"})
	if code != 2 || !strings.Contains(errs, "only read-only access is supported") {
		t.Fatalf("expected exit 2 with read-only message, got %d %q", code, errs)
	}
}

func TestRunSQLEmptyQuery(t *testing.T) {
	path, _ := newWALTestDB(t, false)
	code, _, _ := runSQLCapture([]string{"--db-path", path})
	if code != 2 {
		t.Fatalf("expected exit 2 for an empty query, got %d", code)
	}
}
