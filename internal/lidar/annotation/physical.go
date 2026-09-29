package annotation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// Physical references (docs/plans/lidar-physical-reference-review-plan.md).
//
// A mask says which returns belong to an object. A physical reference says
// what is known about the object's body: where its centre or a named face
// was, which way it faced, how long, wide and high it is, and how each of
// those is known. The two are kept in separate files with separate reviews,
// because a reviewed mask establishes membership and nothing about extent:
// the minimum and maximum of a partial mask is not a vehicle.
//
// The record is linked rather than embedded. The sidecar's optional Pose is
// left as it was: its confidence scalar is not a positional or dimensional
// bound, and widening it would change a file the macOS client writes. A
// physical reference names its pack by digest and its object by the
// sidecar's object ID, and carries its own revision history beside the
// sidecar's.
//
// Every quantity carries an evidence status. Observed means the sensor saw
// it, in named frames. Inferred means it follows from named frames or an
// external reference. Prior-only means it is a class prior, named. Unknown
// carries no value at all. A partial span is a lower bound: a person drawing
// a plausible box does not make its far faces measured.
//
// Bounds are conservative half-widths or intervals under the record's stated
// uncertainty assumptions. Quantities derived from several of them, such as a
// bumper from a centre, a length and a yaw, are bounded by linear, worst-case
// propagation, which holds whatever the correlation between their errors. A
// keyframe may also name the observation several of its components share, so
// a reader can see that separate fields are not independent evidence.

// PhysicalReferenceSchema names the document kind.
const PhysicalReferenceSchema = "velocity.report/physical-reference"

// PhysicalReferenceSchemaVersion is the record layout version.
const PhysicalReferenceSchemaVersion = 1

// BodyAxisConvention is the only body-axis convention version 1 reads: x
// towards the body's front, y towards its left, z up. Yaw is the direction
// of +x in the pack's coordinate frame, anticlockwise from its x axis.
const BodyAxisConvention = "x_front_y_left_z_up"

// PhysicalUnits is the only linear unit version 1 reads. Angles are radians
// and say so in their field names.
const PhysicalUnits = "metres"

// ObservedSpanSlackM is how far a supporting frame's returns may fall short
// of an observed full dimension's lower bound before the claim is refused.
// Sparse returns at range understate a span, so the slack is generous; a
// claim that exceeds its own evidence by more than this is a partial span
// presented as a full one.
const ObservedSpanSlackM = 0.5

// EvidenceStatus is how one component is known.
type EvidenceStatus string

const (
	EvidenceObserved  EvidenceStatus = "observed"
	EvidenceInferred  EvidenceStatus = "inferred"
	EvidencePriorOnly EvidenceStatus = "prior_only"
	EvidenceUnknown   EvidenceStatus = "unknown"
)

// strength orders statuses from none to direct observation, so a derived
// quantity can be held to its weakest input.
func (s EvidenceStatus) strength() int {
	switch s {
	case EvidenceObserved:
		return 3
	case EvidenceInferred:
		return 2
	case EvidencePriorOnly:
		return 1
	}
	return 0
}

func (s EvidenceStatus) valid() bool { return s.strength() > 0 || s == EvidenceUnknown }

// Scorable reports whether a component with this status is independent
// evidence about the object: observed or inferred. A class prior is evidence
// about the class, and unknown is no evidence.
func (s EvidenceStatus) Scorable() bool { return s == EvidenceObserved || s == EvidenceInferred }

// ReferenceOrigin says whether a record was authored without tracker output.
type ReferenceOrigin string

const (
	// OriginIndependent was authored from raw evidence or an independent
	// measurement, without the tracker's boxes in view.
	OriginIndependent ReferenceOrigin = "independent"
	// OriginTrackerAssisted was seeded from, copied from or adjusted
	// against tracker output. It never becomes independent: not by review,
	// not by editing, not by deleting it and adding it back.
	OriginTrackerAssisted ReferenceOrigin = "tracker_assisted"
)

