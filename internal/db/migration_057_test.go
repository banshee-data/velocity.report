package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// legacyEstimates are lidar_track_estimates rows a database at 000056 can
// hold, one per measurement source a writer or a fixture has used, at every
// stage, with the reference and support the behaviour adapter inferred for
// each before 000057. An empty reference is a row the adapter refused.
var legacyEstimates = []struct {
	id, stage, source, reference string
}{
	{"e-obb", "online", "obb_centre_v1", "visible_obb_centre"},
	{"e-medoid", "online", "medoid_v0", "cluster_medoid"},
	{"e-fallback", "online", "medoid_fallback_v1", "cluster_medoid"},
	{"e-obb-final", "final", "obb_centre_v1", "visible_obb_centre"},
	{"e-medoid-lag", "fixed_lag", "medoid_v0", "cluster_medoid"},
	{"e-near-edge", "online", "near_edge_candidate_v1", ""},
	{"e-fixture", "final", "medoid_v1", ""},
}

// databaseAt056 holds point estimates written before any row stated its
// reference or support.
func databaseAt056(t *testing.T) (*DB, func(uint)) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "at056.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := &DB{sqlDB}
	t.Cleanup(func() { _ = db.Close() })
	migrations, err := getMigrationsFS()
	if err != nil {
		t.Fatal(err)
	}
	migrateTo := func(version uint) {
		t.Helper()
		if err := db.MigrateTo(migrations, version); err != nil {
			t.Fatalf("migrate to %d: %v", version, err)
		}
	}
	migrateTo(56)
	for i, e := range legacyEstimates {
		if _, err := db.Exec(`INSERT INTO lidar_track_estimates
			(estimate_id, track_id, creation_sequence, observation_id, source_id, calibration_id, frame_unix_nanos,
			 measurement_unix_nanos, estimator_id, observation_model_id, param_hash, stage, measurement_source,
			 x, y, vx, vy, covariance_json, inserted_at_ns)
			VALUES (?, 'trk', 1, 'o', 's', 'c', ?, ?, 'cv_kf_v1', 'm', 'p', ?, ?, 1, 2, 3, 4, '[1]', 1)`,
			e.id, 100+i, 101+i, e.stage, e.source); err != nil {
			t.Fatalf("insert %s: %v", e.id, err)
		}
	}
	return db, migrateTo
}

// estimateRows is every column 000056 knew, row by row in key order.
func estimateRows(t *testing.T, db *DB) []string {
	t.Helper()
	return queryStrings(t, db, `SELECT estimate_id || '|' || track_id || '|' || creation_sequence || '|' || observation_id
		|| '|' || source_id || '|' || calibration_id || '|' || frame_unix_nanos || '|' || measurement_unix_nanos
		|| '|' || estimator_id || '|' || observation_model_id || '|' || param_hash || '|' || stage
		|| '|' || measurement_source || '|' || x || '|' || y || '|' || vx || '|' || vy || '|' || covariance_json
		|| '|' || inserted_at_ns || '|' || state_model
		FROM lidar_track_estimates ORDER BY estimate_id`)
}

func insertStatedEstimate(db *DB, id, reference, support string) error {
	_, err := db.Exec(`INSERT INTO lidar_track_estimates
		(estimate_id, track_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos,
		 estimator_id, observation_model_id, param_hash, stage, measurement_source, x, y, vx, vy, covariance_json,
		 inserted_at_ns, reference_point, support_instant)
		VALUES (?, 'trk', 'o', 's', 'c', 900, 900, 'cv_kf_v1', 'm', 'p', 'online', 'medoid_v0', 0, 0, 0, 0, '[1]', 1, ?, ?)`,
		id, reference, support)
	return err
}

func TestMigration057BackfillsTheAdaptersMapping(t *testing.T) {
	db, migrateTo := databaseAt056(t)
	before := estimateRows(t, db)
	migrateTo(57)
	if after := estimateRows(t, db); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("estimates changed:\n was %v\n now %v", before, after)
	}
	for _, e := range legacyEstimates {
		got := queryStrings(t, db, `SELECT reference_point || '|' || support_instant FROM lidar_track_estimates WHERE estimate_id = ?`, e.id)
		if want := e.reference + "|observed"; len(got) != 1 || got[0] != want {
			t.Errorf("%s (%s): %v, want %s", e.id, e.source, got, want)
		}
	}
	if check := queryStrings(t, db, `PRAGMA integrity_check`); strings.Join(check, ",") != "ok" {
		t.Fatalf("integrity check: %v", check)
	}

	// From now on a row states both, or is not written.
	for _, c := range []struct{ reference, support string }{{"", "observed"}, {"body_centre", ""}, {"", ""}} {
		if err := insertStatedEstimate(db, "e-new", c.reference, c.support); err == nil ||
			!strings.Contains(err.Error(), "states its reference point and support token") {
			t.Errorf("reference %q, support %q: %v", c.reference, c.support, err)
		}
	}
	if err := insertStatedEstimate(db, "e-new", "body_centre", "observed"); err != nil {
		t.Fatalf("a row stating both was refused: %v", err)
	}
}

func TestMigration057RollsBackToTheColumnsItFound(t *testing.T) {
	db, migrateTo := databaseAt056(t)
	columns := func() string {
		return strings.Join(queryStrings(t, db, `SELECT name FROM pragma_table_xinfo('lidar_track_estimates') ORDER BY cid`), ",")
	}
	columnsBefore := columns()
	migrateTo(57)
	if !strings.HasSuffix(columns(), ",reference_point,support_instant") {
		t.Fatalf("columns at 57: %s", columns())
	}
	if err := insertStatedEstimate(db, "e-new", "body_centre", "observed"); err != nil {
		t.Fatal(err)
	}
	rowsAt57 := estimateRows(t, db)

	migrateTo(56)
	if got := columns(); got != columnsBefore {
		t.Fatalf("columns after the rollback: %s, want %s", got, columnsBefore)
	}
	// Every row stays, the one written at 57 as well; only its statements go
	// with the columns.
	if got := estimateRows(t, db); strings.Join(got, "\n") != strings.Join(rowsAt57, "\n") {
		t.Fatalf("rows after the rollback:\n got %v\nwant %v", got, rowsAt57)
	}
	if triggers := queryStrings(t, db, `SELECT name FROM sqlite_master WHERE type='trigger' AND tbl_name='lidar_track_estimates'`); len(triggers) != 0 {
		t.Fatalf("the rollback left %v", triggers)
	}

	// Rolling forward again backfills from measurement_source, so the row
	// that stated the body centre over a medoid measurement now reads as the
	// medoid: the loss the down migration names.
	migrateTo(57)
	if got := queryStrings(t, db, `SELECT reference_point FROM lidar_track_estimates WHERE estimate_id = 'e-new'`); len(got) != 1 || got[0] != "cluster_medoid" {
		t.Fatalf("the row written at 57 reads %v after down and up", got)
	}
}
