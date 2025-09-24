package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// RunBackfill opens the DB at dbPathPtr and delegates to RunBackfillDB.
func RunBackfill(dbPathPtr *string, embeddedElevs []float64, dry bool) (int, int, int, error) {
	if dbPathPtr == nil || *dbPathPtr == "" {
		return 0, 0, 0, fmt.Errorf("db path nil or empty")
	}
	db, err := sql.Open("sqlite", *dbPathPtr)
	if err != nil {
		return 0, 0, 0, err
	}
	defer db.Close()

	// set busy timeout to avoid transient locks when used from CLI
	_, _ = db.Exec("PRAGMA busy_timeout = 5000;")
	return RunBackfillDB(db, embeddedElevs, dry)
}

// RunBackfillDB performs the backfill on an existing *sql.DB (useful for tests)
func RunBackfillDB(db *sql.DB, embeddedElevs []float64, dry bool) (int, int, int, error) {
	q := `SELECT snapshot_id, sensor_id, rings FROM lidar_bg_snapshot
		 WHERE ring_elevations_json IS NULL OR ring_elevations_json = ''`
	rows, err := db.Query(q)
	if err != nil {
		return 0, 0, 0, err
	}

	type candidate struct {
		id     int64
		sensor string
		rings  int
	}
	var cand []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.sensor, &c.rings); err != nil {
			rows.Close()
			return 0, 0, 0, err
		}
		cand = append(cand, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, 0, err
	}
	rows.Close()

	total := len(cand)
	updated := 0
	skipped := 0

	for _, c := range cand {
		id := c.id
		rings := c.rings

		if embeddedElevs != nil && len(embeddedElevs) == rings {
			b, err := json.Marshal(embeddedElevs)
			if err != nil {
				skipped++
				continue
			}
			if dry {
				updated++
				continue
			}
			if _, err = db.Exec(`UPDATE lidar_bg_snapshot SET ring_elevations_json = ? WHERE snapshot_id = ?`, string(b), id); err != nil {
				skipped++
				continue
			}
			updated++
			continue
		}
		skipped++
	}

	return total, updated, skipped, nil
}
