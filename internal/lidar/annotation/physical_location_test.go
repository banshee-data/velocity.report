package annotation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func intp(v int) *int { return &v }

// A refusal names the record and field it is about, so a client can show it
// beside the control that sets that field. The message is unchanged: each
// case also pins the full text, which is what the CLI and older clients read.
func TestPhysicalRefusalLocations(t *testing.T) {
	p := physPack(t)
	kf := func(r *PhysicalReferenceSet) *PhysicalKeyframe { return &r.Objects[0].Keyframes[0] }
	rear := func(r *PhysicalReferenceSet) *PhysicalKeyframe { return &r.Objects[0].Keyframes[1] }
	body := func(r *PhysicalReferenceSet) *BodyGeometry { return r.Objects[0].Body }
	follow := func(r *PhysicalReferenceSet) *FollowingReference { return &r.Following[0] }
	pose := func(id string, sample int, field, detail string) *PhysicalLocation {
		return &PhysicalLocation{ObjectID: "car-1", Record: "keyframe", RecordID: id, SampleID: intp(sample), Field: field, Detail: detail}
	}
	size := func(field, detail string) *PhysicalLocation {
		return &PhysicalLocation{ObjectID: "car-1", Record: "body", RecordID: "body-car-1", Field: field, Detail: detail}
	}

	cases := []struct {
		name    string
		mutate  func(*PhysicalReferenceSet)
		message string
		want    *PhysicalLocation
	}{
		// The pilot's two stops: a position with no ±, and a record that
		// declares bounds with no assumptions stated.
		{"position without its bound", func(r *PhysicalReferenceSet) { kf(r).Position.BoundM = nil },
			`object "car-1": keyframe "kf-car-1-s0": position: a known position needs a finite, non-negative bound_m`,
			pose("kf-car-1-s0", 0, "position.bound_m", "a known position needs a finite, non-negative bound_m")},
		{"pose without assumptions", func(r *PhysicalReferenceSet) { kf(r).Review.UncertaintyAssumptions = "" },
			`object "car-1": keyframe "kf-car-1-s0": a record that declares bounds must state its uncertainty assumptions`,
			pose("kf-car-1-s0", 0, "review.uncertainty_assumptions", "a record that declares bounds must state its uncertainty assumptions")},
		{"size without assumptions", func(r *PhysicalReferenceSet) { body(r).Review.UncertaintyAssumptions = " " },
			`object "car-1": body "body-car-1": a record that declares bounds must state its uncertainty assumptions`,
			size("review.uncertainty_assumptions", "a record that declares bounds must state its uncertainty assumptions")},

		// Every other field the pane edits.
		{"length without its upper bound", func(r *PhysicalReferenceSet) { body(r).Length.UpperM = nil },
			`object "car-1": body "body-car-1": length: a full span needs a finite upper_m no less than lower_m`,
			size("length", "a full span needs a finite upper_m no less than lower_m")},
		{"width without support", func(r *PhysicalReferenceSet) { body(r).Width.Support = EvidenceSupport{} },
			`object "car-1": body "body-car-1": width: an inferred component names neither supporting frames nor an external reference`,
			size("width.support", "an inferred component names neither supporting frames nor an external reference")},
		{"size without author", func(r *PhysicalReferenceSet) { body(r).Review.Provenance.Author = "" },
			`object "car-1": body "body-car-1": review names no author`,
			size("review.provenance.author", "review names no author")},
		{"size axis convention", func(r *PhysicalReferenceSet) { body(r).AxisConvention = "x_left" },
			`object "car-1": body "body-car-1": axis convention "x_left", want "x_front_y_left_z_up"`,
			size("axis_convention", `axis convention "x_left", want "x_front_y_left_z_up"`)},
		{"position observed elsewhere", func(r *PhysicalReferenceSet) { kf(r).Position.Support = frames(1) },
			`object "car-1": keyframe "kf-car-1-s0": position is observed but does not cite its own sample 0`,
			pose("kf-car-1-s0", 0, "position.support", "position is observed but does not cite its own sample 0")},
		{"heading without its value", func(r *PhysicalReferenceSet) { kf(r).Yaw.YawRad = nil },
			`object "car-1": keyframe "kf-car-1-s0": yaw: a known yaw needs a finite yaw_rad and a bound_rad in [0, pi]`,
			pose("kf-car-1-s0", 0, "yaw", "a known yaw needs a finite yaw_rad and a bound_rad in [0, pi]")},
		{"centre with an offset", func(r *PhysicalReferenceSet) { kf(r).Anchor.OffsetM = fp(1) },
			`object "car-1": keyframe "kf-car-1-s0": anchor: a body-centre anchor has no offset`,
			pose("kf-car-1-s0", 0, "anchor", "a body-centre anchor has no offset")},
		{"front under an ambiguous axis", func(r *PhysicalReferenceSet) {
			kf(r).Yaw.Axis = AxisFrontRearAmbiguous
		}, `object "car-1": keyframe "kf-car-1-s0": front: a bumper cannot be named while the axis is front_rear_ambiguous`,
			pose("kf-car-1-s0", 0, "front", "a bumper cannot be named while the axis is front_rear_ambiguous")},
		{"rear without support", func(r *PhysicalReferenceSet) { rear(r).Rear.Support = EvidenceSupport{} },
			`object "car-1": keyframe "kf-car-1-s3": rear: an observed component names no supporting frames`,
			pose("kf-car-1-s3", 3, "rear.support", "an observed component names no supporting frames")},
		{"shared error of one component", func(r *PhysicalReferenceSet) { kf(r).SharedErrors[0].Components = []string{"yaw"} },
			`object "car-1": keyframe "kf-car-1-s0": shared error "outline fit at sample 0" links fewer than two components`,
			pose("kf-car-1-s0", 0, "shared_errors.0.components", `shared error "outline fit at sample 0" links fewer than two components`)},
		{"pose capture time", func(r *PhysicalReferenceSet) { kf(r).TimestampNs++ },
			fmt.Sprintf(`object "car-1": keyframe "kf-car-1-s0": sample 0 was captured at %d ns, the record says %d`, physTime(0), physTime(0)+1),
			pose("kf-car-1-s0", 0, "timestamp_ns", fmt.Sprintf("sample 0 was captured at %d ns, the record says %d", physTime(0), physTime(0)+1))},
		{"two poses at one sample", func(r *PhysicalReferenceSet) { rear(r).SampleID, rear(r).TimestampNs = 0, physTime(0) },
			`object "car-1": two keyframes at sample 0`,
			pose("kf-car-1-s3", 0, "sample_id", "two keyframes at sample 0")},
		{"one id for two records", func(r *PhysicalReferenceSet) { rear(r).KeyframeID = "body-car-1" },
			`object "car-1": keyframe "body-car-1": id "body-car-1" names both a body and a keyframe record`,
			pose("body-car-1", 3, "keyframe_id", `id "body-car-1" names both a body and a keyframe record`)},

		// Following records are located by their own id.
		{"gap without assumptions", func(r *PhysicalReferenceSet) { follow(r).Review.UncertaintyAssumptions = "" },
			`following "follow-car-1": a record that declares bounds must state its uncertainty assumptions`,
			&PhysicalLocation{Record: "following", RecordID: "follow-car-1", Field: "review.uncertainty_assumptions",
				Detail: "a record that declares bounds must state its uncertainty assumptions"}},
		{"negative gap", func(r *PhysicalReferenceSet) { follow(r).Gaps[0].LowerM = fp(-1) },
			`following "follow-car-1": gap at sample 0: a known gap needs finite, non-negative lower_m and upper_m in order`,
			&PhysicalLocation{Record: "following", RecordID: "follow-car-1", Field: "gaps.0",
				Detail: "a known gap needs finite, non-negative lower_m and upper_m in order"}},

		// A refusal of the document as a whole has nowhere narrower to go.
		{"schema", func(r *PhysicalReferenceSet) { r.Schema = "other" }, "", nil},
		{"origin ledger", func(r *PhysicalReferenceSet) { delete(r.RecordOrigins, "keyframe/kf-car-1-s0") }, "", nil},
		{"object without id", func(r *PhysicalReferenceSet) { r.Objects[0].ObjectID = "" }, "", nil},
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
			if c.message != "" && err.Error() != c.message {
				t.Fatalf("the message changed:\n got %s\nwant %s", err, c.message)
			}
			got := LocatePhysicalError(err)
			if !sameLocation(got, c.want) {
				t.Fatalf("located at %s, want %s (%v)", describeLocation(got), describeLocation(c.want), err)
			}
		})
	}
}

