package lidar

import (
	"math"
	"testing"
)

// TestDefaultGroundTruthWeights verifies the default weights match the design doc.
func TestDefaultGroundTruthWeights(t *testing.T) {
	weights := DefaultGroundTruthWeights()

	expected := map[string]float64{
		"DetectionRate":     1.0,
		"Fragmentation":     5.0,
		"FalsePositives":    2.0,
		"VelocityCoverage":  0.5,
		"QualityPremium":    0.3,
		"TruncationRate":    0.4,
		"VelocityNoiseRate": 0.4,
		"StoppedRecovery":   0.2,
	}

	if weights.DetectionRate != expected["DetectionRate"] {
		t.Errorf("DetectionRate: got %f, want %f", weights.DetectionRate, expected["DetectionRate"])
	}
	if weights.Fragmentation != expected["Fragmentation"] {
		t.Errorf("Fragmentation: got %f, want %f", weights.Fragmentation, expected["Fragmentation"])
	}
	if weights.FalsePositives != expected["FalsePositives"] {
		t.Errorf("FalsePositives: got %f, want %f", weights.FalsePositives, expected["FalsePositives"])
	}
	if weights.VelocityCoverage != expected["VelocityCoverage"] {
		t.Errorf("VelocityCoverage: got %f, want %f", weights.VelocityCoverage, expected["VelocityCoverage"])
	}
	if weights.QualityPremium != expected["QualityPremium"] {
		t.Errorf("QualityPremium: got %f, want %f", weights.QualityPremium, expected["QualityPremium"])
	}
	if weights.TruncationRate != expected["TruncationRate"] {
		t.Errorf("TruncationRate: got %f, want %f", weights.TruncationRate, expected["TruncationRate"])
	}
	if weights.VelocityNoiseRate != expected["VelocityNoiseRate"] {
		t.Errorf("VelocityNoiseRate: got %f, want %f", weights.VelocityNoiseRate, expected["VelocityNoiseRate"])
	}
	if weights.StoppedRecovery != expected["StoppedRecovery"] {
		t.Errorf("StoppedRecovery: got %f, want %f", weights.StoppedRecovery, expected["StoppedRecovery"])
	}
}

// TestComputeTemporalIoU tests the temporal IoU calculation with various overlap scenarios.
func TestComputeTemporalIoU(t *testing.T) {
	tests := []struct {
		name     string
		ref      *RunTrack
		cand     *RunTrack
		expected float64
	}{
		{
			name: "identical ranges",
			ref: &RunTrack{
				StartUnixNanos: 1000,
				EndUnixNanos:   2000,
			},
			cand: &RunTrack{
				StartUnixNanos: 1000,
				EndUnixNanos:   2000,
			},
			expected: 1.0,
		},
		{
			name: "50% overlap",
			ref: &RunTrack{
				StartUnixNanos: 1000,
				EndUnixNanos:   2000,
			},
			cand: &RunTrack{
				StartUnixNanos: 1500,
				EndUnixNanos:   2500,
			},
			expected: 0.333333, // intersection=500, union=1500
		},
		{
			name: "no overlap - separate",
			ref: &RunTrack{
				StartUnixNanos: 1000,
				EndUnixNanos:   2000,
			},
			cand: &RunTrack{
				StartUnixNanos: 3000,
				EndUnixNanos:   4000,
			},
			expected: 0.0,
		},
		{
			name: "no overlap - adjacent",
			ref: &RunTrack{
				StartUnixNanos: 1000,
				EndUnixNanos:   2000,
			},
			cand: &RunTrack{
				StartUnixNanos: 2000,
				EndUnixNanos:   3000,
			},
			expected: 0.0,
		},
		{
			name: "candidate contained in reference",
			ref: &RunTrack{
				StartUnixNanos: 1000,
				EndUnixNanos:   3000,
			},
			cand: &RunTrack{
				StartUnixNanos: 1500,
				EndUnixNanos:   2500,
			},
			expected: 0.5, // intersection=1000, union=2000
		},
		{
			name: "reference contained in candidate",
			ref: &RunTrack{
				StartUnixNanos: 1500,
				EndUnixNanos:   2500,
			},
			cand: &RunTrack{
				StartUnixNanos: 1000,
				EndUnixNanos:   3000,
			},
			expected: 0.5, // intersection=1000, union=2000
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iou := computeTemporalIoU(tt.ref, tt.cand)
			if math.Abs(iou-tt.expected) > 0.001 {
				t.Errorf("computeTemporalIoU() = %f, want %f", iou, tt.expected)
			}
		})
	}
}

