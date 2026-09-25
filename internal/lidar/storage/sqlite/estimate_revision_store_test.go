package sqlite

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// A refined stage lives beside the online estimate it revises: same
// observation, same frame, its own version key and estimate ID, and a
// revision record. These tests pin that it can never replace the online row,
// that the older source-wide readers keep returning only what they were
// written for, and that a batch is all or nothing.

func revisionFixture(trackID, observationID string, frame int64, stage, paramHash string) (TrackEstimate, TrackResidual, RevisedStateEstimate) {
	online := TrackEstimate{
		EstimateID: fmt.Sprintf("estimate/%s/cv_kf_v1/medoid_v0/%d", trackID, frame), TrackID: trackID,
		ObservationID: observationID, SourceID: "source/v1/test", CalibrationID: "calibration/v1/test",
		FrameUnixNanos: frame, MeasurementUnixNanos: frame, EstimatorID: "cv_kf_v1", ObservationModelID: "medoid_v0",
		ParamHash: "sha256:online", Stage: EstimateStageOnline, MeasurementSource: "medoid_v0", CreationSequence: 2,
		X: 1, Y: 2, VX: 3, VY: 4, Covariance: [16]float32{0.1, 0, 0, 0, 0, 0.1},
	}
	residual := TrackResidual{
		EstimateID: online.EstimateID, ObservationID: observationID, PredictedX: 0.9, PredictedY: 1.9,
		MeasurementX: 1.1, MeasurementY: 2.1, InnovationX: 0.2, InnovationY: 0.2, NIS: 0.8,
		Disposition: "accepted", Reason: "association_accepted",
	}
	refined := online
	refined.EstimateID = fmt.Sprintf("estimate/%s/cv_kf_v1+%s/medoid_v0/%s/%s/%d", trackID, "rts_fixed_assignment_v1", paramHash, stage, frame)
	refined.EstimatorID = "cv_kf_v1+rts_fixed_assignment_v1"
	refined.ParamHash = paramHash
	refined.Stage = stage
	refined.X, refined.Y = 1.05, 2.02
	refinedResidual := residual
	refinedResidual.EstimateID = refined.EstimateID
	refinedResidual.Reason = "fixed_assignment_" + stage
	return online, residual, RevisedStateEstimate{
		Estimate: refined, Residual: refinedResidual,
		Revision: EstimateRevision{
			EstimateID: refined.EstimateID, RevisesEstimateID: online.EstimateID,
			SmootherID: "rts_fixed_assignment_v1", Lag: "3f", LookaheadSteps: 3, LookaheadSecs: 0.3,
			ReleasedAtUnixNanos: frame + 300, ReleaseReason: "lag", Flags: []string{"clamped_prediction"},
			PreviousX: online.X, PreviousY: online.Y, PreviousVX: online.VX, PreviousVY: online.VY,
			RevisionPositionMetres: 0.054, RevisionVelocityMps: 0.1, EvidenceCount: 2,
			EvidenceFirstFrameUnixNanos: frame + 100, EvidenceLastFrameUnixNanos: frame + 300,
			StrongestEvidenceObservationID: "observation/v1/later", StrongestEvidenceNIS: 4.5,
		},
	}
}

