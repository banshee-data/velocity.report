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

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

func refFloat(v float64) *float64 { return &v }

// referencePack writes a two-sample pack in metres with one reviewed car,
// and returns it with a valid import file for that car.
func referencePack(t *testing.T) (*annotation.Pack, string) {
	t.Helper()
	pts := annotation.Points{X: []float32{8, 12}, Y: []float32{0, 0}, Z: []float32{0.5, 0.5}}
	block, err := annotation.EncodePoints(pts)
	if err != nil {
		t.Fatal(err)
	}
	samples := []annotation.Sample{
		{SourceOrdinal: 0, TimestampNs: 1_000_000_000, SensorID: "s", PointCount: 2},
		{SourceOrdinal: 1, TimestampNs: 1_100_000_000, SensorID: "s", PointCount: 2},
	}
	dir := filepath.Join(t.TempDir(), "pack")
	m := annotation.Manifest{
		Coverage:   annotation.CoverageForegroundOnly,
		Source:     annotation.SourceProvenance{SensorID: "s", VRLOGHeaderSHA: "sha256:h", VRLOGFramesSHA: "sha256:f"},
		Coordinate: annotation.CoordinateContract{Units: "metres", FrameID: "sensor", ReferenceFrame: "sensor"},
	}
	if err := annotation.WritePack(dir, m, samples, [][]byte{block, block}); err != nil {
		t.Fatal(err)
	}
	pack, err := annotation.OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := annotation.NewSidecar(pack)
	s.Change = annotation.Provenance{Author: "op"}
	s.Objects = []annotation.Object{{ObjectID: "car", Class: "car", Confidence: 1, Status: annotation.StatusReviewed}}
	for i := 0; i < 2; i++ {
		s.Masks = append(s.Masks, annotation.FrameMask{ObjectID: "car", SampleID: i, PointIndices: []int{0, 1},
			Completeness: annotation.MaskComplete, Visibility: annotation.VisiblePresent, Status: annotation.StatusReviewed})
	}
	if err := annotation.SaveSidecar(pack, s); err != nil {
		t.Fatal(err)
	}
	review := annotation.PhysicalReview{
		Status: annotation.StatusReviewed, Origin: annotation.OriginIndependent, Method: "surveyed",
		UncertaintyAssumptions: "hard bounds", Provenance: annotation.Provenance{Author: "surveyor"},
	}
	imp := annotation.PhysicalReferenceImport{
		Schema: annotation.PhysicalImportSchema, SchemaVersion: annotation.PhysicalImportSchemaVersion,
		PackDigest: pack.Manifest.PackDigest, DatasetID: pack.Manifest.DatasetID,
		Source: annotation.PackPhysicalSource(pack),
		Objects: []annotation.PhysicalObject{{
			ObjectID: "car",
			Body: &annotation.BodyGeometry{
				BodyID: "body-car", AxisConvention: annotation.BodyAxisConvention,
				Length: annotation.DimensionBound{Status: annotation.EvidenceInferred, Span: annotation.SpanFull,
					LowerM: refFloat(4.4), UpperM: refFloat(4.6), Support: annotation.EvidenceSupport{External: "registration record"}},
				Width:  annotation.DimensionBound{Status: annotation.EvidenceUnknown},
				Height: annotation.DimensionBound{Status: annotation.EvidenceUnknown},
				Review: review,
			},
			Keyframes: []annotation.PhysicalKeyframe{{
				KeyframeID: "kf-0", SampleID: 0, TimestampNs: 1_000_000_000,
				Anchor: annotation.PhysicalAnchor{Kind: annotation.AnchorBodyCentre},
				Position: annotation.PositionBound{Status: annotation.EvidenceObserved, XM: refFloat(10), YM: refFloat(0),
					BoundM: refFloat(0.2), Support: annotation.EvidenceSupport{Frames: []int{0}}},
				Yaw:    annotation.YawBound{Status: annotation.EvidenceUnknown, Axis: annotation.AxisUnknown},
				Front:  annotation.EndpointEvidence{Status: annotation.EvidenceUnknown},
				Rear:   annotation.EndpointEvidence{Status: annotation.EvidenceUnknown},
				Review: review,
			}},
		}},
	}
	b, err := json.MarshalIndent(imp, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "import.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return pack, path
}

func runReference(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := annotationReferenceMain(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestAnnotationReferenceRouting(t *testing.T) {
	if code := silence(t, func() int { return Main([]string{"annotation-reference", "help"}) }); code != 0 {
		t.Errorf("help exited %d", code)
	}
	if code := silence(t, func() int { return AnnotationReferenceMain(nil) }); code != 2 {
		t.Errorf("no command exited %d", code)
	}
	if code, _, stderr := runReference("export"); code != 2 || !strings.Contains(stderr, "unknown annotation-reference command") {
		t.Errorf("unknown command exited %d: %s", code, stderr)
	}
	for _, args := range [][]string{{"import", "-h"}, {"validate", "-h"}} {
		if code, _, _ := runReference(args...); code != 0 {
			t.Errorf("%v exited %d", args, code)
		}
	}
	for _, args := range [][]string{
		{"import", "--nope"}, {"validate", "--nope"},
		{"import", "--pack", "p", "--file", "f"}, {"import", "--pack", "p", "--author", "a"},
		{"validate"}, {"validate", "--pack", "p", "--file", "f", "--revision", "1"},
		{"validate", "--pack", "p", "extra"},
	} {
		if code, _, _ := runReference(args...); code != 2 {
			t.Errorf("%v exited %d, want 2", args, code)
		}
	}
}

func TestAnnotationReferenceImportAndValidate(t *testing.T) {
	pack, file := referencePack(t)

	code, stdout, stderr := runReference("validate", "--pack", pack.Dir, "--file", file)
	if code != 0 || !strings.Contains(stdout, "valid: pack "+pack.Manifest.PackDigest) || !strings.Contains(stdout, "content digest sha256:") {
		t.Fatalf("validate file exited %d: %s%s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(pack.Dir, "physical-references.json")); !os.IsNotExist(err) {
		t.Fatal("validating an import wrote references")
	}

	code, stdout, stderr = runReference("import", "--pack", pack.Dir, "--file", file, "--author", "surveyor", "--session", "s1")
	if code != 0 || !strings.Contains(stdout, "imported: pack") || !strings.Contains(stdout, "revision 1") ||
		!strings.Contains(stdout, "1 objects, 1 bodies, 1 keyframes (1 reviewed and independent)") || !strings.Contains(stdout, "revision digest sha256:") {
		t.Fatalf("import exited %d: %s%s", code, stdout, stderr)
	}
	code, _, stderr = runReference("import", "--pack", pack.Dir, "--file", file, "--author", "surveyor")
	if code != 1 || !strings.Contains(stderr, "would replace existing records") {
		t.Fatalf("a repeated import exited %d: %s", code, stderr)
	}
	if code, stdout, _ = runReference("import", "--pack", pack.Dir, "--file", file, "--author", "surveyor", "--replace"); code != 0 ||
		!strings.Contains(stdout, "revision 2") {
		t.Fatalf("a replacing import exited %d: %s", code, stdout)
	}
	for _, args := range [][]string{
		{"validate", "--pack", pack.Dir},
		{"validate", "--pack", pack.Dir, "--revision", "1"},
	} {
		if code, stdout, stderr := runReference(args...); code != 0 || !strings.Contains(stdout, "valid: pack") {
			t.Fatalf("%v exited %d: %s%s", args, code, stdout, stderr)
		}
	}
	if code, _, _ := runReference("validate", "--pack", pack.Dir, "--revision", "9"); code != 1 {
		t.Fatal("a missing revision validated")
	}
}

func TestAnnotationReferenceRefusals(t *testing.T) {
	pack, file := referencePack(t)
	bad := filepath.Join(t.TempDir(), "bad.json")
	b, _ := os.ReadFile(file)
	if err := os.WriteFile(bad, bytes.Replace(b, []byte(`"sample_id": 0`), []byte(`"sample_id": 7`), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"import of a frame not in the pack":   {"import", "--pack", pack.Dir, "--file", bad, "--author", "a"},
		"validate of a frame not in the pack": {"validate", "--pack", pack.Dir, "--file", bad},
		"import of a missing file":            {"import", "--pack", pack.Dir, "--file", bad + ".missing", "--author", "a"},
		"import into a missing pack":          {"import", "--pack", pack.Dir + ".missing", "--file", file, "--author", "a"},
		"validate of a missing pack":          {"validate", "--pack", pack.Dir + ".missing"},
	} {
		if code, _, stderr := runReference(args...); code != 1 {
			t.Errorf("%s exited %d: %s", name, code, stderr)
		}
	}
	if code, _, stderr := runReference("import", "--pack", pack.Dir, "--file", bad, "--author", "a"); !strings.Contains(stderr, "sample 7 is not in the pack") {
		t.Errorf("exited %d without naming the frame: %s", code, stderr)
	}
	if code, _, _ := runReference("validate", "--pack", pack.Dir, "--replace"); code != 2 {
		t.Error("--replace without --file was accepted")
	}
}

// validate --file is a dry run of the import itself: what it passes, the
// import passes, and what the import would refuse, it refuses, writing
// nothing either way.
func TestAnnotationReferenceValidateIsADryRun(t *testing.T) {
	pack, file := referencePack(t)
	head := filepath.Join(pack.Dir, "physical-references.json")

	code, stdout, _ := runReference("validate", "--pack", pack.Dir)
	if code != 0 || !strings.Contains(stdout, "no physical references stored") || strings.Contains(stdout, "revision") {
		t.Fatalf("an empty store reported a revision: exit %d: %s", code, stdout)
	}
	code, stdout, _ = runReference("validate", "--pack", pack.Dir, "--file", file)
	if code != 0 || !strings.Contains(stdout, "(none stored); importing would save revision 1") || strings.Contains(stdout, "revision digest") {
		t.Fatalf("dry run on an empty store: exit %d: %s", code, stdout)
	}
	if code, _, stderr := runReference("import", "--pack", pack.Dir, "--file", file, "--author", "a"); code != 0 {
		t.Fatalf("import: %s", stderr)
	}
	before, err := os.ReadFile(head)
	if err != nil {
		t.Fatal(err)
	}
	// The same file again collides with what is stored: the dry run says so
	// rather than calling it valid.
	if code, _, stderr := runReference("validate", "--pack", pack.Dir, "--file", file); code != 1 || !strings.Contains(stderr, "would replace existing records") {
		t.Fatalf("dry run of a colliding import: exit %d: %s", code, stderr)
	}
	code, stdout, _ = runReference("validate", "--pack", pack.Dir, "--file", file, "--replace")
	if code != 0 || !strings.Contains(stdout, "(current revision 1); importing would save revision 2") {
		t.Fatalf("dry run with --replace: exit %d: %s", code, stdout)
	}
	if after, _ := os.ReadFile(head); !bytes.Equal(before, after) {
		t.Fatal("a dry run wrote the references")
	}

	// A later membership edit rejects the car: the references no longer hold,
	// and validate names them.
	s, err := annotation.LoadSidecar(pack)
	if err != nil {
		t.Fatal(err)
	}
	s.Objects[0].Status = annotation.StatusRejected
	s.Change.Operation = "reject"
	if err := annotation.SaveSidecar(pack, s); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runReference("validate", "--pack", pack.Dir)
	if code != 1 || !strings.Contains(stderr, "1 record(s) that do not hold against annotation revision 2") ||
		!strings.Contains(stderr, `object "car": object "car" is rejected`) {
		t.Fatalf("stale references: exit %d: %s", code, stderr)
	}
	if err := os.WriteFile(filepath.Join(pack.Dir, "annotations.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runReference("validate", "--pack", pack.Dir); code != 1 {
		t.Error("validated against a damaged sidecar")
	}
}
