package sqlite

import (
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// Refined solid bodies (near-edge plan S2.4): a smoothed solid body is filed
// beside its refined point estimate, in the same transaction and version,
// with the revised state and the online beliefs. These tests pin that it is
// written and read back as it was given, that it cannot be filed beside an
// estimate it does not describe, that the source-wide reader keeps returning
// online bodies only, and that it cannot replace another version's row.

// refinedBodyFor is the solid body filed beside a refined estimate: the
// estimate's version, track, observation and revised state, with testSolidBody's
// beliefs standing in for the online body's.
func refinedBodyFor(item RevisedStateEstimate) TrackSolidBody {
	e := item.Estimate
	sb := testSolidBody(e.ObservationID, e.FrameUnixNanos)
	sb.EstimateID = strings.Replace(e.EstimateID, "estimate/", "solid_body/", 1)
	sb.TrackID, sb.CreationSequence = e.TrackID, e.CreationSequence
	sb.SourceID, sb.CalibrationID = e.SourceID, e.CalibrationID
	sb.EstimatorID, sb.ParamHash, sb.Stage = e.EstimatorID, e.ParamHash, e.Stage
	sb.MeasurementUnixNanos = e.MeasurementUnixNanos
	r := &sb.Reading
	r.Estimate.Stage = l5tracks.StageSmoothed
	r.Estimate.Reference = e.Reference
	r.Estimate.X, r.Estimate.Y, r.VX, r.VY = e.X, e.Y, e.VX, e.VY
	r.Covariance = e.Covariance
	r.Estimate.PositionCovariance = [4]float32{e.Covariance[0], e.Covariance[1], e.Covariance[4], e.Covariance[5]}
	return sb
}

// withRefinedBody attaches the matching refined body to a revision fixture.
func withRefinedBody(item RevisedStateEstimate) RevisedStateEstimate {
	sb := refinedBodyFor(item)
	item.SolidBody = &sb
	return item
}

func TestRefinedSolidBodiesAreWrittenBesideTheirEstimatesAndReadBack(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	online, residual, fixedLag := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFixedLag, "sha256:lag3f")
	_, _, final := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFinal, "sha256:track")
	fixedLag, final = withRefinedBody(fixedLag), withRefinedBody(final)
	insertBareObservation(t, database, online.ObservationID, online.SourceID, "sensor_a", online.FrameUnixNanos)
	if _, err := database.Exec(`UPDATE lidar_observations SET calibration_id = ? WHERE observation_id = ?`, online.CalibrationID, online.ObservationID); err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(online, residual); err != nil {
		t.Fatal(err)
	}
	onlineBody := testSolidBody(online.ObservationID, online.FrameUnixNanos)
	onlineBody.SourceID, onlineBody.CalibrationID = online.SourceID, online.CalibrationID
	if err := store.InsertSolidBody(onlineBody); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertRevised([]RevisedStateEstimate{fixedLag, final}); err != nil {
		t.Fatal(err)
	}
	// Writing the same versions again replaces only themselves.
	if err := store.InsertRevised([]RevisedStateEstimate{fixedLag}); err != nil {
		t.Fatal(err)
	}

	for _, item := range []RevisedStateEstimate{fixedLag, final} {
		key := EstimateVersionKey{SourceID: item.Estimate.SourceID, EstimatorID: item.Estimate.EstimatorID,
			ObservationModelID: item.SolidBody.ObservationModelID, ParamHash: item.Estimate.ParamHash, Stage: item.Estimate.Stage}
		got, err := store.ListVersionSolidBodies(key)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].SensorID != "sensor_a" || !reflect.DeepEqual(got[0].TrackSolidBody, *item.SolidBody) {
			t.Fatalf("%s solid body did not round-trip:\n got %+v\nwant %+v", item.Estimate.Stage, got, *item.SolidBody)
		}
		if got[0].Reading.Estimate.Stage != l5tracks.StageSmoothed {
			t.Fatalf("a %s row read back as %s", item.Estimate.Stage, got[0].Reading.Estimate.Stage)
		}
	}

	// The source-wide reader is the online population only, as ListBySource is.
	bodies, err := store.ListSolidBodiesBySource(online.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 1 || !reflect.DeepEqual(bodies[0], onlineBody) {
		t.Fatalf("ListSolidBodiesBySource returned %d rows with refined stages present: %+v", len(bodies), bodies)
	}
	versions, err := store.ListSolidBodyVersions()
	if err != nil {
		t.Fatal(err)
	}
	var stages []string
	for _, v := range versions {
		stages = append(stages, v.Stage)
	}
	if strings.Join(stages, ",") != "online,fixed_lag,final" {
		t.Fatalf("solid-body versions %v, want the online one and one per refined stage", stages)
	}
}

