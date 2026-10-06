package l3grid

import (
	"testing"
	"time"
)

// countNonzeroCells is the ground truth nonzeroCellCount is supposed to track.
func countNonzeroCells(g *BackgroundGrid) int {
	n := 0
	for i := range g.Cells {
		if g.Cells[i].TimesSeenCount > 0 {
			n++
		}
	}
	return n
}

// drainCells pushes every populated cell to the edge of the floor, then sends
// one frame that disagrees with all of them, and reports the resulting counts.
func drainCells(t *testing.T, bm *BackgroundManager, azBins int, startAt uint32) (tracked, actual int) {
	t.Helper()

	for i := 0; i < 6; i++ {
		if _, err := bm.ProcessFramePolarWithMaskAt(polarRing(0, azBins, 10, int64(i)), time.Now()); err != nil {
			t.Fatalf("seed frame %d: %v", i, err)
		}
	}

	bm.Grid.mu.Lock()
	populated := 0
	for i := range bm.Grid.Cells {
		if bm.Grid.Cells[i].TimesSeenCount > 0 {
			bm.Grid.Cells[i].TimesSeenCount = startAt
			populated++
		}
	}
	bm.Grid.nonzeroCellCount = populated
	bm.Grid.mu.Unlock()

	if populated == 0 {
		t.Fatal("expected the seeding frames to populate some cells")
	}

	for i := 0; i < 10; i++ {
		if _, err := bm.ProcessFramePolarWithMaskAt(polarRing(0, azBins, 2, int64(900+i)), time.Now()); err != nil {
			t.Fatalf("divergent frame %d: %v", i, err)
		}
	}

	bm.Grid.mu.RLock()
	defer bm.Grid.mu.RUnlock()
	return bm.Grid.nonzeroCellCount, countNonzeroCells(bm.Grid)
}

// TestConfidenceFloorStopsDrainAboveZero pins the behaviour that makes the
// removed full-drain branch dead code, and that keeps nonzeroCellCount honest
// without any bookkeeping on the divergence path.
//
// A divergent observation decrements confidence only while it is above the
// floor, and the floor is never zero: an unset MinConfidenceFloor is coerced to
// DefaultMinConfidenceFloor. A cell therefore cannot reach zero here, so it
// never leaves the non-zero count, so the count cannot drift.
func TestConfidenceFloorStopsDrainAboveZero(t *testing.T) {
	const azBins = 16

	tests := []struct {
		name      string
		floor     uint32
		wantFloor uint32
	}{
		{"unset floor uses the default", 0, DefaultMinConfidenceFloor},
		{"explicit floor is honoured", 5, 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bm := NewBackgroundManagerDI(t.Name(), 1, azBins, BackgroundParams{
				SeedFromFirstObservation: true,
				BackgroundUpdateFraction: 0.05,
				SafetyMarginMetres:       0.05,
				NoiseRelativeFraction:    0.001,
				MinConfidenceFloor:       tc.floor,
			}, nil)

			// Start well above the floor so the drain has somewhere to go.
			tracked, actual := drainCells(t, bm, azBins, tc.wantFloor+4)

			bm.Grid.mu.RLock()
			defer bm.Grid.mu.RUnlock()

			for i := range bm.Grid.Cells {
				c := bm.Grid.Cells[i].TimesSeenCount
				if c > 0 && c < tc.wantFloor {
					t.Errorf("cell %d drained to %d, below the floor of %d", i, c, tc.wantFloor)
				}
			}
			if tracked != actual {
				t.Errorf("nonzeroCellCount = %d, actual non-zero cells = %d", tracked, actual)
			}
			if actual == 0 {
				t.Error("every cell reached zero; the floor did not hold")
			}
		})
	}
}

// TestUnsetConfidenceFloorIsNotNoFloor documents the config semantics
// explicitly, because the removed branch was written as though a zero meant
// "no floor". It does not: zero selects the default, so a full drain cannot be
// configured through MinConfidenceFloor at all.
func TestUnsetConfidenceFloorIsNotNoFloor(t *testing.T) {
	const azBins = 16
	bm := NewBackgroundManagerDI(t.Name(), 1, azBins, BackgroundParams{
		SeedFromFirstObservation: true,
		BackgroundUpdateFraction: 0.05,
		SafetyMarginMetres:       0.05,
		NoiseRelativeFraction:    0.001,
		MinConfidenceFloor:       0, // reads as "unset", not "no floor"
	}, nil)

	tracked, actual := drainCells(t, bm, azBins, DefaultMinConfidenceFloor+4)

	if actual == 0 {
		t.Fatal("cells drained to zero: a zero MinConfidenceFloor now means no floor, " +
			"which changes background retention and needs its own decision")
	}
	if tracked != actual {
		t.Errorf("nonzeroCellCount = %d, actual non-zero cells = %d", tracked, actual)
	}
}
