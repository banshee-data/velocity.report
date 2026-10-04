package annotation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// synthPack writes a small pack directly, so the format's guarantees can be
// tested without a recording. Point counts differ per sample on purpose:
// a uniform pack would hide offset arithmetic errors.
func synthPack(t *testing.T, counts ...int) *Pack {
	t.Helper()
	if len(counts) == 0 {
		counts = []int{4, 7, 1}
	}
	dir := filepath.Join(t.TempDir(), "pack")

	var samples []Sample
	var blocks [][]byte
	for i, n := range counts {
		p := Points{
			X: make([]float32, n), Y: make([]float32, n), Z: make([]float32, n),
			Intensity: make([]uint8, n), Classification: make([]uint8, n),
		}
		for j := 0; j < n; j++ {
			// Two coincident points in every sample: identical coordinates
			// must remain distinct members, which is why membership is by
			// index and never by coordinate matching.
			if j < 2 {
				p.X[j], p.Y[j], p.Z[j] = 1, 2, 3
			} else {
				p.X[j], p.Y[j], p.Z[j] = float32(j), float32(i), 0.5
			}
			p.Intensity[j] = uint8(j)
			p.Classification[j] = uint8(j % 3)
		}
		block, err := encodePoints(p)
		if err != nil {
			t.Fatalf("encode sample %d: %v", i, err)
		}
		samples = append(samples, Sample{
			SourceOrdinal: i, SourceFrameID: uint64(100 + i),
			TimestampNs: int64(1_000_000_000 * (i + 1)), SensorID: "synthetic", PointCount: n,
		})
		blocks = append(blocks, block)
	}

	m := Manifest{Coverage: CoverageFull, Source: SourceProvenance{SensorID: "synthetic"}}
	if err := WritePack(dir, m, samples, blocks); err != nil {
		t.Fatalf("write pack: %v", err)
	}
	p, err := OpenPack(dir)
	if err != nil {
		t.Fatalf("open pack: %v", err)
	}
	return p
}

// Gate: round-trip membership is exact, including coincident points, empty
// masks and boundary indices.
func TestPackRoundTripsPointsAndMembership(t *testing.T) {
	p := synthPack(t)

	first, err := p.PointsAt(0)
	if err != nil {
		t.Fatalf("read sample 0: %v", err)
	}
	if first.X[0] != first.X[1] || first.Y[0] != first.Y[1] {
		t.Fatal("coincident points did not survive the round trip")
	}
	if len(first.X) != 4 || first.X[3] != 3 || first.Classification[3] != 0 {
		t.Fatalf("sample 0 decoded wrong: %+v", first)
	}
	last, err := p.PointsAt(2)
	if err != nil || len(last.X) != 1 {
		t.Fatalf("single-point sample decoded wrong: %v %+v", err, last)
	}

	s := NewSidecar(p)
	s.Objects = []Object{{ObjectID: "obj_a", Class: "car", Status: StatusReviewed}}
	s.Masks = []FrameMask{
		// Both coincident points plus the boundary index.
		{ObjectID: "obj_a", SampleID: 0, PointIndices: []int{0, 1, 3},
			Completeness: MaskComplete, Visibility: VisiblePresent, Status: StatusReviewed},
		// An empty mask on a frame where the object was fully occluded is a
		// statement, not an absence.
		{ObjectID: "obj_a", SampleID: 1, PointIndices: []int{},
			Completeness: MaskComplete, Visibility: VisibleFullyOccluded, Status: StatusReviewed},
	}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded, err := LoadSidecar(p)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Masks) != 2 {
		t.Fatalf("reloaded %d masks, want 2", len(reloaded.Masks))
	}
	got := reloaded.Masks[0]
	if len(got.PointIndices) != 3 || got.PointIndices[0] != 0 || got.PointIndices[2] != 3 {
		t.Fatalf("membership changed across the round trip: %v", got.PointIndices)
	}
	if len(reloaded.Masks[1].PointIndices) != 0 || reloaded.Masks[1].Visibility != VisibleFullyOccluded {
		t.Fatalf("empty mask lost its meaning: %+v", reloaded.Masks[1])
	}
}

