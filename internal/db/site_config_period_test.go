package db

import (
	"testing"
	"time"
)

func TestCreateSiteConfigPeriod(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Create a test site first
	site := &Site{
		Name:             "Test Site",
		Location:         "Test Location",
		CosineErrorAngle: 5.0,
		SpeedLimit:       25,
		Surveyor:         "Test Surveyor",
		Contact:          "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	now := float64(time.Now().Unix())
	notes := "Test period"

	period := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: now,
		EffectiveEndUnix:   nil, // Open-ended
		IsActive:           true,
		Notes:              &notes,
	}

	if err := db.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("Failed to create site config period: %v", err)
	}

	if period.ID == 0 {
		t.Error("Expected non-zero ID after creation")
	}
}

func TestGetActiveSiteConfigPeriod(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Create a test site
	site := &Site{
		Name:             "Test Site",
		Location:         "Test Location",
		CosineErrorAngle: 10.0,
		SpeedLimit:       30,
		Surveyor:         "Test Surveyor",
		Contact:          "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	now := float64(time.Now().Unix())

	// Create an active period
	period := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: now,
		EffectiveEndUnix:   nil,
		IsActive:           true,
	}
	if err := db.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("Failed to create site config period: %v", err)
	}

	// Retrieve the active period
	activePeriod, err := db.GetActiveSiteConfigPeriod()
	if err != nil {
		t.Fatalf("Failed to get active period: %v", err)
	}

	if activePeriod.SiteID != site.ID {
		t.Errorf("Expected site ID %d, got %d", site.ID, activePeriod.SiteID)
	}
	if !activePeriod.IsActive {
		t.Error("Expected period to be active")
	}
	if activePeriod.Site == nil {
		t.Fatal("Expected site to be populated")
	}
	if activePeriod.Site.CosineErrorAngle != 10.0 {
		t.Errorf("Expected cosine angle 10.0, got %f", activePeriod.Site.CosineErrorAngle)
	}
}

func TestGetSiteConfigPeriodForTimestamp(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Create test sites
	site1 := &Site{
		Name:             "Site 1",
		Location:         "Location 1",
		CosineErrorAngle: 5.0,
		SpeedLimit:       25,
		Surveyor:         "Surveyor 1",
		Contact:          "site1@example.com",
	}
	if err := db.CreateSite(site1); err != nil {
		t.Fatalf("Failed to create site 1: %v", err)
	}

	site2 := &Site{
		Name:             "Site 2",
		Location:         "Location 2",
		CosineErrorAngle: 10.0,
		SpeedLimit:       30,
		Surveyor:         "Surveyor 2",
		Contact:          "site2@example.com",
	}
	if err := db.CreateSite(site2); err != nil {
		t.Fatalf("Failed to create site 2: %v", err)
	}

	// Create periods with specific time ranges
	baseTime := float64(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix())

	// Period 1: Jan 1 - Jan 31 (site 1, angle 5.0)
	endTime1 := float64(time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC).Unix())
	period1 := &SiteConfigPeriod{
		SiteID:             site1.ID,
		EffectiveStartUnix: baseTime,
		EffectiveEndUnix:   &endTime1,
		IsActive:           false,
	}
	if err := db.CreateSiteConfigPeriod(period1); err != nil {
		t.Fatalf("Failed to create period 1: %v", err)
	}

	// Period 2: Feb 1 onwards (site 2, angle 10.0)
	startTime2 := float64(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC).Unix())
	period2 := &SiteConfigPeriod{
		SiteID:             site2.ID,
		EffectiveStartUnix: startTime2,
		EffectiveEndUnix:   nil, // Open-ended
		IsActive:           false,
	}
	if err := db.CreateSiteConfigPeriod(period2); err != nil {
		t.Fatalf("Failed to create period 2: %v", err)
	}

	// Test timestamp in January (should get period 1 with angle 5.0)
	janTimestamp := float64(time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC).Unix())
	janPeriod, err := db.GetSiteConfigPeriodForTimestamp(janTimestamp)
	if err != nil {
		t.Fatalf("Failed to get period for January: %v", err)
	}
	if janPeriod.Site.CosineErrorAngle != 5.0 {
		t.Errorf("Expected cosine angle 5.0 for January, got %f", janPeriod.Site.CosineErrorAngle)
	}

	// Test timestamp in February (should get period 2 with angle 10.0)
	febTimestamp := float64(time.Date(2024, 2, 15, 12, 0, 0, 0, time.UTC).Unix())
	febPeriod, err := db.GetSiteConfigPeriodForTimestamp(febTimestamp)
	if err != nil {
		t.Fatalf("Failed to get period for February: %v", err)
	}
	if febPeriod.Site.CosineErrorAngle != 10.0 {
		t.Errorf("Expected cosine angle 10.0 for February, got %f", febPeriod.Site.CosineErrorAngle)
	}
}

