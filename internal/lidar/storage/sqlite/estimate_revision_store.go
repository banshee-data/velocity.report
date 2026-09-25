package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Estimate stages, as persisted in lidar_track_estimates.stage (plan §10.1).
// An online estimate is what the filter believed at the time; fixed_lag and
// final are retrospective revisions of it and always carry a revision record.
const (
	EstimateStageOnline   = "online"
	EstimateStageFixedLag = "fixed_lag"
	EstimateStageFinal    = "final"
)

// EstimateRevision is the audit record of one retrospectively refined
// estimate: which online estimate it revised, how far and how it moved, with
// which look-ahead, and the evidence that justified it. The previous state is
// copied, not only referenced: online estimates are retained for 7 days and
// refined ones indefinitely (plan §11.1), and an audit record that points at
// a pruned row audits nothing.
type EstimateRevision struct {
	EstimateID          string
	RevisesEstimateID   string
	SmootherID          string
	Lag                 string
	LookaheadSteps      int
	LookaheadSecs       float64
	ReleasedAtUnixNanos int64
	ReleaseReason       string
	ChainEndReason      string
	Flags               []string

	PreviousX, PreviousY, PreviousVX, PreviousVY float32
	RevisionPositionMetres                       float64
	RevisionVelocityMps                          float64

	// Under fixed assignment the evidence is exactly the track's observed
	// steps in (the estimate's frame, EvidenceLastFrameUnixNanos]; the bounds
	// and count pin it, and the strongest entry is kept by identity for the
	// abnormal-motion audit. Zero values when EvidenceCount is zero.
	EvidenceCount                  int
	EvidenceFirstFrameUnixNanos    int64
	EvidenceLastFrameUnixNanos     int64
	StrongestEvidenceObservationID string
	StrongestEvidenceNIS           float32
}

// RevisedStateEstimate is a refined estimate, the residual row every estimate
// carries (the association evidence it was computed under), and its revision.
type RevisedStateEstimate struct {
	Estimate TrackEstimate
	Residual TrackResidual
	Revision EstimateRevision
}

// InsertRevised writes a batch of refined estimates in one transaction: all
// of it, or none. An online estimate is never written or replaced here.
func (s *StateEstimateStore) InsertRevised(batch []RevisedStateEstimate) error {
	return InsertRevisedStateEstimates(s.db, batch)
}

// InsertRevisedStateEstimates is InsertRevised for a caller holding the
// database. A replay calls it once per frame with what its smoothers released.
func InsertRevisedStateEstimates(db DBClient, batch []RevisedStateEstimate) error {
	if len(batch) == 0 {
		return nil
	}
	for _, item := range batch {
		if err := validateRevisedStateEstimate(item); err != nil {
			return err
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin revised estimates: %w", err)
	}
	insertedAt := time.Now().UnixNano()
	for _, item := range batch {
		if err := refuseForeignEstimate(tx, item.Estimate); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := insertStateEstimate(tx, item.Estimate, item.Residual, insertedAt); err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.Exec(estimateRevisionInsertSQL, estimateRevisionInsertArgs(item.Revision, insertedAt)...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert estimate revision %s: %w", item.Revision.EstimateID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit revised estimates: %w", err)
	}
	return nil
}

// refuseForeignEstimate stops a refined write from replacing a stored row of
// another version under the same estimate_id. The insert is INSERT OR REPLACE
// so that a replay can be persisted twice, which would otherwise let a
// colliding ID swap out an online estimate, or another stage's, without trace.
// The check is on the stored row, not the ID's shape, so it holds whatever
// scheme produced the ID.
func refuseForeignEstimate(tx *sql.Tx, e TrackEstimate) error {
	var estimatorID, observationModelID, paramHash, stage string
	err := tx.QueryRow(`SELECT estimator_id, observation_model_id, param_hash, stage
		  FROM lidar_track_estimates WHERE estimate_id = ?`, e.EstimateID).
		Scan(&estimatorID, &observationModelID, &paramHash, &stage)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check existing estimate %s: %w", e.EstimateID, err)
	}
	if stage != e.Stage || estimatorID != e.EstimatorID || observationModelID != e.ObservationModelID || paramHash != e.ParamHash {
		return fmt.Errorf("revised estimate %s would replace a stored %s estimate of another version (%s/%s/%s)",
			e.EstimateID, stage, estimatorID, observationModelID, paramHash)
	}
	return nil
}

