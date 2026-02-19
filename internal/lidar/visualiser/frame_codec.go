// Package visualiser provides gRPC streaming of LiDAR perception data.
// This file contains the point cloud memory pool and decimation codec.
package visualiser

import (
	"math"
	"sync"
)

// pointSlicePool reduces allocations by reusing large float32 slices.
// Slices are sized for ~70k points (typical for Pandar40P at 10Hz).
var pointSlicePool = sync.Pool{
	New: func() interface{} {
		// Pre-allocate for typical point cloud size
		return make([]float32, 0, 75000)
	},
}

// byteSlicePool reduces allocations for intensity/classification arrays.
var byteSlicePool = sync.Pool{
	New: func() interface{} {
		return make([]uint8, 0, 75000)
	},
}

// getFloat32Slice gets a slice from the pool and resets it.
func getFloat32Slice(n int) []float32 {
	s := pointSlicePool.Get().([]float32)
	if cap(s) < n {
		// Slice too small, allocate new one (rare for normal point clouds)
		pointSlicePool.Put(s)
		return make([]float32, n)
	}
	return s[:n]
}

// putFloat32Slice returns a slice to the pool.
func putFloat32Slice(s []float32) {
	// Only pool reasonably sized slices to avoid memory bloat
	if cap(s) > 0 && cap(s) <= 150000 {
		pointSlicePool.Put(s[:0])
	}
}

// getUint8Slice gets a slice from the pool and resets it.
func getUint8Slice(n int) []uint8 {
	s := byteSlicePool.Get().([]uint8)
	if cap(s) < n {
		byteSlicePool.Put(s)
		return make([]uint8, n)
	}
	return s[:n]
}

// putUint8Slice returns a slice to the pool.
func putUint8Slice(s []uint8) {
	if cap(s) > 0 && cap(s) <= 150000 {
		byteSlicePool.Put(s[:0])
	}
}

// Release returns the PointCloudFrame's slices to the pool. Reference-counted:
// only releases when the last consumer calls Release.
func (pc *PointCloudFrame) Release() {
	if pc == nil {
		return
	}

	// Decrement reference count. Only release to pool when count reaches zero.
	// If refCount was 0 before decrement (frame was never Retain'd), we get -1,
	// which means this is a single-use frame that should be released immediately.
	newCount := pc.refCount.Add(-1)
	if newCount > 0 {
		// Other consumers still hold references
		return
	}

	// Release slices to pool
	putFloat32Slice(pc.X)
	putFloat32Slice(pc.Y)
	putFloat32Slice(pc.Z)
	putUint8Slice(pc.Intensity)
	putUint8Slice(pc.Classification)
	pc.X = nil
	pc.Y = nil
	pc.Z = nil
	pc.Intensity = nil
	pc.Classification = nil
}

// ApplyDecimation decimates the point cloud according to the specified mode and ratio.
// This modifies the PointCloudFrame in place.
// For uniform/voxel modes, ratio should be in (0, 1]. A ratio of 1.0 keeps all points.
func (pc *PointCloudFrame) ApplyDecimation(mode DecimationMode, ratio float32) {
	if mode == DecimationNone {
		return
	}

	// ForegroundOnly mode ignores ratio
	if mode == DecimationForegroundOnly {
		pc.applyForegroundOnlyDecimation()
		pc.DecimationMode = mode
		pc.DecimationRatio = ratio
		return
	}

	// For other modes, check ratio validity (must be in range (0, 1])
	if ratio <= 0 || ratio > 1 {
		return
	}

	switch mode {
	case DecimationUniform:
		pc.applyUniformDecimation(ratio)
	case DecimationVoxel:
		// Voxel grid decimation: leaf size derived from ratio.
		// At ratio 0.5, leaf ≈ 0.08m; at ratio 0.25, leaf ≈ 0.12m.
		leafSize := float32(0.04 / float64(ratio))
		pc.applyVoxelDecimation(leafSize)
	}

	pc.DecimationMode = mode
	pc.DecimationRatio = ratio
}

// applyUniformDecimation keeps every Nth point based on the ratio.
// A ratio of 1.0 keeps all points, 0.5 keeps approximately half, etc.
// Precondition: ratio is in range (0, 1] - callers must validate.
func (pc *PointCloudFrame) applyUniformDecimation(ratio float32) {
	// If ratio is 1.0, keep all points (no decimation needed)
	if ratio == 1.0 {
		return
	}

	targetCount := int(float32(pc.PointCount) * ratio)
	if targetCount <= 0 {
		targetCount = 1
	}

	stride := pc.PointCount / targetCount
	if stride < 1 {
		stride = 1
	}

	newX := make([]float32, 0, targetCount)
	newY := make([]float32, 0, targetCount)
	newZ := make([]float32, 0, targetCount)
	newIntensity := make([]uint8, 0, targetCount)
	newClassification := make([]uint8, 0, targetCount)

	for i := 0; i < pc.PointCount && len(newX) < targetCount; i += stride {
		newX = append(newX, pc.X[i])
		newY = append(newY, pc.Y[i])
		newZ = append(newZ, pc.Z[i])
		newIntensity = append(newIntensity, pc.Intensity[i])
		newClassification = append(newClassification, pc.Classification[i])
	}

	pc.X = newX
	pc.Y = newY
	pc.Z = newZ
	pc.Intensity = newIntensity
	pc.Classification = newClassification
	pc.PointCount = len(newX)
}

