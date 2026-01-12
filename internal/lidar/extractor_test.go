package lidar

import (
	"testing"
	"time"
)

func TestBackgroundSubtractorExtractor_Interface(t *testing.T) {
	// Verify that BackgroundSubtractorExtractor implements ForegroundExtractor
	var _ ForegroundExtractor = (*BackgroundSubtractorExtractor)(nil)
}

func TestVelocityCoherentExtractor_Interface(t *testing.T) {
	// Verify that VelocityCoherentExtractor implements ForegroundExtractor
	var _ ForegroundExtractor = (*VelocityCoherentExtractor)(nil)
}

func TestHybridExtractor_Interface(t *testing.T) {
	// Verify that HybridExtractor implements ForegroundExtractor
	var _ ForegroundExtractor = (*HybridExtractor)(nil)
}

func TestMergeForegroundMasks_Union(t *testing.T) {
	mask1 := []bool{true, false, true, false}
	mask2 := []bool{false, true, false, false}

	result := MergeForegroundMasks([][]bool{mask1, mask2}, MergeModeUnion)

	expected := []bool{true, true, true, false}
	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("at index %d: expected %v, got %v", i, v, result[i])
		}
	}
}

func TestMergeForegroundMasks_Intersection(t *testing.T) {
	mask1 := []bool{true, true, true, false}
	mask2 := []bool{true, false, true, true}

	result := MergeForegroundMasks([][]bool{mask1, mask2}, MergeModeIntersection)

	expected := []bool{true, false, true, false}
	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("at index %d: expected %v, got %v", i, v, result[i])
		}
	}
}

