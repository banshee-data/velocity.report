package l8behaviour

// The local following path of Section 8.3 of
// docs/plans/lidar-behaviour-analytics-plan.md: "a directed empirical
// centreline from validated final trajectories over the capture, group tracks
// only while their tangent and lateral offset are compatible, then project
// physical endpoints onto that line". It is deliberately local and claims no
// lane, route or scene semantics: no lane map, no L7 scene graph, no planner.
// Durable population paths and deviation metrics are Phase 6C's, not this.
//
// Method following_local_path_v1, for one follower F:
//
//  1. Window. The capture window is F's own passage, [first, last] sample.
//     Only evidence inside it is used, so a fork taken after F has gone, or a
//     crossing an hour later, does not touch F's path.
//  2. Evidence. A sample shapes a path when it was observed (never coasted),
//     its estimator stands behind its pose (geometry_converging or
//     established), its reference is a place on the body whose centre can be
//     computed, and it moves at or above MinSpeedMps, so its velocity names a
//     direction. A track with fewer than MinTrackEvidence such samples is not
//     directed and joins no path.
//  3. Frame. The axis is F's direction: the normalised sum of its evidence
//     velocity unit vectors. Every evidence sample gets an along-axis u, a
//     lateral v (positive left) and a direction relative to the axis, and is
//     binned by floor(u / KnotSpacingM). Bins are absolute multiples of the
//     spacing, so the same geometry bins the same way whatever the window.
//  4. Relations. Two directed tracks are compared where they share bins: each
//     sample's lateral against the other track's lateral at the same
//     along-axis position (its bin mean carried along its bin-mean direction),
//     and the two bin-mean directions against each other. Near is within
//     GroupLateralM; aligned is
//     within MaxTangentRad, opposed within MaxTangentRad of a half turn. Any
//     near and opposed comparison makes them opposing, any near and neither
//     makes them crossing, near-aligned and far together a fork or merge,
//     near-aligned alone over at least MinOverlapKnots shared bins the same
//     path, and far alone separate. No shared bin is no relation.
//  5. Group. F's group is its connected component over same-path relations.
//     It is refused, never partially fitted, when a member moves backwards
//     along the axis (direction_reversal) or when any two members are anything
//     but the same path or unrelated: opposing (direction_reversal), crossing,
//     fork_or_merge, or separate (lateral_incompatible). Compatibility is not
//     transitive, and a group whose members disagree is two paths, not one.
//  6. Fit. Each bin's knot sits at the bin centre along the axis. Its
//     lateral is fitted as v(member, bin) = c(bin) + o(member), where
//     v(member, bin) is the member's lateral at the bin centre (its mean
//     carried along its own direction, which removes the bias of samples that
//     do not sit at the centre of a curving bin), each member counts once (so
//     a slow or stopped member does not outweigh a fast one), and each
//     member's persistent offset within the
//     group, estimated where it shares bins, is removed before averaging, with
//     the offsets summing to zero. The centreline is then the group's mean
//     line everywhere, and does not swing toward whichever member alone
//     supports the far end. A bin with fewer than MinSamplesPerKnot samples is
//     unsupported; interior runs of at most MaxBridgeKnots unsupported bins are
//     bridged by linear interpolation and marked, never extrapolated. The
//     longest run is the path (the earliest on a tie), extended half a bin at
//     each end so it spans whole bins; shorter than MinExtentM is
//     weak_support.
//
// Crossing, reversing and other non-member bodies do not shape the centreline,
// which is fitted from members alone. They matter at the instants they stand
// between a follower and its leader, which is the pairing step's to decide;
// the result records, for every other track in the window, why it is not on
// the path, so that step can say so.
//
// Limits a reviewer should know. The axis is one direction, so the path must
// progress along it: a member turning through more than a quarter turn from
// the axis is refused as a reversal. Projection onto the polyline is unique
// only while the corridor is narrower than the path's radius of curvature,
// which a simple approach satisfies and a tight turn may not.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// LocalPathMethodID versions the local path fit, its refusal rules and its
// parameters' meaning. Change it when any of them changes.
const LocalPathMethodID = "following_local_path_v1"

