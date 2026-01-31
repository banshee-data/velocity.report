package db

import (
	"path/filepath"
	"testing"
)

// TestCreateSite_InvalidData tests site creation with various edge cases
func TestCreateSite_InvalidData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	testCases := []struct {
		name string
		site Site
	}{
		{
			name: "empty_name",
			site: Site{
				Name:     "",
				Location: "Test Location",
				Surveyor: "Test Surveyor",
				Contact:  "test@example.com",
			},
		},
		{
			name: "empty_location",
			site: Site{
				Name:     "Test Site",
				Location: "",
				Surveyor: "Test Surveyor",
				Contact:  "test@example.com",
			},
		},
		{
			name: "very_long_name",
			site: Site{
				Name:     string(make([]byte, 10000)), // Very long name
				Location: "Test Location",
				Surveyor: "Test Surveyor",
				Contact:  "test@example.com",
			},
		},
		{
			name: "special_characters",
			site: Site{
				Name:     "Site with 'quotes' and \"doubles\" and \\ backslash",
				Location: "Location with; semicolons; and -- dashes",
				Surveyor: "Test Surveyor",
				Contact:  "test@example.com",
			},
		},
		{
			name: "unicode_characters",
			site: Site{
				Name:     "Site with émojis 🚗 and ǘnicode",
				Location: "日本語の場所",
				Surveyor: "テスト surveyor",
				Contact:  "test@example.com",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := db.CreateSite(&tc.site)
			// Just verify no panics occur - some may succeed, some may fail
			t.Logf("CreateSite result: err=%v, ID=%d", err, tc.site.ID)
		})
	}
}

// TestGetSite_NonexistentID tests retrieval of non-existent site
func TestGetSite_NonexistentID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Try to get site with ID that doesn't exist
	site, err := db.GetSite(99999)
	if err == nil {
		t.Errorf("Expected error for non-existent site, got: %+v", site)
	}
}

// TestGetSite_NegativeID tests retrieval with negative ID
func TestGetSite_NegativeID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Try to get site with negative ID
	site, err := db.GetSite(-1)
	if err == nil {
		t.Errorf("Expected error for negative ID, got: %+v", site)
	}
}

// TestGetSite_ZeroID tests retrieval with zero ID
func TestGetSite_ZeroID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Try to get site with zero ID
	site, err := db.GetSite(0)
	if err == nil {
		t.Errorf("Expected error for zero ID, got: %+v", site)
	}
}

// TestUpdateSite_NonexistentID tests updating non-existent site
func TestUpdateSite_NonexistentID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	site := &Site{
		ID:       99999,
		Name:     "Updated Name",
		Location: "Updated Location",
		Surveyor: "Updated Surveyor",
		Contact:  "updated@example.com",
	}

	err = db.UpdateSite(site)
	// Should handle gracefully (may update 0 rows)
	t.Logf("UpdateSite for non-existent ID result: %v", err)
}

// TestDeleteSite_NonexistentID tests deleting non-existent site
func TestDeleteSite_NonexistentID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	err = db.DeleteSite(99999)
	// Should handle gracefully
	t.Logf("DeleteSite for non-existent ID result: %v", err)
}

// TestGetAllSites_EmptyDatabase tests listing sites when none exist
func TestGetAllSites_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	sites, err := db.GetAllSites()
	if err != nil {
		t.Fatalf("GetAllSites on empty database failed: %v", err)
	}

	if len(sites) != 0 {
		t.Errorf("Expected empty list, got %d sites", len(sites))
	}
}

// TestCreateSite_WithOptionalFields tests creation with nil optional fields
func TestCreateSite_WithOptionalFields(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create site with all nil optional fields
	site := Site{
		Name:            "Test Site",
		Location:        "Test Location",
		Description:     nil,
		Surveyor:        "Test Surveyor",
		Contact:         "test@example.com",
		Address:         nil,
		Latitude:        nil,
		Longitude:       nil,
		MapAngle:        nil,
		IncludeMap:      false,
		SiteDescription: nil,
	}

	err = db.CreateSite(&site)
	if err != nil {
		t.Fatalf("CreateSite with nil optional fields failed: %v", err)
	}

	if site.ID == 0 {
		t.Error("Expected non-zero ID after creation")
	}

	// Retrieve and verify
	retrieved, err := db.GetSite(site.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve site: %v", err)
	}

	if retrieved.Description != nil {
		t.Error("Expected nil Description")
	}
	if retrieved.Latitude != nil {
		t.Error("Expected nil Latitude")
	}
}

