package perframeeval

import (
	"errors"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

func physConfig(f *evalfixture.PhysicalFixture) Config {
	opts := DefaultPhysicalOptions()
	return Config{
		Reference: physRefOpts(f),
		Score:     DefaultScoreOptions(),
		A:         physArm(f, "exact", evalfixture.ParamsExact),
		B:         physArm(f, "face_bias", evalfixture.ParamsFaceBias),
		Physical:  &opts,
	}
}

// The physical path runs beside the mask-position comparison: both arms get
// a physical result against one reference, the MOT numbers are unchanged, and
// the output says which is which.
func TestRunScoresBothArmsPhysically(t *testing.T) {
	f := physFixture(t)
	c, err := Run(physConfig(f))
	if err != nil {
		t.Fatal(err)
	}
	if c.Physical == nil || c.Physical.A.Arm.ParamHash != evalfixture.ParamsExact || c.Physical.B.Arm.ParamHash != evalfixture.ParamsFaceBias ||
		c.Physical.Reference.Digest != c.Physical.A.Reference.Digest || c.Physical.A.Reference.Digest != c.Physical.B.Reference.Digest {
		t.Fatalf("physical section: %+v", c.Physical)
	}
	if c.Total.A.NumGT == 0 || c.Reference.Policy.Position != annotation.PositionFootprintCentre {
		t.Fatalf("the mask-position comparison changed: %+v", c.Total)
	}
	found := false
	for _, cv := range c.Caveats {
		found = found || strings.Contains(cv, "not a body centre")
	}
	if !found {
		t.Fatalf("no caveat separates the mask position from the body centre: %v", c.Caveats)
	}
	md := RenderMarkdown(*c)
	for _, want := range []string{"## Physical references", "| centre | 6 |", "| following_gap | 2 |", "Where arm face_bias's expected instants went",
		"unknown_geometry/no_keyframe 11", "> Split \"tune-physical\" has role \"tuning\""} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}

	// Without the option, nothing physical appears.
	cfg := physConfig(f)
	cfg.Physical = nil
	if plain, err := Run(cfg); err != nil || plain.Physical != nil || strings.Contains(RenderMarkdown(*plain), "Physical references") {
		t.Fatalf("physical output without asking: %v", err)
	}
}

func TestRunRefusesPhysicalScoringItCannotDo(t *testing.T) {
	f := physFixture(t)
	cfg := physConfig(f)
	cfg.A = ArmSpec{Label: "a", DBPath: f.DBPath, RunID: "run-a", DeclaredBaseline: true}
	cfg.B = ArmSpec{Label: "b", DBPath: f.DBPath, RunID: "run-b", DeclaredBaseline: true}
	if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "analysis runs") {
		t.Fatalf("analysis runs were physically scored: %v", err)
	}

	m := f.Manifest()
	m.Splits[0].Role = annotation.SplitRoleHeldOut
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(physConfig(f)); !errors.Is(err, ErrPhysicalHeldOut) {
		t.Fatalf("a held-out split was physically scored: %v", err)
	}
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, f.Manifest()); err != nil {
		t.Fatal(err)
	}

	cfg = physConfig(f)
	cfg.B = physArm(f, "other", evalfixture.ParamsOtherCalibration)
	if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "calibration") {
		t.Fatalf("an arm under another calibration was scored: %v", err)
	}

	// An arm the mask-position path can read but the physical path cannot.
	insertUnreadableRows(t, f.DBPath)
	cfg = physConfig(f)
	cfg.B = physArm(f, "near_face", "params/near-face")
	if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "no declared offset") {
		t.Fatalf("an unreadable physical arm: %v", err)
	}
}
