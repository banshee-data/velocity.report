package lidar

import (
	"math"
	"sync"
	"time"
)

// VelocityEstimationConfig holds configuration for point-level velocity estimation.
type VelocityEstimationConfig struct {
	// SearchRadius is the maximum distance (meters) to search for correspondences
	SearchRadius float64

	// MaxVelocityMps is the maximum plausible velocity (m/s)
	MaxVelocityMps float64

	// VelocityVarianceThreshold is the maximum velocity variance (m/s) for neighbor consistency
	VelocityVarianceThreshold float64

	// MinConfidence is the minimum velocity confidence to accept
	MinConfidence float32

	// NeighborRadius is the radius (meters) for local velocity context estimation
	NeighborRadius float64

	// MinNeighborsForContext is the minimum number of neighbors needed for context
	MinNeighborsForContext int
}

// DefaultVelocityEstimationConfig returns sensible defaults for velocity estimation.
func DefaultVelocityEstimationConfig() VelocityEstimationConfig {
	return VelocityEstimationConfig{
		SearchRadius:              2.0,
		MaxVelocityMps:            50.0, // Vehicle max speed
		VelocityVarianceThreshold: 2.0,
		MinConfidence:             0.3,
		NeighborRadius:            1.0,
		MinNeighborsForContext:    3,
	}
}

// PointVelocity represents a point with estimated velocity from frame correspondence.
type PointVelocity struct {
	// Position (world frame)
	X, Y, Z float64

	// Estimated velocity (m/s)
	VX, VY, VZ float64

	// Velocity confidence [0, 1]
	VelocityConfidence float32

	// Correspondence metadata
	CorrespondingPointIdx int   // Index in previous frame (-1 if none)
	TimestampNanos        int64 // Point timestamp

	// Original point data
	Intensity uint8
	SensorID  string
}

// PointVelocityFrame represents a frame of points with velocities and spatial index.
type PointVelocityFrame struct {
	Points       []PointVelocity
	Timestamp    time.Time
	SpatialIndex *SpatialIndex
}

// FrameHistory maintains a sliding window of recent frames for velocity estimation.
type FrameHistory struct {
	frames     []PointVelocityFrame
	capacity   int
	writeIndex int
	mu         sync.RWMutex
}

// NewFrameHistory creates a new frame history with the given capacity.
func NewFrameHistory(capacity int) *FrameHistory {
	if capacity <= 0 {
		capacity = 5 // Default to 5 frames
	}
	return &FrameHistory{
		frames:   make([]PointVelocityFrame, capacity),
		capacity: capacity,
	}
}

// Add adds a new frame to the history.
func (h *FrameHistory) Add(frame PointVelocityFrame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frames[h.writeIndex] = frame
	h.writeIndex = (h.writeIndex + 1) % h.capacity
}

// Previous returns the frame at the given offset from the most recent.
// offset=0 returns the most recent frame, offset=1 returns the previous, etc.
func (h *FrameHistory) Previous(offset int) *PointVelocityFrame {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if offset >= h.capacity {
		return nil
	}
	idx := (h.writeIndex - 1 - offset + h.capacity) % h.capacity
	frame := &h.frames[idx]
	if frame.Timestamp.IsZero() {
		return nil // Frame not yet populated
	}
	return frame
}

// VelocityEstimator computes per-point velocities from frame-to-frame correspondence.
type VelocityEstimator struct {
	Config  VelocityEstimationConfig
	History *FrameHistory
	mu      sync.RWMutex
}

// NewVelocityEstimator creates a new velocity estimator with the given configuration.
func NewVelocityEstimator(config VelocityEstimationConfig, historyCapacity int) *VelocityEstimator {
	return &VelocityEstimator{
		Config:  config,
		History: NewFrameHistory(historyCapacity),
	}
}

