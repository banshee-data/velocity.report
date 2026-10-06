package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxUnixTime = 32503680000.0 // 3000-01-01T00:00:00Z

// Posted speed limit units. A limit is stored in km/h; the unit records what
// the sign shows, so a 25 mph limit reads back as 25 mph.
const (
	SpeedLimitUnitKph = "kph"
	SpeedLimitUnitMph = "mph"
)

// minSpeedLimitKph and maxSpeedLimitKph bound a posted limit; the schema's
// CHECK uses the same values. The highest posted road limits are about
// 160 km/h, so a larger value is a unit mistake. The floor admits every real
// posted limit and refuses a denormal or rounding artefact that would read
// back as 0 km/h.
const (
	minSpeedLimitKph = 1.0
	maxSpeedLimitKph = 200.0
)

// maxJurisdictionLength bounds the free-text jurisdiction; the schema's CHECK
// uses the same value.
const maxJurisdictionLength = 100

// SiteConfigPeriodValidationError reports a period that fails validation, as
// distinct from a storage failure, so the API can answer 400 rather than 500.
type SiteConfigPeriodValidationError struct{ msg string }

func (e *SiteConfigPeriodValidationError) Error() string { return e.msg }

func invalidPeriod(format string, args ...any) error {
	return &SiteConfigPeriodValidationError{msg: fmt.Sprintf(format, args...)}
}

// IsSiteConfigPeriodValidationError reports whether err is a validation
// failure.
func IsSiteConfigPeriodValidationError(err error) bool {
	var v *SiteConfigPeriodValidationError
	return errors.As(err, &v)
}

