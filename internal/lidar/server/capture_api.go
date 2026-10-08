package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capindex"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// captureScanMu serialises scans. A scan reads whole capture files, and two of
// them racing over one volume would double the I/O to produce the same index.
var captureScanMu sync.Mutex

// captureScanSchedule is what this process knows about scans: which roots
// have a background scan scheduled or running, how far each has got, and how
// the last scan of each ended. None of it is persisted. A restart cannot
// truthfully claim a cancelled probe continues, and the outcome of the last
// scan is on the root row in the shape that matters (state, error, time).
var captureScanSchedule = struct {
	sync.Mutex
	roots    map[string]bool
	progress map[string]*captureScanProgress
	outcomes map[string]captureScanOutcome
}{
	roots:    map[string]bool{},
	progress: map[string]*captureScanProgress{},
	outcomes: map[string]captureScanOutcome{},
}

const captureScanStateRunning = "scanning"

// Phases a background scan of one root passes through.
const (
	// captureScanQueued waits for another scan to release the volume lock.
	captureScanQueued = "queued"
	// captureScanListing walks the volume and records what changed.
	captureScanListing = "listing"
	// captureScanProbing reads captures in full for their packet extents.
	captureScanProbing = "probing"
	// captureScanDeriving rebuilds the root's sessions from what was probed.
	captureScanDeriving = "deriving"
)

// captureScanProgress is how far a background scan of one root has got. It
// exists from when the scan is scheduled until it ends.
type captureScanProgress struct {
	Phase string `json:"phase"`
	// Done is how many captures the probe has finished with, and Total how
	// many it set out to read. Both stay zero until listing has found them.
	Done  int `json:"done"`
	Total int `json:"total"`
	// Current is the capture being read now.
	Current string `json:"current,omitempty"`
	// Probed and Failed split Done by outcome.
	Probed int `json:"probed"`
	Failed int `json:"probe_failed"`
	// StartedAtNs is when this root's scan began (or was queued, while it is
	// queued); ProbeStartedAtNs when its first capture read began, which is
	// the base for a rate, since listing a slow volume takes time of its own.
	StartedAtNs      int64 `json:"started_at_ns"`
	ProbeStartedAtNs int64 `json:"probe_started_at_ns,omitempty"`
	UpdatedAtNs      int64 `json:"updated_at_ns"`
	// ElapsedNs and ProbeElapsedNs are measured by this process when the root
	// is listed, so a browser whose clock disagrees with it still shows the
	// right times.
	ElapsedNs      int64 `json:"elapsed_ns"`
	ProbeElapsedNs int64 `json:"probe_elapsed_ns,omitempty"`
}

// captureScanOutcome is how the last scan of a root ended, kept so a page
// that was not watching when a background scan finished can still say what
// it found.
type captureScanOutcome struct {
	captureScanResult
	Probe        bool  `json:"probe"`
	StartedAtNs  int64 `json:"started_at_ns"`
	FinishedAtNs int64 `json:"finished_at_ns"`
}

// captureRootView is a root as GET /api/lidar/capture/roots lists it: the
// stored row plus what this process knows about scans of it.
type captureRootView struct {
	sqlite.CaptureRoot
	ScanProgress   *captureScanProgress `json:"scan_progress,omitempty"`
	LastScanResult *captureScanOutcome  `json:"last_scan_result,omitempty"`
}

