package lidar

// GroundRemover defines the interface for vertical filtering operations
// that remove road surface and overhead structure points from world-frame
// point clouds before clustering, reducing phantom detections.
type GroundRemover interface {
	// FilterVertical returns only points within the valid detection zone,
	// discarding ground plane and overhead structure returns.
	FilterVertical(worldPts []WorldPoint) []WorldPoint
}

// HeightBandFilter implements vertical filtering using configurable elevation bands.
// Designed for street scenes with approximately level terrain where valid objects
// (vehicles, pedestrians, cyclists) occupy a known vertical band above the road surface.
type HeightBandFilter struct {
	// FloorHeightM defines the lower bound (metres above world Z=0).
	// Points below this are assumed to be road surface returns.
	// Typical value: 0.2m (above wheel contact patches).
	FloorHeightM float64

	// CeilingHeightM defines the upper bound (metres above world Z=0).
	// Points above this are assumed to be overhead structures (signs, bridges, trees).
	// Typical value: 3.0m (tallest vehicle plus margin).
	CeilingHeightM float64

	// Statistics (optional, for tuning and validation)
	pointsProcessed    int64
	pointsInBand       int64
	pointsBelowFloor   int64
	pointsAboveCeiling int64
}

// NewHeightBandFilter constructs a vertical filter with floor and ceiling bounds.
// For street traffic monitoring, recommended defaults are floor=0.2m, ceiling=3.0m.
func NewHeightBandFilter(floorM, ceilingM float64) *HeightBandFilter {
	return &HeightBandFilter{
		FloorHeightM:   floorM,
		CeilingHeightM: ceilingM,
	}
}

// DefaultHeightBandFilter returns a filter configured for typical street scenes:
// - Floor at 0.2m (excludes road surface, includes vehicle chassis)
// - Ceiling at 3.0m (includes trucks, excludes overhead signs)
func DefaultHeightBandFilter() *HeightBandFilter {
	return NewHeightBandFilter(0.2, 3.0)
}

// FilterVertical applies the elevation band test to each point, retaining only
// those within [FloorHeightM, CeilingHeightM]. This operation is in-place unsafe
// (modifies the backing array) but returns a new slice header for safety.
//
// Performance: O(n) single pass with no heap allocations for typical use.
func (f *HeightBandFilter) FilterVertical(worldPts []WorldPoint) []WorldPoint {
	if worldPts == nil || len(worldPts) == 0 {
		return nil
	}
	n := len(worldPts)

	// Compact in-place: iterate through source, copy keepers to destination.
	// This avoids allocating a new slice when most points pass the filter.
	writeIdx := 0
	for readIdx := 0; readIdx < n; readIdx++ {
		pt := worldPts[readIdx]
		f.pointsProcessed++

		// Elevation band test: keep if Z is within [floor, ceiling]
		if pt.Z < f.FloorHeightM {
			f.pointsBelowFloor++
			continue
		}
		if pt.Z > f.CeilingHeightM {
			f.pointsAboveCeiling++
			continue
		}

		// Point passed filter - copy to output position
		f.pointsInBand++
		worldPts[writeIdx] = pt
		writeIdx++
	}

	// Return slice truncated to kept points
	return worldPts[:writeIdx]
}

// Stats returns current filter statistics for monitoring and parameter tuning.
func (f *HeightBandFilter) Stats() (processed, kept, belowFloor, aboveCeiling int64) {
	return f.pointsProcessed, f.pointsInBand, f.pointsBelowFloor, f.pointsAboveCeiling
}

// ResetStats clears accumulated statistics counters.
func (f *HeightBandFilter) ResetStats() {
	f.pointsProcessed = 0
	f.pointsInBand = 0
	f.pointsBelowFloor = 0
	f.pointsAboveCeiling = 0
}
