package lidar

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// defaultExportDir is the base directory for all ASC exports.
// It is intentionally restricted to a single directory to avoid writing
// outside controlled locations.
var defaultExportDir = func() string {
	tmp := os.TempDir()
	abs, err := filepath.Abs(tmp)
	if err != nil {
		// Fall back to tmp as-is but log for visibility.
		log.Printf("export: could not resolve absolute temp dir from %q: %v", tmp, err)
		return tmp
	}
	return filepath.Clean(abs)
}()

// generateExportFilename creates a unique filename for export operations.
// The filename is generated entirely from trusted internal sources (timestamp + random bytes)
// to prevent any user-controlled data from flowing into file paths.
// The extension parameter allows callers to specify a file extension (default: ".asc").
func generateExportFilename(extension string) string {
	if extension == "" {
		extension = ".asc"
	}
	// Generate 8 random bytes for uniqueness
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to timestamp-only if random fails
		return fmt.Sprintf("export_%d%s", time.Now().UnixNano(), extension)
	}
	return fmt.Sprintf("export_%d_%s%s", time.Now().UnixNano(), hex.EncodeToString(randomBytes), extension)
}

// buildExportPath constructs a safe export path in the defaultExportDir.
// The filename is generated internally - no user input is used in path construction.
func buildExportPath(extension string) string {
	filename := generateExportFilename(extension)
	return filepath.Join(defaultExportDir, filename)
}

// PointASC is a cartesian point with optional extra columns for export
// (X, Y, Z, Intensity, ...extra)
type PointASC struct {
	X, Y, Z   float64
	Intensity int
	Extra     []interface{}
}

// ExportPointsToASC exports a slice of PointASC to a CloudCompare-compatible .asc file.
// The export path is generated internally using a timestamp and random suffix to prevent
// path traversal attacks. Returns the actual path where the file was written.
// extraHeader is a string describing extra columns (optional)
func ExportPointsToASC(points []PointASC, extraHeader string) (string, error) {
	if len(points) == 0 {
		return "", fmt.Errorf("no points to export")
	}

	// Generate a safe export path entirely from trusted internal sources.
	exportPath := buildExportPath(".asc")

	f, err := os.Create(exportPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Write header
	fmt.Fprintf(f, "# Exported points\n")
	fmt.Fprintf(f, "# Format: X Y Z Intensity%s\n", extraHeader)

	for _, p := range points {
		fmt.Fprintf(f, "%.6f %.6f %.6f %d", p.X, p.Y, p.Z, p.Intensity)
		for _, col := range p.Extra {
			switch v := col.(type) {
			case int:
				fmt.Fprintf(f, " %d", v)
			case float64:
				fmt.Fprintf(f, " %.6f", v)
			case string:
				fmt.Fprintf(f, " %s", v)
			default:
				fmt.Fprintf(f, " %v", v)
			}
		}
		fmt.Fprintln(f)
	}
	log.Printf("Exported %d points to %s", len(points), exportPath)
	return exportPath, nil
}

// ExportBgSnapshotToASC decodes a BgSnapshot's grid blob, constructs a temporary
// BackgroundGrid and BackgroundManager, supplies per-ring elevations (preferring
// a live BackgroundManager and falling back to embedded parser config), and
// exports the resulting points to an ASC file.
// Returns the path where the file was written.
func ExportBgSnapshotToASC(snap *BgSnapshot, ringElevations []float64) (string, error) {
	if snap == nil {
		return "", fmt.Errorf("nil snapshot")
	}

	// Decode grid blob
	gz, err := gzip.NewReader(bytes.NewReader(snap.GridBlob))
	if err != nil {
		return "", fmt.Errorf("gunzip error: %v", err)
	}
	defer gz.Close()
	var cells []BackgroundCell
	dec := gob.NewDecoder(gz)
	if err := dec.Decode(&cells); err != nil {
		return "", fmt.Errorf("gob decode error: %v", err)
	}

	grid := &BackgroundGrid{
		SensorID:    snap.SensorID,
		Rings:       snap.Rings,
		AzimuthBins: snap.AzimuthBins,
		Cells:       cells,
	}
	mgr := &BackgroundManager{Grid: grid}

	// Prefer snapshot-stored elevations, then caller-supplied, then live manager copy.
	if snap.RingElevationsJSON != "" {
		var elevs []float64
		if err := json.Unmarshal([]byte(snap.RingElevationsJSON), &elevs); err == nil && len(elevs) == grid.Rings {
			_ = mgr.SetRingElevations(elevs)
			log.Printf("Export: used ring elevations embedded in snapshot for sensor %s", snap.SensorID)
			return mgr.ExportBackgroundGridToASC()
		}
	}

	if ringElevations != nil && len(ringElevations) == grid.Rings {
		if err := mgr.SetRingElevations(ringElevations); err == nil {
			log.Printf("Export: set ring elevations from caller for sensor %s", snap.SensorID)
			return mgr.ExportBackgroundGridToASC()
		}
	}

	if live := GetBackgroundManager(snap.SensorID); live != nil && live.Grid != nil && len(live.Grid.RingElevations) == grid.Rings {
		elevCopy := make([]float64, len(live.Grid.RingElevations))
		copy(elevCopy, live.Grid.RingElevations)
		_ = mgr.SetRingElevations(elevCopy)
		log.Printf("Export: copied ring elevations from live BackgroundManager for sensor %s", snap.SensorID)
	}

	return mgr.ExportBackgroundGridToASC()
}
