package db

import (
	"path/filepath"
	"testing"
)

// TestCreateSiteReport_InvalidData tests report creation with edge cases
func TestCreateSiteReport_InvalidData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// First create a site to associate reports with
	site := Site{
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Test",
		Contact:  "test@example.com",
	}
	err = db.CreateSite(&site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	testCases := []struct {
		name   string
		report SiteReport
	}{
		{
			name: "empty_dates",
			report: SiteReport{
				SiteID:    site.ID,
				StartDate: "",
				EndDate:   "",
				Filepath:  "/path/to/report.pdf",
				Filename:  "report.pdf",
				RunID:     "123",
				Timezone:  "UTC",
				Units:     "mph",
				Source:    "radar_data",
			},
		},
		{
			name: "invalid_date_format",
			report: SiteReport{
				SiteID:    site.ID,
				StartDate: "2024/01/01", // Wrong format
				EndDate:   "01-31-2024", // Wrong format
				Filepath:  "/path/to/report.pdf",
				Filename:  "report.pdf",
				RunID:     "123",
				Timezone:  "UTC",
				Units:     "mph",
				Source:    "radar_data",
			},
		},
		{
			name: "end_before_start",
			report: SiteReport{
				SiteID:    site.ID,
				StartDate: "2024-12-31",
				EndDate:   "2024-01-01", // Before start
				Filepath:  "/path/to/report.pdf",
				Filename:  "report.pdf",
				RunID:     "123",
				Timezone:  "UTC",
				Units:     "mph",
				Source:    "radar_data",
			},
		},
		{
			name: "empty_filepath",
			report: SiteReport{
				SiteID:    site.ID,
				StartDate: "2024-01-01",
				EndDate:   "2024-01-31",
				Filepath:  "",
				Filename:  "report.pdf",
				RunID:     "123",
				Timezone:  "UTC",
				Units:     "mph",
				Source:    "radar_data",
			},
		},
		{
			name: "very_long_path",
			report: SiteReport{
				SiteID:    site.ID,
				StartDate: "2024-01-01",
				EndDate:   "2024-01-31",
				Filepath:  string(make([]byte, 10000)), // Very long path
				Filename:  "report.pdf",
				RunID:     "123",
				Timezone:  "UTC",
				Units:     "mph",
				Source:    "radar_data",
			},
		},
		{
			name: "special_chars_in_path",
			report: SiteReport{
				SiteID:    site.ID,
				StartDate: "2024-01-01",
				EndDate:   "2024-01-31",
				Filepath:  "/path/with spaces/and'quotes/and\"doubles.pdf",
				Filename:  "file with spaces.pdf",
				RunID:     "123",
				Timezone:  "UTC",
				Units:     "mph",
				Source:    "radar_data",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := db.CreateSiteReport(&tc.report)
			// Just verify no panics occur
			t.Logf("CreateSiteReport result: err=%v, ID=%d", err, tc.report.ID)
		})
	}
}

// TestGetSiteReport_NonexistentID tests retrieval of non-existent report
func TestGetSiteReport_NonexistentID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	report, err := db.GetSiteReport(99999)
	if err == nil {
		t.Errorf("Expected error for non-existent report, got: %+v", report)
	}
}

// TestGetSiteReport_NegativeID tests retrieval with negative ID
func TestGetSiteReport_NegativeID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	report, err := db.GetSiteReport(-1)
	if err == nil {
		t.Errorf("Expected error for negative ID, got: %+v", report)
	}
}

// TestGetRecentReportsForSite_EmptyDatabase tests getting reports when none exist
func TestGetRecentReportsForSite_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	reports, err := db.GetRecentReportsForSite(1, 10)
	if err != nil {
		t.Fatalf("GetRecentReportsForSite failed: %v", err)
	}

	if len(reports) != 0 {
		t.Errorf("Expected empty list, got %d reports", len(reports))
	}
}

