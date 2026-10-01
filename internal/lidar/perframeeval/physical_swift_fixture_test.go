package perframeeval

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// The macOS annotation window opens this report in its Compare mode, so its
// decoder is held to what Run actually writes: the report below is the
// physical fixture scored by both arms, written to testdata, and
// PhysicalComparisonReportTests.swift decodes the same file. A change to the
// report's shape fails here first; rerun with -update-physical-report-fixture
// and the Swift tests say whether the reader still agrees.

var updatePhysicalReportFixture = flag.Bool("update-physical-report-fixture", false,
	"rewrite testdata/physical_report_fixture.json from the current report")

const physicalReportFixture = "testdata/physical_report_fixture.json"

func TestPhysicalReportSwiftFixture(t *testing.T) {
	f, err := evalfixture.WritePhysical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, err := Run(physConfig(f))
	if err != nil {
		t.Fatal(err)
	}
	// The exact-byte digest covers the stored document's save time, and so
	// changes run to run; the content digest does not. The fixture blanks
	// the first so its bytes are stable.
	for _, ref := range []*PhysicalReferenceIdentity{&c.Physical.Reference, &c.Physical.A.Reference, &c.Physical.B.Reference} {
		ref.PhysicalRevisionDigest = "sha256:fixture-revision-bytes"
		ref.Digest = "sha256:fixture-reference-identity"
	}
	want, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if bytes.Contains(want, []byte(os.TempDir())) || bytes.Contains(want, []byte(filepath.Dir(f.SplitManifestPath))) {
		t.Fatal("the report names a temporary path; the fixture would not be stable")
	}
	if *updatePhysicalReportFixture {
		if err := os.MkdirAll(filepath.Dir(physicalReportFixture), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(physicalReportFixture, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(physicalReportFixture)
	if err != nil {
		t.Fatalf("read %s (run with -update-physical-report-fixture to create it): %v", physicalReportFixture, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is stale: rerun with -update-physical-report-fixture and check the Swift report tests", physicalReportFixture)
	}
}
