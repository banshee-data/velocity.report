package annotation

import (
	"errors"
	"fmt"
	"math"
)

// Links between physical references and the membership sidecar.
//
// A physical reference names its object by the sidecar's ID and its evidence
// by frames of the pack. Both have to hold against the masks: the object must
// be declared and not rejected, every cited frame must hold returns of the
// object, and every observed claim must be one the returns can bear out. A
// dimension observed along the body is measured along a keyframe's axis at a
// cited frame; a bumper observed at a keyframe must be reached by that
// keyframe's own returns. A claim the returns cannot test is refused as
// observed; state it as inferred instead.
//
// These checks read the sidecar, which the macOS client and other writers
// save without reading the references, so a later membership edit can
// invalidate references that held when they were saved. LinkProblems lists
// every such record rather than stopping at the first, which is what a load
// reports and what a repair has to address.

// LinkProblem is one record that does not hold against a membership
// revision, and why.
type LinkProblem struct {
	Record  string `json:"record"`
	Problem string `json:"problem"`
}

func (l LinkProblem) String() string { return l.Record + ": " + l.Problem }

// ValidateLinks refuses references that do not hold against a sidecar for
// this pack, naming every record that fails.
func (r *PhysicalReferenceSet) ValidateLinks(p *Pack, s *Sidecar) error {
	if s.PackDigest != r.PackDigest {
		return fmt.Errorf("annotation was written against pack %s, the physical references against %s", s.PackDigest, r.PackDigest)
	}
	problems := r.LinkProblems(p, s)
	if len(problems) == 0 {
		return nil
	}
	errs := make([]error, len(problems))
	for i, lp := range problems {
		errs[i] = errors.New(lp.String())
	}
	return fmt.Errorf("physical references do not hold against annotation revision %d: %w", s.Revision, errors.Join(errs...))
}

// LinkProblems lists every record that does not hold against the sidecar,
// the first problem of each, in document order.
func (r *PhysicalReferenceSet) LinkProblems(p *Pack, s *Sidecar) []LinkProblem {
	status := make(map[string]ReviewStatus, len(s.Objects))
	for _, o := range s.Objects {
		status[o.ObjectID] = o.Status
	}
	c := linkChecker{p: p, returns: map[string]map[int][]int{}, points: map[int]Points{}}
	for _, m := range s.Masks {
		if m.Status == StatusRejected || len(m.PointIndices) == 0 {
			continue
		}
		if c.returns[m.ObjectID] == nil {
			c.returns[m.ObjectID] = map[int][]int{}
		}
		c.returns[m.ObjectID][m.SampleID] = m.PointIndices
	}
	declared := func(what, id string) error {
		switch st, ok := status[id]; {
		case !ok:
			return fmt.Errorf("%s %q is not an object of annotation revision %d: declare it there first", what, id, s.Revision)
		case st == StatusRejected:
			return fmt.Errorf("%s %q is rejected in annotation revision %d", what, id, s.Revision)
		}
		return nil
	}
	var out []LinkProblem
	add := func(record string, err error) {
		if err != nil {
			out = append(out, LinkProblem{Record: record, Problem: err.Error()})
		}
	}
	for _, o := range r.Objects {
		record := fmt.Sprintf("object %q", o.ObjectID)
		if err := declared("object", o.ObjectID); err != nil {
			add(record, err)
			continue
		}
		if o.Body != nil {
			add(fmt.Sprintf("%s body %q", record, o.Body.BodyID), c.body(o))
		}
		for _, k := range o.Keyframes {
			add(fmt.Sprintf("%s keyframe %q", record, k.KeyframeID), c.keyframe(o, k))
		}
	}
	for _, f := range r.Following {
		err := declared("follower", f.FollowerObjectID)
		if err == nil && f.LeaderObjectID != "" {
			err = declared("leader", f.LeaderObjectID)
		}
		if err == nil {
			err = c.following(f)
		}
		add(fmt.Sprintf("following %q", f.FollowingID), err)
	}
	return out
}

type linkChecker struct {
	p *Pack
	// returns are each object's certain mask members by sample.
	returns map[string]map[int][]int
	points  map[int]Points
}

