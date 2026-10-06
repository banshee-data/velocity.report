package l8behaviour

// Clipping a stored following encounter to a time window, for surfaces that
// serve one window of a capture (a scene). Review R4 of the 0.5.2 sprint
// review: a window that keeps only encounters lying wholly inside it loses
// every encounter that crosses either edge, including one longer than the
// window, and source discovery then reports no encounters although stored
// data covers the window.
//
// An instant belongs to a window [start, end) when its capture time does:
// the left endpoint of the interval it stands for, as the whole-encounter
// containment already placed instants. The window is half-open so that
// windows partitioning a capture partition its instants, and so its valid
// time, exactly. This keeps whole instants rather than intersecting their
// intervals with the window (review R4's wording): the time an instant
// stands for may run up to one interval past the window's end, and an
// instant before its start whose interval reaches into it is left out.
// Each edge's error is under one frame interval, and the two edges' errors
// cancel on average. Trimming intervals would need an instant before the
// window, or an invented one, for the start.
// The clipped record is rebuilt from the instants that remain, with nothing
// carried over from the untrimmed encounter:
//
//   - the event's interval, accounting and worst support, and its exposure
//     windows, by the rules Validate checks them with;
//   - every measurement, by running the encounter statistics and
//     measurements again over those instants, with the encounter's own
//     Monte Carlo seed, parameters and class decision.
//
// A clipped piece is an encounter in its own right for its measurements: one
// with less valid time than the minimum opportunity has its band durations
// and rates suppressed as insufficient_observation, and a distribution then
// counts its time under that reason rather than in its bins. That time stays
// in the denominator, so nothing is lost silently, but a short piece at an
// edge is labelled by its length in the window, not by how well it was
// observed.
//
// The recomputation needs the parameters the encounter was analysed with,
// and the stored record names them only by hash. So it is attempted only
// when the caller's parameters hash to the stored method id, and only after
// recomputing over all of the encounter's instants reproduces its stored
// measurements exactly. An encounter that fails either check is not clipped:
// it is reported, and the caller leaves it out rather than serving a
// minimum or interval that belongs to time outside the window.

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// ClipOutcome says what ClipFollowingInteraction did with an encounter.
type ClipOutcome uint8

const (
	// ClipContained: every instant lies inside the window; the encounter is
	// returned unchanged.
	ClipContained ClipOutcome = iota + 1
	// ClipClipped: some instants lie inside; the encounter is rebuilt from
	// them.
	ClipClipped
	// ClipEmpty: no instant lies inside the window; nothing is returned.
	ClipEmpty
	// ClipUnrecomputable: some instants lie inside, but the encounter's
	// measurements cannot be recomputed for them; nothing is returned.
	ClipUnrecomputable
)

// ClipFollowingInteraction returns the part of fi whose instants lie in
// [startUnixNanos, endUnixNanos), rebuilt as the module comment describes. params must be the parameters fi was analysed with.
// The returned detail explains ClipUnrecomputable. An error is returned for
// an invalid encounter or window, not for one that cannot be clipped.
func ClipFollowingInteraction(fi FollowingInteraction, startUnixNanos, endUnixNanos int64,
	params FollowingAnalysisParams) (FollowingInteraction, ClipOutcome, string, error) {
	if startUnixNanos <= 0 || endUnixNanos <= startUnixNanos {
		return FollowingInteraction{}, 0, "", fmt.Errorf("window %d to %d is not ordered", startUnixNanos, endUnixNanos)
	}
	if err := fi.Validate(); err != nil {
		return FollowingInteraction{}, 0, "", err
	}
	var kept []InteractionInstant
	for _, in := range fi.Instants {
		if in.CaptureUnixNanos >= startUnixNanos && in.CaptureUnixNanos < endUnixNanos {
			kept = append(kept, in)
		}
	}
	switch len(kept) {
	case len(fi.Instants):
		return fi, ClipContained, "", nil
	case 0:
		return FollowingInteraction{}, ClipEmpty, "", nil
	}

	ev := fi.Event
	paramsHash := params.Hash()
	if ev.Version.MethodID != FollowingEncounterMethodID+"/"+paramsHash {
		return FollowingInteraction{}, ClipUnrecomputable,
			fmt.Sprintf("analysed under method %s, not with the given parameters (%s)", ev.Version.MethodID, paramsHash), nil
	}
	whole, err := recomputeEncounter(ev, fi.Instants, params, paramsHash)
	if err != nil {
		return FollowingInteraction{}, 0, "", err
	}
	if detail := sameRecord(whole, ev); detail != "" {
		return FollowingInteraction{}, ClipUnrecomputable, "recomputing the whole encounter does not reproduce it: " + detail, nil
	}

	clipped, err := recomputeEncounter(ev, kept, params, paramsHash)
	if err != nil {
		return FollowingInteraction{}, 0, "", err
	}
	out := FollowingInteraction{Event: clipped, Instants: kept, Windows: deriveWindows(clipped, kept)}
	if err := out.Validate(); err != nil {
		return FollowingInteraction{}, 0, "", fmt.Errorf("clipped encounter %s: %w", ev.EventID, err)
	}
	return out, ClipClipped, "", nil
}

