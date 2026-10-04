//go:build pcap
// +build pcap

package lidar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"google.golang.org/protobuf/proto"
)

var splitNow = func() time.Time { return time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC) }

// kirk0Digest stands in for the kirk0 capture's SHA-256 in a draft.
const kirk0Digest = "sha256:2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a"

// reviewedPack writes a three-sample pack whose two objects, obj_a and obj_b,
// are reviewed cars with reviewed masks in every sample.
func reviewedPack(t *testing.T, dir string, startNs int64) *annotation.Pack {
	t.Helper()
	var samples []annotation.Sample
	var blocks [][]byte
	for i := 0; i < 3; i++ {
		block, err := annotation.EncodePoints(annotation.Points{
			X: []float32{1, 2, 10, 11}, Y: make([]float32, 4), Z: make([]float32, 4),
		})
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, annotation.Sample{SourceOrdinal: i, TimestampNs: startNs + int64(i)*1e9, PointCount: 4})
		blocks = append(blocks, block)
	}
	m := annotation.Manifest{Coverage: annotation.CoverageForegroundOnly, Source: annotation.SourceProvenance{PCAPBasename: "cap.pcap"},
		Coordinate: annotation.CoordinateContract{Units: "metres", FrameID: "sensor", ReferenceFrame: "sensor", Handedness: "right", OriginNote: "test"}}
	if err := annotation.WritePack(dir, m, samples, blocks); err != nil {
		t.Fatal(err)
	}
	p, err := annotation.OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	saveReview(t, p, "")
	return p
}

// saveReview saves a new revision of the pack's review, with a note on obj_b
// so successive revisions differ.
func saveReview(t *testing.T, p *annotation.Pack, note string) {
	t.Helper()
	s, err := annotation.LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Objects = []annotation.Object{
		{ObjectID: "obj_a", Class: "car", Confidence: 1, Status: annotation.StatusReviewed},
		{ObjectID: "obj_b", Class: "car", Confidence: 1, Status: annotation.StatusReviewed, Notes: note},
	}
	s.Masks = nil
	for i := range p.Samples {
		for k, id := range []string{"obj_a", "obj_b"} {
			s.Masks = append(s.Masks, annotation.FrameMask{ObjectID: id, SampleID: i, PointIndices: []int{2 * k, 2*k + 1},
				Completeness: annotation.MaskComplete, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed})
		}
	}
	s.Change = annotation.Provenance{Author: "reviewer"}
	if err := annotation.SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
}

// savePhysical commits a physical-reference revision for the pack: obj_a
// with a reviewed independent body that states no dimension, and one
// keyframe at sample 0 that states nothing but its instant, a proposal an
// operator has yet to fill in. It links to any membership that declares
// obj_a, so it holds against every revision saveReview writes.
func savePhysical(t *testing.T, p *annotation.Pack) *annotation.PhysicalReferenceSet {
	t.Helper()
	r, err := annotation.LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	r.Change = annotation.Provenance{Author: "op", Operation: "author"}
	unknown := annotation.DimensionBound{Status: annotation.EvidenceUnknown}
	r.Objects = []annotation.PhysicalObject{{ObjectID: "obj_a", Body: &annotation.BodyGeometry{
		BodyID: "body-a", AxisConvention: annotation.BodyAxisConvention, Length: unknown, Width: unknown, Height: unknown,
		Review: annotation.PhysicalReview{Status: annotation.StatusReviewed, Origin: annotation.OriginIndependent,
			Method: "manual_box", Provenance: annotation.Provenance{Author: "op"}},
	}, Keyframes: []annotation.PhysicalKeyframe{{
		KeyframeID: fmt.Sprintf("kf-a-%d", r.Revision), SampleID: 0, TimestampNs: p.Samples[0].TimestampNs,
		Anchor:   annotation.PhysicalAnchor{Kind: annotation.AnchorBodyCentre},
		Position: annotation.PositionBound{Status: annotation.EvidenceUnknown},
		Yaw:      annotation.YawBound{Status: annotation.EvidenceUnknown, Axis: annotation.AxisUnknown},
		Front:    annotation.EndpointEvidence{Status: annotation.EvidenceUnknown},
		Rear:     annotation.EndpointEvidence{Status: annotation.EvidenceUnknown},
		Review: annotation.PhysicalReview{Status: annotation.StatusProposed, Origin: annotation.OriginIndependent,
			Method: "manual_box", Provenance: annotation.Provenance{Author: "op"}},
	}}}}
	if err := annotation.SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	return r
}

