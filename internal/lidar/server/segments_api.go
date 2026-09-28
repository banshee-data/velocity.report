package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/segments"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/security"
)

type segmentRequest struct {
	RunID string `json:"run_id"`
	// Selector names the selector to rank with. Finder names a finder's
	// standard selector, as it did before selectors existed.
	Selector string `json:"selector"`
	Finder   string `json:"finder"`
	Role     string `json:"role"`
	// Parameters replaces the selector's own, for a tuning window only: a
	// held-out window is chosen at its selector's own parameters.
	Parameters *segments.Params `json:"parameters,omitempty"`
	// SelectorDigest is the digest of the selector the page ranked with. A
	// case is refused when the selector has changed since.
	SelectorDigest string `json:"selector_digest,omitempty"`
	// windowSeconds and seed are a query's overrides of single parameters.
	windowSeconds *float64
	seed          *int64
}

// selectorID is the selector a request names, following's by default.
func (req segmentRequest) selectorID() (string, error) {
	switch {
	case req.Selector != "" && req.Finder != "":
		return "", fmt.Errorf("name a selector or a finder, not both")
	case req.Selector != "":
		return req.Selector, nil
	case req.Finder != "":
		return req.Finder, nil
	}
	return "following", nil
}

func (ws *Server) handleSegmentFinders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, 405, "method not allowed")
		return
	}
	ws.writeJSON(w, 200, map[string]any{"finders": segments.Finders(), "defaults": segments.DefaultParams()})
}

// handleSegmentSelectors lists the selectors the server ranks with, in the
// file's order, with each one's digest and whether it may choose held-out
// windows.
func (ws *Server) handleSegmentSelectors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, 405, "method not allowed")
		return
	}
	catalogue, err := ws.selectorCatalogue()
	if err != nil {
		ws.writeJSONError(w, 500, err.Error())
		return
	}
	ws.writeJSON(w, 200, map[string]any{"version": catalogue.Version, "digest": catalogue.Digest, "source": catalogue.Source, "selectors": catalogue.Listing()})
}

func querySegmentRequest(r *http.Request) (segmentRequest, error) {
	q := r.URL.Query()
	req := segmentRequest{RunID: q.Get("run_id"), Selector: q.Get("selector"), Finder: q.Get("finder"), Role: q.Get("role")}
	if req.Role == "" {
		req.Role = "tuning"
	}
	if raw := q.Get("window_seconds"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return req, err
		}
		req.windowSeconds = &v
	}
	if raw := q.Get("seed"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return req, err
		}
		req.seed = &v
	}
	return req, nil
}

// selectorCatalogue is the catalogue the server was started with, or the
// default one.
func (ws *Server) selectorCatalogue() (*segments.Catalogue, error) {
	if ws.segmentSelectors != nil {
		return ws.segmentSelectors, nil
	}
	return segments.DefaultCatalogue()
}

// requestSelector is the selector a request ranks with, at the parameters it
// gives. A finder's name is the id of its standard selector.
func (ws *Server) requestSelector(req segmentRequest) (segments.Selector, error) {
	id, err := req.selectorID()
	if err != nil {
		return segments.Selector{}, err
	}
	catalogue, err := ws.selectorCatalogue()
	if err != nil {
		return segments.Selector{}, fmt.Errorf("segment selectors: %w", err)
	}
	sel, ok := catalogue.Selector(id)
	if !ok {
		return segments.Selector{}, fmt.Errorf("unknown selector %q", id)
	}
	// A caller's parameters could steer even a traffic measure towards
	// tracker failure, so a held-out window takes none.
	if req.Role == "held_out" && (req.Parameters != nil || req.windowSeconds != nil || req.seed != nil) {
		return segments.Selector{}, fmt.Errorf("a held-out window is chosen at its selector's own parameters")
	}
	if req.Parameters != nil {
		sel.Parameters = *req.Parameters
	}
	if req.windowSeconds != nil {
		sel.Parameters.WindowSeconds = *req.windowSeconds
	}
	if req.seed != nil {
		sel.Parameters.RandomSeed = *req.seed
	}
	return sel, nil
}

