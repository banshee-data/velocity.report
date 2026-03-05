package l5tracks

import (
	"math"
	"sort"
)

// speedWindow maintains a bounded sliding window of speed observations with
// a sorted view for O(1) percentile queries. The queue stores values in
// insertion order (ring-buffer semantics); the sorted slice holds the same
// elements in ascending order so that any percentile can be read by index.
//
// Complexity per Add(): O(n) for n ≤ maxLen (binary search + slice shift).
// Complexity per Percentile(): O(1).
type speedWindow struct {
	queue  []float32 // insertion order
	sorted []float32 // ascending order, same elements as queue
	maxLen int
}

// newSpeedWindow creates a speed window with the given maximum length.
// Both internal slices are pre-allocated to avoid repeated growth.
func newSpeedWindow(maxLen int) *speedWindow {
	return &speedWindow{
		queue:  make([]float32, 0, maxLen),
		sorted: make([]float32, 0, maxLen),
		maxLen: maxLen,
	}
}

// Add inserts a new speed observation. If the window is at capacity the
// oldest observation is evicted first. No-op if maxLen is 0.
func (sw *speedWindow) Add(speed float32) {
	if sw.maxLen <= 0 {
		return
	}
	if len(sw.queue) >= sw.maxLen {
		// Evict oldest entry from both views.
		oldest := sw.queue[0]
		sw.queue = sw.queue[1:]
		idx := sort.Search(len(sw.sorted), func(i int) bool {
			return sw.sorted[i] >= oldest
		})
		if idx < len(sw.sorted) {
			sw.sorted = append(sw.sorted[:idx], sw.sorted[idx+1:]...)
		}
	}

	// Append to insertion-order queue.
	sw.queue = append(sw.queue, speed)

	// Insert into sorted position via binary search.
	idx := sort.Search(len(sw.sorted), func(i int) bool {
		return sw.sorted[i] >= speed
	})
	sw.sorted = append(sw.sorted, 0)
	copy(sw.sorted[idx+1:], sw.sorted[idx:])
	sw.sorted[idx] = speed
}

// Percentile returns the value at the given percentile p ∈ [0, 1].
// Uses floor-based indexing consistent with ComputeSpeedPercentiles.
// Returns 0 if the window is empty.
func (sw *speedWindow) Percentile(p float64) float32 {
	n := len(sw.sorted)
	if n == 0 {
		return 0
	}
	idx := int(math.Floor(float64(n) * p))
	if idx >= n {
		idx = n - 1
	}
	return sw.sorted[idx]
}

// P50 returns the 50th percentile (median) speed. O(1).
func (sw *speedWindow) P50() float32 {
	n := len(sw.sorted)
	if n == 0 {
		return 0
	}
	return sw.sorted[n/2]
}

// Len returns the number of observations in the window.
func (sw *speedWindow) Len() int { return len(sw.queue) }

// Values returns a copy of the insertion-order observations.
// Callers may sort or mutate the returned slice freely.
func (sw *speedWindow) Values() []float32 {
	if len(sw.queue) == 0 {
		return nil
	}
	out := make([]float32, len(sw.queue))
	copy(out, sw.queue)
	return out
}
