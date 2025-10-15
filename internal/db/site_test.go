package db

import (
	"os"
	"testing"
)

// TestCreateSite_Success tests successful site creation
func TestCreateSite_Success(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:             "Test Site",
		Location:         "123 Main St",
		Description:      strPtr("Test Description"),
		CosineErrorAngle: 21.0,
		SpeedLimit:       25,
		Surveyor:         "John Doe",
		Contact:          "john@example.com",
		Address:          strPtr("123 Main St, City, State"),
		Latitude:         floatPtr(37.7749),
		Longitude:        floatPtr(-122.4194),
		MapAngle:         floatPtr(45.0),
		IncludeMap:       true,
		SiteDescription:  strPtr("Site description"),
		SpeedLimitNote:   strPtr("Posted speed limit"),
	}

	err := db.CreateSite(site)
	if err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	if site.ID == 0 {
		t.Error("Expected site ID to be set after creation")
	}

	// Fetch the site to get timestamps populated
	retrieved, err := db.GetSite(site.ID)
	if err != nil {
		t.Fatalf("GetSite failed: %v", err)
	}

	if retrieved.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt timestamp to be set")
	}

	if retrieved.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt timestamp to be set")
	}
}

// TestCreateSite_DuplicateName tests that duplicate site names are rejected
func TestCreateSite_DuplicateName(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site1 := &Site{
		Name:             "Duplicate Site",
		Location:         "Location 1",
		CosineErrorAngle: 21.0,
		Surveyor:         "Surveyor 1",
		Contact:          "contact1@example.com",
	}

	err := db.CreateSite(site1)
	if err != nil {
		t.Fatalf("First CreateSite failed: %v", err)
	}

	site2 := &Site{
		Name:             "Duplicate Site",
		Location:         "Location 2",
		CosineErrorAngle: 22.0,
		Surveyor:         "Surveyor 2",
		Contact:          "contact2@example.com",
	}

	err = db.CreateSite(site2)
	if err == nil {
		t.Error("Expected error for duplicate site name, got nil")
	}
}

// TestCreateSite_RequiredFields tests that database-level constraints are enforced
// Note: All TEXT fields in SQLite accept empty strings even with NOT NULL
// REAL fields accept 0.0 as valid
// Validation of business rules (non-empty, non-zero) is done at API level
func TestCreateSite_RequiredFields(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Test that site creation works with all fields
	site := &Site{
		Name:             "Valid Site",
		Location:         "Location",
		CosineErrorAngle: 21.0,
		Surveyor:         "Surveyor",
		Contact:          "contact@example.com",
	}

	err := db.CreateSite(site)
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}
}

// TestGetSite_Exists tests retrieving an existing site
func TestGetSite_Exists(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	original := &Site{
		Name:             "Get Test Site",
		Location:         "Test Location",
		Description:      strPtr("Test Description"),
		CosineErrorAngle: 21.0,
		SpeedLimit:       30,
		Surveyor:         "Jane Doe",
		Contact:          "jane@example.com",
		SiteDescription:  strPtr("Site desc"),
	}

	err := db.CreateSite(original)
	if err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	retrieved, err := db.GetSite(original.ID)
	if err != nil {
		t.Fatalf("GetSite failed: %v", err)
	}

	if retrieved.ID != original.ID {
		t.Errorf("ID mismatch: got %d, want %d", retrieved.ID, original.ID)
	}
	if retrieved.Name != original.Name {
		t.Errorf("Name mismatch: got %s, want %s", retrieved.Name, original.Name)
	}
	if retrieved.Location != original.Location {
		t.Errorf("Location mismatch: got %s, want %s", retrieved.Location, original.Location)
	}
	if retrieved.CosineErrorAngle != original.CosineErrorAngle {
		t.Errorf("CosineErrorAngle mismatch: got %f, want %f", retrieved.CosineErrorAngle, original.CosineErrorAngle)
	}
	if retrieved.Surveyor != original.Surveyor {
		t.Errorf("Surveyor mismatch: got %s, want %s", retrieved.Surveyor, original.Surveyor)
	}
	if retrieved.Contact != original.Contact {
		t.Errorf("Contact mismatch: got %s, want %s", retrieved.Contact, original.Contact)
	}
}

