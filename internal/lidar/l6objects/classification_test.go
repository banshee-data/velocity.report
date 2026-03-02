package l6objects

import (
	"testing"
)

func TestTrackClassifier_Classify_Bird(t *testing.T) {
	classifier := NewTrackClassifier()

	// Create a bird-like track
	track := &TrackedObject{
		TrackID:              "test-bird",
		ObservationCount:     10,
		BoundingBoxHeightAvg: 0.3, // Small height
		BoundingBoxLengthAvg: 0.4,
		BoundingBoxWidthAvg:  0.3,
		MedianSpeedMps:       0.5, // Low speed
		PeakSpeedMps:         0.8,
	}
	track.SetSpeedHistory([]float32{0.3, 0.5, 0.4, 0.6, 0.5, 0.4, 0.5, 0.6, 0.5, 0.4})

	result := classifier.Classify(track)

	if result.Class != ClassBird {
		t.Errorf("Expected bird classification, got %s", result.Class)
	}
	if result.Confidence < 0.5 {
		t.Errorf("Expected confidence >= 0.5, got %.2f", result.Confidence)
	}
	if result.Model != "rule-based-v1.2" {
		t.Errorf("Expected model version 'rule-based-v1.2', got %s", result.Model)
	}
}

func TestTrackClassifier_Classify_Vehicle(t *testing.T) {
	classifier := NewTrackClassifier()

	// Create a vehicle-like track
	track := &TrackedObject{
		TrackID:              "test-car",
		ObservationCount:     20,
		BoundingBoxHeightAvg: 1.5,  // Typical car height
		BoundingBoxLengthAvg: 4.5,  // Typical car length
		BoundingBoxWidthAvg:  2.0,  // Typical car width
		MedianSpeedMps:       10.0, // ~36 km/h
		PeakSpeedMps:         15.0,
	}

	// Fill speed history
	speeds := make([]float32, 20)
	for i := range speeds {
		speeds[i] = float32(8 + i%5)
	}
	track.SetSpeedHistory(speeds)

	result := classifier.Classify(track)

	if result.Class != ClassCar {
		t.Errorf("Expected car classification, got %s", result.Class)
	}
	if result.Confidence < 0.7 {
		t.Errorf("Expected confidence >= 0.7, got %.2f", result.Confidence)
	}
}

func TestTrackClassifier_Classify_Pedestrian(t *testing.T) {
	classifier := NewTrackClassifier()

	// Create a pedestrian-like track
	track := &TrackedObject{
		TrackID:              "test-pedestrian",
		ObservationCount:     15,
		BoundingBoxHeightAvg: 1.7, // Typical human height
		BoundingBoxLengthAvg: 0.5, // Small footprint
		BoundingBoxWidthAvg:  0.5,
		MedianSpeedMps:       1.5, // Walking speed ~5.4 km/h
		PeakSpeedMps:         2.5,
	}

	// Fill speed history
	speeds := make([]float32, 15)
	for i := range speeds {
		speeds[i] = float32(1.2 + float32(i%5)*0.1)
	}
	track.SetSpeedHistory(speeds)

	result := classifier.Classify(track)

	if result.Class != ClassPedestrian {
		t.Errorf("Expected pedestrian classification, got %s", result.Class)
	}
	if result.Confidence < 0.6 {
		t.Errorf("Expected confidence >= 0.6, got %.2f", result.Confidence)
	}
}

func TestTrackClassifier_Classify_Bus(t *testing.T) {
	classifier := NewTrackClassifier()

	// Create a bus-like track: very long, wide, and fast.
	track := &TrackedObject{
		TrackID:              "test-bus",
		ObservationCount:     25,
		BoundingBoxHeightAvg: 3.2,  // Tall
		BoundingBoxLengthAvg: 10.0, // Very long (bus)
		BoundingBoxWidthAvg:  2.5,  // Wide
		MedianSpeedMps:       8.0,  // ~29 km/h
		PeakSpeedMps:         12.0,
	}

	speeds := make([]float32, 25)
	for i := range speeds {
		speeds[i] = float32(6 + i%5)
	}
	track.SetSpeedHistory(speeds)

	result := classifier.Classify(track)

	if result.Class != ClassBus {
		t.Errorf("Expected bus classification, got %s", result.Class)
	}
	if result.Confidence < 0.7 {
		t.Errorf("Expected confidence >= 0.7, got %.2f", result.Confidence)
	}
}

func TestTrackClassifier_Classify_Cyclist(t *testing.T) {
	classifier := NewTrackClassifier()

	// Create a cyclist-like track: narrow, moderate speed, human height.
	// Length must be <1.5 m to avoid matching motorcyclist rule.
	track := &TrackedObject{
		TrackID:              "test-cyclist",
		ObservationCount:     15,
		BoundingBoxHeightAvg: 1.5, // Seated cyclist
		BoundingBoxLengthAvg: 1.4, // Short bike (below motorcyclist 1.5 m threshold)
		BoundingBoxWidthAvg:  0.6, // Narrow
		MedianSpeedMps:       5.0, // ~18 km/h
		PeakSpeedMps:         7.0,
	}

	speeds := make([]float32, 15)
	for i := range speeds {
		speeds[i] = float32(4 + float32(i%4)*0.5)
	}
	track.SetSpeedHistory(speeds)

	result := classifier.Classify(track)

	if result.Class != ClassCyclist {
		t.Errorf("Expected cyclist classification, got %s", result.Class)
	}
	if result.Confidence < 0.6 {
		t.Errorf("Expected confidence >= 0.6, got %.2f", result.Confidence)
	}
}