// findRunSegments ranks a run's windows, and returns them with the selector
// that ranked them as it ran.
func (ws *Server) findRunSegments(req segmentRequest) ([]segments.Window, segments.Selector, error) {
	if req.RunID == "" {
		return nil, segments.Selector{}, fmt.Errorf("run_id is required")
	}
	if req.Role == "" {
		req.Role = "tuning"
	}
	sel, err := ws.requestSelector(req)
	if err != nil {
		return nil, sel, err
	}
	if !segments.Allowed(sel.Finder, req.Role) {
		return nil, sel, fmt.Errorf("finder %q cannot choose a %s window", sel.Finder, req.Role)
	}
	run, err := sqlite.NewAnalysisRunStore(ws.db).GetRun(req.RunID)
	if err != nil {
		return nil, sel, fmt.Errorf("run not found: %w", err)
	}
	points, err := ws.loadRunSeries(run)
	if err != nil {
		return nil, sel, err
	}
	captures := []segments.Capture{}
	if sel.Finder == "random" {
		// A random held-out window must be drawn from the capture timeline,
		// including spans for which the tracker produced no observations.
		captures, err = ws.capturesForRun(run, 0, 1<<63-1)
		if err != nil {
			return nil, sel, err
		}
	} else if len(points) > 0 {
		captures, err = ws.capturesForRun(run, points[0].TimeNs, points[len(points)-1].TimeNs)
		if err != nil {
			return nil, sel, err
		}
	}
	windows, err := segments.Rank(points, sel, req.RunID, req.Role, captures)
	if err != nil {
		return nil, sel, err
	}
	store := sqlite.NewSegmentStore(ws.db)
	for i := range windows {
		status, err := store.Status(windows[i].ID)
		if errors.Is(err, sqlite.ErrNotFound) {
			continue
		}
		if err != nil {
			// A window whose state cannot be read must not be offered as a
			// fresh candidate: choosing it again would make a second case.
			return nil, sel, fmt.Errorf("read segment status: %w", err)
		}
		windows[i].Status = "case"
		windows[i].ReplayCaseID = status.ReplayCaseID
		windows[i].JobID = status.JobID
		if status.JobState == sqlite.JobQueued || status.JobState == sqlite.JobRunning {
			windows[i].Status = "clipping"
		}
		if status.PackDir != "" {
			windows[i].Status = "packed"
			windows[i].PackDir = ws.segmentPackPath(status.PackDir)
		}
	}
	return windows, sel, nil
}

// loadRunSeries reads where a run's tracks were. A live run stores its
// observations; an analysis replay does not write to that table and keeps its
// tracks in the run's recording, so a run without observations is read from
// there. A run with neither has nothing to rank, which is not an error.
func (ws *Server) loadRunSeries(run *sqlite.AnalysisRun) ([]segments.Point, error) {
	points, err := segments.LoadRun(ws.db.DB, run.RunID)
	if err != nil || len(points) > 0 || run.VRLogPath == "" {
		return points, err
	}
	// The path comes from the run record, so it is held to the same read
	// boundary as a replay of that recording.
	recording, err := security.ResolvePathWithinDirectory(run.VRLogPath, ws.vrlogSafeDir)
	if err != nil {
		return nil, fmt.Errorf("run's recording is not within the allowed directory: %w", err)
	}
	points, err = segments.LoadRunRecording(ws.db.DB, run.RunID, recording)
	if err != nil {
		return nil, fmt.Errorf("run has no stored observations and its recording could not be read: %w", err)
	}
	return points, nil
}

// Restrict the time index to the run's own source files. Two sensors can
// capture the same second; time overlap alone must never choose the other one.
func (ws *Server) capturesForRun(run *sqlite.AnalysisRun, start, end int64) ([]segments.Capture, error) {
	captures, err := segments.CapturesForRange(ws.db.DB, start, end)
	if err != nil {
		return nil, err
	}
	paths := []string{}
	if run.ReplayCaseID != "" {
		paths, err = sqlite.NewReplayCaseStore(ws.db).CasePaths(run.ReplayCaseID)
		if err != nil {
			return nil, err
		}
	}
	if len(paths) == 0 && run.SourcePath != "" {
		paths = []string{run.SourcePath}
	}
	allowed := map[string]bool{}
	for _, path := range paths {
		resolved, err := ws.resolveSegmentCapture(path)
		if err == nil {
			allowed[filepath.Clean(resolved)] = true
		}
	}
	filtered := make([]segments.Capture, 0, len(captures))
	for _, c := range captures {
		resolved, err := ws.resolveSegmentCapture(c.Path)
		if err == nil && allowed[filepath.Clean(resolved)] {
			filtered = append(filtered, c)
		}
	}
	return filtered, nil
}

func (ws *Server) resolveSegmentCapture(path string) (string, error) {
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(ws.pcapSafeDir, path)
		if err != nil {
			return "", err
		}
		path = rel
	}
	return ws.resolvePCAPPath(path)
}

