package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/labelworkdb"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

type evidenceFailureFixture struct {
	args                        []string
	capture, database, output   string
	bindingPath, annotationRoot string
}

func newEvidenceFailureFixture(t *testing.T) evidenceFailureFixture {
	t.Helper()
	dir := t.TempDir()
	f := evidenceFailureFixture{capture: filepath.Join(dir, "capture.pcap"), database: seedHeadwayEvidence(t), output: filepath.Join(dir, "export"), bindingPath: filepath.Join(dir, "binding.json"), annotationRoot: filepath.Join(dir, "annotations")}
	if err := os.WriteFile(f.capture, []byte("independent failure-contract fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := eligibility.NewHasher().Capture(f.capture)
	if err != nil {
		t.Fatal(err)
	}
	source := l4bobserve.CaptureSource{ClipID: "failure-test", CapturePaths: []string{f.capture}, CaptureSHA256s: []string{strings.TrimPrefix(id.SHA256, "sha256:")}, ExtractorID: "failure-test-extractor"}
	sourceID, err := l4bobserve.SourceID(source)
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.NewDB(f.database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	for _, table := range []string{"lidar_track_estimates", "lidar_track_solid_bodies", "lidar_observations"} {
		if _, err := database.Exec("UPDATE "+table+" SET source_id=? WHERE source_id=?", sourceID, headwayTestSource); err != nil {
			t.Fatal(err)
		}
	}
	manifest := eligibility.Manifest{Schema: "velocity.report/cohort-manifest", SchemaVersion: 1, Revision: 1, FrozenUTC: "2026-11-14T00:00:00Z", Sites: []eligibility.ManifestSite{{SiteID: "failure-site", Cohort: eligibility.CohortDevelopment, Rationale: "authored fixture"}}, Captures: []eligibility.CaptureEntry{{CaptureID: "failure-capture", SiteID: "failure-site", RawSHA256: id.SHA256, Basename: filepath.Base(f.capture)}}}
	manifestPath := filepath.Join(dir, "cohort.json")
	write := func(path string, v any) {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(manifestPath, manifest)
	if err := os.MkdirAll(filepath.Join(f.annotationRoot, "splits"), 0700); err != nil {
		t.Fatal(err)
	}
	policy := eligibility.New(eligibility.Config{ManifestPath: manifestPath, SplitsDir: filepath.Join(f.annotationRoot, "splits"), Ledger: labelworkdb.UseLedger{DB: database.DB}}).Check(eligibility.Request{Purpose: eligibility.PurposeDeveloperDiagnostic, Inputs: []eligibility.Input{{Identity: id}}}).PolicyDigest
	write(f.bindingPath, followingEvidenceBinding{Context: fieldrun.EvidenceContext{SiteNamespace: "test", SiteID: "failure-site", SourceID: sourceID, BindingID: "reviewed-failure-fixture", PolicyID: policy}, Source: source})
	f.args = []string{"--db", f.database, "--source", sourceID, "--solid-bodies", "--stage", "online", "--evidence-context", f.bindingPath, "--output", f.output, "--cohort-manifest", manifestPath, "--annotation-root", f.annotationRoot, "--eligibility-db", f.database}
	return f
}

func TestHeadwayEvidenceFailureContracts(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*testing.T, *evidenceFailureFixture)
		want string
	}{
		{"capture disappeared", func(t *testing.T, f *evidenceFailureFixture) {
			if err := os.Remove(f.capture); err != nil {
				t.Fatal(err)
			}
		}, "no such file"},
		{"source database disappeared", func(t *testing.T, f *evidenceFailureFixture) {
			f.args[1] = filepath.Join(t.TempDir(), "missing.db")
		}, "unable to open"},
		{"source is not a database", func(t *testing.T, f *evidenceFailureFixture) {
			p := filepath.Join(t.TempDir(), "corrupt.db")
			if err := os.WriteFile(p, []byte("not sqlite"), 0600); err != nil {
				t.Fatal(err)
			}
			f.args[1] = p
		}, "not a database"},
		{"incomplete binding", func(t *testing.T, f *evidenceFailureFixture) {
			b, err := os.ReadFile(f.bindingPath)
			if err != nil {
				t.Fatal(err)
			}
			var binding followingEvidenceBinding
			json.Unmarshal(b, &binding)
			binding.Context.BindingID = ""
			b, _ = json.Marshal(binding)
			os.WriteFile(f.bindingPath, b, 0600)
		}, "explicit site namespace"},
		{"corrupt sample evidence", func(t *testing.T, f *evidenceFailureFixture) {
			d, err := db.NewDB(f.database)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if _, err = d.Exec("UPDATE lidar_track_solid_bodies SET covariance_json='corrupt'"); err != nil {
				t.Fatal(err)
			}
		}, "invalid character"},
		{"unavailable estimate version", func(t *testing.T, f *evidenceFailureFixture) {
			f.args = append(f.args, "--estimator", "unavailable-estimator")
		}, "no online"},
		{"invalid annotation registry", func(t *testing.T, f *evidenceFailureFixture) {
			if err := os.WriteFile(filepath.Join(f.annotationRoot, "splits", "broken.json"), []byte("{"), 0600); err != nil {
				t.Fatal(err)
			}
		}, "not eligible"},
		{"output is a regular file", func(t *testing.T, f *evidenceFailureFixture) {
			if err := os.WriteFile(f.output, []byte("keep output"), 0600); err != nil {
				t.Fatal(err)
			}
		}, "not a directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEvidenceFailureFixture(t)
			tc.edit(t, &f)
			var out, stderr bytes.Buffer
			code := runHeadway(f.args, &out, &stderr)
			if code != 1 || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("got %d %q; want error containing %q", code, stderr.String(), tc.want)
			}
			if out.Len() != 0 {
				t.Fatalf("reported success: %s", out.String())
			}
		})
	}
}

type brokenEvidenceOutput struct{}

func (brokenEvidenceOutput) Write([]byte) (int, error) { return 0, errors.New("report pipe closed") }

func TestHeadwayEvidenceRetainsFileWhenSuccessOutputFails(t *testing.T) {
	f := newEvidenceFailureFixture(t)
	var stderr bytes.Buffer
	if code := runHeadway(f.args, brokenEvidenceOutput{}, &stderr); code != 1 || !strings.Contains(stderr.String(), "report pipe closed") {
		t.Fatalf("got %d %s", code, stderr.String())
	}
	files, err := filepath.Glob(filepath.Join(f.output, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("lost completed export: %v %v", files, err)
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var d fieldrun.EvidenceDocument
	if err = json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	if err = d.Validate(); err != nil {
		t.Fatalf("completed file became invalid: %v", err)
	}
}
