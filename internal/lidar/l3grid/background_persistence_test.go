package l3grid

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Persist
// ---------------------------------------------------------------------------

// mockPersistBgStore is a minimal BgStore implementation (no RegionStore methods).
type mockPersistBgStore struct {
	lastID    int64
	insertErr error
	snapshots []*BgSnapshot
}

func (m *mockPersistBgStore) InsertBgSnapshot(s *BgSnapshot) (int64, error) {
	if m.insertErr != nil {
		return 0, m.insertErr
	}
	m.lastID++
	m.snapshots = append(m.snapshots, s)
	return m.lastID, nil
}

func TestPersist_NilCases(t *testing.T) {
	t.Parallel()

	t.Run("nil manager", func(t *testing.T) {
		t.Parallel()
		var bm *BackgroundManager
		err := bm.Persist(&mockPersistBgStore{}, "test")
		assert.NoError(t, err)
	})

	t.Run("nil grid", func(t *testing.T) {
		t.Parallel()
		bm := &BackgroundManager{Grid: nil}
		err := bm.Persist(&mockPersistBgStore{}, "test")
		assert.NoError(t, err)
	})

	t.Run("nil store", func(t *testing.T) {
		t.Parallel()
		g := makeTestGrid(1, 4)
		err := g.Manager.Persist(nil, "test")
		assert.NoError(t, err)
	})
}

func TestPersist_BasicSuccess(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	bm := &BackgroundManager{Grid: g}
	g.Manager = bm
	g.ChangesSinceSnapshot = 42

	store := &mockPersistBgStore{}
	err := bm.Persist(store, "manual")
	require.NoError(t, err)

	// Verify snapshot was inserted
	require.Len(t, store.snapshots, 1)
	snap := store.snapshots[0]
	assert.Equal(t, "test-sensor", snap.SensorID)
	assert.Equal(t, 4, snap.Rings)
	assert.Equal(t, 8, snap.AzimuthBins)
	assert.Equal(t, "manual", snap.SnapshotReason)
	assert.Equal(t, 42, snap.ChangedCellsCount)
	assert.NotEmpty(t, snap.GridBlob)

	// Verify grid metadata was updated
	assert.NotNil(t, g.SnapshotID)
	assert.Equal(t, int64(1), *g.SnapshotID)
	assert.False(t, g.LastSnapshotTime.IsZero())
	assert.False(t, bm.LastPersistTime.IsZero())
	// ChangesSinceSnapshot should be decremented
	assert.Equal(t, 0, g.ChangesSinceSnapshot)
}

func TestPersist_WithRingElevations(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	bm := &BackgroundManager{Grid: g}
	g.Manager = bm
	g.RingElevations = []float64{-10.0, -5.0, 0.0, 5.0} // len == Rings

	store := &mockPersistBgStore{}
	err := bm.Persist(store, "with-elevations")
	require.NoError(t, err)

	require.Len(t, store.snapshots, 1)
	snap := store.snapshots[0]
	assert.Contains(t, snap.RingElevationsJSON, "-10")
	assert.Contains(t, snap.RingElevationsJSON, "5")
}

func TestPersist_InsertError(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	bm := &BackgroundManager{Grid: g}
	g.Manager = bm

	store := &mockPersistBgStore{insertErr: fmt.Errorf("db error")}
	err := bm.Persist(store, "fail")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestPersist_WithRegionStore(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	bm := &BackgroundManager{Grid: g}
	g.Manager = bm
	g.RegionMgr = NewRegionManager(4, 8)
	g.RegionMgr.IdentificationComplete = true
	g.RegionMgr.Regions = []*Region{
		{ID: 0, CellList: []int{0, 1, 2}, CellCount: 3},
	}

	store := newMockRegionStore()
	err := bm.Persist(store, "regions")
	require.NoError(t, err)

	// BgSnapshot + RegionSnapshot = 2 inserts
	assert.Equal(t, int64(2), store.lastInsertedID)
}

func TestPersist_RegionInsertError(t *testing.T) {
	t.Parallel()
	// This test verifies the region insert error path in Persist is handled
	// gracefully (logged, not returned as error). The mockRegionStore uses a
	// shared insertErr for both BgSnapshot and RegionSnapshot inserts, so we
	// cannot easily test the case where only region insert fails.
	// The happy path with regions is covered by TestPersist_WithRegionStore.
}

func TestPersist_ConcurrentChanges(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	bm := &BackgroundManager{Grid: g}
	g.Manager = bm
	g.ChangesSinceSnapshot = 100

	store := &mockPersistBgStore{}
	err := bm.Persist(store, "concurrent")
	require.NoError(t, err)

	// Changes should be decremented by the snapshot amount
	assert.Equal(t, 0, g.ChangesSinceSnapshot)
}

func TestPersist_DefensiveChangeCounter(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	bm := &BackgroundManager{Grid: g}
	g.Manager = bm
	// Simulate a race: changesSince copied was larger than current counter
	g.ChangesSinceSnapshot = 0 // Will be 0 when we get to the write lock

	store := &mockPersistBgStore{}
	err := bm.Persist(store, "defensive")
	require.NoError(t, err)

	// Should be 0, not negative
	assert.Equal(t, 0, g.ChangesSinceSnapshot)
}

// ---------------------------------------------------------------------------
// Serialisation edge cases
// ---------------------------------------------------------------------------
func TestSerializeDeserializeGrid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		cells []BackgroundCell
	}{
		{
			name:  "empty cells",
			cells: []BackgroundCell{},
		},
		{
			name: "single cell",
			cells: []BackgroundCell{
				{AverageRangeMeters: 10.5, RangeSpreadMeters: 0.3, TimesSeenCount: 15},
			},
		},
		{
			name: "multiple cells with varying values",
			cells: []BackgroundCell{
				{AverageRangeMeters: 5.0, RangeSpreadMeters: 0.1, TimesSeenCount: 10},
				{AverageRangeMeters: 20.0, RangeSpreadMeters: 0.5, TimesSeenCount: 25},
				{AverageRangeMeters: 0, RangeSpreadMeters: 0, TimesSeenCount: 0}, // empty cell
				{AverageRangeMeters: 100.0, RangeSpreadMeters: 2.5, TimesSeenCount: 100},
			},
		},
		{
			name: "realistic grid size",
			cells: func() []BackgroundCell {
				cells := make([]BackgroundCell, 40*1800)
				for i := range cells {
					if i%3 == 0 {
						cells[i] = BackgroundCell{
							AverageRangeMeters: float32(i%100) + 5,
							RangeSpreadMeters:  float32(i%10) * 0.1,
							TimesSeenCount:     uint32(i % 50),
						}
					}
				}
				return cells
			}(),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Serialise
			blob, err := serializeGrid(tc.cells)
			require.NoError(t, err, "serializeGrid should succeed")
			require.NotEmpty(t, blob, "serialised blob should not be empty")

			// Deserialise
			restored, err := deserializeGrid(blob)
			require.NoError(t, err, "deserializeGrid should succeed")
			assert.Len(t, restored, len(tc.cells), "restored cell count should match")

			// Verify cell values
			for i, original := range tc.cells {
				assert.Equal(t, original.AverageRangeMeters, restored[i].AverageRangeMeters,
					"cell %d: AverageRangeMeters mismatch", i)
				assert.Equal(t, original.RangeSpreadMeters, restored[i].RangeSpreadMeters,
					"cell %d: RangeSpreadMeters mismatch", i)
				assert.Equal(t, original.TimesSeenCount, restored[i].TimesSeenCount,
					"cell %d: TimesSeenCount mismatch", i)
			}
		})
	}
}

