package db

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// TestTransitWorker_EmptyDatabase tests RunOnce with no data
func TestTransitWorker_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 5, "test-v1")

	// RunOnce on empty database should not error
	err = worker.RunOnce(context.Background())
	if err != nil {
		t.Errorf("RunOnce on empty database should not error, got: %v", err)
	}
}

// TestTransitWorker_RunFullHistory_EmptyDatabase tests RunFullHistory with no data
func TestTransitWorker_RunFullHistory_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 5, "test-v1")

	// RunFullHistory on empty database should complete without error
	err = worker.RunFullHistory(context.Background())
	if err != nil {
		t.Errorf("RunFullHistory on empty database should not error, got: %v", err)
	}
}

// TestTransitWorker_RunRange_InvalidRange tests RunRange with invalid time range
func TestTransitWorker_RunRange_InvalidRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "invalid_range.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 5, "test-v1")

	// RunRange with end before start (inverted range)
	start := float64(time.Now().Unix())
	end := start - 1000 // End before start

	err = worker.RunRange(context.Background(), start, end)
	// Should handle gracefully (may return nil or specific error)
	t.Logf("RunRange with inverted range returned: %v", err)
}

// TestTransitWorker_RunRange_ZeroRange tests RunRange with zero-width range
func TestTransitWorker_RunRange_ZeroRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "zero_range.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 5, "test-v1")

	// RunRange with same start and end
	ts := float64(time.Now().Unix())

	err = worker.RunRange(context.Background(), ts, ts)
	// Should complete without error (no data in zero-width range)
	if err != nil {
		t.Errorf("RunRange with zero-width range should not error, got: %v", err)
	}
}

// TestTransitWorker_RunRange_FarFutureRange tests RunRange with future timestamps
func TestTransitWorker_RunRange_FarFutureRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "future_range.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 5, "test-v1")

	// RunRange with far future timestamps (no data expected)
	futureStart := float64(time.Now().Add(365 * 24 * time.Hour).Unix())
	futureEnd := futureStart + 3600

	err = worker.RunRange(context.Background(), futureStart, futureEnd)
	// Should complete without error (no data in future range)
	if err != nil {
		t.Errorf("RunRange with future range should not error, got: %v", err)
	}
}

// TestTransitWorker_MigrateModelVersion_SameVersionEdge tests migration with same version
func TestTransitWorker_MigrateModelVersion_SameVersionEdge(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "same_version.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 5, "test-v1")

	// MigrateModelVersion with same old and new version should error
	err = worker.MigrateModelVersion(context.Background(), "test-v1")
	if err == nil {
		t.Error("MigrateModelVersion with same version should error")
	}
}

// TestTransitWorker_MigrateModelVersion_NoExistingTransits tests migration with no existing transits
func TestTransitWorker_MigrateModelVersion_NoExistingTransits(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "no_transits.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 5, "test-v2")

	// MigrateModelVersion from non-existent version
	err = worker.MigrateModelVersion(context.Background(), "test-v1")
	// Should complete (deletes 0 rows, then runs full history)
	if err != nil {
		t.Errorf("MigrateModelVersion with no existing transits should not error, got: %v", err)
	}
}

// TestTransitWorker_DeleteAllTransits_NonexistentVersion tests deleting non-existent version
func TestTransitWorker_DeleteAllTransits_NonexistentVersion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "delete_nonexistent.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 5, "test-v1")

	// Delete transits for non-existent version
	count, err := worker.DeleteAllTransits(context.Background(), "nonexistent-version")
	if err != nil {
		t.Fatalf("DeleteAllTransits failed: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 deleted rows, got %d", count)
	}
}

// TestTransitWorker_DeleteAllTransits_EmptyVersionString tests deletion with empty version
func TestTransitWorker_DeleteAllTransits_EmptyVersionString(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "delete_empty.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 5, "test-v1")

	// Delete transits for empty version string
	count, err := worker.DeleteAllTransits(context.Background(), "")
	if err != nil {
		t.Fatalf("DeleteAllTransits with empty version failed: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 deleted rows, got %d", count)
	}
}

// TestTransitWorker_ContextCancellation tests context cancellation during RunRange
func TestTransitWorker_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "context_cancel.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 5, "test-v1")

	// Create a pre-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// RunRange should respect the cancelled context
	err = worker.RunRange(ctx, 0, 1000)
	if err != nil && err != context.Canceled {
		t.Logf("RunRange with cancelled context returned: %v", err)
	}
}

// TestTransitWorker_VaryingThresholds tests different threshold values
func TestTransitWorker_VaryingThresholds(t *testing.T) {
	testCases := []struct {
		name      string
		threshold int
	}{
		{"threshold_1", 1},
		{"threshold_5", 5},
		{"threshold_10", 10},
		{"threshold_60", 60},
		{"threshold_0", 0}, // Edge case: zero threshold
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "test.db")

			db, err := NewDB(dbPath)
			if err != nil {
				t.Fatalf("Failed to create database: %v", err)
			}
			defer db.Close()

			worker := NewTransitWorker(db, tc.threshold, "test-v1")

			err = worker.RunOnce(context.Background())
			if err != nil {
				t.Errorf("RunOnce with threshold %d failed: %v", tc.threshold, err)
			}
		})
	}
}

