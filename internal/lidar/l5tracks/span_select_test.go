package l5tracks

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// trimmedSpanBySort is trimmedSpan as it was before selection replaced the
// sort. The selection must return exactly what this returns.
func trimmedSpanBySort(points []l4perception.WorldPoint, scratch []float64, dirX, dirY float64) (float64, bool) {
	for i, p := range points {
		scratch[i] = p.X*dirX + p.Y*dirY
	}
	sort.Float64s(scratch)
	last := len(scratch) - 1
	lo := int(spanTrimPercent / 100 * float64(last))
	span := scratch[last-lo] - scratch[lo]
	if !(span > 0) || math.IsInf(span, 0) {
		return 0, false
	}
	return span, true
}

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
				got := nthFloat64(a, k)
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

func pointsFrom(xs, ys []float64) []l4perception.WorldPoint {
	points := make([]l4perception.WorldPoint, len(xs))
	for i := range xs {
		points[i] = l4perception.WorldPoint{X: xs[i], Y: ys[i]}
	}
	return points
}

// The span the selection measures is the sort's, bit for bit, whatever the
// points and the direction, so every solid body admits the same extents.
func TestTrimmedSpanMatchesSort(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for shape := 0; shape <= 5; shape++ {
		for _, n := range []int{DefaultMinFaceSupport, 9, 50, 101, 256, 1024, 4000} {
			points := pointsFrom(randomValues(rng, n, shape), randomValues(rng, n, (shape+1)%6))
			for deg := -10; deg <= 10; deg += 5 {
				angle := float64(deg) * math.Pi / 180
				dirX, dirY := math.Cos(angle), math.Sin(angle)
				gotSpan, gotOK := trimmedSpan(points, make([]float64, n), dirX, dirY)
				wantSpan, wantOK := trimmedSpanBySort(points, make([]float64, n), dirX, dirY)
				if gotOK != wantOK || math.Float64bits(gotSpan) != math.Float64bits(wantSpan) {
					t.Fatalf("shape %d n %d deg %d: got (%v, %v), sort gives (%v, %v)", shape, n, deg, gotSpan, gotOK, wantSpan, wantOK)
				}
			}
		}
	}
}

// BenchmarkTrimmedSpan compares the selection with the sort it replaced, at
// cluster sizes a solid body sees with full members.
func BenchmarkTrimmedSpan(b *testing.B) {
	rng := rand.New(rand.NewSource(4))
	for _, n := range []int{256, 1024, 4096} {
		points := pointsFrom(randomValues(rng, n, 0), randomValues(rng, n, 0))
		scratch := make([]float64, n)
		dirX, dirY := math.Cos(0.1), math.Sin(0.1)
		b.Run(fmt.Sprintf("select/n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				trimmedSpan(points, scratch, dirX, dirY)
			}
		})
		b.Run(fmt.Sprintf("sort/n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				trimmedSpanBySort(points, scratch, dirX, dirY)
			}
		})
	}
}