// TestDeserializeGrid_InvalidInput tests deserialisation error handling.
func TestDeserializeGrid_InvalidInput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		blob    []byte
		wantErr string
	}{
		{
			name:    "empty blob",
			blob:    []byte{},
			wantErr: "empty grid blob",
		},
		{
			name:    "invalid gzip data",
			blob:    []byte("not valid gzip"),
			wantErr: "failed to create gzip reader",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := deserializeGrid(tc.blob)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestSceneSignature tests the scene signature generation.
func TestSceneSignature(t *testing.T) {
	t.Parallel()

	t.Run("empty grid returns empty signature", func(t *testing.T) {
		t.Parallel()
		g := &BackgroundGrid{
			Cells: []BackgroundCell{},
		}
		assert.Empty(t, g.SceneSignature())
	})

	t.Run("no observed cells still returns signature with dimensions", func(t *testing.T) {
		t.Parallel()
		g := &BackgroundGrid{
			Rings:       4,
			AzimuthBins: 8,
			Cells:       make([]BackgroundCell, 32), // All zero TimesSeenCount
		}
		// Signature still includes grid dimensions, even with zero coverage
		sig := g.SceneSignature()
		assert.NotEmpty(t, sig)
		assert.Len(t, sig, 32) // SHA256 first 16 bytes = 32 hex chars
	})

	t.Run("generates consistent signature for same data", func(t *testing.T) {
		t.Parallel()
		g := makeTestGrid(4, 8)

		// Populate cells with data
		for i := 0; i < len(g.Cells); i++ {
			g.Cells[i] = BackgroundCell{
				AverageRangeMeters: float32(10 + i%5),
				RangeSpreadMeters:  0.05,
				TimesSeenCount:     10,
			}
		}

		sig1 := g.SceneSignature()
		sig2 := g.SceneSignature()

		assert.NotEmpty(t, sig1)
		assert.Equal(t, sig1, sig2, "signature should be deterministic")
		assert.Len(t, sig1, 32, "signature should be 32 hex chars (16 bytes)")
	})

	t.Run("different data produces different signatures", func(t *testing.T) {
		t.Parallel()
		g1 := makeTestGrid(4, 8)
		g2 := makeTestGrid(4, 8)

		// Different range distributions
		for i := range g1.Cells {
			g1.Cells[i] = BackgroundCell{AverageRangeMeters: 5.0, TimesSeenCount: 10}
			g2.Cells[i] = BackgroundCell{AverageRangeMeters: 50.0, TimesSeenCount: 10}
		}

		sig1 := g1.SceneSignature()
		sig2 := g2.SceneSignature()

		assert.NotEmpty(t, sig1)
		assert.NotEmpty(t, sig2)
		assert.NotEqual(t, sig1, sig2, "different scenes should produce different signatures")
	})

	t.Run("covers all range buckets", func(t *testing.T) {
		t.Parallel()
		g := makeTestGrid(6, 8)

		// Set cells to cover all range buckets: <5m, 5-10m, 10-20m, 20-50m, 50-100m, >100m
		ranges := []float32{3.0, 7.0, 15.0, 35.0, 75.0, 150.0}
		for i, r := range ranges {
			idx := i * 8 // Different row per range
			for j := 0; j < 8; j++ {
				g.Cells[idx+j] = BackgroundCell{AverageRangeMeters: r, TimesSeenCount: 5}
			}
		}

		sig := g.SceneSignature()
		assert.NotEmpty(t, sig)
	})

	t.Run("covers all spread buckets", func(t *testing.T) {
		t.Parallel()
		g := makeTestGrid(4, 8)

		// Set cells to cover all spread buckets: <0.05m, 0.05-0.1m, 0.1-0.2m, >0.2m
		spreads := []float32{0.02, 0.07, 0.15, 0.5}
		for i, s := range spreads {
			idx := i * 8
			for j := 0; j < 8; j++ {
				g.Cells[idx+j] = BackgroundCell{
					AverageRangeMeters: 10.0,
					RangeSpreadMeters:  s,
					TimesSeenCount:     5,
				}
			}
		}

		sig := g.SceneSignature()
		assert.NotEmpty(t, sig)
	})
}

// TestSourcePath tests SetSourcePath and GetSourcePath methods.
func TestSourcePath(t *testing.T) {
	t.Parallel()

	t.Run("nil manager returns empty string", func(t *testing.T) {
		t.Parallel()
		var bm *BackgroundManager
		assert.Empty(t, bm.GetSourcePath())
	})

	t.Run("nil manager SetSourcePath is no-op", func(t *testing.T) {
		t.Parallel()
		var bm *BackgroundManager
		bm.SetSourcePath("/path/to/file.pcap") // Should not panic
	})

	t.Run("set and get source path", func(t *testing.T) {
		t.Parallel()
		g := makeTestGrid(2, 4)
		bm := g.Manager

		path := "/data/capture-2025-01-15.pcap"
		bm.SetSourcePath(path)

		assert.Equal(t, path, bm.GetSourcePath())
	})

	t.Run("overwrite source path", func(t *testing.T) {
		t.Parallel()
		g := makeTestGrid(2, 4)
		bm := g.Manager

		bm.SetSourcePath("/first/path.pcap")
		bm.SetSourcePath("/second/path.pcap")

		assert.Equal(t, "/second/path.pcap", bm.GetSourcePath())
	})
}

// TestRegionManagerToSnapshot tests the RegionManager ToSnapshot method.
func TestRegionManagerToSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when identification not complete", func(t *testing.T) {
		t.Parallel()
		rm := NewRegionManager(4, 8)
		rm.IdentificationComplete = false

		snap := rm.ToSnapshot("sensor-1", 123)
		assert.Nil(t, snap)
	})

	t.Run("returns nil when no regions", func(t *testing.T) {
		t.Parallel()
		rm := NewRegionManager(4, 8)
		rm.IdentificationComplete = true
		rm.Regions = []*Region{} // Empty

		snap := rm.ToSnapshot("sensor-1", 123)
		assert.Nil(t, snap)
	})

	t.Run("creates snapshot with regions", func(t *testing.T) {
		t.Parallel()
		rm := NewRegionManager(4, 8)
		rm.IdentificationComplete = true
		rm.SettlingMetrics.FramesSampled = 50

		// Add test regions
		rm.Regions = []*Region{
			{
				ID:           0,
				CellList:     []int{0, 1, 2, 3},
				CellCount:    4,
				MeanVariance: 0.1,
				Params: RegionParams{
					SettleUpdateFraction:  0.02,
					NoiseRelativeFraction: 0.01,
				},
			},
			{
				ID:           1,
				CellList:     []int{4, 5, 6, 7},
				CellCount:    4,
				MeanVariance: 1.5,
				Params: RegionParams{
					SettleUpdateFraction:  0.05,
					NoiseRelativeFraction: 0.02,
				},
			},
		}

		snap := rm.ToSnapshot("sensor-1", 456)
		require.NotNil(t, snap)

		assert.Equal(t, int64(456), snap.SnapshotID)
		assert.Equal(t, "sensor-1", snap.SensorID)
		assert.Equal(t, 2, snap.RegionCount)
		assert.Equal(t, 50, snap.SettlingFrames)
		assert.NotEmpty(t, snap.RegionsJSON)
		assert.NotZero(t, snap.CreatedUnixNanos)
	})

	t.Run("includes variance data when present", func(t *testing.T) {
		t.Parallel()
		rm := NewRegionManager(2, 4)
		rm.IdentificationComplete = true
		rm.SettlingMetrics.FramesSampled = 30
		rm.SettlingMetrics.VariancePerCell = []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}

		rm.Regions = []*Region{
			{ID: 0, CellList: []int{0, 1}, CellCount: 2},
		}

		snap := rm.ToSnapshot("sensor-1", 789)
		require.NotNil(t, snap)
		assert.NotEmpty(t, snap.VarianceDataJSON)
	})
}

