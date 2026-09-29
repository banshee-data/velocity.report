//go:build pcap
// +build pcap

package lidar

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
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
	m := annotation.Manifest{Coverage: annotation.CoverageForegroundOnly, Source: annotation.SourceProvenance{PCAPBasename: "cap.pcap"}}
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