// recomputeEncounter rebuilds ev over instants: its interval, accounting,
// worst support, input provenance and measurements, as buildEncounter
// derives them. The version, and so the event id, is ev's own: the stage is
// a minimum over samples the stored rows do not keep, and a clipped encounter
// is the same encounter at the same version.
func recomputeEncounter(ev InteractionEvent, instants []InteractionInstant, params FollowingAnalysisParams,
	paramsHash string) (InteractionEvent, error) {
	out := ev
	out.StartUnixNanos = instants[0].CaptureUnixNanos
	out.EndUnixNanos = instants[len(instants)-1].CaptureUnixNanos
	out.Accounting = instantAccounting(instants)

	var worst SupportState
	var observed, coasted int
	var mc []mcInstant
	for _, in := range instants {
		worst = WorseSupport(worst, WorseSupport(in.FollowerSupport, in.LeaderSupport))
		counted := in.IntervalNanos
		if in.RecordGap {
			worst = WorseSupport(worst, SupportMissedUnknown)
			counted = 0
		}
		var x mcInstant
		if v, ok := in.Values[MetricFollowingNetTimeGap]; ok && in.Valid {
			x.thw, x.hasThw, x.nanos = SeriesPoint{in.CaptureUnixNanos, v.Value, v.Sigma}, true, counted
		}
		if v, ok := in.Values[MetricFollowingSpatialGap]; ok {
			x.gap, x.hasGap = SeriesPoint{in.CaptureUnixNanos, v.Value, v.Sigma}, true
		}
		if evaluated(in) {
			for _, sup := range []SupportState{in.FollowerSupport, in.LeaderSupport} {
				if sup == SupportObserved {
					observed++
				} else {
					coasted++
				}
			}
		}
		if x.hasGap || x.hasThw {
			mc = append(mc, x)
		}
	}
	out.WorstSupport = worst

	leaderID, followerID := ev.SecondaryTrackID, ev.PrimaryTrackID
	out.Input = InputProvenance{
		ContributingTrackIDs: []string{leaderID, followerID},
		FirstUnixNanos:       out.StartUnixNanos, LastUnixNanos: out.EndUnixNanos,
		ObservedFrames: observed, CoastedFrames: coasted, PlanarFallback: true,
	}
	prov := Provenance{Version: ev.Version, Input: out.Input}
	stats := encounterStatistics(mc, out.EncounterAccounting(), params.Exposure,
		monteCarloSeed(FollowingEncounterMethodID, paramsHash, ev.Version.GeometryID, leaderID, followerID))
	classReason := storedClassReason(ev)
	notFinal := ev.Version.EstimateStage != StageFinal
	ms, err := encounterMeasurements(stats, classReason, notFinal, params.Exposure, prov)
	if err != nil {
		return InteractionEvent{}, err
	}
	if out.Measurements, err = keyMeasurements(ms); err != nil {
		return InteractionEvent{}, err
	}
	out.Provisional = nil
	if notFinal {
		ps, err := encounterMeasurements(stats, classReason, false, params.Exposure, prov)
		if err != nil {
			return InteractionEvent{}, err
		}
		if out.Provisional, err = keyMeasurements(ps); err != nil {
			return InteractionEvent{}, err
		}
	}
	return out, nil
}

