package main

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// lowerBodyFrame is a vehicle 4.5 × 1.8 m across the line of sight 20 m
// ahead (azimuth 0), its lowest return 1 m above a road 2.3 m below the
// sensor, and one ring per class across its span: each of the six rings
// between the road and that return at the near face (19.1 m) returns one
// class, one ring above the return and one below the road return nothing
// that counts.
func lowerBodyFrame(t *testing.T) (replayeval.FrameTapFrame, *l5tracks.TrackedObject) {
	t.Helper()
	type ring struct {
		channel   int
		elevation float64
		rangeM    float64 // 0: no return in the span
		fg        bool
		z         float64 // with fg: the return's height, overriding the beam's
	}
	rings := []ring{
		{1, -4.5, 19.3, false, 0},   // absorbed: background at the body
		{2, -5.5, 10, false, 0},     // occluded: background well short of it
		{3, -6.5, 0, false, 0},      // dropout
		{4, -6.0, 19.2, true, -2.9}, // clipped: foreground below the floor
		{5, -5.0, 19.2, true, -1.7}, // foreground above the floor
		{6, -6.2, 30, false, 0},     // behind: background past the far side
		{7, -3.0, 19.2, false, 0},   // meets the face above the lowest return
		{8, -8.0, 19.2, false, 0},   // meets the road before the face
	}
	var pts []l2frames.Point
	add := func(r ring, az, rng float64) {
		x, y, z := l2frames.SphericalToCartesian(rng, az, r.elevation)
		if r.fg {
			z = r.z
		}
		pts = append(pts, l2frames.Point{X: x, Y: y, Z: z, Distance: rng, Azimuth: az, Elevation: r.elevation, Channel: r.channel})
	}
	var mask []bool
	for _, r := range rings {
		// Every ring returns behind the sensor too, so its elevation is known.
		add(r, 180.1, 15)
		mask = append(mask, false)
		if r.rangeM == 0 {
			continue
		}
		for k := -40; k < 40; k++ {
			az := math.Mod(360+(float64(k)+0.5)*0.2, 360)
			add(r, az, r.rangeM)
			mask = append(mask, r.fg)
		}
	}
	at := time.Unix(1_700_000_000, 0)
	track := &l5tracks.TrackedObject{TrackID: "car", X: 0, Y: 20, VX: 10, LatestZ: -1.3,
		OBBLength: 4.5, OBBWidth: 1.8, OBBHeight: 1.5}
	surface := &l3grid.RegionalGroundSurface{CellMetres: 10, Global: l3grid.GroundSurface{C: -2.3}}
	return replayeval.FrameTapFrame{At: at, Scored: true, Frame: &l2frames.LiDARFrame{Points: pts}, Mask: mask,
		Tracks: []*l5tracks.TrackedObject{track}, Surface: surface, AzimuthBins: 1800, HeightBandFloor: -2.8,
		Background: l3grid.BackgroundParams{ClosenessSensitivityMultiplier: 2, NoiseRelativeFraction: 0.01,
			SafetyMarginMetres: 0.3}}, track
}

func TestLowerBodyClassifiesEachRingThatShouldMeetTheBody(t *testing.T) {
	f, _ := lowerBodyFrame(t)
	lb := newLowerBody()
	lb.frame(f)
	s := lb.summary()
	b := s.Bins[2] // the centre is 20 m out: the 20 to 30 m bin
	if b.TrackFrames != 1 || s.All.TrackFrames != 1 || s.Far.TrackFrames != 1 {
		t.Fatalf("track frames: bin %d, all %d, from 20 m %d; want 1 each", b.TrackFrames, s.All.TrackFrames, s.Far.TrackFrames)
	}
	n := b.Counts[lbAbsorbed]
	if n < 60 || n > 70 {
		t.Fatalf("absorbed slots %d, want the span's 60 to 70", n)
	}
	for _, c := range lowerBodyClasses {
		if b.Counts[c] != n {
			t.Errorf("%s slots %d, want %d like every ring in the band", c, b.Counts[c], n)
		}
	}
	if b.Slots != 6*n {
		t.Errorf("ring slots %d, want six rings' %d: the rings above the return and below the road count", b.Slots, 6*n)
	}
	if math.Abs(b.Shares[lbDropout]-1.0/6) > 1e-12 {
		t.Errorf("dropout share %v, want 1/6", b.Shares[lbDropout])
	}
	if got := b.LowestAboveSurface[1]; math.Abs(got-1.0) > 1e-6 {
		t.Errorf("lowest return above the surface %v, want 1 m", got)
	}
}

func TestLowerBodySkipsWhatItCannotRead(t *testing.T) {
	f, track := lowerBodyFrame(t)
	lb := newLowerBody()
	f.Scored = false
	lb.frame(f)
	f.Scored = true
	f.Surface = nil
	lb.frame(f)
	if s := lb.summary(); s.All.TrackFrames != 0 || s.WithoutSurface != 1 {
		t.Fatalf("all %d, without surface %d; want the warm-up frame skipped and the surfaceless one counted apart",
			s.All.TrackFrames, s.WithoutSurface)
	}
	f, track = lowerBodyFrame(t)
	track.VX = 1 // under 2 m/s
	lb = newLowerBody()
	lb.frame(f)
	if s := lb.summary(); s.All.TrackFrames != 0 {
		t.Fatal("a slow track was read")
	}
}

func TestRayFootprintAndSpan(t *testing.T) {
	b := l5tracks.PublishedBox{X: 0, Y: 20, Length: 4.5, Width: 1.8}
	tin, tout, hit := rayFootprint(b, 0)
	if !hit || math.Abs(tin-19.1) > 1e-6 || math.Abs(tout-20.9) > 1e-6 {
		t.Fatalf("ray at 0°: %v %v %v, want 19.1 to 20.9", tin, tout, hit)
	}
	if _, _, hit := rayFootprint(b, 10); hit {
		t.Fatal("a ray 10° off the box hit it")
	}
	lo, hi, ok := azimuthSpan(b)
	want := math.Atan2(2.25, 19.1) * 180 / math.Pi
	if !ok || math.Abs(lo+want) > 1e-6 || math.Abs(hi-want) > 1e-6 {
		t.Fatalf("span %v to %v (%v), want ±%v about north, across the 0/360 seam", lo, hi, ok, want)
	}
	// A heading of a quarter turn swaps which extent lies across the ray.
	b.HeadingRad = math.Pi / 2
	if tin, tout, _ := rayFootprint(b, 0); math.Abs(tin-17.75) > 1e-6 || math.Abs(tout-22.25) > 1e-6 {
		t.Fatalf("turned box: %v to %v, want 17.75 to 22.25", tin, tout)
	}
	if _, _, ok := azimuthSpan(l5tracks.PublishedBox{X: 0, Y: 0.2, Length: 4.5, Width: 1.8}); ok {
		t.Fatal("a box over the sensor has a span")
	}
}
