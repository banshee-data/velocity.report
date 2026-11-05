package server

import (
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/labelworkdb"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/version"
)

// Development eligibility at the server's entry points.
//
// Every entry point that tunes, sweeps or runs an experiment arm asks the
// eligibility service before a sample loads, with the purpose the entry
// point implies: a client never names its own purpose. The service
// identifies each capture by the SHA-256 of its bytes, refuses what the
// cohort manifest or any frozen split protects, and records the decision.
//
//	GET  /api/lidar/eligibility          status, gaps, entry-point coverage
//	POST /api/lidar/eligibility/check    what a purpose would be decided, unrecorded
//	GET  /api/lidar/eligibility/uses     the use ledger, newest first

// actorHeader carries the name an operator gives. The server has no
// sign-in, so the ledger records it as stated, beside the remote address.
const actorHeader = "X-Velocity-Actor"

// EntryPointCoverage is one development entry point and whether the guard
// runs there.
type EntryPointCoverage struct {
	EntryPoint string `json:"entry_point"`
	Purpose    string `json:"purpose"`
	Enforced   string `json:"enforced"` // enforced, not_gated, not_yet
	Note       string `json:"note"`
}

// eligibilityCoverage is the audit of development entry points, kept here
// beside the guard calls so a change to one shows beside the other. The
// workspace guide reproduces it.
var eligibilityCoverage = []EntryPointCoverage{
	{"POST /api/lidar/sweep/start", "tuning", "enforced", "the PCAP or the live sensor, before the sweep starts"},
	{"POST /api/lidar/sweep/auto", "tuning", "enforced", "the PCAP or the live sensor, before the auto-tuner starts"},
	{"POST /api/lidar/sweep/hint", "tuning", "enforced", "every capture of the clip, before the HINT run starts"},
	{"POST /api/lidar/pcap/start (tuning_config under experiments/)", "development_evaluation", "enforced", "an experiment arm's replay, before its configuration is applied"},
	{"POST /api/lidar/pcap/start (otherwise)", "production", "not_gated", "ordinary replay, scene assets and publication keep their behaviour"},
	{"lidar-state-estimation-baseline", "development_evaluation", "enforced", "every case's captures, before they are read; a held-out score names its split"},
	{"lidar-refinement-eval", "development_evaluation", "enforced", "the case's capture, before it is replayed"},
	{"Labelling workspace detail", "developer_diagnostic", "enforced", "qualification items are served as counts only; no labels, objects or previews"},
	{"Other evaluators (lidar-box-eval, lidar-near-face-eval, lidar-ground-truth-eval, settling-eval, lidar-fg-diag, lidar-track-scorecard, lidar-evidence-oracle, lidar-closeness-audit, lidar-bench, vrlog-analyse)", "development_evaluation", "not_yet", "they read a split when given one; a run without one is not checked"},
	{"Tracks, Runs and scene pages", "developer_diagnostic", "not_yet", "they show any run the database holds"},
	{"velocity lidar annotation-split freeze / verify", "annotation_review", "not_gated", "the frozen-split rules apply as before"},
}

// eligibilityService is the server's guard, built on first use.
func (ws *Server) eligibilityService() *eligibility.Service {
	ws.eligibilityOnce.Do(func() {
		cfg := eligibility.Config{ManifestPath: ws.cohortManifestPath}
		if ws.annotationPacksDir != "" {
			cfg.SplitsDir = filepath.Join(ws.annotationPacksDir, splitsDirName)
			cfg.ExtraHashes = ws.labelSetCaptureHashes
		}
		if ws.db != nil {
			cfg.Ledger = labelworkdb.UseLedger{DB: ws.db.DB}
		}
		ws.eligibility = eligibility.New(cfg)
	})
	return ws.eligibility
}

// labelSetCaptureHashes are the capture files each pack's label set has
// verified, by pack digest: a capture bound after its pack was frozen still
// protects renamed copies.
func (ws *Server) labelSetCaptureHashes() map[string][]string {
	out := map[string][]string{}
	for _, p := range ws.listPacks() {
		for _, c := range p.LabelSetCaptures {
			if c.SHA256 != "" && p.PackDigest != "" {
				out[p.PackDigest] = append(out[p.PackDigest], c.SHA256)
			}
		}
	}
	return out
}

// listPacks is every pack's listing, from the cache the packs API keeps.
func (ws *Server) listPacks() []packListing {
	if ws.annotationPacksDir == "" {
		return nil
	}
	dirs, err := annotation.ListPackDirs(ws.annotationPacksDir)
	if err != nil {
		return nil
	}
	return ws.packListings.list(dirs, time.Now())
}

// actorOf is the requester as the ledger records it.
func actorOf(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	stated := strings.TrimSpace(r.Header.Get(actorHeader))
	if len(stated) > 100 {
		stated = stated[:100]
	}
	if stated == "" {
		return "http:" + host
	}
	return fmt.Sprintf("http:%s (stated: %s)", host, stated)
}

// captureInputs identifies the named PCAP files beneath the PCAP directory.
// A file that cannot be resolved or hashed is an input whose identity is
// unknown, which the decision refuses.
func (ws *Server) captureInputs(files ...string) []eligibility.Input {
	svc := ws.eligibilityService()
	var out []eligibility.Input
	for _, f := range files {
		path, err := ws.resolvePCAPPath(f)
		if err != nil {
			out = append(out, eligibility.Input{Identity: eligibility.Identity{Kind: eligibility.KindCapture, Basename: filepath.Base(f)}, Err: err})
			continue
		}
		// Replays are bounded by offsets in seconds, not sample times, so
		// the whole capture is the stretch: the stricter reading.
		out = append(out, svc.CaptureInput(path, 0, 0))
	}
	return out
}