// TestMatchTracks tests the optimal bipartite matching algorithm.
func TestMatchTracks(t *testing.T) {
	tests := []struct {
		name          string
		reference     []*RunTrack
		candidate     []*RunTrack
		expectedCount int
		expectedPairs map[string]string // ref ID -> cand ID
		minIoU        float64
	}{
		{
			name: "single perfect match",
			reference: []*RunTrack{
				{TrackID: "ref-1", StartUnixNanos: 1000, EndUnixNanos: 2000},
			},
			candidate: []*RunTrack{
				{TrackID: "cand-1", StartUnixNanos: 1000, EndUnixNanos: 2000},
			},
			expectedCount: 1,
			expectedPairs: map[string]string{"ref-1": "cand-1"},
			minIoU:        1.0,
		},
		{
			name: "multiple matches",
			reference: []*RunTrack{
				{TrackID: "ref-1", StartUnixNanos: 1000, EndUnixNanos: 2000},
				{TrackID: "ref-2", StartUnixNanos: 3000, EndUnixNanos: 4000},
			},
			candidate: []*RunTrack{
				{TrackID: "cand-1", StartUnixNanos: 1000, EndUnixNanos: 2000},
				{TrackID: "cand-2", StartUnixNanos: 3000, EndUnixNanos: 4000},
			},
			expectedCount: 2,
			expectedPairs: map[string]string{
				"ref-1": "cand-1",
				"ref-2": "cand-2",
			},
			minIoU: 1.0,
		},
		{
			name: "no matches - below threshold",
			reference: []*RunTrack{
				{TrackID: "ref-1", StartUnixNanos: 1000, EndUnixNanos: 2000},
			},
			candidate: []*RunTrack{
				{TrackID: "cand-1", StartUnixNanos: 5000, EndUnixNanos: 6000},
			},
			expectedCount: 0,
			expectedPairs: map[string]string{},
			minIoU:        0.0,
		},
		{
			name: "partial overlap - above threshold",
			reference: []*RunTrack{
				{TrackID: "ref-1", StartUnixNanos: 1000, EndUnixNanos: 2000},
			},
			candidate: []*RunTrack{
				{TrackID: "cand-1", StartUnixNanos: 1500, EndUnixNanos: 2500},
			},
			expectedCount: 1,
			expectedPairs: map[string]string{"ref-1": "cand-1"},
			minIoU:        0.3, // Just above threshold
		},
		{
			name:          "empty reference",
			reference:     []*RunTrack{},
			candidate:     []*RunTrack{{TrackID: "cand-1", StartUnixNanos: 1000, EndUnixNanos: 2000}},
			expectedCount: 0,
			expectedPairs: map[string]string{},
			minIoU:        0.0,
		},
		{
			name:          "empty candidate",
			reference:     []*RunTrack{{TrackID: "ref-1", StartUnixNanos: 1000, EndUnixNanos: 2000}},
			candidate:     []*RunTrack{},
			expectedCount: 0,
			expectedPairs: map[string]string{},
			minIoU:        0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := matchTracks(tt.reference, tt.candidate)

			if len(matches) != tt.expectedCount {
				t.Errorf("matchTracks() returned %d matches, want %d", len(matches), tt.expectedCount)
			}

			// Verify expected pairs
			for _, match := range matches {
				expectedCandID, ok := tt.expectedPairs[match.ReferenceTrackID]
				if !ok {
					t.Errorf("unexpected match for reference track %s", match.ReferenceTrackID)
					continue
				}
				if match.CandidateTrackID != expectedCandID {
					t.Errorf("reference track %s matched to %s, want %s",
						match.ReferenceTrackID, match.CandidateTrackID, expectedCandID)
				}
				if match.TemporalIoU < tt.minIoU {
					t.Errorf("match IoU %f below expected minimum %f", match.TemporalIoU, tt.minIoU)
				}
			}
		})
	}
}

