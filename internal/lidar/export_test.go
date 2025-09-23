package lidar

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

// Test ExportBgSnapshotToASC uses a tiny snapshot and a live BackgroundManager with
// ring elevations to ensure exported ASC contains non-zero Z values.
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

	out, err := ioutil.TempFile("", "bg-*.asc")
	if err != nil {
		t.Fatalf("temp out: %v", err)
	}
	outPath := out.Name()
	out.Close()
	defer os.Remove(outPath)

	if err := ExportBgSnapshotToASC(snap, outPath, nil); err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// Read exported file and ensure there's a line with a non-zero Z value
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	s := string(b)
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
