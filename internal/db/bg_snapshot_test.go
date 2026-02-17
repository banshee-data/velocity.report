package db

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
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
		snap := &l3grid.BgSnapshot{
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
		snap := &l3grid.BgSnapshot{
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
		snap := &l3grid.BgSnapshot{
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
		snap := &l3grid.BgSnapshot{
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

// ============================================================================
// Tests for GetBgSnapshotByID
// ============================================================================

// TestGetBgSnapshotByID tests retrieving a snapshot by its ID
func TestGetBgSnapshotByID(t *testing.T) {
	fname := t.TempDir() + "/test_bg_by_id.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert a snapshot
	snap := &l3grid.BgSnapshot{
		SensorID:           "test-sensor",
		TakenUnixNanos:     time.Now().UnixNano(),
		Rings:              40,
		AzimuthBins:        1800,
		ParamsJSON:         `{"mode": "test"}`,
		RingElevationsJSON: `[1.0, 2.0]`,
		GridBlob:           []byte("test-grid-data"),
		ChangedCellsCount:  42,
		SnapshotReason:     "test-reason",
	}

	insertedID, err := db.InsertBgSnapshot(snap)
	if err != nil {
		t.Fatalf("InsertBgSnapshot failed: %v", err)
	}

	// Retrieve by ID
	retrieved, err := db.GetBgSnapshotByID(insertedID)
	if err != nil {
		t.Fatalf("GetBgSnapshotByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil snapshot")
	}

	// Verify all fields match
	if retrieved.SensorID != snap.SensorID {
		t.Errorf("SensorID mismatch: got %q, want %q", retrieved.SensorID, snap.SensorID)
	}
	if retrieved.Rings != snap.Rings {
		t.Errorf("Rings mismatch: got %d, want %d", retrieved.Rings, snap.Rings)
	}
	if retrieved.AzimuthBins != snap.AzimuthBins {
		t.Errorf("AzimuthBins mismatch: got %d, want %d", retrieved.AzimuthBins, snap.AzimuthBins)
	}
	if retrieved.ParamsJSON != snap.ParamsJSON {
		t.Errorf("ParamsJSON mismatch: got %q, want %q", retrieved.ParamsJSON, snap.ParamsJSON)
	}
	if retrieved.RingElevationsJSON != snap.RingElevationsJSON {
		t.Errorf("RingElevationsJSON mismatch: got %q, want %q", retrieved.RingElevationsJSON, snap.RingElevationsJSON)
	}
	if string(retrieved.GridBlob) != string(snap.GridBlob) {
		t.Errorf("GridBlob mismatch: got %q, want %q", retrieved.GridBlob, snap.GridBlob)
	}
	if retrieved.ChangedCellsCount != snap.ChangedCellsCount {
		t.Errorf("ChangedCellsCount mismatch: got %d, want %d", retrieved.ChangedCellsCount, snap.ChangedCellsCount)
	}
	if retrieved.SnapshotReason != snap.SnapshotReason {
		t.Errorf("SnapshotReason mismatch: got %q, want %q", retrieved.SnapshotReason, snap.SnapshotReason)
	}
	if retrieved.SnapshotID == nil || *retrieved.SnapshotID != insertedID {
		t.Errorf("SnapshotID mismatch: got %v, want %d", retrieved.SnapshotID, insertedID)
	}
}

// TestGetBgSnapshotByID_NotFound tests retrieval of non-existent snapshot
func TestGetBgSnapshotByID_NotFound(t *testing.T) {
	fname := t.TempDir() + "/test_bg_by_id_notfound.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Try to retrieve a snapshot that doesn't exist
	retrieved, err := db.GetBgSnapshotByID(99999)
	if err != nil {
		t.Fatalf("GetBgSnapshotByID failed: %v", err)
	}

	if retrieved != nil {
		t.Error("Expected nil for non-existent snapshot")
	}
}

// TestGetBgSnapshotByID_InvalidID tests retrieval with invalid (zero/negative) ID
func TestGetBgSnapshotByID_InvalidID(t *testing.T) {
	fname := t.TempDir() + "/test_bg_by_id_invalid.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Zero ID should return nil
	retrieved, err := db.GetBgSnapshotByID(0)
	if err != nil {
		t.Fatalf("GetBgSnapshotByID(0) failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Expected nil for zero ID")
	}

	// Negative ID should return nil
	retrieved, err = db.GetBgSnapshotByID(-1)
	if err != nil {
		t.Fatalf("GetBgSnapshotByID(-1) failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Expected nil for negative ID")
	}
}

// ============================================================================
// Tests for Region Snapshot functions
// ============================================================================

// TestInsertRegionSnapshot tests inserting a region snapshot
func TestInsertRegionSnapshot(t *testing.T) {
	fname := t.TempDir() + "/test_insert_region.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// First insert a background snapshot (required foreign key reference)
	bgSnap := &l3grid.BgSnapshot{
		SensorID:          "test-sensor",
		TakenUnixNanos:    time.Now().UnixNano(),
		Rings:             40,
		AzimuthBins:       1800,
		GridBlob:          []byte("bg-blob"),
		ChangedCellsCount: 10,
		SnapshotReason:    "test",
	}
	bgID, err := db.InsertBgSnapshot(bgSnap)
	if err != nil {
		t.Fatalf("InsertBgSnapshot failed: %v", err)
	}

	// Insert a region snapshot
	regionSnap := &l3grid.RegionSnapshot{
		SnapshotID:       bgID,
		SensorID:         "test-sensor",
		CreatedUnixNanos: time.Now().UnixNano(),
		RegionCount:      5,
		RegionsJSON:      `[{"id":1,"params":{},"cell_list":[1,2,3],"mean_variance":0.5,"cell_count":3}]`,
		VarianceDataJSON: `{"total_variance":0.8}`,
		SettlingFrames:   100,
		SceneHash:        "abc123hash",
		SourcePath:       "/path/to/test.pcap",
	}

	regionID, err := db.InsertRegionSnapshot(regionSnap)
	if err != nil {
		t.Fatalf("InsertRegionSnapshot failed: %v", err)
	}

	if regionID <= 0 {
		t.Errorf("Expected positive region_set_id, got %d", regionID)
	}
}

// TestInsertRegionSnapshot_NilSnapshot tests handling of nil snapshot
func TestInsertRegionSnapshot_NilSnapshot(t *testing.T) {
	fname := t.TempDir() + "/test_insert_region_nil.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	id, err := db.InsertRegionSnapshot(nil)
	if err != nil {
		t.Errorf("InsertRegionSnapshot(nil) returned error: %v", err)
	}
	if id != 0 {
		t.Errorf("Expected id 0 for nil snapshot, got %d", id)
	}
}

// TestGetRegionSnapshotBySceneHash tests retrieval by scene hash
func TestGetRegionSnapshotBySceneHash(t *testing.T) {
	fname := t.TempDir() + "/test_region_by_hash.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert background snapshot
	bgID, err := db.InsertBgSnapshot(&l3grid.BgSnapshot{
		SensorID:          "test-sensor",
		TakenUnixNanos:    time.Now().UnixNano(),
		Rings:             40,
		AzimuthBins:       1800,
		GridBlob:          []byte("bg-blob"),
		ChangedCellsCount: 10,
		SnapshotReason:    "test",
	})
	if err != nil {
		t.Fatalf("InsertBgSnapshot failed: %v", err)
	}

	// Insert region snapshot with a specific scene hash
	sceneHash := "unique-scene-hash-12345"
	regionSnap := &l3grid.RegionSnapshot{
		SnapshotID:       bgID,
		SensorID:         "test-sensor",
		CreatedUnixNanos: time.Now().UnixNano(),
		RegionCount:      3,
		RegionsJSON:      `[]`,
		VarianceDataJSON: `{}`,
		SettlingFrames:   50,
		SceneHash:        sceneHash,
		SourcePath:       "/test/file.pcap",
	}

	_, err = db.InsertRegionSnapshot(regionSnap)
	if err != nil {
		t.Fatalf("InsertRegionSnapshot failed: %v", err)
	}

	// Retrieve by scene hash
	retrieved, err := db.GetRegionSnapshotBySceneHash("test-sensor", sceneHash)
	if err != nil {
		t.Fatalf("GetRegionSnapshotBySceneHash failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil region snapshot")
	}

	if retrieved.SceneHash != sceneHash {
		t.Errorf("SceneHash mismatch: got %q, want %q", retrieved.SceneHash, sceneHash)
	}
	if retrieved.SensorID != "test-sensor" {
		t.Errorf("SensorID mismatch: got %q, want %q", retrieved.SensorID, "test-sensor")
	}
	if retrieved.RegionCount != 3 {
		t.Errorf("RegionCount mismatch: got %d, want %d", retrieved.RegionCount, 3)
	}
}

// TestGetRegionSnapshotBySceneHash_NotFound tests retrieval with non-existent hash
func TestGetRegionSnapshotBySceneHash_NotFound(t *testing.T) {
	fname := t.TempDir() + "/test_region_hash_notfound.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	retrieved, err := db.GetRegionSnapshotBySceneHash("test-sensor", "nonexistent-hash")
	if err != nil {
		t.Fatalf("GetRegionSnapshotBySceneHash failed: %v", err)
	}

	if retrieved != nil {
		t.Error("Expected nil for non-existent scene hash")
	}
}

// TestGetRegionSnapshotBySceneHash_EmptyHash tests retrieval with empty hash
func TestGetRegionSnapshotBySceneHash_EmptyHash(t *testing.T) {
	fname := t.TempDir() + "/test_region_hash_empty.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Empty hash should return nil without error
	retrieved, err := db.GetRegionSnapshotBySceneHash("test-sensor", "")
	if err != nil {
		t.Fatalf("GetRegionSnapshotBySceneHash failed: %v", err)
	}

	if retrieved != nil {
		t.Error("Expected nil for empty scene hash")
	}
}

// TestGetRegionSnapshotBySourcePath tests retrieval by source path
func TestGetRegionSnapshotBySourcePath(t *testing.T) {
	fname := t.TempDir() + "/test_region_by_path.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert background snapshot
	bgID, err := db.InsertBgSnapshot(&l3grid.BgSnapshot{
		SensorID:          "test-sensor",
		TakenUnixNanos:    time.Now().UnixNano(),
		Rings:             40,
		AzimuthBins:       1800,
		GridBlob:          []byte("bg-blob"),
		ChangedCellsCount: 10,
		SnapshotReason:    "test",
	})
	if err != nil {
		t.Fatalf("InsertBgSnapshot failed: %v", err)
	}

	// Insert region snapshot with a specific source path
	sourcePath := "/data/captures/test-capture-2025.pcap"
	regionSnap := &l3grid.RegionSnapshot{
		SnapshotID:       bgID,
		SensorID:         "test-sensor",
		CreatedUnixNanos: time.Now().UnixNano(),
		RegionCount:      7,
		RegionsJSON:      `[{"id":1}]`,
		VarianceDataJSON: `{"settling":true}`,
		SettlingFrames:   200,
		SceneHash:        "some-hash",
		SourcePath:       sourcePath,
	}

	_, err = db.InsertRegionSnapshot(regionSnap)
	if err != nil {
		t.Fatalf("InsertRegionSnapshot failed: %v", err)
	}

	// Retrieve by source path
	retrieved, err := db.GetRegionSnapshotBySourcePath("test-sensor", sourcePath)
	if err != nil {
		t.Fatalf("GetRegionSnapshotBySourcePath failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil region snapshot")
	}

	if retrieved.SourcePath != sourcePath {
		t.Errorf("SourcePath mismatch: got %q, want %q", retrieved.SourcePath, sourcePath)
	}
	if retrieved.RegionCount != 7 {
		t.Errorf("RegionCount mismatch: got %d, want %d", retrieved.RegionCount, 7)
	}
	if retrieved.SettlingFrames != 200 {
		t.Errorf("SettlingFrames mismatch: got %d, want %d", retrieved.SettlingFrames, 200)
	}
}

// TestGetRegionSnapshotBySourcePath_NotFound tests retrieval with non-existent path
func TestGetRegionSnapshotBySourcePath_NotFound(t *testing.T) {
	fname := t.TempDir() + "/test_region_path_notfound.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	retrieved, err := db.GetRegionSnapshotBySourcePath("test-sensor", "/nonexistent/path.pcap")
	if err != nil {
		t.Fatalf("GetRegionSnapshotBySourcePath failed: %v", err)
	}

	if retrieved != nil {
		t.Error("Expected nil for non-existent source path")
	}
}

// TestGetRegionSnapshotBySourcePath_EmptyPath tests retrieval with empty path
func TestGetRegionSnapshotBySourcePath_EmptyPath(t *testing.T) {
	fname := t.TempDir() + "/test_region_path_empty.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Empty path should return nil without error
	retrieved, err := db.GetRegionSnapshotBySourcePath("test-sensor", "")
	if err != nil {
		t.Fatalf("GetRegionSnapshotBySourcePath failed: %v", err)
	}

	if retrieved != nil {
		t.Error("Expected nil for empty source path")
	}
}

// TestGetLatestRegionSnapshot tests retrieval of most recent region snapshot
func TestGetLatestRegionSnapshot(t *testing.T) {
	fname := t.TempDir() + "/test_latest_region.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert background snapshot
	bgID, err := db.InsertBgSnapshot(&l3grid.BgSnapshot{
		SensorID:          "test-sensor",
		TakenUnixNanos:    time.Now().UnixNano(),
		Rings:             40,
		AzimuthBins:       1800,
		GridBlob:          []byte("bg-blob"),
		ChangedCellsCount: 10,
		SnapshotReason:    "test",
	})
	if err != nil {
		t.Fatalf("InsertBgSnapshot failed: %v", err)
	}

	// Insert multiple region snapshots
	for i := 0; i < 5; i++ {
		regionSnap := &l3grid.RegionSnapshot{
			SnapshotID:       bgID,
			SensorID:         "test-sensor",
			CreatedUnixNanos: time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			RegionCount:      i + 1,
			RegionsJSON:      `[]`,
			SettlingFrames:   i * 10,
			SceneHash:        "hash-" + string(rune('a'+i)),
		}

		_, err = db.InsertRegionSnapshot(regionSnap)
		if err != nil {
			t.Fatalf("InsertRegionSnapshot failed: %v", err)
		}
	}

	// Get latest region snapshot
	latest, err := db.GetLatestRegionSnapshot("test-sensor")
	if err != nil {
		t.Fatalf("GetLatestRegionSnapshot failed: %v", err)
	}

	if latest == nil {
		t.Fatal("Expected non-nil region snapshot")
	}

	// Should be the last one inserted (highest region_set_id)
	if latest.RegionCount != 5 {
		t.Errorf("RegionCount mismatch: got %d, want 5 (latest)", latest.RegionCount)
	}
	if latest.SettlingFrames != 40 {
		t.Errorf("SettlingFrames mismatch: got %d, want 40", latest.SettlingFrames)
	}
}

// TestGetLatestRegionSnapshot_NotFound tests retrieval when no snapshots exist
func TestGetLatestRegionSnapshot_NotFound(t *testing.T) {
	fname := t.TempDir() + "/test_latest_region_notfound.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	retrieved, err := db.GetLatestRegionSnapshot("nonexistent-sensor")
	if err != nil {
		t.Fatalf("GetLatestRegionSnapshot failed: %v", err)
	}

	if retrieved != nil {
		t.Error("Expected nil for non-existent sensor")
	}
}

// TestGetRegionSnapshotBySceneHash_MostRecent tests that the most recent matching hash is returned
func TestGetRegionSnapshotBySceneHash_MostRecent(t *testing.T) {
	fname := t.TempDir() + "/test_region_hash_recent.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert background snapshot
	bgID, err := db.InsertBgSnapshot(&l3grid.BgSnapshot{
		SensorID:          "test-sensor",
		TakenUnixNanos:    time.Now().UnixNano(),
		Rings:             40,
		AzimuthBins:       1800,
		GridBlob:          []byte("bg-blob"),
		ChangedCellsCount: 10,
		SnapshotReason:    "test",
	})
	if err != nil {
		t.Fatalf("InsertBgSnapshot failed: %v", err)
	}

	// Insert multiple region snapshots with the same scene hash
	sceneHash := "duplicate-hash"
	for i := 0; i < 3; i++ {
		regionSnap := &l3grid.RegionSnapshot{
			SnapshotID:       bgID,
			SensorID:         "test-sensor",
			CreatedUnixNanos: time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			RegionCount:      (i + 1) * 10, // 10, 20, 30
			RegionsJSON:      `[]`,
			SettlingFrames:   i,
			SceneHash:        sceneHash,
		}

		_, err = db.InsertRegionSnapshot(regionSnap)
		if err != nil {
			t.Fatalf("InsertRegionSnapshot failed: %v", err)
		}
	}

	// Retrieve by scene hash - should get the most recent (highest ID)
	retrieved, err := db.GetRegionSnapshotBySceneHash("test-sensor", sceneHash)
	if err != nil {
		t.Fatalf("GetRegionSnapshotBySceneHash failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil region snapshot")
	}

	// Should return the most recent one (RegionCount = 30)
	if retrieved.RegionCount != 30 {
		t.Errorf("Expected most recent snapshot (RegionCount=30), got RegionCount=%d", retrieved.RegionCount)
	}
}

// TestRegionSnapshotAllFields tests that all fields are correctly stored and retrieved
func TestRegionSnapshotAllFields(t *testing.T) {
	fname := t.TempDir() + "/test_region_all_fields.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert background snapshot
	bgID, err := db.InsertBgSnapshot(&l3grid.BgSnapshot{
		SensorID:          "sensor-xyz",
		TakenUnixNanos:    time.Now().UnixNano(),
		Rings:             40,
		AzimuthBins:       1800,
		GridBlob:          []byte("bg-blob"),
		ChangedCellsCount: 10,
		SnapshotReason:    "test",
	})
	if err != nil {
		t.Fatalf("InsertBgSnapshot failed: %v", err)
	}

	now := time.Now().UnixNano()
	regionSnap := &l3grid.RegionSnapshot{
		SnapshotID:       bgID,
		SensorID:         "sensor-xyz",
		CreatedUnixNanos: now,
		RegionCount:      42,
		RegionsJSON:      `[{"id":1,"params":{"min_radius":5},"cell_list":[10,20,30],"mean_variance":1.5,"cell_count":3}]`,
		VarianceDataJSON: `{"total_variance":2.5,"settled":true}`,
		SettlingFrames:   150,
		SceneHash:        "abc123def456",
		SourcePath:       "/data/pcap/capture-20250206.pcap",
	}

	insertedID, err := db.InsertRegionSnapshot(regionSnap)
	if err != nil {
		t.Fatalf("InsertRegionSnapshot failed: %v", err)
	}

	// Retrieve and verify all fields
	retrieved, err := db.GetLatestRegionSnapshot("sensor-xyz")
	if err != nil {
		t.Fatalf("GetLatestRegionSnapshot failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil region snapshot")
	}

	// Verify RegionSetID was set
	if retrieved.RegionSetID == nil || *retrieved.RegionSetID != insertedID {
		t.Errorf("RegionSetID mismatch: got %v, want %d", retrieved.RegionSetID, insertedID)
	}
	if retrieved.SnapshotID != bgID {
		t.Errorf("SnapshotID mismatch: got %d, want %d", retrieved.SnapshotID, bgID)
	}
	if retrieved.SensorID != "sensor-xyz" {
		t.Errorf("SensorID mismatch: got %q, want %q", retrieved.SensorID, "sensor-xyz")
	}
	if retrieved.CreatedUnixNanos != now {
		t.Errorf("CreatedUnixNanos mismatch: got %d, want %d", retrieved.CreatedUnixNanos, now)
	}
	if retrieved.RegionCount != 42 {
		t.Errorf("RegionCount mismatch: got %d, want %d", retrieved.RegionCount, 42)
	}
	if retrieved.RegionsJSON != regionSnap.RegionsJSON {
		t.Errorf("RegionsJSON mismatch: got %q, want %q", retrieved.RegionsJSON, regionSnap.RegionsJSON)
	}
	if retrieved.VarianceDataJSON != regionSnap.VarianceDataJSON {
		t.Errorf("VarianceDataJSON mismatch: got %q, want %q", retrieved.VarianceDataJSON, regionSnap.VarianceDataJSON)
	}
	if retrieved.SettlingFrames != 150 {
		t.Errorf("SettlingFrames mismatch: got %d, want %d", retrieved.SettlingFrames, 150)
	}
	if retrieved.SceneHash != "abc123def456" {
		t.Errorf("SceneHash mismatch: got %q, want %q", retrieved.SceneHash, "abc123def456")
	}
	if retrieved.SourcePath != "/data/pcap/capture-20250206.pcap" {
		t.Errorf("SourcePath mismatch: got %q, want %q", retrieved.SourcePath, "/data/pcap/capture-20250206.pcap")
	}
}

// TestRegionSnapshotNullableFields tests correct handling of nullable fields
func TestRegionSnapshotNullableFields(t *testing.T) {
	fname := t.TempDir() + "/test_region_nullable.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert background snapshot
	bgID, err := db.InsertBgSnapshot(&l3grid.BgSnapshot{
		SensorID:          "test-sensor",
		TakenUnixNanos:    time.Now().UnixNano(),
		Rings:             40,
		AzimuthBins:       1800,
		GridBlob:          []byte("bg-blob"),
		ChangedCellsCount: 10,
		SnapshotReason:    "test",
	})
	if err != nil {
		t.Fatalf("InsertBgSnapshot failed: %v", err)
	}

	// Insert region snapshot with minimal/empty optional fields
	regionSnap := &l3grid.RegionSnapshot{
		SnapshotID:       bgID,
		SensorID:         "test-sensor",
		CreatedUnixNanos: time.Now().UnixNano(),
		RegionCount:      1,
		RegionsJSON:      `[]`,
		VarianceDataJSON: "", // Empty
		SettlingFrames:   0,  // Zero
		SceneHash:        "", // Empty
		SourcePath:       "", // Empty
	}

	_, err = db.InsertRegionSnapshot(regionSnap)
	if err != nil {
		t.Fatalf("InsertRegionSnapshot failed: %v", err)
	}

	// Retrieve and verify nullable fields are handled correctly
	retrieved, err := db.GetLatestRegionSnapshot("test-sensor")
	if err != nil {
		t.Fatalf("GetLatestRegionSnapshot failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil region snapshot")
	}

	// Nullable string fields should be empty strings (not cause errors)
	if retrieved.VarianceDataJSON != "" {
		t.Errorf("Expected empty VarianceDataJSON, got %q", retrieved.VarianceDataJSON)
	}
	if retrieved.SceneHash != "" {
		t.Errorf("Expected empty SceneHash, got %q", retrieved.SceneHash)
	}
	if retrieved.SourcePath != "" {
		t.Errorf("Expected empty SourcePath, got %q", retrieved.SourcePath)
	}
}

// TestGetRegionSnapshotBySourcePath_MostRecent tests that the most recent matching path is returned
func TestGetRegionSnapshotBySourcePath_MostRecent(t *testing.T) {
	fname := t.TempDir() + "/test_region_path_recent.db"
	db, err := NewDB(fname)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	defer db.Close()

	// Insert background snapshot
	bgID, err := db.InsertBgSnapshot(&l3grid.BgSnapshot{
		SensorID:          "test-sensor",
		TakenUnixNanos:    time.Now().UnixNano(),
		Rings:             40,
		AzimuthBins:       1800,
		GridBlob:          []byte("bg-blob"),
		ChangedCellsCount: 10,
		SnapshotReason:    "test",
	})
	if err != nil {
		t.Fatalf("InsertBgSnapshot failed: %v", err)
	}

	// Insert multiple region snapshots with the same source path
	sourcePath := "/data/test.pcap"
	for i := 0; i < 3; i++ {
		regionSnap := &l3grid.RegionSnapshot{
			SnapshotID:       bgID,
			SensorID:         "test-sensor",
			CreatedUnixNanos: time.Now().Add(time.Duration(i) * time.Second).UnixNano(),
			RegionCount:      (i + 1) * 100, // 100, 200, 300
			RegionsJSON:      `[]`,
			SettlingFrames:   i,
			SourcePath:       sourcePath,
		}

		_, err = db.InsertRegionSnapshot(regionSnap)
		if err != nil {
			t.Fatalf("InsertRegionSnapshot failed: %v", err)
		}
	}

	// Retrieve by source path - should get the most recent (highest ID)
	retrieved, err := db.GetRegionSnapshotBySourcePath("test-sensor", sourcePath)
	if err != nil {
		t.Fatalf("GetRegionSnapshotBySourcePath failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil region snapshot")
	}

	// Should return the most recent one (RegionCount = 300)
	if retrieved.RegionCount != 300 {
		t.Errorf("Expected most recent snapshot (RegionCount=300), got RegionCount=%d", retrieved.RegionCount)
	}
}
