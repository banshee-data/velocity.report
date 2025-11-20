package db

import (
	"testing"
	"time"
)

// Helper functions for creating pointer values
func strPtr(s string) *string {
	return &s
}

func floatPtr(f float64) *float64 {
	return &f
}

// createTestSiteWithConfig creates a site with an associated variable config and active config period
// This is a helper function for tests that need a complete site setup with cosine error angle
func createTestSiteWithConfig(t *testing.T, db *DB, siteName string, cosineErrorAngle float64) (*Site, *SiteVariableConfig, *SiteConfigPeriod) {
	t.Helper()

	// Create the site (without cosine_error_angle which is now in SiteVariableConfig)
	site := &Site{
		Name:       siteName,
		Location:   "Test Location",
		Surveyor:   "Test Surveyor",
		Contact:    "test@example.com",
		SpeedLimit: 25,
	}

	err := db.CreateSite(site)
	if err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Create the variable config
	varConfig := &SiteVariableConfig{
		CosineErrorAngle: cosineErrorAngle,
	}

	err = db.CreateSiteVariableConfig(varConfig)
	if err != nil {
		t.Fatalf("CreateSiteVariableConfig failed: %v", err)
	}

	// Create an active config period linking site to variable config
	now := float64(time.Now().Unix())
	varConfigIDPtr := &varConfig.ID
	configPeriod := &SiteConfigPeriod{
		SiteID:               site.ID,
		SiteVariableConfigID: varConfigIDPtr,
		EffectiveStartUnix:   now - 86400, // Started yesterday
		EffectiveEndUnix:     nil,         // Open-ended (currently active)
		IsActive:             true,
	}

	err = db.CreateSiteConfigPeriod(configPeriod)
	if err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	return site, varConfig, configPeriod
}

// createTestSiteWithFullDetails creates a site with all optional fields populated
// along with an associated variable config and active config period
func createTestSiteWithFullDetails(t *testing.T, db *DB, siteName string, cosineErrorAngle float64) (*Site, *SiteVariableConfig, *SiteConfigPeriod) {
	t.Helper()

	// Create the site with all fields
	site := &Site{
		Name:            siteName,
		Location:        "123 Main St",
		Description:     strPtr("Test Description"),
		SpeedLimit:      25,
		Surveyor:        "John Doe",
		Contact:         "john@example.com",
		Address:         strPtr("123 Main St, City, State"),
		Latitude:        floatPtr(37.7749),
		Longitude:       floatPtr(-122.4194),
		MapAngle:        floatPtr(45.0),
		IncludeMap:      true,
		SiteDescription: strPtr("Site description"),
		SpeedLimitNote:  strPtr("Posted speed limit"),
	}

	err := db.CreateSite(site)
	if err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Create the variable config
	varConfig := &SiteVariableConfig{
		CosineErrorAngle: cosineErrorAngle,
	}

	err = db.CreateSiteVariableConfig(varConfig)
	if err != nil {
		t.Fatalf("CreateSiteVariableConfig failed: %v", err)
	}

	// Create an active config period
	now := float64(time.Now().Unix())
	varConfigIDPtr := &varConfig.ID
	configPeriod := &SiteConfigPeriod{
		SiteID:               site.ID,
		SiteVariableConfigID: varConfigIDPtr,
		EffectiveStartUnix:   now - 86400, // Started yesterday
		EffectiveEndUnix:     nil,         // Open-ended (currently active)
		IsActive:             true,
	}

	err = db.CreateSiteConfigPeriod(configPeriod)
	if err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	return site, varConfig, configPeriod
}
