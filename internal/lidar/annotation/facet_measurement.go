package annotation

import (
	"fmt"
	"math"
)

// Bounds as measurements.
//
// A FacetPoseProposal states worst-case half-widths: the true value lies
// within the bound of the reported one, under the solver's assumptions. The
// tracker's update takes Gaussian variances. A proposal that enters the
// tracked state must cross from one to the other by a declared rule, or the
// facet arm looks confident by construction (face-state maths review,
// Section 3.6; facet registration plan, Section 5, "Uncertainty"). This file
// is that crossing, declared once:
//
//   - A worst-case half-width b becomes a standard deviation by one named rule.
//     The default, BoundToSigmaUniform, reads the error as uniform within the
//     bound: sigma = b/sqrt(3). BoundToSigmaThreeSigma reads the bound as a
//     Gaussian's three sigma: sigma = b/3, a narrower and more confident
//     measurement, selectable but never the default.
//   - A direction the proposal lists as unresolved produces no measurement in
//     that direction, under either rule. A segment's tangent position is the
//     prior's, and a compact spot's yaw is the prior's; reporting either as a
//     Gaussian sigma would feed the prior back as if it were evidence, which
//     the facet plan's uncertainty row fails.
//   - Consumers that truncate rather than update take FacetIntervals: the
//     bound itself, as a set-membership interval along a stated direction,
//     the form the tracker's truncateAxis applies.
//
// What the conversion does not carry, named so a consumer does not assume it:
//
//   - The correlation between a segment's normal offset and its yaw. The
//     normal bound already holds the yaw error's lever-arm term at its worst
//     case, so using the two as independent measurements is conservative in
//     the normal direction, not exact.
//   - The prior's own information. A compact spot is conditioned on the prior
//     yaw, and its position bound is floored at the prior's position bound by
//     the solver. If the prior came from the tracked state, the measurement
//     is not independent of it, and updating with it counts the prior twice.
//     The consumer that chooses the prior owns that.
//   - The pi ambiguity of a segment's yaw. A line normal has no sign, so the
//     solver folds the yaw to the mode nearest the prior; FacetYawMeasurement
//     says so with ModPi, and a consumer must fold its own heading the same
//     way before forming an innovation.

// BoundToSigma names the rule that turns a worst-case half-width into a
// Gaussian standard deviation.
type BoundToSigma string

const (
	// BoundToSigmaUniform reads the error as uniform within the bound:
	// sigma = b/sqrt(3). It is the default because it is the wider of the two.
	BoundToSigmaUniform BoundToSigma = "uniform_sqrt3"
	// BoundToSigmaThreeSigma reads the bound as three standard deviations of
	// a Gaussian: sigma = b/3.
	BoundToSigmaThreeSigma BoundToSigma = "three_sigma"
	// DefaultBoundToSigma is the rule used when none is named.
	DefaultBoundToSigma = BoundToSigmaUniform
)

// Directions a proposal can list as unresolved: the solver's vocabulary, held
// here so the conversion and the solver read the same words.
const (
	UnresolvedYawFromPrior     = "yaw_from_prior_not_measured"
	UnresolvedTangentFromPrior = "tangent_from_prior_not_measured"
	UnresolvedHeight           = "height_not_measured"
)

// Sigma converts a worst-case half-width to a standard deviation under the
// rule. An empty rule is the default. A bound that is not finite and positive
// is refused: zero would read as an exact measurement.
func (r BoundToSigma) Sigma(bound float64) (float64, error) {
	if err := checkFacetBound(bound); err != nil {
		return 0, err
	}
	switch r {
	case "", BoundToSigmaUniform:
		return bound / math.Sqrt(3), nil
	case BoundToSigmaThreeSigma:
		return bound / 3, nil
	}
	return 0, fmt.Errorf("unknown bound-to-sigma rule %q: want %q or %q", r, BoundToSigmaUniform, BoundToSigmaThreeSigma)
}

// resolved is the rule a measurement records: the default when none was named.
func (r BoundToSigma) resolved() BoundToSigma {
	if r == "" {
		return DefaultBoundToSigma
	}
	return r
}

func checkFacetBound(bound float64) error {
	if math.IsNaN(bound) || math.IsInf(bound, 0) || bound <= 0 {
		return fmt.Errorf("bound %g: want a finite positive half-width", bound)
	}
	return nil
}

// FacetMeasurement is a proposal as a tracker update reads it: one
// measurement per direction the proposal measured, and none for a direction
// it did not. Exactly one of Position and Normal is set, by the constraint.
type FacetMeasurement struct {
	Constraint string `json:"constraint"`
	// Rule is the bound-to-sigma rule every variance here was made with.
	Rule BoundToSigma `json:"bound_to_sigma"`
	// Position is a compact spot's planar body centre.
	Position *FacetPlanarMeasurement `json:"position,omitempty"`
	// Normal is a segment's body-centre position along its normal.
	Normal *FacetNormalMeasurement `json:"normal,omitempty"`
	// Yaw is a segment's measured yaw; nil when the yaw is the prior's.
	Yaw *FacetYawMeasurement `json:"yaw,omitempty"`
	// Unmeasured is the proposal's unresolved list, verbatim: the directions
	// with no measurement here.
	Unmeasured []string `json:"unmeasured"`
}

