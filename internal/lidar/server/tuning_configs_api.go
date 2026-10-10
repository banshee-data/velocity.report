package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// TuningConfigInfo describes one .json file under the server's config
// directory as the replay form sees it.
type TuningConfigInfo struct {
	// Path is relative to the config directory, slash separated.
	Path string `json:"path"`
	// Kind is "tuning" for a file the strict loader accepts and "other"
	// for any other JSON (a sweep plan, the segment selectors), with Error
	// saying why.
	Kind  string `json:"kind"`
	Error string `json:"error,omitempty"`
	// Fingerprint, Profile and Engines describe a tuning config; SolidBody
	// summarises its solid-body block, "off" when it has none.
	Fingerprint string `json:"fingerprint,omitempty"`
	Profile     string `json:"profile,omitempty"`
	Engines     string `json:"engines,omitempty"`
	SolidBody   string `json:"solid_body,omitempty"`
	// Applicable says the file can be applied to the running server: its
	// startup-only settings (the engine selectors and the pipeline block)
	// match the running config's. Reason says what differs when not.
	Applicable bool   `json:"applicable"`
	Reason     string `json:"reason,omitempty"`
	// Active says this is the file the last replay start applied.
	Active bool `json:"active"`
}

// handleListTuningConfigs lists every .json file under the config directory,
// classified and checked against the running config, for the replay form's
// dropdown and for scripts.
//
// GET /api/lidar/configs
func (ws *Server) handleListTuningConfigs(w http.ResponseWriter, r *http.Request) {
	if ws.tuningConfigDir == "" {
		ws.writeJSONError(w, http.StatusServiceUnavailable, "no tuning config directory is configured (--lidar-config-dir)")
		return
	}
	running := ws.snapshotTuningConfig()
	activePath, activeFingerprint := ws.activeTuning()

	const maxFiles = 200
	configs := []TuningConfigInfo{}
	_ = filepath.WalkDir(ws.tuningConfigDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.ToLower(filepath.Ext(path)) != ".json" {
			return nil
		}
		rel, relErr := filepath.Rel(ws.tuningConfigDir, path)
		if relErr != nil {
			return nil
		}
		info := describeTuningConfig(path, filepath.ToSlash(rel), running)
		info.Active = info.Path == activePath
		configs = append(configs, info)
		if len(configs) >= maxFiles {
			return filepath.SkipAll
		}
		return nil
	})
	sort.Slice(configs, func(i, j int) bool { return configs[i].Path < configs[j].Path })

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"config_dir": ws.tuningConfigDir,
		"configs":    configs,
		"count":      len(configs),
		"active": map[string]string{
			"path":        activePath,
			"fingerprint": activeFingerprint,
		},
	})
}

// describeTuningConfig classifies one file: loaded by the strict loader or
// not, and if loaded, whether it could be applied to the running config.
func describeTuningConfig(absPath, rel string, running *cfgpkg.TuningConfig) TuningConfigInfo {
	info := TuningConfigInfo{Path: rel, Kind: "other"}
	cfg, err := cfgpkg.LoadTuningConfig(absPath)
	if err != nil {
		info.Error = err.Error()
		info.Reason = "not a tuning config"
		return info
	}
	info.Kind = "tuning"
	info.Fingerprint = cfg.Fingerprint()
	info.Profile = string(cfg.Profile())
	info.Engines = cfg.L3.Engine + "/" + cfg.L4.Engine + "/" + cfg.L5.Engine
	info.SolidBody = describeSolidBodyBlock(cfg)
	if reason := startupIncompatibility(running, cfg); reason != "" {
		info.Reason = reason
		return info
	}
	info.Applicable = true
	return info
}