// A link problem carries the same location, so a client can place a
// membership refusal (an end the returns do not reach) beside its control.
func TestPhysicalLinkProblemLocations(t *testing.T) {
	p := physPack(t)
	s := physSidecar(t, p)

	r := validPhysical(p)
	r.Objects[0].Body.Length.Support = frames(3)
	r.Objects = append(r.Objects, PhysicalObject{ObjectID: "car-9", Keyframes: []PhysicalKeyframe{}})
	r.RecordOrigins = r.currentOrigins()
	if err := r.Validate(p); err != nil {
		t.Fatal(err)
	}
	problems := r.LinkProblems(p, s)
	if len(problems) != 2 {
		t.Fatalf("problems: %v", problems)
	}
	length := problems[0]
	if length.Record != `object "car-1" body "body-car-1"` || !strings.HasPrefix(length.Problem, "observed full length is at least 4.300 m") {
		t.Fatalf("the length problem changed: %v", length)
	}
	if want := (&PhysicalLocation{ObjectID: "car-1", Record: "body", RecordID: "body-car-1", Field: "length", Detail: length.Problem}); !sameLocation(length.At, want) {
		t.Fatalf("length located at %s, want %s", describeLocation(length.At), describeLocation(want))
	}
	undeclared := problems[1]
	if want := (&PhysicalLocation{ObjectID: "car-9", Detail: undeclared.Problem}); !sameLocation(undeclared.At, want) {
		t.Fatalf("an undeclared object located at %s, want %s", describeLocation(undeclared.At), describeLocation(want))
	}

	// With no masks, a pose citing a frame is refused at the component.
	r = validPhysical(p)
	r.Objects[0].Body = nil
	r.Objects[1].Body = nil
	r.Following = nil
	unmasked := *s
	unmasked.Masks = nil
	problems = r.LinkProblems(p, &unmasked)
	if len(problems) == 0 {
		t.Fatal("no problems without masks")
	}
	if want := (&PhysicalLocation{ObjectID: "car-1", Record: "keyframe", RecordID: "kf-car-1-s0", SampleID: intp(0),
		Field: "position.support", Detail: problems[0].Problem}); !sameLocation(problems[0].At, want) {
		t.Fatalf("an unmasked pose located at %s, want %s", describeLocation(problems[0].At), describeLocation(want))
	}
	if problems[0].String() != `object "car-1" keyframe "kf-car-1-s0": position cites frame 0, where "car-1" has no returns in its mask` {
		t.Fatalf("the problem text changed: %s", problems[0])
	}
}

