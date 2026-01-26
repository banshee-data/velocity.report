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
	result, err := dbinst.RadarObjectRollupRange(start, end, 300, 2.0, "radar_objects", "", 0, 0, 0, 0)
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
	result, err := dbinst.RadarObjectRollupRange(now-1000, now, 300, 0, "radar_objects", "", 0, 0, 0, 0)
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
	result, err := dbinst.RadarObjectRollupRange(start, end, 300, 0, "radar_objects", "", bucketSize, histMax, 0, 0)
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
	result, err := dbinst.RadarObjectRollupRange(start, end, 300, 0, "radar_data_transits", "rebuild-full", 0, 0, 0, 0)
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
			result, err := dbinst.RadarObjectRollupRange(start, end, tc.groupByNSec, 0, "radar_objects", "", 0, 0, 0, 0)
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
	_, err = dbinst.RadarObjectRollupRange(now-1000, now, 300, 0, "invalid_table", "", 0, 0, 0, 0)
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
	result, err := dbinst.RadarObjectRollupRange(start, end, 300, 0, "radar_data_transits", "rebuild-full", 0, 0, 0, 0)
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
	result2, err := dbinst.RadarObjectRollupRange(start, end, 300, 0, "radar_data_transits", "v2", 0, 0, 0, 0)
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

// TestRadarObjectRollupRange_BoundaryThreshold tests the boundary hour filtering feature
func TestRadarObjectRollupRange_BoundaryThreshold(t *testing.T) {
	fname := t.TempDir() + "/boundary_rollup.db"
	dbinst, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer dbinst.Close()

	// Create data across 3 days with varying counts per boundary hour:
	// Day 1: first hour has 2 events (below threshold of 5), last hour has 10 events
	// Day 2: first hour has 10 events, last hour has 10 events
	// Day 3: first hour has 10 events, last hour has 3 events (below threshold of 5)

	baseTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	testData := []struct {
		dayOffset   int
		hourOffset  int
		eventCount  int
		description string
	}{
		// Day 1: Jun 1
		{0, 8, 2, "Day1 first hour - below threshold"},  // 08:00 - boundary, low count
		{0, 12, 15, "Day1 midday - not boundary"},       // 12:00 - not boundary
		{0, 17, 10, "Day1 last hour - above threshold"}, // 17:00 - boundary, high count
		// Day 2: Jun 2
		{1, 7, 10, "Day2 first hour - above threshold"}, // 07:00 - boundary, high count
		{1, 12, 20, "Day2 midday - not boundary"},       // 12:00 - not boundary
		{1, 18, 10, "Day2 last hour - above threshold"}, // 18:00 - boundary, high count
		// Day 3: Jun 3
		{2, 9, 10, "Day3 first hour - above threshold"}, // 09:00 - boundary, high count
		{2, 14, 25, "Day3 midday - not boundary"},       // 14:00 - not boundary
		{2, 16, 3, "Day3 last hour - below threshold"},  // 16:00 - boundary, low count
	}

	for _, td := range testData {
		hourTime := baseTime.AddDate(0, 0, td.dayOffset).Add(time.Duration(td.hourOffset) * time.Hour)
		for i := 0; i < td.eventCount; i++ {
			ts := hourTime.Add(time.Duration(i) * time.Minute).Unix()
			raw := fmt.Sprintf(`{"start_time": %d, "end_time": %d, "delta_time_msec": 100, "max_speed_mps": 10.0, "min_speed_mps": 5, "speed_change": 0, "max_magnitude": 50, "avg_magnitude": 40, "total_frames": 10, "frames_per_mps": 1, "length_m": 5, "classifier": "all"}`, ts, ts+1)
			if _, err := dbinst.Exec(`INSERT INTO radar_objects (raw_event, write_timestamp) VALUES (?, ?)`, raw, float64(ts)); err != nil {
				t.Fatalf("insert failed for %s: %v", td.description, err)
			}
		}
	}

	startUnix := baseTime.Unix()
	endUnix := baseTime.AddDate(0, 0, 3).Unix()

	t.Run("without boundary filtering", func(t *testing.T) {
		// With boundaryThreshold=0, no filtering should occur
		result, err := dbinst.RadarObjectRollupRange(startUnix, endUnix, 3600, 0, "radar_objects", "", 0, 0, 0, 0)
		if err != nil {
			t.Fatalf("RadarObjectRollupRange failed: %v", err)
		}

		// Should include all hours: 2 + 15 + 10 + 10 + 20 + 10 + 10 + 25 + 3 = 105
		totalCount := int64(0)
		for _, m := range result.Metrics {
			totalCount += m.Count
		}
		if totalCount != 105 {
			t.Errorf("expected 105 total events without filtering, got %d", totalCount)
		}
	})

	t.Run("with boundary filtering threshold=5", func(t *testing.T) {
		// With boundaryThreshold=5, filter out first/last hours with < 5 events
		result, err := dbinst.RadarObjectRollupRange(startUnix, endUnix, 3600, 0, "radar_objects", "", 0, 0, 0, 5)
		if err != nil {
			t.Fatalf("RadarObjectRollupRange failed: %v", err)
		}

		// Should exclude: Day1 08:00 (2 events), Day3 16:00 (3 events)
		// Expected: 15 + 10 + 10 + 20 + 10 + 10 + 25 = 100
		totalCount := int64(0)
		for _, m := range result.Metrics {
			totalCount += m.Count
		}
		if totalCount != 100 {
			t.Errorf("expected 100 total events with threshold=5, got %d", totalCount)
		}
	})

	t.Run("with higher boundary filtering threshold=15", func(t *testing.T) {
		// With boundaryThreshold=15, filter out boundary hours with < 15 events
		result, err := dbinst.RadarObjectRollupRange(startUnix, endUnix, 3600, 0, "radar_objects", "", 0, 0, 0, 15)
		if err != nil {
			t.Fatalf("RadarObjectRollupRange failed: %v", err)
		}

		// Should exclude: Day1 08:00 (2), Day1 17:00 (10), Day2 07:00 (10), Day2 18:00 (10), Day3 09:00 (10), Day3 16:00 (3)
		// Expected: 15 + 20 + 25 = 60
		totalCount := int64(0)
		for _, m := range result.Metrics {
			totalCount += m.Count
		}
		if totalCount != 60 {
			t.Errorf("expected 60 total events with threshold=15, got %d", totalCount)
		}
	})
}