// writeDraft writes a draft beside the packs, naming them relative to it.
func writeDraft(t *testing.T, dir string, packs ...annotation.DraftPack) string {
	t.Helper()
	d := annotation.SplitDraft{Schema: annotation.SplitDraftSchema, SchemaVersion: annotation.SplitDraftSchemaVersion,
		Cases: []annotation.SplitCase{{CaseID: "kirk0", Role: annotation.SplitRoleTuning,
			Captures: []annotation.CaseCapture{{Basename: "kirk0.pcapng", SHA256: kirk0Digest}}}}, Packs: packs}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "draft.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func draftPack(dir string, heldOut []string) annotation.DraftPack {
	role := annotation.SplitRoleTuning
	name := "tune"
	if len(heldOut) > 0 {
		role, name = annotation.SplitRoleHeldOut, "hold"
	}
	ids := []string{"obj_a", "obj_b"}
	return annotation.DraftPack{
		Dir:      dir,
		Splits:   []annotation.Split{{Name: name, Role: role, ObjectIDs: ids}},
		Episodes: []annotation.Episode{{EpisodeID: dir, Split: name, ObjectIDs: ids, FrameIntervals: []annotation.FrameInterval{{FirstSample: 0, LastSample: 2}}}},
	}
}

func runSplit(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := annotationSplitMain(args, &stdout, &stderr, splitNow)
	return code, stdout.String(), stderr.String()
}

func TestAnnotationSplitRoutingAndFlags(t *testing.T) {
	if code := silence(t, func() int { return Main([]string{"annotation-split", "help"}) }); code != 0 {
		t.Errorf("help exited %d", code)
	}
	for name, args := range map[string][]string{
		"no command":        nil,
		"unknown command":   {"thaw"},
		"bad freeze flag":   {"freeze", "-nope"},
		"freeze extra args": {"freeze", "-draft", "d", "-author", "a", "-output", "o", "extra"},
		"freeze no draft":   {"freeze", "-author", "a", "-output", "o"},
		"freeze no author":  {"freeze", "-draft", "d", "-author", " ", "-output", "o"},
		"verify no split":   {"verify"},
		"bad verify flag":   {"verify", "-nope"},
	} {
		if code, _, stderr := runSplit(args...); code != 2 {
			t.Errorf("%s: exit %d, want 2 (%s)", name, code, stderr)
		}
	}
	for _, sub := range []string{"freeze", "verify"} {
		if code, _, _ := runSplit(sub, "-h"); code != 0 {
			t.Errorf("%s -h exited %d", sub, code)
		}
	}
}

