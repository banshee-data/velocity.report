package db

import (
	"database/sql"
	"fmt"
)

// SerialConfig represents a serial port configuration for a radar sensor
type SerialConfig struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	PortPath    string `json:"port_path"`
	BaudRate    int    `json:"baud_rate"`
	DataBits    int    `json:"data_bits"`
	StopBits    int    `json:"stop_bits"`
	Parity      string `json:"parity"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
	SensorModel string `json:"sensor_model"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// GetSerialConfigs returns all serial configurations
func (db *DB) GetSerialConfigs() ([]SerialConfig, error) {
	query := `SELECT id, name, port_path, baud_rate, data_bits, stop_bits, parity, enabled, description, sensor_model, created_at, updated_at
	          FROM radar_serial_config
	          ORDER BY created_at ASC`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query serial configs: %w", err)
	}
	defer rows.Close()

	var configs []SerialConfig
	for rows.Next() {
		var c SerialConfig
		var enabled int
		err := rows.Scan(&c.ID, &c.Name, &c.PortPath, &c.BaudRate, &c.DataBits, &c.StopBits,
			&c.Parity, &enabled, &c.Description, &c.SensorModel, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan serial config: %w", err)
		}
		c.Enabled = enabled == 1
		configs = append(configs, c)
	}

	return configs, nil
}

// GetSerialConfig returns a single serial configuration by ID
func (db *DB) GetSerialConfig(id int) (*SerialConfig, error) {
	query := `SELECT id, name, port_path, baud_rate, data_bits, stop_bits, parity, enabled, description, sensor_model, created_at, updated_at
	          FROM radar_serial_config
	          WHERE id = ?`

	var c SerialConfig
	var enabled int
	err := db.QueryRow(query, id).Scan(&c.ID, &c.Name, &c.PortPath, &c.BaudRate, &c.DataBits,
		&c.StopBits, &c.Parity, &enabled, &c.Description, &c.SensorModel, &c.CreatedAt, &c.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get serial config: %w", err)
	}

	c.Enabled = enabled == 1
	return &c, nil
}

// GetEnabledSerialConfigs returns all enabled serial configurations
func (db *DB) GetEnabledSerialConfigs() ([]SerialConfig, error) {
	query := `SELECT id, name, port_path, baud_rate, data_bits, stop_bits, parity, enabled, description, sensor_model, created_at, updated_at
	          FROM radar_serial_config
	          WHERE enabled = 1
	          ORDER BY created_at ASC`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled serial configs: %w", err)
	}
	defer rows.Close()

	var configs []SerialConfig
	for rows.Next() {
		var c SerialConfig
		var enabled int
		err := rows.Scan(&c.ID, &c.Name, &c.PortPath, &c.BaudRate, &c.DataBits, &c.StopBits,
			&c.Parity, &enabled, &c.Description, &c.SensorModel, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan serial config: %w", err)
		}
		c.Enabled = enabled == 1
		configs = append(configs, c)
	}

	return configs, nil
}

// CreateSerialConfig creates a new serial configuration
func (db *DB) CreateSerialConfig(c *SerialConfig) (int64, error) {
	query := `INSERT INTO radar_serial_config (name, port_path, baud_rate, data_bits, stop_bits, parity, enabled, description, sensor_model)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	enabled := 0
	if c.Enabled {
		enabled = 1
	}

	result, err := db.Exec(query, c.Name, c.PortPath, c.BaudRate, c.DataBits, c.StopBits,
		c.Parity, enabled, c.Description, c.SensorModel)
	if err != nil {
		return 0, fmt.Errorf("failed to create serial config: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return id, nil
}

// UpdateSerialConfig updates an existing serial configuration
func (db *DB) UpdateSerialConfig(c *SerialConfig) error {
	query := `UPDATE radar_serial_config
	          SET name = ?, port_path = ?, baud_rate = ?, data_bits = ?, stop_bits = ?,
	              parity = ?, enabled = ?, description = ?, sensor_model = ?
	          WHERE id = ?`

	enabled := 0
	if c.Enabled {
		enabled = 1
	}

	result, err := db.Exec(query, c.Name, c.PortPath, c.BaudRate, c.DataBits, c.StopBits,
		c.Parity, enabled, c.Description, c.SensorModel, c.ID)
	if err != nil {
		return fmt.Errorf("failed to update serial config: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("serial config with ID %d not found", c.ID)
	}

	return nil
}

// DeleteSerialConfig deletes a serial configuration
func (db *DB) DeleteSerialConfig(id int) error {
	query := `DELETE FROM radar_serial_config WHERE id = ?`

	result, err := db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete serial config: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("serial config with ID %d not found", id)
	}

	return nil
}
