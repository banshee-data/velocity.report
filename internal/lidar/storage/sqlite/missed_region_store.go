package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MissedRegion represents an area where an object should have been tracked
// but was not detected by the tracker. Used for ground truth evaluation.
type MissedRegion struct {
	RegionID      string  `json:"region_id"`
	RunID         string  `json:"run_id"`
	CenterX       float64 `json:"center_x"`
	CenterY       float64 `json:"center_y"`
	RadiusM       float64 `json:"radius_m"`
	TimeStartNs   int64   `json:"time_start_ns"`
	TimeEndNs     int64   `json:"time_end_ns"`
	ExpectedLabel string  `json:"expected_label"`
	LabelerID     string  `json:"labeler_id,omitempty"`
	LabeledAt     *int64  `json:"labeled_at,omitempty"`
	Notes         string  `json:"notes,omitempty"`
}

// MissedRegionStore provides persistence for missed region annotations.
type MissedRegionStore struct {
	db *sql.DB
}

// NewMissedRegionStore creates a new MissedRegionStore.
func NewMissedRegionStore(db *sql.DB) *MissedRegionStore {
	return &MissedRegionStore{db: db}
}

// Insert creates a new missed region in the database.
// If region.RegionID is empty, a new UUID is generated.
func (s *MissedRegionStore) Insert(region *MissedRegion) error {
	if region.RegionID == "" {
		region.RegionID = uuid.New().String()
	}
	if region.LabeledAt == nil {
		now := time.Now().UnixNano()
		region.LabeledAt = &now
	}
	if region.RadiusM <= 0 {
		region.RadiusM = 3.0
	}
	if region.ExpectedLabel == "" {
		region.ExpectedLabel = "car"
	}

	query := `
		INSERT INTO lidar_missed_regions (
			region_id, run_id, center_x, center_y, radius_m,
			time_start_ns, time_end_ns, expected_label,
			labeler_id, labeled_at, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		region.RegionID,
		region.RunID,
		region.CenterX,
		region.CenterY,
		region.RadiusM,
		region.TimeStartNs,
		region.TimeEndNs,
		region.ExpectedLabel,
		nullString(region.LabelerID),
		nullInt64(region.LabeledAt),
		nullString(region.Notes),
	)
	if err != nil {
		return fmt.Errorf("insert missed region: %w", err)
	}
	return nil
}

// ListByRun returns all missed regions for a given run.
func (s *MissedRegionStore) ListByRun(runID string) ([]*MissedRegion, error) {
	query := `
		SELECT region_id, run_id, center_x, center_y, radius_m,
		       time_start_ns, time_end_ns, expected_label,
		       labeler_id, labeled_at, notes
		FROM lidar_missed_regions
		WHERE run_id = ?
		ORDER BY time_start_ns
	`

	rows, err := s.db.Query(query, runID)
	if err != nil {
		return nil, fmt.Errorf("list missed regions: %w", err)
	}
	defer rows.Close()

	var regions []*MissedRegion
	for rows.Next() {
		r := &MissedRegion{}
		var labelerID, notes sql.NullString
		var labeledAt sql.NullInt64

		err := rows.Scan(
			&r.RegionID, &r.RunID, &r.CenterX, &r.CenterY, &r.RadiusM,
			&r.TimeStartNs, &r.TimeEndNs, &r.ExpectedLabel,
			&labelerID, &labeledAt, &notes,
		)
		if err != nil {
			return nil, fmt.Errorf("scan missed region: %w", err)
		}

		if labelerID.Valid {
			r.LabelerID = labelerID.String
		}
		if labeledAt.Valid {
			r.LabeledAt = &labeledAt.Int64
		}
		if notes.Valid {
			r.Notes = notes.String
		}

		regions = append(regions, r)
	}

	return regions, rows.Err()
}

// Delete removes a missed region by ID.
func (s *MissedRegionStore) Delete(regionID string) error {
	result, err := s.db.Exec("DELETE FROM lidar_missed_regions WHERE region_id = ?", regionID)
	if err != nil {
		return fmt.Errorf("delete missed region: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete missed region rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
