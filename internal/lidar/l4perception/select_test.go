package l4perception

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
)

// sameOrderValue is equality in sort.Float64Slice's order: NaN equals NaN,
// and -0 equals +0, whose relative order no sort promises.
func sameOrderValue(x, y float64) bool {
	return x == y || (math.IsNaN(x) && math.IsNaN(y))
}

// randomValues draws n values from one of several shapes that stress a
// selection: spread, heavy ties, already ordered, reversed, constant, and
// laced with NaN, ±Inf and signed zeros.
func randomValues(rng *rand.Rand, n, shape int) []float64 {
	v := make([]float64, n)
	for i := range v {
		switch shape {
		case 0:
			v[i] = rng.NormFloat64() * 3
		case 1:
			v[i] = float64(rng.Intn(4)) // heavy ties
		case 2:
			v[i] = float64(i) * 0.01 // ascending, as scan order can be
		case 3:
			v[i] = float64(n-i) * 0.01 // descending
		case 4:
			v[i] = 1.25 // constant
		default:
			switch rng.Intn(8) {
			case 0:
				v[i] = math.NaN()
			case 1:
				v[i] = math.Inf(1)
			case 2:
				v[i] = math.Inf(-1)
			case 3:
				v[i] = math.Copysign(0, -1)
			case 4:
				v[i] = 0
			default:
				v[i] = rng.NormFloat64()
			}
		}
	}
	return v
}

func TestNthFloat64MatchesSort(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for shape := 0; shape <= 5; shape++ {
		for _, n := range []int{1, 2, 3, 7, 8, 31, 100, 257} {
			values := randomValues(rng, n, shape)
			sorted := append([]float64(nil), values...)
			sort.Float64s(sorted)
			for k := 0; k < n; k++ {
				a := append([]float64(nil), values...)
				got := NthFloat64(a, k)
				if !sameOrderValue(got, sorted[k]) {
					t.Fatalf("shape %d n %d k %d: got %v, sort gives %v", shape, n, k, got, sorted[k])
				}
				for i := 0; i < k; i++ {
					if float64Less(a[k], a[i]) {
						t.Fatalf("shape %d n %d k %d: a[%d]=%v ordered after a[k]=%v", shape, n, k, i, a[i], a[k])
					}
				}
				for i := k + 1; i < n; i++ {
					if float64Less(a[i], a[k]) {
						t.Fatalf("shape %d n %d k %d: a[%d]=%v ordered before a[k]=%v", shape, n, k, i, a[i], a[k])
					}
				}
			}
		}
	}
}

// With no partition budget the selection sorts the range, the bound on its
// worst case; with a small one it partitions and then sorts what remains.
func TestNthFloat64BudgetFallsBackToSort(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	values := randomValues(rng, 200, 0)
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	for _, budget := range []int{0, 1, 2} {
		for _, k := range []int{0, 2, 99, 197, 199} {
			a := append([]float64(nil), values...)
			if got := nthFloat64Budget(a, k, budget); got != sorted[k] {
				t.Fatalf("budget %d k %d: got %v, want %v", budget, k, got, sorted[k])
			}
		}
	}
}

func TestMedianOfThree(t *testing.T) {
	nan := math.NaN()
	for _, c := range []struct {
		a    []float64
		want float64
	}{
		{[]float64{1, 2, 3}, 2}, {[]float64{3, 2, 1}, 2}, {[]float64{2, 3, 1}, 2},
		{[]float64{2, 1, 3}, 2}, {[]float64{1, 3, 2}, 2}, {[]float64{3, 1, 2}, 2},
		{[]float64{5, 5, 1}, 5}, {[]float64{nan, 1, 2}, 1},
	} {
		if got := c.a[medianOfThree(c.a, 0, 1, 2)]; !sameOrderValue(got, c.want) {
			t.Errorf("medianOfThree(%v) = %v, want %v", c.a, got, c.want)
		}
	}
}

