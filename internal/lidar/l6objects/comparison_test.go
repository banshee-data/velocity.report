package l6objects

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
		{
			name:   "perfect overlap",
			startA: 100, endA: 200, startB: 100, endB: 200,
			want: 1.0,
		},
		{
			name:   "no overlap B after A",
			startA: 100, endA: 200, startB: 300, endB: 400,
			want: 0.0,
		},
		{
			name:   "no overlap B before A",
			startA: 300, endA: 400, startB: 100, endB: 200,
			want: 0.0,
		},
		{
			name:   "touching edges no overlap",
			startA: 100, endA: 200, startB: 200, endB: 300,
			want: 0.0,
		},
		{
			name:   "partial overlap",
			startA: 100, endA: 300, startB: 200, endB: 400,
			// intersection: 200-300 = 100, union: 100-400 = 300
			want: 1.0 / 3.0,
		},
		{
			name:   "B contained in A",
			startA: 100, endA: 400, startB: 200, endB: 300,
			// intersection: 200-300 = 100, union: 100-400 = 300
			want: 1.0 / 3.0,
		},
		{
			name:   "A contained in B",
			startA: 200, endA: 300, startB: 100, endB: 400,
			// intersection: 200-300 = 100, union: 100-400 = 300
			want: 1.0 / 3.0,
		},
		{
			name:   "equal start different end",
			startA: 100, endA: 200, startB: 100, endB: 300,
			// intersection: 100-200 = 100, union: 100-300 = 200
			want: 0.5,
		},
		{
			name:   "different start equal end",
			startA: 100, endA: 300, startB: 200, endB: 300,
			// intersection: 200-300 = 100, union: 100-300 = 200
			want: 0.5,
		},
		{
			name:   "zero length A",
			startA: 100, endA: 100, startB: 100, endB: 200,
			want: 0.0,
		},
		{
			name:   "zero length B",
			startA: 100, endA: 200, startB: 150, endB: 150,
			want: 0.0,
		},
		{
			name:   "negative values",
			startA: -300, endA: -100, startB: -200, endB: 0,
			// intersection: -200 to -100 = 100, union: -300 to 0 = 300
			want: 1.0 / 3.0,
		},
		{
			name:   "large nanosecond values",
			startA: 1_000_000_000, endA: 2_000_000_000,
			startB: 1_500_000_000, endB: 2_500_000_000,
			// intersection: 1.5B-2B = 500M, union: 1B-2.5B = 1.5B
			want: 1.0 / 3.0,
		},
		{
			name:   "half overlap symmetric",
			startA: 0, endA: 200, startB: 100, endB: 300,
			// intersection: 100-200 = 100, union: 0-300 = 300
			want: 1.0 / 3.0,
		},
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
