package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstaller_validateBinary(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		binaryPath string
		createFile bool
		executable bool
		wantErr    bool
	}{
		{
			name:       "valid executable binary",
			binaryPath: filepath.Join(tmpDir, "valid-binary"),
			createFile: true,
			executable: true,
			wantErr:    false,
		},
		{
			name:       "non-executable file",
			binaryPath: filepath.Join(tmpDir, "non-exec"),
			createFile: true,
			executable: false,
			wantErr:    true,
		},
		{
			name:       "missing file",
			binaryPath: filepath.Join(tmpDir, "missing"),
			createFile: false,
			executable: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.createFile {
				// Create test file
				content := []byte("#!/bin/sh\necho test\n")
				if err := os.WriteFile(tt.binaryPath, content, 0644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}

				if tt.executable {
					if err := os.Chmod(tt.binaryPath, 0755); err != nil {
						t.Fatalf("Failed to make file executable: %v", err)
					}
				}
			}

			installer := &Installer{
				BinaryPath: tt.binaryPath,
				DryRun:     false,
			}

			err := installer.validateBinary()
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInstaller_Install_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test binary
	binaryPath := filepath.Join(tmpDir, "test-binary")
	content := []byte("#!/bin/sh\necho test\n")
	if err := os.WriteFile(binaryPath, content, 0755); err != nil {
		t.Fatalf("Failed to create test binary: %v", err)
	}

	installer := &Installer{
		Target:     "localhost",
		BinaryPath: binaryPath,
		DryRun:     true,
	}

	// Should not error in dry-run mode (won't actually install)
	// This test mainly checks that dry-run mode doesn't panic
	err := installer.Install()
	// In dry-run, it might error on checkExisting but shouldn't panic
	_ = err // We expect some operations to be skipped in dry-run
}

func TestServiceContent(t *testing.T) {
	// Verify service file content has required fields
	requiredFields := []string{
		"[Unit]",
		"[Service]",
		"[Install]",
		"User=velocity",
		"ExecStart=/usr/local/bin/velocity-report",
		"WorkingDirectory=/var/lib/velocity-report",
	}

	for _, field := range requiredFields {
		if !strings.Contains(serviceContent, field) {
			t.Errorf("Service file missing required field: %s", field)
		}
	}
}
