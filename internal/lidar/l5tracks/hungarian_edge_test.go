package l5tracks

import "testing"

// Gaps H1 and H2 in data/maths/paper-implementation-gap-analysis.md. The
// forbidden sentinel is 1e18 cast to float32 beside real costs of order
// 0.01-36, so the extreme-range case is the shipped configuration, and the
// all-forbidden matrix is the state of every frame in which no cluster falls
// inside any track's guards.

func TestHungarianAssign_ExtremeCostRange(t *testing.T) {
	cases := []struct {
		name string
		cost [][]float32
		want []int
	}{
		{
			name: "diagonal optimum across eighteen orders of magnitude",
			cost: [][]float32{
				{1e-6, 1e12, 1e12},
				{1e12, 1e-6, 1e12},
				{1e12, 1e12, 1e-6},
			},
			want: []int{0, 1, 2},
		},
		{
			name: "off-diagonal optimum with large finite alternatives",
			cost: [][]float32{
				{1e12, 1e-6, 5},
				{1e-6, 1e12, 7},
				{3, 4, 1e-6},
			},
			want: []int{1, 0, 2},
		},
		{
			name: "tiny costs beside the forbidden sentinel",
			cost: [][]float32{
				{1e-6, float32(hungarianlnf)},
				{float32(hungarianlnf), 2e-6},
			},
			want: []int{0, 1},
		},
	}
	for _, tc := range cases {
		got := HungarianAssign(tc.cost)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: assignment length %d, want %d", tc.name, len(got), len(tc.want))
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: assignment = %v, want %v", tc.name, got, tc.want)
			}
		}
	}
}

func TestHungarianAssign_AllForbidden(t *testing.T) {
	for _, n := range []int{1, 3, 7} {
		cost := make([][]float32, n)
		for i := range cost {
			cost[i] = make([]float32, n)
			for j := range cost[i] {
				cost[i][j] = float32(hungarianlnf)
			}
		}
		got := HungarianAssign(cost)
		if len(got) != n {
			t.Fatalf("n=%d: assignment length %d", n, len(got))
		}
		for i, v := range got {
			if v != -1 {
				t.Fatalf("n=%d: row %d assigned to %d with every pairing forbidden, want -1", n, i, v)
			}
		}
	}
}
