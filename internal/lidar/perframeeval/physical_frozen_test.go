package perframeeval

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// physFrozen freezes the physical fixture's split and returns it and its path.
func physFrozen(t *testing.T, f *evalfixture.PhysicalFixture) (*annotation.FrozenSplit, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "frozen.json")
	frozen, err := f.WriteFrozen(path)
	if err != nil {
		t.Fatal(err)
	}
	return frozen, path
}

// Physical scoring takes the frozen split membership scoring takes, through
// the same binding: both sections name the same split, sidecar revision and
// episodes, and the physical numbers are what the version 1 form of the same
// split gives.
func TestPhysicalScoringBindsAFrozenSplit(t *testing.T) {
	f := physFixture(t)
	_, path := physFrozen(t, f)
	frozen, err := annotation.LoadFrozenSplit(path) // sets FileDigest
	if err != nil {
		t.Fatal(err)
	}

	v1, err := Run(physConfig(f))
	if err != nil {
		t.Fatal(err)
	}
	cfg := physConfig(f)
	cfg.Reference.SplitManifestPath = path
	c, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ref, phys := c.Reference, c.Physical.Reference
	if phys.SplitDigest != frozen.SplitDigest || phys.SplitRevision != frozen.Revision ||
		ref.SplitDigest != phys.SplitDigest || ref.SplitRevision != phys.SplitRevision {
		t.Fatalf("physical identity %+v and reference %+v do not both name frozen split %s", phys, ref, frozen.SplitDigest)
	}
	if phys.SidecarRevision != ref.SidecarRevision || phys.SplitManifestDigest != ref.SplitManifestDigest ||
		phys.SplitManifestDigest != frozen.FileDigest || phys.Split != ref.Split || phys.SplitRole != ref.SplitRole ||
		!reflect.DeepEqual(phys.Episodes, ref.Episodes) {
		t.Fatalf("the physical and membership sections bound different things:\n%+v\n%+v", phys, ref)
	}
	if !reflect.DeepEqual(c.Physical.A.Summary, v1.Physical.A.Summary) || !reflect.DeepEqual(c.Physical.B.Summary, v1.Physical.B.Summary) ||
		!reflect.DeepEqual(c.Physical.A.Instants, v1.Physical.A.Instants) {
		t.Fatal("the frozen split's physical scores differ from its version 1 form's")
	}
	// The version 1 identity is what it was; the frozen one names its pins.
	if v1.Physical.Reference.SplitDigest != "" || v1.Physical.Reference.SplitRevision != 0 || phys.Digest == v1.Physical.Reference.Digest {
		t.Fatalf("version 1 identity %+v, frozen digest %s", v1.Physical.Reference, phys.Digest)
	}
}

// A membership edit after freezing moves the sidecar head, and here makes a
// physical link stale against it. The frozen split still binds the revision
// it pinned, and the links are checked against that revision: the physical
// reference, and so its digest, are unchanged.
func TestPhysicalFrozenSplitIgnoresALaterMembershipEdit(t *testing.T) {
	f := physFixture(t)
	_, path := physFrozen(t, f)
	opts := physRefOpts(f)
	opts.SplitManifestPath = path
	before, err := LoadPhysicalReference(opts, DefaultPhysicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	membershipBefore, err := LoadReference(opts)
	if err != nil {
		t.Fatal(err)
	}

	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
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
	head, err := annotation.LoadPhysicalReferences(pack)
	if err != nil {
		t.Fatal(err)
	}
	if head.StaleAgainst() != 2 || len(head.Stale()) == 0 {
		t.Fatalf("the edit did not invalidate a link at the head (against %d: %v)", head.StaleAgainst(), head.Stale())
	}
	// Unpinned, the version 1 path now checks the links against the head and
	// refuses: the edit is real.
	m := f.Manifest()
	m.SidecarRevision = 0
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions()); err == nil {
		t.Fatal("an unpinned manifest scored against a head whose links no longer hold")
	}

	after, err := LoadPhysicalReference(opts, DefaultPhysicalOptions())
	if err != nil {
		t.Fatalf("the frozen split no longer binds after a membership edit: %v", err)
	}
	if !reflect.DeepEqual(after.Identity, before.Identity) {
		t.Fatalf("identity moved from %+v to %+v", before.Identity, after.Identity)
	}
	if after.Identity.SidecarRevision != 1 {
		t.Fatalf("bound annotation revision %d, want the pinned 1", after.Identity.SidecarRevision)
	}
	membershipAfter, err := LoadReference(opts)
	if err != nil {
		t.Fatal(err)
	}
	if membershipAfter.Identity.Digest != membershipBefore.Identity.Digest {
		t.Fatal("the membership reference moved with the head")
	}
}

// A frozen split's pins hold for physical scoring as for membership scoring,
// and freezing does not lift the held-out refusal.
func TestPhysicalFrozenSplitRefusals(t *testing.T) {
	f := physFixture(t)
	_, path := physFrozen(t, f)
	cfg := physConfig(f)
	cfg.Reference.SplitManifestPath = path

	manifest := filepath.Join(f.PackDir, "manifest.json")
	original, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, append(append([]byte(nil), original...), '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPhysicalReference(cfg.Reference, DefaultPhysicalOptions()); err == nil ||
		!strings.Contains(err.Error(), "manifest.json changed after freezing") {
		t.Fatalf("error %v, want the changed pack refused", err)
	}
	if err := os.WriteFile(manifest, original, 0o644); err != nil {
		t.Fatal(err)
	}

	draft := f.FrozenDraft()
	draft.Packs[0].Splits[0].Role = annotation.SplitRoleHeldOut
	frozen, err := annotation.FreezeSplit(annotation.FreezeOptions{
		Draft: &draft, Author: "operator", Now: time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC), BuildVersion: "test", BuildGitSHA: "abc",
		GuardSeconds: annotation.DefaultSplitGuardSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	heldOut := filepath.Join(t.TempDir(), "held-out.json")
	if err := annotation.WriteFrozenSplit(heldOut, frozen); err != nil {
		t.Fatal(err)
	}
	for _, allowTuning := range []bool{true, false} {
		cfg.Reference.SplitManifestPath, cfg.Reference.AllowTuningSplit = heldOut, allowTuning
		if _, err := Run(cfg); !errors.Is(err, ErrPhysicalHeldOut) {
			t.Fatalf("a frozen held-out split was physically scored (allow tuning %v): %v", allowTuning, err)
		}
	}
}