func TestRefinedSolidBodyMustDescribeItsEstimate(t *testing.T) {
	_, _, good := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFixedLag, "sha256:lag3f")
	good = withRefinedBody(good)
	if err := validateRevisedStateEstimate(good); err != nil {
		t.Fatalf("a matching refined solid body was refused: %v", err)
	}
	cases := map[string]func(*TrackSolidBody){
		"another stage":       func(sb *TrackSolidBody) { sb.Stage = EstimateStageFinal },
		"another estimator":   func(sb *TrackSolidBody) { sb.EstimatorID = "cv_kf_v1" },
		"another param hash":  func(sb *TrackSolidBody) { sb.ParamHash = "sha256:other" },
		"another track":       func(sb *TrackSolidBody) { sb.TrackID = "trk_b" },
		"another sequence":    func(sb *TrackSolidBody) { sb.CreationSequence++ },
		"another observation": func(sb *TrackSolidBody) { sb.ObservationID = "observation/v1/b" },
		"another source":      func(sb *TrackSolidBody) { sb.SourceID = "source/v1/other" },
		"another calibration": func(sb *TrackSolidBody) { sb.CalibrationID = "calibration/v1/other" },
		"another frame":       func(sb *TrackSolidBody) { sb.FrameUnixNanos++ },
		"another reference":   func(sb *TrackSolidBody) { sb.Reading.Estimate.Reference = l5tracks.ReferenceClusterMedoid },
		"the online position": func(sb *TrackSolidBody) { sb.Reading.Estimate.X = 1 },
		"the online velocity": func(sb *TrackSolidBody) { sb.Reading.VY = 0 },
		"another covariance":  func(sb *TrackSolidBody) { sb.Reading.Covariance[10] = 9 },
	}
	for name, mutate := range cases {
		item := good
		sb := *good.SolidBody
		mutate(&sb)
		item.SolidBody = &sb
		if err := validateRevisedStateEstimate(item); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// A refined solid body that cannot be stored takes its estimate and revision
// with it: the batch is all or nothing across the three tables and the
// fourth.
func TestRefinedSolidBodyBatchIsAtomic(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	_, _, first := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFixedLag, "sha256:lag3f")
	_, _, second := revisionFixture("trk_a", "observation/v1/b", 200, EstimateStageFixedLag, "sha256:lag3f")
	first, second = withRefinedBody(first), withRefinedBody(second)
	// Its own contents are checked on insert, inside the transaction: a live
	// body filed under a refined stage is refused there.
	second.SolidBody.Reading.Estimate.Stage = l5tracks.StageLive
	if err := InsertRevisedStateEstimates(database, []RevisedStateEstimate{first, second}); err == nil {
		t.Fatal("a batch with a live body filed as fixed_lag was accepted")
	}
	for _, table := range []string{"lidar_track_estimates", "lidar_track_residuals", "lidar_track_estimate_revisions", "lidar_track_solid_bodies"} {
		var n int
		if err := database.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s kept %d rows from a failed batch", table, n)
		}
	}
}

// A refined solid body whose ID collides with a stored body of another
// version, the online one included, is refused, and the online body survives.
func TestRefinedSolidBodyCannotReplaceAnotherVersionByID(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	_, _, fixedLag := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFixedLag, "sha256:lag3f")
	fixedLag = withRefinedBody(fixedLag)
	onlineBody := testSolidBody(fixedLag.Estimate.ObservationID, 100)
	onlineBody.EstimateID = fixedLag.SolidBody.EstimateID
	if err := store.InsertSolidBody(onlineBody); err != nil {
		t.Fatal(err)
	}
	err := store.InsertRevised([]RevisedStateEstimate{fixedLag})
	if err == nil || !strings.Contains(err.Error(), "another version") {
		t.Fatalf("a refined body reusing the online body's ID was not refused: %v", err)
	}
	var stage string
	var x float64
	if err := database.QueryRow(`SELECT stage, x FROM lidar_track_solid_bodies WHERE estimate_id = ?`, onlineBody.EstimateID).Scan(&stage, &x); err != nil {
		t.Fatal(err)
	}
	if stage != EstimateStageOnline || float32(x) != onlineBody.Reading.Estimate.X {
		t.Fatalf("online body replaced: stage %q x %v", stage, x)
	}
	var estimates int
	if err := database.QueryRow(`SELECT COUNT(*) FROM lidar_track_estimates`).Scan(&estimates); err != nil {
		t.Fatal(err)
	}
	if estimates != 0 {
		t.Fatalf("the refused batch left %d estimates", estimates)
	}
}

// A database error while checking for a stored body under the refined body's
// ID fails the batch, and the estimate already written in the same
// transaction is rolled back with it, rather than read as "no stored body".
func TestRefinedSolidBodyCheckErrorRollsBackTheBatch(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	_, _, fixedLag := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFixedLag, "sha256:lag3f")
	fixedLag = withRefinedBody(fixedLag)
	// The estimate, residual and revision inserts do not touch the solid-body
	// table, so the check is the first statement to fail.
	if _, err := database.Exec(`ALTER TABLE lidar_track_solid_bodies RENAME TO lidar_track_solid_bodies_moved`); err != nil {
		t.Fatal(err)
	}
	err := InsertRevisedStateEstimates(database, []RevisedStateEstimate{fixedLag})
	if err == nil || !strings.Contains(err.Error(), "check existing solid body") {
		t.Fatalf("got %v, want the solid-body check's error", err)
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

// The stage vocabulary a solid body is filed under: live is online only, and
// smoothed is fixed_lag or final, the stages the smoother releases.
func TestSolidBodyStageAgreesWithItsVersionStage(t *testing.T) {
	for _, tc := range []struct {
		stage l5tracks.EstimateStage
		as    string
		want  bool
	}{
		{l5tracks.StageLive, EstimateStageOnline, true},
		{l5tracks.StageLive, EstimateStageFixedLag, false},
		{l5tracks.StageLive, EstimateStageFinal, false},
		{l5tracks.StageSmoothed, EstimateStageOnline, false},
		{l5tracks.StageSmoothed, EstimateStageFixedLag, true},
		{l5tracks.StageSmoothed, EstimateStageFinal, true},
		{l5tracks.StageSmoothed, "draft", false},
	} {
		if got := stageAgrees(tc.stage, tc.as); got != tc.want {
			t.Errorf("%s filed as %q: %v, want %v", tc.stage, tc.as, got, tc.want)
		}
	}
}
