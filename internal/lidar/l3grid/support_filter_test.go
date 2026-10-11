package l3grid

import (
	"testing"
	"time"
)

// fgPoint places a foreground point in the given channel and azimuth bin of
// a grid with azBins bins, at range r.
func fgPoint(azBins, channel, bin int, r float64) PointPolar {
	return PointPolar{Channel: channel, Azimuth: (float64(bin) + 0.5) * 360.0 / float64(azBins), Distance: r}
}

func allTrue(n int) []bool {
	m := make([]bool, n)
	for i := range m {
		m[i] = true
	}
	return m
}

func TestSupportFilter_IsolatedCellIsFlipped(t *testing.T) {
	g := makeCoverageGrid(5, 64)
	p := DefaultSupportFilterParams()
	var s supportFilterScratch
	pts := []PointPolar{fgPoint(64, 3, 20, 12.0)}
	mask := allTrue(1)
	if got := g.applySupportFilter(p, &s, pts, mask); got != 1 || mask[0] {
		t.Fatalf("isolated foreground cell kept: flipped=%d mask=%v", got, mask)
	}
}

func TestSupportFilter_BlockOfSimilarRangeIsKept(t *testing.T) {
	g := makeCoverageGrid(5, 64)
	p := DefaultSupportFilterParams()
	var s supportFilterScratch
	var pts []PointPolar
	for ch := 2; ch <= 4; ch++ {
		for bin := 10; bin <= 14; bin++ {
			pts = append(pts, fgPoint(64, ch, bin, 10.0+0.01*float64(bin)))
		}
	}
	mask := allTrue(len(pts))
	if got := g.applySupportFilter(p, &s, pts, mask); got != 0 {
		t.Fatalf("a 3x5 block of foreground at one range lost %d points", got)
	}
}

func TestSupportFilter_RangeDissimilarNeighboursDoNotSupport(t *testing.T) {
	g := makeCoverageGrid(5, 64)
	p := DefaultSupportFilterParams()
	var s supportFilterScratch
	var pts []PointPolar
	for ch := 2; ch <= 4; ch++ {
		for bin := 10; bin <= 14; bin++ {
			pts = append(pts, fgPoint(64, ch, bin, 10.0))
		}
	}
	// A lone return 20 m behind the block, inside its window.
	pts = append(pts, fgPoint(64, 3, 15, 30.0))
	mask := allTrue(len(pts))
	got := g.applySupportFilter(p, &s, pts, mask)
	if got != 1 || mask[len(pts)-1] {
		t.Fatalf("the far return was not the one flipped: flipped=%d last=%v", got, mask[len(pts)-1])
	}
	for i := 0; i < len(pts)-1; i++ {
		if !mask[i] {
			t.Fatalf("block point %d was flipped", i)
		}
	}
}

func TestSupportFilter_AzimuthWrapsAroundAndNearestRangeRepresents(t *testing.T) {
	g := makeCoverageGrid(1, 64)
	p := SupportFilterParams{VertNeighbours: 0, HorNeighbours: 1, MinSupport: 2, TauBaseMetres: 0.2}
	var s supportFilterScratch
	// Bins 63, 0 and 1 on one ring. Bin 0's two neighbours are 63 and 1, so it
	// is supported only if the search wraps; bins 63 and 1 each see one
	// foreground neighbour and are flipped.
	pts := []PointPolar{fgPoint(64, 1, 63, 5.0), fgPoint(64, 1, 0, 5.0), fgPoint(64, 1, 1, 5.0)}
	mask := allTrue(3)
	if got := g.applySupportFilter(p, &s, pts, mask); got != 2 || !mask[1] || mask[0] || mask[2] {
		t.Fatalf("wrap: flipped=%d mask=%v", got, mask)
	}
	// A cell with two returns is represented by its nearest: 10.0 against a
	// neighbour at 10.1 is within 0.2 m, while 12.0 would not be.
	p.MinSupport = 1
	pts = []PointPolar{fgPoint(64, 1, 20, 12.0), fgPoint(64, 1, 20, 10.0), fgPoint(64, 1, 21, 10.1)}
	mask = allTrue(3)
	if got := g.applySupportFilter(p, &s, pts, mask); got != 0 {
		t.Fatalf("nearest-range representative: flipped=%d mask=%v", got, mask)
	}
	// Every point of an unsupported cell is flipped, not just one.
	pts = []PointPolar{fgPoint(64, 1, 40, 8.0), fgPoint(64, 1, 40, 8.2), fgPoint(64, 1, 40, 8.1)}
	mask = allTrue(3)
	if got := g.applySupportFilter(p, &s, pts, mask); got != 3 {
		t.Fatalf("multi-point cell: flipped=%d mask=%v", got, mask)
	}
}

