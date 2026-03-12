package l6objects

import (
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/config"
)

// ObjectClass represents the classification of a tracked object.
type ObjectClass string

const (
	// ClassCar indicates a car or small/medium vehicle
	ClassCar ObjectClass = "car"
	// ClassTruck indicates a truck or medium-large vehicle
	ClassTruck ObjectClass = "truck"
	// ClassBus indicates a bus or large vehicle (length > 7 m)
	ClassBus ObjectClass = "bus"
	// ClassPedestrian indicates a pedestrian or person
	ClassPedestrian ObjectClass = "pedestrian"
	// ClassCyclist indicates a cyclist
	ClassCyclist ObjectClass = "cyclist"
	// ClassMotorcyclist indicates a person on a motorcycle
	ClassMotorcyclist ObjectClass = "motorcyclist"
	// ClassBird indicates a bird or small flying object
	ClassBird ObjectClass = "bird"
	// ClassDynamic indicates an unclassified dynamic object
	ClassDynamic ObjectClass = "dynamic"

	// Aliases
	ClassPed = ClassPedestrian // Short-form alias
)

// Classification thresholds (configurable for tuning)
const (
	// Height thresholds (metres)
	BirdHeightMax       = 0.5 // Birds are typically small
	PedestrianHeightMin = 1.0 // Pedestrians are at least 1 m tall
	PedestrianHeightMax = 2.2 // Pedestrians are typically under 2.2 m
	VehicleHeightMin    = 1.2 // Vehicles are at least 1.2 m
	VehicleLengthMin    = 3.0 // Vehicles are at least 3 m long
	VehicleWidthMin     = 1.5 // Vehicles are at least 1.5 m wide

	// Bus thresholds (very large vehicles)
	BusLengthMin = 7.0 // Buses/coaches are typically ≥ 7 m
	BusWidthMin  = 2.3 // Buses are typically ≥ 2.3 m wide

	// Truck thresholds (medium-large vehicles, smaller than buses)
	TruckLengthMin = 5.5 // Trucks are typically ≥ 5.5 m
	TruckWidthMin  = 2.0 // Trucks are typically ≥ 2.0 m wide
	TruckHeightMin = 2.0 // Trucks tend to be taller than cars

	// Cyclist thresholds
	CyclistHeightMin = 1.0  // Seated cyclist ≥ 1 m
	CyclistHeightMax = 2.0  // Cyclist ≤ 2 m
	CyclistSpeedMin  = 2.0  // Faster than walking (≥ 2 m/s ≈ 7.2 km/h)
	CyclistSpeedMax  = 10.0 // Slower than most motor vehicles (≤ 10 m/s ≈ 36 km/h)
	CyclistWidthMax  = 1.2  // Narrow profile
	CyclistLengthMax = 2.5  // Bike ≤ 2.5 m

	// Motorcyclist thresholds (similar to cyclist but faster)
	MotorcyclistSpeedMin  = 5.0  // Faster than cyclists (≥ 5 m/s ≈ 18 km/h)
	MotorcyclistSpeedMax  = 30.0 // Up to highway speed (≤ 30 m/s ≈ 108 km/h)
	MotorcyclistWidthMax  = 1.2  // Narrow profile
	MotorcyclistLengthMin = 1.5  // Motorcycle ≥ 1.5 m
	MotorcyclistLengthMax = 3.0  // Motorcycle ≤ 3.0 m

	// Speed thresholds (m/s)
	BirdSpeedMax       = 1.0 // Birds detected at low speeds
	PedestrianSpeedMax = 3.0 // Pedestrians walk up to ~3 m/s (10.8 km/h)
	VehicleSpeedMin    = 5.0 // Vehicles typically move faster than 5 m/s
	StationarySpeedMax = 0.5 // Stationary threshold

	// Confidence levels
	HighConfidence   = 0.85
	MediumConfidence = 0.70
	LowConfidence    = 0.50
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
	AvgSpeed float32 // Average speed
	MaxSpeed float32 // Max speed
	P50Speed float32 // Median speed
	P85Speed float32 // 85th percentile speed
	P95Speed float32 // 95th percentile speed

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
	ModelVersion    string
	MinObservations int // Minimum observations before classification
}

// NewTrackClassifier creates a new track classifier.
func NewTrackClassifier() *TrackClassifier {
	cfg := config.MustLoadDefaultConfig()
	return NewTrackClassifierWithMinObservations(cfg.GetMinObservationsForClassification())
}

// NewTrackClassifierWithMinObservations creates a new classifier with an
// explicit minimum-observation threshold.
func NewTrackClassifierWithMinObservations(minObservations int) *TrackClassifier {
	if minObservations <= 0 {
		minObservations = 1
	}
	classifier := &TrackClassifier{
		ModelVersion:    "rule-based-v1.2",
		MinObservations: minObservations,
	}
	diagf("Track classifier created: model=%s min_observations=%d",
		classifier.ModelVersion, classifier.MinObservations)
	return classifier
}