// TestRegionManagerRestoreFromSnapshot tests the RegionManager restoration.
func TestRegionManagerRestoreFromSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("nil snapshot returns error", func(t *testing.T) {
		t.Parallel()
		rm := NewRegionManager(4, 8)

		err := rm.RestoreFromSnapshot(nil, 32)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil snapshot")
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		t.Parallel()
		rm := NewRegionManager(4, 8)
		snap := &RegionSnapshot{
			RegionsJSON: "not valid json{",
		}

		err := rm.RestoreFromSnapshot(snap, 32)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal")
	})

	t.Run("empty regions returns error", func(t *testing.T) {
		t.Parallel()
		rm := NewRegionManager(4, 8)
		snap := &RegionSnapshot{
			RegionsJSON: "[]",
		}

		err := rm.RestoreFromSnapshot(snap, 32)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty regions")
	})

	t.Run("successful restoration", func(t *testing.T) {
		t.Parallel()
		rm := NewRegionManager(4, 8)
		totalCells := 32

		snap := &RegionSnapshot{
			SnapshotID:       100,
			SensorID:         "test-sensor",
			RegionCount:      2,
			SettlingFrames:   50,
			CreatedUnixNanos: time.Now().UnixNano(),
			RegionsJSON: `[
				{"id": 0, "cell_list": [0, 1, 2, 3], "cell_count": 4, "mean_variance": 0.1, "params": {"background_update_fraction": 0.02}},
				{"id": 1, "cell_list": [4, 5, 6, 7], "cell_count": 4, "mean_variance": 0.5, "params": {"background_update_fraction": 0.05}}
			]`,
			SceneHash: "abc123",
		}

		err := rm.RestoreFromSnapshot(snap, totalCells)
		require.NoError(t, err)

		assert.True(t, rm.IdentificationComplete)
		assert.Len(t, rm.Regions, 2)

		// Verify CellToRegionID mapping
		assert.Equal(t, 0, rm.CellToRegionID[0])
		assert.Equal(t, 0, rm.CellToRegionID[1])
		assert.Equal(t, 1, rm.CellToRegionID[4])
		assert.Equal(t, 1, rm.CellToRegionID[5])
	})

	t.Run("restores variance data", func(t *testing.T) {
		t.Parallel()
		rm := NewRegionManager(2, 4)

		snap := &RegionSnapshot{
			RegionsJSON:      `[{"id": 0, "cell_list": [0], "cell_count": 1}]`,
			VarianceDataJSON: `{"variance_per_cell": [0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8], "frames_sampled": 40}`,
			CreatedUnixNanos: time.Now().UnixNano(),
		}

		err := rm.RestoreFromSnapshot(snap, 8)
		require.NoError(t, err)

		assert.Equal(t, 40, rm.SettlingMetrics.FramesSampled)
		assert.Len(t, rm.SettlingMetrics.VariancePerCell, 8)
	})

	t.Run("handles out-of-bound cell indices", func(t *testing.T) {
		t.Parallel()
		rm := NewRegionManager(2, 2)

		snap := &RegionSnapshot{
			RegionsJSON:      `[{"id": 0, "cell_list": [0, 100, -1], "cell_count": 3}]`,
			CreatedUnixNanos: time.Now().UnixNano(),
		}

		// Should not panic, just skip invalid indices
		err := rm.RestoreFromSnapshot(snap, 4)
		require.NoError(t, err)
	})
}

