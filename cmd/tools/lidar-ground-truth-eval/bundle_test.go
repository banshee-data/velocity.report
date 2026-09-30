package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
	_ "modernc.org/sqlite"
)

// frozenPhysical writes the physical fixture and freezes its split, which
// pins the fixture's physical revision; the returned fixture's
// SplitManifestPath is the frozen split. The version 1 manifest it replaces
// is at versionOneManifest.
func frozenPhysical(t *testing.T) (*evalfixture.PhysicalFixture, *annotation.FrozenSplit) {
	t.Helper()
	dir := t.TempDir()
	f, err := evalfixture.WritePhysical(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "frozen.json")
	frozen, err := f.WriteFrozen(path)
	if err != nil {
		t.Fatal(err)
	}
	f.SplitManifestPath = path
	return f, frozen
}

// versionOneManifest is the fixture's hand-written split manifest, which
// pins no physical revision, so a run through it scores the head.
func versionOneManifest(f *evalfixture.PhysicalFixture) string {
	return filepath.Join(filepath.Dir(f.PackDir), "split.json")
}

// The command scores physical references against a frozen membership split
// at the physical revision it pins, names the split in both sections, and
// accepts an explicit revision only when it is the pinned one.
func TestPerFramePhysicalWithFrozenSplit(t *testing.T) {
	f, frozen := frozenPhysical(t)
	code, c, stderr := runPerFrameCapture(t, physicalArgs(f, "-physical-reference", "-physical-reference-revision", "1"))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if c.Physical == nil || c.Physical.Reference.SplitDigest != frozen.SplitDigest || c.Reference.SplitDigest != frozen.SplitDigest ||
		c.Physical.Reference.PhysicalRevision != 1 || c.Physical.Reference.SidecarRevision != 1 || !c.Physical.Reference.PhysicalPinned {
		t.Fatalf("physical section %+v, reference %+v", c.Physical, c.Reference)
	}
	if !strings.Contains(stderr, "frozen split "+frozen.SplitDigest) ||
		!strings.Contains(stderr, "physical references: revision 1, content "+frozen.Packs[0].Physical.ContentSHA256) ||
		!strings.Contains(stderr, "pinned by the frozen split") {
		t.Fatalf("stderr: %s", stderr)
	}
	code, _, stderr = runPerFrameCapture(t, physicalArgs(f, "-physical-reference", "-physical-reference-revision", "2"))
	if code != 1 || !strings.Contains(stderr, "revision 2 was asked for, but frozen split "+frozen.SplitDigest+" pins revision 1") {
		t.Fatalf("a revision conflicting with the pin: exit %d: %s", code, stderr)
	}
	// Through the version 1 manifest the same revision is scored, unpinned.
	f.SplitManifestPath = versionOneManifest(f)
	code, plain, stderr := runPerFrameCapture(t, physicalArgs(f, "-physical-reference"))
	if code != 0 || plain.Physical.Reference.PhysicalPinned || strings.Contains(stderr, "pinned by the frozen split") {
		t.Fatalf("version 1 manifest: exit %d, %+v: %s", code, plain.Physical.Reference, stderr)
	}
}

// writeFixtureBundle runs perframe on a frozen physical fixture with a
// bundle, JSON and Markdown, and returns the fixture and bundle directory.
func writeFixtureBundle(t *testing.T, f *evalfixture.PhysicalFixture, extra ...string) string {
	t.Helper()
	out := t.TempDir()
	bundle := filepath.Join(out, "bundle")
	args := physicalArgs(f, append([]string{"-physical-reference", "-physical-bundle-dir", bundle,
		"-json", filepath.Join(out, "c.json"), "-markdown", filepath.Join(out, "c.md")}, extra...)...)
	var stdout, stderr bytes.Buffer
	if code := runPerFrame(args, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "verification bundle "+bundle) {
		t.Fatalf("stderr does not name the bundle: %s", stderr.String())
	}
	return bundle
}

func verify(dir string) (int, string) {
	var stderr bytes.Buffer
	code := runVerifyBundle([]string{"-bundle", dir}, &stderr)
	return code, stderr.String()
}

