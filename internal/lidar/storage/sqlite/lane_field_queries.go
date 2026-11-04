package sqlite

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// RunLaneField is one part of a stored run's lane field
// (docs/plans/lidar-lane-proposals-from-tracks-plan.md §1), from the run's
// track observations, with the plan's thresholds and k = 5 always. The
// HTTP route and `velocity lidar lane-field` both call this and write the
// result's JSON, so the two give the same bytes. It returns ErrNotFound when
// the run does not exist; a run with no observations gives an empty field
// with its header.
func (s *RunStore) RunLaneField(runID string, part l8analytics.LaneFieldPart) (l8analytics.LaneField, error) {
	run, err := s.GetRun(runID)
	if err != nil {
		return l8analytics.LaneField{}, err
	}
	tracks, err := s.LaneFieldTracks(run)
	if err != nil {
		return l8analytics.LaneField{}, err
	}
	return l8analytics.BuildLaneField(tracks, LaneFieldMeta(run), part, l8analytics.DefaultLaneFieldOptions())
}

// LaneFieldMeta is the header a run's lane field carries. The site token and
// the registration record are left empty: a run does not know them, and
// `gerardus upload-lane-field` takes them as flags.
//
// The capture ref names the capture and where in it the run began, the
// source file's name and the first frame's time, so a re-export of the same
// segment after a pipeline change supersedes the field before it in
// Gerardus and two segments of one capture stay apart. A live run, which has
// no file, is named by its sensor. The tuning fingerprint is the run's
// effective parameter-set hash.
func LaneFieldMeta(run *Run) l8analytics.LaneFieldMeta {
	meta := l8analytics.LaneFieldMeta{SensorID: run.SensorID, TuningFingerprint: run.ParamsHash}
	if run.FrameStartNs != nil {
		meta.WindowStartNs = *run.FrameStartNs
	}
	if run.FrameEndNs != nil {
		meta.WindowEndNs = *run.FrameEndNs
	}
	switch {
	case run.BuildVersion != "" && run.BuildGitSHA != "":
		meta.PipelineBuild = run.BuildVersion + "+" + run.BuildGitSHA
	case run.BuildVersion != "":
		meta.PipelineBuild = run.BuildVersion
	}
	name := "live:" + run.SensorID
	if run.SourcePath != "" {
		name = filepath.Base(run.SourcePath)
	}
	if meta.WindowStartNs != 0 {
		name += "@" + time.Unix(0, meta.WindowStartNs).UTC().Format(time.RFC3339Nano)
	} else {
		name += "@run:" + run.RunID
	}
	meta.CaptureRef = name
	return meta
}

// LaneFieldTracks reads a run's tracks and their stored observations for the
// lane field. Only tracks the run recorded count, which are those it
// confirmed; observations are stored only at instants a cluster was
// associated, so coasted instants are already absent. When the run's frame
// window is known, observations outside it are dropped, so a track id reused
// by another run's rows cannot bring them in.
func (s *RunStore) LaneFieldTracks(run *Run) ([]l8analytics.LaneFieldTrack, error) {
	query := `
		SELECT r.track_id, COALESCE(r.object_class, ''),
		       COALESCE(o.frame_unix_nanos, o.ts_unix_nanos), o.x, o.y,
		       o.velocity_x, o.velocity_y, o.speed_mps,
		       o.box_x, o.box_y, o.box_heading_rad, o.box_length, o.box_width,
		       o.heading_rad, o.bounding_box_length, o.bounding_box_width
		  FROM lidar_run_tracks r
		  JOIN lidar_track_observations o ON o.track_id = r.track_id
		 WHERE r.run_id = ? AND o.x IS NOT NULL AND o.y IS NOT NULL`
	args := []any{run.RunID}
	if run.FrameStartNs != nil && run.FrameEndNs != nil && *run.FrameEndNs >= *run.FrameStartNs {
		query += ` AND COALESCE(o.frame_unix_nanos, o.ts_unix_nanos) BETWEEN ? AND ?`
		args = append(args, *run.FrameStartNs, *run.FrameEndNs)
	}
	query += ` ORDER BY r.track_id, 3`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("lane field observations of run %s: %w", run.RunID, err)
	}
	defer rows.Close()

	var out []l8analytics.LaneFieldTrack
	for rows.Next() {
		var id, class string
		var t int64
		var x, y float64
		var vx, vy, speed, bx, by, bh, bl, bw, oh, ol, ow sql.NullFloat64
		if err := rows.Scan(&id, &class, &t, &x, &y, &vx, &vy, &speed, &bx, &by, &bh, &bl, &bw, &oh, &ol, &ow); err != nil {
			return nil, fmt.Errorf("scan lane field observation: %w", err)
		}
		if len(out) == 0 || out[len(out)-1].TrackID != id {
			out = append(out, l8analytics.LaneFieldTrack{TrackID: id, Class: class})
		}
		in := l8analytics.LaneFieldInstant{
			TimeNs: t, X: x, Y: y,
			VX: vx.Float64, VY: vy.Float64, SpeedMps: speed.Float64,
		}
		// The published box when the track had one, else the cluster's
		// oriented box at the estimate, which runs before the box model
		// stored instead.
		switch {
		case bl.Valid && bw.Valid && bl.Float64 > 0 && bw.Float64 > 0:
			in.BoxX, in.BoxY, in.BoxHeadingRad = bx.Float64, by.Float64, bh.Float64
			in.BoxLength, in.BoxWidth = bl.Float64, bw.Float64
		case ol.Valid && ow.Valid && oh.Valid:
			in.BoxX, in.BoxY, in.BoxHeadingRad = x, y, oh.Float64
			in.BoxLength, in.BoxWidth = ol.Float64, ow.Float64
		}
		tr := &out[len(out)-1]
		tr.Instants = append(tr.Instants, in)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lane field observations: %w", err)
	}
	return out, nil
}
