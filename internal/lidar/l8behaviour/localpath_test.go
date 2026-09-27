package l8behaviour

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func buildPath(t *testing.T, follower string, trs []Trajectory, p LocalPathParams) LocalPathResult {
	t.Helper()
	res, err := BuildLocalPath(follower, trs, p)
	if err != nil {
		t.Fatalf("build %s: %v", follower, err)
	}
	return res
}

// TestLocalPathFitsAStraightPairExactly: two cars along +x give knots at the
// odd metres from 21 to 81, extended half a bin to 20 and 82, and a path that
// locates like the equivalent straight line, to the bit.
func TestLocalPathFitsAStraightPairExactly(t *testing.T) {
	sc := ScenarioSteadyApproach()
	res := buildPath(t, "trk_s_steady_follower", sc.Trajectories, sc.Params.Path)
	p := res.Path
	if p == nil || len(p.Knots) != 31 || p.LengthM() != 62 || p.AxisRad != 0 || !strings.HasPrefix(p.ID, LocalPathMethodID+"/") {
		t.Fatalf("path %+v", p)
	}
	for i, k := range p.Knots {
		if k.X != float64(21+2*i) || k.Y != 0 || k.ArcM != float64(1+2*i) || k.Bridged || k.Tracks == 0 {
			t.Fatalf("knot %d %+v", i, k)
		}
	}
	if v := p.Vertices; v[0] != (PathVertex{20, 0, 0}) || v[len(v)-1] != (PathVertex{82, 0, 62}) {
		t.Fatalf("ends %+v %+v", v[0], v[len(v)-1])
	}
	line := StraightPath{ID: "line", OriginX: 20, LengthM: 62}
	for _, pt := range [][2]float64{{20, 0}, {21, 0.5}, {44.25, -1.2}, {63.9, 3}, {82, 0}} {
		got, ok := p.Locate(pt[0], pt[1])
		want, _ := line.Locate(pt[0], pt[1])
		if !ok || got != want {
			t.Errorf("locate %v = %+v %v, want %+v", pt, got, ok, want)
		}
	}
	for _, pt := range [][2]float64{{19.99, 0}, {82.01, 0}, {-5, 3}} {
		if _, ok := p.Locate(pt[0], pt[1]); ok {
			t.Errorf("%v is beyond the path's ends", pt)
		}
	}
	// Both parties' paths are the same fit, so the same geometry.
	if other := buildPath(t, "trk_s_steady_leader", sc.Trajectories, sc.Params.Path); other.Path.ID != p.ID {
		t.Fatalf("same evidence, different ids: %s and %s", other.Path.ID, p.ID)
	}
}

// arcCar places a car at arc length s on a left-hand circle of radius r that
// starts at the origin heading +x, facing and moving along it.
func arcCar(t int64, r, s, speed float64) bodySpec {
	a := s / r
	b := fixtureCar(t, r*math.Sin(a), r-r*math.Cos(a), 0)
	b.psi, b.vx, b.vy = a, speed*math.Cos(a), speed*math.Sin(a)
	return b
}