// An error from outside the rules, or a plain one, has no location.
func TestLocatePhysicalErrorWithoutLocation(t *testing.T) {
	if got := LocatePhysicalError(nil); got != nil {
		t.Fatalf("nil error located: %+v", got)
	}
	if got := LocatePhysicalError(errors.New("plain")); got != nil {
		t.Fatalf("plain error located: %+v", got)
	}
	if atField("x", nil) != nil || inRecord("body", "b", nil, nil) != nil || inObject("o", nil) != nil {
		t.Fatal("a wrapper turned no error into one")
	}
	wrapped := fmt.Errorf("outer: %w", atField("a", atField("b", errors.New("leaf"))))
	got := LocatePhysicalError(wrapped)
	if got == nil || got.Field != "a.b" || got.Detail != "leaf" || wrapped.Error() != "outer: leaf" {
		t.Fatalf("nested fields: %+v (%v)", got, wrapped)
	}
}

func sameLocation(a, b *PhysicalLocation) bool {
	if a == nil || b == nil {
		return a == b
	}
	if (a.SampleID == nil) != (b.SampleID == nil) || (a.SampleID != nil && *a.SampleID != *b.SampleID) {
		return false
	}
	return a.ObjectID == b.ObjectID && a.Record == b.Record && a.RecordID == b.RecordID && a.Field == b.Field && a.Detail == b.Detail
}

func describeLocation(l *PhysicalLocation) string {
	if l == nil {
		return "nowhere"
	}
	sample := "-"
	if l.SampleID != nil {
		sample = fmt.Sprint(*l.SampleID)
	}
	return fmt.Sprintf("{object %q %s %q sample %s field %q detail %q}", l.ObjectID, l.Record, l.RecordID, sample, l.Field, l.Detail)
}

const physicalLocationSwiftFixture = "testdata/physical_location_fixture.json"

// physicalLocationFixtureCase is one refusal as the edit endpoint reports
// it, with the object and frame a client has on screen.
type physicalLocationFixtureCase struct {
	Name     string                    `json:"name"`
	ObjectID string                    `json:"object_id"`
	SampleID int                       `json:"sample_id"`
	Result   physicalLocationEditReply `json:"result"`
}

// physicalLocationEditReply spells the fields of the edit endpoint's reply
// that carry refusals, as the server does.
type physicalLocationEditReply struct {
	Valid        bool              `json:"valid"`
	Invalid      string            `json:"invalid,omitempty"`
	InvalidAt    *PhysicalLocation `json:"invalid_at,omitempty"`
	LinkProblems []LinkProblem     `json:"link_problems"`
	ResetReviews []string          `json:"reset_reviews"`
}

