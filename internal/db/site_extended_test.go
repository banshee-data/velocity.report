package db

import (
	"path/filepath"
	"testing"
	"time"
)

// TestSiteReport_CreateAndRetrieve tests creating and retrieving site reports
func TestSiteReport_CreateAndRetrieve(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a site first
	site := &Site{
		Name:      "Test Site",
		Address:   strPtr("123 Test St"),
		Latitude:  floatPtr(51.5074),
		Longitude: floatPtr(-0.1278),
	}

	err = db.CreateSite(site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create a report using the actual SiteReport structure
	report := &SiteReport{
		SiteID:    site.ID,
		StartDate: "2025-01-01",
		EndDate:   "2025-01-07",
		Filepath:  "/reports/test_report.pdf",
		Filename:  "test_report.pdf",
		RunID:     "run-123",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}

	err = db.CreateSiteReport(report)
	if err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	if report.ID <= 0 {
		t.Errorf("Expected positive report ID, got %d", report.ID)
	}

	// Retrieve the report
	retrieved, err := db.GetSiteReport(report.ID)
	if err != nil {
		t.Fatalf("Failed to get report: %v", err)
	}

	if retrieved.SiteID != site.ID {
		t.Errorf("Expected site ID %d, got %d", site.ID, retrieved.SiteID)
	}
	if retrieved.Source != "radar_objects" {
		t.Errorf("Expected source 'radar_objects', got '%s'", retrieved.Source)
	}
}

// TestSiteReport_GetRecentForSite tests getting recent reports for a site
func TestSiteReport_GetRecentForSite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a site
	site := &Site{
		Name: "Test Site",
	}
	err = db.CreateSite(site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create multiple reports
	for i := 0; i < 5; i++ {
		report := &SiteReport{
			SiteID:    site.ID,
			StartDate: "2025-01-01",
			EndDate:   "2025-01-07",
			Filepath:  "/reports/report.pdf",
			Filename:  "report.pdf",
			RunID:     "run-" + string(rune('0'+i)),
			Timezone:  "UTC",
			Units:     "mph",
			Source:    "radar_objects",
		}
		if err := db.CreateSiteReport(report); err != nil {
			t.Fatalf("Failed to create report: %v", err)
		}
	}

	// Get recent reports
	reports, err := db.GetRecentReportsForSite(site.ID, 3)
	if err != nil {
		t.Fatalf("GetRecentReportsForSite failed: %v", err)
	}

	if len(reports) != 3 {
		t.Errorf("Expected 3 reports, got %d", len(reports))
	}
}

// TestSiteReport_GetRecentAllSites tests getting recent reports across all sites
func TestSiteReport_GetRecentAllSites(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create multiple sites with reports
	for i := 0; i < 3; i++ {
		site := &Site{
			Name: "Test Site " + string(rune('A'+i)),
		}
		err := db.CreateSite(site)
		if err != nil {
			t.Fatalf("Failed to create site: %v", err)
		}

		report := &SiteReport{
			SiteID:    site.ID,
			StartDate: "2025-01-01",
			EndDate:   "2025-01-07",
			Filepath:  "/reports/report.pdf",
			Filename:  "report.pdf",
			RunID:     "run-" + string(rune('0'+i)),
			Timezone:  "UTC",
			Units:     "mph",
			Source:    "radar_objects",
		}
		if err := db.CreateSiteReport(report); err != nil {
			t.Fatalf("Failed to create report: %v", err)
		}
	}

	// Get all recent reports
	reports, err := db.GetRecentReportsAllSites(10)
	if err != nil {
		t.Fatalf("GetRecentReportsAllSites failed: %v", err)
	}

	if len(reports) != 3 {
		t.Errorf("Expected 3 reports, got %d", len(reports))
	}
}

// TestSiteReport_Delete tests deleting a report
func TestSiteReport_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a site and report
	site := &Site{Name: "Test Site"}
	err = db.CreateSite(site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	report := &SiteReport{
		SiteID:    site.ID,
		StartDate: "2025-01-01",
		EndDate:   "2025-01-07",
		Filepath:  "/reports/report.pdf",
		Filename:  "report.pdf",
		RunID:     "run-123",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_objects",
	}
	if err := db.CreateSiteReport(report); err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	// Delete the report
	if err := db.DeleteSiteReport(report.ID); err != nil {
		t.Fatalf("DeleteSiteReport failed: %v", err)
	}

	// Verify it's deleted - GetSiteReport returns error when not found
	_, err = db.GetSiteReport(report.ID)
	if err == nil {
		t.Error("Expected error after deletion, got nil")
	}
}

// TestSite_UpdateSite tests updating a site
func TestSite_UpdateSite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a site
	site := &Site{
		Name:    "Original Name",
		Address: strPtr("Original Address"),
	}
	err = db.CreateSite(site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Update the site
	update := &Site{
		ID:      site.ID,
		Name:    "Updated Name",
		Address: strPtr("Updated Address"),
	}
	if err := db.UpdateSite(update); err != nil {
		t.Fatalf("UpdateSite failed: %v", err)
	}

	// Verify update
	retrieved, err := db.GetSite(site.ID)
	if err != nil {
		t.Fatalf("GetSite failed: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", retrieved.Name)
	}
	if retrieved.Address == nil || *retrieved.Address != "Updated Address" {
		t.Error("Address not updated correctly")
	}
}

// TestSite_DeleteSite tests deleting a site
func TestSite_DeleteSite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a site
	site := &Site{Name: "Test Site"}
	err = db.CreateSite(site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Delete the site
	if err := db.DeleteSite(site.ID); err != nil {
		t.Fatalf("DeleteSite failed: %v", err)
	}

	// Verify it's deleted - GetSite returns error when not found
	_, err = db.GetSite(site.ID)
	if err == nil {
		t.Error("Expected error after deletion, got nil")
	}
}

// TestSite_GetAllSites tests getting all sites
func TestSite_GetAllSitesWithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create multiple sites
	for i := 0; i < 5; i++ {
		site := &Site{
			Name:      "Site " + string(rune('A'+i)),
			Latitude:  floatPtr(51.5074 + float64(i)*0.01),
			Longitude: floatPtr(-0.1278 + float64(i)*0.01),
		}
		if err := db.CreateSite(site); err != nil {
			t.Fatalf("Failed to create site: %v", err)
		}
	}

	// Get all sites
	sites, err := db.GetAllSites()
	if err != nil {
		t.Fatalf("GetAllSites failed: %v", err)
	}

	if len(sites) != 5 {
		t.Errorf("Expected 5 sites, got %d", len(sites))
	}
}

