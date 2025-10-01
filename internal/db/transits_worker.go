package db

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"fmt"
	"math"
	"time"
)

// TransitWorker periodically scans recent radar_data and upserts sessionized transits
// into radar_data_transits and radar_transit_links. Designed to run every 15 minutes
// and process the last 20 minutes window (with a small overlap to allow updates).
type TransitWorker struct {
	DB *DB
	// Threshold in seconds used to split sessions (1,2,3,4,5 -> 1000,2000,...ms)
	ThresholdSeconds int
	ModelVersion     string
	Interval         time.Duration // how often to run (e.g., 15m)
	Window           time.Duration // lookback window (e.g., 20m)
	StopChan         chan struct{}
}

func NewTransitWorker(db *DB, thresholdSeconds int, modelVersion string) *TransitWorker {
	return &TransitWorker{
		DB:               db,
		ThresholdSeconds: thresholdSeconds,
		ModelVersion:     modelVersion,
		Interval:         15 * time.Minute,
		Window:           20 * time.Minute,
		StopChan:         make(chan struct{}),
	}
}

// Start runs the periodic worker loop in a goroutine.
func (w *TransitWorker) Start() {
	go func() {
		ticker := time.NewTicker(w.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := w.RunOnce(context.Background()); err != nil {
					fmt.Printf("transit worker run error: %v\n", err)
				}
			case <-w.StopChan:
				return
			}
		}
	}()
}

// Stop requests the worker to stop.
func (w *TransitWorker) Stop() {
	close(w.StopChan)
}

// RunOnce scans the last w.Window (+ small overlap) and upserts transits.
func (w *TransitWorker) RunOnce(ctx context.Context) error {
	now := time.Now().UTC()
	end := float64(now.Unix())
	start := float64(now.Add(-w.Window).Unix())

	return w.RunRange(ctx, start, end)

}

