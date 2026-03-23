package db

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// --- Test helpers ---

func twCovDB(t *testing.T) *DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func twCovWorker(db *DB) *TransitWorker {
	return NewTransitWorker(db, 5, "cov-v1")
}

func twCovInsert(t *testing.T, db *DB, ts, speed, magnitude float64) {
	t.Helper()
	raw := fmt.Sprintf(`{"speed":%g,"magnitude":%g}`, speed, magnitude)
	_, err := db.Exec("INSERT INTO radar_data (raw_event, write_timestamp) VALUES (?, ?)", raw, ts)
	if err != nil {
		t.Fatalf("twCovInsert: %v", err)
	}
}

// --- L124-126: deleted > 0 log (run twice to trigger deduplication) ---

func TestCov_RunRange_OverlapDedup(t *testing.T) {
	db := twCovDB(t)
	w := twCovWorker(db)
	twCovInsert(t, db, 1000, 10, 50)
	twCovInsert(t, db, 1001, 11, 55)

	if err := w.RunRange(context.Background(), 999, 1002); err != nil {
		t.Fatalf("first RunRange: %v", err)
	}
	// Second run deletes overlapping transits before re-inserting.
	if err := w.RunRange(context.Background(), 999, 1002); err != nil {
		t.Fatalf("second RunRange: %v", err)
	}
}

// --- L230-232, L245-247, L371-373: clustering branches ---
// L230-232: p.Mag.Valid when creating a new transit
// L245-247: p.Speed < t.MinSp in append-to-existing-transit
// L371-373: score < minScore causes link skip

func TestCov_RunRange_ClusteringBranches(t *testing.T) {
	db := twCovDB(t)
	w := twCovWorker(db)

	// Escalating speeds: 10→12→9→12→15→18 all within gap threshold (5s).
	// Creates one transit with MaxSp=18. During scoring, points with
	// |speed - MaxSp| > maxSpeedTol(5) get score=0, triggering the
	// score < minScore continue branch. Also covers MinSp update (9<10).
	twCovInsert(t, db, 1000, 10, 50)
	twCovInsert(t, db, 1001, 12, 55)
	twCovInsert(t, db, 1002, 9, 45)
	twCovInsert(t, db, 1003, 12, 50)
	twCovInsert(t, db, 1004, 15, 50)
	twCovInsert(t, db, 1005, 18, 50)

	if err := w.RunRange(context.Background(), 999, 1006); err != nil {
		t.Fatalf("RunRange: %v", err)
	}

	// Verify transits were created.
	var cnt int
	db.QueryRow("SELECT COUNT(*) FROM radar_data_transits").Scan(&cnt)
	if cnt == 0 {
		t.Fatal("expected at least one transit")
	}
}

// --- L151-153: QueryContext error (SELECT from radar_data fails) ---

