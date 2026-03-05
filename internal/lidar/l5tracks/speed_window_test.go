package l5tracks

import (
	"math"
	"testing"
)

func TestSpeedWindowEmpty(t *testing.T) {
	sw := newSpeedWindow(10)
	if sw.Len() != 0 {
		t.Errorf("expected Len()=0, got %d", sw.Len())
	}
	if p := sw.P50(); p != 0 {
		t.Errorf("expected P50()=0 for empty window, got %f", p)
	}
	if p := sw.Percentile(0.85); p != 0 {
		t.Errorf("expected Percentile(0.85)=0 for empty window, got %f", p)
	}
	if v := sw.Values(); v != nil {
		t.Errorf("expected Values()=nil for empty window, got %v", v)
	}
}

func TestSpeedWindowSingleElement(t *testing.T) {
	sw := newSpeedWindow(10)
	sw.Add(5.0)
	if sw.Len() != 1 {
		t.Errorf("expected Len()=1, got %d", sw.Len())
	}
	if p := sw.P50(); p != 5.0 {
		t.Errorf("expected P50()=5.0, got %f", p)
	}
	if p := sw.Percentile(0.98); p != 5.0 {
		t.Errorf("expected Percentile(0.98)=5.0, got %f", p)
	}
}

func TestSpeedWindowFillsToCapacity(t *testing.T) {
	sw := newSpeedWindow(5)
	for i := 1; i <= 5; i++ {
		sw.Add(float32(i))
	}
	if sw.Len() != 5 {
		t.Errorf("expected Len()=5, got %d", sw.Len())
	}

	// sorted: [1,2,3,4,5], median index = 5/2 = 2 → value 3
	if p := sw.P50(); p != 3.0 {
		t.Errorf("expected P50()=3.0, got %f", p)
	}

	// Values should be in insertion order
	vals := sw.Values()
	expected := []float32{1, 2, 3, 4, 5}
	for i, v := range expected {
		if vals[i] != v {
			t.Errorf("Values()[%d]: expected %f, got %f", i, v, vals[i])
		}
	}
}

func TestSpeedWindowEviction(t *testing.T) {
	sw := newSpeedWindow(3)
	sw.Add(10)
	sw.Add(20)
	sw.Add(30)
	// Window: [10, 20, 30]

	sw.Add(5)
	// Evicts 10; window: [20, 30, 5]
	if sw.Len() != 3 {
		t.Errorf("expected Len()=3 after eviction, got %d", sw.Len())
	}

	// sorted: [5, 20, 30], median index = 3/2 = 1 → value 20
	if p := sw.P50(); p != 20.0 {
		t.Errorf("expected P50()=20.0, got %f", p)
	}

	// Values in insertion order
	vals := sw.Values()
	expectedQueue := []float32{20, 30, 5}
	for i, v := range expectedQueue {
		if vals[i] != v {
			t.Errorf("Values()[%d]: expected %f, got %f", i, v, vals[i])
		}
	}
}

func TestSpeedWindowPercentiles(t *testing.T) {
	// 20 elements: 1..20
	sw := newSpeedWindow(20)
	for i := 1; i <= 20; i++ {
		sw.Add(float32(i))
	}

	// sorted: [1,2,...,20]
	// p50: index = floor(20 * 0.5) = 10 → value 11 (0-indexed: sorted[10]=11)
	if p := sw.P50(); p != 11.0 {
		t.Errorf("P50: expected 11.0, got %f", p)
	}

	// p85: index = floor(20 * 0.85) = 17 → sorted[17] = 18
	if p := sw.Percentile(0.85); p != 18.0 {
		t.Errorf("P85: expected 18.0, got %f", p)
	}

	// p98: index = floor(20 * 0.98) = 19 → sorted[19] = 20
	if p := sw.Percentile(0.98); p != 20.0 {
		t.Errorf("P98: expected 20.0, got %f", p)
	}
}

func TestSpeedWindowDuplicateValues(t *testing.T) {
	sw := newSpeedWindow(5)
	sw.Add(3.0)
	sw.Add(3.0)
	sw.Add(3.0)
	sw.Add(3.0)
	sw.Add(3.0)

	if p := sw.P50(); p != 3.0 {
		t.Errorf("expected P50()=3.0 with all duplicates, got %f", p)
	}

	// Evict one, add different value
	sw.Add(7.0)
	// Window: [3,3,3,3,7], sorted: [3,3,3,3,7]
	if p := sw.P50(); p != 3.0 {
		t.Errorf("expected P50()=3.0, got %f", p)
	}
	if sw.Len() != 5 {
		t.Errorf("expected Len()=5, got %d", sw.Len())
	}
}

func TestSpeedWindowValuesCopySemantic(t *testing.T) {
	sw := newSpeedWindow(5)
	sw.Add(1.0)
	sw.Add(2.0)

	vals := sw.Values()
	vals[0] = 999 // mutate returned copy

	// Original should be unaffected
	vals2 := sw.Values()
	if vals2[0] != 1.0 {
		t.Errorf("Values() should return a copy; mutation leaked through: got %f", vals2[0])
	}
}

func TestSpeedWindowConsistentWithSortBased(t *testing.T) {
	// Verify that the running window produces the same P50 as sorting the
	// full array, across a sequence of adds with eviction.
	sw := newSpeedWindow(10)
	speeds := []float32{
		4.2, 3.1, 5.5, 2.8, 6.0, 3.9, 7.1, 1.5, 4.8, 5.2,
		8.0, 2.5, 3.3, 6.6, 4.1, 9.0, 1.2, 5.9, 3.7, 4.4,
	}

	for _, s := range speeds {
		sw.Add(s)

		// Compute reference p50 from the current queue contents
		vals := sw.Values()
		sorted := make([]float32, len(vals))
		copy(sorted, vals)
		sortFloat32s(sorted)
		refP50 := sorted[len(sorted)/2]

		got := sw.P50()
		if math.Abs(float64(got-refP50)) > 1e-6 {
			t.Errorf("after adding %.1f: P50()=%.4f, reference=%.4f", s, got, refP50)
		}
	}
}

// sortFloat32s sorts a float32 slice in ascending order.
func sortFloat32s(s []float32) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func TestSpeedWindowZeroMaxLen(t *testing.T) {
	sw := newSpeedWindow(0)
	// Should not panic when maxLen is 0.
	sw.Add(1.0)
	sw.Add(2.0)
	if sw.Len() != 0 {
		t.Errorf("expected Len()=0 with maxLen=0, got %d", sw.Len())
	}
	if p := sw.P50(); p != 0 {
		t.Errorf("expected P50()=0 with maxLen=0, got %f", p)
	}
}