// EstimateVelocities computes per-point velocities for the current frame.
// It uses frame-to-frame correspondence to estimate velocity vectors.
func (ve *VelocityEstimator) EstimateVelocities(
	currentPoints []WorldPoint,
	timestamp time.Time,
	sensorID string,
) []PointVelocity {
	if len(currentPoints) == 0 {
		return nil
	}

	result := make([]PointVelocity, len(currentPoints))

	// Get previous frame for correspondence
	prevFrame := ve.History.Previous(0)

	// If no previous frame, initialize points without velocity
	if prevFrame == nil || len(prevFrame.Points) == 0 {
		for i, p := range currentPoints {
			result[i] = PointVelocity{
				X:                     p.X,
				Y:                     p.Y,
				Z:                     p.Z,
				VX:                    0,
				VY:                    0,
				VZ:                    0,
				VelocityConfidence:    0,
				CorrespondingPointIdx: -1,
				TimestampNanos:        p.Timestamp.UnixNano(),
				Intensity:             p.Intensity,
				SensorID:              sensorID,
			}
		}
		ve.storeFrame(result, timestamp)
		return result
	}

	// Compute dt in seconds
	dtSeconds := timestamp.Sub(prevFrame.Timestamp).Seconds()
	if dtSeconds <= 0 || dtSeconds > 1.0 {
		// Invalid or too large time gap, reset velocities
		for i, p := range currentPoints {
			result[i] = PointVelocity{
				X:                     p.X,
				Y:                     p.Y,
				Z:                     p.Z,
				VX:                    0,
				VY:                    0,
				VZ:                    0,
				VelocityConfidence:    0,
				CorrespondingPointIdx: -1,
				TimestampNanos:        p.Timestamp.UnixNano(),
				Intensity:             p.Intensity,
				SensorID:              sensorID,
			}
		}
		ve.storeFrame(result, timestamp)
		return result
	}

	// Estimate velocities using previous frame correspondence
	for i, curr := range currentPoints {
		result[i] = ve.estimatePointVelocity(
			curr, prevFrame, dtSeconds, sensorID,
		)
	}

	// Store current frame for next iteration
	ve.storeFrame(result, timestamp)

	return result
}

// estimatePointVelocity estimates velocity for a single point.
func (ve *VelocityEstimator) estimatePointVelocity(
	curr WorldPoint,
	prevFrame *PointVelocityFrame,
	dtSeconds float64,
	sensorID string,
) PointVelocity {
	result := PointVelocity{
		X:                     curr.X,
		Y:                     curr.Y,
		Z:                     curr.Z,
		VX:                    0,
		VY:                    0,
		VZ:                    0,
		VelocityConfidence:    0,
		CorrespondingPointIdx: -1,
		TimestampNanos:        curr.Timestamp.UnixNano(),
		Intensity:             curr.Intensity,
		SensorID:              sensorID,
	}

	// Estimate local velocity from previous frame's velocities in the neighborhood
	localVX, localVY := ve.estimateLocalVelocity(curr, prevFrame)

	// Back-project search position using local velocity context
	// This is where we expect the point was in the previous frame
	searchX := curr.X - localVX*dtSeconds
	searchY := curr.Y - localVY*dtSeconds
	searchZ := curr.Z // Assume minimal vertical movement

	// Find candidates in previous frame near the back-projected position
	if prevFrame.SpatialIndex == nil {
		return result
	}

	// Find the best corresponding point in the previous frame
	bestIdx := -1
	bestScore := float32(0)
	eps := ve.Config.SearchRadius

	for j, prev := range prevFrame.Points {
		dx := searchX - prev.X
		dy := searchY - prev.Y
		dz := searchZ - prev.Z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

		if dist > eps {
			continue
		}

		// Compute velocity if we match this point
		velX := (curr.X - prev.X) / dtSeconds
		velY := (curr.Y - prev.Y) / dtSeconds
		velZ := (curr.Z - prev.Z) / dtSeconds
		velMag := math.Sqrt(velX*velX + velY*velY + velZ*velZ)

		// Reject implausible velocities
		if velMag > ve.Config.MaxVelocityMps {
			continue
		}

		// Compute velocity confidence
		score := ve.computeVelocityConfidence(dist, velMag, prev.VelocityConfidence)

		if score > bestScore {
			bestScore = score
			bestIdx = j
		}
	}

	if bestIdx >= 0 && bestScore >= ve.Config.MinConfidence {
		prev := prevFrame.Points[bestIdx]
		result.VX = (curr.X - prev.X) / dtSeconds
		result.VY = (curr.Y - prev.Y) / dtSeconds
		result.VZ = (curr.Z - prev.Z) / dtSeconds
		result.VelocityConfidence = bestScore
		result.CorrespondingPointIdx = bestIdx
	}

	return result
}

