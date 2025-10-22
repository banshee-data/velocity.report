package db

import (
	"database/sql"
	"fmt"
	"time"
)

// SiteReport represents a generated PDF report for a site
type SiteReport struct {
	ID          int       `json:"id"`
	SiteID      int       `json:"site_id"`
	StartDate   string    `json:"start_date"`   // YYYY-MM-DD
	EndDate     string    `json:"end_date"`     // YYYY-MM-DD
	Filepath    string    `json:"filepath"`     // Relative path from pdf-generator directory
	Filename    string    `json:"filename"`     // PDF filename
	ZipFilepath *string   `json:"zip_filepath"` // Relative path to sources ZIP
	ZipFilename *string   `json:"zip_filename"` // ZIP filename
	RunID       string    `json:"run_id"`       // Timestamp-based run ID
	Timezone    string    `json:"timezone"`     // Report timezone
	Units       string    `json:"units"`        // mph or kph
	Source      string    `json:"source"`       // radar_objects or radar_data_transits
	CreatedAt   time.Time `json:"created_at"`
}

// CreateSiteReport creates a new report record in the database
func (db *DB) CreateSiteReport(report *SiteReport) error {
	query := `
		INSERT INTO site_reports (
			site_id, start_date, end_date, filepath, filename,
			zip_filepath, zip_filename, run_id, timezone, units, source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.DB.Exec(
		query,
		report.SiteID,
		report.StartDate,
		report.EndDate,
		report.Filepath,
		report.Filename,
		report.ZipFilepath,
		report.ZipFilename,
		report.RunID,
		report.Timezone,
		report.Units,
		report.Source,
	)
	if err != nil {
		return fmt.Errorf("failed to create site report: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	report.ID = int(id)
	return nil
}

// GetSiteReport retrieves a report by ID
func (db *DB) GetSiteReport(id int) (*SiteReport, error) {
	query := `
		SELECT id, site_id, start_date, end_date, filepath, filename,
		       zip_filepath, zip_filename, run_id, timezone, units, source, created_at
		FROM site_reports
		WHERE id = ?
	`

	var report SiteReport
	err := db.DB.QueryRow(query, id).Scan(
		&report.ID,
		&report.SiteID,
		&report.StartDate,
		&report.EndDate,
		&report.Filepath,
		&report.Filename,
		&report.ZipFilepath,
		&report.ZipFilename,
		&report.RunID,
		&report.Timezone,
		&report.Units,
		&report.Source,
		&report.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("report not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get site report: %w", err)
	}

	return &report, nil
}

// GetRecentReportsForSite retrieves the most recent N reports for a specific site
func (db *DB) GetRecentReportsForSite(siteID int, limit int) ([]SiteReport, error) {
	query := `
		SELECT id, site_id, start_date, end_date, filepath, filename,
		       zip_filepath, zip_filename, run_id, timezone, units, source, created_at
		FROM site_reports
		WHERE site_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := db.DB.Query(query, siteID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query site reports: %w", err)
	}
	defer rows.Close()

	var reports []SiteReport
	for rows.Next() {
		var report SiteReport
		err := rows.Scan(
			&report.ID,
			&report.SiteID,
			&report.StartDate,
			&report.EndDate,
			&report.Filepath,
			&report.Filename,
			&report.ZipFilepath,
			&report.ZipFilename,
			&report.RunID,
			&report.Timezone,
			&report.Units,
			&report.Source,
			&report.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site report: %w", err)
		}
		reports = append(reports, report)
	}

	return reports, nil
}

// GetRecentReportsAllSites retrieves the most recent N reports across all sites
func (db *DB) GetRecentReportsAllSites(limit int) ([]SiteReport, error) {
	query := `
		SELECT id, site_id, start_date, end_date, filepath, filename,
		       zip_filepath, zip_filename, run_id, timezone, units, source, created_at
		FROM site_reports
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := db.DB.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query site reports: %w", err)
	}
	defer rows.Close()

	var reports []SiteReport
	for rows.Next() {
		var report SiteReport
		err := rows.Scan(
			&report.ID,
			&report.SiteID,
			&report.StartDate,
			&report.EndDate,
			&report.Filepath,
			&report.Filename,
			&report.ZipFilepath,
			&report.ZipFilename,
			&report.RunID,
			&report.Timezone,
			&report.Units,
			&report.Source,
			&report.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site report: %w", err)
		}
		reports = append(reports, report)
	}

	return reports, nil
}

// DeleteSiteReport deletes a site report by ID
func (db *DB) DeleteSiteReport(id int) error {
	query := `DELETE FROM site_reports WHERE id = ?`

	result, err := db.DB.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete site report: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("report not found")
	}

	return nil
}
