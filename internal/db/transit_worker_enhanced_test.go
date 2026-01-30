package db

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestNewTransitWorker tests worker construction
func TestNewTransitWorker(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 5, "test-model-v1")

	if worker.DB != db {
		t.Error("Expected DB to be set")
	}
	if worker.ThresholdSeconds != 5 {
		t.Errorf("Expected ThresholdSeconds 5, got %d", worker.ThresholdSeconds)
	}
	if worker.ModelVersion != "test-model-v1" {
		t.Errorf("Expected ModelVersion 'test-model-v1', got %s", worker.ModelVersion)
	}
	if worker.Interval != 15*time.Minute {
		t.Errorf("Expected Interval 15m, got %v", worker.Interval)
	}
	if worker.Window != 20*time.Minute {
		t.Errorf("Expected Window 20m, got %v", worker.Window)
	}
	if worker.StopChan == nil {
		t.Error("Expected StopChan to be initialized")
	}
}

// TestTransitWorker_Lifecycle tests worker lifecycle
func TestTransitWorker_Lifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 5, "test-model-v1")
	worker.Interval = 100 * time.Millisecond

	worker.Start()
	time.Sleep(50 * time.Millisecond)
	worker.Stop()

	// Should not panic
}

// TestTransitWorker_RunFullHistory_EmptyDB tests full history run with no data
func TestTransitWorker_RunFullHistory_EmptyDB(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 5, "test-model-v1")
	err := worker.RunFullHistory(context.Background())
	if err != nil {
		t.Fatalf("RunFullHistory failed on empty DB: %v", err)
	}
}

// TestTransitWorker_MigrateModelVersion_SameVersion tests error on same version
func TestTransitWorker_MigrateModelVersion_SameVersion(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 5, "v1")
	err := worker.MigrateModelVersion(context.Background(), "v1")
	if err == nil {
		t.Error("Expected error for same version migration, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "must differ") {
		t.Errorf("Expected 'must differ' error, got: %v", err)
	}
}

// TestTransitWorker_DeleteTransits_NoData tests deleting when no data exists
func TestTransitWorker_DeleteTransits_NoData(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 5, "test-model-v1")
	deleted, err := worker.DeleteAllTransits(context.Background(), "test-model-v1")
	if err != nil {
		t.Fatalf("DeleteAllTransits failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Expected 0 deleted, got %d", deleted)
	}
}

// TestAnalyseTransitOverlaps_Empty tests overlap analysis with no data
func TestAnalyseTransitOverlaps_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	stats, err := db.AnalyseTransitOverlaps(context.Background())
	if err != nil {
		t.Fatalf("AnalyseTransitOverlaps failed: %v", err)
	}

	if stats.TotalTransits != 0 {
		t.Errorf("Expected 0 total transits, got %d", stats.TotalTransits)
	}
	if len(stats.ModelVersionCounts) != 0 {
		t.Errorf("Expected 0 model versions, got %d", len(stats.ModelVersionCounts))
	}
	if len(stats.Overlaps) != 0 {
		t.Errorf("Expected 0 overlaps, got %d", len(stats.Overlaps))
	}
}
