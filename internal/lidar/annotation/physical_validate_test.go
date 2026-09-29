package annotation

import (
	"math"
	"strings"
	"testing"
)

// Every refusal the contract promises, each from a valid document with one
// thing wrong. The expected text pins which rule fired, so a document
// refused for some other reason cannot pass a case by accident.
func TestPhysicalValidationRefusals(t *testing.T) {
	p := physPack(t)
	kf := func(r *PhysicalReferenceSet) *PhysicalKeyframe { return &r.Objects[0].Keyframes[0] }
	rear := func(r *PhysicalReferenceSet) *PhysicalKeyframe { return &r.Objects[0].Keyframes[1] }
	body := func(r *PhysicalReferenceSet) *BodyGeometry { return r.Objects[0].Body }
	follow := func(r *PhysicalReferenceSet) *FollowingReference { return &r.Following[0] }
	gap := func(r *PhysicalReferenceSet) *FollowingGap { return &r.Following[0].Gaps[0] }

	cases := []struct {
		name   string
		mutate func(*PhysicalReferenceSet)
		want   string
	}{
		// Document identity and source.
		{"schema", func(r *PhysicalReferenceSet) { r.Schema = "other" }, "schema"},
		{"schema version", func(r *PhysicalReferenceSet) { r.SchemaVersion = 2 }, "schema version 2"},
		{"revision zero", func(r *PhysicalReferenceSet) { r.Revision = 0 }, "invalid physical reference revision"},
		{"revision maximum", func(r *PhysicalReferenceSet) { r.Revision = math.MaxInt }, "invalid physical reference revision"},
		{"pack digest mismatch", func(r *PhysicalReferenceSet) { r.PackDigest = "sha256:other" }, "written against pack"},
		{"dataset mismatch", func(r *PhysicalReferenceSet) { r.DatasetID = "ds_other" }, "dataset"},
		{"source digest mismatch", func(r *PhysicalReferenceSet) { r.Source.VRLOGFramesSHA = "sha256:elsewhere" }, "vrlog_frames_sha256"},
		{"source header mismatch", func(r *PhysicalReferenceSet) { r.Source.VRLOGHeaderSHA = "sha256:elsewhere" }, "vrlog_header_sha256"},
		{"source sensor mismatch", func(r *PhysicalReferenceSet) { r.Source.SensorID = "other-sensor" }, "sensor_id"},
		{"source frame mismatch", func(r *PhysicalReferenceSet) { r.Source.CoordinateFrame = "site" }, "coordinate_frame"},
		{"source reference frame mismatch", func(r *PhysicalReferenceSet) { r.Source.ReferenceFrame = "site" }, "reference_frame"},
		{"source transform mismatch", func(r *PhysicalReferenceSet) { r.Source.TransformVersion = "v9" }, "transform_version"},
		{"source units mismatch", func(r *PhysicalReferenceSet) { r.Source.Units = "feet" }, "units"},

		// Frames and samples not in the pack.
		{"keyframe sample not in pack", func(r *PhysicalReferenceSet) { kf(r).SampleID = 99 }, "sample 99 is not in the pack"},
		{"keyframe sample negative", func(r *PhysicalReferenceSet) { kf(r).SampleID = -1 }, "sample -1 is not in the pack"},
		{"keyframe capture time", func(r *PhysicalReferenceSet) { kf(r).TimestampNs++ }, "was captured at"},
		{"supporting frame not in pack", func(r *PhysicalReferenceSet) { kf(r).Position.Support.Frames = []int{0, 6} }, "supporting frame 6 is not in the pack"},
		{"supporting frame negative", func(r *PhysicalReferenceSet) { body(r).Length.Support.Frames = []int{-1} }, "supporting frame -1"},
		{"supporting frames repeated", func(r *PhysicalReferenceSet) { kf(r).Yaw.Support.Frames = []int{0, 0} }, "sorted and unique"},
		{"gap sample not in pack", func(r *PhysicalReferenceSet) { gap(r).SampleID = 6; follow(r).Interval.LastSample = 6 }, "not inside the pack"},
		{"gap capture time", func(r *PhysicalReferenceSet) { gap(r).TimestampNs = 1 }, "was captured at"},
		{"following interval past the pack", func(r *PhysicalReferenceSet) { follow(r).Interval.LastSample = 6 }, "not inside the pack"},
		{"following interval reversed", func(r *PhysicalReferenceSet) { follow(r).Interval = FrameInterval{FirstSample: 3, LastSample: 1} }, "not inside the pack"},

		// Non-finite and negative bounds.
		{"non-finite lower", func(r *PhysicalReferenceSet) { body(r).Length.LowerM = fp(math.NaN()) }, "non-negative lower_m"},
		{"negative lower", func(r *PhysicalReferenceSet) { body(r).Width.LowerM = fp(-0.1) }, "non-negative lower_m"},
		{"infinite upper", func(r *PhysicalReferenceSet) { body(r).Length.UpperM = fp(math.Inf(1)) }, "finite upper_m"},
		{"upper below lower", func(r *PhysicalReferenceSet) { body(r).Length.UpperM = fp(4.0) }, "no less than lower_m"},
		{"value outside interval", func(r *PhysicalReferenceSet) { body(r).Length.ValueM = fp(4.9) }, "within [lower_m, upper_m]"},
		{"non-finite position", func(r *PhysicalReferenceSet) { kf(r).Position.XM = fp(math.Inf(-1)) }, "finite x_m"},
		{"missing position bound", func(r *PhysicalReferenceSet) { kf(r).Position.BoundM = nil }, "bound_m"},
		{"negative position bound", func(r *PhysicalReferenceSet) { kf(r).Position.BoundM = fp(-1) }, "bound_m"},
		{"non-finite height", func(r *PhysicalReferenceSet) { kf(r).Position.ZM = fp(math.NaN()) }, "z_m"},
		{"negative yaw bound", func(r *PhysicalReferenceSet) { kf(r).Yaw.BoundRad = fp(-0.1) }, "bound_rad"},
		{"yaw bound above pi", func(r *PhysicalReferenceSet) { kf(r).Yaw.BoundRad = fp(4) }, "bound_rad"},
		{"non-finite yaw", func(r *PhysicalReferenceSet) { kf(r).Yaw.YawRad = fp(math.NaN()) }, "yaw_rad"},
		{"negative offset", func(r *PhysicalReferenceSet) { rear(r).Anchor.OffsetM = fp(-2) }, "finite and non-negative"},
		{"negative offset bound", func(r *PhysicalReferenceSet) { rear(r).Anchor.OffsetBoundM = fp(math.NaN()) }, "finite and non-negative"},
		{"negative gap", func(r *PhysicalReferenceSet) { gap(r).LowerM = fp(-1) }, "non-negative lower_m and upper_m"},
		{"gap upper below lower", func(r *PhysicalReferenceSet) { gap(r).UpperM = fp(5) }, "in order"},
		{"gap value outside", func(r *PhysicalReferenceSet) { gap(r).ValueM = fp(7) }, "within [lower_m, upper_m]"},

		// Evidence status and support.
		{"observed without frames", func(r *PhysicalReferenceSet) { body(r).Length.Support = EvidenceSupport{External: "tape"} }, "names no supporting frames"},
		{"observed keyframe without frames", func(r *PhysicalReferenceSet) { kf(r).Position.Support.Frames = nil }, "names no supporting frames"},
		{"observed bumper without frames", func(r *PhysicalReferenceSet) { kf(r).Front.Support.Frames = nil }, "names no supporting frames"},
		{"inferred without anything", func(r *PhysicalReferenceSet) { body(r).Width.Support = EvidenceSupport{} }, "neither supporting frames nor"},
		{"prior without a named prior", func(r *PhysicalReferenceSet) { r.Objects[1].Body.Height.Support = frames(0) }, "must name its prior"},
		{"unknown with support", func(r *PhysicalReferenceSet) { r.Objects[1].Body.Width.Support = frames(0) }, "no support to name"},
		{"unknown with value", func(r *PhysicalReferenceSet) { r.Objects[1].Body.Width.LowerM = fp(1) }, "carries no span or value"},
		{"unknown with span", func(r *PhysicalReferenceSet) { r.Objects[1].Body.Width.Span = SpanFull }, "carries no span or value"},
		{"unknown position with value", func(r *PhysicalReferenceSet) {
			kf(r).Position = PositionBound{Status: EvidenceUnknown, XM: fp(1)}
		}, "unknown position carries no value"},
		{"unknown gap with value", func(r *PhysicalReferenceSet) {
			g := gap(r)
			g.Status, g.Support = EvidenceUnknown, EvidenceSupport{}
		}, "unknown gap carries no value"},
		{"gap stronger than its bumper", func(r *PhysicalReferenceSet) {
			gap(r).LeaderRear = EndpointEvidence{Status: EvidenceInferred, Support: EvidenceSupport{External: "spec"}}
		}, "cannot rest on a inferred leader_rear"},
		{"gap bumper without support", func(r *PhysicalReferenceSet) { gap(r).FollowerFront.Support = EvidenceSupport{} }, "follower_front"},

		// Partial spans cannot claim a full dimension.
		{"partial span with upper bound", func(r *PhysicalReferenceSet) { body(r).Height.UpperM = fp(1.6) }, "supports only a lower bound"},
		{"partial span with value", func(r *PhysicalReferenceSet) { body(r).Height.ValueM = fp(1.2) }, "supports only a lower bound"},
		{"partial span inferred", func(r *PhysicalReferenceSet) {
			body(r).Height.Status = EvidenceInferred
		}, "partial span is an observation"},
		{"full span without upper", func(r *PhysicalReferenceSet) { body(r).Length.UpperM = nil }, "full span needs"},

		// Tracker-assisted provenance.
		{"independent with tracker source", func(r *PhysicalReferenceSet) {
			kf(r).Review.TrackerSource = "lidar_track_estimates seq-000001"
		}, "cannot claim independent provenance"},
		{"independent with algorithm", func(r *PhysicalReferenceSet) {
			body(r).Review.Provenance.Algorithm = "near_edge_v1"
		}, "cannot claim independent provenance"},
		{"tracker-assisted relabelled", func(r *PhysicalReferenceSet) {
			r.Objects[0].Keyframes[2].Review.Origin = OriginIndependent
		}, "cannot claim independent provenance"},
		{"tracker-assisted without source", func(r *PhysicalReferenceSet) {
			r.Objects[0].Keyframes[2].Review.TrackerSource = ""
		}, "must name the tracker output"},
		{"origin ledger missing a record", func(r *PhysicalReferenceSet) {
			delete(r.RecordOrigins, "keyframe/kf-car-1-s0")
		}, "missing from record_origins"},
		{"origin ledger disagrees", func(r *PhysicalReferenceSet) {
			r.RecordOrigins["keyframe/kf-car-1-s0"] = OriginTrackerAssisted
		}, "an origin never changes"},
		{"origin ledger value", func(r *PhysicalReferenceSet) { r.RecordOrigins["keyframe/old"] = "borrowed" }, `origin "borrowed"`},

		// Unknown enum values.
		{"unknown evidence status", func(r *PhysicalReferenceSet) { body(r).Length.Status = "measured" }, `status "measured"`},
		{"unknown position status", func(r *PhysicalReferenceSet) { kf(r).Position.Status = "guessed" }, `status "guessed"`},
		{"unknown yaw status", func(r *PhysicalReferenceSet) { kf(r).Yaw.Status = "eyeballed" }, `status "eyeballed"`},
		{"unknown endpoint status", func(r *PhysicalReferenceSet) { kf(r).Front.Status = "glimpsed" }, `status "glimpsed"`},
		{"unknown span", func(r *PhysicalReferenceSet) { body(r).Length.Span = "most" }, `span "most"`},
		{"unknown axis", func(r *PhysicalReferenceSet) { kf(r).Yaw.Axis = "sideways" }, `axis "sideways"`},
		{"unknown anchor", func(r *PhysicalReferenceSet) { kf(r).Anchor.Kind = "roof" }, `kind "roof"`},
		{"unknown origin", func(r *PhysicalReferenceSet) { kf(r).Review.Origin = "borrowed" }, `origin "borrowed"`},
		{"unknown review status", func(r *PhysicalReferenceSet) { kf(r).Review.Status = "probably" }, `review status "probably"`},
		{"unknown decision", func(r *PhysicalReferenceSet) { follow(r).Decision = "perhaps" }, `decision "perhaps"`},
		{"unknown gap status", func(r *PhysicalReferenceSet) { gap(r).Status = "likely" }, `status "likely"`},
		{"unknown gap bumper status", func(r *PhysicalReferenceSet) { gap(r).LeaderRear.Status = "likely" }, `status "likely"`},
		{"unknown gap definition", func(r *PhysicalReferenceSet) { follow(r).GapDefinition = "along_path" }, `gap definition "along_path"`},
		{"unknown axis convention", func(r *PhysicalReferenceSet) { body(r).AxisConvention = "x_left" }, "axis convention"},
		{"unknown shared component", func(r *PhysicalReferenceSet) {
			kf(r).SharedErrors[0].Components = []string{"length", "wheelbase"}
		}, `component "wheelbase"`},

		// Axis and anchor consistency.
		{"bumper under an ambiguous axis", func(r *PhysicalReferenceSet) {
			kf(r).Yaw.Axis = AxisFrontRearAmbiguous
			kf(r).Rear = EndpointEvidence{Status: EvidenceUnknown}
		}, "bumper cannot be named while the axis is front_rear_ambiguous"},
		{"face anchor under an ambiguous axis", func(r *PhysicalReferenceSet) {
			rear(r).Yaw.Axis = AxisFrontRearAmbiguous
			rear(r).Rear = EndpointEvidence{Status: EvidenceUnknown}
		}, "rear_face cannot be named"},
		{"unknown axis with a yaw", func(r *PhysicalReferenceSet) {
			r.Objects[0].Keyframes[2].Yaw.Axis = AxisUnknown
		}, "unknown axis carries no yaw"},
		{"known axis without a yaw", func(r *PhysicalReferenceSet) {
			kf(r).Yaw = YawBound{Status: EvidenceUnknown, Axis: AxisResolved}
		}, "needs a yaw status other than unknown"},
		{"known axis missing its value", func(r *PhysicalReferenceSet) { kf(r).Yaw.YawRad = nil }, "yaw_rad"},
		{"body centre with offset", func(r *PhysicalReferenceSet) { kf(r).Anchor.OffsetM = fp(1) }, "body-centre anchor has no offset"},
		{"offset without bound", func(r *PhysicalReferenceSet) { rear(r).Anchor.OffsetBoundM = nil }, "both offset_m and offset_bound_m"},
		{"offset beyond half the length", func(r *PhysicalReferenceSet) { rear(r).Anchor.OffsetM = fp(3.0) }, "more than half the body's length"},
		{"offset short of half the length", func(r *PhysicalReferenceSet) { rear(r).Anchor.OffsetM = fp(1.0) }, "less than half the body's length"},
		{"side offset against the width", func(r *PhysicalReferenceSet) {
			rear(r).Anchor = PhysicalAnchor{Kind: AnchorLeftFace, OffsetM: fp(2.0), OffsetBoundM: fp(0.1)}
		}, "more than half the body's width"},

		// Shared errors.
		{"shared error without observation", func(r *PhysicalReferenceSet) { kf(r).SharedErrors[0].Observation = " " }, "names no observation"},
		{"shared error of one component", func(r *PhysicalReferenceSet) { kf(r).SharedErrors[0].Components = []string{"yaw"} }, "fewer than two"},
		{"shared error repeated component", func(r *PhysicalReferenceSet) {
			kf(r).SharedErrors[0].Components = []string{"yaw", "yaw"}
		}, "sorted and unique"},

		// Review and provenance.
		{"review without method", func(r *PhysicalReferenceSet) { kf(r).Review.Method = "" }, "names no method"},
		{"review without author", func(r *PhysicalReferenceSet) { body(r).Review.Provenance.Author = "" }, "names no author"},
		{"bounds without assumptions", func(r *PhysicalReferenceSet) { kf(r).Review.UncertaintyAssumptions = "" }, "uncertainty assumptions"},
		{"gap without assumptions", func(r *PhysicalReferenceSet) { follow(r).Review.UncertaintyAssumptions = "" }, "uncertainty assumptions"},

		// Identity.
		{"object without id", func(r *PhysicalReferenceSet) { r.Objects[0].ObjectID = "" }, "no object id"},
		{"object twice", func(r *PhysicalReferenceSet) { r.Objects[1].ObjectID = "car-1" }, "two physical references"},
		{"two keyframes at one sample", func(r *PhysicalReferenceSet) { rear(r).SampleID, rear(r).TimestampNs = 0, physTime(0) }, "two keyframes at sample 0"},
		{"keyframe without id", func(r *PhysicalReferenceSet) { kf(r).KeyframeID = "" }, "has no id"},
		{"one id for two records", func(r *PhysicalReferenceSet) { rear(r).KeyframeID = "body-car-1" }, "names both a body and a keyframe"},
		{"following without follower", func(r *PhysicalReferenceSet) { follow(r).FollowerObjectID = "" }, "no follower object"},
		{"leader decision without leader", func(r *PhysicalReferenceSet) { follow(r).LeaderObjectID = "" }, "names a leader other than the follower"},
		{"leader is the follower", func(r *PhysicalReferenceSet) { follow(r).LeaderObjectID = "car-1" }, "names a leader other than the follower"},
		{"no leader with a gap", func(r *PhysicalReferenceSet) {
			follow(r).Decision, follow(r).LeaderObjectID = FollowingNoLeader, ""
		}, "no_leader decision has no leader and no gap"},
		{"ambiguous with a leader", func(r *PhysicalReferenceSet) {
			follow(r).Decision, follow(r).Gaps, follow(r).GapDefinition = FollowingAmbiguous, nil, ""
		}, "ambiguous decision has no leader"},
		{"gap definition without gaps", func(r *PhysicalReferenceSet) { follow(r).Gaps = nil }, "gap definition with no gaps"},
		{"gap outside the interval", func(r *PhysicalReferenceSet) { follow(r).Interval.FirstSample = 1 }, "outside the interval"},
		{"gaps out of order", func(r *PhysicalReferenceSet) {
			follow(r).Gaps = append(follow(r).Gaps, follow(r).Gaps[0])
		}, "in sample order"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := validPhysical(p)
			r.RecordOrigins = r.currentOrigins()
			c.mutate(r)
			err := r.Validate(p)
			if err == nil {
				t.Fatal("an invalid document validated")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("refused for the wrong reason: %v (want %q)", err, c.want)
			}
		})
	}

	r := validPhysical(p)
	r.RecordOrigins = r.currentOrigins()
	if err := r.Validate(p); err != nil {
		t.Fatalf("the valid document was refused: %v", err)
	}
}