// AnchorKind names the point a keyframe's position refers to.
type AnchorKind string

const (
	AnchorBodyCentre AnchorKind = "body_centre"
	AnchorFrontFace  AnchorKind = "front_face"
	AnchorRearFace   AnchorKind = "rear_face"
	AnchorLeftFace   AnchorKind = "left_face"
	AnchorRightFace  AnchorKind = "right_face"
)

func (a AnchorKind) valid() bool {
	switch a {
	case AnchorBodyCentre, AnchorFrontFace, AnchorRearFace, AnchorLeftFace, AnchorRightFace:
		return true
	}
	return false
}

// AxisState says what is known of the body's orientation.
type AxisState string

const (
	// AxisResolved knows the axis and which end is the front.
	AxisResolved AxisState = "resolved"
	// AxisFrontRearAmbiguous knows the axis to within half a turn.
	AxisFrontRearAmbiguous AxisState = "front_rear_ambiguous"
	// AxisUnknown knows neither.
	AxisUnknown AxisState = "unknown"
)

// DimensionSpan says whether a dimension's evidence covered it end to end.
type DimensionSpan string

const (
	SpanFull    DimensionSpan = "full"
	SpanPartial DimensionSpan = "partial"
)

// FollowingDecision is the reviewed answer to "what is this object
// following", including the answers that are not a leader.
type FollowingDecision string

const (
	FollowingLeader    FollowingDecision = "leader"
	FollowingNoLeader  FollowingDecision = "no_leader"
	FollowingAmbiguous FollowingDecision = "ambiguous"
)

// GapAlongFollowerAxis is the only gap definition version 1 reads: the
// leader's rear bumper minus the follower's front bumper, measured along the
// follower's body axis at that instant. It is a straight chord, not the
// along-path arc the headway metric uses; the two agree on a straight road
// and must never be quoted as each other on a bend.
const GapAlongFollowerAxis = "along_follower_axis"

// Components a shared error may name.
var physicalComponents = map[string]bool{
	"position": true, "yaw": true, "anchor_offset": true,
	"length": true, "width": true, "height": true, "front": true, "rear": true,
}

// EvidenceSupport names what a component rests on: frames of this pack, or
// an external reference such as a measurement or a named class prior.
type EvidenceSupport struct {
	Frames   []int  `json:"frames,omitempty"`
	External string `json:"external,omitempty"`
}

// DimensionBound is one body dimension. A full span is an interval; a
// partial span is a lower bound only, whatever the operator believes the
// rest to be.
type DimensionBound struct {
	Status  EvidenceStatus  `json:"status"`
	Span    DimensionSpan   `json:"span,omitempty"`
	LowerM  *float64        `json:"lower_m,omitempty"`
	UpperM  *float64        `json:"upper_m,omitempty"`
	ValueM  *float64        `json:"value_m,omitempty"`
	Support EvidenceSupport `json:"support"`
}

// Bounded reports whether the dimension is an interval, not a lower bound.
func (d DimensionBound) Bounded() bool { return d.LowerM != nil && d.UpperM != nil }

// Best is the dimension's stated value, or its interval's midpoint, and the
// half-width that covers the interval from it. ok is false unless bounded.
func (d DimensionBound) Best() (value, halfWidth float64, ok bool) {
	if !d.Bounded() {
		return 0, 0, false
	}
	lo, hi := *d.LowerM, *d.UpperM
	value = (lo + hi) / 2
	if d.ValueM != nil {
		value = *d.ValueM
	}
	return value, math.Max(value-lo, hi-value), true
}

// BodyGeometry is an object's persistent body belief, apart from any one
// keyframe's pose. A change of visible face changes the evidence, not the
// body; a revised belief is a new body record under a new ID.
type BodyGeometry struct {
	BodyID         string         `json:"body_id"`
	AxisConvention string         `json:"axis_convention"`
	Length         DimensionBound `json:"length"`
	Width          DimensionBound `json:"width"`
	Height         DimensionBound `json:"height"`
	Review         PhysicalReview `json:"review"`
}

