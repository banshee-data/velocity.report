package l3grid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetFrameSettlingMetrics_NilAndEmpty(t *testing.T) {
	t.Parallel()
	var bm *BackgroundManager
	assert.Equal(t, FrameSettlingMetrics{}, bm.GetFrameSettlingMetrics(10))

	g := makeTestGrid(2, 4)
	m := g.Manager.GetFrameSettlingMetrics(10)
	assert.Equal(t, 8, m.TotalCells)
	assert.Equal(t, 0, m.NonzeroCells)
	assert.Equal(t, 0, m.SettledCells)
	assert.Equal(t, 0.0, m.PercentSettled)
}

func TestGetFrameSettlingMetrics_CountsAndConsistency(t *testing.T) {
	t.Parallel()
	rings, azBins := 4, 8
	g := makeTestGrid(rings, azBins)
	bm := g.Manager
	total := rings * azBins // 32
	future := time.Now().Add(time.Hour).UnixNano()

	// 16 nonzero cells; of those, the first 10 are settled (>= threshold 20);
	// the first 4 are frozen.
	for i := range 16 {
		g.Cells[i].TimesSeenCount = 5
		g.Cells[i].AverageRangeMeters = 10
	}
	for i := range 10 {
		g.Cells[i].TimesSeenCount = 25
	}
	for i := range 4 {
		g.Cells[i].FrozenUntilUnixNanos = future
	}
	g.ForegroundCount = 123
	g.BackgroundCount = 456

	const threshold = 20
	m := bm.GetFrameSettlingMetrics(threshold)

	assert.Equal(t, total, m.TotalCells)
	assert.Equal(t, 16, m.NonzeroCells)
	assert.Equal(t, 10, m.SettledCells)
	assert.Equal(t, 4, m.FrozenCells)
	assert.Equal(t, int64(123), m.ForegroundCount)
	assert.Equal(t, int64(456), m.BackgroundCount)
	assert.InDelta(t, 16.0/32.0, m.PercentNonzero, 1e-9)
	assert.InDelta(t, 10.0/32.0, m.PercentSettled, 1e-9)
	assert.InDelta(t, 4.0/32.0, m.PercentFrozen, 1e-9)

	// Consistency with GridStatus.
	gs := bm.GridStatus()
	assert.Equal(t, m.FrozenCells, gs["frozen_cells"].(int))
	assert.Equal(t, m.ForegroundCount, gs["foreground_count"].(int64))
	assert.Equal(t, m.BackgroundCount, gs["background_count"].(int64))
	assert.Equal(t, m.TotalCells, gs["total_cells"].(int))

	// Consistency with GetGridHeatmap (one bucket per ring covers all cells).
	hm := bm.GetGridHeatmap(360.0, threshold)
	if assert.NotNil(t, hm) {
		assert.Equal(t, m.NonzeroCells, hm.Summary["total_filled"].(int))
		assert.Equal(t, m.SettledCells, hm.Summary["total_settled"].(int))
		assert.Equal(t, m.FrozenCells, hm.Summary["total_frozen"].(int))
	}
}

func TestNoiseBounds_CleanVsNoisy(t *testing.T) {
	t.Parallel()
	g := makeTestGrid(2, 8)
	bm := g.Manager
	// makeTestGrid sets NoiseRelativeFraction=0.01, so the expected floor at
	// range 10 m is 0.01*10 + 0.01 = 0.11 m.
	for i := range g.Cells {
		g.Cells[i].TimesSeenCount = 30
		g.Cells[i].AverageRangeMeters = 10
		g.Cells[i].RangeSpreadMeters = 0.05 // below floor -> within bounds
	}
	dev := bm.GetNoiseBoundsDeviation()
	assert.Greater(t, dev, 0.0)
	assert.Less(t, dev, 1.0, "clean spread should sit below the expected floor")
	assert.True(t, bm.IsWithinNoiseBounds(2.0))

	// Make every cell very noisy (spread 1.0 m >> floor 0.11 m).
	for i := range g.Cells {
		g.Cells[i].RangeSpreadMeters = 1.0
	}
	assert.Greater(t, bm.GetNoiseBoundsDeviation(), 1.0)
	assert.False(t, bm.IsWithinNoiseBounds(2.0))
}

func TestNoiseBounds_NilAndEmpty(t *testing.T) {
	t.Parallel()
	var bm *BackgroundManager
	assert.Equal(t, 0.0, bm.GetNoiseBoundsDeviation())
	assert.True(t, bm.IsWithinNoiseBounds(2.0), "nil manager is vacuously within bounds")

	g := makeTestGrid(2, 4) // no observed cells
	assert.Equal(t, 0.0, g.Manager.GetNoiseBoundsDeviation())
	assert.True(t, g.Manager.IsWithinNoiseBounds(2.0))
}
