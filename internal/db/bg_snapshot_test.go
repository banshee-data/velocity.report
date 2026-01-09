package db

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// TestListRecentBgSnapshots tests listing recent background snapshots
func TestListRecentBgSnapshots(t *testing.T) {
	fname := t.TempDir() + "/test_bg_snapshots.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert some background snapshots
	for i := 0; i < 3; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:           "test-sensor",
			TakenUnixNanos:     time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			Rings:              40,
			AzimuthBins:        1800,
			ParamsJSON:         `{}`,
			RingElevationsJSON: `[]`,
			GridBlob:           []byte("test-blob"),
			ChangedCellsCount:  i,
			SnapshotReason:     "test",
		}
		if _, err := db.InsertBgSnapshot(snap); err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
	}

	// List recent snapshots
	snapshots, err := db.ListRecentBgSnapshots("test-sensor", 10)
	if err != nil {
		t.Fatalf("ListRecentBgSnapshots failed: %v", err)
	}

	if len(snapshots) != 3 {
		t.Errorf("Expected 3 snapshots, got %d", len(snapshots))
	}

	// Verify order (most recent first)
	for i := 0; i < len(snapshots)-1; i++ {
		if snapshots[i].SnapshotID != nil && snapshots[i+1].SnapshotID != nil {
			if *snapshots[i].SnapshotID < *snapshots[i+1].SnapshotID {
				t.Error("Snapshots should be ordered by snapshot_id DESC")
			}
		}
	}
}

// TestListRecentBgSnapshots_Limit tests the limit parameter
func TestListRecentBgSnapshots_Limit(t *testing.T) {
	fname := t.TempDir() + "/test_bg_snapshots_limit.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert 5 snapshots
	for i := 0; i < 5; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
			TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			Rings:             40,
			AzimuthBins:       1800,
			GridBlob:          []byte("test-blob"),
			ChangedCellsCount: i,
			SnapshotReason:    "test",
		}
		if _, err := db.InsertBgSnapshot(snap); err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
	}

	// List only 2 recent snapshots
	snapshots, err := db.ListRecentBgSnapshots("test-sensor", 2)
	if err != nil {
		t.Fatalf("ListRecentBgSnapshots failed: %v", err)
	}

	if len(snapshots) != 2 {
		t.Errorf("Expected 2 snapshots, got %d", len(snapshots))
	}
}

// TestListRecentBgSnapshots_EmptyResult tests with no matching snapshots
func TestListRecentBgSnapshots_EmptyResult(t *testing.T) {
	fname := t.TempDir() + "/test_bg_snapshots_empty.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	snapshots, err := db.ListRecentBgSnapshots("non-existent-sensor", 10)
	if err != nil {
		t.Fatalf("ListRecentBgSnapshots failed: %v", err)
	}

	if len(snapshots) != 0 {
		t.Errorf("Expected 0 snapshots, got %d", len(snapshots))
	}
}

// TestGetLatestBgSnapshot tests getting the latest background snapshot
func TestGetLatestBgSnapshot(t *testing.T) {
	fname := t.TempDir() + "/test_latest_bg.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert some snapshots
	for i := 0; i < 3; i++ {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
			TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Minute).UnixNano(),
			Rings:             40,
			AzimuthBins:       1800,
			GridBlob:          []byte("test-blob"),
			ChangedCellsCount: i * 10,
			SnapshotReason:    "test",
		}
		if _, err := db.InsertBgSnapshot(snap); err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
	}

	// Get latest snapshot
	latest, err := db.GetLatestBgSnapshot("test-sensor")
	if err != nil {
		t.Fatalf("GetLatestBgSnapshot failed: %v", err)
	}

	if latest == nil {
		t.Fatal("Expected non-nil snapshot")
	}

	if latest.SensorID != "test-sensor" {
		t.Errorf("Expected sensor_id 'test-sensor', got '%s'", latest.SensorID)
	}

	// The latest should be the one with the highest snapshot_id (last inserted)
	if latest.ChangedCellsCount != 20 {
		t.Errorf("Expected ChangedCellsCount 20, got %d", latest.ChangedCellsCount)
	}
}