// PhysicalAnchor is the explicit point a keyframe's position refers to: the
// body centre, or the centre of a named face with a declared distance from
// that face to the body centre along the face's inward normal. A face with
// no declared offset locates that face and not the centre.
type PhysicalAnchor struct {
	Kind         AnchorKind `json:"kind"`
	OffsetM      *float64   `json:"offset_m,omitempty"`
	OffsetBoundM *float64   `json:"offset_bound_m,omitempty"`
}

// PositionBound is the anchor's position in the pack's frame. BoundM is a
// horizontal radius; ZM is optional and unbounded.
type PositionBound struct {
	Status  EvidenceStatus  `json:"status"`
	XM      *float64        `json:"x_m,omitempty"`
	YM      *float64        `json:"y_m,omitempty"`
	ZM      *float64        `json:"z_m,omitempty"`
	BoundM  *float64        `json:"bound_m,omitempty"`
	Support EvidenceSupport `json:"support"`
}

// YawBound is the body's orientation. Under an ambiguous axis the yaw is
// known only modulo half a turn, and nothing may be called the front.
type YawBound struct {
	Status   EvidenceStatus  `json:"status"`
	Axis     AxisState       `json:"axis"`
	YawRad   *float64        `json:"yaw_rad,omitempty"`
	BoundRad *float64        `json:"bound_rad,omitempty"`
	Support  EvidenceSupport `json:"support"`
}

// EndpointEvidence is how one bumper is known. Its position is derived from
// the keyframe's anchor, yaw and the body's length, never stated apart from
// them, so a bumper cannot be placed where its body is not.
type EndpointEvidence struct {
	Status  EvidenceStatus  `json:"status"`
	Support EvidenceSupport `json:"support"`
}

// SharedError names one observation that several components rest on, such
// as a rear-face fit that places the anchor and bounds the length. It makes
// the dependence visible; the bounds themselves are already combined
// conservatively.
type SharedError struct {
	Observation string   `json:"observation"`
	Components  []string `json:"components"`
	Note        string   `json:"note,omitempty"`
}

// PhysicalReview is a record's own review and provenance, separate from the
// membership review of the masks. Reviewing a mask does not review a pose.
type PhysicalReview struct {
	Status ReviewStatus    `json:"status"`
	Origin ReferenceOrigin `json:"origin"`
	// Method is how the record was made: "manual_box", "surveyed",
	// "import:<tool>" and so on.
	Method string `json:"method"`
	// TrackerSource names the tracker output a tracker-assisted record was
	// seeded from. An independent record has none.
	TrackerSource          string     `json:"tracker_source,omitempty"`
	UncertaintyAssumptions string     `json:"uncertainty_assumptions,omitempty"`
	Provenance             Provenance `json:"provenance"`
}

// PhysicalKeyframe is one reviewed instant of an object's pose. It covers
// its own sample and nothing else: frames between keyframes are unreferenced
// until someone reviews them.
type PhysicalKeyframe struct {
	KeyframeID   string           `json:"keyframe_id"`
	SampleID     int              `json:"sample_id"`
	TimestampNs  int64            `json:"timestamp_ns"`
	Anchor       PhysicalAnchor   `json:"anchor"`
	Position     PositionBound    `json:"position"`
	Yaw          YawBound         `json:"yaw"`
	Front        EndpointEvidence `json:"front"`
	Rear         EndpointEvidence `json:"rear"`
	SharedErrors []SharedError    `json:"shared_errors,omitempty"`
	Review       PhysicalReview   `json:"review"`
}

