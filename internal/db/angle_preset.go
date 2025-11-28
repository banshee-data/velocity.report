package db

import (
	"database/sql"
	"fmt"
)

// AnglePreset represents an angle configuration with color coding
type AnglePreset struct {
	ID        int     `json:"id"`
	Angle     float64 `json:"angle"`
	ColorHex  string  `json:"color_hex"`
	IsSystem  bool    `json:"is_system"`
	CreatedAt float64 `json:"created_at"`
	UpdatedAt float64 `json:"updated_at"`
}

// GetAllAnglePresets retrieves all angle presets
func (db *DB) GetAllAnglePresets() ([]AnglePreset, error) {
	query := `
		SELECT id, angle, color_hex, is_system, created_at, updated_at
		FROM angle_presets
		ORDER BY angle ASC
	`

	rows, err := db.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query angle presets: %w", err)
	}
	defer rows.Close()

	var presets []AnglePreset
	for rows.Next() {
		var preset AnglePreset
		var isSystem int
		err := rows.Scan(
			&preset.ID,
			&preset.Angle,
			&preset.ColorHex,
			&isSystem,
			&preset.CreatedAt,
			&preset.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan angle preset: %w", err)
		}
		preset.IsSystem = isSystem == 1
		presets = append(presets, preset)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating angle presets: %w", err)
	}

	return presets, nil
}

// GetAnglePreset retrieves a specific angle preset by ID
func (db *DB) GetAnglePreset(id int) (*AnglePreset, error) {
	query := `
		SELECT id, angle, color_hex, is_system, created_at, updated_at
		FROM angle_presets
		WHERE id = ?
	`

	var preset AnglePreset
	var isSystem int
	err := db.DB.QueryRow(query, id).Scan(
		&preset.ID,
		&preset.Angle,
		&preset.ColorHex,
		&isSystem,
		&preset.CreatedAt,
		&preset.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("angle preset not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query angle preset: %w", err)
	}

	preset.IsSystem = isSystem == 1
	return &preset, nil
}

// GetAnglePresetByAngle retrieves a preset by angle value
func (db *DB) GetAnglePresetByAngle(angle float64) (*AnglePreset, error) {
	query := `
		SELECT id, angle, color_hex, is_system, created_at, updated_at
		FROM angle_presets
		WHERE angle = ?
	`

	var preset AnglePreset
	var isSystem int
	err := db.DB.QueryRow(query, angle).Scan(
		&preset.ID,
		&preset.Angle,
		&preset.ColorHex,
		&isSystem,
		&preset.CreatedAt,
		&preset.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Not found is OK, return nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query angle preset: %w", err)
	}

	preset.IsSystem = isSystem == 1
	return &preset, nil
}

// CreateAnglePreset creates a new angle preset
func (db *DB) CreateAnglePreset(preset AnglePreset) (*AnglePreset, error) {
	query := `
		INSERT INTO angle_presets (angle, color_hex, is_system)
		VALUES (?, ?, ?)
	`

	isSystem := 0
	if preset.IsSystem {
		isSystem = 1
	}

	result, err := db.DB.Exec(query, preset.Angle, preset.ColorHex, isSystem)
	if err != nil {
		return nil, fmt.Errorf("failed to create angle preset: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get last insert id: %w", err)
	}

	return db.GetAnglePreset(int(id))
}

// UpdateAnglePreset updates an existing angle preset
func (db *DB) UpdateAnglePreset(id int, preset AnglePreset) (*AnglePreset, error) {
	// First check if it's a system preset
	existing, err := db.GetAnglePreset(id)
	if err != nil {
		return nil, err
	}

	if existing.IsSystem {
		return nil, fmt.Errorf("cannot update system preset")
	}

	query := `
		UPDATE angle_presets
		SET angle = ?, color_hex = ?
		WHERE id = ?
	`

	_, err = db.DB.Exec(query, preset.Angle, preset.ColorHex, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update angle preset: %w", err)
	}

	return db.GetAnglePreset(id)
}

// DeleteAnglePreset deletes an angle preset (only non-system presets)
func (db *DB) DeleteAnglePreset(id int) error {
	// The trigger will prevent deletion of system presets
	query := `DELETE FROM angle_presets WHERE id = ?`

	result, err := db.DB.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete angle preset: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("angle preset not found or cannot be deleted: %d", id)
	}

	return nil
}