func TestTrackClassifier_Classify_Truck(t *testing.T) {
	classifier := NewTrackClassifier()

	// Create a truck-like track: longer and taller than a car.
	track := &TrackedObject{
		TrackID:              "test-truck",
		ObservationCount:     20,
		BoundingBoxHeightAvg: 2.5, // Taller than a car
		BoundingBoxLengthAvg: 6.5, // Longer than a car (>5.5 m)
		BoundingBoxWidthAvg:  2.3, // Wider than a car (>2.0 m)
		MedianSpeedMps:       9.0, // ~32 km/h
		PeakSpeedMps:         14.0,
	}

	speeds := make([]float32, 20)
	for i := range speeds {
		speeds[i] = float32(7 + i%5)
	}
	track.SetSpeedHistory(speeds)

	result := classifier.Classify(track)

	if result.Class != ClassTruck {
		t.Errorf("Expected truck classification, got %s", result.Class)
	}
	if result.Confidence < 0.6 {
		t.Errorf("Expected confidence >= 0.6, got %.2f", result.Confidence)
	}
}

func TestTrackClassifier_Classify_Motorcyclist(t *testing.T) {
	classifier := NewTrackClassifier()

	// Create a motorcyclist-like track: narrow, fast, longer than a bicycle.
	track := &TrackedObject{
		TrackID:              "test-motorcyclist",
		ObservationCount:     20,
		BoundingBoxHeightAvg: 1.5,  // Rider height
		BoundingBoxLengthAvg: 2.2,  // Motorcycle length (>1.5 m)
		BoundingBoxWidthAvg:  0.8,  // Narrow (<1.2 m)
		MedianSpeedMps:       12.0, // ~43 km/h (faster than cyclist)
		PeakSpeedMps:         18.0,
	}

	speeds := make([]float32, 20)
	for i := range speeds {
		speeds[i] = float32(10 + float32(i%5)*1.0)
	}
	track.SetSpeedHistory(speeds)

	result := classifier.Classify(track)

	if result.Class != ClassMotorcyclist {
		t.Errorf("Expected motorcyclist classification, got %s", result.Class)
	}
	if result.Confidence < 0.6 {
		t.Errorf("Expected confidence >= 0.6, got %.2f", result.Confidence)
	}
}

func TestTrackClassifier_Classify_Other(t *testing.T) {
	classifier := NewTrackClassifier()

	// Create an ambiguous track
	track := &TrackedObject{
		TrackID:              "test-other",
		ObservationCount:     10,
		BoundingBoxHeightAvg: 0.8, // Between bird and pedestrian
		BoundingBoxLengthAvg: 1.5,
		BoundingBoxWidthAvg:  1.0,
		MedianSpeedMps:       4.0, // Too fast for pedestrian, too slow for car
		PeakSpeedMps:         5.0,
	}
	track.SetSpeedHistory([]float32{3.5, 4.0, 4.2, 3.8, 4.0, 4.5, 4.0, 3.8, 4.2, 4.0})

	result := classifier.Classify(track)

	if result.Class != ClassDynamic {
		t.Errorf("Expected dynamic classification, got %s", result.Class)
	}
}

func TestTrackClassifier_Classify_InsufficientObservations(t *testing.T) {
	classifier := NewTrackClassifier()

	// Create a track with few observations
	track := &TrackedObject{
		TrackID:              "test-insufficient",
		ObservationCount:     2, // Less than minimum
		BoundingBoxHeightAvg: 1.7,
		BoundingBoxLengthAvg: 0.5,
		BoundingBoxWidthAvg:  0.5,
		MedianSpeedMps:       1.5,
	}
	track.SetSpeedHistory([]float32{1.5, 1.6})

	result := classifier.Classify(track)

	if result.Class != ClassDynamic {
		t.Errorf("Expected dynamic classification for insufficient observations, got %s", result.Class)
	}
	if result.Confidence >= 0.5 {
		t.Errorf("Expected low confidence for insufficient observations, got %.2f", result.Confidence)
	}
}

