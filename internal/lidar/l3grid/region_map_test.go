package l3grid

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingRegionStore keeps every snapshot PersistAt writes.
type capturingRegionStore struct {
	*mockRegionStore
	regions []*RegionSnapshot
}

func (s *capturingRegionStore) InsertRegionSnapshot(r *RegionSnapshot) (int64, error) {
	s.regions = append(s.regions, r)
	return s.mockRegionStore.InsertRegionSnapshot(r)
}

// regionMapFixture is a two-ring, four-bin grid (90 degree bins) with ring
// elevations 0 and 30 degrees. Ring 0 settled quiet, ring 1 settled with
// rising spread, and cell 3 was never seen. Regions are identified with no
// merge, so each connected run of one tercile is its own region.
func regionMapFixture(t *testing.T) *BackgroundManager {
	t.Helper()
	bm := NewBackgroundManagerDI("map-test", 2, 4, BackgroundParams{NoiseRelativeFraction: 0.01}, nil)
	require.NotNil(t, bm)
	require.NoError(t, bm.SetRingElevations([]float64{0, 30}))
	g := bm.Grid
	ranges := []float32{10, 20, 30, 0, 10, 10, 10, 10}
	for i, r := range ranges {
		if r > 0 {
			g.Cells[i] = BackgroundCell{AverageRangeMeters: r, TimesSeenCount: 50, LockedBaseline: r + 0.5, RangeSpreadMeters: 0.1}
		}
	}
	rm := g.RegionMgr
	copy(rm.SettlingMetrics.VariancePerCell, []float64{0.01, 0.01, 0.01, 0, 0.5, 0.5, 2.0, 2.0})
	rm.SettlingMetrics.FramesSampled = 12
	require.NoError(t, rm.IdentifyRegions(g, 50))
	return bm
}

func TestIdentifyRegionsRecordsTerciles(t *testing.T) {
	t.Parallel()
	bm := regionMapFixture(t)
	rm := bm.Grid.RegionMgr

	// Seven seen cells sorted: 0.01 x3, 0.5 x2, 2.0 x2. The cut points are
	// the values at indices 7/3 = 2 and 14/3 = 4.
	assert.Equal(t, []float64{0.01, 0.5}, rm.TercileThresholds)
	assert.Equal(t, []VarianceTercile{
		TercileVariable, TercileVariable, TercileVariable, TercileNone,
		TercileVolatile, TercileVolatile, TercileVolatile, TercileVolatile,
	}, rm.CellTercile, "a cell at a cut point falls in the tercile above it")
	for _, r := range rm.Regions {
		assert.Equal(t, r.Volatile, r.Tercile == TercileVolatile, "region %d", r.ID)
		assert.NotEqual(t, TercileNone, r.Tercile, "region %d", r.ID)
	}

	snap := rm.ToSnapshot("map-test", 0)
	require.NotNil(t, snap)
	var regions []RegionData
	require.NoError(t, json.Unmarshal([]byte(snap.RegionsJSON), &regions))
	for i, r := range rm.Regions {
		assert.Equal(t, r.Tercile, regions[i].Tercile)
	}
	var vd VarianceData
	require.NoError(t, json.Unmarshal([]byte(snap.VarianceDataJSON), &vd))
	assert.Equal(t, rm.CellTercile, vd.CellTercile)
	assert.Equal(t, rm.TercileThresholds, vd.TercileThresholds)

	// A restore must not carry terciles it did not compute.
	restored := NewRegionManager(2, 4)
	restored.CellTercile = []VarianceTercile{TercileStable}
	require.NoError(t, restored.RestoreFromSnapshot(snap, 8))
	assert.Nil(t, restored.CellTercile)
	assert.Nil(t, restored.TercileThresholds)
	for _, r := range restored.Regions {
		assert.Equal(t, TercileNone, r.Tercile)
		assert.False(t, r.Volatile, "restoring the tercile is out of scope; volatility stays unrestored")
	}
}

