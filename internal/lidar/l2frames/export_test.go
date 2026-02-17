package l2frames

import (
"os"
"path/filepath"
"strings"
"testing"
)

func TestGenerateExportFilename(t *testing.T) {
t.Run("default extension", func(t *testing.T) {
filename := generateExportFilename("")
if !strings.HasSuffix(filename, ".asc") {
t.Errorf("expected .asc suffix, got: %s", filename)
}
if !strings.HasPrefix(filename, "export_") {
t.Errorf("expected export_ prefix, got: %s", filename)
}
})

t.Run("custom extension", func(t *testing.T) {
filename := generateExportFilename(".txt")
if !strings.HasSuffix(filename, ".txt") {
t.Errorf("expected .txt suffix, got: %s", filename)
}
})

t.Run("unique filenames", func(t *testing.T) {
seen := make(map[string]bool)
for i := 0; i < 100; i++ {
name := generateExportFilename(".asc")
if seen[name] {
t.Errorf("duplicate filename generated: %s", name)
}
seen[name] = true
}
})

t.Run("filename does not contain path separators", func(t *testing.T) {
// Verify that generateExportFilename consistently produces valid filenames
// without path separators (preventing path traversal even from internal use)
for i := 0; i < 10; i++ {
filename := generateExportFilename(".asc")
if strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
t.Errorf("filename contains path separator: %s", filename)
}
}
})
}

func TestBuildExportPath(t *testing.T) {
path := buildExportPath(".asc")
if !filepath.IsAbs(path) {
t.Errorf("expected absolute path, got: %s", path)
}
if !strings.HasSuffix(path, ".asc") {
t.Errorf("expected .asc suffix, got: %s", path)
}
if !strings.HasPrefix(path, defaultExportDir) {
t.Errorf("expected path to be in %s, got: %s", defaultExportDir, path)
}
}

func TestExportPointsToASC(t *testing.T) {
points := []PointASC{
{X: 1.0, Y: 2.0, Z: 3.0, Intensity: 100},
{X: 4.0, Y: 5.0, Z: 6.0, Intensity: 200},
}

// Export to ASC
path, err := ExportPointsToASC(points, "")
if err != nil {
t.Fatalf("ExportPointsToASC failed: %v", err)
}
defer os.Remove(path)

// Verify file exists and can be read
content, err := os.ReadFile(path)
if err != nil {
t.Fatalf("failed to read exported file: %v", err)
}

// Verify content contains expected coordinates
contentStr := string(content)
if !strings.Contains(contentStr, "1.000000 2.000000 3.000000 100") {
t.Errorf("content does not contain first point")
}
if !strings.Contains(contentStr, "4.000000 5.000000 6.000000 200") {
t.Errorf("content does not contain second point")
}
}

func TestExportPointsToASC_WithExtra(t *testing.T) {
points := []PointASC{
{X: 1.0, Y: 2.0, Z: 3.0, Intensity: 100, Extra: []interface{}{5.5, 10}},
}

path, err := ExportPointsToASC(points, " Range Count")
if err != nil {
t.Fatalf("ExportPointsToASC failed: %v", err)
}
defer os.Remove(path)

content, err := os.ReadFile(path)
if err != nil {
t.Fatalf("failed to read exported file: %v", err)
}

contentStr := string(content)
if !strings.Contains(contentStr, "Range Count") {
t.Errorf("content does not contain extra header")
}
if !strings.Contains(contentStr, "5.500000 10") {
t.Errorf("content does not contain extra columns")
}
}

func TestExportPointsToASC_EmptyPoints(t *testing.T) {
_, err := ExportPointsToASC([]PointASC{}, "")
if err == nil {
t.Error("expected error for empty points, got nil")
}
}

func TestDefaultExportDir(t *testing.T) {
// Verify defaultExportDir is set and is an absolute path
if defaultExportDir == "" {
t.Error("defaultExportDir should not be empty")
}

if !filepath.IsAbs(defaultExportDir) {
t.Errorf("defaultExportDir should be absolute, got: %s", defaultExportDir)
}
}