// Classify determines the object class for a tracked object.
// Returns the classification result with class, confidence, and features used.
func (tc *TrackClassifier) Classify(track *TrackedObject) ClassificationResult {
	features := tc.extractFeatures(track)
	return tc.ClassifyFeatures(features)
}

// ClassifyFeatures classifies an object from pre-built feature values.
// Use this when the full TrackedObject is unavailable — e.g. during VRLOG
// replay where only aggregate metrics (bbox, speed, observation count) are
// stored in the recorded FrameBundle tracks.
func (tc *TrackClassifier) ClassifyFeatures(features ClassificationFeatures) ClassificationResult {
	result := ClassificationResult{
		Model:    tc.ModelVersion,
		Features: features,
	}
	finish := func(class ObjectClass, confidence float32) ClassificationResult {
		result.Class = class
		result.Confidence = confidence
		tracef("Classification result: class=%s confidence=%.2f observations=%d avg_speed=%.2f length=%.2f width=%.2f height=%.2f",
			result.Class, result.Confidence, features.ObservationCount, features.AvgSpeed,
			features.AvgLength, features.AvgWidth, features.AvgHeight)
		return result
	}

	// Not enough observations for reliable classification
	if features.ObservationCount < tc.MinObservations {
		return finish(ClassDynamic, LowConfidence*0.5) // Very low confidence
	}

	// Classification rules (priority order)
	// 1. Check for bird (small, low-speed)
	if tc.isBird(features) {
		return finish(ClassBird, tc.birdConfidence(features))
	}

	// 2. Check for bus (very large vehicle)
	if tc.isBus(features) {
		return finish(ClassBus, tc.busConfidence(features))
	}

	// 3. Check for truck (medium-large vehicle, between car and bus)
	if tc.isTruck(features) {
		return finish(ClassTruck, tc.truckConfidence(features))
	}

	// 4. Check for car (medium vehicle — not bus/truck-sized)
	if tc.isVehicle(features) {
		return finish(ClassCar, tc.vehicleConfidence(features))
	}

	// 5. Check for motorcyclist (fast, narrow, elongated)
	if tc.isMotorcyclist(features) {
		return finish(ClassMotorcyclist, tc.motorcyclistConfidence(features))
	}

	// 6. Check for cyclist (moderate speed, narrow profile)
	if tc.isCyclist(features) {
		return finish(ClassCyclist, tc.cyclistConfidence(features))
	}

	// 7. Check for pedestrian (human-sized, slow)
	if tc.isPedestrian(features) {
		return finish(ClassPedestrian, tc.pedestrianConfidence(features))
	}

	// 8. Default to dynamic
	return finish(ClassDynamic, LowConfidence)
}