// TestCreateSite_WithAllOptionalFields tests creation with all optional fields set
func TestCreateSite_WithAllOptionalFields(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	desc := "A detailed description"
	addr := "123 Test Street"
	lat := 51.5074
	lon := -0.1278
	angle := 45.0
	siteDesc := "Site-specific description"

	site := Site{
		Name:            "Full Site",
		Location:        "Full Location",
		Description:     &desc,
		Surveyor:        "Full Surveyor",
		Contact:         "full@example.com",
		Address:         &addr,
		Latitude:        &lat,
		Longitude:       &lon,
		MapAngle:        &angle,
		IncludeMap:      true,
		SiteDescription: &siteDesc,
	}

	err = db.CreateSite(&site)
	if err != nil {
		t.Fatalf("CreateSite with all optional fields failed: %v", err)
	}

	// Retrieve and verify all fields
	retrieved, err := db.GetSite(site.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve site: %v", err)
	}

	if *retrieved.Description != desc {
		t.Errorf("Description mismatch: expected %q, got %q", desc, *retrieved.Description)
	}
	if *retrieved.Latitude != lat {
		t.Errorf("Latitude mismatch: expected %f, got %f", lat, *retrieved.Latitude)
	}
	if !retrieved.IncludeMap {
		t.Error("Expected IncludeMap to be true")
	}
}

// TestCreateSite_BoundaryCoordinates tests sites with boundary coordinate values
func TestCreateSite_BoundaryCoordinates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	testCases := []struct {
		name string
		lat  float64
		lon  float64
	}{
		{"north_pole", 90.0, 0.0},
		{"south_pole", -90.0, 0.0},
		{"prime_meridian", 0.0, 0.0},
		{"date_line_east", 0.0, 180.0},
		{"date_line_west", 0.0, -180.0},
		{"max_values", 90.0, 180.0},
		{"min_values", -90.0, -180.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			site := Site{
				Name:      "Coord Test " + tc.name,
				Location:  tc.name,
				Surveyor:  "Test",
				Contact:   "test@example.com",
				Latitude:  &tc.lat,
				Longitude: &tc.lon,
			}

			err := db.CreateSite(&site)
			if err != nil {
				t.Errorf("CreateSite with boundary coords (%f, %f) failed: %v", tc.lat, tc.lon, err)
			}
		})
	}
}

// TestUpdateSite_AllFields tests updating all fields of a site
func TestUpdateSite_AllFields(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create initial site
	site := Site{
		Name:       "Original Name",
		Location:   "Original Location",
		Surveyor:   "Original Surveyor",
		Contact:    "original@example.com",
		IncludeMap: false,
	}

	err = db.CreateSite(&site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Update all fields
	newDesc := "New Description"
	newAddr := "New Address"
	newLat := 40.7128
	newLon := -74.0060
	newAngle := 90.0
	newSiteDesc := "New Site Description"

	site.Name = "Updated Name"
	site.Location = "Updated Location"
	site.Description = &newDesc
	site.Surveyor = "Updated Surveyor"
	site.Contact = "updated@example.com"
	site.Address = &newAddr
	site.Latitude = &newLat
	site.Longitude = &newLon
	site.MapAngle = &newAngle
	site.IncludeMap = true
	site.SiteDescription = &newSiteDesc

	err = db.UpdateSite(&site)
	if err != nil {
		t.Fatalf("UpdateSite failed: %v", err)
	}

	// Verify updates
	retrieved, err := db.GetSite(site.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve updated site: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("Name not updated: expected 'Updated Name', got %q", retrieved.Name)
	}
	if !retrieved.IncludeMap {
		t.Error("IncludeMap not updated to true")
	}
}

// TestDeleteSite_CascadeReports tests that deleting a site handles related reports
func TestDeleteSite_CascadeReports(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create a site
	site := Site{
		Name:     "Site with Reports",
		Location: "Test Location",
		Surveyor: "Test",
		Contact:  "test@example.com",
	}

	err = db.CreateSite(&site)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create a report for this site
	report := SiteReport{
		SiteID:    site.ID,
		StartDate: "2024-01-01",
		EndDate:   "2024-01-31",
		Filepath:  "/path/to/report.pdf",
		Filename:  "report.pdf",
		RunID:     "20240101_120000",
		Timezone:  "UTC",
		Units:     "mph",
		Source:    "radar_data",
	}

	err = db.CreateSiteReport(&report)
	if err != nil {
		t.Fatalf("Failed to create report: %v", err)
	}

	// Delete the site
	err = db.DeleteSite(site.ID)
	if err != nil {
		t.Fatalf("DeleteSite failed: %v", err)
	}

	// Verify site is deleted
	_, err = db.GetSite(site.ID)
	if err == nil {
		t.Error("Expected error getting deleted site")
	}
}
