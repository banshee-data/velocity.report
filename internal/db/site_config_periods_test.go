package db

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func TestSiteConfigPeriodOverlap(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:     "Overlap Site",
		Location: "Test Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Create initial site config period
	initialNotes := "Initial site configuration"
	initialConfig := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 0,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		Notes:              &initialNotes,
		CosineErrorAngle:   0.0,
	}
	if err := db.CreateSiteConfigPeriod(initialConfig); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	active, err := db.GetActiveSiteConfigPeriod(site.ID)
	if err != nil {
		t.Fatalf("GetActiveSiteConfigPeriod failed: %v", err)
	}
	active.IsActive = false
	active.EffectiveEndUnix = floatPtr(1000)
	if err := db.UpdateSiteConfigPeriod(active); err != nil {
		t.Fatalf("UpdateSiteConfigPeriod failed: %v", err)
	}

	first := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1200,
		EffectiveEndUnix:   floatPtr(2000),
		IsActive:           true,
		CosineErrorAngle:   12.0,
	}
	if err := db.CreateSiteConfigPeriod(first); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	overlap := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1500,
		EffectiveEndUnix:   floatPtr(2500),
		IsActive:           false,
		CosineErrorAngle:   15.0,
	}
	if err := db.CreateSiteConfigPeriod(overlap); err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("Expected overlap error, got: %v", err)
	}
}

func TestRadarObjectRollupRangeCosineCorrection(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:     "Correction Site",
		Location: "Test Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Create initial site config period with 60 degree cosine error angle
	initialNotes := "Initial site configuration"
	initialConfig := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 0,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		Notes:              &initialNotes,
		CosineErrorAngle:   60.0,
	}
	if err := db.CreateSiteConfigPeriod(initialConfig); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	// Capture timestamp once for consistency
	now := time.Now().Unix()
	event := map[string]interface{}{
		"site_id":         site.ID,
		"classifier":      "all",
		"start_time":      float64(now),
		"end_time":        float64(now + 1),
		"delta_time_msec": 100,
		"max_speed_mps":   10.0,
		"min_speed_mps":   10.0,
		"speed_change":    0.0,
		"max_magnitude":   10,
		"avg_magnitude":   10,
		"total_frames":    1,
		"frames_per_mps":  1.0,
		"length_m":        1.0,
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}
	if err := db.RecordRadarObject(string(eventJSON)); err != nil {
		t.Fatalf("Failed to insert radar object: %v", err)
	}

	result, err := db.RadarObjectRollupRange(now-60, now+60, 0, 0, "radar_objects", "", 0, 0, site.ID, 0)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}
	if len(result.Metrics) == 0 {
		t.Fatalf("Expected metrics data, got none")
	}

	expected := 10.0 / math.Cos(60.0*math.Pi/180.0)
	if math.Abs(result.Metrics[0].MaxSpeed-expected) > 0.01 {
		t.Errorf("Expected corrected speed %.2f, got %.2f", expected, result.Metrics[0].MaxSpeed)
	}
}

func TestRadarDataRollupRangeCosineCorrection(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:     "Correction Site",
		Location: "Test Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Create initial site config period with 60 degree cosine error angle
	initialNotes := "Initial site configuration"
	initialConfig := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 0,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		Notes:              &initialNotes,
		CosineErrorAngle:   60.0,
	}
	if err := db.CreateSiteConfigPeriod(initialConfig); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	// Capture timestamp once for consistency
	now := time.Now().Unix()
	event := map[string]interface{}{
		"site_id":   site.ID,
		"speed":     10.0,
		"uptime":    float64(now),
		"magnitude": 5.0,
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}
	if err := db.RecordRawData(string(eventJSON)); err != nil {
		t.Fatalf("Failed to insert radar data: %v", err)
	}

	result, err := db.RadarObjectRollupRange(now-60, now+60, 0, 0, "radar_data", "", 0, 0, site.ID, 0)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}
	if len(result.Metrics) == 0 {
		t.Fatalf("Expected metrics data, got none")
	}

	expected := 10.0 / math.Cos(60.0*math.Pi/180.0)
	if math.Abs(result.Metrics[0].MaxSpeed-expected) > 0.01 {
		t.Errorf("Expected corrected speed %.2f, got %.2f", expected, result.Metrics[0].MaxSpeed)
	}
}