func readManifest(t *testing.T, dir string) bundleManifest {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, bundleManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var m bundleManifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// The bundle holds every pinned input and output, lists each with its
// digest, and verifies.
func TestBundleRoundTrip(t *testing.T) {
	f, frozen := frozenPhysical(t)
	bundle := writeFixtureBundle(t, f)
	m := readManifest(t, bundle)

	head, err := os.ReadFile(filepath.Join(f.PackDir, physicalHeadFile))
	if err != nil {
		t.Fatal(err)
	}
	copyOf := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(bundle, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if !m.Physical.WasHead || m.Physical.Revision != 1 || !bytes.Equal(copyOf(m.Physical.File), head) ||
		m.Physical.SHA256 != sha256Hex(head) || !strings.HasPrefix(m.Physical.ContentDigest, "sha256:") {
		t.Fatalf("physical pin %+v", m.Physical)
	}
	split, _ := os.ReadFile(f.SplitManifestPath)
	if m.Split.SplitDigest != frozen.SplitDigest || m.Split.SHA256 != sha256Hex(split) || !bytes.Equal(copyOf(bundleSplitFile), split) {
		t.Fatalf("split pin %+v", m.Split)
	}
	if m.Pack.PackDigest != f.PackDigest || m.Pack.DatasetID != f.DatasetID || m.Pack.Source.VRLOGHeaderSHA != "sha256:fixture-header" ||
		m.Pack.Source.VRLOGFramesSHA != "sha256:fixture-frames" {
		t.Fatalf("pack pin %+v", m.Pack)
	}
	if len(m.Arms) != 2 || m.Arms[0].ParamHash != evalfixture.ParamsExact || m.Outputs.Markdown != bundleMarkdownFile ||
		m.Build.GoVersion == "" || len(m.Args) == 0 || !filepath.IsAbs(m.WorkingDir) {
		t.Fatalf("manifest %+v", m)
	}
	want := []string{bundleJSONFile, bundleMarkdownFile, bundlePackFile, m.Physical.File, bundleSplitFile}
	if len(m.Files) != len(want) {
		t.Fatalf("files %+v", m.Files)
	}
	for _, file := range m.Files {
		if sha256Hex(copyOf(file.Name)) != file.SHA256 {
			t.Fatalf("file %s listed with the wrong digest", file.Name)
		}
	}
	if code, stderr := verify(bundle); code != 0 || !strings.Contains(stderr, "verified") {
		t.Fatalf("verify: exit %d: %s", code, stderr)
	}
}

// After the physical head advances, the bundle still verifies: it re-scores
// the pinned revision, which the store has now archived, not the head.
func TestBundleVerifiesThePinnedRevisionAfterTheHeadAdvances(t *testing.T) {
	f, _ := frozenPhysical(t)
	bundle := writeFixtureBundle(t, f)

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
	refs.Change = annotation.Provenance{Author: "operator", Operation: "save"}
	if err := annotation.SavePhysicalReferences(pack, refs); err != nil {
		t.Fatal(err)
	}
	if refs.Revision != 2 {
		t.Fatalf("head is revision %d", refs.Revision)
	}
	if code, stderr := verify(bundle); code != 0 {
		t.Fatalf("verify after the head advanced: exit %d: %s", code, stderr)
	}
	// The frozen split keeps scoring the revision it pinned.
	code, c, stderr := runPerFrameCapture(t, physicalArgs(f, "-physical-reference"))
	if code != 0 || c.Physical.Reference.PhysicalRevision != 1 || !c.Physical.Reference.PhysicalPinned ||
		c.Physical.Reference.PhysicalContentDigest != readManifest(t, bundle).Physical.ContentDigest {
		t.Fatalf("pinned run after the head advanced: exit %d, %+v: %s", code, c.Physical.Reference, stderr)
	}
	// The head itself, scored through the version 1 manifest, scores
	// differently, so the pin is doing the work.
	f.SplitManifestPath = versionOneManifest(f)
	code, c, stderr = runPerFrameCapture(t, physicalArgs(f, "-physical-reference"))
	if code != 0 || c.Physical.Reference.PhysicalRevision != 2 || c.Physical.Reference.PhysicalContentDigest == readManifest(t, bundle).Physical.ContentDigest {
		t.Fatalf("head run: exit %d: %s", code, stderr)
	}
}

// A bundle written under a pinned split records the pinned revision and its
// digests, its comparison says the revision was pinned, and it verifies. A
// new split revision pinning a new physical revision does not disturb it:
// the bundle names the split file it was scored under.
func TestBundleWithAPinnedSplit(t *testing.T) {
	f, frozen := frozenPhysical(t)
	bundle := writeFixtureBundle(t, f)
	m := readManifest(t, bundle)
	pin := frozen.Packs[0].Physical
	if m.Physical.Revision != pin.Revision || m.Physical.SHA256 != pin.SHA256 || m.Physical.ContentDigest != pin.ContentSHA256 ||
		m.Split.SplitDigest != frozen.SplitDigest || !m.Physical.WasHead {
		t.Fatalf("bundle manifest %+v does not record the pin %+v", m.Physical, pin)
	}
	comparison, err := os.ReadFile(filepath.Join(bundle, m.Outputs.JSON))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(comparison), `"physical_pinned": true`) {
		t.Fatal("the bundled comparison does not say the physical revision was pinned")
	}
	if code, stderr := verify(bundle); code != 0 {
		t.Fatalf("verify: exit %d: %s", code, stderr)
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
	opts := f.FreezeOptions()
	opts.Supersedes = frozen
	next, err := annotation.FreezeSplit(opts)
	if err != nil {
		t.Fatal(err)
	}
	if next.Packs[0].Physical.Revision != 2 {
		t.Fatalf("the successor pins revision %d", next.Packs[0].Physical.Revision)
	}
	if err := annotation.WriteFrozenSplit(filepath.Join(filepath.Dir(f.PackDir), "frozen-2.json"), next); err != nil {
		t.Fatal(err)
	}
	if code, stderr := verify(bundle); code != 0 {
		t.Fatalf("verify after a successor split: exit %d: %s", code, stderr)
	}
}

// Every changed pin, and every altered bundle file, is refused by name.
func TestBundleRefusesChangedPins(t *testing.T) {
	for name, c := range map[string]struct {
		change func(t *testing.T, f *evalfixture.PhysicalFixture, bundle string)
		want   string
	}{
		"altered output": {func(t *testing.T, _ *evalfixture.PhysicalFixture, bundle string) {
			appendTo(t, filepath.Join(bundle, bundleJSONFile), " ")
		}, "bundle file comparison.json was altered"},
		"altered reference copy": {func(t *testing.T, _ *evalfixture.PhysicalFixture, bundle string) {
			appendTo(t, filepath.Join(bundle, "physical-reference-0000000001.json"), " ")
		}, "bundle file physical-reference-0000000001.json was altered"},
		"missing file": {func(t *testing.T, _ *evalfixture.PhysicalFixture, bundle string) {
			removeFile(t, filepath.Join(bundle, bundleSplitFile))
		}, "bundle file split.json is missing"},
		"unlisted file": {func(t *testing.T, _ *evalfixture.PhysicalFixture, bundle string) {
			writeFile(t, filepath.Join(bundle, "extra.json"), "{}")
		}, "does not list"},
		"missing manifest": {func(t *testing.T, _ *evalfixture.PhysicalFixture, bundle string) {
			removeFile(t, filepath.Join(bundle, bundleManifestFile))
		}, "read bundle manifest"},
		"changed split": {func(t *testing.T, f *evalfixture.PhysicalFixture, _ string) {
			appendTo(t, f.SplitManifestPath, "\n")
		}, "split " + "%SPLIT%" + " changed"},
		"changed pack manifest": {func(t *testing.T, f *evalfixture.PhysicalFixture, _ string) {
			appendTo(t, filepath.Join(f.PackDir, packManifestFile), "\n")
		}, "pack manifest"},
		"changed reference bytes": {func(t *testing.T, f *evalfixture.PhysicalFixture, _ string) {
			// The same content, re-encoded: the exact-byte pin catches it.
			path := filepath.Join(f.PackDir, physicalHeadFile)
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := json.Compact(&buf, b); err != nil {
				t.Fatal(err)
			}
			writeFile(t, path, buf.String())
		}, "physical reference revision 1 bytes changed"},
		"missing reference": {func(t *testing.T, f *evalfixture.PhysicalFixture, _ string) {
			removeFile(t, filepath.Join(f.PackDir, physicalHeadFile))
		}, "physical reference revision 1 cannot be read"},
		"changed options": {func(t *testing.T, _ *evalfixture.PhysicalFixture, bundle string) {
			editManifest(t, bundle, func(m *bundleManifest) { m.Args = append(m.Args, "-physical-gate-metres", "2.5") })
		}, "scoring options"},
		"unparseable options": {func(t *testing.T, _ *evalfixture.PhysicalFixture, bundle string) {
			editManifest(t, bundle, func(m *bundleManifest) { m.Args = append(m.Args, "-no-such-flag") })
		}, "no longer parse"},
		"changed estimates": {func(t *testing.T, f *evalfixture.PhysicalFixture, _ string) {
			database, err := sql.Open("sqlite", f.DBPath)
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if _, err := database.Exec(`DELETE FROM lidar_track_solid_bodies WHERE param_hash = ? AND estimate_id LIKE ?`,
				evalfixture.ParamsFaceBias, "%/9"); err != nil {
				t.Fatal(err)
			}
		}, "re-run arm identities"},
		"wrong schema": {func(t *testing.T, _ *evalfixture.PhysicalFixture, bundle string) {
			editManifest(t, bundle, func(m *bundleManifest) { m.SchemaVersion = 99 })
		}, "want velocity.report/physical-verification-bundle version 1"},
		"pin not naming its copy": {func(t *testing.T, _ *evalfixture.PhysicalFixture, bundle string) {
			editManifest(t, bundle, func(m *bundleManifest) { m.Physical.SHA256 = "sha256:00" })
		}, "physical reference pin"},
		"path file name": {func(t *testing.T, _ *evalfixture.PhysicalFixture, bundle string) {
			editManifest(t, bundle, func(m *bundleManifest) { m.Files[0].Name = "../escape.json" })
		}, "not a plain, unique bundle file name"},
	} {
		t.Run(name, func(t *testing.T) {
			f, _ := frozenPhysical(t)
			bundle := writeFixtureBundle(t, f)
			c.change(t, f, bundle)
			code, stderr := verify(bundle)
			want := strings.ReplaceAll(c.want, "%SPLIT%", f.SplitManifestPath)
			if code != 1 || !strings.Contains(stderr, want) {
				t.Fatalf("exit %d, want 1 naming %q: %s", code, want, stderr)
			}
		})
	}
}

// With a version 1 manifest that pins no annotation revision, a later
// membership save moves what the split binds to; verify names it.
func TestBundleRefusesAMovedAnnotationRevision(t *testing.T) {
	f, err := evalfixture.WritePhysical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := f.Manifest()
	m.SidecarRevision = 0
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	bundle := writeFixtureBundle(t, f)
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := annotation.LoadSidecar(pack)
	if err != nil {
		t.Fatal(err)
	}
	s.Change = annotation.Provenance{Author: "operator", Operation: "save"}
	if err := annotation.SaveSidecar(pack, s); err != nil {
		t.Fatal(err)
	}
	if code, stderr := verify(bundle); code != 1 || !strings.Contains(stderr, "annotation (sidecar) revision is 2, pinned 1") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestBundleUsage(t *testing.T) {
	f, _ := frozenPhysical(t)
	var stdout, stderr bytes.Buffer
	if code := runPerFrame(physicalArgs(f, "-physical-bundle-dir", t.TempDir()+"/b"), &stdout, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), "-physical-bundle-dir needs -physical-reference") {
		t.Fatalf("bundle without physical scoring: exit %d: %s", code, stderr.String())
	}
	full := t.TempDir()
	writeFile(t, filepath.Join(full, "occupied"), "")
	stderr.Reset()
	if code := runPerFrame(physicalArgs(f, "-physical-reference", "-physical-bundle-dir", full), &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "is not empty") {
		t.Fatalf("occupied bundle directory: exit %d: %s", code, stderr.String())
	}
	stderr.Reset()
	if code := runPerFrame(physicalArgs(f, "-physical-reference", "-physical-bundle-dir", filepath.Join(full, "occupied", "b")), &stdout, &stderr); code != 1 {
		t.Fatalf("bundle under a file: exit %d: %s", code, stderr.String())
	}
	for args, want := range map[string]int{"": 2, "-h": 0, "-bundle x y": 2, "-nope": 2} {
		stderr.Reset()
		if code := runVerifyBundle(strings.Fields(args), &stderr); code != want {
			t.Errorf("verify-bundle %q: exit %d, want %d: %s", args, code, want, stderr.String())
		}
	}
	if code := run([]string{"verify-bundle", "-bundle", filepath.Join(t.TempDir(), "none")}); code != 1 {
		t.Fatalf("verify of a missing bundle: exit %d", code)
	}
}

func appendTo(t *testing.T, path, s string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(b)+s)
}

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func editManifest(t *testing.T, bundle string, edit func(*bundleManifest)) {
	t.Helper()
	m := readManifest(t, bundle)
	edit(&m)
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(bundle, bundleManifestFile), string(b))
}