// TestAnalyseTransitOverlaps_EmptyDB_EdgeCase tests overlap analysis on empty DB
func TestAnalyseTransitOverlaps_EmptyDB_EdgeCase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty_overlaps.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	stats, err := db.AnalyseTransitOverlaps(context.Background())
	if err != nil {
		t.Fatalf("AnalyseTransitOverlaps failed: %v", err)
	}

	if stats.TotalTransits != 0 {
		t.Errorf("Expected 0 total transits on empty DB, got %d", stats.TotalTransits)
	}

	if len(stats.ModelVersionCounts) != 0 {
		t.Errorf("Expected empty ModelVersionCounts, got %v", stats.ModelVersionCounts)
	}

	if len(stats.Overlaps) != 0 {
		t.Errorf("Expected no overlaps, got %v", stats.Overlaps)
	}
}

// TestTransitWorker_RunRange_WithDeduplication tests that deduplication works correctly
func TestTransitWorker_RunRange_WithDeduplication(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "dedup.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now()
	nowUnix := float64(now.Unix())

	// Insert radar data
	for i := 0; i < 10; i++ {
		event := map[string]interface{}{
			"uptime":    nowUnix + float64(i),
			"magnitude": 50,
			"speed":     float64(10),
		}
		eventJSON, _ := json.Marshal(event)
		db.RecordRawData(string(eventJSON))
	}

	worker := NewTransitWorker(db, 5, "test-v1")

	// Run twice over the same range - should not create duplicates
	err = worker.RunRange(context.Background(), nowUnix-10, nowUnix+20)
	if err != nil {
		t.Fatalf("First RunRange failed: %v", err)
	}

	var countAfterFirst int
	db.QueryRow("SELECT COUNT(*) FROM radar_data_transits WHERE model_version = ?", "test-v1").Scan(&countAfterFirst)

	err = worker.RunRange(context.Background(), nowUnix-10, nowUnix+20)
	if err != nil {
		t.Fatalf("Second RunRange failed: %v", err)
	}

	var countAfterSecond int
	db.QueryRow("SELECT COUNT(*) FROM radar_data_transits WHERE model_version = ?", "test-v1").Scan(&countAfterSecond)

	// Count should be the same after both runs (deduplication working)
	if countAfterSecond != countAfterFirst {
		t.Errorf("Expected same transit count after deduplication, got %d then %d", countAfterFirst, countAfterSecond)
	}
}

// TestTransitWorker_StartStop_EdgeCase tests the background worker start/stop
func TestTransitWorker_StartStop_EdgeCase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "startstop.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 5, "test-v1")
	worker.Interval = 50 * time.Millisecond // Short interval for testing

	// Start the worker
	worker.Start()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Stop the worker
	worker.Stop()

	// Ensure stop completes without panic
	t.Log("Worker started and stopped successfully")
}

// TestTransitWorker_LargeThreshold tests with very large threshold
func TestTransitWorker_LargeThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "large_threshold.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now()
	nowUnix := float64(now.Unix())

	// Insert some data
	for i := 0; i < 5; i++ {
		event := map[string]interface{}{
			"uptime":    nowUnix + float64(i*10), // 10 second gaps
			"magnitude": 50,
			"speed":     float64(10),
		}
		eventJSON, _ := json.Marshal(event)
		db.RecordRawData(string(eventJSON))
	}

	// Use a very large threshold (3600 seconds = 1 hour)
	worker := NewTransitWorker(db, 3600, "test-v1")

	err = worker.RunRange(context.Background(), nowUnix-100, nowUnix+100)
	if err != nil {
		t.Errorf("RunRange with large threshold failed: %v", err)
	}
}

// TestTransitWorker_NegativeSpeeds tests handling of negative speed values
func TestTransitWorker_NegativeSpeeds(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "negative_speeds.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now()
	nowUnix := float64(now.Unix())

	// Insert data with negative speeds (approaching vehicle)
	for i := 0; i < 10; i++ {
		event := map[string]interface{}{
			"uptime":    nowUnix + float64(i),
			"magnitude": 50,
			"speed":     float64(-15), // Negative speed
		}
		eventJSON, _ := json.Marshal(event)
		db.RecordRawData(string(eventJSON))
	}

	worker := NewTransitWorker(db, 5, "test-v1")

	err = worker.RunRange(context.Background(), nowUnix-10, nowUnix+20)
	if err != nil {
		t.Errorf("RunRange with negative speeds failed: %v", err)
	}
}

// TestTransitWorker_ExtremeSpeedValues tests handling of extreme speed values
func TestTransitWorker_ExtremeSpeedValues(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "extreme_speeds.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now()
	nowUnix := float64(now.Unix())

	extremeSpeeds := []float64{0, 0.001, 999.99, -999.99}

	for i, speed := range extremeSpeeds {
		event := map[string]interface{}{
			"uptime":    nowUnix + float64(i),
			"magnitude": 50,
			"speed":     speed,
		}
		eventJSON, _ := json.Marshal(event)
		db.RecordRawData(string(eventJSON))
	}

	worker := NewTransitWorker(db, 5, "test-v1")

	err = worker.RunRange(context.Background(), nowUnix-10, nowUnix+20)
	if err != nil {
		t.Errorf("RunRange with extreme speeds failed: %v", err)
	}
}
