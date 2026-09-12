package l3grid

import (
	"math"
	"testing"
)

func TestFitGroundSurfaceRecoversConstantGradeDespiteHighOutliers(t *testing.T) {
	points := make([]PointASC, 0, 36)
	for x := -2; x <= 3; x++ {
		for y := -2; y <= 3; y++ {
			points = append(points, PointASC{X: float64(x), Y: float64(y), Z: .1*float64(x) - .05*float64(y) - 3})
		}
	}
	points = append(points, PointASC{X: 0, Y: 0, Z: 8}, PointASC{X: 1, Y: 1, Z: 9})
	surface, err := FitGroundSurface(points)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(surface.A-.1) > .01 || math.Abs(surface.B+.05) > .01 || math.Abs(surface.C+3) > .02 {
		t.Fatalf("surface = %+v, want z=0.1x-0.05y-3", surface)
	}
	if surface.RMSEMetres > .02 {
		t.Fatalf("surface RMSE = %f, want <= .02", surface.RMSEMetres)
	}
}

func TestFitGroundSurfaceRejectsInsufficientAndDegenerateEvidence(t *testing.T) {
	if _, err := FitGroundSurface(nil); err == nil {
		t.Fatal("accepted no evidence")
	}
	points := make([]PointASC, 12)
	for i := range points {
		points[i] = PointASC{X: float64(i), Y: 0, Z: -3}
	}
	if _, err := FitGroundSurface(points); err == nil {
		t.Fatal("accepted a line as a surface")
	}
	if _, err := FitGroundSurfaceFromBackground(nil); err == nil {
		t.Fatal("accepted nil background")
	}
}

func TestFitGroundSurfaceRejectsNonFiniteAndTooNarrowLowEvidence(t *testing.T) {
	points := make([]PointASC, 12)
	for i := range points {
		points[i] = PointASC{X: float64(i), Y: float64(i % 3), Z: -3}
	}
	points[0].Z = math.NaN()
	if _, err := FitGroundSurface(points); err == nil {
		t.Fatal("accepted fewer than twelve finite points")
	}

	// Every z is unique, so the lower quartile cannot make a 12-point
	// surface candidate. This must fail rather than extrapolating a plane
	// from an arbitrarily thin height slice.
	for i := range points {
		points[i] = PointASC{X: float64(i), Y: float64(i % 3), Z: float64(i)}
	}
	if _, err := FitGroundSurface(points); err == nil {
		t.Fatal("accepted too few low-surface candidates")
	}
}

func TestFitGroundSurfaceFromBackgroundFailsClosedBeforeGridEvidence(t *testing.T) {
	manager := NewBackgroundManagerDI(t.Name(), 2, 8, BackgroundParams{}, nil)
	if _, err := FitGroundSurfaceFromBackground(manager); err == nil {
		t.Fatal("accepted an unsettled grid")
	}
	manager.Grid.mu.Lock()
	manager.Grid.SettlingComplete = true
	manager.Grid.mu.Unlock()
	if _, err := FitGroundSurfaceFromBackground(manager); err == nil {
		t.Fatal("accepted a settled grid without ring elevations")
	}
	if err := manager.SetRingElevations([]float64{-0.1, 0.1}); err != nil {
		t.Fatal(err)
	}
	if _, err := FitGroundSurfaceFromBackground(manager); err == nil {
		t.Fatal("accepted a settled but empty grid")
	}
}
