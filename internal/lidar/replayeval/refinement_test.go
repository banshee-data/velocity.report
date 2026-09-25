package replayeval

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
)

// The harness without a capture: a synthetic two-object scene through the
// shipped tracker, observed by the fixed_lag_rts harness. Every horizon must
// report the online arm's population, no revision without evidence, and a
// distinct version key; the online arm must carry the replay's own key.
func TestRefinementHarnessReportsEveryHorizonOnOnePopulation(t *testing.T) {
	cfg := l5tracks.DefaultTrackerConfig()
	tracker := l5tracks.NewTracker(cfg)
	epoch := time.Unix(1_750_000_000, 0)
	harness, err := newRefinementHarness(float64(cfg.MeasurementNoise), epoch.Add(time.Second).UnixNano(),
		"sha256:online", "medoid_v0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tracker.SetFilterStepObserver(harness)
	for k := 0; k < 60; k++ {
		secs := 0.1 * float64(k)
		var clusters []l5tracks.WorldCluster
		for o := 0; o < 2; o++ {
			if o == 1 && k >= 30 && k < 34 {
				continue // an occlusion, so coasted states are refined too
			}
			clusters = append(clusters, l5tracks.WorldCluster{
				ClusterID: int64(2*k + o + 1), SensorID: "harness",
				CentroidX: float32(1.5*secs) + float32(o)*30, CentroidY: float32(10*o) + 0.03*float32(k%3),
			})
		}
		tracker.Update(clusters, epoch.Add(time.Duration(secs*float64(time.Second))))
		harness.flush()
	}
	report, err := harness.finish("")
	if err != nil {
		t.Fatal(err)
	}
	if report.Persisted || len(report.Arms) != len(RefinementHorizons)+1 || len(report.Smoothers) != len(RefinementHorizons) {
		t.Fatalf("report shape: persisted=%v arms=%d smoothers=%d", report.Persisted, len(report.Arms), len(report.Smoothers))
	}
	online := report.Arms[0]
	if online.Label != "online" || online.EstimatorID != onlineEstimatorID || online.ParamHash != "sha256:online" || online.States == 0 {
		t.Fatalf("online arm %+v", online)
	}
	keys := map[string]bool{online.ParamHash: true}
	for i, arm := range report.Arms[1:] {
		if arm.States != online.States || arm.ObservedStates != online.ObservedStates {
			t.Errorf("%s scored %d states, online %d", arm.Label, arm.States, online.States)
		}
		if arm.RevisionsWithoutEvidence != 0 {
			t.Errorf("%s: %d revisions without evidence", arm.Label, arm.RevisionsWithoutEvidence)
		}
		if keys[arm.ParamHash] || arm.EstimatorID != pipeline.RefinedEstimatorID(onlineEstimatorID) {
			t.Errorf("%s: version key %s / %s is not distinct", arm.Label, arm.EstimatorID, arm.ParamHash)
		}
		keys[arm.ParamHash] = true
		if stats := report.Smoothers[i].Stats; stats.Released != stats.Steps || stats.HeldSteps != 0 {
			t.Errorf("%s: released %d of %d", arm.Label, stats.Released, stats.Steps)
		}
		if report.Smoothers[i].Persisted != 0 {
			t.Errorf("%s persisted without a database", arm.Label)
		}
	}
	final := report.Arms[len(report.Arms)-1]
	if final.Stage != string(l5tracks.RefinementFinal) || math.IsNaN(final.SpeedStepRMSMps2) {
		t.Fatalf("final arm %+v", final)
	}
}

// Persistence needs identities and a database together: an identity without
// one is refused before any frame is processed.
func TestRefinementHarnessRefusesPersistenceWithoutADatabase(t *testing.T) {
	identity := &pipeline.RefinedEstimateIdentity{SourceID: "s", CalibrationID: "c", OnlineEstimatorID: onlineEstimatorID,
		ObservationModelID: "m", OnlineParamHash: "p"}
	if _, err := newRefinementHarness(0.05, 0, "p", "m", identity, nil); err == nil {
		t.Fatal("accepted a persistence identity without a database")
	}
	database, err := db.NewDB(filepath.Join(t.TempDir(), "refinement.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := newRefinementHarness(0.05, 0, "p", "m", identity, database); err != nil {
		t.Fatal(err)
	}
}
