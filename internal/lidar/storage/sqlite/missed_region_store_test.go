package sqlite

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func insertAnalysisRunForMissedRegions(t *testing.T, db *sql.DB, runID string) {
	t.Helper()
	store := NewAnalysisRunStore(db)
	err := store.InsertRun(&AnalysisRun{
		RunID:      runID,
		CreatedAt:  time.Now(),
		SourceType: "pcap",
		SourcePath: "test.pcap",
		SensorID:   "sensor-test",
		ParamsJSON: []byte(`{"version":"1.0"}`),
		Status:     "completed",
	})
	if err != nil {
		t.Fatalf("insert analysis run: %v", err)
	}
}

func TestMissedRegionStore_InsertListDelete(t *testing.T) {
	sqlDB, cleanup := setupTrackingPipelineTestDB(t)
	defer cleanup()

	runID := "run-missed-region-1"
	insertAnalysisRunForMissedRegions(t, sqlDB, runID)

	store := NewMissedRegionStore(sqlDB)
	if store == nil {
		t.Fatal("NewMissedRegionStore returned nil")
	}

	region := &MissedRegion{
		RunID:       runID,
		CenterX:     12.5,
		CenterY:     -3.2,
		TimeStartNs: 1000,
		TimeEndNs:   2000,
		Notes:       "first region",
	}

	if err := store.Insert(region); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Defaults should be populated.
	if region.RegionID == "" {
		t.Fatal("expected RegionID to be auto-generated")
	}
	if region.LabeledAt == nil || *region.LabeledAt == 0 {
		t.Fatal("expected LabeledAt default to be set")
	}
	if region.RadiusM != 3.0 {
		t.Fatalf("expected default radius 3.0, got %f", region.RadiusM)
	}
	if region.ExpectedLabel != "car" {
		t.Fatalf("expected default label car, got %q", region.ExpectedLabel)
	}

	regions, err := store.ListByRun(runID)
	if err != nil {
		t.Fatalf("ListByRun failed: %v", err)
	}
	if len(regions) != 1 {
		t.Fatalf("expected 1 region, got %d", len(regions))
	}
	if regions[0].RunID != runID {
		t.Fatalf("expected run_id %q, got %q", runID, regions[0].RunID)
	}
	if regions[0].LabelerID != "" {
		t.Fatalf("expected empty labeler_id, got %q", regions[0].LabelerID)
	}

	if err := store.Delete(region.RegionID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	regions, err = store.ListByRun(runID)
	if err != nil {
		t.Fatalf("ListByRun after delete failed: %v", err)
	}
	if len(regions) != 0 {
		t.Fatalf("expected 0 regions after delete, got %d", len(regions))
	}

	// Deleting again should report sql.ErrNoRows.
	if err := store.Delete(region.RegionID); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestMissedRegionStore_ErrorPaths(t *testing.T) {
	dbNoTable, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer dbNoTable.Close()

	store := NewMissedRegionStore(dbNoTable)

	err = store.Insert(&MissedRegion{
		RunID:       "missing-table",
		CenterX:     1,
		CenterY:     2,
		TimeStartNs: 1,
		TimeEndNs:   2,
	})
	if err == nil || !strings.Contains(err.Error(), "insert missed region") {
		t.Fatalf("expected wrapped insert error, got %v", err)
	}

	_, err = store.ListByRun("missing-table")
	if err == nil || !strings.Contains(err.Error(), "list missed regions") {
		t.Fatalf("expected wrapped list error, got %v", err)
	}

	err = store.Delete("missing-id")
	if err == nil || !strings.Contains(err.Error(), "delete missed region") {
		t.Fatalf("expected wrapped delete error, got %v", err)
	}
}

func TestMissedRegionStore_ListByRun_ScanError(t *testing.T) {
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer sqlDB.Close()

	// Create a minimal table with a TEXT labeled_at column so scan into sql.NullInt64 fails.
	_, err = sqlDB.Exec(`
		CREATE TABLE lidar_missed_regions (
			region_id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			center_x REAL NOT NULL,
			center_y REAL NOT NULL,
			radius_m REAL NOT NULL,
			time_start_ns INTEGER NOT NULL,
			time_end_ns INTEGER NOT NULL,
			expected_label TEXT NOT NULL,
			labeler_id TEXT,
			labeled_at TEXT,
			notes TEXT
		)
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	_, err = sqlDB.Exec(`
		INSERT INTO lidar_missed_regions (
			region_id, run_id, center_x, center_y, radius_m,
			time_start_ns, time_end_ns, expected_label, labeler_id, labeled_at, notes
		) VALUES (
			"r1", "run-1", 1.0, 2.0, 3.0,
			100, 200, "car", "labeler-1", "not-an-int", "bad row"
		)
	`)
	if err != nil {
		t.Fatalf("insert row: %v", err)
	}

	store := NewMissedRegionStore(sqlDB)
	_, err = store.ListByRun("run-1")
	if err == nil || !strings.Contains(err.Error(), "scan missed region") {
		t.Fatalf("expected scan missed region error, got %v", err)
	}
}
