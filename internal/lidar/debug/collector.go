// Package debug provides instrumentation for LiDAR tracking algorithms.
// The DebugCollector captures algorithm internals (association decisions,
// gating thresholds, Kalman residuals) for visualisation and tuning.
package debug

// Pre-allocation capacities for debug frame slices.
// Based on typical street scene complexity:
//   - ~10-20 active tracks × ~2-3 clusters each = ~30-60 association evaluations
//   - ~10-20 active tracks for gating regions and predictions
const (
	defaultAssociationCapacity = 32 // Typical max association candidates per frame
	defaultGatingCapacity      = 16 // Typical max active tracks
	defaultInnovationCapacity  = 16 // Matches gating (one per track update)
	defaultPredictionCapacity  = 16 // Matches gating (one per track predict)
)

// DebugCollector accumulates debug artifacts during a single frame's processing.
// When enabled, it records tracking algorithm internals: which clusters were
// considered for association, gating decisions, Kalman filter innovations, etc.
//
// The collector is stateful: call Record*() methods during processing, then
// Emit() at frame completion to extract the artifacts. Reset() before the next frame.
type DebugCollector struct {
	enabled bool
	current *DebugFrame
}

// DebugFrame contains all debug artifacts for a single frame.
// This structure is exported to the visualiser for overlay rendering.
type DebugFrame struct {
	FrameID uint64

	// Association stage: which cluster-track pairs were evaluated
	AssociationCandidates []AssociationRecord

	// Gating stage: Mahalanobis distance thresholds for each track
	GatingRegions []GatingRegion

	// Kalman update: innovation vectors (measurement - prediction)
	Innovations []KalmanInnovation

	// Kalman predict: state predictions before measurement update
	StatePredictions []StatePrediction
}

// AssociationRecord captures a single cluster-track pairing considered during association.
type AssociationRecord struct {
	ClusterID              int64   // Which cluster
	TrackID                string  // Which track
	MahalanobisDistSquared float32 // Distance metric
	Accepted               bool    // Whether association was accepted
}

// GatingRegion represents a Mahalanobis distance threshold ellipse around a track.
// Used to visualise the association search region.
type GatingRegion struct {
	TrackID     string
	CenterX     float32
	CenterY     float32
	SemiMajorM  float32 // Semi-major axis length (metres)
	SemiMinorM  float32 // Semi-minor axis length (metres)
	RotationRad float32 // Ellipse rotation (radians)
}

// KalmanInnovation represents a measurement residual in the Kalman update step.
type KalmanInnovation struct {
	TrackID     string
	PredictedX  float32 // Predicted position before measurement
	PredictedY  float32
	MeasuredX   float32 // Actual measurement
	MeasuredY   float32
	ResidualMag float32 // ||measured - predicted||
}

// StatePrediction represents a track's predicted state after the predict step
// but before the update step. Useful for debugging motion models.
type StatePrediction struct {
	TrackID string
	X       float32 // Predicted X
	Y       float32 // Predicted Y
	VX      float32 // Predicted VX
	VY      float32 // Predicted VY
}

// NewDebugCollector creates a collector that's initially disabled.
// Call SetEnabled(true) to begin collecting artifacts.
func NewDebugCollector() *DebugCollector {
	return &DebugCollector{
		enabled: false,
		current: nil,
	}
}

// SetEnabled controls whether the collector records artifacts.
// When disabled, all Record*() calls are no-ops for zero overhead.
func (c *DebugCollector) SetEnabled(enabled bool) {
	c.enabled = enabled
	// Don't create a frame automatically - caller must call BeginFrame
}

// IsEnabled returns true if the collector is actively recording.
func (c *DebugCollector) IsEnabled() bool {
	return c.enabled
}

// BeginFrame initialises collection for a new frame.
// Must be called before any Record*() calls.
func (c *DebugCollector) BeginFrame(frameID uint64) {
	if !c.enabled {
		return
	}
	c.current = &DebugFrame{
		FrameID:               frameID,
		AssociationCandidates: make([]AssociationRecord, 0, defaultAssociationCapacity),
		GatingRegions:         make([]GatingRegion, 0, defaultGatingCapacity),
		Innovations:           make([]KalmanInnovation, 0, defaultInnovationCapacity),
		StatePredictions:      make([]StatePrediction, 0, defaultPredictionCapacity),
	}
}

// RecordAssociation captures a cluster-track pairing evaluation.
// Called during the association stage for each candidate pair.
func (c *DebugCollector) RecordAssociation(clusterID int64, trackID string, distSquared float32, accepted bool) {
	if !c.enabled || c.current == nil {
		return
	}
	c.current.AssociationCandidates = append(c.current.AssociationCandidates, AssociationRecord{
		ClusterID:              clusterID,
		TrackID:                trackID,
		MahalanobisDistSquared: distSquared,
		Accepted:               accepted,
	})
}

// RecordGatingRegion captures the Mahalanobis gating ellipse for a track.
// Called after computing the innovation covariance matrix.
func (c *DebugCollector) RecordGatingRegion(trackID string, centerX, centerY, semiMajor, semiMinor, rotation float32) {
	if !c.enabled || c.current == nil {
		return
	}
	c.current.GatingRegions = append(c.current.GatingRegions, GatingRegion{
		TrackID:     trackID,
		CenterX:     centerX,
		CenterY:     centerY,
		SemiMajorM:  semiMajor,
		SemiMinorM:  semiMinor,
		RotationRad: rotation,
	})
}

// RecordInnovation captures a Kalman filter innovation (measurement residual).
// Called during the update step after computing predicted vs. measured state.
func (c *DebugCollector) RecordInnovation(trackID string, predX, predY, measX, measY, residualMag float32) {
	if !c.enabled || c.current == nil {
		return
	}
	c.current.Innovations = append(c.current.Innovations, KalmanInnovation{
		TrackID:     trackID,
		PredictedX:  predX,
		PredictedY:  predY,
		MeasuredX:   measX,
		MeasuredY:   measY,
		ResidualMag: residualMag,
	})
}

// RecordPrediction captures a track's predicted state before measurement update.
// Called during the predict step for each active track.
func (c *DebugCollector) RecordPrediction(trackID string, x, y, vx, vy float32) {
	if !c.enabled || c.current == nil {
		return
	}
	c.current.StatePredictions = append(c.current.StatePredictions, StatePrediction{
		TrackID: trackID,
		X:       x,
		Y:       y,
		VX:      vx,
		VY:      vy,
	})
}

// Emit returns the accumulated debug frame and prepares for the next frame.
// Returns nil if collection is disabled or no frame was begun.
func (c *DebugCollector) Emit() *DebugFrame {
	if !c.enabled || c.current == nil {
		return nil
	}
	frame := c.current
	c.current = nil // Clear for next frame (caller must BeginFrame again)
	return frame
}

// Reset clears any pending artifacts without emitting them.
// Useful when aborting frame processing.
func (c *DebugCollector) Reset() {
	c.current = nil
}
