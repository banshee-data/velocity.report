package sqlite

import (
	"fmt"
)

// Read paths for the label-free track scorecard (cmd/tools/lidar-track-scorecard).
//
// Every query has a total ORDER BY on reproducible columns. The scorecard is
// required to be byte-identical across two independent replays of the same
// input, float sums included, and floating-point addition is not associative:
// the row order is part of the result. track_id is never an ordering key or an
// output, because it is a random UUID; creation_sequence is the track identity.

// ClusterSummary is the part of a stored observation the scorecard needs. The
// full record carries up to 512 points per cluster and runs to about 14 kB, so
// decoding every record through ListBySource costs gigabytes on a busy site.
// These fields are extracted in SQL instead.
type ClusterSummary struct {
	ObservationID  string
	FrameUnixNanos int64
	// X and Y are the position under the ClusterPosition asked for.
	X, Y        float64
	PointsCount int
}

// ListSourceIDs returns the distinct evidence sources in the database, sorted.
func (s *ObservationStore) ListSourceIDs() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT source_id FROM lidar_observations ORDER BY source_id`)
	if err != nil {
		return nil, fmt.Errorf("list observation sources: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan observation source: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate observation sources: %w", err)
	}
	return ids, nil
}

// ClusterPosition is which point of a stored cluster stands for its position.
// It must match the position model the run's tracker measured, or a
// prediction is compared against a point it never tracked.
type ClusterPosition int

const (
	// ClusterCentroid is the cluster medoid, the production measurement.
	ClusterCentroid ClusterPosition = iota
	// ClusterOBBCentre is the OBB centre when the cluster has one, its
	// centroid otherwise: D2's obb_centre_v1 candidate.
	ClusterOBBCentre
)

// ListClusterSummariesBySource returns every cluster of a source in frame
// order, then observation_id order within a frame.
func (s *ObservationStore) ListClusterSummariesBySource(sourceID string, position ClusterPosition) ([]ClusterSummary, error) {
	query := `
		SELECT observation_id
		     , frame_unix_nanos
		     , json_extract(record_json, '$.raw_cluster.CentroidX')
		     , json_extract(record_json, '$.raw_cluster.CentroidY')
		     , COALESCE(json_extract(record_json, '$.raw_cluster.PointsCount'), 0)
		  FROM lidar_observations
		 WHERE source_id = ?
		 ORDER BY frame_unix_nanos, observation_id`
	if position == ClusterOBBCentre {
		query = `
		SELECT observation_id
		     , frame_unix_nanos
		     , COALESCE(json_extract(record_json, '$.raw_cluster.OBB.CenterX'), json_extract(record_json, '$.raw_cluster.CentroidX'))
		     , COALESCE(json_extract(record_json, '$.raw_cluster.OBB.CenterY'), json_extract(record_json, '$.raw_cluster.CentroidY'))
		     , COALESCE(json_extract(record_json, '$.raw_cluster.PointsCount'), 0)
		  FROM lidar_observations
		 WHERE source_id = ?
		 ORDER BY frame_unix_nanos, observation_id`
	}
	rows, err := s.db.Query(query, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list cluster summaries for source %s: %w", sourceID, err)
	}
	defer rows.Close()
	out := []ClusterSummary{}
	for rows.Next() {
		var c ClusterSummary
		if err := rows.Scan(&c.ObservationID, &c.FrameUnixNanos, &c.X, &c.Y, &c.PointsCount); err != nil {
			return nil, fmt.Errorf("scan cluster summary: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cluster summaries: %w", err)
	}
	return out, nil
}

// ListFrameStateEstimatesBySource returns each online estimate of a source
// with the residual that produced it, in the same deterministic order as
// ListBySource: creation_sequence, then frame, then estimate_id. Covariance is
// not decoded; the scorecard does not use it. Refined stages are excluded for
// the reason ListBySource gives.
func (s *StateEstimateStore) ListFrameStateEstimatesBySource(sourceID string) ([]FrameStateEstimate, error) {
	rows, err := s.db.Query(`
		SELECT e.estimate_id, e.observation_id, e.source_id
		     , e.frame_unix_nanos, e.measurement_unix_nanos
		     , e.creation_sequence, e.x, e.y, e.vx, e.vy
		     , r.predicted_x, r.predicted_y, r.measurement_x, r.measurement_y
		     , r.innovation_x, r.innovation_y, r.nis, r.disposition, r.reason
		  FROM lidar_track_estimates e
		  JOIN lidar_track_residuals r ON r.estimate_id = e.estimate_id
		 WHERE e.source_id = ? AND e.stage = ?
		 ORDER BY e.creation_sequence, e.frame_unix_nanos, e.estimate_id`, sourceID, EstimateStageOnline)
	if err != nil {
		return nil, fmt.Errorf("list frame state estimates for source %s: %w", sourceID, err)
	}
	defer rows.Close()
	out := []FrameStateEstimate{}
	for rows.Next() {
		var f FrameStateEstimate
		e, r := &f.Estimate, &f.Residual
		if err := rows.Scan(
			&e.EstimateID, &e.ObservationID, &e.SourceID,
			&e.FrameUnixNanos, &e.MeasurementUnixNanos,
			&e.CreationSequence, &e.X, &e.Y, &e.VX, &e.VY,
			&r.PredictedX, &r.PredictedY, &r.MeasurementX, &r.MeasurementY,
			&r.InnovationX, &r.InnovationY, &r.NIS, &r.Disposition, &r.Reason,
		); err != nil {
			return nil, fmt.Errorf("scan frame state estimate: %w", err)
		}
		r.EstimateID, r.ObservationID = e.EstimateID, e.ObservationID
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate frame state estimates: %w", err)
	}
	return out, nil
}
