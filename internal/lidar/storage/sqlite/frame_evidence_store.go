package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
)

// FrameEvidenceStore commits the immutable L4 evidence and all L5 records
// derived from one frame together. It intentionally changes transaction
// mechanics only: record construction, JSON encoding, IDs, and values remain
// owned by the existing stores.
type FrameEvidenceStore struct {
	db     DBClient
	commit func(*sql.Tx) error
}

func NewFrameEvidenceStore(db DBClient) *FrameEvidenceStore {
	return &FrameEvidenceStore{db: db, commit: func(tx *sql.Tx) error { return tx.Commit() }}
}

// InsertFrame writes a complete frame in one transaction. A failed insert or
// commit leaves none of that frame's observation, estimate, or residual rows
// behind. Duplicate immutable observation IDs retain the existing fail-closed
// behaviour.
func (s *FrameEvidenceStore) InsertFrame(observations []l4bobserve.DetectionObservation, estimates []FrameStateEstimate) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin frame evidence transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	observationStmt, err := tx.Prepare(observationInsertSQL)
	if err != nil {
		return fmt.Errorf("prepare observation insert: %w", err)
	}
	defer observationStmt.Close()

	// The derived statements are prepared once per bounded frame transaction.
	// They retain INSERT OR REPLACE semantics for a versioned estimate key.
	estimateStmt, err := tx.Prepare(stateEstimateInsertSQL)
	if err != nil {
		return fmt.Errorf("prepare track estimate insert: %w", err)
	}
	defer estimateStmt.Close()
	residualStmt, err := tx.Prepare(stateResidualInsertSQL)
	if err != nil {
		return fmt.Errorf("prepare track residual insert: %w", err)
	}
	defer residualStmt.Close()

	insertedAtNanos := time.Now().UnixNano()
	for _, observation := range observations {
		record, payload, err := marshalObservation(observation)
		if err != nil {
			return err
		}
		if _, err := observationStmt.Exec(observationInsertArgs(record, payload, insertedAtNanos)...); err != nil {
			if isUniqueConstraint(err) {
				return fmt.Errorf("%w: %s", ErrObservationExists, record.ObservationID)
			}
			return fmt.Errorf("insert observation %s: %w", record.ObservationID, err)
		}
	}
	for _, pair := range estimates {
		if err := insertStateEstimateStatements(estimateStmt, residualStmt, pair.Estimate, pair.Residual, insertedAtNanos); err != nil {
			return err
		}
	}
	if err := s.commit(tx); err != nil {
		return fmt.Errorf("commit frame evidence transaction: %w", err)
	}
	committed = true
	return nil
}

func insertStateEstimateStatements(estimateStmt, residualStmt statementExecutor, estimate TrackEstimate, residual TrackResidual, insertedAtNanos int64) error {
	if err := validateStateEstimate(estimate, residual); err != nil {
		return err
	}
	covariance, err := json.Marshal(estimate.Covariance)
	if err != nil {
		return fmt.Errorf("marshal estimate covariance: %w", err)
	}
	if _, err := estimateStmt.Exec(stateEstimateInsertArgs(estimate, covariance, insertedAtNanos)...); err != nil {
		return fmt.Errorf("insert track estimate %s: %w", estimate.EstimateID, err)
	}
	if _, err := residualStmt.Exec(stateResidualInsertArgs(residual, insertedAtNanos)...); err != nil {
		return fmt.Errorf("insert track residual %s: %w", residual.EstimateID, err)
	}
	return nil
}

type statementExecutor interface {
	Exec(args ...any) (sql.Result, error)
}
