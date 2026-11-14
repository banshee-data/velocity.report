package server

import (
	"bytes"
	"encoding/json"
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

func TestHeadwayEvidenceCLI(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "capture.pcap")
	os.WriteFile(capture, []byte("authored capture fixture"), 0600)
	id, err := eligibility.NewHasher().Capture(capture)
	if err != nil {
		t.Fatal(err)
	}
	source := l4bobserve.CaptureSource{ClipID: "test", CapturePaths: []string{capture}, CaptureSHA256s: []string{strings.TrimPrefix(id.SHA256, "sha256:")}, ExtractorID: "test-extractor"}
	sourceID, err := l4bobserve.SourceID(source)
	if err != nil {
		t.Fatal(err)
	}
	path := seedHeadwayEvidence(t)
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, table := range []string{"lidar_track_estimates", "lidar_track_solid_bodies", "lidar_observations"} {
		if _, err = database.Exec("UPDATE "+table+" SET source_id = ? WHERE source_id = ?", sourceID, headwayTestSource); err != nil {
			t.Fatal(err)
		}
	}
	manifest := eligibility.Manifest{Schema: "velocity.report/cohort-manifest", SchemaVersion: 1, Revision: 1, FrozenUTC: "2026-11-14T00:00:00Z", Sites: []eligibility.ManifestSite{{SiteID: "fixture-site", Cohort: eligibility.CohortDevelopment, Rationale: "authored development fixture"}}, Captures: []eligibility.CaptureEntry{{CaptureID: "fixture", SiteID: "fixture-site", RawSHA256: id.SHA256, Basename: filepath.Base(capture)}}}
	manifestPath := filepath.Join(dir, "cohorts.json")
	b, _ := json.Marshal(manifest)
	os.WriteFile(manifestPath, b, 0600)
	root := filepath.Join(dir, "annotations")
	os.MkdirAll(filepath.Join(root, "splits"), 0700)
	policy := eligibility.New(eligibility.Config{ManifestPath: manifestPath, SplitsDir: filepath.Join(root, "splits"), Ledger: labelworkdb.UseLedger{DB: database.DB}}).Check(eligibility.Request{Purpose: eligibility.PurposeDeveloperDiagnostic, Inputs: []eligibility.Input{{Identity: id}}}).PolicyDigest
	binding := followingEvidenceBinding{Context: fieldrun.EvidenceContext{SiteNamespace: "test", SiteID: "fixture-site", SourceID: sourceID, BindingID: "reviewed-fixture", PolicyID: policy}, Source: source}
	bindingPath := filepath.Join(dir, "binding.json")
	writeBinding := func() {
		b, _ := json.Marshal(binding)
		if err := os.WriteFile(bindingPath, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeBinding()
	out := filepath.Join(dir, "export")
	args := []string{"--db", path, "--source", sourceID, "--solid-bodies", "--stage", "online", "--evidence-context", bindingPath, "--output", out, "--cohort-manifest", manifestPath, "--annotation-root", root, "--eligibility-db", path}
	call := func() (int, string) {
		var stdout, stderr bytes.Buffer
		code := runHeadway(args, &stdout, &stderr)
		return code, stderr.String()
	}
	if code, msg := call(); code != 0 {
		t.Fatalf("CLI %d: %s", code, msg)
	}
	files, _ := filepath.Glob(filepath.Join(out, "*.json"))
	if len(files) != 1 {
		t.Fatal("missing exported document")
	}
	first, _ := os.ReadFile(files[0])
	if code, msg := call(); code != 0 {
		t.Fatalf("repeat %d: %s", code, msg)
	}
	second, _ := os.ReadFile(files[0])
	if !bytes.Equal(first, second) {
		t.Fatal("repeat changed output")
	}
	var doc fieldrun.EvidenceDocument
	if err = json.Unmarshal(first, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Context.SourceManifest == nil || doc.Context.SourceManifest.ExtractorID != source.ExtractorID || len(doc.Rows) == 0 {
		t.Fatal("lost inputs or rows")
	}
	var n int
	database.QueryRow("SELECT count(*) FROM lidar_interaction_events").Scan(&n)
	if n != 0 {
		t.Fatal("export mutated interactions")
	}
	alias := filepath.Join(dir, "alias.pcap")
	original, _ := os.ReadFile(capture)
	os.WriteFile(alias, original, 0600)
	binding.Context.CaptureFiles = []string{alias}
	writeBinding()
	if code, msg := call(); code == 0 || !strings.Contains(msg, "conflicting evidence") {
		t.Fatalf("alias duplicated evidence: %d %s", code, msg)
	}
	binding.Context.CaptureFiles = []string{alias, alias}
	writeBinding()
	if code, msg := call(); code == 0 || !strings.Contains(msg, "capture_files") {
		t.Fatalf("bad location mapping accepted: %d %s", code, msg)
	}
	binding.Context.CaptureFiles = nil
	for _, c := range []struct {
		name, want string
		change     func()
	}{
		{"site", "site binding", func() { binding.Context.SiteID = "other" }},
		{"policy", "binding policy", func() { binding.Context.SiteID = "fixture-site"; binding.Context.PolicyID = "wrong" }},
		{"source", "capture identity", func() { binding.Context.SourceID = "wrong" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.change()
			writeBinding()
			code, msg := call()
			if code == 0 || !strings.Contains(msg, c.want) {
				t.Fatalf("got %d %s", code, msg)
			}
		})
	}
	binding.Context.SourceID = sourceID
	binding.Context.SiteID = "fixture-site"
	binding.Context.PolicyID = policy
	writeBinding()
	manifest.Sites[0].Cohort = eligibility.CohortValidation
	b, _ = json.Marshal(manifest)
	os.WriteFile(manifestPath, b, 0600)
	if code, msg := call(); code == 0 || !strings.Contains(msg, "not eligible") {
		t.Fatalf("validation cohort admitted: %d %s", code, msg)
	}
	if err := os.WriteFile(capture, []byte("changed fixture bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, msg := call(); code == 0 || !strings.Contains(msg, "capture hash") {
		t.Fatalf("capture mismatch admitted: %d %s", code, msg)
	}
}

func TestHeadwayEvidenceRejectsMissingBinding(t *testing.T) {
	var out, errs bytes.Buffer
	if code := runHeadway([]string{"--oracle", "--evidence-context", "missing"}, &out, &errs); code != 2 {
		t.Fatal(code)
	}
	if code := runHeadway([]string{"--db", "missing", "--evidence-context", "missing", "--output", t.TempDir()}, &out, &errs); code != 1 {
		t.Fatal(code)
	}
}

func TestHeadwayEvidenceBindingSyntax(t *testing.T) {
	for _, c := range []struct{ name, content, want string }{
		{"unknown", "{\"surprise\":1}", "unknown field"},
		{"multiple", "{} {}", "exactly one JSON"},
		{"incomplete", "{", "EOF"},
		{"identity", "{\"context\":{},\"source\":{}}", "clip identity"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "binding.json")
			os.WriteFile(p, []byte(c.content), 0600)
			var out, errs bytes.Buffer
			code := runHeadway([]string{"--db", "missing", "--source", "test", "--evidence-context", p, "--output", t.TempDir()}, &out, &errs)
			if code != 1 || !strings.Contains(errs.String(), c.want) {
				t.Fatalf("%d: %s", code, errs.String())
			}
		})
	}
	for _, args := range [][]string{{"--db", "missing", "--evidence-context", "missing"}, {"--db", "missing", "--evidence-context", "missing", "--output", t.TempDir(), "--paper", "a4"}} {
		var out, errs bytes.Buffer
		if runHeadway(args, &out, &errs) != 2 {
			t.Fatal("irrelevant/missing options accepted")
		}
	}
}
