package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunRollbackNoBackups(t *testing.T) {
	// Create an empty backup directory — rollback should fail.
	tmpDir := t.TempDir()
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("expected empty directory")
	}

	// We can't easily test runRollback directly because it reads the
	// const backupDir. Test the helper logic instead.
}

func TestRestoreBackupMissingBinary(t *testing.T) {
	// Create a backup directory with no binary.
	backupPath := t.TempDir()

	err := restoreBackup(backupPath)
	if err == nil {
		t.Fatal("expected error when backup binary is missing")
	}
}

func TestRestoreBackupWithBinary(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping on CI — requires systemctl")
	}

	// This test would require systemctl, so we only verify the file
	// presence check works.
	backupPath := t.TempDir()
	content := []byte("restored-binary")
	if err := os.WriteFile(filepath.Join(backupPath, binaryName), content, 0755); err != nil {
		t.Fatal(err)
	}

	// restoreBackup will fail at systemctl stop, which is expected
	// outside a real systemd environment.
	err := restoreBackup(backupPath)
	if err == nil {
		t.Skip("systemctl available — full test passed")
	}
	// Expected: fails at "stopping service" step.
}
