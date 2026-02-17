package l4perception

// GroundRemover defines the interface for vertical filtering operations
// that remove road surface and overhead structure points from point clouds
// before clustering, reducing phantom detections.
//
// IMPORTANT — coordinate frame: unless a real sensor→world pose transform
// is applied beforehand, Z=0 is the sensor's horizontal plane, NOT the
// ground surface. Filter bounds must be expressed in the same frame as the
// input points. For a sensor mounted ~3 m above ground level with an
// identity pose, the ground surface sits at approximately Z = −3.0 m.
type GroundRemover interface {
	// FilterVertical returns only points within the valid detection zone,
	// discarding ground plane and overhead structure returns.
	FilterVertical(worldPts []WorldPoint) []WorldPoint
}

// HeightBandFilter implements vertical filtering using configurable elevation bands.
// Designed for street scenes with approximately level terrain where valid objects
// (vehicles, pedestrians, cyclists) occupy a known vertical band.
//
// When operating in sensor frame (identity pose), typical values for a sensor
// mounted ~3 m above road level are floor ≈ −2.8 m and ceiling ≈ +1.5 m.
type HeightBandFilter struct {
	// FloorHeightM defines the lower bound (metres in the input frame).
	// Points below this are assumed to be road surface returns.
	FloorHeightM float64

	// CeilingHeightM defines the upper bound (metres in the input frame).
	// Points above this are assumed to be overhead structures (signs, bridges, trees).
	CeilingHeightM float64

	// Statistics (optional, for tuning and validation)
	pointsProcessed    int64
	pointsInBand       int64
	pointsBelowFloor   int64
	pointsAboveCeiling int64
}

// NewHeightBandFilter constructs a vertical filter with floor and ceiling bounds.
// Bounds are in the same coordinate frame as the input points. For sensor-frame
// operation with a ~3 m mount height, recommended defaults are floor=−2.8 m,
// ceiling=+1.5 m.
func NewHeightBandFilter(floorM, ceilingM float64) *HeightBandFilter {
	return &HeightBandFilter{
		FloorHeightM:   floorM,
		CeilingHeightM: ceilingM,
	}
}

// DefaultHeightBandFilter returns a filter configured for a sensor mounted
// approximately 3 m above road level, operating in sensor frame (identity
// pose, Z=0 at the sensor's horizontal plane):
//   - Floor at −2.8 m (≈ 0.2 m above road surface, excludes ground returns)
//   - Ceiling at +1.5 m (includes tall trucks, excludes overhead structures)
func DefaultHeightBandFilter() *HeightBandFilter {
	return NewHeightBandFilter(-2.8, 1.5)
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