// TestSiteConfigPeriod_GetActive tests getting the active config period
func TestSiteConfigPeriod_GetActive(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a site
	site := &Site{Name: "Test Site"}
	err = db.CreateSite(site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create a config period
	configPeriod := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: float64(time.Now().Add(-24 * time.Hour).Unix()),
		EffectiveEndUnix:   nil, // Still active
		CosineErrorAngle:   15.0,
		IsActive:           true,
	}
	if err := db.CreateSiteConfigPeriod(configPeriod); err != nil {
		t.Fatalf("Failed to create config period: %v", err)
	}

	// Get active config period
	active, err := db.GetActiveSiteConfigPeriod(site.ID)
	if err != nil {
		t.Fatalf("GetActiveSiteConfigPeriod failed: %v", err)
	}

	if active == nil {
		t.Fatal("Expected active config period")
	}

	if active.SiteID != site.ID {
		t.Errorf("Expected site ID %d, got %d", site.ID, active.SiteID)
	}
}

// TestSiteConfigPeriod_List tests listing config periods
func TestSiteConfigPeriod_List(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a site
	site := &Site{Name: "Test Site"}
	err = db.CreateSite(site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create multiple config periods
	now := time.Now()
	periods := []struct {
		start time.Duration
		end   *time.Duration
	}{
		{-72 * time.Hour, ptrDuration(-48 * time.Hour)},
		{-48 * time.Hour, ptrDuration(-24 * time.Hour)},
		{-24 * time.Hour, nil},
	}

	for _, p := range periods {
		var endUnix *float64
		if p.end != nil {
			e := float64(now.Add(*p.end).Unix())
			endUnix = &e
		}
		configPeriod := &SiteConfigPeriod{
			SiteID:             site.ID,
			EffectiveStartUnix: float64(now.Add(p.start).Unix()),
			EffectiveEndUnix:   endUnix,
			CosineErrorAngle:   15.0,
		}
		if err := db.CreateSiteConfigPeriod(configPeriod); err != nil {
			t.Fatalf("Failed to create config period: %v", err)
		}
	}

	// List config periods
	list, err := db.ListSiteConfigPeriods(&site.ID)
	if err != nil {
		t.Fatalf("ListSiteConfigPeriods failed: %v", err)
	}

	if len(list) != 3 {
		t.Errorf("Expected 3 config periods, got %d", len(list))
	}
}

// TestSiteConfigPeriod_Update tests updating a config period
func TestSiteConfigPeriod_Update(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a site
	site := &Site{Name: "Test Site"}
	err = db.CreateSite(site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create a config period
	configPeriod := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: float64(time.Now().Add(-24 * time.Hour).Unix()),
		EffectiveEndUnix:   nil,
		CosineErrorAngle:   15.0,
	}
	err = db.CreateSiteConfigPeriod(configPeriod)
	if err != nil {
		t.Fatalf("Failed to create config period: %v", err)
	}

	// Update the config period
	update := &SiteConfigPeriod{
		ID:                 configPeriod.ID,
		SiteID:             site.ID,
		EffectiveStartUnix: float64(time.Now().Add(-48 * time.Hour).Unix()),
		EffectiveEndUnix:   nil,
		CosineErrorAngle:   20.0,
	}
	if err := db.UpdateSiteConfigPeriod(update); err != nil {
		t.Fatalf("UpdateSiteConfigPeriod failed: %v", err)
	}

	// Verify update
	retrieved, err := db.GetSiteConfigPeriod(configPeriod.ID)
	if err != nil {
		t.Fatalf("GetSiteConfigPeriod failed: %v", err)
	}

	if retrieved.CosineErrorAngle != 20.0 {
		t.Errorf("CosineErrorAngle not updated correctly, expected 20.0, got %f", retrieved.CosineErrorAngle)
	}
}

// ptrDuration returns a pointer to a time.Duration value
func ptrDuration(d time.Duration) *time.Duration {
	return &d
}

// TestSite_UpdateSite_NotFound tests updating a non-existent site
func TestSite_UpdateSite_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Try to update a site that doesn't exist
	update := &Site{
		ID:   99999, // Non-existent ID
		Name: "Non-existent Site",
	}
	err = db.UpdateSite(update)
	if err == nil {
		t.Error("Expected error for non-existent site, got nil")
	}
}