func TestPersistAtWritesRegionMapAtGivenTime(t *testing.T) {
	t.Parallel()
	bm := regionMapFixture(t)
	store := &capturingRegionStore{mockRegionStore: newMockRegionStore()}
	at := time.Unix(1700000000, 123456789)

	require.NoError(t, bm.PersistAt(store, SnapshotReasonRegionsIdentified, at))
	require.Len(t, store.regions, 1)
	regions := store.regions[0]
	bg := store.bgSnapshots[regions.SnapshotID]
	require.NotNil(t, bg)
	assert.Equal(t, at.UnixNano(), bg.TakenUnixNanos)
	assert.Equal(t, at.UnixNano(), regions.CreatedUnixNanos, "both rows carry the given time, not the wall clock")
	assert.Equal(t, SnapshotReasonRegionsIdentified, bg.SnapshotReason)

	id := regions.SnapshotID
	bg.SnapshotID = &id
	m, err := BuildRegionMap(bg, regions)
	require.NoError(t, err)
	assert.Equal(t, 2, m.Rings)
	assert.Equal(t, 4, m.AzimuthBins)
	assert.Equal(t, len(bm.Grid.RegionMgr.Regions), m.RegionCount)
	assert.Equal(t, 12, m.SettlingFrames)
	require.Len(t, m.Cells, 8)

	// Cell 1: ring 0 (elevation 0), bin 1, whose centre is 135 degrees.
	c := m.Cells[1]
	assert.Equal(t, 0, c.Ring)
	assert.Equal(t, 1, c.AzimuthBin)
	assert.InDelta(t, 0, c.ElevationDeg, 1e-12)
	assert.InDelta(t, 135, c.AzimuthDeg, 1e-12)
	assert.Equal(t, float32(20), c.SettledRangeM)
	assert.Equal(t, float32(20.5), c.LockedBaselineM)
	assert.InDelta(t, 20*math.Sin(135*math.Pi/180), c.XM, 1e-9)
	assert.InDelta(t, 20*math.Cos(135*math.Pi/180), c.YM, 1e-9)
	assert.InDelta(t, 0, c.ZM, 1e-9)
	assert.Equal(t, "variable", c.CellTercile)
	assert.InDelta(t, 0.01, c.SettlingVariance, 1e-12)

	// Cell 4: ring 1 (elevation 30), bin 0, centre 45 degrees, range 10.
	c = m.Cells[4]
	assert.Equal(t, 1, c.Ring)
	assert.InDelta(t, 30, c.ElevationDeg, 1e-12)
	assert.InDelta(t, 45, c.AzimuthDeg, 1e-12)
	horizontal := 10 * math.Cos(30*math.Pi/180)
	assert.InDelta(t, horizontal*math.Sin(45*math.Pi/180), c.XM, 1e-9)
	assert.InDelta(t, horizontal*math.Cos(45*math.Pi/180), c.YM, 1e-9)
	assert.InDelta(t, 5, c.ZM, 1e-6)
	assert.Equal(t, "volatile", c.CellTercile)
	assert.Equal(t, "volatile", c.RegionTercile)
	assert.True(t, c.RegionVolatile)
	assert.Equal(t, uint32(50), c.TimesSeen)

	// Cell 3 was never seen: no region, no tercile, at the origin.
	c = m.Cells[3]
	assert.Equal(t, -1, c.RegionID)
	assert.Equal(t, "none", c.RegionTercile)
	assert.Equal(t, "none", c.CellTercile)
	assert.Zero(t, c.TimesSeen)
	assert.Zero(t, c.XM)

	// Every assigned cell carries its region's tercile and parameters.
	for _, r := range bm.Grid.RegionMgr.Regions {
		for _, idx := range r.CellList {
			assert.Equal(t, r.ID, m.Cells[idx].RegionID)
			assert.Equal(t, r.Tercile.String(), m.Cells[idx].RegionTercile)
			assert.Equal(t, r.Params.NoiseRelativeFraction, m.Cells[idx].RegionNoiseRelativeFraction)
		}
	}
}

func TestBuildRegionMapRefusesUnplaceableOrMismatchedPairs(t *testing.T) {
	t.Parallel()
	bm := regionMapFixture(t)
	store := &capturingRegionStore{mockRegionStore: newMockRegionStore()}
	require.NoError(t, bm.PersistAt(store, SnapshotReasonReplayEnd, time.Unix(1, 0)))
	regions := store.regions[0]
	bg := *store.bgSnapshots[regions.SnapshotID]

	other := regions.SnapshotID + 1
	mismatched := bg
	mismatched.SnapshotID = &other
	_, err := BuildRegionMap(&mismatched, regions)
	assert.ErrorContains(t, err, "links grid snapshot")

	noElevations := bg
	noElevations.RingElevationsJSON = ""
	_, err = BuildRegionMap(&noElevations, regions)
	assert.ErrorContains(t, err, "no ring elevations")

	noVariance := *regions
	noVariance.VarianceDataJSON = ""
	_, err = BuildRegionMap(&bg, &noVariance)
	assert.ErrorContains(t, err, "no settling variance")

	_, err = BuildRegionMap(nil, regions)
	assert.Error(t, err)
}
