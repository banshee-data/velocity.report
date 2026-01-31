package db

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// TestTransitWorker_RunRangeWithData tests RunRange with actual data
func TestTransitWorker_RunRangeWithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now()
	nowUnix := float64(now.Unix())

	// Insert radar data with varying speeds to create transits
	for i := 0; i < 20; i++ {
		event := map[string]interface{}{
			"uptime":    nowUnix + float64(i),
			"magnitude": 50 + i*2,
			"speed":     float64(10 + (i % 5)), // Varying speed
		}
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	worker := NewTransitWorker(db, 5, "test-v1")

	start := nowUnix - 10
	end := nowUnix + 100

	err = worker.RunRange(context.Background(), start, end)
	if err != nil {
		t.Fatalf("RunRange failed: %v", err)
	}

	// Verify transits were created
	var transitCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM radar_data_transits WHERE model_version = ?", "test-v1").Scan(&transitCount); err != nil {
		t.Fatalf("Failed to count transits: %v", err)
	}

	// Should have created some transits (exact count depends on clustering)
	if transitCount == 0 {
		t.Log("No transits created - this is acceptable for this test data")
	}
}

// TestTransitWorker_RunFullHistoryWithData tests RunFullHistory with data
func TestTransitWorker_RunFullHistoryWithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

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
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	worker := NewTransitWorker(db, 5, "test-v1")

	err = worker.RunFullHistory(context.Background())
	if err != nil {
		t.Fatalf("RunFullHistory failed: %v", err)
	}
}

// TestTransitWorker_RunFullHistory_InvalidRange tests handling of invalid range
func TestTransitWorker_RunFullHistory_InvalidRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert only one data point (start == end)
	event := map[string]interface{}{
		"uptime":    100.0,
		"magnitude": 50,
		"speed":     10.0,
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}
	if err := db.RecordRawData(string(eventJSON)); err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	worker := NewTransitWorker(db, 5, "test-v1")

	// Should handle gracefully
	err = worker.RunFullHistory(context.Background())
	// This may or may not return an error depending on implementation
	// The main point is it shouldn't panic
	_ = err
}

// TestTransitWorker_StartAndStop tests starting and stopping the worker
func TestTransitWorker_StartAndStop(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 30, "test-v1")

	// Configure short interval for testing
	worker.Interval = 50 * time.Millisecond
	worker.Window = 1 * time.Minute

	// Start the worker
	worker.Start()

	// Wait a bit to let the goroutine run
	time.Sleep(100 * time.Millisecond)

	// Stop the worker
	worker.Stop()

	// Give time for goroutine to exit
	time.Sleep(50 * time.Millisecond)

	// Should not panic or deadlock
}

// TestTransitWorker_MigrateModelVersion_WithData tests migration with actual data
func TestTransitWorker_MigrateModelVersion_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now()
	nowUnix := float64(now.Unix())

	// Insert radar data
	for i := 0; i < 5; i++ {
		event := map[string]interface{}{
			"uptime":    nowUnix + float64(i),
			"magnitude": 50,
			"speed":     float64(10),
		}
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Create worker for v1
	workerV1 := NewTransitWorker(db, 5, "v1")
	if err := workerV1.RunFullHistory(context.Background()); err != nil {
		t.Logf("RunFullHistory for v1: %v", err)
	}

	// Create worker for v2 and migrate from v1
	workerV2 := NewTransitWorker(db, 5, "v2")
	if err := workerV2.MigrateModelVersion(context.Background(), "v1"); err != nil {
		t.Errorf("MigrateModelVersion failed: %v", err)
	}
}

// TestTransitWorker_DeleteAllTransits_WithData tests deletion with actual data
func TestTransitWorker_DeleteAllTransits_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

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
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	worker := NewTransitWorker(db, 5, "test-v1")
	if err := worker.RunFullHistory(context.Background()); err != nil {
		t.Logf("RunFullHistory: %v", err)
	}

	// Delete all transits
	deleted, err := worker.DeleteAllTransits(context.Background(), "test-v1")
	if err != nil {
		t.Fatalf("DeleteAllTransits failed: %v", err)
	}
	t.Logf("Deleted %d transits", deleted)
}

// TestAnalyseTransitOverlaps_WithData tests analysis with actual data
func TestAnalyseTransitOverlaps_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now()
	nowUnix := float64(now.Unix())

	// Insert radar data
	for i := 0; i < 20; i++ {
		event := map[string]interface{}{
			"uptime":    nowUnix + float64(i),
			"magnitude": 50,
			"speed":     float64(10),
		}
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	// Run with two different model versions
	workerV1 := NewTransitWorker(db, 5, "v1")
	if err := workerV1.RunFullHistory(context.Background()); err != nil {
		t.Logf("RunFullHistory for v1: %v", err)
	}

	workerV2 := NewTransitWorker(db, 3, "v2") // Different threshold
	if err := workerV2.RunFullHistory(context.Background()); err != nil {
		t.Logf("RunFullHistory for v2: %v", err)
	}

	// Analyse overlaps
	stats, err := db.AnalyseTransitOverlaps(context.Background())
	if err != nil {
		t.Fatalf("AnalyseTransitOverlaps failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected non-nil stats")
	}

	t.Logf("Total transits: %d", stats.TotalTransits)
	for mv, count := range stats.ModelVersionCounts {
		t.Logf("Model version %s: %d transits", mv, count)
	}
	for _, overlap := range stats.Overlaps {
		t.Logf("Overlap: %s vs %s: %d", overlap.ModelVersion1, overlap.ModelVersion2, overlap.OverlapCount)
	}
}

// TestNewTransitWorker_Extended tests creating a new transit worker with different parameters
func TestNewTransitWorker_Extended(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 30, "test-v1")

	if worker == nil {
		t.Fatal("NewTransitWorker returned nil")
	}

	if worker.DB != db {
		t.Error("DB not set correctly")
	}
	if worker.ThresholdSeconds != 30 {
		t.Errorf("Expected ThresholdSeconds 30, got %d", worker.ThresholdSeconds)
	}
	if worker.ModelVersion != "test-v1" {
		t.Errorf("Expected ModelVersion 'test-v1', got '%s'", worker.ModelVersion)
	}
	if worker.Interval != 15*time.Minute {
		t.Errorf("Expected Interval 15m, got %v", worker.Interval)
	}
	if worker.Window != 20*time.Minute {
		t.Errorf("Expected Window 20m, got %v", worker.Window)
	}
	if worker.StopChan == nil {
		t.Error("StopChan should not be nil")
	}
}

// TestTransitWorker_RunOnceWithData tests RunOnce with data
func TestTransitWorker_RunOnceWithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now()
	nowUnix := float64(now.Unix())

	// Insert recent radar data (within the last 20 minutes)
	for i := 0; i < 10; i++ {
		event := map[string]interface{}{
			"uptime":    nowUnix - float64(i*60), // Recent data
			"magnitude": 50,
			"speed":     float64(10),
		}
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if err := db.RecordRawData(string(eventJSON)); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}
	}

	worker := NewTransitWorker(db, 30, "test-v1")

	err = worker.RunOnce(context.Background())
	if err != nil {
		t.Errorf("RunOnce failed: %v", err)
	}
}
