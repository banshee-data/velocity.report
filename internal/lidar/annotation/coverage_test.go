package annotation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
)

// The error paths matter more than the happy one here: every branch below is a
// way for a pack or a sidecar to be wrong, and each must fail rather than
// produce something that reads as valid.

func TestOpenPackRejectsStructurallyBrokenPacks(t *testing.T) {
	rewriteManifest := func(t *testing.T, dir string, mutate func(*Manifest)) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, manifestFile))
		if err != nil {
			t.Fatal(err)
		}
		var m Manifest
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		mutate(&m)
		out, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, manifestFile), append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cases := map[string]func(*Manifest){
		"future schema":       func(m *Manifest) { m.SchemaVersion = PackSchemaVersion + 1 },
		"sample count lies":   func(m *Manifest) { m.SampleCount = 99 },
		"pack digest rewired": func(m *Manifest) { m.PackDigest = "sha256:deadbeef" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := synthPack(t)
			rewriteManifest(t, p.Dir, mutate)
			if _, err := OpenPack(p.Dir); err == nil {
				t.Fatal("a broken manifest opened")
			}
		})
	}
}

func TestOpenPackRejectsBrokenSampleTable(t *testing.T) {
	cases := map[string]func([]Sample) []Sample{
		"non-dense ids":   func(s []Sample) []Sample { s[1].SampleID = 7; return s },
		"negative points": func(s []Sample) []Sample { s[0].PointCount = -1; return s },
		"offset past eof": func(s []Sample) []Sample { s[0].ByteOffset = 1 << 40; return s },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := synthPack(t)
			samples := mutate(append([]Sample(nil), p.Samples...))
			b, err := json.MarshalIndent(samples, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			// Keep the digest honest so the structural check is what fires,
			// not the tamper check.
			rewriteWithDigest(t, p.Dir, b)
			if _, err := OpenPack(p.Dir); err == nil {
				t.Fatal("a broken sample table opened")
			}
		})
	}
}

// rewriteWithDigest replaces samples.json and updates the manifest digests, so
// a test can exercise structural validation rather than digest validation.
func rewriteWithDigest(t *testing.T, dir string, samplesJSON []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, samplesFile), append(samplesJSON, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m.SamplesSHA256 = sha256Hex(samplesJSON)
	m.PackDigest = sha256Hex([]byte(m.PointsSHA256 + "\n" + m.SamplesSHA256))
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestFile), append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenPackRejectsMissingAndUnparseableFiles(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nothing")
	if _, err := OpenPack(missing); err == nil {
		t.Fatal("a directory that does not exist opened")
	}

	for _, name := range []string{manifestFile, samplesFile, pointsFile} {
		p := synthPack(t)
		if err := os.Remove(filepath.Join(p.Dir, name)); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenPack(p.Dir); err == nil {
			t.Fatalf("a pack missing %s opened", name)
		}
	}

	p := synthPack(t)
	if err := os.WriteFile(filepath.Join(p.Dir, manifestFile), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenPack(p.Dir); err == nil {
		t.Fatal("an unparseable manifest opened")
	}
}

func TestSamplesMustParseAsWellAsMatch(t *testing.T) {
	p := synthPack(t)
	rewriteWithDigest(t, p.Dir, []byte("[not json"))
	if _, err := OpenPack(p.Dir); err == nil {
		t.Fatal("unparseable samples opened")
	}
}

func TestWritePackRejectsInconsistentInput(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pack")
	block, _ := encodePoints(Points{X: []float32{1}, Y: []float32{1}, Z: []float32{1}})

	if err := WritePack(dir, Manifest{}, []Sample{{PointCount: 1}}, nil); err == nil {
		t.Fatal("samples without blocks were written")
	}
	// A declared point count that disagrees with the block is the mismatch
	// that would silently shift every later sample's offset.
	if err := WritePack(dir, Manifest{}, []Sample{{PointCount: 5}}, [][]byte{block}); err == nil {
		t.Fatal("a block of the wrong size was written")
	}
}

func TestPointsAtRejectsOutOfRange(t *testing.T) {
	p := synthPack(t)
	for _, id := range []int{-1, 99} {
		if _, err := p.PointsAt(id); err == nil {
			t.Fatalf("sample %d decoded", id)
		}
	}
}

func TestDecodeRejectsShortBlock(t *testing.T) {
	if _, err := decodePoints([]byte{1, 2, 3}, 4); err == nil {
		t.Fatal("a truncated block decoded")
	}
}

func TestEncodeRejectsOversizedSample(t *testing.T) {
	n := MaxPointsPerSample + 1
	_, err := encodePoints(Points{X: make([]float32, n), Y: make([]float32, n), Z: make([]float32, n)})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("an oversized sample encoded: %v", err)
	}
}

