package lidar

import (
	"math"
	"sort"
)

// VelocityEstimationConfig holds configuration for velocity estimation.
type VelocityEstimationConfig struct {
	// SearchRadius is the maximum distance (meters) to search for correspondences
	SearchRadius float64

	// MaxVelocityMps is the maximum plausible velocity (m/s) - correspondences
	// implying higher velocities are rejected
	MaxVelocityMps float64

	// VelocityWeight controls how much velocity consistency affects correspondence score
	VelocityWeight float64

	// MinConfidence is the minimum velocity confidence threshold for consideration
	MinConfidence float32

	// SpatialIndexCellSize is the cell size for the spatial index (meters)
	SpatialIndexCellSize float64
}

// DefaultVelocityEstimationConfig returns sensible defaults for traffic monitoring.
func DefaultVelocityEstimationConfig() VelocityEstimationConfig {
	return VelocityEstimationConfig{
		SearchRadius:         2.0,  // 2 meters
		MaxVelocityMps:       50.0, // ~180 km/h, max highway speed
		VelocityWeight:       2.0,  // Weight for velocity in scoring
		MinConfidence:        0.3,  // 30% confidence minimum
		SpatialIndexCellSize: 0.6,  // Same as DBSCAN eps
	}
}

// EstimatePointVelocities estimates per-point velocities by finding correspondences
// between the current frame and a previous frame.
//
// Algorithm:
// 1. For each point in currentPoints, search for nearby points in previousFrame
// 2. Score each candidate by spatial distance + velocity consistency with neighbors
// 3. Select best correspondence and compute velocity
// 4. Assign confidence based on match quality
func EstimatePointVelocities(
	currentPoints []PointWithVelocity,
	previousFrame *VelocityFrame,
	dt float64,
	config VelocityEstimationConfig,
) []PointWithVelocity {
	if len(currentPoints) == 0 || previousFrame == nil || dt <= 0 {
		return currentPoints
	}

	// Build a WorldPoint slice for spatial queries
	currentWorldPoints := make([]WorldPoint, len(currentPoints))
	for i, p := range currentPoints {
		currentWorldPoints[i] = WorldPoint{X: p.X, Y: p.Y, Z: p.Z}
	}

	// First pass: estimate raw velocities from best correspondences
	rawVelocities := make([][2]float64, len(currentPoints))
	hasCorrespondence := make([]bool, len(currentPoints))

	for i, p := range currentPoints {
		// Find candidates within search radius
		candidates := findCandidates(p.X, p.Y, previousFrame, config.SearchRadius)

		if len(candidates) == 0 {
			continue
		}

		// Find best correspondence
		bestIdx, bestScore := findBestCorrespondence(p, candidates, previousFrame.Points, dt, config)

		if bestIdx >= 0 && bestScore < math.MaxFloat64 {
			prev := previousFrame.Points[bestIdx]
			vx := (p.X - prev.X) / dt
			vy := (p.Y - prev.Y) / dt

			// Check velocity magnitude is plausible
			speed := math.Sqrt(vx*vx + vy*vy)
			if speed <= config.MaxVelocityMps {
				rawVelocities[i] = [2]float64{vx, vy}
				hasCorrespondence[i] = true
				currentPoints[i].CorrespondenceIdx = bestIdx
			}
		}
	}

	// Second pass: compute local median velocities and assign confidence
	for i := range currentPoints {
		if !hasCorrespondence[i] {
			currentPoints[i].VX = 0
			currentPoints[i].VY = 0
			currentPoints[i].Confidence = 0
			continue
		}

		// Use raw velocity
		vx := rawVelocities[i][0]
		vy := rawVelocities[i][1]

		// Compute local median velocity from neighbors
		medVX, medVY, neighborCount := computeLocalMedianVelocity(
			i, currentPoints, rawVelocities, hasCorrespondence, config.SearchRadius,
		)

		// Compute confidence based on:
		// 1. How close our velocity is to the local median
		// 2. How many neighbors we have
		var confidence float32
		if neighborCount > 0 {
			// Velocity deviation from median
			velDev := math.Sqrt((vx-medVX)*(vx-medVX) + (vy-medVY)*(vy-medVY))
			speed := math.Sqrt(vx*vx + vy*vy)

			// Relative deviation (clamped to [0, 1])
			relDev := 0.0
			if speed > 0.1 { // Avoid division by zero for stationary
				relDev = math.Min(velDev/speed, 1.0)
			}

			// Confidence decreases with deviation, increases with neighbors
			confidence = float32(1.0 - relDev*0.5)
			confidence *= float32(math.Min(float64(neighborCount)/3.0, 1.0)) // Scale by neighbor count
		} else {
			// No neighbors - lower confidence
			confidence = 0.3
		}

		currentPoints[i].VX = vx
		currentPoints[i].VY = vy
		currentPoints[i].Confidence = confidence
	}

	return currentPoints
}

