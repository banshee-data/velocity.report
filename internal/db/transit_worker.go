package db

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"fmt"
	"log"
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

// RunFullHistory scans the full available radar_data range and upserts transits.
func (w *TransitWorker) RunFullHistory(ctx context.Context) error {
	var start sql.NullFloat64
	var end sql.NullFloat64
	if err := w.DB.QueryRowContext(ctx, `SELECT MIN(write_timestamp), MAX(write_timestamp) FROM radar_data`).Scan(&start, &end); err != nil {
		return err
	}
	if !start.Valid || !end.Valid {
		log.Printf("Transit worker full-history run skipped (no radar data)")
		return nil
	}
	if start.Float64 >= end.Float64 {
		log.Printf("Transit worker full-history run skipped (invalid range): start=%v end=%v", start.Float64, end.Float64)
		return nil
	}
	return w.RunRange(ctx, start.Float64, end.Float64)
}

// RunRange scans the provided [start,end] (unix seconds as float64) and upserts transits.
func (w *TransitWorker) RunRange(ctx context.Context, start, end float64) error {
	// We'll perform individual-record clustering in Go to allow
	// multiple simultaneous objects distinguished by speed continuity.
	tx, err := w.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			// ErrTxDone means transaction was already committed/rolled back
			log.Printf("warning: failed to rollback transaction: %v", err)
		}
	}()

	// Delete overlapping transits with the same model_version before inserting.
	// This handles hourly re-runs and window overlaps, preventing duplicates.
	// We delete transits that:
	// 1. Start within the processing range, OR
	// 2. End within the processing range, OR
	// 3. Span the entire processing range
	deleteQuery := `
		DELETE FROM radar_data_transits
		WHERE model_version = ?
		  AND (
			  (transit_start_unix BETWEEN ? AND ?)
			  OR (transit_end_unix BETWEEN ? AND ?)
			  OR (transit_start_unix <= ? AND transit_end_unix >= ?)
		  )
	`
	result, err := tx.ExecContext(ctx, deleteQuery,
		w.ModelVersion,
		start, end, // transit starts in range
		start, end, // transit ends in range
		start, end, // transit spans entire range
	)
	if err != nil {
		return fmt.Errorf("failed to delete overlapping transits: %w", err)
	}

	deleted, _ := result.RowsAffected()
	if deleted > 0 {
		log.Printf("Transit worker: deleted %d overlapping %s transits in range [%v, %v]",
			deleted, w.ModelVersion, start, end)
	}

	// Query individual radar_data rows in the window (no per-second rollup)
	q := `
		SELECT
			rowid,
			write_timestamp AS ts,
			ABS(speed) AS abs_speed,
			magnitude
		FROM
			radar_data
		WHERE
			write_timestamp BETWEEN ? AND ?
			AND (speed IS NOT NULL OR magnitude IS NOT NULL)
		ORDER BY
			ts
	`

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
	upsertStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO radar_data_transits (
			transit_key,
			threshold_ms,
			transit_start_unix,
			transit_end_unix,
			transit_max_speed,
			transit_min_speed,
			transit_max_magnitude,
			transit_min_magnitude,
			point_count,
			model_version,
			created_at,
			updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, UNIXEPOCH('subsec'), UNIXEPOCH('subsec')
		)
		ON CONFLICT(transit_key) DO UPDATE SET
			transit_end_unix = excluded.transit_end_unix,
			transit_max_speed = excluded.transit_max_speed,
			transit_min_speed = excluded.transit_min_speed,
			transit_max_magnitude = excluded.transit_max_magnitude,
			transit_min_magnitude = excluded.transit_min_magnitude,
			point_count = excluded.point_count,
			model_version = excluded.model_version,
			updated_at = UNIXEPOCH('subsec')
	`)
	if err != nil {
		return err
	}
	defer upsertStmt.Close()

	// generate stable transit keys using SHA1(start|threshold|model_version)
	// Note: we intentionally omit end time so the key doesn't change as new points extend the transit

	// Refresh links for transits in the window: delete previous links, we'll insert as we go
	deleteLinks := `
		DELETE FROM radar_transit_links
		WHERE transit_id IN (
			SELECT transit_id
			FROM radar_data_transits
			WHERE transit_start_unix BETWEEN ? AND ?
		);
	`
	if _, err := tx.ExecContext(ctx, deleteLinks, start, end); err != nil {
		return err
	}

	// Prepare upsert for links with score (data_rowid)
	linkUpsert, err := tx.PrepareContext(ctx, `
		INSERT INTO radar_transit_links (
			transit_id,
			data_rowid,
			link_score,
			created_at
		) VALUES (
			?, ?, ?, UNIXEPOCH('subsec')
		)
		ON CONFLICT(transit_id, data_rowid) DO UPDATE SET
			link_score = excluded.link_score,
			created_at = excluded.created_at
	`)
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
		if err := tx.QueryRowContext(
			ctx,
			`
			SELECT
				transit_id
			FROM
				radar_data_transits
			WHERE
				transit_key = ?
			`,
			transitKey,
		).Scan(&transitID); err != nil {
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

// MigrateModelVersion replaces all transits from oldVersion with the worker's
// current ModelVersion by deleting old transits and re-running over full history.
func (w *TransitWorker) MigrateModelVersion(ctx context.Context, oldVersion string) error {
	if oldVersion == w.ModelVersion {
		return fmt.Errorf("old and new model versions must differ (both are %q)", oldVersion)
	}

	log.Printf("Transit worker: migrating from %s to %s", oldVersion, w.ModelVersion)

	// Delete all old version transits
	result, err := w.DB.ExecContext(ctx,
		`DELETE FROM radar_data_transits WHERE model_version = ?`,
		oldVersion,
	)
	if err != nil {
		return fmt.Errorf("failed to delete old version transits: %w", err)
	}

	deleted, _ := result.RowsAffected()
	log.Printf("Transit worker: deleted %d %s transits", deleted, oldVersion)

	// Re-run over full history with new version
	return w.RunFullHistory(ctx)
}

// DeleteAllTransits removes all transits for a given model version.
func (w *TransitWorker) DeleteAllTransits(ctx context.Context, modelVersion string) (int64, error) {
	result, err := w.DB.ExecContext(ctx,
		`DELETE FROM radar_data_transits WHERE model_version = ?`,
		modelVersion,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete transits: %w", err)
	}
	return result.RowsAffected()
}

// TransitOverlapStats contains statistics about overlapping transits.
type TransitOverlapStats struct {
	TotalTransits      int64
	ModelVersionCounts map[string]int64
	Overlaps           []TransitOverlap
}

// TransitOverlap represents a pair of overlapping transits with different model versions.
type TransitOverlap struct {
	ModelVersion1 string
	ModelVersion2 string
	OverlapCount  int64
}

// AnalyseTransitOverlaps returns statistics about overlapping transits across model versions.
func (db *DB) AnalyseTransitOverlaps(ctx context.Context) (*TransitOverlapStats, error) {
	stats := &TransitOverlapStats{
		ModelVersionCounts: make(map[string]int64),
	}

	// Get total count
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM radar_data_transits`).Scan(&stats.TotalTransits); err != nil {
		return nil, fmt.Errorf("failed to count transits: %w", err)
	}

	// Get counts per model version
	rows, err := db.QueryContext(ctx, `SELECT model_version, COUNT(*) FROM radar_data_transits GROUP BY model_version`)
	if err != nil {
		return nil, fmt.Errorf("failed to count by model version: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var mv sql.NullString
		var count int64
		if err := rows.Scan(&mv, &count); err != nil {
			return nil, err
		}
		key := "(null)"
		if mv.Valid {
			key = mv.String
		}
		stats.ModelVersionCounts[key] = count
	}

	// Find overlapping transits between different model versions
	overlapQuery := `
		WITH overlaps AS (
			SELECT
				t1.model_version as mv1,
				t2.model_version as mv2
			FROM radar_data_transits t1
			JOIN radar_data_transits t2
				ON t1.transit_id < t2.transit_id
				AND COALESCE(t1.model_version, '') != COALESCE(t2.model_version, '')
				AND (
					(t1.transit_start_unix BETWEEN t2.transit_start_unix AND t2.transit_end_unix)
					OR (t1.transit_end_unix BETWEEN t2.transit_start_unix AND t2.transit_end_unix)
					OR (t1.transit_start_unix <= t2.transit_start_unix
						AND t1.transit_end_unix >= t2.transit_end_unix)
				)
		)
		SELECT COALESCE(mv1, '(null)'), COALESCE(mv2, '(null)'), COUNT(*)
		FROM overlaps
		GROUP BY mv1, mv2
	`

	overlapRows, err := db.QueryContext(ctx, overlapQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to find overlaps: %w", err)
	}
	defer overlapRows.Close()

	for overlapRows.Next() {
		var o TransitOverlap
		if err := overlapRows.Scan(&o.ModelVersion1, &o.ModelVersion2, &o.OverlapCount); err != nil {
			return nil, err
		}
		stats.Overlaps = append(stats.Overlaps, o)
	}

	return stats, nil
}
