package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/security"
)

// annotationExportRequest is the POST body for /api/lidar/runs/{run_id}/annotation-export.
//
// Coverage has no default: the exporter refuses to guess it, because a
// foreground-only recording and a sparse full-scene one are indistinguishable
// after the fact, and the caller — a person looking at the run they just
// recorded — is the only one who can state it.
type annotationExportRequest struct {
	Coverage     string `json:"coverage"`
	CoverageNote string `json:"coverage_note,omitempty"`
	StartNs      int64  `json:"start_ns,omitempty"`
	EndNs        int64  `json:"end_ns,omitempty"`
	MaxSamples   int    `json:"max_samples,omitempty"`
}

// annotationExportResponse reports where the pack landed and enough of the
// manifest for a client to open it immediately without a second round trip.
type annotationExportResponse struct {
	PackDir           string `json:"pack_dir"`
	DatasetID         string `json:"dataset_id"`
	SampleCount       int    `json:"sample_count"`
	PointCount        int64  `json:"point_count"`
	Coverage          string `json:"coverage"`
	FramesWithoutPts  int    `json:"frames_without_points"`
	HasIntensity      bool   `json:"has_intensity"`
	HasClassification bool   `json:"has_classification"`
}

// defaultAnnotationMaxSamples matches the CLI's own default (see
// internal/cmd/lidar/annotation.go): annotation is human work measured in
// minutes per frame, and an unbounded export from a UI button is an easy way
// to produce a pack nobody will ever finish labelling.
const defaultAnnotationMaxSamples = 200

// handleAnnotationExport cuts a frozen annotation pack from a run's VRLOG.
// POST /api/lidar/runs/{run_id}/annotation-export
//
// This wraps the same annotation.Export used by `velocity lidar
// annotation-export`, so a pack can be produced directly from a run an
// operator is already looking at instead of first finding its VRLOG
// directory on disk and shelling out separately.
func (ws *Server) handleAnnotationExport(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "this endpoint only accepts POST requests")
		return
	}
	if ws.annotationPacksDir == "" {
		ws.writeJSONError(w, http.StatusNotImplemented,
			"annotation pack export is not configured: check server was started with --lidar-annotation-dir")
		return
	}
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusInternalServerError, "database is not configured: check server startup includes --db-path")
		return
	}

	var body annotationExportRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "the request body is not valid JSON")
		return
	}
	if body.Coverage == "" {
		ws.writeJSONError(w, http.StatusBadRequest,
			"coverage is required: state what the run could see (full, foreground_only, or decimated)")
		return
	}
	maxSamples := body.MaxSamples
	if maxSamples == 0 {
		maxSamples = defaultAnnotationMaxSamples
	}
	if maxSamples < 0 {
		ws.writeJSONError(w, http.StatusBadRequest, "max_samples must not be negative")
		return
	}

	store := sqlite.NewAnalysisRunStore(ws.db)
	run, err := store.GetRun(runID)
	if errors.Is(err, sqlite.ErrNotFound) {
		ws.writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not retrieve run: %v", err))
		return
	}
	if run.VRLogPath == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "this run has no VRLOG recording to export from")
		return
	}

	// The VRLOG path is read from the run record rather than the request, so
	// it is validated against the read boundary that already governs replay
	// (vrlogSafeDir) rather than the write boundary this endpoint owns.
	vrlogPath, err := security.ResolvePathWithinDirectory(run.VRLogPath, ws.vrlogSafeDir)
	if err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("run's VRLOG path is not within the allowed directory: %v", err))
		return
	}

	// One directory per request, named for the run and the moment: two
	// exports of the same run must not collide, and Export refuses to write
	// into an existing directory rather than merge into one.
	outDir, err := security.ResolvePathWithinDirectory(
		filepath.Join(ws.annotationPacksDir, fmt.Sprintf("%s-%s", runID, time.Now().UTC().Format("20060102-150405.000000000"))),
		ws.annotationPacksDir)
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not resolve pack output directory: %v", err))
		return
	}

	pack, err := annotation.Export(annotation.ExportConfig{
		VRLOGPath:    vrlogPath,
		OutDir:       outDir,
		StartNs:      body.StartNs,
		EndNs:        body.EndNs,
		MaxSamples:   maxSamples,
		Coverage:     annotation.CaptureCoverage(body.Coverage),
		CoverageNote: body.CoverageNote,
	})
	if err != nil {
		// Export can fail after creating outDir (e.g. no point-bearing frames
		// in the requested window). Remove the partial directory rather than
		// leaving an empty one a later export would then refuse to reuse.
		_ = os.RemoveAll(outDir)
		// "no point-bearing frames" is the one failure mode caused by the
		// caller's own start/end window; everything else here is a read
		// failure against a VRLOG this server itself recorded.
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "no point-bearing frames") {
			status = http.StatusBadRequest
		}
		ws.writeJSONError(w, status, fmt.Sprintf("annotation export failed: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(annotationExportResponse{
		PackDir:           pack.Dir,
		DatasetID:         pack.Manifest.DatasetID,
		SampleCount:       pack.Manifest.SampleCount,
		PointCount:        pack.Manifest.PointCount,
		Coverage:          string(pack.Manifest.Coverage),
		FramesWithoutPts:  pack.Manifest.Completeness.FramesWithoutPoints,
		HasIntensity:      pack.Manifest.HasIntensity,
		HasClassification: pack.Manifest.HasClassification,
	})
}