// Freeze, then verify against the packs: the round trip an operator makes
// before handing a split to the evaluators. A later review revision does not
// fail verification, but is reported.
func TestAnnotationSplitFreezeThenVerify(t *testing.T) {
	root := t.TempDir()
	tuned := reviewedPack(t, filepath.Join(root, "tuned"), 1e9)
	held := reviewedPack(t, filepath.Join(root, "held"), 100e9)
	draft := writeDraft(t, root, draftPack("tuned", nil), draftPack("held", []string{"obj_a", "obj_b"}))
	out := filepath.Join(root, "frozen.json")

	code, stdout, stderr := runSplit("freeze", "-draft", draft, "-author", "operator", "-output", out, "-for-params-hash", "sha256:p")
	if code != 0 {
		t.Fatalf("freeze exited %d: %s", code, stderr)
	}
	f, err := annotation.LoadFrozenSplit(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"frozen split: " + out, "split " + f.SplitDigest + ", revision 1\n", "frozen by operator at 2026-09-29T10:00:00Z",
		"case kirk0: tuning, captures kirk0.pcapng", "tuned in this lineage: 1 pack(s), 1 case(s)",
		"objects tune 2; 1 episode(s); geometry review none 2", "objects hold 2",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("freeze output lacks %q:\n%s", want, stdout)
		}
	}
	if f.Frozen.ForParamsHash != "sha256:p" || f.GuardSeconds != annotation.DefaultSplitGuardSeconds {
		t.Fatalf("frozen record %+v, guard %g", f.Frozen, f.GuardSeconds)
	}
	if code, _, stderr := runSplit("freeze", "-draft", draft, "-author", "operator", "-output", out); code != 1 || !strings.Contains(stderr, "create frozen split") {
		t.Fatalf("an existing frozen split was replaced: exit %d, %s", code, stderr)
	}

	saveReview(t, held, "revised")
	code, stdout, stderr = runSplit("verify", "-split", out, "-pack", tuned.Dir, "-pack", held.Dir)
	if code != 0 {
		t.Fatalf("verify exited %d: %s", code, stderr)
	}
	for _, want := range []string{
		"pack " + tuned.Manifest.PackDigest + ": every pin holds at annotation revision 1",
		"annotation revision 2 exists; this split scores revision 1",
		"verified: 2 pack(s), 1 case role(s)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("verify output lacks %q:\n%s", want, stdout)
		}
	}

	// A successor revision against the new review.
	next := filepath.Join(root, "frozen-2.json")
	code, stdout, stderr = runSplit("freeze", "-draft", draft, "-author", "operator", "-output", next, "-supersedes", out)
	if code != 0 || !strings.Contains(stdout, "revision 2, supersedes "+f.SplitDigest) {
		t.Fatalf("successor: exit %d\n%s\n%s", code, stdout, stderr)
	}
	// Its successor cannot hold out the pack revision 1 tuned on.
	undo := filepath.Join(t.TempDir(), "undo")
	if err := os.Mkdir(undo, 0o755); err != nil {
		t.Fatal(err)
	}
	undoDraft := writeDraft(t, undo, draftPack(tuned.Dir, []string{"obj_a", "obj_b"}))
	code, _, stderr = runSplit("freeze", "-draft", undoDraft, "-author", "operator", "-output", filepath.Join(undo, "frozen-3.json"), "-supersedes", next)
	if code != 1 || !strings.Contains(stderr, "but revision 1 of this split's lineage tuned on it") {
		t.Fatalf("holding out a tuned pack: exit %d, %s", code, stderr)
	}
}

