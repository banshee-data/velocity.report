package lidar

import (
	"fmt"
	"log"
	"os"
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
	f, err := os.Create(filePath)
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
