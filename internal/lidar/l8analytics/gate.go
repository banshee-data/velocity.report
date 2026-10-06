package l8analytics

import (
	"fmt"
	"math"
)

// The association gate, as a value rather than a bare number.
//
// Why a type: the D2 A/B (lidar-state-estimation-plan.md §21.1) scored an
// estimate against an annotated object with a gate of one metre plus half the
// object's own horizontal footprint diagonal, so that a bus tolerates more
// centre drift than a pedestrian: a partial view of a long body moves its
// centre further than a partial view of a short one. A float cannot say which
// rule a number was scored under, and two arms scored under different rules
// are not a comparison. The fixed gate every existing caller passes is one
// case of this type and scores exactly as it always has: FixedGate ignores the
// footprint entirely.
//
// The gate belongs to the reference point, never the hypothesis. The object's
// extent is what a human certified; a tracker's box is what is being judged,
// and letting it widen its own gate would let a bloated box score better.

// GateKind names the rule a MatchGate applies.
type GateKind string

const (
	// GateFixed matches within Metres of the reference point, whatever its
	// size. Every caller before the footprint gate used this.
	GateFixed GateKind = "fixed"
	// GateFootprint matches within Metres plus half the reference point's
	// footprint diagonal. A point with no footprint gets Metres alone.
	GateFootprint GateKind = "footprint"
)

// MatchGate is the association gate for the per-frame matcher, and for HOTA the
// distance at which similarity reaches zero.
type MatchGate struct {
	Kind GateKind `json:"kind"`
	// Metres is the gate for GateFixed, and the slack added to half the
	// footprint diagonal for GateFootprint.
	Metres float64 `json:"metres"`
}

// FixedGate is the gate ComputeTrackMetrics and ComputeHOTA have always used.
func FixedGate(metres float64) MatchGate { return MatchGate{Kind: GateFixed, Metres: metres} }

// FootprintGate is the D2 A/B gate: slackMetres plus half the reference
// object's footprint diagonal. The A/B used one metre of slack.
func FootprintGate(slackMetres float64) MatchGate {
	return MatchGate{Kind: GateFootprint, Metres: slackMetres}
}

// Validate refuses a gate that cannot match anything or names no rule. The
// metric functions do not call it, so their long-standing behaviour on a zero
// gate (nothing matches) is unchanged; callers that take a gate from a user
// should.
func (g MatchGate) Validate() error {
	if math.IsNaN(g.Metres) || math.IsInf(g.Metres, 0) {
		return fmt.Errorf("gate metres %v is not finite", g.Metres)
	}
	switch g.Kind {
	case GateFixed:
		if g.Metres <= 0 {
			return fmt.Errorf("fixed gate must be positive, got %v m", g.Metres)
		}
	case GateFootprint:
		if g.Metres < 0 {
			return fmt.Errorf("footprint gate slack must not be negative, got %v m", g.Metres)
		}
	default:
		return fmt.Errorf("unknown gate kind %q (want %q or %q)", g.Kind, GateFixed, GateFootprint)
	}
	return nil
}

// String is the form a report prints, so the rule is legible beside the number.
func (g MatchGate) String() string {
	if g.Kind == GateFootprint {
		return fmt.Sprintf("%g m + half footprint diagonal", g.Metres)
	}
	return fmt.Sprintf("%g m fixed", g.Metres)
}

// forReference is the gate for one reference point.
func (g MatchGate) forReference(e seriesEntry) float64 {
	if g.Kind == GateFootprint {
		return g.Metres + 0.5*float64(e.footprintDiag)
	}
	return g.Metres
}
