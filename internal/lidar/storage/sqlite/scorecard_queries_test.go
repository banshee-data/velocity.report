package sqlite

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func scorecardObservation(t *testing.T, id, source string, frame int64, cx, cy float32, obb *l4perception.OrientedBoundingBox, points int) l4bobserve.DetectionObservation {
	t.Helper()
	record := l4bobserve.Record{SchemaVersion: 1, ObservationID: id, SourceID: source,
		CalibrationID: "calibration/v1/test", FrameUnixNanos: frame,
		Cluster: l4perception.WorldCluster{ClusterID: 1, SensorID: "hesai-pandar40p", FrameID: "site/test",
			TSUnixNanos: frame + 3, CentroidX: cx, CentroidY: cy, PointsCount: points, OBB: obb,
			RetainedPoints: []l4perception.WorldPoint{{X: 1, Y: 2, Z: 3, Timestamp: time.Unix(0, frame+3).UTC(), SensorID: "hesai-pandar40p"}}},
	}
	observation, err := l4bobserve.New(record)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

// The scorecard must be byte-identical across independent replays, so the
// read paths it uses have to return rows in a total, reproducible order
// whatever order they were written in.
func TestScorecardQueriesReturnADeterministicOrder(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	observations := NewObservationStore(database)
	states := NewStateEstimateStore(database)
	const source = "source/v1/scorecard"

	// Written out of order on purpose.
	withOBB := &l4perception.OrientedBoundingBox{CenterX: 7, CenterY: 8, Length: 4, Width: 2, Height: 1.5}
	for _, o := range []l4bobserve.DetectionObservation{
		scorecardObservation(t, "observation/v1/c", source, 200, 1, 2, nil, 12),
		scorecardObservation(t, "observation/v1/a", source, 100, 3, 4, withOBB, 40),
		scorecardObservation(t, "observation/v1/b", source, 100, 5, 6, nil, 9),
		scorecardObservation(t, "observation/v1/z", "source/v1/other", 100, 9, 9, nil, 1),
	} {
		if err := observations.Insert(o); err != nil {
			t.Fatal(err)
		}
	}

	ids, err := observations.ListSourceIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "source/v1/other" || ids[1] != source {
		t.Fatalf("source ids = %v, want both sources sorted", ids)
	}

	clusters, err := observations.ListClusterSummariesBySource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 3 {
		t.Fatalf("got %d cluster summaries, want 3 (the other source must be excluded)", len(clusters))
	}
	wantOrder := []string{"observation/v1/a", "observation/v1/b", "observation/v1/c"}
	for i, want := range wantOrder {
		if clusters[i].ObservationID != want {
			t.Fatalf("cluster %d is %s, want %s (frame, then observation id)", i, clusters[i].ObservationID, want)
		}
	}
	// obb_centre_v1: the OBB centre where there is one, the centroid otherwise.
	if clusters[0].X != 7 || clusters[0].Y != 8 || clusters[0].PointsCount != 40 {
		t.Errorf("cluster with an OBB = %+v, want its OBB centre (7, 8) and 40 points", clusters[0])
	}
	if clusters[1].X != 5 || clusters[1].Y != 6 || clusters[1].PointsCount != 9 {
		t.Errorf("cluster without an OBB = %+v, want its centroid (5, 6) and 9 points", clusters[1])
	}

	estimate := func(id, observationID string, sequence, frame int64) (TrackEstimate, TrackResidual) {
		e := TrackEstimate{EstimateID: id, TrackID: "trk_" + id, ObservationID: observationID, SourceID: source,
			CalibrationID: "calibration/v1/test", FrameUnixNanos: frame, MeasurementUnixNanos: frame + 1,
			EstimatorID: "cv_kf_v1", ObservationModelID: "obb_centre_v1", ParamHash: "params/test", Stage: "online",
			MeasurementSource: "obb_centre_v1", CreationSequence: sequence, X: 1, Y: 2, VX: 3, VY: 4}
		r := TrackResidual{EstimateID: id, ObservationID: observationID, MeasurementX: 1.5, MeasurementY: 2.5,
			InnovationX: 0.25, InnovationY: -0.5, NIS: 1.75, Disposition: "accepted", Reason: "association_accepted"}
		return e, r
	}
	// Track 2 before track 1, and track 1's frames reversed.
	for _, in := range []struct {
		id, obs         string
		sequence, frame int64
	}{
		{"estimate/2-100", "observation/v1/b", 2, 100},
		{"estimate/1-200", "observation/v1/c", 1, 200},
		{"estimate/1-100", "observation/v1/a", 1, 100},
	} {
		e, r := estimate(in.id, in.obs, in.sequence, in.frame)
		if err := states.Insert(e, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := states.ListFrameStateEstimatesBySource(source)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{"estimate/1-100", "estimate/1-200", "estimate/2-100"}
	if len(got) != len(wantIDs) {
		t.Fatalf("got %d frame states, want %d", len(got), len(wantIDs))
	}
	for i, want := range wantIDs {
		if got[i].Estimate.EstimateID != want {
			t.Fatalf("frame state %d is %s, want %s (creation sequence, then frame)", i, got[i].Estimate.EstimateID, want)
		}
	}
	first := got[0]
	if first.Residual.InnovationX != 0.25 || first.Residual.InnovationY != -0.5 || first.Residual.NIS != 1.75 ||
		first.Residual.MeasurementX != 1.5 || first.Estimate.VX != 3 || first.Estimate.CreationSequence != 1 {
		t.Errorf("frame state lost fields: %+v", first)
	}
	if first.Residual.EstimateID != first.Estimate.EstimateID || first.Residual.ObservationID != "observation/v1/a" {
		t.Errorf("residual identity does not match its estimate: %+v", first.Residual)
	}
}
