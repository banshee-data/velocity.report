package annotation

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Drafting a following reference.
//
// A following reference is the operator's answer to "what is this object
// following" over an interval. The macOS window shows these records but
// authors none, so they arrive as import files, and DraftFollowing writes one
// from a decision. The decision is the operator's. Each gap is derived from
// the parties' stored keyframes, never typed in: only at a sample where both
// have a reviewed, independent keyframe that places the bumper the gap needs.
// Every other sample where either party has a keyframe is reported with the
// reason it has no gap, so the operator can see what the record leaves out.
//
// A following record has no review step after import: ReviewPhysicalRecord
// reviews bodies and keyframes. The draft's review is the one it is imported
// with, so a reviewed record is drafted as reviewed, deliberately.

// DraftFollowingMethod is the review method a drafted record carries.
const DraftFollowingMethod = "import:draft-following"

// draftGapAssumptions states how a drafted gap is bounded.
const draftGapAssumptions = "each gap is derived from the parties' reviewed, independent keyframes at its sample: " +
	"the leader's rear minus the follower's front, along the follower's axis. Its half-width adds both bumpers' bounds, " +
	"the chord the follower's yaw bound sweeps across the separation, and that chord's effect on the bumper bounds " +
	"(worst case, whatever the correlation). A lower bound the propagation puts below zero is clamped to zero."

// FollowingDraft is one decision to draft as a following reference.
type FollowingDraft struct {
	// FollowingID names the record; empty draws a fresh ID. Drafting a stored
	// record's ID again replaces it when imported with replace.
	FollowingID      string
	FollowerObjectID string
	Decision         FollowingDecision
	LeaderObjectID   string
	Interval         FrameInterval
	// Reviewed records the decision and its gaps as reviewed by Author.
	Reviewed bool
	// TrackerSource names the tracker output the decision was made against.
	// Set, the record is tracker-assisted and is never scored as truth.
	TrackerSource string
	Author        string
	Session       string
}

// SkippedGap is a sample in the interval where a party has a keyframe but no
// gap was drafted, and why.
type SkippedGap struct {
	SampleID int
	Reason   string
}

// DraftFollowing turns a decision into an import file for the document's
// pack, with gaps derived from the document's keyframes. It does not check
// the result against the store; PreparePhysicalImport does that, exactly as
// the import will.
func DraftFollowing(doc *PhysicalReferenceSet, d FollowingDraft) (*PhysicalReferenceImport, []SkippedGap, error) {
	switch d.Decision {
	case FollowingLeader:
		if d.LeaderObjectID == "" {
			return nil, nil, fmt.Errorf("a leader decision needs the leader's object ID")
		}
	case FollowingNoLeader, FollowingAmbiguous:
		if d.LeaderObjectID != "" {
			return nil, nil, fmt.Errorf("a %s decision names no leader, but %q was given", d.Decision, d.LeaderObjectID)
		}
	default:
		return nil, nil, fmt.Errorf("decision %q (want %q, %q or %q)", d.Decision, FollowingLeader, FollowingNoLeader, FollowingAmbiguous)
	}
	id := d.FollowingID
	if id == "" {
		id = newPhysicalID("following")
	}
	review := PhysicalReview{
		Status: StatusProposed, Origin: OriginIndependent, Method: DraftFollowingMethod,
		Provenance: Provenance{Author: d.Author, Session: d.Session, Operation: "draft_following",
			CreatedUTC: time.Now().UTC().Format(time.RFC3339Nano)},
	}
	if d.Reviewed {
		review.Status = StatusReviewed
	}
	if d.TrackerSource != "" {
		review.Origin, review.TrackerSource = OriginTrackerAssisted, d.TrackerSource
	}
	rec := FollowingReference{
		FollowingID: id, FollowerObjectID: d.FollowerObjectID, Decision: d.Decision,
		LeaderObjectID: d.LeaderObjectID, Interval: d.Interval, Review: review,
	}
	var skipped []SkippedGap
	if d.Decision == FollowingLeader {
		rec.Gaps, skipped = draftGaps(doc, d)
	}
	if len(rec.Gaps) > 0 {
		rec.GapDefinition = GapAlongFollowerAxis
		rec.Review.UncertaintyAssumptions = draftGapAssumptions
	}
	return &PhysicalReferenceImport{
		Schema: PhysicalImportSchema, SchemaVersion: PhysicalImportSchemaVersion,
		PackDigest: doc.PackDigest, DatasetID: doc.DatasetID, Source: doc.Source,
		Objects: []PhysicalObject{}, Following: []FollowingReference{rec},
	}, skipped, nil
}

