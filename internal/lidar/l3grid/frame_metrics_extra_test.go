package l3grid

import "testing"

// TestExpectedNoiseFloor_DefaultFraction exercises the default-fraction branch
// in expectedNoiseFloor (Params.NoiseRelativeFraction <= 0).
func TestExpectedNoiseFloor_DefaultFraction(t *testing.T) {
	t.Parallel()
	g := makeTestGrid(2, 4)
	g.Params.NoiseRelativeFraction = 0
	for i := range g.Cells {
		g.Cells[i].TimesSeenCount = 5
		g.Cells[i].AverageRangeMeters = 10
		g.Cells[i].RangeSpreadMeters = 0.1
	}
	if dev := g.Manager.GetNoiseBoundsDeviation(); dev <= 0 {
		t.Errorf("deviation = %v, want > 0", dev)
	}
	if !g.Manager.IsWithinNoiseBounds(5.0) {
		t.Error("expected within bounds with default fraction")
	}
}
