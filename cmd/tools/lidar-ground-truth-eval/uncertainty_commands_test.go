package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

type uncertaintyWriteError struct{}

func (uncertaintyWriteError) Write([]byte) (int, error) { return 0, errors.New("write failure") }

func TestManoeuvreCommand(t *testing.T) {
	f := perFrameFixture(t)
	ref, err := perframeeval.LoadReference(perframeeval.ReferenceOptions{PackDir: f.PackDir,
		SplitManifestPath: f.SplitManifestPath, Split: evalfixture.SplitHeldOut, Policy: annotation.DefaultReferencePolicy()})
	if err != nil {
		t.Fatal(err)
	}
	events, samples := filepath.Join(t.TempDir(), "events.json"), filepath.Join(t.TempDir(), "samples.jsonl")
	b, err := json.Marshal(perframeeval.ManoeuvreManifest{SchemaVersion: 1, ReferenceDigest: ref.Identity.Digest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(events, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(samples, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-pack", f.PackDir, "-split-manifest", f.SplitManifestPath, "-split", evalfixture.SplitHeldOut, "-events", events, "-samples", samples}
	var out, stderr bytes.Buffer
	if code := runManoeuvres(args, &out, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	var report perframeeval.ManoeuvreReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.Status != "insufficient_samples" || report.Fraction != nil {
		t.Fatalf("empty labels passed: %s, %v", out.String(), err)
	}
	if code := runManoeuvres(args, uncertaintyWriteError{}, &stderr); code != 1 {
		t.Fatalf("write failure exit %d", code)
	}
	for _, invalid := range [][]string{{}, {"-unknown"}, append(append([]string{}, args...), "positional")} {
		if code := runManoeuvres(invalid, &out, &stderr); code != 2 {
			t.Fatalf("invalid flags exit %d", code)
		}
	}
	if code := runManoeuvres(append(args, "-ignore-objects", "obj-unknown"), &out, &stderr); code != 1 {
		t.Fatalf("changed policy pin accepted: exit %d", code)
	}
	if code := run([]string{"manoeuvres", "-h"}); code != 0 {
		t.Fatalf("top-level help exit %d", code)
	}
}

func TestUncertaintyRecoveryCommand(t *testing.T) {
	// Build a valid content-addressed prior table with the canonical builder.
	r, err := l8analytics.BuildUncertaintyReport(l8analytics.UncertaintyInput{Options: l8analytics.DefaultUncertaintyFitOptions(0.02)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "frozen.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-calibration", path}
	var out, stderr bytes.Buffer
	if code := runUncertaintyRecovery(args, &out, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	var report l8analytics.FrozenRecoveryReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.CalibrationID != r.Fit.Calibration.ID || report.Status != "within" || report.RecoveredFrames != 1 {
		t.Fatalf("recovery: %s, %v", out.String(), err)
	}
	if code := runUncertaintyRecovery(args, uncertaintyWriteError{}, &stderr); code != 1 {
		t.Fatalf("write failure exit %d", code)
	}
	for _, invalid := range [][]string{{}, {"-unknown"}, append(append([]string{}, args...), "positional")} {
		if code := runUncertaintyRecovery(invalid, &out, &stderr); code != 2 {
			t.Fatalf("invalid flags exit %d", code)
		}
	}
	if code := runUncertaintyRecovery([]string{"-calibration", "/missing"}, &out, &stderr); code != 1 {
		t.Fatalf("missing file exit %d", code)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runUncertaintyRecovery(args, &out, &stderr); code != 1 || !strings.Contains(stderr.String(), "parse calibration") {
		t.Fatalf("invalid table exit %d: %s", code, stderr.String())
	}
	if code := run([]string{"uncertainty-recovery", "-h"}); code != 0 {
		t.Fatalf("top-level help exit %d", code)
	}
}
