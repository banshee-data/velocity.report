package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFileWritesTheSameBytes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.asc")
	dst := filepath.Join(dir, "out", "site.asc")
	if err := os.WriteFile(src, []byte("# Exported points\n1 2 3 0 3.7 300\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copy: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Exported points\n1 2 3 0 3.7 300\n" {
		t.Fatalf("copied %q", got)
	}
	if err := copyFile(filepath.Join(dir, "missing.asc"), dst); err == nil {
		t.Fatal("a missing source must fail")
	}
}