func (c *linkChecker) pointsAt(sample int) (Points, error) {
	if pts, ok := c.points[sample]; ok {
		return pts, nil
	}
	pts, err := c.p.PointsAt(sample)
	if err == nil {
		c.points[sample] = pts
	}
	return pts, err
}

// present refuses a cited frame in which the object has no returns: a frame
// cannot show what is not in it.
func (c *linkChecker) present(object, what string, s EvidenceSupport) error {
	for _, f := range s.Frames {
		if _, ok := c.returns[object][f]; !ok {
			return fmt.Errorf("%s cites frame %d, where %q has no returns in its mask", what, f, object)
		}
	}
	return nil
}

// body holds each dimension's frames to the object's returns, and an
// observed dimension to what those returns span: along a keyframe's axis for
// length and width, and in height for height. An observed dimension none of
// whose frames can measure it is refused.
func (c *linkChecker) body(o PhysicalObject) error {
	yaws := map[int]float64{}
	for _, k := range o.Keyframes {
		if k.Yaw.YawRad != nil {
			yaws[k.SampleID] = *k.Yaw.YawRad
		}
	}
	for _, d := range []struct {
		name string
		d    DimensionBound
		axis int // 0 length, 1 width, 2 height
	}{{"length", o.Body.Length, 0}, {"width", o.Body.Width, 1}, {"height", o.Body.Height, 2}} {
		if err := c.present(o.ObjectID, d.name, d.d.Support); err != nil {
			return err
		}
		if d.d.Status != EvidenceObserved {
			continue
		}
		best, checked := 0.0, false
		for _, f := range d.d.Support.Frames {
			yaw, hasYaw := yaws[f]
			if d.axis != 2 && !hasYaw {
				continue
			}
			pts, err := c.pointsAt(f)
			if err != nil {
				return err
			}
			best, checked = math.Max(best, returnSpan(pts, c.returns[o.ObjectID][f], d.axis, yaw)), true
		}
		if !checked {
			return fmt.Errorf("observed %s cites no frame with a keyframe yaw to measure it along: it cannot be checked, so state it as inferred", d.name)
		}
		if best+ObservedSpanSlackM < *d.d.LowerM {
			suffix := ""
			if d.d.Span == SpanFull {
				suffix = ": a partial span supports only a lower bound"
			}
			return fmt.Errorf("observed %s %s is at least %.3f m, but its supporting frames' returns span at most %.3f m%s",
				d.d.Span, d.name, *d.d.LowerM, best, suffix)
		}
	}
	return nil
}

// keyframe holds every component's frames to the object's returns, and each
// observed bumper to the returns at the keyframe's own sample.
func (c *linkChecker) keyframe(o PhysicalObject, k PhysicalKeyframe) error {
	for _, comp := range []struct {
		name string
		s    EvidenceSupport
	}{{"position", k.Position.Support}, {"yaw", k.Yaw.Support}, {"front", k.Front.Support}, {"rear", k.Rear.Support}} {
		if err := c.present(o.ObjectID, comp.name, comp.s); err != nil {
			return err
		}
	}
	for _, b := range []struct {
		name string
		e    EndpointEvidence
		sign float64
		face AnchorKind
	}{{"front", k.Front, 1, AnchorFrontFace}, {"rear", k.Rear, -1, AnchorRearFace}} {
		if b.e.Status == EvidenceObserved {
			if err := c.bumper(o, k, b.name, b.sign, b.face); err != nil {
				return err
			}
		}
	}
	return nil
}