// LocalPathParams are the bounds of the local path fit. None has a default: a
// caller states them, and they enter the geometry id.
type LocalPathParams struct {
	// KnotSpacingM is the bin width along the axis, and so the knot spacing.
	KnotSpacingM float64 `json:"knot_spacing_m"`
	// GroupLateralM is the largest lateral separation, sample to bin mean, at
	// which two tracks may share a path.
	GroupLateralM float64 `json:"group_lateral_m"`
	// MaxTangentRad is the largest direction difference at which two tracks
	// may share a path. It must be under a quarter turn.
	MaxTangentRad float64 `json:"max_tangent_rad"`
	// MinSpeedMps is the speed below which a sample names no direction and is
	// not path evidence. It should sit well above the velocity noise.
	MinSpeedMps float64 `json:"min_speed_mps"`
	// MinTrackEvidence is the evidence samples a track needs to join a path.
	MinTrackEvidence int `json:"min_track_evidence"`
	// MinOverlapKnots is the shared bins two tracks need before they can be
	// the same path. A conflict needs only to be seen once.
	MinOverlapKnots int `json:"min_overlap_knots"`
	// MinSamplesPerKnot is the evidence samples a bin needs to be supported.
	MinSamplesPerKnot int `json:"min_samples_per_knot"`
	// MaxBridgeKnots is the longest interior run of unsupported bins that is
	// bridged by interpolation; zero bridges nothing.
	MaxBridgeKnots int `json:"max_bridge_knots"`
	// MinExtentM is the shortest supported path; it must span two knots.
	MinExtentM float64 `json:"min_extent_m"`
}

// Validate requires every bound, positive and finite where it is a length.
func (p LocalPathParams) Validate() error {
	for _, f := range []struct {
		name string
		v    float64
	}{
		{"knot spacing", p.KnotSpacingM}, {"group lateral", p.GroupLateralM},
		{"max tangent", p.MaxTangentRad}, {"min speed", p.MinSpeedMps}, {"min extent", p.MinExtentM},
	} {
		if !(f.v > 0) || !finite(f.v) {
			return fmt.Errorf("local path %s must be positive and finite", f.name)
		}
	}
	if !(p.MaxTangentRad < math.Pi/2) {
		return fmt.Errorf("local path max tangent must be under a quarter turn")
	}
	if p.MinTrackEvidence < 2 || p.MinOverlapKnots < 1 || p.MinSamplesPerKnot < 1 || p.MaxBridgeKnots < 0 {
		return fmt.Errorf("local path needs at least two evidence samples per track, one overlap knot and one sample per knot, and a non-negative bridge")
	}
	if p.MinExtentM < 2*p.KnotSpacingM {
		return fmt.Errorf("local path min extent %.3g m must span at least two knots of %.3g m", p.MinExtentM, p.KnotSpacingM)
	}
	return nil
}

// PathVertex is one polyline vertex with its arc length from the start.
type PathVertex struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	ArcM float64 `json:"arc_m"`
}

// PathKnot is one fitted knot with the evidence behind it, for review.
type PathKnot struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	ArcM float64 `json:"arc_m"`
	// Samples and Tracks count the member evidence in the knot's bin.
	Samples int `json:"samples"`
	Tracks  int `json:"tracks"`
	// Bridged marks a knot interpolated across a short gap in the evidence.
	Bridged bool `json:"bridged"`
}

// LocalPath is a fitted local path: a directed polyline through the knots,
// extended half a bin at each end. It implements PathFrame.
type LocalPath struct {
	ID            string        `json:"id"`
	EstimateStage EstimateStage `json:"estimate_stage"`
	// AxisRad is the fit axis: the follower's direction of travel.
	AxisRad  float64      `json:"axis_rad"`
	Vertices []PathVertex `json:"vertices"`
	Knots    []PathKnot   `json:"knots"`
}