// applyForegroundOnlyDecimation keeps only foreground points (classification == 1).
func (pc *PointCloudFrame) applyForegroundOnlyDecimation() {
	newX := make([]float32, 0, pc.PointCount/2)
	newY := make([]float32, 0, pc.PointCount/2)
	newZ := make([]float32, 0, pc.PointCount/2)
	newIntensity := make([]uint8, 0, pc.PointCount/2)
	newClassification := make([]uint8, 0, pc.PointCount/2)

	for i := 0; i < pc.PointCount; i++ {
		if pc.Classification[i] == 1 {
			newX = append(newX, pc.X[i])
			newY = append(newY, pc.Y[i])
			newZ = append(newZ, pc.Z[i])
			newIntensity = append(newIntensity, pc.Intensity[i])
			newClassification = append(newClassification, pc.Classification[i])
		}
	}

	pc.X = newX
	pc.Y = newY
	pc.Z = newZ
	pc.Intensity = newIntensity
	pc.Classification = newClassification
	pc.PointCount = len(newX)
}

// applyVoxelDecimation reduces point density using 3D voxel grid downsampling.
// Each cubic voxel of the given leaf size retains the point closest to the
// voxel centroid, preserving spatial structure better than uniform stride.
func (pc *PointCloudFrame) applyVoxelDecimation(leafSize float32) {
	if pc.PointCount == 0 || leafSize <= 0 {
		return
	}

	invLeaf := float64(1.0 / leafSize)

	type voxelAccum struct {
		sumX, sumY, sumZ float64
		count            int
		bestIdx          int
		bestDist2        float64
	}

	voxels := make(map[[3]int64]*voxelAccum, pc.PointCount/4)

	for i := 0; i < pc.PointCount; i++ {
		x, y, z := float64(pc.X[i]), float64(pc.Y[i]), float64(pc.Z[i])
		key := [3]int64{
			int64(math.Floor(x * invLeaf)),
			int64(math.Floor(y * invLeaf)),
			int64(math.Floor(z * invLeaf)),
		}
		acc, ok := voxels[key]
		if !ok {
			acc = &voxelAccum{bestIdx: i, bestDist2: math.MaxFloat64}
			voxels[key] = acc
		}
		acc.sumX += x
		acc.sumY += y
		acc.sumZ += z
		acc.count++
	}

	for i := 0; i < pc.PointCount; i++ {
		x, y, z := float64(pc.X[i]), float64(pc.Y[i]), float64(pc.Z[i])
		key := [3]int64{
			int64(math.Floor(x * invLeaf)),
			int64(math.Floor(y * invLeaf)),
			int64(math.Floor(z * invLeaf)),
		}
		acc := voxels[key]
		cx := acc.sumX / float64(acc.count)
		cy := acc.sumY / float64(acc.count)
		cz := acc.sumZ / float64(acc.count)
		dx, dy, dz := x-cx, y-cy, z-cz
		d2 := dx*dx + dy*dy + dz*dz
		if d2 < acc.bestDist2 {
			acc.bestDist2 = d2
			acc.bestIdx = i
		}
	}

	keepSet := make(map[int]bool, len(voxels))
	for _, acc := range voxels {
		keepSet[acc.bestIdx] = true
	}

	kept := 0
	for i := 0; i < pc.PointCount; i++ {
		if keepSet[i] {
			pc.X[kept] = pc.X[i]
			pc.Y[kept] = pc.Y[i]
			pc.Z[kept] = pc.Z[i]
			if i < len(pc.Intensity) {
				pc.Intensity[kept] = pc.Intensity[i]
			}
			if i < len(pc.Classification) {
				pc.Classification[kept] = pc.Classification[i]
			}
			kept++
		}
	}

	pc.X = pc.X[:kept]
	pc.Y = pc.Y[:kept]
	pc.Z = pc.Z[:kept]
	if len(pc.Intensity) >= kept {
		pc.Intensity = pc.Intensity[:kept]
	}
	if len(pc.Classification) >= kept {
		pc.Classification = pc.Classification[:kept]
	}
	pc.PointCount = kept
}
