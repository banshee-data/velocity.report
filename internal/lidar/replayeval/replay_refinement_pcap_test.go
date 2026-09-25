//go:build pcap

package replayeval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// TestFixedLagRTSReplayOnKirk0 runs the fixed_lag_rts experiment once, on a
// short moving-traffic window, and checks what one replay must guarantee for
// every horizon at once: each smoother released every state exactly once and
// never moved one without evidence; each refined arm scores the same
// population as the online arm; and, persisted, each arm is its own version
// with exactly the online sink's rows, every revision naming an online row
// that exists and every estimate linking to its immutable observation. The
// per-horizon table is logged; the full-capture figures are in the criteria
// document.
//
// The window scores 2 s of the moving window after its 6 s of warm-up:
// the replay dominates this test's cost under -race, and the checks need
// traffic, not a settled background.
func TestFixedLagRTSReplayOnKirk0(t *testing.T) {
	dir := t.TempDir()
	cfg := kirk0MovingWindow(t, filepath.Join(dir, "vrlog"))
	cfg.DurationSeconds = 2
	cfg.Experiments = []string{ExperimentFixedLagRTS}
	cfg.ObservationDBPath = filepath.Join(dir, "evidence.db")
	cfg.ReplayCaseID = "kirk0-refinement-test"
	cfg.ObservationCalibration = identityReplayCalibration(cfg.SensorID)
	cfg.ObservationMaxSamplePoints = 16
	result, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	report := result.Refinement
	if report == nil || len(report.Arms) != len(RefinementHorizons)+1 || !report.Persisted {
		t.Fatalf("refinement report missing or incomplete: %+v", report)
	}
	onFile, err := os.ReadFile(filepath.Join(cfg.OutDir, "refinement_report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded RefinementReport
	if err := json.Unmarshal(onFile, &decoded); err != nil || len(decoded.Arms) != len(report.Arms) {
		t.Fatalf("refinement_report.json unreadable: %v", err)
	}
	t.Logf("kirk0 70-72 s, one replay:\n%s", l8analytics.RefinementComparisonMarkdown(report.Arms))

	online := report.Arms[0]
	if online.States == 0 || online.ObservedStates == 0 {
		t.Fatal("the window scored no confirmed states; it no longer exercises the smoother")
	}
	for i, arm := range report.Arms[1:] {
		stats := report.Smoothers[i].Stats
		if stats.Released != stats.Steps || stats.HeldSteps != 0 {
			t.Errorf("%s: released %d of %d steps, %d still held", arm.Label, stats.Released, stats.Steps, stats.HeldSteps)
		}
		if arm.RevisionsWithoutEvidence != 0 || stats.RevisionsWithoutEvidence != 0 {
			t.Errorf("%s: %d revisions without evidence", arm.Label, stats.RevisionsWithoutEvidence)
		}
		if arm.States != online.States || arm.ObservedStates != online.ObservedStates {
			t.Errorf("%s scored %d/%d states, online %d/%d: the arms must share one population",
				arm.Label, arm.States, arm.ObservedStates, online.States, online.ObservedStates)
		}
		t.Logf("%s: %+v", arm.Label, stats)
	}

	database, err := db.NewDB(cfg.ObservationDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := database.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	onlineRows := count(`SELECT COUNT(*) FROM lidar_track_estimates WHERE stage = 'online' AND estimator_id = ?`, onlineEstimatorID)
	if onlineRows == 0 {
		t.Fatal("no online estimates were persisted")
	}
	for _, smoother := range report.Smoothers {
		rows := count(`SELECT COUNT(*) FROM lidar_track_estimates WHERE estimator_id = ? AND param_hash = ?`,
			pipeline.RefinedEstimatorID(onlineEstimatorID), smoother.ParamHash)
		if rows != smoother.Persisted || rows != onlineRows {
			t.Errorf("%s: %d rows persisted (reported %d), online %d: the refined version must cover exactly the online rows",
				smoother.Lag, rows, smoother.Persisted, onlineRows)
		}
	}
	if n := count(`SELECT COUNT(*) FROM lidar_track_estimate_revisions v
		LEFT JOIN lidar_track_estimates e ON e.estimate_id = v.revises_estimate_id WHERE e.estimate_id IS NULL`); n != 0 {
		t.Errorf("%d revisions name an online estimate that does not exist", n)
	}
	if n := count(`SELECT COUNT(*) FROM lidar_track_estimate_revisions`); n != onlineRows*len(RefinementHorizons) {
		t.Errorf("%d revision records, want one per refined row (%d)", n, onlineRows*len(RefinementHorizons))
	}
	if _, err := observationsqlite.BuildEvidenceOracle(database.DB, map[string]struct{}{result.ObservationSourceID: {}}); err != nil {
		t.Errorf("evidence oracle over online and refined rows: %v", err)
	}
	store := observationsqlite.NewStateEstimateStore(database)
	legacy, err := store.ListBySource(result.ObservationSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy) != onlineRows {
		t.Errorf("ListBySource returned %d rows with refined stages present, want the %d online rows", len(legacy), onlineRows)
	}
}