// authorizeDevelopment decides a development request before any sample
// loads. A refusal is written as 403 with every reason, and false returned.
func (ws *Server) authorizeDevelopment(w http.ResponseWriter, r *http.Request, req eligibility.Request) bool {
	req.EntryPoint = r.Method + " " + r.URL.Path
	req.Actor = actorOf(r)
	req.BuildVersion, req.BuildGitSHA = version.Version, version.GitSHA
	d, err := ws.eligibilityService().Authorize(req)
	if err != nil {
		ws.writeJSON(w, http.StatusForbidden, map[string]any{
			"error": err.Error(), "code": "not_eligible", "decision": d,
		})
		return false
	}
	return true
}

// sweepInputs are the inputs of a sweep or auto-tune request body: its PCAP,
// or the live sensor.
func (ws *Server) sweepInputs(req map[string]any) []eligibility.Input {
	source, _ := req["data_source"].(string)
	file, _ := req["pcap_file"].(string)
	if source == "pcap" || (source == "" && file != "") {
		return ws.captureInputs(file)
	}
	return []eligibility.Input{{Identity: eligibility.Live(ws.sensorID)}}
}

// hintInputs are the captures of the clip a HINT request names.
func (ws *Server) hintInputs(req map[string]any) []eligibility.Input {
	clipID, _ := req["clip_id"].(string)
	unknown := func(err error) []eligibility.Input {
		return []eligibility.Input{{Identity: eligibility.Identity{Kind: eligibility.KindCapture, Basename: "clip " + clipID}, Err: err}}
	}
	if clipID == "" {
		return unknown(fmt.Errorf("clip_id is required"))
	}
	files, err := ws.clipCaptureFiles(clipID)
	if err != nil {
		return unknown(err)
	}
	return ws.captureInputs(files...)
}

// clipCaptureFiles are the PCAP files a clip replays.
func (ws *Server) clipCaptureFiles(clipID string) ([]string, error) {
	if ws.clipFilesForTest != nil {
		return ws.clipFilesForTest(clipID)
	}
	if ws.db == nil {
		return nil, fmt.Errorf("no database to read clip %s from", clipID)
	}
	clip, err := sqlite.NewClipStore(ws.db.DB).GetClip(clipID)
	if err != nil || clip == nil {
		return nil, fmt.Errorf("clip %s: %v", clipID, err)
	}
	var files []string
	for _, f := range clip.Files {
		files = append(files, f.PCAPFile)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("clip %s names no capture", clipID)
	}
	return files, nil
}

// experimentConfig reports whether a tuning config file is an experiment
// arm's parameter set (config/experiments/), which makes its replay a
// development evaluation.
func experimentConfig(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	return strings.HasPrefix(clean, "experiments/") || strings.Contains(clean, "/experiments/")
}

type eligibilityStatusResponse struct {
	eligibility.Status
	Coverage      []EntryPointCoverage `json:"coverage"`
	Exposure      any                  `json:"exposure"`
	ExposureError string               `json:"exposure_error,omitempty"`
}

func (ws *Server) handleEligibilityStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	svc := ws.eligibilityService()
	resp := eligibilityStatusResponse{Status: svc.Status(), Coverage: eligibilityCoverage, Exposure: []eligibility.ExposureSummary{}}
	if ws.db != nil {
		summary, err := labelworkdb.UseLedger{DB: ws.db.DB}.Summary()
		if err != nil {
			resp.ExposureError = err.Error()
		} else {
			resp.Exposure = summary
		}
	} else {
		resp.ExposureError = "no database: the use ledger is unavailable"
	}
	ws.writeJSON(w, http.StatusOK, resp)
}

type eligibilityCheckRequest struct {
	Purpose      string   `json:"purpose"`
	PCAPFiles    []string `json:"pcap_files"`
	Live         bool     `json:"live,omitempty"`
	Measures     string   `json:"measures,omitempty"`
	HeldOutScore bool     `json:"held_out_score,omitempty"`
	SplitDigest  string   `json:"split_digest,omitempty"`
}

// handleEligibilityCheck answers what a purpose would be decided for some
// captures, without recording or granting anything: a page uses it to say
// which actions are available. The entry points decide again for real.
func (ws *Server) handleEligibilityCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req eligibilityCheckRequest
	if !ws.decodePhysicalRequest(w, r, &req) {
		return
	}
	measures, err := eligibility.ParseMeasures(req.Measures)
	if err != nil {
		ws.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.PCAPFiles) > 50 {
		ws.writeJSONError(w, http.StatusBadRequest, "at most 50 captures per check")
		return
	}
	inputs := ws.captureInputs(req.PCAPFiles...)
	if req.Live {
		inputs = append(inputs, eligibility.Input{Identity: eligibility.Live(ws.sensorID)})
	}
	d := ws.eligibilityService().Check(eligibility.Request{
		Purpose: eligibility.Purpose(req.Purpose), Measures: measures, Inputs: inputs,
		HeldOutScore: req.HeldOutScore, SplitDigest: req.SplitDigest, EntryPoint: "check", Actor: actorOf(r),
	})
	ws.writeJSON(w, http.StatusOK, d)
}

func (ws *Server) handleEligibilityUses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		ws.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if ws.db == nil {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "no database: the use ledger is unavailable")
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	uses, total, err := labelworkdb.UseLedger{DB: ws.db.DB}.List(eligibility.UseQuery{
		CaptureSHA256: q.Get("capture"), Decision: q.Get("decision"), Limit: limit, Offset: offset,
	})
	if err != nil {
		ws.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ws.writeJSON(w, http.StatusOK, map[string]any{"uses": uses, "total": total, "limit": limit, "offset": offset})
}
