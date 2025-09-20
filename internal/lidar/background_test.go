package lidar

import (
	"testing"
	"time"
)

// helper to create a small grid for tests
func makeTestGrid(rings, azBins int) *BackgroundGrid {
	cells := make([]BackgroundCell, rings*azBins)
	params := BackgroundParams{
		BackgroundUpdateFraction:       0.5, // use large alpha for deterministic updates
		ClosenessSensitivityMultiplier: 2.0,
		// set a generous safety margin so single-frame initialization is accepted in tests
		SafetyMarginMeters:        20.0,
		FreezeDurationNanos:       int64(1 * time.Second),
		NeighborConfirmationCount: 2,
	}
	g := &BackgroundGrid{
		SensorID:    "test-sensor",
		SensorFrame: "sensor/test",
		Rings:       rings,
		AzimuthBins: azBins,
		Cells:       cells,
		Params:      params,
	}
	g.Manager = &BackgroundManager{Grid: g}
	return g
}

// Test that a single observation initializes a cell and updates AverageRangeMeters
func TestProcessFramePolar_InitialUpdate(t *testing.T) {
	g := makeTestGrid(2, 8)
	bm := g.Manager

	// single point in ring 1, az bin 0
	p := PointPolar{Channel: 1, Azimuth: 0.0, Distance: 10.0}
	bm.ProcessFramePolar([]PointPolar{p})

	idx := g.Idx(0, 0)
	cell := g.Cells[idx]
	if cell.TimesSeenCount == 0 {
		t.Fatalf("expected TimesSeenCount>0 after first observation")
	}
	if cell.AverageRangeMeters != float32(10.0) {
		t.Fatalf("expected average 10.0 got %v", cell.AverageRangeMeters)
	}
}

// Test that repeated consistent observations increase TimesSeenCount and EMA moves toward obs
func TestProcessFramePolar_ConsistentObservations(t *testing.T) {
	g := makeTestGrid(1, 4)
	bm := g.Manager

	// two consistent points at 5m in same bin
	pts := []PointPolar{
		{Channel: 1, Azimuth: 0.0, Distance: 5.0},
		{Channel: 1, Azimuth: 0.0, Distance: 5.0},
	}
	bm.ProcessFramePolar(pts)
	idx := g.Idx(0, 0)
	cell := g.Cells[idx]
	if cell.TimesSeenCount < 1 {
		t.Fatalf("expected TimesSeenCount>=1 got %d", cell.TimesSeenCount)
	}
	// run another frame with 7m observation; with alpha=0.5 new avg should be (5+7)/2=6
	bm.ProcessFramePolar([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 7.0}})
	cell = g.Cells[idx]
	if cell.AverageRangeMeters < 5.9 || cell.AverageRangeMeters > 6.1 {
		t.Fatalf("expected average approx 6.0 got %v", cell.AverageRangeMeters)
	}
}

// Test that a divergent observation decrements TimesSeenCount and can freeze the cell
func TestProcessFramePolar_DivergentFreeze(t *testing.T) {
	g := makeTestGrid(1, 4)
	bm := g.Manager
	idx := g.Idx(0, 0)

	// initialize cell with 5m
	bm.ProcessFramePolar([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 5.0}})
	// feed a very distant point to trigger divergence
	bm.ProcessFramePolar([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 100.0}})
	cell := g.Cells[idx]
	if cell.TimesSeenCount != 0 && cell.FrozenUntilUnixNanos == 0 {
		t.Fatalf("expected either timesSeen decremented or freeze set, got times=%d freeze=%d",
			cell.TimesSeenCount, cell.FrozenUntilUnixNanos)
	}
}

