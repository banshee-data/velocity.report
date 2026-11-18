package annotation

import (
	"math"
	"reflect"
	"testing"
)

func TestJointFacetFeasibilityRejectsEmptyJointIntersections(t *testing.T) {
	prior := FacetPosePrior{XM: fp(0), YM: fp(0), PositionBoundM: fp(10)}
	const side = 1.9
	h := side * math.Sqrt(3) / 2
	disks := []PlanarBound{{XM: -side / 2, YM: -h / 3, BoundM: 1}, {XM: side / 2, YM: -h / 3, BoundM: 1}, {XM: 0, YM: 2 * h / 3, BoundM: 1}}
	for i, a := range disks {
		for _, b := range disks[i+1:] {
			if math.Hypot(a.XM-b.XM, a.YM-b.YM) >= a.BoundM+b.BoundM {
				t.Fatal("fixture disks must overlap pairwise")
			}
		}
	}
	if _, _, ok := jointFeasiblePoint(prior, PlanarBound{}, disks, nil, nil); ok {
		t.Fatal("pairwise overlaps became a non-existent common point")
	}
	strips := []jointNormal{{nx: 1, r: 0, bound: .1}, {nx: 1, r: 1, bound: .1}}
	if _, _, ok := jointFeasiblePoint(prior, PlanarBound{}, nil, strips, nil); ok {
		t.Fatal("wide prior hid conflicting parallel planes")
	}
	// An intersection far outside the prior disk is not supported opportunity.
	strips[1].nx = math.Cos(1e-8)
	strips[1].ny = math.Sin(1e-8)
	if _, _, ok := jointFeasiblePoint(prior, PlanarBound{}, nil, strips, nil); ok {
		t.Fatal("near-parallel intersection outside prior accepted")
	}
}

func TestJointFacetFeasibilityFindsBoundedWitnessWithoutMutatingInputs(t *testing.T) {
	prior := FacetPosePrior{XM: fp(0), YM: fp(0), PositionBoundM: fp(1)}
	for _, tc := range []struct {
		name      string
		preferred PlanarBound
		disks     []PlanarBound
		strips    []jointNormal
		tangent   *[2]float64
	}{
		{name: "preferred", preferred: PlanarBound{XM: .1, YM: .1}},
		{name: "circle-circle", preferred: PlanarBound{XM: 0, YM: 2}, disks: []PlanarBound{{XM: -.5, BoundM: .6}, {XM: .5, BoundM: .6}}},
		{name: "circle-strip", preferred: PlanarBound{XM: 2, YM: 1}, disks: []PlanarBound{{XM: 1, BoundM: .75}}, strips: []jointNormal{{nx: 1, r: .8, bound: 0}}},
		{name: "strip-strip", strips: []jointNormal{{nx: 1, r: .5, bound: .1}, {ny: 1, r: .5, bound: .1}}},
		{name: "fixed tangent", strips: []jointNormal{{nx: 1, r: .5, bound: .1}}, tangent: &[2]float64{0, 1}},
		{name: "nested", preferred: PlanarBound{XM: 3}, disks: []PlanarBound{{BoundM: .2}, {BoundM: .4}}},
		{name: "zero radius", preferred: PlanarBound{XM: 1}, disks: []PlanarBound{{XM: .2, YM: .3, BoundM: 0}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]jointNormal(nil), tc.strips...)
			x, y, ok := jointFeasiblePoint(prior, tc.preferred, tc.disks, tc.strips, tc.tangent)
			if !ok || math.Hypot(x, y) > 1+1e-9 {
				t.Fatal(x, y, ok)
			}
			for _, d := range tc.disks {
				if math.Hypot(x-d.XM, y-d.YM) > d.BoundM+1e-9 {
					t.Fatal("witness outside point bound", x, y, d)
				}
			}
			for _, s := range tc.strips {
				if math.Abs(s.nx*x+s.ny*y-s.r) > s.bound+1e-9 {
					t.Fatal("witness outside plane bound", x, y, s)
				}
			}
			if tc.tangent != nil && math.Abs(tc.tangent[0]*x+tc.tangent[1]*y) > 1e-9 {
				t.Fatal("prior tangent changed")
			}
			if !reflect.DeepEqual(before, tc.strips) {
				t.Fatal("feasibility mutated source strips")
			}
		})
	}
}

