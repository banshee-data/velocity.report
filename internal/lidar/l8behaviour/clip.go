package l8behaviour

// Clipping a stored following encounter to a time window, for surfaces that
// serve one window of a capture (a scene). Review R4 of the 0.5.2 sprint
// review: a window that keeps only encounters lying wholly inside it loses
// every encounter that crosses either edge, including one longer than the
// window, and source discovery then reports no encounters although stored
// data covers the window.
//
// An instant belongs to a window when its capture time does, the rule the
// whole-encounter containment already used: an event spans its first to its
// last instant's capture time, and a window holds it when both lie inside.
// The clipped record is rebuilt from the instants that remain, with nothing
// carried over from the untrimmed encounter:
//
//   - the event's interval, accounting and worst support, and its exposure
//     windows, by the rules Validate checks them with;
//   - every measurement, by running the encounter statistics and
//     measurements again over those instants, with the encounter's own
//     Monte Carlo seed, parameters and class decision.
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
// [startUnixNanos, endUnixNanos], both inclusive, rebuilt as the module
// comment describes. params must be the parameters fi was analysed with.
// The returned detail explains ClipUnrecomputable. An error is returned for
// an invalid encounter or window, not for one that cannot be clipped.
func ClipFollowingInteraction(fi FollowingInteraction, startUnixNanos, endUnixNanos int64,
	params FollowingAnalysisParams) (FollowingInteraction, ClipOutcome, string, error) {
	if startUnixNanos <= 0 || endUnixNanos < startUnixNanos {
		return FollowingInteraction{}, 0, "", fmt.Errorf("window %d to %d is not ordered", startUnixNanos, endUnixNanos)
	}
	if err := fi.Validate(); err != nil {
		return FollowingInteraction{}, 0, "", err
	}
	var kept []InteractionInstant
	for _, in := range fi.Instants {
		if in.CaptureUnixNanos >= startUnixNanos && in.CaptureUnixNanos <= endUnixNanos {
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
// chosen, and it had a sample there. A chosen leader without a sample is
// recorded as not observed, with its support missed_unknown.
func evaluated(in InteractionInstant) bool {
	if in.Role != DispositionLeader {
		return false
	}
	return !(in.Reason == ReasonNotObserved && in.LeaderSupport == SupportMissedUnknown)
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
// as encoded; it returns which part differs, or "" when none does.
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
		a, errA := json.Marshal(part.a)
		b, errB := json.Marshal(part.b)
		if errA != nil || errB != nil || string(a) != string(b) {
			diffs = append(diffs, part.name)
		}
	}
	return strings.Join(diffs, ", ")
}
