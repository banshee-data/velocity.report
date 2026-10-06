package annotation

import (
	"fmt"
	"math"
	"strings"
)

// Validation of physical references. Every rule here is a refusal: a record
// that loads when it should not produces plausible numbers, which is worse
// than one that fails to open.

// Validate checks the document against its pack and its own rules. It does
// not read the sidecar; ValidateLinks does.
func (r *PhysicalReferenceSet) Validate(p *Pack) error {
	if r.Schema != PhysicalReferenceSchema {
		return fmt.Errorf("physical reference schema %q, want %q", r.Schema, PhysicalReferenceSchema)
	}
	if r.SchemaVersion != PhysicalReferenceSchemaVersion {
		return fmt.Errorf("physical reference schema version %d, this build reads %d", r.SchemaVersion, PhysicalReferenceSchemaVersion)
	}
	if r.Revision < 1 || r.Revision == math.MaxInt {
		return fmt.Errorf("invalid physical reference revision %d", r.Revision)
	}
	if r.PackDigest != p.Manifest.PackDigest {
		return fmt.Errorf("physical references were written against pack %s, this pack is %s", r.PackDigest, p.Manifest.PackDigest)
	}
	if r.DatasetID != p.Manifest.DatasetID {
		return fmt.Errorf("physical reference dataset %q does not match pack dataset %q", r.DatasetID, p.Manifest.DatasetID)
	}
	if err := r.validateSource(p); err != nil {
		return err
	}
	v := physicalValidator{p: p, ids: map[string]string{}, keyframes: map[string]map[int]PhysicalKeyframe{}}
	seen := map[string]bool{}
	for _, o := range r.Objects {
		if o.ObjectID == "" {
			return fmt.Errorf("a physical reference has no object id")
		}
		if seen[o.ObjectID] {
			return fmt.Errorf("object %q has two physical references", o.ObjectID)
		}
		seen[o.ObjectID] = true
		if err := v.object(o); err != nil {
			return fmt.Errorf("object %q: %w", o.ObjectID, err)
		}
	}
	for _, f := range r.Following {
		if err := v.following(f); err != nil {
			return fmt.Errorf("following %q: %w", f.FollowingID, err)
		}
	}
	if err := followingConflicts(r.Following); err != nil {
		return err
	}
	return r.validateOrigins(v.origins)
}

// followingConflicts refuses two reviewed records for one follower that
// overlap and disagree: a different decision or leader, or two gaps at one
// sample. A proposal may disagree with a reviewed record, which is what a
// proposal to change one is; two confirmed answers to one question may not.
func followingConflicts(fs []FollowingReference) error {
	for i, a := range fs {
		for _, b := range fs[i+1:] {
			if a.Review.Status != StatusReviewed || b.Review.Status != StatusReviewed || a.FollowerObjectID != b.FollowerObjectID {
				continue
			}
			first, last := max(a.Interval.FirstSample, b.Interval.FirstSample), min(a.Interval.LastSample, b.Interval.LastSample)
			if first > last {
				continue
			}
			if a.Decision != b.Decision || a.LeaderObjectID != b.LeaderObjectID {
				return fmt.Errorf("following %q (%s %s) and %q (%s %s) are both reviewed for follower %q over samples [%d, %d] and disagree",
					a.FollowingID, a.Decision, a.LeaderObjectID, b.FollowingID, b.Decision, b.LeaderObjectID, a.FollowerObjectID, first, last)
			}
			for _, ga := range a.Gaps {
				for _, gb := range b.Gaps {
					if ga.SampleID == gb.SampleID {
						return fmt.Errorf("following %q and %q both give follower %q a gap at sample %d", a.FollowingID, b.FollowingID, a.FollowerObjectID, ga.SampleID)
					}
				}
			}
		}
	}
	return nil
}

