package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Evaluation represents a persisted ground truth evaluation result comparing
// a candidate analysis run against a reference run for a given scene.
type Evaluation struct {
	EvaluationID        string          `json:"evaluation_id"`
	SceneID             string          `json:"scene_id"`
	ReferenceRunID      string          `json:"reference_run_id"`
	CandidateRunID      string          `json:"candidate_run_id"`
	DetectionRate       float64         `json:"detection_rate"`
	Fragmentation       float64         `json:"fragmentation"`
	FalsePositiveRate   float64         `json:"false_positive_rate"`
	VelocityCoverage    float64         `json:"velocity_coverage"`
	QualityPremium      float64         `json:"quality_premium"`
	TruncationRate      float64         `json:"truncation_rate"`
	VelocityNoiseRate   float64         `json:"velocity_noise_rate"`
	StoppedRecoveryRate float64         `json:"stopped_recovery_rate"`
	CompositeScore      float64         `json:"composite_score"`
	MatchedCount        int             `json:"matched_count"`
	ReferenceCount      int             `json:"reference_count"`
	CandidateCount      int             `json:"candidate_count"`
	ParamsJSON          json.RawMessage `json:"params_json,omitempty"`
	CreatedAt           int64           `json:"created_at"`
}

// EvaluationStore provides persistence for ground truth evaluation results.
type EvaluationStore struct {
	db *sql.DB
}

// NewEvaluationStore creates a new EvaluationStore.
func NewEvaluationStore(db *sql.DB) *EvaluationStore {
	return &EvaluationStore{db: db}
}

// Insert persists a new evaluation result. If EvaluationID is empty, a UUID is generated.
func (s *EvaluationStore) Insert(eval *Evaluation) error {
	if eval.EvaluationID == "" {
		eval.EvaluationID = uuid.New().String()
	}
	if eval.CreatedAt == 0 {
		eval.CreatedAt = time.Now().UnixNano()
	}

	var paramsStr interface{}
	if len(eval.ParamsJSON) > 0 {
		paramsStr = string(eval.ParamsJSON)
	}

	return retryOnBusy(func() error {
		_, err := s.db.Exec(`
			INSERT INTO lidar_evaluations (
				evaluation_id, scene_id, reference_run_id, candidate_run_id,
				detection_rate, fragmentation, false_positive_rate, velocity_coverage,
				quality_premium, truncation_rate, velocity_noise_rate, stopped_recovery_rate,
				composite_score, matched_count, reference_count, candidate_count,
				params_json, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			eval.EvaluationID, eval.SceneID, eval.ReferenceRunID, eval.CandidateRunID,
			eval.DetectionRate, eval.Fragmentation, eval.FalsePositiveRate, eval.VelocityCoverage,
			eval.QualityPremium, eval.TruncationRate, eval.VelocityNoiseRate, eval.StoppedRecoveryRate,
			eval.CompositeScore, eval.MatchedCount, eval.ReferenceCount, eval.CandidateCount,
			paramsStr, eval.CreatedAt,
		)
		return err
	})
}

// ListByScene returns all evaluations for a given scene, ordered by creation time descending.
func (s *EvaluationStore) ListByScene(sceneID string) ([]*Evaluation, error) {
	rows, err := s.db.Query(`
		SELECT evaluation_id, scene_id, reference_run_id, candidate_run_id,
		       detection_rate, fragmentation, false_positive_rate, velocity_coverage,
		       quality_premium, truncation_rate, velocity_noise_rate, stopped_recovery_rate,
		       composite_score, matched_count, reference_count, candidate_count,
		       params_json, created_at
		FROM lidar_evaluations
		WHERE scene_id = ?
		ORDER BY created_at DESC`, sceneID)
	if err != nil {
		return nil, fmt.Errorf("query evaluations: %w", err)
	}
	defer rows.Close()

	var evals []*Evaluation
	for rows.Next() {
		e, err := scanEvaluation(rows)
		if err != nil {
			return nil, err
		}
		evals = append(evals, e)
	}
	return evals, rows.Err()
}

// Get returns a single evaluation by ID.
func (s *EvaluationStore) Get(evaluationID string) (*Evaluation, error) {
	row := s.db.QueryRow(`
		SELECT evaluation_id, scene_id, reference_run_id, candidate_run_id,
		       detection_rate, fragmentation, false_positive_rate, velocity_coverage,
		       quality_premium, truncation_rate, velocity_noise_rate, stopped_recovery_rate,
		       composite_score, matched_count, reference_count, candidate_count,
		       params_json, created_at
		FROM lidar_evaluations
		WHERE evaluation_id = ?`, evaluationID)

	var e Evaluation
	var paramsStr sql.NullString
	err := row.Scan(
		&e.EvaluationID, &e.SceneID, &e.ReferenceRunID, &e.CandidateRunID,
		&e.DetectionRate, &e.Fragmentation, &e.FalsePositiveRate, &e.VelocityCoverage,
		&e.QualityPremium, &e.TruncationRate, &e.VelocityNoiseRate, &e.StoppedRecoveryRate,
		&e.CompositeScore, &e.MatchedCount, &e.ReferenceCount, &e.CandidateCount,
		&paramsStr, &e.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("evaluation %s not found", evaluationID)
		}
		return nil, fmt.Errorf("scan evaluation: %w", err)
	}
	if paramsStr.Valid {
		e.ParamsJSON = json.RawMessage(paramsStr.String)
	}
	return &e, nil
}

// Delete removes an evaluation by ID.
func (s *EvaluationStore) Delete(evaluationID string) error {
	return retryOnBusy(func() error {
		result, err := s.db.Exec(`DELETE FROM lidar_evaluations WHERE evaluation_id = ?`, evaluationID)
		if err != nil {
			return fmt.Errorf("delete evaluation: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		if affected == 0 {
			return fmt.Errorf("evaluation %s not found", evaluationID)
		}
		return nil
	})
}

// scanEvaluation scans an evaluation row from a sql.Rows cursor.
func scanEvaluation(rows *sql.Rows) (*Evaluation, error) {
	var e Evaluation
	var paramsStr sql.NullString
	err := rows.Scan(
		&e.EvaluationID, &e.SceneID, &e.ReferenceRunID, &e.CandidateRunID,
		&e.DetectionRate, &e.Fragmentation, &e.FalsePositiveRate, &e.VelocityCoverage,
		&e.QualityPremium, &e.TruncationRate, &e.VelocityNoiseRate, &e.StoppedRecoveryRate,
		&e.CompositeScore, &e.MatchedCount, &e.ReferenceCount, &e.CandidateCount,
		&paramsStr, &e.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan evaluation row: %w", err)
	}
	if paramsStr.Valid {
		e.ParamsJSON = json.RawMessage(paramsStr.String)
	}
	return &e, nil
}
