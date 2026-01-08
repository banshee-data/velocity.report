package lidar

import (
	"fmt"
	"time"
)

// VelocityCoherentConfig holds configuration for velocity-coherent extraction.
type VelocityCoherentConfig struct {
	// Velocity estimation parameters
	VelocityEstimation VelocityEstimationConfig

	// DBSCAN parameters (reduced MinPts compared to background subtraction)
	DBSCANEps    float64 // Neighborhood radius (default: 0.6m)
	DBSCANMinPts int     // Minimum points (default: 3, reduced because velocity confirms)

	// Velocity coherence filtering
	MinVelocityCoherence float64 // Minimum coherence to accept cluster (default: 0.3)
	MinVelocityPoints    int     // Minimum points with velocity (default: 2)

	// Frame history
	FrameHistoryCapacity int // Number of frames to keep (default: 10)
}

// DefaultVelocityCoherentConfig returns sensible defaults.
func DefaultVelocityCoherentConfig() VelocityCoherentConfig {
	return VelocityCoherentConfig{
		VelocityEstimation:   DefaultVelocityEstimationConfig(),
		DBSCANEps:            0.6,
		DBSCANMinPts:         3, // Reduced from 12 because velocity coherence confirms identity
		MinVelocityCoherence: 0.3,
		MinVelocityPoints:    2,
		FrameHistoryCapacity: 10,
	}
}

// VelocityCoherentExtractor implements foreground extraction using velocity coherence.
// This algorithm tracks motion patterns across frames to identify moving objects,
// rather than comparing to a static background model.
//
// Advantages over background subtraction:
// - No trail artifacts (no EMA reconvergence delay)
// - Works with MinPts=3 (velocity confirms cluster identity)
// - Captures sparse distant objects that would be filtered by MinPts=12
// - Pre-entry and post-exit tracking via velocity prediction
//
// Disadvantages:
// - Requires multi-frame history (more memory)
// - May miss stationary objects (needs hybrid with background subtraction)
// - Cannot run on first frame (needs at least 2 frames)
type VelocityCoherentExtractor struct {
	Config       VelocityCoherentConfig
	SensorID     string
	FrameHistory *FrameHistory

	// Internal state
	lastTimestamp time.Time
	frameCount    int64
}

// Ensure VelocityCoherentExtractor implements ForegroundExtractor
var _ ForegroundExtractor = (*VelocityCoherentExtractor)(nil)

// NewVelocityCoherentExtractor creates a new velocity-coherent extractor.
func NewVelocityCoherentExtractor(config VelocityCoherentConfig, sensorID string) *VelocityCoherentExtractor {
	return &VelocityCoherentExtractor{
		Config:       config,
		SensorID:     sensorID,
		FrameHistory: NewFrameHistory(config.FrameHistoryCapacity),
	}
}

// Name returns the algorithm name.
func (e *VelocityCoherentExtractor) Name() string {
	return "velocity_coherent"
}

// ProcessFrame extracts foreground points using velocity coherence.
func (e *VelocityCoherentExtractor) ProcessFrame(points []PointPolar, timestamp time.Time) (
	foregroundMask []bool,
	metrics ExtractorMetrics,
	err error,
) {
	if len(points) == 0 {
		return []bool{}, ExtractorMetrics{}, nil
	}

	start := time.Now()
	e.frameCount++

	// Step 1: Transform all points to world coordinates
	worldPoints := TransformToWorld(points, nil, e.SensorID)

	// Step 2: Build PointWithVelocity array
	pointsWithVel := BuildWorldPointsWithVelocity(worldPoints, points)

	// Step 3: Estimate velocities from previous frame
	prevFrame := e.FrameHistory.Previous(1)
	if prevFrame != nil {
		dt := timestamp.Sub(e.lastTimestamp).Seconds()
		if dt > 0 && dt < 1.0 { // Sanity check: dt should be positive and < 1 second
			pointsWithVel = EstimatePointVelocities(
				pointsWithVel,
				prevFrame,
				dt,
				e.Config.VelocityEstimation,
			)
		}
	}

	// Step 4: Build current frame and add to history
	frameID := fmt.Sprintf("frame_%d", e.frameCount)
	currentFrame := NewVelocityFrame(
		pointsWithVel,
		timestamp,
		frameID,
		e.Config.VelocityEstimation.SpatialIndexCellSize,
	)
	e.FrameHistory.Add(currentFrame)
	e.lastTimestamp = timestamp

	// Step 5: Cluster using DBSCAN with reduced MinPts
	dbscanParams := DBSCANParams{
		Eps:    e.Config.DBSCANEps,
		MinPts: e.Config.DBSCANMinPts,
	}
	clusters := DBSCAN(worldPoints, dbscanParams)

	// Step 6: Build cluster labels for each point
	clusterLabels := buildClusterLabels(worldPoints, clusters, dbscanParams)

	// Step 7: Filter clusters by velocity coherence
	filteredClusters := FilterClustersByVelocityCoherence(
		clusters,
		pointsWithVel,
		clusterLabels,
		e.Config.MinVelocityCoherence,
		e.Config.VelocityEstimation.MinConfidence,
	)

	// Step 8: Build foreground mask from filtered clusters
	foregroundMask = make([]bool, len(points))
	clusterIDSet := make(map[int64]bool)
	for _, c := range filteredClusters {
		clusterIDSet[c.ClusterID] = true
	}

	fgCount := 0
	for i, label := range clusterLabels {
		if label > 0 && clusterIDSet[int64(label)] {
			foregroundMask[i] = true
			fgCount++
		}
	}

	elapsed := time.Since(start)

	// Compute additional metrics
	pointsWithVelocity := 0
	totalConfidence := float32(0)
	for _, p := range pointsWithVel {
		if p.Confidence > 0 {
			pointsWithVelocity++
			totalConfidence += p.Confidence
		}
	}
	avgConfidence := float32(0)
	if pointsWithVelocity > 0 {
		avgConfidence = totalConfidence / float32(pointsWithVelocity)
	}

	metrics = ExtractorMetrics{
		ForegroundCount:  fgCount,
		BackgroundCount:  len(points) - fgCount,
		ProcessingTimeUs: elapsed.Microseconds(),
		AlgorithmSpecific: map[string]interface{}{
			"clusters_total":         len(clusters),
			"clusters_filtered":      len(filteredClusters),
			"points_with_velocity":   pointsWithVelocity,
			"avg_velocity_confidence": avgConfidence,
			"frame_history_size":     e.FrameHistory.Size(),
		},
	}

	return foregroundMask, metrics, nil
}

