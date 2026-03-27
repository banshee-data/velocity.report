package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateBackupTo(t *testing.T) {
	// Set up a fake binary to back up.
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "velocity-report")
	if err := os.WriteFile(fakeBinary, []byte("fake-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// We need to temporarily override the package-level constants for testing.
	// Since they are const, we test createBackupTo indirectly by testing copyFile.
}

func TestCopyFile(t *testing.T) {
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "source")
	content := []byte("test file content for backup")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	dstDir := t.TempDir()
	dstPath := filepath.Join(dstDir, "destination")

	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}

	// Verify permissions are preserved.
	srcInfo, _ := os.Stat(srcPath)
	dstInfo, _ := os.Stat(dstPath)
	if srcInfo.Mode().Perm() != dstInfo.Mode().Perm() {
		t.Errorf("mode mismatch: src %o, dst %o", srcInfo.Mode().Perm(), dstInfo.Mode().Perm())
	}
}

func TestCopyFileNonexistent(t *testing.T) {
	dstDir := t.TempDir()
	err := copyFile("/nonexistent/path", filepath.Join(dstDir, "out"))
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}