// draftGaps drafts a gap at every sample in the interval where either party
// has a keyframe, or says why it cannot.
func draftGaps(doc *PhysicalReferenceSet, d FollowingDraft) ([]FollowingGap, []SkippedGap) {
	follower, _ := doc.Object(d.FollowerObjectID)
	leader, _ := doc.Object(d.LeaderObjectID)
	seen := map[int]bool{}
	for _, o := range []PhysicalObject{follower, leader} {
		for _, k := range o.Keyframes {
			if d.Interval.Contains(k.SampleID) {
				seen[k.SampleID] = true
			}
		}
	}
	samples := make([]int, 0, len(seen))
	for s := range seen {
		samples = append(samples, s)
	}
	sort.Ints(samples)
	var gaps []FollowingGap
	var skipped []SkippedGap
	for _, s := range samples {
		g, reason := draftGap(follower, leader, d, s)
		if reason != "" {
			skipped = append(skipped, SkippedGap{SampleID: s, Reason: reason})
			continue
		}
		gaps = append(gaps, g)
	}
	return gaps, skipped
}

// draftGap derives one sample's gap from the follower's front and the
// leader's rear, as the keyframes there place them.
func draftGap(follower, leader PhysicalObject, d FollowingDraft, sample int) (FollowingGap, string) {
	type party struct {
		role, id string
		o        PhysicalObject
		k        PhysicalKeyframe
	}
	parties := []*party{{role: "follower", id: d.FollowerObjectID, o: follower}, {role: "leader", id: d.LeaderObjectID, o: leader}}
	for _, p := range parties {
		k, ok := keyframeAt(p.o, sample)
		if !ok {
			return FollowingGap{}, fmt.Sprintf("%s %q has no keyframe at this sample", p.role, p.id)
		}
		if !k.Review.ScoredAsTruth() {
			return FollowingGap{}, fmt.Sprintf("%s %q's keyframe %q is %s and %s, not reviewed and independent",
				p.role, p.id, k.KeyframeID, k.Review.Status, k.Review.Origin)
		}
		p.k = k
	}
	fk, lk := parties[0].k, parties[1].k
	fg, lg := follower.Geometry(fk), leader.Geometry(lk)
	if fg.Front == nil {
		return FollowingGap{}, fmt.Sprintf("follower %q's keyframe %q places no front: %s", d.FollowerObjectID, fk.KeyframeID, fg.FrontUnavailable)
	}
	if lg.Rear == nil {
		return FollowingGap{}, fmt.Sprintf("leader %q's keyframe %q places no rear: %s", d.LeaderObjectID, lk.KeyframeID, lg.RearUnavailable)
	}
	// A placed front implies a scorable, resolved yaw: it is the axis the gap
	// is measured along, as the scorer measures it.
	ux, uy := math.Cos(fg.Yaw.Rad), math.Sin(fg.Yaw.Rad)
	dx, dy := lg.Rear.XM-fg.Front.XM, lg.Rear.YM-fg.Front.YM
	value := dx*ux + dy*uy
	if value < 0 {
		return FollowingGap{}, fmt.Sprintf("the leader's rear is %.2f m behind the follower's front along the follower's axis", -value)
	}
	// The axis may be anywhere within its bound, and the true separation
	// anywhere within the bumpers' bounds of the stated one.
	sw := swing(fg.Yaw.BoundRad)
	bumpers := fg.Front.BoundM + lg.Rear.BoundM
	half := math.Hypot(dx, dy)*sw + bumpers*(1+sw)
	lower, upper := math.Max(value-half, 0), value+half
	status := fk.Front.Status
	if lk.Rear.Status.strength() < status.strength() {
		status = lk.Rear.Status
	}
	return FollowingGap{
		SampleID: sample, TimestampNs: fk.TimestampNs, Status: status,
		LowerM: &lower, UpperM: &upper, ValueM: &value,
		FollowerFront: EndpointEvidence{Status: fk.Front.Status, Support: cloneSupport(fk.Front.Support)},
		LeaderRear:    EndpointEvidence{Status: lk.Rear.Status, Support: cloneSupport(lk.Rear.Support)},
		Support:       gapSupport(fk, lk),
	}, ""
}

// gapSupport is the frames both bumpers rest on. A gap with no frame in
// common names the two keyframes it was derived from instead.
func gapSupport(fk, lk PhysicalKeyframe) EvidenceSupport {
	leader := map[int]bool{}
	for _, f := range lk.Rear.Support.Frames {
		leader[f] = true
	}
	var common []int
	for _, f := range fk.Front.Support.Frames {
		if leader[f] {
			common = append(common, f)
		}
	}
	if len(common) > 0 {
		return EvidenceSupport{Frames: common}
	}
	return EvidenceSupport{External: fmt.Sprintf("derived from keyframes %q and %q", fk.KeyframeID, lk.KeyframeID)}
}

func keyframeAt(o PhysicalObject, sample int) (PhysicalKeyframe, bool) {
	if i := indexOfKeyframe(o.Keyframes, sample); i >= 0 {
		return o.Keyframes[i], true
	}
	return PhysicalKeyframe{}, false
}

func cloneSupport(s EvidenceSupport) EvidenceSupport {
	return EvidenceSupport{Frames: append([]int(nil), s.Frames...), External: s.External}
}
