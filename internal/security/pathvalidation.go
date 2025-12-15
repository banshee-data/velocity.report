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

// SanitizeFilename makes a safe filename from an arbitrary string. It replaces
// any characters that are not ASCII letters, digits, dot, underscore or dash
// with an underscore. It also collapses repeated underscores and trims the
// result to a reasonable length. This is intended for use when embedding
// user-provided identifiers into file names.
func SanitizeFilename(s string) string {
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	// Limit resulting filename length to avoid overly long paths
	const maxLen = 128
	lastUnderscore := false
	for _, r := range s {
		if len(b.String()) >= maxLen {
			break
		}
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastUnderscore = false
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteRune('_')
				lastUnderscore = true
			}
		}
	}
	out := b.String()
	// Trim leading/trailing underscores or dots
	out = strings.Trim(out, "._")
	if out == "" {
		return "unknown"
	}
	return out
}
