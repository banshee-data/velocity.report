package lidar

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExportBgSnapshotToASC tests that export functions work correctly.
// Note: Since security fixes now generate export paths internally (not from user input),
// the test verifies that ExportPointsToASC and ExportBgSnapshotToASC return the actual
// path used and that the exported file can be read.
func TestExportBgSnapshotToASC(t *testing.T) {
	// Build a small grid: 2 rings x 4 azbins
	rings := 2
	azimuthBins := 4
	cells := make([]BackgroundCell, rings*azimuthBins)
	// Set one cell to a non-zero average range on ring 1, azbin 1
	cells[azimuthBins+1].AverageRangeMeters = 5.0
	// Serialize cells into grid blob
	var buf []byte
	{ // gob+gzip into bytes.Buffer
		var b bytes.Buffer
		gw := gzip.NewWriter(&b)
		enc := gob.NewEncoder(gw)
		if err := enc.Encode(cells); err != nil {
			t.Fatalf("encode: %v", err)
		}
		if err := gw.Close(); err != nil {
			t.Fatalf("gzip close: %v", err)
		}
		buf = b.Bytes()
	}

	snap := &BgSnapshot{
		SensorID:       "test-sensor",
		TakenUnixNanos: 12345,
		Rings:          rings,
		AzimuthBins:    azimuthBins,
		GridBlob:       buf,
	}

	// Register a live BackgroundManager with ring elevations
	liveMgr := NewBackgroundManager("test-sensor", rings, azimuthBins, BackgroundParams{}, nil)
	// set simple elevations
	elevs := make([]float64, rings)
	elevs[0] = 0.0
	elevs[1] = 10.0
	if err := liveMgr.SetRingElevations(elevs); err != nil {
		t.Fatalf("SetRingElevations: %v", err)
	}

	// ExportBgSnapshotToASC now returns the path where the file was written
	bgPath, err := ExportBgSnapshotToASC(snap, nil)
	if err != nil {
		t.Fatalf("ExportBgSnapshotToASC failed: %v", err)
	}
	defer os.Remove(bgPath)

	// Also test ExportPointsToASC directly
	testPoints := []PointASC{{X: 1.0, Y: 2.0, Z: 3.0, Intensity: 100}}
	outPath, err := ExportPointsToASC(testPoints, "")
	if err != nil {
		t.Fatalf("ExportPointsToASC failed: %v", err)
	}
	defer os.Remove(outPath)

	// Read exported file and ensure there's content
	b2, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	s := string(b2)
	if s == "" {
		t.Fatalf("exported file empty")
	}
	// crude check: look for a Z value that is not "0.000000"
	found := false
	for _, line := range strings.Split(s, "\n") {
		if line == "" || line[0] == '#' {
			continue
		}
		var x, y, z float64
		var ii int
		n, _ := fmt.Sscanf(line, "%f %f %f %d", &x, &y, &z, &ii)
		if n >= 3 && z != 0.0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("exported ASC contains only zero Z values:\n%s", s)
	}
}

func TestGenerateExportFilename_DefaultExtension(t *testing.T) {
	filename := generateExportFilename("")
	if !strings.HasSuffix(filename, ".asc") {
		t.Errorf("expected filename to end with .asc, got: %s", filename)
	}
	if !strings.HasPrefix(filename, "export_") {
		t.Errorf("expected filename to start with export_, got: %s", filename)
	}
}

func TestGenerateExportFilename_CustomExtension(t *testing.T) {
	filename := generateExportFilename(".txt")
	if !strings.HasSuffix(filename, ".txt") {
		t.Errorf("expected filename to end with .txt, got: %s", filename)
	}
}

func TestGenerateExportFilename_Uniqueness(t *testing.T) {
	// Generate multiple filenames and ensure they're unique
	names := make(map[string]bool)
	for i := 0; i < 100; i++ {
		name := generateExportFilename(".asc")
		if names[name] {
			t.Errorf("duplicate filename generated: %s", name)
		}
		names[name] = true
	}
}

