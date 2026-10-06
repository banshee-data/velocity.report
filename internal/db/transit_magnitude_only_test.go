package db

import (
	"context"
	"testing"
	"time"
)

// insertMagnitudeOnly inserts a radar_data row carrying a magnitude and no
// speed, which serial ingest accepts.
func insertMagnitudeOnly(t *testing.T, db *DB, writeTimestamp, magnitude float64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO radar_data (write_timestamp, raw_event) VALUES (?, json_object('magnitude', ?))`,
		writeTimestamp, magnitude); err != nil {
		t.Fatalf("insert magnitude-only row: %v", err)
	}
}

// Transits are speed sessions, so a row without a speed is not a transit
// input. Magnitude-only rows inside an otherwise valid window used to fail
// the whole window, because ABS(speed) was scanned into a non-null float;
// now they are left out and the window's transits are the ones its speed
// rows make.
func TestTransitWorker_MagnitudeOnlyRowsAreNotTransitInputs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	base := float64(time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour).Unix())
	for i := 0; i < 10; i++ {
		insertRadarData(t, db, base+float64(i), 15, 40)
	}
	// Magnitude-only rows before, inside and after the vehicle, and one
	// louder than any speed row.
	insertMagnitudeOnly(t, db, base-1, 30)
	insertMagnitudeOnly(t, db, base+4.5, 90)
	insertMagnitudeOnly(t, db, base+20, 35)

	worker := NewTransitWorker(db, 5, "mag-only-test")
	if err := worker.RunRange(context.Background(), base-10, base+30); err != nil {
		t.Fatalf("RunRange failed on a window with magnitude-only rows: %v", err)
	}

	rows, err := db.Query(`SELECT transit_start_unix, transit_end_unix, transit_max_speed, transit_max_magnitude, point_count
		FROM radar_data_transits WHERE model_version = 'mag-only-test'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var n int
	for rows.Next() {
		var start, end, maxSpeed float64
		var maxMag, points int64
		if err := rows.Scan(&start, &end, &maxSpeed, &maxMag, &points); err != nil {
			t.Fatal(err)
		}
		n++
		if start != base || end != base+9 || maxSpeed != 15 || maxMag != 40 || points != 10 {
			t.Errorf("transit [%v, %v] max speed %v, max magnitude %d, %d points; want [%v, %v], 15, 40, 10",
				start, end, maxSpeed, maxMag, points, base, base+9)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("got %d transits, want 1", n)
	}

	var links int
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_transit_links l
		JOIN radar_data d ON d.data_id = l.data_rowid WHERE d.speed IS NULL`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Errorf("%d magnitude-only rows linked to a transit, want 0", links)
	}
}

// An hour holding only magnitude-only rows cannot produce a transit, so it
// is not a transit gap: listed, it would stay a gap that no run can fill.
func TestFindTransitGaps_MagnitudeOnlyHourIsNotAGap(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	hour := float64(time.Now().UTC().Truncate(time.Hour).Add(-3 * time.Hour).Unix())
	insertMagnitudeOnly(t, db, hour+10, 30)
	insertMagnitudeOnly(t, db, hour+20, 35)

	gaps, err := db.FindTransitGaps()
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 0 {
		t.Fatalf("got %d gaps for an hour of magnitude-only rows, want 0: %+v", len(gaps), gaps)
	}
}