func (ws *Server) handleSegments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, 405, "method not allowed")
		return
	}
	req, err := querySegmentRequest(r)
	if err != nil {
		ws.writeJSONError(w, 400, err.Error())
		return
	}
	windows, sel, err := ws.findRunSegments(req)
	if err != nil {
		ws.writeJSONError(w, 400, err.Error())
		return
	}
	ws.writeJSON(w, 200, map[string]any{"windows": windows, "count": len(windows), "parameters": sel.Parameters, "run_id": req.RunID,
		"finder": sel.Finder, "selector": sel.Provenance(), "role": req.Role})
}

// A compact per-capture score strip. It contains no labels or truth state.
func (ws *Server) handleSegmentStrip(w http.ResponseWriter, r *http.Request) {
	ws.handleSegmentStripWith(w, r, ws.findRunSegments)
}

func (ws *Server) handleSegmentStripWith(w http.ResponseWriter, r *http.Request, find func(segmentRequest) ([]segments.Window, segments.Selector, error)) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, 405, "method not allowed")
		return
	}
	req, err := querySegmentRequest(r)
	if err != nil {
		ws.writeJSONError(w, 400, err.Error())
		return
	}
	windows, sel, err := find(req)
	if err != nil {
		ws.writeJSONError(w, 400, err.Error())
		return
	}
	if err := writeSegmentStrip(w, windows, sel.Score.Order == "ascending"); err != nil {
		ws.writeJSONError(w, 413, err.Error())
	}
}

// writeSegmentStrip shades each window by its score, brightest for the best
// whichever way the selector ranks: a selector that puts the smallest first
// shades the smallest brightest.
func writeSegmentStrip(w http.ResponseWriter, windows []segments.Window, ascending bool) error {
	byCapture := map[string][]segments.Window{}
	names := []string{}
	maxScore := 0.0
	for _, s := range windows {
		key := s.Capture
		if key == "" {
			key = "unplaced"
		}
		if _, ok := byCapture[key]; !ok {
			names = append(names, key)
		}
		byCapture[key] = append(byCapture[key], s)
		if s.Score > maxScore {
			maxScore = s.Score
		}
	}
	if len(names) > 100 || len(windows) > 10000 {
		return fmt.Errorf("too many windows for strip")
	}
	sort.Strings(names)
	width := 800
	height := 24 + len(names)*32
	w.Header().Set("Content-Type", "image/svg+xml")
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" role="img"><title>Segment scores per capture</title><rect width="100%%" height="100%%" fill="#101827"/>`, width, height)
	for row, name := range names {
		cells := byCapture[name]
		sort.Slice(cells, func(i, j int) bool { return cells[i].StartNs < cells[j].StartNs })
		first, last := cells[0].StartNs, cells[len(cells)-1].StartNs
		label := filepath.Base(name)
		if len(label) > 32 {
			label = label[:29] + "..."
		}
		fmt.Fprintf(w, `<text x="8" y="%d" fill="white" font-size="11">%s</text>`, row*32+20, xmlEscape(label))
		for _, s := range cells {
			alpha := 0.2
			if maxScore > 0 {
				share := s.Score / maxScore
				if ascending {
					share = 1 - share
				}
				alpha = 0.2 + 0.8*math.Sqrt(share)
			}
			x := 210
			if last > first {
				x += int(float64(s.StartNs-first) / float64(last-first) * float64(width-220))
			}
			fmt.Fprintf(w, `<rect x="%d" y="%d" width="3" height="18" fill="#36c9b0" opacity="%.3f"><title>%.2f at %d</title></rect>`, x, row*32+5, alpha, s.Score, s.StartNs)
		}
	}
	fmt.Fprint(w, "</svg>")
	return nil
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return r.Replace(s)
}

func (ws *Server) handleSegmentByID(w http.ResponseWriter, r *http.Request) {
	ws.handleSegmentByIDWith(w, r, segmentCaseOperations{sqlite.NewAnalysisRunStore(ws.db).GetRun, ws.capturesForRun, ws.resolveSegmentCapture})
}

type segmentCaseOperations struct {
	getRun   func(string) (*sqlite.AnalysisRun, error)
	captures func(*sqlite.AnalysisRun, int64, int64) ([]segments.Capture, error)
	resolve  func(string) (string, error)
}

