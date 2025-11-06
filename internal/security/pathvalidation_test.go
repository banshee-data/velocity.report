package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePathWithinDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directories for symlink tests
	safeDir := filepath.Join(tmpDir, "safe")
	unsafeDir := filepath.Join(tmpDir, "unsafe")
	if err := os.MkdirAll(safeDir, 0755); err != nil {
		t.Fatalf("Failed to create safe directory: %v", err)
	}
	if err := os.MkdirAll(unsafeDir, 0755); err != nil {
		t.Fatalf("Failed to create unsafe directory: %v", err)
	}

	// Create a file in the unsafe directory
	unsafeFile := filepath.Join(unsafeDir, "secret.txt")
	if err := os.WriteFile(unsafeFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("Failed to create unsafe file: %v", err)
	}

	// Create a symlink inside safe directory pointing to unsafe directory
	symlinkPath := filepath.Join(safeDir, "evil-symlink")
	if err := os.Symlink(unsafeDir, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	tests := []struct {
		name      string
		filePath  string
		safeDir   string
		wantError bool
	}{
		{
			name:      "valid path within directory",
			filePath:  filepath.Join(tmpDir, "file.txt"),
			safeDir:   tmpDir,
			wantError: false,
		},
		{
			name:      "valid nested path",
			filePath:  filepath.Join(tmpDir, "subdir", "file.txt"),
			safeDir:   tmpDir,
			wantError: false,
		},
		{
			name:      "path traversal with ..",
			filePath:  filepath.Join(tmpDir, "..", "file.txt"),
			safeDir:   tmpDir,
			wantError: true,
		},
		{
			name:      "path traversal at start",
			filePath:  "../../../etc/passwd",
			safeDir:   tmpDir,
			wantError: true,
		},
		{
			name:      "absolute path outside safe dir",
			filePath:  "/etc/passwd",
			safeDir:   tmpDir,
			wantError: true,
		},
		{
			name:      "symlink escape attack - following symlink to outside dir",
			filePath:  filepath.Join(symlinkPath, "secret.txt"),
			safeDir:   safeDir,
			wantError: true,
		},
		{
			name:      "symlink escape attack - accessing symlink directly",
			filePath:  symlinkPath,
			safeDir:   safeDir,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathWithinDirectory(tt.filePath, tt.safeDir)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidatePathWithinDirectory() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestValidatePathWithinAllowedDirs(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	tests := []struct {
		name        string
		filePath    string
		allowedDirs []string
		wantError   bool
	}{
		{
			name:        "valid path in first allowed dir",
			filePath:    filepath.Join(tmpDir1, "file.txt"),
			allowedDirs: []string{tmpDir1, tmpDir2},
			wantError:   false,
		},
		{
			name:        "valid path in second allowed dir",
			filePath:    filepath.Join(tmpDir2, "file.txt"),
			allowedDirs: []string{tmpDir1, tmpDir2},
			wantError:   false,
		},
		{
			name:        "invalid path outside all dirs",
			filePath:    "/etc/passwd",
			allowedDirs: []string{tmpDir1, tmpDir2},
			wantError:   true,
		},
		{
			name:        "no allowed directories",
			filePath:    filepath.Join(tmpDir1, "file.txt"),
			allowedDirs: []string{},
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathWithinAllowedDirs(tt.filePath, tt.allowedDirs)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidatePathWithinAllowedDirs() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestValidateExportPath(t *testing.T) {
	// Save current directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		filePath  string
		setupWd   string // Change to this working directory before test
		wantError bool
	}{
		{
			name:      "valid path in temp dir",
			filePath:  filepath.Join(os.TempDir(), "export.asc"),
			setupWd:   originalWd,
			wantError: false,
		},
		{
			name:      "valid path in current dir",
			filePath:  "export.asc",
			setupWd:   tmpDir,
			wantError: false,
		},
		{
			name:      "invalid absolute path",
			filePath:  "/etc/passwd",
			setupWd:   originalWd,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Change working directory if needed
			if tt.setupWd != "" && tt.setupWd != originalWd {
				if err := os.Chdir(tt.setupWd); err != nil {
					t.Fatalf("Failed to change directory: %v", err)
				}
				t.Cleanup(func() {
					if err := os.Chdir(originalWd); err != nil {
						t.Errorf("Failed to restore directory: %v", err)
					}
				})
			}

			err := ValidateExportPath(tt.filePath)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateExportPath() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}