func TestRevisedEstimatesNeverReplaceOnlineRows(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	online, residual, fixedLag := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFixedLag, "sha256:lag3f")
	_, _, final := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFinal, "sha256:track")
	if err := store.Insert(online, residual); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertRevised([]RevisedStateEstimate{fixedLag, final}); err != nil {
		t.Fatal(err)
	}
	// Writing the same versions again replaces only themselves.
	if err := store.InsertRevised([]RevisedStateEstimate{fixedLag}); err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := database.QueryRow(`SELECT COUNT(*) FROM lidar_track_estimates WHERE observation_id = ?`, online.ObservationID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 3 {
		t.Fatalf("%d estimates for the observation, want online + fixed_lag + final", rows)
	}
	var x float32
	var stage string
	if err := database.QueryRow(`SELECT x, stage FROM lidar_track_estimates WHERE estimate_id = ?`, online.EstimateID).Scan(&x, &stage); err != nil {
		t.Fatal(err)
	}
	if x != online.X || stage != EstimateStageOnline {
		t.Fatalf("online row changed to x=%v stage=%q", x, stage)
	}

	// The source-wide readers see the online population only.
	listed, err := store.ListBySource(online.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Stage != EstimateStageOnline {
		t.Fatalf("ListBySource returned %d rows: %+v", len(listed), listed)
	}
	frames, err := store.ListFrameStateEstimatesBySource(online.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].Estimate.EstimateID != online.EstimateID {
		t.Fatalf("ListFrameStateEstimatesBySource returned %d rows", len(frames))
	}

	// The refined read path names its version and returns each item as it was
	// written: estimate, residual and audit record.
	got, err := store.ListRevisedEstimates(EstimateVersionKey{
		SourceID: online.SourceID, EstimatorID: fixedLag.Estimate.EstimatorID,
		ObservationModelID: "medoid_v0", ParamHash: "sha256:lag3f", Stage: EstimateStageFixedLag,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("ListRevisedEstimates returned %d rows", len(got))
	}
	want := fixedLag
	if !reflect.DeepEqual(got[0].Revision, want.Revision) || got[0].Estimate != want.Estimate || got[0].Residual != want.Residual {
		t.Fatalf("round trip differs:\n got %+v\nwant %+v", got[0], want)
	}
	if got[0].Residual.Reason == "" || got[0].Residual.EstimateID != want.Estimate.EstimateID {
		t.Fatalf("refined residual not returned: %+v", got[0].Residual)
	}
}

func TestRevisedEstimateValidation(t *testing.T) {
	_, _, good := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFixedLag, "sha256:lag3f")
	cases := map[string]func(*RevisedStateEstimate){
		"online stage":            func(r *RevisedStateEstimate) { r.Estimate.Stage = EstimateStageOnline },
		"revises itself":          func(r *RevisedStateEstimate) { r.Revision.RevisesEstimateID = r.Estimate.EstimateID },
		"no revised estimate":     func(r *RevisedStateEstimate) { r.Revision.RevisesEstimateID = "" },
		"describes another":       func(r *RevisedStateEstimate) { r.Revision.EstimateID = "estimate/other" },
		"no smoother":             func(r *RevisedStateEstimate) { r.Revision.SmootherID = "" },
		"negative evidence count": func(r *RevisedStateEstimate) { r.Revision.EvidenceCount = -1 },
	}
	for name, mutate := range cases {
		item := good
		mutate(&item)
		if err := validateRevisedStateEstimate(item); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := validateRevisedStateEstimate(good); err != nil {
		t.Fatalf("valid revision refused: %v", err)
	}
}

// A batch that fails part-way leaves nothing behind: the residual check runs
// inside the transaction, after the first item has been written.
func TestRevisedEstimateBatchIsAtomic(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	_, _, first := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFixedLag, "sha256:lag3f")
	_, _, second := revisionFixture("trk_a", "observation/v1/b", 200, EstimateStageFixedLag, "sha256:lag3f")
	second.Residual.Reason = ""
	if err := InsertRevisedStateEstimates(database, []RevisedStateEstimate{first, second}); err == nil {
		t.Fatal("a batch with an invalid residual was accepted")
	}
	for _, table := range []string{"lidar_track_estimates", "lidar_track_residuals", "lidar_track_estimate_revisions"} {
		var n int
		if err := database.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s kept %d rows from a failed batch", table, n)
		}
	}
}

