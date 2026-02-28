// Package visualiser provides gRPC streaming of LiDAR perception data.
// This file provides adapters to convert pipeline outputs to the canonical model.
package visualiser

import (
	"log"
	"sort"
	"sync/atomic"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/debug"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// FrameAdapter converts pipeline outputs to the canonical FrameBundle model.
type FrameAdapter struct {
	sensorID string
	frameID  uint64

	// SplitStreaming, when true, causes adaptPointCloud to emit only
	// foreground-classified points. Background points are delivered via
	// a separate BackgroundSnapshot message, so including them here
	// wastes CPU and bandwidth.
	SplitStreaming bool

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
// debugFrame is optional debug data from the tracking algorithm.
func (a *FrameAdapter) AdaptFrame(
	frame *l2frames.LiDARFrame,
	foregroundMask []bool,
	clusters []l4perception.WorldCluster,
	tracker l5tracks.TrackerInterface,
	debugFrame interface{}, // *debug.DebugFrame or nil
) interface{} {
	startTime := time.Now()
	a.frameID++

	bundle := NewFrameBundle(a.frameID, a.sensorID, frame.StartTimestamp)

	// Adapt point cloud
	if frame != nil && len(frame.Points) > 0 {
		bundle.PointCloud = a.adaptPointCloud(frame, foregroundMask)
	}

	// M3.5: Mark frame type when split streaming is active.
	// This tells the publisher not to re-strip background points
	// (the adapter already emitted foreground-only).
	if a.SplitStreaming && len(foregroundMask) > 0 {
		bundle.FrameType = FrameTypeForeground
	}

	// Adapt tracks first so we can identify which clusters are already tracked
	if tracker != nil {
		bundle.Tracks = a.adaptTracks(tracker, frame.StartTimestamp)
	}

	// Adapt only unassociated clusters. When a cluster has been matched to
	// a track by the tracker's association step, its bounding box would
	// duplicate the track's box. We use the real association data from the
	// tracker rather than spatial proximity so that genuinely close objects
	// (e.g. pedestrians walking together) are preserved.
	if len(clusters) > 0 {
		var associations []string
		if tracker != nil {
			associations = tracker.GetLastAssociations()
		}
		bundle.Clusters = a.adaptUnassociatedClusters(clusters, associations, frame.StartTimestamp)
	}

	// M6: Adapt debug overlays if provided
	if debugFrame != nil {
		bundle.Debug = a.adaptDebugFrame(debugFrame, frame.StartTimestamp)
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
func (a *FrameAdapter) adaptPointCloud(frame *l2frames.LiDARFrame, mask []bool) *PointCloudFrame {
	// When split streaming is active, emit only foreground points.
	// Background is delivered via BackgroundSnapshot every 30s, so
	// including it here wastes allocation, copy, and serialisation
	// time for ~97% of points that the publisher would strip anyway.
	if a.SplitStreaming && len(mask) > 0 {
		return a.adaptForegroundOnly(frame, mask)
	}

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

// adaptForegroundOnly builds a PointCloudFrame containing only foreground-
// classified points. This avoids allocating and copying ~67k background
// points that would be stripped by the publisher anyway.
func (a *FrameAdapter) adaptForegroundOnly(frame *l2frames.LiDARFrame, mask []bool) *PointCloudFrame {
	// Count foreground points for precise allocation
	fgCount := 0
	for _, fg := range mask {
		if fg {
			fgCount++
		}
	}

	pc := &PointCloudFrame{
		FrameID:        a.frameID,
		TimestampNanos: frame.StartTimestamp.UnixNano(),
		SensorID:       a.sensorID,
		X:              make([]float32, 0, fgCount),
		Y:              make([]float32, 0, fgCount),
		Z:              make([]float32, 0, fgCount),
		Intensity:      make([]uint8, 0, fgCount),
		Classification: make([]uint8, 0, fgCount),
		DecimationMode: DecimationForegroundOnly,
		PointCount:     0,
	}

	for i, p := range frame.Points {
		if i < len(mask) && mask[i] {
			pc.X = append(pc.X, float32(p.X))
			pc.Y = append(pc.Y, float32(p.Y))
			pc.Z = append(pc.Z, float32(p.Z))
			pc.Intensity = append(pc.Intensity, p.Intensity)
			pc.Classification = append(pc.Classification, 1)
		}
	}
	pc.PointCount = len(pc.X)

	return pc
}

// Release decrements the reference count and returns slices to the pool when
// the count reaches zero. Call this after the frame has been consumed.
//
// In broadcast scenarios (where the same frame is sent to multiple clients),
// each consumer should call Retain() before use and Release() after use.
// The slices are only returned to the pool when the last consumer releases.
//
// For frames created with SplitStreaming=true (adaptForegroundOnly), pooled
// slices are not used, so Release() is a no-op for those frames.

// adaptUnassociatedClusters converts WorldClusters to the canonical Cluster
// format, skipping clusters that the tracker associated with an existing track.
// associations is the slice returned by tracker.GetLastAssociations() — it is
// indexed by cluster index and contains the matched trackID or "" for
// unassociated clusters. Only unassociated clusters are included so that
// the track's bounding box is the sole representation of a tracked object.
func (a *FrameAdapter) adaptUnassociatedClusters(worldClusters []l4perception.WorldCluster, associations []string, timestamp time.Time) *ClusterSet {
	cs := &ClusterSet{
		FrameID:        a.frameID,
		TimestampNanos: timestamp.UnixNano(),
		Clusters:       make([]Cluster, 0, len(worldClusters)),
		Method:         ClusteringDBSCAN,
	}

	for i, wc := range worldClusters {
		// Skip clusters that are already associated with a track.
		// Their bounding box is rendered via the track instead.
		if i < len(associations) && associations[i] != "" {
			continue
		}

		cluster := Cluster{
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
			SamplePoints:   flattenSamplePoints(wc.SamplePoints),
		}

		// Include OBB if computed
		if wc.OBB != nil {
			cluster.OBB = &OrientedBoundingBox{
				CenterX:    wc.OBB.CenterX,
				CenterY:    wc.OBB.CenterY,
				CenterZ:    wc.OBB.CenterZ,
				Length:     wc.OBB.Length,
				Width:      wc.OBB.Width,
				Height:     wc.OBB.Height,
				HeadingRad: wc.OBB.HeadingRad,
			}
		}

		cs.Clusters = append(cs.Clusters, cluster)
	}

	return cs
}

// adaptClusters converts WorldClusters to the canonical Cluster format.
func (a *FrameAdapter) adaptClusters(worldClusters []l4perception.WorldCluster, timestamp time.Time) *ClusterSet {
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
			SamplePoints:   flattenSamplePoints(wc.SamplePoints),
		}
	}

	return cs
}

// adaptTracks converts TrackedObjects to the canonical Track format.
func (a *FrameAdapter) adaptTracks(tracker l5tracks.TrackerInterface, timestamp time.Time) *TrackSet {
	activeTracks := tracker.GetActiveTracks()

	ts := &TrackSet{
		FrameID:        a.frameID,
		TimestampNanos: timestamp.UnixNano(),
		Tracks:         make([]Track, 0, len(activeTracks)),
		Trails:         make([]TrackTrail, 0, len(activeTracks)),
	}

	for _, t := range activeTracks {
		median, p85, p98 := speedPercentiles(t.SpeedHistory())
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
			Z:                 t.LatestZ, // Ground-level Z from cluster OBB
			VX:                t.VX,
			VY:                t.VY,
			VZ:                0,
			SpeedMps:          t.Speed(),
			HeadingRad:        t.Heading(),
			BBoxLength:        t.OBBLength,
			BBoxWidth:         t.OBBWidth,
			BBoxHeight:        t.OBBHeight,
			BBoxHeadingRad:    t.OBBHeadingRad, // Smoothed OBB heading
			HeightP95Max:      t.HeightP95Max,
			IntensityMeanAvg:  t.IntensityMeanAvg,
			AvgSpeedMps:       t.AvgSpeedMps,
			MedianSpeedMps:    median,
			PeakSpeedMps:      t.PeakSpeedMps,
			P85SpeedMps:       p85,
			P98SpeedMps:       p98,
			ObjectClass:       t.ObjectClass,
			ClassConfidence:   t.ObjectConfidence,
			TrackLengthMetres: t.TrackLengthMeters,
			TrackDurationSecs: t.TrackDurationSecs,
			OcclusionCount:    t.OcclusionCount,
			Alpha:             1.0, // Fully visible
			HeadingSource:     int(t.HeadingSource),
		}

		// Copy covariance
		if t.P != [16]float32{} {
			track.Covariance4x4 = t.P[:]
		}

		ts.Tracks = append(ts.Tracks, track)

		// Build trail from history
		// History is deep-copied by GetActiveTracks(), safe to iterate directly.
		if len(t.History) > 0 {
			trail := TrackTrail{
				TrackID: t.TrackID,
				Points:  make([]TrackPoint, len(t.History)),
			}
			for j, hp := range t.History {
				trail.Points[j] = TrackPoint{
					X:              hp.X,
					Y:              hp.Y,
					TimestampNanos: hp.Timestamp,
				}
			}
			ts.Trails = append(ts.Trails, trail)
		}
	}

	// Include recently-deleted tracks with fade-out alpha for smooth disappearance.
	// Only render fade-out for tracks that were previously confirmed
	// (ObservationCount >= HitsToConfirm). Tentative tracks that never
	// confirmed are short-lived noise — rendering their fade-out produces
	// clusters of stale red boxes around active objects.
	nowNanos := timestamp.UnixNano()
	deletedTracks := tracker.GetRecentlyDeletedTracks(nowNanos)
	gracePeriodNanos := float64(tracker.GetDeletedTrackGracePeriod())

	for _, t := range deletedTracks {
		// Skip tracks that never reached confirmed state.
		if t.ObservationCount < 3 {
			continue
		}

		elapsed := float64(nowNanos - t.LastUnixNanos)
		alpha := float32(1.0 - elapsed/gracePeriodNanos)
		if alpha < 0 {
			alpha = 0
		}

		median, p85, p98 := speedPercentiles(t.SpeedHistory())
		track := Track{
			TrackID:           t.TrackID,
			SensorID:          t.SensorID,
			State:             TrackStateDeleted,
			Hits:              t.Hits,
			Misses:            t.Misses,
			ObservationCount:  t.ObservationCount,
			FirstSeenNanos:    t.FirstUnixNanos,
			LastSeenNanos:     t.LastUnixNanos,
			X:                 t.X,
			Y:                 t.Y,
			Z:                 t.LatestZ,
			VX:                t.VX,
			VY:                t.VY,
			VZ:                0,
			SpeedMps:          t.Speed(),
			HeadingRad:        t.Heading(),
			BBoxLength:        t.OBBLength,
			BBoxWidth:         t.OBBWidth,
			BBoxHeight:        t.OBBHeight,
			BBoxHeadingRad:    t.OBBHeadingRad,
			HeightP95Max:      t.HeightP95Max,
			IntensityMeanAvg:  t.IntensityMeanAvg,
			AvgSpeedMps:       t.AvgSpeedMps,
			MedianSpeedMps:    median,
			PeakSpeedMps:      t.PeakSpeedMps,
			P85SpeedMps:       p85,
			P98SpeedMps:       p98,
			ObjectClass:       t.ObjectClass,
			ClassConfidence:   t.ObjectConfidence,
			TrackLengthMetres: t.TrackLengthMeters,
			TrackDurationSecs: t.TrackDurationSecs,
			OcclusionCount:    t.OcclusionCount,
			Alpha:             alpha, // Fade from 1.0 → 0.0 over grace period
		}

		ts.Tracks = append(ts.Tracks, track)

		// Include trail for fading tracks too
		if len(t.History) > 0 {
			trail := TrackTrail{
				TrackID: t.TrackID,
				Points:  make([]TrackPoint, len(t.History)),
			}
			for j, hp := range t.History {
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

// adaptTrackState converts l5tracks.TrackState to visualiser.TrackState.
func adaptTrackState(state l5tracks.TrackState) TrackState {
	switch state {
	case l5tracks.TrackTentative:
		return TrackStateTentative
	case l5tracks.TrackConfirmed:
		return TrackStateConfirmed
	case l5tracks.TrackDeleted:
		return TrackStateDeleted
	default:
		return TrackStateUnknown
	}
}

// flattenSamplePoints converts [][3]float32 to a flat xyz-interleaved []float32
// for proto serialisation. Returns nil when the input is empty.
func flattenSamplePoints(pts [][3]float32) []float32 {
	if len(pts) == 0 {
		return nil
	}
	flat := make([]float32, 0, len(pts)*3)
	for _, p := range pts {
		flat = append(flat, p[0], p[1], p[2])
	}
	return flat
}

// speedPercentiles computes median (p50), p85, and p98 from a speed history
// slice using floor-index percentile selection. Returns (0, 0, 0) when the
// slice is empty. The input slice is sorted in-place — callers that need
// the original order must pass a copy.
func speedPercentiles(speeds []float32) (median, p85, p98 float32) {
	n := len(speeds)
	if n == 0 {
		return 0, 0, 0
	}
	sort.Slice(speeds, func(i, j int) bool { return speeds[i] < speeds[j] })

	percentile := func(p float64) float32 {
		// int() truncation is equivalent to floor for non-negative values.
		idx := int(p / 100.0 * float64(n))
		if idx >= n {
			idx = n - 1
		}
		return speeds[idx]
	}
	return percentile(50), percentile(85), percentile(98)
}

// adaptDebugFrame converts debug.DebugFrame to visualiser.DebugOverlaySet.
func (a *FrameAdapter) adaptDebugFrame(debugFrame interface{}, timestamp time.Time) *DebugOverlaySet {
	// Type-assert to debug.DebugFrame
	df, ok := debugFrame.(*debug.DebugFrame)
	if !ok || df == nil {
		return nil
	}

	overlay := &DebugOverlaySet{
		FrameID:               df.FrameID,
		TimestampNanos:        timestamp.UnixNano(),
		AssociationCandidates: make([]AssociationCandidate, len(df.AssociationCandidates)),
		GatingEllipses:        make([]GatingEllipse, len(df.GatingRegions)),
		Residuals:             make([]InnovationResidual, len(df.Innovations)),
		Predictions:           make([]StatePrediction, len(df.StatePredictions)),
	}

	// Convert association candidates
	for i, rec := range df.AssociationCandidates {
		overlay.AssociationCandidates[i] = AssociationCandidate{
			ClusterID: rec.ClusterID,
			TrackID:   rec.TrackID,
			Distance:  rec.MahalanobisDistSquared,
			Accepted:  rec.Accepted,
		}
	}

	// Convert gating regions
	for i, region := range df.GatingRegions {
		overlay.GatingEllipses[i] = GatingEllipse{
			TrackID:     region.TrackID,
			CenterX:     region.CenterX,
			CenterY:     region.CenterY,
			SemiMajor:   region.SemiMajorM,
			SemiMinor:   region.SemiMinorM,
			RotationRad: region.RotationRad,
		}
	}

	// Convert innovations
	for i, innov := range df.Innovations {
		overlay.Residuals[i] = InnovationResidual{
			TrackID:           innov.TrackID,
			PredictedX:        innov.PredictedX,
			PredictedY:        innov.PredictedY,
			MeasuredX:         innov.MeasuredX,
			MeasuredY:         innov.MeasuredY,
			ResidualMagnitude: innov.ResidualMag,
		}
	}

	// Convert predictions
	for i, pred := range df.StatePredictions {
		overlay.Predictions[i] = StatePrediction{
			TrackID: pred.TrackID,
			X:       pred.X,
			Y:       pred.Y,
			VX:      pred.VX,
			VY:      pred.VY,
		}
	}

	return overlay
}