// TestSite_DeleteSite_NotFound tests deleting a non-existent site
func TestSite_DeleteSite_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Try to delete a site that doesn't exist
	err = db.DeleteSite(99999)
	if err == nil {
		t.Error("Expected error for non-existent site, got nil")
	}
}

// TestSiteReport_DeleteSiteReport_NotFound tests deleting a non-existent report
func TestSiteReport_DeleteSiteReport_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Try to delete a report that doesn't exist
	err = db.DeleteSiteReport(99999)
	if err == nil {
		t.Error("Expected error for non-existent report, got nil")
	}
}

// TestSite_CreateSite_WithAllFields tests creating a site with all fields populated
func TestSite_CreateSite_WithAllFields(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	site := &Site{
		Name:            "Full Test Site",
		Location:        "Test Location",
		Description:     strPtr("Test Description"),
		Surveyor:        "Test Surveyor",
		Contact:         "test@example.com",
		Address:         strPtr("123 Test St"),
		Latitude:        floatPtr(51.5074),
		Longitude:       floatPtr(-0.1278),
		MapAngle:        floatPtr(45.0),
		IncludeMap:      true,
		SiteDescription: strPtr("Detailed site description"),
	}

	err = db.CreateSite(site)
	if err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	if site.ID <= 0 {
		t.Error("Expected positive site ID")
	}

	// Verify all fields were stored correctly
	retrieved, err := db.GetSite(site.ID)
	if err != nil {
		t.Fatalf("GetSite failed: %v", err)
	}

	if retrieved.Name != site.Name {
		t.Errorf("Name mismatch: expected %q, got %q", site.Name, retrieved.Name)
	}
	if retrieved.IncludeMap != true {
		t.Error("Expected IncludeMap to be true")
	}
}

// TestSiteReport_CreateSiteReport_AllFields tests creating a report with all fields
func TestSiteReport_CreateSiteReport_AllFields(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a site first
	site := &Site{Name: "Test Site"}
	err = db.CreateSite(site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create a report with optional fields
	zipPath := "/reports/report.zip"
	zipName := "report.zip"
	report := &SiteReport{
		SiteID:      site.ID,
		StartDate:   "2025-01-01",
		EndDate:     "2025-01-07",
		Filepath:    "/reports/report.pdf",
		Filename:    "report.pdf",
		ZipFilepath: &zipPath,
		ZipFilename: &zipName,
		RunID:       "run-full",
		Timezone:    "Europe/London",
		Units:       "km/h",
		Source:      "lidar_tracks",
	}

	err = db.CreateSiteReport(report)
	if err != nil {
		t.Fatalf("CreateSiteReport failed: %v", err)
	}

	if report.ID <= 0 {
		t.Error("Expected positive report ID")
	}

	// Verify retrieval
	retrieved, err := db.GetSiteReport(report.ID)
	if err != nil {
		t.Fatalf("GetSiteReport failed: %v", err)
	}

	if retrieved.ZipFilepath == nil || *retrieved.ZipFilepath != zipPath {
		t.Error("ZipFilepath not stored correctly")
	}
	if retrieved.Units != "km/h" {
		t.Errorf("Units mismatch: expected 'km/h', got '%s'", retrieved.Units)
	}
}