func (r *PhysicalReferenceSet) validateSource(p *Pack) error {
	want := PackPhysicalSource(p)
	got := r.Source
	for _, f := range []struct{ name, got, want string }{
		{"sensor_id", got.SensorID, want.SensorID},
		{"vrlog_header_sha256", got.VRLOGHeaderSHA, want.VRLOGHeaderSHA},
		{"vrlog_frames_sha256", got.VRLOGFramesSHA, want.VRLOGFramesSHA},
		{"coordinate_frame", got.CoordinateFrame, want.CoordinateFrame},
		{"reference_frame", got.ReferenceFrame, want.ReferenceFrame},
		{"transform_version", got.TransformVersion, want.TransformVersion},
		{"units", got.Units, want.Units},
	} {
		if f.got != f.want {
			return fmt.Errorf("physical reference source %s is %q, the pack's is %q", f.name, f.got, f.want)
		}
	}
	if got.Units != PhysicalUnits {
		return fmt.Errorf("physical references need a pack in %s, this one is in %q", PhysicalUnits, got.Units)
	}
	return nil
}

// validateOrigins holds every current record to the origin it was created
// under.
func (r *PhysicalReferenceSet) validateOrigins(current map[string]ReferenceOrigin) error {
	for key, origin := range r.RecordOrigins {
		if origin != OriginIndependent && origin != OriginTrackerAssisted {
			return fmt.Errorf("record %s has origin %q", key, origin)
		}
	}
	for key, origin := range current {
		created, ok := r.RecordOrigins[key]
		if !ok {
			return fmt.Errorf("record %s is missing from record_origins", key)
		}
		if created != origin {
			return fmt.Errorf("record %s was created %s and now claims %s: an origin never changes; author a new record", key, created, origin)
		}
	}
	return nil
}

type physicalValidator struct {
	p *Pack
	// ids maps every record ID to its kind, so no two records share one.
	ids     map[string]string
	origins map[string]ReferenceOrigin
	// keyframes are every object's keyframes by sample, for holding a
	// following gap's bumpers to what the parties' keyframes say there.
	keyframes map[string]map[int]PhysicalKeyframe
}

func (v *physicalValidator) claim(kind, id string, review PhysicalReview) error {
	if id == "" {
		return fmt.Errorf("a %s record has no id", kind)
	}
	if other, taken := v.ids[id]; taken {
		return fmt.Errorf("id %q names both a %s and a %s record", id, other, kind)
	}
	v.ids[id] = kind
	if v.origins == nil {
		v.origins = map[string]ReferenceOrigin{}
	}
	v.origins[kind+"/"+id] = review.Origin
	return nil
}

func (v *physicalValidator) object(o PhysicalObject) error {
	if o.Body != nil {
		if err := v.body(*o.Body); err != nil {
			return fmt.Errorf("body %q: %w", o.Body.BodyID, err)
		}
	}
	samples := map[int]PhysicalKeyframe{}
	for _, k := range o.Keyframes {
		if _, dup := samples[k.SampleID]; dup {
			return fmt.Errorf("two keyframes at sample %d", k.SampleID)
		}
		samples[k.SampleID] = k
		if err := v.keyframe(k, o.Body); err != nil {
			return fmt.Errorf("keyframe %q: %w", k.KeyframeID, err)
		}
	}
	v.keyframes[o.ObjectID] = samples
	return nil
}

// ownSample holds an observed component of a record at one instant to that
// instant: what was seen then must be cited from the frame it was seen in.
func ownSample(name string, status EvidenceStatus, s EvidenceSupport, sample int) error {
	if status != EvidenceObserved {
		return nil
	}
	for _, f := range s.Frames {
		if f == sample {
			return nil
		}
	}
	return fmt.Errorf("%s is observed but does not cite its own sample %d", name, sample)
}

func (v *physicalValidator) body(b BodyGeometry) error {
	if err := v.claim("body", b.BodyID, b.Review); err != nil {
		return err
	}
	if b.AxisConvention != BodyAxisConvention {
		return fmt.Errorf("axis convention %q, want %q", b.AxisConvention, BodyAxisConvention)
	}
	for _, d := range []struct {
		name string
		d    DimensionBound
	}{{"length", b.Length}, {"width", b.Width}, {"height", b.Height}} {
		if err := v.dimension(d.d); err != nil {
			return fmt.Errorf("%s: %w", d.name, err)
		}
	}
	known := b.Length.Status != EvidenceUnknown || b.Width.Status != EvidenceUnknown || b.Height.Status != EvidenceUnknown
	return v.review(b.Review, known)
}