// buildClusterLabels creates a label array matching DBSCAN output.
// This is a simplified version that re-runs DBSCAN logic to get labels.
func buildClusterLabels(points []WorldPoint, clusters []WorldCluster, params DBSCANParams) []int {
	if len(points) == 0 {
		return nil
	}

	// Run DBSCAN to get labels
	n := len(points)
	labels := make([]int, n)
	clusterID := 0

	// Build spatial index
	spatialIndex := NewSpatialIndex(params.Eps)
	spatialIndex.Build(points)

	for i := 0; i < n; i++ {
		if labels[i] != 0 {
			continue
		}

		neighbors := spatialIndex.RegionQuery(points, i, params.Eps)

		if len(neighbors) < params.MinPts {
			labels[i] = -1 // Noise
			continue
		}

		clusterID++
		expandClusterLabels(points, spatialIndex, labels, i, neighbors, clusterID, params.Eps, params.MinPts)
	}

	return labels
}

// expandClusterLabels is a helper for buildClusterLabels.
func expandClusterLabels(points []WorldPoint, si *SpatialIndex, labels []int,
	seedIdx int, neighbors []int, clusterID int, eps float64, minPts int) {

	labels[seedIdx] = clusterID

	for j := 0; j < len(neighbors); j++ {
		idx := neighbors[j]

		if labels[idx] == -1 {
			labels[idx] = clusterID
		}

		if labels[idx] != 0 {
			continue
		}

		labels[idx] = clusterID
		newNeighbors := si.RegionQuery(points, idx, eps)

		if len(newNeighbors) >= minPts {
			neighbors = append(neighbors, newNeighbors...)
		}
	}
}

// GetParams returns the current configuration as a map.
func (e *VelocityCoherentExtractor) GetParams() map[string]interface{} {
	return map[string]interface{}{
		"search_radius":           e.Config.VelocityEstimation.SearchRadius,
		"max_velocity_mps":        e.Config.VelocityEstimation.MaxVelocityMps,
		"velocity_weight":         e.Config.VelocityEstimation.VelocityWeight,
		"min_confidence":          e.Config.VelocityEstimation.MinConfidence,
		"dbscan_eps":              e.Config.DBSCANEps,
		"dbscan_min_pts":          e.Config.DBSCANMinPts,
		"min_velocity_coherence":  e.Config.MinVelocityCoherence,
		"min_velocity_points":     e.Config.MinVelocityPoints,
		"frame_history_capacity":  e.Config.FrameHistoryCapacity,
	}
}

// SetParams updates configuration from a map.
func (e *VelocityCoherentExtractor) SetParams(params map[string]interface{}) error {
	if v, ok := params["search_radius"].(float64); ok {
		e.Config.VelocityEstimation.SearchRadius = v
	}
	if v, ok := params["max_velocity_mps"].(float64); ok {
		e.Config.VelocityEstimation.MaxVelocityMps = v
	}
	if v, ok := params["velocity_weight"].(float64); ok {
		e.Config.VelocityEstimation.VelocityWeight = v
	}
	if v, ok := params["min_confidence"].(float64); ok {
		e.Config.VelocityEstimation.MinConfidence = float32(v)
	}
	if v, ok := params["dbscan_eps"].(float64); ok {
		e.Config.DBSCANEps = v
	}
	if v, ok := params["dbscan_min_pts"].(float64); ok {
		e.Config.DBSCANMinPts = int(v)
	}
	if v, ok := params["min_velocity_coherence"].(float64); ok {
		e.Config.MinVelocityCoherence = v
	}
	if v, ok := params["min_velocity_points"].(float64); ok {
		e.Config.MinVelocityPoints = int(v)
	}
	if v, ok := params["frame_history_capacity"].(float64); ok {
		newCapacity := int(v)
		if newCapacity != e.Config.FrameHistoryCapacity {
			e.Config.FrameHistoryCapacity = newCapacity
			e.FrameHistory = NewFrameHistory(newCapacity)
		}
	}
	return nil
}

// Reset clears the frame history and internal state.
func (e *VelocityCoherentExtractor) Reset() {
	e.FrameHistory.Clear()
	e.frameCount = 0
	e.lastTimestamp = time.Time{}
}