func TestMergeForegroundMasks_Empty(t *testing.T) {
	result := MergeForegroundMasks(nil, MergeModeUnion)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestCountForeground(t *testing.T) {
	mask := []bool{true, false, true, true, false}
	count := CountForeground(mask)
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestComputeMaskAgreement(t *testing.T) {
	mask1 := []bool{true, true, false, false}
	mask2 := []bool{true, false, false, true}

	agreement := ComputeMaskAgreement(mask1, mask2)
	expected := 0.5 // 2 out of 4 agree
	if agreement != expected {
		t.Errorf("expected %f, got %f", expected, agreement)
	}
}

func TestComputePrecisionRecall(t *testing.T) {
	// Ground truth: indices 0, 1, 2 are foreground
	// Predicted:    indices 0, 1, 3 are foreground
	// TP = 2 (0, 1), FP = 1 (3), FN = 1 (2)

	predicted := []bool{true, true, false, true}
	groundTruth := []bool{true, true, true, false}

	precision, recall := ComputePrecisionRecall(predicted, groundTruth)

	expectedPrecision := 2.0 / 3.0 // TP / (TP + FP)
	expectedRecall := 2.0 / 3.0    // TP / (TP + FN)

	if precision != expectedPrecision {
		t.Errorf("precision: expected %f, got %f", expectedPrecision, precision)
	}
	if recall != expectedRecall {
		t.Errorf("recall: expected %f, got %f", expectedRecall, recall)
	}
}

func TestFrameHistory_Basic(t *testing.T) {
	fh := NewFrameHistory(3)

	if fh.Size() != 0 {
		t.Errorf("expected size 0, got %d", fh.Size())
	}

	// Add frames
	frame1 := &VelocityFrame{FrameID: "f1", Timestamp: time.Now()}
	frame2 := &VelocityFrame{FrameID: "f2", Timestamp: time.Now().Add(time.Second)}
	frame3 := &VelocityFrame{FrameID: "f3", Timestamp: time.Now().Add(2 * time.Second)}

	fh.Add(frame1)
	if fh.Size() != 1 {
		t.Errorf("expected size 1, got %d", fh.Size())
	}

	fh.Add(frame2)
	fh.Add(frame3)
	if fh.Size() != 3 {
		t.Errorf("expected size 3, got %d", fh.Size())
	}

	// Previous(1) should return most recent
	if fh.Previous(1).FrameID != "f3" {
		t.Errorf("Previous(1) should return f3")
	}
	if fh.Previous(2).FrameID != "f2" {
		t.Errorf("Previous(2) should return f2")
	}
	if fh.Previous(3).FrameID != "f1" {
		t.Errorf("Previous(3) should return f1")
	}

	// Add another frame (should evict oldest)
	frame4 := &VelocityFrame{FrameID: "f4"}
	fh.Add(frame4)

	if fh.Size() != 3 {
		t.Errorf("expected size 3 after overflow, got %d", fh.Size())
	}
	if fh.Previous(1).FrameID != "f4" {
		t.Errorf("Previous(1) should return f4 after overflow")
	}
	if fh.Previous(3).FrameID != "f2" {
		t.Errorf("Previous(3) should return f2 after overflow (f1 evicted)")
	}
}

func TestFrameHistory_Clear(t *testing.T) {
	fh := NewFrameHistory(3)
	fh.Add(&VelocityFrame{FrameID: "f1"})
	fh.Add(&VelocityFrame{FrameID: "f2"})

	fh.Clear()

	if fh.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", fh.Size())
	}
	if fh.Previous(1) != nil {
		t.Errorf("expected nil after clear")
	}
}

func TestVelocityCoherentExtractor_Name(t *testing.T) {
	ext := NewVelocityCoherentExtractor(DefaultVelocityCoherentConfig(), "test-sensor")
	if ext.Name() != "velocity_coherent" {
		t.Errorf("expected name 'velocity_coherent', got '%s'", ext.Name())
	}
}

func TestVelocityCoherentExtractor_EmptyInput(t *testing.T) {
	ext := NewVelocityCoherentExtractor(DefaultVelocityCoherentConfig(), "test-sensor")

	mask, metrics, err := ext.ProcessFrame(nil, time.Now())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(mask) != 0 {
		t.Errorf("expected empty mask for empty input")
	}
	if metrics.ForegroundCount != 0 {
		t.Errorf("expected 0 foreground for empty input")
	}
}

func TestHybridExtractor_NoExtractors(t *testing.T) {
	ext := NewHybridExtractor(DefaultHybridExtractorConfig(), nil, "test-sensor")

	points := []PointPolar{{Distance: 10.0, Azimuth: 45.0}}
	_, _, err := ext.ProcessFrame(points, time.Now())

	if err == nil {
		t.Errorf("expected error with no extractors")
	}
}

func TestEvaluationHarness_EmptyInput(t *testing.T) {
	config := EvaluationHarnessConfig{LogComparisons: true}
	harness := NewEvaluationHarness(config, nil)

	results := harness.ProcessFrame(nil, time.Now())
	if results != nil {
		t.Errorf("expected nil results for empty input")
	}
}

func TestComputeClusterVelocityCoherence(t *testing.T) {
	// Create points with consistent velocities
	points := []PointWithVelocity{
		{VX: 5.0, VY: 0.0, Confidence: 0.8},
		{VX: 5.1, VY: 0.1, Confidence: 0.7},
		{VX: 4.9, VY: -0.1, Confidence: 0.9},
	}

	avgVX, avgVY, variance, coherence, count := ComputeClusterVelocityCoherence(points, 0.3)

	if count != 3 {
		t.Errorf("expected 3 valid points, got %d", count)
	}

	// Average should be close to 5.0, 0.0
	if avgVX < 4.9 || avgVX > 5.1 {
		t.Errorf("avgVX should be ~5.0, got %f", avgVX)
	}
	if avgVY < -0.1 || avgVY > 0.1 {
		t.Errorf("avgVY should be ~0.0, got %f", avgVY)
	}

	// Variance should be very low (coherent cluster)
	if variance > 0.1 {
		t.Errorf("variance should be low for coherent cluster, got %f", variance)
	}

	// Coherence should be high
	if coherence < 0.9 {
		t.Errorf("coherence should be high for coherent cluster, got %f", coherence)
	}
}

func TestMedian(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{"empty", []float64{}, 0},
		{"single", []float64{5.0}, 5.0},
		{"odd", []float64{1.0, 3.0, 2.0}, 2.0},
		{"even", []float64{1.0, 4.0, 2.0, 3.0}, 2.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := median(tt.values)
			if result != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}