func (ws *Server) handleSegmentByIDWith(w http.ResponseWriter, r *http.Request, ops segmentCaseOperations) {
	path := strings.TrimPrefix(r.URL.Path, "/api/lidar/segments/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "case" || r.Method != http.MethodPost {
		ws.writeJSONError(w, 404, "endpoint not found")
		return
	}
	var req segmentRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		ws.writeJSONError(w, 400, "invalid request JSON")
		return
	}
	// Apply the default here as well as in findRunSegments, which works on a
	// copy: the held-out guard and the stored row must name the role that
	// ranked the window, or the clip job refuses the selection as drifted.
	// The finder and parameters come back with the selector that ranked it.
	if req.Role == "" {
		req.Role = "tuning"
	}
	windows, sel, err := ws.findRunSegments(req)
	if err != nil {
		ws.writeJSONError(w, 400, err.Error())
		return
	}
	// The page ranked with the selector it names. If the selector has
	// changed since, the window on the page is not the one that would be
	// stored.
	if req.SelectorDigest != "" && req.SelectorDigest != sel.Digest() {
		ws.writeJSONError(w, 409, "the selector has changed since this window was ranked: refresh the ranking")
		return
	}
	var chosen *segments.Window
	for i := range windows {
		if windows[i].ID == parts[0] {
			chosen = &windows[i]
			break
		}
	}
	if chosen == nil {
		ws.writeJSONError(w, 404, "segment not found for this run, finder and role")
		return
	}
	if chosen.Capture == "" {
		ws.writeJSONError(w, 400, "segment has no indexed capture offset")
		return
	}
	selections := sqlite.NewSegmentStore(ws.db)
	if req.Role == "held_out" && sel.Finder != "random" {
		found, err := selections.HasRandomHeldOut(chosen.Capture)
		if err != nil {
			ws.writeJSONError(w, 500, err.Error())
			return
		}
		if !found {
			ws.writeJSONError(w, 400, "choose a random held-out window from this capture before a traffic window")
			return
		}
	}
	if chosen.Status != "candidate" {
		ws.writeJSON(w, 200, map[string]any{"replay_case_id": chosen.ReplayCaseID, "segment": chosen})
		return
	}
	run, err := ops.getRun(req.RunID)
	if err != nil {
		ws.writeJSONError(w, 404, "run not found")
		return
	}
	captures, err := ops.captures(run, chosen.StartNs, chosen.EndNs-1)
	if err != nil || len(captures) == 0 {
		ws.writeJSONError(w, 400, "segment has no indexed capture sequence")
		return
	}
	if captures[0].FirstNs > chosen.StartNs || captures[len(captures)-1].LastNs < chosen.EndNs-1 {
		ws.writeJSONError(w, 400, "indexed captures do not cover the whole segment")
		return
	}
	paths := make([]string, 0, len(captures))
	for _, c := range captures {
		if _, err = ops.resolve(c.Path); err != nil {
			ws.writeJSONError(w, 400, err.Error())
			return
		}
		rel, err := filepath.Rel(ws.pcapSafeDir, c.Path)
		if err != nil {
			ws.writeJSONError(w, 400, err.Error())
			return
		}
		paths = append(paths, rel)
	}
	if len(paths) > 1 {
		if _, err = ws.validateCaseFiles(paths); err != nil {
			ws.writeJSONError(w, 400, err.Error())
			return
		}
	}
	start := float64(chosen.StartNs-captures[0].FirstNs) / 1e9
	duration := float64(chosen.EndNs-chosen.StartNs) / 1e9
	scene := &sqlite.ReplayCase{SensorID: run.SensorID, PCAPFile: paths[0], PCAPStartSecs: &start, PCAPDurationSecs: &duration, Description: "Annotation segment " + chosen.ID}
	store := sqlite.NewReplayCaseStore(ws.db)
	if err = store.InsertScene(scene); err != nil {
		ws.writeJSONError(w, 500, err.Error())
		return
	}
	caseFiles := make([]sqlite.ReplayCaseFile, 0, len(paths))
	for i, path := range paths {
		caseFiles = append(caseFiles, sqlite.ReplayCaseFile{Ordinal: i, PCAPFile: path})
	}
	if err = store.SetCaseFiles(scene.ReplayCaseID, caseFiles); err != nil {
		_ = store.DeleteScene(scene.ReplayCaseID)
		ws.writeJSONError(w, 500, err.Error())
		return
	}
	paramsJSON, _ := json.Marshal(sel.Parameters)
	windowJSON, _ := json.Marshal(chosen)
	selectorJSON, _ := json.Marshal(sel.Provenance())
	if err = selections.InsertSelection(chosen.ID, req.RunID, scene.ReplayCaseID, paramsJSON, windowJSON, selectorJSON); err != nil {
		_ = store.DeleteScene(scene.ReplayCaseID)
		ws.writeJSONError(w, 500, err.Error())
		return
	}
	ws.writeJSON(w, 201, map[string]any{"replay_case_id": scene.ReplayCaseID, "segment": chosen})
}