// bumper checks an observed bumper against the returns at the keyframe's own
// sample: projected on the keyframe's axis they must reach that end of the
// body, to within the slack. The end is the anchor when the anchor is that
// face, and otherwise half the body's lower-bound length from the centre,
// which is the least the claim needs. A bumper with neither cannot be
// checked, and is refused as observed. Validate has already required a
// resolved axis for any named bumper.
func (c *linkChecker) bumper(o PhysicalObject, k PhysicalKeyframe, name string, sign float64, face AnchorKind) error {
	if k.Position.XM == nil || k.Position.YM == nil {
		return fmt.Errorf("observed %s cannot be checked: the keyframe states no position", name)
	}
	ux, uy := math.Cos(*k.Yaw.YawRad), math.Sin(*k.Yaw.YawRad)
	var end float64
	if k.Anchor.Kind == face {
		end = *k.Position.XM*ux + *k.Position.YM*uy
	} else {
		cx, cy, ok := keyframeCentre(k)
		if !ok {
			return fmt.Errorf("observed %s cannot be checked: the keyframe states neither that face nor the body centre", name)
		}
		if o.Body == nil || o.Body.Length.LowerM == nil {
			return fmt.Errorf("observed %s cannot be checked: the body states no length to place it by", name)
		}
		end = cx*ux + cy*uy + sign**o.Body.Length.LowerM/2
	}
	pts, err := c.pointsAt(k.SampleID)
	if err != nil {
		return err
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, i := range c.returns[o.ObjectID][k.SampleID] {
		s := float64(pts.X[i])*ux + float64(pts.Y[i])*uy
		lo, hi = math.Min(lo, s), math.Max(hi, s)
	}
	if sign > 0 && hi < end-ObservedSpanSlackM {
		return fmt.Errorf("observed front: the returns at sample %d reach %.3f m along the axis, short of the front at %.3f m", k.SampleID, hi, end)
	}
	if sign < 0 && lo > end+ObservedSpanSlackM {
		return fmt.Errorf("observed rear: the returns at sample %d reach back to %.3f m along the axis, short of the rear at %.3f m", k.SampleID, lo, end)
	}
	return nil
}

// following holds a gap's frames to the parties it cites: each bumper's
// frames to its own party, and the gap's frames to both.
func (c *linkChecker) following(f FollowingReference) error {
	for _, g := range f.Gaps {
		what := fmt.Sprintf("gap at sample %d", g.SampleID)
		if err := c.present(f.FollowerObjectID, what+" follower_front", g.FollowerFront.Support); err != nil {
			return err
		}
		if err := c.present(f.LeaderObjectID, what+" leader_rear", g.LeaderRear.Support); err != nil {
			return err
		}
		for _, party := range []string{f.FollowerObjectID, f.LeaderObjectID} {
			if err := c.present(party, what, g.Support); err != nil {
				return err
			}
		}
	}
	return nil
}

// keyframeCentre is the body centre a keyframe with a position states: the
// position for a body-centre anchor, or a face moved by its declared offset
// along the face's inward normal.
func keyframeCentre(k PhysicalKeyframe) (x, y float64, ok bool) {
	x, y = *k.Position.XM, *k.Position.YM
	if k.Anchor.Kind == AnchorBodyCentre {
		return x, y, true
	}
	if k.Anchor.OffsetM == nil || k.Yaw.YawRad == nil {
		return 0, 0, false
	}
	nx, ny := inwardNormal(k.Anchor.Kind, *k.Yaw.YawRad)
	return x + *k.Anchor.OffsetM*nx, y + *k.Anchor.OffsetM*ny, true
}

// inwardNormal is the unit vector from a named face towards the body centre.
func inwardNormal(face AnchorKind, yaw float64) (nx, ny float64) {
	ux, uy := math.Cos(yaw), math.Sin(yaw)
	switch face {
	case AnchorFrontFace:
		return -ux, -uy
	case AnchorLeftFace:
		return uy, -ux
	case AnchorRightFace:
		return -uy, ux
	}
	return ux, uy // from the rear face, inward is forward
}

// returnSpan is the extent of the given returns along the body's length
// (axis 0) or width (axis 1) at a yaw, or in height (axis 2).
func returnSpan(pts Points, indices []int, axis int, yaw float64) float64 {
	ux, uy := math.Cos(yaw), math.Sin(yaw)
	if axis == 1 {
		ux, uy = -uy, ux
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, i := range indices {
		v := float64(pts.Z[i])
		if axis != 2 {
			v = float64(pts.X[i])*ux + float64(pts.Y[i])*uy
		}
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return hi - lo
}
