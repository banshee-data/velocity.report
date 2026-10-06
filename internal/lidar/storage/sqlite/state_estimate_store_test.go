package sqlite

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestStateEstimateStoreWritesVersionedEstimateAndResidual(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	estimate := TrackEstimate{
		EstimateID: "estimate/v1/one", TrackID: "track/one", ObservationID: "observation/v1/one",
		SourceID: "source/v1/test", CalibrationID: "calibration/v1/test", FrameUnixNanos: 100,
		MeasurementUnixNanos: 101, EstimatorID: "cv_kf_v1", ObservationModelID: "obb_centre_v1",
		ParamHash: "params/test", Stage: "online", MeasurementSource: "obb_centre_v1", X: 1, Y: 2, VX: 3, VY: 4,
		Covariance: [16]float32{1, 2, 3, 4},
		Reference:  l5tracks.ReferenceVisibleOBBCentre, Support: l5tracks.SupportObserved,
	}
	residual := TrackResidual{
		EstimateID: estimate.EstimateID, ObservationID: estimate.ObservationID, PredictedX: 0.5, PredictedY: 1.5,
		MeasurementX: 1, MeasurementY: 2, InnovationX: 0.5, InnovationY: 0.5, NIS: 1.25,
		GeometryCovXX: 0.2, GeometryCovXY: 0.01, GeometryCovYY: 0.3, Disposition: "accepted", Reason: "association_accepted",
	}
	if err := store.Insert(estimate, residual); err != nil {
		t.Fatal(err)
	}
	var gotTrack, gotObservation, gotSource, gotCalibration, gotModel, gotReference, gotSupport string
	var gotTime int64
	if err := database.QueryRow(`SELECT track_id, observation_id, source_id, calibration_id, observation_model_id, measurement_unix_nanos, reference_point, support_instant FROM lidar_track_estimates WHERE estimate_id = ?`, estimate.EstimateID).Scan(&gotTrack, &gotObservation, &gotSource, &gotCalibration, &gotModel, &gotTime, &gotReference, &gotSupport); err != nil {
		t.Fatal(err)
	}
	if gotTrack != estimate.TrackID || gotObservation != estimate.ObservationID || gotSource != estimate.SourceID || gotCalibration != estimate.CalibrationID || gotModel != estimate.ObservationModelID || gotTime != estimate.MeasurementUnixNanos {
		t.Fatalf("estimate identity lost: %q %q %q %q %q %d", gotTrack, gotObservation, gotSource, gotCalibration, gotModel, gotTime)
	}
	if gotReference != "visible_obb_centre" || gotSupport != "observed" {
		t.Fatalf("reference %q and support %q, want the estimate's tokens", gotReference, gotSupport)
	}
	var disposition, reason string
	if err := database.QueryRow(`SELECT disposition, reason FROM lidar_track_residuals WHERE estimate_id = ?`, estimate.EstimateID).Scan(&disposition, &reason); err != nil {
		t.Fatal(err)
	}
	if disposition != "accepted" || reason != "association_accepted" {
		t.Fatalf("residual decision = %q %q", disposition, reason)
	}
}

func TestStateEstimateStoreRejectsMismatchedResidualIdentity(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()
	estimate := TrackEstimate{EstimateID: "e", TrackID: "t", ObservationID: "o", SourceID: "s", CalibrationID: "c", EstimatorID: "e", ObservationModelID: "m", ParamHash: "p", Stage: "online", MeasurementSource: "obb",
		Reference: l5tracks.ReferenceClusterMedoid, Support: l5tracks.SupportObserved}
	err := validateStateEstimate(estimate, TrackResidual{EstimateID: "other", ObservationID: "o", Disposition: "accepted", Reason: "ok"})
	if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "residual") {
		t.Fatalf("mismatched residual identity error = %v", err)
	}
}

// stateEstimateFixture is a valid online estimate stating reference and
// support, with its residual.
func stateEstimateFixture(i int, reference l5tracks.ReferencePoint, support l5tracks.ObservationSupport) (TrackEstimate, TrackResidual) {
	e := TrackEstimate{
		EstimateID: fmt.Sprintf("estimate/tokens/%d", i), TrackID: "track/tokens",
		ObservationID: fmt.Sprintf("observation/tokens/%d", i), SourceID: "source/v1/tokens",
		CalibrationID: "calibration/v1/test", FrameUnixNanos: int64(100 + i), MeasurementUnixNanos: int64(100 + i),
		EstimatorID: "cv_kf_v1", ObservationModelID: "obb_centre_v1", ParamHash: "params/tokens", Stage: "online",
		MeasurementSource: "obb_centre_v1", CreationSequence: 1, X: float32(i),
		Reference: reference, Support: support,
	}
	return e, TrackResidual{EstimateID: e.EstimateID, ObservationID: e.ObservationID, Disposition: "accepted", Reason: "association_accepted"}
}