// TestListSiteConfigPeriods_All tests listing all config periods
func TestListSiteConfigPeriods_All(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site1 := &Site{
		Name:     "Site 1",
		Location: "Location 1",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site1); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	site2 := &Site{
		Name:     "Site 2",
		Location: "Location 2",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site2); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Create periods for site1
	period1 := &SiteConfigPeriod{
		SiteID:             site1.ID,
		EffectiveStartUnix: 1000,
		EffectiveEndUnix:   floatPtr(2000),
		IsActive:           false,
		CosineErrorAngle:   10.0,
	}
	if err := db.CreateSiteConfigPeriod(period1); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	period2 := &SiteConfigPeriod{
		SiteID:             site1.ID,
		EffectiveStartUnix: 2000,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		CosineErrorAngle:   15.0,
	}
	if err := db.CreateSiteConfigPeriod(period2); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	// Create period for site2
	period3 := &SiteConfigPeriod{
		SiteID:             site2.ID,
		EffectiveStartUnix: 3000,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		CosineErrorAngle:   20.0,
	}
	if err := db.CreateSiteConfigPeriod(period3); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	// List all periods
	allPeriods, err := db.ListSiteConfigPeriods(nil)
	if err != nil {
		t.Fatalf("ListSiteConfigPeriods failed: %v", err)
	}

	if len(allPeriods) != 3 {
		t.Errorf("Expected 3 periods, got %d", len(allPeriods))
	}

	// Verify they're sorted by effective_start_unix
	if len(allPeriods) >= 2 {
		if allPeriods[0].EffectiveStartUnix > allPeriods[1].EffectiveStartUnix {
			t.Error("Periods not sorted by effective_start_unix")
		}
	}
}

// TestListSiteConfigPeriods_FilteredBySite tests listing periods for a specific site
func TestListSiteConfigPeriods_FilteredBySite(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site1 := &Site{
		Name:     "Site 1",
		Location: "Location 1",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site1); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	site2 := &Site{
		Name:     "Site 2",
		Location: "Location 2",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site2); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Create periods for both sites
	period1 := &SiteConfigPeriod{
		SiteID:             site1.ID,
		EffectiveStartUnix: 1000,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		CosineErrorAngle:   10.0,
	}
	if err := db.CreateSiteConfigPeriod(period1); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	period2 := &SiteConfigPeriod{
		SiteID:             site2.ID,
		EffectiveStartUnix: 2000,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		CosineErrorAngle:   15.0,
	}
	if err := db.CreateSiteConfigPeriod(period2); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	// Filter by site1
	site1Periods, err := db.ListSiteConfigPeriods(&site1.ID)
	if err != nil {
		t.Fatalf("ListSiteConfigPeriods failed: %v", err)
	}

	if len(site1Periods) != 1 {
		t.Errorf("Expected 1 period for site1, got %d", len(site1Periods))
	}

	if len(site1Periods) > 0 && site1Periods[0].SiteID != site1.ID {
		t.Errorf("Expected period for site %d, got site %d", site1.ID, site1Periods[0].SiteID)
	}
}

// TestListSiteConfigPeriods_Empty tests listing when no periods exist
func TestListSiteConfigPeriods_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	periods, err := db.ListSiteConfigPeriods(nil)
	if err != nil {
		t.Fatalf("ListSiteConfigPeriods failed: %v", err)
	}

	if len(periods) != 0 {
		t.Errorf("Expected 0 periods, got %d", len(periods))
	}
}

