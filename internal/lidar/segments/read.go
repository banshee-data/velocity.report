package segments

import (
	"context"
	"fmt"
	"math"
	"path/filepath"

	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// LoadEstimates reads one source and stage in a read transaction. A source
// with several estimator versions is refused: mixing arms invents traffic.
func LoadEstimates(db *sqlite.SQLDB, source, stage string) ([]Point, string, error) {
	if stage == "" {
		stage = "online"
	}
	tx, err := sqlite.BeginReadOnly(context.Background(), db)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT DISTINCT source_id FROM lidar_track_estimates WHERE stage=? ORDER BY source_id`, stage)
	if err != nil {
		return nil, "", err
	}
	var sources []string
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			break
		}
		sources = append(sources, s)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, "", err
	}
	if source == "" {
		if len(sources) != 1 {
			return nil, "", fmt.Errorf("expected one %s source, found %d; supply source", stage, len(sources))
		}
		source = sources[0]
	}
	found := false
	for _, s := range sources {
		if s == source {
			found = true
		}
	}
	if !found {
		return nil, "", fmt.Errorf("source %q has no %s estimates", source, stage)
	}
	var versions int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM (SELECT DISTINCT estimator_id,observation_model_id,param_hash FROM lidar_track_estimates WHERE source_id=? AND stage=?)`, source, stage).Scan(&versions); err != nil {
		return nil, "", err
	}
	if versions != 1 {
		return nil, "", fmt.Errorf("source %q has %d estimate versions at stage %s; choose a single version", source, versions, stage)
	}
	rows, err = tx.Query(`SELECT creation_sequence,frame_unix_nanos,x,y,vx,vy FROM lidar_track_estimates WHERE source_id=? AND stage=? ORDER BY frame_unix_nanos,creation_sequence,estimate_id`, source, stage)
	if err != nil {
		return nil, "", err
	}
	points := []Point{}
	for rows.Next() {
		var seq, t int64
		// A nil pointer is SQL NULL: an estimate without a position or a
		// velocity cannot be paired, and zero would place it at the origin.
		var x, y, vx, vy *float64
		if err = rows.Scan(&seq, &t, &x, &y, &vx, &vy); err != nil {
			rows.Close()
			return nil, "", err
		}
		if x == nil || y == nil || vx == nil || vy == nil || math.IsNaN(*x) || math.IsNaN(*y) || math.IsNaN(*vx) || math.IsNaN(*vy) {
			continue
		}
		points = append(points, Point{Track: fmt.Sprint(seq), TimeNs: t, X: *x, Y: *y, VX: *vx, VY: *vy})
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, "", err
	}
	rows.Close()
	return points, source, tx.Commit()
}

// LoadRun reads the run's own observations, keeping joins scoped by run ID.
func LoadRun(db *sqlite.SQLDB, runID string) ([]Point, error) {
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	rows, err := db.Query(`SELECT o.track_id,COALESCE(o.frame_unix_nanos,o.ts_unix_nanos),o.x,o.y,o.velocity_x,o.velocity_y,COALESCE(t.max_speed_mps,0),COALESCE(r.is_split_candidate,0),COALESCE(r.is_merge_candidate,0) FROM lidar_track_observations o JOIN lidar_run_tracks r ON r.track_id=o.track_id JOIN lidar_tracks t ON t.track_id=o.track_id WHERE r.run_id=? ORDER BY 2,o.track_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := []Point{}
	for rows.Next() {
		var p Point
		var x, y, vx, vy *float64
		var split, merge int
		if err = rows.Scan(&p.Track, &p.TimeNs, &x, &y, &vx, &vy, &p.MaxSpeed, &split, &merge); err != nil {
			return nil, err
		}
		if x == nil || y == nil || vx == nil || vy == nil {
			continue
		}
		p.X = *x
		p.Y = *y
		p.VX = *vx
		p.VY = *vy
		p.SplitFlag = split != 0 || merge != 0
		points = append(points, p)
	}
	return points, rows.Err()
}

// CapturesForRange returns indexed files with real packet bounds. It does not
// infer an offset from filenames: an incorrect offset cuts the wrong evidence.
func CapturesForRange(db *sqlite.SQLDB, start, end int64) ([]Capture, error) {
	rows, err := db.Query(`SELECT r.path,f.rel_path,f.first_packet_ns,f.last_packet_ns FROM lidar_capture_files f JOIN lidar_capture_roots r ON r.root_id=f.root_id WHERE f.present=1 AND f.probe_state='ok' AND f.first_packet_ns<=? AND f.last_packet_ns>=? ORDER BY f.first_packet_ns`, end, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Capture{}
	for rows.Next() {
		var root, rel string
		var c Capture
		if err = rows.Scan(&root, &rel, &c.FirstNs, &c.LastNs); err != nil {
			return nil, err
		}
		c.Path = filepath.Join(root, rel)
		out = append(out, c)
	}
	return out, rows.Err()
}
