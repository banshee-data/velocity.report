package l6objects

import (
	"math"
	"sort"
)

// ClusterFeatures captures per-cluster spatial and intensity features.
// These are extracted from a single frame's cluster for use in classification
// and ML feature-vector export.
type ClusterFeatures struct {
	PointCount     int
	BBoxLength     float32
	BBoxWidth      float32
	BBoxHeight     float32
	HeightP95      float32
	IntensityMean  float32
	IntensityStd   float32
	Elongation     float32 // length / width (clamped to avoid Inf)
	Compactness    float32 // points / bbox_volume (clamped)
	VerticalSpread float32 // std-dev of Z
}

// TrackFeatures aggregates cluster features across a track's lifetime
// and adds kinematic features. This struct is the primary input for
// ML-based classification training data export.
type TrackFeatures struct {
	ClusterFeatures // latest observation features

	// Kinematic features
	AvgSpeedMps       float32
	PeakSpeedMps      float32
	SpeedVariance     float32
	TrackDurationSecs float32
	TrackLengthMeters float32
	HeadingVariance   float32
	OcclusionRatio    float32

	// Speed percentiles
	SpeedP50 float32
	SpeedP85 float32
	SpeedP95 float32
}

// ExtractClusterFeatures computes features from a cluster and its constituent points.
func ExtractClusterFeatures(cluster WorldCluster, points []WorldPoint) ClusterFeatures {
	f := ClusterFeatures{
		PointCount:    cluster.PointsCount,
		BBoxLength:    cluster.BoundingBoxLength,
		BBoxWidth:     cluster.BoundingBoxWidth,
		BBoxHeight:    cluster.BoundingBoxHeight,
		HeightP95:     cluster.HeightP95,
		IntensityMean: cluster.IntensityMean,
	}

	// Elongation = length / width (use OBB dimensions if available)
	length := f.BBoxLength
	width := f.BBoxWidth
	if cluster.OBB != nil {
		length = cluster.OBB.Length
		width = cluster.OBB.Width
	}
	if width > 0.01 {
		f.Elongation = length / width
	} else {
		f.Elongation = 1.0
	}

	// Compactness = points / volume
	volume := f.BBoxLength * f.BBoxWidth * f.BBoxHeight
	if volume > 0.001 {
		f.Compactness = float32(f.PointCount) / volume
	}

	// Intensity std-dev and vertical spread require per-point data
	if len(points) > 1 {
		// Intensity std-dev
		var sumI, sumI2 float64
		for _, p := range points {
			v := float64(p.Intensity)
			sumI += v
			sumI2 += v * v
		}
		n := float64(len(points))
		meanI := sumI / n
		varI := sumI2/n - meanI*meanI
		if varI > 0 {
			f.IntensityStd = float32(math.Sqrt(varI))
		}

		// Vertical spread (std-dev of Z)
		var sumZ, sumZ2 float64
		for _, p := range points {
			sumZ += p.Z
			sumZ2 += p.Z * p.Z
		}
		meanZ := sumZ / n
		varZ := sumZ2/n - meanZ*meanZ
		if varZ > 0 {
			f.VerticalSpread = float32(math.Sqrt(varZ))
		}
	}

	return f
}

// ExtractTrackFeatures computes the full feature vector for a tracked object,
// combining the latest cluster-level features with kinematic history.
func ExtractTrackFeatures(track *TrackedObject) TrackFeatures {
	f := TrackFeatures{}

	// Spatial features from track averages
	f.BBoxLength = track.BoundingBoxLengthAvg
	f.BBoxWidth = track.BoundingBoxWidthAvg
	f.BBoxHeight = track.BoundingBoxHeightAvg
	f.HeightP95 = track.HeightP95Max
	f.IntensityMean = track.IntensityMeanAvg
	f.PointCount = track.ObservationCount
	if f.BBoxWidth > 0.01 {
		f.Elongation = f.BBoxLength / f.BBoxWidth
	} else {
		f.Elongation = 1.0
	}
	volume := f.BBoxLength * f.BBoxWidth * f.BBoxHeight
	if volume > 0.001 {
		f.Compactness = float32(f.PointCount) / volume
	}

	// Kinematic features
	f.AvgSpeedMps = track.AvgSpeedMps
	f.PeakSpeedMps = track.PeakSpeedMps
	f.TrackDurationSecs = track.TrackDurationSecs
	f.TrackLengthMeters = track.TrackLengthMeters

	// Occlusion ratio
	totalFrames := track.ObservationCount + track.OcclusionCount
	if totalFrames > 0 {
		f.OcclusionRatio = float32(track.OcclusionCount) / float32(totalFrames)
	}

	// Speed percentiles and variance
	if len(track.speedHistory) > 0 {
		f.SpeedP50, f.SpeedP85, f.SpeedP95 = ComputeSpeedPercentiles(track.speedHistory)
		f.SpeedVariance = computeVariance(track.speedHistory)
	}

	// Heading variance from history
	if len(track.History) >= 3 {
		f.HeadingVariance = computeHeadingVariance(track.History)
	}

	return f
}

