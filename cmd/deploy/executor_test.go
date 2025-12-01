package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExecutor_IsLocal(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{"localhost", "localhost", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"empty", "", true},
		{"remote IP", "192.168.1.100", false},
		{"remote hostname", "pi.local", false},
		{"remote user@host", "pi@192.168.1.100", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewExecutor(tt.target, "", "", "", false)
			if got := exec.IsLocal(); got != tt.want {
				t.Errorf("IsLocal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExecutor_Run_DryRun(t *testing.T) {
	exec := NewExecutor("localhost", "", "", "", true)

	// Should not error in dry-run mode
	output, err := exec.Run("echo test")
	if err != nil {
		t.Errorf("Run() in dry-run mode should not error: %v", err)
	}

	// Output should be empty in dry-run
	if output != "" {
		t.Errorf("Run() in dry-run mode should return empty output, got: %s", output)
	}
}

func TestExecutor_Run_Local(t *testing.T) {
	exec := NewExecutor("localhost", "", "", "", false)

	output, err := exec.Run("echo test")
	if err != nil {
		t.Errorf("Run() failed: %v", err)
	}

	if !strings.Contains(output, "test") {
		t.Errorf("Run() output does not contain 'test': %s", output)
	}
}

func TestExecutor_WriteFile_Local(t *testing.T) {
	exec := NewExecutor("localhost", "", "", "", false)

	// Create temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "test content\n"

	err := exec.WriteFile(testFile, testContent)
	if err != nil {
		t.Errorf("WriteFile() failed: %v", err)
	}

	// Verify file was written
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Errorf("Failed to read written file: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("File content = %q, want %q", string(content), testContent)
	}
}

func TestExecutor_CopyFile_Local(t *testing.T) {
	// Skip this test on systems where temp directories require sudo (e.g., macOS with /var/folders/...)
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping test on macOS where temp directories typically require sudo")
	}

	exec := NewExecutor("localhost", "", "", "", false)
	tmpDir := t.TempDir()

	// Create source file
	srcFile := filepath.Join(tmpDir, "source.txt")
	srcContent := "source content\n"
	if err := os.WriteFile(srcFile, []byte(srcContent), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Copy to destination
	dstFile := filepath.Join(tmpDir, "dest.txt")
	err := exec.CopyFile(srcFile, dstFile)
	if err != nil {
		t.Errorf("CopyFile() failed: %v", err)
	}

	// Verify destination
	dstContent, err := os.ReadFile(dstFile)
	if err != nil {
		t.Errorf("Failed to read destination file: %v", err)
	}

	if string(dstContent) != srcContent {
		t.Errorf("Destination content = %q, want %q", string(dstContent), srcContent)
	}
}

func TestExecutor_RunSudo_DryRun(t *testing.T) {
	exec := NewExecutor("localhost", "", "", "", true)

	// Should not error in dry-run mode
	output, err := exec.RunSudo("systemctl status test")
	if err != nil {
		t.Errorf("RunSudo() in dry-run mode should not error: %v", err)
	}

	// Output should be empty in dry-run
	if output != "" {
		t.Errorf("RunSudo() in dry-run mode should return empty output, got: %s", output)
	}
}