// RunRange scans the provided [start,end] (unix seconds as float64) and upserts transits.
func (w *TransitWorker) RunRange(ctx context.Context, start, end float64) error {
	// We'll perform individual-record clustering in Go to allow
	// multiple simultaneous objects distinguished by speed continuity.
	tx, err := w.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Query individual radar_data rows in the window (no per-second rollup)
	q := `SELECT rowid, write_timestamp AS ts, ABS(speed) AS abs_speed, magnitude FROM radar_data WHERE write_timestamp BETWEEN ? AND ? AND (speed IS NOT NULL OR magnitude IS NOT NULL) ORDER BY ts`

	rows, err := tx.QueryContext(ctx, q, start, end)
	if err != nil {
		return err
	}
	defer rows.Close()
	type rawPoint struct {
		Rowid int64
		Ts    float64
		Speed float64
		Mag   sql.NullFloat64
	}

	var points []rawPoint
	for rows.Next() {
		var p rawPoint
		if err := rows.Scan(&p.Rowid, &p.Ts, &p.Speed, &p.Mag); err != nil {
			return err
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Cluster points into multiple transits using an online greedy clustering
	// by time continuity and speed similarity.
	type transit struct {
		Start  float64
		End    float64
		MaxSp  float64
		MinSp  float64
		MaxMag float64
		MinMag float64
		Count  int64
		Points []rawPoint
	}

	var transits []transit
	// parameters
	maxGap := float64(w.ThresholdSeconds) // seconds
	speedDelta := 3.0                     // m/s allowed difference to assign same track (tunable)

	for _, p := range points {
		// try to find best matching active transit (end within gap and speed close)
		bestIdx := -1
		bestScore := math.MaxFloat64
		for i := range transits {
			t := &transits[i]
			if p.Ts-t.End > maxGap {
				continue
			}
			// use difference between point speed and transit last max speed as score
			delta := math.Abs(p.Speed - t.MaxSp)
			if delta <= speedDelta && delta < bestScore {
				bestScore = delta
				bestIdx = i
			}
		}

		if bestIdx == -1 {
			// start a new transit
			t := transit{
				Start:  p.Ts,
				End:    p.Ts,
				MaxSp:  p.Speed,
				MinSp:  p.Speed,
				MaxMag: 0,
				MinMag: 0,
				Count:  p.Rowid * 0, // placeholder; we'll count records differently below
				Points: []rawPoint{p},
			}
			if p.Mag.Valid {
				t.MaxMag = p.Mag.Float64
				t.MinMag = p.Mag.Float64
			}
			t.Count = 1
			transits = append(transits, t)
		} else {
			// append to existing transit
			t := &transits[bestIdx]
			if p.Ts < t.Start {
				t.Start = p.Ts
			}
			if p.Ts > t.End {
				t.End = p.Ts
			}
			if p.Speed > t.MaxSp {
				t.MaxSp = p.Speed
			}
			if p.Speed < t.MinSp {
				t.MinSp = p.Speed
			}
			if p.Mag.Valid && p.Mag.Float64 > t.MaxMag {
				t.MaxMag = p.Mag.Float64
			}
			if p.Mag.Valid && (t.MinMag == 0 || p.Mag.Float64 < t.MinMag) {
				t.MinMag = p.Mag.Float64
			}
			t.Count += 1
			t.Points = append(t.Points, p)
		}
	}

	// Upsert transits into radar_data_transits.
	upsertStmt, err := tx.PrepareContext(ctx, `INSERT INTO radar_data_transits (transit_key, threshold_ms, transit_start_unix, transit_end_unix, transit_max_speed, transit_min_speed, transit_max_magnitude, transit_min_magnitude, point_count, model_version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, UNIXEPOCH('subsec'), UNIXEPOCH('subsec')) ON CONFLICT(transit_key) DO UPDATE SET transit_end_unix=excluded.transit_end_unix, transit_max_speed=excluded.transit_max_speed, transit_min_speed=excluded.transit_min_speed, transit_max_magnitude=excluded.transit_max_magnitude, transit_min_magnitude=excluded.transit_min_magnitude, point_count=excluded.point_count, model_version=excluded.model_version, updated_at=UNIXEPOCH('subsec')`)
	if err != nil {
		return err
	}
	defer upsertStmt.Close()

	// generate stable transit keys using SHA1(start|threshold|model_version)
	// Note: we intentionally omit end time so the key doesn't change as new points extend the transit

	// Refresh links for transits in the window: delete previous links, we'll insert as we go
	deleteLinks := `DELETE FROM radar_transit_links WHERE transit_id IN (SELECT transit_id FROM radar_data_transits WHERE transit_start_unix BETWEEN ? AND ?);`
	if _, err := tx.ExecContext(ctx, deleteLinks, start, end); err != nil {
		return err
	}

	// Prepare upsert for links with score (data_rowid)
	linkUpsert, err := tx.PrepareContext(ctx, `INSERT INTO radar_transit_links (transit_id, data_rowid, link_score, created_at) VALUES (?, ?, ?, UNIXEPOCH('subsec')) ON CONFLICT(transit_id, data_rowid) DO UPDATE SET link_score=excluded.link_score, created_at=excluded.created_at`)
	if err != nil {
		return err
	}
	defer linkUpsert.Close()

	// scoring params
	maxSpeedTol := 5.0
	alpha := 0.6
	minScore := 0.01

	for _, t := range transits {
		// use integer start second for stable key
		keyRaw := fmt.Sprintf("%d|%d|%s", int64(math.Floor(t.Start)), w.ThresholdSeconds, w.ModelVersion)
		sum := sha1.Sum([]byte(keyRaw))
		transitKey := fmt.Sprintf("%x", sum)

		_, err := upsertStmt.ExecContext(ctx, transitKey, w.ThresholdSeconds*1000, t.Start, t.End, t.MaxSp, t.MinSp, t.MaxMag, t.MinMag, t.Count, w.ModelVersion)
		if err != nil {
			return err
		}

		// fetch transit_id for this key (either new or existing)
		var transitID int64
		if err := tx.QueryRowContext(ctx, `SELECT transit_id FROM radar_data_transits WHERE transit_key = ?`, transitKey).Scan(&transitID); err != nil {
			return err
		}

		// insert links for points assigned to this transit (O(points) overall)
		tStart := t.Start - 1.0
		tEnd := t.End + 1.0
		tDur := math.Max(1.0, t.End-t.Start+1.0)
		for _, p := range t.Points {
			// time is already within transit by construction, but guard again
			if p.Ts < tStart || p.Ts > tEnd {
				continue
			}
			dt := math.Min(p.Ts, tEnd) - math.Max(p.Ts, tStart)
			timeScore := dt / tDur
			speedDelta := math.Abs(p.Speed - t.MaxSp)
			speedScore := math.Max(0.0, 1.0-(speedDelta/maxSpeedTol))
			score := alpha*timeScore + (1.0-alpha)*speedScore
			if score < minScore {
				continue
			}
			if _, err := linkUpsert.ExecContext(ctx, transitID, p.Rowid, score); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}