// TestGetRecentReportsForSite_VaryingLimits tests with different limit values
func TestGetRecentReportsForSite_VaryingLimits(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a site and multiple reports
	site := Site{
		Name:     "Multi Report Site",
		Location: "Test",
		Surveyor: "Test",
		Contact:  "test@example.com",
	}
	db.CreateSite(&site)

	for i := 0; i < 10; i++ {
		report := SiteReport{
			SiteID:    site.ID,
			StartDate: "2024-01-01",
			EndDate:   "2024-01-31",
			Filepath:  "/path/report.pdf",
			Filename:  "report.pdf",
			RunID:     "run_" + string(rune('a'+i)),
			Timezone:  "UTC",
			Units:     "mph",
			Source:    "radar_data",
		}
		db.CreateSiteReport(&report)
	}

	testCases := []struct {
		name          string
		limit         int
		expectedCount int
	}{
		{"limit_0", 0, 0},
		{"limit_1", 1, 1},
		{"limit_5", 5, 5},
		{"limit_10", 10, 10},
		{"limit_100", 100, 10}, // More than available
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reports, err := db.GetRecentReportsForSite(site.ID, tc.limit)
			if err != nil {
				t.Fatalf("GetRecentReportsForSite failed: %v", err)
			}
			if len(reports) != tc.expectedCount {
				t.Errorf("Expected %d reports, got %d", tc.expectedCount, len(reports))
			}
		})
	}
}

// TestGetRecentReportsAllSites_EmptyDatabase tests getting all reports when none exist
func TestGetRecentReportsAllSites_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	reports, err := db.GetRecentReportsAllSites(10)
	if err != nil {
		t.Fatalf("GetRecentReportsAllSites failed: %v", err)
	}

	if len(reports) != 0 {
		t.Errorf("Expected empty list, got %d reports", len(reports))
	}
}

// TestDeleteSiteReport_NonexistentID tests deleting non-existent report
func TestDeleteSiteReport_NonexistentID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	err = db.DeleteSiteReport(99999)
	// Should handle gracefully
	t.Logf("DeleteSiteReport for non-existent ID result: %v", err)
}

// TestCreateSiteReport_ForNonexistentSite tests creating report for non-existent site
func TestCreateSiteReport_ForNonexistentSite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	report := SiteReport{
		SiteID:    99999, // Non-existent site
		StartDate: "2024-01-01",
		EndDate:   "2024-01-31",
		Filepath:  "/path/to/report.pdf",
		Filename:  "report.pdf",
		RunID:     "123",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_data",
	}

	err = db.CreateSiteReport(&report)
	// May succeed (no foreign key) or fail depending on schema
	t.Logf("CreateSiteReport for non-existent site: err=%v, ID=%d", err, report.ID)
}

// TestCreateSiteReport_WithOptionalZipFields tests reports with and without zip files
func TestCreateSiteReport_WithOptionalZipFields(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	site := Site{
		Name:     "Test Site",
		Location: "Test",
		Surveyor: "Test",
		Contact:  "test@example.com",
	}
	db.CreateSite(&site)

	// Report without zip
	reportNoZip := SiteReport{
		SiteID:      site.ID,
		StartDate:   "2024-01-01",
		EndDate:     "2024-01-31",
		Filepath:    "/path/to/report.pdf",
		Filename:    "report.pdf",
		ZipFilepath: nil,
		ZipFilename: nil,
		RunID:       "no_zip",
		Timezone:    "UTC",
		Units:       "mph",
		Source:      "radar_data",
	}

	err = db.CreateSiteReport(&reportNoZip)
	if err != nil {
		t.Fatalf("CreateSiteReport without zip failed: %v", err)
	}

	// Report with zip
	zipPath := "/path/to/sources.zip"
	zipName := "sources.zip"
	reportWithZip := SiteReport{
		SiteID:      site.ID,
		StartDate:   "2024-02-01",
		EndDate:     "2024-02-28",
		Filepath:    "/path/to/report2.pdf",
		Filename:    "report2.pdf",
		ZipFilepath: &zipPath,
		ZipFilename: &zipName,
		RunID:       "with_zip",
		Timezone:    "UTC",
		Units:       "kph",
		Source:      "radar_data_transits",
	}

	err = db.CreateSiteReport(&reportWithZip)
	if err != nil {
		t.Fatalf("CreateSiteReport with zip failed: %v", err)
	}

	// Verify retrieval
	retrieved, err := db.GetSiteReport(reportWithZip.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve report: %v", err)
	}

	if retrieved.ZipFilepath == nil || *retrieved.ZipFilepath != zipPath {
		t.Errorf("ZipFilepath mismatch: expected %q, got %v", zipPath, retrieved.ZipFilepath)
	}
}