// TestLocalPathFollowsAGentleCurve: on a 150 m radius curve turning through
// 26 degrees, the fitted knots lie on the circle, the bodies project onto the
// path with no lateral offset, and the bumper gap along the fitted path is
// the gap along the road.
func TestLocalPathFollowsAGentleCurve(t *testing.T) {
	const r = 150.0
	follower := frames(30, func(k, t int64) bodySpec { return arcCar(t, r, 20+float64(k), 10).followerBody() })
	leader := frames(30, func(k, t int64) bodySpec { return arcCar(t, r, 40+float64(k), 10) })
	trs := []Trajectory{fixtureTrajectory("trk_curve_follower", follower...), fixtureTrajectory("trk_curve_leader", leader...)}
	params := EncounterScenarioParams()
	res := buildPath(t, "trk_curve_follower", trs, params.Path)
	if res.Path == nil {
		t.Fatalf("curve refused: %v", res.Conditions)
	}
	for _, k := range res.Path.Knots {
		if d := math.Abs(math.Hypot(k.X, k.Y-r) - r); d > 0.01 {
			t.Errorf("knot (%.3f, %.3f) is %.4f m off the circle", k.X, k.Y, d)
		}
	}
	for k := int64(0); k < 30; k++ {
		lp, _ := PartyAt(trs[1], fixtureAt(k))
		fp, _ := PartyAt(trs[0], fixtureAt(k))
		pt, err := EvaluateFollowing(res.Path, lp, fp, params.Following)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(pt.Leader.LateralM) > 0.01 || math.Abs(pt.Follower.LateralM) > 0.01 {
			t.Errorf("frame %d laterals %.4f %.4f", k, pt.Leader.LateralM, pt.Follower.LateralM)
		}
		// The road gap is 20 - 2.25 - 2.0 = 15.75 m of arc.
		if pt.SpatialGap.Suppressed || math.Abs(*pt.SpatialGap.Value-15.75) > 0.02 {
			t.Errorf("frame %d gap %+v, want 15.75 m", k, pt.SpatialGap)
		}
	}
}

// TestLocalPathRefusesAWideGroupThatIsNotOnePath: compatibility is not
// transitive. Two cars 2.2 m apart are each within 1.5 m of a third between
// them, which joins all three into one group, but they are laterally
// incompatible with each other where they overlap, so the group is two paths
// and is refused rather than fitted down the middle.
func TestLocalPathRefusesAWideGroupThatIsNotOnePath(t *testing.T) {
	sc := ScenarioAmbiguousThenResolved()
	params := sc.Params.Path
	params.GroupLateralM = 1.5
	res := buildPath(t, "trk_s_wide_follower", sc.Trajectories, params)
	if res.Path != nil || !reflect.DeepEqual(res.Conditions, []PathCondition{PathLateralIncompatible}) ||
		!reflect.DeepEqual(res.MemberTrackIDs, []string{"trk_s_wide_a", "trk_s_wide_b", "trk_s_wide_follower"}) {
		t.Fatalf("result %+v", res)
	}
}

// TestLocalPathOpposingTrafficIsNotAMember: a car coming the other way inside
// the corridor never joins the path; it is recorded as a direction reversal,
// which pairing reports if it stands between a follower and its leader.
func TestLocalPathOpposingTrafficIsNotAMember(t *testing.T) {
	sc := ScenarioSteadyApproach()
	oncoming := frames(50, func(k, t int64) bodySpec {
		b := fixtureCar(t, 90-float64(k), 0.5, -10)
		b.psi = math.Pi
		return b
	})
	trs := append(sc.Trajectories, fixtureTrajectory("trk_oncoming", oncoming...))
	res := buildPath(t, "trk_s_steady_follower", trs, sc.Params.Path)
	if res.Path == nil || res.IsMember("trk_oncoming") ||
		res.NotEstablishedCondition("trk_oncoming") != PathDirectionReversal {
		t.Fatalf("result members %v not established %v conditions %v", res.MemberTrackIDs, res.NotEstablished, res.Conditions)
	}
	if res.NotEstablishedCondition("trk_nobody") != PathUnestablishedBody || res.IsMember("trk_nobody") {
		t.Fatal("an unknown track is an unestablished body")
	}
}

// TestLocalPathWeakSupport covers the evidence floors: a follower with no
// moving observed evidence, and a path shorter than the minimum extent.
func TestLocalPathWeakSupport(t *testing.T) {
	sc := ScenarioSteadyApproach()
	short := sc.Params.Path
	short.MinExtentM = 70
	if res := buildPath(t, "trk_s_steady_follower", sc.Trajectories, short); res.Path != nil ||
		!reflect.DeepEqual(res.Conditions, []PathCondition{PathWeakSupport}) {
		t.Fatalf("62 m of support under a 70 m minimum: %+v", res)
	}

	queue := ScenarioStandstillQueue()
	stopped := queue.Trajectories[0]
	stopped.Samples = stopped.Samples[10:20] // the stopped frames only
	trs := []Trajectory{stopped, queue.Trajectories[1]}
	if res := buildPath(t, stopped.Passage.TrackID, trs, queue.Params.Path); res.Path != nil ||
		!reflect.DeepEqual(res.Conditions, []PathCondition{PathWeakSupport}) || res.MemberTrackIDs != nil {
		t.Fatalf("a stopped follower names no direction: %+v", res)
	}
}

