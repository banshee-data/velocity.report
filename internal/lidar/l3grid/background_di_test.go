package l3grid

import (
	"testing"
)

// TestNewBackgroundManagerDI verifies the DI constructor creates a manager
// without registering it in the global registry.
func TestNewBackgroundManagerDI(t *testing.T) {
	t.Parallel()

	params := BackgroundParams{
		BackgroundUpdateFraction:       0.1,
		ClosenessSensitivityMultiplier: 2.0,
		SafetyMarginMeters:             20.0,
	}

	t.Run("success without store", func(t *testing.T) {
		t.Parallel()
		mgr := NewBackgroundManagerDI("di-sensor", 4, 8, params, nil)
		if mgr == nil {
			t.Fatal("expected non-nil manager")
		}
		if mgr.Grid == nil {
			t.Fatal("expected non-nil grid")
		}
		if mgr.Grid.SensorID != "di-sensor" {
			t.Errorf("expected sensor ID 'di-sensor', got %s", mgr.Grid.SensorID)
		}
		if mgr.Grid.Rings != 4 || mgr.Grid.AzimuthBins != 8 {
			t.Errorf("expected 4x8 grid, got %dx%d", mgr.Grid.Rings, mgr.Grid.AzimuthBins)
		}
		if len(mgr.Grid.Cells) != 32 {
			t.Errorf("expected 32 cells, got %d", len(mgr.Grid.Cells))
		}
		if mgr.Grid.RegionMgr == nil {
			t.Error("expected RegionMgr to be initialised")
		}
		if mgr.PersistCallback != nil {
			t.Error("expected nil PersistCallback without store")
		}
		if len(mgr.Grid.AcceptanceBucketsMeters) != 11 {
			t.Errorf("expected 11 acceptance buckets, got %d", len(mgr.Grid.AcceptanceBucketsMeters))
		}

		// Verify NOT registered in global registry
		retrieved := GetBackgroundManager("di-sensor")
		if retrieved == mgr {
			t.Error("DI constructor should NOT register in global registry")
		}
	})

	t.Run("success with store", func(t *testing.T) {
		t.Parallel()
		store := &mockBgStore{} // from background_flusher_test.go
		mgr := NewBackgroundManagerDI("di-store", 4, 8, params, store)
		if mgr == nil {
			t.Fatal("expected non-nil manager")
		}
		if mgr.PersistCallback == nil {
			t.Error("expected non-nil PersistCallback with store")
		}
	})

	t.Run("invalid inputs", func(t *testing.T) {
		t.Parallel()
		if mgr := NewBackgroundManagerDI("", 4, 8, params, nil); mgr != nil {
			t.Error("expected nil for empty sensorID")
		}
		if mgr := NewBackgroundManagerDI("s", 0, 8, params, nil); mgr != nil {
			t.Error("expected nil for zero rings")
		}
		if mgr := NewBackgroundManagerDI("s", 4, 0, params, nil); mgr != nil {
			t.Error("expected nil for zero azBins")
		}
	})
}

// TestProcessFramePolar_Diagnostics exercises the diagnostics logging branch.
func TestProcessFramePolar_Diagnostics(t *testing.T) {
	t.Parallel()
	g := makeTestGrid(2, 8)
	g.Manager.EnableDiagnostics = true
	points := []PointPolar{{Channel: 0, Azimuth: 10.0, Distance: 5.0}}
	g.Manager.ProcessFramePolar(points) // covers diagnostics log branch
}

// TestAssignRegionParams_ZeroDefaults tests that assignRegionParams uses
// sensible defaults when base params have zero values.
func TestAssignRegionParams_ZeroDefaults(t *testing.T) {
	t.Parallel()
	rm := NewRegionManager(2, 8)
	zeroed := BackgroundParams{} // all zero

	rp := rm.assignRegionParams(0, zeroed)
	if rp.NoiseRelativeFraction <= 0 {
		t.Errorf("expected positive noise fraction, got %f", rp.NoiseRelativeFraction)
	}
	if rp.NeighborConfirmationCount <= 0 {
		t.Errorf("expected positive neighbor count, got %d", rp.NeighborConfirmationCount)
	}
	if rp.SettleUpdateFraction <= 0 {
		t.Errorf("expected positive update fraction, got %f", rp.SettleUpdateFraction)
	}
}
