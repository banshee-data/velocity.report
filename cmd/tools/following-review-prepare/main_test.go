package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitycli"
	fx "github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitytest"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/version"
)

func TestRecordingBoundsExcludeWarmupWithoutMutatingEvidence(t *testing.T) {
	in := []l8behaviour.Trajectory{
		{Passage: l8behaviour.Passage{TrackID: "warmup"}, Samples: []l8behaviour.TrajectorySample{{CaptureUnixNanos: 9}}},
		{Passage: l8behaviour.Passage{TrackID: "recorded"}, Samples: []l8behaviour.TrajectorySample{{CaptureUnixNanos: 9}, {CaptureUnixNanos: 10}, {CaptureUnixNanos: 20}, {CaptureUnixNanos: 21}}},
	}
	out := recordedTrajectories(in, 10, 20)
	if len(out) != 1 || len(out[0].Samples) != 2 || out[0].Samples[0].CaptureUnixNanos != 10 || out[0].Samples[1].CaptureUnixNanos != 20 {
		t.Fatalf("wrong recording scope: %+v", out)
	}
	out[0].Samples[0].CaptureUnixNanos = 100
	if len(in[1].Samples) != 4 || in[1].Samples[1].CaptureUnixNanos != 10 {
		t.Fatal("changed the retained source evidence")
	}
}

func TestValidationCaptureIsAdmittedOnlyAsAnnotationReview(t *testing.T) {
	dir := t.TempDir()
	path, sha := fx.Capture(t, dir, "review.pcap", "synthetic review bytes")
	manifest := fx.Manifest(t, dir, map[string]any{"sites": []any{fx.Site("review-site", "validation")}, "captures": []any{fx.CaptureEntry("capture", "review-site", sha, "review.pcap")}})
	ledgerPath := filepath.Join(dir, "ledger.db")
	database, err := db.NewDB(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	root := filepath.Join(dir, "annotations")
	if err = os.MkdirAll(filepath.Join(root, "splits"), 0755); err != nil {
		t.Fatal(err)
	}
	empty := ""
	guard := &eligibilitycli.CLIFlags{Manifest: &manifest, AnnotationRoot: &root, LedgerDB: &ledgerPath, Actor: &empty, Measures: &empty}
	id, err := eligibility.NewHasher().Capture(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := authorizeReview(guard, id, "review-site", "checked-in-baseline")
	if err != nil || !d.Allowed {
		t.Fatalf("annotation review refused: %+v %v", d, err)
	}
	var reads int
	if err = database.QueryRow("SELECT COUNT(*) FROM lidar_data_uses WHERE purpose <> 'annotation_review'").Scan(&reads); err != nil {
		t.Fatal(err)
	}
	if reads != 0 {
		t.Fatal("review was recorded as a development read")
	}
	if _, err = authorizeReview(guard, id, "different-site", "baseline"); err == nil {
		t.Fatal("accepted a mismatched site binding")
	}
	if _, err = guard.Authorize("test-development", eligibility.Request{Purpose: eligibility.PurposeDeveloperDiagnostic, Inputs: []eligibility.Input{{Identity: id}}}); err == nil {
		t.Fatal("validation capture became development data")
	}
}

func TestPreparationRequiresExplicitInputsAndStampedBuild(t *testing.T) {
	before := version.GitSHA
	t.Cleanup(func() { version.GitSHA = before })
	version.GitSHA = "test-build"
	dir := t.TempDir()
	tuning := filepath.Join(dir, "tuning.json")
	if err := os.WriteFile(tuning, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	valid := options{Capture: "capture", Output: "output", Site: "site", Clip: "clip", Binding: "binding", Tuning: tuning, Calibration: "calibration", Warmup: 70, Port: 2369}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*options){func(o *options) { o.Site = "" }, func(o *options) { o.Calibration = "" }, func(o *options) { o.Tuning = "missing" }, func(o *options) { o.Warmup = math.NaN() }, func(o *options) { o.Warmup = 0 }, func(o *options) { o.Port = 65536 }} {
		o := valid
		mutate(&o)
		if err := o.validate(); err == nil {
			t.Fatalf("accepted invalid options %+v", o)
		}
	}
	version.GitSHA = "unknown"
	if err := valid.validate(); err == nil {
		t.Fatal("accepted an unstamped producer")
	}
	if err := prepare(valid, nil); err == nil {
		t.Fatal("unstamped preparation proceeded")
	}
	version.GitSHA = "test-build"
	for _, content := range []string{"missing", "not-json", "{}"} {
		o := valid
		o.Calibration = filepath.Join(dir, content+".json")
		if content != "missing" {
			if err := os.WriteFile(o.Calibration, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
		if err := prepare(o, nil); err == nil {
			t.Fatal("preparation accepted missing or invalid calibration")
		}
	}
}
