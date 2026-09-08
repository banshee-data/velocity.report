package l4bobserve

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func TestObservationOwnsEvidenceAndRoundTrips(t *testing.T) {
	ring, az := 2, float32(30)
	r := Record{ObservationID: "obs", SourceID: "capture-hash", FrameUnixNanos: 100,
		Cluster: l4perception.WorldCluster{SensorID: "s", FrameID: "site/test", TSUnixNanos: 120,
			PointsCount: 1, CentroidX: 3, SamplePoints: [][3]float32{{3, 2, 1}},
			RetainedPoints: []l4perception.WorldPoint{{X: 3, Y: 2, Z: 1, Timestamp: time.Unix(0, 120).UTC(), Intensity: 7}},
			OBB:            &l4perception.OrientedBoundingBox{Length: 4}, SensorRingHint: &ring, SensorAzDegHint: &az}}
	obs, err := New(r)
	if err != nil {
		t.Fatal(err)
	}
	want := obs.Snapshot()
	mutate := func(v Record) {
		v.Cluster.SamplePoints[0][0] = 99
		v.Cluster.RetainedPoints[0].X = 99
		v.Cluster.OBB.Length = 99
		*v.Cluster.SensorRingHint = 99
		*v.Cluster.SensorAzDegHint = 99
	}
	mutate(r)
	mutate(obs.Snapshot())
	if !reflect.DeepEqual(obs.Snapshot(), want) {
		t.Fatal("evidence aliases mutable state")
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Record
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatal("round-trip changed acquisition time or geometry")
	}
	if decoded.Cluster.TSUnixNanos == decoded.FrameUnixNanos {
		t.Fatal("capture time collapsed into frame time")
	}
}

func TestObservationIdentityBoundsAndAbsentOptionalGeometry(t *testing.T) {
	valid := Record{ObservationID: "o", SourceID: "s", Cluster: l4perception.WorldCluster{SensorID: "lidar", FrameID: "site/test"}}
	obs, err := New(valid)
	if err != nil || !reflect.DeepEqual(obs.Snapshot(), valid) {
		t.Fatalf("empty evidence: %v", err)
	}
	for _, change := range []func(*Record){
		func(r *Record) { r.ObservationID = "" },
		func(r *Record) { r.SourceID = "" },
		func(r *Record) { r.Cluster.SensorID = "" },
		func(r *Record) { r.Cluster.FrameID = "" },
		func(r *Record) { r.Cluster.PointsCount = -1 },
		func(r *Record) { r.Cluster.RetainedPoints = make([]l4perception.WorldPoint, 1) },
		func(r *Record) {
			r.Cluster.PointsCount = 1025
			r.Cluster.RetainedPoints = make([]l4perception.WorldPoint, 1025)
		},
		func(r *Record) { r.Cluster.SamplePoints = make([][3]float32, 1025) },
	} {
		r := valid
		change(&r)
		if _, err := New(r); err == nil {
			t.Fatalf("accepted invalid evidence: %+v", r)
		}
	}
}
