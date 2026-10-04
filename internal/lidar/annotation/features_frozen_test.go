package annotation

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"google.golang.org/protobuf/proto"
)

func savedFrozenFacets(t *testing.T) (*Pack, *pb.FeatureState, *Sidecar, DraftPack) {
	t.Helper()
	p, edit := featureFixture(t)
	state, err := SaveFeatures(p, edit)
	if err != nil {
		t.Fatal(err)
	}
	s, err := LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	draft := physDraftPack(p, 0)
	head := 0
	draft.FeatureRevision = &head
	return p, state, s, draft
}

func TestFrozenFacetsOptInRetainEvidenceAfterNewHeads(t *testing.T) {
	p, state, s, draft := savedFrozenFacets(t)
	without := draft
	without.FeatureRevision = nil
	old := mustFreeze(t, freezeOptions(without))
	if old.SchemaVersion != 3 || old.Packs[0].Features != nil {
		t.Fatal("feature-free layout changed")
	}
	if d, e := old.BindFeatures(p, s); e != nil || d != nil {
		t.Fatal(e)
	}
	preview, err := PreviewFreeze(freezeOptions(draft))
	if err != nil || !preview.WouldFreeze {
		t.Fatal(err, preview)
	}
	f := mustFreeze(t, freezeOptions(draft))
	if f.SchemaVersion != 4 || preview.Packs[0].Features == nil || *f.Packs[0].Features != *preview.Packs[0].Features {
		t.Fatal("facet preview not pinned")
	}
	_, frozenPath := writeAndLoad(t, f)
	f, err = LoadFrozenSplit(frozenPath)
	if err != nil {
		t.Fatal(err)
	}
	_, bound, err := f.Bind(p)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := f.BindFeatures(p, bound)
	if err != nil || !proto.Equal(doc, state.Document) {
		t.Fatal("wrong frozen evidence", err)
	}
	// Metadata revisions and retired history do not rewrite the frozen subset.
	next := proto.Clone(state.Document).(*pb.FeatureAnnotations)
	next.Features[0].Name = "Changed"
	next.Features[0].Inactive = true
	if _, err = SaveFeatures(p, &pb.FeatureEdit{Document: next, BaseDigest: state.Digest, MembershipDigest: state.MembershipDigest}); err != nil {
		t.Fatal(err)
	}
	s.Change = Provenance{Author: "op", Operation: "review"}
	if err = SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	// Head damage is not consulted when the requested revision is archived.
	os.WriteFile(filepath.Join(p.Dir, featureFile), []byte{0xff}, 0600)
	doc, err = f.BindFeatures(p, bound)
	if err != nil || !proto.Equal(doc, state.Document) {
		t.Fatal("later head changed frozen evidence", err)
	}
	if doc.Features[0].Inactive || doc.Features[0].Name == "Changed" {
		t.Fatal("read latest instead of frozen")
	}
}

func TestFrozenFacetsMembershipDriftIsASeparatePreviewProblem(t *testing.T) {
	p, _, s, draft := savedFrozenFacets(t)
	// A harmless revision retains the subset and can be pinned.
	s.Change = Provenance{Author: "op", Operation: "review"}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewFreeze(freezeOptions(draft))
	if err != nil || !preview.WouldFreeze {
		t.Fatal(err, preview)
	}
	// A formerly definite return now uncertain cannot support a frozen facet.
	s.Masks[0].PointIndices = []int{1, 2, 3, 4, 5, 6, 7}
	s.Masks[0].UncertainIndices = []int{0}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	preview, err = PreviewFreeze(freezeOptions(draft))
	if err != nil || preview.WouldFreeze || len(preview.FacetProblems) != 1 || len(preview.PhysicalProblems) != 0 {
		t.Fatal(err, preview)
	}
	if _, err = FreezeSplit(freezeOptions(draft)); err == nil || !strings.Contains(err.Error(), "facet proposals") {
		t.Fatal("drift froze", err)
	}
	doc, _ := LoadFeatures(p)
	if len(facetMembershipProblems(&Sidecar{}, doc.Document)) != 1 {
		t.Fatal("missing object accepted")
	}
	rejected := &Sidecar{Objects: []Object{{ObjectID: "car-1", Status: StatusRejected}}}
	if len(facetMembershipProblems(rejected, doc.Document)) != 1 {
		t.Fatal("rejected object accepted")
	}
	// Explicit absence has no measured subset to compare.
	absence := proto.Clone(doc.Document).(*pb.FeatureAnnotations)
	absence.Features[0].Observations[0].Decision = pb.FeatureDecision_FEATURE_DECISION_OCCLUDED
	absence.Features[0].Observations[0].PointIndices = nil
	if len(facetMembershipProblems(s, absence)) != 0 {
		t.Fatal("occlusion treated as measured support")
	}
}