// TestCreateSiteReport_DifferentSources tests reports with different source values
func TestCreateSiteReport_DifferentSources(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	site := Site{
		Name:     "Test Site",
		Location: "Test",
		Surveyor: "Test",
		Contact:  "test@example.com",
	}
	db.CreateSite(&site)

	sources := []string{
		"radar_objects",
		"radar_data",
		"radar_data_transits",
		"unknown_source", // Edge case
		"",               // Empty source
	}

	for _, source := range sources {
		t.Run("source_"+source, func(t *testing.T) {
			report := SiteReport{
				SiteID:    site.ID,
				StartDate: "2024-01-01",
				EndDate:   "2024-01-31",
				Filepath:  "/path/report.pdf",
				Filename:  "report.pdf",
				RunID:     "run_" + source,
				Timezone:  "UTC",
				Units:     "mph",
				Source:    source,
			}

			err := db.CreateSiteReport(&report)
			t.Logf("CreateSiteReport with source %q: err=%v", source, err)
		})
	}
}

// TestCreateSiteReport_DifferentUnits tests reports with different unit values
func TestCreateSiteReport_DifferentUnits(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	site := Site{
		Name:     "Test Site",
		Location: "Test",
		Surveyor: "Test",
		Contact:  "test@example.com",
	}
	db.CreateSite(&site)

	units := []string{"mph", "kph", "km/h", "m/s", ""}

	for _, unit := range units {
		t.Run("units_"+unit, func(t *testing.T) {
			report := SiteReport{
				SiteID:    site.ID,
				StartDate: "2024-01-01",
				EndDate:   "2024-01-31",
				Filepath:  "/path/report.pdf",
				Filename:  "report.pdf",
				RunID:     "run_" + unit,
				Timezone:  "UTC",
				Units:     unit,
				Source:    "radar_data",
			}

			err := db.CreateSiteReport(&report)
			t.Logf("CreateSiteReport with units %q: err=%v", unit, err)
		})
	}
}

// TestCreateSiteReport_DifferentTimezones tests reports with various timezones
func TestCreateSiteReport_DifferentTimezones(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	site := Site{
		Name:     "Test Site",
		Location: "Test",
		Surveyor: "Test",
		Contact:  "test@example.com",
	}
	db.CreateSite(&site)

	timezones := []string{
		"UTC",
		"America/New_York",
		"Europe/London",
		"Asia/Tokyo",
		"Australia/Sydney",
		"", // Empty timezone
	}

	for _, tz := range timezones {
		t.Run("tz_"+tz, func(t *testing.T) {
			report := SiteReport{
				SiteID:    site.ID,
				StartDate: "2024-01-01",
				EndDate:   "2024-01-31",
				Filepath:  "/path/report.pdf",
				Filename:  "report.pdf",
				RunID:     "run_" + tz,
				Timezone:  tz,
				Units:     "mph",
				Source:    "radar_data",
			}

			err := db.CreateSiteReport(&report)
			t.Logf("CreateSiteReport with timezone %q: err=%v", tz, err)
		})
	}
}