// findCandidates returns indices of points in previousFrame within searchRadius of (x, y).
func findCandidates(x, y float64, frame *VelocityFrame, searchRadius float64) []int {
	if frame == nil || frame.SpatialIndex == nil {
		return nil
	}

	// Use spatial index to find candidates
	searchRadius2 := searchRadius * searchRadius
	candidates := make([]int, 0, 10)

	// Get cell coordinates for the query point
	cellSize := frame.SpatialIndex.CellSize
	cellX := int64(math.Floor(x / cellSize))
	cellY := int64(math.Floor(y / cellSize))

	// Search radius in cells
	cellRadius := int64(math.Ceil(searchRadius / cellSize))

	// Check cells in range
	for dx := -cellRadius; dx <= cellRadius; dx++ {
		for dy := -cellRadius; dy <= cellRadius; dy++ {
			neighborCellX := cellX + dx
			neighborCellY := cellY + dy

			// Map to non-negative for pairing
			var a, b int64
			if neighborCellX >= 0 {
				a = 2 * neighborCellX
			} else {
				a = -2*neighborCellX - 1
			}
			if neighborCellY >= 0 {
				b = 2 * neighborCellY
			} else {
				b = -2*neighborCellY - 1
			}

			var cellID int64
			if a >= b {
				cellID = a*a + a + b
			} else {
				cellID = a + b*b
			}

			// Check all points in this cell
			for _, idx := range frame.SpatialIndex.Grid[cellID] {
				if idx >= len(frame.Points) {
					continue
				}
				p := frame.Points[idx]
				dx := p.X - x
				dy := p.Y - y
				dist2 := dx*dx + dy*dy
				if dist2 <= searchRadius2 {
					candidates = append(candidates, idx)
				}
			}
		}
	}

	return candidates
}

// findBestCorrespondence finds the best matching point from candidates.
// Returns the index and score (lower is better).
func findBestCorrespondence(
	p PointWithVelocity,
	candidates []int,
	previousPoints []PointWithVelocity,
	dt float64,
	config VelocityEstimationConfig,
) (bestIdx int, bestScore float64) {
	bestIdx = -1
	bestScore = math.MaxFloat64

	for _, idx := range candidates {
		if idx >= len(previousPoints) {
			continue
		}
		prev := previousPoints[idx]

		// Spatial distance
		dx := p.X - prev.X
		dy := p.Y - prev.Y
		spatialDist := math.Sqrt(dx*dx + dy*dy)

		// Implied velocity
		vx := dx / dt
		vy := dy / dt
		speed := math.Sqrt(vx*vx + vy*vy)

		// Reject implausible velocities
		if speed > config.MaxVelocityMps {
			continue
		}

		// Channel/ring consistency bonus (points on same ring are more likely to correspond)
		channelBonus := 0.0
		if prev.Channel == p.Channel {
			channelBonus = -0.1 // Small bonus for matching channel
		}

		// Score = spatial distance + velocity consistency penalty
		// Lower is better
		velocityPenalty := 0.0
		if prev.Confidence > 0 {
			// If previous point had a velocity, check consistency
			expectedX := prev.X + prev.VX*dt
			expectedY := prev.Y + prev.VY*dt
			predDx := p.X - expectedX
			predDy := p.Y - expectedY
			velocityPenalty = math.Sqrt(predDx*predDx+predDy*predDy) * config.VelocityWeight * float64(prev.Confidence)
		}

		score := spatialDist + velocityPenalty + channelBonus

		if score < bestScore {
			bestScore = score
			bestIdx = idx
		}
	}

	return bestIdx, bestScore
}

