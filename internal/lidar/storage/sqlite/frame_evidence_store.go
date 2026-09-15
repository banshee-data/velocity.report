package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
)

// FrameEvidenceStore commits the immutable L4 evidence and all L5 records
// derived from one frame together. It intentionally changes transaction
// mechanics only: record construction, JSON encoding, IDs, and values remain
// owned by the existing stores.
type FrameEvidenceStore struct {
	db         DBClient
	commit     func(*sql.Tx) error
	prepare    sync.Once
	prepareErr error

	observationStmt *sql.Stmt
	estimateStmt    *sql.Stmt
	residualStmt    *sql.Stmt
}

func NewFrameEvidenceStore(db DBClient) *FrameEvidenceStore {
	return &FrameEvidenceStore{db: db, commit: func(tx *sql.Tx) error { return tx.Commit() }}
}

type statementPreparer interface {
	Prepare(query string) (*sql.Stmt, error)
}

// prepareStatements creates the three SQLite statements once for the lifetime
// of the store. Each frame then binds those statements to its own transaction.
// Preparing inside InsertFrame would perform three prepare/close cycles for
// every frame, precisely the overhead the frame batch is meant to avoid.
func (s *FrameEvidenceStore) prepareStatements() error {
	s.prepare.Do(func() {
		preparer, ok := s.db.(statementPreparer)
		if !ok {
			s.prepareErr = fmt.Errorf("prepare frame evidence statements: database does not support prepared statements")
			return
		}
		var err error
		s.observationStmt, err = preparer.Prepare(observationInsertSQL)
		if err != nil {
			s.prepareErr = fmt.Errorf("prepare observation insert: %w", err)
			return
		}
		s.estimateStmt, err = preparer.Prepare(stateEstimateInsertSQL)
		if err != nil {
			_ = s.observationStmt.Close()
			s.observationStmt = nil
			s.prepareErr = fmt.Errorf("prepare track estimate insert: %w", err)
			return
		}
		s.residualStmt, err = preparer.Prepare(stateResidualInsertSQL)
		if err != nil {
			_ = s.estimateStmt.Close()
			_ = s.observationStmt.Close()
			s.estimateStmt = nil
			s.observationStmt = nil
			s.prepareErr = fmt.Errorf("prepare track residual insert: %w", err)
		}
	})
	return s.prepareErr
}

// Close releases the store-owned prepared statements before its database is
// closed. It is safe to call after no frame callbacks remain.
func (s *FrameEvidenceStore) Close() error {
	var result error
	for _, stmt := range []*sql.Stmt{s.residualStmt, s.estimateStmt, s.observationStmt} {
		if stmt != nil {
			if err := stmt.Close(); err != nil && result == nil {
				result = err
			}
		}
	}
	return result
}

// InsertFrame writes a complete frame in one transaction. A failed insert or
// commit leaves none of that frame's observation, estimate, or residual rows
// behind. Duplicate immutable observation IDs retain the existing fail-closed
// behaviour.
func (s *FrameEvidenceStore) InsertFrame(observations []l4bobserve.DetectionObservation, estimates []FrameStateEstimate) error {
	// Frames without retained evidence do not need a transaction. In a static
	// corpus they are common, and committing them would create pure SQLite
	// overhead without changing the corpus or its failure semantics.
	if len(observations) == 0 && len(estimates) == 0 {
		return nil
	}
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

	if err := s.prepareStatements(); err != nil {
		return err
	}
	observationStmt := tx.Stmt(s.observationStmt)
	defer observationStmt.Close()

	// Bind the store-owned statements to this bounded transaction. They retain
	// INSERT OR REPLACE semantics for a versioned estimate key.
	estimateStmt := tx.Stmt(s.estimateStmt)
	defer estimateStmt.Close()
	residualStmt := tx.Stmt(s.residualStmt)
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
