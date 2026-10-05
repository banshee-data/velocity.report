//go:build pcap && !race

package replayeval

// kirk0 evidence that fixed_lag_rts combines with near_edge_track (near-edge
// plan S2.4). Label-free: it proves the wiring end to end on the moving
// window. Excluded under -race for the reason replay_solid_body_pcap_test.go
// gives: a near_edge_track replay measures faces from full members, and the
// race detector adds nothing the l5tracks, pipeline and storage unit tests do
// not already cover.

import (
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

// TestFixedLagRTSWithNearEdgeTrackOnKirk0: every smoother ends chains at the
// window's reference changes and releases every state once; every refined
// estimate has a refined solid body beside it carrying its revised state and
// reference; the source-wide readers still return online rows only; the
// evidence oracle covers the solid bodies, online and refined; and the
// headway field run reads the whole-track arm's solid bodies at final.
func TestFixedLagRTSWithNearEdgeTrackOnKirk0(t *testing.T) {
	dir := t.TempDir()
	cfg := kirk0MovingWindow(t, filepath.Join(dir, "vrlog"))
	cfg.DurationSeconds = 4
	cfg.Experiments = []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentSolidBodyFaceHysteresis,
		ExperimentSolidBodyCourseFaces, ExperimentNearEdgeTrack, ExperimentFixedLagRTS}
	cfg.ObservationDBPath = filepath.Join(dir, "evidence.db")
	cfg.ReplayCaseID = "kirk0-near-edge-refinement-test"
	cfg.ObservationCalibration = identityReplayCalibration(cfg.SensorID)
	cfg.ObservationMaxSamplePoints = 16
	result, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	report := result.Refinement
	if report == nil || !report.Persisted || len(report.Smoothers) != len(RefinementHorizons) {
		t.Fatalf("refinement report missing or incomplete: %+v", report)
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
	onlineRows := count(`SELECT COUNT(*) FROM lidar_track_estimates WHERE stage = 'online'`)
	onlineBodies := count(`SELECT COUNT(*) FROM lidar_track_solid_bodies WHERE stage = 'online'`)
	if onlineRows == 0 || onlineBodies != onlineRows {
		t.Fatalf("%d online solid bodies for %d online estimates; want one beside each", onlineBodies, onlineRows)
	}
	refined := pipeline.RefinedEstimatorID(onlineEstimatorID)
	for _, smoother := range report.Smoothers {
		stats := smoother.Stats
		t.Logf("%s: %d steps, %d reference changes releasing %d states, %d barriers, %d refined estimates and solid bodies",
			smoother.Lag, stats.Steps, stats.ReferenceChanges, stats.ReleasedAtReferenceChange, stats.Barriers, smoother.Persisted)
		if stats.Released != stats.Steps || stats.HeldSteps != 0 || stats.RevisionsWithoutEvidence != 0 {
			t.Errorf("%s: stats %+v", smoother.Lag, stats)
		}
		if stats.ReferenceChanges == 0 {
			t.Errorf("%s: no chain ended at a reference change on kirk0's moving window", smoother.Lag)
		}
		if smoother.Persisted != onlineRows || smoother.PersistedSolidBodies != smoother.Persisted {
			t.Errorf("%s: %d refined estimates and %d refined solid bodies for %d online rows",
				smoother.Lag, smoother.Persisted, smoother.PersistedSolidBodies, onlineRows)
		}
		if n := count(`SELECT COUNT(*) FROM lidar_track_solid_bodies b JOIN lidar_track_estimates e
			  ON e.observation_id = b.observation_id AND e.estimator_id = b.estimator_id AND e.param_hash = b.param_hash
			 AND e.stage = b.stage AND e.track_id = b.track_id
			 WHERE b.estimator_id = ? AND b.param_hash = ? AND e.x = b.x AND e.y = b.y AND e.vx = b.vx AND e.vy = b.vy
			   AND e.reference_point = b.reference_point AND e.covariance_json = b.covariance_json`,
			refined, smoother.ParamHash); n != smoother.Persisted {
			t.Errorf("%s: %d of %d refined solid bodies carry their estimate's revised state and reference", smoother.Lag, n, smoother.Persisted)
		}
	}
	if n := count(`SELECT COUNT(*) FROM lidar_track_estimate_revisions WHERE release_reason = 'reference_changed'`); n == 0 {
		t.Error("no persisted revision records a reference change")
	}

	store := observationsqlite.NewStateEstimateStore(database)
	if bodies, err := store.ListSolidBodiesBySource(result.ObservationSourceID); err != nil || len(bodies) != onlineBodies {
		t.Errorf("ListSolidBodiesBySource returned %d rows (%v) with refined stages present, want the %d online ones", len(bodies), err, onlineBodies)
	}
	oracle, err := observationsqlite.BuildEvidenceOracle(database.DB, map[string]struct{}{result.ObservationSourceID: {}})
	if err != nil {
		t.Fatalf("evidence oracle over online and refined solid bodies: %v", err)
	}
	want := onlineBodies * (1 + len(RefinementHorizons))
	if len(oracle.Tables) != 4 || oracle.Tables[3].Name != "lidar_track_solid_bodies" || oracle.Tables[3].RowCount != int64(want) {
		t.Errorf("oracle tables %+v, want %d solid bodies covered", oracle.Tables, want)
	}

	run, err := fieldrun.Run(database, fieldrun.Spec{SourceID: result.ObservationSourceID, SolidBodies: true, Stage: l8behaviour.StageFinal})
	if err != nil {
		t.Fatalf("field run over the final solid bodies: %v", err)
	}
	if run.Estimates.Stage != "final" || len(run.Trajectories) == 0 {
		t.Fatalf("field run read %+v, %d trajectories", run.Estimates, len(run.Trajectories))
	}
	for _, tr := range run.Trajectories {
		for _, s := range tr.Samples {
			if s.Stage != l8behaviour.StageFinal {
				t.Fatalf("track %s: a final row read back at %s", tr.Passage.TrackID, s.Stage)
			}
		}
	}
}
