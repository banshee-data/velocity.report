package monitor

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func (ws *WebServer) handleExportSnapshotASC(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	var snap *l3grid.BgSnapshot
	snapID := r.URL.Query().Get("snapshot_id")
	if snapID != "" {
		// TODO: implement lookup by snapshot_id if needed
		ws.writeJSONError(w, http.StatusNotImplemented, "snapshot_id lookup not implemented yet")
		return
	} else {
		if ws.db == nil {
			ws.writeJSONError(w, http.StatusInternalServerError, "no database configured for snapshot lookup")
			return
		}
		var err error
		snap, err = ws.db.GetLatestBgSnapshot(sensorID)
		if err != nil || snap == nil {
			ws.writeJSONError(w, http.StatusNotFound, "no snapshot found for sensor")
			return
		}
	}
	// Build elevations argument from embedded config (if available).
	var elevs []float64
	if cfg, err := parse.LoadEmbeddedPandar40PConfig(); err == nil {
		if e := parse.ElevationsFromConfig(cfg); e != nil && len(e) == snap.Rings {
			elevs = e
		}
	}

	// The export path is generated internally by ExportBgSnapshotToASC
	// to prevent user-controlled data from flowing into file system operations.
	if _, err := l3grid.ExportBgSnapshotToASC(snap, elevs); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("export error: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "note": "File exported to temp directory"})
}

func (ws *WebServer) handleExportFrameSequenceASC(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	fb := l2frames.GetFrameBuilder(sensorID)
	if fb == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no FrameBuilder for sensor")
		return
	}

	// Export latest background snapshot immediately
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "no database configured for snapshot lookup")
		return
	}
	snap, err := ws.db.GetLatestBgSnapshot(sensorID)
	if err != nil || snap == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no snapshot found for sensor")
		return
	}
	var elevs []float64
	if cfg, err := parse.LoadEmbeddedPandar40PConfig(); err == nil {
		if e := parse.ElevationsFromConfig(cfg); e != nil && len(e) == snap.Rings {
			elevs = e
		}
	}
	// Export paths are generated internally by the export functions for security
	if _, err := l3grid.ExportBgSnapshotToASC(snap, elevs); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("background export error: %v", err))
		return
	}

	// Queue next 5 frames for export
	fb.RequestExportFrameBatchASC(5)

	// Kick off foreground snapshot exports asynchronously (paths are ignored, generated internally)
	go ws.exportForegroundSequenceInternal(sensorID, 5)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "scheduled",
		"note":   "Background exported immediately to temp directory; frames and foreground exports scheduled. Check temp directory for files with 'export_' prefix.",
	})
}

func (ws *WebServer) handleExportNextFrameASC(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	// Find FrameBuilder for sensorID (assume registry or global)
	fb := l2frames.GetFrameBuilder(sensorID)
	if fb == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no FrameBuilder for sensor")
		return
	}

	// The export path is generated internally by the export functions
	fb.RequestExportNextFrameASC()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "note": "Will export next completed frame to temp directory"})
}

func (ws *WebServer) handleExportForegroundASC(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	snap := l3grid.GetForegroundSnapshot(sensorID)
	if snap == nil || len(snap.ForegroundPoints) == 0 {
		ws.writeJSONError(w, http.StatusNotFound, "no foreground snapshot available")
		return
	}

	// The export path is generated internally by ExportForegroundSnapshotToASC
	if _, err := l3grid.ExportForegroundSnapshotToASC(snap); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("export error: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "note": "File exported to temp directory"})
}

func (ws *WebServer) exportForegroundSequenceInternal(sensorID string, count int) {
	if count <= 0 {
		return
	}

	deadline := time.Now().Add(30 * time.Second)
	var last time.Time
	exported := 0

	for exported < count && time.Now().Before(deadline) {
		snap := l3grid.GetForegroundSnapshot(sensorID)
		if snap == nil || snap.Timestamp.IsZero() || len(snap.ForegroundPoints) == 0 || !snap.Timestamp.After(last) {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Export path is generated internally by ExportForegroundSnapshotToASC
		if _, err := l3grid.ExportForegroundSnapshotToASC(snap); err != nil {
			log.Printf("[ExportSequence] foreground export failed (%d/%d) sensor=%s: %v", exported+1, count, sensorID, err)
		} else {
			log.Printf("[ExportSequence] exported foreground %d/%d for sensor=%s", exported+1, count, sensorID)
		}

		last = snap.Timestamp
		exported++
	}

	if exported < count {
		log.Printf("[ExportSequence] foreground export ended early: got %d/%d snapshots for sensor=%s before timeout", exported, count, sensorID)
	}
}

func (ws *WebServer) handleLidarSnapshots(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if _, err := fmt.Sscanf(l, "%d", &limit); err != nil {
			limit = 10 // Reset to default on parse error
		}
		if limit <= 0 || limit > 100 {
			limit = 10
		}
	}
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "no database configured for snapshot lookup")
		return
	}
	snaps, err := ws.db.ListRecentBgSnapshots(sensorID, limit)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("get recent snapshots: %v", err))
		return
	}
	type SnapSummary struct {
		SnapshotID        interface{} `json:"snapshot_id"`
		SensorID          string      `json:"sensor_id"`
		Taken             string      `json:"taken"`
		BlobBytes         int         `json:"blob_bytes"`
		ChangedCellsCount int         `json:"changed_cells_count"`
		SnapshotReason    string      `json:"snapshot_reason"`
		NonzeroCells      int         `json:"nonzero_cells"`
		TotalCells        int         `json:"total_cells"`
	}
	var summaries []SnapSummary
	for _, snap := range snaps {
		var snapIDVal interface{}
		if snap.SnapshotID != nil {
			snapIDVal = *snap.SnapshotID
		}
		nonzero := 0
		total := 0
		if len(snap.GridBlob) > 0 {
			gz, err := gzip.NewReader(bytes.NewReader(snap.GridBlob))
			if err == nil {
				var cells []l3grid.BackgroundCell
				dec := gob.NewDecoder(gz)
				if err := dec.Decode(&cells); err == nil {
					total = len(cells)
					for _, c := range cells {
						if c.TimesSeenCount > 0 || c.AverageRangeMeters != 0 || c.RangeSpreadMeters != 0 {
							nonzero++
						}
					}
				}
				gz.Close()
			}
		}
		summaries = append(summaries, SnapSummary{
			SnapshotID:        snapIDVal,
			SensorID:          snap.SensorID,
			Taken:             time.Unix(0, snap.TakenUnixNanos).Format(time.RFC3339Nano),
			BlobBytes:         len(snap.GridBlob),
			ChangedCellsCount: snap.ChangedCellsCount,
			SnapshotReason:    snap.SnapshotReason,
			NonzeroCells:      nonzero,
			TotalCells:        total,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summaries)
}

func (ws *WebServer) handleLidarSnapshotsCleanup(w http.ResponseWriter, r *http.Request) {
	// Parse form to get sensor_id if needed, or query param
	if err := r.ParseForm(); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid form data")
		return
	}
	sensorID := r.FormValue("sensor_id")
	if sensorID == "" {
		sensorID = r.URL.Query().Get("sensor_id")
	}
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	if ws.db == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "no database configured")
		return
	}

	count, err := ws.db.DeleteDuplicateBgSnapshots(sensorID)
	if err != nil {
		log.Printf("Failed to cleanup snapshots for %s: %v", sensorID, err)
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("cleanup failed: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"sensor_id": sensorID,
		"deleted":   count,
	})
}