// TestLocalPathKeepsTheLongestRun: a gap wider than the bridge splits the
// evidence, and the longer side is the path.
func TestLocalPathKeepsTheLongestRun(t *testing.T) {
	sc := ScenarioOcclusion()
	params := sc.Params.Path
	params.MaxBridgeKnots = 1
	res := buildPath(t, "trk_s_occlusion_follower", sc.Trajectories, params)
	// Evidence at x 20-34 and 40-79: the two-knot hole at 36-40 is no
	// longer bridged, so the path is x 40 to 80.
	if res.Path == nil || res.Path.Vertices[0].X != 40 || res.Path.LengthM() != 40 {
		t.Fatalf("path %+v conditions %v", res.Path, res.Conditions)
	}
	if _, ok := res.Path.Locate(30, 0); ok {
		t.Fatal("the unbridged side is off the path")
	}
}

// TestLocalPathLocateAtAnInteriorBend checks the clamped case: a point outside
// a convex bend is nearest the vertex, and is located there with its distance
// as the lateral offset, signed by side.
func TestLocalPathLocateAtAnInteriorBend(t *testing.T) {
	p := &LocalPath{ID: "bend", EstimateStage: StageFinal, Vertices: []PathVertex{
		{0, 0, 0}, {10, 0, 10}, {10, 10, 20},
	}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	loc, ok := p.Locate(13, -4)
	if !ok || loc.ArcM != 10 || loc.LateralM != -5 || loc.TangentRad != 0 {
		t.Fatalf("outside the bend: %+v %v", loc, ok)
	}
	loc, ok = p.Locate(8, 5)
	if !ok || loc.ArcM != 15 || loc.LateralM != 2 || loc.TangentRad != math.Pi/2 {
		t.Fatalf("inside the bend: %+v %v", loc, ok)
	}
	if p.Stage() != StageFinal || p.GeometryID() != "bend" {
		t.Fatal("identity")
	}
}

func TestLocalPathValidateAndRoundTrip(t *testing.T) {
	sc := ScenarioOcclusion()
	res := buildPath(t, "trk_s_occlusion_follower", sc.Trajectories, sc.Params.Path)
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var back LocalPathResult
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, res) || back.Path.Validate() != nil {
		t.Fatal("round trip changed the result")
	}
	for name, fn := range map[string]func(*LocalPath){
		"no id":         func(p *LocalPath) { p.ID = "" },
		"no stage":      func(p *LocalPath) { p.EstimateStage = StageUnspecified },
		"one vertex":    func(p *LocalPath) { p.Vertices = p.Vertices[:1] },
		"bad start arc": func(p *LocalPath) { p.Vertices[0].ArcM = 1 },
		"arc mismatch":  func(p *LocalPath) { p.Vertices[3].ArcM += 0.5 },
		"repeated":      func(p *LocalPath) { p.Vertices[2] = p.Vertices[1] },
		"NaN vertex":    func(p *LocalPath) { p.Vertices[2].X = math.NaN() },
		"NaN axis":      func(p *LocalPath) { p.AxisRad = math.NaN() },
	} {
		p := *res.Path
		p.Vertices = append([]PathVertex(nil), res.Path.Vertices...)
		fn(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	var nilPath *LocalPath
	if nilPath.Validate() == nil {
		t.Fatal("a nil path is invalid")
	}
}

// TestLocalPathIDMovesWithItsInputs: the geometry id is a content address of
// the method, parameters, evidence and geometry.
func TestLocalPathIDMovesWithItsInputs(t *testing.T) {
	sc := ScenarioSteadyApproach()
	base := buildPath(t, "trk_s_steady_follower", sc.Trajectories, sc.Params.Path).Path.ID
	params := sc.Params.Path
	params.MinSamplesPerKnot = 2
	if id := buildPath(t, "trk_s_steady_follower", sc.Trajectories, params).Path.ID; id == base {
		t.Error("a parameter change kept the id")
	}
	shorter := append([]Trajectory(nil), sc.Trajectories...)
	shorter[0].Samples = shorter[0].Samples[:40]
	if id := buildPath(t, "trk_s_steady_follower", shorter, sc.Params.Path).Path.ID; id == base {
		t.Error("a window change kept the id")
	}
}

func TestBuildLocalPathRejectsCallerErrors(t *testing.T) {
	sc := ScenarioSteadyApproach()
	bad := sc.Params.Path
	bad.MaxTangentRad = math.Pi / 2
	if _, err := BuildLocalPath("trk_nobody", sc.Trajectories, sc.Params.Path); err == nil {
		t.Error("unknown follower: want an error")
	}
	if _, err := BuildLocalPath("trk_s_steady_follower", sc.Trajectories, bad); err == nil {
		t.Error("quarter-turn tangent: want an error")
	}
	mixed := append([]Trajectory(nil), sc.Trajectories...)
	mixed[1].Estimate.ObsModelID = "other"
	if _, err := BuildLocalPath("trk_s_steady_follower", mixed, sc.Params.Path); err == nil {
		t.Error("mixed estimators: want an error")
	}
}

func TestLocalPathParamsValidate(t *testing.T) {
	good := EncounterScenarioParams().Path
	for name, fn := range map[string]func(*LocalPathParams){
		"no spacing":        func(p *LocalPathParams) { p.KnotSpacingM = 0 },
		"infinite lateral":  func(p *LocalPathParams) { p.GroupLateralM = math.Inf(1) },
		"NaN speed":         func(p *LocalPathParams) { p.MinSpeedMps = math.NaN() },
		"one evidence":      func(p *LocalPathParams) { p.MinTrackEvidence = 1 },
		"no overlap":        func(p *LocalPathParams) { p.MinOverlapKnots = 0 },
		"no knot samples":   func(p *LocalPathParams) { p.MinSamplesPerKnot = 0 },
		"negative bridge":   func(p *LocalPathParams) { p.MaxBridgeKnots = -1 },
		"one-knot extent":   func(p *LocalPathParams) { p.MinExtentM = 3 },
		"quarter-turn bend": func(p *LocalPathParams) { p.MaxTangentRad = 2 },
	} {
		p := good
		fn(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// TestLocalPathUsesNoCoastedOrSlowEvidence: removing every sample that is not
// path evidence (coasted, degraded, stopped) leaves the fit unchanged.
func TestLocalPathUsesNoCoastedOrSlowEvidence(t *testing.T) {
	for _, sc := range []EncounterScenario{ScenarioOcclusion(), ScenarioStandstillQueue()} {
		full := buildPath(t, sc.Trajectories[0].Passage.TrackID, sc.Trajectories, sc.Params.Path)
		var pruned []Trajectory
		for _, tr := range sc.Trajectories {
			keep := tr
			keep.Samples = nil
			for _, s := range tr.Samples {
				if s.Support == SupportObserved && s.Estimation == EstimationEstablished && math.Hypot(s.VX, s.VY) >= 0.5 {
					keep.Samples = append(keep.Samples, s)
				}
			}
			pruned = append(pruned, keep)
		}
		got := buildPath(t, sc.Trajectories[0].Passage.TrackID, pruned, sc.Params.Path)
		if !reflect.DeepEqual(got.Path.Vertices, full.Path.Vertices) {
			t.Errorf("%s: non-evidence samples moved the fit", sc.Name)
		}
	}
}
