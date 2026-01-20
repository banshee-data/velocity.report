package db

import (
	"fmt"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestRadarObjectRollupRange_MinSpeed verifies that the minSpeed parameter filters
// radar_objects correctly and that the rollup returns aggregated buckets.
func TestRadarObjectRollupRange_MinSpeed(t *testing.T) {
	// create temporary DB file
	fname := "test_rollup.db"
	_ = os.Remove(fname)
	defer os.Remove(fname)

	dbinst, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer dbinst.Close()

	// Insert a few radar_objects rows with raw_event JSON. We'll keep write_timestamp
	// and max_speed values encoded in the JSON so the virtual columns populate.
	// Three events spaced by 60s. Speeds: 1.0 m/s, 3.0 m/s, 6.0 m/s
	now := time.Now().Unix()
	events := []struct {
		ts  int64
		max float64
	}{
		{now, 1.0},
		{now + 60, 3.0},
		{now + 120, 6.0},
	}

	for _, e := range events {
		raw := fmt.Sprintf(`{"start_time": 0, "end_time": 0, "delta_time_msec": 0, "max_speed_mps": %f, "min_speed_mps": 0, "speed_change": 0, "max_magnitude": 0, "avg_magnitude": 0, "total_frames": 0, "frames_per_mps": 0, "length_m": 0, "classifier": "all"}`, e.max)
		if _, err := dbinst.Exec(`INSERT INTO radar_objects (raw_event, write_timestamp) VALUES (?, ?)`, raw, float64(e.ts)); err != nil {
			t.Fatalf("insert failed: %v", err)
		}
	}

	// Query rollup with minSpeed = 2.0 m/s (should include 3.0 and 6.0)
	start := now - 10
	end := now + 1000
	result, err := dbinst.RadarObjectRollupRange(start, end, 300, 2.0, "radar_objects", "", 0, 0, 0)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	// We expect at least one bucket and the aggregated Count should be 2
	total := int64(0)
	for _, r := range result.Metrics {
		total += r.Count
	}
	if total != 2 {
		t.Fatalf("expected total count 2 after filtering, got %d", total)
	}
}

// TestRadarObjectRollupRange_EmptyDB verifies behavior with no data
func TestRadarObjectRollupRange_EmptyDB(t *testing.T) {
	fname := t.TempDir() + "/empty_rollup.db"
	dbinst, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer dbinst.Close()

	now := time.Now().Unix()
	result, err := dbinst.RadarObjectRollupRange(now-1000, now, 300, 0, "radar_objects", "", 0, 0, 0)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	// Should return empty metrics, not error
	if result.Metrics == nil {
		t.Error("expected non-nil Metrics slice")
	}
}

// TestRadarObjectRollupRange_WithHistogram verifies histogram generation
func TestRadarObjectRollupRange_WithHistogram(t *testing.T) {
	fname := t.TempDir() + "/hist_rollup.db"
	dbinst, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer dbinst.Close()

	now := time.Now().Unix()
	events := []struct {
		ts  int64
		max float64
	}{
		{now, 5.0},
		{now + 10, 10.0},
		{now + 20, 15.0},
		{now + 30, 20.0},
		{now + 40, 25.0},
	}

	for _, e := range events {
		raw := fmt.Sprintf(`{"start_time": 0, "end_time": 0, "delta_time_msec": 0, "max_speed_mps": %f, "min_speed_mps": 0, "speed_change": 0, "max_magnitude": 0, "avg_magnitude": 0, "total_frames": 0, "frames_per_mps": 0, "length_m": 0, "classifier": "all"}`, e.max)
		if _, err := dbinst.Exec(`INSERT INTO radar_objects (raw_event, write_timestamp) VALUES (?, ?)`, raw, float64(e.ts)); err != nil {
			t.Fatalf("insert failed: %v", err)
		}
	}

	// Query with histogram parameters
	start := now - 10
	end := now + 1000
	bucketSize := 5.0
	histMax := 100.0
	result, err := dbinst.RadarObjectRollupRange(start, end, 300, 0, "radar_objects", "", bucketSize, histMax, 0)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	// Check that metrics are returned
	if len(result.Metrics) == 0 {
		t.Error("expected at least one metric bucket")
	}

	// Check count of events
	total := int64(0)
	for _, r := range result.Metrics {
		total += r.Count
	}
	if total != 5 {
		t.Errorf("expected total count 5, got %d", total)
	}
}

// TestRadarObjectRollupRange_TransitsSource tests the radar_data_transits source
func TestRadarObjectRollupRange_TransitsSource(t *testing.T) {
	fname := t.TempDir() + "/transits_rollup.db"
	dbinst, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer dbinst.Close()

	now := time.Now().Unix()

	// Insert into radar_data_transits table using correct column names
	// Include model_version which is required for the query
	for i := 0; i < 5; i++ {
		ts := float64(now + int64(i*10))
		_, err := dbinst.Exec(`INSERT INTO radar_data_transits (
			transit_key, threshold_ms, transit_start_unix, transit_end_unix, 
			transit_max_speed, transit_min_speed, point_count, model_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("transit_%d", i), 100, ts, ts+1, 10.0+float64(i), 5.0, 10, "rebuild-full")
		if err != nil {
			t.Fatalf("insert failed: %v", err)
		}
	}

	start := now - 10
	end := now + 1000
	// Use "rebuild-full" model version which is what was inserted
	result, err := dbinst.RadarObjectRollupRange(start, end, 300, 0, "radar_data_transits", "rebuild-full", 0, 0, 0)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	total := int64(0)
	for _, r := range result.Metrics {
		total += r.Count
	}
	if total != 5 {
		t.Errorf("expected 5 transits, got %d", total)
	}
}

// TestRadarObjectRollupRange_DifferentGroupSizes tests various grouping intervals
func TestRadarObjectRollupRange_DifferentGroupSizes(t *testing.T) {
	fname := t.TempDir() + "/group_rollup.db"
	dbinst, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer dbinst.Close()

	now := time.Now().Unix()

	// Insert events spread over 2 hours
	for i := 0; i < 24; i++ {
		ts := now + int64(i*300) // Every 5 minutes
		raw := fmt.Sprintf(`{"start_time": %d, "end_time": %d, "delta_time_msec": 100, "max_speed_mps": %f, "min_speed_mps": 5, "speed_change": 0, "max_magnitude": 50, "avg_magnitude": 40, "total_frames": 10, "frames_per_mps": 1, "length_m": 5, "classifier": "all"}`, ts, ts+1, float64(10+i))
		if _, err := dbinst.Exec(`INSERT INTO radar_objects (raw_event, write_timestamp) VALUES (?, ?)`, raw, float64(ts)); err != nil {
			t.Fatalf("insert failed: %v", err)
		}
	}

	testCases := []struct {
		name        string
		groupByNSec int64
	}{
		{"5min", 300},
		{"10min", 600},
		{"1hour", 3600},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			start := now - 10
			end := now + 10000
			result, err := dbinst.RadarObjectRollupRange(start, end, tc.groupByNSec, 0, "radar_objects", "", 0, 0, 0)
			if err != nil {
				t.Fatalf("RadarObjectRollupRange failed: %v", err)
			}

			total := int64(0)
			for _, r := range result.Metrics {
				total += r.Count
			}
			if total != 24 {
				t.Errorf("expected 24 total events with grouping %s, got %d", tc.name, total)
			}
		})
	}
}

// TestRadarObjectRollupRange_InvalidSource tests error handling for invalid source
func TestRadarObjectRollupRange_InvalidSource(t *testing.T) {
	fname := t.TempDir() + "/invalid_source.db"
	dbinst, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer dbinst.Close()

	now := time.Now().Unix()
	_, err = dbinst.RadarObjectRollupRange(now-1000, now, 300, 0, "invalid_table", "", 0, 0, 0)
	if err == nil {
		t.Error("expected error for invalid source, got nil")
	}
}

// TestRadarObjectRollupRange_ClassifierFilter tests that modelVersion parameter works for transits
func TestRadarObjectRollupRange_ClassifierFilter(t *testing.T) {
	fname := t.TempDir() + "/classifier_rollup.db"
	dbinst, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer dbinst.Close()

	now := time.Now().Unix()

	// Insert transits with different model_version values
	models := []string{"rebuild-full", "v2", "rebuild-full", "rebuild-full"}
	for i, model := range models {
		ts := float64(now + int64(i*10))
		_, err := dbinst.Exec(`INSERT INTO radar_data_transits (
			transit_key, threshold_ms, transit_start_unix, transit_end_unix, 
			transit_max_speed, transit_min_speed, point_count, model_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("transit_%d", i), 100, ts, ts+1, 10.0, 5.0, 10, model)
		if err != nil {
			t.Fatalf("insert failed: %v", err)
		}
	}

	// Filter by rebuild-full model only (default)
	start := now - 10
	end := now + 1000
	result, err := dbinst.RadarObjectRollupRange(start, end, 300, 0, "radar_data_transits", "rebuild-full", 0, 0, 0)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	total := int64(0)
	for _, r := range result.Metrics {
		total += r.Count
	}
	if total != 3 {
		t.Errorf("expected 3 rebuild-full model transits, got %d", total)
	}

	// Filter by v2 model
	result2, err := dbinst.RadarObjectRollupRange(start, end, 300, 0, "radar_data_transits", "v2", 0, 0, 0)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	total2 := int64(0)
	for _, r := range result2.Metrics {
		total2 += r.Count
	}
	if total2 != 1 {
		t.Errorf("expected 1 v2 model transit, got %d", total2)
	}
}
