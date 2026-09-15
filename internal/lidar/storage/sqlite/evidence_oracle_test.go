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
	if _, err := db.Exec(`INSERT INTO lidar_track_estimates (estimate_id, track_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos, estimator_id, observation_model_id, param_hash, stage, measurement_source, creation_sequence, x, y, vx, vy, covariance_json, inserted_at_ns) VALUES ('e-1', 'track', 'o-1', 'source-a', 'cal', 100, 100, 'estimator', 'model', 'params', 'online', 'obb', 1, 1, 2, 3, 4, '[]', 1)`); err != nil {
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
	if _, err := db.Exec(`INSERT INTO lidar_track_estimates (estimate_id, track_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos, estimator_id, observation_model_id, param_hash, stage, measurement_source, creation_sequence, x, y, vx, vy, covariance_json, inserted_at_ns) VALUES ('e-1', 'track', 'missing', 'source-a', 'cal', 100, 100, 'estimator', 'model', 'params', 'online', 'obb', 1, 1, 2, 3, 4, '[]', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildEvidenceOracle(db, map[string]struct{}{"source-a": {}}); err == nil {
		t.Fatal("broken estimate link was accepted")
	}
}

// TestBuildEvidenceOracleIgnoresRandomTrackIdentity is a direct regression
// test for a real false-positive found comparing two independent replays of
// the same phase01 corpus: l5tracks assigns track_id a random UUID (by
// design, to stay collision-free across tracker resets and restarts), so
// track_id and the estimate_id derived from it differ between two replays of
// identical input even though the physical estimate is unchanged. A
// comparison that hashed those random strings as content reported ~99% of
// frames as "different" when every x/y/vx/vy/residual value was in fact
// byte-identical. creation_sequence — the tracker's deterministic per-run
// substitute for track identity — must be what the oracle agrees or disagrees
// on instead.
func TestBuildEvidenceOracleIgnoresRandomTrackIdentity(t *testing.T) {
	db, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()
	insertRow := func(estimateID, trackID, observationID string, creationSequence int) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO lidar_observations (observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos, cluster_unix_nanos, cluster_id, record_json, inserted_at_ns) VALUES (?, 1, 'source-a', 'cal', 'sensor', 'frame', 100, 100, 1, '{"cluster":1}', 1)`, observationID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO lidar_track_estimates (estimate_id, track_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos, estimator_id, observation_model_id, param_hash, stage, measurement_source, creation_sequence, x, y, vx, vy, covariance_json, inserted_at_ns) VALUES (?, ?, ?, 'source-a', 'cal', 100, 100, 'estimator', 'model', 'params', 'online', 'obb', ?, 1, 2, 3, 4, '[]', 1)`,
			estimateID, trackID, observationID, creationSequence); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO lidar_track_residuals (estimate_id, observation_id, predicted_x, predicted_y, measurement_x, measurement_y, innovation_x, innovation_y, nis, geometry_cov_xx, geometry_cov_xy, geometry_cov_yy, disposition, reason, inserted_at_ns) VALUES (?, ?, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 'accepted', 'ok', 1)`,
			estimateID, observationID); err != nil {
			t.Fatal(err)
		}
	}
	expected := map[string]struct{}{"source-a": {}}

	insertRow("estimate/trk-AAAA/1", "trk_AAAA", "o-1", 1)
	first, err := BuildEvidenceOracle(db, expected)
	if err != nil {
		t.Fatal(err)
	}

	clearAll := func() {
		t.Helper()
		for _, table := range []string{"lidar_track_residuals", "lidar_track_estimates", "lidar_observations"} {
			if _, err := db.Exec(`DELETE FROM ` + table); err != nil {
				t.Fatal(err)
			}
		}
	}

	clearAll()
	insertRow("estimate/trk-ZZZZ/1", "trk_ZZZZ", "o-1", 1)
	sameContentDifferentIdentity, err := BuildEvidenceOracle(db, expected)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, sameContentDifferentIdentity) {
		t.Fatal("a different random track_id/estimate_id changed the semantic oracle")
	}

	clearAll()
	insertRow("estimate/trk-ZZZZ/1", "trk_ZZZZ", "o-1", 2)
	differentSequence, err := BuildEvidenceOracle(db, expected)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(first, differentSequence) {
		t.Fatal("a changed creation_sequence did not change the semantic oracle")
	}
}
