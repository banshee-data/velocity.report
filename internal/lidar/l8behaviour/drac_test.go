package l8behaviour

import (
	"encoding/json"
	"math"
	"testing"
)

func dracInput(g, vf, vl float64) DRACInput {
	return DRACInput{GapM: &g, FollowerSpeedMPS: &vf, LeaderSpeedMPS: &vl, Observed: true, SamePath: true}
}

func TestDRACAnalyticalCases(t *testing.T) {
	for _, tt := range []struct {
		name            string
		g, vf, vl, want float64
	}{
		{"known closing", 10, 15, 10, 1.25},
		{"below headway floor", 1, 0.1, 0, 0.005},
		{"small gap without clamp", 1e-12, 1, 0, 5e11},
		{"square would overflow", 1e300, 1e200, 0, 5e99},
		{"doubling gap would overflow", 1e308, 1e154, 0, 0.5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := EvaluateDRAC(dracInput(tt.g, tt.vf, tt.vl))
			if r.Status != DRACSupported || r.ValueMPS2 == nil || math.Abs(*r.ValueMPS2-tt.want) > math.Abs(tt.want)*1e-12 {
				t.Fatalf("got %+v want %g", r, tt.want)
			}
			if r.BoundsMPS2 != nil {
				t.Fatal("invented uncertainty")
			}
		})
	}
}

func TestDRACApplicability(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*DRACInput)
		want DRACStatus
	}{
		{"missing gap", func(i *DRACInput) { i.GapM = nil }, DRACMissingInput},
		{"missing follower", func(i *DRACInput) { i.FollowerSpeedMPS = nil }, DRACMissingInput},
		{"missing leader", func(i *DRACInput) { i.LeaderSpeedMPS = nil }, DRACMissingInput},
		{"coasting", func(i *DRACInput) { i.Observed = false }, DRACMissingSupport},
		{"crossing", func(i *DRACInput) { i.SamePath = false }, DRACIncompatiblePath},
		{"zero gap", func(i *DRACInput) { *i.GapM = 0 }, DRACUnresolvedPositiveGap},
		{"overlap", func(i *DRACInput) { *i.GapM = -1 }, DRACUnresolvedPositiveGap},
		{"gap reaches zero", func(i *DRACInput) { i.GapBoundsM = &DRACBounds{0, 20} }, DRACUnresolvedPositiveGap},
		{"equal", func(i *DRACInput) { *i.LeaderSpeedMPS = 15 }, DRACNonClosing},
		{"diverging", func(i *DRACInput) { *i.LeaderSpeedMPS = 20 }, DRACNonClosing},
		{"stopped queue", func(i *DRACInput) { *i.FollowerSpeedMPS = 0; *i.LeaderSpeedMPS = 0 }, DRACNonClosing},
		{"uncertain direction", func(i *DRACInput) { i.ClosingSpeedBoundsMPS = &DRACBounds{-1, 6} }, DRACUncertainClosingDirection},
		{"zero lower closing bound", func(i *DRACInput) { i.ClosingSpeedBoundsMPS = &DRACBounds{0, 6} }, DRACUncertainClosingDirection},
		{"nominal diverging uncertain", func(i *DRACInput) { *i.LeaderSpeedMPS = 16; i.ClosingSpeedBoundsMPS = &DRACBounds{-2, 1} }, DRACUncertainClosingDirection},
		{"negative forward speed", func(i *DRACInput) { *i.LeaderSpeedMPS = -1 }, DRACInvalidInput},
		{"nan", func(i *DRACInput) { *i.GapM = math.NaN() }, DRACInvalidInput},
		{"infinity", func(i *DRACInput) { *i.FollowerSpeedMPS = math.Inf(1) }, DRACInvalidInput},
		{"reversed bounds", func(i *DRACInput) { i.GapBoundsM = &DRACBounds{20, 0} }, DRACInvalidInput},
		{"nonfinite bounds", func(i *DRACInput) { i.GapBoundsM = &DRACBounds{0, math.Inf(1)} }, DRACInvalidInput},
		{"gap outside bounds", func(i *DRACInput) { i.GapBoundsM = &DRACBounds{20, 30} }, DRACInvalidInput},
		{"closing outside bounds", func(i *DRACInput) { i.ClosingSpeedBoundsMPS = &DRACBounds{1, 2} }, DRACInvalidInput},
		{"unrepresentable demand", func(i *DRACInput) { *i.GapM = 1e-300; *i.FollowerSpeedMPS = 1e300 }, DRACInvalidInput},
		{"underflow demand", func(i *DRACInput) { *i.GapM = 1e300; *i.FollowerSpeedMPS = 1e-300; *i.LeaderSpeedMPS = 0 }, DRACInvalidInput},
	} {
		t.Run(tt.name, func(t *testing.T) {
			i := dracInput(10, 15, 10)
			tt.edit(&i)
			r := EvaluateDRAC(i)
			if r.Status != tt.want || r.ValueMPS2 != nil || r.BoundsMPS2 != nil {
				t.Fatalf("got %+v want %s", r, tt.want)
			}
			if _, e := json.Marshal(r); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestDRACBounds(t *testing.T) {
	i := dracInput(10, 15, 10)
	i.GapBoundsM = &DRACBounds{9, 11}
	i.ClosingSpeedBoundsMPS = &DRACBounds{4, 6}
	r := EvaluateDRAC(i)
	if r.Status != DRACSupported || r.BoundsMPS2 == nil {
		t.Fatalf("%+v", r)
	}
	if math.Abs(r.BoundsMPS2.Lower-16.0/22) > 1e-14 || math.Abs(r.BoundsMPS2.Upper-2) > 1e-14 {
		t.Fatalf("%+v", r.BoundsMPS2)
	}
	for _, g := range []float64{9, 10, 11} {
		for _, c := range []float64{4, 5, 6} {
			v := c * c / (2 * g)
			if v < r.BoundsMPS2.Lower-1e-14 || v > r.BoundsMPS2.Upper+1e-14 {
				t.Fatalf("bound excludes %g", v)
			}
		}
	}
	i.ClosingSpeedBoundsMPS = nil
	if r := EvaluateDRAC(i); r.Status != DRACSupported || r.BoundsMPS2 != nil {
		t.Fatalf("partial uncertainty invented: %+v", r)
	}
	i.ClosingSpeedBoundsMPS = &DRACBounds{1, math.MaxFloat64}
	if r := EvaluateDRAC(i); r.Status != DRACInvalidInput {
		t.Fatalf("overflowed bound: %+v", r)
	}
}