// TestGetSiteConfigPeriod_Success tests retrieving a single period by ID
func TestGetSiteConfigPeriod_Success(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	notes := "Test notes"
	period := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1000,
		EffectiveEndUnix:   floatPtr(2000),
		IsActive:           true,
		Notes:              &notes,
		CosineErrorAngle:   25.5,
	}
	if err := db.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	retrieved, err := db.GetSiteConfigPeriod(period.ID)
	if err != nil {
		t.Fatalf("GetSiteConfigPeriod failed: %v", err)
	}

	if retrieved.ID != period.ID {
		t.Errorf("Expected ID %d, got %d", period.ID, retrieved.ID)
	}
	if retrieved.SiteID != site.ID {
		t.Errorf("Expected SiteID %d, got %d", site.ID, retrieved.SiteID)
	}
	if retrieved.EffectiveStartUnix != 1000 {
		t.Errorf("Expected EffectiveStartUnix 1000, got %f", retrieved.EffectiveStartUnix)
	}
	if retrieved.EffectiveEndUnix == nil || *retrieved.EffectiveEndUnix != 2000 {
		t.Errorf("Expected EffectiveEndUnix 2000, got %v", retrieved.EffectiveEndUnix)
	}
	if !retrieved.IsActive {
		t.Error("Expected IsActive to be true")
	}
	if retrieved.Notes == nil || *retrieved.Notes != notes {
		t.Errorf("Expected Notes %q, got %v", notes, retrieved.Notes)
	}
	if math.Abs(retrieved.CosineErrorAngle-25.5) > 0.001 {
		t.Errorf("Expected CosineErrorAngle 25.5, got %f", retrieved.CosineErrorAngle)
	}
	if retrieved.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
	if retrieved.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt to be set")
	}
}

// TestGetSiteConfigPeriod_NotFound tests retrieving a non-existent period
func TestGetSiteConfigPeriod_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	_, err := db.GetSiteConfigPeriod(99999)
	if err == nil {
		t.Error("Expected error for non-existent period, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestGetActiveSiteConfigPeriod_Success tests retrieving the active period
func TestGetActiveSiteConfigPeriod_Success(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Create inactive period
	period1 := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1000,
		EffectiveEndUnix:   floatPtr(2000),
		IsActive:           false,
		CosineErrorAngle:   10.0,
	}
	if err := db.CreateSiteConfigPeriod(period1); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	// Create active period
	period2 := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 2000,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		CosineErrorAngle:   15.0,
	}
	if err := db.CreateSiteConfigPeriod(period2); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	active, err := db.GetActiveSiteConfigPeriod(site.ID)
	if err != nil {
		t.Fatalf("GetActiveSiteConfigPeriod failed: %v", err)
	}

	if active.ID != period2.ID {
		t.Errorf("Expected active period ID %d, got %d", period2.ID, active.ID)
	}
	if !active.IsActive {
		t.Error("Expected IsActive to be true")
	}
}

// TestGetActiveSiteConfigPeriod_NotFound tests retrieving active period when none exists
func TestGetActiveSiteConfigPeriod_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	_, err := db.GetActiveSiteConfigPeriod(site.ID)
	if err == nil {
		t.Error("Expected error for non-existent active period, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestCreateSiteConfigPeriod_Validation tests validation errors
func TestCreateSiteConfigPeriod_Validation(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	tests := []struct {
		name        string
		period      *SiteConfigPeriod
		expectedErr string
	}{
		{
			name: "missing site_id",
			period: &SiteConfigPeriod{
				SiteID:             0,
				EffectiveStartUnix: 1000,
				CosineErrorAngle:   10.0,
			},
			expectedErr: "site_id is required",
		},
		{
			name: "negative start time",
			period: &SiteConfigPeriod{
				SiteID:             site.ID,
				EffectiveStartUnix: -100,
				CosineErrorAngle:   10.0,
			},
			expectedErr: "must be non-negative",
		},
		{
			name: "end before start",
			period: &SiteConfigPeriod{
				SiteID:             site.ID,
				EffectiveStartUnix: 2000,
				EffectiveEndUnix:   floatPtr(1000),
				CosineErrorAngle:   10.0,
			},
			expectedErr: "must be greater than",
		},
		{
			name: "NaN cosine angle",
			period: &SiteConfigPeriod{
				SiteID:             site.ID,
				EffectiveStartUnix: 1000,
				CosineErrorAngle:   math.NaN(),
			},
			expectedErr: "must be a valid number",
		},
		{
			name: "negative cosine angle",
			period: &SiteConfigPeriod{
				SiteID:             site.ID,
				EffectiveStartUnix: 1000,
				CosineErrorAngle:   -5.0,
			},
			expectedErr: "between 0 and 80",
		},
		{
			name: "cosine angle too large",
			period: &SiteConfigPeriod{
				SiteID:             site.ID,
				EffectiveStartUnix: 1000,
				CosineErrorAngle:   85.0,
			},
			expectedErr: "between 0 and 80",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.CreateSiteConfigPeriod(tt.period)
			if err == nil {
				t.Errorf("Expected error containing %q, got nil", tt.expectedErr)
			} else if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error containing %q, got: %v", tt.expectedErr, err)
			}
		})
	}
}

