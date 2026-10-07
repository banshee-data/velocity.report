package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/scene"
	"github.com/banshee-data/velocity.report/internal/security"
)

// A run's own recording, served as a scene export the shared player opens.
//
// The Tracks page used to draw a run from lidar_track_observations, which has
// no run column: every replay of a capture writes rows at the same capture
// timestamps, so a run's window returned thousands of other runs' tracks and
// no background. The run's VRLOG holds exactly its frames and a settled
// background, and the scene exporter already turns one into what the public
// survey pages play. This exports it on first request, caches the export
// beside the recordings, and serves its files.

// runSceneCacheDir is where exports are kept, inside the VRLOG directory. They
// are derived from the recordings and can be deleted at any time.
const runSceneCacheDir = ".scene-cache"

// runSceneStamp records what a cached export was made from, so a recording
// that grew or was replaced since is exported again.
type runSceneStamp struct {
	FormatVersion int    `json:"format_version"`
	VRLOGPath     string `json:"vrlog_path"`
	Signature     string `json:"signature"`
	Background    bool   `json:"background"`
	ExportedAt    string `json:"exported_at"`
}

// runSceneManifest is the one-part manifest SceneSession opens.
type runSceneManifest struct {
	Version int `json:"version"`
	Site    struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"site"`
	Parts []runScenePart `json:"parts"`
}

type runScenePart struct {
	URL          string  `json:"url"`
	StartSeconds float64 `json:"start_seconds"`
}

// runSceneLocks serialises exports per run, so the manifest, index and
// background requests a page makes together do not export the run three times.
var runSceneLocks sync.Map

// handleRunScene serves the files of a run's scene export.
//
// GET /api/lidar/runs/{run_id}/scene/manifest.json
// GET /api/lidar/runs/{run_id}/scene/part-000/{header,index,timeline}.json
// GET /api/lidar/runs/{run_id}/scene/part-000/frames/{chunk}
// GET /api/lidar/runs/{run_id}/scene/background/background.json.gz
//
// A run with no recording, or one outside the VRLOG directory, is a 404: the
// page then falls back to the database observations and says so.
func (ws *Server) handleRunScene(w http.ResponseWriter, r *http.Request, runID, rel string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "this endpoint only accepts GET requests")
		return
	}
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "database is not configured: check server startup includes --db-path")
		return
	}

	run, err := sqlite.NewAnalysisRunStore(ws.db).GetRun(runID)
	if errors.Is(err, sqlite.ErrNotFound) {
		ws.writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not retrieve run: %v", err))
		return
	}
	if run.VRLogPath == "" {
		ws.writeJSONError(w, http.StatusNotFound, "this run has no recording")
		return
	}
	// Read from the run record, so held to the same read boundary as replay.
	vrlogPath, err := security.ResolvePathWithinDirectory(run.VRLogPath, ws.vrlogSafeDir)
	if err != nil {
		ws.writeJSONError(w, http.StatusNotFound, "this run's recording is outside the VRLOG directory")
		return
	}
	if _, err := os.Stat(filepath.Join(vrlogPath, "header.json")); err != nil {
		ws.writeJSONError(w, http.StatusNotFound, "this run's recording is missing")
		return
	}

	dir, err := ws.ensureRunScene(runID, vrlogPath, runSceneTitle(run))
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("could not export the run's recording: %v", err))
		return
	}

	// Only files inside the export are served; never a directory listing.
	clean := strings.TrimPrefix(path.Clean("/"+rel), "/")
	if clean == "" || clean == "." {
		clean = "manifest.json"
	}
	if clean == "stamp.json" {
		// The cache's own bookkeeping, not part of the export.
		ws.writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	full := filepath.Join(dir, filepath.FromSlash(clean))
	if !strings.HasPrefix(full, dir+string(filepath.Separator)) {
		ws.writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		ws.writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	// An export is replaced when its recording changes, so it is revalidated
	// rather than cached as immutable.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, full)
}

// runSceneTitle names a run's scene by the capture it replayed.
func runSceneTitle(run *sqlite.AnalysisRun) string {
	if run.SourcePath != "" {
		return filepath.Base(run.SourcePath)
	}
	return run.RunID
}

// ensureRunScene returns the directory of an up-to-date export of the run's
// recording, exporting it first when there is none or the recording changed.
func (ws *Server) ensureRunScene(runID, vrlogPath, title string) (string, error) {
	lock, _ := runSceneLocks.LoadOrStore(runID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	root := filepath.Join(ws.vrlogSafeDir, runSceneCacheDir)
	dir := filepath.Join(root, runID)
	signature, err := vrlogSignature(vrlogPath)
	if err != nil {
		return "", err
	}
	if stamp, ok := readRunSceneStamp(dir); ok && stamp.FormatVersion == scene.FormatVersion &&
		stamp.VRLOGPath == vrlogPath && stamp.Signature == signature {
		return dir, nil
	}

	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create scene cache: %w", err)
	}
	tmp, err := os.MkdirTemp(root, runID+".tmp-")
	if err != nil {
		return "", fmt.Errorf("create scene export directory: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(tmp)
		}
	}()

	if _, err := scene.Export(scene.Options{
		VRLOGPath:    vrlogPath,
		OutDir:       filepath.Join(tmp, "part-000"),
		Kind:         scene.KindTracks,
		Title:        title,
		KeepTrackIDs: true,
	}); err != nil {
		return "", err
	}
	// A recording without a settled background still plays; the player draws
	// boxes over the grid and asks for a background that is not there.
	background := true
	if _, err := scene.ExportBackground(scene.Options{
		VRLOGPath: vrlogPath,
		OutDir:    filepath.Join(tmp, "background"),
		Title:     title,
	}); err != nil {
		background = false
		diagf("run %s: no background for its scene: %v", runID, err)
	}

	var manifest runSceneManifest
	manifest.Version = scene.FormatVersion
	manifest.Site.ID = runID
	manifest.Site.Title = title
	manifest.Parts = []runScenePart{{URL: "./part-000/", StartSeconds: 0}}
	if err := writeRunSceneJSON(filepath.Join(tmp, "manifest.json"), manifest); err != nil {
		return "", err
	}
	if err := writeRunSceneJSON(filepath.Join(tmp, "stamp.json"), runSceneStamp{
		FormatVersion: scene.FormatVersion,
		VRLOGPath:     vrlogPath,
		Signature:     signature,
		Background:    background,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return "", err
	}

	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("replace scene export: %w", err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		return "", fmt.Errorf("replace scene export: %w", err)
	}
	keep = true
	return dir, nil
}

// vrlogSignature fingerprints a recording by the names, sizes and modification
// times of its top-level entries. A recording that grows rewrites its index,
// so the signature changes; reading every byte to hash it would cost far more
// than the export it guards.
func vrlogSignature(vrlogPath string) (string, error) {
	entries, err := os.ReadDir(vrlogPath)
	if err != nil {
		return "", fmt.Errorf("read recording: %w", err)
	}
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return "", fmt.Errorf("read recording: %w", err)
		}
		lines = append(lines, fmt.Sprintf("%s %d %d", e.Name(), info.Size(), info.ModTime().UnixNano()))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:]), nil
}

func readRunSceneStamp(dir string) (runSceneStamp, bool) {
	var stamp runSceneStamp
	raw, err := os.ReadFile(filepath.Join(dir, "stamp.json"))
	if err != nil {
		return stamp, false
	}
	if err := json.Unmarshal(raw, &stamp); err != nil {
		return stamp, false
	}
	return stamp, true
}

func writeRunSceneJSON(file string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, raw, 0o644)
}