// describeSolidBodyBlock is a one-line summary of a config's solid-body
// block for the dropdown: off, the shadow, or the tracked filter, with the
// options switched on.
func describeSolidBodyBlock(cfg *cfgpkg.TuningConfig) string {
	if cfg.L5.CvKfV1 == nil || cfg.L5.CvKfV1.SolidBody == nil || !cfg.L5.CvKfV1.SolidBody.Enabled {
		return "off"
	}
	sb := cfg.L5.CvKfV1.SolidBody
	var on []string
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"face_hysteresis", sb.FaceHysteresis}, {"face_entry_consider", sb.FaceEntryConsider},
		{"course_aligned_faces", sb.CourseAlignedFaces}, {"reference_translation", sb.ReferenceTranslation},
		{"course_heading", sb.CourseHeading}, {"extent_prior_floor", sb.ExtentPriorFloor},
		{"face_plane_spans", sb.FacePlaneSpans}, {"extent_growth_admission", sb.ExtentGrowthAdmission},
		{"vehicle_extent_floor", sb.VehicleExtentFloor}, {"end_face_centring", sb.EndFaceCentring},
		{"end_face_centring_open_prior", sb.EndFaceCentringOpenPrior}, {"containment", sb.Containment},
		{"rectangle_fit", sb.RectangleFit}, {"rectangle_heading", sb.RectangleHeading},
		{"rectangle_course_fusion", sb.RectangleCourseFusion},
	} {
		if f.set {
			on = append(on, f.name)
		}
	}
	if sb.RankOneMedoidScale != 0 {
		on = append(on, fmt.Sprintf("rank_one_medoid_scale=%g", sb.RankOneMedoidScale))
	}
	if sb.RectangleSigmaScale != 0 {
		on = append(on, fmt.Sprintf("rectangle_sigma_scale=%g", sb.RectangleSigmaScale))
	}
	mode := "shadow"
	if sb.NearEdgeTracking {
		mode = "tracked"
		if sb.NearEdgeMedoidGate {
			mode = "tracked, medoid gate"
		}
	}
	if len(on) == 0 {
		return mode
	}
	return mode + ": " + strings.Join(on, ", ")
}

// startupIncompatibility says what a config changes that only a restart
// can: the engine selectors and the pipeline block. Empty means the file
// can be applied to the running server through the runtime path. l1 is
// not compared: the server sets the sensor and the data source itself.
func startupIncompatibility(running, candidate *cfgpkg.TuningConfig) string {
	if running == nil {
		return ""
	}
	var diffs []string
	for _, e := range []struct{ name, have, want string }{
		{"l3.engine", running.L3.Engine, candidate.L3.Engine},
		{"l4.engine", running.L4.Engine, candidate.L4.Engine},
		{"l5.engine", running.L5.Engine, candidate.L5.Engine},
	} {
		if e.have != e.want {
			diffs = append(diffs, fmt.Sprintf("%s %q (running %q)", e.name, e.want, e.have))
		}
	}
	if running.Pipeline != candidate.Pipeline {
		diffs = append(diffs, "the pipeline block")
	}
	if len(diffs) == 0 {
		return ""
	}
	return "changes startup-only settings: " + strings.Join(diffs, ", ")
}

// resolveTuningConfigPath turns a request's relative path into the file
// under the config directory it names, refusing anything that would leave
// the directory or is not a .json file.
func (ws *Server) resolveTuningConfigPath(rel string) (abs, clean string, err error) {
	if ws.tuningConfigDir == "" {
		return "", "", fmt.Errorf("no tuning config directory is configured (--lidar-config-dir)")
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", "", fmt.Errorf("tuning_config is empty")
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return "", "", fmt.Errorf("tuning_config %q must be relative to the config directory", rel)
	}
	clean = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", "", fmt.Errorf("tuning_config %q leaves the config directory", rel)
	}
	if strings.ToLower(filepath.Ext(clean)) != ".json" {
		return "", "", fmt.Errorf("tuning_config %q is not a .json file", rel)
	}
	abs = filepath.Join(ws.tuningConfigDir, filepath.FromSlash(clean))
	if r, relErr := filepath.Rel(ws.tuningConfigDir, abs); relErr != nil || strings.HasPrefix(r, "..") {
		return "", "", fmt.Errorf("tuning_config %q leaves the config directory", rel)
	}
	st, statErr := os.Stat(abs)
	if statErr != nil || !st.Mode().IsRegular() {
		return "", "", fmt.Errorf("tuning_config %q is not a file under the config directory", rel)
	}
	return abs, clean, nil
}

