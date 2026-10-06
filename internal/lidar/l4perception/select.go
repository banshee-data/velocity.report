package l4perception

import (
	"math"
	"math/bits"
	"sort"
)

// float64Less is sort.Float64Slice's order: numbers ascending, with NaN
// before every number. Selection uses it so that an order statistic is the
// value sort.Float64s would put at the same position.
func float64Less(x, y float64) bool {
	return x < y || (math.IsNaN(x) && !math.IsNaN(y))
}

// NthFloat64 rearranges a so that a[k] holds the value sort.Float64s would put
// at position k, with nothing after it ordered before it and nothing before it
// ordered after it, and returns that value. Two order statistics of one slice
// therefore cost two selections rather than a sort. The only difference from a
// sort is the one no sort promises either: -0 and +0 order as equal, so which
// of them lands at k may differ.
//
// It is a quickselect with a median-of-three pivot and a three-way partition,
// so runs of equal values (and NaNs, which order as equal to each other) cost
// one pass. It runs in linear time on average. After 2·log2(n) partitions that
// have not isolated k it sorts what remains, which bounds the worst case at
// O(n log n), a sort's cost.
func NthFloat64(a []float64, k int) float64 {
	return nthFloat64Budget(a, k, 2*bits.Len(uint(len(a))))
}

// nthFloat64Budget is NthFloat64 with the number of partitions it may make
// before falling back to a sort.
func nthFloat64Budget(a []float64, k, budget int) float64 {
	lo, hi := 0, len(a)-1
	for hi > lo {
		if budget <= 0 {
			sort.Float64s(a[lo : hi+1])
			return a[k]
		}
		budget--
		pivot := a[medianOfThree(a, lo, lo+(hi-lo)/2, hi)]
		// Partition a[lo:hi+1] into below, equal to and above the pivot:
		// a[lo:lt] < pivot, a[lt:gt+1] == pivot, a[gt+1:hi+1] > pivot.
		lt, i, gt := lo, lo, hi
		for i <= gt {
			switch {
			case float64Less(a[i], pivot):
				a[lt], a[i] = a[i], a[lt]
				lt++
				i++
			case float64Less(pivot, a[i]):
				a[i], a[gt] = a[gt], a[i]
				gt--
			default:
				i++
			}
		}
		switch {
		case k < lt:
			hi = lt - 1
		case k > gt:
			lo = gt + 1
		default:
			return a[k]
		}
	}
	return a[k]
}

// medianOfThree is the index, among i, j and k, of the median of their values.
func medianOfThree(a []float64, i, j, k int) int {
	if float64Less(a[j], a[i]) {
		i, j = j, i
	}
	if float64Less(a[k], a[j]) {
		j = k
		if float64Less(a[j], a[i]) {
			j = i
		}
	}
	return j
}
