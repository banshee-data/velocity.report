package main

import (
	"database/sql"
	"encoding/json"
	"os"
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

	// create minimal schema for lidar_bg_snapshot including ring_elevations_json
	_, err = db.Exec(`CREATE TABLE lidar_bg_snapshot (
	 snapshot_id INTEGER PRIMARY KEY,
	 sensor_id TEXT NOT NULL,
	 taken_unix_nanos INTEGER NOT NULL,
	 rings INTEGER NOT NULL,
	 azimuth_bins INTEGER NOT NULL,
	 params_json TEXT NOT NULL,
	 ring_elevations_json TEXT,
	 grid_blob BLOB NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
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