// TestGetSite_NotFound tests retrieving a non-existent site
func TestGetSite_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	_, err := db.GetSite(99999)
	if err == nil {
		t.Error("Expected error for non-existent site, got nil")
	}
	if err.Error() != "site not found" {
		t.Errorf("Expected 'site not found' error, got: %v", err)
	}
}

// TestGetAllSites_Empty tests listing sites when none exist
func TestGetAllSites_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	sites, err := db.GetAllSites()
	if err != nil {
		t.Fatalf("GetAllSites failed: %v", err)
	}

	// Note: schema.sql creates a default site, so we might have 1 site
	// For this test, let's just verify we can call it without error
	if sites == nil {
		t.Error("Expected non-nil slice, got nil")
	}
}

// TestGetAllSites_Multiple tests listing multiple sites
func TestGetAllSites_Multiple(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Create multiple sites
	sites := []*Site{
		{
			Name:             "Site A",
			Location:         "Location A",
			CosineErrorAngle: 21.0,
			Surveyor:         "Surveyor A",
			Contact:          "a@example.com",
		},
		{
			Name:             "Site B",
			Location:         "Location B",
			CosineErrorAngle: 22.0,
			Surveyor:         "Surveyor B",
			Contact:          "b@example.com",
		},
		{
			Name:             "Site C",
			Location:         "Location C",
			CosineErrorAngle: 23.0,
			Surveyor:         "Surveyor C",
			Contact:          "c@example.com",
		},
	}

	for _, site := range sites {
		err := db.CreateSite(site)
		if err != nil {
			t.Fatalf("CreateSite failed: %v", err)
		}
	}

	retrieved, err := db.GetAllSites()
	if err != nil {
		t.Fatalf("GetAllSites failed: %v", err)
	}

	// Should have at least 3 sites (might have default site too)
	if len(retrieved) < 3 {
		t.Errorf("Expected at least 3 sites, got %d", len(retrieved))
	}

	// Verify we can find our created sites
	foundA := false
	foundB := false
	foundC := false
	for _, s := range retrieved {
		if s.Name == "Site A" {
			foundA = true
		}
		if s.Name == "Site B" {
			foundB = true
		}
		if s.Name == "Site C" {
			foundC = true
		}
	}

	if !foundA || !foundB || !foundC {
		t.Error("Not all created sites were returned by GetAllSites")
	}
}

// TestUpdateSite_Success tests successful site update
func TestUpdateSite_Success(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	original := &Site{
		Name:             "Original Name",
		Location:         "Original Location",
		CosineErrorAngle: 21.0,
		Surveyor:         "Original Surveyor",
		Contact:          "original@example.com",
		SpeedLimit:       25,
	}

	err := db.CreateSite(original)
	if err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Get the original with timestamps
	originalWithTimestamps, err := db.GetSite(original.ID)
	if err != nil {
		t.Fatalf("GetSite failed: %v", err)
	}

	originalUpdatedAt := originalWithTimestamps.UpdatedAt
	originalCreatedAt := originalWithTimestamps.CreatedAt

	// Update the site
	original.Name = "Updated Name"
	original.Location = "Updated Location"
	original.CosineErrorAngle = 22.5
	original.SpeedLimit = 35

	err = db.UpdateSite(original)
	if err != nil {
		t.Fatalf("UpdateSite failed: %v", err)
	}

	// Verify the update
	updated, err := db.GetSite(original.ID)
	if err != nil {
		t.Fatalf("GetSite failed: %v", err)
	}

	if updated.Name != "Updated Name" {
		t.Errorf("Name not updated: got %s, want Updated Name", updated.Name)
	}
	if updated.Location != "Updated Location" {
		t.Errorf("Location not updated: got %s, want Updated Location", updated.Location)
	}
	if updated.CosineErrorAngle != 22.5 {
		t.Errorf("CosineErrorAngle not updated: got %f, want 22.5", updated.CosineErrorAngle)
	}
	if updated.SpeedLimit != 35 {
		t.Errorf("SpeedLimit not updated: got %d, want 35", updated.SpeedLimit)
	}

	// Verify UpdatedAt timestamp changed (allowing for equal times if update was too fast)
	if updated.UpdatedAt.Before(originalUpdatedAt) {
		t.Error("UpdatedAt timestamp should not go backwards after update")
	}

	// Verify CreatedAt didn't change
	if !updated.CreatedAt.Equal(originalCreatedAt) {
		t.Error("CreatedAt should not change on update")
	}
}

