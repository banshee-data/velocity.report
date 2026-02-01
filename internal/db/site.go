package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Site represents a survey site configuration
type Site struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Location        string    `json:"location"`
	Description     *string   `json:"description"`
	Surveyor        string    `json:"surveyor"`
	Contact         string    `json:"contact"`
	Address         *string   `json:"address"`
	Latitude        *float64  `json:"latitude"`
	Longitude       *float64  `json:"longitude"`
	MapAngle        *float64  `json:"map_angle"`
	IncludeMap      bool      `json:"include_map"`
	SiteDescription *string   `json:"site_description"`
	BBoxNELat       *float64  `json:"bbox_ne_lat"`
	BBoxNELng       *float64  `json:"bbox_ne_lng"`
	BBoxSWLat       *float64  `json:"bbox_sw_lat"`
	BBoxSWLng       *float64  `json:"bbox_sw_lng"`
	MapRotation     *float64  `json:"map_rotation"`
	MapSVGData      *[]byte   `json:"map_svg_data,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateSite creates a new site in the database
func (db *DB) CreateSite(site *Site) error {
	query := `
		INSERT INTO site (
			name, location, description,
			surveyor, contact, address, latitude, longitude, map_angle,
			include_map, site_description,
			bbox_ne_lat, bbox_ne_lng, bbox_sw_lat, bbox_sw_lng,
			map_rotation, map_svg_data
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	includeMapInt := 0
	if site.IncludeMap {
		includeMapInt = 1
	}

	var mapSVGData []byte
	if site.MapSVGData != nil {
		mapSVGData = *site.MapSVGData
	}

	result, err := db.DB.Exec(
		query,
		site.Name,
		site.Location,
		site.Description,
		site.Surveyor,
		site.Contact,
		site.Address,
		site.Latitude,
		site.Longitude,
		site.MapAngle,
		includeMapInt,
		site.SiteDescription,
		site.BBoxNELat,
		site.BBoxNELng,
		site.BBoxSWLat,
		site.BBoxSWLng,
		site.MapRotation,
		mapSVGData,
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
			id, name, location, description,
			surveyor, contact, address, latitude, longitude, map_angle,
			include_map, site_description,
			bbox_ne_lat, bbox_ne_lng, bbox_sw_lat, bbox_sw_lng,
			map_rotation, map_svg_data,
			created_at, updated_at
		FROM site
		WHERE id = ?
	`

	var site Site
	var includeMapInt int
	var createdAtUnix, updatedAtUnix int64
	var mapSVGData []byte

	err := db.DB.QueryRow(query, id).Scan(
		&site.ID,
		&site.Name,
		&site.Location,
		&site.Description,
		&site.Surveyor,
		&site.Contact,
		&site.Address,
		&site.Latitude,
		&site.Longitude,
		&site.MapAngle,
		&includeMapInt,
		&site.SiteDescription,
		&site.BBoxNELat,
		&site.BBoxNELng,
		&site.BBoxSWLat,
		&site.BBoxSWLng,
		&site.MapRotation,
		&mapSVGData,
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
	if len(mapSVGData) > 0 {
		site.MapSVGData = &mapSVGData
	}
	site.CreatedAt = time.Unix(createdAtUnix, 0)
	site.UpdatedAt = time.Unix(updatedAtUnix, 0)

	return &site, nil
}

// GetAllSites retrieves all sites from the database
func (db *DB) GetAllSites() ([]Site, error) {
	query := `
		SELECT
			id, name, location, description,
			surveyor, contact, address, latitude, longitude, map_angle,
			include_map, site_description,
			bbox_ne_lat, bbox_ne_lng, bbox_sw_lat, bbox_sw_lng,
			map_rotation, map_svg_data,
			created_at, updated_at
		FROM site
		ORDER BY name ASC
	`

	rows, err := db.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query sites: %w", err)
	}
	defer rows.Close()

	sites := []Site{} // Initialise to empty slice, not nil
	for rows.Next() {
		var site Site
		var includeMapInt int
		var createdAtUnix, updatedAtUnix int64
		var mapSVGData []byte

		err := rows.Scan(
			&site.ID,
			&site.Name,
			&site.Location,
			&site.Description,
			&site.Surveyor,
			&site.Contact,
			&site.Address,
			&site.Latitude,
			&site.Longitude,
			&site.MapAngle,
			&includeMapInt,
			&site.SiteDescription,
			&site.BBoxNELat,
			&site.BBoxNELng,
			&site.BBoxSWLat,
			&site.BBoxSWLng,
			&site.MapRotation,
			&mapSVGData,
			&createdAtUnix,
			&updatedAtUnix,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site: %w", err)
		}

		site.IncludeMap = includeMapInt == 1
		if len(mapSVGData) > 0 {
			site.MapSVGData = &mapSVGData
		}
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
			surveyor = ?,
			contact = ?,
			address = ?,
			latitude = ?,
			longitude = ?,
			map_angle = ?,
			include_map = ?,
			site_description = ?,
			bbox_ne_lat = ?,
			bbox_ne_lng = ?,
			bbox_sw_lat = ?,
			bbox_sw_lng = ?,
			map_rotation = ?,
			map_svg_data = ?
		WHERE id = ?
	`

	includeMapInt := 0
	if site.IncludeMap {
		includeMapInt = 1
	}

	var mapSVGData []byte
	if site.MapSVGData != nil {
		mapSVGData = *site.MapSVGData
	}

	result, err := db.DB.Exec(
		query,
		site.Name,
		site.Location,
		site.Description,
		site.Surveyor,
		site.Contact,
		site.Address,
		site.Latitude,
		site.Longitude,
		site.MapAngle,
		includeMapInt,
		site.SiteDescription,
		site.BBoxNELat,
		site.BBoxNELng,
		site.BBoxSWLat,
		site.BBoxSWLng,
		site.MapRotation,
		mapSVGData,
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

	// No longer auto-update SCD periods from site updates
	// SCD periods should be managed explicitly via SiteConfigPeriod API

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
