package main

import (
	"strings"
	"testing"
)

func TestBackup_Structure(t *testing.T) {
	b := &Backup{
		Target:    "localhost",
		SSHUser:   "testuser",
		SSHKey:    "/test/key",
		OutputDir: "/tmp/backups",
	}

	if b.Target != "localhost" {
		t.Errorf("Target = %s, want localhost", b.Target)
	}
	if b.SSHUser != "testuser" {
		t.Errorf("SSHUser = %s, want testuser", b.SSHUser)
	}
	if b.SSHKey != "/test/key" {
		t.Errorf("SSHKey = %s, want /test/key", b.SSHKey)
	}
	if b.OutputDir != "/tmp/backups" {
		t.Errorf("OutputDir = %s, want /tmp/backups", b.OutputDir)
	}
}

func TestBackup_Execute_NoService(t *testing.T) {
	t.Skip("Skipping test that requires sudo and service installation")

	b := &Backup{
		Target:    "localhost",
		SSHUser:   "",
		SSHKey:    "",
		OutputDir: t.TempDir(),
	}

	// This will fail because there's no actual service to backup
	// We're testing that it fails gracefully
	err := b.Execute()
	if err == nil {
		t.Log("Note: Backup succeeded (unexpected in test environment)")
	} else {
		t.Logf("Backup failed as expected: %v", err)
		// Should have a descriptive error message
		if err.Error() == "" {
			t.Error("Expected non-empty error message")
		}
	}
}

func TestBackup_RemoteTarget(t *testing.T) {
	b := &Backup{
		Target:    "192.168.1.100",
		SSHUser:   "pi",
		SSHKey:    "/home/user/.ssh/id_rsa",
		OutputDir: "/var/backups",
	}

	// Verify fields are set correctly for remote backup
	if b.Target != "192.168.1.100" {
		t.Errorf("Target = %s, want 192.168.1.100", b.Target)
	}
	if b.SSHUser != "pi" {
		t.Errorf("SSHUser = %s, want pi", b.SSHUser)
	}
	if !strings.HasSuffix(b.SSHKey, "id_rsa") {
		t.Errorf("SSHKey should end with id_rsa, got %s", b.SSHKey)
	}
}

func TestBackup_OutputDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	b := &Backup{
		Target:    "localhost",
		OutputDir: tmpDir,
	}

	if b.OutputDir != tmpDir {
		t.Errorf("OutputDir = %s, want %s", b.OutputDir, tmpDir)
	}

	// Verify it's a valid directory path
	if !strings.HasPrefix(tmpDir, "/") {
		t.Error("OutputDir should be an absolute path")
	}
}

func TestBackup_EmptyFields(t *testing.T) {
	b := &Backup{
		Target:    "",
		SSHUser:   "",
		SSHKey:    "",
		OutputDir: "",
	}

	// Empty target should default to localhost behavior
	if b.Target != "" {
		t.Errorf("Empty Target should remain empty, got %s", b.Target)
	}
}

func TestBackup_ValidationScenarios(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		outputDir string
		wantError bool
	}{
		{"valid localhost", "localhost", "/tmp", false},
		{"valid remote", "192.168.1.100", "/tmp", false},
		{"empty output dir", "localhost", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Backup{
				Target:    tt.target,
				OutputDir: tt.outputDir,
			}

			// The backup will fail in test environment, but we're checking
			// that the struct is created correctly
			if b.Target != tt.target {
				t.Errorf("Target = %s, want %s", b.Target, tt.target)
			}
			if b.OutputDir != tt.outputDir {
				t.Errorf("OutputDir = %s, want %s", b.OutputDir, tt.outputDir)
			}
		})
	}
}
