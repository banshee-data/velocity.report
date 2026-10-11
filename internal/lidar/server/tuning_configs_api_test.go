package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// writeConfigTree writes a config directory with the defaults, an
// experiment config carrying a solid-body block, a profile with a layer
// switched off, a file that is not a tuning config, and a non-JSON file.
func writeConfigTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	defaults := cfgpkg.MustLoadDefaultConfig()
	write := func(rel string, cfg *cfgpkg.TuningConfig) {
		t.Helper()
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("tuning.defaults.json", defaults)

	candidate := cfgpkg.MustLoadDefaultConfig()
	candidate.L5.CvKfV1.SolidBody = &cfgpkg.L5SolidBody{
		Enabled: true, FullMembers: true, NearEdgeTracking: true, FaceHysteresis: true, CourseAlignedFaces: true,
		Containment: true, RectangleHeading: true, RectangleCourseFusion: true, RectangleSigmaScale: 1.5,
	}
	write("experiments/candidate.json", candidate)

	detect := cfgpkg.MustLoadDefaultConfig()
	if err := detect.ApplyProfile(cfgpkg.ProfileDetect); err != nil {
		t.Fatal(err)
	}
	write("profiles/detect.json", detect)

	if err := os.WriteFile(filepath.Join(dir, "sweep-plan.json"), []byte(`{"version":1,"combos":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# configs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// newConfigTestServer is a server with a tracker and a background manager,
// the two things a config applies to, over the written config tree.
func newConfigTestServer(t *testing.T, sensorID, dir string) *Server {
	t.Helper()
	params := l3grid.DefaultBackgroundConfig().ToBackgroundParams()
	mgr := l3grid.NewBackgroundManager(sensorID, 40, 1800, params, nil)
	l3grid.RegisterBackgroundManager(sensorID, mgr)
	t.Cleanup(func() { l3grid.RegisterBackgroundManager(sensorID, nil) })

	ws := NewServer(Config{
		Address:         ":0",
		Stats:           NewPacketStats(),
		SensorID:        sensorID,
		TuningConfig:    cfgpkg.MustLoadDefaultConfig(),
		TuningConfigDir: dir,
	})
	ws.SetTracker(l5tracks.NewTracker(l5tracks.DefaultTrackerConfig()))
	return ws
}

// Every .json under the directory is listed: the tuning configs with their
// fingerprint, profile and solid-body summary, the profile with a layer off
// as not applicable, and the sweep plan as not a tuning config. The README
// is not listed.
func TestListTuningConfigsClassifiesEveryJSON(t *testing.T) {
	dir := writeConfigTree(t)
	ws := newConfigTestServer(t, "sensor-configs", dir)

	rec := httptest.NewRecorder()
	ws.handleListTuningConfigs(rec, httptest.NewRequest(http.MethodGet, "/api/lidar/configs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ConfigDir string             `json:"config_dir"`
		Configs   []TuningConfigInfo `json:"configs"`
		Count     int                `json:"count"`
		Active    map[string]string  `json:"active"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	byPath := map[string]TuningConfigInfo{}
	for _, c := range resp.Configs {
		byPath[c.Path] = c
	}
	if resp.Count != 4 || len(byPath) != 4 {
		t.Fatalf("listed %d configs, want 4: %+v", resp.Count, resp.Configs)
	}
	if _, listed := byPath["README.md"]; listed {
		t.Fatal("a non-JSON file was listed")
	}
	d := byPath["tuning.defaults.json"]
	if d.Kind != "tuning" || !d.Applicable || d.SolidBody != "off" || d.Fingerprint != cfgpkg.MustLoadDefaultConfig().Fingerprint() || d.Profile != "full" {
		t.Fatalf("the defaults: %+v", d)
	}
	c := byPath["experiments/candidate.json"]
	if c.Kind != "tuning" || !c.Applicable || !strings.HasPrefix(c.SolidBody, "tracked: ") || !strings.Contains(c.SolidBody, "rectangle_sigma_scale=1.5") || c.Engines != "ema_baseline_v1/dbscan_xy_v1/cv_kf_v1" {
		t.Fatalf("the candidate: %+v", c)
	}
	p := byPath["profiles/detect.json"]
	if p.Kind != "tuning" || p.Applicable || !strings.Contains(p.Reason, "l5.engine") || p.Profile != "detect" {
		t.Fatalf("the detect profile: %+v", p)
	}
	s := byPath["sweep-plan.json"]
	if s.Kind != "other" || s.Applicable || s.Error == "" || s.Reason != "not a tuning config" {
		t.Fatalf("the sweep plan: %+v", s)
	}
	if resp.Active["path"] != "" || resp.Active["fingerprint"] != cfgpkg.MustLoadDefaultConfig().Fingerprint() {
		t.Fatalf("active before any apply: %v", resp.Active)
	}
	for _, c := range resp.Configs {
		if c.Active {
			t.Fatalf("%s is active before any apply", c.Path)
		}
	}
}

// A server without a config directory refuses the listing rather than
// listing the working directory.
func TestListTuningConfigsNeedsADirectory(t *testing.T) {
	ws := NewServer(Config{SensorID: "sensor-nodir"})
	rec := httptest.NewRecorder()
	ws.handleListTuningConfigs(rec, httptest.NewRequest(http.MethodGet, "/api/lidar/configs", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// Applying the candidate puts its block on the tracker and the members
// switch, records it as active, and reads back through the runtime config;
// a path that leaves the directory, a non-JSON file, a missing file and a
// file that changes a startup-only setting are refused, and the tracker is
// left as it was.
func TestApplyTuningConfigFile(t *testing.T) {
	dir := writeConfigTree(t)
	ws := newConfigTestServer(t, "sensor-apply", dir)
	bm := l3grid.GetBackgroundManager("sensor-apply")

	for _, bad := range []string{"", "../tuning.defaults.json", "/etc/passwd", "README.md", "experiments/missing.json", "profiles/detect.json"} {
		if _, err := ws.applyTuningConfigFile(bad); err == nil {
			t.Fatalf("%q was applied", bad)
		}
	}
	if got := ws.tracker.GetConfig(); got.SolidBody.Enabled || ws.clusterMembers.Load() {
		t.Fatalf("a refused file reached the tracker: %+v", got.SolidBody)
	}
	if p, _ := ws.activeTuning(); p != "" {
		t.Fatalf("a refused file became active: %q", p)
	}

	info, err := ws.applyTuningConfigFile("experiments/candidate.json")
	if err != nil {
		t.Fatalf("the candidate was refused: %v", err)
	}
	if !info.Active || !info.Applicable || info.Path != "experiments/candidate.json" {
		t.Fatalf("applied info %+v", info)
	}
	got := ws.tracker.GetConfig()
	if !got.SolidBody.Enabled || !got.NearEdgeTracking || !got.SolidBody.RectangleHeading || got.SolidBody.RectangleSigmaScale != 1.5 ||
		got.SolidBody.OriginSource != l5tracks.OriginTrackingTransformIdentity || !ws.clusterMembers.Load() {
		t.Fatalf("the candidate did not reach the tracker: %+v members %v", got.SolidBody, ws.clusterMembers.Load())
	}
	if p, fp := ws.activeTuning(); p != "experiments/candidate.json" || fp != info.Fingerprint {
		t.Fatalf("active %q %q", p, fp)
	}
	back := ws.runtimeTuningConfig(bm)
	if sb := back.L5.CvKfV1.SolidBody; sb == nil || !sb.Enabled || !sb.FullMembers || !sb.NearEdgeTracking || sb.RectangleSigmaScale != 1.5 {
		t.Fatalf("the runtime config does not read the block back: %+v", sb)
	}

	// The listing now marks it active, and applying the defaults switches
	// the body off again, members included.
	rec := httptest.NewRecorder()
	ws.handleListTuningConfigs(rec, httptest.NewRequest(http.MethodGet, "/api/lidar/configs", nil))
	if !strings.Contains(rec.Body.String(), `"path":"experiments/candidate.json","kind":"tuning"`) || !strings.Contains(rec.Body.String(), `"active":true`) {
		t.Fatalf("the listing does not mark the candidate active: %s", rec.Body.String())
	}
	if _, err := ws.applyTuningConfigFile("tuning.defaults.json"); err != nil {
		t.Fatal(err)
	}
	if got := ws.tracker.GetConfig(); got.SolidBody.Enabled || got.NearEdgeTracking || ws.clusterMembers.Load() {
		t.Fatalf("the defaults did not switch the body off: %+v", got.SolidBody)
	}
	if sb := ws.runtimeTuningConfig(bm).L5.CvKfV1.SolidBody; sb != nil {
		t.Fatalf("the runtime config carries a block with the body off: %+v", sb)
	}
}

// The runtime params path takes the block's keys as nested JSON, applies the
// whole block at once, and refuses a half block by the block's own rule.
func TestRuntimeParamsApplySolidBodyKeys(t *testing.T) {
	ws := newConfigTestServer(t, "sensor-params", writeConfigTree(t))
	bm := l3grid.GetBackgroundManager("sensor-params")
	block := map[string]interface{}{
		"enabled": true, "full_members": true, "near_edge_tracking": false, "near_edge_medoid_gate": false,
		"face_hysteresis": true, "face_entry_consider": false, "course_aligned_faces": true, "reference_translation": false,
		"rank_one_medoid_scale": 0, "course_heading": false, "extent_prior_floor": false, "face_plane_spans": false,
		"extent_growth_admission": false, "vehicle_extent_floor": false, "end_face_centring": false,
		"end_face_centring_open_prior": false, "containment": true, "rectangle_fit": false, "rectangle_heading": false,
		"rectangle_course_fusion": false, "rectangle_sigma_scale": 0,
	}
	patch, err := normaliseTuningPatch(map[string]interface{}{"l5": map[string]interface{}{"cv_kf_v1": map[string]interface{}{"solid_body": block}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := applyRuntimeTuningPatch(ws, bm, patch); err != nil {
		t.Fatalf("the block was refused: %v", err)
	}
	got := ws.tracker.GetConfig()
	if !got.SolidBody.Enabled || got.NearEdgeTracking || !got.SolidBody.Containment || !got.SolidBody.FaceHysteresis || !ws.clusterMembers.Load() {
		t.Fatalf("the block did not reach the tracker: %+v", got.SolidBody)
	}
	half, _ := normaliseTuningPatch(map[string]interface{}{"l5": map[string]interface{}{"cv_kf_v1": map[string]interface{}{"solid_body": map[string]interface{}{"full_members": false}}}})
	if err := applyRuntimeTuningPatch(ws, bm, half); err == nil || !strings.Contains(err.Error(), "full_members") {
		t.Fatalf("a block that lost its members was accepted: %v", err)
	}
	if got := ws.tracker.GetConfig(); !got.SolidBody.Enabled || !ws.clusterMembers.Load() {
		t.Fatal("a refused patch changed the tracker")
	}
}

// A replay start names a config file, which is applied before the replay
// and echoed with its fingerprint; a bad path is refused before anything
// is stopped.
func TestHandlePCAPStartAppliesATuningConfig(t *testing.T) {
	dir := writeConfigTree(t)
	ws := newConfigTestServer(t, "sensor-replay-config", dir)
	pcapDir := resolveSymlinks(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(pcapDir, "capture.pcap"), testPCAPHeader, 0o644); err != nil {
		t.Fatal(err)
	}
	ws.pcapSafeDir = pcapDir
	ws.setBaseContext(context.Background())

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-replay-config",
		bytes.NewBufferString(`{"pcap_file":"capture.pcap","tuning_config":"../secret.json"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ws.handlePCAPStart(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "leaves the config directory") {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if ws.PipelineState().PCAPInProgress() {
		t.Fatal("a refused config started a replay")
	}

	form := strings.NewReader("pcap_file=capture.pcap&analysis_mode=false&tuning_config=experiments%2Fcandidate.json")
	req = httptest.NewRequest(http.MethodPost, "/api/lidar/pcap/start?sensor_id=sensor-replay-config", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	ws.handlePCAPStart(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["tuning_config"] != "experiments/candidate.json" || resp["tuning_fingerprint"] == "" {
		t.Fatalf("the response does not name the config: %v", resp)
	}
	if got := ws.tracker.GetConfig(); !got.SolidBody.RectangleHeading || !got.NearEdgeTracking {
		t.Fatalf("the replay started without the config: %+v", got.SolidBody)
	}
}

// The status page's replay form carries the dropdown and fetches the
// listing on load, and the JSON status endpoints name the active config.
func TestStatusPageListsTuningConfigs(t *testing.T) {
	dir := writeConfigTree(t)
	ws := newConfigTestServer(t, "sensor-status-configs", dir)
	rec := httptest.NewRecorder()
	ws.setupRoutes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`name="tuning_config"`, `fetch('/api/lidar/configs')`, "Tuning Config", dir, "(startup config)"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the status page lacks %q", want)
		}
	}
	rec = httptest.NewRecorder()
	ws.handleDataSource(rec, httptest.NewRequest(http.MethodGet, "/api/lidar/data_source?sensor_id=sensor-status-configs", nil))
	if !strings.Contains(rec.Body.String(), `"tuning_fingerprint":"`+cfgpkg.MustLoadDefaultConfig().Fingerprint()+`"`) {
		t.Fatalf("data_source lacks the fingerprint: %s", rec.Body.String())
	}
}
