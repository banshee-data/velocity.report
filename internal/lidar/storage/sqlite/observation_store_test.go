package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
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

func TestFrameEvidenceStoreCommitsObservationsAndDerivedStateTogether(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	first := testObservation(t, "observation/v1/frame-first", "source/v1/frame", 100, 1)
	second := testObservation(t, "observation/v1/frame-second", "source/v1/frame", 100, 2)
	pair := testFrameStateEstimate(first.Snapshot().ObservationID, 100)
	if err := NewFrameEvidenceStore(database).InsertFrame([]l4bobserve.DetectionObservation{first, second}, []FrameStateEstimate{pair}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"lidar_observations", "lidar_track_estimates", "lidar_track_residuals"} {
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		want := 1
		if table == "lidar_observations" {
			want = 2
		}
		if count != want {
			t.Fatalf("%s rows = %d, want %d", table, count, want)
		}
	}
	stored, err := NewObservationStore(database).Get(first.Snapshot().ObservationID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.Snapshot(), first.Snapshot()) {
		t.Fatalf("batched observation changed:\n got %#v\nwant %#v", stored.Snapshot(), first.Snapshot())
	}
}

func TestFrameEvidenceStoreRollsBackWholeFrameOnDuplicateObservation(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	store := NewObservationStore(database)
	existing := testObservation(t, "observation/v1/existing", "source/v1/frame", 100, 1)
	if err := store.Insert(existing); err != nil {
		t.Fatal(err)
	}
	newObservation := testObservation(t, "observation/v1/rolled-back", "source/v1/frame", 100, 2)
	err := NewFrameEvidenceStore(database).InsertFrame(
		[]l4bobserve.DetectionObservation{newObservation, existing},
		[]FrameStateEstimate{testFrameStateEstimate(newObservation.Snapshot().ObservationID, 100)},
	)
	if !errors.Is(err, ErrObservationExists) {
		t.Fatalf("duplicate frame insert error = %v, want ErrObservationExists", err)
	}
	if _, err := store.Get(newObservation.Snapshot().ObservationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("partially committed observation error = %v, want ErrNotFound", err)
	}
	for _, table := range []string{"lidar_track_estimates", "lidar_track_residuals"} {
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows after rollback = %d, want 0", table, count)
		}
	}
}

func TestFrameEvidenceStoreRollsBackObservationsWhenDerivedStateIsInvalid(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	observation := testObservation(t, "observation/v1/invalid-derived", "source/v1/frame", 100, 1)
	pair := testFrameStateEstimate(observation.Snapshot().ObservationID, 100)
	pair.Residual.ObservationID = "observation/v1/not-the-estimate-input"
	err := NewFrameEvidenceStore(database).InsertFrame([]l4bobserve.DetectionObservation{observation}, []FrameStateEstimate{pair})
	if err == nil {
		t.Fatal("accepted an invalid derived record")
	}
	if _, err := NewObservationStore(database).Get(observation.Snapshot().ObservationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("observation survived invalid derived state: %v", err)
	}
}

func TestFrameEvidenceStoreReportsBeginAndPrepareFailures(t *testing.T) {
	beginErr := errors.New("database unavailable")
	observation := testObservation(t, "observation/v1/prepare", "source/v1/frame", 100, 1)
	if err := NewFrameEvidenceStore(frameEvidenceBeginErrorDB{err: beginErr}).InsertFrame([]l4bobserve.DetectionObservation{observation}, nil); !errors.Is(err, beginErr) {
		t.Fatalf("begin error = %v, want %v", err, beginErr)
	}
	for _, table := range []string{"lidar_observations", "lidar_track_estimates", "lidar_track_residuals"} {
		database, cleanup := setupTrackingPipelineTestDB(t)
		if _, err := database.Exec("DROP TABLE " + table); err != nil {
			cleanup()
			t.Fatalf("drop %s: %v", table, err)
		}
		err := NewFrameEvidenceStore(database).InsertFrame([]l4bobserve.DetectionObservation{observation}, nil)
		cleanup()
		if err == nil || !strings.Contains(err.Error(), "prepare") {
			t.Fatalf("missing %s prepare error = %v", table, err)
		}
	}
}

func TestFrameEvidenceStoreSkipsEmptyFrames(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	if err := NewFrameEvidenceStore(database).InsertFrame(nil, nil); err != nil {
		t.Fatalf("empty frame = %v, want no-op", err)
	}
	for _, table := range []string{"lidar_observations", "lidar_track_estimates", "lidar_track_residuals"} {
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows after empty frame = %d, want 0", table, count)
		}
	}
}

