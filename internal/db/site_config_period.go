package db

import (
	"database/sql"
	"fmt"
	"time"
)

// SiteConfigPeriod represents a time period during which a specific site configuration was effective
// This is a Type 6 SCD (Slowly Changing Dimension) that tracks configuration history
type SiteConfigPeriod struct {
	ID                 int      `json:"id"`
	SiteID             int      `json:"site_id"`
	EffectiveStartUnix float64  `json:"effective_start_unix"`
	EffectiveEndUnix   *float64 `json:"effective_end_unix"` // NULL means currently active/open-ended
	IsActive           bool     `json:"is_active"`          // True if this is the active period for new data
	Notes              *string  `json:"notes"`
	CreatedAt          float64  `json:"created_at"`
	UpdatedAt          float64  `json:"updated_at"`
}

// SiteConfigPeriodWithSite includes site details along with the period
type SiteConfigPeriodWithSite struct {
	SiteConfigPeriod
	Site *Site `json:"site"`
}

// CreateSiteConfigPeriod creates a new site configuration period
func (db *DB) CreateSiteConfigPeriod(period *SiteConfigPeriod) error {
	query := `
		INSERT INTO site_config_periods (
			site_id, effective_start_unix, effective_end_unix, is_active, notes
		) VALUES (?, ?, ?, ?, ?)
	`

	isActiveInt := 0
	if period.IsActive {
		isActiveInt = 1
	}

	result, err := db.DB.Exec(
		query,
		period.SiteID,
		period.EffectiveStartUnix,
		period.EffectiveEndUnix,
		isActiveInt,
		period.Notes,
	)
	if err != nil {
		return fmt.Errorf("failed to create site config period: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	period.ID = int(id)
	return nil
}

// GetSiteConfigPeriod retrieves a period by ID
func (db *DB) GetSiteConfigPeriod(id int) (*SiteConfigPeriod, error) {
	query := `
		SELECT id, site_id, effective_start_unix, effective_end_unix, is_active, notes, created_at, updated_at
		FROM site_config_periods
		WHERE id = ?
	`

	var period SiteConfigPeriod
	var isActiveInt int

	err := db.DB.QueryRow(query, id).Scan(
		&period.ID,
		&period.SiteID,
		&period.EffectiveStartUnix,
		&period.EffectiveEndUnix,
		&isActiveInt,
		&period.Notes,
		&period.CreatedAt,
		&period.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("site config period not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get site config period: %w", err)
	}

	period.IsActive = isActiveInt == 1

	return &period, nil
}

// GetActiveSiteConfigPeriod retrieves the currently active site configuration period
func (db *DB) GetActiveSiteConfigPeriod() (*SiteConfigPeriodWithSite, error) {
	query := `
		SELECT 
			p.id, p.site_id, p.effective_start_unix, p.effective_end_unix, p.is_active, p.notes, p.created_at, p.updated_at,
			s.id, s.name, s.location, s.description, s.cosine_error_angle, s.speed_limit,
			s.surveyor, s.contact, s.address, s.latitude, s.longitude, s.map_angle,
			s.include_map, s.site_description, s.speed_limit_note, s.created_at, s.updated_at
		FROM site_config_periods p
		JOIN site s ON p.site_id = s.id
		WHERE p.is_active = 1
		LIMIT 1
	`

	var periodWithSite SiteConfigPeriodWithSite
	periodWithSite.Site = &Site{} // Initialize the Site pointer
	var isActiveInt int
	var includeMapInt int
	var siteCreatedAtUnix, siteUpdatedAtUnix int64

	err := db.DB.QueryRow(query).Scan(
		&periodWithSite.ID,
		&periodWithSite.SiteID,
		&periodWithSite.EffectiveStartUnix,
		&periodWithSite.EffectiveEndUnix,
		&isActiveInt,
		&periodWithSite.Notes,
		&periodWithSite.CreatedAt,
		&periodWithSite.UpdatedAt,
		&periodWithSite.Site.ID,
		&periodWithSite.Site.Name,
		&periodWithSite.Site.Location,
		&periodWithSite.Site.Description,
		&periodWithSite.Site.CosineErrorAngle,
		&periodWithSite.Site.SpeedLimit,
		&periodWithSite.Site.Surveyor,
		&periodWithSite.Site.Contact,
		&periodWithSite.Site.Address,
		&periodWithSite.Site.Latitude,
		&periodWithSite.Site.Longitude,
		&periodWithSite.Site.MapAngle,
		&includeMapInt,
		&periodWithSite.Site.SiteDescription,
		&periodWithSite.Site.SpeedLimitNote,
		&siteCreatedAtUnix,
		&siteUpdatedAtUnix,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no active site config period found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active site config period: %w", err)
	}

	periodWithSite.IsActive = isActiveInt == 1
	periodWithSite.Site.IncludeMap = includeMapInt == 1
	periodWithSite.Site.CreatedAt = time.Unix(siteCreatedAtUnix, 0)
	periodWithSite.Site.UpdatedAt = time.Unix(siteUpdatedAtUnix, 0)

	return &periodWithSite, nil
}

// GetSiteConfigPeriodForTimestamp finds the site configuration period that was effective at a given timestamp
func (db *DB) GetSiteConfigPeriodForTimestamp(timestamp float64) (*SiteConfigPeriodWithSite, error) {
	query := `
		SELECT 
			p.id, p.site_id, p.effective_start_unix, p.effective_end_unix, p.is_active, p.notes, p.created_at, p.updated_at,
			s.id, s.name, s.location, s.description, s.cosine_error_angle, s.speed_limit,
			s.surveyor, s.contact, s.address, s.latitude, s.longitude, s.map_angle,
			s.include_map, s.site_description, s.speed_limit_note, s.created_at, s.updated_at
		FROM site_config_periods p
		JOIN site s ON p.site_id = s.id
		WHERE p.effective_start_unix <= ?
		  AND (p.effective_end_unix IS NULL OR p.effective_end_unix > ?)
		ORDER BY p.effective_start_unix DESC
		LIMIT 1
	`

	var periodWithSite SiteConfigPeriodWithSite
	periodWithSite.Site = &Site{}
	var isActiveInt int
	var includeMapInt int
	var siteCreatedAtUnix, siteUpdatedAtUnix int64

	err := db.DB.QueryRow(query, timestamp, timestamp).Scan(
		&periodWithSite.ID,
		&periodWithSite.SiteID,
		&periodWithSite.EffectiveStartUnix,
		&periodWithSite.EffectiveEndUnix,
		&isActiveInt,
		&periodWithSite.Notes,
		&periodWithSite.CreatedAt,
		&periodWithSite.UpdatedAt,
		&periodWithSite.Site.ID,
		&periodWithSite.Site.Name,
		&periodWithSite.Site.Location,
		&periodWithSite.Site.Description,
		&periodWithSite.Site.CosineErrorAngle,
		&periodWithSite.Site.SpeedLimit,
		&periodWithSite.Site.Surveyor,
		&periodWithSite.Site.Contact,
		&periodWithSite.Site.Address,
		&periodWithSite.Site.Latitude,
		&periodWithSite.Site.Longitude,
		&periodWithSite.Site.MapAngle,
		&includeMapInt,
		&periodWithSite.Site.SiteDescription,
		&periodWithSite.Site.SpeedLimitNote,
		&siteCreatedAtUnix,
		&siteUpdatedAtUnix,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no site config period found for timestamp %f", timestamp)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get site config period for timestamp: %w", err)
	}

	periodWithSite.IsActive = isActiveInt == 1
	periodWithSite.Site.IncludeMap = includeMapInt == 1
	periodWithSite.Site.CreatedAt = time.Unix(siteCreatedAtUnix, 0)
	periodWithSite.Site.UpdatedAt = time.Unix(siteUpdatedAtUnix, 0)

	return &periodWithSite, nil
}

// GetAllSiteConfigPeriods retrieves all site configuration periods, ordered by start time
func (db *DB) GetAllSiteConfigPeriods() ([]SiteConfigPeriodWithSite, error) {
	query := `
		SELECT 
			p.id, p.site_id, p.effective_start_unix, p.effective_end_unix, p.is_active, p.notes, p.created_at, p.updated_at,
			s.id, s.name, s.location, s.description, s.cosine_error_angle, s.speed_limit,
			s.surveyor, s.contact, s.address, s.latitude, s.longitude, s.map_angle,
			s.include_map, s.site_description, s.speed_limit_note, s.created_at, s.updated_at
		FROM site_config_periods p
		JOIN site s ON p.site_id = s.id
		ORDER BY p.effective_start_unix ASC
	`

	rows, err := db.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query site config periods: %w", err)
	}
	defer rows.Close()

	var periods []SiteConfigPeriodWithSite
	for rows.Next() {
		var periodWithSite SiteConfigPeriodWithSite
		periodWithSite.Site = &Site{}
		var isActiveInt int
		var includeMapInt int
		var siteCreatedAtUnix, siteUpdatedAtUnix int64

		err := rows.Scan(
			&periodWithSite.ID,
			&periodWithSite.SiteID,
			&periodWithSite.EffectiveStartUnix,
			&periodWithSite.EffectiveEndUnix,
			&isActiveInt,
			&periodWithSite.Notes,
			&periodWithSite.CreatedAt,
			&periodWithSite.UpdatedAt,
			&periodWithSite.Site.ID,
			&periodWithSite.Site.Name,
			&periodWithSite.Site.Location,
			&periodWithSite.Site.Description,
			&periodWithSite.Site.CosineErrorAngle,
			&periodWithSite.Site.SpeedLimit,
			&periodWithSite.Site.Surveyor,
			&periodWithSite.Site.Contact,
			&periodWithSite.Site.Address,
			&periodWithSite.Site.Latitude,
			&periodWithSite.Site.Longitude,
			&periodWithSite.Site.MapAngle,
			&includeMapInt,
			&periodWithSite.Site.SiteDescription,
			&periodWithSite.Site.SpeedLimitNote,
			&siteCreatedAtUnix,
			&siteUpdatedAtUnix,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site config period: %w", err)
		}

		periodWithSite.IsActive = isActiveInt == 1
		periodWithSite.Site.IncludeMap = includeMapInt == 1
		periodWithSite.Site.CreatedAt = time.Unix(siteCreatedAtUnix, 0)
		periodWithSite.Site.UpdatedAt = time.Unix(siteUpdatedAtUnix, 0)

		periods = append(periods, periodWithSite)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site config periods: %w", err)
	}

	return periods, nil
}