// computeLocalMedianVelocity computes the median velocity of nearby points.
func computeLocalMedianVelocity(
	idx int,
	points []PointWithVelocity,
	rawVelocities [][2]float64,
	hasCorrespondence []bool,
	searchRadius float64,
) (medVX, medVY float64, count int) {
	if idx >= len(points) {
		return 0, 0, 0
	}

	p := points[idx]
	searchRadius2 := searchRadius * searchRadius

	vxValues := make([]float64, 0, 10)
	vyValues := make([]float64, 0, 10)

	for j, other := range points {
		if j == idx || !hasCorrespondence[j] {
			continue
		}

		dx := other.X - p.X
		dy := other.Y - p.Y
		if dx*dx+dy*dy <= searchRadius2 {
			vxValues = append(vxValues, rawVelocities[j][0])
			vyValues = append(vyValues, rawVelocities[j][1])
		}
	}

	if len(vxValues) == 0 {
		return 0, 0, 0
	}

	return median(vxValues), median(vyValues), len(vxValues)
}

// median computes the median of a float64 slice.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Make a copy to avoid modifying original
	sorted := make([]float64, len(values))
	copy(sorted, values)

	// Use efficient O(n log n) sorting
	sort.Float64s(sorted)

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// ComputeClusterVelocityCoherence computes velocity statistics for a cluster.
// Returns average velocity, variance, and a coherence score.
func ComputeClusterVelocityCoherence(points []PointWithVelocity, minConfidence float32) (
	avgVX, avgVY, variance, coherence float64, validCount int,
) {
	if len(points) == 0 {
		return 0, 0, 0, 0, 0
	}

	// Collect velocities above confidence threshold
	vxValues := make([]float64, 0, len(points))
	vyValues := make([]float64, 0, len(points))

	for _, p := range points {
		if p.Confidence >= minConfidence {
			vxValues = append(vxValues, p.VX)
			vyValues = append(vyValues, p.VY)
		}
	}

	validCount = len(vxValues)
	if validCount < 2 {
		if validCount == 1 {
			return vxValues[0], vyValues[0], 0, 1.0, 1
		}
		return 0, 0, 0, 0, 0
	}

	// Compute mean
	for i := 0; i < validCount; i++ {
		avgVX += vxValues[i]
		avgVY += vyValues[i]
	}
	avgVX /= float64(validCount)
	avgVY /= float64(validCount)

	// Compute variance
	for i := 0; i < validCount; i++ {
		dx := vxValues[i] - avgVX
		dy := vyValues[i] - avgVY
		variance += dx*dx + dy*dy
	}
	variance /= float64(validCount)

	// Coherence = 1 / (1 + variance)
	// High coherence = low variance
	coherence = 1.0 / (1.0 + variance)

	return avgVX, avgVY, variance, coherence, validCount
}

// FilterClustersByVelocityCoherence filters clusters to only include those with
// sufficiently coherent internal velocities.
func FilterClustersByVelocityCoherence(
	clusters []WorldCluster,
	allPoints []PointWithVelocity,
	clusterLabels []int,
	minCoherence float64,
	minConfidence float32,
) []WorldCluster {
	if len(clusters) == 0 {
		return nil
	}

	filtered := make([]WorldCluster, 0, len(clusters))

	for _, cluster := range clusters {
		// Collect points belonging to this cluster
		clusterPoints := make([]PointWithVelocity, 0)
		for i, label := range clusterLabels {
			if int64(label) == cluster.ClusterID && i < len(allPoints) {
				clusterPoints = append(clusterPoints, allPoints[i])
			}
		}

		if len(clusterPoints) == 0 {
			continue
		}

		// Compute velocity coherence
		avgVX, avgVY, _, coherence, validCount := ComputeClusterVelocityCoherence(clusterPoints, minConfidence)

		// Filter by coherence threshold
		if coherence >= minCoherence && validCount >= 2 {
			// Annotate cluster with velocity
			cluster.AvgVelocityX = float32(avgVX)
			cluster.AvgVelocityY = float32(avgVY)
			cluster.VelocityCoherence = float32(coherence)
			cluster.VelocityPointCount = validCount
			filtered = append(filtered, cluster)
		}
	}

	return filtered
}
