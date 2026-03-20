package main

import (
	"encoding/json"
	"fmt"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// RunBackfill opens the DB at dbPathPtr via the internal/db bootstrap path
// (which applies production PRAGMAs) and delegates to RunBackfillDB.
func RunBackfill(dbPathPtr *string, embeddedElevs []float64, dry bool) (int, int, int, error) {
	if dbPathPtr == nil || *dbPathPtr == "" {
		return 0, 0, 0, fmt.Errorf("db path nil or empty")
	}
	opened, err := dbpkg.OpenDB(*dbPathPtr)
	if err != nil {
		return 0, 0, 0, err
	}
	defer opened.Close()

	return RunBackfillDB(opened.DB, embeddedElevs, dry)
}

// RunBackfillDB performs the backfill on an existing database connection.
// The parameter uses the sqlite.SQLDB type alias so callers outside the
// storage layer need not import database/sql directly.
func RunBackfillDB(db *sqlite.SQLDB, embeddedElevs []float64, dry bool) (int, int, int, error) {
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
