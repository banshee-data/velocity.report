package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunBackfill(t *testing.T) {
	// create temporary sqlite file-backed DB
	f, err := os.CreateTemp("", "testdb-*.db")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.Close()

	dbPath := f.Name()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Apply essential PRAGMAs
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			t.Fatalf("Failed to execute %q: %v", pragma, err)
		}
	}

	// Read and execute schema.sql from the db package
	// Path is relative from cmd/tools/backfill_ring_elevations/
	schemaPath := filepath.Join("..", "..", "..", "internal", "db", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("Failed to read schema.sql: %v", err)
	}

	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("Failed to execute schema.sql: %v", err)
	}

	// Baseline at latest migration version
	// NOTE: Update this when new migrations are added to internal/db/migrations/
	latestMigrationVersion := 15
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (?, false)`, latestMigrationVersion); err != nil {
		t.Fatalf("Failed to baseline migrations: %v", err)
	}

	// Insert three rows: two with rings matching embedded (e.g., 40), one with different rings
	_, err = db.Exec(`INSERT INTO lidar_bg_snapshot (sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, grid_blob) VALUES
	('s1', 1, 40, 1800, '{}', x'00'),
	('s2', 2, 40, 1800, '{}', x'00'),
	('s3', 3, 16, 360, '{}', x'00')`)
	if err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	// prepare embeddedElevs of length 40
	embedded := make([]float64, 40)
	for i := range embedded {
		embedded[i] = float64(i) * 0.1
	}

	// Call RunBackfillDB directly on the open DB to avoid file-lock races in tests.
	total, updated, skipped, err := RunBackfillDB(db, embedded, false)
	if err != nil {
		t.Fatalf("RunBackfill error: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total=3 got %d", total)
	}
	if updated != 2 {
		t.Fatalf("expected updated=2 got %d", updated)
	}
	if skipped != 1 {
		t.Fatalf("expected skipped=1 got %d", skipped)
	}

	// reopen DB to verify the two updated rows have ring_elevations_json set
	db2, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db2.Close()

	rows, err := db2.Query(`SELECT snapshot_id, ring_elevations_json FROM lidar_bg_snapshot ORDER BY snapshot_id`)
	if err != nil {
		t.Fatalf("query rows: %v", err)
	}
	defer rows.Close()
	countSet := 0
	for rows.Next() {
		var id int
		var ringJSON sql.NullString
		if err := rows.Scan(&id, &ringJSON); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if id == 1 || id == 2 {
			if !ringJSON.Valid || ringJSON.String == "" {
				t.Fatalf("expected ring elevations for id %d", id)
			}
			// try unmarshal to be sure it's valid JSON
			var arr []float64
			if err := json.Unmarshal([]byte(ringJSON.String), &arr); err != nil {
				t.Fatalf("invalid json for id %d: %v", id, err)
			}
			if len(arr) != 40 {
				t.Fatalf("unexpected len for id %d: %d", id, len(arr))
			}
			countSet++
		} else if id == 3 {
			if ringJSON.Valid && ringJSON.String != "" {
				t.Fatalf("expected empty ring elevations for id 3")
			}
		}
	}
	if countSet != 2 {
		t.Fatalf("expected 2 set rows, got %d", countSet)
	}
}