func TestDigestHelpersReportMissingPaths(t *testing.T) {
	if _, err := fileSHA256(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("hashed a file that is not there")
	}
	if _, err := dirSHA256(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("hashed a directory that is not there")
	}
	// A directory digest must ignore subdirectories and cover file content.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := dirSHA256(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := dirSHA256(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("changed content produced the same digest")
	}
}

func TestSidecarRefusesUnwritableLocation(t *testing.T) {
	p := synthPack(t)
	// Replacing the pack directory with a file makes every write beneath it
	// fail, which is the closest portable stand-in for a full or read-only
	// disk.
	if err := os.RemoveAll(p.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Dir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewSidecar(p)
	if err := SaveSidecar(p, s); err == nil {
		t.Fatal("wrote a sidecar into a path that is not a directory")
	}
	if _, err := LoadSidecar(p); err == nil {
		t.Fatal("read a sidecar from a path that is not a directory")
	}
}

func TestSidecarSchemaAndDatasetChecks(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)

	s.SchemaVersion = SidecarSchemaVersion + 1
	if err := s.Validate(p); err == nil {
		t.Fatal("a future schema validated")
	}
	s.SchemaVersion = SidecarSchemaVersion

	s.DatasetID = "ds_someone_elses"
	if err := s.Validate(p); err == nil {
		t.Fatal("a foreign dataset id validated")
	}
	s.DatasetID = p.Manifest.DatasetID

	s.Objects = []Object{{ObjectID: "", Class: "car", Status: StatusReviewed}}
	if err := s.Validate(p); err == nil {
		t.Fatal("an object with no id validated")
	}
}

func TestMaskCompletenessIsChecked(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("obj_a", "car")}
	m := mask("obj_a", 0, 0)
	m.Completeness = "mostly"
	s.Masks = []FrameMask{m}
	if err := s.Validate(p); err == nil {
		t.Fatal("an unknown completeness validated")
	}
	m.Completeness = MaskComplete
	m.Status = "maybe"
	s.Masks = []FrameMask{m}
	if err := s.Validate(p); err == nil {
		t.Fatal("an unknown mask status validated")
	}
}

func TestUncertainIndicesAreValidatedAgainstTheDomain(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("obj_a", "car")}
	m := mask("obj_a", 0, 0)
	m.UncertainIndices = []int{99}
	s.Masks = []FrameMask{m}
	if err := s.Validate(p); err == nil {
		t.Fatal("an uncertain index outside the sample validated")
	}
}

func TestIntersectsWalksBothSets(t *testing.T) {
	if got := intersects([]int{1, 5, 9}, []int{2, 6, 10}); got != -1 {
		t.Fatalf("disjoint sets reported %d", got)
	}
	if got := intersects([]int{1, 5, 9}, []int{9}); got != 9 {
		t.Fatalf("shared tail reported %d", got)
	}
	if got := intersects(nil, []int{1}); got != -1 {
		t.Fatalf("empty set reported %d", got)
	}
}