// GeometryID names the fitted geometry for provenance.
func (p *LocalPath) GeometryID() string { return p.ID }

// Stage is the least final stage of the evidence the path was fitted from.
func (p *LocalPath) Stage() EstimateStage { return p.EstimateStage }

// LengthM is the path's arc length.
func (p *LocalPath) LengthM() float64 { return p.Vertices[len(p.Vertices)-1].ArcM }

// pathLengthTolerance absorbs rounding when a decoded path's arcs are checked
// against its vertices.
const pathLengthTolerance = 1e-9

// Validate checks a path that arrived by decoding: an identity, a stage, two
// or more finite vertices, and arcs that are the running segment lengths.
func (p *LocalPath) Validate() error {
	if p == nil || p.ID == "" || !p.EstimateStage.Valid() {
		return fmt.Errorf("local path requires an id and an estimate stage")
	}
	if len(p.Vertices) < 2 || !finite(p.AxisRad) {
		return fmt.Errorf("local path %s requires two vertices and a finite axis", p.ID)
	}
	for i, v := range p.Vertices {
		if !finite(v.X) || !finite(v.Y) || !finite(v.ArcM) {
			return fmt.Errorf("local path %s vertex %d is not finite", p.ID, i)
		}
		if i == 0 {
			if v.ArcM != 0 {
				return fmt.Errorf("local path %s starts at arc %.6g, want 0", p.ID, v.ArcM)
			}
			continue
		}
		prev := p.Vertices[i-1]
		seg := math.Hypot(v.X-prev.X, v.Y-prev.Y)
		if !(seg > 0) || math.Abs(v.ArcM-prev.ArcM-seg) > pathLengthTolerance*math.Max(1, v.ArcM) {
			return fmt.Errorf("local path %s segment %d has length %.6g but arcs %.6g to %.6g", p.ID, i, seg, prev.ArcM, v.ArcM)
		}
	}
	return nil
}

// Locate projects a point onto the nearest point of the polyline; on a tie the
// earlier segment wins. A point whose nearest point lies before the first
// vertex or beyond the last is off the path. The tangent is the direction of
// the segment the point projects onto.
func (p *LocalPath) Locate(x, y float64) (PathLocation, bool) {
	best, bestD2 := -1, math.Inf(1)
	var bestAlong, bestRaw float64
	for i := 0; i+1 < len(p.Vertices); i++ {
		a, b := p.Vertices[i], p.Vertices[i+1]
		length := b.ArcM - a.ArcM
		c, s := (b.X-a.X)/length, (b.Y-a.Y)/length
		dx, dy := x-a.X, y-a.Y
		raw := dx*c + dy*s
		along := math.Min(math.Max(raw, 0), length)
		d2 := sq(dx-along*c) + sq(dy-along*s)
		if d2 < bestD2 {
			best, bestD2, bestAlong, bestRaw = i, d2, along, raw
		}
	}
	if best < 0 {
		return PathLocation{}, false
	}
	a, b := p.Vertices[best], p.Vertices[best+1]
	length := b.ArcM - a.ArcM
	if (best == 0 && bestRaw < 0) || (best == len(p.Vertices)-2 && bestRaw > length) {
		return PathLocation{}, false
	}
	c, s := (b.X-a.X)/length, (b.Y-a.Y)/length
	dx, dy := x-a.X, y-a.Y
	lateral := -dx*s + dy*c
	if bestAlong != bestRaw {
		// Clamped at an interior vertex: the offset is the distance to it,
		// signed by the side of the segment the point is on.
		lateral = math.Copysign(math.Sqrt(bestD2), lateral)
	}
	return PathLocation{ArcM: a.ArcM + bestAlong, LateralM: lateral, TangentRad: math.Atan2(s, c)}, true
}

// TrackCondition says why one track is not on a follower's path.
type TrackCondition struct {
	TrackID   string        `json:"track_id"`
	Condition PathCondition `json:"condition"`
}