func TestCov_RunRange_QueryError(t *testing.T) {
	db := twCovDB(t)
	w := twCovWorker(db)

	// Rename radar_data instead of dropping it - avoids FK complications.
	if _, err := db.Exec("ALTER TABLE radar_data RENAME TO radar_data_hidden"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	err := w.RunRange(context.Background(), 999, 1002)
	if err == nil {
		t.Fatal("expected error from missing radar_data table")
	}
	t.Logf("got expected error: %v", err)
}

// --- L165-167: Scan error (corrupt magnitude in radar_data) ---

func TestCov_RunRange_ScanError(t *testing.T) {
	db := twCovDB(t)
	w := twCovWorker(db)

	// Insert a row where magnitude is non-numeric text.
	// The generated column stores the text; scanning into NullFloat64 fails.
	_, err := db.Exec(
		`INSERT INTO radar_data (raw_event, write_timestamp) VALUES (?, ?)`,
		`{"speed":10,"magnitude":"not_a_number"}`, 1000.0,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	err = w.RunRange(context.Background(), 999, 1002)
	if err == nil {
		t.Fatal("expected scan error from corrupt magnitude")
	}
}

// --- L281-283: upsertStmt PrepareContext error ---

func TestCov_RunRange_UpsertPrepareError(t *testing.T) {
	db := twCovDB(t)
	w := twCovWorker(db)
	twCovInsert(t, db, 1000, 10, 50)

	// Rename a column used by the INSERT so PrepareContext fails.
	if _, err := db.Exec("ALTER TABLE radar_data_transits RENAME COLUMN transit_key TO transit_key_old"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	err := w.RunRange(context.Background(), 999, 1002)
	if err == nil {
		t.Fatal("expected prepare error")
	}
}

// --- L298-300: deleteLinks ExecContext error ---

func TestCov_RunRange_DeleteLinksError(t *testing.T) {
	db := twCovDB(t)
	w := twCovWorker(db)
	twCovInsert(t, db, 1000, 10, 50)

	// Drop the links table; upsert prepare (radar_data_transits) succeeds, deleteLinks fails.
	if _, err := db.Exec("DROP TABLE radar_transit_links"); err != nil {
		t.Fatalf("drop: %v", err)
	}

	err := w.RunRange(context.Background(), 999, 1002)
	if err == nil {
		t.Fatal("expected deleteLinks error")
	}
}

// --- L316-318: linkUpsert PrepareContext error ---

func TestCov_RunRange_LinkPrepareError(t *testing.T) {
	db := twCovDB(t)
	w := twCovWorker(db)
	twCovInsert(t, db, 1000, 10, 50)

	// Rename data_rowid; DELETE (which uses transit_id) succeeds, but
	// INSERT INTO radar_transit_links (…data_rowid…) prepare fails.
	if _, err := db.Exec("ALTER TABLE radar_transit_links RENAME COLUMN data_rowid TO data_rowid_old"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	err := w.RunRange(context.Background(), 999, 1002)
	if err == nil {
		t.Fatal("expected linkUpsert prepare error")
	}
}

// --- L333-335: upsertStmt ExecContext error (trigger blocks INSERT) ---

func TestCov_RunRange_UpsertExecError(t *testing.T) {
	db := twCovDB(t)
	w := twCovWorker(db)
	twCovInsert(t, db, 1000, 10, 50)

	// BEFORE INSERT trigger blocks transit INSERT; DELETE and PREPARE still succeed.
	if _, err := db.Exec(`CREATE TRIGGER block_transit_ins BEFORE INSERT ON radar_data_transits BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	err := w.RunRange(context.Background(), 999, 1002)
	if err == nil {
		t.Fatal("expected upsert exec error")
	}
}

// --- L350-352: transitID query error (AFTER INSERT trigger changes key) ---

func TestCov_RunRange_TransitIDQueryError(t *testing.T) {
	db := twCovDB(t)
	w := twCovWorker(db)
	twCovInsert(t, db, 1000, 10, 50)

	// AFTER INSERT changes transit_key so the subsequent SELECT by key finds nothing.
	if _, err := db.Exec(`CREATE TRIGGER chg_key_after_ins AFTER INSERT ON radar_data_transits BEGIN UPDATE radar_data_transits SET transit_key = 'changed_' || NEW.transit_key WHERE transit_id = NEW.transit_id; END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	err := w.RunRange(context.Background(), 999, 1002)
	if err == nil {
		t.Fatal("expected transitID query error")
	}
}

// --- L448-450: AnalyseTransitOverlaps second query error ---

func TestCov_AnalyseOverlaps_VersionQueryError(t *testing.T) {
	db := twCovDB(t)

	// Rename model_version; COUNT(*) succeeds but GROUP BY model_version fails.
	if _, err := db.Exec("ALTER TABLE radar_data_transits RENAME COLUMN model_version TO model_version_old"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	_, err := db.AnalyseTransitOverlaps(context.Background())
	if err == nil {
		t.Fatal("expected version query error")
	}
}

// --- L124-126: DELETE overlapping transits error ---

func TestCov_RunRange_DeleteOverlapError(t *testing.T) {
	db := twCovDB(t)
	w := twCovWorker(db)

	// Rename transit_start_unix so the DELETE WHERE clause fails.
	if _, err := db.Exec("ALTER TABLE radar_data_transits RENAME COLUMN transit_start_unix TO transit_start_unix_old"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	err := w.RunRange(context.Background(), 999, 1002)
	if err == nil {
		t.Fatal("expected delete overlap error")
	}
}

// --- L371-373: linkUpsert ExecContext error ---

func TestCov_RunRange_LinkExecError(t *testing.T) {
	db := twCovDB(t)
	w := twCovWorker(db)
	twCovInsert(t, db, 1000, 10, 50)

	// BEFORE INSERT trigger on radar_transit_links blocks link inserts.
	if _, err := db.Exec(`CREATE TRIGGER block_link_ins BEFORE INSERT ON radar_transit_links BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	err := w.RunRange(context.Background(), 999, 1002)
	if err == nil {
		t.Fatal("expected linkUpsert exec error")
	}
}

// --- L489-491: AnalyseTransitOverlaps overlap query error ---

func TestCov_AnalyseOverlaps_OverlapQueryError(t *testing.T) {
	db := twCovDB(t)

	// Rename transit_end_unix; COUNT(*) and GROUP BY model_version still work,
	// but the overlap CTE referencing transit_end_unix fails.
	if _, err := db.Exec("ALTER TABLE radar_data_transits RENAME COLUMN transit_end_unix TO transit_end_unix_old"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	_, err := db.AnalyseTransitOverlaps(context.Background())
	if err == nil {
		t.Fatal("expected overlap query error")
	}
}