// validateRevisedStateEstimate refuses a stage that is not a revision, or a
// revision that does not describe the estimate it is attached to. That a
// write cannot replace a stored row of another version, the online estimate
// included, is checked inside the transaction by refuseForeignEstimate.
func validateRevisedStateEstimate(item RevisedStateEstimate) error {
	e, r := item.Estimate, item.Revision
	if e.Stage != EstimateStageFixedLag && e.Stage != EstimateStageFinal {
		return fmt.Errorf("revised estimate %s has stage %q; only fixed_lag and final are revisions", e.EstimateID, e.Stage)
	}
	if r.EstimateID != e.EstimateID {
		return fmt.Errorf("revision %q does not describe estimate %q", r.EstimateID, e.EstimateID)
	}
	if r.RevisesEstimateID == "" || r.RevisesEstimateID == e.EstimateID {
		return fmt.Errorf("revised estimate %s must name a different online estimate it revises", e.EstimateID)
	}
	if r.SmootherID == "" || r.Lag == "" || r.ReleaseReason == "" {
		return fmt.Errorf("revision %s requires smoother, lag and release reason", e.EstimateID)
	}
	if r.EvidenceCount < 0 || r.LookaheadSteps < 0 {
		return fmt.Errorf("revision %s has negative counts", e.EstimateID)
	}
	return nil
}

