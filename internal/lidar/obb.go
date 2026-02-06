package lidar

import (
	"math"
)

// obbCovarianceEpsilon is the minimum threshold for numerical stability in
// covariance matrix operations during OBB estimation. Values below this are
// considered effectively zero for purposes of eigenvalue computation.
const obbCovarianceEpsilon = 1e-9

// OrientedBoundingBox represents a 7-DOF (7 Degrees of Freedom) 3D bounding box.
// This format conforms to the AV industry standard specification.
//
// 7-DOF parameters:
//   - CenterX/Y/Z: Centre position (metres, world frame)
//   - Length: Box extent along heading direction (metres)
//   - Width: Box extent perpendicular to heading (metres)
//   - Height: Box extent along Z-axis (metres)
//   - HeadingRad: Yaw angle around Z-axis (radians, [-π, π])
type OrientedBoundingBox struct {
	CenterX    float32
	CenterY    float32
	CenterZ    float32
	Length     float32 // Extent along principal axis
	Width      float32 // Extent perpendicular to principal axis
	Height     float32 // Extent along Z
	HeadingRad float32 // Rotation around Z-axis
}

// EstimateOBBFromCluster computes an oriented bounding box for a cluster using
// PCA (Principal Component Analysis) on the X-Y plane to determine the principal
// orientation. This provides a tighter fit than axis-aligned boxes and gives
// object heading information.
//
// Algorithm:
//  1. Compute centroid in X-Y plane
//  2. Build 2x2 covariance matrix
//  3. Compute eigenvectors via closed-form solution
//  4. Project points onto principal axes to find extents
//  5. Heading = atan2 of principal eigenvector
//
// Returns an OBB with heading aligned to the primary axis of variation.
func EstimateOBBFromCluster(points []WorldPoint) OrientedBoundingBox {
	n := len(points)
	if n == 0 {
		return OrientedBoundingBox{}
	}

	// Step 1: Compute centroid (mean position in all dimensions)
	var sumX, sumY, sumZ float64
	for _, pt := range points {
		sumX += pt.X
		sumY += pt.Y
		sumZ += pt.Z
	}
	nf := float64(n)
	meanX := sumX / nf
	meanY := sumY / nf

	// Step 2: Build covariance matrix for X-Y plane
	// Cov = [c00 c01]
	//       [c10 c11]
	// where c01 = c10 (symmetric)
	var c00, c01, c11 float64
	for _, pt := range points {
		dx := pt.X - meanX
		dy := pt.Y - meanY
		c00 += dx * dx
		c01 += dx * dy
		c11 += dy * dy
	}
	c00 /= nf
	c01 /= nf
	c11 /= nf

	// Step 3: Compute eigenvalues and eigenvectors analytically for 2x2 matrix
	// For symmetric 2x2 matrix, eigenvalues are:
	// λ = (trace ± sqrt(trace² - 4*det)) / 2
	trace := c00 + c11
	det := c00*c11 - c01*c01
	discriminant := trace*trace - 4*det

	var lambda1 float64
	if discriminant < 0 {
		// Degenerate case (e.g., all points collinear or single point)
		// Fall back to axis-aligned box
		lambda1 = c00
		_ = c11 // Use c11 as lambda2 conceptually, but we only need lambda1
	} else {
		sqrtDisc := math.Sqrt(discriminant)
		lambda1 = (trace + sqrtDisc) / 2 // Larger eigenvalue (principal axis)
		// lambda2 = (trace - sqrtDisc) / 2 // Smaller eigenvalue (not used)
	}

	// Eigenvector corresponding to lambda1 (principal axis)
	// For 2x2 symmetric matrix: eigenvector = [c01, lambda1 - c00] (unnormalized)
	var evX, evY float64
	if math.Abs(c01) > obbCovarianceEpsilon {
		// Non-diagonal covariance - use eigenvector formula
		evX = c01
		evY = lambda1 - c00
		// Normalise
		mag := math.Sqrt(evX*evX + evY*evY)
		if mag > obbCovarianceEpsilon {
			evX /= mag
			evY /= mag
		} else {
			// Fallback: use X-axis
			evX, evY = 1.0, 0.0
		}
	} else {
		// Diagonal covariance - principal axis is X or Y axis
		if c00 >= c11 {
			evX, evY = 1.0, 0.0 // X-axis is principal
		} else {
			evX, evY = 0.0, 1.0 // Y-axis is principal
		}
	}

	// Heading is the angle of the principal eigenvector
	heading := float32(math.Atan2(evY, evX))

	// Step 4: Project points onto principal axes and perpendicular axes to find extents
	var minProj, maxProj, minPerp, maxPerp, minZ, maxZ float64
	minProj, maxProj = math.MaxFloat64, -math.MaxFloat64
	minPerp, maxPerp = math.MaxFloat64, -math.MaxFloat64
	minZ, maxZ = math.MaxFloat64, -math.MaxFloat64

	for _, pt := range points {
		dx := pt.X - meanX
		dy := pt.Y - meanY

		// Project onto principal axis (length direction)
		projAlong := dx*evX + dy*evY
		if projAlong < minProj {
			minProj = projAlong
		}
		if projAlong > maxProj {
			maxProj = projAlong
		}

		// Project onto perpendicular axis (width direction)
		// Perpendicular vector is [-evY, evX]
		projPerp := dx*(-evY) + dy*evX
		if projPerp < minPerp {
			minPerp = projPerp
		}
		if projPerp > maxPerp {
			maxPerp = projPerp
		}

		// Z extent
		if pt.Z < minZ {
			minZ = pt.Z
		}
		if pt.Z > maxZ {
			maxZ = pt.Z
		}
	}

	length := float32(maxProj - minProj)
	width := float32(maxPerp - minPerp)
	height := float32(maxZ - minZ)

	// CenterX/Y use the mean (centroid) of the cluster points. CenterZ uses
	// the minimum Z (ground plane) rather than the volumetric centre so that
	// the wireframe box, whose unit cube spans Z 0→1, sits flush on the
	// lowest point of the cluster instead of floating above it.
	return OrientedBoundingBox{
		CenterX:    float32(meanX),
		CenterY:    float32(meanY),
		CenterZ:    float32(minZ),
		Length:     length,
		Width:      width,
		Height:     height,
		HeadingRad: heading,
	}
}

// SmoothOBBHeading applies exponential moving average (EMA) smoothing to OBB headings,
// with special handling for angular wraparound at ±π. This reduces jitter in heading
// estimates while maintaining responsiveness to real rotation.
//
// Parameters:
//   - prevHeading: Previous smoothed heading (radians)
//   - newHeading: New raw heading from current observation (radians)
//   - alpha: Smoothing factor [0, 1]. Higher = more responsive. Typical: 0.3
//
// Returns: Smoothed heading in range [-π, π]
func SmoothOBBHeading(prevHeading, newHeading, alpha float32) float32 {
	// Handle angular wraparound: find shortest angular distance
	diff := newHeading - prevHeading

	// Normalise diff to [-π, π]
	for diff > math.Pi {
		diff -= 2 * math.Pi
	}
	for diff < -math.Pi {
		diff += 2 * math.Pi
	}

	// Apply EMA: smoothed = prev + alpha * (new - prev)
	smoothed := prevHeading + alpha*diff

	// Normalise result to [-π, π]
	for smoothed > math.Pi {
		smoothed -= 2 * math.Pi
	}
	for smoothed < -math.Pi {
		smoothed += 2 * math.Pi
	}

	return smoothed
}