// PhysicalObject is one sidecar object's physical reference.
type PhysicalObject struct {
	ObjectID  string             `json:"object_id"`
	Body      *BodyGeometry      `json:"body,omitempty"`
	Keyframes []PhysicalKeyframe `json:"keyframes"`
}

// FollowingGap is one instant's reference gap, with the evidence for the two
// bumpers it spans. The gap is no better evidenced than its weaker bumper.
type FollowingGap struct {
	SampleID      int              `json:"sample_id"`
	TimestampNs   int64            `json:"timestamp_ns"`
	Status        EvidenceStatus   `json:"status"`
	LowerM        *float64         `json:"lower_m,omitempty"`
	UpperM        *float64         `json:"upper_m,omitempty"`
	ValueM        *float64         `json:"value_m,omitempty"`
	FollowerFront EndpointEvidence `json:"follower_front"`
	LeaderRear    EndpointEvidence `json:"leader_rear"`
	Support       EvidenceSupport  `json:"support"`
}

// FollowingReference is a reviewed leader decision over an interval, with
// gap references at the instants where the gap is known.
type FollowingReference struct {
	FollowingID      string            `json:"following_id"`
	FollowerObjectID string            `json:"follower_object_id"`
	Decision         FollowingDecision `json:"decision"`
	LeaderObjectID   string            `json:"leader_object_id,omitempty"`
	Interval         FrameInterval     `json:"interval"`
	GapDefinition    string            `json:"gap_definition,omitempty"`
	Gaps             []FollowingGap    `json:"gaps,omitempty"`
	Review           PhysicalReview    `json:"review"`
}

// PhysicalSource is what the references were measured against. Each field
// must equal the pack's own, so a reference cannot be applied to another
// recording, sensor or frame.
type PhysicalSource struct {
	SensorID         string `json:"sensor_id"`
	CalibrationID    string `json:"calibration_id,omitempty"`
	VRLOGHeaderSHA   string `json:"vrlog_header_sha256"`
	VRLOGFramesSHA   string `json:"vrlog_frames_sha256"`
	CoordinateFrame  string `json:"coordinate_frame"`
	ReferenceFrame   string `json:"reference_frame"`
	TransformVersion string `json:"transform_version"`
	Units            string `json:"units"`
}

// PhysicalReferenceSet is the revisable physical-reference document for one
// pack, stored beside its sidecar.
type PhysicalReferenceSet struct {
	Schema        string     `json:"schema"`
	SchemaVersion int        `json:"schema_version"`
	DatasetID     string     `json:"dataset_id"`
	PackDigest    string     `json:"pack_digest"`
	Revision      int        `json:"revision"`
	UpdatedUTC    string     `json:"updated_utc"`
	Change        Provenance `json:"change"`
	RestoredFrom  int        `json:"restored_from,omitempty"`

	Source    PhysicalSource       `json:"source"`
	Objects   []PhysicalObject     `json:"objects"`
	Following []FollowingReference `json:"following,omitempty"`
	// RecordOrigins is every record ID this document has ever carried, with
	// the origin it was created under. The store carries it forward, so a
	// tracker-assisted record deleted and added back under its old ID is
	// still tracker-assisted. Keys are "body/", "keyframe/" or "following/"
	// followed by the record's ID.
	RecordOrigins map[string]ReferenceOrigin `json:"record_origins"`

	// baseDigest is the optimistic concurrency token: the SHA-256 of the
	// exact bytes this document was loaded from.
	baseDigest string
}

// NewPhysicalReferenceSet starts an empty document whose source is the
// pack's own.
func NewPhysicalReferenceSet(p *Pack) *PhysicalReferenceSet {
	m := p.Manifest
	return &PhysicalReferenceSet{
		Schema: PhysicalReferenceSchema, SchemaVersion: PhysicalReferenceSchemaVersion,
		DatasetID: m.DatasetID, PackDigest: m.PackDigest, Revision: 1,
		Source:        PackPhysicalSource(p),
		Objects:       []PhysicalObject{},
		RecordOrigins: map[string]ReferenceOrigin{},
	}
}