// FacetPlanarMeasurement is a body-centre position measurement, z = (x, y),
// with an isotropic per-axis variance. The proposal's bound is a radius; the
// disc it bounds lies inside the square of that half-width on each axis, so
// each axis takes the bound as its half-width. Under the uniform rule that is
// b/sqrt(3) per axis, wider than the disc's own b/2.
type FacetPlanarMeasurement struct {
	XM         float64 `json:"x_m"`
	YM         float64 `json:"y_m"`
	BoundM     float64 `json:"bound_m"`
	SigmaM     float64 `json:"sigma_m"`
	VarianceM2 float64 `json:"variance_m2"`
}

// FacetNormalMeasurement is a scalar measurement of the body centre along a
// unit normal: z = n . c, H = [n_x, n_y, 0, 0] on the [x, y, vx, vy] state.
// OffsetM is the measured n . c.
type FacetNormalMeasurement struct {
	Normal     [2]float64 `json:"normal"`
	OffsetM    float64    `json:"offset_m"`
	BoundM     float64    `json:"bound_m"`
	SigmaM     float64    `json:"sigma_m"`
	VarianceM2 float64    `json:"variance_m2"`
}

// FacetYawMeasurement is a measured body yaw. ModPi is true because a line
// normal carries no sign: the value is the mode nearest the proposal's prior.
type FacetYawMeasurement struct {
	Rad          float64 `json:"rad"`
	BoundRad     float64 `json:"bound_rad"`
	SigmaRad     float64 `json:"sigma_rad"`
	VarianceRad2 float64 `json:"variance_rad2"`
	ModPi        bool    `json:"mod_pi"`
}

// Quantities a FacetInterval bounds.
const (
	FacetIntervalPositionX      = "position_x"
	FacetIntervalPositionY      = "position_y"
	FacetIntervalNormalPosition = "normal_position"
	FacetIntervalYaw            = "yaw"
)

// FacetInterval is one measured direction as a set-membership constraint:
// the true value lies in [Lower, Upper], which is the reported value plus or
// minus the proposal's bound, unchanged by any sigma rule. For a position,
// the bounded quantity is Direction . c for the body centre c, the projection
// truncateAxis takes (it works relative to the state's projection, so a
// consumer subtracts Direction . state first). For a yaw, Direction is nil.
type FacetInterval struct {
	Quantity  string      `json:"quantity"`
	Direction *[2]float64 `json:"direction,omitempty"`
	Unit      string      `json:"unit"`
	Lower     float64     `json:"lower"`
	Upper     float64     `json:"upper"`
	HalfWidth float64     `json:"half_width"`
}

// facetEvidence is what a proposal measured, read and checked once for both
// the Gaussian and the interval forms, so the two cannot disagree on which
// directions exist.
type facetEvidence struct {
	constraint string
	position   *PlanarBound
	normal     *[2]float64
	offset     float64
	normalB    float64
	yaw        *AngleBound
	unmeasured []string
}

