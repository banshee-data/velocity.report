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

func TestMinAreaRectFindsTheBodyAxisOfAnL(t *testing.T) {
	psi := 0.4
	ref := minAreaRect(&l5tracks.WorldCluster{Members: lShape(psi)})
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
