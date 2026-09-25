package sqlite

import (
	"fmt"
	"testing"
)

func perFrameEstimate(t *testing.T, store *StateEstimateStore, id, track string, seq, frame int64, param, stage string, x float32) {
	t.Helper()
	e := TrackEstimate{
		EstimateID: id, TrackID: track, ObservationID: "observation/" + id, SourceID: "source/v1/a",
		CalibrationID: "calibration/v1/test", FrameUnixNanos: frame, MeasurementUnixNanos: frame + 5,
		EstimatorID: "cv_kf_v1", ObservationModelID: "medoid_v1", ParamHash: param, Stage: stage,
		MeasurementSource: "medoid_v1", CreationSequence: seq, X: x, Y: -x,
	}
	r := TrackResidual{EstimateID: id, ObservationID: e.ObservationID, Disposition: "accepted", Reason: "association_accepted"}
	if err := store.Insert(e, r); err != nil {
		t.Fatal(err)
	}
}

func TestPerFrameEstimateQueriesSeparateVersionsInAStableOrder(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)

	// Written out of order, across two parameter hashes and two stages.
	perFrameEstimate(t, store, "e4", "track-b", 2, 200, "params/a", "final", 4)
	perFrameEstimate(t, store, "e1", "track-a", 1, 100, "params/a", "final", 1)
	perFrameEstimate(t, store, "e3", "track-b", 2, 100, "params/a", "final", 3)
	perFrameEstimate(t, store, "e2", "track-a", 1, 200, "params/a", "final", 2)
	perFrameEstimate(t, store, "o1", "track-a", 1, 100, "params/a", "online", 9)
	perFrameEstimate(t, store, "p1", "track-z", 1, 100, "params/b", "final", 7)

	versions, err := store.ListEstimateVersions()
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatalf("versions = %+v, want three", versions)
	}
	want := []struct {
		param, stage     string
		estimates, track int
	}{{"params/a", "final", 4, 2}, {"params/a", "online", 1, 1}, {"params/b", "final", 1, 1}}
	for i, w := range want {
		v := versions[i]
		if v.ParamHash != w.param || v.Stage != w.stage || v.Estimates != w.estimates || v.Tracks != w.track {
			t.Fatalf("version %d = %+v, want %+v", i, v, w)
		}
	}

	positions, err := store.ListEstimatePositions(versions[0])
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for _, p := range positions {
		got += fmt.Sprintf("%s:%d@%d=%g ", p.TrackID, p.CreationSequence, p.FrameUnixNanos, p.X)
	}
	if wantOrder := "track-a:1@100=1 track-a:1@200=2 track-b:2@100=3 track-b:2@200=4 "; got != wantOrder {
		t.Fatalf("positions %q, want %q (sequence, then frame; other versions excluded)", got, wantOrder)
	}
}

func TestPerFrameRunTrackPositions(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	runs := NewAnalysisRunStore(database)
	confirmed := func(id string) RunTrack {
		return RunTrack{TrackID: id, TrackMeasurement: TrackMeasurement{SensorID: "sensor-cmp", TrackState: TrackConfirmed}}
	}
	insertTestRunWithTracks(t, runs, "run-1", []RunTrack{confirmed("t-2"), confirmed("t-1")})
	insertTestRunWithTracks(t, runs, "run-2", []RunTrack{confirmed("t-9")})

	for _, id := range []string{"t-1", "t-2", "t-9"} {
		track := &TrackedObject{TrackID: id, TrackMeasurement: TrackMeasurement{SensorID: "sensor-cmp", TrackState: TrackConfirmed}}
		if err := InsertTrack(database, track, "site/main"); err != nil {
			t.Fatal(err)
		}
	}
	for _, o := range []TrackObservation{
		{TrackID: "t-2", TSUnixNanos: 205, FrameUnixNanos: 200, FrameID: "f", X: 4, Y: 4},
		{TrackID: "t-1", TSUnixNanos: 105, FrameUnixNanos: 100, FrameID: "f", X: 1, Y: 1},
		{TrackID: "t-2", TSUnixNanos: 105, FrameUnixNanos: 100, FrameID: "f", X: 3, Y: 3},
		{TrackID: "t-9", TSUnixNanos: 105, FrameUnixNanos: 100, FrameID: "f", X: 9, Y: 9},
	} {
		obs := o
		if err := InsertTrackObservation(database, &obs); err != nil {
			t.Fatal(err)
		}
	}
	// A row written before frame_unix_nanos existed falls back to its
	// measurement time; a row without a position is skipped.
	if _, err := database.Exec(`INSERT INTO lidar_track_observations (track_id, ts_unix_nanos, frame_id, x, y) VALUES ('t-1', 300, 'f', 2, 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO lidar_track_observations (track_id, ts_unix_nanos, frame_id, x, y) VALUES ('t-1', 400, 'f', NULL, 2)`); err != nil {
		t.Fatal(err)
	}

	n, err := runs.CountRunTracks("run-1")
	if err != nil || n != 2 {
		t.Fatalf("CountRunTracks = %d, %v; want 2", n, err)
	}
	if n, err := runs.CountRunTracks("missing"); err != nil || n != 0 {
		t.Fatalf("CountRunTracks(missing) = %d, %v; want 0", n, err)
	}

	positions, err := runs.ListRunTrackPositions("run-1")
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for _, p := range positions {
		got += fmt.Sprintf("%s@%d=%g ", p.TrackID, p.FrameUnixNanos, p.X)
	}
	if want := "t-1@100=1 t-1@300=2 t-2@100=3 t-2@200=4 "; got != want {
		t.Fatalf("positions %q, want %q", got, want)
	}
}
