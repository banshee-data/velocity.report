package fieldrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

// Rejection cases use a free-flow trajectory: version checks must not depend on
// an encounter existing, otherwise an empty interaction set hides provenance
// mistakes precisely when the reviewer needs exclusion evidence.
func TestEvidenceRejectsInvalidFreeFlowProvenance(t *testing.T) {
	fixture := evidenceFixture(t)
	fixture.Trajectories = fixture.Trajectories[:1]
	for _, tc := range []struct {
		name string
		edit func(*Result)
	}{
		{"missing version", func(r *Result) { r.Version.MethodID = "" }},
		{"method revision", func(r *Result) { r.Version.MethodID += "/unrelated" }},
		{"selected estimator", func(r *Result) { r.Estimates.EstimatorID += "/unrelated" }},
		{"selected model", func(r *Result) { r.Estimates.ObservationModelID += "/unrelated" }},
		{"selected params", func(r *Result) { r.Estimates.ParamHash += "/unrelated" }},
		{"selected stage", func(r *Result) { r.Estimates.Stage = "final" }},
		{"trajectory estimator", func(r *Result) { r.Trajectories[0].Estimate.EstimatorID += "/unrelated" }},
		{"trajectory model", func(r *Result) { r.Trajectories[0].Estimate.ObsModelID += "/unrelated" }},
		{"trajectory params", func(r *Result) { r.Trajectories[0].Estimate.ParamHash += "/unrelated" }},
		{"trajectory stage", func(r *Result) { r.Trajectories[0].Samples[0].Stage = l8behaviour.StageFinal }},
		{"no trajectories", func(r *Result) { r.Trajectories = nil }},
		{"negative box count", func(r *Result) { r.BoxlessEstimates = -1 }},
		{"negative unfiled count", func(r *Result) { r.UnfiledEstimates = -1 }},
		{"invalid parameters", func(r *Result) { r.Params.Exposure.MaxIntervalNanos = -1 }},
		{"invalid sample", func(r *Result) { r.Trajectories[0].Samples[0].CaptureUnixNanos = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			var r Result
			if err = json.Unmarshal(b, &r); err != nil {
				t.Fatal(err)
			}
			tc.edit(&r)
			if _, err = Evidence(r, evidenceContext()); err == nil {
				t.Fatal("accepted contradictory free-flow evidence")
			}
		})
	}
}

func TestEvidenceRejectsUnverifiableSourceManifest(t *testing.T) {
	r := evidenceFixture(t)
	for _, source := range []l4bobserve.CaptureSource{
		{},
		{ClipID: "another-clip", ExtractorID: "another-extractor", CapturePaths: []string{"another.pcapng"}, CaptureSHA256s: []string{strings.Repeat("a", 64)}},
	} {
		c := evidenceContext()
		c.SourceManifest = &source
		if _, err := Evidence(r, c); err == nil {
			t.Fatal("source manifest did not bind the selected source")
		}
	}
}

func TestWriteEvidenceRefusesTamperedDocumentAndFileCollisions(t *testing.T) {
	d, err := Evidence(evidenceFixture(t), evidenceContext())
	if err != nil {
		t.Fatal(err)
	}
	if err = d.Validate(); err != nil {
		t.Fatal(err)
	}
	validDir := t.TempDir()
	p, err := WriteEvidence(validDir, d)
	if err != nil {
		t.Fatal(err)
	}
	// Another process or a previous failed publication may leave unrelated
	// content at the identity's final path. It must survive an attempted write.
	if err = os.WriteFile(p, []byte("corrupt prior output\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = WriteEvidence(validDir, d); err == nil {
		t.Fatal("overwrote corrupt prior output")
	}
	got, err := os.ReadFile(p)
	if err != nil || string(got) != "corrupt prior output\n" {
		t.Fatalf("prior output changed: %q %v", got, err)
	}
	entries, err := os.ReadDir(validDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("failed publication leaked temporary files: %v %v", entries, err)
	}

	t.Run("identity destination is directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, filepath.Base(p)), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := WriteEvidence(dir, d); err == nil {
			t.Fatal("accepted a directory as an existing evidence document")
		}
	})
	t.Run("output parent is regular file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(file, []byte("keep me"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := WriteEvidence(file, d); err == nil {
			t.Fatal("accepted a regular file as output directory")
		}
	})
	t.Run("tampered derived accounting", func(t *testing.T) {
		d.Accounting["invented_exposure"] = 1
		dir := filepath.Join(t.TempDir(), "must-not-be-created")
		if _, err := WriteEvidence(dir, d); err == nil {
			t.Fatal("published accounting inconsistent with retained samples")
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("invalid document created output: %v", err)
		}
	})
}
