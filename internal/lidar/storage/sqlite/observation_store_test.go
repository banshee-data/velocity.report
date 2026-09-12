package sqlite

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func TestObservationStorePreservesImmutableReplayEvidence(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	store := NewObservationStore(database)
	first := testObservation(t, "observation/v1/first", "source/v1/columbus", 100, 1)
	second := testObservation(t, "observation/v1/second", "source/v1/columbus", 200, 2)
	if err := store.Insert(second); err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(first); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("observation/v1/first")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Snapshot(), first.Snapshot()) {
		t.Fatalf("stored evidence changed:\n got %#v\nwant %#v", got.Snapshot(), first.Snapshot())
	}
	// A duplicate is a proposed revision, even if its payload happens to match.
	if err := store.Insert(first); !errors.Is(err, ErrObservationExists) {
		t.Fatalf("duplicate insert error = %v, want ErrObservationExists", err)
	}
	ordered, err := store.ListBySource("source/v1/columbus")
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 2 || ordered[0].Snapshot().ObservationID != "observation/v1/first" || ordered[1].Snapshot().ObservationID != "observation/v1/second" {
		t.Fatalf("replay order = %#v", ordered)
	}
	if _, err := store.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing read error = %v, want ErrNotFound", err)
	}
}

func TestObservationStoreRejectsInvalidStoredPayload(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	if _, err := database.Exec(`INSERT INTO lidar_observations
		(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos, cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
		VALUES ('bad', 1, 'source', 'calibration', 'sensor', 'frame', 0, 0, 0, '{}', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewObservationStore(database).Get("bad"); err == nil {
		t.Fatal("accepted malformed persisted evidence")
	}
}

func testObservation(t *testing.T, id, source string, frame int64, clusterID int64) l4bobserve.DetectionObservation {
	t.Helper()
	record := l4bobserve.Record{SchemaVersion: 1, ObservationID: id, SourceID: source, CalibrationID: "calibration/v1/test", FrameUnixNanos: frame,
		Cluster: l4perception.WorldCluster{ClusterID: clusterID, SensorID: "hesai-pandar40p", FrameID: "site/test", TSUnixNanos: frame + 3, PointsCount: 3,
			RetainedPoints: []l4perception.WorldPoint{{X: 1, Y: 2, Z: 3, Intensity: 4, Timestamp: time.Unix(0, frame+3).UTC(), SensorID: "hesai-pandar40p"}, {X: 2, Y: 2, Z: 3}, {X: 3, Y: 2, Z: 3}}},
		Primitives: l4bobserve.Primitives{Planes: []l4bobserve.Plane{{Normal: [3]float64{0, 0, 1}, Offset: -3, Support: 3}}, Edges: []l4bobserve.Edge{{Start: [3]float64{1, 2, 3}, End: [3]float64{3, 2, 3}, Support: 3}}}}
	observation, err := l4bobserve.New(record)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}
