package l3grid

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
)

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
