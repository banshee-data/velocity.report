package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/banshee-data/velocity.report/internal/lidar/capindex"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// captureScanMu serialises scans. A scan reads whole capture files, and two of
// them racing over one volume would double the I/O to produce the same index.
var captureScanMu sync.Mutex

// normaliseCaptureRoots merges the safe directory with any extra configured
// roots, resolving each to an absolute path and dropping duplicates and blanks.
// The safe directory comes first so an unchanged deployment behaves exactly as
// it did before roots existed.
func normaliseCaptureRoots(safeDir string, extra []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, candidate := range append([]string{safeDir}, extra...) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, dup := seen[abs]; dup {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}

// CaptureRoots returns the configured capture roots.
func (ws *Server) CaptureRoots() []string {
	return append([]string(nil), ws.captureRoots...)
}

// captureStore returns the capture index store, or an error when the server has
// no database.
func (ws *Server) captureStore() (*sqlite.CaptureStore, error) {
	if ws.db == nil {
		return nil, fmt.Errorf("capture index unavailable: no database configured")
	}
	return sqlite.NewCaptureStore(ws.db), nil
}

// syncCaptureRoots records the configured roots in the index. Roots that were
// configured previously and no longer are stay in the index as disabled rows
// rather than being deleted, so the files indexed under them keep their
// provenance.
func (ws *Server) syncCaptureRoots() error {
	store, err := ws.captureStore()
	if err != nil {
		return err
	}
	configured := map[string]struct{}{}
	for _, path := range ws.captureRoots {
		if _, err := store.UpsertRoot(path, "", true); err != nil {
			return err
		}
		configured[path] = struct{}{}
	}
	known, err := store.ListRoots()
	if err != nil {
		return err
	}
	for _, r := range known {
		if _, ok := configured[r.Path]; ok || !r.Enabled {
			continue
		}
		if _, err := store.UpsertRoot(r.Path, r.Label, false); err != nil {
			return err
		}
	}
	return nil
}

// handleCaptureRoots lists the configured capture volumes and their scan state.
//
// GET /api/lidar/capture/roots
func (ws *Server) handleCaptureRoots(w http.ResponseWriter, r *http.Request) {
	store, err := ws.captureStore()
	if err != nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := ws.syncCaptureRoots(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	roots, err := store.ListRoots()
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCaptureJSON(w, map[string]any{"roots": roots, "count": len(roots)})
}

// handleCaptureScan re-indexes one root, or every configured root when no
// root_id is given, and returns the drift it found.
//
// POST /api/lidar/capture/scan?root_id=…&probe=false
//
// Probing reads every byte of every new capture, so it is opt-out for a
// deliberate scan and would be the wrong default for anything automatic.
func (ws *Server) handleCaptureScan(w http.ResponseWriter, r *http.Request) {
	store, err := ws.captureStore()
	if err != nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := ws.syncCaptureRoots(); err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	probe := r.URL.Query().Get("probe") != "false"
	wanted := r.URL.Query().Get("root_id")

	roots, err := store.ListRoots()
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	captureScanMu.Lock()
	defer captureScanMu.Unlock()

	type rootResult struct {
		RootID   string                  `json:"root_id"`
		Path     string                  `json:"path"`
		State    string                  `json:"state"`
		Error    string                  `json:"error,omitempty"`
		Drift    string                  `json:"drift"`
		Added    int                     `json:"added"`
		Missing  int                     `json:"missing"`
		Changed  int                     `json:"changed"`
		Probed   int                     `json:"probed"`
		Failed   int                     `json:"probe_failed"`
		Sessions []sqlite.CaptureSession `json:"sessions,omitempty"`
	}
	results := []rootResult{}

	for _, root := range roots {
		if wanted != "" && root.RootID != wanted {
			continue
		}
		if !root.Enabled {
			continue
		}
		ix := &capindex.Indexer{
			RootID:   root.RootID,
			RootPath: root.Path,
			Store:    store,
			UDPPort:  ws.udpPort,
		}
		if probe {
			ix.Probe = captureProber
		}
		res, refreshErr := ix.Refresh(r.Context())
		out := rootResult{
			RootID: root.RootID, Path: root.Path, State: res.State,
			Drift: res.Drift.Summary(), Added: len(res.Drift.Added),
			Missing: len(res.Drift.Missing), Changed: len(res.Drift.Changed),
			Probed: res.Probed, Failed: res.ProbeFailed,
		}
		if refreshErr != nil {
			out.Error = refreshErr.Error()
			results = append(results, out)
			continue
		}
		sessions, deriveErr := store.DeriveSessions(root.RootID)
		if deriveErr != nil {
			out.Error = deriveErr.Error()
		} else {
			out.Sessions = sessions
		}
		results = append(results, out)
	}

	if wanted != "" && len(results) == 0 {
		ws.writeJSONError(w, http.StatusNotFound, "no such capture root")
		return
	}
	writeCaptureJSON(w, map[string]any{"roots": results, "count": len(results)})
}

// handleCaptureSessions lists derived sessions, newest first.
//
// GET /api/lidar/capture/sessions?root_id=…
func (ws *Server) handleCaptureSessions(w http.ResponseWriter, r *http.Request) {
	store, err := ws.captureStore()
	if err != nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	sessions, err := store.ListSessions(r.URL.Query().Get("root_id"))
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCaptureJSON(w, map[string]any{"sessions": sessions, "count": len(sessions)})
}

// handleCaptureFiles lists indexed capture files.
//
// GET /api/lidar/capture/files?root_id=…&session_id=…
func (ws *Server) handleCaptureFiles(w http.ResponseWriter, r *http.Request) {
	store, err := ws.captureStore()
	if err != nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	var files []sqlite.CaptureFile
	if sessionID := r.URL.Query().Get("session_id"); sessionID != "" {
		files, err = store.SessionFiles(sessionID)
	} else {
		files, err = store.ListFiles(r.URL.Query().Get("root_id"))
	}
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCaptureJSON(w, map[string]any{"files": files, "count": len(files)})
}

// handleCaptureSessionLabel names a session — the site it was captured at,
// which is the one thing about a derived session an operator supplies.
//
// POST /api/lidar/capture/session/label
func (ws *Server) handleCaptureSessionLabel(w http.ResponseWriter, r *http.Request) {
	store, err := ws.captureStore()
	if err != nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		Label     string `json:"label"`
		SensorID  string `json:"sensor_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.SessionID == "" {
		ws.writeJSONError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	if err := store.SetSessionLabel(req.SessionID, req.Label, req.SensorID); err != nil {
		if err == sqlite.ErrNotFound {
			ws.writeJSONError(w, http.StatusNotFound, "no such session")
			return
		}
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeCaptureJSON(w, map[string]any{"session_id": req.SessionID, "label": req.Label})
}

func writeCaptureJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// captureProber is the extent prober used by the API. It is a variable so tests
// can supply one without libpcap.
var captureProber capindex.Prober = defaultCaptureProber

// probeExtent obtains a capture's packet-time extent. It is the single way
// this package learns one — the index, the motion pass and case validation all
// go through it — and a variable so tests can supply extents without libpcap.
var probeExtent = probeCaptureExtent

// defaultCaptureProber is the indexer's view of probeExtent.
func defaultCaptureProber(absPath string, udpPort int) (capindex.Extent, error) {
	return probeExtent(absPath, udpPort)
}