func (v *physicalValidator) dimension(d DimensionBound) error {
	if !d.Status.valid() {
		return fmt.Errorf("status %q", d.Status)
	}
	if d.Status == EvidenceUnknown {
		if d.Span != "" || d.LowerM != nil || d.UpperM != nil || d.ValueM != nil {
			return fmt.Errorf("an unknown dimension carries no span or value")
		}
		return v.support(d.Status, d.Support)
	}
	if d.LowerM == nil || !finiteNonNegative(*d.LowerM) {
		return fmt.Errorf("a known dimension needs a finite, non-negative lower_m")
	}
	switch d.Span {
	case SpanFull:
		if d.UpperM == nil || !finite(*d.UpperM) || *d.UpperM < *d.LowerM {
			return fmt.Errorf("a full span needs a finite upper_m no less than lower_m")
		}
		if d.ValueM != nil && (!finite(*d.ValueM) || *d.ValueM < *d.LowerM || *d.ValueM > *d.UpperM) {
			return fmt.Errorf("value_m must lie within [lower_m, upper_m]")
		}
	case SpanPartial:
		// A partial span shows that the body is at least this long. The
		// rest is unseen, and an upper bound or a value would claim it.
		if d.UpperM != nil || d.ValueM != nil {
			return fmt.Errorf("a partial span supports only a lower bound: it cannot carry upper_m or value_m")
		}
		if d.Status != EvidenceObserved {
			return fmt.Errorf("a partial span is an observation, not %s", d.Status)
		}
	default:
		return fmt.Errorf("span %q (want %q or %q)", d.Span, SpanFull, SpanPartial)
	}
	return v.support(d.Status, d.Support)
}

func (v *physicalValidator) support(status EvidenceStatus, s EvidenceSupport) error {
	for i, f := range s.Frames {
		if f < 0 || f >= len(v.p.Samples) {
			return fmt.Errorf("supporting frame %d is not in the pack's %d samples", f, len(v.p.Samples))
		}
		if i > 0 && f <= s.Frames[i-1] {
			return fmt.Errorf("supporting frames must be sorted and unique: %d follows %d", f, s.Frames[i-1])
		}
	}
	switch status {
	case EvidenceObserved:
		if len(s.Frames) == 0 {
			return fmt.Errorf("an observed component names no supporting frames")
		}
	case EvidenceInferred:
		if len(s.Frames) == 0 && strings.TrimSpace(s.External) == "" {
			return fmt.Errorf("an inferred component names neither supporting frames nor an external reference")
		}
	case EvidencePriorOnly:
		if strings.TrimSpace(s.External) == "" {
			return fmt.Errorf("a prior-only component must name its prior as an external reference")
		}
	case EvidenceUnknown:
		if len(s.Frames) > 0 || s.External != "" {
			return fmt.Errorf("an unknown component has no support to name")
		}
	}
	return nil
}

func (v *physicalValidator) sample(sampleID int, timestampNs int64) error {
	if sampleID < 0 || sampleID >= len(v.p.Samples) {
		return fmt.Errorf("sample %d is not in the pack's %d samples", sampleID, len(v.p.Samples))
	}
	if want := v.p.Samples[sampleID].TimestampNs; timestampNs != want {
		return fmt.Errorf("sample %d was captured at %d ns, the record says %d", sampleID, want, timestampNs)
	}
	return nil
}