// computeVariance computes the variance of a float32 slice.
func computeVariance(vals []float32) float32 {
	if len(vals) < 2 {
		return 0
	}
	var sum, sum2 float64
	for _, v := range vals {
		fv := float64(v)
		sum += fv
		sum2 += fv * fv
	}
	n := float64(len(vals))
	mean := sum / n
	v := sum2/n - mean*mean
	if v < 0 {
		return 0
	}
	return float32(v)
}

// computeHeadingVariance computes variance of heading changes along a track.
// Uses atan2 of consecutive position differences.
func computeHeadingVariance(history []TrackPoint) float32 {
	if len(history) < 3 {
		return 0
	}

	headings := make([]float64, 0, len(history)-1)
	for i := 1; i < len(history); i++ {
		dx := float64(history[i].X - history[i-1].X)
		dy := float64(history[i].Y - history[i-1].Y)
		if dx*dx+dy*dy < 1e-6 {
			continue // skip stationary segments
		}
		headings = append(headings, math.Atan2(dy, dx))
	}

	if len(headings) < 2 {
		return 0
	}

	// Circular variance of heading changes (angular differences)
	diffs := make([]float64, 0, len(headings)-1)
	for i := 1; i < len(headings); i++ {
		d := headings[i] - headings[i-1]
		// Normalise to [-π, π]
		for d > math.Pi {
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		diffs = append(diffs, d)
	}

	if len(diffs) == 0 {
		return 0
	}

	var sum, sum2 float64
	for _, d := range diffs {
		sum += d
		sum2 += d * d
	}
	n := float64(len(diffs))
	mean := sum / n
	v := sum2/n - mean*mean
	if v < 0 {
		return 0
	}
	return float32(v)
}

// SortedFeatureNames returns the canonical feature names in the order
// they should appear in an exported feature vector (for ML training).
func SortedFeatureNames() []string {
	return []string{
		"point_count",
		"bbox_length",
		"bbox_width",
		"bbox_height",
		"height_p95",
		"intensity_mean",
		"intensity_std",
		"elongation",
		"compactness",
		"vertical_spread",
		"avg_speed_mps",
		"peak_speed_mps",
		"speed_variance",
		"speed_p50",
		"speed_p85",
		"speed_p95",
		"track_duration_secs",
		"track_length_meters",
		"heading_variance",
		"occlusion_ratio",
	}
}

// ToVector converts TrackFeatures to a flat float32 slice in canonical order.
// The order matches SortedFeatureNames().
func (f TrackFeatures) ToVector() []float32 {
	return []float32{
		float32(f.PointCount),
		f.BBoxLength,
		f.BBoxWidth,
		f.BBoxHeight,
		f.HeightP95,
		f.IntensityMean,
		f.IntensityStd,
		f.Elongation,
		f.Compactness,
		f.VerticalSpread,
		f.AvgSpeedMps,
		f.PeakSpeedMps,
		f.SpeedVariance,
		f.SpeedP50,
		f.SpeedP85,
		f.SpeedP95,
		f.TrackDurationSecs,
		f.TrackLengthMeters,
		f.HeadingVariance,
		f.OcclusionRatio,
	}
}

// SortFeatureImportance returns feature names sorted by absolute contribution
// to classification. This is a placeholder for future ML model feature importance.
func SortFeatureImportance(vector []float32) []string {
	type kv struct {
		name string
		val  float32
	}
	names := SortedFeatureNames()
	pairs := make([]kv, len(names))
	for i, n := range names {
		v := float32(0)
		if i < len(vector) {
			v = vector[i]
		}
		if v < 0 {
			v = -v
		}
		pairs[i] = kv{n, v}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].val > pairs[j].val })
	result := make([]string, len(pairs))
	for i, p := range pairs {
		result[i] = p.name
	}
	return result
}
