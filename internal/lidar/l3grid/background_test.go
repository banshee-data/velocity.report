package l3grid

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
		NoiseRelativeFraction:     0.01,
	}
	g := &BackgroundGrid{
		SensorID:    "test-sensor",
		SensorFrame: "sensor/test",
		Rings:       rings,
		AzimuthBins: azBins,
		Cells:       cells,
		Params:      params,
		RegionMgr:   NewRegionManager(rings, azBins), // Initialize RegionManager for tests
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

	// Initialise neighbors around center (ring1, az1) with 10m
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

// TestBackgroundParams_HasDebugRange tests the HasDebugRange method
func TestBackgroundParams_HasDebugRange(t *testing.T) {
	tests := []struct {
		name     string
		params   BackgroundParams
		expected bool
	}{
		{"empty params", BackgroundParams{}, false},
		{"ring min set", BackgroundParams{DebugRingMin: 1}, true},
		{"ring max set", BackgroundParams{DebugRingMax: 5}, true},
		{"az min set", BackgroundParams{DebugAzMin: 10.0}, true},
		{"az max set", BackgroundParams{DebugAzMax: 90.0}, true},
		{"all set", BackgroundParams{DebugRingMin: 1, DebugRingMax: 5, DebugAzMin: 10.0, DebugAzMax: 90.0}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.params.HasDebugRange()
			if result != tt.expected {
				t.Errorf("HasDebugRange() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestBackgroundParams_IsInDebugRange tests the IsInDebugRange method
func TestBackgroundParams_IsInDebugRange(t *testing.T) {
	tests := []struct {
		name     string
		params   BackgroundParams
		ring     int
		az       float64
		expected bool
	}{
		{"no range set", BackgroundParams{}, 5, 45.0, false},
		{"ring in range", BackgroundParams{DebugRingMin: 1, DebugRingMax: 10}, 5, 45.0, true},
		{"ring below min", BackgroundParams{DebugRingMin: 5, DebugRingMax: 10}, 3, 45.0, false},
		{"ring above max", BackgroundParams{DebugRingMin: 1, DebugRingMax: 5}, 7, 45.0, false},
		{"az in range", BackgroundParams{DebugAzMin: 30.0, DebugAzMax: 60.0}, 5, 45.0, true},
		{"az below min", BackgroundParams{DebugAzMin: 50.0, DebugAzMax: 90.0}, 5, 45.0, false},
		{"az above max", BackgroundParams{DebugAzMin: 10.0, DebugAzMax: 30.0}, 5, 45.0, false},
		{"negative az normalized", BackgroundParams{DebugAzMin: 350.0, DebugAzMax: 360.0}, 5, -5.0, true},
		{"az over 360 normalized", BackgroundParams{DebugAzMin: 80.0, DebugAzMax: 100.0}, 5, 450.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.params.IsInDebugRange(tt.ring, tt.az)
			if result != tt.expected {
				t.Errorf("IsInDebugRange(%d, %f) = %v, want %v", tt.ring, tt.az, result, tt.expected)
			}
		})
	}
}

// TestBackgroundManager_GetParams tests GetParams method
func TestBackgroundManager_GetParams(t *testing.T) {
	g := makeTestGrid(2, 8)
	g.Params.BackgroundUpdateFraction = 0.05

	params := g.Manager.GetParams()
	if params.BackgroundUpdateFraction != 0.05 {
		t.Errorf("expected BackgroundUpdateFraction 0.05, got %v", params.BackgroundUpdateFraction)
	}

	// Test with nil manager
	var nilMgr *BackgroundManager
	params = nilMgr.GetParams()
	if params.BackgroundUpdateFraction != 0 {
		t.Error("expected zero params from nil manager")
	}
}

// TestBackgroundManager_SetParams tests SetParams method
func TestBackgroundManager_SetParams(t *testing.T) {
	g := makeTestGrid(2, 8)

	newParams := BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 5.0,
	}

	err := g.Manager.SetParams(newParams)
	if err != nil {
		t.Errorf("SetParams returned error: %v", err)
	}

	params := g.Manager.GetParams()
	if params.BackgroundUpdateFraction != 0.1 {
		t.Errorf("expected BackgroundUpdateFraction 0.1, got %v", params.BackgroundUpdateFraction)
	}

	// Test with nil manager
	var nilMgr *BackgroundManager
	err = nilMgr.SetParams(newParams)
	if err == nil {
		t.Error("expected error from nil manager")
	}
}

// TestBackgroundManager_SetNoiseRelativeFraction tests SetNoiseRelativeFraction method
func TestBackgroundManager_SetNoiseRelativeFraction(t *testing.T) {
	g := makeTestGrid(2, 8)

	err := g.Manager.SetNoiseRelativeFraction(0.02)
	if err != nil {
		t.Errorf("SetNoiseRelativeFraction returned error: %v", err)
	}

	params := g.Manager.GetParams()
	if params.NoiseRelativeFraction != 0.02 {
		t.Errorf("expected NoiseRelativeFraction 0.02, got %v", params.NoiseRelativeFraction)
	}

	// Test with nil manager
	var nilMgr *BackgroundManager
	err = nilMgr.SetNoiseRelativeFraction(0.02)
	if err == nil {
		t.Error("expected error from nil manager")
	}
}

// TestBackgroundManager_SetClosenessSensitivityMultiplier tests SetClosenessSensitivityMultiplier method
func TestBackgroundManager_SetClosenessSensitivityMultiplier(t *testing.T) {
	g := makeTestGrid(2, 8)

	err := g.Manager.SetClosenessSensitivityMultiplier(4.0)
	if err != nil {
		t.Errorf("SetClosenessSensitivityMultiplier returned error: %v", err)
	}

	params := g.Manager.GetParams()
	if params.ClosenessSensitivityMultiplier != 4.0 {
		t.Errorf("expected ClosenessSensitivityMultiplier 4.0, got %v", params.ClosenessSensitivityMultiplier)
	}

	// Test with nil manager
	var nilMgr *BackgroundManager
	err = nilMgr.SetClosenessSensitivityMultiplier(4.0)
	if err == nil {
		t.Error("expected error from nil manager")
	}
}

// TestBackgroundManager_SetNeighborConfirmationCount tests SetNeighborConfirmationCount method
func TestBackgroundManager_SetNeighborConfirmationCount(t *testing.T) {
	g := makeTestGrid(2, 8)

	err := g.Manager.SetNeighborConfirmationCount(5)
	if err != nil {
		t.Errorf("SetNeighborConfirmationCount returned error: %v", err)
	}

	params := g.Manager.GetParams()
	if params.NeighborConfirmationCount != 5 {
		t.Errorf("expected NeighborConfirmationCount 5, got %v", params.NeighborConfirmationCount)
	}

	// Test with nil manager
	var nilMgr *BackgroundManager
	err = nilMgr.SetNeighborConfirmationCount(5)
	if err == nil {
		t.Error("expected error from nil manager")
	}
}

// TestBackgroundManager_SetSeedFromFirstObservation tests SetSeedFromFirstObservation method
func TestBackgroundManager_SetSeedFromFirstObservation(t *testing.T) {
	g := makeTestGrid(2, 8)

	err := g.Manager.SetSeedFromFirstObservation(true)
	if err != nil {
		t.Errorf("SetSeedFromFirstObservation returned error: %v", err)
	}

	params := g.Manager.GetParams()
	if !params.SeedFromFirstObservation {
		t.Error("expected SeedFromFirstObservation to be true")
	}

	// Test with nil manager
	var nilMgr *BackgroundManager
	err = nilMgr.SetSeedFromFirstObservation(true)
	if err == nil {
		t.Error("expected error from nil manager")
	}
}

// TestBackgroundManager_SetWarmupParams tests SetWarmupParams method
func TestBackgroundManager_SetWarmupParams(t *testing.T) {
	g := makeTestGrid(2, 8)

	err := g.Manager.SetWarmupParams(1e9, 10)
	if err != nil {
		t.Errorf("SetWarmupParams returned error: %v", err)
	}

	params := g.Manager.GetParams()
	if params.WarmupDurationNanos != 1e9 {
		t.Errorf("expected WarmupDurationNanos 1e9, got %v", params.WarmupDurationNanos)
	}
	if params.WarmupMinFrames != 10 {
		t.Errorf("expected WarmupMinFrames 10, got %v", params.WarmupMinFrames)
	}

	// Test with nil manager
	var nilMgr *BackgroundManager
	err = nilMgr.SetWarmupParams(1e9, 10)
	if err == nil {
		t.Error("expected error from nil manager")
	}
}

// TestBackgroundManager_SetPostSettleUpdateFraction tests SetPostSettleUpdateFraction method
func TestBackgroundManager_SetPostSettleUpdateFraction(t *testing.T) {
	g := makeTestGrid(2, 8)

	err := g.Manager.SetPostSettleUpdateFraction(0.01)
	if err != nil {
		t.Errorf("SetPostSettleUpdateFraction returned error: %v", err)
	}

	params := g.Manager.GetParams()
	if params.PostSettleUpdateFraction != 0.01 {
		t.Errorf("expected PostSettleUpdateFraction 0.01, got %v", params.PostSettleUpdateFraction)
	}

	// Test with nil manager
	var nilMgr *BackgroundManager
	err = nilMgr.SetPostSettleUpdateFraction(0.01)
	if err == nil {
		t.Error("expected error from nil manager")
	}
}

// TestBackgroundManager_SetForegroundClusterParams tests SetForegroundClusterParams method
func TestBackgroundManager_SetForegroundClusterParams(t *testing.T) {
	g := makeTestGrid(2, 8)

	err := g.Manager.SetForegroundClusterParams(15, 0.8)
	if err != nil {
		t.Errorf("SetForegroundClusterParams returned error: %v", err)
	}

	params := g.Manager.GetParams()
	if params.ForegroundMinClusterPoints != 15 {
		t.Errorf("expected ForegroundMinClusterPoints 15, got %v", params.ForegroundMinClusterPoints)
	}
	if params.ForegroundDBSCANEps != 0.8 {
		t.Errorf("expected ForegroundDBSCANEps 0.8, got %v", params.ForegroundDBSCANEps)
	}

	// Test with nil manager
	var nilMgr *BackgroundManager
	err = nilMgr.SetForegroundClusterParams(15, 0.8)
	if err == nil {
		t.Error("expected error from nil manager")
	}

	// Test with zero values (should not update)
	err = g.Manager.SetForegroundClusterParams(0, 0)
	if err != nil {
		t.Errorf("SetForegroundClusterParams returned error: %v", err)
	}
	// Values should remain unchanged
	params = g.Manager.GetParams()
	if params.ForegroundMinClusterPoints != 15 {
		t.Errorf("expected ForegroundMinClusterPoints to remain 15, got %v", params.ForegroundMinClusterPoints)
	}
}

// TestBackgroundGrid_Idx tests the Idx method
func TestBackgroundGrid_Idx(t *testing.T) {
	g := makeTestGrid(4, 8)

	tests := []struct {
		ring     int
		azBin    int
		expected int
	}{
		{0, 0, 0},
		{0, 1, 1},
		{1, 0, 8},
		{1, 1, 9},
		{3, 7, 31},
	}

	for _, tt := range tests {
		idx := g.Idx(tt.ring, tt.azBin)
		if idx != tt.expected {
			t.Errorf("Idx(%d, %d) = %d, want %d", tt.ring, tt.azBin, idx, tt.expected)
		}
	}
}
