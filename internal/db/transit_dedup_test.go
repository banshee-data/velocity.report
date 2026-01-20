package db

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestTransitWorker_Deduplication verifies that overlapping window runs
// don't create duplicate transits.
func TestTransitWorker_Deduplication(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()

	// Insert sample radar_data spanning 2 hours
	// radar_data.speed is a generated column from raw_event JSON
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC).Unix()
	for i := 0; i < 120; i++ { // 120 minutes of data
		ts := float64(baseTime) + float64(i*60) // one point per minute
		speed := 10.0 + float64(i%5)            // vary speed slightly
		rawEvent := fmt.Sprintf(`{"speed": %f, "magnitude": 100, "uptime": %d}`, speed, i)
		_, err := db.Exec(`INSERT INTO radar_data (write_timestamp, raw_event) VALUES (?, ?)`,
			ts, rawEvent)
		if err != nil {
			t.Fatalf("Failed to insert radar_data: %v", err)
		}
	}

	worker := NewTransitWorker(db, 1, "test-dedup")
	worker.Interval = time.Hour
	worker.Window = 65 * time.Minute // 5 minute overlap

	// Run worker for first hour (simulating first hourly run)
	start1 := float64(baseTime)
	end1 := float64(baseTime + 3600) // +1 hour
	if err := worker.RunRange(ctx, start1, end1); err != nil {
		t.Fatalf("First run failed: %v", err)
	}

	// Count transits after first run
	var count1 int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_data_transits WHERE model_version = 'test-dedup'`).Scan(&count1); err != nil {
		t.Fatalf("Failed to count transits: %v", err)
	}
	if count1 == 0 {
		t.Fatal("Expected at least one transit after first run")
	}
	t.Logf("Transits after first run: %d", count1)

	// Run worker for overlapping window (simulating second hourly run with overlap)
	start2 := float64(baseTime + 3300) // 55 minutes into first hour (5 min overlap)
	end2 := float64(baseTime + 7200)   // +2 hours
	if err := worker.RunRange(ctx, start2, end2); err != nil {
		t.Fatalf("Second run failed: %v", err)
	}

	// Count transits after second run
	var count2 int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_data_transits WHERE model_version = 'test-dedup'`).Scan(&count2); err != nil {
		t.Fatalf("Failed to count transits: %v", err)
	}
	t.Logf("Transits after second run: %d", count2)

	// The second run should have replaced overlapping transits, not added to them
	// We expect roughly the same or fewer transits, not double
	if count2 > count1*2 {
		t.Errorf("Possible duplicate transits: first run=%d, second run=%d (expected similar counts)", count1, count2)
	}

	// Verify no duplicate transit_keys exist
	var dupeCount int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM (SELECT transit_key, COUNT(*) as cnt FROM radar_data_transits GROUP BY transit_key HAVING cnt > 1)`).Scan(&dupeCount); err != nil {
		t.Fatalf("Failed to check for duplicates: %v", err)
	}
	if dupeCount > 0 {
		t.Errorf("Found %d duplicate transit_keys", dupeCount)
	}
}

// TestTransitWorker_DeleteBeforeInsert verifies that overlapping transits
// are deleted before new ones are inserted.
func TestTransitWorker_DeleteBeforeInsert(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()

	// Insert some radar_data (speed is a generated column from raw_event JSON)
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC).Unix()
	for i := 0; i < 10; i++ {
		ts := float64(baseTime) + float64(i*60)
		rawEvent := fmt.Sprintf(`{"speed": 15.0, "magnitude": 100, "uptime": %d}`, i)
		_, err := db.Exec(`INSERT INTO radar_data (write_timestamp, raw_event) VALUES (?, ?)`,
			ts, rawEvent)
		if err != nil {
			t.Fatalf("Failed to insert radar_data: %v", err)
		}
	}

	worker := NewTransitWorker(db, 1, "test-delete")

	// Run worker twice on the same range
	start := float64(baseTime)
	end := float64(baseTime + 600) // 10 minutes

	if err := worker.RunRange(ctx, start, end); err != nil {
		t.Fatalf("First run failed: %v", err)
	}

	var count1 int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_data_transits WHERE model_version = 'test-delete'`).Scan(&count1); err != nil {
		t.Fatalf("Failed to count transits: %v", err)
	}

	if err := worker.RunRange(ctx, start, end); err != nil {
		t.Fatalf("Second run failed: %v", err)
	}

	var count2 int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_data_transits WHERE model_version = 'test-delete'`).Scan(&count2); err != nil {
		t.Fatalf("Failed to count transits: %v", err)
	}

	// Running twice on the same range should produce the same count
	if count1 != count2 {
		t.Errorf("Expected same transit count after re-run: first=%d, second=%d", count1, count2)
	}
}

// TestTransitWorker_DifferentModelVersions verifies that different model versions
// don't interfere with each other (until migrated).
func TestTransitWorker_DifferentModelVersions(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()

	// Insert some radar_data (speed is a generated column from raw_event JSON)
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC).Unix()
	for i := 0; i < 10; i++ {
		ts := float64(baseTime) + float64(i*60)
		rawEvent := fmt.Sprintf(`{"speed": 15.0, "magnitude": 100, "uptime": %d}`, i)
		_, err := db.Exec(`INSERT INTO radar_data (write_timestamp, raw_event) VALUES (?, ?)`,
			ts, rawEvent)
		if err != nil {
			t.Fatalf("Failed to insert radar_data: %v", err)
		}
	}

	start := float64(baseTime)
	end := float64(baseTime + 600)

	// Run with first model version
	worker1 := NewTransitWorker(db, 1, "model-v1")
	if err := worker1.RunRange(ctx, start, end); err != nil {
		t.Fatalf("First model run failed: %v", err)
	}

	var countV1 int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_data_transits WHERE model_version = 'model-v1'`).Scan(&countV1); err != nil {
		t.Fatalf("Failed to count v1 transits: %v", err)
	}

	// Run with second model version on same range
	worker2 := NewTransitWorker(db, 1, "model-v2")
	if err := worker2.RunRange(ctx, start, end); err != nil {
		t.Fatalf("Second model run failed: %v", err)
	}

	var countV2 int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_data_transits WHERE model_version = 'model-v2'`).Scan(&countV2); err != nil {
		t.Fatalf("Failed to count v2 transits: %v", err)
	}

	// Both model versions should have transits (they're independent)
	if countV1 == 0 || countV2 == 0 {
		t.Errorf("Both model versions should have transits: v1=%d, v2=%d", countV1, countV2)
	}

	// Check for overlaps using AnalyseTransitOverlaps
	stats, err := db.AnalyseTransitOverlaps(ctx)
	if err != nil {
		t.Fatalf("Failed to analyse overlaps: %v", err)
	}

	if len(stats.Overlaps) == 0 {
		t.Error("Expected overlaps between model-v1 and model-v2")
	}

	// Now migrate from v1 to v2
	if err := worker2.MigrateModelVersion(ctx, "model-v1"); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// After migration, v1 should be empty
	var countV1After int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_data_transits WHERE model_version = 'model-v1'`).Scan(&countV1After); err != nil {
		t.Fatalf("Failed to count v1 transits after migration: %v", err)
	}
	if countV1After != 0 {
		t.Errorf("Expected 0 v1 transits after migration, got %d", countV1After)
	}

	// v2 should have transits (from the full rebuild)
	var countV2After int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_data_transits WHERE model_version = 'model-v2'`).Scan(&countV2After); err != nil {
		t.Fatalf("Failed to count v2 transits after migration: %v", err)
	}
	if countV2After == 0 {
		t.Error("Expected v2 transits after migration")
	}
}

// TestTransitWorker_DeleteAllTransits verifies the DeleteAllTransits function.
func TestTransitWorker_DeleteAllTransits(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()

	// Insert some radar_data (speed is a generated column from raw_event JSON)
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC).Unix()
	for i := 0; i < 10; i++ {
		ts := float64(baseTime) + float64(i*60)
		rawEvent := fmt.Sprintf(`{"speed": 15.0, "magnitude": 100, "uptime": %d}`, i)
		_, err := db.Exec(`INSERT INTO radar_data (write_timestamp, raw_event) VALUES (?, ?)`,
			ts, rawEvent)
		if err != nil {
			t.Fatalf("Failed to insert radar_data: %v", err)
		}
	}

	start := float64(baseTime)
	end := float64(baseTime + 600)

	worker := NewTransitWorker(db, 1, "test-delete-all")
	if err := worker.RunRange(ctx, start, end); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var countBefore int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_data_transits WHERE model_version = 'test-delete-all'`).Scan(&countBefore); err != nil {
		t.Fatalf("Failed to count transits: %v", err)
	}
	if countBefore == 0 {
		t.Fatal("Expected transits before delete")
	}

	deleted, err := worker.DeleteAllTransits(ctx, "test-delete-all")
	if err != nil {
		t.Fatalf("DeleteAllTransits failed: %v", err)
	}
	if deleted != countBefore {
		t.Errorf("Expected to delete %d transits, deleted %d", countBefore, deleted)
	}

	var countAfter int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM radar_data_transits WHERE model_version = 'test-delete-all'`).Scan(&countAfter); err != nil {
		t.Fatalf("Failed to count transits after delete: %v", err)
	}
	if countAfter != 0 {
		t.Errorf("Expected 0 transits after delete, got %d", countAfter)
	}
}

// TestAnalyseTransitOverlaps verifies the overlap analysis function.
func TestAnalyseTransitOverlaps(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()

	// Test with empty database
	stats, err := db.AnalyseTransitOverlaps(ctx)
	if err != nil {
		t.Fatalf("AnalyseTransitOverlaps failed on empty db: %v", err)
	}
	if stats.TotalTransits != 0 {
		t.Errorf("Expected 0 transits on empty db, got %d", stats.TotalTransits)
	}
	if len(stats.Overlaps) != 0 {
		t.Errorf("Expected no overlaps on empty db, got %d", len(stats.Overlaps))
	}
}
