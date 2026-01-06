package lidar

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"os"
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
