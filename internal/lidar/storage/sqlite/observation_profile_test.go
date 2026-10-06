package sqlite

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// reducedRecordGolden is the stored record_json of the fixture below. The
// evidence oracle hashes these bytes, so the foreground-complete work must not
// change them, including when a retained point carries in-memory lineage.
const reducedRecordGolden = `{"schema_version":1,"observation_id":"observation/v1/golden","source_id":"source/v1/golden","calibration_id":"calibration/v1/test","frame_unix_nanos":100,"raw_cluster":{"ClusterID":7,"SensorID":"hesai-pandar40p","FrameID":"site/test","TSUnixNanos":103,"CentroidX":1.5,"CentroidY":2,"CentroidZ":3,"BoundingBoxLength":2,"BoundingBoxWidth":0.25,"BoundingBoxHeight":0.5,"PointsCount":3,"HeightP95":3,"IntensityMean":4,"GroundClipped":false,"SensorRingHint":null,"SensorAzDegHint":null,"SamplePoints":[[1,2,3]],"OBB":null,"RetainedPoints":[{"X":1,"Y":2,"Z":3,"Intensity":4,"Timestamp":"1970-01-01T00:00:00.000000103Z","SensorID":"hesai-pandar40p"}]},"primitives":{}}`

func goldenReducedObservation(t *testing.T) l4bobserve.DetectionObservation {
	t.Helper()
	point := l4perception.WorldPoint{X: 1, Y: 2, Z: 3, Intensity: 4, Timestamp: time.Unix(0, 103).UTC(), SensorID: "hesai-pandar40p"}
	point.SetSourceOrdinal(41) // as the foreground-complete tap stamps it
	observation, err := l4bobserve.New(l4bobserve.Record{SchemaVersion: 1, ObservationID: "observation/v1/golden",
		SourceID: "source/v1/golden", CalibrationID: "calibration/v1/test", FrameUnixNanos: 100,
		Cluster: l4perception.WorldCluster{ClusterID: 7, SensorID: "hesai-pandar40p", FrameID: "site/test", TSUnixNanos: 103,
			CentroidX: 1.5, CentroidY: 2, CentroidZ: 3, BoundingBoxLength: 2, BoundingBoxWidth: 0.25, BoundingBoxHeight: 0.5,
			PointsCount: 3, HeightP95: 3, IntensityMean: 4, SamplePoints: [][3]float32{{1, 2, 3}},
			RetainedPoints: []l4perception.WorldPoint{point}}})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func TestReducedRecordsRoundTripUnchangedAndAreLabelledReduced(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	store := NewObservationStore(database)
	observation := goldenReducedObservation(t)
	if err := store.Insert(observation); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := database.QueryRow(`SELECT record_json FROM lidar_observations WHERE observation_id = ?`, "observation/v1/golden").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != reducedRecordGolden {
		t.Fatalf("stored reduced record changed:\n got %s\nwant %s", stored, reducedRecordGolden)
	}
	decoded, err := store.Get("observation/v1/golden")
	if err != nil {
		t.Fatal(err)
	}
	// Lineage is in-memory only; everything the reduced record carries survives.
	want := observation.Snapshot()
	want.Cluster.RetainedPoints[0].SetSourceOrdinal(-1)
	if !reflect.DeepEqual(decoded.Snapshot(), want) {
		t.Fatalf("round trip changed the reduced record:\n got %#v\nwant %#v", decoded.Snapshot(), want)
	}
	if decoded.Profile().Name != l4bobserve.ProfileReducedClusterSample {
		t.Fatalf("decoded record profile = %q", decoded.Profile().Name)
	}

	profile, err := store.SourceProfile("source/v1/golden")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != l4bobserve.ProfileReducedClusterSample {
		t.Fatalf("source profile = %q", profile.Name)
	}
	err = profile.Satisfies(l4bobserve.ForegroundComplete())
	var missing *l4bobserve.MissingCapabilitiesError
	if !errors.Is(err, l4bobserve.ErrMissingCapability) || !errors.As(err, &missing) || len(missing.Missing) == 0 {
		t.Fatalf("a capped-sample source satisfied a full-evidence request: %v", err)
	}
	for _, capability := range []l4bobserve.Capability{l4bobserve.CapabilityFrameRecords, l4bobserve.CapabilityCompleteForeground, l4bobserve.CapabilityClusterMembership} {
		if !containsCapability(missing.Missing, capability) {
			t.Fatalf("missing list %v omits %s", missing.Missing, capability)
		}
	}
}

// SourceProfile's reduced label rests on the table admitting schema 1 only.
// If a later migration relaxes that, this test fails and the label must be
// derived per version instead of assumed.
func TestSourceProfileRestsOnTheSchemaOneConstraint(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	store := NewObservationStore(database)
	if _, err := store.SourceProfile("source/v1/absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent source error = %v, want ErrNotFound", err)
	}
	if _, err := database.Exec(`INSERT INTO lidar_observations
		(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos, cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
		VALUES ('future', 2, 'source/v1/future', 'calibration', 'sensor', 'frame', 0, 0, 0, '{}', 1)`); err == nil {
		t.Fatal("lidar_observations admitted a schema-2 record; SourceProfile can no longer assume the reduced profile")
	}
}

func containsCapability(list []l4bobserve.Capability, want l4bobserve.Capability) bool {
	for _, capability := range list {
		if capability == want {
			return true
		}
	}
	return false
}
