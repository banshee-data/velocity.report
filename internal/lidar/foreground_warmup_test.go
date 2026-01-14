package lidar

import (
	"testing"
)

// TestProcessFramePolarWithMask_WarmupSensitivity verifies that the warmup sensitivity scaling
// correctly suppresses noise during the initialization phase while still allowing large deviations (vehicles).
func TestProcessFramePolarWithMask_WarmupSensitivity(t *testing.T) {
	// Setup a grid with known parameters
	rings := 1
	azBins := 1
	g := &BackgroundGrid{
		SensorID:    "test-sensor",
		Rings:       rings,
		AzimuthBins: azBins,
		Cells:       make([]BackgroundCell, 1),
		Params: BackgroundParams{
			BackgroundUpdateFraction:       0.02, // Slow update
			ClosenessSensitivityMultiplier: 3.0,
			SafetyMarginMeters:             0.1,  // 10cm safety
			NoiseRelativeFraction:          0.01, // 1% relative noise
			SeedFromFirstObservation:       true,
		},
	}
	g.Manager = &BackgroundManager{Grid: g}

	// Helper to set cell state directly
	setCellState := func(count uint32, avg float32, spread float32) {
		g.Cells[0].TimesSeenCount = count
		g.Cells[0].AverageRangeMeters = avg
		g.Cells[0].RangeSpreadMeters = spread
		// Reset locked baseline state to prevent interference between scenarios
		g.Cells[0].LockedBaseline = 0
		g.Cells[0].LockedSpread = 0
		g.Cells[0].LockedAtCount = 0
		g.Cells[0].RecentForegroundCount = 0
		g.Cells[0].FrozenUntilUnixNanos = 0
	}

	bgDist := float32(10.0) // 10m background
	spread := float32(0.0)  // Perfect 0 spread (simulating new/frozen cell)

	// Scenario 1: Early Warmup (Count = 5)
	// Multiplier should be ~3.85x (1.0 + 3.0 * 95/100)
	// Base Threshold = 3.0*(0 + 0.01*10 + 0.01) + 0.1 = 3.0*0.11 + 0.1 = 0.43m
	// With Multiplier ~3.85 => ~1.65m threshold
	t.Run("EarlyWarmup_SuppressesNoise", func(t *testing.T) {
		setCellState(5, bgDist, spread)

		// Test point with 0.5m deviation (Noise)
		// Should be BACKGROUND because 0.5 < 1.65
		points := []PointPolar{{Channel: 1, Azimuth: 0.0, Distance: float64(bgDist + 0.5)}}
		mask, _ := g.Manager.ProcessFramePolarWithMask(points)
		if mask[0] {
			t.Errorf("Expected 0.5m deviation to be filtered as background during warmup (count=5), got Foreground")
		}

		// Test point with 5.0m deviation (Vehicle)
		// Should be FOREGROUND because 5.0 > 1.65
		points = []PointPolar{{Channel: 1, Azimuth: 0.0, Distance: float64(bgDist - 5.0)}}
		mask, _ = g.Manager.ProcessFramePolarWithMask(points)
		if !mask[0] {
			t.Errorf("Expected 5.0m deviation (Vehicle) to be detected as foreground during warmup (count=5), got Background")
		}
	})

	// Scenario 2: Mid Warmup (Count = 50)
	// Multiplier should be 2.5x (1.0 + 3.0 * 50/100)
	// Base Threshold ~ 0.43m
	// With Multiplier 2.5 => ~1.075m threshold
	t.Run("MidWarmup_RelaxedSensitivity", func(t *testing.T) {
		setCellState(50, bgDist, spread)

		// Test point with 0.8m deviation
		// Should be BACKGROUND because 0.8 < 1.075
		points := []PointPolar{{Channel: 1, Azimuth: 0.0, Distance: float64(bgDist + 0.8)}}
		mask, _ := g.Manager.ProcessFramePolarWithMask(points)
		if mask[0] {
			t.Errorf("Expected 0.8m deviation to be filtered as background during mid-warmup (count=50), got Foreground")
		}
	})

	// Scenario 3: Fully Warmed Up (Count = 100)
	// Multiplier should be 1.0x
	// Base Threshold ~ 0.43m
	t.Run("WarmedUp_NormalSensitivity", func(t *testing.T) {
		setCellState(100, bgDist, spread)

		// Test point with 0.5m deviation
		// Should be FOREGROUND because 0.5 > 0.43
		points := []PointPolar{{Channel: 1, Azimuth: 0.0, Distance: float64(bgDist + 0.5)}}
		mask, _ := g.Manager.ProcessFramePolarWithMask(points)
		if !mask[0] {
			t.Errorf("Expected 0.5m deviation to be detected as foreground after warmup (count=100), got Background")
		}
	})
}
