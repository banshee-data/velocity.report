package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

func TestPairSpec(t *testing.T) {
	s, err := parsePairSpec("null/q-600=-611,-601,-599,-589")
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "null/q-600" || s.Group != "null" || s.A0 != -611 || s.A1 != -601 || s.B0 != -599 || s.B1 != -589 {
		t.Fatalf("parsed %+v", s)
	}
	if s, _ := parsePairSpec("event=-11,-1,1,11"); s.Group != "event" {
		t.Fatalf("a name without a slash is its own group: %+v", s)
	}
	for _, bad := range []string{"=1,2,3,4", "x=1,2,3", "x=2,1,3,4", "x=1,2,4,3", "x=a,2,3,4", "x"} {
		if _, err := parsePairSpec(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestAnchorMapping(t *testing.T) {
	const s = int64(1e9)
	caps := []captureInfo{
		{Path: "/p/a.pcap", FirstNanos: 1000 * s, LastNanos: 1300 * s},
		{Path: "/p/b.pcap", FirstNanos: 1300 * s, LastNanos: 1600 * s},
	}
	m := mapAnchor(caps, 1540*s+500_000_000)
	if m.Capture != "/p/b.pcap" || m.OffsetInFileS != 240.5 || m.SinceFirstS != 540.5 || m.UntilFileEndS != 59.5 {
		t.Fatalf("mapping %+v", m)
	}
	if m.SinceFirstClock != "9:00.500" {
		t.Fatalf("clock %q", m.SinceFirstClock)
	}
	if gap := mapAnchor(caps, 1700*s); gap.Capture != "" {
		t.Fatalf("an anchor after the last capture names %q", gap.Capture)
	}
}

func TestSeriesFindsTheRotationAStepStartsIn(t *testing.T) {
	ref := make([]cellStat, numCells)
	w := newWindowAccum()
	w.seriesRef, w.seriesTau = ref, 0.15
	for c := range ref {
		ref[c] = cellStat{Median: 20, Static: true}
	}
	// Three rotations, each sweeping every bin of one ring; the range steps
	// by 0.5 m halfway through the second.
	ts := int64(0)
	for rot := 0; rot < 3; rot++ {
		for bin := 0; bin < numAzBins; bin++ {
			az := binCentreDeg(bin)
			d := 20.0
			if rot > 1 || (rot == 1 && az >= 180) {
				d = 20.5
			}
			w.addPoint(l2frames.PointPolar{Channel: 20, Azimuth: az, Distance: d, RawBlockAzimuth: uint16(az * 100), Timestamp: ts})
			ts += int64(1e8) / numAzBins
		}
	}
	if len(w.series) != 3 {
		t.Fatalf("%d rotations in the series, want 3", len(w.series))
	}
	if w.series[0].Changed != 0 || w.series[2].Changed != w.series[2].Returns {
		t.Fatalf("rotations 0 and 2: %+v %+v", w.series[0].Changed, w.series[2].Changed)
	}
	mid := w.series[1]
	for k := 0; k < seriesSectors; k++ {
		want := 0
		if k >= seriesSectors/2 {
			want = mid.SectorReturns[k]
		}
		if mid.SectorChanged[k] != want {
			t.Fatalf("rotation 1 sector %d: %d of %d changed, want %d", k, mid.SectorChanged[k], mid.SectorReturns[k], want)
		}
	}
}

func TestEmptyStatisticsMarshalAsNull(t *testing.T) {
	b, err := json.Marshal(summarise(nil, 0.1))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"median_abs_m":null`) {
		t.Fatalf("empty stats marshalled as %s", b)
	}
	b, err = json.Marshal(spreadOf(nil))
	if err != nil || string(b) != `{"min":null,"median":null,"max":null}` {
		t.Fatalf("empty spread: %s %v", b, err)
	}
	if s := spreadOf([]float64{3, 1, 2}); s.Min != 1 || s.Median != 2 || s.Max != 3 || math.IsNaN(s.Median) {
		t.Fatalf("spread %+v", s)
	}
}