// Gate: source mismatch fails explicitly.
func TestPackFailsClosedOnTamperedContent(t *testing.T) {
	for _, file := range []string{pointsFile, samplesFile} {
		p := synthPack(t)
		path := filepath.Join(p.Dir, file)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		b[len(b)/2]++ // one byte
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenPack(p.Dir); err == nil {
			t.Fatalf("a tampered %s opened without complaint", file)
		}
	}
}

func TestSidecarRejectsADifferentPack(t *testing.T) {
	a, b := synthPack(t), synthPack(t, 5, 5)

	s := NewSidecar(a)
	s.Objects = []Object{{ObjectID: "obj_a", Class: "car", Status: StatusReviewed}}
	if err := SaveSidecar(a, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(a.Dir, sidecarFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.Dir, sidecarFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSidecar(b); err == nil {
		t.Fatal("a sidecar from another pack loaded against this one")
	}
}

// Malformed arrays and impossible references fail rather than being coerced.
func TestPackRejectsMalformedInput(t *testing.T) {
	if _, err := encodePoints(Points{X: []float32{1, 2}, Y: []float32{1}, Z: []float32{1}}); err == nil {
		t.Fatal("mismatched array lengths encoded")
	}
	nan := float32(0)
	nan = nan / nan // NaN without importing math into the test
	if _, err := encodePoints(Points{X: []float32{nan}, Y: []float32{0}, Z: []float32{0}}); err == nil {
		t.Fatal("a non-finite coordinate encoded")
	}

	p := synthPack(t)
	for _, bad := range [][]int{{-1}, {4}, {1, 1}, {2, 1}} {
		if err := p.ValidateIndices(0, bad); err == nil {
			t.Fatalf("index set %v was accepted for a 4-point sample", bad)
		}
	}
	if err := p.ValidateIndices(99, []int{0}); err == nil {
		t.Fatal("an out-of-range sample was accepted")
	}
}

func TestWritePackRefusesToOverwrite(t *testing.T) {
	p := synthPack(t)
	m := Manifest{Coverage: CoverageFull}
	block, _ := encodePoints(Points{X: []float32{1}, Y: []float32{1}, Z: []float32{1}})
	err := WritePack(p.Dir, m, []Sample{{PointCount: 1}}, [][]byte{block})
	if err == nil {
		t.Fatal("overwrote an existing pack; a changed point domain must be a new pack")
	}
}

func TestWritePackRefusesEmpty(t *testing.T) {
	if err := WritePack(filepath.Join(t.TempDir(), "p"), Manifest{}, nil, nil); err == nil {
		t.Fatal("wrote an empty pack")
	}
}

// Gate: a limitation of the source survives every export.
func TestCoverageSurvivesTheRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pack")
	block, _ := encodePoints(Points{X: []float32{1}, Y: []float32{2}, Z: []float32{3}})
	m := Manifest{
		Coverage:     CoverageForegroundOnly,
		CoverageNote: "recording dropped background; export applied no further filter",
	}
	if err := WritePack(dir, m, []Sample{{PointCount: 1}}, [][]byte{block}); err != nil {
		t.Fatal(err)
	}
	p, err := OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Manifest.Coverage != CoverageForegroundOnly || p.Manifest.CoverageNote == "" {
		t.Fatalf("coverage was lost: %+v", p.Manifest)
	}
}

func TestCanonicalIndicesHasOneEncoding(t *testing.T) {
	a := CanonicalIndices([]int{5, 1, 3, 1, 5})
	b := CanonicalIndices([]int{1, 3, 5})
	if len(a) != 3 || a[0] != 1 || a[1] != 3 || a[2] != 5 {
		t.Fatalf("canonical form = %v", a)
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatalf("the same membership set encoded two ways: %s vs %s", ja, jb)
	}
	if got := CanonicalIndices(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty set = %v, want an empty non-nil slice", got)
	}
}

// A pack written before samples recorded their own presence has only the
// manifest's word, and the reader must say so rather than invent a flag.
func TestLegacyPackFallsBackToTheManifestIntensityFlag(t *testing.T) {
	for _, packWide := range []bool{true, false} {
		dir := filepath.Join(t.TempDir(), "pack")
		block, _ := encodePoints(Points{X: []float32{1}, Y: []float32{2}, Z: []float32{3}})
		m := Manifest{Coverage: CoverageFull, HasIntensity: packWide}
		if err := WritePack(dir, m, []Sample{{PointCount: 1}}, [][]byte{block}); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, samplesFile))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "has_intensity") {
			t.Fatalf("a sample with no recorded presence wrote the key anyway:\n%s", raw)
		}
		p, err := OpenPack(dir)
		if err != nil {
			t.Fatal(err)
		}
		if p.IntensityPresenceIsPerSample() {
			t.Error("a legacy pack claimed per-sample presence")
		}
		if got := p.SampleHasIntensity(0); got != packWide {
			t.Errorf("manifest %v: sample 0 has intensity = %v", packWide, got)
		}
		if p.SampleHasIntensity(1) || p.SampleHasIntensity(-1) {
			t.Error("a sample outside the pack reported intensity")
		}
	}
}