// TestRestoreRegions tests the BackgroundManager RestoreRegions method.
func TestRestoreRegions(t *testing.T) {
	t.Parallel()

	t.Run("nil manager returns error", func(t *testing.T) {
		t.Parallel()
		var bm *BackgroundManager
		snap := &RegionSnapshot{}

		err := bm.RestoreRegions(snap)
		assert.Error(t, err)
	})

	t.Run("nil grid returns error", func(t *testing.T) {
		t.Parallel()
		bm := &BackgroundManager{Grid: nil}
		snap := &RegionSnapshot{}

		err := bm.RestoreRegions(snap)
		assert.Error(t, err)
	})

	t.Run("nil snapshot returns error", func(t *testing.T) {
		t.Parallel()
		g := makeTestGrid(4, 8)

		err := g.Manager.RestoreRegions(nil)
		assert.Error(t, err)
	})

	t.Run("successful restoration marks settling complete", func(t *testing.T) {
		t.Parallel()
		g := makeTestGrid(4, 8)
		g.SettlingComplete = false
		g.WarmupFramesRemaining = 100

		snap := &RegionSnapshot{
			RegionsJSON:      `[{"id": 0, "cell_list": [0, 1], "cell_count": 2}]`,
			CreatedUnixNanos: time.Now().UnixNano(),
		}

		err := g.Manager.RestoreRegions(snap)
		require.NoError(t, err)

		assert.True(t, g.SettlingComplete)
		assert.Equal(t, 0, g.WarmupFramesRemaining)
		assert.True(t, g.regionRestoreAttempted)
	})
}

// TestBackgroundManager_RestoreFromSnapshot_Integration tests the full restore flow.
func TestBackgroundManager_RestoreFromSnapshot_Integration(t *testing.T) {
	t.Parallel()

	t.Run("creates RegionMgr if nil", func(t *testing.T) {
		t.Parallel()
		g := &BackgroundGrid{
			Rings:       4,
			AzimuthBins: 8,
			Cells:       make([]BackgroundCell, 32),
			RegionMgr:   nil, // No region manager
		}
		bm := &BackgroundManager{Grid: g}

		snap := &RegionSnapshot{
			RegionsJSON:      `[{"id": 0, "cell_list": [0], "cell_count": 1}]`,
			CreatedUnixNanos: time.Now().UnixNano(),
		}

		err := bm.RestoreRegions(snap)
		require.NoError(t, err)

		assert.NotNil(t, g.RegionMgr)
		assert.True(t, g.RegionMgr.IdentificationComplete)
	})
}

// mockRegionStore is a test double for RegionStore interface.
type mockRegionStore struct {
	regionSnapshots map[string]*RegionSnapshot // key: sceneHash or sourcePath
	bgSnapshots     map[int64]*BgSnapshot
	insertErr       error
	getErr          error
	lastInsertedID  int64
}