// A mask rejected in the frozen membership supports no facet, even though its
// indices are still recorded: authoring refuses such support, and so must the
// freeze.
func TestFrozenFacetsRejectedMaskGivesNoSupport(t *testing.T) {
	p, state, s, draft := savedFrozenFacets(t)
	if problems := facetMembershipProblems(s, state.Document); len(problems) != 0 {
		t.Fatal(problems)
	}
	s.Masks[0].Status = StatusRejected
	if s.Masks[0].ObjectID != "car-1" || s.Masks[0].SampleID != 0 {
		t.Fatal("fixture moved the facet's mask")
	}
	s.Change = Provenance{Author: "op", Operation: "reject"}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewFreeze(freezeOptions(draft))
	if err != nil || preview.WouldFreeze || len(preview.MembershipProblems) != 0 || len(preview.FacetProblems) != 1 ||
		!strings.HasPrefix(preview.FacetProblems[0], "pack "+p.Manifest.PackDigest+" facet revision 1: facet mirror sample 0") ||
		!strings.Contains(preview.FacetProblems[0], "is not definite support of object car-1") {
		t.Fatal(err, preview)
	}
}

// Opting a pack with no saved facets into a head pin is a listed facet
// problem: the preview still reports every pack, and the freeze refuses.
func TestFrozenFacetHeadPinWithoutSavedFacetsIsAProblem(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	draft := physDraftPack(p, 0)
	head := 0
	draft.FeatureRevision = &head
	preview, err := PreviewFreeze(freezeOptions(draft))
	if err != nil {
		t.Fatalf("an unsaved facet head aborted the preview: %v", err)
	}
	want := "pack " + p.Manifest.PackDigest + ": feature_revision 0 pins the saved facet head, but the pack has no saved facet proposals"
	if preview.WouldFreeze || len(preview.Packs) != 1 || preview.Packs[0].Features != nil || len(preview.MembershipProblems) != 0 ||
		len(preview.FacetProblems) != 1 || !strings.HasPrefix(preview.FacetProblems[0], want) {
		t.Fatalf("%+v", preview)
	}
	if _, err = FreezeSplit(freezeOptions(draft)); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("froze without the facets it asked for: %v", err)
	}
	// Omitting the pin freezes the feature-free layout.
	draft.FeatureRevision = nil
	if f := mustFreeze(t, freezeOptions(draft)); f.SchemaVersion != FrozenSplitSchemaVersion || f.Packs[0].Features != nil {
		t.Fatal("feature-free freeze changed")
	}
}

// A saved head whose revision no pin can name is refused, not listed: it is a
// damaged file, not a review problem an operator can fix by editing facets.
func TestFrozenFacetHeadWithUnpinnableRevisionIsRefused(t *testing.T) {
	p, state, s, draft := savedFrozenFacets(t)
	state.Document.Revision = math.MaxInt32 + 1
	b, err := proto.Marshal(state.Document)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(p.Dir, featureFile), b, 0600); err != nil {
		t.Fatal(err)
	}
	if pin, problems, err := freezeFeatures(p, s, draft.FeatureRevision); err == nil || pin != nil || problems != nil {
		t.Fatal("unpinnable head froze", pin, problems)
	}
}