// readFacetEvidence decides which directions a proposal measured. It refuses
// a proposal whose constraint, yaw status and unresolved list disagree,
// rather than guess which of them is right.
func readFacetEvidence(p *FacetPoseProposal) (facetEvidence, error) {
	var e facetEvidence
	if p == nil {
		return e, fmt.Errorf("no proposal")
	}
	unresolved := map[string]bool{}
	for _, u := range p.Unresolved {
		switch u {
		case UnresolvedYawFromPrior, UnresolvedTangentFromPrior, UnresolvedHeight:
			unresolved[u] = true
		default:
			// A direction this rule does not know might be one it would
			// otherwise report as measured.
			return e, fmt.Errorf("proposal lists an unresolved direction %q the measurement rule does not know", u)
		}
	}
	e.constraint = p.Constraint
	e.unmeasured = append([]string{}, p.Unresolved...)

	yawMeasured := p.Yaw.Status == EvidenceObserved
	if yawMeasured == unresolved[UnresolvedYawFromPrior] {
		return e, fmt.Errorf("proposal yaw status %q disagrees with its unresolved list %v", p.Yaw.Status, p.Unresolved)
	}
	if yawMeasured {
		if !finiteFeature(p.Yaw.Rad) {
			return e, fmt.Errorf("measured yaw is not finite")
		}
		if err := checkFacetBound(p.Yaw.BoundRad); err != nil {
			return e, fmt.Errorf("yaw: %w", err)
		}
		y := p.Yaw
		e.yaw = &y
	}

	switch p.Constraint {
	case ConstraintPlanarPositionGivenPriorYaw:
		if yawMeasured {
			return e, fmt.Errorf("a compact spot measures no yaw, but the proposal reports one")
		}
		if unresolved[UnresolvedTangentFromPrior] {
			return e, fmt.Errorf("a compact spot fixes both position axes, but the proposal lists the tangent unresolved")
		}
		if !finiteFeature(p.Centre.XM) || !finiteFeature(p.Centre.YM) {
			return e, fmt.Errorf("compact spot centre is not finite")
		}
		if err := checkFacetBound(p.Centre.BoundM); err != nil {
			return e, fmt.Errorf("position: %w", err)
		}
		c := p.Centre
		e.position = &c
	case ConstraintNormalPositionAndYaw:
		if !unresolved[UnresolvedTangentFromPrior] {
			// The solver leaves the tangent at the prior; a proposal that does
			// not say so is not one this rule can trust to omit it.
			return e, fmt.Errorf("a segment measures no tangent position, but the proposal does not list it unresolved")
		}
		if p.Normal == nil || p.NormalBoundM == nil {
			return e, fmt.Errorf("segment proposal lacks its normal or normal bound")
		}
		n := *p.Normal
		if norm := math.Hypot(n[0], n[1]); !finiteFeature(norm) || math.Abs(norm-1) > 1e-6 {
			return e, fmt.Errorf("segment normal is not a unit vector")
		}
		if !finiteFeature(p.Centre.XM) || !finiteFeature(p.Centre.YM) {
			return e, fmt.Errorf("segment centre is not finite")
		}
		if err := checkFacetBound(*p.NormalBoundM); err != nil {
			return e, fmt.Errorf("normal: %w", err)
		}
		// The centre is the prior moved along the normal, so n . centre is the
		// measured offset; the tangent component it carries is the prior's and
		// is not read.
		e.normal, e.offset, e.normalB = &n, n[0]*p.Centre.XM+n[1]*p.Centre.YM, *p.NormalBoundM
	default:
		return e, fmt.Errorf("unknown facet constraint %q", p.Constraint)
	}
	return e, nil
}

// FacetMeasurementFromProposal converts a proposal into tracker-ready
// Gaussian measurements under one bound-to-sigma rule (empty: the default,
// BoundToSigmaUniform). Unresolved directions produce no measurement.
func FacetMeasurementFromProposal(p *FacetPoseProposal, rule BoundToSigma) (*FacetMeasurement, error) {
	if _, err := rule.Sigma(1); err != nil {
		return nil, err
	}
	e, err := readFacetEvidence(p)
	if err != nil {
		return nil, err
	}
	m := &FacetMeasurement{Constraint: e.constraint, Rule: rule.resolved(), Unmeasured: e.unmeasured}
	if e.position != nil {
		s, err := rule.Sigma(e.position.BoundM)
		if err != nil {
			return nil, err
		}
		m.Position = &FacetPlanarMeasurement{XM: e.position.XM, YM: e.position.YM, BoundM: e.position.BoundM, SigmaM: s, VarianceM2: s * s}
	}
	if e.normal != nil {
		s, err := rule.Sigma(e.normalB)
		if err != nil {
			return nil, err
		}
		m.Normal = &FacetNormalMeasurement{Normal: *e.normal, OffsetM: e.offset, BoundM: e.normalB, SigmaM: s, VarianceM2: s * s}
	}
	if e.yaw != nil {
		s, err := rule.Sigma(e.yaw.BoundRad)
		if err != nil {
			return nil, err
		}
		m.Yaw = &FacetYawMeasurement{Rad: e.yaw.Rad, BoundRad: e.yaw.BoundRad, SigmaRad: s, VarianceRad2: s * s, ModPi: true}
	}
	return m, nil
}

// FacetIntervals is the set-membership form of a proposal: one interval per
// measured direction, each the reported value plus or minus its bound. A
// compact spot's disc becomes its two axis intervals, the disc's exact
// projections; truncating both keeps the square that contains the disc, a
// relaxation rather than a tightening. Unresolved directions have none.
func FacetIntervals(p *FacetPoseProposal) ([]FacetInterval, error) {
	e, err := readFacetEvidence(p)
	if err != nil {
		return nil, err
	}
	interval := func(q string, dir *[2]float64, unit string, value, bound float64) FacetInterval {
		return FacetInterval{Quantity: q, Direction: dir, Unit: unit, Lower: value - bound, Upper: value + bound, HalfWidth: bound}
	}
	var out []FacetInterval
	if e.position != nil {
		out = append(out,
			interval(FacetIntervalPositionX, &[2]float64{1, 0}, "m", e.position.XM, e.position.BoundM),
			interval(FacetIntervalPositionY, &[2]float64{0, 1}, "m", e.position.YM, e.position.BoundM))
	}
	if e.normal != nil {
		n := *e.normal
		out = append(out, interval(FacetIntervalNormalPosition, &n, "m", e.offset, e.normalB))
	}
	if e.yaw != nil {
		out = append(out, interval(FacetIntervalYaw, nil, "rad", e.yaw.Rad, e.yaw.BoundRad))
	}
	return out, nil
}