// A refined estimate whose ID collides with a stored row of another version
// is refused inside the transaction, whatever the ID looks like: the online
// row survives untouched, the batch writes nothing, and one stage cannot
// replace another either.
func TestRevisedEstimateCannotReplaceAnotherVersionByID(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	online, residual, fixedLag := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFixedLag, "sha256:lag3f")
	_, _, other := revisionFixture("trk_a", "observation/v1/b", 200, EstimateStageFixedLag, "sha256:lag3f")
	if err := store.Insert(online, residual); err != nil {
		t.Fatal(err)
	}

	colliding := fixedLag
	colliding.Estimate.EstimateID = online.EstimateID
	colliding.Residual.EstimateID = online.EstimateID
	colliding.Revision.EstimateID = online.EstimateID
	colliding.Revision.RevisesEstimateID = "estimate/elsewhere"
	err := store.InsertRevised([]RevisedStateEstimate{other, colliding})
	if err == nil || !strings.Contains(err.Error(), "another version") {
		t.Fatalf("a refined estimate reusing the online ID was not refused: %v", err)
	}
	var x float32
	var stage string
	if err := database.QueryRow(`SELECT x, stage FROM lidar_track_estimates WHERE estimate_id = ?`, online.EstimateID).Scan(&x, &stage); err != nil {
		t.Fatal(err)
	}
	if x != online.X || stage != EstimateStageOnline {
		t.Fatalf("online row replaced: x=%v stage=%q", x, stage)
	}
	for _, table := range []string{"lidar_track_estimates", "lidar_track_residuals"} {
		var n int
		if err := database.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("%s holds %d rows after a refused batch, want the online row alone", table, n)
		}
	}

	// Nor may one refined stage take over another's row.
	if err := store.InsertRevised([]RevisedStateEstimate{fixedLag}); err != nil {
		t.Fatal(err)
	}
	final := fixedLag
	final.Estimate.Stage = EstimateStageFinal
	if err := store.InsertRevised([]RevisedStateEstimate{final}); err == nil {
		t.Fatal("a final estimate replaced the fixed_lag row under its ID")
	}
}

// The evidence oracle must stay reproducible when an observation carries
// online and refined estimates: rows written in either order give one digest.
func TestEvidenceOracleOrdersRefinedStagesDeterministically(t *testing.T) {
	build := func(reverse bool) EvidenceOracle {
		db, cleanup := setupTrackingPipelineTestDB(t)
		defer cleanup()
		if _, err := db.Exec(`INSERT INTO lidar_observations (observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos, cluster_unix_nanos, cluster_id, record_json, inserted_at_ns) VALUES ('observation/v1/a', 1, 'source/v1/test', 'calibration/v1/test', 'sensor', 'frame', 100, 100, 1, '{}', 1)`); err != nil {
			t.Fatal(err)
		}
		online, residual, fixedLag := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFixedLag, "sha256:lag3f")
		_, _, final := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFinal, "sha256:track")
		writes := []func(*sql.DB) error{
			func(db *sql.DB) error { return InsertStateEstimate(db, online, residual) },
			func(db *sql.DB) error { return InsertRevisedStateEstimates(db, []RevisedStateEstimate{fixedLag}) },
			func(db *sql.DB) error { return InsertRevisedStateEstimates(db, []RevisedStateEstimate{final}) },
		}
		if reverse {
			writes[0], writes[2] = writes[2], writes[0]
		}
		for _, write := range writes {
			if err := write(db); err != nil {
				t.Fatal(err)
			}
		}
		oracle, err := BuildEvidenceOracle(db, map[string]struct{}{"source/v1/test": {}})
		if err != nil {
			t.Fatal(err)
		}
		return oracle
	}
	forward, backward := build(false), build(true)
	if !reflect.DeepEqual(forward, backward) {
		t.Fatalf("write order changed the oracle:\n%+v\n%+v", forward.Tables, backward.Tables)
	}
	if forward.Tables[1].RowCount != 3 || !strings.HasPrefix(forward.Tables[1].SHA256, "sha256:") {
		t.Fatalf("estimates table %+v", forward.Tables[1])
	}
}
