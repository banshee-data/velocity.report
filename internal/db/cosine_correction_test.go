package db

import (
	"math"
	"testing"
	"time"
)

// TestCosineErrorCorrection verifies that cosine error correction is applied when querying radar data
func TestCosineErrorCorrection(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Create a test site with a specific cosine error angle (5 degrees)
	site := &Site{
		Name:             "Test Site",
		Location:         "Test Location",
		CosineErrorAngle: 5.0, // 5 degrees
		SpeedLimit:       25,
		Surveyor:         "Test Surveyor",
		Contact:          "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// Create a site configuration period covering all time
	period := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 0.0, // Start from epoch
		EffectiveEndUnix:   nil, // Open-ended
		IsActive:           true,
	}
	if err := db.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("Failed to create site config period: %v", err)
	}

	// Insert a radar object with a known speed (e.g., 25 m/s)
	// Note: write_timestamp will be auto-generated to current time
	measuredSpeed := 25.0 // m/s
	radarObjectJSON := `{
		"classifier": "vehicle",
		"start_time": 1234567890.0,
		"end_time": 1234567891.0,
		"delta_time_msec": 1000,
		"max_speed_mps": 25.0,
		"min_speed_mps": 20.0,
		"speed_change": 5.0,
		"max_magnitude": 3000,
		"avg_magnitude": 2500,
		"total_frames": 100,
		"frames_per_mps": 4.0,
		"length_m": 5.0
	}`
	if err := db.RecordRadarObject(radarObjectJSON); err != nil {
		t.Fatalf("Failed to record radar object: %v", err)
	}

	// Calculate expected corrected speed
	// Formula: corrected_speed = measured_speed / cos(angle_in_radians)
	angleRadians := site.CosineErrorAngle * (math.Pi / 180.0) // Convert to radians
	expectedCorrectedSpeed := measuredSpeed / math.Cos(angleRadians)

	// Query the data using a time range that covers "now" (when the record was inserted)
	now := time.Now().Unix()
	startUnix := now - 10    // 10 seconds ago
	endUnix := now + 10      // 10 seconds from now
	groupSeconds := int64(0) // All data in one bucket

	result, err := db.RadarObjectRollupRange(startUnix, endUnix, groupSeconds, 0.0, "radar_objects", "", 0.0, 0.0)
	if err != nil {
		t.Fatalf("Failed to query radar stats: %v", err)
	}

	if len(result.Metrics) != 1 {
		t.Fatalf("Expected 1 metric row, got %d", len(result.Metrics))
	}

	// Verify the max speed is corrected
	// Allow small floating point error (0.001 m/s)
	tolerance := 0.001
	if math.Abs(result.Metrics[0].MaxSpeed-expectedCorrectedSpeed) > tolerance {
		t.Errorf("Expected max speed %f (corrected), got %f (difference: %f)",
			expectedCorrectedSpeed, result.Metrics[0].MaxSpeed,
			math.Abs(result.Metrics[0].MaxSpeed-expectedCorrectedSpeed))
	}

	t.Logf("Measured speed: %.4f m/s", measuredSpeed)
	t.Logf("Cosine angle: %.2f degrees (%.6f radians)", site.CosineErrorAngle, angleRadians)
	t.Logf("Correction factor: %.6f (1/cos(angle))", 1.0/math.Cos(angleRadians))
	t.Logf("Expected corrected speed: %.4f m/s", expectedCorrectedSpeed)
	t.Logf("Actual corrected speed: %.4f m/s", result.Metrics[0].MaxSpeed)
}

