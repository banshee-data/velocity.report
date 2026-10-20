package perframeeval

import (
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

func boxFixture(t *testing.T) *evalfixture.PhysicalFixture {
	t.Helper()
	f := physFixture(t)
	if err := evalfixture.WritePhysicalBoxes(f); err != nil {
		t.Fatal(err)
	}
	return f
}

func boxArm(f *evalfixture.PhysicalFixture) ArmSpec {
	return ArmSpec{Label: "box", DBPath: f.DBPath, Stage: "online", DeclaredBaseline: true, Boxes: true}
}

// TestPhysicalScoringOfPublishedBoxes: an arm read through the published
// boxes filed beside its online estimates is scored as the body the boxes
// draw, not the estimates' visible-box positions half a metre behind them.
// The boxes are the truth, so the centre, axis, extents, bumpers and gap are
// exact; every gap's endpoints are temporally inferred, because a box records
// no face, and the first, at the track's first box, carries the unconverged
// extent as its prediction reason.
func TestPhysicalScoringOfPublishedBoxes(t *testing.T) {
	f := boxFixture(t)
	pr, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	arm, err := LoadPhysicalArm(boxArm(f))
	if err != nil {
		t.Fatal(err)
	}
	id := arm.Identity
	if id.Table != boxTable || id.ObservationModelID != "published_box/"+evalfixture.BoxObservationID ||
		id.ParamHash != evalfixture.ParamsBox || id.Stage != StageOnline || id.Tracks != 2 || id.Points != 19 {
		t.Fatalf("identity %+v", id)
	}
	r, err := ScorePhysical(pr, arm)
	if err != nil {
		t.Fatal(err)
	}
	checkComplete(t, r)
	in := instantOf(t, r, evalfixture.Follower, 0)
	for _, c := range []PhysicalComponent{ComponentCentre, ComponentYaw, ComponentLength, ComponentWidth,
		ComponentFront, ComponentRear, ComponentEnds, ComponentBox} {
		wantOutcome(t, in, c, OutcomeScored, "")
	}
	if cmp := in.Comparison; !approx(cmp.Centre.ErrorM, 0) || !approx(cmp.Yaw.ErrorRad, 0) || !approx(cmp.Length.ErrorM, 0) ||
		!approx(cmp.Front.DistanceM, 0) || !approx(cmp.Box.IoU, 1) {
		t.Fatalf("exact box at sample 0: %+v", cmp)
	}
	if in.Prediction.Reference != "body_centre" || !approx(in.Prediction.XM, evalfixture.FollowerX(0)) {
		t.Fatalf("prediction %+v is not the box", in.Prediction)
	}
	var reasons []string
	for _, g := range r.Following {
		if g.Outcome.Category != OutcomeScored {
			continue
		}
		p := g.PredictedGap
		if !approx(*g.ErrorM, 0) || !approx(p.ValueM, evalfixture.TrueGapM) || p.FollowerSource != "temporally_inferred" ||
			p.LeaderSource != "temporally_inferred" {
			t.Fatalf("gap at %d: %+v", g.SampleID, p)
		}
		reasons = append(reasons, p.PredictionReason)
	}
	if strings.Join(reasons, ",") != "extent_not_converged," {
		t.Fatalf("gap prediction reasons %v", reasons)
	}
	s := r.Summary
	if s.Components[ComponentCentre].Scored != 6 || s.Components[ComponentCentre].WithinReferenceBound != 6 ||
		s.Following.Scored != 2 || s.Following.WithinReferenceBound != 2 {
		t.Fatalf("summary %+v", s)
	}
}

// TestLoadArmReadsTheBoxCentres: the per-frame arm of a box version tracks
// the boxes' centres, keyed by creation sequence, and names the box's table
// and observation model.
func TestLoadArmReadsTheBoxCentres(t *testing.T) {
	f := boxFixture(t)
	h, err := LoadArm(boxArm(f))
	if err != nil {
		t.Fatal(err)
	}
	if h.Identity.Table != boxTable || h.Identity.ObservationModelID != "published_box/"+evalfixture.BoxObservationID ||
		h.Identity.Points != 19 || len(h.Series) != 2 || h.Series[0].ID != "seq-000001" {
		t.Fatalf("identity %+v, %d series", h.Identity, len(h.Series))
	}
	for _, p := range h.Series[1].Points {
		i := int((p.TimestampNanos - evalfixture.EstimateOffsetNs - evalfixture.SampleTime(0)) / (evalfixture.SampleTime(1) - evalfixture.SampleTime(0)))
		if !approx(float64(p.X), evalfixture.LeaderX(i)) {
			t.Fatalf("leader at sample %d: x %g, want the box's %g", i, p.X, evalfixture.LeaderX(i))
		}
	}
	c := strings.Join(caveats(Comparison{A: ArmResult{Arm: h.Identity}, B: ArmResult{Arm: h.Identity}}), "\n")
	if !strings.Contains(c, "published box read as the body") {
		t.Errorf("caveats lack the box: %s", c)
	}
}

// TestBoxArmRefusals: a box arm that also names solid bodies, a box arm at a
// refined stage, and a box arm over a database whose replay filed no box.
func TestBoxArmRefusals(t *testing.T) {
	f := boxFixture(t)
	both := boxArm(f)
	both.SolidBodies = true
	final := boxArm(f)
	final.Stage = StageFinal
	run := boxArm(f)
	run.Stage, run.RunID = "", "run"
	for name, c := range map[string]struct {
		spec ArmSpec
		want string
	}{
		"with solid bodies": {both, "not both"},
		"final":             {final, "online estimates only"},
	} {
		if _, err := LoadPhysicalArm(c.spec); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("physical %s: %v", name, err)
		}
		if _, err := LoadArm(c.spec); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("per-frame %s: %v", name, err)
		}
	}
	if _, err := LoadArm(run); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("a box arm naming a run: %v", err)
	}
	plain := physFixture(t)
	if _, err := LoadPhysicalArm(boxArm(plain)); err == nil || !strings.Contains(err.Error(), "no estimates match") {
		t.Errorf("a database with no boxes: %v", err)
	}
}
