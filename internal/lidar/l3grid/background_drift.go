package l3grid

import (
	"fmt"
	"math"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// =============================================================================
// M3.5: Split Streaming Support
// =============================================================================

const defaultSensorMovementDriftRatioThreshold = 0.35

// SensorMotionEvidence is the raw, per-frame evidence used to classify sensor
// ego-motion. Timeline hysteresis belongs to callers such as pcap-split.
type SensorMotionEvidence struct {
	ForegroundFraction float64
	DriftRatio         float64
	Moving             bool
}

// EvaluateSensorMotion applies the shared L3 sensor-motion classifier. A
// foreground spike detects an abrupt scene change; background-drift ratio — the
// fraction of settled cells whose range has shifted from its locked baseline —
// is the robust sustained-motion signal: driving shifts most of the grid, while
// a parked sensor (even one watching heavy traffic) shifts only the cells the
// traffic crosses. Scene-activity signals like foreground fraction or noise
// deviation conflate a busy parked scene with driving; drift ratio does not.
func (bm *BackgroundManager) EvaluateSensorMotion(mask []bool) SensorMotionEvidence {
	if bm == nil || bm.Grid == nil || len(mask) == 0 {
		return SensorMotionEvidence{}
	}

	foregroundCount := 0
	for _, isForeground := range mask {
		if isForeground {
			foregroundCount++
		}
	}

	foregroundThreshold := 0.20
	driftRatioThreshold := defaultSensorMovementDriftRatioThreshold
	g := bm.Grid
	g.mu.RLock()
	if v := float64(g.Params.SensorMovementForegroundThreshold); v > 0 {
		foregroundThreshold = v
	}
	if v := float64(g.Params.SensorMovementDriftRatioThreshold); v > 0 {
		driftRatioThreshold = v
	}
	g.mu.RUnlock()

	// CheckBackgroundDrift takes its own RLock, so it must be called after the
	// threshold read above has released the lock (RWMutex RLock is not reentrant).
	_, drift := bm.CheckBackgroundDrift()
	evidence := SensorMotionEvidence{
		ForegroundFraction: float64(foregroundCount) / float64(len(mask)),
		DriftRatio:         drift.DriftRatio,
	}
	evidence.Moving = evidence.ForegroundFraction >= foregroundThreshold ||
		evidence.DriftRatio >= driftRatioThreshold
	return evidence
}

// CheckForSensorMovement reports the shared raw motion decision. Callers that
// require consecutive-frame handling must apply their own hysteresis.
func (bm *BackgroundManager) CheckForSensorMovement(mask []bool) bool {
	return bm.EvaluateSensorMotion(mask).Moving
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
	driftThresholdMetres := float64(g.Params.BackgroundDriftThresholdMetres)
	if driftThresholdMetres == 0 {
		driftThresholdMetres = 0.5
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
		if drift > driftThresholdMetres {
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

			xVal, yVal, zVal := l2frames.SphericalToCartesian(r, azimuthDeg, elevationDeg)

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