// LocalPathResult is one follower's local path, or its refusal.
type LocalPathResult struct {
	FollowerTrackID      string `json:"follower_track_id"`
	MethodID             string `json:"method_id"`
	WindowFirstUnixNanos int64  `json:"window_first_unix_nanos"`
	WindowLastUnixNanos  int64  `json:"window_last_unix_nanos"`
	// MemberTrackIDs is the follower's group, sorted. It and NotEstablished
	// are empty when the follower has no direction to group by: too little
	// evidence, or evidence that cancels.
	MemberTrackIDs []string `json:"member_track_ids,omitempty"`
	// NotEstablished names every other track with samples in the window and
	// why it is not on the path, sorted by track id.
	NotEstablished []TrackCondition `json:"not_established,omitempty"`
	// Conditions is why the path was refused, in precedence order; empty
	// exactly when Path is present.
	Conditions []PathCondition `json:"conditions,omitempty"`
	Path       *LocalPath      `json:"path,omitempty"`
}

// IsMember reports whether a track is established on the path.
func (r LocalPathResult) IsMember(trackID string) bool {
	i := sort.SearchStrings(r.MemberTrackIDs, trackID)
	return r.Path != nil && i < len(r.MemberTrackIDs) && r.MemberTrackIDs[i] == trackID
}

// NotEstablishedCondition is why a track is not on the path; a track with no
// evidence in the window at all is an unestablished body.
func (r LocalPathResult) NotEstablishedCondition(trackID string) PathCondition {
	i := sort.Search(len(r.NotEstablished), func(i int) bool { return r.NotEstablished[i].TrackID >= trackID })
	if i < len(r.NotEstablished) && r.NotEstablished[i].TrackID == trackID {
		return r.NotEstablished[i].Condition
	}
	return PathUnestablishedBody
}

// BuildLocalPath fits the local path for one follower from a capture's
// trajectories. The trajectories must be valid, uniquely identified and from
// one estimator run; a refusal is a result, not an error.
func BuildLocalPath(followerTrackID string, trajectories []Trajectory, params LocalPathParams) (LocalPathResult, error) {
	sorted, err := prepareTrajectories(trajectories)
	if err != nil {
		return LocalPathResult{}, err
	}
	if err := params.Validate(); err != nil {
		return LocalPathResult{}, err
	}
	i := sort.Search(len(sorted), func(i int) bool { return sorted[i].Passage.TrackID >= followerTrackID })
	if i == len(sorted) || sorted[i].Passage.TrackID != followerTrackID {
		return LocalPathResult{}, fmt.Errorf("no trajectory for follower %q", followerTrackID)
	}
	return buildLocalPath(sorted[i], sorted, params)
}

// prepareTrajectories validates a capture's trajectories and returns a copy
// sorted by track id, which is the order every later step iterates in, so the
// output does not depend on the order the caller supplied.
func prepareTrajectories(trajectories []Trajectory) ([]Trajectory, error) {
	sorted := append([]Trajectory(nil), trajectories...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Passage.TrackID < sorted[j].Passage.TrackID })
	for i, t := range sorted {
		if err := t.Validate(); err != nil {
			return nil, err
		}
		if i > 0 && t.Passage.TrackID == sorted[i-1].Passage.TrackID {
			return nil, fmt.Errorf("track %s appears twice", t.Passage.TrackID)
		}
		if t.Estimate != sorted[0].Estimate {
			return nil, fmt.Errorf("capture mixes estimator versions: %+v and %+v", sorted[0].Estimate, t.Estimate)
		}
	}
	return sorted, nil
}

// bodyLocation is where a sample places its body: the body centre when the
// reference and heading allow it, and otherwise the reported position itself
// (a medoid, or a face whose offset cannot be rotated without a heading).
func bodyLocation(s TrajectorySample) (x, y float64, centred bool) {
	switch {
	case s.Reference == ReferenceBodyCentre:
		return s.X, s.Y, true
	case s.Reference == ReferenceNearFaceCentre && s.Heading.Provenance.Valid():
		c, sn := math.Cos(s.Heading.Rad), math.Sin(s.Heading.Rad)
		o := s.AnchorToCentre
		return s.X + o.LongitudinalM*c - o.LateralM*sn, s.Y + o.LongitudinalM*sn + o.LateralM*c, true
	}
	return s.X, s.Y, false
}

