package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// writeTestCriteria writes a following-criteria record beside the fixture.
// Its values are synthetic fixtures, not limits anyone has chosen.
func writeTestCriteria(t *testing.T, dir, name string, pinned bool) string {
	t.Helper()
	v := func(x float64) *float64 { return &x }
	draws, seed, enc, provisional := 200, int64(3), 3, false
	c := perframeeval.FollowingCriteria{
		Schema: perframeeval.FollowingCriteriaSchema, SchemaVersion: perframeeval.FollowingCriteriaSchemaVersion,
		RecordID: "following-limits-cli-test", Status: perframeeval.CriteriaDraft,
		Limits: perframeeval.FollowingLimits{
			GapErrorLimitM: v(0.5), ReferencePrecisionM: v(1), NominalCoverage: v(0.9), MinCoverage: v(0.5), MaxCoverage: v(1),
			MinIndependentEncounters: &enc, MaxMissingOpportunity: v(0.5), MaxSuppressedOpportunity: v(0.5), WrongLeaderTolerance: v(0.2),
		},
		ValidFollowing: perframeeval.ValidFollowingDefinition{
			MeasurementFloorMps: v(0.4), OpportunityFloorMph: v(2), SpatialOnlyBelowMph: v(10), NetCeilingS: v(4),
			CeilingProvisional: &provisional, HysteresisMarginS: v(0.5), MinDwellS: v(1), SensitivityCeilingsS: []float64{3, 4, 6},
		},
		Bootstrap: perframeeval.FollowingBootstrapPlan{Draws: &draws, Seed: &seed, FamilyConfidence: v(0.95),
			Correction: perframeeval.CorrectionBonferroni},
	}
	if pinned {
		c.Status, c.Decision, c.PinnedOn = perframeeval.CriteriaPinned, "D-98", "2026-11-05"
		c.DraftedOn = perframeeval.FollowingCriteriaProvenance{Split: "tune", SplitDigest: "sha256:split", Report: "docs/report.md"}
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// -following-criteria is cited in the output and summary; it needs
// -physical-reference; a held-out split is refused under a draft and scored
// under a pinned record.
func TestPerFrameFollowingCriteria(t *testing.T) {
	f, err := evalfixture.WritePhysical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(f.SplitManifestPath)
	draft := writeTestCriteria(t, dir, "draft.json", false)
	pinned := writeTestCriteria(t, dir, "pinned.json", true)

	if code, _, stderr := runPerFrameCapture(t, physicalArgs(f, "-following-criteria", draft)); code != 2 ||
		!strings.Contains(stderr, "-following-criteria needs -physical-reference") {
		t.Fatalf("criteria without physical scoring: exit %d: %s", code, stderr)
	}
	code, c, stderr := runPerFrameCapture(t, physicalArgs(f, "-physical-reference", "-following-criteria", draft))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if cite := c.Physical.Reference.FollowingCriteria; cite == nil || cite.Path != draft || cite.Status != perframeeval.CriteriaDraft {
		t.Fatalf("citation %+v", cite)
	}
	if !strings.Contains(stderr, "following criteria following-limits-cli-test: "+draft) ||
		!strings.Contains(stderr, "exact following acceptance: insufficient") {
		t.Fatalf("stderr summary: %s", stderr)
	}

	m := f.Manifest()
	m.Splits[0].Role = annotation.SplitRoleHeldOut
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runPerFrameCapture(t, physicalArgs(f, "-physical-reference", "-following-criteria", draft)); code != 1 ||
		!strings.Contains(stderr, "held-out split is refused") || !strings.Contains(stderr, "not pinned") {
		t.Fatalf("held out under a draft: exit %d: %s", code, stderr)
	}
	code, c, stderr = runPerFrameCapture(t, physicalArgs(f, "-physical-reference", "-following-criteria", pinned))
	if code != 0 || c.Physical.Reference.SplitRole != annotation.SplitRoleHeldOut || !c.Physical.Reference.FollowingCriteria.Pinned() {
		t.Fatalf("held out under a pinned record: exit %d: %s", code, stderr)
	}
}

// A bundle holds the cited record's bytes and verifies; a change to the
// record where it was cited is refused.
func TestBundlePinsFollowingCriteria(t *testing.T) {
	f, _ := frozenPhysical(t)
	criteria := writeTestCriteria(t, filepath.Dir(f.PackDir), "criteria.json", false)
	bundle := writeFixtureBundle(t, f, "-following-criteria", criteria)
	m := readManifest(t, bundle)
	raw, err := os.ReadFile(criteria)
	if err != nil {
		t.Fatal(err)
	}
	if m.FollowingCriteria == nil || m.FollowingCriteria.Path != criteria || m.FollowingCriteria.SHA256 != sha256Hex(raw) {
		t.Fatalf("criteria pin %+v", m.FollowingCriteria)
	}
	copied, err := os.ReadFile(filepath.Join(bundle, bundleCriteriaFile))
	if err != nil || string(copied) != string(raw) {
		t.Fatalf("bundled criteria copy: %v", err)
	}
	if code, stderr := verify(bundle); code != 0 {
		t.Fatalf("verify: exit %d: %s", code, stderr)
	}
	appendTo(t, criteria, "\n")
	if code, stderr := verify(bundle); code != 1 || !strings.Contains(stderr, "following criteria "+criteria+" changed") {
		t.Fatalf("verify after the record changed: exit %d: %s", code, stderr)
	}
}