func TestJointFacetFeasibilityRefusesInvalidOrIncompatibleBounds(t *testing.T) {
	prior := FacetPosePrior{XM: fp(0), YM: fp(0), PositionBoundM: fp(1)}
	for _, tc := range []struct {
		name    string
		disks   []PlanarBound
		strips  []jointNormal
		tangent *[2]float64
	}{
		{name: "negative disk", disks: []PlanarBound{{BoundM: -1}}},
		{name: "nonfinite disk", disks: []PlanarBound{{XM: math.NaN(), BoundM: 1}}},
		{name: "nonfinite strip", strips: []jointNormal{{nx: 1, r: math.Inf(1), bound: 1}}},
		{name: "non-unit normal", strips: []jointNormal{{nx: 2, bound: 1}}},
		{name: "negative strip", strips: []jointNormal{{nx: 1, bound: -1}}},
		{name: "disjoint prior", disks: []PlanarBound{{XM: 5, BoundM: 1}}},
		{name: "fixed tangent contradiction", strips: []jointNormal{{ny: 1, r: .5, bound: .1}}, tangent: &[2]float64{0, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, ok := jointFeasiblePoint(prior, PlanarBound{}, tc.disks, tc.strips, tc.tangent); ok {
				t.Fatal("accepted invalid or infeasible geometry")
			}
		})
	}
	prior.XM = fp(math.MaxFloat64)
	if _, _, ok := jointFeasiblePoint(prior, PlanarBound{}, []PlanarBound{{XM: -math.MaxFloat64, BoundM: 1}}, nil, nil); ok {
		t.Fatal("numeric overflow accepted")
	}
}

func TestJointFacetFeasibilityDoesNotLetLargeFiniteScalesSwallowSmallBounds(t *testing.T) {
	prior := FacetPosePrior{XM: fp(0), YM: fp(0), PositionBoundM: fp(1e15)}
	strips := []jointNormal{{nx: 1, r: 0, bound: .01}, {nx: 1, r: 1, bound: .01}}
	if _, _, ok := jointFeasiblePoint(prior, PlanarBound{}, nil, strips, nil); ok {
		t.Fatal("normalisation swallowed contradictory centimetre strips")
	}
	disks := []PlanarBound{{XM: 0, BoundM: .01}, {XM: 1, BoundM: .01}}
	if _, _, ok := jointFeasiblePoint(prior, PlanarBound{}, disks, nil, nil); ok {
		t.Fatal("normalisation swallowed contradictory centimetre disks")
	}
	prior.XM, prior.YM, prior.PositionBoundM = fp(1e15), fp(1e15), fp(1)
	if _, _, ok := jointFeasiblePoint(prior, PlanarBound{XM: 1e15, YM: 1e15}, []PlanarBound{{XM: 1e15, YM: 1e15, BoundM: .01}}, nil, nil); ok {
		t.Fatal("coordinate ulps exceed the declared measurement resolution")
	}
}

func TestJointFacetFeasibilityRechecksOriginalUnitsAndHardPrior(t *testing.T) {
	prior := FacetPosePrior{XM: fp(0), YM: fp(0), PositionBoundM: fp(1)}
	for _, tc := range []struct {
		name      string
		preferred PlanarBound
		disks     []PlanarBound
		strips    []jointNormal
	}{
		{name: "representable disk gap", preferred: PlanarBound{XM: .1}, disks: []PlanarBound{{BoundM: .1}, {XM: .2 + 1e-14, BoundM: .1}}},
		{name: "representable strip gap", preferred: PlanarBound{XM: .1}, strips: []jointNormal{{nx: 1, bound: .1}, {nx: 1, r: .2 + 1e-14, bound: .1}}},
		{name: "nonfinite preferred", preferred: PlanarBound{XM: math.NaN()}},
		{name: "circle misses strip", disks: []PlanarBound{{BoundM: .1}}, strips: []jointNormal{{nx: 1, r: .9, bound: .05}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if x, y, ok := jointFeasiblePoint(prior, tc.preferred, tc.disks, tc.strips, nil); ok {
				t.Fatal("infeasible witness", x, y)
			}
		})
	}
	x, y, ok := jointFeasiblePoint(prior, PlanarBound{XM: .1, YM: 1e-14}, nil, nil, &[2]float64{0, 1})
	if !ok || y != 0 {
		t.Fatal("tiny tangent drift accepted", x, y, ok)
	}
	prior.PositionBoundM = fp(0)
	x, y, ok = jointFeasiblePoint(prior, PlanarBound{XM: 1e-16}, []PlanarBound{{BoundM: .1}}, nil, nil)
	if !ok || x != 0 || y != 0 {
		t.Fatal("hard zero moved", x, y, ok)
	}
	if _, _, ok = jointFeasiblePoint(prior, PlanarBound{}, []PlanarBound{{XM: 1, BoundM: .1}}, nil, nil); ok {
		t.Fatal("incompatible hard zero accepted")
	}
	prior.XM, prior.PositionBoundM = fp(1e308), fp(1e308)
	if _, _, ok = jointFeasiblePoint(prior, PlanarBound{XM: -1e308}, []PlanarBound{{XM: 1e308, YM: 1e308, BoundM: 1e308}}, nil, nil); ok {
		t.Fatal("unrepresentable coordinate transform accepted")
	}
}