func TestFrameEvidenceStoreReusesPreparedStatementsAcrossFrames(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	countingDB := &countingPrepareDB{DB: database}
	store := NewFrameEvidenceStore(countingDB)
	for frame := int64(100); frame <= 200; frame += 100 {
		observation := testObservation(t, fmt.Sprintf("observation/v1/reuse/%d", frame), "source/v1/frame", frame, frame)
		if err := store.InsertFrame([]l4bobserve.DetectionObservation{observation}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if store.observationStmt == nil || store.estimateStmt == nil || store.residualStmt == nil {
		t.Fatal("frame evidence statements were not retained for reuse")
	}
	if countingDB.prepares != 3 {
		t.Fatalf("prepared statements = %d, want exactly 3 for every frame", countingDB.prepares)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close prepared statements: %v", err)
	}
}

func TestFrameEvidenceStoreRollsBackOnNonUniqueWriteAndCommitFailure(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	if _, err := database.Exec(`CREATE TRIGGER reject_observation_write
		BEFORE INSERT ON lidar_observations
		BEGIN SELECT RAISE(FAIL, 'write blocked'); END`); err != nil {
		t.Fatal(err)
	}
	observation := testObservation(t, "observation/v1/write-blocked", "source/v1/frame", 100, 1)
	err := NewFrameEvidenceStore(database).InsertFrame([]l4bobserve.DetectionObservation{observation}, nil)
	if err == nil || !strings.Contains(err.Error(), "insert observation") {
		t.Fatalf("non-unique observation write error = %v", err)
	}

	database2, cleanup2 := setupTrackingPipelineTestDB(t)
	defer cleanup2()
	commitErr := errors.New("commit failed")
	store := NewFrameEvidenceStore(database2)
	store.commit = func(*sql.Tx) error { return commitErr }
	err = store.InsertFrame([]l4bobserve.DetectionObservation{observation}, nil)
	if !errors.Is(err, commitErr) {
		t.Fatalf("commit error = %v, want %v", err, commitErr)
	}
	if _, err := NewObservationStore(database2).Get(observation.Snapshot().ObservationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("observation survived failed commit: %v", err)
	}
}

func TestFrameEvidenceStoreRejectsNonCanonicalObservationPayload(t *testing.T) {
	database, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	observation := testObservation(t, "observation/v1/non-canonical", "source/v1/frame", 100, 1)
	record := observation.Snapshot()
	record.Cluster.RetainedPoints[0].X = math.NaN()
	observation, err := l4bobserve.New(record)
	if err != nil {
		t.Fatal(err)
	}
	err = NewFrameEvidenceStore(database).InsertFrame([]l4bobserve.DetectionObservation{observation}, nil)
	if err == nil || !strings.Contains(err.Error(), "marshal observation") {
		t.Fatalf("non-canonical payload error = %v", err)
	}
}

func TestInsertStateEstimateStatementsRejectsEachStatementFailure(t *testing.T) {
	pair := testFrameStateEstimate("observation/v1/statement", 100)
	estimateErr := errors.New("estimate statement failed")
	if err := insertStateEstimateStatements(frameStatement{err: estimateErr}, frameStatement{}, pair.Estimate, pair.Residual, 1); !errors.Is(err, estimateErr) {
		t.Fatalf("estimate statement error = %v, want %v", err, estimateErr)
	}
	residualErr := errors.New("residual statement failed")
	if err := insertStateEstimateStatements(frameStatement{}, frameStatement{err: residualErr}, pair.Estimate, pair.Residual, 1); !errors.Is(err, residualErr) {
		t.Fatalf("residual statement error = %v, want %v", err, residualErr)
	}
	pair.Estimate.Covariance[0] = float32(math.NaN())
	if err := insertStateEstimateStatements(frameStatement{}, frameStatement{}, pair.Estimate, pair.Residual, 1); err == nil || !strings.Contains(err.Error(), "marshal estimate covariance") {
		t.Fatalf("non-canonical covariance error = %v", err)
	}
}

type frameEvidenceBeginErrorDB struct{ err error }

func (d frameEvidenceBeginErrorDB) Exec(string, ...any) (sql.Result, error) { return nil, d.err }
func (d frameEvidenceBeginErrorDB) Query(string, ...any) (*sql.Rows, error) { return nil, d.err }
func (d frameEvidenceBeginErrorDB) QueryRow(string, ...any) *sql.Row        { return nil }
func (d frameEvidenceBeginErrorDB) Begin() (*sql.Tx, error)                 { return nil, d.err }

type countingPrepareDB struct {
	*sql.DB
	prepares int
}

func (d *countingPrepareDB) Prepare(query string) (*sql.Stmt, error) {
	d.prepares++
	return d.DB.Prepare(query)
}

type frameStatement struct{ err error }

func (s frameStatement) Exec(...any) (sql.Result, error) { return nil, s.err }

func testFrameStateEstimate(observationID string, frameUnixNanos int64) FrameStateEstimate {
	estimate := TrackEstimate{
		EstimateID: "estimate/v1/frame/" + observationID, TrackID: "track/frame", ObservationID: observationID,
		SourceID: "source/v1/frame", CalibrationID: "calibration/v1/test", FrameUnixNanos: frameUnixNanos,
		MeasurementUnixNanos: frameUnixNanos + 1, EstimatorID: "cv_kf_v1", ObservationModelID: "medoid_v0",
		ParamHash: "params/frame", Stage: "online", MeasurementSource: "medoid_v0", Covariance: [16]float32{1},
	}
	return FrameStateEstimate{Estimate: estimate, Residual: TrackResidual{
		EstimateID: estimate.EstimateID, ObservationID: observationID, Disposition: "accepted", Reason: "association_accepted",
	}}
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
