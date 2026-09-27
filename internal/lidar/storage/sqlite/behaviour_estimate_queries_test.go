package sqlite

import (
	"fmt"
	"strings"
	"testing"
)

// insertBareObservation stores an observation row with the identity columns
// the join reads. Its payload is a placeholder: this reader never decodes it.
func insertBareObservation(t *testing.T, db DBClient, id, source, sensor string, frame int64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO lidar_observations
		(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos,
		 cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
		VALUES (?, 1, ?, 'calibration/test', ?, 'frame', ?, ?, 1, '{}', 1)`, id, source, sensor, frame, frame); err != nil {
		t.Fatal(err)
	}
}

func versionEstimate(track, stage, paramHash string, frame int64) (TrackEstimate, TrackResidual) {
	id := fmt.Sprintf("estimate/%s/%s/%s/%d", track, stage, paramHash, frame)
	obs := fmt.Sprintf("observation/%d/%s", frame, track)
	e := TrackEstimate{
		EstimateID: id, TrackID: track, ObservationID: obs, SourceID: "source/test", CalibrationID: "calibration/test",
		FrameUnixNanos: frame, MeasurementUnixNanos: frame - 5, EstimatorID: "cv_kf_v1", ObservationModelID: "obb_centre_v1",
		ParamHash: paramHash, Stage: stage, MeasurementSource: "obb_centre_v1", X: float32(frame), VX: 10,
		Covariance: [16]float32{1, 0.5, 0, 0, 0.5, 1, 0, 0, 0, 0, 2, 0, 0, 0, 0, 2},
	}
	return e, TrackResidual{EstimateID: id, ObservationID: obs, Disposition: "accepted", Reason: "association_accepted"}
}

// TestListVersionEstimates reads exactly one version, in track and frame
// order, with each row's observing sensor, and refuses a row whose
// observation is missing.
func TestListVersionEstimates(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	for _, track := range []string{"trk_b", "trk_a"} {
		for _, frame := range []int64{200, 100} {
			insertBareObservation(t, database, fmt.Sprintf("observation/%d/%s", frame, track), "source/test", "sensor_a", frame)
			for _, v := range []struct{ stage, hash string }{{"online", "p1"}, {"final", "p1"}, {"online", "p2"}} {
				e, r := versionEstimate(track, v.stage, v.hash, frame)
				if err := store.Insert(e, r); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	key := EstimateVersionKey{SourceID: "source/test", EstimatorID: "cv_kf_v1", ObservationModelID: "obb_centre_v1",
		ParamHash: "p1", Stage: "final"}
	got, err := store.ListVersionEstimates(key)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, r := range got {
		order = append(order, fmt.Sprintf("%s@%d", r.TrackID, r.FrameUnixNanos))
		if r.Stage != "final" || r.ParamHash != "p1" || r.SensorID != "sensor_a" || r.Covariance[1] != 0.5 || r.VX != 10 {
			t.Errorf("row %s = %+v", r.EstimateID, r)
		}
	}
	if strings.Join(order, " ") != "trk_a@100 trk_a@200 trk_b@100 trk_b@200" {
		t.Errorf("order = %v", order)
	}

	key.Stage = "fixed_lag"
	if none, err := store.ListVersionEstimates(key); err != nil || len(none) != 0 {
		t.Errorf("an absent version = %d rows, %v", len(none), err)
	}

	e, r := versionEstimate("trk_c", "final", "p1", 300)
	if err := store.Insert(e, r); err != nil {
		t.Fatal(err)
	}
	key.Stage = "final"
	if _, err := store.ListVersionEstimates(key); err == nil || !strings.Contains(err.Error(), "not stored") {
		t.Errorf("an estimate without its observation: error %v", err)
	}
}
