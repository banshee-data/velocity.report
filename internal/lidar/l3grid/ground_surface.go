package l3grid

import (
	"fmt"
	"math"
	"sort"
)

// GroundSurface is a local site-frame plane z = AX + BY + C fitted from the
// settled L3 background. It deliberately models only a coarse road surface;
// kerbs, steps and intersecting approaches remain evidence of an ambiguous
// surface rather than an excuse to force one global ground height.
type GroundSurface struct {
	A, B, C       float64
	Support       int
	RMSEMetres    float64
	GradientMetre float64
}

// HeightAt returns the expected ground height at a site-frame position.
func (s GroundSurface) HeightAt(x, y float64) float64 { return s.A*x + s.B*y + s.C }

// FitGroundSurface fits a plane to the low, stable portion of a settled
// background. A first lower-quartile pass excludes most walls and awnings;
// a second fit drops residual outliers. The caller must supply points from a
// settled background, not foreground returns from a live frame.
func FitGroundSurface(points []PointASC) (GroundSurface, error) {
	if len(points) < 12 {
		return GroundSurface{}, fmt.Errorf("at least 12 settled background points are required")
	}
	usable := make([]PointASC, 0, len(points))
	for _, point := range points {
		if finite(point.X) && finite(point.Y) && finite(point.Z) {
			usable = append(usable, point)
		}
	}
	if len(usable) < 12 {
		return GroundSurface{}, fmt.Errorf("settled background has fewer than 12 finite points")
	}
	z := make([]float64, len(usable))
	for i, point := range usable {
		z[i] = point.Z
	}
	sort.Float64s(z)
	cutoff := z[(len(z)-1)/4]
	candidates := make([]PointASC, 0, len(usable)/4)
	for _, point := range usable {
		if point.Z <= cutoff {
			candidates = append(candidates, point)
		}
	}
	if len(candidates) < 12 {
		return GroundSurface{}, fmt.Errorf("settled background has too few low-surface candidates")
	}
	first, err := fitPlane(candidates)
	if err != nil {
		return GroundSurface{}, err
	}
	residuals := make([]float64, len(candidates))
	for i, point := range candidates {
		residuals[i] = math.Abs(point.Z - first.HeightAt(point.X, point.Y))
	}
	sort.Float64s(residuals)
	median := residuals[len(residuals)/2]
	threshold := math.Max(0.05, 3*median)
	inliers := make([]PointASC, 0, len(candidates))
	for _, point := range candidates {
		if math.Abs(point.Z-first.HeightAt(point.X, point.Y)) <= threshold {
			inliers = append(inliers, point)
		}
	}
	if len(inliers) < 12 {
		return GroundSurface{}, fmt.Errorf("ground fit rejected too many settled background points")
	}
	return fitPlane(inliers)
}

// FitGroundSurfaceFromBackground turns the settled L3 grid into the points
// used for the fit. It fails closed until the grid has ring elevations and has
// declared itself settled: projecting a 2D range grid onto z=0 would fabricate
// a flat surface precisely where the remedy is needed.
func FitGroundSurfaceFromBackground(manager *BackgroundManager) (GroundSurface, error) {
	if manager == nil || manager.Grid == nil {
		return GroundSurface{}, fmt.Errorf("background manager or grid is nil")
	}
	if !manager.IsSettlingComplete() {
		return GroundSurface{}, fmt.Errorf("background is not settled")
	}
	grid := manager.Grid
	grid.mu.RLock()
	hasElevations := len(grid.RingElevations) == grid.Rings && grid.Rings > 0
	grid.mu.RUnlock()
	if !hasElevations {
		return GroundSurface{}, fmt.Errorf("settled background has no ring elevations")
	}
	return FitGroundSurface(manager.ToASCPoints())
}