// TestRadarObjectRollupRange_BoundaryThreshold_SingleDay tests that boundary filtering
// does not apply to single-day data
func TestRadarObjectRollupRange_BoundaryThreshold_SingleDay(t *testing.T) {
	fname := t.TempDir() + "/boundary_single_day.db"
	dbinst, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer dbinst.Close()

	// Create data for a single day with low count boundary hours
	baseTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	// First hour: 2 events (would be filtered if multi-day)
	// Last hour: 3 events (would be filtered if multi-day)
	testData := []struct {
		hourOffset int
		eventCount int
	}{
		{8, 2},   // First hour - low count
		{12, 15}, // Midday
		{17, 3},  // Last hour - low count
	}

	for _, td := range testData {
		hourTime := baseTime.Add(time.Duration(td.hourOffset) * time.Hour)
		for i := 0; i < td.eventCount; i++ {
			ts := hourTime.Add(time.Duration(i) * time.Minute).Unix()
			raw := fmt.Sprintf(`{"start_time": %d, "end_time": %d, "delta_time_msec": 100, "max_speed_mps": 10.0, "min_speed_mps": 5, "speed_change": 0, "max_magnitude": 50, "avg_magnitude": 40, "total_frames": 10, "frames_per_mps": 1, "length_m": 5, "classifier": "all"}`, ts, ts+1)
			if _, err := dbinst.Exec(`INSERT INTO radar_objects (raw_event, write_timestamp) VALUES (?, ?)`, raw, float64(ts)); err != nil {
				t.Fatalf("insert failed: %v", err)
			}
		}
	}

	startUnix := baseTime.Unix()
	endUnix := baseTime.Add(24 * time.Hour).Unix()

	// Even with threshold=5, single-day data should NOT be filtered
	result, err := dbinst.RadarObjectRollupRange(startUnix, endUnix, 3600, 0, "radar_objects", "", 0, 0, 0, 5)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	// All events should be included: 2 + 15 + 3 = 20
	totalCount := int64(0)
	for _, m := range result.Metrics {
		totalCount += m.Count
	}
	if totalCount != 20 {
		t.Errorf("expected 20 total events (no filtering for single day), got %d", totalCount)
	}
}

