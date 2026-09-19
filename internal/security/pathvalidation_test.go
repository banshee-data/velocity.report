package security

import (
	"os"
	"path/filepath"
	"strings"
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

	// Create nested directory structure with symlinks for testing deep parent resolution
	deepDir := filepath.Join(tmpDir, "deep")
	deepSafeDir := filepath.Join(deepDir, "safe")
	if err := os.MkdirAll(deepSafeDir, 0755); err != nil {
		t.Fatalf("Failed to create deep safe directory: %v", err)
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
		{
			name:      "non-existent file in valid directory",
			filePath:  filepath.Join(safeDir, "newfile.txt"),
			safeDir:   safeDir,
			wantError: false,
		},
		{
			name:      "non-existent nested path in valid directory",
			filePath:  filepath.Join(safeDir, "newdir", "newfile.txt"),
			safeDir:   safeDir,
			wantError: false,
		},
		{
			name:      "non-existent file via symlink directory",
			filePath:  filepath.Join(symlinkPath, "newfile.txt"),
			safeDir:   safeDir,
			wantError: true,
		},
		{
			name:      "deeply nested non-existent path for parent resolution",
			filePath:  filepath.Join(deepSafeDir, "newdir", "subdir", "file.txt"),
			safeDir:   deepSafeDir,
			wantError: false,
		},
		{
			name:      "root-level path for parent resolution loop",
			filePath:  "/nonexistent/path/file.txt",
			safeDir:   "/tmp",
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
			name:      "dirty path resolving to valid location",
			filePath:  filepath.Join(os.TempDir(), "subdir", "..", "export.asc"),
			setupWd:   originalWd,
			wantError: false,
		},
		{
			name:      "dirty path attempting traversal to root",
			filePath:  filepath.Join(tmpDir, "..", "..", "..", "..", "..", "..", "..", "etc", "passwd"),
			setupWd:   tmpDir,
			wantError: true,
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

func TestValidateOutputPath(t *testing.T) {
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
			filePath:  filepath.Join(os.TempDir(), "output.csv"),
			setupWd:   originalWd,
			wantError: false,
		},
		{
			name:      "valid path in current dir",
			filePath:  "output.csv",
			setupWd:   tmpDir,
			wantError: false,
		},
		{
			name:      "valid relative path in current dir",
			filePath:  "subdir/output.csv",
			setupWd:   tmpDir,
			wantError: false,
		},
		{
			name:      "invalid absolute path",
			filePath:  "/etc/passwd",
			setupWd:   originalWd,
			wantError: true,
		},
		{
			name:      "invalid path traversal",
			filePath:  "../../../etc/passwd",
			setupWd:   tmpDir,
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

			err := ValidateOutputPath(tt.filePath)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateOutputPath() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "unknown",
		},
		{
			name:     "simple alphanumeric",
			input:    "file123",
			expected: "file123",
		},
		{
			name:     "with allowed special chars",
			input:    "file-name_v1.0.txt",
			expected: "file-name_v1.0.txt",
		},
		{
			name:     "with spaces",
			input:    "my file name",
			expected: "my_file_name",
		},
		{
			name:     "with path separators",
			input:    "../../../etc/passwd",
			expected: "etc_passwd",
		},
		{
			name:     "with special characters",
			input:    "file@#$%name!",
			expected: "file_name",
		},
		{
			name:     "with consecutive special chars",
			input:    "file@@@name",
			expected: "file_name",
		},
		{
			name:     "leading and trailing underscores",
			input:    "__file__",
			expected: "file",
		},
		{
			name:     "leading and trailing dots",
			input:    "..file..",
			expected: "file",
		},
		{
			name:     "mixed leading/trailing",
			input:    "._file_.",
			expected: "file",
		},
		{
			name:     "only special characters",
			input:    "@#$%^&*()",
			expected: "unknown",
		},
		{
			name:     "unicode characters",
			input:    "file_名前_name",
			expected: "file___name",
		},
		{
			name:     "very long name - 200 characters",
			input:    strings.Repeat("abcd", 50), // 200 characters
			expected: strings.Repeat("abcd", 32), // Truncated to 128 characters
		},
		{
			name:     "max length boundary - 128 characters",
			input:    strings.Repeat("x", 128), // Exactly 128 characters
			expected: strings.Repeat("x", 128), // Should remain 128 characters
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeFilename() = %q, want %q", result, tt.expected)
			}
			// Additional validation: result should never exceed 128 characters
			if len(result) > 128 {
				t.Errorf("SanitizeFilename() result too long: got %d chars, want <= 128", len(result))
			}
		})
	}
}

// ResolvePathWithinDirectory exists for one reason the Validate wrapper
// cannot express: it hands back the canonical path, so a caller opens what
// was checked rather than the string it was given. These tests pin that
// contract directly. Through the wrapper the return value is discarded, and
// a regression that returned the unresolved input would pass every other
// test in this file.

func TestResolvePathWithinDirectoryReturnsTheSymlinkTargetNotTheInput(t *testing.T) {
	safeDir := canonicalTempDir(t)
	target := filepath.Join(safeDir, "real.pack")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(safeDir, "alias.pack")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := ResolvePathWithinDirectory(link, safeDir)
	if err != nil {
		t.Fatalf("a symlink that stays inside the safe directory was rejected: %v", err)
	}
	if got != target {
		t.Errorf("resolved to %q, want the symlink's target %q: acting on the unresolved input reopens the check-to-use gap", got, target)
	}
}

func TestResolvePathWithinDirectoryRejectsASymlinkThatLeavesTheSafeDir(t *testing.T) {
	safeDir := canonicalTempDir(t)
	outside := filepath.Join(canonicalTempDir(t), "secret")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(safeDir, "innocent.pack")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := ResolvePathWithinDirectory(link, safeDir)
	if err == nil {
		t.Fatalf("a symlink out of the safe directory resolved to %q, want it rejected", got)
	}
	if got != "" {
		t.Errorf("rejection returned %q alongside the error, want the empty string: a caller that ignores the error must not be handed a usable path", got)
	}
}

func TestResolvePathWithinDirectoryCanonicalisesAPathThatDoesNotExistYet(t *testing.T) {
	// An export names a directory it is about to create. The path cannot be
	// resolved itself, so its nearest existing parent is, and the remainder
	// is rejoined onto that canonical parent.
	realDir := canonicalTempDir(t)
	safeDir := filepath.Join(canonicalTempDir(t), "packs")
	if err := os.Symlink(realDir, safeDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := ResolvePathWithinDirectory(filepath.Join(safeDir, "run-1", "pack"), safeDir)
	if err != nil {
		t.Fatalf("a not-yet-created path under the safe directory was rejected: %v", err)
	}
	if want := filepath.Join(realDir, "run-1", "pack"); got != want {
		t.Errorf("resolved to %q, want %q: the symlinked safe directory should be resolved away", got, want)
	}
}

func TestResolvePathWithinDirectoryFailsWhenTheSafeDirDoesNotExist(t *testing.T) {
	missing := filepath.Join(canonicalTempDir(t), "never-created")

	got, err := ResolvePathWithinDirectory(filepath.Join(missing, "pack"), missing)
	if err == nil {
		t.Fatalf("resolved to %q against a safe directory that does not exist, want an error: a boundary that cannot be resolved cannot be enforced", got)
	}
	if got != "" {
		t.Errorf("failure returned %q alongside the error, want the empty string", got)
	}
}

// canonicalTempDir returns a temp dir with its own symlinks resolved. On
// macOS t.TempDir() sits under /var, itself a symlink to /private/var, so an
// unresolved temp path never equals the canonical path these tests compare.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