func TestTrackClassifier_ClassifyAndUpdate(t *testing.T) {
	classifier := NewTrackClassifier()

	track := &TrackedObject{
		TrackID:              "test-update",
		ObservationCount:     20,
		BoundingBoxHeightAvg: 1.5,
		BoundingBoxLengthAvg: 4.5,
		BoundingBoxWidthAvg:  2.0,
		MedianSpeedMps:       12.0,
		PeakSpeedMps:         18.0,
	}

	speeds := make([]float32, 20)
	for i := range speeds {
		speeds[i] = float32(10 + i%5)
	}
	track.SetSpeedHistory(speeds)

	classifier.ClassifyAndUpdate(track)

	if track.ObjectClass != string(ClassCar) {
		t.Errorf("Expected ObjectClass to be 'car', got '%s'", track.ObjectClass)
	}
	if track.ObjectConfidence < 0.5 {
		t.Errorf("Expected ObjectConfidence >= 0.5, got %.2f", track.ObjectConfidence)
	}
	if track.ClassificationModel != "rule-based-v1.2" {
		t.Errorf("Expected ClassificationModel 'rule-based-v1.2', got '%s'", track.ClassificationModel)
	}
}

func TestComputeSpeedPercentiles(t *testing.T) {
	// Test with 20 values
	speeds := []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

	p50, p85, p95 := ComputeSpeedPercentiles(speeds)

	// P50 (median) should be around 10-11
	if p50 < 10 || p50 > 11 {
		t.Errorf("Expected P50 around 10-11, got %.2f", p50)
	}

	// P85 should be around 17
	if p85 < 16 || p85 > 18 {
		t.Errorf("Expected P85 around 17, got %.2f", p85)
	}

	// P95 should be around 19
	if p95 < 18 || p95 > 20 {
		t.Errorf("Expected P95 around 19, got %.2f", p95)
	}
}

func TestComputeSpeedPercentiles_Empty(t *testing.T) {
	p50, p85, p95 := ComputeSpeedPercentiles([]float32{})

	if p50 != 0 || p85 != 0 || p95 != 0 {
		t.Errorf("Expected all zeros for empty input, got p50=%.2f, p85=%.2f, p95=%.2f", p50, p85, p95)
	}
}

// TestClassifyFeatures_MatchesClassify verifies that ClassifyFeatures produces
// the same result as Classify for the same input features.
func TestClassifyFeatures_MatchesClassify(t *testing.T) {
	classifier := NewTrackClassifierWithMinObservations(3)

	track := &TrackedObject{
		TrackID:              "roundtrip-car",
		ObservationCount:     20,
		BoundingBoxHeightAvg: 1.5,
		BoundingBoxLengthAvg: 4.5,
		BoundingBoxWidthAvg:  2.0,
		MedianSpeedMps:       12.0,
		PeakSpeedMps:         15.0,
	}

	fromTrack := classifier.Classify(track)
	fromFeatures := classifier.ClassifyFeatures(fromTrack.Features)

	if fromTrack.Class != fromFeatures.Class {
		t.Errorf("ClassifyFeatures class=%s, Classify class=%s; should match", fromFeatures.Class, fromTrack.Class)
	}
	if fromTrack.Confidence != fromFeatures.Confidence {
		t.Errorf("ClassifyFeatures confidence=%.2f, Classify confidence=%.2f; should match", fromFeatures.Confidence, fromTrack.Confidence)
	}
}

// TestClassifyFeatures_AllClasses tests ClassifyFeatures for representative
// feature sets of each class.
func TestClassifyFeatures_AllClasses(t *testing.T) {
	classifier := NewTrackClassifierWithMinObservations(3)

	tests := []struct {
		desc     string
		features ClassificationFeatures
		expected ObjectClass
	}{
		{
			desc: "bird",
			features: ClassificationFeatures{
				AvgHeight: 0.2, AvgLength: 0.3, AvgWidth: 0.3,
				AvgSpeed: 0.5, PeakSpeed: 0.8, ObservationCount: 10,
			},
			expected: ClassBird,
		},
		{
			desc: "car",
			features: ClassificationFeatures{
				AvgHeight: 1.5, AvgLength: 4.5, AvgWidth: 2.0,
				AvgSpeed: 12.0, PeakSpeed: 15.0, ObservationCount: 20,
			},
			expected: ClassCar,
		},
		{
			desc: "pedestrian",
			features: ClassificationFeatures{
				AvgHeight: 1.7, AvgLength: 0.5, AvgWidth: 0.5,
				AvgSpeed: 1.2, PeakSpeed: 2.0, ObservationCount: 15,
			},
			expected: ClassPedestrian,
		},
		{
			desc: "bus",
			features: ClassificationFeatures{
				AvgHeight: 3.0, AvgLength: 10.0, AvgWidth: 2.5,
				AvgSpeed: 8.0, PeakSpeed: 12.0, ObservationCount: 30,
			},
			expected: ClassBus,
		},
		{
			desc: "too few observations → dynamic",
			features: ClassificationFeatures{
				AvgHeight: 1.5, AvgLength: 4.5, AvgWidth: 2.0,
				AvgSpeed: 12.0, PeakSpeed: 15.0, ObservationCount: 1,
			},
			expected: ClassDynamic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := classifier.ClassifyFeatures(tt.features)
			if result.Class != tt.expected {
				t.Errorf("ClassifyFeatures() class=%s, want %s", result.Class, tt.expected)
			}
		})
	}
}
