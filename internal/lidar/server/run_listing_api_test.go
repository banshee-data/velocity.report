package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// With sweeps=1 the runs list names the sweep whose stored results list a
// run, so the Runs page can fold a sweep's runs under it; a run no sweep
// lists names none, and without sweeps=1 no run names one.
func TestListRunsNamesEachRunsSweep(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	store := sqlite.NewRunStore(sqlDB)
	setupTestRun(t, store, "run-swept")
	setupTestRun(t, store, "run-alone")
	if _, err := sqlDB.Exec(`INSERT INTO lidar_tuning_sweeps (sweep_id, sensor_id, mode, status, request, results, started_at)
		VALUES ('sweep-1', 'test-sensor', 'sweep', 'completed', '{}', '[{"run_id":"run-swept"}]', '2026-11-07T00:00:00Z')`); err != nil {
		t.Fatalf("insert sweep: %v", err)
	}

	ws := &Server{db: &db.DB{DB: sqlDB}}

	// Without sweeps=1 the scan is skipped.
	w := httptest.NewRecorder()
	ws.handleListRuns(w, httptest.NewRequest(http.MethodGet, "/api/lidar/runs?limit=10", nil))
	if strings.Contains(w.Body.String(), "sweep_id") {
		t.Errorf("a list that did not ask names sweeps: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	ws.handleListRuns(w, httptest.NewRequest(http.MethodGet, "/api/lidar/runs?limit=10&sweeps=1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Runs []struct {
			RunID     string `json:"run_id"`
			SweepID   string `json:"sweep_id"`
			SweepMode string `json:"sweep_mode"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	seen := map[string]bool{}
	for _, run := range body.Runs {
		seen[run.RunID] = true
		switch run.RunID {
		case "run-swept":
			if run.SweepID != "sweep-1" || run.SweepMode != "sweep" {
				t.Errorf("run-swept = %+v, want sweep-1/sweep", run)
			}
		case "run-alone":
			if run.SweepID != "" || run.SweepMode != "" {
				t.Errorf("run-alone should name no sweep, got %+v", run)
			}
		}
	}
	if !seen["run-swept"] || !seen["run-alone"] {
		t.Fatalf("both runs should be listed, got %+v", body.Runs)
	}

	// One run, as a link to a run past the listed page fetches it.
	w = httptest.NewRecorder()
	ws.handleGetRun(w, httptest.NewRequest(http.MethodGet, "/api/lidar/runs/run-swept?sweeps=1", nil), "run-swept")
	var one struct {
		SweepID string `json:"sweep_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&one); err != nil {
		t.Fatalf("decode one run: %v", err)
	}
	if one.SweepID != "sweep-1" {
		t.Errorf("GET one run: sweep_id = %q, want sweep-1", one.SweepID)
	}
}

// The clips list names a located clip's site and its label.
func TestListClipsNamesEachLocatedClipsSite(t *testing.T) {
	sqlDB, cleanup := setupTestDB(t)
	defer cleanup()

	clips := sqlite.NewClipStore(sqlDB)
	if err := clips.InsertClip(&sqlite.Clip{ClipID: "clip-1", SensorID: "s", Files: []sqlite.ClipCapture{{PCAPFile: "a.pcap"}}}); err != nil {
		t.Fatalf("InsertClip: %v", err)
	}
	loc, err := clips.SetClipLocation("clip-1", 37.7987, -122.4073, "")
	if err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}
	if _, err := sqlite.NewSiteStore(sqlDB).SetCanonicalPose(loc.L16Token, 37.7987, -122.4073, "operator", "Broadway at Columbus"); err != nil {
		t.Fatalf("SetCanonicalPose: %v", err)
	}

	ws := &Server{db: &db.DB{DB: sqlDB}}
	w := httptest.NewRecorder()
	ws.handleClips(w, httptest.NewRequest(http.MethodGet, "/api/lidar/clips", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Clips []struct {
			ClipID    string `json:"clip_id"`
			SiteID    string `json:"site_id"`
			SiteLabel string `json:"site_label"`
		} `json:"clips"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Clips) != 1 || body.Clips[0].SiteID != loc.L16Token || body.Clips[0].SiteLabel != "Broadway at Columbus" {
		t.Fatalf("clips = %+v, want clip-1 at %s named Broadway at Columbus", body.Clips, loc.L16Token)
	}
}
