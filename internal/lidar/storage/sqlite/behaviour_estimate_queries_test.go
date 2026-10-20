package sqlite

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
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
		Reference:  l5tracks.ReferenceVisibleOBBCentre, Support: l5tracks.SupportObserved,
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
		if r.Stage != "final" || r.ParamHash != "p1" || r.SensorID != "sensor_a" || r.Covariance[1] != 0.5 || r.VX != 10 ||
			r.Reference != l5tracks.ReferenceVisibleOBBCentre || r.Support != l5tracks.SupportObserved {
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

// TestListVersionSolidBodies reads exactly one version of solid bodies, in
// track and frame order, with each row's observing sensor and its support
// detail, and refuses a row whose observation is missing.
func TestListVersionSolidBodies(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	for _, track := range []string{"trk_b", "trk_a"} {
		for _, frame := range []int64{200, 100} {
			obs := fmt.Sprintf("observation/%d/%s", frame, track)
			insertBareObservation(t, database, obs, "source/v1/frame", "sensor_a", frame)
			for _, hash := range []string{"p1", "p2"} {
				sb := testSolidBody(obs, frame)
				sb.TrackID, sb.ParamHash = track, hash
				sb.EstimateID = fmt.Sprintf("solid_body/%s/%s/%d", track, hash, frame)
				if err := store.InsertSolidBody(sb); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	key := EstimateVersionKey{SourceID: "source/v1/frame", EstimatorID: "cv_kf_v1",
		ObservationModelID: "near_edge_candidate_v1", ParamHash: "p1", Stage: "online"}
	got, err := store.ListVersionSolidBodies(key)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, r := range got {
		order = append(order, fmt.Sprintf("%s@%d", r.TrackID, r.FrameUnixNanos))
		want := testSolidBody(r.ObservationID, r.FrameUnixNanos)
		want.TrackID, want.EstimateID, want.ParamHash = r.TrackID, r.EstimateID, "p1"
		if r.SensorID != "sensor_a" || !reflect.DeepEqual(r.TrackSolidBody, want) {
			t.Errorf("row %s = %+v", r.EstimateID, r)
		}
	}
	if strings.Join(order, " ") != "trk_a@100 trk_a@200 trk_b@100 trk_b@200" {
		t.Errorf("order = %v", order)
	}

	key.Stage = "fixed_lag"
	if none, err := store.ListVersionSolidBodies(key); err != nil || len(none) != 0 {
		t.Errorf("an absent version = %d rows, %v", len(none), err)
	}

	orphan := testSolidBody("observation/missing", 300)
	orphan.ParamHash = "p1"
	if err := store.InsertSolidBody(orphan); err != nil {
		t.Fatal(err)
	}
	key.Stage = "online"
	if _, err := store.ListVersionSolidBodies(key); err == nil || !strings.Contains(err.Error(), "not stored") {
		t.Errorf("a solid body without its observation: error %v", err)
	}
}

// boxTrack and boxObservation are a confirmed track's lidar_tracks row and its
// observation at one frame, as the frame evidence store files them beside the
// estimate.
func boxTrack(id, label string, confidence float32) *TrackedObject {
	t := &TrackedObject{TrackID: id}
	t.SensorID, t.TrackState, t.StartUnixNanos = "hesai-pandar40p", TrackConfirmed, 1
	t.ObjectClass, t.ObjectConfidence = label, confidence
	return t
}

func boxObservation(track string, frame int64, length float32) *TrackObservation {
	return &TrackObservation{
		TrackID: track, TSUnixNanos: frame + 1, FrameUnixNanos: frame, FrameID: "site/hesai-pandar40p",
		MeasurementSource: "medoid_v0", X: 1, Y: 2,
		BoxX: 1.5, BoxY: 2.25, BoxHeadingRad: 0.125, BoxLength: length, BoxWidth: 1.75,
	}
}

// TestFrameEvidenceFilesTheBoxBesideItsEstimate: the frame evidence store
// writes each pair's track row and observation in the frame's transaction,
// and the box reader lists and reads them back with the estimate, the label
// and the observing sensor. A box filed against another frame or without its
// track row is refused, and the frame leaves nothing behind.
func TestFrameEvidenceFilesTheBoxBesideItsEstimate(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	store := NewFrameEvidenceStore(database)
	defer store.Close()
	for frame := int64(100); frame <= 300; frame += 100 {
		observation := testObservation(t, fmt.Sprintf("observation/v1/box/%d", frame), "source/v1/frame", frame, frame)
		pair := testFrameStateEstimate(observation.Snapshot().ObservationID, frame)
		pair.Track = boxTrack(pair.Estimate.TrackID, "car", 0.75)
		// The box model had not seeded at the first frame.
		length := float32(4.5)
		if frame == 100 {
			length = 0
		}
		pair.Observation = boxObservation(pair.Estimate.TrackID, frame, length)
		if err := store.InsertFrame([]l4bobserve.DetectionObservation{observation}, []FrameStateEstimate{pair}); err != nil {
			t.Fatal(err)
		}
	}
	reader := NewStateEstimateStore(database)
	versions, err := reader.ListBoxVersions()
	if err != nil {
		t.Fatal(err)
	}
	want := []EstimateVersion{{SourceID: "source/v1/frame", EstimatorID: "cv_kf_v1", ObservationModelID: "medoid_v0",
		ParamHash: "params/frame", Stage: "online", Estimates: 2, Tracks: 1}}
	if !reflect.DeepEqual(versions, want) {
		t.Fatalf("box versions %+v, want %+v", versions, want)
	}
	v := versions[0]
	rows, err := reader.ListVersionBoxes(EstimateVersionKey{SourceID: v.SourceID, EstimatorID: v.EstimatorID,
		ObservationModelID: v.ObservationModelID, ParamHash: v.ParamHash, Stage: v.Stage})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].HasBox || !rows[1].HasBox || !rows[2].HasBox {
		t.Fatalf("%d rows: %+v", len(rows), rows)
	}
	r := rows[2]
	if r.FrameUnixNanos != 300 || r.SensorID != "hesai-pandar40p" || r.BoxX != 1.5 || r.BoxY != 2.25 || r.BoxHeadingRad != 0.125 ||
		r.BoxLength != 4.5 || r.BoxWidth != 1.75 || r.ClassLabel != "car" || r.ClassConfidence == nil || *r.ClassConfidence != 0.75 ||
		r.Support != l5tracks.SupportObserved || r.Covariance[0] != 1 {
		t.Errorf("row %+v", r)
	}

	for name, mutate := range map[string]func(*FrameStateEstimate){
		"no track row":  func(p *FrameStateEstimate) { p.Track = nil },
		"another frame": func(p *FrameStateEstimate) { p.Observation.FrameUnixNanos++ },
		"another track": func(p *FrameStateEstimate) { p.Track.TrackID = "track/other" },
	} {
		t.Run(name, func(t *testing.T) {
			observation := testObservation(t, "observation/v1/box/bad", "source/v1/frame", 400, 400)
			pair := testFrameStateEstimate(observation.Snapshot().ObservationID, 400)
			pair.Track, pair.Observation = boxTrack(pair.Estimate.TrackID, "car", 0.75), boxObservation(pair.Estimate.TrackID, 400, 4.5)
			mutate(&pair)
			if err := store.InsertFrame([]l4bobserve.DetectionObservation{observation}, []FrameStateEstimate{pair}); err == nil {
				t.Fatal("want an error")
			}
			var n int
			if err := database.QueryRow(`SELECT COUNT(*) FROM lidar_track_estimates WHERE frame_unix_nanos = 400`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("%d estimates left behind (%v)", n, err)
			}
		})
	}
}

// TestListVersionBoxesRefusesWhatWasNotFiled: an estimate with no
// observation row at its frame (a database from a replay that did not file
// the box), a box only partly recorded, and a refined stage are refused; a
// track with no label reads as none.
func TestListVersionBoxesRefusesWhatWasNotFiled(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	key := EstimateVersionKey{SourceID: "source/test", EstimatorID: "cv_kf_v1", ObservationModelID: "obb_centre_v1",
		ParamHash: "p1", Stage: "online"}
	insertBareObservation(t, database, "observation/100/trk_a", "source/test", "sensor_a", 100)
	e, r := versionEstimate("trk_a", "online", "p1", 100)
	if err := store.Insert(e, r); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListVersionBoxes(key); err == nil || !strings.Contains(err.Error(), "did not record the box") {
		t.Fatalf("an estimate without its observation row: error %v", err)
	}
	if versions, err := store.ListBoxVersions(); err != nil || len(versions) != 0 {
		t.Fatalf("versions without boxes = %+v, %v", versions, err)
	}

	if err := InsertTrack(database, boxTrack("trk_a", "", 0), "site/hesai-pandar40p"); err != nil {
		t.Fatal(err)
	}
	if err := InsertTrackObservation(database, boxObservation("trk_a", 100, 4.5)); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListVersionBoxes(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].HasBox || rows[0].ClassLabel != "" || rows[0].ClassConfidence != nil {
		t.Fatalf("rows %+v", rows)
	}

	if _, err := database.Exec(`UPDATE lidar_track_observations SET box_width = NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListVersionBoxes(key); err == nil || !strings.Contains(err.Error(), "4 of the box's 5") {
		t.Fatalf("a partly recorded box: error %v", err)
	}
	key.Stage = "final"
	if _, err := store.ListVersionBoxes(key); err == nil || !strings.Contains(err.Error(), "online") {
		t.Fatalf("a final key: error %v", err)
	}
}

// TestAnEstimateWithoutAnObservationRowIsReadUnfiled: an estimate whose
// frame has no lidar_track_observations row (as when the table's key once let
// a frame-seam cluster replace the previous frame's row) is read back unfiled,
// without a box, rather than refused. The row is removed directly, so the test
// does not depend on how the table is keyed.
func TestAnEstimateWithoutAnObservationRowIsReadUnfiled(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	store := NewFrameEvidenceStore(database)
	defer store.Close()
	for _, frame := range []int64{100, 200, 300} {
		observation := testObservation(t, fmt.Sprintf("observation/v1/seam/%d", frame), "source/v1/frame", frame, frame)
		pair := testFrameStateEstimate(observation.Snapshot().ObservationID, frame)
		pair.Track = boxTrack(pair.Estimate.TrackID, "car", 0.75)
		pair.Observation = boxObservation(pair.Estimate.TrackID, frame, 4.5)
		if err := store.InsertFrame([]l4bobserve.DetectionObservation{observation}, []FrameStateEstimate{pair}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(`DELETE FROM lidar_track_observations WHERE frame_unix_nanos = 100`); err != nil {
		t.Fatal(err)
	}
	rows, err := NewStateEstimateStore(database).ListVersionBoxes(EstimateVersionKey{SourceID: "source/v1/frame",
		EstimatorID: "cv_kf_v1", ObservationModelID: "medoid_v0", ParamHash: "params/frame", Stage: "online"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Filed || rows[0].HasBox || !rows[1].Filed || !rows[1].HasBox || !rows[2].Filed {
		t.Fatalf("rows %+v", rows)
	}
	if rows[0].ClassLabel != "car" || rows[0].SensorID == "" {
		t.Errorf("the unfiled estimate lost its track's label or sensor: %+v", rows[0])
	}
}