// TestRadarObjectRollupRange_HistogramWithMaxBucket tests that when histMax is set,
// all speeds >= histMax are aggregated into a single bucket at histMax
func TestRadarObjectRollupRange_HistogramWithMaxBucket(t *testing.T) {
	fname := t.TempDir() + "/hist_max_bucket.db"
	dbinst, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create DB: %v", err)
	}
	defer dbinst.Close()

	now := time.Now().Unix()
	// Insert speeds: 10, 20, 30, 40, 50, 60, 70, 80, 90, 100 mph
	// With bucket size 5 mph and max 75 mph, we expect:
	// - Regular buckets up to 70
	// - All speeds >= 75 aggregated into bucket 75
	speeds := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	for i, spd := range speeds {
		ts := now + int64(i*10)
		raw := fmt.Sprintf(`{"start_time": %d, "end_time": %d, "delta_time_msec": 100, "max_speed_mps": %f, "min_speed_mps": 5, "speed_change": 0, "max_magnitude": 50, "avg_magnitude": 40, "total_frames": 10, "frames_per_mps": 1, "length_m": 5, "classifier": "all"}`, ts, ts+1, spd)
		if _, err := dbinst.Exec(`INSERT INTO radar_objects (raw_event, write_timestamp) VALUES (?, ?)`, raw, float64(ts)); err != nil {
			t.Fatalf("insert failed: %v", err)
		}
	}

	start := now - 10
	end := now + 1000
	bucketSize := 5.0
	histMax := 75.0

	result, err := dbinst.RadarObjectRollupRange(start, end, 300, 0, "radar_objects", "", bucketSize, histMax, 0, 0)
	if err != nil {
		t.Fatalf("RadarObjectRollupRange failed: %v", err)
	}

	// Verify histogram contains expected buckets
	if result.Histogram == nil {
		t.Fatal("expected histogram to be non-nil")
	}

	// Check that bucket 75 exists and contains all speeds >= 75 (80, 90, 100 = 3 values)
	bucket75Count, ok := result.Histogram[75.0]
	if !ok {
		t.Error("expected histogram to contain bucket 75")
	}
	if bucket75Count != 3 {
		t.Errorf("expected bucket 75 to have count 3 (speeds 80, 90, 100), got %d", bucket75Count)
	}

	// Verify regular buckets below 75 exist
	expectedBuckets := map[float64]int64{
		10.0: 1,
		20.0: 1,
		30.0: 1,
		40.0: 1,
		50.0: 1,
		60.0: 1,
		70.0: 1,
		75.0: 3, // 80, 90, 100 aggregated here
	}

	for bucket, expectedCount := range expectedBuckets {
		count, ok := result.Histogram[bucket]
		if !ok {
			t.Errorf("expected histogram to contain bucket %v", bucket)
			continue
		}
		if count != expectedCount {
			t.Errorf("bucket %v: expected count %d, got %d", bucket, expectedCount, count)
		}
	}

	// Verify total count in histogram equals total events
	totalHistCount := int64(0)
	for _, count := range result.Histogram {
		totalHistCount += count
	}
	if totalHistCount != 10 {
		t.Errorf("expected total histogram count 10, got %d", totalHistCount)
	}
}