func TestCanonicaliseDropsEmptyUncertainSets(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("obj_a", "car")}
	m := mask("obj_a", 0, 0)
	m.UncertainIndices = []int{}
	s.Masks = []FrameMask{m}
	s.Canonicalise()
	if s.Masks[0].UncertainIndices != nil {
		t.Fatal("an empty uncertain set survived canonicalisation and would show up in diffs")
	}
}

func TestTrimTrailingNewline(t *testing.T) {
	if got := string(trimTrailingNewline([]byte("a\n"))); got != "a" {
		t.Fatalf("got %q", got)
	}
	if got := string(trimTrailingNewline([]byte("a"))); got != "a" {
		t.Fatalf("got %q", got)
	}
	if got := string(trimTrailingNewline(nil)); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestExportRejectsMissingArgumentsAndSources(t *testing.T) {
	if _, err := Export(ExportConfig{Coverage: CoverageFull}); err == nil {
		t.Fatal("exported with no paths")
	}
	_, err := Export(ExportConfig{
		VRLOGPath: filepath.Join(t.TempDir(), "absent"),
		OutDir:    filepath.Join(t.TempDir(), "pack"), Coverage: CoverageFull,
	})
	if err == nil {
		t.Fatal("exported from a VRLOG that is not there")
	}
}

func TestExportRefusesAnExistingOutput(t *testing.T) {
	src := writeVRLOG(t, true, 100, 200)
	out := filepath.Join(t.TempDir(), "pack")
	if _, err := Export(ExportConfig{VRLOGPath: src, OutDir: out, Coverage: CoverageFull}); err != nil {
		t.Fatalf("first export: %v", err)
	}
	if _, err := Export(ExportConfig{VRLOGPath: src, OutDir: out, Coverage: CoverageFull}); err == nil {
		t.Fatal("a second export overwrote the first")
	}
}

func TestExportWindowExcludingEverythingFails(t *testing.T) {
	src := writeVRLOG(t, true, 100, 200)
	_, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "pack"),
		Coverage: CoverageFull, StartNs: 10_000, EndNs: 20_000,
	})
	if err == nil {
		t.Fatal("an empty window exported successfully")
	}
}

func TestFrameIDFallsBackWithoutAPointCloud(t *testing.T) {
	if got := frameID(&l9endpoints.FrameBundle{}); got != 0 {
		t.Fatalf("frame id = %d, want 0", got)
	}
}

func TestExportRefusesUndigestibleSources(t *testing.T) {
	// A directory with no header, then one with a header but no frames: both
	// are sources whose identity cannot be recorded, and a pack without
	// provenance is not worth exporting.
	empty := t.TempDir()
	if _, err := Export(ExportConfig{
		VRLOGPath: empty, OutDir: filepath.Join(t.TempDir(), "p"), Coverage: CoverageFull,
	}); err == nil {
		t.Fatal("exported from a directory with no header")
	}

	headerOnly := t.TempDir()
	if err := os.WriteFile(filepath.Join(headerOnly, "header.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(ExportConfig{
		VRLOGPath: headerOnly, OutDir: filepath.Join(t.TempDir(), "p"), Coverage: CoverageFull,
	}); err == nil {
		t.Fatal("exported from a source with no frames directory")
	}
}

func TestWritePackReportsAnUnusableDestination(t *testing.T) {
	// A destination whose parent is a file cannot hold a staging directory.
	base := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(base, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	block, _ := encodePoints(Points{X: []float32{1}, Y: []float32{1}, Z: []float32{1}})
	err := WritePack(filepath.Join(base, "pack"), Manifest{Coverage: CoverageFull},
		[]Sample{{PointCount: 1}}, [][]byte{block})
	if err == nil {
		t.Fatal("wrote a pack beneath a regular file")
	}
}

func TestCanonicaliseOrdersMasksWithinASample(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("obj_b", "car"), reviewedObject("obj_a", "car")}
	// Same sample, different objects: ordering must fall through to the
	// object id, which is the branch a single-sample document never reaches.
	s.Masks = []FrameMask{mask("obj_b", 0, 2), mask("obj_a", 0, 0)}
	s.Canonicalise()
	if s.Masks[0].ObjectID != "obj_a" || s.Masks[1].ObjectID != "obj_b" {
		t.Fatalf("masks within one sample were not ordered by object: %+v", s.Masks)
	}
}

