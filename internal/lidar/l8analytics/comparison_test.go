package l8analytics

import (
	"math"
	"testing"
)

func TestComputeTemporalIoU(t *testing.T) {
	tests := []struct {
		name                       string
		startA, endA, startB, endB int64
		want                       float64
	}{
		{"perfect overlap", 100, 200, 100, 200, 1.0},
		{"no overlap B after A", 100, 200, 300, 400, 0.0},
		{"no overlap B before A", 300, 400, 100, 200, 0.0},
		{"touching edges no overlap", 100, 200, 200, 300, 0.0},
		{"partial overlap", 100, 300, 200, 400, 1.0 / 3.0},
		{"B contained in A", 100, 400, 200, 300, 1.0 / 3.0},
		{"A contained in B", 200, 300, 100, 400, 1.0 / 3.0},
		{"equal start different end", 100, 200, 100, 300, 0.5},
		{"different start equal end", 100, 300, 200, 300, 0.5},
		{"zero length A", 100, 100, 100, 200, 0.0},
		{"zero length B", 100, 200, 150, 150, 0.0},
		{"negative values", -300, -100, -200, 0, 1.0 / 3.0},
		{"large nanosecond values", 1_000_000_000, 2_000_000_000, 1_500_000_000, 2_500_000_000, 1.0 / 3.0},
		{"half overlap symmetric", 0, 200, 100, 300, 1.0 / 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeTemporalIoU(tt.startA, tt.endA, tt.startB, tt.endB)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("ComputeTemporalIoU(%d, %d, %d, %d) = %f, want %f",
					tt.startA, tt.endA, tt.startB, tt.endB, got, tt.want)
			}
		})
	}
}
