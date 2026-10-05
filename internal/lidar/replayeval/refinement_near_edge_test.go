package replayeval

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
)

// The fixed_lag_rts harness over a near_edge_track tracker, without a
// capture: the synthetic pass, with a faceless run so the vehicle's track
// re-references at its first fix, lapses and re-references again. Every arm
// must break its chains at those changes, persist a refined solid body beside
// each refined estimate, and file each beside the estimate it revises.
func TestRefinementHarnessFilesRefinedSolidBodiesUnderNearEdgeTracking(t *testing.T) {
	cfg := l5tracks.DefaultTrackerConfig()
	cfg.SolidBody = l5tracks.SolidBodyOptions{Enabled: true, OriginSource: "test: synthetic pass in the sensor frame"}
	cfg.NearEdgeTracking = true
	tracker := l5tracks.NewTracker(cfg)
	database, cleanup := db.NewTestDB(t)
	defer cleanup()
	identity := &pipeline.RefinedEstimateIdentity{SourceID: "source/v1/synthetic", CalibrationID: "calibration/v1/identity",
		OnlineEstimatorID: onlineEstimatorID, ObservationModelID: string(l5tracks.MeasurementNearEdgeCandidateV1),
		OnlineParamHash: "sha256:online"}
	harness, err := newRefinementHarness(float64(cfg.MeasurementNoise), 0, identity.OnlineParamHash,
		identity.ObservationModelID, identity, database)
	if err != nil {
		t.Fatal(err)
	}
	tracker.SetFilterStepObserver(harness)

	params := l4perception.DefaultDBSCANParams()
	params.Eps, params.MinPts, params.MaxSamplePoints = 0.6, 5, 512
	base := time.Unix(1_700_000_000, 0).Sub(time.Unix(0, 0))
	const faceless = 14
	for i, f := range l4perception.GenerateSyntheticPass(l4perception.DefaultSyntheticPass()) {
		points := make([]l4perception.WorldPoint, len(f.Points))
		for n, p := range f.Points {
			p.Timestamp = p.Timestamp.Add(base)
			points[n] = p
		}
		clusters := l4perception.DBSCAN(points, params)
		if i >= faceless && i < faceless+cfg.MaxMisses {
			for c := range clusters {
				clusters[c].RetainedPoints = nil
			}
		}
		tracker.Update(clusters, f.Timestamp.Add(base))
		harness.flush()
	}
	report, err := harness.finish(identity.SourceID)
	if err != nil {
		t.Fatal(err)
	}

	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := database.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, smoother := range report.Smoothers {
		stats := smoother.Stats
		if stats.ReferenceChanges < 3 || stats.ReleasedAtReferenceChange == 0 || stats.Released != stats.Steps || stats.RevisionsWithoutEvidence != 0 {
			t.Errorf("%s: stats %+v; want the fix, the lapse and the second fix to end chains", smoother.Lag, stats)
		}
		if smoother.Persisted == 0 || smoother.PersistedSolidBodies != smoother.Persisted {
			t.Errorf("%s: %d refined solid bodies for %d refined estimates", smoother.Lag, smoother.PersistedSolidBodies, smoother.Persisted)
		}
		estimator := pipeline.RefinedEstimatorID(onlineEstimatorID)
		if n := count(`SELECT COUNT(*) FROM lidar_track_solid_bodies WHERE estimator_id = ? AND param_hash = ?`, estimator, smoother.ParamHash); n != smoother.Persisted {
			t.Errorf("%s: %d refined solid bodies stored, %d reported", smoother.Lag, n, smoother.Persisted)
		}
		// Each refined body is the revised state of the estimate it is filed
		// beside, at the reference that estimate states.
		if n := count(`SELECT COUNT(*) FROM lidar_track_solid_bodies b JOIN lidar_track_estimates e
			  ON e.observation_id = b.observation_id AND e.estimator_id = b.estimator_id AND e.param_hash = b.param_hash
			 AND e.stage = b.stage AND e.track_id = b.track_id
			 WHERE b.param_hash = ? AND e.x = b.x AND e.y = b.y AND e.vx = b.vx AND e.vy = b.vy
			   AND e.reference_point = b.reference_point AND e.covariance_json = b.covariance_json`, smoother.ParamHash); n != smoother.Persisted {
			t.Errorf("%s: %d of %d refined solid bodies match their estimate", smoother.Lag, n, smoother.Persisted)
		}
	}
	if n := count(`SELECT COUNT(*) FROM lidar_track_estimate_revisions WHERE release_reason = 'reference_changed'`); n == 0 {
		t.Error("no persisted revision records a reference change")
	}
	// The faceless frames' refined residuals say, as the online ones would,
	// that the filter did not apply them.
	if n := count(`SELECT COUNT(*) FROM lidar_track_residuals WHERE disposition IN ('not_applied', 'reference_changed')`); n == 0 {
		t.Error("no refined residual records an observation the filter did not apply")
	}
}

// A replay without near_edge_track writes the refinement report it wrote
// before refined solid bodies and reference changes were counted.
func TestRefinementReportOmitsWhatADefaultReplayNeverHas(t *testing.T) {
	report := RefinementSmootherReport{Lag: "3f", Stats: l5tracks.SmootherStats{Steps: 4, Released: 4}, Persisted: 4}
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"persisted_solid_bodies", "reference_change"} {
		if strings.Contains(string(payload), key) {
			t.Errorf("a default smoother report writes %q: %s", key, payload)
		}
	}
}