// extractFeatures extracts classification features from a track.
func (tc *TrackClassifier) extractFeatures(track *TrackedObject) ClassificationFeatures {
	features := ClassificationFeatures{
		AvgHeight:        track.BoundingBoxHeightAvg,
		AvgLength:        track.BoundingBoxLengthAvg,
		AvgWidth:         track.BoundingBoxWidthAvg,
		HeightP95:        track.HeightP95Max,
		AvgSpeed:         track.AvgSpeedMps,
		MaxSpeed:         track.MaxSpeedMps,
		ObservationCount: track.ObservationCount,
	}

	// Compute speed percentiles from history using shared function
	speedHistory := track.SpeedHistory()
	if len(speedHistory) > 0 {
		features.P50Speed, features.P85Speed, features.P95Speed = ComputeSpeedPercentiles(speedHistory)
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

// isBus checks if features match bus/large-vehicle classification.
// A bus is distinguished from a car by its significantly larger footprint.
func (tc *TrackClassifier) isBus(f ClassificationFeatures) bool {
	isVeryLong := f.AvgLength > BusLengthMin
	isWide := f.AvgWidth > BusWidthMin
	isFast := f.AvgSpeed > VehicleSpeedMin || f.MaxSpeed > VehicleSpeedMin*1.5
	isTall := f.AvgHeight > VehicleHeightMin

	return isVeryLong && isWide && (isFast || isTall)
}

// busConfidence computes confidence for bus classification.
func (tc *TrackClassifier) busConfidence(f ClassificationFeatures) float32 {
	confidence := float32(MediumConfidence)

	// Very long objects are more likely buses
	if f.AvgLength > 10.0 {
		confidence += 0.10
	}
	if f.AvgWidth > 2.5 {
		confidence += 0.05
	}
	if f.AvgHeight > 2.5 {
		confidence += 0.05
	}

	// Speed factors
	if f.AvgSpeed > 8.0 {
		confidence += 0.05
	}

	if f.ObservationCount > 20 {
		confidence += 0.05
	}

	return clampConfidence(confidence, LowConfidence, HighConfidence)
}

// isTruck checks if features match truck classification.
// Trucks are larger than cars but smaller than buses.
func (tc *TrackClassifier) isTruck(f ClassificationFeatures) bool {
	isLong := f.AvgLength > TruckLengthMin
	isWide := f.AvgWidth > TruckWidthMin
	isTall := f.AvgHeight > TruckHeightMin
	isFast := f.AvgSpeed > VehicleSpeedMin || f.MaxSpeed > VehicleSpeedMin*1.5

	return isLong && isWide && isTall && isFast
}

// truckConfidence computes confidence for truck classification.
func (tc *TrackClassifier) truckConfidence(f ClassificationFeatures) float32 {
	confidence := float32(MediumConfidence)

	if f.AvgLength > 6.0 {
		confidence += 0.05
	}
	if f.AvgHeight > 2.5 {
		confidence += 0.05
	}
	if f.AvgWidth > 2.2 {
		confidence += 0.05
	}
	if f.AvgSpeed > 8.0 {
		confidence += 0.05
	}
	if f.ObservationCount > 20 {
		confidence += 0.05
	}

	return clampConfidence(confidence, LowConfidence, HighConfidence)
}

// isVehicle checks if features match car/vehicle classification.
// Bus- and truck-sized objects are excluded (checked earlier in the cascade).
func (tc *TrackClassifier) isVehicle(f ClassificationFeatures) bool {
	// Large size AND high speed
	isLarge := f.AvgLength > VehicleLengthMin || f.AvgWidth > VehicleWidthMin
	isFast := f.AvgSpeed > VehicleSpeedMin || f.MaxSpeed > VehicleSpeedMin*1.5
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
	if f.MaxSpeed > 15.0 {
		confidence += 0.05
	}

	// More observations = more confidence
	if f.ObservationCount > 20 {
		confidence += 0.05
	}

	return clampConfidence(confidence, LowConfidence, HighConfidence)
}

// isCyclist checks if features match cyclist classification.
// Cyclists are faster than pedestrians but narrower than vehicles.
func (tc *TrackClassifier) isCyclist(f ClassificationFeatures) bool {
	heightOK := f.AvgHeight >= CyclistHeightMin && f.AvgHeight <= CyclistHeightMax
	speedOK := f.AvgSpeed >= CyclistSpeedMin && f.AvgSpeed <= CyclistSpeedMax
	narrowOK := f.AvgWidth < CyclistWidthMax && f.AvgLength < CyclistLengthMax

	return heightOK && speedOK && narrowOK
}

// cyclistConfidence computes confidence for cyclist classification.
func (tc *TrackClassifier) cyclistConfidence(f ClassificationFeatures) float32 {
	confidence := float32(MediumConfidence)

	// Speed in typical cycling range (3-8 m/s ≈ 11-29 km/h)
	if f.AvgSpeed >= 3.0 && f.AvgSpeed <= 8.0 {
		confidence += 0.10
	}

	// Narrow profile is very characteristic
	if f.AvgWidth < 0.8 {
		confidence += 0.05
	}

	// Height in seated-cyclist range
	if f.AvgHeight >= 1.2 && f.AvgHeight <= 1.8 {
		confidence += 0.05
	}

	if f.ObservationCount > 15 {
		confidence += 0.05
	}

	return clampConfidence(confidence, LowConfidence, HighConfidence)
}

// isMotorcyclist checks if features match motorcyclist classification.
// Motorcyclists are faster than cyclists with a narrow, elongated profile.
func (tc *TrackClassifier) isMotorcyclist(f ClassificationFeatures) bool {
	speedOK := f.AvgSpeed >= MotorcyclistSpeedMin && f.AvgSpeed <= MotorcyclistSpeedMax
	narrowOK := f.AvgWidth <= MotorcyclistWidthMax
	lengthOK := f.AvgLength >= MotorcyclistLengthMin && f.AvgLength <= MotorcyclistLengthMax

	return speedOK && narrowOK && lengthOK
}

// motorcyclistConfidence computes confidence for motorcyclist classification.
func (tc *TrackClassifier) motorcyclistConfidence(f ClassificationFeatures) float32 {
	confidence := float32(MediumConfidence)

	// Speed in typical motorcycling range (5-30 m/s ≈ 18-108 km/h)
	if f.AvgSpeed >= 8.0 && f.AvgSpeed <= 25.0 {
		confidence += 0.10
	}

	// Narrow profile is very characteristic of two-wheelers
	if f.AvgWidth <= 0.9 {
		confidence += 0.05
	}

	// Longer than a bicycle
	if f.AvgLength >= 2.0 {
		confidence += 0.05
	}

	// Higher max speed distinguishes from cyclist
	if f.MaxSpeed > 12.0 {
		confidence += 0.05
	}

	if f.ObservationCount > 15 {
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
	prevClass := track.ObjectClass
	result := tc.Classify(track)
	track.ObjectClass = string(result.Class)
	track.ObjectConfidence = result.Confidence
	track.ClassificationModel = result.Model
	if prevClass != track.ObjectClass {
		diagf("Track classification updated: track_id=%s class=%s->%s confidence=%.2f observations=%d",
			track.TrackID, prevClass, track.ObjectClass, result.Confidence, track.ObservationCount)
	}
}

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