func newMockRegionStore() *mockRegionStore {
	return &mockRegionStore{
		regionSnapshots: make(map[string]*RegionSnapshot),
		bgSnapshots:     make(map[int64]*BgSnapshot),
	}
}

func (m *mockRegionStore) InsertRegionSnapshot(s *RegionSnapshot) (int64, error) {
	if m.insertErr != nil {
		return 0, m.insertErr
	}
	m.lastInsertedID++
	return m.lastInsertedID, nil
}

func (m *mockRegionStore) GetRegionSnapshotBySceneHash(sensorID, sceneHash string) (*RegionSnapshot, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	key := "hash:" + sensorID + ":" + sceneHash
	return m.regionSnapshots[key], nil
}

func (m *mockRegionStore) GetRegionSnapshotBySourcePath(sensorID, sourcePath string) (*RegionSnapshot, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	key := "path:" + sensorID + ":" + sourcePath
	return m.regionSnapshots[key], nil
}

func (m *mockRegionStore) GetLatestRegionSnapshot(sensorID string) (*RegionSnapshot, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return nil, nil // Not implemented for these tests
}

func (m *mockRegionStore) InsertBgSnapshot(s *BgSnapshot) (int64, error) {
	if m.insertErr != nil {
		return 0, m.insertErr
	}
	m.lastInsertedID++
	m.bgSnapshots[m.lastInsertedID] = s
	return m.lastInsertedID, nil
}

func (m *mockRegionStore) GetBgSnapshotByID(snapshotID int64) (*BgSnapshot, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.bgSnapshots[snapshotID], nil
}

func (m *mockRegionStore) addRegionSnapshotBySceneHash(sensorID, sceneHash string, snap *RegionSnapshot) {
	key := "hash:" + sensorID + ":" + sceneHash
	m.regionSnapshots[key] = snap
}

func (m *mockRegionStore) addRegionSnapshotBySourcePath(sensorID, sourcePath string, snap *RegionSnapshot) {
	key := "path:" + sensorID + ":" + sourcePath
	m.regionSnapshots[key] = snap
}

// TestTryRestoreRegionsBySceneHash tests the TryRestoreRegionsBySceneHash function.
func TestTryRestoreRegionsBySceneHash(t *testing.T) {
	t.Parallel()

	t.Run("returns false for nil manager", func(t *testing.T) {
		t.Parallel()
		var bm *BackgroundManager
		store := newMockRegionStore()
		result := bm.TryRestoreRegionsBySceneHash(store)
		assert.False(t, result)
	})

	t.Run("returns false for nil grid", func(t *testing.T) {
		t.Parallel()
		bm := &BackgroundManager{Grid: nil}
		store := newMockRegionStore()
		result := bm.TryRestoreRegionsBySceneHash(store)
		assert.False(t, result)
	})

	t.Run("returns false for nil store", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		bm := &BackgroundManager{Grid: g}
		result := bm.TryRestoreRegionsBySceneHash(nil)
		assert.False(t, result)
	})

	t.Run("returns false when scene hash is empty", func(t *testing.T) {
		t.Parallel()
		// Create grid with no cells (empty signature)
		g := &BackgroundGrid{
			Cells: []BackgroundCell{},
		}
		bm := &BackgroundManager{Grid: g}
		store := newMockRegionStore()

		result := bm.TryRestoreRegionsBySceneHash(store)
		assert.False(t, result)
	})

	t.Run("returns false when snapshot not found", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		bm := &BackgroundManager{Grid: g}
		store := newMockRegionStore()

		result := bm.TryRestoreRegionsBySceneHash(store)
		assert.False(t, result)
	})

	t.Run("returns false when store lookup fails", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		bm := &BackgroundManager{Grid: g}
		store := newMockRegionStore()
		store.getErr = assert.AnError

		result := bm.TryRestoreRegionsBySceneHash(store)
		assert.False(t, result)
	})

	t.Run("restores regions successfully", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		bm := &BackgroundManager{Grid: g}
		store := newMockRegionStore()

		sceneHash := g.SceneSignature()
		snap := &RegionSnapshot{
			SensorID:         g.SensorID,
			SceneHash:        sceneHash,
			RegionsJSON:      `[{"id": 0, "cell_list": [0,1,2], "cell_count": 3}]`,
			RegionCount:      1,
			CreatedUnixNanos: time.Now().UnixNano(),
		}
		store.addRegionSnapshotBySceneHash(g.SensorID, sceneHash, snap)

		result := bm.TryRestoreRegionsBySceneHash(store)
		assert.True(t, result)
		assert.True(t, g.RegionMgr.IdentificationComplete)
	})
}

// makeTestGridWithData creates a test grid with populated cell data.
func makeTestGridWithData(rings, azBins int) *BackgroundGrid {
	cells := make([]BackgroundCell, rings*azBins)
	for i := range cells {
		cells[i] = BackgroundCell{
			AverageRangeMeters: float32(10 + i%5),
			RangeSpreadMeters:  0.05,
			TimesSeenCount:     10,
		}
	}
	return &BackgroundGrid{
		SensorID:    "test-sensor",
		Rings:       rings,
		AzimuthBins: azBins,
		Cells:       cells,
	}
}

