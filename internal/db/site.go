package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Site represents a survey site configuration (static properties)
// Time-varying configuration (like cosine_error_angle) is in SiteVariableConfig
type Site struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Location        string    `json:"location"`
	Description     *string   `json:"description"`
	SpeedLimit      int       `json:"speed_limit"`
	Surveyor        string    `json:"surveyor"`
	Contact         string    `json:"contact"`
	Address         *string   `json:"address"`
	Latitude        *float64  `json:"latitude"`
	Longitude       *float64  `json:"longitude"`
	MapAngle        *float64  `json:"map_angle"`
	IncludeMap      bool      `json:"include_map"`
	SiteDescription *string   `json:"site_description"`
	SpeedLimitNote  *string   `json:"speed_limit_note"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SiteVariableConfig represents time-varying site configuration values
// Multiple site_config_periods can reference the same config (many-to-one)
type SiteVariableConfig struct {
	ID               int     `json:"id"`
	CosineErrorAngle float64 `json:"cosine_error_angle"`
	CreatedAt        float64 `json:"created_at"`
	UpdatedAt        float64 `json:"updated_at"`
}

// CreateSite creates a new site in the database
func (db *DB) CreateSite(site *Site) error {
	query := `
		INSERT INTO site (
			name, location, description, speed_limit,
			surveyor, contact, address, latitude, longitude, map_angle,
			include_map, site_description, speed_limit_note
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	includeMapInt := 0
	if site.IncludeMap {
		includeMapInt = 1
	}

	result, err := db.DB.Exec(
		query,
		site.Name,
		site.Location,
		site.Description,
		site.SpeedLimit,
		site.Surveyor,
		site.Contact,
		site.Address,
		site.Latitude,
		site.Longitude,
		site.MapAngle,
		includeMapInt,
		site.SiteDescription,
		site.SpeedLimitNote,
	)
	if err != nil {
		return fmt.Errorf("failed to create site: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	site.ID = int(id)
	return nil
}

// GetSite retrieves a site by ID
func (db *DB) GetSite(id int) (*Site, error) {
	query := `
		SELECT
			id, name, location, description, speed_limit,
			surveyor, contact, address, latitude, longitude, map_angle,
			include_map, site_description, speed_limit_note,
			created_at, updated_at
		FROM site
		WHERE id = ?
	`

	var site Site
	var includeMapInt int
	var createdAtUnix, updatedAtUnix int64

	err := db.DB.QueryRow(query, id).Scan(
		&site.ID,
		&site.Name,
		&site.Location,
		&site.Description,
		&site.SpeedLimit,
		&site.Surveyor,
		&site.Contact,
		&site.Address,
		&site.Latitude,
		&site.Longitude,
		&site.MapAngle,
		&includeMapInt,
		&site.SiteDescription,
		&site.SpeedLimitNote,
		&createdAtUnix,
		&updatedAtUnix,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("site not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get site: %w", err)
	}

	site.IncludeMap = includeMapInt == 1
	site.CreatedAt = time.Unix(createdAtUnix, 0)
	site.UpdatedAt = time.Unix(updatedAtUnix, 0)

	return &site, nil
}

// GetAllSites retrieves all sites from the database
func (db *DB) GetAllSites() ([]Site, error) {
	query := `
		SELECT
			id, name, location, description, speed_limit,
			surveyor, contact, address, latitude, longitude, map_angle,
			include_map, site_description, speed_limit_note,
			created_at, updated_at
		FROM site
		ORDER BY name ASC
	`

	rows, err := db.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query sites: %w", err)
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		var site Site
		var includeMapInt int
		var createdAtUnix, updatedAtUnix int64

		err := rows.Scan(
			&site.ID,
			&site.Name,
			&site.Location,
			&site.Description,
			&site.SpeedLimit,
			&site.Surveyor,
			&site.Contact,
			&site.Address,
			&site.Latitude,
			&site.Longitude,
			&site.MapAngle,
			&includeMapInt,
			&site.SiteDescription,
			&site.SpeedLimitNote,
			&createdAtUnix,
			&updatedAtUnix,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site: %w", err)
		}

		site.IncludeMap = includeMapInt == 1
		site.CreatedAt = time.Unix(createdAtUnix, 0)
		site.UpdatedAt = time.Unix(updatedAtUnix, 0)

		sites = append(sites, site)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sites: %w", err)
	}

	return sites, nil
}

// UpdateSite updates an existing site in the database
func (db *DB) UpdateSite(site *Site) error {
	query := `
		UPDATE site SET
			name = ?,
			location = ?,
			description = ?,
			speed_limit = ?,
			surveyor = ?,
			contact = ?,
			address = ?,
			latitude = ?,
			longitude = ?,
			map_angle = ?,
			include_map = ?,
			site_description = ?,
			speed_limit_note = ?
		WHERE id = ?
	`

	includeMapInt := 0
	if site.IncludeMap {
		includeMapInt = 1
	}

	result, err := db.DB.Exec(
		query,
		site.Name,
		site.Location,
		site.Description,
		site.SpeedLimit,
		site.Surveyor,
		site.Contact,
		site.Address,
		site.Latitude,
		site.Longitude,
		site.MapAngle,
		includeMapInt,
		site.SiteDescription,
		site.SpeedLimitNote,
		site.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update site: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("site not found")
	}

	return nil
}

// DeleteSite deletes a site from the database
func (db *DB) DeleteSite(id int) error {
	query := `DELETE FROM site WHERE id = ?`

	result, err := db.DB.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete site: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("site not found")
	}

	return nil
}