// PackPhysicalSource is the source a reference against this pack must state.
func PackPhysicalSource(p *Pack) PhysicalSource {
	m := p.Manifest
	return PhysicalSource{
		SensorID: m.Source.SensorID, VRLOGHeaderSHA: m.Source.VRLOGHeaderSHA, VRLOGFramesSHA: m.Source.VRLOGFramesSHA,
		CoordinateFrame: m.Coordinate.FrameID, ReferenceFrame: m.Coordinate.ReferenceFrame,
		TransformVersion: m.Coordinate.TransformVersion, Units: m.Coordinate.Units,
	}
}

// Digest is the SHA-256 of the exact bytes this revision was loaded from or
// saved as, empty for a document never saved.
func (r *PhysicalReferenceSet) Digest() string { return r.baseDigest }

// ContentDigest is the SHA-256 of the references alone: schema, pack,
// source, objects and following, in canonical order, without the revision,
// timestamps or change record. Two revisions with the same content share it,
// which is what a frozen evaluation pins.
func (r *PhysicalReferenceSet) ContentDigest() (string, error) {
	c := *r
	c.Objects = clonePhysicalObjects(r.Objects)
	c.Following = cloneFollowing(r.Following)
	c.canonicalise()
	b, err := json.Marshal(struct {
		Schema        string               `json:"schema"`
		SchemaVersion int                  `json:"schema_version"`
		DatasetID     string               `json:"dataset_id"`
		PackDigest    string               `json:"pack_digest"`
		Source        PhysicalSource       `json:"source"`
		Objects       []PhysicalObject     `json:"objects"`
		Following     []FollowingReference `json:"following,omitempty"`
	}{c.Schema, c.SchemaVersion, c.DatasetID, c.PackDigest, c.Source, c.Objects, c.Following})
	if err != nil {
		return "", fmt.Errorf("encode physical references for digest: %w", err)
	}
	return sha256Hex(b), nil
}

// Keyframe returns an object's keyframe at a sample.
func (r *PhysicalReferenceSet) Keyframe(objectID string, sampleID int) (PhysicalKeyframe, bool) {
	for _, o := range r.Objects {
		if o.ObjectID != objectID {
			continue
		}
		for _, k := range o.Keyframes {
			if k.SampleID == sampleID {
				return k, true
			}
		}
	}
	return PhysicalKeyframe{}, false
}

// Object returns an object's physical reference.
func (r *PhysicalReferenceSet) Object(objectID string) (PhysicalObject, bool) {
	for _, o := range r.Objects {
		if o.ObjectID == objectID {
			return o, true
		}
	}
	return PhysicalObject{}, false
}

// ScoredAsTruth reports whether a record may be scored as reference truth:
// reviewed by a person, and independent of the tracker it would judge.
func (v PhysicalReview) ScoredAsTruth() bool {
	return v.Status == StatusReviewed && v.Origin == OriginIndependent
}