// A pack with physical references is frozen with a pin at the revision the
// draft names, or its head, and the summary says which; a pack without is
// said to have none. A revision named for a pack without references is
// refused.
func TestAnnotationSplitFreezePinsPhysicalReferences(t *testing.T) {
	root := t.TempDir()
	tuned := reviewedPack(t, filepath.Join(root, "tuned"), 1e9)
	held := reviewedPack(t, filepath.Join(root, "held"), 100e9)
	first := savePhysical(t, tuned)
	second := savePhysical(t, tuned)
	if first.Revision != 1 || second.Revision != 2 {
		t.Fatalf("revisions %d and %d", first.Revision, second.Revision)
	}
	pinned := draftPack("tuned", nil)
	pinned.PhysicalRevision = 1
	draft := writeDraft(t, root, pinned, draftPack("held", []string{"obj_a", "obj_b"}))
	out := filepath.Join(root, "frozen.json")

	code, stdout, stderr := runSplit("freeze", "-draft", draft, "-author", "operator", "-output", out)
	if code != 0 {
		t.Fatalf("freeze exited %d: %s", code, stderr)
	}
	f, err := annotation.LoadFrozenSplit(out)
	if err != nil {
		t.Fatal(err)
	}
	var pin *annotation.FrozenPhysical
	for _, p := range f.Packs {
		if p.PackDigest == tuned.Manifest.PackDigest {
			pin = p.Physical
		} else if p.Physical != nil {
			t.Fatalf("pack %s without references was pinned: %+v", p.PackDigest, p.Physical)
		}
	}
	if pin == nil || pin.Revision != 1 || pin.SHA256 != first.Digest() {
		t.Fatalf("pin %+v, want revision 1 (%s)", pin, first.Digest())
	}
	for _, want := range []string{
		"pack " + tuned.Manifest.PackDigest + " (" + tuned.Manifest.DatasetID + ") at annotation revision 1: objects tune 2; 1 episode(s); geometry review none 2; " +
			"physical revision 1 (" + first.Digest() + "): 1 object(s), 1 reviewed independent bod(ies), 0 of 1 keyframe(s) reviewed; " +
			"scorable position 0/0, yaw 0/0, length 0/0, width 0/0, height 0/0, front 0/0, rear 0/0",
		"pack " + held.Manifest.PackDigest + " (" + held.Manifest.DatasetID + ") at annotation revision 1: objects hold 2; 1 episode(s); geometry review none 2; no physical references",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("freeze output lacks %q:\n%s", want, stdout)
		}
	}
	code, stdout, stderr = runSplit("verify", "-split", out, "-pack", tuned.Dir, "-pack", held.Dir)
	if code != 0 || !strings.Contains(stdout, "physical revision 1 ("+first.Digest()+")") {
		t.Fatalf("verify: exit %d\n%s\n%s", code, stdout, stderr)
	}

	// The head, when the draft names no revision.
	headDir := t.TempDir()
	headDraft := writeDraft(t, headDir, draftPack(tuned.Dir, nil))
	headOut := filepath.Join(headDir, "frozen.json")
	code, stdout, stderr = runSplit("freeze", "-draft", headDraft, "-author", "operator", "-output", headOut)
	if code != 0 || !strings.Contains(stdout, "physical revision 2 ("+second.Digest()+")") {
		t.Fatalf("freeze at the head: exit %d\n%s\n%s", code, stdout, stderr)
	}

	// A revision for a pack with none.
	none := draftPack(held.Dir, nil)
	none.PhysicalRevision = 1
	noneDir := t.TempDir()
	noneDraft := writeDraft(t, noneDir, none)
	code, _, stderr = runSplit("freeze", "-draft", noneDraft, "-author", "operator", "-output", filepath.Join(noneDir, "frozen.json"))
	if code != 1 || !strings.Contains(stderr, "physical_revision 1 is named, but the pack has no physical references to pin") {
		t.Fatalf("naming a revision for a pack without references: exit %d, %s", code, stderr)
	}
}

func TestAnnotationSplitRefusals(t *testing.T) {
	root := t.TempDir()
	tuned := reviewedPack(t, filepath.Join(root, "tuned"), 1e9)
	held := reviewedPack(t, filepath.Join(root, "held"), 100e9)
	draft := writeDraft(t, root, draftPack("tuned", nil))
	out := filepath.Join(root, "frozen.json")
	if code, _, stderr := runSplit("freeze", "-draft", draft, "-author", "operator", "-output", out); code != 0 {
		t.Fatalf("freeze: %s", stderr)
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing draft", []string{"freeze", "-draft", filepath.Join(root, "none.json"), "-author", "a", "-output", filepath.Join(root, "x.json")}, "open split draft"},
		{"missing predecessor", []string{"freeze", "-draft", draft, "-author", "a", "-output", filepath.Join(root, "x.json"), "-supersedes", filepath.Join(root, "none.json")}, "supersedes"},
		{"unfreezable draft", []string{"freeze", "-draft", writeDraft(t, t.TempDir(), draftPack(filepath.Join(root, "absent"), nil)), "-author", "a", "-output", filepath.Join(root, "x.json")}, "read pack manifest"},
		{"missing split", []string{"verify", "-split", filepath.Join(root, "none.json")}, "open split manifest"},
		{"unreadable pack", []string{"verify", "-split", out, "-pack", filepath.Join(root, "absent")}, "read manifest"},
		{"pack not in the split", []string{"verify", "-split", out, "-pack", tuned.Dir, "-pack", held.Dir}, "is not in frozen split"},
		{"pack not supplied", []string{"verify", "-split", out}, "was not verified: pass its directory with --pack"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runSplit(tc.args...)
			if code != 1 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit %d, stderr %q; want 1 mentioning %q", code, stderr, tc.want)
			}
		})
	}
}

