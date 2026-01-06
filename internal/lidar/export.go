package lidar

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/banshee-data/velocity.report/internal/security"
)

// defaultExportDir is the base directory for all ASC exports.
// It is intentionally restricted to a single directory to avoid writing
// outside controlled locations, even if callers provide arbitrary paths.
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

// safeExportPath constructs a safe absolute path for an export file based on a
// user-supplied path string. It restricts exports to defaultExportDir and
// validates the final path with the shared security.ValidateExportPath helper.
func safeExportPath(userPath string) (string, error) {
	if userPath == "" {
		return "", fmt.Errorf("empty export path")
	}
	// Use only the last path component to avoid any directory traversal and
	// to ensure we control the export root directory.
	base := filepath.Base(userPath)
	if base == "." || base == ".." || base == "" {
		return "", fmt.Errorf("invalid export filename")
	}

	joined := filepath.Join(defaultExportDir, base)
	absPath, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("cannot resolve export path: %w", err)
	}
	cleanPath := filepath.Clean(absPath)

	// Ensure the cleaned absolute path is still within the defaultExportDir.
	baseDir := defaultExportDir
	if baseDir == "" {
		return "", fmt.Errorf("export base directory not configured")
	}
	// Normalize baseDir to an absolute, cleaned form for the prefix check.
	baseDirAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("cannot resolve export base directory: %w", err)
	}
	baseDirAbs = filepath.Clean(baseDirAbs)
	if !strings.HasPrefix(cleanPath, baseDirAbs+string(os.PathSeparator)) && cleanPath != baseDirAbs {
		return "", fmt.Errorf("export path escapes base directory")
	}

	if err := security.ValidateExportPath(cleanPath); err != nil {
		log.Printf("Security: rejected export path %s (from %s, cleaned: %s): %v", joined, userPath, cleanPath, err)
		return "", fmt.Errorf("invalid export path: %w", err)
	}
	return cleanPath, nil
}

// PointASC is a cartesian point with optional extra columns for export
// (X, Y, Z, Intensity, ...extra)
type PointASC struct {
	X, Y, Z   float64
	Intensity int
	Extra     []interface{}
}

// ExportPointsToASC exports a slice of PointASC to a CloudCompare-compatible .asc file
// extraHeader is a string describing extra columns (optional)
func ExportPointsToASC(points []PointASC, filePath string, extraHeader string) error {
	if len(points) == 0 {
		return fmt.Errorf("no points to export")
	}

	// Build a safe export path anchored under defaultExportDir to prevent path
	// traversal and unintended file overwrites.
	safePath, err := safeExportPath(filePath)
	if err != nil {
		return err
	}

	f, err := os.Create(safePath)
	if err != nil {
		return err
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
	log.Printf("Exported %d points to %s", len(points), safePath)
	return nil
}

// ExportBgSnapshotToASC decodes a BgSnapshot's grid blob, constructs a temporary
// BackgroundGrid and BackgroundManager, supplies per-ring elevations (preferring
// a live BackgroundManager and falling back to embedded parser config), and
// exports the resulting points to an ASC file at outPath.
func ExportBgSnapshotToASC(snap *BgSnapshot, outPath string, ringElevations []float64) error {
	if snap == nil {
		return fmt.Errorf("nil snapshot")
	}

	// Build a safe export path anchored under defaultExportDir to prevent path
	// traversal and unintended file overwrites.
	safePath, err := safeExportPath(outPath)
	if err != nil {
		return err

	}
	// Decode grid blob
	gz, err := gzip.NewReader(bytes.NewReader(snap.GridBlob))
	if err != nil {
		return fmt.Errorf("gunzip error: %v", err)
	}
	defer gz.Close()
	var cells []BackgroundCell
	dec := gob.NewDecoder(gz)
	if err := dec.Decode(&cells); err != nil {
		return fmt.Errorf("gob decode error: %v", err)
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
			return mgr.ExportBackgroundGridToASC(safePath)
		}
	}

	if ringElevations != nil && len(ringElevations) == grid.Rings {
		if err := mgr.SetRingElevations(ringElevations); err == nil {
			log.Printf("Export: set ring elevations from caller for sensor %s", snap.SensorID)
			return mgr.ExportBackgroundGridToASC(safePath)
		}
	}

	if live := GetBackgroundManager(snap.SensorID); live != nil && live.Grid != nil && len(live.Grid.RingElevations) == grid.Rings {
		elevCopy := make([]float64, len(live.Grid.RingElevations))
		copy(elevCopy, live.Grid.RingElevations)
		_ = mgr.SetRingElevations(elevCopy)
		log.Printf("Export: copied ring elevations from live BackgroundManager for sensor %s", snap.SensorID)
	}

	return mgr.ExportBackgroundGridToASC(safePath)
}
