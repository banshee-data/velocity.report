package db

import (
	"testing"
)

func setupSiteReportTestDB(t *testing.T) *DB {
	t.Helper()
	fname := t.TempDir() + "/test_site_reports.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	return db
}

// TestCreateSiteReport tests creating a new site report
func TestCreateSiteReport(t *testing.T) {
	db := setupSiteReportTestDB(t)
	defer db.Close()

	// First create a site to reference
	site := &Site{
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Tester",
		Contact:  "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	report := &SiteReport{
		SiteID:    site.ID,
		StartDate: "2024-01-01",
		EndDate:   "2024-01-07",
		Filepath:  "output/20240101/report.pdf",
		Filename:  "report.pdf",
		RunID:     "20240101-120000",
		Timezone:  "US/Pacific",
		Units:     "mph",
		Source:    "radar_objects",
	}

	err := db.CreateSiteReport(report)
	if err != nil {
		t.Fatalf("CreateSiteReport failed: %v", err)
	}

	if report.ID == 0 {
		t.Error("Expected report ID to be set")
	}
}

// TestCreateSiteReport_WithZip tests creating a report with ZIP file
func TestCreateSiteReport_WithZip(t *testing.T) {
	db := setupSiteReportTestDB(t)
	defer db.Close()

	zipPath := "output/20240101/sources.zip"
	zipName := "sources.zip"
	report := &SiteReport{
		SiteID:      0, // No associated site
		StartDate:   "2024-01-01",
		EndDate:     "2024-01-07",
		Filepath:    "output/20240101/report.pdf",
		Filename:    "report.pdf",
		ZipFilepath: &zipPath,
		ZipFilename: &zipName,
		RunID:       "20240101-120000",
		Timezone:    "UTC",
		Units:       "kph",
		Source:      "radar_data_transits",
	}

	err := db.CreateSiteReport(report)
	if err != nil {
		t.Fatalf("CreateSiteReport failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := db.GetSiteReport(report.ID)
	if err != nil {
		t.Fatalf("GetSiteReport failed: %v", err)
	}

	if retrieved.ZipFilepath == nil || *retrieved.ZipFilepath != zipPath {
		t.Errorf("Expected ZipFilepath %q, got %v", zipPath, retrieved.ZipFilepath)
	}
	if retrieved.ZipFilename == nil || *retrieved.ZipFilename != zipName {
		t.Errorf("Expected ZipFilename %q, got %v", zipName, retrieved.ZipFilename)
	}
}

// TestGetSiteReport tests retrieving a site report by ID
func TestGetSiteReport(t *testing.T) {
	db := setupSiteReportTestDB(t)
	defer db.Close()

	report := &SiteReport{
		SiteID:    0,
		StartDate: "2024-02-01",
		EndDate:   "2024-02-07",
		Filepath:  "output/20240201/report.pdf",
		Filename:  "report.pdf",
		RunID:     "20240201-120000",
		Timezone:  "US/Eastern",
		Units:     "mph",
		Source:    "radar_objects",
	}

	if err := db.CreateSiteReport(report); err != nil {
		t.Fatalf("CreateSiteReport failed: %v", err)
	}

	retrieved, err := db.GetSiteReport(report.ID)
	if err != nil {
		t.Fatalf("GetSiteReport failed: %v", err)
	}

	if retrieved.ID != report.ID {
		t.Errorf("Expected ID %d, got %d", report.ID, retrieved.ID)
	}
	if retrieved.StartDate != "2024-02-01" {
		t.Errorf("Expected StartDate '2024-02-01', got '%s'", retrieved.StartDate)
	}
	if retrieved.EndDate != "2024-02-07" {
		t.Errorf("Expected EndDate '2024-02-07', got '%s'", retrieved.EndDate)
	}
	if retrieved.Timezone != "US/Eastern" {
		t.Errorf("Expected Timezone 'US/Eastern', got '%s'", retrieved.Timezone)
	}
	if retrieved.Units != "mph" {
		t.Errorf("Expected Units 'mph', got '%s'", retrieved.Units)
	}
	if retrieved.Source != "radar_objects" {
		t.Errorf("Expected Source 'radar_objects', got '%s'", retrieved.Source)
	}
	if retrieved.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
}

// TestGetSiteReport_NotFound tests retrieving a non-existent report
func TestGetSiteReport_NotFound(t *testing.T) {
	db := setupSiteReportTestDB(t)
	defer db.Close()

	_, err := db.GetSiteReport(99999)
	if err == nil {
		t.Error("Expected error for non-existent report")
	}
	if err.Error() != "report not found" {
		t.Errorf("Expected 'report not found' error, got '%s'", err.Error())
	}
}

// TestGetRecentReportsForSite tests getting reports for a specific site
func TestGetRecentReportsForSite(t *testing.T) {
	db := setupSiteReportTestDB(t)
	defer db.Close()

	// Create a site
	site := &Site{
		Name:     "Test Site",
		Location: "Test Location",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create multiple reports for this site
	for i := 0; i < 3; i++ {
		report := &SiteReport{
			SiteID:    site.ID,
			StartDate: "2024-01-01",
			EndDate:   "2024-01-07",
			Filepath:  "output/report.pdf",
			Filename:  "report.pdf",
			RunID:     "run-id",
			Timezone:  "UTC",
			Units:     "mph",
			Source:    "radar_objects",
		}
		if err := db.CreateSiteReport(report); err != nil {
			t.Fatalf("CreateSiteReport failed: %v", err)
		}
	}

	// Create a report for another site
	otherReport := &SiteReport{
		SiteID:    0, // Different site
		StartDate: "2024-01-01",
		EndDate:   "2024-01-07",
		Filepath:  "output/other.pdf",
		Filename:  "other.pdf",
		RunID:     "other-run",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}
	if err := db.CreateSiteReport(otherReport); err != nil {
		t.Fatalf("CreateSiteReport failed: %v", err)
	}

	// Get reports for specific site
	reports, err := db.GetRecentReportsForSite(site.ID, 10)
	if err != nil {
		t.Fatalf("GetRecentReportsForSite failed: %v", err)
	}

	if len(reports) != 3 {
		t.Errorf("Expected 3 reports, got %d", len(reports))
	}

	// Verify all reports belong to the correct site
	for _, r := range reports {
		if r.SiteID != site.ID {
			t.Errorf("Expected SiteID %d, got %d", site.ID, r.SiteID)
		}
	}
}

// TestGetRecentReportsForSite_Limit tests the limit parameter
func TestGetRecentReportsForSite_Limit(t *testing.T) {
	db := setupSiteReportTestDB(t)
	defer db.Close()

	// Create 5 reports
	for i := 0; i < 5; i++ {
		report := &SiteReport{
			SiteID:    0,
			StartDate: "2024-01-01",
			EndDate:   "2024-01-07",
			Filepath:  "output/report.pdf",
			Filename:  "report.pdf",
			RunID:     "run-id",
			Timezone:  "UTC",
			Units:     "mph",
			Source:    "radar_objects",
		}
		if err := db.CreateSiteReport(report); err != nil {
			t.Fatalf("CreateSiteReport failed: %v", err)
		}
	}

	// Get only 2 reports
	reports, err := db.GetRecentReportsForSite(0, 2)
	if err != nil {
		t.Fatalf("GetRecentReportsForSite failed: %v", err)
	}

	if len(reports) != 2 {
		t.Errorf("Expected 2 reports, got %d", len(reports))
	}
}

// TestGetRecentReportsAllSites tests getting reports across all sites
func TestGetRecentReportsAllSites(t *testing.T) {
	db := setupSiteReportTestDB(t)
	defer db.Close()

	// Create reports for different sites
	for i := 0; i < 3; i++ {
		report := &SiteReport{
			SiteID:    i,
			StartDate: "2024-01-01",
			EndDate:   "2024-01-07",
			Filepath:  "output/report.pdf",
			Filename:  "report.pdf",
			RunID:     "run-id",
			Timezone:  "UTC",
			Units:     "mph",
			Source:    "radar_objects",
		}
		if err := db.CreateSiteReport(report); err != nil {
			t.Fatalf("CreateSiteReport failed: %v", err)
		}
	}

	reports, err := db.GetRecentReportsAllSites(10)
	if err != nil {
		t.Fatalf("GetRecentReportsAllSites failed: %v", err)
	}

	if len(reports) != 3 {
		t.Errorf("Expected 3 reports, got %d", len(reports))
	}
}

// TestGetRecentReportsAllSites_Limit tests the limit parameter
func TestGetRecentReportsAllSites_Limit(t *testing.T) {
	db := setupSiteReportTestDB(t)
	defer db.Close()

	// Create 5 reports
	for i := 0; i < 5; i++ {
		report := &SiteReport{
			SiteID:    0,
			StartDate: "2024-01-01",
			EndDate:   "2024-01-07",
			Filepath:  "output/report.pdf",
			Filename:  "report.pdf",
			RunID:     "run-id",
			Timezone:  "UTC",
			Units:     "mph",
			Source:    "radar_objects",
		}
		if err := db.CreateSiteReport(report); err != nil {
			t.Fatalf("CreateSiteReport failed: %v", err)
		}
	}

	// Get only 3 reports
	reports, err := db.GetRecentReportsAllSites(3)
	if err != nil {
		t.Fatalf("GetRecentReportsAllSites failed: %v", err)
	}

	if len(reports) != 3 {
		t.Errorf("Expected 3 reports, got %d", len(reports))
	}
}

// TestDeleteSiteReport tests deleting a site report
func TestDeleteSiteReport(t *testing.T) {
	db := setupSiteReportTestDB(t)
	defer db.Close()

	report := &SiteReport{
		SiteID:    0,
		StartDate: "2024-01-01",
		EndDate:   "2024-01-07",
		Filepath:  "output/report.pdf",
		Filename:  "report.pdf",
		RunID:     "run-id",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}

	if err := db.CreateSiteReport(report); err != nil {
		t.Fatalf("CreateSiteReport failed: %v", err)
	}

	// Delete the report
	err := db.DeleteSiteReport(report.ID)
	if err != nil {
		t.Fatalf("DeleteSiteReport failed: %v", err)
	}

	// Verify it's deleted
	_, err = db.GetSiteReport(report.ID)
	if err == nil {
		t.Error("Expected error when getting deleted report")
	}
}

// TestDeleteSiteReport_NotFound tests deleting a non-existent report
func TestDeleteSiteReport_NotFound(t *testing.T) {
	db := setupSiteReportTestDB(t)
	defer db.Close()

	err := db.DeleteSiteReport(99999)
	if err == nil {
		t.Error("Expected error for non-existent report")
	}
	if err.Error() != "report not found" {
		t.Errorf("Expected 'report not found' error, got '%s'", err.Error())
	}
}

// TestGetRecentReportsForSite_EmptyResult tests empty result handling
func TestGetRecentReportsForSite_EmptyResult(t *testing.T) {
	db := setupSiteReportTestDB(t)
	defer db.Close()

	reports, err := db.GetRecentReportsForSite(99999, 10)
	if err != nil {
		t.Fatalf("GetRecentReportsForSite failed: %v", err)
	}

	// Empty result should have length 0 (can be nil or empty slice)
	if len(reports) != 0 {
		t.Errorf("Expected 0 reports, got %d", len(reports))
	}
}

// TestGetRecentReportsAllSites_EmptyResult tests empty result handling
func TestGetRecentReportsAllSites_EmptyResult(t *testing.T) {
	fname := t.TempDir() + "/empty_reports.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	reports, err := db.GetRecentReportsAllSites(10)
	if err != nil {
		t.Fatalf("GetRecentReportsAllSites failed: %v", err)
	}

	// Empty result should have length 0 (can be nil or empty slice)
	if len(reports) != 0 {
		t.Errorf("Expected 0 reports, got %d", len(reports))
	}
}
