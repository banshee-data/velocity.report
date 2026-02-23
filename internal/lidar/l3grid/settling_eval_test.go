package l3grid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettlingMetrics_IsConverged(t *testing.T) {
	t.Parallel()
	thresholds := DefaultSettlingThresholds()

	tests := []struct {
		name    string
		metrics SettlingMetrics
		want    bool
	}{
		{
			name: "all thresholds met",
			metrics: SettlingMetrics{
				CoverageRate:    0.90,
				SpreadDeltaRate: 0.0005,
				RegionStability: 0.98,
				MeanConfidence:  15.0,
			},
			want: true,
		},
		{
			name: "coverage below threshold",
			metrics: SettlingMetrics{
				CoverageRate:    0.50,
				SpreadDeltaRate: 0.0005,
				RegionStability: 0.98,
				MeanConfidence:  15.0,
			},
			want: false,
		},
		{
			name: "spread delta too high",
			metrics: SettlingMetrics{
				CoverageRate:    0.90,
				SpreadDeltaRate: 0.01,
				RegionStability: 0.98,
				MeanConfidence:  15.0,
			},
			want: false,
		},
		{
			name: "region stability too low",
			metrics: SettlingMetrics{
				CoverageRate:    0.90,
				SpreadDeltaRate: 0.0005,
				RegionStability: 0.50,
				MeanConfidence:  15.0,
			},
			want: false,
		},
		{
			name: "confidence too low",
			metrics: SettlingMetrics{
				CoverageRate:    0.90,
				SpreadDeltaRate: 0.0005,
				RegionStability: 0.98,
				MeanConfidence:  5.0,
			},
			want: false,
		},
		{
			name: "all exactly at threshold",
			metrics: SettlingMetrics{
				CoverageRate:    0.80,
				SpreadDeltaRate: 0.001,
				RegionStability: 0.95,
				MeanConfidence:  10.0,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.metrics.IsConverged(thresholds)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultSettlingThresholds(t *testing.T) {
	t.Parallel()
	th := DefaultSettlingThresholds()
	assert.Equal(t, 0.80, th.MinCoverage)
	assert.Equal(t, 0.001, th.MaxSpreadDelta)
	assert.Equal(t, 0.95, th.MinRegionStability)
	assert.Equal(t, 10.0, th.MinConfidence)
}

func TestEvaluateSettling_NilManager(t *testing.T) {
	t.Parallel()
	var bm *BackgroundManager
	m := bm.EvaluateSettling(0)
	assert.Equal(t, 0, m.FrameNumber)
	assert.Equal(t, 0.0, m.CoverageRate)
}

func TestEvaluateSettling_EmptyGrid(t *testing.T) {
	t.Parallel()
	g := makeTestGrid(4, 8)
	bm := g.Manager

	m := bm.EvaluateSettling(1)
	assert.Equal(t, 1, m.FrameNumber)
	assert.Equal(t, 0.0, m.CoverageRate)
	assert.Equal(t, 0.0, m.MeanConfidence)
}

func TestEvaluateSettling_CoverageAndConfidence(t *testing.T) {
	t.Parallel()
	rings, azBins := 2, 4
	g := makeTestGrid(rings, azBins)
	bm := g.Manager

	// Seed half the cells with observations
	total := rings * azBins
	for i := 0; i < total/2; i++ {
		g.Cells[i].TimesSeenCount = 20
		g.Cells[i].AverageRangeMeters = 10.0
	}

	m := bm.EvaluateSettling(5)
	assert.InDelta(t, 0.5, m.CoverageRate, 1e-9)
	assert.InDelta(t, 20.0, m.MeanConfidence, 1e-9)
	assert.Equal(t, 5, m.FrameNumber)
}

func TestEvaluateSettling_SpreadDeltaRate(t *testing.T) {
	t.Parallel()
	rings, azBins := 1, 4
	g := makeTestGrid(rings, azBins)
	bm := g.Manager

	// Mark all cells as observed
	for i := range g.Cells {
		g.Cells[i].TimesSeenCount = 10
		g.Cells[i].RangeSpreadMeters = 0.10
	}

	// First call: no previous data → delta is 0
	m1 := bm.EvaluateSettling(1)
	assert.Equal(t, 0.0, m1.SpreadDeltaRate)

	// Change spread on two of four cells
	g.Cells[0].RangeSpreadMeters = 0.12 // delta = 0.02
	g.Cells[1].RangeSpreadMeters = 0.08 // delta = 0.02

	m2 := bm.EvaluateSettling(2)
	// Expected delta = (0.02 + 0.02 + 0 + 0) / 4 = 0.01
	assert.InDelta(t, 0.01, m2.SpreadDeltaRate, 1e-6)
}

func TestEvaluateSettling_RegionStability(t *testing.T) {
	t.Parallel()
	rings, azBins := 1, 4
	g := makeTestGrid(rings, azBins)
	bm := g.Manager

	// Set initial region IDs
	require.Equal(t, 4, len(g.RegionMgr.CellToRegionID))
	g.RegionMgr.CellToRegionID = []int{0, 0, 1, 1}

	// First call: sets baseline → stability defaults to 1.0
	m1 := bm.EvaluateSettling(1)
	assert.Equal(t, 1.0, m1.RegionStability)

	// Change one cell's region
	g.RegionMgr.CellToRegionID[0] = 1 // was 0
	m2 := bm.EvaluateSettling(2)
	// 1 of 4 changed → stability = 1 − 0.25 = 0.75
	assert.InDelta(t, 0.75, m2.RegionStability, 1e-9)
}

func TestEvaluateSettling_TimestampSet(t *testing.T) {
	t.Parallel()
	g := makeTestGrid(1, 1)
	bm := g.Manager

	before := time.Now()
	m := bm.EvaluateSettling(0)
	after := time.Now()
	assert.True(t, !m.EvaluatedAt.Before(before), "EvaluatedAt should be on or after before")
	assert.True(t, !m.EvaluatedAt.After(after), "EvaluatedAt should be on or before after")
}