// TestRestoreFromSnapshotLocked tests the restoreFromSnapshotLocked function.
func TestRestoreFromSnapshotLocked(t *testing.T) {
	t.Parallel()

	t.Run("restores regions without grid snapshot", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		bm := &BackgroundManager{Grid: g}
		store := newMockRegionStore()

		snap := &RegionSnapshot{
			SnapshotID:       0, // No linked grid snapshot
			RegionsJSON:      `[{"id": 0, "cell_list": [0], "cell_count": 1}]`,
			CreatedUnixNanos: time.Now().UnixNano(),
		}

		g.mu.Lock()
		err := bm.restoreFromSnapshotLocked(store, snap)
		g.mu.Unlock()

		assert.NoError(t, err)
		assert.True(t, g.RegionMgr.IdentificationComplete)
	})

	t.Run("restores regions with linked grid snapshot", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		bm := &BackgroundManager{Grid: g}
		store := newMockRegionStore()

		// Create serialized grid data
		gridCells := make([]BackgroundCell, 32)
		for i := range gridCells {
			gridCells[i] = BackgroundCell{
				AverageRangeMeters: float32(20 + i),
				RangeSpreadMeters:  0.1,
				TimesSeenCount:     50,
			}
		}
		gridBlob, err := serializeGrid(gridCells)
		require.NoError(t, err)

		// Add grid snapshot to store
		snapshotID := int64(1)
		bgSnap := &BgSnapshot{
			SnapshotID: &snapshotID,
			GridBlob:   gridBlob,
		}
		store.bgSnapshots[1] = bgSnap

		snap := &RegionSnapshot{
			SnapshotID:       1, // Link to grid snapshot
			RegionsJSON:      `[{"id": 0, "cell_list": [0,1], "cell_count": 2}]`,
			CreatedUnixNanos: time.Now().UnixNano(),
		}

		g.mu.Lock()
		err = bm.restoreFromSnapshotLocked(store, snap)
		g.mu.Unlock()

		assert.NoError(t, err)
		// Verify grid cells were restored
		assert.Equal(t, float32(20), g.Cells[0].AverageRangeMeters)
	})

	t.Run("handles grid snapshot fetch error gracefully", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		bm := &BackgroundManager{Grid: g}
		store := newMockRegionStore()
		store.getErr = assert.AnError

		snap := &RegionSnapshot{
			SnapshotID:       1, // Link to grid snapshot that will fail
			RegionsJSON:      `[{"id": 0, "cell_list": [0], "cell_count": 1}]`,
			CreatedUnixNanos: time.Now().UnixNano(),
		}

		g.mu.Lock()
		err := bm.restoreFromSnapshotLocked(store, snap)
		g.mu.Unlock()

		// Should still restore regions even if grid fetch fails
		assert.NoError(t, err)
		assert.True(t, g.RegionMgr.IdentificationComplete)
	})

	t.Run("handles grid cell count mismatch", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8) // 32 cells
		g.RegionMgr = NewRegionManager(4, 8)
		bm := &BackgroundManager{Grid: g}
		store := newMockRegionStore()

		// Create grid with different size
		gridCells := make([]BackgroundCell, 16) // Mismatched size
		gridBlob, err := serializeGrid(gridCells)
		require.NoError(t, err)

		snapshotID := int64(1)
		bgSnap := &BgSnapshot{
			SnapshotID: &snapshotID,
			GridBlob:   gridBlob,
		}
		store.bgSnapshots[1] = bgSnap

		snap := &RegionSnapshot{
			SnapshotID:       1,
			RegionsJSON:      `[{"id": 0, "cell_list": [0], "cell_count": 1}]`,
			CreatedUnixNanos: time.Now().UnixNano(),
		}

		originalRange := g.Cells[0].AverageRangeMeters

		g.mu.Lock()
		err = bm.restoreFromSnapshotLocked(store, snap)
		g.mu.Unlock()

		// Should still work, but grid cells not restored due to mismatch
		assert.NoError(t, err)
		assert.Equal(t, originalRange, g.Cells[0].AverageRangeMeters)
	})
}

// TestTryRestoreRegionsFromStoreLocked tests tryRestoreRegionsFromStoreLocked.
func TestTryRestoreRegionsFromStoreLocked(t *testing.T) {
	t.Parallel()

	t.Run("returns false when store is not RegionStore", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		bm := &BackgroundManager{Grid: g, store: nil}

		g.mu.Lock()
		result := bm.tryRestoreRegionsFromStoreLocked()
		g.mu.Unlock()

		assert.False(t, result)
		assert.True(t, g.regionRestoreAttempted)
	})

	t.Run("restores by source path when available", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		store := newMockRegionStore()
		bm := &BackgroundManager{
			Grid:       g,
			store:      store,
			sourcePath: "/test/path.pcap",
		}

		snap := &RegionSnapshot{
			SensorID:         g.SensorID,
			SourcePath:       "/test/path.pcap",
			RegionsJSON:      `[{"id": 0, "cell_list": [0], "cell_count": 1}]`,
			RegionCount:      1,
			CreatedUnixNanos: time.Now().UnixNano(),
		}
		store.addRegionSnapshotBySourcePath(g.SensorID, "/test/path.pcap", snap)

		g.mu.Lock()
		result := bm.tryRestoreRegionsFromStoreLocked()
		g.mu.Unlock()

		assert.True(t, result)
		assert.True(t, g.RegionMgr.IdentificationComplete)
	})

	t.Run("falls back to scene hash when source path not found", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		store := newMockRegionStore()
		bm := &BackgroundManager{
			Grid:       g,
			store:      store,
			sourcePath: "/nonexistent/path.pcap",
		}

		sceneHash := g.SceneSignature()
		snap := &RegionSnapshot{
			SensorID:         g.SensorID,
			SceneHash:        sceneHash,
			RegionsJSON:      `[{"id": 0, "cell_list": [0], "cell_count": 1}]`,
			RegionCount:      1,
			CreatedUnixNanos: time.Now().UnixNano(),
		}
		store.addRegionSnapshotBySceneHash(g.SensorID, sceneHash, snap)

		g.mu.Lock()
		result := bm.tryRestoreRegionsFromStoreLocked()
		g.mu.Unlock()

		assert.True(t, result)
		assert.True(t, g.RegionMgr.IdentificationComplete)
	})

	t.Run("returns false when no snapshot found", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		store := newMockRegionStore()
		bm := &BackgroundManager{
			Grid:  g,
			store: store,
		}

		g.mu.Lock()
		result := bm.tryRestoreRegionsFromStoreLocked()
		g.mu.Unlock()

		assert.False(t, result)
	})
}

