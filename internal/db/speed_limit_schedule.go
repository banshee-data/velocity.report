package db

import (
	"database/sql"
	"fmt"
	"time"
)

// SpeedLimitSchedule represents a time-based speed limit entry for a site
type SpeedLimitSchedule struct {
	ID         int       `json:"id"`
	SiteID     int       `json:"site_id"`
	DayOfWeek  int       `json:"day_of_week"`  // 0=Sunday, 1=Monday, ..., 6=Saturday
	StartTime  string    `json:"start_time"`   // HH:MM format
	EndTime    string    `json:"end_time"`     // HH:MM format
	SpeedLimit int       `json:"speed_limit"`  // Speed limit for this time block
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreateSpeedLimitSchedule creates a new speed limit schedule entry
func (db *DB) CreateSpeedLimitSchedule(schedule *SpeedLimitSchedule) error {
	query := `
		INSERT INTO speed_limit_schedule (
			site_id, day_of_week, start_time, end_time, speed_limit
		) VALUES (?, ?, ?, ?, ?)
	`

	result, err := db.DB.Exec(
		query,
		schedule.SiteID,
		schedule.DayOfWeek,
		schedule.StartTime,
		schedule.EndTime,
		schedule.SpeedLimit,
	)
	if err != nil {
		return fmt.Errorf("failed to create speed limit schedule: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	schedule.ID = int(id)
	return nil
}

// GetSpeedLimitSchedule retrieves a speed limit schedule by ID
func (db *DB) GetSpeedLimitSchedule(id int) (*SpeedLimitSchedule, error) {
	query := `
		SELECT
			id, site_id, day_of_week, start_time, end_time, speed_limit,
			created_at, updated_at
		FROM speed_limit_schedule
		WHERE id = ?
	`

	var schedule SpeedLimitSchedule
	var createdAtUnix, updatedAtUnix int64

	err := db.DB.QueryRow(query, id).Scan(
		&schedule.ID,
		&schedule.SiteID,
		&schedule.DayOfWeek,
		&schedule.StartTime,
		&schedule.EndTime,
		&schedule.SpeedLimit,
		&createdAtUnix,
		&updatedAtUnix,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("speed limit schedule not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get speed limit schedule: %w", err)
	}

	schedule.CreatedAt = time.Unix(createdAtUnix, 0)
	schedule.UpdatedAt = time.Unix(updatedAtUnix, 0)

	return &schedule, nil
}

// GetSpeedLimitSchedulesForSite retrieves all speed limit schedules for a site
func (db *DB) GetSpeedLimitSchedulesForSite(siteID int) ([]SpeedLimitSchedule, error) {
	query := `
		SELECT
			id, site_id, day_of_week, start_time, end_time, speed_limit,
			created_at, updated_at
		FROM speed_limit_schedule
		WHERE site_id = ?
		ORDER BY day_of_week ASC, start_time ASC
	`

	rows, err := db.DB.Query(query, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to query speed limit schedules: %w", err)
	}
	defer rows.Close()

	var schedules []SpeedLimitSchedule
	for rows.Next() {
		var schedule SpeedLimitSchedule
		var createdAtUnix, updatedAtUnix int64

		err := rows.Scan(
			&schedule.ID,
			&schedule.SiteID,
			&schedule.DayOfWeek,
			&schedule.StartTime,
			&schedule.EndTime,
			&schedule.SpeedLimit,
			&createdAtUnix,
			&updatedAtUnix,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan speed limit schedule: %w", err)
		}

		schedule.CreatedAt = time.Unix(createdAtUnix, 0)
		schedule.UpdatedAt = time.Unix(updatedAtUnix, 0)

		schedules = append(schedules, schedule)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating speed limit schedules: %w", err)
	}

	return schedules, nil
}

// UpdateSpeedLimitSchedule updates an existing speed limit schedule
func (db *DB) UpdateSpeedLimitSchedule(schedule *SpeedLimitSchedule) error {
	query := `
		UPDATE speed_limit_schedule SET
			site_id = ?,
			day_of_week = ?,
			start_time = ?,
			end_time = ?,
			speed_limit = ?
		WHERE id = ?
	`

	result, err := db.DB.Exec(
		query,
		schedule.SiteID,
		schedule.DayOfWeek,
		schedule.StartTime,
		schedule.EndTime,
		schedule.SpeedLimit,
		schedule.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update speed limit schedule: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("speed limit schedule not found")
	}

	return nil
}

// DeleteSpeedLimitSchedule deletes a speed limit schedule
func (db *DB) DeleteSpeedLimitSchedule(id int) error {
	query := `DELETE FROM speed_limit_schedule WHERE id = ?`

	result, err := db.DB.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete speed limit schedule: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("speed limit schedule not found")
	}

	return nil
}

// DeleteAllSpeedLimitSchedulesForSite deletes all speed limit schedules for a site
func (db *DB) DeleteAllSpeedLimitSchedulesForSite(siteID int) error {
	query := `DELETE FROM speed_limit_schedule WHERE site_id = ?`

	_, err := db.DB.Exec(query, siteID)
	if err != nil {
		return fmt.Errorf("failed to delete speed limit schedules: %w", err)
	}

	return nil
}
