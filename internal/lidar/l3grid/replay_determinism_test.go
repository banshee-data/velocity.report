package l3grid

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sceneFrame is one revolution of a 4-ring, 8-bin scene: every cell at base
// metres, jittered by frame so each cell's spread keeps changing and the
// convergence check only passes when its spread delta is not measured. The
// jitter grows with the base, so two scenes settle to different spreads.
func sceneFrame(base float64, frame int) []PointPolar {
	pts := make([]PointPolar, 0, 32)
	for ring := 1; ring <= 4; ring++ {
		for bin := 0; bin < 8; bin++ {
			jitter := 0.005 * base * base * math.Sin(float64(frame*7+ring*3+bin))
			pts = append(pts, PointPolar{Channel: ring, Azimuth: 22.5 + 45*float64(bin), Distance: base + jitter})
		}
	}
	return pts
}

// settleFrame feeds frames of a scene until the grid settles, and returns the
// frame (1-based) it settled on, or 0 if it had not by limit.
func settleFrame(t *testing.T, bm *BackgroundManager, base float64, start time.Time, limit int) int {
	t.Helper()
	for f := 1; f <= limit; f++ {
		if _, err := bm.ProcessFramePolarWithMaskAt(sceneFrame(base, f), start.Add(time.Duration(f)*100*time.Millisecond)); err != nil {
			t.Fatalf("frame %d: %v", f, err)
		}
		if bm.IsSettlingComplete() {
			return f
		}
	}
	return 0
}

func determinismParams(honest bool) BackgroundParams {
	return BackgroundParams{
		SeedFromFirstObservation:   true,
		WarmupMinFrames:            3,
		WarmupDurationNanos:        int64(time.Hour), // the ceiling is never reached
		SettlingCheckInterval:      1,
		SettlingMinCoverage:        0.5,
		SettlingMaxSpreadDelta:     1e-6, // only an unmeasured delta passes
		SettlingMinRegionStability: 0.01,
		SettlingMinConfidence:      1,
		SettlingHonestFirstCheck:   honest,
	}
}

func TestResetGridClearsTheConvergenceState(t *testing.T) {
	bm := NewBackgroundManagerDI(t.Name(), 4, 8, determinismParams(false), nil)
	g := bm.Grid
	g.prevSpreads = make([]float32, len(g.Cells))
	g.prevRegionIDs = make([]int, len(g.Cells))
	g.settlingCheckCounter = 7
	g.settlingAnnounced = true
	g.LastObservedNoiseRel = 0.02

	require.NoError(t, bm.ResetGrid())

	assert.Nil(t, g.prevSpreads)
	assert.Nil(t, g.prevRegionIDs)
	assert.Zero(t, g.settlingCheckCounter)
	assert.False(t, g.settlingAnnounced)
	assert.Zero(t, g.LastObservedNoiseRel)
}

// A manager that settled on one scene and was reset settles on the next scene
// exactly as a new manager does, under either first-check semantics. Before
// ResetGrid cleared the convergence state, the reused manager's first check
// compared the new scene's spreads with the old scene's and failed, so it
// settled later than a new one (l3bg0020).
func TestAResetManagerSettlesAsANewOne(t *testing.T) {
	start := time.Unix(1_800_000_000, 0)
	for _, honest := range []bool{false, true} {
		name := map[bool]string{false: "first-check", true: "honest"}[honest]
		t.Run(name, func(t *testing.T) {
			fresh := NewBackgroundManagerDI(t.Name()+"-fresh", 4, 8, determinismParams(honest), nil)
			want := settleFrame(t, fresh, 20, start, 50)
			if honest {
				// Honest never passes a check whose spread keeps moving against
				// a 1e-6 threshold, so both settle at the ceiling instead.
				fresh = NewBackgroundManagerDI(t.Name()+"-fresh-ceiling", 4, 8, determinismParams(honest), nil)
				fresh.Grid.Params.WarmupDurationNanos = int64(2 * time.Second)
				want = settleFrame(t, fresh, 20, start, 50)
			}
			require.NotZero(t, want, "a new manager never settled")

			reused := NewBackgroundManagerDI(t.Name()+"-reused", 4, 8, determinismParams(honest), nil)
			if honest {
				// Honest never settles on a scene whose spread keeps moving
				// against a 1e-6 threshold; give the first scene a ceiling.
				reused.Grid.Params.WarmupDurationNanos = int64(time.Second)
			}
			if settleFrame(t, reused, 10, start, 50) == 0 {
				t.Fatal("the first scene never settled")
			}
			reused.Grid.Params.WarmupDurationNanos = fresh.Grid.Params.WarmupDurationNanos
			require.NoError(t, reused.ResetGrid())
			got := settleFrame(t, reused, 20, start.Add(time.Hour), 50)

			assert.Equal(t, want, got, "a reset manager settled on frame %d, a new one on frame %d", got, want)
		})
	}
}