func (v *physicalValidator) keyframe(k PhysicalKeyframe, body *BodyGeometry) error {
	if err := v.claim("keyframe", k.KeyframeID, k.Review); err != nil {
		return err
	}
	if err := v.sample(k.SampleID, k.TimestampNs); err != nil {
		return err
	}
	if err := v.yaw(k.Yaw); err != nil {
		return fmt.Errorf("yaw: %w", err)
	}
	if err := v.anchor(k.Anchor, k.Yaw.Axis, body); err != nil {
		return fmt.Errorf("anchor: %w", err)
	}
	if err := v.position(k.Position); err != nil {
		return fmt.Errorf("position: %w", err)
	}
	for _, e := range []struct {
		name string
		e    EndpointEvidence
	}{{"front", k.Front}, {"rear", k.Rear}} {
		if !e.e.Status.valid() {
			return fmt.Errorf("%s: status %q", e.name, e.e.Status)
		}
		// Which end is the front is exactly what an unresolved axis does
		// not know, so neither end can be claimed.
		if e.e.Status != EvidenceUnknown && k.Yaw.Axis != AxisResolved {
			return fmt.Errorf("%s: a bumper cannot be named while the axis is %s", e.name, k.Yaw.Axis)
		}
		if err := v.support(e.e.Status, e.e.Support); err != nil {
			return fmt.Errorf("%s: %w", e.name, err)
		}
	}
	for _, c := range []struct {
		name   string
		status EvidenceStatus
		s      EvidenceSupport
	}{{"position", k.Position.Status, k.Position.Support}, {"yaw", k.Yaw.Status, k.Yaw.Support},
		{"front", k.Front.Status, k.Front.Support}, {"rear", k.Rear.Status, k.Rear.Support}} {
		if err := ownSample(c.name, c.status, c.s, k.SampleID); err != nil {
			return err
		}
	}
	for i, se := range k.SharedErrors {
		if strings.TrimSpace(se.Observation) == "" {
			return fmt.Errorf("shared error %d names no observation", i)
		}
		if len(se.Components) < 2 {
			return fmt.Errorf("shared error %q links fewer than two components", se.Observation)
		}
		for j, c := range se.Components {
			if !physicalComponents[c] {
				return fmt.Errorf("shared error %q names component %q", se.Observation, c)
			}
			if j > 0 && c <= se.Components[j-1] {
				return fmt.Errorf("shared error %q components must be sorted and unique", se.Observation)
			}
		}
	}
	known := k.Position.Status != EvidenceUnknown || k.Yaw.Status != EvidenceUnknown
	return v.review(k.Review, known)
}

func (v *physicalValidator) yaw(y YawBound) error {
	if !y.Status.valid() {
		return fmt.Errorf("status %q", y.Status)
	}
	switch y.Axis {
	case AxisUnknown:
		if y.Status != EvidenceUnknown || y.YawRad != nil || y.BoundRad != nil {
			return fmt.Errorf("an unknown axis carries no yaw and has status unknown")
		}
	case AxisResolved, AxisFrontRearAmbiguous:
		if y.Status == EvidenceUnknown {
			return fmt.Errorf("an axis stated %s needs a yaw status other than unknown", y.Axis)
		}
		if y.YawRad == nil || !finite(*y.YawRad) || y.BoundRad == nil || !finiteNonNegative(*y.BoundRad) || *y.BoundRad > math.Pi {
			return fmt.Errorf("a known yaw needs a finite yaw_rad and a bound_rad in [0, pi]")
		}
	default:
		return fmt.Errorf("axis %q", y.Axis)
	}
	return v.support(y.Status, y.Support)
}

func (v *physicalValidator) anchor(a PhysicalAnchor, axis AxisState, body *BodyGeometry) error {
	if !a.Kind.valid() {
		return fmt.Errorf("kind %q", a.Kind)
	}
	if a.Kind == AnchorBodyCentre {
		if a.OffsetM != nil || a.OffsetBoundM != nil {
			return fmt.Errorf("a body-centre anchor has no offset")
		}
		return nil
	}
	// A face is named relative to the front, which only a resolved axis
	// knows.
	if axis != AxisResolved {
		return fmt.Errorf("a %s cannot be named while the axis is %s", a.Kind, axis)
	}
	if (a.OffsetM == nil) != (a.OffsetBoundM == nil) {
		return fmt.Errorf("a face offset needs both offset_m and offset_bound_m")
	}
	if a.OffsetM == nil {
		return nil
	}
	if !finiteNonNegative(*a.OffsetM) || !finiteNonNegative(*a.OffsetBoundM) {
		return fmt.Errorf("a face offset must be finite and non-negative")
	}
	if body == nil {
		return nil
	}
	// The offset from a face to the centre is half the dimension across
	// that face; a declared offset the body's own bounds exclude is two
	// answers to one question.
	d, name := body.Length, "length"
	if a.Kind == AnchorLeftFace || a.Kind == AnchorRightFace {
		d, name = body.Width, "width"
	}
	lo, hi := *a.OffsetM-*a.OffsetBoundM, *a.OffsetM+*a.OffsetBoundM
	if d.LowerM != nil && hi < *d.LowerM/2 {
		return fmt.Errorf("offset %.3f±%.3f m is less than half the body's %s lower bound %.3f m", *a.OffsetM, *a.OffsetBoundM, name, *d.LowerM)
	}
	if d.UpperM != nil && lo > *d.UpperM/2 {
		return fmt.Errorf("offset %.3f±%.3f m is more than half the body's %s upper bound %.3f m", *a.OffsetM, *a.OffsetBoundM, name, *d.UpperM)
	}
	return nil
}

