package l8behaviour

// Leader choice and synchronisation, per Section 8.3 of
// docs/plans/lidar-behaviour-analytics-plan.md: "choose the nearest credible
// leader" and "suppress ... ordering or common-path ambiguity" rather than
// choosing the argmax.
//
// Method following_pairing_v1, for one follower instant along the follower's
// path:
//
//  1. Placement. Every other body present at the instant is placed by its
//     body centre (bodyLocation) and located on the path. It is off_path if it
//     does not project onto the supported extent; outside_corridor if its
//     lateral offset exceeds the corridor half-width either from the path or
//     from the follower, so that on a wide path two bodies side by side are
//     never each other's leader; behind if its centre is not ahead of the
//     follower's; beyond_range if more than MaxLeaderRangeM ahead; and
//     otherwise ahead.
//  2. Nearest. The nearest body ahead is the one with the least centre arc,
//     the lower track id on a tie.
//  3. Separability. Each other body ahead is separable from the nearest when
//     its trailing extreme clears the nearest's leading extreme by more than
//     SeparationSigmas combined one-sigmas, which means it is fully ahead of
//     the nearest and the nearest blocks it. When either extreme cannot be
//     projected (no synchronised sample, an unresolved orientation, a missing
//     extent), the centres must be more than UnresolvedSeparationM apart
//     instead. A body not separable from the nearest competes with it.
//  4. Decision. If the nearest is not established on the path, the instant is
//     suppressed with no_common_path and the reason the body is not
//     established (a crossing, a reversal, a fork, or no evidence at all): it
//     can be neither chosen nor ruled out. Otherwise, if anything competes
//     with it, ambiguous_leader. Otherwise it is the leader. No body ahead is
//     free flow: no leader and no suppression.
//
// Synchronisation, following_sync_frame_exact_v1: evaluation instants are the
// follower's sample times, and a body is present when its first and last
// samples span the instant. A pair is evaluated only on a sample at exactly
// the follower's capture time, which is what one sensor's shared frame clock
// gives every track. A body present without a sample there (a missing row) is
// still placed, by linear interpolation between its bracketing samples, so
// that a gap in the leader's record cannot promote the car behind it; if it
// is chosen, the instant is not evaluated and is accounted as not_observed. A
// multi-sensor or cross-clock capture needs a resampling rule this does not
// provide, and must change this method id when it gains one.

import (
	"fmt"
	"math"
	"sort"
)

// PairingMethodID versions leader choice and its separability rule.
const PairingMethodID = "following_pairing_v1"

// SyncMethodID versions how a pair is synchronised in capture time.
const SyncMethodID = "following_sync_frame_exact_v1"

// PairingParams bound leader choice. None has a default.
type PairingParams struct {
	// MaxLeaderRangeM is how far ahead, centre to centre along the path, a
	// body may be and still be a leader.
	MaxLeaderRangeM float64 `json:"max_leader_range_m"`
	// SeparationSigmas is k in the separability rule: a body is separable
	// from the nearest when its trailing extreme clears the nearest's leading
	// extreme by more than k combined one-sigmas.
	SeparationSigmas float64 `json:"separation_sigmas"`
	// UnresolvedSeparationM is the centre separation that separates two bodies
	// when either's extremes cannot be projected. It should exceed the longest
	// plausible body, so two such bodies can never overlap along the path.
	UnresolvedSeparationM float64 `json:"unresolved_separation_m"`
}

// Validate requires every bound, positive and finite.
func (p PairingParams) Validate() error {
	for _, v := range []float64{p.MaxLeaderRangeM, p.SeparationSigmas, p.UnresolvedSeparationM} {
		if !(v > 0) || !finite(v) {
			return fmt.Errorf("pairing parameters must all be positive and finite")
		}
	}
	return nil
}

