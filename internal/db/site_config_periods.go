package db

import (
	"database/sql"
	"fmt"
	"math"
	"time"
)

const maxUnixTime = 32503680000.0 // 3000-01-01T00:00:00Z

// SiteConfigPeriod represents a time-based configuration period for a site.
type SiteConfigPeriod struct {
	ID                 int       `json:"id"`
	SiteID             int       `json:"site_id"`
	EffectiveStartUnix float64   `json:"effective_start_unix"`
	EffectiveEndUnix   *float64  `json:"effective_end_unix"`
	IsActive           bool      `json:"is_active"`
	Notes              *string   `json:"notes"`
	CosineErrorAngle   float64   `json:"cosine_error_angle"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// ListSiteConfigPeriods returns all site config periods, optionally filtered by site.
func (db *DB) ListSiteConfigPeriods(siteID *int) ([]SiteConfigPeriod, error) {
	query := `
		SELECT
			id, site_id, effective_start_unix, effective_end_unix, is_active,
			notes, cosine_error_angle, created_at, updated_at
		FROM site_config_periods
	`
	var args []interface{}
	if siteID != nil {
		query += " WHERE site_id = ?"
		args = append(args, *siteID)
	}
	query += " ORDER BY effective_start_unix ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query site config periods: %w", err)
	}
	defer rows.Close()

	var periods []SiteConfigPeriod
	for rows.Next() {
		var period SiteConfigPeriod
		var endUnix sql.NullFloat64
		var isActive int
		var createdAtUnix, updatedAtUnix float64
		if err := rows.Scan(
			&period.ID,
			&period.SiteID,
			&period.EffectiveStartUnix,
			&endUnix,
			&isActive,
			&period.Notes,
			&period.CosineErrorAngle,
			&createdAtUnix,
			&updatedAtUnix,
		); err != nil {
			return nil, fmt.Errorf("failed to scan site config period: %w", err)
		}
		if endUnix.Valid {
			value := endUnix.Float64
			period.EffectiveEndUnix = &value
		}
		period.IsActive = isActive == 1
		period.CreatedAt = time.Unix(int64(createdAtUnix), 0)
		period.UpdatedAt = time.Unix(int64(updatedAtUnix), 0)
		periods = append(periods, period)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site config periods: %w", err)
	}

	return periods, nil
}

// GetSiteConfigPeriod retrieves a single config period by ID.
func (db *DB) GetSiteConfigPeriod(id int) (*SiteConfigPeriod, error) {
	query := `
		SELECT
			id, site_id, effective_start_unix, effective_end_unix, is_active,
			notes, cosine_error_angle, created_at, updated_at
		FROM site_config_periods
		WHERE id = ?
	`
	var period SiteConfigPeriod
	var endUnix sql.NullFloat64
	var isActive int
	var createdAtUnix, updatedAtUnix float64

	err := db.DB.QueryRow(query, id).Scan(
		&period.ID,
		&period.SiteID,
		&period.EffectiveStartUnix,
		&endUnix,
		&isActive,
		&period.Notes,
		&period.CosineErrorAngle,
		&createdAtUnix,
		&updatedAtUnix,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("site config period not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get site config period: %w", err)
	}
	if endUnix.Valid {
		value := endUnix.Float64
		period.EffectiveEndUnix = &value
	}
	period.IsActive = isActive == 1
	period.CreatedAt = time.Unix(int64(createdAtUnix), 0)
	period.UpdatedAt = time.Unix(int64(updatedAtUnix), 0)

	return &period, nil
}

// GetActiveSiteConfigPeriod returns the active config period for a site.
func (db *DB) GetActiveSiteConfigPeriod(siteID int) (*SiteConfigPeriod, error) {
	query := `
		SELECT
			id, site_id, effective_start_unix, effective_end_unix, is_active,
			notes, cosine_error_angle, created_at, updated_at
		FROM site_config_periods
		WHERE site_id = ? AND is_active = 1
		ORDER BY effective_start_unix DESC
		LIMIT 1
	`
	var period SiteConfigPeriod
	var endUnix sql.NullFloat64
	var isActive int
	var createdAtUnix, updatedAtUnix float64

	err := db.DB.QueryRow(query, siteID).Scan(
		&period.ID,
		&period.SiteID,
		&period.EffectiveStartUnix,
		&endUnix,
		&isActive,
		&period.Notes,
		&period.CosineErrorAngle,
		&createdAtUnix,
		&updatedAtUnix,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("site config period not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active site config period: %w", err)
	}
	if endUnix.Valid {
		value := endUnix.Float64
		period.EffectiveEndUnix = &value
	}
	period.IsActive = isActive == 1
	period.CreatedAt = time.Unix(int64(createdAtUnix), 0)
	period.UpdatedAt = time.Unix(int64(updatedAtUnix), 0)

	return &period, nil
}

// CreateSiteConfigPeriod inserts a new site config period.
func (db *DB) CreateSiteConfigPeriod(period *SiteConfigPeriod) error {
	if err := validateSiteConfigPeriod(period); err != nil {
		return err
	}
	if err := db.ensureNoOverlap(period, nil); err != nil {
		return err
	}

	query := `
		INSERT INTO site_config_periods (
			site_id, effective_start_unix, effective_end_unix, is_active,
			notes, cosine_error_angle
		) VALUES (?, ?, ?, ?, ?, ?)
	`
	isActive := 0
	if period.IsActive {
		isActive = 1
	}

	result, err := db.DB.Exec(
		query,
		period.SiteID,
		period.EffectiveStartUnix,
		period.EffectiveEndUnix,
		isActive,
		period.Notes,
		period.CosineErrorAngle,
	)
	if err != nil {
		return fmt.Errorf("failed to create site config period: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get site config period ID: %w", err)
	}
	period.ID = int(id)
	return nil
}

// UpdateSiteConfigPeriod updates an existing site config period.
func (db *DB) UpdateSiteConfigPeriod(period *SiteConfigPeriod) error {
	if period.ID == 0 {
		return fmt.Errorf("site config period ID is required")
	}
	if err := validateSiteConfigPeriod(period); err != nil {
		return err
	}
	if err := db.ensureNoOverlap(period, &period.ID); err != nil {
		return err
	}

	query := `
		UPDATE site_config_periods SET
			site_id = ?,
			effective_start_unix = ?,
			effective_end_unix = ?,
			is_active = ?,
			notes = ?,
			cosine_error_angle = ?
		WHERE id = ?
	`
	isActive := 0
	if period.IsActive {
		isActive = 1
	}

	result, err := db.DB.Exec(
		query,
		period.SiteID,
		period.EffectiveStartUnix,
		period.EffectiveEndUnix,
		isActive,
		period.Notes,
		period.CosineErrorAngle,
		period.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update site config period: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("site config period not found")
	}

	return nil
}

func validateSiteConfigPeriod(period *SiteConfigPeriod) error {
	if period.SiteID == 0 {
		return fmt.Errorf("site_id is required")
	}
	if period.EffectiveStartUnix < 0 {
		return fmt.Errorf("effective_start_unix must be non-negative")
	}
	if period.EffectiveEndUnix != nil && *period.EffectiveEndUnix <= period.EffectiveStartUnix {
		return fmt.Errorf("effective_end_unix must be greater than effective_start_unix")
	}
	if math.IsNaN(period.CosineErrorAngle) {
		return fmt.Errorf("cosine error angle must be a valid number")
	}
	if period.CosineErrorAngle < 0.0 || period.CosineErrorAngle > 80.0 {
		return fmt.Errorf("cosine error angle must be between 0 and 80 degrees")
	}
	return nil
}

func (db *DB) ensureNoOverlap(period *SiteConfigPeriod, excludeID *int) error {
	endUnix := maxUnixTime
	if period.EffectiveEndUnix != nil {
		endUnix = *period.EffectiveEndUnix
	}

	query := `
		SELECT COUNT(1)
		FROM site_config_periods
		WHERE site_id = ?
		  AND ? < COALESCE(effective_end_unix, ?)
		  AND ? > effective_start_unix
	`
	args := []interface{}{period.SiteID, period.EffectiveStartUnix, maxUnixTime, endUnix}
	if excludeID != nil {
		query += " AND id != ?"
		args = append(args, *excludeID)
	}

	var count int
	if err := db.DB.QueryRow(query, args...).Scan(&count); err != nil {
		return fmt.Errorf("failed to check site config period overlap: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("site config period overlaps an existing period")
	}
	return nil
}