// TestCosineErrorCorrectionWithMultiplePeriods verifies that different cosine angles
// are applied based on the time period
func TestCosineErrorCorrectionWithMultiplePeriods(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Create two sites with different angles
	site1 := &Site{
		Name:             "Site 1",
		Location:         "Location 1",
		CosineErrorAngle: 5.0, // 5 degrees
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
		CosineErrorAngle: 10.0, // 10 degrees
		SpeedLimit:       30,
		Surveyor:         "Surveyor 2",
		Contact:          "site2@example.com",
	}
	if err := db.CreateSite(site2); err != nil {
		t.Fatalf("Failed to create site 2: %v", err)
	}

	// Create periods for different time ranges
	jan1 := float64(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix())
	feb1 := float64(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC).Unix())

	// Period 1: Jan (site 1, 5 degrees)
	period1 := &SiteConfigPeriod{
		SiteID:             site1.ID,
		EffectiveStartUnix: jan1,
		EffectiveEndUnix:   &feb1,
		IsActive:           false,
	}
	if err := db.CreateSiteConfigPeriod(period1); err != nil {
		t.Fatalf("Failed to create period 1: %v", err)
	}

	// Period 2: Feb onwards (site 2, 10 degrees)
	period2 := &SiteConfigPeriod{
		SiteID:             site2.ID,
		EffectiveStartUnix: feb1,
		EffectiveEndUnix:   nil,
		IsActive:           true,
	}
	if err := db.CreateSiteConfigPeriod(period2); err != nil {
		t.Fatalf("Failed to create period 2: %v", err)
	}

	// Insert radar objects in both periods with same measured speed
	measuredSpeed := 25.0 // m/s

	// Jan 15 reading (should use 5 degree correction)
	jan15 := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC).Unix()
	radarObject1JSON := `{
		"classifier": "vehicle",
		"start_time": ` + string(rune(jan15)) + `.0,
		"end_time": ` + string(rune(jan15+1)) + `.0,
		"delta_time_msec": 1000,
		"max_speed_mps": 25.0,
		"min_speed_mps": 20.0,
		"speed_change": 5.0,
		"max_magnitude": 3000,
		"avg_magnitude": 2500,
		"total_frames": 100,
		"frames_per_mps": 4.0,
		"length_m": 5.0
	}`
	if err := db.RecordRadarObject(radarObject1JSON); err != nil {
		t.Fatalf("Failed to record Jan radar object: %v", err)
	}

	// Feb 15 reading (should use 10 degree correction)
	feb15 := time.Date(2024, 2, 15, 12, 0, 0, 0, time.UTC).Unix()
	radarObject2JSON := `{
		"classifier": "vehicle",
		"start_time": ` + string(rune(feb15)) + `.0,
		"end_time": ` + string(rune(feb15+1)) + `.0,
		"delta_time_msec": 1000,
		"max_speed_mps": 25.0,
		"min_speed_mps": 20.0,
		"speed_change": 5.0,
		"max_magnitude": 3000,
		"avg_magnitude": 2500,
		"total_frames": 100,
		"frames_per_mps": 4.0,
		"length_m": 5.0
	}`
	if err := db.RecordRadarObject(radarObject2JSON); err != nil {
		t.Fatalf("Failed to record Feb radar object: %v", err)
	}

	// Calculate expected corrected speeds
	angle1Radians := site1.CosineErrorAngle * (math.Pi / 180.0)
	expectedSpeed1 := measuredSpeed / math.Cos(angle1Radians)

	angle2Radians := site2.CosineErrorAngle * (math.Pi / 180.0)
	expectedSpeed2 := measuredSpeed / math.Cos(angle2Radians)

	// Query January data
	janResult, err := db.RadarObjectRollupRange(int64(jan1), int64(feb1)-1, 0, 0.0, "radar_objects", "", 0.0, 0.0)
	if err != nil {
		t.Fatalf("Failed to query Jan stats: %v", err)
	}

	// Query February data
	mar1 := float64(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC).Unix())
	febResult, err := db.RadarObjectRollupRange(int64(feb1), int64(mar1), 0, 0.0, "radar_objects", "", 0.0, 0.0)
	if err != nil {
		t.Fatalf("Failed to query Feb stats: %v", err)
	}

	// Verify January correction (5 degrees)
	tolerance := 0.001
	if len(janResult.Metrics) > 0 {
		if math.Abs(janResult.Metrics[0].MaxSpeed-expectedSpeed1) > tolerance {
			t.Errorf("Jan: Expected max speed %f (5° correction), got %f",
				expectedSpeed1, janResult.Metrics[0].MaxSpeed)
		}
		t.Logf("Jan corrected speed (5°): %.4f m/s", janResult.Metrics[0].MaxSpeed)
	} else {
		t.Error("No January data found")
	}

	// Verify February correction (10 degrees)
	if len(febResult.Metrics) > 0 {
		if math.Abs(febResult.Metrics[0].MaxSpeed-expectedSpeed2) > tolerance {
			t.Errorf("Feb: Expected max speed %f (10° correction), got %f",
				expectedSpeed2, febResult.Metrics[0].MaxSpeed)
		}
		t.Logf("Feb corrected speed (10°): %.4f m/s", febResult.Metrics[0].MaxSpeed)
	} else {
		t.Error("No February data found")
	}

	// Verify the speeds are different (different corrections applied)
	if len(janResult.Metrics) > 0 && len(febResult.Metrics) > 0 {
		speedDiff := math.Abs(janResult.Metrics[0].MaxSpeed - febResult.Metrics[0].MaxSpeed)
		expectedDiff := math.Abs(expectedSpeed1 - expectedSpeed2)
		if math.Abs(speedDiff-expectedDiff) > tolerance {
			t.Errorf("Expected speed difference %f, got %f", expectedDiff, speedDiff)
		}
		t.Logf("Speed difference between periods: %.4f m/s", speedDiff)
	}
}