// saveFacet saves one facet proposal on obj_a, supported by its two returns
// at sample 0, as the pack's facet head.
func saveFacet(t *testing.T, p *annotation.Pack) *pb.FeatureState {
	t.Helper()
	s, err := annotation.LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	state, err := annotation.LoadFeatures(p)
	if err != nil {
		t.Fatal(err)
	}
	state.Document.Author = "operator"
	state.Document.Features = []*pb.FeatureCandidate{{FeatureId: "tip", ObjectId: "obj_a", Name: "Tip", PartId: "body", PartRelation: "unknown",
		Geometry: pb.FeatureGeometry_FEATURE_GEOMETRY_PROTRUSION, Observations: []*pb.FeatureObservation{{
			SampleId: 0, TimestampNs: p.Samples[0].TimestampNs, SourceOrdinal: 0, PointIndices: []uint32{0, 1},
			Sphere:   &pb.FeatureSphere{XM: 1.5, RadiusM: 0.6},
			Decision: pb.FeatureDecision_FEATURE_DECISION_ACCEPTED_PROPOSAL, Method: "manual_sphere", Origin: "human_proposal", Author: "operator",
			MembershipRevision: uint64(s.Revision), MembershipDigest: s.Digest()}}}}
	saved, err := annotation.SaveFeatures(p, &pb.FeatureEdit{Document: state.Document, MembershipDigest: state.MembershipDigest})
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

// No evaluator reads facet proposals, so verify is where their pin is held:
// it passes while the pinned bytes hold, and fails once they change.
func TestAnnotationSplitVerifyHoldsTheFacetPin(t *testing.T) {
	root := t.TempDir()
	tuned := reviewedPack(t, filepath.Join(root, "tuned"), 1e9)
	state := saveFacet(t, tuned)
	pinned := draftPack("tuned", nil)
	head := 0
	pinned.FeatureRevision = &head
	out := filepath.Join(root, "frozen.json")
	if code, _, stderr := runSplit("freeze", "-draft", writeDraft(t, root, pinned), "-author", "operator", "-output", out); code != 0 {
		t.Fatalf("freeze: %s", stderr)
	}
	code, stdout, stderr := runSplit("verify", "-split", out, "-pack", tuned.Dir)
	if code != 0 || !strings.Contains(stdout, "facet revision 1 holds against that membership: 1 candidate(s), proposals, not truth") {
		t.Fatalf("verify: exit %d\n%s\n%s", code, stdout, stderr)
	}

	// The same revision with other bytes is not the pin.
	state.Document.Features[0].Name = "Edited after freezing"
	b, err := proto.Marshal(state.Document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tuned.Dir, "feature-proposals.pb"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runSplit("verify", "-split", out, "-pack", tuned.Dir)
	if code != 1 || !strings.Contains(stderr, "facet bytes or derived summary differ from the frozen pin") || strings.Contains(stdout, "every pin holds") {
		t.Fatalf("an edited facet pin verified: exit %d\n%s\n%s", code, stdout, stderr)
	}
}
