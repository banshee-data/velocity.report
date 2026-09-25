package annotation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Reference positions for per-frame tracking evaluation.
//
// A mask is a set of returns; a tracker reports a point. Scoring one against
// the other needs a declared rule for where the object is in a frame, and a
// declared rule for which masks a human certified. Both are policy, both move
// the numbers, and both are therefore recorded with every result.
//
// Position. The footprint centre is the centre of the mask's horizontal
// axis-aligned extent; the point mean is the mean of its returns. Neither is
// the physical centre: both sit towards the faces the sensor saw. The
// footprint centre is the default because it is less pulled by point density,
// which varies with range and aspect for reasons unrelated to where the
// object is. The point mean is what the annotation-scored tuning scripts use
// (data/explore/annotation-scored-tuning/gt_extract.py), kept so their numbers
// can be reproduced. A reviewed physical pose would be better than either;
// the client does not yet write one.
//
// Footprint. The horizontal extent of the same returns, and its diagonal,
// which the footprint gate turns into a per-object tolerance: a bus's centre
// moves further under a partial view than a pedestrian's.
//
// What is certifiable truth, and what is ignored. MOT16's distractor rule: an
// object a human cannot certify is neither missed nor found. A mask is scored
// only if every one of these holds, and otherwise becomes an ignore point at
// the position its returns give:
//
//   - its object and the mask itself pass the status policy: both reviewed by
//     default; reviewed or proposed when proposals are included by request;
//   - the object's class is a road user;
//   - the object was visible in the frame (present or partly occluded);
//   - the mask has certain members, not only returns marked uncertain;
//   - its completeness was stated: never "unreviewed", and not "partial"
//     unless the policy scores partial masks.
//
// A rejected object or mask is dropped outright: a person examined it and
// found nothing there. A mask with no returns at all is dropped too, because
// it has no position to absorb a hypothesis at.
//
// Partial masks are scored by default because the macOS client saves every
// mask as partial unless the operator changes it. Treating partial as
// uncertifiable would empty the reference of almost every reviewed mask; a
// reviewed partial mask still certifies that the object was there, only not
// every return it had.

// ReferenceStatus is the review-status policy.
type ReferenceStatus string

const (
	// ReferenceReviewedOnly certifies only a reviewed mask of a reviewed
	// object: the point-annotation tool's "what counts".
	ReferenceReviewedOnly ReferenceStatus = "reviewed_only"
	// ReferenceIncludeProposed also scores proposed masks and objects. It is
	// never the default, and a result made with it says so.
	ReferenceIncludeProposed ReferenceStatus = "reviewed_and_proposed"
)

// ReferencePosition is the rule for an object's position in a frame.
type ReferencePosition string

const (
	PositionFootprintCentre ReferencePosition = "footprint_centre"
	PositionPointMean       ReferencePosition = "point_mean"
)

// IgnoreReason says why a reference point is not certifiable. Empty means it
// is scored.
type IgnoreReason string

const (
	IgnoreUnreviewed          IgnoreReason = "unreviewed"
	IgnoreNotRoadUser         IgnoreReason = "not_road_user"
	IgnoreVisibility          IgnoreReason = "visibility"
	IgnoreUncertainMembership IgnoreReason = "uncertain_membership"
	IgnoreIncompleteMask      IgnoreReason = "incomplete_mask"
)

// RoadUserClasses are the seven production labels the client offers for
// moving road users. Everything else it offers (ground, building, sign,
// vegetation, noise) describes the street.
func RoadUserClasses() []string {
	return []string{"bus", "car", "cyclist", "motorcycle", "pedestrian", "truck", "van"}
}

// ReferencePolicy is every choice that decides the reference, recorded with
// every result made from it.
type ReferencePolicy struct {
	Status            ReferenceStatus   `json:"status"`
	Position          ReferencePosition `json:"position"`
	ScorePartialMasks bool              `json:"score_partial_masks"`
	// RoadUserClasses is sorted; classes are compared lower-cased.
	RoadUserClasses []string `json:"road_user_classes"`
}

// DefaultReferencePolicy is reviewed-only truth at the footprint centre.
func DefaultReferencePolicy() ReferencePolicy {
	return ReferencePolicy{
		Status:            ReferenceReviewedOnly,
		Position:          PositionFootprintCentre,
		ScorePartialMasks: true,
		RoadUserClasses:   RoadUserClasses(),
	}
}

// Validate refuses a policy that names no rule it can apply.
func (p ReferencePolicy) Validate() error {
	if p.Status != ReferenceReviewedOnly && p.Status != ReferenceIncludeProposed {
		return fmt.Errorf("reference status policy %q (want %q or %q)", p.Status, ReferenceReviewedOnly, ReferenceIncludeProposed)
	}
	if p.Position != PositionFootprintCentre && p.Position != PositionPointMean {
		return fmt.Errorf("reference position %q (want %q or %q)", p.Position, PositionFootprintCentre, PositionPointMean)
	}
	if len(p.RoadUserClasses) == 0 {
		return fmt.Errorf("reference policy names no road-user classes")
	}
	return nil
}