// canonicalise puts the document into its one permitted encoding. Sorting is
// permitted; deduplicating is not, so a repeated frame stays an error.
func (r *PhysicalReferenceSet) canonicalise() {
	sort.Slice(r.Objects, func(i, j int) bool { return r.Objects[i].ObjectID < r.Objects[j].ObjectID })
	for i := range r.Objects {
		o := &r.Objects[i]
		if o.Keyframes == nil {
			o.Keyframes = []PhysicalKeyframe{}
		}
		if o.Body != nil {
			for _, d := range []*DimensionBound{&o.Body.Length, &o.Body.Width, &o.Body.Height} {
				sort.Ints(d.Support.Frames)
			}
		}
		sort.Slice(o.Keyframes, func(a, b int) bool { return o.Keyframes[a].SampleID < o.Keyframes[b].SampleID })
		for k := range o.Keyframes {
			kf := &o.Keyframes[k]
			for _, s := range []*EvidenceSupport{&kf.Position.Support, &kf.Yaw.Support, &kf.Front.Support, &kf.Rear.Support} {
				sort.Ints(s.Frames)
			}
			for e := range kf.SharedErrors {
				sort.Strings(kf.SharedErrors[e].Components)
			}
			sort.Slice(kf.SharedErrors, func(a, b int) bool {
				return kf.SharedErrors[a].Observation < kf.SharedErrors[b].Observation
			})
		}
	}
	sort.Slice(r.Following, func(i, j int) bool { return r.Following[i].FollowingID < r.Following[j].FollowingID })
	for i := range r.Following {
		f := &r.Following[i]
		sort.Slice(f.Gaps, func(a, b int) bool { return f.Gaps[a].SampleID < f.Gaps[b].SampleID })
		for g := range f.Gaps {
			for _, s := range []*EvidenceSupport{&f.Gaps[g].Support, &f.Gaps[g].FollowerFront.Support, &f.Gaps[g].LeaderRear.Support} {
				sort.Ints(s.Frames)
			}
		}
	}
}

// clonePhysicalObjects deep-copies everything canonicalise may reorder, so a
// failed save or a digest leaves the caller's document untouched.
func clonePhysicalObjects(in []PhysicalObject) []PhysicalObject {
	out := make([]PhysicalObject, len(in))
	for i, o := range in {
		out[i] = o
		if o.Body != nil {
			b := *o.Body
			for _, d := range []*DimensionBound{&b.Length, &b.Width, &b.Height} {
				d.Support.Frames = append([]int(nil), d.Support.Frames...)
			}
			out[i].Body = &b
		}
		out[i].Keyframes = append([]PhysicalKeyframe(nil), o.Keyframes...)
		for k := range out[i].Keyframes {
			kf := &out[i].Keyframes[k]
			for _, s := range []*EvidenceSupport{&kf.Position.Support, &kf.Yaw.Support, &kf.Front.Support, &kf.Rear.Support} {
				s.Frames = append([]int(nil), s.Frames...)
			}
			kf.SharedErrors = append([]SharedError(nil), kf.SharedErrors...)
			for e := range kf.SharedErrors {
				kf.SharedErrors[e].Components = append([]string(nil), kf.SharedErrors[e].Components...)
			}
		}
	}
	return out
}

func cloneFollowing(in []FollowingReference) []FollowingReference {
	if in == nil {
		return nil
	}
	out := append([]FollowingReference(nil), in...)
	for i := range out {
		out[i].Gaps = append([]FollowingGap(nil), in[i].Gaps...)
		for g := range out[i].Gaps {
			gp := &out[i].Gaps[g]
			for _, s := range []*EvidenceSupport{&gp.Support, &gp.FollowerFront.Support, &gp.LeaderRear.Support} {
				s.Frames = append([]int(nil), s.Frames...)
			}
		}
	}
	return out
}

// currentOrigins lists every record in the document under its ledger key.
func (r *PhysicalReferenceSet) currentOrigins() map[string]ReferenceOrigin {
	out := map[string]ReferenceOrigin{}
	for _, o := range r.Objects {
		if o.Body != nil {
			out["body/"+o.Body.BodyID] = o.Body.Review.Origin
		}
		for _, k := range o.Keyframes {
			out["keyframe/"+k.KeyframeID] = k.Review.Origin
		}
	}
	for _, f := range r.Following {
		out["following/"+f.FollowingID] = f.Review.Origin
	}
	return out
}

// decodePhysicalJSON is a strict single-object decode: an unknown field is
// refused rather than dropped on the next save.
func decodePhysicalJSON(b []byte, into any, what string) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("parse %s: %w", what, err)
	}
	if dec.More() {
		return fmt.Errorf("parse %s: trailing data after the document", what)
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func finiteNonNegative(v float64) bool { return finite(v) && v >= 0 }
