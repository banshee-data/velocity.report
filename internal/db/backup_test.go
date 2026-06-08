package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBackupDatabaseCapturesWALData is the case a plain file copy gets wrong: a
// live WAL database with committed rows still in the -wal (auto-checkpoint
// disabled, writer held open). The VACUUM INTO backup must capture them.
func TestBackupDatabaseCapturesWALData(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "sensor_data.db")

	writer, err := sql.Open("sqlite", "file:"+src+"?_pragma=journal_mode(WAL)&_pragma=wal_autocheckpoint(0)")
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer writer.Close()
	if _, err := writer.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := writer.Exec(`INSERT INTO t (v) VALUES ('a'),('b'),('c')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	dst := filepath.Join(dir, "backup.db")
	if err := BackupDatabase(src, dst); err != nil {
		t.Fatalf("BackupDatabase: %v", err)
	}

	// The backup is a single self-contained file (no -wal/-shm siblings).
	if _, err := os.Stat(dst + "-wal"); !os.IsNotExist(err) {
		t.Errorf("backup should have no -wal sibling, stat err=%v", err)
	}

	// Read the backup standalone and confirm all committed rows are present.
	bk, err := sql.Open("sqlite", "file:"+dst+"?mode=ro")
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer bk.Close()
	var n int
	if err := bk.QueryRow(`SELECT count(*) FROM t`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Fatalf("backup row count = %d, want 3 (WAL data lost)", n)
	}
}

func TestBackupDatabaseOverwritesExistingDest(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.db")
	conn, err := sql.Open("sqlite", "file:"+src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`CREATE TABLE t (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()

	dst := filepath.Join(dir, "dest.db")
	if err := os.WriteFile(dst, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := BackupDatabase(src, dst); err != nil {
		t.Fatalf("BackupDatabase over existing dest: %v", err)
	}
}

func TestBackupDatabaseNonexistentSource(t *testing.T) {
	dir := t.TempDir()
	if err := BackupDatabase(filepath.Join(dir, "nope.db"), filepath.Join(dir, "out.db")); err == nil {
		t.Fatal("expected error backing up a non-existent database")
	}
}

// TestBackupDatabaseRejectsURLDelimiters guards against DSN injection: a source
// path with '?' or '#' must be refused before it can override the mode=ro DSN
// parameters (mirrors ReadOnlyQuery's guard).
func TestBackupDatabaseRejectsURLDelimiters(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{
		filepath.Join(dir, "x.db") + "?mode=rwc",
		filepath.Join(dir, "x.db") + "#frag",
	} {
		err := BackupDatabase(bad, filepath.Join(dir, "out.db"))
		if err == nil || !strings.Contains(err.Error(), "must not contain") {
			t.Fatalf("BackupDatabase(%q) = %v; want rejection for URL delimiter", bad, err)
		}
	}
}
