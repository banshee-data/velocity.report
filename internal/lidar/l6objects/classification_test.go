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
		AvgSpeedMps:          0.5, // Low speed
		PeakSpeedMps:         0.8,
		speedHistory:         []float32{0.3, 0.5, 0.4, 0.6, 0.5, 0.4, 0.5, 0.6, 0.5, 0.4},
	}

	result := classifier.Classify(track)

	if result.Class != ClassBird {
		t.Errorf("Expected bird classification, got %s", result.Class)
	}
	if result.Confidence < 0.5 {
		t.Errorf("Expected confidence >= 0.5, got %.2f", result.Confidence)
	}
	if result.Model != "rule-based-v1.0" {
		t.Errorf("Expected model version 'rule-based-v1.0', got %s", result.Model)
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
		AvgSpeedMps:          10.0, // ~36 km/h
		PeakSpeedMps:         15.0,
		speedHistory:         make([]float32, 20),
	}

	// Fill speed history
	for i := range track.speedHistory {
		track.speedHistory[i] = float32(8 + i%5)
	}

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
		AvgSpeedMps:          1.5, // Walking speed ~5.4 km/h
		PeakSpeedMps:         2.5,
		speedHistory:         make([]float32, 15),
	}

	// Fill speed history
	for i := range track.speedHistory {
		track.speedHistory[i] = float32(1.2 + float32(i%5)*0.1)
	}

	result := classifier.Classify(track)

	if result.Class != ClassPedestrian {
		t.Errorf("Expected pedestrian classification, got %s", result.Class)
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
		AvgSpeedMps:          4.0, // Too fast for pedestrian, too slow for car
		PeakSpeedMps:         5.0,
		speedHistory:         []float32{3.5, 4.0, 4.2, 3.8, 4.0, 4.5, 4.0, 3.8, 4.2, 4.0},
	}

	result := classifier.Classify(track)

	if result.Class != ClassOther {
		t.Errorf("Expected other classification, got %s", result.Class)
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
		AvgSpeedMps:          1.5,
		speedHistory:         []float32{1.5, 1.6},
	}

	result := classifier.Classify(track)

	if result.Class != ClassOther {
		t.Errorf("Expected other classification for insufficient observations, got %s", result.Class)
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
		AvgSpeedMps:          12.0,
		PeakSpeedMps:         18.0,
		speedHistory:         make([]float32, 20),
	}

	for i := range track.speedHistory {
		track.speedHistory[i] = float32(10 + i%5)
	}

	classifier.ClassifyAndUpdate(track)

	if track.ObjectClass != string(ClassCar) {
		t.Errorf("Expected ObjectClass to be 'car', got '%s'", track.ObjectClass)
	}
	if track.ObjectConfidence < 0.5 {
		t.Errorf("Expected ObjectConfidence >= 0.5, got %.2f", track.ObjectConfidence)
	}
	if track.ClassificationModel != "rule-based-v1.0" {
		t.Errorf("Expected ClassificationModel 'rule-based-v1.0', got '%s'", track.ClassificationModel)
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