func TestFrozenFacetPinAndLoadRefusals(t *testing.T) {
	p, state, s, draft := savedFrozenFacets(t)
	f := mustFreeze(t, freezeOptions(draft))
	pin := *f.Packs[0].Features
	mutations := []func(*FrozenFeatures){
		func(p *FrozenFeatures) { p.Revision = 0 }, func(p *FrozenFeatures) { p.Revision = math.MaxInt32 + 1 },
		func(p *FrozenFeatures) { p.SHA256 = "no" }, func(p *FrozenFeatures) { p.ContentSHA256 = "sha256:" + strings.Repeat("g", 64) },
		func(p *FrozenFeatures) { p.Candidates = -1 }, func(p *FrozenFeatures) { p.Active = 2 },
		func(p *FrozenFeatures) { p.SupportedObservations = -1 }, func(p *FrozenFeatures) { p.AbsenceDecisions = -1 },
		func(p *FrozenFeatures) { p.Registrations = 2 }, func(p *FrozenFeatures) { p.TrackerSeededRegistrations = 1 },
	}
	for i, change := range mutations {
		bad := pin
		change(&bad)
		if bad.validate() == nil {
			t.Fatalf("invalid pin %d accepted", i)
		}
		f.Packs[0].Features = &bad
		if f.validate() == nil {
			t.Fatalf("invalid frozen structure %d accepted", i)
		}
	}
	f.Packs[0].Features = &pin
	f.SchemaVersion = 3
	if f.validate() == nil {
		t.Fatal("v3 carried facets")
	}
	f.SchemaVersion = 4
	wrong := splitPack(t, "other", "other.pcap", 1, 2)
	if _, err := f.BindFeatures(wrong, s); err == nil {
		t.Fatal("wrong pack")
	}
	for _, given := range []*Sidecar{nil, NewSidecar(p)} {
		if _, err := f.BindFeatures(p, given); err == nil {
			t.Fatal("wrong membership")
		}
	}
	f.Packs[0].Features.SupportedObservations++
	if _, err := f.BindFeatures(p, s); err == nil {
		t.Fatal("forged summary")
	}
	*f.Packs[0].Features = pin
	// Byte identity is checked even when decoded evidence remains valid.
	state.Document.Features[0].Name = "Changed bytes"
	b, _ := proto.Marshal(state.Document)
	os.WriteFile(filepath.Join(p.Dir, featureFile), b, 0600)
	if _, err := f.BindFeatures(p, s); err == nil {
		t.Fatal("changed bytes")
	}
	os.WriteFile(filepath.Join(p.Dir, featureFile), []byte{0xff}, 0600)
	if _, err := f.BindFeatures(p, s); err == nil {
		t.Fatal("corrupt pinned bytes")
	}
	if _, _, err := freezeFeatures(p, s, draft.FeatureRevision); err == nil {
		t.Fatal("corrupt head froze")
	}
	negative := -1
	if _, _, err := freezeFeatures(p, s, &negative); err == nil {
		t.Fatal("negative revision")
	}
	missing := 99
	if _, _, err := freezeFeatures(p, s, &missing); err == nil {
		t.Fatal("missing revision")
	}
	// With no head and no history the head is unsaved: a problem, not a pin.
	os.Remove(filepath.Join(p.Dir, featureFile))
	if unsaved, problems, err := freezeFeatures(p, s, draft.FeatureRevision); err != nil || unsaved != nil || len(problems) != 1 {
		t.Fatal("unsaved revision", err, unsaved, problems)
	}
	// Protobuf encoding failures are refusals, never synthetic digests.
	state.Document.Features[0].Name = string([]byte{0xff})
	if _, err := newFrozenFeatures(state); err == nil {
		t.Fatal("invalid text")
	}
	// The optional field survives draft JSON without enabling it implicitly.
	encoded, _ := json.Marshal(draft)
	var roundTrip DraftPack
	if err := json.Unmarshal(encoded, &roundTrip); err != nil || roundTrip.FeatureRevision == nil || *roundTrip.FeatureRevision != 0 {
		t.Fatal(err)
	}
}

func TestFrozenFacetSummaryRetainsAssistedAndRetiredHistory(t *testing.T) {
	p, e := registeredFeatureFixture(t)
	f := e.Document.Features[0]
	retired := proto.Clone(f).(*pb.FeatureCandidate)
	retired.FeatureId = "retired"
	retired.Inactive = true
	retired.Anchor = nil
	retired.PartRelation = "unknown"
	retired.Observations[0].Decision = pb.FeatureDecision_FEATURE_DECISION_MISSING
	retired.Observations[0].PointIndices = nil
	e.Document.Features = append(e.Document.Features, retired)
	state, err := SaveFeatures(p, e)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := newFrozenFeatures(state)
	if err != nil {
		t.Fatal(err)
	}
	if pin.Candidates != 2 || pin.Active != 1 || pin.Registrations != 1 || pin.TrackerSeededRegistrations != 0 || pin.SupportedObservations != 3 || pin.AbsenceDecisions != 1 {
		t.Fatal(pin)
	}
	// Counting assistance remains separate even for an already validated record.
	state.Document.Features[0].Anchor.Origin = "tracker_seeded_proposal"
	assisted, err := newFrozenFeatures(state)
	if err != nil || assisted.TrackerSeededRegistrations != 1 {
		t.Fatal(err, assisted)
	}
}

func TestFrozenFacetHistoricalRequestAndBindDrift(t *testing.T) {
	p, state, s, draft := savedFrozenFacets(t)
	next, err := SaveFeatures(p, &pb.FeatureEdit{Document: state.Document, BaseDigest: state.Digest, MembershipDigest: state.MembershipDigest})
	if err != nil {
		t.Fatal(err)
	}
	revision := 1
	draft.FeatureRevision = &revision
	f := mustFreeze(t, freezeOptions(draft))
	if f.Packs[0].Features.Revision != 1 || f.Packs[0].Features.SHA256 == next.Digest {
		t.Fatal("did not honour historical revision")
	}
	// Bind refuses altered evidence even if the supplied object claims the pin.
	s.Masks[0].PointIndices = []int{2, 3}
	if _, err := f.BindFeatures(p, s); err == nil || !strings.Contains(err.Error(), "frozen membership") {
		t.Fatal(err)
	}
}