// TestEvaluateGroundTruth tests the full ground truth evaluation with various scenarios.
func TestEvaluateGroundTruth(t *testing.T) {
	weights := DefaultGroundTruthWeights()

	tests := []struct {
		name              string
		reference         []*RunTrack
		candidate         []*RunTrack
		expectedDetection float64
		expectedFP        float64
		minComposite      float64
	}{
		{
			name: "perfect detection - all matched",
			reference: []*RunTrack{
				{TrackID: "ref-1", StartUnixNanos: 1000, EndUnixNanos: 2000, UserLabel: "good_vehicle"},
				{TrackID: "ref-2", StartUnixNanos: 3000, EndUnixNanos: 4000, UserLabel: "good_vehicle"},
			},
			candidate: []*RunTrack{
				{TrackID: "cand-1", StartUnixNanos: 1000, EndUnixNanos: 2000, AvgSpeedMps: 10.0},
				{TrackID: "cand-2", StartUnixNanos: 3000, EndUnixNanos: 4000, AvgSpeedMps: 15.0},
			},
			expectedDetection: 1.0,
			expectedFP:        0.0,
			minComposite:      0.9,
		},
		{
			name: "50% detection - one missed",
			reference: []*RunTrack{
				{TrackID: "ref-1", StartUnixNanos: 1000, EndUnixNanos: 2000, UserLabel: "good_vehicle"},
				{TrackID: "ref-2", StartUnixNanos: 3000, EndUnixNanos: 4000, UserLabel: "good_vehicle"},
			},
			candidate: []*RunTrack{
				{TrackID: "cand-1", StartUnixNanos: 1000, EndUnixNanos: 2000, AvgSpeedMps: 10.0},
			},
			expectedDetection: 0.5,
			expectedFP:        0.0,
			minComposite:      0.4,
		},
		{
			name: "false positives present",
			reference: []*RunTrack{
				{TrackID: "ref-1", StartUnixNanos: 1000, EndUnixNanos: 2000, UserLabel: "good_vehicle"},
			},
			candidate: []*RunTrack{
				{TrackID: "cand-1", StartUnixNanos: 1000, EndUnixNanos: 2000, AvgSpeedMps: 10.0},
				{TrackID: "cand-2", StartUnixNanos: 5000, EndUnixNanos: 6000, AvgSpeedMps: 5.0},
				{TrackID: "cand-3", StartUnixNanos: 7000, EndUnixNanos: 8000, AvgSpeedMps: 8.0},
			},
			expectedDetection: 1.0,
			expectedFP:        0.666, // 2/3 unmatched
			minComposite:      -0.5,  // Penalty for false positives
		},
		{
			name: "quality labels affect score",
			reference: []*RunTrack{
				{TrackID: "ref-1", StartUnixNanos: 1000, EndUnixNanos: 2000, UserLabel: "good_vehicle"},
			},
			candidate: []*RunTrack{
				{TrackID: "cand-1", StartUnixNanos: 1000, EndUnixNanos: 2000, AvgSpeedMps: 10.0, QualityLabel: "perfect"},
			},
			expectedDetection: 1.0,
			expectedFP:        0.0,
			minComposite:      1.0, // Detection + quality premium
		},
		{
			name: "noise tracks filtered from reference",
			reference: []*RunTrack{
				{TrackID: "ref-1", StartUnixNanos: 1000, EndUnixNanos: 2000, UserLabel: "good_vehicle"},
				{TrackID: "ref-2", StartUnixNanos: 3000, EndUnixNanos: 4000, UserLabel: "noise"},
				{TrackID: "ref-3", StartUnixNanos: 5000, EndUnixNanos: 6000, UserLabel: "noise_flora"},
			},
			candidate: []*RunTrack{
				{TrackID: "cand-1", StartUnixNanos: 1000, EndUnixNanos: 2000, AvgSpeedMps: 10.0},
			},
			expectedDetection: 1.0, // Only good_vehicle counts in reference
			expectedFP:        0.0,
			minComposite:      0.9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := EvaluateGroundTruth(tt.reference, tt.candidate, weights)

			if math.Abs(score.DetectionRate-tt.expectedDetection) > 0.01 {
				t.Errorf("DetectionRate = %f, want %f", score.DetectionRate, tt.expectedDetection)
			}

			if math.Abs(score.FalsePositiveRate-tt.expectedFP) > 0.01 {
				t.Errorf("FalsePositiveRate = %f, want %f", score.FalsePositiveRate, tt.expectedFP)
			}

			if score.CompositeScore < tt.minComposite {
				t.Errorf("CompositeScore = %f, want >= %f", score.CompositeScore, tt.minComposite)
			}

			// Sanity checks
			if score.DetectionRate < 0 || score.DetectionRate > 1 {
				t.Errorf("DetectionRate %f out of range [0, 1]", score.DetectionRate)
			}
			if score.FalsePositiveRate < 0 || score.FalsePositiveRate > 1 {
				t.Errorf("FalsePositiveRate %f out of range [0, 1]", score.FalsePositiveRate)
			}
		})
	}
}