// CreateSiteVariableConfig creates a new site variable configuration
func (db *DB) CreateSiteVariableConfig(config *SiteVariableConfig) error {
query := `
INSERT INTO site_variable_config (cosine_error_angle)
VALUES (?)
`

result, err := db.DB.Exec(query, config.CosineErrorAngle)
if err != nil {
return fmt.Errorf("failed to create site variable config: %w", err)
}

id, err := result.LastInsertId()
if err != nil {
return fmt.Errorf("failed to get last insert ID: %w", err)
}

config.ID = int(id)
return nil
}

// GetSiteVariableConfig retrieves a site variable config by ID
func (db *DB) GetSiteVariableConfig(id int) (*SiteVariableConfig, error) {
query := `
SELECT id, cosine_error_angle, created_at, updated_at
FROM site_variable_config
WHERE id = ?
`

var config SiteVariableConfig
err := db.DB.QueryRow(query, id).Scan(
&config.ID,
&config.CosineErrorAngle,
&config.CreatedAt,
&config.UpdatedAt,
)

if err == sql.ErrNoRows {
return nil, fmt.Errorf("site variable config not found")
}
if err != nil {
return nil, fmt.Errorf("failed to get site variable config: %w", err)
}

return &config, nil
}

// GetAllSiteVariableConfigs retrieves all site variable configs
func (db *DB) GetAllSiteVariableConfigs() ([]SiteVariableConfig, error) {
query := `
SELECT id, cosine_error_angle, created_at, updated_at
FROM site_variable_config
ORDER BY id ASC
`

rows, err := db.DB.Query(query)
if err != nil {
return nil, fmt.Errorf("failed to query site variable configs: %w", err)
}
defer rows.Close()

var configs []SiteVariableConfig
for rows.Next() {
var config SiteVariableConfig
err := rows.Scan(
&config.ID,
&config.CosineErrorAngle,
&config.CreatedAt,
&config.UpdatedAt,
)
if err != nil {
return nil, fmt.Errorf("failed to scan site variable config: %w", err)
}
configs = append(configs, config)
}

if err = rows.Err(); err != nil {
return nil, fmt.Errorf("error iterating site variable configs: %w", err)
}

return configs, nil
}

// UpdateSiteVariableConfig updates an existing site variable config
func (db *DB) UpdateSiteVariableConfig(config *SiteVariableConfig) error {
query := `
UPDATE site_variable_config SET
cosine_error_angle = ?
WHERE id = ?
`

result, err := db.DB.Exec(query, config.CosineErrorAngle, config.ID)
if err != nil {
return fmt.Errorf("failed to update site variable config: %w", err)
}

rowsAffected, err := result.RowsAffected()
if err != nil {
return fmt.Errorf("failed to get rows affected: %w", err)
}

if rowsAffected == 0 {
return fmt.Errorf("site variable config not found")
}

return nil
}

// DeleteSiteVariableConfig deletes a site variable config
func (db *DB) DeleteSiteVariableConfig(id int) error {
query := `DELETE FROM site_variable_config WHERE id = ?`

result, err := db.DB.Exec(query, id)
if err != nil {
return fmt.Errorf("failed to delete site variable config: %w", err)
}

rowsAffected, err := result.RowsAffected()
if err != nil {
return fmt.Errorf("failed to get rows affected: %w", err)
}

if rowsAffected == 0 {
return fmt.Errorf("site variable config not found")
}

return nil
}
