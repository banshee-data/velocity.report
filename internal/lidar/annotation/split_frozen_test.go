package annotation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

// splitPack writes a pack of n samples a second apart from startNs, cut from
// the named capture, with two objects' returns in every sample: points 0-3
// and 4-7. Distinct starts give distinct pack digests.
func splitPack(t *testing.T, name, pcap string, startNs int64, n int) *Pack {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	var samples []Sample
	var blocks [][]byte
	for i := 0; i < n; i++ {
		block, err := encodePoints(Points{
			X: []float32{1, 2, 3, 4, 10, 11, 12, 13}, Y: make([]float32, 8), Z: make([]float32, 8),
		})
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, Sample{SourceOrdinal: i, TimestampNs: startNs + int64(i)*1e9, SensorID: "synthetic", PointCount: 8})
		blocks = append(blocks, block)
	}
	m := Manifest{Coverage: CoverageForegroundOnly, Source: SourceProvenance{
		SensorID: "synthetic", PCAPBasename: pcap, VRLOGFramesSHA: "sha256:frames-" + name, ParamsHash: "sha256:params",
	}}
	if err := WritePack(dir, m, samples, blocks); err != nil {
		t.Fatal(err)
	}
	p, err := OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// reviewSplitPack saves a revision in which obj_a and obj_b are reviewed cars
// with reviewed, complete masks in every sample, after edit has had its say.
func reviewSplitPack(t *testing.T, p *Pack, edit func(*Sidecar)) *Sidecar {
	t.Helper()
	s, err := LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Objects = []Object{reviewedObject("obj_a", "car"), reviewedObject("obj_b", "car")}
	s.Masks = nil
	for i := range p.Samples {
		s.Masks = append(s.Masks, mask("obj_a", i, 0, 1, 2, 3), mask("obj_b", i, 4, 5, 6, 7))
	}
	if edit != nil {
		edit(s)
	}
	s.Change = Provenance{Author: "reviewer", Operation: "review"}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	return s
}

// splitDraftPack puts obj_a in partition tune and obj_b in hold, each with
// one episode over every sample. Episode IDs carry the prefix, since they
// are split-wide.
func splitDraftPack(p *Pack, prefix string) DraftPack {
	last := len(p.Samples) - 1
	return DraftPack{
		Dir: p.Dir,
		Splits: []Split{
			{Name: "tune", Role: SplitRoleTuning, ObjectIDs: []string{"obj_a"}},
			{Name: "hold", Role: SplitRoleHeldOut, ObjectIDs: []string{"obj_b"}},
		},
		Episodes: []Episode{
			{EpisodeID: prefix + "-tune", Split: "tune", ObjectIDs: []string{"obj_a"}, FrameIntervals: []FrameInterval{{0, last}}},
			{EpisodeID: prefix + "-hold", Split: "hold", ObjectIDs: []string{"obj_b"}, FrameIntervals: []FrameInterval{{0, last}}},
		},
	}
}

// onePartition puts both objects in one partition.
func onePartition(p *Pack, prefix, name string, role SplitRole) DraftPack {
	return DraftPack{
		Dir:    p.Dir,
		Splits: []Split{{Name: name, Role: role, ObjectIDs: []string{"obj_a", "obj_b"}}},
		Episodes: []Episode{{EpisodeID: prefix, Split: name, ObjectIDs: []string{"obj_a", "obj_b"},
			FrameIntervals: []FrameInterval{{0, len(p.Samples) - 1}}}},
	}
}

// splitCase declares a corpus case replaying one capture, <id>.pcap, whose
// content digest is a digest of the ID.
func splitCase(id string, role SplitRole) SplitCase {
	return SplitCase{CaseID: id, Role: role, Captures: []CaseCapture{{Basename: id + ".pcap", SHA256: sha256Hex([]byte(id))}}}
}

var freezeTime = time.Date(2026, 9, 29, 12, 0, 0, 0, time.FixedZone("BST", 3600))

func freezeOptions(packs ...DraftPack) FreezeOptions {
	return FreezeOptions{
		Draft:  &SplitDraft{Schema: SplitDraftSchema, SchemaVersion: SplitDraftSchemaVersion, Packs: packs},
		Author: "operator", Now: freezeTime, BuildVersion: "0.5.2-test", BuildGitSHA: "abc123",
		GuardSeconds: DefaultSplitGuardSeconds,
	}
}

func mustFreeze(t *testing.T, opts FreezeOptions) *FrozenSplit {
	t.Helper()
	f, err := FreezeSplit(opts)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// writeAndLoad round-trips a frozen split through its file, as an evaluator
// reads it.
func writeAndLoad(t *testing.T, f *FrozenSplit) (*FrozenSplit, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "frozen.json")
	if err := WriteFrozenSplit(path, f); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFrozenSplit(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded, path
}

func wantError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one mentioning %q", err, want)
	}
}