// Test neighbor confirmation: neighbors with similar ranges help classify as background
func TestProcessFramePolar_NeighborConfirmation(t *testing.T) {
	g := makeTestGrid(3, 3)
	bm := g.Manager

	// Initialize neighbors around center (ring1, az1) with 10m
	// compute azimuths that map to the intended az bins (use bin centers)
	azStep := 360.0 / float64(g.AzimuthBins)
	for dr := -1; dr <= 1; dr++ {
		for da := -1; da <= 1; da++ {
			if dr == 0 && da == 0 {
				continue
			}
			r := 1 + dr
			a := 1 + da
			az := (float64(a) + 0.5) * azStep
			bm.ProcessFramePolar([]PointPolar{{Channel: r + 1, Azimuth: az, Distance: 10.0}})
		}
	}

	// Now observe center with slightly different distance; neighbor confirmation should help
	centerAz := (float64(1) + 0.5) * azStep
	bm.ProcessFramePolar([]PointPolar{{Channel: 2, Azimuth: centerAz, Distance: 10.5}})
	centerIdx := g.Idx(1, 1)
	cell := g.Cells[centerIdx]
	if cell.TimesSeenCount == 0 {
		t.Fatalf("expected center to be updated due to neighbor confirmation")
	}
}

// Test edge cases: empty frames and out-of-range channels/azimuths are ignored
func TestProcessFramePolar_EdgeCases(t *testing.T) {
	g := makeTestGrid(2, 8)
	bm := g.Manager

	// empty frame should be a no-op
	bm.ProcessFramePolar([]PointPolar{})

	// out-of-range channel (0) and negative azimuth are ignored
	pts := []PointPolar{
		{Channel: 0, Azimuth: 10.0, Distance: 5.0},   // invalid channel
		{Channel: 1, Azimuth: -720.0, Distance: 6.0}, // normalized but valid
		{Channel: 99, Azimuth: 30.0, Distance: 4.0},  // invalid channel
	}
	bm.ProcessFramePolar(pts)

	// Only the normalized azimuth point (Channel 1) should have updated
	idx := g.Idx(0, 0) // azimuth 0 maps to bin 0 with our az= -720 normalized -> 0
	cell := g.Cells[idx]
	if cell.TimesSeenCount == 0 {
		t.Fatalf("expected at least one valid update from normalized azimuth")
	}
}

// Test snapshot/persistence: simulate PersistCallback and ensure snapshots are created
func TestBackgroundSnapshotPersistence(t *testing.T) {
	g := makeTestGrid(1, 4)
	// tune params to make snapshotting deterministic
	g.Params.ChangeThresholdForSnapshot = 1
	g.Params.SettlingPeriodNanos = int64(0) // no settling wait
	g.Manager = &BackgroundManager{Grid: g}

	var captured *BgSnapshot
	g.Manager.PersistCallback = func(s *BgSnapshot) error {
		// copy snapshot for assertions
		snap := *s
		captured = &snap
		return nil
	}

	// simulate a frame that changes one cell
	bm := g.Manager
	bm.ProcessFramePolar([]PointPolar{{Channel: 1, Azimuth: 0.0, Distance: 7.0}})

	// When ChangesSinceSnapshot >= threshold, call the persist callback as main app would
	if g.ChangesSinceSnapshot >= g.Params.ChangeThresholdForSnapshot {
		// build a minimal BgSnapshot and call PersistCallback
		snap := &BgSnapshot{
			SensorID:          g.SensorID,
			TakenUnixNanos:    time.Now().UnixNano(),
			Rings:             g.Rings,
			AzimuthBins:       g.AzimuthBins,
			ParamsJSON:        "{}",
			GridBlob:          []byte("fakeblob"),
			ChangedCellsCount: g.ChangesSinceSnapshot,
			SnapshotReason:    "test-trigger",
		}
		if err := g.Manager.PersistCallback(snap); err != nil {
			t.Fatalf("persist callback failed: %v", err)
		}
	}

	if captured == nil {
		t.Fatalf("expected persist callback to be invoked and capture snapshot")
	}
	if captured.ChangedCellsCount == 0 {
		t.Fatalf("expected ChangedCellsCount>0 in captured snapshot")
	}
}
