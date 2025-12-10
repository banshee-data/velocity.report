package lidar

import "math"

// Phase 2: ML-Ready Feature Extraction (Scaffolding)
// This module provides feature extraction for machine learning classification.

// SpatialFeatures captures geometric properties of a track.
type SpatialFeatures struct {
	BBoxLength   float32 // Bounding box length (m)
	BBoxWidth    float32 // Bounding box width (m)
	BBoxHeight   float32 // Bounding box height (m)
	BBoxVolume   float32 // Volume (m³)
	BBoxAspectXY float32 // Length/Width
	BBoxAspectXZ float32 // Length/Height
	HeightP50    float32 // Median height
	HeightP95    float32 // 95th percentile height
	HeightStdDev float32 // Height variance
	PointDensity float32 // Points per m³
}

// KinematicFeatures captures motion properties of a track.
type KinematicFeatures struct {
	SpeedMean       float32 // Average speed (m/s)
	SpeedStdDev     float32 // Speed variance
	SpeedP50        float32 // Median speed
	SpeedP85        float32 // 85th percentile
	SpeedP95        float32 // 95th percentile
	AccelMean       float32 // Average acceleration (m/s²)
	AccelStdDev     float32 // Acceleration variance
	HeadingVariance float32 // Angular dispersion (radians²)
	Straightness    float32 // Path straightness (0-1)
}

// TemporalFeatures captures lifecycle properties of a track.
type TemporalFeatures struct {
	Duration         float32 // Track duration (seconds)
	ObservationCount int     // Frame count
	ObservationRate  float32 // Observations per second
	OcclusionRatio   float32 // Gaps / total duration
	MaxOcclusionSecs float32 // Longest gap (seconds)
}

// AppearanceFeatures captures intensity/reflectivity properties.
type AppearanceFeatures struct {
	IntensityMean   float32 // Average intensity
	IntensityStdDev float32 // Intensity variance
	IntensityP50    float32 // Median intensity
	IntensityP95    float32 // 95th percentile
}

// TrackFeatureVector combines all feature categories for ML classification.
type TrackFeatureVector struct {
	TrackID    string
	Spatial    SpatialFeatures
	Kinematic  KinematicFeatures
	Temporal   TemporalFeatures
	Appearance AppearanceFeatures

	// Normalized features for ML (z-score or min-max)
	FeaturesNormalized []float32 // 30-40 features
}

// ExtractFeatures computes a feature vector from a TrackedObject.
// Phase 2: Implementation placeholder - currently extracts basic features.
func ExtractFeatures(track *TrackedObject) *TrackFeatureVector {
	fv := &TrackFeatureVector{
		TrackID: track.TrackID,
	}

	// Spatial features
	fv.Spatial.BBoxLength = track.BoundingBoxLengthAvg
	fv.Spatial.BBoxWidth = track.BoundingBoxWidthAvg
	fv.Spatial.BBoxHeight = track.BoundingBoxHeightAvg
	fv.Spatial.BBoxVolume = track.BoundingBoxLengthAvg * track.BoundingBoxWidthAvg * track.BoundingBoxHeightAvg
	if track.BoundingBoxWidthAvg > 0 {
		fv.Spatial.BBoxAspectXY = track.BoundingBoxLengthAvg / track.BoundingBoxWidthAvg
	}
	if track.BoundingBoxHeightAvg > 0 {
		fv.Spatial.BBoxAspectXZ = track.BoundingBoxLengthAvg / track.BoundingBoxHeightAvg
	}
	fv.Spatial.HeightP95 = track.HeightP95Max
	// TODO: Compute HeightP50, HeightStdDev, PointDensity from observation history

	// Kinematic features
	fv.Kinematic.SpeedMean = track.AvgSpeedMps
	p50, p85, p95 := ComputeSpeedPercentiles(track.speedHistory)
	fv.Kinematic.SpeedP50 = p50
	fv.Kinematic.SpeedP85 = p85
	fv.Kinematic.SpeedP95 = p95
	// TODO: Compute SpeedStdDev, AccelMean, AccelStdDev, HeadingVariance, Straightness

	// Temporal features
	fv.Temporal.Duration = track.TrackDurationSecs
	fv.Temporal.ObservationCount = track.ObservationCount
	if track.TrackDurationSecs > 0 {
		fv.Temporal.ObservationRate = float32(track.ObservationCount) / track.TrackDurationSecs
	}
	if track.ObservationCount > 0 {
		fv.Temporal.OcclusionRatio = float32(track.OcclusionCount) / float32(track.ObservationCount)
	}
	if track.MaxOcclusionFrames > 0 {
		fv.Temporal.MaxOcclusionSecs = float32(track.MaxOcclusionFrames) * 0.1 // Assume 10Hz
	}

	// Appearance features
	fv.Appearance.IntensityMean = track.IntensityMeanAvg
	// TODO: Compute IntensityStdDev, IntensityP50, IntensityP95 from observation history

	// TODO Phase 2: Normalize features and populate FeaturesNormalized array

	return fv
}

// ComputeStraightness calculates path straightness metric (0=curved, 1=straight).
// Straightness = (start-to-end distance) / (total path length)
func ComputeStraightness(history []TrackPoint) float32 {
	if len(history) < 2 {
		return 0
	}

	// Start to end distance
	dx := history[len(history)-1].X - history[0].X
	dy := history[len(history)-1].Y - history[0].Y
	directDist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

	// Total path length
	var totalDist float32
	for i := 1; i < len(history); i++ {
		dx := history[i].X - history[i-1].X
		dy := history[i].Y - history[i-1].Y
		totalDist += float32(math.Sqrt(float64(dx*dx + dy*dy)))
	}

	if totalDist > 0 {
		return directDist / totalDist
	}
	return 0
}

// FeatureScaler provides z-score normalization for feature vectors.
// Phase 2: Placeholder for feature scaling infrastructure.
type FeatureScaler struct {
	Mean   []float32
	StdDev []float32
}

// Classifier interface for ML models (Phase 2 scaffolding).
type Classifier interface {
	Classify(features *TrackFeatureVector) ClassificationResult
	Model() string // Model identifier/version
}

// ONNXClassifier loads and runs ONNX models for classification.
// Phase 2: Scaffolding - actual ONNX integration to be implemented.
type ONNXClassifier struct {
	ModelPath string
	ModelType string // "random_forest", "xgboost", "neural_net"
	Scaler    *FeatureScaler
	ClassMap  map[int]string // Model output → class name
}

// Classify runs inference using the ONNX model.
// Phase 2: Placeholder implementation.
func (c *ONNXClassifier) Classify(features *TrackFeatureVector) ClassificationResult {
	// TODO Phase 2: Implement ONNX model loading and inference
	// For now, return a placeholder result
	return ClassificationResult{
		Class:      ClassOther,
		Confidence: 0.5,
		Model:      "onnx_placeholder",
	}
}

// Model returns the classifier identifier.
func (c *ONNXClassifier) Model() string {
	return c.ModelPath
}
