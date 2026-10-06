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

// flatRegion returns a 5x5 grid of points at a constant height, standing in
// for one locally flat patch of settled background.
func flatRegion(originX, originY, z float64) []PointASC {
	points := make([]PointASC, 0, 25)
	for x := 0; x < 5; x++ {
		for y := 0; y < 5; y++ {
			points = append(points, PointASC{X: originX + float64(x), Y: originY + float64(y), Z: z})
		}
	}
	return points
}

func TestFitRegionalGroundSurfaceRecoversACrestNoSinglePlaneCanFit(t *testing.T) {
	// Three flat regions in a row: two at -3.0m bracketing one 1m higher, at
	// x=0-4, x=50-54, x=100-104. No single plane fits "up then back down";
	// a global linear fit is pulled toward the symmetric mean of all three
	// (~-2.67m) at every one of them.
	var points []PointASC
	points = append(points, flatRegion(0, 0, -3.0)...)
	points = append(points, flatRegion(50, 0, -2.0)...)
	points = append(points, flatRegion(100, 0, -3.0)...)

	regional, err := FitRegionalGroundSurface(points, 10)
	if err != nil {
		t.Fatal(err)
	}
	if regional.RegionCount != 3 {
		t.Fatalf("region count = %d, want 3 (one per flat patch)", regional.RegionCount)
	}
	for _, tc := range []struct{ x, y, want float64 }{
		{2, 2, -3.0},
		{52, 2, -2.0},
		{102, 2, -3.0},
	} {
		if got := regional.HeightAt(tc.x, tc.y); math.Abs(got-tc.want) > 1e-6 {
			t.Errorf("HeightAt(%.0f,%.0f) = %f, want %f", tc.x, tc.y, got, tc.want)
		}
	}
	// The crest region is exactly where a single global plane cannot keep
	// up: it must be pulled well away from the true -2.0m by the two
	// bracketing regions it shares a plane with.
	if globalAtCrest := regional.Global.HeightAt(52, 2); math.Abs(globalAtCrest-(-2.0)) < 0.3 {
		t.Fatalf("global-only fit at the crest = %f, expected it to miss -2.0m by more than 0.3m", globalAtCrest)
	}
}

func TestFitRegionalGroundSurfaceFallsBackToGlobalForSparseCells(t *testing.T) {
	var points []PointASC
	points = append(points, flatRegion(0, 0, -3.0)...)
	// Ten metres per cell puts these three points in a cell of their own,
	// far short of the twelve a local fit requires.
	points = append(points, PointASC{X: 200, Y: 0, Z: -5.0}, PointASC{X: 201, Y: 0, Z: -5.0}, PointASC{X: 202, Y: 0, Z: -5.0})

	regional, err := FitRegionalGroundSurface(points, 10)
	if err != nil {
		t.Fatal(err)
	}
	if regional.RegionCount != 1 {
		t.Fatalf("region count = %d, want 1 (the sparse cell has too few points to fit its own plane)", regional.RegionCount)
	}
	if got := regional.HeightAt(2, 2); math.Abs(got-(-3.0)) > 1e-6 {
		t.Fatalf("HeightAt in the well-populated region = %f, want -3.0", got)
	}
	if got, want := regional.HeightAt(201, 0), regional.Global.HeightAt(201, 0); got != want {
		t.Fatalf("HeightAt in the sparse region = %f, want the global fallback %f exactly", got, want)
	}
}

func TestFitRegionalGroundSurfaceSingleCellMatchesGlobalFit(t *testing.T) {
	points := flatRegion(0, 0, -3.0)
	regional, err := FitRegionalGroundSurface(points, 10)
	if err != nil {
		t.Fatal(err)
	}
	if regional.RegionCount != 1 {
		t.Fatalf("region count = %d, want 1 (every point falls in one cell)", regional.RegionCount)
	}
	// A capture entirely within one region cell has nothing left for that
	// cell's local fit to disagree with the global fit about: both are
	// fit from the exact same points.
	for _, x := range []float64{0, 2, 4} {
		if got, want := regional.HeightAt(x, 2), regional.Global.HeightAt(x, 2); math.Abs(got-want) > 1e-9 {
			t.Fatalf("HeightAt(%.0f,2) = %f, want the global fit's %f", x, got, want)
		}
	}
}

func TestFitRegionalGroundSurfaceDefaultsCellSize(t *testing.T) {
	regional, err := FitRegionalGroundSurface(flatRegion(0, 0, -3.0), 0)
	if err != nil {
		t.Fatal(err)
	}
	if regional.CellMetres != DefaultRegionSizeMetres {
		t.Fatalf("cell size = %f, want the default %f when 0 is passed", regional.CellMetres, DefaultRegionSizeMetres)
	}
}

func TestFitRegionalGroundSurfaceRejectsInsufficientEvidence(t *testing.T) {
	if _, err := FitRegionalGroundSurface(nil, 0); err == nil {
		t.Fatal("accepted no evidence")
	}
}

func TestFitRegionalGroundSurfaceFromBackgroundFailsClosedBeforeGridEvidence(t *testing.T) {
	manager := NewBackgroundManagerDI(t.Name(), 2, 8, BackgroundParams{}, nil)
	if _, err := FitRegionalGroundSurfaceFromBackground(manager, 0); err == nil {
		t.Fatal("accepted an unsettled grid")
	}
	manager.Grid.mu.Lock()
	manager.Grid.SettlingComplete = true
	manager.Grid.mu.Unlock()
	if _, err := FitRegionalGroundSurfaceFromBackground(manager, 0); err == nil {
		t.Fatal("accepted a settled grid without ring elevations")
	}
	if err := manager.SetRingElevations([]float64{-0.1, 0.1}); err != nil {
		t.Fatal(err)
	}
	if _, err := FitRegionalGroundSurfaceFromBackground(manager, 0); err == nil {
		t.Fatal("accepted a settled but empty grid")
	}
	if _, err := FitRegionalGroundSurfaceFromBackground(nil, 0); err == nil {
		t.Fatal("accepted nil manager")
	}
}