// TestGetLatestBgSnapshot_NotFound tests getting snapshot when none exist
func TestGetLatestBgSnapshot_NotFound(t *testing.T) {
	fname := t.TempDir() + "/test_latest_notfound.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	latest, err := db.GetLatestBgSnapshot("non-existent-sensor")
	if err != nil {
		t.Fatalf("GetLatestBgSnapshot failed: %v", err)
	}

	if latest != nil {
		t.Error("Expected nil snapshot for non-existent sensor")
	}
}

// TestCountUniqueBgSnapshotHashes tests counting unique blob hashes
func TestCountUniqueBgSnapshotHashes(t *testing.T) {
	fname := t.TempDir() + "/test_unique_hashes.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert snapshots with same and different blobs
	blobs := [][]byte{
		[]byte("blob-1"),
		[]byte("blob-1"), // duplicate
		[]byte("blob-2"),
		[]byte("blob-1"), // duplicate
		[]byte("blob-3"),
	}

	for i, blob := range blobs {
		snap := &lidar.BgSnapshot{
			SensorID:          "test-sensor",
			TakenUnixNanos:    time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			Rings:             40,
			AzimuthBins:       1800,
			GridBlob:          blob,
			ChangedCellsCount: i,
			SnapshotReason:    "test",
		}
		if _, err := db.InsertBgSnapshot(snap); err != nil {
			t.Fatalf("InsertBgSnapshot failed: %v", err)
		}
	}

	// Count unique hashes
	count, err := db.CountUniqueBgSnapshotHashes("test-sensor")
	if err != nil {
		t.Fatalf("CountUniqueBgSnapshotHashes failed: %v", err)
	}

	// We should have 3 unique hashes
	if count != 3 {
		t.Errorf("Expected 3 unique hashes, got %d", count)
	}
}

// TestCountUniqueBgSnapshotHashes_EmptyResult tests with no snapshots
func TestCountUniqueBgSnapshotHashes_EmptyResult(t *testing.T) {
	fname := t.TempDir() + "/test_unique_hashes_empty.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	count, err := db.CountUniqueBgSnapshotHashes("non-existent-sensor")
	if err != nil {
		t.Fatalf("CountUniqueBgSnapshotHashes failed: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 unique hashes, got %d", count)
	}
}

// TestAttachAdminRoutes tests attaching admin routes
func TestAttachAdminRoutes(t *testing.T) {
	fname := t.TempDir() + "/test_admin_routes.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	db.AttachAdminRoutes(mux)

	// Test /debug/sql endpoint is registered
	req := httptest.NewRequest(http.MethodGet, "/debug/sql", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should be registered (might return 403 due to tailscale auth, but shouldn't be 404)
	if w.Code == http.StatusNotFound {
		t.Error("Expected /debug/sql route to be registered")
	}
}

// TestTransitWorker_StartStop tests the transit worker start and stop
func TestTransitWorker_StartStop(t *testing.T) {
	fname := t.TempDir() + "/test_transit_worker.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 30, "v1")
	if worker == nil {
		t.Fatal("NewTransitWorker returned nil")
	}

	// Start the worker
	worker.Start()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Stop the worker
	worker.Stop()

	// Verify it stopped (should not panic or hang)
}

// TestTransitWorker_DoubleStart tests that double start is handled
func TestTransitWorker_DoubleStart(t *testing.T) {
	fname := t.TempDir() + "/test_transit_double_start.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	worker := NewTransitWorker(db, 30, "v1")

	// Start twice
	worker.Start()
	time.Sleep(20 * time.Millisecond)
	worker.Start() // Should not start a second goroutine

	// Stop
	worker.Stop()
}

// TestRadarObjectsRollupRow_String tests the String method
func TestRadarObjectsRollupRow_String(t *testing.T) {
	row := RadarObjectsRollupRow{
		Classifier: "all",
		StartTime:  time.Unix(1700000000, 0),
		Count:      42,
		P50Speed:   10.5,
		P85Speed:   15.2,
		P98Speed:   20.1,
		MaxSpeed:   25.0,
	}

	s := row.String()
	if s == "" {
		t.Error("String() returned empty string")
	}
	// Should contain formatted speed values
	if len(s) == 0 {
		t.Error("Expected non-empty string representation")
	}
}