func TestSupportFilter_ReusesBuffersAcrossFrames(t *testing.T) {
	g := makeCoverageGrid(2, 32)
	p := SupportFilterParams{VertNeighbours: 0, HorNeighbours: 1, MinSupport: 1, TauBaseMetres: 0.5}
	var s supportFilterScratch
	// Frame 1: a supported pair. Frame 2: the same cells but only one point;
	// a stale stamp from frame 1 must not support it.
	pts := []PointPolar{fgPoint(32, 1, 4, 6.0), fgPoint(32, 1, 5, 6.0)}
	if got := g.applySupportFilter(p, &s, pts, allTrue(2)); got != 0 {
		t.Fatalf("frame 1 flipped %d", got)
	}
	mask := allTrue(1)
	if got := g.applySupportFilter(p, &s, pts[:1], mask); got != 1 || mask[0] {
		t.Fatalf("frame 2 used a stale neighbour: flipped=%d", got)
	}
}

// Through the mask path: the filter changes the returned mask and the frame
// counters, and nothing else. The cell the flipped return hit still records
// the rejection, so the background model is exactly what it is without the
// filter.
func TestProcessFramePolarWithMask_SupportFilterIsOutputOnly(t *testing.T) {
	run := func(withFilter bool) ([]bool, BackgroundCell, int64) {
		g := makeCoverageGrid(1, 32)
		bm := g.Manager
		if withFilter {
			p := SupportFilterParams{VertNeighbours: 0, HorNeighbours: 2, MinSupport: 1, TauBaseMetres: 0.5}
			if err := bm.SetSupportFilter(&p); err != nil {
				t.Fatal(err)
			}
		}
		frame := make([]PointPolar, 32)
		for bin := range frame {
			frame[bin] = fgPoint(32, 1, bin, 10.0)
		}
		at := time.Unix(1_700_000_000, 0)
		for i := 0; i < 200; i++ {
			if _, err := bm.ProcessFramePolarWithMaskAt(frame, at); err != nil {
				t.Fatal(err)
			}
		}
		frame[5] = fgPoint(32, 1, 5, 20.0) // one far return, no foreground neighbours
		mask, err := bm.ProcessFramePolarWithMaskAt(frame, at)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]bool, len(mask))
		copy(out, mask)
		return out, g.Cells[g.Idx(0, 5)], g.ForegroundCount
	}
	maskOff, cellOff, fgOff := run(false)
	maskOn, cellOn, fgOn := run(true)
	if !maskOff[5] || fgOff != 1 {
		t.Fatalf("without the filter the far return should be foreground: mask=%v count=%d", maskOff[5], fgOff)
	}
	if maskOn[5] || fgOn != 0 {
		t.Fatalf("with the filter the unsupported return should be background: mask=%v count=%d", maskOn[5], fgOn)
	}
	if cellOn != cellOff {
		t.Fatalf("the filter changed the cell state:\n off %+v\n on  %+v", cellOff, cellOn)
	}
	if cellOn.RecentForegroundCount != 1 {
		t.Fatalf("the cell should still have recorded the rejection: %+v", cellOn)
	}
}

func TestSetSupportFilter_NilDisablesAndInvalidIsRefused(t *testing.T) {
	g := makeCoverageGrid(1, 8)
	bm := g.Manager
	p := DefaultSupportFilterParams()
	if err := bm.SetSupportFilter(&p); err != nil {
		t.Fatal(err)
	}
	if g.Params.SupportFilter == nil || *g.Params.SupportFilter != p {
		t.Fatalf("filter not set: %+v", g.Params.SupportFilter)
	}
	p.MinSupport = 99 // the manager keeps its own copy
	if g.Params.SupportFilter.MinSupport == 99 {
		t.Fatal("SetSupportFilter stored the caller's pointer")
	}
	if err := bm.SetSupportFilter(nil); err != nil {
		t.Fatal(err)
	}
	if g.Params.SupportFilter != nil {
		t.Fatal("nil did not switch the filter off")
	}
	bad := SupportFilterParams{MinSupport: 0}
	if err := bm.SetSupportFilter(&bad); err == nil {
		t.Fatal("MinSupport 0 was accepted")
	}
	if err := (*BackgroundManager)(nil).SetSupportFilter(&p); err == nil {
		t.Fatal("nil manager was accepted")
	}
}
