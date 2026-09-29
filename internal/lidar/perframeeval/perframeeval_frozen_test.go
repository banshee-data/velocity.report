package perframeeval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// freezeFixture freezes the fixture's frozen draft and returns its path.
func freezeFixture(t *testing.T, f *evalfixture.Fixture) (*annotation.FrozenSplit, string) {
	t.Helper()
	draft := f.FrozenDraft()
	frozen, err := annotation.FreezeSplit(annotation.FreezeOptions{
		Draft: &draft, Author: "operator", Now: time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC),
		BuildVersion: "test", BuildGitSHA: "abc", GuardSeconds: annotation.DefaultSplitGuardSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "frozen.json")
	if err := annotation.WriteFrozenSplit(path, frozen); err != nil {
		t.Fatal(err)
	}
	return frozen, path
}

// A frozen split scores what its version 1 form scores, is bound through its
// pins, and names itself in the reference identity. Without it, a held-out
// result carries a caveat saying the split was not frozen.
func TestFrozenSplitIsBoundAndRecorded(t *testing.T) {
	f := fixture(t)
	frozen, path := freezeFixture(t, f)

	v1, err := Run(baseConfig(f))
	if err != nil {
		t.Fatal(err)
	}
	cfg := baseConfig(f)
	cfg.Reference.SplitManifestPath = path
	c, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ref := c.Reference
	if ref.SplitDigest != frozen.SplitDigest || ref.SplitRevision != 1 || !ref.HeldOut || ref.SidecarRevision != 1 {
		t.Fatalf("reference identity %+v, want frozen split %s revision 1", ref, frozen.SplitDigest)
	}
	if c.Total.A != v1.Total.A || c.Total.B != v1.Total.B {
		t.Fatalf("frozen totals %+v differ from version 1's %+v", c.Total, v1.Total)
	}
	if ref.Digest == v1.Reference.Digest {
		t.Fatal("a frozen and an unfrozen reference share a digest")
	}
	if len(c.Caveats) != 1 || !strings.Contains(c.Caveats[0], "upper bound") {
		t.Fatalf("frozen caveats %q, want only the false-positive bound", c.Caveats)
	}
	if !strings.Contains(strings.Join(v1.Caveats, " "), "version 1, not frozen") {
		t.Fatalf("version 1 caveats %q do not say the split is not frozen", v1.Caveats)
	}
	md := RenderMarkdown(*c)
	if !strings.Contains(md, "| Frozen split        | `"+frozen.SplitDigest+"`, revision 1 |") {
		t.Fatalf("markdown lacks the frozen split:\n%s", md)
	}

	// A tuning partition of a frozen split is still not held out, and says so
	// rather than asking for a freeze it already has.
	cfg.Reference.Split, cfg.Reference.AllowTuningSplit = evalfixture.SplitTuning, true
	tuning, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(tuning.Caveats, " "); !strings.Contains(joined, "not a held-out result") || strings.Contains(joined, "not frozen") {
		t.Fatalf("tuning caveats %q", tuning.Caveats)
	}
}

// The evaluator refuses a frozen split once a pin no longer holds.
func TestFrozenSplitRefusesAChangedPack(t *testing.T) {
	f := fixture(t)
	_, path := freezeFixture(t, f)
	manifest := filepath.Join(f.PackDir, "manifest.json")
	b, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := baseConfig(f).Reference
	opts.SplitManifestPath = path
	if _, err := LoadReference(opts); err == nil || !strings.Contains(err.Error(), "manifest.json changed after freezing") {
		t.Fatalf("error %v, want the changed manifest refused", err)
	}
	opts.SplitManifestPath = filepath.Join(t.TempDir(), "missing.json")
	if _, err := LoadReference(opts); err == nil || !strings.Contains(err.Error(), "open split manifest") {
		t.Fatalf("error %v, want the missing split refused", err)
	}
}

// A version 1 manifest that pins no revision scores the current one, and
// records which that was.
func TestUnpinnedManifestScoresTheCurrentRevision(t *testing.T) {
	f := fixture(t)
	m := f.Manifest()
	m.SidecarRevision = 0
	path := filepath.Join(t.TempDir(), "unpinned.json")
	if err := evalfixture.WriteSplitManifest(path, m); err != nil {
		t.Fatal(err)
	}
	opts := baseConfig(f).Reference
	opts.SplitManifestPath = path
	ref, err := LoadReference(opts)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Identity.SidecarRevision != 1 || ref.Identity.SplitDigest != "" {
		t.Fatalf("identity %+v", ref.Identity)
	}
}
