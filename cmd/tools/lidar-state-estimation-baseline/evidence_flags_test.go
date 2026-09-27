package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEvidenceFlags(t *testing.T) {
	for _, c := range []struct {
		name         string
		samplePoints int
		perCase      bool
		discard      bool
		evidenceDir  string
		want         string
	}{
		{"defaults", 256, false, false, "", ""},
		{"per case, kept", 1024, true, false, "/srv/evidence", ""},
		{"per case, discarded", 16, true, true, "/srv/evidence", ""},
		{"no sample", 0, false, false, "", "-sample-points"},
		{"over the cap", 1025, false, false, "", "-sample-points"},
		{"per case without a directory", 256, true, false, "", "-evidence-dir"},
		{"discard without per case", 256, false, true, "/srv/evidence", "-evidence-per-case"},
	} {
		err := validateEvidenceFlags(c.samplePoints, c.perCase, c.discard, c.evidenceDir)
		if c.want == "" && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: error %v, want one naming %s", c.name, err, c.want)
		}
	}
}

func TestRemoveDatabaseTakesItsWriteAheadFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marina-webster-beach.db")
	for _, p := range []string{path, path + "-wal", path + "-shm", filepath.Join(dir, "columbus-broadway.db")} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeDatabase(path); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "columbus-broadway.db" {
		t.Fatalf("left %v; want only the other case's database", entries)
	}
	// Already gone is not an error: a case with no evidence written.
	if err := removeDatabase(path); err != nil {
		t.Fatal(err)
	}
}

// A missing evidence database is an error, and is not created: a fresh one
// would read as a replay that filed no solid bodies.
func TestSummariseSolidBodiesRefusesAMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := summariseSolidBodies(path, "source/v1/x"); err == nil || !strings.Contains(err.Error(), "evidence database") {
		t.Fatalf("missing database: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a missing evidence database was created: %v", err)
	}
}