// TestEvaluateGroundTruthDetectionByClass tests class-specific detection rates.
func TestEvaluateGroundTruthDetectionByClass(t *testing.T) {
	weights := DefaultGroundTruthWeights()

	reference := []*RunTrack{
		{TrackID: "ref-v1", StartUnixNanos: 1000, EndUnixNanos: 2000, UserLabel: "good_vehicle"},
		{TrackID: "ref-v2", StartUnixNanos: 3000, EndUnixNanos: 4000, UserLabel: "good_vehicle"},
		{TrackID: "ref-p1", StartUnixNanos: 5000, EndUnixNanos: 6000, UserLabel: "good_pedestrian"},
		{TrackID: "ref-o1", StartUnixNanos: 7000, EndUnixNanos: 8000, UserLabel: "good_other"},
	}

	candidate := []*RunTrack{
		{TrackID: "cand-v1", StartUnixNanos: 1000, EndUnixNanos: 2000, AvgSpeedMps: 10.0}, // Matches ref-v1
		// ref-v2 missed
		{TrackID: "cand-p1", StartUnixNanos: 5000, EndUnixNanos: 6000, AvgSpeedMps: 2.0}, // Matches ref-p1
		{TrackID: "cand-o1", StartUnixNanos: 7000, EndUnixNanos: 8000, AvgSpeedMps: 5.0}, // Matches ref-o1
	}

	score := EvaluateGroundTruth(reference, candidate, weights)

	// Check overall detection rate: 3/4 = 0.75
	if math.Abs(score.DetectionRate-0.75) > 0.01 {
		t.Errorf("DetectionRate = %f, want 0.75", score.DetectionRate)
	}

	// Check class-specific rates
	expectedRates := map[string]float64{
		"good_vehicle":    0.5, // 1/2 vehicles detected
		"good_pedestrian": 1.0, // 1/1 pedestrian detected
		"good_other":      1.0, // 1/1 other detected
	}

	for class, expectedRate := range expectedRates {
		actualRate, ok := score.DetectionRateByClass[class]
		if !ok {
			t.Errorf("missing detection rate for class %s", class)
			continue
		}
		if math.Abs(actualRate-expectedRate) > 0.01 {
			t.Errorf("DetectionRateByClass[%s] = %f, want %f", class, actualRate, expectedRate)
		}
	}
}