// The macOS pane places refusals by their location, so the location's
// spelling is a contract between the two. This writes refusals for every
// control the pane maps to testdata; PhysicalFieldErrorsTests.swift reads
// the same file and checks where each lands. Rerun with
// -update-physical-swift-fixture after changing a location.
func TestPhysicalLocationSwiftFixture(t *testing.T) {
	p := physPack(t)
	s := physSidecar(t, p)
	kf := func(r *PhysicalReferenceSet) *PhysicalKeyframe { return &r.Objects[0].Keyframes[0] }
	body := func(r *PhysicalReferenceSet) *BodyGeometry { return r.Objects[0].Body }
	structural := []struct {
		name   string
		sample int
		mutate func(*PhysicalReferenceSet)
	}{
		{"position bound", 0, func(r *PhysicalReferenceSet) { kf(r).Position.BoundM = nil }},
		{"pose assumptions", 0, func(r *PhysicalReferenceSet) { kf(r).Review.UncertaintyAssumptions = "" }},
		{"size assumptions", 0, func(r *PhysicalReferenceSet) { body(r).Review.UncertaintyAssumptions = "" }},
		{"length", 0, func(r *PhysicalReferenceSet) { body(r).Length.UpperM = nil }},
		{"width", 0, func(r *PhysicalReferenceSet) { body(r).Width.Support = EvidenceSupport{} }},
		{"height", 0, func(r *PhysicalReferenceSet) { body(r).Height.UpperM = fp(1.6) }},
		{"axis convention", 0, func(r *PhysicalReferenceSet) { body(r).AxisConvention = "x_left" }},
		{"heading", 0, func(r *PhysicalReferenceSet) { kf(r).Yaw.YawRad = nil }},
		{"anchor", 0, func(r *PhysicalReferenceSet) { kf(r).Anchor.OffsetM = fp(1) }},
		{"front", 0, func(r *PhysicalReferenceSet) { kf(r).Yaw.Axis = AxisFrontRearAmbiguous }},
		{"rear", 3, func(r *PhysicalReferenceSet) { r.Objects[0].Keyframes[1].Rear.Support = EvidenceSupport{} }},
		{"shared errors", 0, func(r *PhysicalReferenceSet) { kf(r).SharedErrors[0].Components = []string{"yaw"} }},
		{"capture time", 0, func(r *PhysicalReferenceSet) { kf(r).TimestampNs++ }},
		{"pose at another frame", 3, func(r *PhysicalReferenceSet) { kf(r).Position.BoundM = nil }},
		{"following", 0, func(r *PhysicalReferenceSet) { r.Following[0].Review.UncertaintyAssumptions = "" }},
		{"document", 0, func(r *PhysicalReferenceSet) { delete(r.RecordOrigins, "keyframe/kf-car-1-s0") }},
	}
	var cases []physicalLocationFixtureCase
	for _, c := range structural {
		r := validPhysical(p)
		r.RecordOrigins = r.currentOrigins()
		c.mutate(r)
		err := r.Validate(p)
		if err == nil {
			t.Fatalf("%s: the mutation validated", c.name)
		}
		cases = append(cases, physicalLocationFixtureCase{Name: c.name, ObjectID: "car-1", SampleID: c.sample,
			Result: physicalLocationEditReply{Invalid: err.Error(), InvalidAt: LocatePhysicalError(err),
				LinkProblems: []LinkProblem{}, ResetReviews: []string{}}})
	}
	// A membership refusal: a length longer than its frame's returns span.
	r := validPhysical(p)
	r.Objects[0].Body.Length.Support = frames(3)
	r.RecordOrigins = r.currentOrigins()
	cases = append(cases, physicalLocationFixtureCase{Name: "length beyond the returns", ObjectID: "car-1",
		Result: physicalLocationEditReply{LinkProblems: r.LinkProblems(p, s), ResetReviews: []string{}}})

	want, err := json.MarshalIndent(cases, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if *updatePhysicalSwiftFixture {
		if err := os.WriteFile(physicalLocationSwiftFixture, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(physicalLocationSwiftFixture)
	if err != nil {
		t.Fatalf("read %s (run with -update-physical-swift-fixture to create it): %v", physicalLocationSwiftFixture, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is stale: rerun with -update-physical-swift-fixture and check PhysicalFieldErrorsTests.swift", physicalLocationSwiftFixture)
	}
}