// skipIfRoot guards the permission-based cases: root ignores the mode bits
// they rely on, so the test would assert nothing rather than fail honestly.
func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits do not apply")
	}
}

func TestExportRejectsAnUnreadableSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "header.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "frames"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(ExportConfig{
		VRLOGPath: dir, OutDir: filepath.Join(t.TempDir(), "p"), Coverage: CoverageFull,
	}); err == nil {
		t.Fatal("exported from a recording whose header does not parse")
	}
}

func TestWritePackReportsAnUncreatableStagingDirectory(t *testing.T) {
	skipIfRoot(t)
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	block, _ := encodePoints(Points{X: []float32{1}, Y: []float32{1}, Z: []float32{1}})
	err := WritePack(filepath.Join(parent, "pack"), Manifest{Coverage: CoverageFull},
		[]Sample{{PointCount: 1}}, [][]byte{block})
	if err == nil {
		t.Fatal("wrote a pack into a read-only parent")
	}
}

func TestDirDigestReportsAnUnreadableFile(t *testing.T) {
	skipIfRoot(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "chunk")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	if _, err := dirSHA256(dir); err == nil {
		t.Fatal("digested a directory holding a file it could not read")
	}
}

func TestSidecarReportsAFailedCommit(t *testing.T) {
	p := synthPack(t)
	// A non-empty directory where the sidecar belongs cannot be replaced by a
	// rename, so the commit step fails after the temporary file is written.
	target := filepath.Join(p.Dir, sidecarFile)
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveSidecar(p, NewSidecar(p)); err == nil {
		t.Fatal("a sidecar reported success without committing")
	}
}

func TestCanonicaliseOrdersCorrespondences(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("obj_a", "car"), reviewedObject("obj_b", "car")}
	s.Correspondences = []TrackCorrespondence{{ObjectID: "obj_b"}, {ObjectID: "obj_a"}}
	s.Canonicalise()
	if s.Correspondences[0].ObjectID != "obj_a" {
		t.Fatalf("correspondences not ordered: %+v", s.Correspondences)
	}
}

func TestExportReportsACorruptFrameStream(t *testing.T) {
	// Replacing a chunk wholesale is caught when the recording is opened.
	src := writeVRLOG(t, true, 100, 200, 300)
	entries, err := os.ReadDir(filepath.Join(src, "frames"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("no frame chunks to damage: %v", err)
	}
	chunk := filepath.Join(src, "frames", entries[0].Name())
	if err := os.WriteFile(chunk, []byte("garbage that is not a frame"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(ExportConfig{
		VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "p"), Coverage: CoverageFull,
	}); err == nil {
		t.Fatal("exported a pack from an unreadable frame stream")
	}
}

// A recording cut off mid-frame is the dangerous case, because a pack built
// from the frames that did parse would look complete. Export must stop rather
// than quietly annotate half a window.
func TestExportRefusesATruncatedRecording(t *testing.T) {
	src := writeVRLOG(t, true, 100, 200, 300, 400, 500)
	chunk := filepath.Join(src, "frames", "chunk_0000.pb")
	b, err := os.ReadFile(chunk)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chunk, b[:len(b)*7/10], 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Export(ExportConfig{
		VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "p"), Coverage: CoverageFull,
	})
	if err == nil {
		t.Fatal("a truncated recording exported as though it were whole")
	}
	if !strings.Contains(err.Error(), "read frame") {
		t.Fatalf("error does not identify where the stream stopped: %v", err)
	}
}