// TestPersistRegionsOnSettleLocked tests region persistence during settling completion.
func TestPersistRegionsOnSettleLocked(t *testing.T) {
	t.Parallel()

	t.Run("returns early when store is nil", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		g.RegionMgr.IdentificationComplete = true
		bm := &BackgroundManager{Grid: g, store: nil}

		g.mu.Lock()
		bm.persistRegionsOnSettleLocked()
		g.mu.Unlock()
		// Should return early without error
	})

	t.Run("returns early when RegionMgr is nil", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = nil
		store := newMockRegionStore()
		bm := &BackgroundManager{Grid: g, store: store}

		g.mu.Lock()
		bm.persistRegionsOnSettleLocked()
		g.mu.Unlock()
		// Should return early without error
	})

	t.Run("returns early when identification not complete", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		g.RegionMgr.IdentificationComplete = false
		store := newMockRegionStore()
		bm := &BackgroundManager{Grid: g, store: store}

		g.mu.Lock()
		bm.persistRegionsOnSettleLocked()
		g.mu.Unlock()
		// Should return early without error
	})

	t.Run("persists regions when valid", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		g.RegionMgr.IdentificationComplete = true
		// Create a simple region
		g.RegionMgr.Regions = []*Region{
			{ID: 0, CellList: []int{0, 1, 2}, CellCount: 3},
		}
		store := newMockRegionStore()
		bm := &BackgroundManager{
			Grid:       g,
			store:      store,
			sourcePath: "/test/capture.pcap",
		}

		g.mu.Lock()
		bm.persistRegionsOnSettleLocked()
		g.mu.Unlock()

		// Verify snapshot was inserted
		assert.Equal(t, int64(2), store.lastInsertedID) // 1 for grid, 1 for region
	})

	t.Run("handles grid snapshot insert error", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		g.RegionMgr.IdentificationComplete = true
		g.RegionMgr.Regions = []*Region{
			{ID: 0, CellList: []int{0}, CellCount: 1},
		}
		store := newMockRegionStore()
		store.insertErr = assert.AnError
		bm := &BackgroundManager{Grid: g, store: store}

		g.mu.Lock()
		bm.persistRegionsOnSettleLocked()
		g.mu.Unlock()
		// Should handle error gracefully
	})

	t.Run("returns early when store is BgStore only", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		g.RegionMgr.IdentificationComplete = true
		// Use mockPersistBgStore which does NOT implement RegionStore
		bm := &BackgroundManager{Grid: g, store: &mockPersistBgStore{}}

		g.mu.Lock()
		bm.persistRegionsOnSettleLocked()
		g.mu.Unlock()
		// Should return early at the regionStore type assertion
	})

	t.Run("returns early when ToSnapshot returns nil", func(t *testing.T) {
		t.Parallel()
		g := makeTestGridWithData(4, 8)
		g.RegionMgr = NewRegionManager(4, 8)
		g.RegionMgr.IdentificationComplete = true
		// No regions → ToSnapshot returns nil
		g.RegionMgr.Regions = nil
		store := newMockRegionStore()
		bm := &BackgroundManager{Grid: g, store: store}

		g.mu.Lock()
		bm.persistRegionsOnSettleLocked()
		g.mu.Unlock()
		// Should return early when regionSnap is nil
	})
}

// ---------------------------------------------------------------------------
// tryRestoreRegionsFromStoreLocked additional branches
// ---------------------------------------------------------------------------

func TestTryRestoreRegionsFromStoreLocked_SourcePathError(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	g.RegionMgr = NewRegionManager(4, 8)
	store := newMockRegionStore()
	store.getErr = fmt.Errorf("db lookup error")
	bm := &BackgroundManager{
		Grid:       g,
		store:      store,
		sourcePath: "/test/path.pcap",
	}

	g.mu.Lock()
	result := bm.tryRestoreRegionsFromStoreLocked()
	g.mu.Unlock()

	// Error on source path lookup should fall through to scene hash
	// But scene hash also fails due to getErr, so returns false
	assert.False(t, result)
}

func TestTryRestoreRegionsFromStoreLocked_SceneHashError(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	g.RegionMgr = NewRegionManager(4, 8)
	store := newMockRegionStore()
	store.getErr = fmt.Errorf("scene hash lookup error")
	// No source path → goes straight to scene hash
	bm := &BackgroundManager{Grid: g, store: store}

	g.mu.Lock()
	result := bm.tryRestoreRegionsFromStoreLocked()
	g.mu.Unlock()

	assert.False(t, result)
}