func (ws *WebServer) handleLidarSnapshot(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query().Get("sensor_id")
	if sensorID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "missing 'sensor_id' parameter")
		return
	}

	// Use the configured DB instance. We no longer probe multiple DB files.
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "no database configured for snapshot lookup")
		return
	}
	snap, err := ws.db.GetLatestBgSnapshot(sensorID)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("get latest snapshot: %v", err))
		return
	}
	if snap == nil {
		ws.writeJSONError(w, http.StatusNotFound, "no snapshot found for sensor")
		return
	}

	// helper for optional snapshot id
	var snapIDVal interface{}
	if snap.SnapshotID != nil {
		snapIDVal = *snap.SnapshotID
	}

	summary := map[string]interface{}{
		"snapshot_id":         snapIDVal,
		"sensor_id":           snap.SensorID,
		"taken":               time.Unix(0, snap.TakenUnixNanos).Format(time.RFC3339Nano),
		"rings":               snap.Rings,
		"azimuth_bins":        snap.AzimuthBins,
		"params_json":         snap.ParamsJSON,
		"blob_bytes":          len(snap.GridBlob),
		"changed_cells_count": snap.ChangedCellsCount,
		"snapshot_reason":     snap.SnapshotReason,
	}

	// quick hex prefix for inspection
	prefix := 64
	if len(snap.GridBlob) < prefix {
		prefix = len(snap.GridBlob)
	}
	summary["blob_hex_prefix"] = hex.EncodeToString(snap.GridBlob[:prefix])

	// Try to gunzip + gob decode
	if len(snap.GridBlob) == 0 {
		summary["total_cells"] = 0
		summary["non_empty_cells"] = 0
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(summary)
		return
	}

	gz, err := gzip.NewReader(bytes.NewReader(snap.GridBlob))
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("gunzip: %v", err))
		return
	}
	defer gz.Close()

	var cells []l3grid.BackgroundCell
	dec := gob.NewDecoder(gz)
	if err := dec.Decode(&cells); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("gob decode: %v", err))
		return
	}

	total := len(cells)
	nonZero := 0
	samples := make([]map[string]interface{}, 0, 10)
	maxSamples := 10
	for i, c := range cells {
		if c.TimesSeenCount > 0 || c.AverageRangeMeters != 0 || c.RangeSpreadMeters != 0 {
			nonZero++
			if len(samples) < maxSamples {
				ring := i / snap.AzimuthBins
				azbin := i % snap.AzimuthBins
				samples = append(samples, map[string]interface{}{
					"idx":          i,
					"ring":         ring,
					"azbin":        azbin,
					"avg_m":        c.AverageRangeMeters,
					"spread_m":     c.RangeSpreadMeters,
					"times_seen":   c.TimesSeenCount,
					"last_update":  time.Unix(0, c.LastUpdateUnixNanos).Format(time.RFC3339Nano),
					"frozen_until": time.Unix(0, c.FrozenUntilUnixNanos).Format(time.RFC3339Nano),
				})
			}
		}
	}

	summary["total_cells"] = total
	summary["non_empty_cells"] = nonZero
	summary["samples"] = samples

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}
