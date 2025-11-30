package lidar

import (
	"math"
	"sort"
)

// ObjectClass represents the classification of a tracked object.
type ObjectClass string

const (
	// ClassPedestrian indicates a pedestrian or person
	ClassPedestrian ObjectClass = "pedestrian"
	// ClassCar indicates a car or vehicle
	ClassCar ObjectClass = "car"
	// ClassBird indicates a bird or small flying object
	ClassBird ObjectClass = "bird"
	// ClassOther indicates an unclassified object
	ClassOther ObjectClass = "other"
)

// Classification thresholds (configurable for tuning)
const (
	// Height thresholds (meters)
	BirdHeightMax       = 0.5 // Birds are typically small
	PedestrianHeightMin = 1.0 // Pedestrians are at least 1m tall
	PedestrianHeightMax = 2.2 // Pedestrians are typically under 2.2m
	VehicleHeightMin    = 1.2 // Vehicles are at least 1.2m
	VehicleLengthMin    = 3.0 // Vehicles are at least 3m long
	VehicleWidthMin     = 1.5 // Vehicles are at least 1.5m wide

	// Speed thresholds (m/s)
	BirdSpeedMax       = 1.0 // Birds detected at low speeds
	PedestrianSpeedMax = 3.0 // Pedestrians walk up to ~3 m/s (10.8 km/h)
	VehicleSpeedMin    = 5.0 // Vehicles typically move faster than 5 m/s
	StationarySpeedMax = 0.5 // Stationary threshold

	// Confidence levels
	HighConfidence   = 0.85
	MediumConfidence = 0.70
	LowConfidence    = 0.50

	// Minimum observations for classification
	MinObservationsForClassification = 5
)

// ClassificationResult holds the result of track classification.
type ClassificationResult struct {
	Class      ObjectClass
	Confidence float32
	Model      string // Model version used
	Features   ClassificationFeatures
}

// ClassificationFeatures holds the features used for classification.
type ClassificationFeatures struct {
	// Spatial features
	AvgHeight float32 // Average bounding box height
	AvgLength float32 // Average bounding box length
	AvgWidth  float32 // Average bounding box width
	HeightP95 float32 // Maximum P95 height

	// Kinematic features
	AvgSpeed  float32 // Average speed
	PeakSpeed float32 // Peak speed
	P50Speed  float32 // Median speed
	P85Speed  float32 // 85th percentile speed
	P95Speed  float32 // 95th percentile speed

	// Temporal features
	ObservationCount int
	DurationSecs     float32
}

// clampConfidence clamps a confidence value to the range [min, max].
func clampConfidence(value, min, max float32) float32 {
	if value > max {
		return max
	}
	if value < min {
		return min
	}
	return value
}

// TrackClassifier performs rule-based classification of tracked objects.
// This can be replaced with an ML model in future iterations.
type TrackClassifier struct {
	ModelVersion string
}

// NewTrackClassifier creates a new track classifier.
func NewTrackClassifier() *TrackClassifier {
	return &TrackClassifier{
		ModelVersion: "rule-based-v1.0",
	}
}

// Classify determines the object class for a tracked object.
// Returns the classification result with class, confidence, and features used.
func (tc *TrackClassifier) Classify(track *TrackedObject) ClassificationResult {
	features := tc.extractFeatures(track)

	result := ClassificationResult{
		Model:    tc.ModelVersion,
		Features: features,
	}

	// Not enough observations for reliable classification
	if features.ObservationCount < MinObservationsForClassification {
		result.Class = ClassOther
		result.Confidence = LowConfidence * 0.5 // Very low confidence
		return result
	}

	// Classification rules (priority order)
	// 1. Check for bird (small, low-speed)
	if tc.isBird(features) {
		result.Class = ClassBird
		result.Confidence = tc.birdConfidence(features)
		return result
	}

	// 2. Check for vehicle (large, fast)
	if tc.isVehicle(features) {
		result.Class = ClassCar
		result.Confidence = tc.vehicleConfidence(features)
		return result
	}

	// 3. Check for pedestrian (human-sized, moderate speed)
	if tc.isPedestrian(features) {
		result.Class = ClassPedestrian
		result.Confidence = tc.pedestrianConfidence(features)
		return result
	}

	// 4. Default to other
	result.Class = ClassOther
	result.Confidence = LowConfidence
	return result
}

// extractFeatures extracts classification features from a track.
func (tc *TrackClassifier) extractFeatures(track *TrackedObject) ClassificationFeatures {
	features := ClassificationFeatures{
		AvgHeight:        track.BoundingBoxHeightAvg,
		AvgLength:        track.BoundingBoxLengthAvg,
		AvgWidth:         track.BoundingBoxWidthAvg,
		HeightP95:        track.HeightP95Max,
		AvgSpeed:         track.AvgSpeedMps,
		PeakSpeed:        track.PeakSpeedMps,
		ObservationCount: track.ObservationCount,
	}

	// Compute speed percentiles from history using shared function
	if len(track.speedHistory) > 0 {
		features.P50Speed, features.P85Speed, features.P95Speed = ComputeSpeedPercentiles(track.speedHistory)
	}

	// Compute duration
	if track.LastUnixNanos > track.FirstUnixNanos {
		features.DurationSecs = float32(track.LastUnixNanos-track.FirstUnixNanos) / 1e9
	}

	return features
}

