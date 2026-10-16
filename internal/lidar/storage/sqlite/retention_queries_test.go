package sqlite

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func sampledObservation(t *testing.T, id, source string, frame, cluster int64, points int, sample [][3]float32) l4bobserve.DetectionObservation {
	t.Helper()
	retained := make([]l4perception.WorldPoint, len(sample))
	for i, p := range sample {
		retained[i] = l4perception.WorldPoint{X: float64(p[0]), Y: float64(p[1]), Z: float64(p[2])}
	}
	observation, err := l4bobserve.New(l4bobserve.Record{SchemaVersion: 1, ObservationID: id, SourceID: source,
		CalibrationID: "calibration/v1/test", FrameUnixNanos: frame,
		Cluster: l4perception.WorldCluster{ClusterID: cluster, SensorID: "hesai-pandar40p", FrameID: "site/test",
			TSUnixNanos: frame, PointsCount: points, SamplePoints: sample, RetainedPoints: retained}})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

// The retention measure matches a sample to pack points by exact float32
// identity, so the read path must return the stored coordinates bit for bit,
// with the population beside them, in a reproducible order and only inside
// the window asked for.
func TestListClusterSamplesReturnsExactSamplesInWindowOrder(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewObservationStore(database)
	const source = "source/v1/retention"
	odd := [3]float32{0.1, -12.345678, 1.0000001} // not exactly representable in decimal
	for _, o := range []l4bobserve.DetectionObservation{
		sampledObservation(t, "observation/v1/c", source, 300, 1, 1, [][3]float32{{9, 9, 9}}),
		sampledObservation(t, "observation/v1/b", source, 200, 2, 5000, [][3]float32{odd, {1, 2, 3}}),
		sampledObservation(t, "observation/v1/a", source, 200, 1, 2, nil),
		sampledObservation(t, "observation/v1/z", "source/v1/other", 200, 1, 1, [][3]float32{{4, 4, 4}}),
		sampledObservation(t, "observation/v1/early", source, 100, 1, 1, [][3]float32{{5, 5, 5}}),
	} {
		if err := store.Insert(o); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.ListClusterSamples(source, 200, 300)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{"observation/v1/a", "observation/v1/b", "observation/v1/c"}
	if len(got) != len(wantIDs) {
		t.Fatalf("got %d clusters, want %d (other source and out-of-window frame excluded): %+v", len(got), len(wantIDs), got)
	}
	for i, id := range wantIDs {
		if got[i].ObservationID != id {
			t.Fatalf("cluster %d is %s, want %s", i, got[i].ObservationID, id)
		}
	}
	if len(got[0].Sample) != 0 || got[0].PointsCount != 2 {
		t.Fatalf("sample-less cluster = %+v, want an empty sample and its population", got[0])
	}
	b := got[1]
	if b.FrameUnixNanos != 200 || b.ClusterID != 2 || b.PointsCount != 5000 || len(b.Sample) != 2 || b.Sample[0] != odd {
		t.Fatalf("capped cluster = %+v, want frame 200, cluster 2, 5000 points and the sample bit for bit", b)
	}
}
