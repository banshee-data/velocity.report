package db

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// testTemplatePath is a database built once by NewDB in TestMain. Tests that
// need a ready, current-schema database copy it instead of building their own:
// a fresh NewDB runs schema.sql and baselines the migration table, which costs
// about six times as much as copying the file and opening the copy (257 ms
// against 41 ms per database under -race). Tests of NewDB's own
// initialisation, of migrations or of schema detection still build theirs.
var testTemplatePath string

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	dir, err := os.MkdirTemp("", "velocity-db-template-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create test template directory: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "template.db")
	template, err := NewDB(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build test template database: %v\n", err)
		return 1
	}
	// Fold the WAL into the main file so a copy of that one file is complete.
	if _, err := template.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = template.Close()
		fmt.Fprintf(os.Stderr, "checkpoint test template database: %v\n", err)
		return 1
	}
	if err := template.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close test template database: %v\n", err)
		return 1
	}

	testTemplatePath = path
	return m.Run()
}

// copyTemplateDB writes a copy of the template database to path, which the
// test then opens itself, usually with NewDB.
func copyTemplateDB(t testing.TB, path string) {
	t.Helper()
	if testTemplatePath == "" {
		t.Fatal("test template database not built")
	}
	if err := copyTemplateFile(testTemplatePath, path); err != nil {
		t.Fatalf("copy test template database: %v", err)
	}
}

func copyTemplateFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