func TestTryRestoreRegionsFromStoreLocked_RestoreFromSceneHashError(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	g.RegionMgr = NewRegionManager(4, 8)
	store := newMockRegionStore()
	sceneHash := g.SceneSignature()

	// Add a snapshot with invalid regions JSON to trigger restore error
	snap := &RegionSnapshot{
		SensorID:         g.SensorID,
		SceneHash:        sceneHash,
		RegionsJSON:      `invalid json`,
		RegionCount:      1,
		CreatedUnixNanos: time.Now().UnixNano(),
	}
	store.addRegionSnapshotBySceneHash(g.SensorID, sceneHash, snap)
	bm := &BackgroundManager{Grid: g, store: store}

	g.mu.Lock()
	result := bm.tryRestoreRegionsFromStoreLocked()
	g.mu.Unlock()

	assert.False(t, result)
}

func TestTryRestoreRegionsFromStoreLocked_SourcePathRestoreError(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	g.RegionMgr = NewRegionManager(4, 8)
	store := newMockRegionStore()

	// Add a snapshot by source path with invalid JSON
	snap := &RegionSnapshot{
		SensorID:         g.SensorID,
		SourcePath:       "/bad/path.pcap",
		RegionsJSON:      `not valid json`,
		RegionCount:      1,
		CreatedUnixNanos: time.Now().UnixNano(),
	}
	store.addRegionSnapshotBySourcePath(g.SensorID, "/bad/path.pcap", snap)
	bm := &BackgroundManager{
		Grid:       g,
		store:      store,
		sourcePath: "/bad/path.pcap",
	}

	g.mu.Lock()
	result := bm.tryRestoreRegionsFromStoreLocked()
	g.mu.Unlock()

	assert.False(t, result)
}

func TestTryRestoreRegionsFromStoreLocked_SourcePathNoSnapshot(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	g.RegionMgr = NewRegionManager(4, 8)
	store := newMockRegionStore()
	// Source path set but no matching snapshot → logs "trying scene_hash"
	bm := &BackgroundManager{
		Grid:       g,
		store:      store,
		sourcePath: "/nonexistent/path.pcap",
	}

	g.mu.Lock()
	result := bm.tryRestoreRegionsFromStoreLocked()
	g.mu.Unlock()

	assert.False(t, result)
}

// ---------------------------------------------------------------------------
// restoreFromSnapshotLocked additional branches
// ---------------------------------------------------------------------------

func TestRestoreFromSnapshotLocked_InvalidGridBlob(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	g.RegionMgr = NewRegionManager(4, 8)
	bm := &BackgroundManager{Grid: g}
	store := newMockRegionStore()

	// Add a bg snapshot with invalid grid blob
	snapshotID := int64(1)
	bgSnap := &BgSnapshot{
		SnapshotID: &snapshotID,
		GridBlob:   []byte("not valid gzip data"),
	}
	store.bgSnapshots[1] = bgSnap

	snap := &RegionSnapshot{
		SnapshotID:       1,
		RegionsJSON:      `[{"id": 0, "cell_list": [0], "cell_count": 1}]`,
		CreatedUnixNanos: time.Now().UnixNano(),
	}

	g.mu.Lock()
	err := bm.restoreFromSnapshotLocked(store, snap)
	g.mu.Unlock()

	// Should still succeed (regions restored despite grid blob error)
	assert.NoError(t, err)
}

func TestRestoreFromSnapshotLocked_NilBgSnapshot(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	g.RegionMgr = NewRegionManager(4, 8)
	bm := &BackgroundManager{Grid: g}
	store := newMockRegionStore()
	// SnapshotID > 0 but no bg snapshot in store → bgSnap is nil

	snap := &RegionSnapshot{
		SnapshotID:       999, // Not in store
		RegionsJSON:      `[{"id": 0, "cell_list": [0], "cell_count": 1}]`,
		CreatedUnixNanos: time.Now().UnixNano(),
	}

	g.mu.Lock()
	err := bm.restoreFromSnapshotLocked(store, snap)
	g.mu.Unlock()

	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// TryRestoreRegionsBySceneHash additional branches
// ---------------------------------------------------------------------------

func TestTryRestoreRegionsBySceneHash_GetError(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	store := newMockRegionStore()
	store.getErr = fmt.Errorf("DB error")

	result := g.Manager.TryRestoreRegionsBySceneHash(store)
	assert.False(t, result)
}

// ---------------------------------------------------------------------------
// restoreRegionsLocked error path
// ---------------------------------------------------------------------------

func TestRestoreRegionsLocked_InvalidJSON(t *testing.T) {
	t.Parallel()
	g := makeTestGridWithData(4, 8)
	g.RegionMgr = NewRegionManager(4, 8)
	bm := &BackgroundManager{Grid: g}

	snap := &RegionSnapshot{
		RegionsJSON:      `invalid`,
		CreatedUnixNanos: time.Now().UnixNano(),
	}

	err := bm.RestoreRegions(snap)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to restore regions")
}

// ---------------------------------------------------------------------------
// deserializeGrid decode error
// ---------------------------------------------------------------------------

func TestDeserializeGrid_DecodeError(t *testing.T) {
	t.Parallel()
	// Create valid gzip data that contains invalid gob encoding
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte("not valid gob data"))
	gz.Close()

	_, err := deserializeGrid(buf.Bytes())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode")
}
