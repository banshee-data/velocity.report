package db

import (
	"fmt"
	"testing"
	"time"
)

func TestTransitGap_Struct(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Hour)
	gap := TransitGap{
		Start:       now,
		End:         now.Add(time.Hour),
		RecordCount: 100,
	}

	if gap.Start != now {
		t.Errorf("Start mismatch: got %v, want %v", gap.Start, now)
	}
	if gap.End != now.Add(time.Hour) {
		t.Errorf("End mismatch: got %v, want %v", gap.End, now.Add(time.Hour))
	}
	if gap.RecordCount != 100 {
		t.Errorf("RecordCount mismatch: got %d, want 100", gap.RecordCount)
	}
}

func TestFindTransitGaps_EmptyDB(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	gaps, err := db.FindTransitGaps()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(gaps) != 0 {
		t.Errorf("Expected 0 gaps for empty database, got %d", len(gaps))
	}
}

// insertRadarData is a test helper that inserts radar_data using raw_event JSON
func insertRadarData(t *testing.T, db *DB, writeTimestamp float64, speed, magnitude float64) {
	rawEvent := fmt.Sprintf(`{"speed": %f, "magnitude": %f}`, speed, magnitude)
	_, err := db.Exec(`INSERT INTO radar_data (write_timestamp, raw_event) VALUES (?, ?)`,
		writeTimestamp, rawEvent)
	if err != nil {
		t.Fatalf("Failed to insert radar_data: %v", err)
	}
}

// insertTransit is a test helper that inserts a transit record
func insertTransit(t *testing.T, db *DB, key string, startUnix, endUnix float64) {
	_, err := db.Exec(`
		INSERT INTO radar_data_transits (transit_key, threshold_ms, transit_start_unix, transit_end_unix, transit_max_speed, point_count, model_version)
		VALUES (?, 100, ?, ?, 50.0, 10, 'test')
	`, key, startUnix, endUnix)
	if err != nil {
		t.Fatalf("Failed to insert transit: %v", err)
	}
}

func TestFindTransitGaps_NoGaps(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert matching radar_data and transits
	now := time.Now().UTC().Truncate(time.Hour)
	writeTimestamp := float64(now.Unix())

	// Insert radar_data using raw_event
	insertRadarData(t, db, writeTimestamp, 50.0, 100.0)

	// Insert matching transit
	insertTransit(t, db, "test_transit_1", writeTimestamp, writeTimestamp+1)

	gaps, err := db.FindTransitGaps()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(gaps) != 0 {
		t.Errorf("Expected 0 gaps when transits match, got %d", len(gaps))
	}
}

func TestFindTransitGaps_WithGaps(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert radar_data without any transits
	now := time.Now().UTC().Truncate(time.Hour)
	writeTimestamp := float64(now.Unix())

	// Insert radar_data for two hours
	insertRadarData(t, db, writeTimestamp, 50.0, 100.0)
	insertRadarData(t, db, writeTimestamp+3600, 60.0, 120.0) // Next hour

	gaps, err := db.FindTransitGaps()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(gaps) != 2 {
		t.Fatalf("Expected 2 gaps, got %d", len(gaps))
	}

	// First gap should be the earlier hour
	if gaps[0].Start.Unix() != now.Unix() {
		t.Errorf("First gap start mismatch: got %v, want %v", gaps[0].Start, now)
	}
	if gaps[0].RecordCount != 1 {
		t.Errorf("First gap record count mismatch: got %d, want 1", gaps[0].RecordCount)
	}
}

func TestFindTransitGaps_PartialCoverage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert radar_data for 3 hours, with transits only for the middle hour
	now := time.Now().UTC().Truncate(time.Hour)

	hours := []float64{
		float64(now.Unix()),
		float64(now.Add(time.Hour).Unix()),
		float64(now.Add(2 * time.Hour).Unix()),
	}

	// Insert radar_data for all 3 hours
	for _, ts := range hours {
		insertRadarData(t, db, ts, 50.0, 100.0)
	}

	// Insert transit only for middle hour
	insertTransit(t, db, "test_transit_middle", hours[1], hours[1]+1)

	gaps, err := db.FindTransitGaps()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should find 2 gaps (first and third hour)
	if len(gaps) != 2 {
		t.Fatalf("Expected 2 gaps, got %d", len(gaps))
	}

	// Check that gaps are for correct hours
	if gaps[0].Start.Unix() != now.Unix() {
		t.Errorf("First gap start mismatch: got %v, want %v", gaps[0].Start, now)
	}
	if gaps[1].Start.Unix() != now.Add(2*time.Hour).Unix() {
		t.Errorf("Second gap start mismatch: got %v, want %v", gaps[1].Start, now.Add(2*time.Hour))
	}
}

func TestFindTransitGaps_NullData(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert radar_data with no speed or magnitude values (should be excluded)
	now := time.Now().UTC().Truncate(time.Hour)
	writeTimestamp := float64(now.Unix())

	// Insert radar_data with empty raw_event (no speed/magnitude)
	_, err := db.Exec(`INSERT INTO radar_data (write_timestamp, raw_event) VALUES (?, ?)`,
		writeTimestamp, `{}`)
	if err != nil {
		t.Fatalf("Failed to insert radar_data: %v", err)
	}

	gaps, err := db.FindTransitGaps()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// No gaps because the data with NULL values is excluded
	if len(gaps) != 0 {
		t.Errorf("Expected 0 gaps for NULL data, got %d", len(gaps))
	}
}

func TestFindTransitGaps_MultipleRecordsPerHour(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert multiple radar_data records in the same hour
	now := time.Now().UTC().Truncate(time.Hour)
	baseTimestamp := float64(now.Unix())

	for i := 0; i < 10; i++ {
		insertRadarData(t, db, baseTimestamp+float64(i*60), 50.0+float64(i), 100.0+float64(i))
	}

	gaps, err := db.FindTransitGaps()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(gaps) != 1 {
		t.Fatalf("Expected 1 gap, got %d", len(gaps))
	}

	// Should count all 10 records
	if gaps[0].RecordCount != 10 {
		t.Errorf("Expected 10 records in gap, got %d", gaps[0].RecordCount)
	}
}
