package db

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestTransitCLI_NewTransitCLI(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	var buf bytes.Buffer
	cli := NewTransitCLI(db, "test-model", 1, &buf)

	if cli.DB != db {
		t.Error("expected DB to be set")
	}
	if cli.ModelVersion != "test-model" {
		t.Errorf("expected ModelVersion 'test-model', got %q", cli.ModelVersion)
	}
	if cli.Threshold != 1 {
		t.Errorf("expected Threshold 1, got %d", cli.Threshold)
	}
	if cli.Output != &buf {
		t.Error("expected Output to be set")
	}
}

func TestTransitCLI_Analyse_EmptyDB(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	var buf bytes.Buffer
	cli := NewTransitCLI(db, "test-model", 1, &buf)

	stats, err := cli.Analyse(context.Background())
	if err != nil {
		t.Fatalf("Analyse failed: %v", err)
	}

	if stats.TotalTransits != 0 {
		t.Errorf("expected 0 total transits, got %d", stats.TotalTransits)
	}

	output := buf.String()
	if !strings.Contains(output, "Transit Statistics") {
		t.Error("expected output to contain 'Transit Statistics'")
	}
	if !strings.Contains(output, "Total transits: 0") {
		t.Error("expected output to contain 'Total transits: 0'")
	}
	if !strings.Contains(output, "No overlapping transits found") {
		t.Error("expected output to indicate no overlaps")
	}
}

func TestTransitCLI_Analyse_WithData(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// Insert radar_data to enable transit creation
	for i := 0; i < 5; i++ {
		insertRadarData(t, db, float64(now.Unix())+float64(i)*0.5, 10.0, 100.0)
	}

	// Create a transit worker and run it
	worker := NewTransitWorker(db, 1, "test-model-v1")
	if err := worker.RunFullHistory(ctx); err != nil {
		t.Fatalf("failed to run transit worker: %v", err)
	}

	var buf bytes.Buffer
	cli := NewTransitCLI(db, "test-model-v1", 1, &buf)

	stats, err := cli.Analyse(ctx)
	if err != nil {
		t.Fatalf("Analyse failed: %v", err)
	}

	if stats.TotalTransits == 0 {
		t.Error("expected at least 1 transit after running worker")
	}

	output := buf.String()
	if !strings.Contains(output, "By model version:") {
		t.Error("expected output to contain model version breakdown")
	}
}

func TestTransitCLI_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// Insert radar_data
	for i := 0; i < 3; i++ {
		insertRadarData(t, db, float64(now.Unix())+float64(i)*0.5, 10.0, 100.0)
	}

	// Create transits
	worker := NewTransitWorker(db, 1, "delete-test-model")
	if err := worker.RunFullHistory(ctx); err != nil {
		t.Fatalf("failed to run transit worker: %v", err)
	}

	var buf bytes.Buffer
	cli := NewTransitCLI(db, "delete-test-model", 1, &buf)

	deleted, err := cli.Delete(ctx, "delete-test-model")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if deleted == 0 {
		t.Error("expected at least 1 transit to be deleted")
	}

	output := buf.String()
	if !strings.Contains(output, "Deleted") {
		t.Error("expected output to contain 'Deleted'")
	}
	if !strings.Contains(output, "delete-test-model") {
		t.Error("expected output to contain model version")
	}
}

func TestTransitCLI_Delete_NonExistent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	var buf bytes.Buffer
	cli := NewTransitCLI(db, "test-model", 1, &buf)

	deleted, err := cli.Delete(context.Background(), "nonexistent-model")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("expected 0 deletions for nonexistent model, got %d", deleted)
	}

	output := buf.String()
	if !strings.Contains(output, "Deleted 0") {
		t.Error("expected output to indicate 0 deletions")
	}
}