// estimateLocalVelocity estimates the local velocity context from nearby points in the previous frame.
func (ve *VelocityEstimator) estimateLocalVelocity(curr WorldPoint, prevFrame *PointVelocityFrame) (vx, vy float64) {
	if prevFrame == nil || len(prevFrame.Points) == 0 {
		return 0, 0
	}

	// Find points in previous frame near the current point
	var sumVX, sumVY float64
	var count int

	for _, prev := range prevFrame.Points {
		dx := curr.X - prev.X
		dy := curr.Y - prev.Y
		dist := math.Sqrt(dx*dx + dy*dy)

		if dist <= ve.Config.NeighborRadius && prev.VelocityConfidence >= ve.Config.MinConfidence {
			sumVX += prev.VX
			sumVY += prev.VY
			count++
		}
	}

	if count < ve.Config.MinNeighborsForContext {
		return 0, 0
	}

	return sumVX / float64(count), sumVY / float64(count)
}

// computeVelocityConfidence computes a confidence score for a velocity estimate.
func (ve *VelocityEstimator) computeVelocityConfidence(
	spatialDist float64,
	velocityMagnitude float64,
	prevConfidence float32,
) float32 {
	// Spatial proximity score [0, 1] - closer is better
	spatialScore := math.Exp(-spatialDist / ve.Config.SearchRadius)

	// Velocity plausibility score [0, 1] - slower is more likely
	var plausibilityScore float64
	if velocityMagnitude > ve.Config.MaxVelocityMps {
		plausibilityScore = 0
	} else {
		plausibilityScore = 1.0 - (velocityMagnitude / (2 * ve.Config.MaxVelocityMps))
		if plausibilityScore < 0 {
			plausibilityScore = 0
		}
	}

	// Temporal consistency - boost if previous point had high confidence
	temporalBoost := 1.0 + 0.2*float64(prevConfidence)

	// Combined confidence
	confidence := float32(spatialScore * plausibilityScore * temporalBoost)
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// storeFrame stores the current frame in history for future correspondence.
func (ve *VelocityEstimator) storeFrame(points []PointVelocity, timestamp time.Time) {
	// Build spatial index for the new frame
	worldPoints := make([]WorldPoint, len(points))
	for i, pv := range points {
		worldPoints[i] = WorldPoint{
			X:         pv.X,
			Y:         pv.Y,
			Z:         pv.Z,
			Intensity: pv.Intensity,
			Timestamp: time.Unix(0, pv.TimestampNanos),
			SensorID:  pv.SensorID,
		}
	}

	spatialIndex := NewSpatialIndex(ve.Config.SearchRadius)
	spatialIndex.Build(worldPoints)

	frame := PointVelocityFrame{
		Points:       points,
		Timestamp:    timestamp,
		SpatialIndex: spatialIndex,
	}

	ve.History.Add(frame)
}

// GetConfig returns the current velocity estimation configuration.
func (ve *VelocityEstimator) GetConfig() VelocityEstimationConfig {
	ve.mu.RLock()
	defer ve.mu.RUnlock()
	return ve.Config
}

// SetConfig updates the velocity estimation configuration.
func (ve *VelocityEstimator) SetConfig(config VelocityEstimationConfig) {
	ve.mu.Lock()
	defer ve.mu.Unlock()
	ve.Config = config
}