func fitPlane(points []PointASC) (GroundSurface, error) {
	var sx, sy, sz, sxx, syy, sxy, sxz, syz float64
	for _, point := range points {
		sx += point.X
		sy += point.Y
		sz += point.Z
		sxx += point.X * point.X
		syy += point.Y * point.Y
		sxy += point.X * point.Y
		sxz += point.X * point.Z
		syz += point.Y * point.Z
	}
	n := float64(len(points))
	// Solve normal equations for [A B C] with Cramer's rule. This small,
	// deterministic system avoids a dependency on a general matrix package.
	det := sxx*(syy*n-sy*sy) - sxy*(sxy*n-sy*sx) + sx*(sxy*sy-syy*sx)
	if math.Abs(det) < 1e-9 {
		return GroundSurface{}, fmt.Errorf("ground candidates do not span a surface")
	}
	detA := sxz*(syy*n-sy*sy) - sxy*(syz*n-sy*sz) + sx*(syz*sy-syy*sz)
	detB := sxx*(syz*n-sy*sz) - sxz*(sxy*n-sy*sx) + sx*(sxy*sz-syz*sx)
	detC := sxx*(syy*sz-syz*sy) - sxy*(sxy*sz-syz*sx) + sxz*(sxy*sy-syy*sx)
	surface := GroundSurface{A: detA / det, B: detB / det, C: detC / det, Support: len(points)}
	var sumSq float64
	for _, point := range points {
		delta := point.Z - surface.HeightAt(point.X, point.Y)
		sumSq += delta * delta
	}
	surface.RMSEMetres = math.Sqrt(sumSq / float64(len(points)))
	surface.GradientMetre = math.Hypot(surface.A, surface.B)
	return surface, nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

// DefaultRegionSizeMetres is the per-side size of a ground-plane region cell
// when the caller does not specify one. Large enough that a typical settled
// background gives a cell 12+ points to fit its own plane; small enough to
// separate a road segment from an adjoining driveway apron or shoulder at a
// grade change.
const DefaultRegionSizeMetres = 10.0

// regionKey identifies one square cell of a region grid, floor-divided from a
// site-frame position by the grid's cell size. Two positions share a key
// exactly when they fall in the same cell, so the map lookup is deterministic
// and independent of point insertion order.
type regionKey struct{ ix, iy int }

func regionKeyFor(x, y, cellMetres float64) regionKey {
	return regionKey{ix: int(math.Floor(x / cellMetres)), iy: int(math.Floor(y / cellMetres))}
}

// RegionalGroundSurface generalises GroundSurface from one plane for the
// whole capture to a grid of independently fitted local planes, so a crest,
// valley, or a driveway apron meeting the road at a different grade does not
// have to share one global slope. A cell without enough settled background
// points to fit its own plane falls back to Global, so this is a strict
// generalisation of the single-plane fit: a capture small enough to fit one
// cell behaves exactly as GroundSurface did.
type RegionalGroundSurface struct {
	CellMetres float64
	Global     GroundSurface
	// RegionCount is the number of cells with their own locally-fitted
	// plane, for logging; it does not include cells served by Global.
	RegionCount int
	regions     map[regionKey]GroundSurface
}

// HeightAt returns the expected ground height at a site-frame position: the
// local region's plane if one was fitted, otherwise Global's.
func (s RegionalGroundSurface) HeightAt(x, y float64) float64 {
	if surface, ok := s.regions[regionKeyFor(x, y, s.CellMetres)]; ok {
		return surface.HeightAt(x, y)
	}
	return s.Global.HeightAt(x, y)
}

// FitRegionalGroundSurface fits one global plane exactly as FitGroundSurface
// does, then independently re-fits every cellMetres x cellMetres cell that
// has at least enough settled background points of its own, using the same
// two-pass robust method per cell. cellMetres <= 0 uses
// DefaultRegionSizeMetres. Fails exactly when FitGroundSurface would: the
// global fit is not optional, since every cell without its own evidence
// depends on it.
func FitRegionalGroundSurface(points []PointASC, cellMetres float64) (RegionalGroundSurface, error) {
	global, err := FitGroundSurface(points)
	if err != nil {
		return RegionalGroundSurface{}, err
	}
	if cellMetres <= 0 {
		cellMetres = DefaultRegionSizeMetres
	}

	byCell := make(map[regionKey][]PointASC)
	for _, point := range points {
		if !finite(point.X) || !finite(point.Y) || !finite(point.Z) {
			continue
		}
		key := regionKeyFor(point.X, point.Y, cellMetres)
		byCell[key] = append(byCell[key], point)
	}

	regions := make(map[regionKey]GroundSurface, len(byCell))
	for key, cellPoints := range byCell {
		surface, err := FitGroundSurface(cellPoints)
		if err != nil {
			// Too few points, or too narrow a height band, to trust a
			// cell-local fit. Leave this cell to the global fallback
			// rather than extrapolating from thin per-cell evidence.
			continue
		}
		regions[key] = surface
	}

	return RegionalGroundSurface{
		CellMetres:  cellMetres,
		Global:      global,
		RegionCount: len(regions),
		regions:     regions,
	}, nil
}

// FitRegionalGroundSurfaceFromBackground is FitGroundSurfaceFromBackground's
// regional counterpart: it applies the same settling and ring-elevation
// guards before turning the background into per-cell plane fits.
func FitRegionalGroundSurfaceFromBackground(manager *BackgroundManager, cellMetres float64) (RegionalGroundSurface, error) {
	if manager == nil || manager.Grid == nil {
		return RegionalGroundSurface{}, fmt.Errorf("background manager or grid is nil")
	}
	if !manager.IsSettlingComplete() {
		return RegionalGroundSurface{}, fmt.Errorf("background is not settled")
	}
	grid := manager.Grid
	grid.mu.RLock()
	hasElevations := len(grid.RingElevations) == grid.Rings && grid.Rings > 0
	grid.mu.RUnlock()
	if !hasElevations {
		return RegionalGroundSurface{}, fmt.Errorf("settled background has no ring elevations")
	}
	return FitRegionalGroundSurface(manager.ToASCPoints(), cellMetres)
}