type captureScanResult struct {
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
	views := captureRootViews(roots, time.Now())
	writeCaptureJSON(w, map[string]any{"roots": views, "count": len(views)})
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
	background := probe && r.URL.Query().Get("async") == "true"
	wanted := r.URL.Query().Get("root_id")

	roots, err := store.ListRoots()
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if wanted != "" && !hasEnabledCaptureRoot(roots, wanted) {
		ws.writeJSONError(w, http.StatusNotFound, "no such capture root")
		return
	}
	if background {
		results, claimed := scheduledCaptureScans(roots, wanted)
		if len(claimed) > 0 {
			go ws.refreshCaptureRoots(claimed)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		writeCaptureJSON(w, map[string]any{"roots": results, "count": len(results)})
		return
	}

	results := ws.refreshCaptureRootsWithContext(r.Context(), roots, wanted, probe, false)

	if wanted != "" && len(results) == 0 {
		ws.writeJSONError(w, http.StatusNotFound, "no such capture root")
		return
	}
	writeCaptureJSON(w, map[string]any{"roots": results, "count": len(results)})
}

func hasEnabledCaptureRoot(roots []sqlite.CaptureRoot, wanted string) bool {
	for _, root := range roots {
		if root.RootID == wanted && root.Enabled {
			return true
		}
	}
	return false
}

// scheduledCaptureScans makes the action visible before the goroutine starts,
// so the UI can poll normal root state instead of holding an HTTP request open
// for a multi-hour first probe of an archive volume.
//
// It returns the roots this call claimed: those with no scan already
// scheduled. Only they are handed to the new goroutine, so asking for every
// root while one is already being probed neither probes that one twice nor
// lets the first scan to finish clear the flag of a scan still waiting.
func scheduledCaptureScans(roots []sqlite.CaptureRoot, wanted string) ([]captureScanResult, []sqlite.CaptureRoot) {
	results := []captureScanResult{}
	var claimed []sqlite.CaptureRoot
	now := time.Now().UnixNano()
	captureScanSchedule.Lock()
	defer captureScanSchedule.Unlock()
	for _, root := range roots {
		if !root.Enabled || (wanted != "" && root.RootID != wanted) {
			continue
		}
		drift := "a scan and probe is already running"
		if !captureScanSchedule.roots[root.RootID] {
			captureScanSchedule.roots[root.RootID] = true
			captureScanSchedule.progress[root.RootID] = &captureScanProgress{
				Phase: captureScanQueued, StartedAtNs: now, UpdatedAtNs: now,
			}
			claimed = append(claimed, root)
			drift = "scan and probe queued"
		}
		results = append(results, captureScanResult{
			RootID: root.RootID, Path: root.Path, State: captureScanStateRunning,
			Drift: drift,
		})
	}
	return results, claimed
}

// refreshCaptureRoots scans and probes the roots a scheduling call claimed,
// reporting its progress as it goes.
func (ws *Server) refreshCaptureRoots(claimed []sqlite.CaptureRoot) {
	defer clearScheduledRoots(claimed)
	ws.refreshCaptureRootsWithContext(context.Background(), claimed, "", true, true)
}

// captureRootViews adds this process's scan state to each root: whether a
// scan is running, how far it has got, and how the last one ended. Elapsed
// times are measured against now so the client need not trust its own clock.
func captureRootViews(roots []sqlite.CaptureRoot, now time.Time) []captureRootView {
	nowNs := now.UnixNano()
	captureScanSchedule.Lock()
	defer captureScanSchedule.Unlock()
	views := make([]captureRootView, 0, len(roots))
	for _, root := range roots {
		root.ScanInProgress = captureScanSchedule.roots[root.RootID]
		view := captureRootView{CaptureRoot: root}
		if p := captureScanSchedule.progress[root.RootID]; p != nil && root.ScanInProgress {
			snapshot := *p
			snapshot.ElapsedNs = max(0, nowNs-snapshot.StartedAtNs)
			if snapshot.ProbeStartedAtNs > 0 {
				snapshot.ProbeElapsedNs = max(0, nowNs-snapshot.ProbeStartedAtNs)
			}
			view.ScanProgress = &snapshot
		}
		if o, ok := captureScanSchedule.outcomes[root.RootID]; ok {
			view.LastScanResult = &o
		}
		views = append(views, view)
	}
	return views
}

// clearScheduledRoots forgets the scheduled scans of the given roots and
// their progress. A root that finished normally has already been cleared;
// this covers a pass that stopped before reaching it.
func clearScheduledRoots(roots []sqlite.CaptureRoot) {
	captureScanSchedule.Lock()
	defer captureScanSchedule.Unlock()
	for _, root := range roots {
		delete(captureScanSchedule.roots, root.RootID)
		delete(captureScanSchedule.progress, root.RootID)
	}
}

// updateCaptureScanProgress applies change to a root's progress under the
// schedule lock. A root with no scan scheduled has no progress to update.
func updateCaptureScanProgress(rootID string, change func(p *captureScanProgress, nowNs int64)) {
	captureScanSchedule.Lock()
	defer captureScanSchedule.Unlock()
	p := captureScanSchedule.progress[rootID]
	if p == nil {
		return
	}
	now := time.Now().UnixNano()
	change(p, now)
	p.UpdatedAtNs = now
}

// setCaptureScanPhase moves a root's background scan to a new phase.
// Entering listing restarts the clock, because time spent queued behind
// another scan is not time spent on this one.
func setCaptureScanPhase(rootID, phase string) {
	updateCaptureScanProgress(rootID, func(p *captureScanProgress, nowNs int64) {
		p.Phase = phase
		if phase == captureScanListing {
			p.StartedAtNs = nowNs
		}
	})
}

// recordCaptureProbeProgress is the indexer's progress callback for one root.
func recordCaptureProbeProgress(rootID string, ip capindex.Progress) {
	updateCaptureScanProgress(rootID, func(p *captureScanProgress, nowNs int64) {
		p.Phase = captureScanProbing
		if p.ProbeStartedAtNs == 0 {
			p.ProbeStartedAtNs = nowNs
		}
		p.Done, p.Total, p.Current = ip.Done, ip.Total, ip.Current
		p.Probed, p.Failed = ip.Probed, ip.Failed
	})
}

// finishCaptureScan records how a root's scan ended. For a background scan
// it also ends that root's visible scan state at once, rather than when
// every root the pass covers is done, so a page watching one volume sees it
// finish when it does.
func finishCaptureScan(result captureScanResult, probe bool, startedAt time.Time, background bool) {
	result.Sessions = nil
	outcome := captureScanOutcome{
		captureScanResult: result, Probe: probe,
		StartedAtNs: startedAt.UnixNano(), FinishedAtNs: time.Now().UnixNano(),
	}
	captureScanSchedule.Lock()
	defer captureScanSchedule.Unlock()
	captureScanSchedule.outcomes[result.RootID] = outcome
	if background {
		delete(captureScanSchedule.roots, result.RootID)
		delete(captureScanSchedule.progress, result.RootID)
	}
}

// refreshCaptureRootsWithContext scans the enabled roots wanted (every one
// when wanted is empty), probing when asked. A background pass reports its
// progress per root as it goes.
func (ws *Server) refreshCaptureRootsWithContext(ctx context.Context, roots []sqlite.CaptureRoot, wanted string, probe, background bool) []captureScanResult {
	captureScanMu.Lock()
	defer captureScanMu.Unlock()

	store, err := ws.captureStore()
	if err != nil {
		return nil
	}
	results := []captureScanResult{}
	for _, root := range roots {
		if !root.Enabled || (wanted != "" && root.RootID != wanted) {
			continue
		}
		startedAt := time.Now()
		ix := &capindex.Indexer{RootID: root.RootID, RootPath: root.Path, Store: store, UDPPort: ws.udpPort}
		if probe {
			ix.Probe = captureProber
		}
		if background {
			setCaptureScanPhase(root.RootID, captureScanListing)
			rootID := root.RootID
			ix.OnProgress = func(p capindex.Progress) { recordCaptureProbeProgress(rootID, p) }
		}
		res, refreshErr := ix.Refresh(ctx)
		out := captureScanResult{
			RootID: root.RootID, Path: root.Path, State: res.State,
			Drift: res.Drift.Summary(), Added: len(res.Drift.Added), Missing: len(res.Drift.Missing),
			Changed: len(res.Drift.Changed), Probed: res.Probed, Failed: res.ProbeFailed,
		}
		if refreshErr != nil {
			out.Error = refreshErr.Error()
			finishCaptureScan(out, probe, startedAt, background)
			results = append(results, out)
			continue
		}
		if background {
			setCaptureScanPhase(root.RootID, captureScanDeriving)
		}
		if sessions, deriveErr := store.DeriveSessions(root.RootID); deriveErr != nil {
			out.Error = deriveErr.Error()
		} else {
			out.Sessions = sessions
		}
		finishCaptureScan(out, probe, startedAt, background)
		results = append(results, out)
	}
	return results
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