func TestEnforceSingleActiveperiod(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Create test sites
	site1 := &Site{
		Name:             "Site 1",
		Location:         "Location 1",
		CosineErrorAngle: 5.0,
		SpeedLimit:       25,
		Surveyor:         "Surveyor 1",
		Contact:          "site1@example.com",
	}
	if err := db.CreateSite(site1); err != nil {
		t.Fatalf("Failed to create site 1: %v", err)
	}

	site2 := &Site{
		Name:             "Site 2",
		Location:         "Location 2",
		CosineErrorAngle: 10.0,
		SpeedLimit:       30,
		Surveyor:         "Surveyor 2",
		Contact:          "site2@example.com",
	}
	if err := db.CreateSite(site2); err != nil {
		t.Fatalf("Failed to create site 2: %v", err)
	}

	now := float64(time.Now().Unix())

	// Create first active period
	period1 := &SiteConfigPeriod{
		SiteID:             site1.ID,
		EffectiveStartUnix: now,
		IsActive:           true,
	}
	if err := db.CreateSiteConfigPeriod(period1); err != nil {
		t.Fatalf("Failed to create period 1: %v", err)
	}

	// Verify it's active
	activePeriod, err := db.GetActiveSiteConfigPeriod()
	if err != nil {
		t.Fatalf("Failed to get active period: %v", err)
	}
	if activePeriod.SiteID != site1.ID {
		t.Errorf("Expected site 1 to be active, got site %d", activePeriod.SiteID)
	}

	// Create second active period - should deactivate the first
	period2 := &SiteConfigPeriod{
		SiteID:             site2.ID,
		EffectiveStartUnix: now + 3600,
		IsActive:           true,
	}
	if err := db.CreateSiteConfigPeriod(period2); err != nil {
		t.Fatalf("Failed to create period 2: %v", err)
	}

	// Verify period 2 is now active
	activePeriod, err = db.GetActiveSiteConfigPeriod()
	if err != nil {
		t.Fatalf("Failed to get active period after second insert: %v", err)
	}
	if activePeriod.SiteID != site2.ID {
		t.Errorf("Expected site 2 to be active, got site %d", activePeriod.SiteID)
	}

	// Verify period 1 is no longer active
	period1Retrieved, err := db.GetSiteConfigPeriod(period1.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve period 1: %v", err)
	}
	if period1Retrieved.IsActive {
		t.Error("Expected period 1 to be deactivated")
	}
}

func TestCloseSiteConfigPeriod(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Create a test site
	site := &Site{
		Name:             "Test Site",
		Location:         "Test Location",
		CosineErrorAngle: 5.0,
		SpeedLimit:       25,
		Surveyor:         "Test Surveyor",
		Contact:          "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	now := float64(time.Now().Unix())

	// Create an open-ended period
	period := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: now,
		EffectiveEndUnix:   nil,
		IsActive:           true,
	}
	if err := db.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("Failed to create period: %v", err)
	}

	// Close the period
	closeTime := now + 3600
	if err := db.CloseSiteConfigPeriod(period.ID, closeTime); err != nil {
		t.Fatalf("Failed to close period: %v", err)
	}

	// Verify the period is closed
	closedPeriod, err := db.GetSiteConfigPeriod(period.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve closed period: %v", err)
	}

	if closedPeriod.EffectiveEndUnix == nil {
		t.Error("Expected period to have an end time")
	} else if *closedPeriod.EffectiveEndUnix != closeTime {
		t.Errorf("Expected end time %f, got %f", closeTime, *closedPeriod.EffectiveEndUnix)
	}

	if closedPeriod.IsActive {
		t.Error("Expected period to be deactivated after closing")
	}
}

