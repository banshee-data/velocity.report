package sqlite

import "fmt"

// Read paths for the per-frame acceptance evaluator (internal/lidar/perframeeval).
//
// Two hypothesis sources, each read in a total, reproducible order. Versioned
// estimates are the source acceptance is meant to use: one arm is one exact
// (source, estimator, observation model, parameter hash, stage), and the
// evaluator refuses to guess which when a database holds several. Analysis-run
// tracks are the older path: a run's track IDs from lidar_run_tracks, and their
// per-frame positions from lidar_track_observations, which a replay writes
// only when legacy track persistence is on.

// EstimateVersion identifies one versioned set of estimates in a database, with
// how much it holds.
type EstimateVersion struct {
	SourceID           string `json:"source_id"`
	EstimatorID        string `json:"estimator_id"`
	ObservationModelID string `json:"observation_model_id"`
	ParamHash          string `json:"param_hash"`
	Stage              string `json:"stage"`
	Estimates          int    `json:"estimates"`
	Tracks             int    `json:"tracks"`
}

// ListEstimateVersions returns every distinct version key in the database,
// sorted by all five fields.
func (s *StateEstimateStore) ListEstimateVersions() ([]EstimateVersion, error) {
	rows, err := s.db.Query(`
		SELECT source_id, estimator_id, observation_model_id, param_hash, stage
		     , COUNT(*), COUNT(DISTINCT track_id)
		  FROM lidar_track_estimates
		 GROUP BY source_id, estimator_id, observation_model_id, param_hash, stage
		 ORDER BY source_id, estimator_id, observation_model_id, param_hash, stage`)
	if err != nil {
		return nil, fmt.Errorf("list estimate versions: %w", err)
	}
	defer rows.Close()
	out := []EstimateVersion{}
	for rows.Next() {
		var v EstimateVersion
		if err := rows.Scan(&v.SourceID, &v.EstimatorID, &v.ObservationModelID, &v.ParamHash, &v.Stage,
			&v.Estimates, &v.Tracks); err != nil {
			return nil, fmt.Errorf("scan estimate version: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate estimate versions: %w", err)
	}
	return out, nil
}

// EstimatePosition is the part of an estimate a per-frame score needs.
type EstimatePosition struct {
	TrackID          string
	CreationSequence int64
	FrameUnixNanos   int64
	X, Y             float32
}

// ListEstimatePositions returns one version's estimates in creation_sequence,
// frame, estimate_id order. The version's counts are ignored; its five
// identity fields must all match.
func (s *StateEstimateStore) ListEstimatePositions(v EstimateVersion) ([]EstimatePosition, error) {
	rows, err := s.db.Query(`
		SELECT track_id, creation_sequence, frame_unix_nanos, x, y
		  FROM lidar_track_estimates
		 WHERE source_id = ? AND estimator_id = ? AND observation_model_id = ?
		   AND param_hash = ? AND stage = ?
		 ORDER BY creation_sequence, frame_unix_nanos, estimate_id`,
		v.SourceID, v.EstimatorID, v.ObservationModelID, v.ParamHash, v.Stage)
	if err != nil {
		return nil, fmt.Errorf("list estimate positions for %s/%s: %w", v.SourceID, v.EstimatorID, err)
	}
	defer rows.Close()
	out := []EstimatePosition{}
	for rows.Next() {
		var p EstimatePosition
		if err := rows.Scan(&p.TrackID, &p.CreationSequence, &p.FrameUnixNanos, &p.X, &p.Y); err != nil {
			return nil, fmt.Errorf("scan estimate position: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate estimate positions: %w", err)
	}
	return out, nil
}

// RunTrackPosition is one analysis-run track's position at one frame.
type RunTrackPosition struct {
	TrackID        string
	FrameUnixNanos int64
	X, Y           float32
}

// CountRunTracks returns how many tracks a run recorded. Zero means the run
// does not exist or recorded nothing; the caller cannot tell which, and says
// so.
func (s *AnalysisRunStore) CountRunTracks(runID string) (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM lidar_run_tracks WHERE run_id = ?`, runID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count tracks of run %s: %w", runID, err)
	}
	return n, nil
}

// ListRunTrackPositions returns the per-frame positions of a run's tracks, in
// track, frame, measurement-time order. Observations without a position are
// skipped. The frame time is the observation's frame_unix_nanos, falling back
// to its measurement time for rows written before that column existed.
func (s *AnalysisRunStore) ListRunTrackPositions(runID string) ([]RunTrackPosition, error) {
	rows, err := s.db.Query(`
		SELECT o.track_id, COALESCE(o.frame_unix_nanos, o.ts_unix_nanos), o.x, o.y
		  FROM lidar_track_observations o
		  JOIN lidar_run_tracks r ON r.track_id = o.track_id
		 WHERE r.run_id = ? AND o.x IS NOT NULL AND o.y IS NOT NULL
		 ORDER BY o.track_id, COALESCE(o.frame_unix_nanos, o.ts_unix_nanos), o.ts_unix_nanos`, runID)
	if err != nil {
		return nil, fmt.Errorf("list track positions of run %s: %w", runID, err)
	}
	defer rows.Close()
	out := []RunTrackPosition{}
	for rows.Next() {
		var p RunTrackPosition
		if err := rows.Scan(&p.TrackID, &p.FrameUnixNanos, &p.X, &p.Y); err != nil {
			return nil, fmt.Errorf("scan run track position: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run track positions: %w", err)
	}
	return out, nil
}