type framedSample struct {
	u, v, rel float64
	bin       int
}

type binAgg struct {
	n                          int
	sumU, sumV, sumCos, sumSin float64
}

func (b binAgg) meanDir() float64 { return math.Atan2(b.sumSin, b.sumCos) }

// maxSlopeRad bounds the direction at which a bin's lateral is carried along
// its own slope. Steeper than half a quarter turn, a bin has no single lateral
// at a given along-axis position (a crossing track spans many), and its mean
// lateral is used as it is.
const maxSlopeRad = math.Pi / 4

// lateralAt is the track's lateral at along-axis position u within the bin:
// its mean lateral carried along its mean direction from its mean position.
// Samples do not sit at the bin centre, so on a curve the uncorrected mean
// would misplace a knot by the slope times that offset.
func (b binAgg) lateralAt(u float64) float64 {
	n := float64(b.n)
	v := b.sumV / n
	if dir := b.meanDir(); math.Abs(dir) <= maxSlopeRad {
		v += math.Tan(dir) * (u - b.sumU/n)
	}
	return v
}

// trackEvidence is one track's path evidence in the follower's frame.
type trackEvidence struct {
	id       string
	samples  []framedSample
	bins     map[int]binAgg
	stage    EstimateStage
	directed bool
}

// relation is how two tracks' evidence relates; internal, surfaced through
// PathCondition.
type relation uint8

const (
	relNone relation = iota
	relSamePath
	relSeparate
	relForkOrMerge
	relCrossing
	relOpposing
)

// condition maps a relation between two tracks to the condition it imposes;
// unspecified for the two relations that impose none.
func (r relation) condition() PathCondition {
	switch r {
	case relOpposing:
		return PathDirectionReversal
	case relCrossing:
		return PathCrossing
	case relForkOrMerge:
		return PathForkOrMerge
	case relSeparate:
		return PathLateralIncompatible
	}
	return PathConditionUnspecified
}

func relate(a, b *trackEvidence, p LocalPathParams) relation {
	var nearAligned, nearCrossing, nearOpposed, far int
	shared := map[int]bool{}
	compare := func(x, y *trackEvidence) {
		for _, s := range x.samples {
			yb, ok := y.bins[s.bin]
			if !ok {
				continue
			}
			shared[s.bin] = true
			if math.Abs(s.v-yb.lateralAt(s.u)) > p.GroupLateralM {
				far++
				continue
			}
			d := math.Abs(math.Remainder(x.bins[s.bin].meanDir()-yb.meanDir(), 2*math.Pi))
			switch {
			case d <= p.MaxTangentRad:
				nearAligned++
			case d >= math.Pi-p.MaxTangentRad:
				nearOpposed++
			default:
				nearCrossing++
			}
		}
	}
	compare(a, b)
	compare(b, a)
	switch {
	case len(shared) == 0:
		return relNone
	case nearOpposed > 0:
		return relOpposing
	case nearCrossing > 0:
		return relCrossing
	case nearAligned > 0 && far > 0:
		return relForkOrMerge
	case nearAligned > 0 && len(shared) >= p.MinOverlapKnots:
		return relSamePath
	case nearAligned > 0:
		return relNone
	}
	return relSeparate
}

// conditionSet collects path conditions and reports them in precedence order.
type conditionSet uint16

func (c *conditionSet) add(cond PathCondition) {
	if cond != PathConditionUnspecified {
		*c |= 1 << cond
	}
}

func (c conditionSet) ordered() []PathCondition {
	var out []PathCondition
	for _, cond := range PathConditions() {
		if c&(1<<cond) != 0 {
			out = append(out, cond)
		}
	}
	return out
}

func (c conditionSet) first() PathCondition {
	if o := c.ordered(); len(o) > 0 {
		return o[0]
	}
	return PathConditionUnspecified
}