// lateralPercentileBySort is LateralPercentile as it was before selection
// replaced the sort. The selection must return what this returns.
func lateralPercentileBySort(points []WorldPoint, dirX, dirY, percentile float64) (float64, bool) {
	if len(points) == 0 || percentile < 0 || percentile > 100 {
		return 0, false
	}
	norm := math.Hypot(dirX, dirY)
	if norm < 1e-12 {
		return 0, false
	}
	dirX, dirY = dirX/norm, dirY/norm
	projections := make([]float64, len(points))
	for i, p := range points {
		projections[i] = p.X*dirX + p.Y*dirY
	}
	sort.Float64s(projections)
	idx := int(percentile / 100 * float64(len(projections)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(projections) {
		idx = len(projections) - 1
	}
	return projections[idx], true
}

func worldPointsFrom(xs, ys []float64) []WorldPoint {
	points := make([]WorldPoint, len(xs))
	for i := range xs {
		points[i] = WorldPoint{X: xs[i], Y: ys[i]}
	}
	return points
}

// The near-edge offset the selection measures is the sort's at every
// percentile, so every face plane is where it was. Exact zeros are the one
// value whose sign the two may order differently, and no sort promises it.
func TestLateralPercentileMatchesSort(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	for shape := 0; shape <= 5; shape++ {
		for _, n := range []int{1, 2, 5, 50, 201, 1024, 4000} {
			points := worldPointsFrom(randomValues(rng, n, shape), randomValues(rng, n, (shape+1)%6))
			for _, percentile := range []float64{0, 1, NearEdgePercentile, 50, 95, 100} {
				for deg := 0; deg < 360; deg += 45 {
					angle := float64(deg) * math.Pi / 180
					dirX, dirY := math.Cos(angle), math.Sin(angle)
					got, gotOK := LateralPercentile(points, dirX, dirY, percentile)
					want, wantOK := lateralPercentileBySort(points, dirX, dirY, percentile)
					same := math.Float64bits(got) == math.Float64bits(want) || (got == 0 && want == 0) || (math.IsNaN(got) && math.IsNaN(want))
					if gotOK != wantOK || !same {
						t.Fatalf("shape %d n %d p %v deg %d: got (%v, %v), sort gives (%v, %v)", shape, n, percentile, deg, got, gotOK, want, wantOK)
					}
				}
			}
		}
	}
}

// On points with no exact zero projection the offset is the sort's bit for
// bit, as every real cluster's is.
func TestLateralPercentileBitIdenticalOnClusters(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	for _, n := range []int{3, 40, 600, 3000} {
		xs, ys := make([]float64, n), make([]float64, n)
		for i := range xs {
			xs[i] = 20 + rng.NormFloat64()*2
			ys[i] = -7 + rng.NormFloat64()*0.8
		}
		points := worldPointsFrom(xs, ys)
		for deg := 0; deg < 360; deg += 7 {
			angle := float64(deg) * math.Pi / 180
			got, _ := LateralPercentile(points, math.Cos(angle), math.Sin(angle), NearEdgePercentile)
			want, _ := lateralPercentileBySort(points, math.Cos(angle), math.Sin(angle), NearEdgePercentile)
			if math.Float64bits(got) != math.Float64bits(want) {
				t.Fatalf("n %d deg %d: got %v, sort gives %v", n, deg, got, want)
			}
		}
	}
}

// BenchmarkLateralPercentile compares the selection with the sort it replaced
// at the near-edge percentile.
func BenchmarkLateralPercentile(b *testing.B) {
	rng := rand.New(rand.NewSource(7))
	for _, n := range []int{256, 1024, 4096} {
		points := worldPointsFrom(randomValues(rng, n, 0), randomValues(rng, n, 0))
		dirX, dirY := math.Cos(0.3), math.Sin(0.3)
		b.Run(fmt.Sprintf("select/n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				LateralPercentile(points, dirX, dirY, NearEdgePercentile)
			}
		})
		b.Run(fmt.Sprintf("sort/n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				lateralPercentileBySort(points, dirX, dirY, NearEdgePercentile)
			}
		})
	}
}