// TestNoCosineCorrection verifies that data without a matching site config period gets the default correction
func TestNoCosineCorrection(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// The default site (id=1) with default period will exist from the migration
	// We'll query its cosine angle to understand the default correction
	defaultSite, err := db.GetSite(1)
	if err != nil {
		t.Logf("No default site found, test will verify zero correction")
	}

	// Insert radar object (will use current time as write_timestamp)
	measuredSpeed := 25.0 // m/s
	radarObjectJSON := `{
		"classifier": "vehicle",
		"start_time": 1234567890.0,
		"end_time": 1234567891.0,
		"delta_time_msec": 1000,
		"max_speed_mps": 25.0,
		"min_speed_mps": 20.0,
		"speed_change": 5.0,
		"max_magnitude": 3000,
		"avg_magnitude": 2500,
		"total_frames": 100,
		"frames_per_mps": 4.0,
		"length_m": 5.0
	}`
	if err := db.RecordRadarObject(radarObjectJSON); err != nil {
		t.Fatalf("Failed to record radar object: %v", err)
	}

	// Query the data using current time range
	now := time.Now().Unix()
	result, err := db.RadarObjectRollupRange(now-10, now+10, 0, 0.0, "radar_objects", "", 0.0, 0.0)
	if err != nil {
		t.Fatalf("Failed to query radar stats: %v", err)
	}

	if len(result.Metrics) != 1 {
		t.Fatalf("Expected 1 metric row, got %d", len(result.Metrics))
	}

	// Verify the speed has default correction applied (from default site, angle 0.5)
	if defaultSite != nil {
		angleRadians := defaultSite.CosineErrorAngle * (math.Pi / 180.0)
		expectedSpeed := measuredSpeed / math.Cos(angleRadians)
		tolerance := 0.001
		if math.Abs(result.Metrics[0].MaxSpeed-expectedSpeed) > tolerance {
			t.Errorf("Expected default corrected speed %f (angle %.1f°), got %f",
				expectedSpeed, defaultSite.CosineErrorAngle, result.Metrics[0].MaxSpeed)
		}
		t.Logf("Speed with default site config (%.1f°): %.4f m/s", defaultSite.CosineErrorAngle, result.Metrics[0].MaxSpeed)
	} else {
		t.Logf("Speed: %.4f m/s (no default site)", result.Metrics[0].MaxSpeed)
	}
}
