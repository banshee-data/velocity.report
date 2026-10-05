package sqlite

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// The oracle's coverage of lidar_track_solid_bodies (near-edge plan S2.4).
// What carries the weight: a run with no solid bodies builds the oracle it
// built before the table was covered, byte for byte, whether the table is
// empty or absent; a run with solid bodies has their content in the oracle,
// in an order that does not depend on how the rows were written or on the
// tracker's random identities; and a solid body that names no stored
// observation fails closed, as an estimate does.

// oracleFixtureSources is the fixture's source manifest.
var oracleFixtureSources = map[string]struct{}{"source-a": {}, "source-b": {}}

// oracleFixture is a database with three observations over two sources, an
// online estimate and residual on two of them, and no solid body.
func oracleFixture(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	db, cleanup := setupTrackingPipelineTestDB(t)
	for _, o := range []struct {
		id, source string
		frame      int64
	}{{"o-1", "source-a", 100}, {"o-2", "source-a", 200}, {"o-3", "source-b", 100}} {
		if _, err := db.Exec(`INSERT INTO lidar_observations (observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos, cluster_unix_nanos, cluster_id, record_json, inserted_at_ns) VALUES (?, 1, ?, 'cal', 'sensor', 'frame', ?, ?, 1, '{"cluster":1}', 1)`, o.id, o.source, o.frame, o.frame); err != nil {
			cleanup()
			t.Fatal(err)
		}
	}
	for _, e := range []struct {
		id, observation string
		frame           int64
	}{{"e-1", "o-1", 100}, {"e-2", "o-2", 200}} {
		if _, err := db.Exec(`INSERT INTO lidar_track_estimates (estimate_id, track_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos, estimator_id, observation_model_id, param_hash, stage, measurement_source, creation_sequence, x, y, vx, vy, covariance_json, inserted_at_ns, reference_point, support_instant) VALUES (?, 'track', ?, 'source-a', 'cal', ?, ?, 'cv_kf_v1', 'near_edge_candidate_v1', 'params', 'online', 'near_edge_candidate_v1', 1, 1, 2, 3, 4, '[]', 1, 'body_centre', 'observed')`, e.id, e.observation, e.frame, e.frame); err != nil {
			cleanup()
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO lidar_track_residuals (estimate_id, observation_id, predicted_x, predicted_y, measurement_x, measurement_y, innovation_x, innovation_y, nis, geometry_cov_xx, geometry_cov_xy, geometry_cov_yy, disposition, reason, inserted_at_ns) VALUES (?, ?, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 'accepted', 'ok', 1)`, e.id, e.observation); err != nil {
			cleanup()
			t.Fatal(err)
		}
	}
	return db, cleanup
}

// oracleFixtureBody is a valid solid body on one of the fixture's
// observations, at a stage.
func oracleFixtureBody(observationID string, frame int64, stage string) TrackSolidBody {
	sb := testSolidBody(observationID, frame)
	sb.EstimateID = "solid_body/track/" + stage + "/" + observationID
	sb.TrackID = "track"
	sb.SourceID, sb.CalibrationID = "source-a", "cal"
	sb.Stage = stage
	if stage != "online" {
		sb.Reading.Estimate.Stage = l5tracks.StageSmoothed
		sb.EstimatorID = "cv_kf_v1+rts_fixed_assignment_v1"
	}
	return sb
}

// oracleBytes is the oracle as lidar-evidence-oracle writes it.
func oracleBytes(t *testing.T, oracle EvidenceOracle) []byte {
	t.Helper()
	payload, err := json.MarshalIndent(oracle, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(payload, '\n')
}

func buildFixtureOracle(t *testing.T, db *sql.DB) EvidenceOracle {
	t.Helper()
	oracle, err := BuildEvidenceOracle(db, oracleFixtureSources)
	if err != nil {
		t.Fatal(err)
	}
	return oracle
}

// fixtureOracleSHA256 is the SHA-256 of the fixture's oracle file as the
// oracle wrote it before lidar_track_solid_bodies was covered (main at
// 3ee42366c). A run without solid bodies must keep producing it.
const fixtureOracleSHA256 = "1910b808deba2ba8ad0a47b3a48cc07d79b8f423fc50c955059da8849c6976b0"

func TestEvidenceOracleWithoutSolidBodiesIsUnchanged(t *testing.T) {
	db, cleanup := oracleFixture(t)
	defer cleanup()
	empty := buildFixtureOracle(t, db)
	sum := sha256.Sum256(oracleBytes(t, empty))
	if got := hex.EncodeToString(sum[:]); got != fixtureOracleSHA256 {
		t.Fatalf("an oracle with no solid bodies moved: sha256 %s, want %s\n%s", got, fixtureOracleSHA256, oracleBytes(t, empty))
	}
	if len(empty.Tables) != 3 {
		t.Fatalf("an empty solid-body table is listed: %+v", empty.Tables)
	}
	for _, frame := range empty.Frames {
		if len(frame.Tables) != 3 {
			t.Fatalf("frame %s@%d lists %d tables", frame.SourceID, frame.FrameUnixNanos, len(frame.Tables))
		}
	}
	// An evidence database written before the table existed reads as one
	// with no solid bodies.
	if _, err := db.Exec(`DROP TABLE lidar_track_solid_bodies`); err != nil {
		t.Fatal(err)
	}
	if absent := buildFixtureOracle(t, db); !reflect.DeepEqual(absent, empty) {
		t.Fatalf("an absent solid-body table changed the oracle:\n%+v\n%+v", absent.Tables, empty.Tables)
	}
}

// insertFixtureBodies writes the given solid bodies in order.
func insertFixtureBodies(t *testing.T, db *sql.DB, bodies ...TrackSolidBody) {
	t.Helper()
	for _, sb := range bodies {
		if err := InsertSolidBody(db, sb); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEvidenceOracleCoversSolidBodies(t *testing.T) {
	bodies := []TrackSolidBody{
		oracleFixtureBody("o-1", 100, "online"),
		oracleFixtureBody("o-1", 100, "fixed_lag"),
		oracleFixtureBody("o-2", 200, "online"),
	}
	build := func(order ...int) (EvidenceOracle, *sql.DB, func()) {
		db, cleanup := oracleFixture(t)
		for _, i := range order {
			insertFixtureBodies(t, db, bodies[i])
		}
		return buildFixtureOracle(t, db), db, cleanup
	}
	db, cleanup := oracleFixture(t)
	without := buildFixtureOracle(t, db)
	cleanup()
	with, db, cleanup := build(0, 1, 2)
	defer cleanup()

	if len(with.Tables) != 4 || with.Tables[3].Name != "lidar_track_solid_bodies" || with.Tables[3].RowCount != 3 {
		t.Fatalf("solid bodies are not covered: %+v", with.Tables)
	}
	if !reflect.DeepEqual(with.Tables[:3], without.Tables) {
		t.Fatalf("covering solid bodies moved the other tables' digests:\n%+v\n%+v", with.Tables[:3], without.Tables)
	}
	for _, frame := range with.Frames {
		// Every frame lists the table, a frame without solid bodies with a
		// count of zero, so frames compare like with like.
		if len(frame.Tables) != 4 || frame.Tables[3].Name != "lidar_track_solid_bodies" {
			t.Fatalf("frame %s@%d lists %+v", frame.SourceID, frame.FrameUnixNanos, frame.Tables)
		}
	}
	if counts := []int64{with.Frames[0].Tables[3].RowCount, with.Frames[1].Tables[3].RowCount, with.Frames[2].Tables[3].RowCount}; !reflect.DeepEqual(counts, []int64{2, 1, 0}) {
		t.Fatalf("per-frame solid-body counts %v, want [2 1 0] (source-a@100, source-a@200, source-b@100)", counts)
	}

	// The order rows were written in is not content.
	reversed, _, reversedCleanup := build(2, 1, 0)
	defer reversedCleanup()
	if !reflect.DeepEqual(with, reversed) {
		t.Fatal("the order solid bodies were written in changed the oracle")
	}

	// Nor are the tracker's random identities or the insert time.
	for _, statement := range []string{
		`UPDATE lidar_track_solid_bodies SET track_id = 'trk_other'`,
		`UPDATE lidar_track_solid_bodies SET estimate_id = estimate_id || '/renamed'`,
		`UPDATE lidar_track_solid_bodies SET inserted_at_ns = 999`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
		if got := buildFixtureOracle(t, db); !reflect.DeepEqual(got, with) {
			t.Fatalf("%s changed the oracle", statement)
		}
	}

	// Every belief, the revised state and the version key are.
	for _, statement := range []string{
		`UPDATE lidar_track_solid_bodies SET length_m = 4.5 WHERE stage = 'fixed_lag'`,
		`UPDATE lidar_track_solid_bodies SET x = 13 WHERE stage = 'fixed_lag'`,
		`UPDATE lidar_track_solid_bodies SET reference_point = 'cluster_medoid' WHERE stage = 'fixed_lag'`,
		`UPDATE lidar_track_solid_bodies SET aspect_rad = NULL WHERE stage = 'fixed_lag'`,
		`UPDATE lidar_track_solid_bodies SET creation_sequence = 8 WHERE stage = 'fixed_lag'`,
		`UPDATE lidar_track_solid_bodies SET stage = 'final' WHERE stage = 'fixed_lag'`,
	} {
		before := buildFixtureOracle(t, db)
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
		after := buildFixtureOracle(t, db)
		if reflect.DeepEqual(before, after) {
			t.Fatalf("%s did not change the oracle", statement)
		}
		if !reflect.DeepEqual(before.Tables[:3], after.Tables[:3]) {
			t.Fatalf("%s moved another table's digest", statement)
		}
	}
}

func TestEvidenceOracleRefusesABrokenSolidBodyLink(t *testing.T) {
	for name, tc := range map[string]struct {
		statement string
		want      string
	}{
		"missing observation": {`UPDATE lidar_track_solid_bodies SET observation_id = 'o-missing'`, "links missing observation"},
		"another frame":       {`UPDATE lidar_track_solid_bodies SET frame_unix_nanos = 999`, "identity differs"},
		"another calibration": {`UPDATE lidar_track_solid_bodies SET calibration_id = 'cal-2'`, "identity differs"},
		"undeclared source":   {`UPDATE lidar_track_solid_bodies SET source_id = 'source-z'`, "identity differs"},
		"unreadable position": {`UPDATE lidar_track_solid_bodies SET x = 'not a number'`, "scan solid body"},
		// SQLite stores an overflowing literal as infinity, which has no JSON
		// form: the row cannot be hashed, so the oracle cannot be built.
		"infinite position": {`UPDATE lidar_track_solid_bodies SET x = 9e999`, "hash lidar_track_solid_bodies row"},
	} {
		db, cleanup := oracleFixture(t)
		insertFixtureBodies(t, db, oracleFixtureBody("o-1", 100, "online"))
		if _, err := db.Exec(tc.statement); err != nil {
			cleanup()
			t.Fatalf("%s: %v", name, err)
		}
		_, err := BuildEvidenceOracle(db, oracleFixtureSources)
		cleanup()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want %q", name, err, tc.want)
		}
	}
}

// An evidence database written before migration 000053 has a solid-body
// table without the support columns. Empty, it builds the oracle it always
// built; with rows, the oracle refuses rather than hash a partial row.
func TestEvidenceOracleReadsAnOlderSolidBodyTableOnlyWhenItHasRows(t *testing.T) {
	db, cleanup := oracleFixture(t)
	defer cleanup()
	for _, column := range []string{"support_instant", "support_fragmented", "support_truncated"} {
		if _, err := db.Exec(`ALTER TABLE lidar_track_solid_bodies DROP COLUMN ` + column); err != nil {
			t.Fatal(err)
		}
	}
	sum := sha256.Sum256(oracleBytes(t, buildFixtureOracle(t, db)))
	if got := hex.EncodeToString(sum[:]); got != fixtureOracleSHA256 {
		t.Fatalf("an empty pre-000053 solid-body table moved the oracle: sha256 %s", got)
	}
	if _, err := db.Exec(`INSERT INTO lidar_track_solid_bodies (estimate_id, track_id, creation_sequence, observation_id, source_id,
		calibration_id, frame_unix_nanos, measurement_unix_nanos, estimator_id, observation_model_id, param_hash, stage, state_model,
		reference_point, x, y, vx, vy, covariance_json, heading_rad, heading_variance_rad2, heading_ambiguous_weight, heading_provenance,
		length_m, length_sigma_m, length_frames, length_provenance, width_m, width_sigma_m, width_frames, width_provenance,
		height_m, height_sigma_m, height_frames, height_provenance, ground_z, ground_surface_model, motion_class, motion_posterior,
		estimation_state, last_observed_unix_nanos, support_points, coasted_frames, measurement_source, measurement_rank,
		visible_faces, inferred_extent, aspect_rad, nis, fallback_reason, inserted_at_ns)
		VALUES ('sb-1', 'track', 1, 'o-1', 'source-a', 'cal', 100, 100, 'cv_kf_v1', 'near_edge_candidate_v1', 'params', 'online',
		'cv_cartesian_v1', 'body_centre', 1, 2, 3, 4, '[]', 0, 0, 0, 'none', 4.5, 1.5, 0, 'class_prior', 1.9, 1.5, 0, 'class_prior',
		1.6, 1.5, 0, 'class_prior', 0, '', 'rigid_vehicle', 1, 'initialising', 100, 10, 0, 'medoid_v0', 2, '', 0, NULL, 0, '', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildEvidenceOracle(db, oracleFixtureSources); err == nil || !strings.Contains(err.Error(), "query solid bodies") {
		t.Fatalf("pre-000053 solid bodies: error %v", err)
	}
}

// The table lookup's own failure is reported, not read as an absent table.
func TestEvidenceOracleReportsAFailedSolidBodyLookup(t *testing.T) {
	db, cleanup := oracleFixture(t)
	cleanup()
	b := oracleBuilder{}
	if err := b.readSolidBodies(db); err == nil || !strings.Contains(err.Error(), "count solid bodies") {
		t.Fatalf("a closed database: error %v", err)
	}
}
