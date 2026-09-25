package sqlite

import (
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