func (v *physicalValidator) position(pb PositionBound) error {
	if !pb.Status.valid() {
		return fmt.Errorf("status %q", pb.Status)
	}
	if pb.Status == EvidenceUnknown {
		if pb.XM != nil || pb.YM != nil || pb.ZM != nil || pb.BoundM != nil {
			return fmt.Errorf("an unknown position carries no value")
		}
		return v.support(pb.Status, pb.Support)
	}
	if pb.XM == nil || pb.YM == nil || !finite(*pb.XM) || !finite(*pb.YM) {
		return fmt.Errorf("a known position needs finite x_m and y_m")
	}
	if pb.ZM != nil && !finite(*pb.ZM) {
		return fmt.Errorf("z_m must be finite")
	}
	if pb.BoundM == nil || !finiteNonNegative(*pb.BoundM) {
		return fmt.Errorf("a known position needs a finite, non-negative bound_m")
	}
	return v.support(pb.Status, pb.Support)
}

func (v *physicalValidator) review(r PhysicalReview, declaresBounds bool) error {
	if !validStatus(r.Status) {
		return fmt.Errorf("review status %q", r.Status)
	}
	if strings.TrimSpace(r.Method) == "" {
		return fmt.Errorf("review names no method")
	}
	if strings.TrimSpace(r.Provenance.Author) == "" {
		return fmt.Errorf("review names no author")
	}
	switch r.Origin {
	case OriginIndependent:
		// Tracker output in the provenance is the tracker grading itself,
		// whatever the origin field says.
		if r.TrackerSource != "" || r.Provenance.Algorithm != "" {
			return fmt.Errorf("a record seeded from tracker output (source %q, algorithm %q) cannot claim independent provenance",
				r.TrackerSource, r.Provenance.Algorithm)
		}
	case OriginTrackerAssisted:
		if strings.TrimSpace(r.TrackerSource) == "" {
			return fmt.Errorf("a tracker-assisted record must name the tracker output it came from")
		}
	default:
		return fmt.Errorf("origin %q", r.Origin)
	}
	if r.ReviewedAgainst != nil {
		if r.Status != StatusReviewed {
			return fmt.Errorf("a %s record cannot carry the membership it was reviewed against", r.Status)
		}
		if r.ReviewedAgainst.Revision < 1 {
			return fmt.Errorf("reviewed_against names membership revision %d", r.ReviewedAgainst.Revision)
		}
	}
	if declaresBounds && strings.TrimSpace(r.UncertaintyAssumptions) == "" {
		return fmt.Errorf("a record that declares bounds must state its uncertainty assumptions")
	}
	return nil
}

func (v *physicalValidator) following(f FollowingReference) error {
	if err := v.claim("following", f.FollowingID, f.Review); err != nil {
		return err
	}
	if f.FollowerObjectID == "" {
		return fmt.Errorf("no follower object")
	}
	switch f.Decision {
	case FollowingLeader:
		if f.LeaderObjectID == "" || f.LeaderObjectID == f.FollowerObjectID {
			return fmt.Errorf("a leader decision names a leader other than the follower")
		}
	case FollowingNoLeader, FollowingAmbiguous:
		if f.LeaderObjectID != "" || len(f.Gaps) > 0 {
			return fmt.Errorf("a %s decision has no leader and no gap", f.Decision)
		}
	default:
		return fmt.Errorf("decision %q", f.Decision)
	}
	iv := f.Interval
	if iv.FirstSample < 0 || iv.LastSample < iv.FirstSample || iv.LastSample >= len(v.p.Samples) {
		return fmt.Errorf("interval [%d, %d] is not inside the pack's %d samples", iv.FirstSample, iv.LastSample, len(v.p.Samples))
	}
	if len(f.Gaps) > 0 && f.GapDefinition != GapAlongFollowerAxis {
		return fmt.Errorf("gap definition %q (want %q)", f.GapDefinition, GapAlongFollowerAxis)
	}
	if len(f.Gaps) == 0 && f.GapDefinition != "" {
		return fmt.Errorf("a gap definition with no gaps")
	}
	declares := false
	for i, g := range f.Gaps {
		if i > 0 && g.SampleID <= f.Gaps[i-1].SampleID {
			return fmt.Errorf("gaps must be in sample order, one per sample")
		}
		if !iv.Contains(g.SampleID) {
			return fmt.Errorf("gap at sample %d is outside the interval", g.SampleID)
		}
		if err := v.gap(g, f); err != nil {
			return fmt.Errorf("gap at sample %d: %w", g.SampleID, err)
		}
		declares = declares || g.Status != EvidenceUnknown
	}
	return v.review(f.Review, declares)
}