func TestGetAllSiteConfigPeriods(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Create test sites
	site := &Site{
		Name:             "Test Site",
		Location:         "Test Location",
		CosineErrorAngle: 5.0,
		SpeedLimit:       25,
		Surveyor:         "Test Surveyor",
		Contact:          "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	baseTime := float64(time.Now().Unix())

	// Create multiple periods
	for i := 0; i < 3; i++ {
		period := &SiteConfigPeriod{
			SiteID:             site.ID,
			EffectiveStartUnix: baseTime + float64(i*3600),
			IsActive:           i == 2, // Last one is active
		}
		if err := db.CreateSiteConfigPeriod(period); err != nil {
			t.Fatalf("Failed to create period %d: %v", i, err)
		}
	}

	// Retrieve all periods
	periods, err := db.GetAllSiteConfigPeriods()
	if err != nil {
		t.Fatalf("Failed to get all periods: %v", err)
	}

	// We expect 4 periods: 1 default from migration + 3 we created
	if len(periods) != 4 {
		t.Errorf("Expected 4 periods (1 default + 3 created), got %d", len(periods))
	}

	// Verify they're ordered by start time
	for i := 1; i < len(periods); i++ {
		if periods[i].EffectiveStartUnix < periods[i-1].EffectiveStartUnix {
			t.Error("Periods are not ordered by start time")
		}
	}

	// Verify the last one we created is active (period index 3)
	if !periods[3].IsActive {
		t.Error("Expected last created period to be active")
	}
}

func TestTimeline(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Create test sites with different cosine angles
	site1 := &Site{
		Name:             "Site 1",
		Location:         "Location 1",
		CosineErrorAngle: 5.0,
		SpeedLimit:       25,
		Surveyor:         "Surveyor 1",
		Contact:          "site1@example.com",
	}
	if err := db.CreateSite(site1); err != nil {
		t.Fatalf("Failed to create site 1: %v", err)
	}

	site2 := &Site{
		Name:             "Site 2",
		Location:         "Location 2",
		CosineErrorAngle: 10.0,
		SpeedLimit:       30,
		Surveyor:         "Surveyor 2",
		Contact:          "site2@example.com",
	}
	if err := db.CreateSite(site2); err != nil {
		t.Fatalf("Failed to create site 2: %v", err)
	}

	// Create time periods
	jan1 := float64(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix())
	jan15 := float64(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Unix())
	feb1 := float64(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC).Unix())
	mar1 := float64(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC).Unix())

	// Period 1: Jan 1 - Jan 15 (site 1)
	period1 := &SiteConfigPeriod{
		SiteID:             site1.ID,
		EffectiveStartUnix: jan1,
		EffectiveEndUnix:   &jan15,
		IsActive:           false,
	}
	if err := db.CreateSiteConfigPeriod(period1); err != nil {
		t.Fatalf("Failed to create period 1: %v", err)
	}

	// Period 2: Feb 1 onwards (site 2)
	period2 := &SiteConfigPeriod{
		SiteID:             site2.ID,
		EffectiveStartUnix: feb1,
		EffectiveEndUnix:   nil,
		IsActive:           true,
	}
	if err := db.CreateSiteConfigPeriod(period2); err != nil {
		t.Fatalf("Failed to create period 2: %v", err)
	}

	// Add some radar data
	// Data in Jan (covered by site 1)
	for i := 0; i < 5; i++ {
		eventJSON := `{"uptime": 1000.0, "magnitude": 2000, "speed": 25.0}`
		if err := db.RecordRawData(eventJSON); err != nil {
			t.Fatalf("Failed to record Jan data: %v", err)
		}
	}

	// Data in the gap (Jan 16-31, no site config)
	// We can't easily insert with specific timestamps in the current schema,
	// so we'll skip this part of the test for now

	// Data in Feb (covered by site 2)
	// Same issue - would need to modify RecordRawData to accept timestamp

	// Get timeline for Jan-Mar
	timeline, err := db.GetTimeline(jan1, mar1)
	if err != nil {
		t.Fatalf("Failed to get timeline: %v", err)
	}

	// We should have at least one entry
	if len(timeline) == 0 {
		t.Error("Expected at least one timeline entry")
	}

	// Note: More comprehensive timeline testing would require ability to insert
	// radar_data with specific timestamps, which would require schema changes
	// or test utilities. For now, we verify the query executes without error.
}