const estimateRevisionInsertSQL = `INSERT OR REPLACE INTO lidar_track_estimate_revisions
		(estimate_id, revises_estimate_id, smoother_id, lag, lookahead_steps, lookahead_secs, released_at_unix_nanos,
		 release_reason, chain_end_reason, flags, previous_x, previous_y, previous_vx, previous_vy,
		 revision_position_m, revision_velocity_mps, evidence_count, evidence_first_frame_unix_nanos,
		 evidence_last_frame_unix_nanos, strongest_evidence_observation_id, strongest_evidence_nis, inserted_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

func estimateRevisionInsertArgs(r EstimateRevision, insertedAtNanos int64) []any {
	return []any{
		r.EstimateID, r.RevisesEstimateID, r.SmootherID, r.Lag, r.LookaheadSteps, r.LookaheadSecs, r.ReleasedAtUnixNanos,
		r.ReleaseReason, r.ChainEndReason, strings.Join(r.Flags, ","), r.PreviousX, r.PreviousY, r.PreviousVX, r.PreviousVY,
		r.RevisionPositionMetres, r.RevisionVelocityMps, r.EvidenceCount, r.EvidenceFirstFrameUnixNanos,
		r.EvidenceLastFrameUnixNanos, r.StrongestEvidenceObservationID, r.StrongestEvidenceNIS, insertedAtNanos,
	}
}

// EstimateVersionKey selects one versioned set of estimates: the five fields
// the per-frame evaluator also keys arms by.
type EstimateVersionKey struct {
	SourceID           string
	EstimatorID        string
	ObservationModelID string
	ParamHash          string
	Stage              string
}

// ListRevisedEstimates returns one refined version's estimates with their
// residual rows and revision records, in creation-sequence and frame order:
// each item as InsertRevised wrote it. It is the read path for the refined
// stages; the older source-wide readers return online rows only, which is all
// they were written for.
func (s *StateEstimateStore) ListRevisedEstimates(key EstimateVersionKey) ([]RevisedStateEstimate, error) {
	rows, err := s.db.Query(`
		SELECT e.estimate_id, e.track_id, e.observation_id, e.source_id, e.calibration_id
		     , e.frame_unix_nanos, e.measurement_unix_nanos, e.estimator_id
		     , e.observation_model_id, e.param_hash, e.stage, e.measurement_source
		     , e.creation_sequence, e.x, e.y, e.vx, e.vy, e.covariance_json
		     , r.observation_id, r.predicted_x, r.predicted_y, r.measurement_x, r.measurement_y
		     , r.innovation_x, r.innovation_y, r.nis
		     , r.geometry_cov_xx, r.geometry_cov_xy, r.geometry_cov_yy
		     , r.disposition, r.reason
		     , v.revises_estimate_id, v.smoother_id, v.lag, v.lookahead_steps, v.lookahead_secs
		     , v.released_at_unix_nanos, v.release_reason, v.chain_end_reason, v.flags
		     , v.previous_x, v.previous_y, v.previous_vx, v.previous_vy
		     , v.revision_position_m, v.revision_velocity_mps, v.evidence_count
		     , v.evidence_first_frame_unix_nanos, v.evidence_last_frame_unix_nanos
		     , v.strongest_evidence_observation_id, v.strongest_evidence_nis
		  FROM lidar_track_estimates e
		  JOIN lidar_track_residuals r ON r.estimate_id = e.estimate_id
		  JOIN lidar_track_estimate_revisions v ON v.estimate_id = e.estimate_id
		 WHERE e.source_id = ? AND e.estimator_id = ? AND e.observation_model_id = ?
		   AND e.param_hash = ? AND e.stage = ?
		 ORDER BY e.creation_sequence, e.frame_unix_nanos, e.estimate_id`,
		key.SourceID, key.EstimatorID, key.ObservationModelID, key.ParamHash, key.Stage)
	if err != nil {
		return nil, fmt.Errorf("list revised estimates %s/%s/%s: %w", key.SourceID, key.EstimatorID, key.Stage, err)
	}
	defer rows.Close()
	out := []RevisedStateEstimate{}
	for rows.Next() {
		var item RevisedStateEstimate
		e, res, r := &item.Estimate, &item.Residual, &item.Revision
		var covariance []byte
		var flags string
		if err := rows.Scan(
			&e.EstimateID, &e.TrackID, &e.ObservationID, &e.SourceID, &e.CalibrationID,
			&e.FrameUnixNanos, &e.MeasurementUnixNanos, &e.EstimatorID,
			&e.ObservationModelID, &e.ParamHash, &e.Stage, &e.MeasurementSource,
			&e.CreationSequence, &e.X, &e.Y, &e.VX, &e.VY, &covariance,
			&res.ObservationID, &res.PredictedX, &res.PredictedY, &res.MeasurementX, &res.MeasurementY,
			&res.InnovationX, &res.InnovationY, &res.NIS,
			&res.GeometryCovXX, &res.GeometryCovXY, &res.GeometryCovYY,
			&res.Disposition, &res.Reason,
			&r.RevisesEstimateID, &r.SmootherID, &r.Lag, &r.LookaheadSteps, &r.LookaheadSecs,
			&r.ReleasedAtUnixNanos, &r.ReleaseReason, &r.ChainEndReason, &flags,
			&r.PreviousX, &r.PreviousY, &r.PreviousVX, &r.PreviousVY,
			&r.RevisionPositionMetres, &r.RevisionVelocityMps, &r.EvidenceCount,
			&r.EvidenceFirstFrameUnixNanos, &r.EvidenceLastFrameUnixNanos,
			&r.StrongestEvidenceObservationID, &r.StrongestEvidenceNIS,
		); err != nil {
			return nil, fmt.Errorf("scan revised estimate: %w", err)
		}
		if err := json.Unmarshal(covariance, &e.Covariance); err != nil {
			return nil, fmt.Errorf("unmarshal estimate covariance %s: %w", e.EstimateID, err)
		}
		res.EstimateID, r.EstimateID = e.EstimateID, e.EstimateID
		if flags != "" {
			r.Flags = strings.Split(flags, ",")
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate revised estimates: %w", err)
	}
	return out, nil
}