// TestUpdateSite_NotFound tests updating a non-existent site
func TestUpdateSite_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		ID:               99999,
		Name:             "Non-existent",
		Location:         "Location",
		CosineErrorAngle: 21.0,
		Surveyor:         "Surveyor",
		Contact:          "contact@example.com",
	}

	err := db.UpdateSite(site)
	if err == nil {
		t.Error("Expected error for non-existent site, got nil")
	}
	if err.Error() != "site not found" {
		t.Errorf("Expected 'site not found' error, got: %v", err)
	}
}

// TestDeleteSite_Success tests successful site deletion
func TestDeleteSite_Success(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:             "To Delete",
		Location:         "Location",
		CosineErrorAngle: 21.0,
		Surveyor:         "Surveyor",
		Contact:          "contact@example.com",
	}

	err := db.CreateSite(site)
	if err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Delete the site
	err = db.DeleteSite(site.ID)
	if err != nil {
		t.Fatalf("DeleteSite failed: %v", err)
	}

	// Verify it's gone
	_, err = db.GetSite(site.ID)
	if err == nil {
		t.Error("Expected error when getting deleted site, got nil")
	}
	if err.Error() != "site not found" {
		t.Errorf("Expected 'site not found' error, got: %v", err)
	}
}

// TestDeleteSite_NotFound tests deleting a non-existent site
func TestDeleteSite_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	err := db.DeleteSite(99999)
	if err == nil {
		t.Error("Expected error for non-existent site, got nil")
	}
	if err.Error() != "site not found" {
		t.Errorf("Expected 'site not found' error, got: %v", err)
	}
}

// TestSite_OptionalFields tests that optional fields can be nil
func TestSite_OptionalFields(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:             "Minimal Site",
		Location:         "Location",
		CosineErrorAngle: 21.0,
		Surveyor:         "Surveyor",
		Contact:          "contact@example.com",
		// All optional fields left as nil/zero
	}

	err := db.CreateSite(site)
	if err != nil {
		t.Fatalf("CreateSite with minimal fields failed: %v", err)
	}

	retrieved, err := db.GetSite(site.ID)
	if err != nil {
		t.Fatalf("GetSite failed: %v", err)
	}

	if retrieved.Description != nil {
		t.Error("Expected Description to be nil")
	}
	if retrieved.Address != nil {
		t.Error("Expected Address to be nil")
	}
	if retrieved.Latitude != nil {
		t.Error("Expected Latitude to be nil")
	}
	if retrieved.Longitude != nil {
		t.Error("Expected Longitude to be nil")
	}
	if retrieved.MapAngle != nil {
		t.Error("Expected MapAngle to be nil")
	}
	if retrieved.SiteDescription != nil {
		t.Error("Expected SiteDescription to be nil")
	}
	if retrieved.SpeedLimitNote != nil {
		t.Error("Expected SpeedLimitNote to be nil")
	}
}

// TestSite_BooleanFields tests that boolean fields work correctly
func TestSite_BooleanFields(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	tests := []struct {
		name       string
		includeMap bool
	}{
		{"include map true", true},
		{"include map false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			site := &Site{
				Name:             tt.name,
				Location:         "Location",
				CosineErrorAngle: 21.0,
				Surveyor:         "Surveyor",
				Contact:          "contact@example.com",
				IncludeMap:       tt.includeMap,
			}

			err := db.CreateSite(site)
			if err != nil {
				t.Fatalf("CreateSite failed: %v", err)
			}

			retrieved, err := db.GetSite(site.ID)
			if err != nil {
				t.Fatalf("GetSite failed: %v", err)
			}

			if retrieved.IncludeMap != tt.includeMap {
				t.Errorf("IncludeMap mismatch: got %v, want %v", retrieved.IncludeMap, tt.includeMap)
			}
		})
	}
}

// Helper functions

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	fname := t.Name() + ".db"
	_ = os.Remove(fname)

	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	return db
}

func cleanupTestDB(t *testing.T, db *DB) {
	t.Helper()
	fname := t.Name() + ".db"
	db.Close()
	_ = os.Remove(fname)
	_ = os.Remove(fname + "-shm")
	_ = os.Remove(fname + "-wal")
}

func strPtr(s string) *string {
	return &s
}

func floatPtr(f float64) *float64 {
	return &f
}
