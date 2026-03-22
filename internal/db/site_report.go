package db

import (
	"context"
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
	Source      string    `json:"source"`       // radar_objects, radar_data, or radar_data_transits
	CreatedAt   time.Time `json:"created_at"`
}

func nullableSiteID(siteID int) interface{} {
	if siteID <= 0 {
		return nil
	}
	return siteID
}

func scanSiteID(dst *int) interface{} {
	return &sql.NullInt64{}
}

func assignScannedSiteID(dst *int, src interface{}) {
	value, ok := src.(*sql.NullInt64)
	if !ok || value == nil || !value.Valid {
		*dst = 0
		return
	}
	*dst = int(value.Int64)
}

// CreateSiteReport creates a new report record in the database
func (db *DB) CreateSiteReport(ctx context.Context, report *SiteReport) error {
	query := `
		INSERT INTO site_reports (
			site_id, start_date, end_date, filepath, filename,
			zip_filepath, zip_filename, run_id, timezone, units, source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.DB.ExecContext(
		ctx,
		query,
		nullableSiteID(report.SiteID),
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
func (db *DB) GetSiteReport(ctx context.Context, id int) (*SiteReport, error) {
	query := `
		SELECT id, site_id, start_date, end_date, filepath, filename,
		       zip_filepath, zip_filename, run_id, timezone, units, source, created_at
		FROM site_reports
		WHERE id = ?
	`

	var report SiteReport
	siteID := scanSiteID(&report.SiteID)
	err := db.DB.QueryRowContext(ctx, query, id).Scan(
		&report.ID,
		siteID,
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
	assignScannedSiteID(&report.SiteID, siteID)

	return &report, nil
}

// GetRecentReportsForSite retrieves the most recent N reports for a specific site
func (db *DB) GetRecentReportsForSite(ctx context.Context, siteID int, limit int) ([]SiteReport, error) {
	query := `
		SELECT id, site_id, start_date, end_date, filepath, filename,
		       zip_filepath, zip_filename, run_id, timezone, units, source, created_at
		FROM site_reports
		WHERE site_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := db.DB.QueryContext(ctx, query, siteID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query site reports: %w", err)
	}
	defer rows.Close()

	var reports []SiteReport
	for rows.Next() {
		var report SiteReport
		siteID := scanSiteID(&report.SiteID)
		err := rows.Scan(
			&report.ID,
			siteID,
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
		assignScannedSiteID(&report.SiteID, siteID)
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate site reports: %w", err)
	}

	return reports, nil
}

// GetRecentReportsAllSites retrieves the most recent N reports across all sites
func (db *DB) GetRecentReportsAllSites(ctx context.Context, limit int) ([]SiteReport, error) {
	query := `
		SELECT id, site_id, start_date, end_date, filepath, filename,
		       zip_filepath, zip_filename, run_id, timezone, units, source, created_at
		FROM site_reports
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := db.DB.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query site reports: %w", err)
	}
	defer rows.Close()

	var reports []SiteReport
	for rows.Next() {
		var report SiteReport
		siteID := scanSiteID(&report.SiteID)
		err := rows.Scan(
			&report.ID,
			siteID,
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
		assignScannedSiteID(&report.SiteID, siteID)
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate site reports: %w", err)
	}

	return reports, nil
}

// DeleteSiteReport deletes a site report by ID
func (db *DB) DeleteSiteReport(ctx context.Context, id int) error {
	query := `DELETE FROM site_reports WHERE id = ?`

	result, err := db.DB.ExecContext(ctx, query, id)
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