// Under the default semantics the first check passes on its unmeasured delta;
// honest semantics fails it and passes the next check that measures a delta
// under the threshold.
func TestHonestFirstCheckWaitsForAMeasuredDelta(t *testing.T) {
	for _, honest := range []bool{false, true} {
		bm := newSettlingGrid(t, armedThresholds(BackgroundParams{
			SeedFromFirstObservation: true,
			WarmupMinFrames:          1,
			WarmupDurationNanos:      int64(time.Hour),
			SettlingCheckInterval:    1,
			SettlingHonestFirstCheck: honest,
		}))
		bm.Grid.mu.Lock()
		seedAllCells(bm.Grid)
		first := bm.Grid.settlingConvergedLocked()
		second := bm.Grid.settlingConvergedLocked()
		bm.Grid.mu.Unlock()

		assert.Equal(t, !honest, first, "honest=%v: first check", honest)
		assert.True(t, second, "honest=%v: second check, spreads unchanged", honest)
	}
}

func TestRegionStoreUse(t *testing.T) {
	path := "/captures/site.pcapng"
	snapshot := func(g *BackgroundGrid) *RegionSnapshot {
		return &RegionSnapshot{
			SensorID:    g.SensorID,
			RegionCount: 1,
			RegionsJSON: `[{"id":0,"cell_list":[0,1],"cell_count":2}]`,
			SourcePath:  path,
		}
	}

	t.Run("live restores a snapshot of its source", func(t *testing.T) {
		store := newMockRegionStore()
		g := makeTestGrid(4, 8)
		g.Manager.store = store
		g.Manager.SetSourcePath(path)
		store.addRegionSnapshotBySourcePath(g.SensorID, path, snapshot(g))
		restore, persist := g.Manager.RegionStoreUse()
		require.True(t, restore && persist, "a new manager uses the store both ways")

		g.mu.Lock()
		restored := g.Manager.tryRestoreRegionsFromStoreLocked()
		g.mu.Unlock()
		assert.True(t, restored)
		assert.True(t, g.SettlingComplete)
	})

	t.Run("a replay restores none", func(t *testing.T) {
		store := newMockRegionStore()
		g := makeTestGrid(4, 8)
		g.Manager.store = store
		g.Manager.SetSourcePath(path)
		store.addRegionSnapshotBySourcePath(g.SensorID, path, snapshot(g))
		g.Manager.SetRegionStoreUse(false, false)

		g.mu.Lock()
		restored := g.Manager.tryRestoreRegionsFromStoreLocked()
		g.mu.Unlock()
		assert.False(t, restored)
		assert.False(t, g.SettlingComplete)
		assert.True(t, g.regionRestoreAttempted, "the attempt is spent, so it is not retried each frame")
	})

	for _, tc := range []struct {
		name    string
		persist bool
		want    int64
	}{{"a replay persists none", false, 0}, {"a settling pass persists its own", true, 2}} {
		t.Run(tc.name, func(t *testing.T) {
			g := makeTestGridWithData(4, 8)
			g.RegionMgr = NewRegionManager(4, 8)
			g.RegionMgr.IdentificationComplete = true
			g.RegionMgr.Regions = []*Region{{ID: 0, CellList: []int{0, 1, 2}, CellCount: 3}}
			store := newMockRegionStore()
			bm := &BackgroundManager{Grid: g, store: store, sourcePath: path}
			bm.SetRegionStoreUse(false, tc.persist)

			g.mu.Lock()
			bm.persistRegionsOnSettleLocked()
			g.mu.Unlock()
			assert.Equal(t, tc.want, store.lastInsertedID)
		})
	}
}
