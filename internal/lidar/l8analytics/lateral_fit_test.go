package l8analytics

import (
	"math"
	"testing"
)

// straightPass is a track at speed m/s along +X, lateral offset y, sampled
// every 100 ms.
func straightPass(id string, speed float64, n int, y float32) LateralFitTrack {
	track := LateralFitTrack{ID: id, MaxSpeedMps: speed}
	for i := 0; i < n; i++ {
		track.Points = append(track.Points, SeriesPoint{
			TimestampNanos: int64(i) * 100_000_000,
			X:              float32(speed * float64(i) * 0.1),
			Y:              y,
		})
	}
	return track
}

func TestLateralFitOfAStraightPassIsZero(t *testing.T) {
	s := SummariseLateralFit([]LateralFitTrack{straightPass("a", 12, 20, 5)})
	if s.Method != LateralFitMethod || s.MovingTracks != 1 || s.ScoredTracks != 1 || s.Windows != 16 {
		t.Fatalf("summary %+v", s)
	}
	if s.MaxMetres > 1e-5 || s.TracksWithExcursion != 0 {
		t.Fatalf("a straight pass has residual %v and %d excursions", s.MaxMetres, s.TracksWithExcursion)
	}
}

func TestLateralFitAttenuatesAHopByTheCentresOwnWeight(t *testing.T) {
	// The centre point is one of the five it is fitted against, so a lateral
	// hop of d at the centre reads as d - d/5: the method's documented
	// attenuation, and the reason it is a proxy.
	track := straightPass("hop", 12, 11, 5)
	track.Points[5].Y += 1
	s := SummariseLateralFit([]LateralFitTrack{track})
	if math.Abs(s.MaxMetres-0.8) > 1e-5 {
		t.Fatalf("max residual %v for a 1 m hop, want 0.8", s.MaxMetres)
	}
	if s.TracksWithExcursion != 1 || s.ExcursionShare != 1 {
		t.Fatalf("excursions %d, share %v", s.TracksWithExcursion, s.ExcursionShare)
	}
}

func TestLateralFitScoresOnlyMovingTracksAndMovingWindows(t *testing.T) {
	slowLifetime := straightPass("slow-lifetime", 5, 20, 0)
	slowWindows := straightPass("slow-windows", 1.5, 20, 0)
	slowWindows.MaxSpeedMps = 10 // lifetime says moving; every window is below 2 m/s
	s := SummariseLateralFit([]LateralFitTrack{slowLifetime, slowWindows})
	if s.MovingTracks != 1 || s.ScoredTracks != 0 || s.Windows != 0 {
		t.Fatalf("summary %+v: want one moving track with no eligible window", s)
	}
	if s.ExcursionShare != 0 {
		t.Fatalf("excursion share %v with nothing scored", s.ExcursionShare)
	}
}

func TestLateralFitSplitsAtGaps(t *testing.T) {
	// A 0.4 s gap after the fourth sample leaves four and sixteen: only the
	// second run can hold a five-point window.
	track := straightPass("gap", 12, 20, 0)
	for i := 4; i < len(track.Points); i++ {
		track.Points[i].TimestampNanos += 300_000_000
	}
	s := SummariseLateralFit([]LateralFitTrack{track})
	if s.Windows != 12 {
		t.Fatalf("windows %d across a gap, want 12", s.Windows)
	}
}

func TestLateralFitPercentilesAreNearestRank(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for p, want := range map[float64]float64{50: 5, 95: 10, 99: 10, 10: 1, 0: 1} {
		if got := nearestRank(sorted, p); got != want {
			t.Errorf("p%v = %v, want %v", p, got, want)
		}
	}
}