func TestTransitCLI_Migrate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// Insert radar_data
	for i := 0; i < 3; i++ {
		insertRadarData(t, db, float64(now.Unix())+float64(i)*0.5, 10.0, 100.0)
	}

	// Create transits with old model
	worker := NewTransitWorker(db, 1, "old-model")
	if err := worker.RunFullHistory(ctx); err != nil {
		t.Fatalf("failed to run transit worker: %v", err)
	}

	var buf bytes.Buffer
	cli := NewTransitCLI(db, "new-model", 1, &buf)

	err := cli.Migrate(ctx, "old-model", "new-model")
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Migrating transits from") {
		t.Error("expected output to contain migration message")
	}
	if !strings.Contains(output, "Migration complete") {
		t.Error("expected output to contain completion message")
	}

	// Verify old model transits are gone
	var oldCount int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM radar_data_transits WHERE model_version = ?`, "old-model").Scan(&oldCount)
	if oldCount != 0 {
		t.Errorf("expected 0 old-model transits after migration, got %d", oldCount)
	}
}

func TestTransitCLI_Rebuild(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// Insert radar_data
	for i := 0; i < 5; i++ {
		insertRadarData(t, db, float64(now.Unix())+float64(i)*0.5, 10.0, 100.0)
	}

	// First create some transits
	worker := NewTransitWorker(db, 1, "rebuild-model")
	if err := worker.RunFullHistory(ctx); err != nil {
		t.Fatalf("failed to run transit worker: %v", err)
	}

	var buf bytes.Buffer
	cli := NewTransitCLI(db, "rebuild-model", 1, &buf)

	err := cli.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Rebuilding transits") {
		t.Error("expected output to contain rebuild message")
	}
	if !strings.Contains(output, "Deleted") {
		t.Error("expected output to contain deletion count")
	}
	if !strings.Contains(output, "Rebuild complete") {
		t.Error("expected output to contain completion message")
	}
}

func TestTransitCLI_Rebuild_EmptyDB(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	var buf bytes.Buffer
	cli := NewTransitCLI(db, "empty-model", 1, &buf)

	err := cli.Rebuild(context.Background())
	if err != nil {
		t.Fatalf("Rebuild failed on empty DB: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Deleted 0") {
		t.Error("expected output to indicate 0 deletions")
	}
}

func TestTransitCLI_PrintUsage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	var buf bytes.Buffer
	cli := NewTransitCLI(db, "test-model", 1, &buf)

	cli.PrintUsage()

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Error("expected output to contain 'Usage:'")
	}
	if !strings.Contains(output, "analyse") {
		t.Error("expected output to contain 'analyse' command")
	}
	if !strings.Contains(output, "delete") {
		t.Error("expected output to contain 'delete' command")
	}
	if !strings.Contains(output, "migrate") {
		t.Error("expected output to contain 'migrate' command")
	}
	if !strings.Contains(output, "rebuild") {
		t.Error("expected output to contain 'rebuild' command")
	}
}

func TestTransitCLI_Analyse_WithOverlaps(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// Insert radar_data covering a time range
	for i := 0; i < 10; i++ {
		insertRadarData(t, db, float64(now.Unix())+float64(i)*0.5, 10.0, 100.0)
	}

	// Create transits with two different model versions covering same time range
	worker1 := NewTransitWorker(db, 1, "model-v1")
	if err := worker1.RunFullHistory(ctx); err != nil {
		t.Fatalf("failed to run transit worker v1: %v", err)
	}

	worker2 := NewTransitWorker(db, 1, "model-v2")
	if err := worker2.RunFullHistory(ctx); err != nil {
		t.Fatalf("failed to run transit worker v2: %v", err)
	}

	var buf bytes.Buffer
	cli := NewTransitCLI(db, "model-v1", 1, &buf)

	stats, err := cli.Analyse(ctx)
	if err != nil {
		t.Fatalf("Analyse failed: %v", err)
	}

	// Both models should have created transits
	if stats.TotalTransits < 2 {
		t.Errorf("expected at least 2 transits from two models, got %d", stats.TotalTransits)
	}

	output := buf.String()
	// Check that we detect overlaps (both models cover same time range)
	if len(stats.Overlaps) > 0 && !strings.Contains(output, "Overlapping transits detected") {
		t.Error("expected output to warn about overlaps when present")
	}
}
