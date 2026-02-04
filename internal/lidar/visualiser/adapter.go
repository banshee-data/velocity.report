// Package visualiser provides gRPC streaming of LiDAR perception data.
// This file provides adapters to convert pipeline outputs to the canonical model.
package visualiser

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
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

// FrameAdapter converts pipeline outputs to the canonical FrameBundle model.
type FrameAdapter struct {
	sensorID string
	frameID  uint64

	// Performance tracking
	totalAdaptTimeNs atomic.Int64
	frameCount       atomic.Uint64
	lastLogTime      time.Time
}

// NewFrameAdapter creates a new FrameAdapter for the given sensor.
func NewFrameAdapter(sensorID string) *FrameAdapter {
	return &FrameAdapter{
		sensorID:    sensorID,
		lastLogTime: time.Now(),
	}
}

// AdaptFrame converts a LiDARFrame and tracking outputs to a FrameBundle.
func (a *FrameAdapter) AdaptFrame(
	frame *lidar.LiDARFrame,
	foregroundMask []bool,
	clusters []lidar.WorldCluster,
	tracker *lidar.Tracker,
) interface{} {
	startTime := time.Now()
	a.frameID++

	bundle := NewFrameBundle(a.frameID, a.sensorID, frame.StartTimestamp)

	// Adapt point cloud
	if frame != nil && len(frame.Points) > 0 {
		bundle.PointCloud = a.adaptPointCloud(frame, foregroundMask)
	}

	// Adapt clusters
	if len(clusters) > 0 {
		bundle.Clusters = a.adaptClusters(clusters, frame.StartTimestamp)
	}

	// Adapt tracks
	if tracker != nil {
		bundle.Tracks = a.adaptTracks(tracker, frame.StartTimestamp)
	}

	// Track performance
	adaptTime := time.Since(startTime)
	a.totalAdaptTimeNs.Add(adaptTime.Nanoseconds())
	count := a.frameCount.Add(1)

	// Log stats every 100 frames
	if count%100 == 0 {
		avgAdaptMs := float64(a.totalAdaptTimeNs.Load()) / float64(count) / 1e6
		pointCount := 0
		if bundle.PointCloud != nil {
			pointCount = bundle.PointCloud.PointCount
		}
		trackCount := 0
		if bundle.Tracks != nil {
			trackCount = len(bundle.Tracks.Tracks)
		}
		// Estimate memory size: ~16 bytes per point (4x float32) + overhead
		estimatedSizeMB := float64(pointCount*16) / (1024 * 1024)
		log.Printf("[Adapter] Stats: frames=%d avg_adapt_ms=%.3f last_frame: points=%d tracks=%d est_size_mb=%.2f",
			count, avgAdaptMs, pointCount, trackCount, estimatedSizeMB)
	}

	return bundle
}

// adaptPointCloud converts a LiDARFrame to a PointCloudFrame.
// Uses sync.Pool for slice allocation to reduce GC pressure.
func (a *FrameAdapter) adaptPointCloud(frame *lidar.LiDARFrame, mask []bool) *PointCloudFrame {
	n := len(frame.Points)
	pc := &PointCloudFrame{
		FrameID:        a.frameID,
		TimestampNanos: frame.StartTimestamp.UnixNano(),
		SensorID:       a.sensorID,
		X:              getFloat32Slice(n),
		Y:              getFloat32Slice(n),
		Z:              getFloat32Slice(n),
		Intensity:      getUint8Slice(n),
		Classification: getUint8Slice(n),
		DecimationMode: DecimationNone,
		PointCount:     n,
	}

	for i, p := range frame.Points {
		pc.X[i] = float32(p.X)
		pc.Y[i] = float32(p.Y)
		pc.Z[i] = float32(p.Z)
		pc.Intensity[i] = p.Intensity

		// Classification: 0=background, 1=foreground
		if i < len(mask) && mask[i] {
			pc.Classification[i] = 1
		} else {
			pc.Classification[i] = 0
		}
	}

	return pc
}

