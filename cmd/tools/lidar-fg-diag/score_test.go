package main

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

func TestFindModes_PersistentSurfacesOnly(t *testing.T) {
	var ranges []float32
	for i := 0; i < 80; i++ {
		ranges = append(ranges, 30+float32(i%5)*0.02)
	}
	for i := 0; i < 15; i++ {
		ranges = append(ranges, 12+float32(i%3)*0.02)
	}
	// Traffic: scattered, on no surface.
	for i := 0; i < 5; i++ {
		ranges = append(ranges, 5+2.3*float32(i))
	}
	c := findModes(ranges, 100, 0.08)
	if math.Abs(float64(c.Modes[0]-30)) > 0.1 || math.Abs(float64(c.Modes[1]-12)) > 0.1 || c.Modes[2] != 0 {
		t.Fatalf("modes %v", c.Modes)
	}
	if c.Hits < 0.9 {
		t.Fatalf("hits %v", c.Hits)
	}
	if got := findModes(nil, 100, 0.08); got.Count != 0 {
		t.Fatalf("empty cell: %+v", got)
	}
}

func TestOracleLabel(t *testing.T) {
	c := oracleCell{Modes: [oracleModes]float32{30, 12}, Hits: 0.9, Count: 100}
	for _, tc := range []struct {
		r    float32
		want uint8
	}{{30.05, labelStatic}, {12.1, labelStatic}, {20, labelTransient}, {6, labelTransient}, {45, labelUnknown}} {
		if got := oracleLabel(&c, tc.r); got != tc.want {
			t.Errorf("r=%v: label %d, want %d", tc.r, got, tc.want)
		}
	}
	if got := oracleLabel(&oracleCell{}, 10); got != labelUnknown {
		t.Errorf("empty cell: %d", got)
	}
	vegetation := oracleCell{Modes: [oracleModes]float32{30}, Hits: 0.3, Count: 100}
	if got := oracleLabel(&vegetation, 20); got != labelUnknown {
		t.Errorf("mostly impersistent cell: %d", got)
	}
}

func TestOnStructure_SeparatesAQueueFromAWall(t *testing.T) {
	c := oracleCell{Modes: [oracleModes]float32{30, 12}, Support: [oracleModes]float32{0.7, 0.2}}
	if !onStructure(&c, 30) {
		t.Fatal("the wall is structure")
	}
	if onStructure(&c, 12) {
		t.Fatal("a vehicle present for a fifth of the capture is not structure")
	}
}

func TestCellOf(t *testing.T) {
	p := l2frames.Point{Channel: 2, Azimuth: 359.99}
	if got, want := cellOf(&p), oracleBins+oracleBins-1; got != want {
		t.Fatalf("cell %d, want %d", got, want)
	}
	p.Azimuth = -0.1
	if got, want := cellOf(&p), oracleBins+oracleBins-1; got != want {
		t.Fatalf("negative azimuth: cell %d, want %d", got, want)
	}
	p.Channel = 41
	if got := cellOf(&p); got != -1 {
		t.Fatalf("out of range channel: %d", got)
	}
}

func TestJitter_ReadsTheOutlierAcrossTheTrack(t *testing.T) {
	var clean, noisy []sample
	for i := 0; i < 40; i++ {
		s := sample{t: float64(i) * 0.1, x: float32(i) * 0.5}
		clean = append(clean, s)
		if i%2 == 0 {
			s.y = 0.2
		}
		noisy = append(noisy, s)
	}
	if j, _, ok := jitter(clean); !ok || j > 1e-3 {
		t.Fatalf("a straight track has jitter %v (ok %v)", j, ok)
	}
	j, c, ok := jitter(noisy)
	if !ok || j < 0.1 || math.Abs(c-j) > 0.05 {
		t.Fatalf("alternating lateral noise: jitter %v, cross %v, ok %v", j, c, ok)
	}
	if _, _, ok := jitter(clean[:5]); ok {
		t.Fatal("a short track was measured")
	}
}

func TestPercentile(t *testing.T) {
	if got := percentile([]float64{3, 1, 2}, 0.5); got != 2 {
		t.Fatalf("median %v", got)
	}
	if got := percentile(nil, 0.5); got != 0 {
		t.Fatalf("empty %v", got)
	}
}
