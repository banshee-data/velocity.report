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
	result, err := dbinst.RadarObjectRollupRange(start, end, 300, 2.0, "radar_objects", "", 0, 0)
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

// ...existing code...