// evaluated reports whether the pair was evaluated at an instant, which is
// when buildEncounter counts its parties' frames: this leader was the one
// chosen, and it had a sample there. The stored instant keeps the point's
// reasons exactly when a point exists, and a point that is not valid always
// has one, so a valid instant or one with reasons was evaluated. Support
// alone cannot tell: a chosen leader without a sample is missed_unknown, but
// so can a leader sample be, and that one was evaluated.
func evaluated(in InteractionInstant) bool {
	return in.Role == DispositionLeader && (in.Valid || len(in.Reasons) > 0)
}

// storedClassReason recovers the class decision the encounter's measurements
// were made under. The motion classes are not stored, but class is the most
// fundamental reason, so a class the rule does not support suppresses every
// measurement with it; the provisional block shows it on a non-final
// encounter, whose production block is suppressed by stage.
func storedClassReason(ev InteractionEvent) SuppressionReason {
	block := ev.Measurements
	if ev.Version.EstimateStage != StageFinal {
		block = ev.Provisional
	}
	for _, m := range block {
		if m.Suppressed && m.Reason == ReasonClassNotSupported {
			return ReasonClassNotSupported
		}
	}
	return ReasonUnspecified
}

// sameRecord compares a recomputed event with the stored one, field by field,
// as encoded; it returns which part differs, or "" when none does. Every
// value must match exactly except an interval's lower and upper bound, which
// may differ in their last bits: the Monte Carlo draws compute value plus
// sigma times a deviate, and Go may fuse that into one multiply-add on arm64
// but not on amd64, so an encounter analysed on one machine and served from
// another would otherwise never reproduce.
func sameRecord(got, want InteractionEvent) string {
	var diffs []string
	for _, part := range []struct {
		name string
		a, b any
	}{
		{"interval", [2]int64{got.StartUnixNanos, got.EndUnixNanos}, [2]int64{want.StartUnixNanos, want.EndUnixNanos}},
		{"accounting", got.Accounting, want.Accounting},
		{"worst support", got.WorstSupport, want.WorstSupport},
		{"input provenance", got.Input, want.Input},
		{"measurements", got.Measurements, want.Measurements},
		{"provisional measurements", got.Provisional, want.Provisional},
	} {
		if !sameEncoded(part.a, part.b) {
			diffs = append(diffs, part.name)
		}
	}
	return strings.Join(diffs, ", ")
}

// intervalBoundTolerance is the relative difference allowed between an
// interval bound recomputed here and the one stored: a few units in the last
// place of a float64, with room for the sum of a few such differences.
const intervalBoundTolerance = 1e-12

// sameEncoded compares two values by their JSON encoding: exactly, except
// that numbers under a "lower" or "upper" key may differ by
// intervalBoundTolerance.
func sameEncoded(a, b any) bool {
	ra, errA := json.Marshal(a)
	rb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	if string(ra) == string(rb) {
		return true
	}
	var va, vb any
	if json.Unmarshal(ra, &va) != nil || json.Unmarshal(rb, &vb) != nil {
		return false
	}
	return sameJSON(va, vb, "")
}

func sameJSON(a, b any, key string) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !sameJSON(v, w, k) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !sameJSON(x[i], y[i], key) {
				return false
			}
		}
		return true
	case float64:
		y, ok := b.(float64)
		if !ok {
			return false
		}
		if x == y {
			return true
		}
		if key != "lower" && key != "upper" {
			return false
		}
		return math.Abs(x-y) <= intervalBoundTolerance*math.Max(math.Abs(x), math.Abs(y))
	default:
		return a == b
	}
}