func (v *physicalValidator) gap(g FollowingGap, f FollowingReference) error {
	if err := v.sample(g.SampleID, g.TimestampNs); err != nil {
		return err
	}
	if !g.Status.valid() {
		return fmt.Errorf("status %q", g.Status)
	}
	for _, e := range []struct {
		name, party string
		front       bool
		e           EndpointEvidence
	}{{"follower_front", f.FollowerObjectID, true, g.FollowerFront}, {"leader_rear", f.LeaderObjectID, false, g.LeaderRear}} {
		if !e.e.Status.valid() {
			return fmt.Errorf("%s: status %q", e.name, e.e.Status)
		}
		if err := v.support(e.e.Status, e.e.Support); err != nil {
			return fmt.Errorf("%s: %w", e.name, err)
		}
		if err := ownSample(e.name, e.e.Status, e.e.Support, g.SampleID); err != nil {
			return err
		}
		// A gap is no better known than the bumper it is measured from.
		if g.Status.strength() > e.e.Status.strength() {
			return fmt.Errorf("a %s gap cannot rest on a %s %s", g.Status, e.e.Status, e.name)
		}
		if err := v.gapBumper(e.name, e.party, e.front, e.e.Status, g.SampleID); err != nil {
			return err
		}
	}
	if err := ownSample("gap", g.Status, g.Support, g.SampleID); err != nil {
		return err
	}
	if g.Status == EvidenceUnknown {
		if g.LowerM != nil || g.UpperM != nil || g.ValueM != nil {
			return fmt.Errorf("an unknown gap carries no value")
		}
		return v.support(g.Status, g.Support)
	}
	if g.LowerM == nil || g.UpperM == nil || !finiteNonNegative(*g.LowerM) || !finite(*g.UpperM) || *g.UpperM < *g.LowerM {
		return fmt.Errorf("a known gap needs finite, non-negative lower_m and upper_m in order")
	}
	if g.ValueM != nil && (!finite(*g.ValueM) || *g.ValueM < *g.LowerM || *g.ValueM > *g.UpperM) {
		return fmt.Errorf("value_m must lie within [lower_m, upper_m]")
	}
	return v.support(g.Status, g.Support)
}

// gapBumper holds a gap's bumper to its party's own keyframe at that sample,
// as a keyframe's own bumpers are held to its axis: a bumper the keyframe
// cannot name, because its axis is unresolved, or states less well, cannot be
// better known in a gap; and an observed bumper needs the keyframe that saw
// it.
func (v *physicalValidator) gapBumper(name, party string, front bool, status EvidenceStatus, sample int) error {
	if status == EvidenceUnknown {
		return nil
	}
	k, ok := v.keyframes[party][sample]
	if !ok {
		if status == EvidenceObserved {
			return fmt.Errorf("an observed %s needs %q's keyframe at sample %d", name, party, sample)
		}
		return nil
	}
	if k.Yaw.Axis != AxisResolved {
		return fmt.Errorf("%s cannot be named: %q's keyframe at sample %d has a %s axis", name, party, sample, k.Yaw.Axis)
	}
	stated := k.Rear.Status
	if front {
		stated = k.Front.Status
	}
	if status.strength() > stated.strength() {
		return fmt.Errorf("a %s %s cannot rest on %q's keyframe at sample %d, which states that bumper %s", status, name, party, sample, stated)
	}
	return nil
}