// ReferencePoint is one object's reference in one frame.
type ReferencePoint struct {
	ObjectID    string  `json:"object_id"`
	Class       string  `json:"class"`
	SampleID    int     `json:"sample_id"`
	TimestampNs int64   `json:"timestamp_ns"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	// FootprintX and FootprintY are the horizontal axis-aligned extent of
	// the returns the position was taken from, in the pack's frame.
	FootprintX        float64 `json:"footprint_x_m"`
	FootprintY        float64 `json:"footprint_y_m"`
	FootprintDiagonal float64 `json:"footprint_diagonal_m"`
	// Returns is how many returns the position was taken from.
	Returns int `json:"returns"`
	// Ignore is empty for a scored point.
	Ignore IgnoreReason `json:"ignore,omitempty"`
}

// ReferenceSummary counts what the policy did to the sidecar's masks.
type ReferenceSummary struct {
	Masks           int                  `json:"masks"`
	Scored          int                  `json:"scored"`
	Ignored         map[IgnoreReason]int `json:"ignored"`
	DroppedRejected int                  `json:"dropped_rejected"`
	DroppedEmpty    int                  `json:"dropped_empty"`
}

// BuildReference turns every mask in the sidecar into a reference point under
// the policy, in (sample, object) order. The sidecar is validated against the
// pack first, so an index that does not exist in the point domain is an error
// rather than a panic or a silently wrong position.
func BuildReference(p *Pack, s *Sidecar, policy ReferencePolicy) ([]ReferencePoint, ReferenceSummary, error) {
	summary := ReferenceSummary{Ignored: map[IgnoreReason]int{}}
	if err := policy.Validate(); err != nil {
		return nil, summary, err
	}
	if err := s.Validate(p); err != nil {
		return nil, summary, fmt.Errorf("annotation does not validate against its pack: %w", err)
	}

	roadUser := make(map[string]bool, len(policy.RoadUserClasses))
	for _, c := range policy.RoadUserClasses {
		roadUser[strings.ToLower(strings.TrimSpace(c))] = true
	}
	objects := make(map[string]Object, len(s.Objects))
	for _, o := range s.Objects {
		objects[o.ObjectID] = o
	}

	order := make([]int, len(s.Masks))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		ma, mb := s.Masks[order[a]], s.Masks[order[b]]
		if ma.SampleID != mb.SampleID {
			return ma.SampleID < mb.SampleID
		}
		return ma.ObjectID < mb.ObjectID
	})

	var (
		out     []ReferencePoint
		decoded = -1
		points  Points
	)
	for _, i := range order {
		m := s.Masks[i]
		obj := objects[m.ObjectID]
		summary.Masks++

		if obj.Status == StatusRejected || m.Status == StatusRejected {
			summary.DroppedRejected++
			continue
		}
		indices, reason := m.PointIndices, IgnoreReason("")
		if len(indices) == 0 {
			indices, reason = m.UncertainIndices, IgnoreUncertainMembership
		}
		if len(indices) == 0 {
			summary.DroppedEmpty++
			continue
		}

		if m.SampleID != decoded {
			var err error
			if points, err = p.PointsAt(m.SampleID); err != nil {
				return nil, summary, fmt.Errorf("mask %s sample %d: %w", m.ObjectID, m.SampleID, err)
			}
			decoded = m.SampleID
		}
		ref := referencePoint(points, indices, policy.Position)
		ref.ObjectID, ref.Class, ref.SampleID = m.ObjectID, obj.Class, m.SampleID
		ref.TimestampNs = p.Samples[m.SampleID].TimestampNs

		// First failing condition names the reason; the order is fixed so
		// the counts are reproducible.
		switch {
		case !policy.admits(obj.Status, m.Status):
			ref.Ignore = IgnoreUnreviewed
		case !roadUser[strings.ToLower(strings.TrimSpace(obj.Class))]:
			ref.Ignore = IgnoreNotRoadUser
		case m.Visibility != VisiblePresent && m.Visibility != VisiblePartlyOccluded:
			ref.Ignore = IgnoreVisibility
		case reason != "":
			ref.Ignore = reason
		case m.Completeness == MaskUnreviewed || (m.Completeness == MaskPartial && !policy.ScorePartialMasks):
			ref.Ignore = IgnoreIncompleteMask
		}
		if ref.Ignore == "" {
			summary.Scored++
		} else {
			summary.Ignored[ref.Ignore]++
		}
		out = append(out, ref)
	}
	return out, summary, nil
}

func (p ReferencePolicy) admits(object, mask ReviewStatus) bool {
	if p.Status == ReferenceIncludeProposed {
		ok := func(s ReviewStatus) bool { return s == StatusReviewed || s == StatusProposed }
		return ok(object) && ok(mask)
	}
	return object == StatusReviewed && mask == StatusReviewed
}

// referencePoint takes the position and footprint of the given returns.
// Indices are valid: the sidecar was validated against the pack.
func referencePoint(pts Points, indices []int, position ReferencePosition) ReferencePoint {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	var sumX, sumY float64
	for _, i := range indices {
		x, y := float64(pts.X[i]), float64(pts.Y[i])
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
		sumX += x
		sumY += y
	}
	n := float64(len(indices))
	ref := ReferencePoint{
		FootprintX: maxX - minX,
		FootprintY: maxY - minY,
		Returns:    len(indices),
	}
	ref.FootprintDiagonal = math.Hypot(ref.FootprintX, ref.FootprintY)
	if position == PositionPointMean {
		ref.X, ref.Y = sumX/n, sumY/n
	} else {
		ref.X, ref.Y = (minX+maxX)/2, (minY+maxY)/2
	}
	return ref
}