// buildLocalPath fits one follower's path; trajectories are prepared.
func buildLocalPath(follower Trajectory, trajectories []Trajectory, params LocalPathParams) (LocalPathResult, error) {
	first := follower.Samples[0].CaptureUnixNanos
	last := follower.Samples[len(follower.Samples)-1].CaptureUnixNanos
	res := LocalPathResult{
		FollowerTrackID: follower.Passage.TrackID, MethodID: LocalPathMethodID,
		WindowFirstUnixNanos: first, WindowLastUnixNanos: last,
	}

	// Raw evidence per track in the window, and the tracks present at all.
	type rawEvidence struct {
		x, y, dir float64
		stage     EstimateStage
	}
	raw := map[string][]rawEvidence{}
	var present []string
	for _, t := range trajectories {
		inWindow := false
		for _, s := range t.Samples {
			if s.CaptureUnixNanos < first || s.CaptureUnixNanos > last {
				continue
			}
			inWindow = true
			if s.Support != SupportObserved || !(s.Estimation == EstimationGeometryConverging || s.Estimation == EstimationEstablished) {
				continue
			}
			x, y, centred := bodyLocation(s)
			if !centred || math.Hypot(s.VX, s.VY) < params.MinSpeedMps {
				continue
			}
			raw[t.Passage.TrackID] = append(raw[t.Passage.TrackID], rawEvidence{x, y, math.Atan2(s.VY, s.VX), s.Stage})
		}
		if inWindow {
			present = append(present, t.Passage.TrackID)
		}
	}

	followerID := follower.Passage.TrackID
	if len(raw[followerID]) < params.MinTrackEvidence {
		res.Conditions = []PathCondition{PathWeakSupport}
		return res, nil
	}

	// The axis: F's direction of travel.
	var sx, sy float64
	for _, e := range raw[followerID] {
		sx += math.Cos(e.dir)
		sy += math.Sin(e.dir)
	}
	norm := math.Hypot(sx, sy)
	if !(norm > 1e-9*float64(len(raw[followerID]))) {
		// Evidence that cancels has no direction: F went both ways.
		res.Conditions = []PathCondition{PathDirectionReversal}
		return res, nil
	}
	dx, dy := sx/norm, sy/norm
	axis := math.Atan2(dy, dx)

	evidence := map[string]*trackEvidence{}
	var directed []string
	for _, id := range present {
		es := raw[id]
		if len(es) < params.MinTrackEvidence {
			continue
		}
		te := &trackEvidence{id: id, bins: map[int]binAgg{}, stage: StageFinal, directed: true}
		for _, e := range es {
			u := e.x*dx + e.y*dy
			v := -e.x*dy + e.y*dx
			rel := math.Remainder(e.dir-axis, 2*math.Pi)
			bin := int(math.Floor(u / params.KnotSpacingM))
			te.samples = append(te.samples, framedSample{u: u, v: v, rel: rel, bin: bin})
			b := te.bins[bin]
			b.n++
			b.sumU += u
			b.sumV += v
			b.sumCos += math.Cos(rel)
			b.sumSin += math.Sin(rel)
			te.bins[bin] = b
			te.stage = min(te.stage, e.stage)
		}
		evidence[id] = te
		directed = append(directed, id)
	}

	// Relations among directed tracks, and F's component over same-path ones.
	rel := map[[2]string]relation{}
	for i, a := range directed {
		for _, b := range directed[i+1:] {
			rel[[2]string{a, b}] = relate(evidence[a], evidence[b], params)
		}
	}
	relOf := func(a, b string) relation {
		if a > b {
			a, b = b, a
		}
		return rel[[2]string{a, b}]
	}
	member := map[string]bool{followerID: true}
	for queue := []string{followerID}; len(queue) > 0; queue = queue[1:] {
		for _, id := range directed {
			if !member[id] && relOf(queue[0], id) == relSamePath {
				member[id] = true
				queue = append(queue, id)
			}
		}
	}
	for _, id := range directed {
		if member[id] {
			res.MemberTrackIDs = append(res.MemberTrackIDs, id)
		}
	}
	for _, id := range present {
		if member[id] {
			continue
		}
		cond := PathWeakSupport
		if evidence[id] != nil {
			var worst conditionSet
			for _, m := range res.MemberTrackIDs {
				worst.add(relOf(id, m).condition())
			}
			if cond = worst.first(); cond == PathConditionUnspecified {
				cond = PathUnestablishedBody
			}
		}
		res.NotEstablished = append(res.NotEstablished, TrackCondition{id, cond})
	}

	// Refusals: a member moving backwards along the axis, or two members that
	// are neither the same path nor unrelated.
	var conditions conditionSet
	stage := StageFinal
	for i, m := range res.MemberTrackIDs {
		te := evidence[m]
		stage = min(stage, te.stage)
		for _, s := range te.samples {
			if math.Abs(s.rel) >= math.Pi/2 {
				conditions.add(PathDirectionReversal)
				break
			}
		}
		for _, o := range res.MemberTrackIDs[i+1:] {
			conditions.add(relOf(m, o).condition())
		}
	}
	if conditions != 0 {
		res.Conditions = conditions.ordered()
		return res, nil
	}

	path, ok := fitLocalPath(evidence, res.MemberTrackIDs, dx, dy, params)
	if !ok {
		res.Conditions = []PathCondition{PathWeakSupport}
		return res, nil
	}
	path.EstimateStage = stage
	path.AxisRad = axis
	path.ID = localPathID(path, res, params)
	res.Path = path
	return res, nil
}

