package main

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestVisibleTurnDiscountsOnlyALabelSwap(t *testing.T) {
	a := box{psi: 0, length: 4.5, width: 1.8}
	for _, c := range []struct {
		name string
		b    box
		want float64
	}{
		{"a 5° turn", box{psi: 5 * math.Pi / 180, length: 4.5, width: 1.8}, 5},
		{"a quarter turn that swaps the extents draws the same box", box{psi: math.Pi / 2, length: 1.8, width: 4.5}, 0},
		{"a quarter turn that keeps them is a quarter turn", box{psi: math.Pi / 2, length: 4.5, width: 1.8}, 90},
		{"a half turn is a change of front", box{psi: math.Pi, length: 4.5, width: 1.8}, 0},
	} {
		if got := visibleTurn(a, c.b) * 180 / math.Pi; math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s: %.2f°, want %.2f°", c.name, got, c.want)
		}
	}
	// Extents that fluctuate about a square without the heading turning are
	// not a quarter turn.
	sq := box{psi: 0, length: 0.50, width: 0.45}
	if got := visibleTurn(sq, box{psi: 2 * math.Pi / 180, length: 0.44, width: 0.48}) * 180 / math.Pi; math.Abs(got-2) > 1e-6 {
		t.Errorf("near-square wobble read as %.2f°", got)
	}
}

func lShape(psi float64) []l4perception.WorldPoint {
	c, s := math.Cos(psi), math.Sin(psi)
	var out []l4perception.WorldPoint
	for i := 0; i <= 45; i++ {
		a := -2.25 + 0.1*float64(i)
		out = append(out, l4perception.WorldPoint{X: 10 + a*c + 0.9*s, Y: 5 + a*s - 0.9*c})
	}
	for i := 0; i <= 18; i++ {
		n := -0.9 + 0.1*float64(i)
		out = append(out, l4perception.WorldPoint{X: 10 + 2.25*c - n*s, Y: 5 + 2.25*s + n*c})
	}
	return out
}

// outline is a 4.5 × 1.8 m body's four sides.
func outline(psi float64) []l4perception.WorldPoint {
	c, s := math.Cos(psi), math.Sin(psi)
	var out []l4perception.WorldPoint
	add := func(a, n float64) {
		out = append(out, l4perception.WorldPoint{X: 10 + a*c - n*s, Y: 5 + a*s + n*c})
	}
	for i := 0; i <= 45; i++ {
		a := -2.25 + 0.1*float64(i)
		add(a, -0.9)
		add(a, 0.9)
	}
	for i := 1; i < 18; i++ {
		n := -0.9 + 0.1*float64(i)
		add(-2.25, n)
		add(2.25, n)
	}
	return out
}

func TestMinAreaRectFindsTheBodyAxisOfAnOutline(t *testing.T) {
	psi := 0.4
	ref := minAreaRect(&l5tracks.WorldCluster{Members: outline(psi)})
	if !ref.ok || !ref.axisOK {
		t.Fatalf("no reference: %+v", ref)
	}
	if err := foldQuarterAbs(ref.axis-psi) * 180 / math.Pi; err > 0.5 {
		t.Fatalf("axis %.2f° off the body", err)
	}
	if math.Abs(ref.area-4.5*1.8) > 0.05 {
		t.Fatalf("area %.2f m², want the 4.5 × 1.8 body", ref.area)
	}
}

// An exact L's hull is a right triangle, and the rectangle on its hypotenuse
// has the legs' product for its area, the same as the body's: the reference
// cannot choose between them, so it is a coarse check on L views.
func TestMinAreaRectOfAnLHasTheBodyArea(t *testing.T) {
	ref := minAreaRect(&l5tracks.WorldCluster{Members: lShape(0.4)})
	if !ref.ok || math.Abs(ref.area-4.5*1.8) > 0.05 {
		t.Fatalf("area %.2f m², want 8.1 whichever rectangle it chose", ref.area)
	}
}

func TestDiagnoseTightHoldsEveryMember(t *testing.T) {
	pts := lShape(0.4)
	tr := &l5tracks.TrackedObject{OBBHeadingRad: 0.1, OBBLength: 1, OBBWidth: 1}
	b, ok := diagnoseTight(tr, &l5tracks.WorldCluster{Members: pts})
	if !ok {
		t.Fatal("no box")
	}
	c, s := math.Cos(b.psi), math.Sin(b.psi)
	for _, p := range pts {
		dx, dy := p.X-b.x, p.Y-b.y
		if math.Abs(dx*c+dy*s) > b.length/2+1e-9 || math.Abs(-dx*s+dy*c) > b.width/2+1e-9 {
			t.Fatalf("member (%.2f, %.2f) outside %+v", p.X, p.Y, b)
		}
	}
}

func TestSelectVariants(t *testing.T) {
	if vs, err := selectVariants("none"); err != nil || len(vs) != 0 {
		t.Fatalf("none: %v %v", vs, err)
	}
	if vs, err := selectVariants("km,dx-pca"); err != nil || len(vs) != 2 || vs[1].diagnose == nil {
		t.Fatalf("km,dx-pca: %v %v", vs, err)
	}
	if _, err := selectVariants("km,nonesuch"); err == nil {
		t.Fatal("an unknown variant was accepted")
	}
}