// Release returns the PointCloudFrame's slices to the pool.
// Call this after the frame has been sent to free memory for reuse.
func (pc *PointCloudFrame) Release() {
	if pc == nil {
		return
	}
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
		// Voxel decimation is more complex and not implemented yet
		// Fall back to uniform decimation
		pc.applyUniformDecimation(ratio)
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

// adaptClusters converts WorldClusters to the canonical Cluster format.
func (a *FrameAdapter) adaptClusters(worldClusters []lidar.WorldCluster, timestamp time.Time) *ClusterSet {
	cs := &ClusterSet{
		FrameID:        a.frameID,
		TimestampNanos: timestamp.UnixNano(),
		Clusters:       make([]Cluster, len(worldClusters)),
		Method:         ClusteringDBSCAN,
	}

	for i, wc := range worldClusters {
		cs.Clusters[i] = Cluster{
			ClusterID:      wc.ClusterID,
			SensorID:       wc.SensorID,
			TimestampNanos: wc.TSUnixNanos,
			CentroidX:      wc.CentroidX,
			CentroidY:      wc.CentroidY,
			CentroidZ:      wc.CentroidZ,
			AABBLength:     wc.BoundingBoxLength,
			AABBWidth:      wc.BoundingBoxWidth,
			AABBHeight:     wc.BoundingBoxHeight,
			PointsCount:    wc.PointsCount,
			HeightP95:      wc.HeightP95,
			IntensityMean:  wc.IntensityMean,
		}
	}

	return cs
}

// adaptTracks converts TrackedObjects to the canonical Track format.
func (a *FrameAdapter) adaptTracks(tracker *lidar.Tracker, timestamp time.Time) *TrackSet {
	activeTracks := tracker.GetActiveTracks()

	ts := &TrackSet{
		FrameID:        a.frameID,
		TimestampNanos: timestamp.UnixNano(),
		Tracks:         make([]Track, 0, len(activeTracks)),
		Trails:         make([]TrackTrail, 0, len(activeTracks)),
	}

	for _, t := range activeTracks {
		track := Track{
			TrackID:           t.TrackID,
			SensorID:          t.SensorID,
			State:             adaptTrackState(t.State),
			Hits:              t.Hits,
			Misses:            t.Misses,
			ObservationCount:  t.ObservationCount,
			FirstSeenNanos:    t.FirstUnixNanos,
			LastSeenNanos:     t.LastUnixNanos,
			X:                 t.X,
			Y:                 t.Y,
			Z:                 0, // 2D tracking
			VX:                t.VX,
			VY:                t.VY,
			VZ:                0,
			SpeedMps:          t.Speed(),
			HeadingRad:        t.Heading(),
			BBoxLengthAvg:     t.BoundingBoxLengthAvg,
			BBoxWidthAvg:      t.BoundingBoxWidthAvg,
			BBoxHeightAvg:     t.BoundingBoxHeightAvg,
			HeightP95Max:      t.HeightP95Max,
			IntensityMeanAvg:  t.IntensityMeanAvg,
			AvgSpeedMps:       t.AvgSpeedMps,
			PeakSpeedMps:      t.PeakSpeedMps,
			ClassLabel:        t.ObjectClass,
			ClassConfidence:   t.ObjectConfidence,
			TrackLengthMetres: t.TrackLengthMeters,
			TrackDurationSecs: t.TrackDurationSecs,
			OcclusionCount:    t.OcclusionCount,
		}

		// Copy covariance
		if t.P != [16]float32{} {
			track.Covariance4x4 = t.P[:]
		}

		ts.Tracks = append(ts.Tracks, track)

		// Build trail from history
		// Note: Take a snapshot of history length to avoid race conditions
		// if history is being modified concurrently
		historyLen := len(t.History)
		if historyLen > 0 {
			trail := TrackTrail{
				TrackID: t.TrackID,
				Points:  make([]TrackPoint, historyLen),
			}
			for j := 0; j < historyLen; j++ {
				hp := t.History[j]
				trail.Points[j] = TrackPoint{
					X:              hp.X,
					Y:              hp.Y,
					TimestampNanos: hp.Timestamp,
				}
			}
			ts.Trails = append(ts.Trails, trail)
		}
	}

	return ts
}

// adaptTrackState converts lidar.TrackState to visualiser.TrackState.
func adaptTrackState(state lidar.TrackState) TrackState {
	switch state {
	case lidar.TrackTentative:
		return TrackStateTentative
	case lidar.TrackConfirmed:
		return TrackStateConfirmed
	case lidar.TrackDeleted:
		return TrackStateDeleted
	default:
		return TrackStateUnknown
	}
}
