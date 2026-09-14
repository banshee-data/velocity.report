package sqlite

import (
	"reflect"
	"testing"
)

func TestBuildEvidenceOracleCanonicalSemantics(t *testing.T) {
	db, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	for _, row := range []struct {
		id, source string
		frame      int64
	}{{"o-1", "source-a", 100}, {"o-2", "source-b", 200}} {
		if _, err := db.Exec(`INSERT INTO lidar_observations (observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos, cluster_unix_nanos, cluster_id, record_json, inserted_at_ns) VALUES (?, 1, ?, 'cal', 'sensor', 'frame', ?, ?, 1, '{"cluster":1}', 1)`, row.id, row.source, row.frame, row.frame); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO lidar_track_estimates (estimate_id, track_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos, estimator_id, observation_model_id, param_hash, stage, measurement_source, x, y, vx, vy, covariance_json, inserted_at_ns) VALUES ('e-1', 'track', 'o-1', 'source-a', 'cal', 100, 100, 'estimator', 'model', 'params', 'online', 'obb', 1, 2, 3, 4, '[]', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lidar_track_residuals (estimate_id, observation_id, predicted_x, predicted_y, measurement_x, measurement_y, innovation_x, innovation_y, nis, geometry_cov_xx, geometry_cov_xy, geometry_cov_yy, disposition, reason, inserted_at_ns) VALUES ('e-1', 'o-1', 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 'accepted', 'ok', 1)`); err != nil {
		t.Fatal(err)
	}
	expected := map[string]struct{}{"source-a": {}, "source-b": {}}
	first, err := BuildEvidenceOracle(db, expected)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Tables) != 3 || first.Tables[0].RowCount != 2 || first.Tables[1].RowCount != 1 || first.Tables[2].RowCount != 1 || len(first.Frames) != 2 {
		t.Fatalf("unexpected oracle: %#v", first)
	}
	if _, err := db.Exec(`UPDATE lidar_observations SET inserted_at_ns = 999`); err != nil {
		t.Fatal(err)
	}
	second, err := BuildEvidenceOracle(db, expected)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("inserted_at_ns changed the semantic oracle")
	}
	if _, err := db.Exec(`UPDATE lidar_observations SET record_json = '{"cluster":2}' WHERE observation_id = 'o-1'`); err != nil {
		t.Fatal(err)
	}
	changed, err := BuildEvidenceOracle(db, expected)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(first, changed) {
		t.Fatal("changed observation payload did not change the semantic oracle")
	}
}

func TestBuildEvidenceOracleRejectsBrokenEstimateLink(t *testing.T) {
	db, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	if _, err := db.Exec(`INSERT INTO lidar_observations (observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos, cluster_unix_nanos, cluster_id, record_json, inserted_at_ns) VALUES ('o-1', 1, 'source-a', 'cal', 'sensor', 'frame', 100, 100, 1, '{}', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lidar_track_estimates (estimate_id, track_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos, estimator_id, observation_model_id, param_hash, stage, measurement_source, x, y, vx, vy, covariance_json, inserted_at_ns) VALUES ('e-1', 'track', 'missing', 'source-a', 'cal', 100, 100, 'estimator', 'model', 'params', 'online', 'obb', 1, 2, 3, 4, '[]', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildEvidenceOracle(db, map[string]struct{}{"source-a": {}}); err == nil {
		t.Fatal("broken estimate link was accepted")
	}
}
