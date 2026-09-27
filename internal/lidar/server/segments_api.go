package server

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/segments"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

type segmentRequest struct {
	RunID      string           `json:"run_id"`
	Finder     string           `json:"finder"`
	Role       string           `json:"role"`
	Parameters *segments.Params `json:"parameters,omitempty"`
}

func (ws *Server) handleSegmentFinders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, 405, "method not allowed")
		return
	}
	ws.writeJSON(w, 200, map[string]any{"finders": segments.Finders(), "defaults": segments.DefaultParams()})
}

func querySegmentRequest(r *http.Request) (segmentRequest, error) {
	q := r.URL.Query()
	req := segmentRequest{RunID: q.Get("run_id"), Finder: q.Get("finder"), Role: q.Get("role")}
	if req.Finder == "" {
		req.Finder = "following"
	}
	if req.Role == "" {
		req.Role = "tuning"
	}
	p := segments.DefaultParams()
	if raw := q.Get("window_seconds"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return req, err
		}
		p.WindowSeconds = v
	}
	if raw := q.Get("seed"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return req, err
		}
		p.RandomSeed = v
	}
	req.Parameters = &p
	return req, nil
}

func (ws *Server) findRunSegments(req segmentRequest) ([]segments.Window, segments.Params, error) {
	p := segments.DefaultParams()
	if req.Parameters != nil {
		p = *req.Parameters
	}
	if req.RunID == "" {
		return nil, p, fmt.Errorf("run_id is required")
	}
	if req.Finder == "" {
		req.Finder = "following"
	}
	if req.Role == "" {
		req.Role = "tuning"
	}
	if !segments.Allowed(req.Finder, req.Role) {
		return nil, p, fmt.Errorf("finder %q cannot choose a %s window", req.Finder, req.Role)
	}
	run, err := sqlite.NewAnalysisRunStore(ws.db).GetRun(req.RunID)
	if err != nil {
		return nil, p, fmt.Errorf("run not found: %w", err)
	}
	points, err := segments.LoadRun(ws.db.DB, req.RunID)
	if err != nil {
		return nil, p, err
	}
	captures := []segments.Capture{}
	if req.Finder == "random" {
		// A random held-out window must be drawn from the capture timeline,
		// including spans for which the tracker produced no observations.
		captures, err = ws.capturesForRun(run, 0, 1<<63-1)
		if err != nil {
			return nil, p, err
		}
	} else if len(points) > 0 {
		captures, err = ws.capturesForRun(run, points[0].TimeNs, points[len(points)-1].TimeNs)
		if err != nil {
			return nil, p, err
		}
	}
	windows, err := segments.Find(points, req.Finder, req.RunID, req.Role, p, captures)
	if err != nil {
		return nil, p, err
	}
	for i := range windows {
		var caseID, jobID, jobState, packDir string
		err = ws.db.QueryRow(`SELECT s.replay_case_id,COALESCE(j.job_id,''),COALESCE(j.state,''),COALESCE(c.pack_dir,'') FROM lidar_segment_selections s LEFT JOIN lidar_segment_clip_jobs c ON c.segment_id=s.segment_id LEFT JOIN lidar_capture_jobs j ON j.job_id=c.job_id WHERE s.segment_id=? ORDER BY j.queued_at_ns DESC LIMIT 1`, windows[i].ID).Scan(&caseID, &jobID, &jobState, &packDir)
		if err == nil {
			windows[i].Status = "case"
			windows[i].ReplayCaseID = caseID
			windows[i].JobID = jobID
			windows[i].PackDir = packDir
			if jobState == "queued" || jobState == "running" {
				windows[i].Status = "clipping"
			}
			if packDir != "" {
				windows[i].Status = "packed"
			}
		}
	}
	return windows, p, nil
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
	windows, p, err := ws.findRunSegments(req)
	if err != nil {
		ws.writeJSONError(w, 400, err.Error())
		return
	}
	ws.writeJSON(w, 200, map[string]any{"windows": windows, "count": len(windows), "parameters": p, "run_id": req.RunID, "finder": req.Finder, "role": req.Role})
}

// A compact per-capture score strip. It contains no labels or truth state.
func (ws *Server) handleSegmentStrip(w http.ResponseWriter, r *http.Request) {
	ws.handleSegmentStripWith(w, r, ws.findRunSegments)
}

func (ws *Server) handleSegmentStripWith(w http.ResponseWriter, r *http.Request, find func(segmentRequest) ([]segments.Window, segments.Params, error)) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, 405, "method not allowed")
		return
	}
	req, err := querySegmentRequest(r)
	if err != nil {
		ws.writeJSONError(w, 400, err.Error())
		return
	}
	windows, _, err := find(req)
	if err != nil {
		ws.writeJSONError(w, 400, err.Error())
		return
	}
	if err := writeSegmentStrip(w, windows); err != nil {
		ws.writeJSONError(w, 413, err.Error())
	}
}

func writeSegmentStrip(w http.ResponseWriter, windows []segments.Window) error {
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
				alpha = 0.2 + 0.8*math.Sqrt(s.Score/maxScore)
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
	// Apply the defaults here as well as in findRunSegments, which works on a
	// copy: the held-out guard and the stored row must name the finder and role
	// that ranked the window, or the clip job refuses the selection as drifted.
	if req.Finder == "" {
		req.Finder = "following"
	}
	if req.Role == "" {
		req.Role = "tuning"
	}
	windows, p, err := ws.findRunSegments(req)
	if err != nil {
		ws.writeJSONError(w, 400, err.Error())
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
	if req.Role == "held_out" && req.Finder != "random" {
		var randomCases int
		if err := ws.db.QueryRow(`SELECT COUNT(*) FROM lidar_segment_selections WHERE role='held_out' AND finder='random' AND json_extract(window_json,'$.capture')=?`, chosen.Capture).Scan(&randomCases); err != nil {
			ws.writeJSONError(w, 500, err.Error())
			return
		}
		if randomCases == 0 {
			ws.writeJSONError(w, 400, "choose a random held-out window from this capture before a traffic window")
			return
		}
	}
	if chosen.Status != "candidate" {
		var caseID string
		if err := ws.db.QueryRow(`SELECT replay_case_id FROM lidar_segment_selections WHERE segment_id=?`, chosen.ID).Scan(&caseID); err == nil {
			ws.writeJSON(w, 200, map[string]any{"replay_case_id": caseID, "segment": chosen})
			return
		}
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
	paramsJSON, _ := json.Marshal(p)
	windowJSON, _ := json.Marshal(chosen)
	_, err = ws.db.Exec(`INSERT INTO lidar_segment_selections(segment_id,run_id,replay_case_id,role,finder,parameters_json,window_json,created_at_ns) VALUES(?,?,?,?,?,?,?,?)`, chosen.ID, req.RunID, scene.ReplayCaseID, chosen.Role, chosen.Finder, string(paramsJSON), string(windowJSON), time.Now().UnixNano())
	if err != nil {
		_ = store.DeleteScene(scene.ReplayCaseID)
		ws.writeJSONError(w, 500, err.Error())
		return
	}
	ws.writeJSON(w, 201, map[string]any{"replay_case_id": scene.ReplayCaseID, "segment": chosen})
}