// Presence is one road user at one follower instant.
type Presence struct {
	Passage Passage `json:"passage"`
	// Sample is the track's sample at the instant; nil when the track spans
	// the instant without a sample there.
	Sample *TrajectorySample `json:"sample,omitempty"`
	// X and Y place the body for ordering: its centre where that can be
	// computed, interpolated between bracketing samples when Sample is nil.
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// PresenceAt is a trajectory's presence at a capture time: false when its
// samples do not span the time.
func PresenceAt(t Trajectory, captureUnixNanos int64) (Presence, bool) {
	n := len(t.Samples)
	if n == 0 || captureUnixNanos < t.Samples[0].CaptureUnixNanos || captureUnixNanos > t.Samples[n-1].CaptureUnixNanos {
		return Presence{}, false
	}
	i := sort.Search(n, func(i int) bool { return t.Samples[i].CaptureUnixNanos >= captureUnixNanos })
	if t.Samples[i].CaptureUnixNanos == captureUnixNanos {
		s := t.Samples[i]
		x, y, _ := bodyLocation(s)
		return Presence{Passage: t.Passage, Sample: &s, X: x, Y: y}, true
	}
	a, b := t.Samples[i-1], t.Samples[i]
	ax, ay, _ := bodyLocation(a)
	bx, by, _ := bodyLocation(b)
	f := float64(captureUnixNanos-a.CaptureUnixNanos) / float64(b.CaptureUnixNanos-a.CaptureUnixNanos)
	return Presence{Passage: t.Passage, X: ax + f*(bx-ax), Y: ay + f*(by-ay)}, true
}

// PathMembership says which tracks are established on a path and why the
// others are not. LocalPathResult is one; EstablishedTracks states it
// outright for analytic geometry.
type PathMembership interface {
	IsMember(trackID string) bool
	NotEstablishedCondition(trackID string) PathCondition
}

// EstablishedTracks declares membership explicitly, for a path that was
// stated rather than fitted, such as the analytic fixtures' StraightPath.
type EstablishedTracks map[string]bool

// IsMember reports whether the track was declared established.
func (e EstablishedTracks) IsMember(trackID string) bool { return e[trackID] }

// NotEstablishedCondition is unestablished_body: nothing else is known.
func (e EstablishedTracks) NotEstablishedCondition(string) PathCondition {
	return PathUnestablishedBody
}

// LeaderCandidate is one other body at one follower instant, and what the
// pairing decided about it.
type LeaderCandidate struct {
	TrackID     string               `json:"track_id"`
	Disposition CandidateDisposition `json:"disposition"`
	// Established is true when the body is a member of the follower's path.
	Established bool `json:"established"`
	// Synchronised is true when the body had a sample at this instant.
	Synchronised bool `json:"synchronised"`
	// CentreArcM and LateralM place the body centre on the path; absent when
	// it does not project onto the path.
	CentreArcM *float64 `json:"centre_arc_m,omitempty"`
	LateralM   *float64 `json:"lateral_m,omitempty"`
}

// PairingDecision is the leader choice for one follower instant: a leader, a
// suppression (ambiguous_leader, or no_common_path with its condition), or
// neither, which is free flow.
type PairingDecision struct {
	FollowerTrackID  string            `json:"follower_track_id"`
	CaptureUnixNanos int64             `json:"capture_unix_nanos"`
	LeaderTrackID    string            `json:"leader_track_id,omitempty"`
	Reason           SuppressionReason `json:"reason,omitempty"`
	Condition        PathCondition     `json:"condition,omitempty"`
	// Candidates is every other body present at the instant, sorted by track
	// id; empty when the follower itself could not be placed.
	Candidates []LeaderCandidate `json:"candidates,omitempty"`
}

// Involved lists the established bodies whose encounter with the follower
// this instant belongs to: the leader; or, when suppressed, every established
// body competing with the nearest, or else, when the nearest body in the
// corridor is unestablished, the nearest established body beyond it (further
// ahead along the path). It is sorted by track id.
func (d PairingDecision) Involved() []string {
	if d.LeaderTrackID != "" {
		return []string{d.LeaderTrackID}
	}
	var out []string
	var nearest *LeaderCandidate
	for i, c := range d.Candidates {
		if !c.Established {
			continue
		}
		if c.Disposition == DispositionCompeting {
			out = append(out, c.TrackID)
		}
		if (c.Disposition == DispositionCompeting || c.Disposition == DispositionBlocked) &&
			(nearest == nil || *c.CentreArcM < *nearest.CentreArcM) {
			nearest = &d.Candidates[i]
		}
	}
	if d.Reason == ReasonNoCommonPath && nearest != nil {
		return []string{nearest.TrackID}
	}
	return out
}

// Candidate returns the decision's record for one track.
func (d PairingDecision) Candidate(trackID string) (LeaderCandidate, bool) {
	i := sort.Search(len(d.Candidates), func(i int) bool { return d.Candidates[i].TrackID >= trackID })
	if i < len(d.Candidates) && d.Candidates[i].TrackID == trackID {
		return d.Candidates[i], true
	}
	return LeaderCandidate{}, false
}

// DecideLeader chooses the follower's leader at one instant, or says why it
// cannot. The follower must have a sample at the instant; the others are
// every other body present.
func DecideLeader(path PathFrame, members PathMembership, follower Presence, others []Presence,
	following FollowingParams, pairing PairingParams) (PairingDecision, error) {
	if err := following.Validate(); err != nil {
		return PairingDecision{}, err
	}
	if err := pairing.Validate(); err != nil {
		return PairingDecision{}, err
	}
	if path == nil || members == nil {
		return PairingDecision{}, fmt.Errorf("leader choice requires a path and its membership")
	}
	if follower.Sample == nil {
		return PairingDecision{}, fmt.Errorf("follower %s has no sample at the instant", follower.Passage.TrackID)
	}
	fid := follower.Passage.TrackID
	dec := PairingDecision{FollowerTrackID: fid, CaptureUnixNanos: follower.Sample.CaptureUnixNanos}
	floc, ok := path.Locate(follower.X, follower.Y)
	if !ok {
		dec.Reason, dec.Condition = ReasonNoCommonPath, PathOutsideExtent
		return dec, nil
	}

	sorted := append([]Presence(nil), others...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Passage.TrackID < sorted[j].Passage.TrackID })
	type ahead struct {
		index int
		arc   float64
	}
	var candidates []ahead
	dec.Candidates = make([]LeaderCandidate, len(sorted))
	for i, o := range sorted {
		id := o.Passage.TrackID
		if id == fid || (i > 0 && id == sorted[i-1].Passage.TrackID) {
			return PairingDecision{}, fmt.Errorf("track %s is present twice at one instant", id)
		}
		if o.Sample != nil && o.Sample.CaptureUnixNanos != dec.CaptureUnixNanos {
			return PairingDecision{}, fmt.Errorf("track %s sample is not at the follower's instant", id)
		}
		rec := LeaderCandidate{TrackID: id, Established: members.IsMember(id), Synchronised: o.Sample != nil}
		loc, located := path.Locate(o.X, o.Y)
		switch {
		case !located:
			rec.Disposition = DispositionOffPath
		case math.Abs(loc.LateralM) > following.CorridorHalfWidthM ||
			math.Abs(loc.LateralM-floc.LateralM) > following.CorridorHalfWidthM:
			rec.Disposition = DispositionOutsideCorridor
		case !(loc.ArcM > floc.ArcM):
			rec.Disposition = DispositionBehind
		case loc.ArcM-floc.ArcM > pairing.MaxLeaderRangeM:
			rec.Disposition = DispositionBeyondRange
		default:
			candidates = append(candidates, ahead{i, loc.ArcM})
		}
		if located {
			rec.CentreArcM, rec.LateralM = ptr(loc.ArcM), ptr(loc.LateralM)
		}
		dec.Candidates[i] = rec
	}
	if len(candidates) == 0 {
		return dec, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].arc < candidates[j].arc })

	nearest := candidates[0].index
	competing := false
	for _, c := range candidates[1:] {
		sep, err := separable(path, sorted[nearest], sorted[c.index], candidates[0].arc, c.arc, pairing)
		if err != nil {
			return PairingDecision{}, err
		}
		if sep {
			dec.Candidates[c.index].Disposition = DispositionBlocked
		} else {
			dec.Candidates[c.index].Disposition = DispositionCompeting
			competing = true
		}
	}
	n := &dec.Candidates[nearest]
	switch {
	case !n.Established:
		n.Disposition = DispositionUnestablished
		dec.Reason, dec.Condition = ReasonNoCommonPath, members.NotEstablishedCondition(n.TrackID)
	case competing:
		n.Disposition = DispositionCompeting
		dec.Reason = ReasonAmbiguousLeader
	default:
		n.Disposition = DispositionLeader
		dec.LeaderTrackID = n.TrackID
	}
	return dec, nil
}

// separable applies the separability rule to the nearest body and one
// further ahead.
func separable(path PathFrame, nearest, other Presence, nearestArc, otherArc float64, p PairingParams) (bool, error) {
	if nearest.Sample != nil && other.Sample != nil {
		nb, nr, err := ProjectBody(path, nearest.Passage.TrackID, *nearest.Sample)
		if err != nil {
			return false, err
		}
		ob, or, err := ProjectBody(path, other.Passage.TrackID, *other.Sample)
		if err != nil {
			return false, err
		}
		if nr == ReasonUnspecified && or == ReasonUnspecified {
			clearance := ob.Trailing.ArcM - nb.Leading.ArcM
			return clearance > p.SeparationSigmas*math.Hypot(ob.Trailing.SigmaM, nb.Leading.SigmaM), nil
		}
	}
	return otherArc-nearestArc > p.UnresolvedSeparationM, nil
}