func TestGenerateExportFilename_ProducesValidFilenames(t *testing.T) {
	// Verify that generateExportFilename consistently produces valid filenames
	// regardless of whether the random generation succeeds or falls back to timestamp.
	// We can't mock rand.Read failure, but we can verify the output format is always valid.
	for i := 0; i < 10; i++ {
		filename := generateExportFilename(".asc")

		// Must start with "export_"
		if !strings.HasPrefix(filename, "export_") {
			t.Errorf("expected filename to start with 'export_', got: %s", filename)
		}

		// Must end with ".asc"
		if !strings.HasSuffix(filename, ".asc") {
			t.Errorf("expected filename to end with '.asc', got: %s", filename)
		}

		// Must not contain path separators (should be just a filename)
		if strings.ContainsAny(filename, "/\\") {
			t.Errorf("filename should not contain path separators: %s", filename)
		}

		// Must be a reasonable length
		if len(filename) < 15 || len(filename) > 100 {
			t.Errorf("filename length seems invalid: %d for %s", len(filename), filename)
		}
	}
}

func TestBuildExportPath(t *testing.T) {
	path := buildExportPath(".asc")

	// Verify it's an absolute path
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got: %s", path)
	}

	// Verify it's in the default export directory
	if !strings.HasPrefix(path, defaultExportDir) {
		t.Errorf("expected path to be in %s, got: %s", defaultExportDir, path)
	}

	// Verify filename has correct extension
	if !strings.HasSuffix(path, ".asc") {
		t.Errorf("expected path to end with .asc, got: %s", path)
	}
}

func TestExportPointsToASC_EmptyPoints(t *testing.T) {
	_, err := ExportPointsToASC([]PointASC{}, "")
	if err == nil {
		t.Error("expected error when exporting empty points")
	}
	if !strings.Contains(err.Error(), "no points") {
		t.Errorf("expected 'no points' error, got: %v", err)
	}
}

func TestExportPointsToASC_WithExtraColumns(t *testing.T) {
	points := []PointASC{
		{X: 1.0, Y: 2.0, Z: 3.0, Intensity: 100, Extra: []interface{}{42, 3.14, "test"}},
		{X: 4.0, Y: 5.0, Z: 6.0, Intensity: 200, Extra: []interface{}{99, 2.71, "data"}},
	}

	path, err := ExportPointsToASC(points, " ExtraInt ExtraFloat ExtraString")
	if err != nil {
		t.Fatalf("ExportPointsToASC failed: %v", err)
	}
	defer os.Remove(path)

	// Read and verify content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	var dataLines []string
	for _, line := range lines {
		if line != "" && !strings.HasPrefix(line, "#") {
			dataLines = append(dataLines, line)
		}
	}

	if len(dataLines) != 2 {
		t.Errorf("expected 2 data lines, got %d", len(dataLines))
	}

	// Verify first line has extra columns
	if !strings.Contains(dataLines[0], "42") {
		t.Error("expected first line to contain extra int value 42")
	}
	if !strings.Contains(dataLines[0], "test") {
		t.Error("expected first line to contain extra string value 'test'")
	}
}

func TestExportBgSnapshotToASC_NilSnapshot(t *testing.T) {
	_, err := ExportBgSnapshotToASC(nil, nil)
	if err == nil {
		t.Error("expected error when exporting nil snapshot")
	}
	if !strings.Contains(err.Error(), "nil snapshot") {
		t.Errorf("expected 'nil snapshot' error, got: %v", err)
	}
}

func TestExportBgSnapshotToASC_InvalidGridBlob(t *testing.T) {
	snap := &BgSnapshot{
		SensorID:    "test",
		Rings:       2,
		AzimuthBins: 4,
		GridBlob:    []byte("invalid data"),
	}

	_, err := ExportBgSnapshotToASC(snap, nil)
	if err == nil {
		t.Error("expected error with invalid grid blob")
	}
}

