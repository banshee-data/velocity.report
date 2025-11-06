package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidatePathWithinDirectory checks if a file path is within a safe directory.
// It prevents path traversal attacks by ensuring the resolved path doesn't escape
// the specified safe directory. This includes protection against symlink-based attacks.
func ValidatePathWithinDirectory(filePath, safeDir string) error {
	// Clean the path to resolve . and .. components
	cleanPath := filepath.Clean(filePath)

	// Get absolute paths for proper validation
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	absSafeDir, err := filepath.Abs(safeDir)
	if err != nil {
		return fmt.Errorf("failed to resolve safe directory path: %w", err)
	}

	// Resolve symlinks to get canonical paths (prevents symlink-based traversal attacks)
	// Note: EvalSymlinks returns an error if the path doesn't exist
	canonicalPath := absPath
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		// Path exists and all symlinks resolved successfully
		canonicalPath = resolved
	} else {
		// Path doesn't exist. Check parent directories for symlinks to prevent attacks like:
		// /tmp/evil-symlink/newfile.txt where evil-symlink -> /etc
		// We walk up the directory tree until we find an existing directory
		checkPath := absPath
		for {
			parentDir := filepath.Dir(checkPath)
			if parentDir == checkPath {
				// Reached root without finding existing directory
				// This shouldn't happen in practice, but use original path
				break
			}

			if resolved, err := filepath.EvalSymlinks(parentDir); err == nil {
				// Found an existing parent directory with resolved symlinks
				// Reconstruct the path with canonical parent and remaining path components
				relToParent, _ := filepath.Rel(parentDir, absPath)
				canonicalPath = filepath.Join(resolved, relToParent)
				break
			}

			checkPath = parentDir
		}
	}

	canonicalSafeDir, err := filepath.EvalSymlinks(absSafeDir)
	if err != nil {
		return fmt.Errorf("failed to resolve safe directory symlinks: %w", err)
	}

	// Check if canonical path is within canonical safe directory
	relPath, err := filepath.Rel(canonicalSafeDir, canonicalPath)
	if err != nil {
		return fmt.Errorf("path is outside safe directory: %w", err)
	}

	// Reject paths that escape the safe directory
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || filepath.IsAbs(relPath) {
		return fmt.Errorf("path traversal detected: %s attempts to escape %s", filePath, safeDir)
	}

	return nil
}

// ValidatePathWithinAllowedDirs checks if a file path is within any of the allowed directories.
// Returns nil if the path is valid, or an error describing why it was rejected.
func ValidatePathWithinAllowedDirs(filePath string, allowedDirs []string) error {
	if len(allowedDirs) == 0 {
		return fmt.Errorf("no allowed directories specified")
	}

	for _, dir := range allowedDirs {
		if err := ValidatePathWithinDirectory(filePath, dir); err == nil {
			return nil // Path is valid within this directory
		}
	}

	// Path is not within any allowed directory
	return fmt.Errorf("path must be within one of the allowed directories: %v", allowedDirs)
}

// ValidateExportPath validates a file path for export operations.
// It ensures the path is within either the temp directory or current working directory.
func ValidateExportPath(filePath string) error {
	tempDir := os.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	allowedDirs := []string{tempDir, cwd}
	return ValidatePathWithinAllowedDirs(filePath, allowedDirs)
}