// isBird checks if features match bird classification.
func (tc *TrackClassifier) isBird(f ClassificationFeatures) bool {
	return f.AvgHeight < BirdHeightMax &&
		f.AvgSpeed < BirdSpeedMax &&
		f.AvgLength < 1.0 &&
		f.AvgWidth < 1.0
}

// birdConfidence computes confidence for bird classification.
func (tc *TrackClassifier) birdConfidence(f ClassificationFeatures) float32 {
	confidence := float32(MediumConfidence)

	// Higher confidence for very small objects
	if f.AvgHeight < 0.3 {
		confidence += 0.1
	}

	// Lower confidence if speed is very low (might be noise)
	if f.AvgSpeed < 0.1 {
		confidence -= 0.15
	}

	return clampConfidence(confidence, 0.0, 1.0)
}

// isVehicle checks if features match vehicle classification.
func (tc *TrackClassifier) isVehicle(f ClassificationFeatures) bool {
	// Large size AND high speed
	isLarge := f.AvgLength > VehicleLengthMin || f.AvgWidth > VehicleWidthMin
	isFast := f.AvgSpeed > VehicleSpeedMin || f.PeakSpeed > VehicleSpeedMin*1.5
	isTall := f.AvgHeight > VehicleHeightMin

	return (isLarge && isFast) || (isLarge && isTall)
}

// vehicleConfidence computes confidence for vehicle classification.
func (tc *TrackClassifier) vehicleConfidence(f ClassificationFeatures) float32 {
	confidence := float32(MediumConfidence)

	// Size factors
	if f.AvgLength > 4.0 {
		confidence += 0.1
	}
	if f.AvgWidth > 2.0 {
		confidence += 0.05
	}

	// Speed factors
	if f.AvgSpeed > 10.0 {
		confidence += 0.1
	}
	if f.PeakSpeed > 15.0 {
		confidence += 0.05
	}

	// More observations = more confidence
	if f.ObservationCount > 20 {
		confidence += 0.05
	}

	return clampConfidence(confidence, LowConfidence, HighConfidence)
}

// isPedestrian checks if features match pedestrian classification.
func (tc *TrackClassifier) isPedestrian(f ClassificationFeatures) bool {
	heightOK := f.AvgHeight >= PedestrianHeightMin && f.AvgHeight <= PedestrianHeightMax
	speedOK := f.AvgSpeed <= PedestrianSpeedMax
	sizeOK := f.AvgLength < VehicleLengthMin && f.AvgWidth < VehicleWidthMin

	return heightOK && speedOK && sizeOK
}

// pedestrianConfidence computes confidence for pedestrian classification.
func (tc *TrackClassifier) pedestrianConfidence(f ClassificationFeatures) float32 {
	confidence := float32(MediumConfidence)

	// Height in typical range
	if f.AvgHeight >= 1.5 && f.AvgHeight <= 1.9 {
		confidence += 0.1
	}

	// Speed in typical walking range
	if f.AvgSpeed >= 0.5 && f.AvgSpeed <= 2.0 {
		confidence += 0.1
	}

	// Consistent movement
	if f.ObservationCount > 15 {
		confidence += 0.05
	}

	return clampConfidence(confidence, LowConfidence, HighConfidence)
}

// ClassifyAndUpdate classifies a track and updates its classification fields.
// This should be called periodically or when track state changes.
func (tc *TrackClassifier) ClassifyAndUpdate(track *TrackedObject) {
	result := tc.Classify(track)
	track.ObjectClass = string(result.Class)
	track.ObjectConfidence = result.Confidence
	track.ClassificationModel = result.Model
}

// ObjectClass and ObjectConfidence fields to add to TrackedObject
// These are used by the classifier to store results.

// Extending TrackedObject with classification fields
// Add these fields to TrackedObject struct in tracking.go:
// ObjectClass         string  // Classification result
// ObjectConfidence    float32 // Classification confidence
// ClassificationModel string  // Model version used

// ComputeSpeedPercentiles computes speed percentiles from a track's speed history.
// Uses floor-based indexing for percentiles. For small arrays (n<3), all percentiles
// may return similar values. For production use with precise percentile requirements,
// consider using linear interpolation between neighboring values.
func ComputeSpeedPercentiles(speedHistory []float32) (p50, p85, p95 float32) {
	if len(speedHistory) == 0 {
		return 0, 0, 0
	}

	speeds := make([]float32, len(speedHistory))
	copy(speeds, speedHistory)
	sort.Slice(speeds, func(i, j int) bool { return speeds[i] < speeds[j] })

	n := len(speeds)
	p50 = speeds[n/2]

	p85Idx := int(math.Floor(float64(n) * 0.85))
	if p85Idx >= n {
		p85Idx = n - 1
	}
	p85 = speeds[p85Idx]

	p95Idx := int(math.Floor(float64(n) * 0.95))
	if p95Idx >= n {
		p95Idx = n - 1
	}
	p95 = speeds[p95Idx]

	return
}