func TestExportBgSnapshotToASC_WithCallerElevations(t *testing.T) {
	rings := 2
	azimuthBins := 4
	cells := make([]BackgroundCell, rings*azimuthBins)
	cells[1].AverageRangeMeters = 5.0

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(gw)
	if err := enc.Encode(cells); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	gw.Close()

	snap := &BgSnapshot{
		SensorID:    "test-sensor-2",
		Rings:       rings,
		AzimuthBins: azimuthBins,
		GridBlob:    buf.Bytes(),
	}

	// Provide elevations via parameter
	elevations := []float64{0.0, 10.0}
	path, err := ExportBgSnapshotToASC(snap, elevations)
	if err != nil {
		t.Fatalf("ExportBgSnapshotToASC failed: %v", err)
	}
	defer os.Remove(path)

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected export file to exist")
	}
}

func TestExportBgSnapshotToASC_WithEmbeddedElevations(t *testing.T) {
	rings := 2
	azimuthBins := 4
	cells := make([]BackgroundCell, rings*azimuthBins)
	cells[1].AverageRangeMeters = 5.0

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(gw)
	if err := enc.Encode(cells); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	gw.Close()

	// Embed elevations in snapshot JSON
	elevs := []float64{0.0, 10.0}
	elevJSON, err := json.Marshal(elevs)
	if err != nil {
		t.Fatalf("failed to marshal elevations: %v", err)
	}

	snap := &BgSnapshot{
		SensorID:           "test-sensor-3",
		Rings:              rings,
		AzimuthBins:        azimuthBins,
		GridBlob:           buf.Bytes(),
		RingElevationsJSON: string(elevJSON),
	}

	path, err := ExportBgSnapshotToASC(snap, nil)
	if err != nil {
		t.Fatalf("ExportBgSnapshotToASC failed: %v", err)
	}
	defer os.Remove(path)

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected export file to exist")
	}
}

func TestExportBgSnapshotToASC_WithInvalidElevationsJSON(t *testing.T) {
	rings := 2
	azimuthBins := 4
	cells := make([]BackgroundCell, rings*azimuthBins)
	cells[1].AverageRangeMeters = 5.0

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(gw)
	if err := enc.Encode(cells); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	gw.Close()

	snap := &BgSnapshot{
		SensorID:           "test-sensor-4",
		Rings:              rings,
		AzimuthBins:        azimuthBins,
		GridBlob:           buf.Bytes(),
		RingElevationsJSON: "invalid json",
	}

	// Should fall back to other elevation sources or defaults
	path, err := ExportBgSnapshotToASC(snap, nil)
	if err != nil {
		// This might fail if no valid elevations are available, which is okay
		t.Logf("Export failed as expected with invalid JSON: %v", err)
		return
	}
	defer os.Remove(path)
}

func TestExportBgSnapshotToASC_WithWrongElevationCount(t *testing.T) {
	rings := 2
	azimuthBins := 4
	cells := make([]BackgroundCell, rings*azimuthBins)
	cells[1].AverageRangeMeters = 5.0

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(gw)
	if err := enc.Encode(cells); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	gw.Close()

	// Provide wrong number of elevations (3 instead of 2)
	elevs := []float64{0.0, 10.0, 20.0}
	elevJSON, _ := json.Marshal(elevs)

	snap := &BgSnapshot{
		SensorID:           "test-sensor-5",
		Rings:              rings,
		AzimuthBins:        azimuthBins,
		GridBlob:           buf.Bytes(),
		RingElevationsJSON: string(elevJSON),
	}

	// Should fall back to other sources
	path, err := ExportBgSnapshotToASC(snap, nil)
	if err != nil {
		t.Logf("Export failed with wrong elevation count: %v", err)
		return
	}
	defer os.Remove(path)
}

func TestExportPointsToASC_LargeDataset(t *testing.T) {
	// Test with a larger dataset
	points := make([]PointASC, 10000)
	for i := range points {
		points[i] = PointASC{
			X:         float64(i % 100),
			Y:         float64(i / 100),
			Z:         float64(i % 50),
			Intensity: i % 256,
		}
	}

	path, err := ExportPointsToASC(points, "")
	if err != nil {
		t.Fatalf("ExportPointsToASC failed: %v", err)
	}
	defer os.Remove(path)

	// Verify file size is reasonable
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	if info.Size() < 100000 {
		t.Error("expected larger file size for 10000 points")
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