// Every reference point and support token the l5tracks vocabulary names is
// written as the estimate states it and read back as itself by each reader,
// whatever the measurement source says: every row here was measured at the
// OBB centre.
func TestStateEstimateStoreRoundTripsReferenceAndSupport(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	references := []l5tracks.ReferencePoint{l5tracks.ReferenceBodyCentre, l5tracks.ReferenceNearFaceCentre,
		l5tracks.ReferenceClusterMedoid, l5tracks.ReferenceVisibleOBBCentre}
	supports := []l5tracks.ObservationSupport{l5tracks.SupportObserved, l5tracks.SupportCoasted,
		l5tracks.SupportOccludedInferred, l5tracks.SupportMissedUnknown, l5tracks.SupportClusterMerged,
		l5tracks.SupportClusterSplit, l5tracks.SupportOutOfFOV}
	var want []TrackEstimate
	for i, support := range supports {
		e, r := stateEstimateFixture(i, references[i%len(references)], support)
		insertBareObservation(t, database, e.ObservationID, e.SourceID, "sensor_a", e.FrameUnixNanos)
		if err := store.Insert(e, r); err != nil {
			t.Fatal(err)
		}
		want = append(want, e)
	}
	bySource, err := store.ListBySource("source/v1/tokens")
	if err != nil {
		t.Fatal(err)
	}
	byVersion, err := store.ListVersionEstimates(EstimateVersionKey{SourceID: "source/v1/tokens", EstimatorID: "cv_kf_v1",
		ObservationModelID: "obb_centre_v1", ParamHash: "params/tokens", Stage: "online"})
	if err != nil {
		t.Fatal(err)
	}
	if len(bySource) != len(want) || len(byVersion) != len(want) {
		t.Fatalf("read %d and %d rows, wrote %d", len(bySource), len(byVersion), len(want))
	}
	for i, w := range want {
		if bySource[i] != w || byVersion[i].TrackEstimate != w {
			t.Errorf("row %d: ListBySource %s/%q, ListVersionEstimates %s/%q, wrote %s/%q", i,
				bySource[i].Reference, bySource[i].Support, byVersion[i].Reference, byVersion[i].Support, w.Reference, w.Support)
		}
	}
}

// An estimate that states no reference point or no support token, or one no
// token names, is refused before it is written, and the database refuses the
// same row from a writer that bypasses this store.
func TestStateEstimateStoreRefusesAnEstimateThatStatesNeither(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	for name, c := range map[string]struct {
		reference l5tracks.ReferencePoint
		support   l5tracks.ObservationSupport
		want      string
	}{
		"no reference":      {l5tracks.ReferenceUnknown, l5tracks.SupportObserved, "states no reference point"},
		"unnamed reference": {l5tracks.ReferencePoint(99), l5tracks.SupportObserved, "states no reference point"},
		"no support":        {l5tracks.ReferenceClusterMedoid, l5tracks.SupportUnrecorded, "states no support token"},
		"unnamed support":   {l5tracks.ReferenceClusterMedoid, l5tracks.ObservationSupport(99), "states no support token"},
	} {
		e, r := stateEstimateFixture(0, c.reference, c.support)
		if err := store.Insert(e, r); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want %q", name, err, c.want)
		}
	}
	for _, columns := range []string{"", ", reference_point", ", support_instant"} {
		values := strings.Repeat(", 'observed'", strings.Count(columns, ","))
		_, err := database.Exec(`INSERT INTO lidar_track_estimates (estimate_id, track_id, observation_id, source_id,
			calibration_id, frame_unix_nanos, measurement_unix_nanos, estimator_id, observation_model_id, param_hash,
			stage, measurement_source, x, y, vx, vy, covariance_json, inserted_at_ns` + columns + `)
			VALUES ('raw', 't', 'o', 's', 'c', 1, 1, 'cv_kf_v1', 'medoid_v0', 'p', 'online', 'medoid_v0', 0, 0, 0, 0, '[]', 1` + values + `)`)
		if err == nil || !strings.Contains(err.Error(), "states its reference point and support token") {
			t.Errorf("a raw row naming only%q was not refused: %v", columns, err)
		}
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM lidar_track_estimates`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d refused rows were written", n)
	}
}

// A stored value that no l5tracks parser knows is refused by every reader,
// never read as a default. The empty reference is the one migration 000057
// leaves on a row whose measurement source it could not map.
func TestStateEstimateReadersRefuseAnUnknownStatement(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	online, residual, fixedLag := revisionFixture("trk_a", "observation/v1/a", 100, EstimateStageFixedLag, "sha256:lag3f")
	insertBareObservation(t, database, online.ObservationID, online.SourceID, "sensor_a", online.FrameUnixNanos)
	if err := store.Insert(online, residual); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertRevised([]RevisedStateEstimate{fixedLag}); err != nil {
		t.Fatal(err)
	}
	readers := map[string]func() error{
		"ListBySource": func() error { _, err := store.ListBySource(online.SourceID); return err },
		"ListVersionEstimates": func() error {
			_, err := store.ListVersionEstimates(EstimateVersionKey{SourceID: online.SourceID, EstimatorID: online.EstimatorID,
				ObservationModelID: online.ObservationModelID, ParamHash: online.ParamHash, Stage: online.Stage})
			return err
		},
		"ListRevisedEstimates": func() error {
			_, err := store.ListRevisedEstimates(EstimateVersionKey{SourceID: online.SourceID, EstimatorID: fixedLag.Estimate.EstimatorID,
				ObservationModelID: online.ObservationModelID, ParamHash: fixedLag.Estimate.ParamHash, Stage: EstimateStageFixedLag})
			return err
		},
	}
	for name, read := range readers {
		if err := read(); err != nil {
			t.Fatalf("%s before the value was changed: %v", name, err)
		}
	}
	for _, c := range []struct{ column, value, want string }{
		{"reference_point", "", "unknown reference point"},
		{"reference_point", "body_center", "unknown reference point"},
		{"support_instant", "", "unknown support token"},
		{"support_instant", "seen", "unknown support token"},
	} {
		if _, err := database.Exec(`UPDATE lidar_track_estimates SET `+c.column+` = ?`, c.value); err != nil {
			t.Fatal(err)
		}
		for name, read := range readers {
			if err := read(); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s with %s %q: error %v, want %q", name, c.column, c.value, err, c.want)
			}
		}
		if _, err := database.Exec(`UPDATE lidar_track_estimates SET reference_point = 'cluster_medoid', support_instant = 'observed'`); err != nil {
			t.Fatal(err)
		}
	}
}