// Links to the membership review, and the evidence check the masks make
// possible: an observed full dimension that its own frames' returns do not
// reach is a partial span dressed as a full one.
func TestPhysicalLinkRefusals(t *testing.T) {
	p := physPack(t)
	s := physSidecar(t, p)
	cases := []struct {
		name   string
		mutate func(*PhysicalReferenceSet)
		want   string
	}{
		{"object not declared", func(r *PhysicalReferenceSet) { r.Objects[1].ObjectID = "car-9" }, `"car-9" is not an object`},
		{"object rejected", func(r *PhysicalReferenceSet) { r.Objects[1].ObjectID = "ghost" }, `"ghost" is rejected`},
		{"follower not declared", func(r *PhysicalReferenceSet) { r.Following[0].FollowerObjectID = "car-9" }, "follower"},
		{"leader rejected", func(r *PhysicalReferenceSet) { r.Following[0].LeaderObjectID = "ghost" }, "leader"},
		{"length beyond the returns", func(r *PhysicalReferenceSet) {
			// Sample 3 saw only the rear 1.5 m: a full length claimed on it
			// alone is refused.
			b := r.Objects[0].Body
			b.Length.Support = frames(3)
		}, "observed full length is at least 4.300 m"},
		{"height beyond the returns", func(r *PhysicalReferenceSet) {
			b := r.Objects[0].Body
			b.Height = DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(2.0), UpperM: fp(2.2), Support: frames(0)}
		}, "observed full height"},
		{"width beyond the returns", func(r *PhysicalReferenceSet) {
			b := r.Objects[0].Body
			b.Width = DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(2.5), UpperM: fp(2.7), Support: frames(0)}
		}, "observed full width"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := validPhysical(p)
			r.RecordOrigins = r.currentOrigins()
			c.mutate(r)
			if err := r.Validate(p); err != nil {
				t.Fatalf("the mutation broke structural validation: %v", err)
			}
			err := r.ValidateLinks(p, s)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("refused for the wrong reason: %v (want %q)", err, c.want)
			}
		})
	}

	r := validPhysical(p)
	if err := r.ValidateLinks(p, s); err != nil {
		t.Fatalf("the valid document's links were refused: %v", err)
	}
	// Frames that cannot say anything are not evidence either way: no mask
	// at the supporting frame, or no yaw to know the axis by.
	r.Objects[0].Body.Length.Support = frames(4)
	if err := r.ValidateLinks(p, s); err != nil {
		t.Fatalf("a frame with no keyframe yaw was held against the length: %v", err)
	}
	unmasked := *s
	unmasked.Masks = nil
	r.Objects[0].Body.Length.Support = frames(3)
	if err := r.ValidateLinks(p, &unmasked); err != nil {
		t.Fatalf("a frame with no mask was held against the length: %v", err)
	}
	other := *s
	other.PackDigest = "sha256:other"
	if err := r.ValidateLinks(p, &other); err == nil || !strings.Contains(err.Error(), "annotation was written against pack") {
		t.Fatalf("a sidecar for another pack: %v", err)
	}
	// A body without keyframes, and an object with no body, validate.
	r = validPhysical(p)
	r.Objects[1].Body = nil
	r.Objects[0].Keyframes = nil
	r.RecordOrigins = r.currentOrigins()
	if err := r.Validate(p); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateLinks(p, s); err != nil {
		t.Fatal(err)
	}
}

func TestDimensionBoundBest(t *testing.T) {
	d := DimensionBound{LowerM: fp(4), UpperM: fp(5)}
	if v, h, ok := d.Best(); !ok || v != 4.5 || h != 0.5 {
		t.Fatalf("midpoint: %v ± %v (%v)", v, h, ok)
	}
	d.ValueM = fp(4.2)
	if v, h, ok := d.Best(); !ok || v != 4.2 || math.Abs(h-0.8) > 1e-12 {
		t.Fatalf("stated value: %v ± %v", v, h)
	}
	if _, _, ok := (DimensionBound{LowerM: fp(4)}).Best(); ok {
		t.Fatal("a lower bound gave a best value")
	}
	for s, want := range map[EvidenceStatus]bool{EvidenceObserved: true, EvidenceInferred: true, EvidencePriorOnly: false, EvidenceUnknown: false} {
		if s.Scorable() != want {
			t.Fatalf("%s scorable = %v", s, !want)
		}
	}
}