// TestUpdateSiteConfigPeriod_Success tests successful update
func TestUpdateSiteConfigPeriod_Success(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	period := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1000,
		EffectiveEndUnix:   floatPtr(2000),
		IsActive:           false,
		CosineErrorAngle:   10.0,
	}
	if err := db.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	// Update the period
	period.CosineErrorAngle = 20.0
	period.IsActive = true
	newNotes := "Updated notes"
	period.Notes = &newNotes

	if err := db.UpdateSiteConfigPeriod(period); err != nil {
		t.Fatalf("UpdateSiteConfigPeriod failed: %v", err)
	}

	// Verify the update
	retrieved, err := db.GetSiteConfigPeriod(period.ID)
	if err != nil {
		t.Fatalf("GetSiteConfigPeriod failed: %v", err)
	}

	if math.Abs(retrieved.CosineErrorAngle-20.0) > 0.001 {
		t.Errorf("Expected CosineErrorAngle 20.0, got %f", retrieved.CosineErrorAngle)
	}
	if !retrieved.IsActive {
		t.Error("Expected IsActive to be true")
	}
	if retrieved.Notes == nil || *retrieved.Notes != newNotes {
		t.Errorf("Expected Notes %q, got %v", newNotes, retrieved.Notes)
	}
}

// TestUpdateSiteConfigPeriod_MissingID tests update without ID
func TestUpdateSiteConfigPeriod_MissingID(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	period := &SiteConfigPeriod{
		ID:                 0,
		SiteID:             1,
		EffectiveStartUnix: 1000,
		CosineErrorAngle:   10.0,
	}

	err := db.UpdateSiteConfigPeriod(period)
	if err == nil {
		t.Error("Expected error for missing ID, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "ID is required") {
		t.Errorf("Expected 'ID is required' error, got: %v", err)
	}
}

// TestUpdateSiteConfigPeriod_NotFound tests updating non-existent period
func TestUpdateSiteConfigPeriod_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	period := &SiteConfigPeriod{
		ID:                 99999,
		SiteID:             site.ID,
		EffectiveStartUnix: 1000,
		CosineErrorAngle:   10.0,
	}

	err := db.UpdateSiteConfigPeriod(period)
	if err == nil {
		t.Error("Expected error for non-existent period, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestUpdateSiteConfigPeriod_OverlapDetection tests overlap detection during update
func TestUpdateSiteConfigPeriod_OverlapDetection(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Create two non-overlapping periods
	period1 := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1000,
		EffectiveEndUnix:   floatPtr(2000),
		IsActive:           false,
		CosineErrorAngle:   10.0,
	}
	if err := db.CreateSiteConfigPeriod(period1); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	period2 := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 3000,
		EffectiveEndUnix:   floatPtr(4000),
		IsActive:           false,
		CosineErrorAngle:   15.0,
	}
	if err := db.CreateSiteConfigPeriod(period2); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	// Try to update period2 to overlap with period1
	period2.EffectiveStartUnix = 1500
	err := db.UpdateSiteConfigPeriod(period2)
	if err == nil {
		t.Error("Expected overlap error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "overlaps") {
		t.Errorf("Expected 'overlaps' error, got: %v", err)
	}
}

// TestSiteConfigPeriod_NullableFields tests handling of nullable fields
func TestSiteConfigPeriod_NullableFields(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:     "Test Site",
		Location: "Test Location",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	// Create period with nil EffectiveEndUnix and Notes
	period := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1000,
		EffectiveEndUnix:   nil,
		IsActive:           true,
		Notes:              nil,
		CosineErrorAngle:   10.0,
	}
	if err := db.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := db.GetSiteConfigPeriod(period.ID)
	if err != nil {
		t.Fatalf("GetSiteConfigPeriod failed: %v", err)
	}

	if retrieved.EffectiveEndUnix != nil {
		t.Errorf("Expected nil EffectiveEndUnix, got %v", retrieved.EffectiveEndUnix)
	}
	if retrieved.Notes != nil {
		t.Errorf("Expected nil Notes, got %v", retrieved.Notes)
	}
}

// TestSiteConfigPeriod_OverlapScenarios tests various overlap scenarios
func TestSiteConfigPeriod_OverlapScenarios(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Create multiple sites to avoid inter-test overlap
	tests := []struct {
		name          string
		existing      []struct{ start, end float64 }
		newStart      float64
		newEnd        *float64
		shouldOverlap bool
	}{
		{
			name:          "no overlap - before",
			existing:      []struct{ start, end float64 }{{1000, 2000}},
			newStart:      500,
			newEnd:        floatPtr(800),
			shouldOverlap: false,
		},
		{
			name:          "no overlap - after",
			existing:      []struct{ start, end float64 }{{1000, 2000}},
			newStart:      2500,
			newEnd:        floatPtr(3000),
			shouldOverlap: false,
		},
		{
			name:          "overlap - start inside",
			existing:      []struct{ start, end float64 }{{1000, 2000}},
			newStart:      1500,
			newEnd:        floatPtr(2500),
			shouldOverlap: true,
		},
		{
			name:          "overlap - end inside",
			existing:      []struct{ start, end float64 }{{1000, 2000}},
			newStart:      500,
			newEnd:        floatPtr(1500),
			shouldOverlap: true,
		},
		{
			name:          "overlap - completely contains",
			existing:      []struct{ start, end float64 }{{1000, 2000}},
			newStart:      500,
			newEnd:        floatPtr(2500),
			shouldOverlap: true,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create unique site for this test to avoid overlap between tests
			site := &Site{
				Name:     fmt.Sprintf("Test Site %d", i),
				Location: "Test Location",
				Surveyor: "Surveyor",
				Contact:  "contact@example.com",
			}
			if err := db.CreateSite(site); err != nil {
				t.Fatalf("CreateSite failed: %v", err)
			}

			// Create existing periods
			for _, existing := range tt.existing {
				var endPtr *float64
				if existing.end != 0 {
					endPtr = &existing.end
				}
				period := &SiteConfigPeriod{
					SiteID:             site.ID,
					EffectiveStartUnix: existing.start,
					EffectiveEndUnix:   endPtr,
					IsActive:           false,
					CosineErrorAngle:   10.0,
				}
				if err := db.CreateSiteConfigPeriod(period); err != nil {
					t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
				}
			}

			// Try to create new period
			newPeriod := &SiteConfigPeriod{
				SiteID:             site.ID,
				EffectiveStartUnix: tt.newStart,
				EffectiveEndUnix:   tt.newEnd,
				IsActive:           false,
				CosineErrorAngle:   15.0,
			}
			err := db.CreateSiteConfigPeriod(newPeriod)

			if tt.shouldOverlap {
				if err == nil {
					t.Error("Expected overlap error, got nil")
				} else if !strings.Contains(err.Error(), "overlaps") {
					t.Errorf("Expected 'overlaps' error, got: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}
