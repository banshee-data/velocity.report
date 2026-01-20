package db

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestSiteConfigPeriodOverlap(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	site := &Site{
		Name:             "Overlap Site",
		Location:         "Test Location",
		CosineErrorAngle: 10.0,
		Surveyor:         "Surveyor",
		Contact:          "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
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
		Name:             "Correction Site",
		Location:         "Test Location",
		CosineErrorAngle: 60.0,
		Surveyor:         "Surveyor",
		Contact:          "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	active, err := db.GetActiveSiteConfigPeriod(site.ID)
	if err != nil {
		t.Fatalf("GetActiveSiteConfigPeriod failed: %v", err)
	}
	active.CosineErrorAngle = 60.0
	if err := db.UpdateSiteConfigPeriod(active); err != nil {
		t.Fatalf("UpdateSiteConfigPeriod failed: %v", err)
	}

	now := time.Now().Unix()
	event := map[string]interface{}{
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

	result, err := db.RadarObjectRollupRange(now-60, now+60, 0, 0, "radar_objects", "", 0, 0, site.ID)
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
		Name:             "Correction Site",
		Location:         "Test Location",
		CosineErrorAngle: 60.0,
		Surveyor:         "Surveyor",
		Contact:          "contact@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}

	active, err := db.GetActiveSiteConfigPeriod(site.ID)
	if err != nil {
		t.Fatalf("GetActiveSiteConfigPeriod failed: %v", err)
	}
	active.CosineErrorAngle = 60.0
	if err := db.UpdateSiteConfigPeriod(active); err != nil {
		t.Fatalf("UpdateSiteConfigPeriod failed: %v", err)
	}

	now := time.Now().Unix()
	event := map[string]interface{}{
		"speed":     10.0,
		"uptime":    1.0,
		"magnitude": 5.0,
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}
	if err := db.RecordRawData(string(eventJSON)); err != nil {
		t.Fatalf("Failed to insert radar data: %v", err)
	}

	result, err := db.RadarObjectRollupRange(now-60, now+60, 0, 0, "radar_data", "", 0, 0, site.ID)
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