// The member-offset fit stops when no offset moves by more than
// offsetTolerance, or after maxOffsetIterations, whichever is first; both are
// part of the method.
const (
	offsetTolerance     = 1e-9
	maxOffsetIterations = 32
)

// fitLocalPath fits the knots and the polyline, or reports weak support.
func fitLocalPath(evidence map[string]*trackEvidence, members []string, dx, dy float64, p LocalPathParams) (*LocalPath, bool) {
	lo, hi := math.MaxInt, math.MinInt
	for _, m := range members {
		for bin := range evidence[m].bins {
			lo, hi = min(lo, bin), max(hi, bin)
		}
	}
	n := hi - lo + 1
	samples := make([]int, n)
	tracks := make([]int, n)
	type cell struct {
		member int
		v      float64
	}
	cells := make([][]cell, n)
	centre := func(k int) float64 { return (float64(lo+k) + 0.5) * p.KnotSpacingM }
	for k := 0; k < n; k++ {
		for i, m := range members {
			if b, ok := evidence[m].bins[lo+k]; ok {
				cells[k] = append(cells[k], cell{i, b.lateralAt(centre(k))})
				samples[k] += b.n
			}
		}
		tracks[k] = len(cells[k])
	}

	// v(member, bin) = c(bin) + o(member): each member holds a persistent
	// lateral offset within the group, estimated where it shares a bin with
	// another member and removed before averaging, so a knot supported by one
	// member sits on the group's line rather than on that member's. The
	// offsets are normalised to sum to zero, which makes the centreline the
	// group's mean line. Alternating least squares, to a fixed tolerance.
	offset := make([]float64, len(members))
	lateral := make([]float64, n)
	centreline := func() {
		for k, cs := range cells {
			if len(cs) == 0 {
				continue
			}
			var sum float64
			for _, c := range cs {
				sum += c.v - offset[c.member]
			}
			lateral[k] = sum / float64(len(cs))
		}
	}
	for iter := 0; iter < maxOffsetIterations; iter++ {
		centreline()
		next := make([]float64, len(members))
		count := make([]int, len(members))
		for k, cs := range cells {
			if len(cs) < 2 {
				continue
			}
			for _, c := range cs {
				next[c.member] += c.v - lateral[k]
				count[c.member]++
			}
		}
		var mean float64
		for i := range next {
			if count[i] > 0 {
				next[i] /= float64(count[i])
			}
			mean += next[i]
		}
		mean /= float64(len(next))
		var change float64
		for i := range next {
			next[i] -= mean
			change = math.Max(change, math.Abs(next[i]-offset[i]))
		}
		offset = next
		if change < offsetTolerance {
			break
		}
	}
	centreline()
	supported := func(k int) bool { return samples[k] >= p.MinSamplesPerKnot }

	// Runs of supported bins joined across short interior gaps; the longest
	// wins, the earliest on a tie.
	bestStart, bestEnd := -1, -1
	for k := 0; k < n; {
		if !supported(k) {
			k++
			continue
		}
		start, end := k, k
		for j := k + 1; j < n; j++ {
			if supported(j) {
				if j-end-1 > p.MaxBridgeKnots {
					break
				}
				end = j
			}
		}
		if bestStart < 0 || end-start > bestEnd-bestStart {
			bestStart, bestEnd = start, end
		}
		k = end + 1
	}
	if bestStart < 0 || float64(bestEnd-bestStart+1)*p.KnotSpacingM < p.MinExtentM {
		return nil, false
	}

	path := &LocalPath{}
	prevSupported := bestStart
	for k := bestStart; k <= bestEnd; k++ {
		knot := PathKnot{Samples: samples[k], Tracks: tracks[k]}
		c := lateral[k]
		if !supported(k) {
			// Interior by construction: bracketed by supported bins.
			next := k + 1
			for !supported(next) {
				next++
			}
			f := float64(k-prevSupported) / float64(next-prevSupported)
			c = lateral[prevSupported] + f*(lateral[next]-lateral[prevSupported])
			knot.Bridged = true
		} else {
			prevSupported = k
		}
		u := centre(k)
		knot.X, knot.Y = u*dx-c*dy, u*dy+c*dx
		path.Knots = append(path.Knots, knot)
	}

	// Vertices: the knots, extended half a bin at each end along the end
	// segments, with running arc length.
	extend := func(from, to PathKnot) (float64, float64) {
		ex, ey := to.X-from.X, to.Y-from.Y
		l := math.Hypot(ex, ey)
		return to.X + ex/l*p.KnotSpacingM/2, to.Y + ey/l*p.KnotSpacingM/2
	}
	k := path.Knots
	sx, sy := extend(k[1], k[0])
	ex, ey := extend(k[len(k)-2], k[len(k)-1])
	points := [][2]float64{{sx, sy}}
	for _, knot := range k {
		points = append(points, [2]float64{knot.X, knot.Y})
	}
	points = append(points, [2]float64{ex, ey})
	var arc float64
	for i, pt := range points {
		if i > 0 {
			arc += math.Hypot(pt[0]-points[i-1][0], pt[1]-points[i-1][1])
		}
		path.Vertices = append(path.Vertices, PathVertex{X: pt[0], Y: pt[1], ArcM: arc})
		if i >= 1 && i <= len(k) {
			path.Knots[i-1].ArcM = arc
		}
	}
	return path, true
}

// localPathID content-addresses a fitted path: the method, its parameters,
// the evidence it was fitted from (members and window) and the geometry
// itself, so the same fit always has the same id and any change moves it.
func localPathID(path *LocalPath, res LocalPathResult, p LocalPathParams) string {
	h := sha256.New()
	params, _ := json.Marshal(p) // a struct of numbers always encodes
	for _, s := range append([]string{LocalPathMethodID, string(params), path.EstimateStage.String()}, res.MemberTrackIDs...) {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	var buf [8]byte
	for _, v := range []int64{res.WindowFirstUnixNanos, res.WindowLastUnixNanos} {
		binary.BigEndian.PutUint64(buf[:], uint64(v))
		h.Write(buf[:])
	}
	for _, v := range path.Vertices {
		for _, f := range []float64{v.X, v.Y} {
			binary.BigEndian.PutUint64(buf[:], math.Float64bits(f))
			h.Write(buf[:])
		}
	}
	return LocalPathMethodID + "/" + hex.EncodeToString(h.Sum(nil))[:16]
}