func TestWritePackRoundTripsPerSampleIntensityPresence(t *testing.T) {
	yes, no := true, false
	dir := filepath.Join(t.TempDir(), "pack")
	block, _ := encodePoints(Points{X: []float32{1}, Y: []float32{2}, Z: []float32{3}})
	samples := []Sample{
		{PointCount: 1, HasIntensity: &yes},
		{PointCount: 1, HasIntensity: &no},
		{PointCount: 1}, // recorded by nothing; the manifest speaks for it
	}
	m := Manifest{Coverage: CoverageFull, HasIntensity: true}
	if err := WritePack(dir, m, samples, [][]byte{block, block, block}); err != nil {
		t.Fatal(err)
	}
	p, err := OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	for sampleID, want := range []bool{true, false, true} {
		if got := p.SampleHasIntensity(sampleID); got != want {
			t.Errorf("sample %d has intensity = %v, want %v", sampleID, got, want)
		}
	}
	if p.IntensityPresenceIsPerSample() {
		t.Error("one sample recorded nothing, yet the pack claims per-sample presence")
	}
	if SamplesRecordIntensityPresence(nil) {
		t.Error("no samples at all record their presence")
	}
	if !SamplesRecordIntensityPresence(p.Samples[:2]) {
		t.Error("two samples with flags do not count as recorded")
	}
}

// The manifest says "some sample was measured". When every sample says for
// itself, the two cannot disagree, and neither writer nor reader lets them.
func TestPackRefusesAContradictoryIntensityFlag(t *testing.T) {
	yes, no := true, false
	block, _ := encodePoints(Points{X: []float32{1}, Y: []float32{2}, Z: []float32{3}})
	for _, tc := range []struct {
		name     string
		samples  []Sample
		packWide bool
	}{
		{"manifest true over samples all false", []Sample{{PointCount: 1, HasIntensity: &no}}, true},
		{"manifest false over a sample that is true", []Sample{{PointCount: 1, HasIntensity: &yes}, {PointCount: 1, HasIntensity: &no}}, false},
	} {
		blocks := make([][]byte, len(tc.samples))
		for i := range blocks {
			blocks[i] = block
		}
		err := WritePack(filepath.Join(t.TempDir(), "pack"), Manifest{HasIntensity: tc.packWide}, tc.samples, blocks)
		if err == nil || !strings.Contains(err.Error(), "has_intensity") {
			t.Errorf("%s: written, or refused for another reason: %v", tc.name, err)
		}
	}

	// The manifest sits outside the pack digest, so flipping its flag after
	// the fact passes every digest check. The reader has to look for itself.
	dir := filepath.Join(t.TempDir(), "pack")
	if err := WritePack(dir, Manifest{HasIntensity: true},
		[]Sample{{PointCount: 1, HasIntensity: &yes}}, [][]byte{block}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, manifestFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), `"has_intensity": true`, `"has_intensity": false`, 1)
	if edited == string(raw) {
		t.Fatal("fixture manifest did not carry the flag to flip")
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenPack(dir); err == nil || !strings.Contains(err.Error(), "has_intensity") {
		t.Errorf("opened a pack whose manifest denies the intensity its samples record: %v", err)
	}
}
