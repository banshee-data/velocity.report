package main

import (
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

func physicalArgs(f *evalfixture.PhysicalFixture, extra ...string) []string {
	args := []string{
		"-pack", f.PackDir, "-split-manifest", f.SplitManifestPath, "-split", evalfixture.PhysSplit, "-allow-tuning-split",
		"-a-label", "exact", "-a-db", f.DBPath, "-a-param-hash", evalfixture.ParamsExact, "-a-stage", "online",
		"-a-declared-baseline", "-a-solid-body",
		"-b-label", "face_bias", "-b-db", f.DBPath, "-b-param-hash", evalfixture.ParamsFaceBias, "-b-stage", "online",
		"-b-declared-baseline", "-b-solid-body",
	}
	return append(args, extra...)
}

func TestPerFramePhysicalReferenceFlag(t *testing.T) {
	f, err := evalfixture.WritePhysical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	code, c, stderr := runPerFrameCapture(t, physicalArgs(f, "-physical-reference", "-physical-gate-metres", "2.5"))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if c.Physical == nil || c.Physical.Reference.GateMetres != 2.5 || c.Physical.Reference.PhysicalRevision != 1 ||
		c.Physical.A.Summary.Components[perframeeval.ComponentCentre].Scored != 6 {
		t.Fatalf("physical section: %+v", c.Physical)
	}
	if !strings.Contains(stderr, "physical references: revision 1") || !strings.Contains(stderr, "face_bias physical: centre 6 scored") {
		t.Fatalf("stderr summary: %s", stderr)
	}

	// Pinning the current revision gives the same reference.
	code, pinned, _ := runPerFrameCapture(t, physicalArgs(f, "-physical-reference", "-physical-reference-revision", "1", "-physical-gate-metres", "2.5"))
	if code != 0 || pinned.Physical.Reference.Digest != c.Physical.Reference.Digest {
		t.Fatalf("pinned revision: exit %d", code)
	}

	// Without the flag the output has no physical section.
	if code, plain, _ := runPerFrameCapture(t, physicalArgs(f)); code != 0 || plain.Physical != nil {
		t.Fatalf("physical output without the flag: exit %d", code)
	}
}

func TestPerFramePhysicalReferenceRefusals(t *testing.T) {
	f, err := evalfixture.WritePhysical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		args []string
		code int
	}{
		"revision without the flag": {physicalArgs(f, "-physical-reference-revision", "1"), 2},
		"gate without the flag":     {physicalArgs(f, "-physical-gate-metres", "2.5"), 2},
		"zero gate":                 {physicalArgs(f, "-physical-reference", "-physical-gate-metres", "0"), 2},
		"negative revision":         {physicalArgs(f, "-physical-reference", "-physical-reference-revision", "-1"), 2},
		"missing revision":          {physicalArgs(f, "-physical-reference", "-physical-reference-revision", "4"), 1},
	} {
		if code, _, stderr := runPerFrameCapture(t, c.args); code != c.code {
			t.Errorf("%s: exit %d, want %d: %s", name, code, c.code, stderr)
		}
	}
	m := f.Manifest()
	m.Splits[0].Role = annotation.SplitRoleHeldOut
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runPerFrameCapture(t, physicalArgs(f, "-physical-reference")); code != 1 || !strings.Contains(stderr, "held-out split is refused") {
		t.Fatalf("held-out physical scoring: exit %d: %s", code, stderr)
	}
}

// TestPerFramePhysicalBoxArm: -a-boxes reads arm A through the published
// boxes filed beside its online estimates, scored beside a solid-body arm;
// naming both row kinds for one arm is a usage error.
func TestPerFramePhysicalBoxArm(t *testing.T) {
	f, err := evalfixture.WritePhysical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := evalfixture.WritePhysicalBoxes(f); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"-pack", f.PackDir, "-split-manifest", f.SplitManifestPath, "-split", evalfixture.PhysSplit, "-allow-tuning-split",
		"-physical-reference",
		"-a-label", "box", "-a-db", f.DBPath, "-a-param-hash", evalfixture.ParamsBox, "-a-stage", "online",
		"-a-declared-baseline", "-a-boxes",
		"-b-label", "exact", "-b-db", f.DBPath, "-b-param-hash", evalfixture.ParamsExact, "-b-stage", "online",
		"-b-declared-baseline", "-b-solid-body",
	}
	code, c, stderr := runPerFrameCapture(t, args)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	a := c.Physical.A
	if c.A.Arm.Table != "lidar_track_observations" || a.Arm.ObservationModelID != "published_box/"+evalfixture.BoxObservationID ||
		a.Summary.Components[perframeeval.ComponentCentre].Scored != 6 || a.Summary.Following.Scored != 2 ||
		c.Physical.B.Arm.Table != "lidar_track_solid_bodies" {
		t.Fatalf("box arm: %+v / %+v", c.A.Arm, a.Summary)
	}
	both := append(append([]string{}, args...), "-a-solid-body")
	if code, _, stderr := runPerFrameCapture(t, both); code != 2 || !strings.Contains(stderr, "-a-boxes and -a-solid-body") {
		t.Fatalf("boxes and solid bodies for one arm: exit %d: %s", code, stderr)
	}
}