// applyTuningConfigFile loads a tuning config from under the config
// directory and applies it to the running pipeline through the runtime
// path, as a POST of the whole file to /api/lidar/params would, after
// refusing a file whose startup-only settings differ from the running
// config's. It records the file as the active tuning config. The replay
// start and the sweep backend call it before a replay begins.
func (ws *Server) applyTuningConfigFile(rel string) (*TuningConfigInfo, error) {
	abs, clean, err := ws.resolveTuningConfigPath(rel)
	if err != nil {
		return nil, err
	}
	cfg, err := cfgpkg.LoadTuningConfig(abs)
	if err != nil {
		return nil, fmt.Errorf("tuning_config %s: %w", clean, err)
	}
	if reason := startupIncompatibility(ws.snapshotTuningConfig(), cfg); reason != "" {
		return nil, fmt.Errorf("tuning_config %s %s; restart the server with --config to run it", clean, reason)
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("tuning_config %s: %w", clean, err)
	}
	var nested map[string]interface{}
	if err := json.Unmarshal(raw, &nested); err != nil {
		return nil, fmt.Errorf("tuning_config %s: %w", clean, err)
	}
	patch, err := normaliseTuningPatch(nested)
	if err != nil {
		return nil, fmt.Errorf("tuning_config %s: %w", clean, err)
	}
	bm := l3grid.GetBackgroundManager(ws.sensorID)
	if bm != nil && bm.Grid == nil {
		bm = nil
	}
	if err := applyRuntimeTuningPatch(ws, bm, patch); err != nil {
		return nil, fmt.Errorf("tuning_config %s: %w", clean, err)
	}
	// The patch is additive, so a file without the block leaves whatever
	// block the server was running. A file is a whole config, and no block
	// in it means the estimator off.
	if cfg.L5.CvKfV1 != nil && cfg.L5.CvKfV1.SolidBody == nil {
		ws.clearSolidBodyTuning()
	}

	info := describeTuningConfig(abs, clean, nil)
	ws.tuningConfigMu.Lock()
	ws.activeTuningPath, ws.activeTuningFingerprint = clean, info.Fingerprint
	ws.tuningConfigMu.Unlock()
	info.Applicable, info.Active = true, true
	return &info, nil
}

// clearSolidBodyTuning switches the solid body off on the tracker and the
// members switch, and drops the block from the stored config.
func (ws *Server) clearSolidBodyTuning() {
	if ws.tracker != nil {
		_ = applySolidBodyTuning(ws, &cfgpkg.L5Common{})
	}
	ws.clusterMembers.Store(false)
	ws.tuningConfigMu.Lock()
	if ws.tuningConfig != nil && ws.tuningConfig.L5.CvKfV1 != nil {
		ws.tuningConfig.L5.CvKfV1.SolidBody = nil
	}
	ws.tuningConfigMu.Unlock()
}

// activeTuning is the config file the last replay start applied and its
// fingerprint, or the empty path and the fingerprint of the config the
// server is running.
func (ws *Server) activeTuning() (path, fingerprint string) {
	ws.tuningConfigMu.RLock()
	path, fingerprint = ws.activeTuningPath, ws.activeTuningFingerprint
	cfg := ws.tuningConfig
	ws.tuningConfigMu.RUnlock()
	if fingerprint == "" && cfg != nil {
		fingerprint = cfg.Fingerprint()
	}
	return path, fingerprint
}
