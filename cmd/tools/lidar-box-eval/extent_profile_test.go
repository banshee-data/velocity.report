package main

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// The withheld-frame score places the profile's extent, which need not be
// centred on the reference, at its pose and grows it and the box by the same
// quarter-metre before counting the returns inside each.
func TestWithheldScorePlacesTheProfileAtItsPose(t *testing.T) {
	// A 4 by 2 m outline centred at (11, 5), heading +Y.
	var points []l4perception.WorldPoint
	for i := 0; i <= 40; i++ {
		y := 3 + 0.1*float64(i)
		points = append(points, l4perception.WorldPoint{X: 10, Y: y}, l4perception.WorldPoint{X: 12, Y: y})
	}
	// The profile's reference sits at (11, 4), a metre behind the centre,
	// so its along extent is [-1, 3]; its axis is +Y.
	p := l5tracks.ExtentProfileReading{Known: true, X: 11, Y: 4, PsiRad: math.Pi / 2,
		AlongLo: -1, AlongHi: 3, AcrossLo: -1, AcrossHi: 1, LengthMetres: 4, WidthMetres: 2}
	prof, boxShare, area, n := withheldScore(p, box{x: 11, y: 5, psi: math.Pi / 2, length: 2, width: 2}, points)
	if n != len(points) || prof != 1 {
		t.Errorf("profile holds %.3f of %d returns, want all", prof, n)
	}
	// The 2 by 2 m box, grown to 2.5 m along, holds y in [3.75, 6.25].
	want := 0.0
	for _, q := range points {
		if q.Y >= 3.75-1e-9 && q.Y <= 6.25+1e-9 {
			want++
		}
	}
	if math.Abs(boxShare-want/float64(n)) > 1e-9 {
		t.Errorf("box share %.3f, want %.3f", boxShare, want/float64(n))
	}
	if area != 2 {
		t.Errorf("area ratio %.2f, want 2", area)
	}
}

func TestProfileEndsNamesEachEnd(t *testing.T) {
	p := l5tracks.ExtentProfileReading{Known: true, RearObserved: true, LeftObserved: true}
	if got := profileEnds(p); got != "occo" {
		t.Errorf("ends %q, want occo", got)
	}
	if got := profileEnds(l5tracks.ExtentProfileReading{}); got != "" {
		t.Errorf("unknown profile: ends %q, want none", got)
	}
}

// On a withheld frame the row also scores the profile against the solid
// body's own extent at the solid body's pose (geom0020's restated P4): the
// share of the frame's returns inside it grown by the same quarter-metre,
// and the profile's area over its area.
func TestSolidBodyRowScoresTheSolidBodyExtent(t *testing.T) {
	if got, want := solidBodyRowHeader[len(solidBodyRowHeader)-2:], []string{"wh_sb_share", "wh_sb_area_ratio"}; got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("last columns %q, want %q", got, want)
	}
	var points []l4perception.WorldPoint
	for i := 0; i <= 40; i++ {
		points = append(points, l4perception.WorldPoint{X: 10 + 0.1*float64(i), Y: 0})
	}
	p := l5tracks.ExtentProfileReading{Known: true, X: 12, Y: 0, AlongLo: -2, AlongHi: 2, AcrossLo: -1, AcrossHi: 1, LengthMetres: 4, WidthMetres: 2}
	// A solid body 2 by 1 m at (12, 0): grown, it holds x in [10.75, 13.25].
	_, share, area, n := withheldScore(p, box{x: 12, length: 2, width: 1}, points)
	want := 0.0
	for _, q := range points {
		if q.X >= 10.75-1e-9 && q.X <= 13.25+1e-9 {
			want++
		}
	}
	if n != len(points) || math.Abs(share-want/float64(n)) > 1e-9 || area != 4 {
		t.Errorf("share %.3f (want %.3f), area %.2f (want 4), n %d", share, want/float64(n), area, n)
	}
}
