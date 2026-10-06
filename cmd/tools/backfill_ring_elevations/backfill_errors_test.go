package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// seedDB creates a file-backed database carrying the production schema and
// one snapshot row with the given ring count, and returns its path.
func seedDB(t *testing.T, rings int) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "backfill.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer db.Close()

	schemaPath := filepath.Join("..", "..", "..", "internal", "db", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("reading schema.sql: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("applying schema: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO lidar_bg_snapshot (sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, grid_blob)
		 VALUES ('s1', 1, ?, 1800, '{}', x'00')`, rings); err != nil {
		t.Fatalf("seeding snapshot: %v", err)
	}
	return dbPath
}

func elevations(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i) * 0.1
	}
	return out
}

func TestRunBackfillRejectsMissingPath(t *testing.T) {
	tests := []struct {
		name string
		path *string
	}{
		{"nil pointer", nil},
		{"empty string", strPtr("")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := RunBackfill(tc.path, elevations(40), true)
			if err == nil {
				t.Fatal("RunBackfill succeeded without a db path, want an error")
			}
			if !strings.Contains(err.Error(), "db path nil or empty") {
				t.Errorf("error = %v, want it to name the missing path", err)
			}
		})
	}
}

func TestRunBackfillReportsOpenFailure(t *testing.T) {
	// A path inside a nonexistent directory cannot be opened.
	bad := filepath.Join(t.TempDir(), "absent", "backfill.db")

	if _, _, _, err := RunBackfill(&bad, elevations(40), true); err == nil {
		t.Fatal("RunBackfill on an unopenable path succeeded, want an error")
	}
}

func TestRunBackfillDelegatesToRunBackfillDB(t *testing.T) {
	dbPath := seedDB(t, 40)

	// Dry run so the wrapper's own open/close path is what is under test.
	total, updated, skipped, err := RunBackfill(&dbPath, elevations(40), true)
	if err != nil {
		t.Fatalf("RunBackfill: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
}

func TestRunBackfillDBDryRunLeavesRowsUntouched(t *testing.T) {
	dbPath := seedDB(t, 40)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer db.Close()

	if _, updated, _, err := RunBackfillDB(db, elevations(40), true); err != nil {
		t.Fatalf("RunBackfillDB: %v", err)
	} else if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}

	// A dry run reports what it would do without writing.
	var stored sql.NullString
	if err := db.QueryRow(`SELECT ring_elevations_json FROM lidar_bg_snapshot`).Scan(&stored); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if stored.Valid && stored.String != "" {
		t.Errorf("ring_elevations_json = %q, want it untouched by a dry run", stored.String)
	}
}

func TestRunBackfillDBReportsQueryFailure(t *testing.T) {
	dbPath := seedDB(t, 40)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing database: %v", err)
	}

	if _, _, _, err := RunBackfillDB(db, elevations(40), false); err == nil {
		t.Fatal("RunBackfillDB on a closed database succeeded, want an error")
	}
}

func TestRunBackfillDBCountsUpdateFailuresAsSkipped(t *testing.T) {
	dbPath := seedDB(t, 40)

	// A read-only connection can run the SELECT but not the UPDATE, so the
	// row is counted as skipped rather than failing the whole backfill.
	ro, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("opening read-only database: %v", err)
	}
	defer ro.Close()

	total, updated, skipped, err := RunBackfillDB(ro, elevations(40), false)
	if err != nil {
		t.Fatalf("RunBackfillDB: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if updated != 0 {
		t.Errorf("updated = %d, want 0 on a read-only database", updated)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}

func TestRunBackfillDBSkipsRingCountMismatch(t *testing.T) {
	dbPath := seedDB(t, 16) // snapshot has 16 rings
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer db.Close()

	// The embedded elevation table is for a 40-ring sensor, so it must not be
	// written onto a 16-ring snapshot.
	total, updated, skipped, err := RunBackfillDB(db, elevations(40), false)
	if err != nil {
		t.Fatalf("RunBackfillDB: %v", err)
	}
	if total != 1 || updated != 0 || skipped != 1 {
		t.Errorf("total/updated/skipped = %d/%d/%d, want 1/0/1", total, updated, skipped)
	}
}

func TestRunBackfillDBSkipsWhenNoEmbeddedElevations(t *testing.T) {
	dbPath := seedDB(t, 40)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer db.Close()

	total, updated, skipped, err := RunBackfillDB(db, nil, false)
	if err != nil {
		t.Fatalf("RunBackfillDB: %v", err)
	}
	if total != 1 || updated != 0 || skipped != 1 {
		t.Errorf("total/updated/skipped = %d/%d/%d, want 1/0/1", total, updated, skipped)
	}
}

func strPtr(s string) *string { return &s }