// UpdateSiteConfigPeriod updates an existing site configuration period
func (db *DB) UpdateSiteConfigPeriod(period *SiteConfigPeriod) error {
	query := `
		UPDATE site_config_periods SET
			site_id = ?,
			effective_start_unix = ?,
			effective_end_unix = ?,
			is_active = ?,
			notes = ?
		WHERE id = ?
	`

	isActiveInt := 0
	if period.IsActive {
		isActiveInt = 1
	}

	result, err := db.DB.Exec(
		query,
		period.SiteID,
		period.EffectiveStartUnix,
		period.EffectiveEndUnix,
		isActiveInt,
		period.Notes,
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

// SetActiveSiteConfigPeriod sets a specific period as active (and deactivates all others)
// This is useful for marking which site configuration should be used for new incoming data
func (db *DB) SetActiveSiteConfigPeriod(periodID int) error {
	// The trigger will handle deactivating other periods
	query := `
		UPDATE site_config_periods
		SET is_active = 1
		WHERE id = ?
	`

	result, err := db.DB.Exec(query, periodID)
	if err != nil {
		return fmt.Errorf("failed to set active site config period: %w", err)
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

// CloseSiteConfigPeriod closes an open-ended period by setting its end time
func (db *DB) CloseSiteConfigPeriod(periodID int, endTime float64) error {
	query := `
		UPDATE site_config_periods
		SET effective_end_unix = ?,
		    is_active = 0
		WHERE id = ?
		  AND effective_end_unix IS NULL
	`

	result, err := db.DB.Exec(query, endTime, periodID)
	if err != nil {
		return fmt.Errorf("failed to close site config period: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("site config period not found or already closed")
	}

	return nil
}

// DeleteSiteConfigPeriod deletes a site config period
func (db *DB) DeleteSiteConfigPeriod(id int) error {
	query := `DELETE FROM site_config_periods WHERE id = ?`

	result, err := db.DB.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete site config period: %w", err)
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

// TimelineEntry represents a time period with associated data and site configuration
type TimelineEntry struct {
	StartUnix          float64                   `json:"start_unix"`
	EndUnix            *float64                  `json:"end_unix"` // NULL means ongoing
	HasData            bool                      `json:"has_data"`
	DataCount          int                       `json:"data_count"`
	SiteConfigPeriod   *SiteConfigPeriodWithSite `json:"site_config_period"`   // NULL if no config assigned
	UnconfiguredPeriod bool                      `json:"unconfigured_period"`  // True if data exists but no site config
}

// GetTimeline returns a timeline showing all time periods where data exists,
// along with their associated site configurations (or lack thereof)
func (db *DB) GetTimeline(startUnix, endUnix float64) ([]TimelineEntry, error) {
	// This query finds all distinct time periods where either:
	// 1. radar_data exists
	// 2. A site_config_period is defined
	// Then associates each data period with the relevant site config (if any)
	
	query := `
		WITH data_periods AS (
			-- Get time boundaries where radar data exists
			SELECT DISTINCT
				write_timestamp as period_start,
				write_timestamp as period_end
			FROM radar_data
			WHERE write_timestamp BETWEEN ? AND ?
		),
		config_periods AS (
			-- Get all site config periods in the time range
			SELECT 
				p.id,
				p.site_id,
				p.effective_start_unix,
				p.effective_end_unix,
				p.is_active,
				p.notes,
				p.created_at,
				p.updated_at,
				s.name,
				s.cosine_error_angle
			FROM site_config_periods p
			JOIN site s ON p.site_id = s.id
			WHERE p.effective_start_unix <= ?
			  AND (p.effective_end_unix IS NULL OR p.effective_end_unix >= ?)
		),
		period_boundaries AS (
			-- Collect all unique boundaries (starts and ends of both data and config periods)
			SELECT DISTINCT effective_start_unix as boundary FROM config_periods
			UNION
			SELECT DISTINCT effective_end_unix FROM config_periods WHERE effective_end_unix IS NOT NULL
			UNION
			SELECT ? as boundary -- Query start
			UNION
			SELECT ? as boundary -- Query end
		),
		time_segments AS (
			-- Create continuous time segments from boundaries
			SELECT 
				boundary as seg_start,
				LEAD(boundary) OVER (ORDER BY boundary) as seg_end
			FROM period_boundaries
		)
		SELECT 
			ts.seg_start,
			ts.seg_end,
			COUNT(DISTINCT rd.rowid) as data_count,
			cp.id as config_id,
			cp.site_id,
			cp.effective_start_unix,
			cp.effective_end_unix,
			cp.is_active,
			cp.notes,
			cp.name as site_name,
			cp.cosine_error_angle
		FROM time_segments ts
		LEFT JOIN radar_data rd ON rd.write_timestamp >= ts.seg_start 
		                        AND (ts.seg_end IS NULL OR rd.write_timestamp < ts.seg_end)
		LEFT JOIN config_periods cp ON ts.seg_start >= cp.effective_start_unix
		                           AND (cp.effective_end_unix IS NULL OR ts.seg_start < cp.effective_end_unix)
		WHERE ts.seg_end IS NOT NULL
		  AND ts.seg_start BETWEEN ? AND ?
		GROUP BY ts.seg_start, ts.seg_end, cp.id
		HAVING data_count > 0 OR cp.id IS NOT NULL
		ORDER BY ts.seg_start
	`

	rows, err := db.DB.Query(query, startUnix, endUnix, endUnix, startUnix, startUnix, endUnix, startUnix, endUnix)
	if err != nil {
		return nil, fmt.Errorf("failed to query timeline: %w", err)
	}
	defer rows.Close()

	var timeline []TimelineEntry
	for rows.Next() {
		var entry TimelineEntry
		var configID sql.NullInt64
		var siteID sql.NullInt64
		var effectiveStart sql.NullFloat64
		var effectiveEnd sql.NullFloat64
		var isActive sql.NullInt64
		var notes sql.NullString
		var siteName sql.NullString
		var cosineAngle sql.NullFloat64

		err := rows.Scan(
			&entry.StartUnix,
			&entry.EndUnix,
			&entry.DataCount,
			&configID,
			&siteID,
			&effectiveStart,
			&effectiveEnd,
			&isActive,
			&notes,
			&siteName,
			&cosineAngle,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan timeline entry: %w", err)
		}

		entry.HasData = entry.DataCount > 0

		// If we have a config period, populate it
		if configID.Valid {
			period := &SiteConfigPeriodWithSite{
				SiteConfigPeriod: SiteConfigPeriod{
					ID:                 int(configID.Int64),
					SiteID:             int(siteID.Int64),
					EffectiveStartUnix: effectiveStart.Float64,
					IsActive:           isActive.Int64 == 1,
				},
				Site: &Site{
					ID:               int(siteID.Int64),
					Name:             siteName.String,
					CosineErrorAngle: cosineAngle.Float64,
				},
			}
			if effectiveEnd.Valid {
				period.EffectiveEndUnix = &effectiveEnd.Float64
			}
			if notes.Valid {
				period.Notes = &notes.String
			}
			entry.SiteConfigPeriod = period
		}

		// Mark as unconfigured if we have data but no config
		entry.UnconfiguredPeriod = entry.HasData && !configID.Valid

		timeline = append(timeline, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating timeline: %w", err)
	}

	return timeline, nil
}
