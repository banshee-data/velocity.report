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

	"github.com/banshee-data/velocity.report/internal/security"
)

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

	// Validate path to prevent path traversal attacks
	// Note: We deliberately clean the path first to ensure the validated path matches
	// the path we essentially open.
	cleanPath := filepath.Clean(filePath)
	if err := security.ValidateExportPath(cleanPath); err != nil {
		log.Printf("Security: rejected export path %s (cleaned: %s): %v", filePath, cleanPath, err)
		return fmt.Errorf("invalid export path: %w", err)
	}

	f, err := os.Create(cleanPath)
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
	log.Printf("Exported %d points to %s", len(points), filePath)
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

	// Validate path to prevent path traversal attacks
	// Note: We deliberately clean the path first to ensure the validated path matches
	// the path we essentially open.
	cleanPath := filepath.Clean(outPath)
	if err := security.ValidateExportPath(cleanPath); err != nil {
		log.Printf("Security: rejected export path %s (cleaned: %s): %v", outPath, cleanPath, err)
		return fmt.Errorf("invalid export path: %w", err)
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
			return mgr.ExportBackgroundGridToASC(cleanPath)
		}
	}

	if ringElevations != nil && len(ringElevations) == grid.Rings {
		if err := mgr.SetRingElevations(ringElevations); err == nil {
			log.Printf("Export: set ring elevations from caller for sensor %s", snap.SensorID)
			return mgr.ExportBackgroundGridToASC(cleanPath)
		}
	}

	if live := GetBackgroundManager(snap.SensorID); live != nil && live.Grid != nil && len(live.Grid.RingElevations) == grid.Rings {
		elevCopy := make([]float64, len(live.Grid.RingElevations))
		copy(elevCopy, live.Grid.RingElevations)
		_ = mgr.SetRingElevations(elevCopy)
		log.Printf("Export: copied ring elevations from live BackgroundManager for sensor %s", snap.SensorID)
	}

	return mgr.ExportBackgroundGridToASC(cleanPath)
}
