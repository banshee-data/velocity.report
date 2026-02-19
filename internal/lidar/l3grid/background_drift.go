package l3grid

import (
	"fmt"
	"math"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// =============================================================================
// M3.5: Split Streaming Support
// =============================================================================

// CheckForSensorMovement detects if the sensor has moved based on foreground ratio.
// Returns true if a high percentage of points are classified as foreground for
// multiple consecutive frames, suggesting the background model is stale.
func (bm *BackgroundManager) CheckForSensorMovement(mask []bool) bool {
	if bm == nil || len(mask) == 0 {
		return false
	}

	foregroundCount := 0
	for _, isFg := range mask {
		if isFg {
			foregroundCount++
		}
	}

	foregroundRatio := float64(foregroundCount) / float64(len(mask))

	// Get threshold from params, default to 20%
	movementThreshold := float64(bm.Grid.Params.SensorMovementForegroundThreshold)
	if movementThreshold == 0 {
		movementThreshold = 0.20
	}

	// For proper detection, we'd want to track a streak counter
	// For now, just return true if ratio is high (caller should implement streak logic)
	return foregroundRatio > movementThreshold
}

// DriftMetrics contains metrics about background drift.
type DriftMetrics struct {
	DriftingCells int     // Number of cells that have drifted
	AverageDrift  float64 // Average drift magnitude (metres)
	DriftRatio    float64 // Fraction of cells that have drifted
}

// CheckBackgroundDrift monitors how much the background model is shifting.
// Returns true if a significant portion of the grid has drifted beyond threshold.
func (bm *BackgroundManager) CheckBackgroundDrift() (bool, DriftMetrics) {
	if bm == nil || bm.Grid == nil {
		return false, DriftMetrics{}
	}

	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()

	var totalDrift float64
	var driftingCells int
	settledCount := 0

	// Get thresholds from params with sensible defaults
	driftThresholdMeters := float64(g.Params.BackgroundDriftThresholdMeters)
	if driftThresholdMeters == 0 {
		driftThresholdMeters = 0.5
	}

	driftRatioThreshold := float64(g.Params.BackgroundDriftRatioThreshold)
	if driftRatioThreshold == 0 {
		driftRatioThreshold = 0.10
	}

	for _, cell := range g.Cells {
		// Only check settled cells
		if cell.TimesSeenCount < g.Params.LockedBaselineThreshold {
			continue
		}
		settledCount++

		// Check if locked baseline is set
		if cell.LockedBaseline <= 0 {
			continue
		}

		// Calculate drift from locked baseline
		drift := math.Abs(float64(cell.AverageRangeMeters - cell.LockedBaseline))
		if drift > driftThresholdMeters {
			driftingCells++
			totalDrift += drift
		}
	}

	if settledCount == 0 {
		return false, DriftMetrics{}
	}

	driftRatio := float64(driftingCells) / float64(settledCount)
	avgDrift := 0.0
	if driftingCells > 0 {
		avgDrift = totalDrift / float64(driftingCells)
	}

	metrics := DriftMetrics{
		DriftingCells: driftingCells,
		AverageDrift:  avgDrift,
		DriftRatio:    driftRatio,
	}

	// Consider drifted if ratio exceeds threshold
	drifted := driftRatio > driftRatioThreshold

	return drifted, metrics
}

// BackgroundSnapshotData contains the raw data for a background snapshot.
// This is kept in the lidar package to avoid circular imports.
type BackgroundSnapshotData struct {
	SequenceNumber   uint64
	TimestampNanos   int64
	X                []float32
	Y                []float32
	Z                []float32
	Confidence       []uint32
	Rings            int
	AzimuthBins      int
	RingElevations   []float32
	SettlingComplete bool
}

// GenerateBackgroundSnapshot converts the settled background grid to snapshot data.
// This is used for efficient split streaming where background is sent infrequently.
// Returns nil if the grid is not ready (no ring elevations or not settled).
func (bm *BackgroundManager) GenerateBackgroundSnapshot() (*BackgroundSnapshotData, error) {
	if bm == nil || bm.Grid == nil {
		return nil, fmt.Errorf("background manager or grid is nil")
	}

	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Check if we have the necessary data
	if len(g.RingElevations) != g.Rings {
		return nil, fmt.Errorf("ring elevations not configured (have %d, need %d)", len(g.RingElevations), g.Rings)
	}

	// Estimate capacity based on non-zero cells
	estimatedPoints := g.nonzeroCellCount
	if estimatedPoints <= 0 {
		estimatedPoints = 10000 // reasonable default
	}

	// Pre-allocate slices for efficiency
	x := make([]float32, 0, estimatedPoints)
	y := make([]float32, 0, estimatedPoints)
	z := make([]float32, 0, estimatedPoints)
	confidence := make([]uint32, 0, estimatedPoints)

	// Settled threshold: cells must have been seen enough times
	settledThreshold := uint32(10)
	if g.Params.LockedBaselineThreshold > 0 {
		settledThreshold = g.Params.LockedBaselineThreshold
	}

	// Iterate through all cells and extract settled background points
	azBinResDeg := 360.0 / float64(g.AzimuthBins)

	for ring := 0; ring < g.Rings; ring++ {
		elevationDeg := g.RingElevations[ring]

		for azBin := 0; azBin < g.AzimuthBins; azBin++ {
			idx := g.Idx(ring, azBin)
			cell := g.Cells[idx]

			// Skip unsettled or empty cells
			if cell.TimesSeenCount < settledThreshold {
				continue
			}

			// Skip cells with invalid range
			if cell.AverageRangeMeters <= 0 || cell.AverageRangeMeters > 200 {
				continue
			}

			// Convert polar to Cartesian using the same convention as
			// SphericalToCartesian: X=right (sin az), Y=forward (cos az), Z=up.
			azimuthDeg := float64(azBin) * azBinResDeg
			r := float64(cell.AverageRangeMeters)

			xVal, yVal, zVal := l4perception.SphericalToCartesian(r, azimuthDeg, elevationDeg)

			x = append(x, float32(xVal))
			y = append(y, float32(yVal))
			z = append(z, float32(zVal))
			confidence = append(confidence, cell.TimesSeenCount)
		}
	}

	// Prepare ring elevations
	ringElevations := make([]float32, len(g.RingElevations))
	for i, elev := range g.RingElevations {
		ringElevations[i] = float32(elev)
	}

	// Determine sequence number (use snapshot ID or default to 0)
	sequenceNumber := uint64(0)
	if g.SnapshotID != nil {
		sequenceNumber = uint64(*g.SnapshotID)
	}

	snapshot := &BackgroundSnapshotData{
		SequenceNumber:   sequenceNumber,
		TimestampNanos:   time.Now().UnixNano(),
		X:                x,
		Y:                y,
		Z:                z,
		Confidence:       confidence,
		Rings:            g.Rings,
		AzimuthBins:      g.AzimuthBins,
		RingElevations:   ringElevations,
		SettlingComplete: g.SettlingComplete,
	}

	return snapshot, nil
}

// GetBackgroundSequenceNumber returns the current background sequence number.
// This increments whenever the grid is reset (sensor movement, etc.)
func (bm *BackgroundManager) GetBackgroundSequenceNumber() uint64 {
	if bm == nil || bm.Grid == nil {
		return 0
	}

	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.SnapshotID != nil {
		return uint64(*g.SnapshotID)
	}
	return 0
}
