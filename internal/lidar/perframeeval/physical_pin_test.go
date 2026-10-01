package perframeeval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// hasCaveat reports whether any caveat mentions s.
func hasCaveat(caveats []string, s string) bool {
	for _, c := range caveats {
		if strings.Contains(c, s) {
			return true
		}
	}
	return false
}

// A frozen split that pins a physical revision is scored at that revision,
// bound through the pin: the identity says so, an explicit revision may only
// agree, and no caveat says the revision was unpinned.
func TestPhysicalScoringThroughAPinnedSplit(t *testing.T) {
	f := physFixture(t)
	frozen, path := physFrozen(t, f)
	pin := frozen.Packs[0].Physical
	if pin == nil || pin.Revision != 1 {
		t.Fatalf("the fixture's split pins %+v", pin)
	}
	opts := physRefOpts(f)
	opts.SplitManifestPath = path

	pr, err := LoadPhysicalReference(opts, DefaultPhysicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	id := pr.Identity
	if !id.PhysicalPinned || id.PhysicalRevision != 1 || id.PhysicalRevisionDigest != pin.SHA256 || id.PhysicalContentDigest != pin.ContentSHA256 ||
		id.SplitDigest != frozen.SplitDigest || pr.FrozenWithoutPhysicalPin {
		t.Fatalf("identity %+v against pin %+v", id, pin)
	}

	matching := DefaultPhysicalOptions()
	matching.Revision = 1
	same, err := LoadPhysicalReference(opts, matching)
	if err != nil {
		t.Fatalf("the pinned revision, named explicitly, was refused: %v", err)
	}
	if same.Identity.Digest != id.Digest {
		t.Fatal("naming the pinned revision changed the reference")
	}
	conflicting := DefaultPhysicalOptions()
	conflicting.Revision = 2
	_, err = LoadPhysicalReference(opts, conflicting)
	if err == nil || !strings.Contains(err.Error(), "revision 2 was asked for") || !strings.Contains(err.Error(), "pins revision 1") {
		t.Fatalf("a conflicting revision was not refused by name: %v", err)
	}

	cfg := physConfig(f)
	cfg.Reference.SplitManifestPath = path
	c, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Physical.Reference.PhysicalPinned || hasCaveat(c.Physical.A.Caveats, "pins no physical revision") {
		t.Fatalf("physical section %+v, caveats %v", c.Physical.Reference, c.Physical.A.Caveats)
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"physical_pinned": true`) {
		t.Fatal("the comparison JSON does not record physical_pinned")
	}
}

// A membership save and a physical save after freezing move both heads and
// make the head's links stale. The frozen split still binds the revisions it
// pinned: the identity and every outcome are what they were.
func TestPhysicalPinnedSplitSurvivesLaterSaves(t *testing.T) {
	f := physFixture(t)
	_, path := physFrozen(t, f)
	cfg := physConfig(f)
	cfg.Reference.SplitManifestPath = path
	before, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}

	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := annotation.LoadPhysicalReferences(pack)
	if err != nil {
		t.Fatal(err)
	}
	bound := 0.3
	refs.Objects[0].Keyframes[0].Position.BoundM = &bound
	refs.Change = annotation.Provenance{Author: "operator", Operation: "widen"}
	if err := annotation.SavePhysicalReferences(pack, refs); err != nil {
		t.Fatal(err)
	}
	s, err := annotation.LoadSidecar(pack)
	if err != nil {
		t.Fatal(err)
	}
	for i := range s.Objects {
		if s.Objects[i].ObjectID == evalfixture.Follower {
			s.Objects[i].Status = annotation.StatusRejected
		}
	}
	s.Change = annotation.Provenance{Author: "operator", Operation: "reject"}
	if err := annotation.SaveSidecar(pack, s); err != nil {
		t.Fatal(err)
	}
	if head, err := annotation.LoadPhysicalReferences(pack); err != nil || head.Revision != 2 || len(head.Stale()) == 0 {
		t.Fatalf("the heads did not move as intended: %+v, %v", head, err)
	}

	after, err := Run(cfg)
	if err != nil {
		t.Fatalf("the pinned split no longer scores after later saves: %v", err)
	}
	if !reflect.DeepEqual(after.Physical.Reference, before.Physical.Reference) || after.Physical.Reference.PhysicalRevision != 1 ||
		after.Physical.Reference.SidecarRevision != 1 {
		t.Fatalf("identity moved from %+v to %+v", before.Physical.Reference, after.Physical.Reference)
	}
	for _, arm := range []struct{ a, b PhysicalResult }{{before.Physical.A, after.Physical.A}, {before.Physical.B, after.Physical.B}} {
		if !reflect.DeepEqual(arm.a.Instants, arm.b.Instants) || !reflect.DeepEqual(arm.a.Following, arm.b.Following) ||
			!reflect.DeepEqual(arm.a.Summary, arm.b.Summary) || !reflect.DeepEqual(arm.a.Caveats, arm.b.Caveats) {
			t.Fatalf("arm %s outcomes moved", arm.a.Arm.Label)
		}
	}
	// The head, scored through the version 1 manifest, is a different reference.
	head, err := Run(physConfig(f))
	if err == nil && head.Physical.Reference.Digest == before.Physical.Reference.Digest {
		t.Fatal("the head scores as the pin")
	}
}

// A version 2 split, frozen before physical pins existed, still binds and
// scores as it did: the head physical revision, with a caveat that the
// split pins none, and physical_pinned absent from the identity.
func TestPhysicalLegacyVersion2SplitScoresTheHead(t *testing.T) {
	f := physFixture(t)
	path := filepath.Join(t.TempDir(), "v2.json")
	legacy, err := f.WriteFrozenMembershipOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := annotation.LoadFrozenSplit(path)
	if err != nil || loaded.SchemaVersion != annotation.FrozenSplitSchemaVersionMembershipOnly || loaded.Packs[0].Physical != nil {
		t.Fatalf("legacy split %+v, %v", loaded, err)
	}
	cfg := physConfig(f)
	cfg.Reference.SplitManifestPath = path
	c, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ref := c.Physical.Reference
	if ref.PhysicalPinned || ref.SplitDigest != legacy.SplitDigest || ref.SplitRevision != 1 || ref.PhysicalRevision != 1 ||
		c.Reference.SplitDigest != legacy.SplitDigest {
		t.Fatalf("physical section %+v, reference %+v", ref, c.Reference)
	}
	if !hasCaveat(c.Physical.A.Caveats, "pins no physical revision") || !hasCaveat(c.Physical.B.Caveats, "revision 1 was the pack's current") {
		t.Fatalf("caveats %v", c.Physical.A.Caveats)
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "physical_pinned") {
		t.Fatal("an unpinned identity records physical_pinned")
	}
	// The scores are the version 1 manifest's, as before pins existed.
	v1, err := Run(physConfig(f))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Physical.A.Instants, v1.Physical.A.Instants) || !reflect.DeepEqual(c.Physical.A.Summary, v1.Physical.A.Summary) {
		t.Fatal("the legacy split's physical scores differ from its version 1 form's")
	}

	// Unpinned, an explicit revision is honoured, and the head moves the score.
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := annotation.LoadPhysicalReferences(pack)
	if err != nil {
		t.Fatal(err)
	}
	bound := 0.3
	refs.Objects[0].Keyframes[0].Position.BoundM = &bound
	refs.Change = annotation.Provenance{Author: "operator", Operation: "widen"}
	if err := annotation.SavePhysicalReferences(pack, refs); err != nil {
		t.Fatal(err)
	}
	moved, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Physical.Revision = 1
	explicit, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Physical.Reference.PhysicalRevision != 2 || explicit.Physical.Reference.PhysicalRevision != 1 ||
		explicit.Physical.Reference.Digest != ref.Digest {
		t.Fatalf("head %+v, explicit %+v", moved.Physical.Reference, explicit.Physical.Reference)
	}
}

// A pin that no longer holds refuses the evaluation: retained bytes that
// were rewritten are not the revision the split pinned.
func TestPhysicalScoringRefusesATamperedPin(t *testing.T) {
	f := physFixture(t)
	frozen, path := physFrozen(t, f)
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	// Move the head on, so revision 1 is read from the archive, then
	// rewrite the archived bytes without changing their content.
	refs, err := annotation.LoadPhysicalReferences(pack)
	if err != nil {
		t.Fatal(err)
	}
	bound := 0.3
	refs.Objects[0].Keyframes[0].Position.BoundM = &bound
	refs.Change = annotation.Provenance{Author: "operator", Operation: "widen"}
	if err := annotation.SavePhysicalReferences(pack, refs); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(f.PackDir, "physical-reference-revisions", "0000000001.json")
	b, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, append(append([]byte(nil), b...), '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := physRefOpts(f)
	opts.SplitManifestPath = path
	_, err = LoadPhysicalReference(opts, DefaultPhysicalOptions())
	if err == nil || !strings.Contains(err.Error(), "is not the bytes that were frozen") || !strings.Contains(err.Error(), frozen.Packs[0].Physical.SHA256) {
		t.Fatalf("a tampered pinned revision was scored: %v", err)
	}
}