// A freeze pins everything an evaluator re-checks, records who froze it, and
// reads back, digest intact, as a version 2 split that binds to its pack.
func TestFreezeSplitPinsReviewAndDigests(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
	s := reviewSplitPack(t, p, nil)
	opts := freezeOptions(splitDraftPack(p, "p1"))
	opts.ForParamsHash, opts.ForConfigHash = "sha256:chosen", "sha256:config"
	f := mustFreeze(t, opts)

	if f.Schema != SplitSchema || f.SchemaVersion != FrozenSplitSchemaVersion || f.Revision != 1 || f.Supersedes != "" {
		t.Fatalf("identity %+v", f)
	}
	want := FreezeRecord{Author: "operator", FrozenUTC: "2026-09-29T11:00:00Z", BuildVersion: "0.5.2-test", BuildGitSHA: "abc123",
		ForConfigHash: "sha256:config", ForParamsHash: "sha256:chosen"}
	if f.Frozen != want {
		t.Fatalf("freeze record %+v, want %+v", f.Frozen, want)
	}
	if len(f.Partitions) != 2 || f.Partitions[0] != (SplitPartition{"hold", SplitRoleHeldOut}) || f.Partitions[1] != (SplitPartition{"tune", SplitRoleTuning}) {
		t.Fatalf("partitions %+v, want hold and tune, by name", f.Partitions)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(p.Dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	fp := f.Packs[0]
	if fp.PackDigest != p.Manifest.PackDigest || fp.DatasetID != p.Manifest.DatasetID || fp.ManifestSHA256 != sha256Hex(manifestBytes) ||
		fp.SidecarRevision != s.Revision || fp.SidecarSHA256 != s.baseDigest || fp.Selection != nil {
		t.Fatalf("pack pins %+v", fp)
	}
	if src := fp.Source; src.PCAPBasename != "cap.pcap" || src.ParamsHash != "sha256:params" || src.FirstSampleNs != 1e9 || src.LastSampleNs != 3e9 {
		t.Fatalf("source %+v", src)
	}
	wantObjects := []FrozenObject{
		{ObjectID: "obj_a", Partition: "tune", Class: "car", ReviewedMasks: 3, Geometry: GeometryReview{Status: GeometryNone}},
		{ObjectID: "obj_b", Partition: "hold", Class: "car", ReviewedMasks: 3, Geometry: GeometryReview{Status: GeometryNone}},
	}
	if fmt.Sprint(fp.Objects) != fmt.Sprint(wantObjects) {
		t.Fatalf("objects %+v, want %+v", fp.Objects, wantObjects)
	}
	if !strings.HasPrefix(f.SplitDigest, "sha256:") || f.SplitDigest != f.contentDigest() {
		t.Fatalf("split digest %q is not the content digest %q", f.SplitDigest, f.contentDigest())
	}

	loaded, path := writeAndLoad(t, f)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SplitDigest != f.SplitDigest || loaded.FileDigest != sha256Hex(b) {
		t.Fatalf("loaded digests %s / %s", loaded.SplitDigest, loaded.FileDigest)
	}
	view, bound, err := loaded.Bind(p)
	if err != nil {
		t.Fatal(err)
	}
	if bound.Revision != s.Revision || view.SidecarRevision != s.Revision || view.PackDigest != p.Manifest.PackDigest ||
		view.Digest != loaded.FileDigest || len(view.Splits) != 2 || len(view.Episodes) != 2 {
		t.Fatalf("bound view %+v at revision %d", view, bound.Revision)
	}
	got, err := view.SelectEpisodes("hold", nil, true)
	if err != nil || len(got) != 1 || got[0].EpisodeID != "p1-hold" {
		t.Fatalf("held-out selection %+v, %v", got, err)
	}
	if _, err := view.SelectEpisodes("tune", nil, true); !errors.Is(err, ErrNotHeldOut) {
		t.Fatalf("a tuning partition scored as held out: %v", err)
	}
}

// Every way membership review can be unfinished is refused, and all of them
// are listed at once, so one review pass can clear them.
func TestFreezeSplitRefusesIncompleteMembershipReview(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
	reviewSplitPack(t, p, func(s *Sidecar) {
		s.Objects = append(s.Objects,
			Object{ObjectID: "obj_c", Class: "car", Status: StatusProposed},
			reviewedObject("obj_d", "car"), reviewedObject("obj_e", "car"), reviewedObject("obj_f", "car"))
		s.Masks[0].Status = StatusProposed       // obj_a, sample 0
		s.Masks[1].Completeness = MaskUnreviewed // obj_b, sample 0
		s.Masks = append(s.Masks, FrameMask{ObjectID: "obj_d", SampleID: 0, PointIndices: []int{},
			Completeness: MaskComplete, Visibility: VisiblePresent, Status: StatusRejected})
		// obj_e is reviewed only outside its episode's frames.
		s.Masks = append(s.Masks, mask("obj_e", 2))
		// obj_f's only reviewed mask is unstated, and it is not in an episode.
		s.Masks = append(s.Masks, FrameMask{ObjectID: "obj_f", SampleID: 1, PointIndices: []int{},
			Completeness: MaskUnreviewed, Visibility: VisiblePresent, Status: StatusReviewed})
	})
	dp := splitDraftPack(p, "p1")
	dp.Splits[0].ObjectIDs = []string{"obj_a", "obj_c", "obj_d", "obj_e", "obj_f"}
	dp.Episodes = append(dp.Episodes, Episode{EpisodeID: "p1-e", Split: "tune", ObjectIDs: []string{"obj_e"},
		FrameIntervals: []FrameInterval{{0, 1}}})
	_, err := FreezeSplit(freezeOptions(dp))
	for _, want := range []string{
		"membership review is not complete",
		"object obj_a: 1 mask(s) still proposed",
		"object obj_b: 1 reviewed mask(s) without a stated completeness",
		"object obj_c: object is proposed, no reviewed mask",
		"object obj_d: no reviewed mask",
		"episode p1-e: object obj_e has no reviewed mask in the episode's frames",
		"object obj_f: 1 reviewed mask(s) without a stated completeness",
	} {
		wantError(t, err, want)
	}
}

// Geometry review is counted over the reviewed masks' poses and recorded
// beside membership review; it never blocks a freeze.
func TestFreezeSplitRecordsGeometryReviewSeparately(t *testing.T) {
	pose := func(status ReviewStatus) *Pose { return &Pose{Length: 4, Width: 2, Status: status} }
	cases := []struct {
		name  string
		poses []*Pose
		want  GeometryReview
	}{
		{"none", []*Pose{nil, nil, nil}, GeometryReview{Status: GeometryNone}},
		{"proposed", []*Pose{pose(StatusProposed), nil, pose(StatusRejected)}, GeometryReview{Status: GeometryProposed, ProposedPoses: 1}},
		{"partial", []*Pose{pose(StatusReviewed), pose(StatusProposed), nil}, GeometryReview{Status: GeometryPartial, ReviewedPoses: 1, ProposedPoses: 1}},
		{"complete", []*Pose{pose(StatusReviewed), pose(StatusReviewed), pose(StatusReviewed)}, GeometryReview{Status: GeometryComplete, ReviewedPoses: 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
			reviewSplitPack(t, p, func(s *Sidecar) {
				for i := range s.Masks {
					if s.Masks[i].ObjectID == "obj_a" {
						s.Masks[i].Pose = tc.poses[s.Masks[i].SampleID]
					}
				}
			})
			f := mustFreeze(t, freezeOptions(splitDraftPack(p, "p1")))
			if got := f.Packs[0].Objects[0].Geometry; got != tc.want {
				t.Fatalf("geometry review %+v, want %+v", got, tc.want)
			}
			if _, err := ParseFrozenSplit(marshalFrozen(t, f)); err != nil {
				t.Fatalf("a frozen geometry status does not read back: %v", err)
			}
		})
	}
}

func marshalFrozen(t *testing.T, f *FrozenSplit) []byte {
	t.Helper()
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Object IDs are pack-local, so two packs of one capture close enough in
// time to share a vehicle may feed one partition only.
func TestFreezeSplitKeepsOneSourceInOnePartition(t *testing.T) {
	early := splitPack(t, "early", "cap.pcap", 1e9, 3) // 1-3 s
	near := splitPack(t, "near", "cap.pcap", 20e9, 3)  // 20-22 s: 17 s after early
	far := splitPack(t, "far", "cap.pcap", 100e9, 3)   // 100-102 s
	other := splitPack(t, "other", "other.pcap", 2e9, 3)
	for _, p := range []*Pack{early, near, far, other} {
		reviewSplitPack(t, p, nil)
	}
	tuneEarly := onePartition(early, "early", "tune", SplitRoleTuning)
	holdNear := onePartition(near, "near", "hold", SplitRoleHeldOut)

	_, err := FreezeSplit(freezeOptions(tuneEarly, holdNear))
	wantError(t, err, "are cut from pcap:cap.pcap within 30s of each other and feed partitions hold, tune")

	// Without the guard the spans do not overlap.
	opts := freezeOptions(tuneEarly, holdNear)
	opts.GuardSeconds = 0
	mustFreeze(t, opts)
	// Far enough apart, another capture, or one partition between them.
	mustFreeze(t, freezeOptions(tuneEarly, onePartition(far, "far", "hold", SplitRoleHeldOut)))
	mustFreeze(t, freezeOptions(tuneEarly, onePartition(other, "other", "hold", SplitRoleHeldOut)))
	mustFreeze(t, freezeOptions(tuneEarly, onePartition(near, "near", "tune", SplitRoleTuning)))

	// With no capture named, the VRLOG is the source.
	f := mustFreeze(t, freezeOptions(tuneEarly, onePartition(far, "far", "hold", SplitRoleHeldOut)))
	f.Packs[0].Source.PCAPBasename, f.Packs[1].Source.PCAPBasename = "", ""
	f.Packs[1].Source.VRLOGFramesSHA = f.Packs[0].Source.VRLOGFramesSHA
	f.Packs[1].Source.FirstSampleNs, f.Packs[1].Source.LastSampleNs = 2e9, 4e9
	f.Tuned = mergeLedgers(f.ownTuning())
	wantError(t, f.validate(), "are cut from vrlog:sha256:frames-")
	// And with neither, nothing relates them.
	f.Packs[0].Source.VRLOGFramesSHA, f.Packs[1].Source.VRLOGFramesSHA = "", ""
	f.Tuned = mergeLedgers(f.ownTuning())
	if err := f.validate(); err != nil {
		t.Fatalf("packs with no declared source were related: %v", err)
	}
}

func writeSegmentRecord(t *testing.T, p *Pack, role string) {
	t.Helper()
	params := segments.DefaultParams()
	r := segments.Record{Schema: "velocity.report/annotation-segment", SchemaVersion: 1, PackDigest: p.Manifest.PackDigest,
		Role: role, Finder: "manual", FinderVersion: 1, Parameters: params,
		Segment: segments.Window{Finder: "manual", Version: 1, Source: "cap.pcap", StartNs: 1, EndNs: 2}}
	if role == string(SplitRoleHeldOut) {
		start := int64(1e9)
		r.Finder = "random"
		r.Segment = segments.Window{ID: segments.Identity("random", "cap.pcap", role, params, start), Finder: "random",
			Version: 1, Source: "cap.pcap", Role: role, StartNs: start, EndNs: start + int64(params.WindowSeconds*1e9)}
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Dir, segmentRecordFile), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A window chosen for tuning feeds no held-out partition; a held-out window's
// selection record is pinned; a record that does not bind to the pack is
// refused rather than ignored.
func TestFreezeSplitHoldsSelectionRecordsToTheirRole(t *testing.T) {
	tuned := splitPack(t, "tuned", "cap.pcap", 1e9, 3)
	reviewSplitPack(t, tuned, nil)
	writeSegmentRecord(t, tuned, "tuning")
	_, err := FreezeSplit(freezeOptions(splitDraftPack(tuned, "tuned")))
	wantError(t, err, `object "obj_b" is held out, but the pack was selected for tuning (finder manual)`)
	f := mustFreeze(t, freezeOptions(onePartition(tuned, "tuned", "tune", SplitRoleTuning)))
	if sel := f.Packs[0].Selection; sel == nil || sel.Role != "tuning" || sel.Finder != "manual" || !strings.HasPrefix(sel.SHA256, "sha256:") {
		t.Fatalf("selection %+v", sel)
	}

	held := splitPack(t, "held", "cap.pcap", 100e9, 3)
	reviewSplitPack(t, held, nil)
	writeSegmentRecord(t, held, "held_out")
	f = mustFreeze(t, freezeOptions(splitDraftPack(held, "held")))
	if sel := f.Packs[0].Selection; sel == nil || sel.Role != "held_out" || sel.Finder != "random" || !strings.HasPrefix(sel.SegmentID, "seg-") {
		t.Fatalf("selection %+v", sel)
	}

	bad := splitPack(t, "bad", "cap.pcap", 200e9, 3)
	reviewSplitPack(t, bad, nil)
	path := filepath.Join(bad.Dir, segmentRecordFile)
	for name, content := range map[string]string{
		"parse selection record": "{",
		"selection record:":      `{"schema":"other"}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := FreezeSplit(freezeOptions(splitDraftPack(bad, "bad")))
		wantError(t, err, name)
	}
	writeSegmentRecord(t, held, "held_out")
	b, err := os.ReadFile(filepath.Join(held.Dir, segmentRecordFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = FreezeSplit(freezeOptions(splitDraftPack(bad, "bad")))
	wantError(t, err, "selection record names pack "+held.Manifest.PackDigest)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = FreezeSplit(freezeOptions(splitDraftPack(bad, "bad")))
	wantError(t, err, "read selection record")
}

// A pack cut from a declared corpus case takes the case's role, and a screen
// case holds no references at all.
func TestFreezeSplitHoldsPacksToTheirCaseRole(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
	reviewSplitPack(t, p, nil)
	cases := []SplitCase{
		splitCase("tuning-site", SplitRoleTuning), splitCase("held-site", SplitRoleHeldOut), splitCase("screen-site", SplitRoleScreen),
	}
	freeze := func(dp DraftPack) (*FrozenSplit, error) {
		opts := freezeOptions(dp)
		opts.Draft.Cases = cases
		return FreezeSplit(opts)
	}
	mixed := splitDraftPack(p, "p1")
	mixed.CaseID = "tuning-site"
	_, err := freeze(mixed)
	wantError(t, err, `object "obj_b" is in held_out partition "hold", but case "tuning-site" is tuning`)
	mixed.CaseID = "held-site"
	_, err = freeze(mixed)
	wantError(t, err, `object "obj_a" is in tuning partition "tune", but case "held-site" is held_out`)
	mixed.CaseID = "screen-site"
	_, err = freeze(mixed)
	wantError(t, err, "a screen site holds no reference partition")
	mixed.CaseID = "unknown-site"
	_, err = freeze(mixed)
	wantError(t, err, `case "unknown-site" is not declared`)

	held := onePartition(p, "p1", "hold", SplitRoleHeldOut)
	held.CaseID = "held-site"
	f, err := freeze(held)
	if err != nil {
		t.Fatal(err)
	}
	if f.Packs[0].CaseID != "held-site" || len(f.Cases) != 3 || f.Cases[0].CaseID != "held-site" {
		t.Fatalf("cases %+v, pack case %q; want them sorted by id", f.Cases, f.Packs[0].CaseID)
	}
	// The pack gave its case its capture, by the basename it records.
	want := []CaseCapture{{Basename: "cap.pcap"}, cases[1].Captures[0]}
	if fmt.Sprint(f.Cases[0].Captures) != fmt.Sprint(want) {
		t.Fatalf("held-site captures %+v, want %+v", f.Cases[0].Captures, want)
	}
}

// A split of corpus cases alone is the label-free partition: nothing to
// review, but every case has a role.
func TestFreezeSplitOfCasesAlone(t *testing.T) {
	opts := freezeOptions()
	opts.Draft.Cases = []SplitCase{splitCase("b", SplitRoleHeldOut), splitCase("a", SplitRoleTuning)}
	f := mustFreeze(t, opts)
	loaded, _ := writeAndLoad(t, f)
	if len(loaded.Packs) != 0 || len(loaded.Partitions) != 0 || len(loaded.Cases) != 2 || loaded.Cases[0].CaseID != "a" {
		t.Fatalf("cases-only split %+v", loaded)
	}
}

// An existing version 1 manifest freezes as it stands, pinned revision and
// all; given with inline partitions as well, it is ambiguous.
func TestFreezeSplitFromAVersion1Manifest(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
	first := reviewSplitPack(t, p, nil)
	reviewSplitPack(t, p, nil) // revision 2, which the manifest does not pin
	dp := splitDraftPack(p, "p1")
	v1 := SplitManifest{Schema: SplitSchema, SchemaVersion: SplitSchemaVersion, PackDigest: p.Manifest.PackDigest,
		SidecarRevision: first.Revision, Splits: dp.Splits, Episodes: dp.Episodes}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "v1.json"), marshalManifest(t, v1), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := freezeOptions(DraftPack{Dir: p.Dir, SplitManifest: "v1.json"})
	opts.BaseDir = dir
	f := mustFreeze(t, opts)
	if f.Packs[0].SidecarRevision != first.Revision || f.Packs[0].SidecarSHA256 != first.baseDigest {
		t.Fatalf("pinned revision %d (%s), want the manifest's %d", f.Packs[0].SidecarRevision, f.Packs[0].SidecarSHA256, first.Revision)
	}

	both := DraftPack{Dir: p.Dir, SplitManifest: "v1.json", Splits: dp.Splits}
	opts = freezeOptions(both)
	opts.BaseDir = dir
	_, err := FreezeSplit(opts)
	wantError(t, err, "not both")
	opts = freezeOptions(DraftPack{Dir: p.Dir, SplitManifest: "missing.json"})
	opts.BaseDir = dir
	_, err = FreezeSplit(opts)
	wantError(t, err, "open split manifest")
}

func TestFreezeSplitRefusesWhatItCannotFreeze(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
	reviewSplitPack(t, p, nil)
	other := splitPack(t, "p2", "other.pcap", 50e9, 3)
	reviewSplitPack(t, other, nil)
	unannotated := splitPack(t, "p3", "third.pcap", 90e9, 3)
	corrupt := splitPack(t, "p4", "fourth.pcap", 130e9, 3)
	if err := os.WriteFile(filepath.Join(corrupt.Dir, pointsFile), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}

	good := splitDraftPack(p, "p1")
	cases := []struct {
		name   string
		mutate func(*FreezeOptions)
		want   string
	}{
		{"no draft", func(o *FreezeOptions) { o.Draft = nil }, "no draft"},
		{"no time", func(o *FreezeOptions) { o.Now = time.Time{} }, "no freezing time"},
		{"no author", func(o *FreezeOptions) { o.Author = "  " }, "say who froze the split"},
		{"no build", func(o *FreezeOptions) { o.BuildGitSHA = "" }, "git SHA are required"},
		{"negative guard", func(o *FreezeOptions) { o.GuardSeconds = -1 }, "guard_seconds"},
		{"nothing to freeze", func(o *FreezeOptions) { o.Draft.Packs = nil }, "nothing is partitioned"},
		{"no pack dir", func(o *FreezeOptions) { o.Draft.Packs[0].Dir = "" }, "no pack dir"},
		{"missing pack", func(o *FreezeOptions) { o.Draft.Packs[0].Dir = filepath.Join(t.TempDir(), "gone") }, "read pack manifest"},
		{"corrupt pack", func(o *FreezeOptions) { o.Draft.Packs[0].Dir = corrupt.Dir }, "points digest mismatch"},
		{"no episodes", func(o *FreezeOptions) { o.Draft.Packs[0].Episodes = nil }, "no episodes"},
		{"unsaved revision", func(o *FreezeOptions) { o.Draft.Packs[0].SidecarRevision = 9 }, "load annotation"},
		{"no annotation", func(o *FreezeOptions) {
			o.Draft.Packs[0] = onePartition(unannotated, "p3", "tune", SplitRoleTuning)
		}, "no saved annotation"},
		{"object the annotation lacks", func(o *FreezeOptions) {
			o.Draft.Packs[0].Splits[0].ObjectIDs = []string{"obj_a", "obj_z"}
		}, "does not carry"},
		{"one partition, two roles", func(o *FreezeOptions) {
			dp := onePartition(other, "p2", "tune", SplitRoleHeldOut)
			o.Draft.Packs = append(o.Draft.Packs, dp)
		}, `partition "tune" is`},
		{"one pack twice", func(o *FreezeOptions) {
			dp := splitDraftPack(p, "again")
			o.Draft.Packs = append(o.Draft.Packs, dp)
		}, "is listed twice"},
		{"one episode ID in two packs", func(o *FreezeOptions) {
			o.Draft.Packs = append(o.Draft.Packs, onePartition(other, "p1-tune", "tune", SplitRoleTuning))
		}, `episode "p1-tune" is in pack`},
		{"a screen partition", func(o *FreezeOptions) { o.Draft.Packs[0].Splits[0].Role = SplitRoleScreen }, "role"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dp := good
			dp.Splits = append([]Split(nil), good.Splits...)
			opts := freezeOptions(dp)
			tc.mutate(&opts)
			_, err := FreezeSplit(opts)
			wantError(t, err, tc.want)
		})
	}
}

// A frozen split refuses to be read once its content no longer matches its
// digest, and refuses to bind once anything it pinned has changed. A new
// annotation revision is not a change to the split: it keeps scoring the
// revision it pinned.
func TestFrozenSplitRefusesChangesAfterFreezing(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
	s := reviewSplitPack(t, p, nil)
	f, path := writeAndLoad(t, mustFreeze(t, freezeOptions(splitDraftPack(p, "p1"))))

	t.Run("edited content", func(t *testing.T) {
		edited := *f
		edited.Packs = append([]FrozenPack(nil), f.Packs...)
		edited.Packs[0].Objects = append([]FrozenObject(nil), f.Packs[0].Objects...)
		edited.Packs[0].Objects[0].Partition, edited.Packs[0].Objects[1].Partition = "hold", "tune"
		edited.Packs[0].Episodes = []Episode{
			{EpisodeID: "e", Split: "hold", ObjectIDs: []string{"obj_a"}, FrameIntervals: []FrameInterval{{0, 2}}},
		}
		// Left as it was, the tuned record no longer holds the pack's tuning.
		_, err := ParseFrozenSplit(marshalFrozen(t, &edited))
		wantError(t, err, "the tuned record does not hold pack")
		// Made consistent, the edit is still not what was frozen.
		edited.Tuned = mergeLedgers(edited.ownTuning())
		_, err = ParseFrozenSplit(marshalFrozen(t, &edited))
		wantError(t, err, "it was edited after freezing")
	})

	t.Run("a new revision keeps the pinned one", func(t *testing.T) {
		reviewSplitPack(t, p, func(sc *Sidecar) { sc.Objects[1].Notes = "revised" })
		view, bound, err := f.Bind(p)
		if err != nil {
			t.Fatal(err)
		}
		if bound.Revision != s.Revision || view.SidecarRevision != s.Revision || bound.Objects[1].Notes != "" {
			t.Fatalf("bound revision %d, want the pinned %d", bound.Revision, s.Revision)
		}
	})

	t.Run("pack not in the split", func(t *testing.T) {
		other := splitPack(t, "p2", "cap.pcap", 50e9, 3)
		_, _, err := f.Bind(other)
		wantError(t, err, "is not in frozen split")
	})

	for _, tc := range []struct {
		name   string
		change func(t *testing.T, q *Pack)
		want   string
	}{
		{"dataset", func(t *testing.T, q *Pack) { q.Manifest.DatasetID = "ds_other" }, "frozen split names dataset"},
		{"manifest edited", func(t *testing.T, q *Pack) {
			path := filepath.Join(q.Dir, manifestFile)
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(b, ' '), 0o644); err != nil {
				t.Fatal(err)
			}
		}, "manifest.json changed after freezing"},
		{"manifest removed", func(t *testing.T, q *Pack) {
			if err := os.Remove(filepath.Join(q.Dir, manifestFile)); err != nil {
				t.Fatal(err)
			}
		}, "read pack manifest"},
		{"selection record added", func(t *testing.T, q *Pack) { writeSegmentRecord(t, q, "tuning") }, "selection record changed after freezing"},
		{"selection record broken", func(t *testing.T, q *Pack) {
			if err := os.WriteFile(filepath.Join(q.Dir, segmentRecordFile), []byte("{"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, "parse selection record"},
		{"pinned revision lost", func(t *testing.T, q *Pack) {
			if err := os.Remove(filepath.Join(q.Dir, sidecarFile)); err != nil {
				t.Fatal(err)
			}
		}, "load pinned annotation revision 1"},
		{"pinned revision rewritten", func(t *testing.T, q *Pack) {
			s, err := LoadSidecar(q)
			if err != nil {
				t.Fatal(err)
			}
			s.Objects[0].Notes = "rewritten in place"
			b, err := json.MarshalIndent(s, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(q.Dir, sidecarFile), append(b, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}, "is not the bytes that were frozen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := splitPack(t, "q", "cap.pcap", 1e9, 3)
			reviewSplitPack(t, q, nil)
			g := mustFreeze(t, freezeOptions(splitDraftPack(q, "q")))
			tc.change(t, q)
			_, _, err := g.Bind(q)
			wantError(t, err, tc.want)
		})
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

// Bind does not take the frozen review on trust: a split whose pins are
// consistent but whose claims the pinned revision does not support is
// refused. Such a file cannot come from FreezeSplit; it is written here by
// hand, digest and all, as someone could.
func TestBindChecksTheFrozenClaimsAgainstThePinnedRevision(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
	reviewSplitPack(t, p, nil)
	f := mustFreeze(t, freezeOptions(splitDraftPack(p, "p1")))
	// Revision 2 makes obj_a a proposal again and drops obj_b.
	s := reviewSplitPack(t, p, func(sc *Sidecar) {
		sc.Objects[0].Status = StatusProposed
	})

	forged := *f
	forged.Packs = append([]FrozenPack(nil), f.Packs...)
	forged.Packs[0].SidecarRevision, forged.Packs[0].SidecarSHA256 = s.Revision, s.baseDigest
	forged.SplitDigest = forged.contentDigest()
	_, _, err := forged.Bind(p)
	wantError(t, err, "the pinned revision does not support the frozen review")
	wantError(t, err, "object obj_a: object is proposed")

	s = reviewSplitPack(t, p, func(sc *Sidecar) {
		sc.Objects = sc.Objects[:1]
		var kept []FrameMask
		for _, m := range sc.Masks {
			if m.ObjectID == "obj_a" {
				kept = append(kept, m)
			}
		}
		sc.Masks = kept
	})
	forged.Packs[0].SidecarRevision, forged.Packs[0].SidecarSHA256 = s.Revision, s.baseDigest
	_, _, err = forged.Bind(p)
	wantError(t, err, "does not carry")
}

// A later revision may give a held-out object to tuning, once it has been
// looked at, but never the reverse: tuned is tuned. The tuned record carries
// forward and says which revision first tuned on each thing.
func TestFreezeSplitSupersedes(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
	reviewSplitPack(t, p, nil)
	opts := freezeOptions(splitDraftPack(p, "p1"))
	opts.Draft.Cases = []SplitCase{splitCase("site", SplitRoleTuning), splitCase("held", SplitRoleHeldOut)}
	first := mustFreeze(t, opts)
	wantFirst := TunedLedger{
		Spans: []TunedSpan{{Revision: 1, PackDigest: p.Manifest.PackDigest, Source: "pcap:cap.pcap", FirstSampleNs: 1e9, LastSampleNs: 3e9,
			GuardSeconds: DefaultSplitGuardSeconds, ObjectIDs: []string{"obj_a"}}},
		Cases: []TunedCase{{Revision: 1, CaseID: "site", Captures: splitCase("site", SplitRoleTuning).Captures}},
	}
	if fmt.Sprint(first.Tuned) != fmt.Sprint(wantFirst) {
		t.Fatalf("revision 1 tuned %+v, want %+v", first.Tuned, wantFirst)
	}

	next := freezeOptions(onePartition(p, "p1", "tune", SplitRoleTuning))
	next.Draft.Cases = []SplitCase{splitCase("site", SplitRoleTuning), splitCase("held", SplitRoleTuning)}
	next.Supersedes = first
	second := mustFreeze(t, next)
	if second.Revision != 2 || second.Supersedes != first.SplitDigest || second.SplitDigest == first.SplitDigest {
		t.Fatalf("revision %d supersedes %s (digest %s)", second.Revision, second.Supersedes, second.SplitDigest)
	}
	loaded, _ := writeAndLoad(t, second)
	if loaded.Supersedes != first.SplitDigest {
		t.Fatalf("supersedes did not read back: %q", loaded.Supersedes)
	}
	if s := loaded.Tuned.Spans; len(s) != 1 || s[0].Revision != 1 || fmt.Sprint(s[0].ObjectIDs) != "[obj_a obj_b]" {
		t.Fatalf("revision 2 tuned spans %+v, want obj_a and obj_b, first tuned in revision 1", s)
	}
	if c := loaded.Tuned.Cases; len(c) != 2 || c[0].CaseID != "held" || c[0].Revision != 2 || c[1].CaseID != "site" || c[1].Revision != 1 {
		t.Fatalf("revision 2 tuned cases %+v", c)
	}

	undo := freezeOptions(onePartition(p, "p1", "hold", SplitRoleHeldOut))
	undo.Supersedes = second
	_, err := FreezeSplit(undo)
	wantError(t, err, "object obj_a is held out, but revision 1 of this split's lineage tuned on it")

	caseUndo := freezeOptions(onePartition(p, "p1", "tune", SplitRoleTuning))
	caseUndo.Draft.Cases = []SplitCase{splitCase("site", SplitRoleHeldOut)}
	caseUndo.Supersedes = second
	_, err = FreezeSplit(caseUndo)
	wantError(t, err, `case "site" is held out, but revision 1 of this split's lineage tuned on it`)

	// A pack held out in revision 1 beside a tuned one in the same frames
	// stays held out when its successor keeps the partition as it was.
	same := freezeOptions(splitDraftPack(p, "p1"))
	same.Draft.Cases = opts.Draft.Cases
	same.Supersedes = first
	if kept := mustFreeze(t, same); fmt.Sprint(kept.Tuned) != fmt.Sprint(first.Tuned) {
		t.Fatalf("an unchanged successor's tuned record %+v, want revision 1's %+v", kept.Tuned, first.Tuned)
	}

	// A case whose capture was replaced keeps both in the record: the one
	// tuned on is not forgotten because the file changed.
	replaced := freezeOptions(onePartition(p, "p1", "tune", SplitRoleTuning))
	replaced.Draft.Cases = []SplitCase{{CaseID: "site", Role: SplitRoleTuning,
		Captures: []CaseCapture{{Basename: "site.pcap", SHA256: sha256Hex([]byte("site, recaptured"))}}}}
	replaced.Supersedes = second
	third := mustFreeze(t, replaced)
	if c := third.Tuned.Cases[1]; c.CaseID != "site" || len(c.Captures) != 2 || c.Captures[0].SHA256 > c.Captures[1].SHA256 {
		t.Fatalf("site's tuned captures %+v, want both, ordered by digest", c.Captures)
	}

	// Without --supersedes a freeze starts a new lineage, whose record is its
	// own tuning alone.
	fresh := mustFreeze(t, freezeOptions(onePartition(p, "p1", "hold", SplitRoleHeldOut)))
	if fresh.Revision != 1 || len(fresh.Tuned.Spans) != 0 || len(fresh.Tuned.Cases) != 0 {
		t.Fatalf("a new lineage inherited a tuned record: %+v", fresh.Tuned)
	}
}

// What a revision drops stays tuned: revision 1 tunes a pack and a case,
// revision 2 drops both, and revision 3 cannot hold either out.
func TestSupersedesRemembersWhatARevisionDropped(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
	q := splitPack(t, "q1", "other.pcap", 500e9, 3)
	reviewSplitPack(t, p, nil)
	reviewSplitPack(t, q, nil)
	o1 := freezeOptions(onePartition(p, "p1", "tune", SplitRoleTuning))
	o1.Draft.Cases = []SplitCase{splitCase("site", SplitRoleTuning)}
	r1 := mustFreeze(t, o1)

	o2 := freezeOptions(onePartition(q, "q1", "tune", SplitRoleTuning))
	o2.Supersedes = r1
	r2 := mustFreeze(t, o2)
	if len(r2.Tuned.Spans) != 2 || len(r2.Tuned.Cases) != 1 || r2.Tuned.Cases[0].CaseID != "site" {
		t.Fatalf("revision 2 forgot what revision 1 tuned on: %+v", r2.Tuned)
	}
	r2, _ = writeAndLoad(t, r2)

	o3 := freezeOptions(onePartition(p, "p1", "hold", SplitRoleHeldOut))
	o3.Supersedes = r2
	_, err := FreezeSplit(o3)
	wantError(t, err, "object obj_a is held out, but revision 1 of this split's lineage tuned on it")

	o3 = freezeOptions(onePartition(q, "q1", "tune", SplitRoleTuning))
	o3.Draft.Cases = []SplitCase{splitCase("site", SplitRoleHeldOut)}
	o3.Supersedes = r2
	_, err = FreezeSplit(o3)
	wantError(t, err, `case "site" is held out, but revision 1 of this split's lineage tuned on it`)
}

// Object IDs are pack-local, so a pack cut again from a tuned stretch of the
// same capture holds the tuned vehicles under new IDs: it cannot be held out,
// under the wider of the two revisions' guards. Nor can a pack cut from a
// tuned case's capture, a case whose capture a tuned pack was cut from, or a
// renamed case replaying a tuned case's capture.
func TestSupersedesRefusesToHoldOutWhatWasTunedUnderAnotherName(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)       // 1-3 s
	recut := splitPack(t, "p1b", "cap.pcap", 1e9, 4)  // 1-4 s
	near := splitPack(t, "near", "cap.pcap", 23e9, 3) // 20 s after p
	far := splitPack(t, "far", "cap.pcap", 100e9, 3)
	caseCut := splitPack(t, "case", "site.pcap", 7e9, 3)
	other := splitPack(t, "other", "other.pcap", 300e9, 3)
	for _, q := range []*Pack{p, recut, near, far, caseCut, other} {
		reviewSplitPack(t, q, nil)
	}
	o1 := freezeOptions(onePartition(p, "p1", "tune", SplitRoleTuning))
	o1.Draft.Cases = []SplitCase{splitCase("site", SplitRoleTuning)}
	r1 := mustFreeze(t, o1)

	for _, tc := range []struct {
		name  string
		draft DraftPack
		cases []SplitCase
		guard float64
		want  string
	}{
		{"a re-cut of the tuned pack", onePartition(recut, "p1b", "hold", SplitRoleHeldOut), nil, DefaultSplitGuardSeconds,
			"holds out object obj_a, but pack " + p.Manifest.PackDigest + ", tuned on in revision 1, is cut from pcap:cap.pcap within 30s of it"},
		{"a pack within the tuned guard, under a narrower one", onePartition(near, "near", "hold", SplitRoleHeldOut), nil, 0,
			"is cut from pcap:cap.pcap within 30s of it"},
		{"a pack of a tuned case's capture", onePartition(caseCut, "case", "hold", SplitRoleHeldOut), nil, DefaultSplitGuardSeconds,
			`it is cut from capture site.pcap of case "site", tuned on in revision 1`},
		{"a renamed case replaying a tuned capture", onePartition(other, "other", "tune", SplitRoleTuning),
			[]SplitCase{{CaseID: "renamed", Role: SplitRoleHeldOut, Captures: []CaseCapture{{Basename: "copy.pcap", SHA256: sha256Hex([]byte("site"))}}}},
			DefaultSplitGuardSeconds, `case "renamed" is held out, but its capture copy.pcap is case "site"'s, tuned on in revision 1`},
		{"a case whose capture a tuned pack was cut from", onePartition(other, "other", "tune", SplitRoleTuning),
			[]SplitCase{{CaseID: "cap", Role: SplitRoleHeldOut, Captures: []CaseCapture{{Basename: "cap.pcap", SHA256: sha256Hex([]byte("cap"))}}}},
			DefaultSplitGuardSeconds, `case "cap" is held out, but pack ` + p.Manifest.PackDigest + ", tuned on in revision 1, is cut from its capture cap.pcap"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := freezeOptions(tc.draft)
			o.Draft.Cases, o.GuardSeconds, o.Supersedes = tc.cases, tc.guard, r1
			_, err := FreezeSplit(o)
			wantError(t, err, tc.want)
		})
	}

	// Far enough from everything tuned, a pack of the same capture can be
	// held out.
	o := freezeOptions(onePartition(far, "far", "hold", SplitRoleHeldOut))
	o.Supersedes = r1
	mustFreeze(t, o)
}

// A case's role binds to its captures: a pack cut from a case's capture takes
// the case's role whether or not it names the case, and a capture cannot
// belong to cases of two roles.
func TestFreezeSplitBindsCasesToTheirCaptures(t *testing.T) {
	kirk := splitPack(t, "kirk", "kirk0.pcap", 1e9, 3)
	anon := splitPack(t, "anon", "", 1e9, 3)
	for _, q := range []*Pack{kirk, anon} {
		reviewSplitPack(t, q, nil)
	}
	kirk0 := splitCase("kirk0", SplitRoleHeldOut)
	freeze := func(cases []SplitCase, packs ...DraftPack) (*FrozenSplit, error) {
		opts := freezeOptions(packs...)
		opts.Draft.Cases = cases
		return FreezeSplit(opts)
	}

	// The review's scenario: kirk0 is held out, and a pack cut from its
	// capture that does not name it is refused a tuning partition.
	_, err := freeze([]SplitCase{kirk0}, onePartition(kirk, "kirk", "tune", SplitRoleTuning))
	wantError(t, err, `object "obj_a" is in tuning partition "tune", but case "kirk0" is held_out: a pack cut from a case's capture (kirk0.pcap)`)
	f, err := freeze([]SplitCase{kirk0}, onePartition(kirk, "kirk", "hold", SplitRoleHeldOut))
	if err != nil || f.Packs[0].CaseID != "" {
		t.Fatalf("a held-out pack of a held-out case's capture: %+v, %v", f, err)
	}
	_, err = freeze([]SplitCase{splitCase("kirk0", SplitRoleScreen)}, onePartition(kirk, "kirk", "hold", SplitRoleHeldOut))
	wantError(t, err, `is cut from capture "kirk0.pcap" of case "kirk0", a screen case`)

	for _, tc := range []struct {
		name  string
		cases []SplitCase
		packs []DraftPack
		want  string
	}{
		{"an undeclared digest", []SplitCase{{CaseID: "kirk0", Role: SplitRoleTuning, Captures: []CaseCapture{{Basename: "kirk0.pcap"}}}},
			nil, `draft case "kirk0" capture "kirk0.pcap" has no sha256`},
		{"no capture", []SplitCase{{CaseID: "kirk0", Role: SplitRoleTuning}}, nil, `case "kirk0" names no capture`},
		{"one basename, two roles", []SplitCase{kirk0, {CaseID: "copy", Role: SplitRoleTuning,
			Captures: []CaseCapture{{Basename: "kirk0.pcap", SHA256: sha256Hex([]byte("copy"))}}}},
			nil, `capture "kirk0.pcap" is in tuning case "copy" and held_out case "kirk0"`},
		{"one digest, two roles", []SplitCase{kirk0, {CaseID: "copy", Role: SplitRoleTuning,
			Captures: []CaseCapture{{Basename: "copy.pcap", SHA256: kirk0.Captures[0].SHA256}}}},
			nil, "one capture cannot take two roles"},
		{"a pack naming another case's capture", []SplitCase{kirk0, splitCase("other", SplitRoleTuning)},
			[]DraftPack{func() DraftPack {
				d := onePartition(kirk, "kirk", "tune", SplitRoleTuning)
				d.CaseID = "other"
				return d
			}()},
			"one capture cannot take two roles"},
		{"a pack naming a case with no capture of its own", []SplitCase{kirk0},
			[]DraftPack{func() DraftPack {
				d := onePartition(anon, "anon", "hold", SplitRoleHeldOut)
				d.CaseID = "kirk0"
				return d
			}()},
			`the pack names case "kirk0" but records no capture file`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := freeze(tc.cases, tc.packs...)
			wantError(t, err, tc.want)
		})
	}

	// Cases of one role may share a capture.
	shared := SplitCase{CaseID: "kirk0-late", Role: SplitRoleHeldOut, Captures: kirk0.Captures}
	if _, err := freeze([]SplitCase{kirk0, shared}); err != nil {
		t.Fatal(err)
	}
}

// A replay under a split is checked against the case's captures: by content
// where the case declares a digest, by basename where it has only a pack's.
func TestCheckCaseCaptures(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	kirk0 := write("kirk0.pcap", "kirk0 capture")
	renamed := write("renamed.pcap", "kirk0 capture")
	impostor := write("impostor.pcap", "another capture")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	sameName := write("sub/kirk0.pcap", "another capture")
	held := write("held.pcap", "held capture")

	p := splitPack(t, "p1", "derived.pcap", 1e9, 3)
	reviewSplitPack(t, p, nil)
	dp := onePartition(p, "p1", "tune", SplitRoleTuning)
	dp.CaseID = "derived"
	opts := freezeOptions(dp)
	sum := func(path string) string {
		s, err := captureFileSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	opts.Draft.Cases = []SplitCase{
		{CaseID: "kirk0", Role: SplitRoleTuning, Captures: []CaseCapture{{Basename: "kirk0.pcap", SHA256: sum(kirk0)}}},
		{CaseID: "held", Role: SplitRoleHeldOut, Captures: []CaseCapture{{Basename: "held.pcap", SHA256: sum(held)}}},
		{CaseID: "derived", Role: SplitRoleTuning},
	}
	f := mustFreeze(t, opts)

	for _, ok := range []struct {
		caseID string
		paths  []string
	}{
		{"kirk0", []string{kirk0}},
		{"kirk0", []string{renamed}},                                        // by content, whatever its name
		{"derived", []string{filepath.Join(dir, "absent", "derived.pcap")}}, // by basename, unread
	} {
		if err := f.CheckCaseCaptures(ok.caseID, ok.paths); err != nil {
			t.Errorf("%s %v: %v", ok.caseID, ok.paths, err)
		}
	}
	for _, tc := range []struct {
		caseID string
		paths  []string
		want   string
	}{
		{"kirk0", []string{held}, `is not one of case "kirk0"'s captures`}, // a held-out capture under a tuning name
		{"kirk0", []string{sameName}, `is not one of case "kirk0"'s captures`},
		{"kirk0", []string{kirk0, impostor}, "impostor.pcap"},
		{"derived", []string{impostor}, `is not one of case "derived"'s captures`},
		{"kirk0", []string{filepath.Join(dir, "absent.pcap")}, "hash capture"},
		{"kirk0", []string{filepath.Join(dir, "sub")}, "hash capture"},
		{"kirk0", nil, "no capture to check"},
		{"columbus", []string{kirk0}, `case "columbus" has no role`},
	} {
		if err := f.CheckCaseCaptures(tc.caseID, tc.paths); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s %v: error %v, want one mentioning %q", tc.caseID, tc.paths, err, tc.want)
		}
	}
}

// Bind derives again what the file copies from pinned bytes, so a file edited
// with its digest recomputed cannot change the selection record's role, the
// capture span or an object's review counts.
func TestBindDerivesWhatTheFileCopies(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
	reviewSplitPack(t, p, nil)
	writeSegmentRecord(t, p, "tuning")
	f := mustFreeze(t, freezeOptions(onePartition(p, "p1", "tune", SplitRoleTuning)))
	for _, tc := range []struct {
		name  string
		forge func(*FrozenPack)
		want  string
	}{
		{"selection role", func(fp *FrozenPack) {
			fp.Selection = &FrozenSelection{SHA256: fp.Selection.SHA256, Role: "held_out", Finder: "random"}
		},
			"is not the pinned record's"},
		{"capture span", func(fp *FrozenPack) { fp.Source.FirstSampleNs += 60e9 }, "is not the pack's"},
		{"capture file", func(fp *FrozenPack) { fp.Source.PCAPBasename = "other.pcap" }, "is not the pack's"},
		{"review counts", func(fp *FrozenPack) {
			fp.Objects[0].ReviewedMasks, fp.Objects[0].Class = 2, "truck"
		}, "but the pinned revision gives"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forged := *f
			pk := f.Packs[0]
			pk.Objects = append([]FrozenObject(nil), pk.Objects...)
			tc.forge(&pk)
			forged.Packs = []FrozenPack{pk}
			forged.SplitDigest = forged.contentDigest()
			_, _, err := forged.Bind(p)
			wantError(t, err, tc.want)
		})
	}
}

// Every structural rule refuses on read, with its digest recomputed so the
// rule itself, not the digest, is what refuses.
func TestFrozenSplitStructuralRefusals(t *testing.T) {
	p := splitPack(t, "p1", "cap.pcap", 1e9, 3)
	reviewSplitPack(t, p, nil)
	opts := freezeOptions(splitDraftPack(p, "p1"))
	opts.Draft.Cases = []SplitCase{splitCase("site", SplitRoleTuning)}
	base := mustFreeze(t, opts)

	siteCapture := base.Cases[0].Captures[0]
	cases := []struct {
		name   string
		mutate func(*FrozenSplit)
		want   string
	}{
		{"schema", func(f *FrozenSplit) { f.Schema = "other" }, "schema"},
		{"version", func(f *FrozenSplit) { f.SchemaVersion = 3 }, "schema version 3"},
		{"revision 0", func(f *FrozenSplit) { f.Revision = 0 }, "revisions count from 1"},
		{"revision 1 superseding", func(f *FrozenSplit) { f.Supersedes = "sha256:x" }, "only revision 1 supersedes nothing"},
		{"revision 2 superseding nothing", func(f *FrozenSplit) { f.Revision = 2 }, "only revision 1 supersedes nothing"},
		{"bare supersedes", func(f *FrozenSplit) { f.Revision, f.Supersedes = 2, "x" }, "supersedes"},
		{"no author", func(f *FrozenSplit) { f.Frozen.Author = "" }, "frozen.author"},
		{"bad time", func(f *FrozenSplit) { f.Frozen.FrozenUTC = "yesterday" }, "RFC 3339"},
		{"guard too long", func(f *FrozenSplit) { f.GuardSeconds = 1e6 }, "guard_seconds"},
		{"unnamed partition", func(f *FrozenSplit) { f.Partitions[0].Name = "" }, "has no name"},
		{"partition twice", func(f *FrozenSplit) { f.Partitions[1].Name = f.Partitions[0].Name }, "declared twice"},
		{"screen partition", func(f *FrozenSplit) { f.Partitions[0].Role = SplitRoleScreen }, `partition "hold" has role "screen"`},
		{"unnamed case", func(f *FrozenSplit) { f.Cases[0].CaseID = " " }, "case 0 has no id"},
		{"case twice", func(f *FrozenSplit) { f.Cases = append(f.Cases, f.Cases[0]) }, `case "site" is declared twice`},
		{"case role", func(f *FrozenSplit) { f.Cases[0].Role = "gate" }, `case "site" has role "gate"`},
		{"bare pack digest", func(f *FrozenSplit) { f.Packs[0].PackDigest = "abc" }, "pack_digest"},
		{"no dataset", func(f *FrozenSplit) { f.Packs[0].DatasetID = "" }, "no dataset_id"},
		{"unpinned revision", func(f *FrozenSplit) { f.Packs[0].SidecarRevision = 0 }, "pins a saved revision"},
		{"bare selection digest", func(f *FrozenSplit) {
			f.Packs[0].Selection = &FrozenSelection{SHA256: "x", Role: "tuning", Finder: "manual"}
		}, "selection sha256"},
		{"selection role", func(f *FrozenSplit) {
			f.Packs[0].Selection = &FrozenSelection{SHA256: "sha256:x", Role: "screen", Finder: "manual"}
		}, "selection role"},
		{"no objects", func(f *FrozenSplit) { f.Packs[0].Objects = nil }, "no objects"},
		{"unnamed object", func(f *FrozenSplit) { f.Packs[0].Objects[0].ObjectID = "" }, "an object has no id"},
		{"object twice", func(f *FrozenSplit) { f.Packs[0].Objects[1].ObjectID = "obj_a" }, "object-disjoint"},
		{"unknown partition", func(f *FrozenSplit) { f.Packs[0].Objects[0].Partition = "gate" }, "unknown partition"},
		{"no reviewed mask", func(f *FrozenSplit) { f.Packs[0].Objects[0].ReviewedMasks = 0 }, "has no reviewed mask"},
		{"pose counts", func(f *FrozenSplit) { f.Packs[0].Objects[0].Geometry.ReviewedPoses = 4 }, "reviewed masks"},
		{"negative pose count", func(f *FrozenSplit) { f.Packs[0].Objects[0].Geometry.ProposedPoses = -1 }, "reviewed masks"},
		{"geometry status", func(f *FrozenSplit) { f.Packs[0].Objects[0].Geometry.Status = GeometryComplete }, "but the counts make it"},
		{"episode scoring another partition", func(f *FrozenSplit) { f.Packs[0].Episodes[0].Split = "hold" }, "scores object"},
		{"case with no capture", func(f *FrozenSplit) { f.Cases[0].Captures = nil }, `case "site" names no capture`},
		{"capture twice", func(f *FrozenSplit) { f.Cases[0].Captures = []CaseCapture{siteCapture, siteCapture} }, `names capture "site.pcap" twice`},
		{"capture path", func(f *FrozenSplit) {
			f.Cases[0].Captures = []CaseCapture{{Basename: "dir/site.pcap", SHA256: siteCapture.SHA256}}
		}, "is not a file name"},
		{"short capture digest", func(f *FrozenSplit) {
			f.Cases[0].Captures = []CaseCapture{{Basename: "site.pcap", SHA256: "sha256:abc"}}
		}, "is not a sha256:<64 hex> digest"},
		{"capture digest not hex", func(f *FrozenSplit) {
			f.Cases[0].Captures = []CaseCapture{{Basename: "site.pcap", SHA256: "sha256:" + strings.Repeat("g", 64)}}
		}, "is not a sha256:<64 hex> digest"},
		{"pack naming a case it was not cut from", func(f *FrozenSplit) { f.Packs[0].CaseID = "site" }, `its capture "cap.pcap" is not one of the case's captures`},
		{"tuned span revision", func(f *FrozenSplit) { f.Tuned.Spans[0].Revision = 2 }, "tuned span 0: revision 2 is outside this lineage's 1 to 1"},
		{"tuned span digest", func(f *FrozenSplit) { f.Tuned.Spans[0].PackDigest = "abc" }, "tuned span 0: pack_digest"},
		{"tuned span backwards", func(f *FrozenSplit) { f.Tuned.Spans[0].FirstSampleNs = 4e9 }, "ends before it starts"},
		{"tuned span guard", func(f *FrozenSplit) { f.Tuned.Spans[0].GuardSeconds = -1 }, "tuned span 0: guard_seconds"},
		{"tuned span without objects", func(f *FrozenSplit) { f.Tuned.Spans[0].ObjectIDs = nil }, "names no tuned object"},
		{"tuned object twice", func(f *FrozenSplit) { f.Tuned.Spans[0].ObjectIDs = []string{"obj_a", "obj_a"} }, "must be distinct and non-empty"},
		{"tuned span twice", func(f *FrozenSplit) { f.Tuned.Spans = append(f.Tuned.Spans, f.Tuned.Spans[0]) }, "tuned span 1 repeats pack"},
		{"tuned case revision", func(f *FrozenSplit) { f.Tuned.Cases[0].Revision = 0 }, "tuned case 0: revision 0"},
		{"tuned case without id", func(f *FrozenSplit) { f.Tuned.Cases[0].CaseID = " " }, "tuned case 0 has no id"},
		{"tuned case twice", func(f *FrozenSplit) { f.Tuned.Cases = append(f.Tuned.Cases, f.Tuned.Cases[0]) }, `tuned case "site" is recorded twice`},
		{"tuned case without capture", func(f *FrozenSplit) { f.Tuned.Cases[0].Captures = nil }, `tuned case "site" names no capture`},
		{"tuned case capture", func(f *FrozenSplit) { f.Tuned.Cases[0].Captures = []CaseCapture{{}} }, `tuned case "site": capture basename`},
		{"own tuning missing", func(f *FrozenSplit) { f.Tuned.Spans = nil }, "the tuned record does not hold pack"},
		{"own guard narrowed", func(f *FrozenSplit) { f.Tuned.Spans[0].GuardSeconds = 0 }, "the tuned record does not hold pack"},
		{"own tuned object missing", func(f *FrozenSplit) { f.Tuned.Spans[0].ObjectIDs = []string{"obj_z"} }, "the tuned record does not hold pack"},
		{"own tuned case missing", func(f *FrozenSplit) { f.Tuned.Cases = nil }, `the tuned record does not hold case "site"`},
		{"own tuned capture missing", func(f *FrozenSplit) {
			f.Tuned.Cases[0].Captures = []CaseCapture{{Basename: "elsewhere.pcap"}}
		}, `the tuned record does not hold case "site"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := *base
			f.Partitions = append([]SplitPartition(nil), base.Partitions...)
			f.Cases = append([]SplitCase(nil), base.Cases...)
			f.Tuned = TunedLedger{Spans: append([]TunedSpan(nil), base.Tuned.Spans...), Cases: append([]TunedCase(nil), base.Tuned.Cases...)}
			pk := base.Packs[0]
			pk.Objects = append([]FrozenObject(nil), pk.Objects...)
			pk.Episodes = append([]Episode(nil), pk.Episodes...)
			f.Packs = []FrozenPack{pk}
			tc.mutate(&f)
			f.SplitDigest = f.contentDigest()
			_, err := ParseFrozenSplit(marshalFrozen(t, &f))
			wantError(t, err, tc.want)
		})
	}

	b := marshalFrozen(t, base)
	_, err := ParseFrozenSplit(append([]byte(`{"splits": [],`), b[1:]...))
	wantError(t, err, "unknown field")
	for _, trailing := range []string{" {}", "}", "\n]]]"} {
		_, err = ParseFrozenSplit(append(append([]byte(nil), b...), trailing...))
		wantError(t, err, "trailing data")
	}
	if _, err := ParseFrozenSplit(append(append([]byte(nil), b...), "\n"...)); err != nil {
		t.Fatalf("a trailing newline was refused: %v", err)
	}
}

func TestCaseRolesRefuseWhatTheSplitForbids(t *testing.T) {
	opts := freezeOptions()
	opts.Draft.Cases = []SplitCase{
		splitCase("marina", SplitRoleTuning), splitCase("embarcadero", SplitRoleHeldOut), splitCase("ashbury", SplitRoleScreen),
	}
	f := mustFreeze(t, opts)

	roles, err := f.CaseRoles([]string{"marina", "ashbury"}, false)
	if err != nil || roles["marina"] != SplitRoleTuning || roles["ashbury"] != SplitRoleScreen {
		t.Fatalf("roles %v, %v", roles, err)
	}
	_, err = f.CaseRoles([]string{"marina", "embarcadero"}, false)
	wantError(t, err, `case "embarcadero" is held out`)
	roles, err = f.CaseRoles([]string{"embarcadero"}, true)
	if err != nil || roles["embarcadero"] != SplitRoleHeldOut {
		t.Fatalf("held-out roles %v, %v", roles, err)
	}
	if _, err := f.CaseRoles([]string{"embarcadero", "marina"}, true); !errors.Is(err, ErrNotHeldOut) {
		t.Fatalf("a tuning case in a held-out score: %v", err)
	}
	_, err = f.CaseRoles([]string{"columbus"}, false)
	wantError(t, err, `case "columbus" has no role`)
}

func TestLoadSplitDraft(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	d, err := LoadSplitDraft(write("good.json", `{"schema": "velocity.report/annotation-split-draft", "schema_version": 1,
		"note": "n", "cases": [{"case_id": "a", "role": "tuning"}], "packs": [{"dir": "p", "split_manifest": "m.json"}]}`))
	if err != nil || d.Note != "n" || len(d.Cases) != 1 || d.Packs[0].SplitManifest != "m.json" {
		t.Fatalf("draft %+v, %v", d, err)
	}
	for name, tc := range map[string]struct{ content, want string }{
		"unknown":  {`{"schema": "velocity.report/annotation-split-draft", "schema_version": 1, "pack": []}`, "unknown field"},
		"trailing": {`{"schema": "velocity.report/annotation-split-draft", "schema_version": 1} {}`, "trailing data"},
		"brace":    {`{"schema": "velocity.report/annotation-split-draft", "schema_version": 1}}`, "trailing data"},
		"brackets": {`{"schema": "velocity.report/annotation-split-draft", "schema_version": 1}]]]`, "trailing data"},
		"schema":   {`{"schema": "velocity.report/annotation-split", "schema_version": 1}`, "split draft schema"},
	} {
		_, err := LoadSplitDraft(write(name+".json", tc.content))
		wantError(t, err, tc.want)
	}
	_, err = LoadSplitDraft(filepath.Join(dir, "missing.json"))
	wantError(t, err, "open split draft")
}

// Either version reads through one entry point; a caller that needs a frozen
// split refuses a hand-written one.
func TestLoadAnySplitReadsBothVersions(t *testing.T) {
	dir := t.TempDir()
	v1Path := filepath.Join(dir, "v1.json")
	if err := os.WriteFile(v1Path, marshalManifest(t, validSplitManifest("sha256:abc")), 0o644); err != nil {
		t.Fatal(err)
	}
	v1, frozen, err := LoadAnySplit(v1Path)
	if err != nil || v1 == nil || frozen != nil || v1.PackDigest != "sha256:abc" {
		t.Fatalf("version 1: %+v, %+v, %v", v1, frozen, err)
	}
	_, err = LoadFrozenSplit(v1Path)
	wantError(t, err, "is version 1, not frozen")

	opts := freezeOptions()
	opts.Draft.Cases = []SplitCase{splitCase("a", SplitRoleTuning)}
	_, path := writeAndLoad(t, mustFreeze(t, opts))
	v1, frozen, err = LoadAnySplit(path)
	if err != nil || v1 != nil || frozen == nil {
		t.Fatalf("version 2: %+v, %+v, %v", v1, frozen, err)
	}

	big := filepath.Join(dir, "big.json")
	if err := os.WriteFile(big, make([]byte, MaxSplitManifestBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = LoadAnySplit(big)
	wantError(t, err, "exceeds")
	_, err = LoadFrozenSplit(dir)
	wantError(t, err, "read split manifest")
	_, err = LoadFrozenSplit(filepath.Join(dir, "missing.json"))
	wantError(t, err, "open split manifest")
}

type failingSplitOutput struct{ path string }

func (f failingSplitOutput) Write(b []byte) (int, error) {
	return len(b), os.WriteFile(f.path, b, 0o644)
}
func (failingSplitOutput) Sync() error  { return errors.New("disk full") }
func (failingSplitOutput) Close() error { return nil }

// A frozen split is written once: an existing file is never replaced, and a
// failed write leaves nothing behind.
func TestWriteFrozenSplitIsWriteOnce(t *testing.T) {
	opts := freezeOptions()
	opts.Draft.Cases = []SplitCase{splitCase("a", SplitRoleTuning)}
	f := mustFreeze(t, opts)
	_, path := writeAndLoad(t, f)
	wantError(t, WriteFrozenSplit(path, f), "create frozen split")

	failing := filepath.Join(t.TempDir(), "failing.json")
	err := writeFrozenSplitWithOpen(failing, f, func(p string) (splitOutput, error) { return failingSplitOutput{p}, nil })
	wantError(t, err, "disk full")
	if _, statErr := os.Stat(failing); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a failed write left %s behind: %v", failing, statErr)
	}
}
