package l8behaviour

import "math"

// DRACStatus records applicability independently of headway eligibility.
type DRACStatus string

const (
	DRACSupported                 DRACStatus = "supported"
	DRACInvalidInput              DRACStatus = "invalid_input"
	DRACMissingInput              DRACStatus = "missing_input"
	DRACMissingSupport            DRACStatus = "missing_support"
	DRACIncompatiblePath          DRACStatus = "incompatible_path"
	DRACUnresolvedPositiveGap     DRACStatus = "unresolved_positive_gap"
	DRACNonClosing                DRACStatus = "non_closing"
	DRACUncertainClosingDirection DRACStatus = "uncertain_closing_direction"
)

// DRACBounds is a declared inclusive deterministic interval. It is not a sigma
// or a probabilistic confidence interval.
type DRACBounds struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

// DRACInput contains same-instant speeds projected along the shared forward path
// and the supported bumper gap. Nil values are unavailable, never zero. Observed
// requires observed support for both bodies and the gap; SamePath requires an
// established compatible following geometry. These declarations are supplied by
// the evidence adapter, not inferred here from distance or heading alone.
type DRACInput struct {
	GapM                  *float64
	FollowerSpeedMPS      *float64
	LeaderSpeedMPS        *float64
	Observed              bool
	SamePath              bool
	GapBoundsM            *DRACBounds
	ClosingSpeedBoundsMPS *DRACBounds
}

// DRACResult is experimental counterfactual deceleration demand in m/s², assuming
// constant leader speed and immediate uniform follower braking. It is not observed
// braking or collision probability. A suppressed value is nil. BoundsMPS2 is
// present only when both input intervals are supplied: absent uncertainty is not
// zero uncertainty. The result alone makes no physical qualification claim.
type DRACResult struct {
	Status          DRACStatus  `json:"status"`
	ValueMPS2       *float64    `json:"value_mps2,omitempty"`
	ClosingSpeedMPS *float64    `json:"closing_speed_mps,omitempty"`
	BoundsMPS2      *DRACBounds `json:"bounds_mps2,omitempty"`
}

// EvaluateDRAC evaluates c²/(2g), with c=vFollower-vLeader. It applies no
// headway speed floor, ceiling, or near-zero gap clamp. Non-closing pairs are
// explicitly inapplicable rather than assigned zero demand. Declared intervals
// must contain their nominal value; a positive gap and closing direction must
// remain resolved over every supplied interval. When both intervals are present,
// monotonic extrema give conservative bounds without assuming independence.
func EvaluateDRAC(in DRACInput) DRACResult {
	r := DRACResult{Status: DRACMissingInput}
	for _, v := range []*float64{in.GapM, in.FollowerSpeedMPS, in.LeaderSpeedMPS} {
		if v != nil && !dracFinite(*v) {
			r.Status = DRACInvalidInput
			return r
		}
	}
	for _, b := range []*DRACBounds{in.GapBoundsM, in.ClosingSpeedBoundsMPS} {
		if b != nil && (!dracFinite(b.Lower) || !dracFinite(b.Upper) || b.Lower > b.Upper) {
			r.Status = DRACInvalidInput
			return r
		}
	}
	if in.GapM == nil || in.FollowerSpeedMPS == nil || in.LeaderSpeedMPS == nil {
		return r
	}
	g, vf, vl := *in.GapM, *in.FollowerSpeedMPS, *in.LeaderSpeedMPS
	c := vf - vl
	if vf < 0 || vl < 0 || !dracFinite(c) || !dracContains(in.GapBoundsM, g) || !dracContains(in.ClosingSpeedBoundsMPS, c) {
		r.Status = DRACInvalidInput
		return r
	}
	r.ClosingSpeedMPS = &c
	if !in.Observed {
		r.Status = DRACMissingSupport
		return r
	}
	if !in.SamePath {
		r.Status = DRACIncompatiblePath
		return r
	}
	if g <= 0 || in.GapBoundsM != nil && in.GapBoundsM.Lower <= 0 {
		r.Status = DRACUnresolvedPositiveGap
		return r
	}
	if b := in.ClosingSpeedBoundsMPS; b != nil && b.Lower <= 0 && b.Upper > 0 {
		r.Status = DRACUncertainClosingDirection
		return r
	}
	if c <= 0 {
		r.Status = DRACNonClosing
		return r
	}
	value := dracDemand(g, c)
	if !dracFinite(value) || value == 0 {
		r.Status = DRACInvalidInput
		return r
	}
	if gB, cB := in.GapBoundsM, in.ClosingSpeedBoundsMPS; gB != nil && cB != nil {
		b := DRACBounds{Lower: dracDemand(gB.Upper, cB.Lower), Upper: dracDemand(gB.Lower, cB.Upper)}
		if !dracFinite(b.Lower) || !dracFinite(b.Upper) || b.Lower == 0 {
			r.Status = DRACInvalidInput
			return r
		}
		r.BoundsMPS2 = &b
	}
	r.Status, r.ValueMPS2 = DRACSupported, &value
	return r
}

func dracFinite(v float64) bool                  { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func dracContains(b *DRACBounds, v float64) bool { return b == nil || b.Lower <= v && v <= b.Upper }

// Avoid squaring c or doubling g first: either can overflow even when the final
// answer is representable. The square-root scaling also avoids ratio underflow.
func dracDemand(g, c float64) float64 { scaled := c / math.Sqrt(g); return (scaled / 2) * scaled }
