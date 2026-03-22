package sqlite

import (
	"database/sql"
	"fmt"
)

// LidarLabel represents a manual label applied to a track for training or validation.
type LidarLabel struct {
	LabelID          string   `json:"label_id"`
	TrackID          string   `json:"track_id"`
	ClassLabel       string   `json:"class_label"`
	StartTimestampNs int64    `json:"start_timestamp_ns"`
	EndTimestampNs   *int64   `json:"end_timestamp_ns,omitempty"`
	Confidence       *float32 `json:"confidence,omitempty"`
	CreatedBy        *string  `json:"created_by,omitempty"`
	CreatedAtNs      int64    `json:"created_at_ns"`
	UpdatedAtNs      *int64   `json:"updated_at_ns,omitempty"`
	Notes            *string  `json:"notes,omitempty"`
	ReplayCaseID     *string  `json:"replay_case_id,omitempty"`
	SourceFile       *string  `json:"source_file,omitempty"`
}

// LabelFilter describes optional list filters for lidar labels.
// Timestamp fields stay as strings so HTTP callers can preserve existing
// SQLite coercion semantics for query parameters.
type LabelFilter struct {
	TrackID          string
	ClassLabel       string
	StartTimestampNs string
	EndTimestampNs   string
	Limit            int
}

// LabelStore provides CRUD and export access for lidar_track_annotations.
type LabelStore struct {
	db DBClient
}

// NewLabelStore creates a new LabelStore.
func NewLabelStore(db DBClient) *LabelStore {
	return &LabelStore{db: db}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanLidarLabel(scanner rowScanner) (*LidarLabel, error) {
	var label LidarLabel
	if err := scanner.Scan(
		&label.LabelID,
		&label.TrackID,
		&label.ClassLabel,
		&label.StartTimestampNs,
		&label.EndTimestampNs,
		&label.Confidence,
		&label.CreatedBy,
		&label.CreatedAtNs,
		&label.UpdatedAtNs,
		&label.Notes,
		&label.ReplayCaseID,
		&label.SourceFile,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &label, nil
}

func (s *LabelStore) labelSelectBase() string {
	return `SELECT label_id, track_id, class_label, start_timestamp_ns,
	               end_timestamp_ns, confidence, created_by, created_at_ns,
	               updated_at_ns, notes, replay_case_id, source_file
	        FROM lidar_track_annotations`
}

// ListLabels returns labels filtered by the supplied fields, ordered newest-first.
func (s *LabelStore) ListLabels(filter LabelFilter) ([]LidarLabel, error) {
	query := s.labelSelectBase() + " WHERE 1=1"
	args := make([]any, 0, 4)

	if filter.TrackID != "" {
		query += " AND track_id = ?"
		args = append(args, filter.TrackID)
	}
	if filter.ClassLabel != "" {
		query += " AND class_label = ?"
		args = append(args, filter.ClassLabel)
	}
	if filter.StartTimestampNs != "" {
		query += " AND start_timestamp_ns >= ?"
		args = append(args, filter.StartTimestampNs)
	}
	if filter.EndTimestampNs != "" {
		query += " AND (end_timestamp_ns IS NULL OR end_timestamp_ns <= ?)"
		args = append(args, filter.EndTimestampNs)
	}

	query += " ORDER BY start_timestamp_ns DESC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	defer rows.Close()

	labels := make([]LidarLabel, 0)
	for rows.Next() {
		label, err := scanLidarLabel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan label row: %w", err)
		}
		labels = append(labels, *label)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate labels: %w", err)
	}

	return labels, nil
}

// CreateLabel inserts a new manual label.
func (s *LabelStore) CreateLabel(label *LidarLabel) error {
	query := `INSERT INTO lidar_track_annotations (
		label_id, track_id, class_label, start_timestamp_ns, end_timestamp_ns,
		confidence, created_by, created_at_ns, updated_at_ns, notes, replay_case_id, source_file
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := s.db.Exec(query,
		label.LabelID,
		label.TrackID,
		label.ClassLabel,
		label.StartTimestampNs,
		label.EndTimestampNs,
		label.Confidence,
		label.CreatedBy,
		label.CreatedAtNs,
		label.UpdatedAtNs,
		label.Notes,
		label.ReplayCaseID,
		label.SourceFile,
	); err != nil {
		return fmt.Errorf("create label: %w", err)
	}

	return nil
}

// GetLabel returns a label by ID.
func (s *LabelStore) GetLabel(labelID string) (*LidarLabel, error) {
	label, err := scanLidarLabel(s.db.QueryRow(s.labelSelectBase()+" WHERE label_id = ?", labelID))
	if err != nil {
		if err == ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get label: %w", err)
	}
	return label, nil
}

// UpdateLabel updates explicitly provided fields on a label.
func (s *LabelStore) UpdateLabel(labelID string, updates *LidarLabel) error {
	query := "UPDATE lidar_track_annotations SET updated_at_ns = ?"
	args := []any{updates.UpdatedAtNs}

	if updates.ClassLabel != "" {
		query += ", class_label = ?"
		args = append(args, updates.ClassLabel)
	}
	if updates.EndTimestampNs != nil {
		query += ", end_timestamp_ns = ?"
		args = append(args, updates.EndTimestampNs)
	}
	if updates.Confidence != nil {
		query += ", confidence = ?"
		args = append(args, updates.Confidence)
	}
	if updates.Notes != nil {
		query += ", notes = ?"
		args = append(args, updates.Notes)
	}
	if updates.ReplayCaseID != nil {
		query += ", replay_case_id = ?"
		args = append(args, updates.ReplayCaseID)
	}
	if updates.SourceFile != nil {
		query += ", source_file = ?"
		args = append(args, updates.SourceFile)
	}

	query += " WHERE label_id = ?"
	args = append(args, labelID)

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update label: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update label rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteLabel removes a label by ID.
func (s *LabelStore) DeleteLabel(labelID string) error {
	result, err := s.db.Exec("DELETE FROM lidar_track_annotations WHERE label_id = ?", labelID)
	if err != nil {
		return fmt.Errorf("delete label: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete label rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// ExportLabels returns all labels ordered oldest-first for stable export output.
func (s *LabelStore) ExportLabels() ([]LidarLabel, error) {
	rows, err := s.db.Query(s.labelSelectBase() + " ORDER BY start_timestamp_ns ASC")
	if err != nil {
		return nil, fmt.Errorf("export labels: %w", err)
	}
	defer rows.Close()

	labels := make([]LidarLabel, 0)
	for rows.Next() {
		label, err := scanLidarLabel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan export label row: %w", err)
		}
		labels = append(labels, *label)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate export labels: %w", err)
	}

	return labels, nil
}