// SiteConfigPeriod represents a time-based configuration period for a site.
type SiteConfigPeriod struct {
	ID                 int      `json:"id"`
	SiteID             int      `json:"site_id"`
	EffectiveStartUnix float64  `json:"effective_start_unix"`
	EffectiveEndUnix   *float64 `json:"effective_end_unix"`
	IsActive           bool     `json:"is_active"`
	Notes              *string  `json:"notes"`
	CosineErrorAngle   float64  `json:"cosine_error_angle"`
	// SpeedLimitKph is the posted limit in km/h, or nil when none is recorded.
	SpeedLimitKph *float64 `json:"speed_limit_kph"`
	// SpeedLimitUnit is the unit the posted limit is signed in, kph or mph.
	// It is set exactly when SpeedLimitKph is.
	SpeedLimitUnit *string `json:"speed_limit_unit"`
	// Jurisdiction names whose rule the limit is, such as US-CA.
	Jurisdiction *string   `json:"jurisdiction"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// siteConfigPeriodColumns is the column list every reader selects, in the
// order scanSiteConfigPeriod reads it.
const siteConfigPeriodColumns = `
			id, site_id, effective_start_unix, effective_end_unix, is_active,
			notes, cosine_error_angle, speed_limit_kph, speed_limit_unit,
			jurisdiction, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSiteConfigPeriod(row rowScanner) (SiteConfigPeriod, error) {
	var period SiteConfigPeriod
	var endUnix, speedLimitKph sql.NullFloat64
	var speedLimitUnit, jurisdiction sql.NullString
	var isActive int
	var createdAtUnix, updatedAtUnix float64
	if err := row.Scan(
		&period.ID,
		&period.SiteID,
		&period.EffectiveStartUnix,
		&endUnix,
		&isActive,
		&period.Notes,
		&period.CosineErrorAngle,
		&speedLimitKph,
		&speedLimitUnit,
		&jurisdiction,
		&createdAtUnix,
		&updatedAtUnix,
	); err != nil {
		return SiteConfigPeriod{}, err
	}
	if endUnix.Valid {
		value := endUnix.Float64
		period.EffectiveEndUnix = &value
	}
	if speedLimitKph.Valid {
		value := speedLimitKph.Float64
		period.SpeedLimitKph = &value
	}
	if speedLimitUnit.Valid {
		value := speedLimitUnit.String
		period.SpeedLimitUnit = &value
	}
	if jurisdiction.Valid {
		value := jurisdiction.String
		period.Jurisdiction = &value
	}
	period.IsActive = isActive == 1
	period.CreatedAt = time.Unix(int64(createdAtUnix), 0)
	period.UpdatedAt = time.Unix(int64(updatedAtUnix), 0)
	return period, nil
}

// ListSiteConfigPeriods returns all site config periods, optionally filtered by site.
func (db *DB) ListSiteConfigPeriods(siteID *int) ([]SiteConfigPeriod, error) {
	query := `SELECT` + siteConfigPeriodColumns + `
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
		period, err := scanSiteConfigPeriod(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site config period: %w", err)
		}
		periods = append(periods, period)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site config periods: %w", err)
	}

	return periods, nil
}

// GetSiteConfigPeriod retrieves a single config period by ID.
func (db *DB) GetSiteConfigPeriod(id int) (*SiteConfigPeriod, error) {
	query := `SELECT` + siteConfigPeriodColumns + `
		FROM site_config_periods
		WHERE id = ?
	`
	period, err := scanSiteConfigPeriod(db.DB.QueryRow(query, id))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("site config period not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get site config period: %w", err)
	}
	return &period, nil
}

// GetActiveSiteConfigPeriod returns the active config period for a site.
func (db *DB) GetActiveSiteConfigPeriod(siteID int) (*SiteConfigPeriod, error) {
	query := `SELECT` + siteConfigPeriodColumns + `
		FROM site_config_periods
		WHERE site_id = ? AND is_active = 1
		ORDER BY effective_start_unix DESC
		LIMIT 1
	`
	period, err := scanSiteConfigPeriod(db.DB.QueryRow(query, siteID))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("site config period not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active site config period: %w", err)
	}
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
			notes, cosine_error_angle, speed_limit_kph, speed_limit_unit,
			jurisdiction
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		period.SpeedLimitKph,
		period.SpeedLimitUnit,
		period.Jurisdiction,
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
			cosine_error_angle = ?,
			speed_limit_kph = ?,
			speed_limit_unit = ?,
			jurisdiction = ?
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
		period.SpeedLimitKph,
		period.SpeedLimitUnit,
		period.Jurisdiction,
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

// validateSiteConfigPeriod checks a period before it is written, and
// normalises its speed limit unit and jurisdiction in place: the unit to
// lower case, and a blank jurisdiction to none.
func validateSiteConfigPeriod(period *SiteConfigPeriod) error {
	if period.SiteID == 0 {
		return invalidPeriod("site_id is required")
	}
	if period.EffectiveStartUnix < 0 {
		return invalidPeriod("effective_start_unix must be non-negative")
	}
	if period.EffectiveEndUnix != nil && *period.EffectiveEndUnix <= period.EffectiveStartUnix {
		return invalidPeriod("effective_end_unix must be greater than effective_start_unix")
	}
	if math.IsNaN(period.CosineErrorAngle) {
		return invalidPeriod("cosine error angle must be a valid number")
	}
	if period.CosineErrorAngle < 0.0 || period.CosineErrorAngle > 80.0 {
		return invalidPeriod("cosine error angle must be between 0 and 80 degrees")
	}
	if err := validateSpeedLimit(period); err != nil {
		return err
	}
	if period.Jurisdiction != nil {
		jurisdiction := strings.TrimSpace(*period.Jurisdiction)
		switch {
		case jurisdiction == "":
			period.Jurisdiction = nil
		case strings.IndexFunc(jurisdiction, unicode.IsControl) >= 0:
			// A NUL would also end SQLite's LENGTH early and fail the CHECK.
			return invalidPeriod("jurisdiction must not contain control characters")
		case utf8.RuneCountInString(jurisdiction) > maxJurisdictionLength:
			// Characters, as SQLite's LENGTH and the web form count them.
			return invalidPeriod("jurisdiction must be at most %d characters", maxJurisdictionLength)
		default:
			period.Jurisdiction = &jurisdiction
		}
	}
	return nil
}

// validateSpeedLimit requires a limit and its unit together, and a limit
// between minSpeedLimitKph and maxSpeedLimitKph km/h.
func validateSpeedLimit(period *SiteConfigPeriod) error {
	if period.SpeedLimitUnit != nil {
		unit := strings.ToLower(strings.TrimSpace(*period.SpeedLimitUnit))
		if unit != SpeedLimitUnitKph && unit != SpeedLimitUnitMph {
			return invalidPeriod("speed_limit_unit must be %q or %q", SpeedLimitUnitKph, SpeedLimitUnitMph)
		}
		period.SpeedLimitUnit = &unit
	}
	switch {
	case period.SpeedLimitKph == nil && period.SpeedLimitUnit == nil:
		return nil
	case period.SpeedLimitKph == nil:
		return invalidPeriod("speed_limit_unit is set without speed_limit_kph")
	case period.SpeedLimitUnit == nil:
		return invalidPeriod("speed_limit_kph needs speed_limit_unit, the unit the limit is signed in")
	}
	limit := *period.SpeedLimitKph
	if math.IsNaN(limit) || math.IsInf(limit, 0) || limit < minSpeedLimitKph || limit > maxSpeedLimitKph {
		return invalidPeriod("speed_limit_kph must be at least %g and at most %g", minSpeedLimitKph, maxSpeedLimitKph)
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
