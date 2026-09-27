package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

func TestOpenReadOnlyReadsButRefusesWrites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	writable, err := dbpkg.NewDB(dbPath)
	if err != nil {
		t.Fatalf("create test DB: %v", err)
	}
	defer writable.Close()

	if _, err := writable.Exec(`CREATE TABLE probe (id INTEGER PRIMARY KEY, val TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := writable.Exec(`INSERT INTO probe (id, val) VALUES (1, 'hello')`); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	readOnly, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer readOnly.Close()

	var val string
	if err := readOnly.QueryRow(`SELECT val FROM probe WHERE id = 1`).Scan(&val); err != nil {
		t.Fatalf("read via read-only handle: %v", err)
	}
	if val != "hello" {
		t.Fatalf("val = %q, want %q", val, "hello")
	}

	if _, err := readOnly.Exec(`INSERT INTO probe (id, val) VALUES (2, 'nope')`); err == nil {
		t.Fatal("expected write through a read-only handle to fail")
	}
}

func TestBeginReadOnlyKeepsOneSnapshotAcrossQueries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	writable, err := dbpkg.NewDB(dbPath)
	if err != nil {
		t.Fatalf("create test DB: %v", err)
	}
	defer writable.Close()
	if _, err := writable.Exec(`CREATE TABLE probe (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := writable.Exec(`INSERT INTO probe (id) VALUES (1)`); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	reader, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer reader.Close()
	tx, err := BeginReadOnly(context.Background(), reader)
	if err != nil {
		t.Fatalf("BeginReadOnly: %v", err)
	}
	defer tx.Rollback()
	count := func() int {
		t.Helper()
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM probe`).Scan(&n); err != nil {
			t.Fatalf("count inside read transaction: %v", err)
		}
		return n
	}
	if got := count(); got != 1 {
		t.Fatalf("first read saw %d rows, want 1", got)
	}
	// A row committed by a writer after the first read belongs to a later
	// snapshot: a reader that mixed the two would invent evidence.
	if _, err := writable.Exec(`INSERT INTO probe (id) VALUES (2)`); err != nil {
		t.Fatalf("concurrent write: %v", err)
	}
	if got := count(); got != 1 {
		t.Fatalf("second read saw %d rows: the snapshot moved under the reader", got)
	}
	if _, err := tx.Exec(`INSERT INTO probe (id) VALUES (3)`); err == nil {
		t.Fatal("expected a write inside a read-only transaction to fail")
	}

	closed, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	if err := closed.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := BeginReadOnly(context.Background(), closed); err == nil {
		t.Fatal("expected a closed database to refuse a read transaction")
	}
}