func TestOverlapShareOfBoxes(t *testing.T) {
	a := box{x: 0, y: 0, psi: 0, length: 4, width: 2}
	for _, c := range []struct {
		name string
		b    box
		want float64
	}{
		{"the same box", a, 1},
		{"half along its length", box{x: 2, y: 0, psi: 0, length: 4, width: 2}, 0.5},
		{"apart", box{x: 5, y: 0, psi: 0, length: 4, width: 2}, 0},
		{"a small box inside", box{x: 0.5, y: 0, psi: 0.3, length: 1, width: 0.5}, 1},
		// A 2 m square crossing the middle at a quarter turn shares 2 × 2 m².
		{"a quarter turn across", box{x: 0, y: 0, psi: math.Pi / 2, length: 2, width: 2}, 1},
		{"a quarter turn of itself", box{x: 0, y: 0, psi: math.Pi / 2, length: 4, width: 2}, 0.5},
	} {
		if got := overlapShare(a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: overlap %.4f, want %.4f", c.name, got, c.want)
		}
		if got := overlapShare(c.b, a); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s, the other way: overlap %.4f, want %.4f", c.name, got, c.want)
		}
	}
}

func TestDuplicatesCountTwoTracksOnOneVehicle(t *testing.T) {
	s := newScorer(nil, 0.1)
	car := box{x: 10, y: 0, psi: 0, length: 4.5, width: 1.8}
	twin := box{x: 10.4, y: 0.1, psi: 0.05, length: 3.8, width: 1.8}
	next := box{x: 16, y: 0, psi: 0, length: 4.5, width: 1.8}
	person := box{x: 10, y: 0, psi: 0, length: 0.6, width: 0.5}
	confirmed := map[string]bool{"a": true, "b": true, "c": true, "p": true}
	for i := 0; i < 6; i++ {
		rows := []crossRow{
			{id: "a", cluster: 0, boxes: map[string]box{published: car}},
			{id: "c", cluster: 1, boxes: map[string]box{published: next}},
			{id: "p", cluster: 2, boxes: map[string]box{published: person}},
		}
		if i < 5 {
			// The twin coasts on the car: no cluster of its own.
			rows = append(rows, crossRow{id: "b", cluster: -1, boxes: map[string]box{published: twin}})
		}
		s.countDuplicates(rows, confirmed)
	}
	d := s.duplicates()
	if d.VehicleTracks != 3 || d.Pairs != 1 || d.Tracks != 2 || d.PairFrames != 5 || d.ObservedVehicleFrames != 12 {
		t.Fatalf("duplicates %+v, want 3 vehicle tracks and one pair of two tracks over 5 frames", d)
	}
}

// The published box's vertical is the anchored box's when box_ground drew
// one, and otherwise the frame's lowest return and cluster height: what a
// viewer draws either way.
func TestPublishedVerticalIsWhatAViewerDraws(t *testing.T) {
	tr := &l5tracks.TrackedObject{LatestZ: -1.2, OBBHeight: 0.4}
	if base, h := publishedVertical(tr); base != float64(float32(-1.2)) || h != float64(float32(0.4)) {
		t.Fatalf("legacy vertical %.2f / %.2f, want the frame's -1.2 / 0.4", base, h)
	}
	tr.BoxZ, tr.BoxHeight = -2.3, 1.5
	if base, h := publishedVertical(tr); base != float64(float32(-2.3)) || h != 1.5 {
		t.Fatalf("anchored vertical %.2f / %.2f, want the box's -2.3 / 1.5", base, h)
	}
}

func TestScoreVerticalMeasuresTheDrawnBaseFrameToFrame(t *testing.T) {
	s := newScorer(nil, 0.1)
	agg, st := &rowAgg{}, &trackState{}
	lows := []float32{-2.0, -1.4, -0.8}
	for i, low := range lows {
		tr := &l5tracks.TrackedObject{LatestZ: low, OBBHeight: -0.5 - low}
		cluster := &l5tracks.WorldCluster{OBB: &l4perception.OrientedBoundingBox{CenterZ: low, Height: -0.5 - low}}
		now := int64(i) * 1e8
		s.scoreVertical(agg, st, tr, cluster, now)
		st.lastAt = now
	}
	if len(agg.baseStep) != 2 || agg.vertRows != 3 {
		t.Fatalf("%d base steps over %d rows, want 2 over 3", len(agg.baseStep), agg.vertRows)
	}
	if got := rms(agg.baseStep); math.Abs(got-0.6) > 1e-6 {
		t.Fatalf("legacy base step RMS %.3f, want the lowest return's 0.6 m hops", got)
	}
	if agg.lowAbove != 0 || agg.lowBelow != 0 {
		t.Fatalf("a legacy base is the lowest return, so nothing lies off it: %+v", agg)
	}
	// The same frames, anchored on a road at -2.3: the base holds, and two
	// of the three lowest returns lie more than 0.5 m above it.
	agg, st = &rowAgg{}, &trackState{}
	for i, low := range lows {
		tr := &l5tracks.TrackedObject{LatestZ: low, OBBHeight: -0.5 - low, BoxZ: -2.3, BoxHeight: 1.8}
		cluster := &l5tracks.WorldCluster{OBB: &l4perception.OrientedBoundingBox{CenterZ: low, Height: -0.5 - low}}
		now := int64(i) * 1e8
		s.scoreVertical(agg, st, tr, cluster, now)
		st.lastAt = now
	}
	if rms(agg.baseStep) != 0 || rms(agg.heightStep) != 0 || agg.lowAbove != 2 {
		t.Fatalf("anchored: base RMS %.3f, height RMS %.3f, %d above; want 0, 0 and 2", rms(agg.baseStep), rms(agg.heightStep), agg.lowAbove)
	}
}
