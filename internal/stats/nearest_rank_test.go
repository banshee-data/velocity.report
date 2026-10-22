package stats

import (
	"math"
	"testing"
)

func TestNearestRank(t *testing.T) {
	t.Parallel()
	five := []float64{10, 20, 30, 40, 50}
	tests := []struct {
		name   string
		sorted []float64
		p      float64
		want   float64
	}{
		{"zero clamps to first", five, 0, 10},
		{"median of five is the third", five, 0.5, 30},
		{"p85 of five is the fifth", five, 0.85, 50},
		{"p98 of five is the fifth", five, 0.98, 50},
		{"one clamps to last", five, 1, 50},
		{"single value", []float64{7}, 0.85, 7},
		{"median of an even count is the lower middle", []float64{1, 2, 3, 4}, 0.5, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NearestRank(tt.sorted, tt.p); got != tt.want {
				t.Fatalf("NearestRank(p=%v) = %v, want %v", tt.p, got, tt.want)
			}
		})
	}
}

// An exact rank must not be pushed to the next value by rounding in p*n.
// 0.85*100 and 0.98*50 are integers in decimal arithmetic.
func TestNearestRankExactRanks(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		p    float64
		n    int
		rank int
	}{
		{0.85, 100, 85},
		{0.98, 50, 49},
		{0.85, 20, 17},
		{0.5, 10, 5},
		{0.85, 1000, 850},
		{0.98, 10000, 9800},
	} {
		sorted := make([]float64, tt.n)
		for i := range sorted {
			sorted[i] = float64(i + 1)
		}
		if got := NearestRank(sorted, tt.p); got != float64(tt.rank) {
			t.Errorf("NearestRank(p=%v, n=%d) = %v, want rank %d", tt.p, tt.n, got, tt.rank)
		}
	}
}

func TestNearestRankAlwaysReturnsAnObservedValue(t *testing.T) {
	t.Parallel()
	sorted := []float64{1.5, 2.25, 9}
	for _, p := range []float64{0, 0.1, 0.33, 0.5, 0.67, 0.85, 0.98, 1} {
		got := NearestRank(sorted, p)
		if got != 1.5 && got != 2.25 && got != 9 {
			t.Fatalf("NearestRank(p=%v) = %v, not one of the values", p, got)
		}
	}
}

func TestNearestRankEmpty(t *testing.T) {
	t.Parallel()
	if got := NearestRank(nil, 0.85); !math.IsNaN(got) {
		t.Fatalf("NearestRank(nil) = %v, want NaN", got)
	}
}
