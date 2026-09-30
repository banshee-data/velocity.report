package l5tracks

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Tests for remedy T5, the rank-one medoid (SolidBodyOptions.RankOneMedoidScale).
// What carries the weight: off, a rank-one fix moves nothing along its face's
// tangent; on, it moves the tangent toward the medoid by exactly the scalar
// gain A2's loose noise gives, with the width behind an end face and the
// length behind a side face; a rank-two fix is untouched; the fix's rank and
// NIS stay the faces'; the record names the medoid along the tangent; and on
// the synthetic pass the tracked state still anchors the centre and keeps a
// lane change.

// rankOneFaces are a rear and a right face whose implied centre is the a2Track
// prediction (the origin), so the faces themselves move nothing.
var (
	rankOneRear = EdgeMeasurement{Face: FaceRear, NormalX: -1, NormalY: 0, PlaneOffsetMetres: 2.25,
		HalfExtentMetres: 2.25, HalfExtentProvenance: ProvenanceAccumulated}
	rankOneRight = EdgeMeasurement{Face: FaceRight, NormalX: 0, NormalY: -1, PlaneOffsetMetres: 0.9,
		HalfExtentMetres: 0.9, HalfExtentProvenance: ProvenanceAccumulated}
)

type rankOneOutcome struct {
	sb      *solidBodyTrack
	m       SolidBodyMeasurement
	outcome nearEdgeOutcome
	applied nearEdgeApplication
	noise   float64
}

// stepRankOne takes one state-machine step for a body-centre body at the
// origin with a 4.5 by 1.8 m belief, with T5 at scale (zero is off).
func stepRankOne(t *testing.T, scale float32, faces []EdgeMeasurement, cluster WorldCluster,
	edit func(*TrackedObject, *TrackerConfig, *nearEdgeFrame)) rankOneOutcome {
	t.Helper()
	cfg := nearEdgeTrackingConfig()
	cfg.SolidBody.RankOneMedoidScale = scale
	track := a2Track()
	f := nearEdgeFrame{edges: EdgeMeasurementSet{Rank: len(faces), Edges: faces},
		length: DimensionBelief{Metres: 4.5}, width: DimensionBelief{Metres: 1.8}}
	if edit != nil {
		edit(track, &cfg, &f)
	}
	tracker := NewTracker(cfg)
	sb := &track.solidBody
	sb.syncFromTrack(track)
	m, outcome, applied := tracker.stepNearEdge(sb, cluster, nearEdgePriorFor(track), f)
	return rankOneOutcome{sb: sb, m: m, outcome: outcome, applied: applied, noise: float64(cfg.MeasurementNoise)}
}

func TestRankOneMedoidIsOffByDefault(t *testing.T) {
	if got := DefaultTrackerConfig().SolidBody.RankOneMedoidScale; got != 0 {
		t.Fatalf("T5 scale %v by default, want off", got)
	}
}

func TestAnEndFaceTakesTheMedoidAcrossTheBody(t *testing.T) {
	// A rear face constrains along the body; the medoid of a body seen from
	// behind sits 0.5 m to one side of the prediction. The width is what the
	// medoid can be off by that way.
	cluster := WorldCluster{CentroidX: -2.0, CentroidY: 0.5}
	off := stepRankOne(t, 0, []EdgeMeasurement{rankOneRear}, cluster, nil)
	if off.outcome != nearEdgeFix || off.sb.state[1] != 0 {
		t.Fatalf("without T5 a rank-one fix moved across its face: outcome %v, y %v", off.outcome, off.sb.state[1])
	}
	for _, scale := range []float32{1, 0.25} {
		on := stepRankOne(t, scale, []EdgeMeasurement{rankOneRear}, cluster, nil)
		half := 0.9
		want := 0.5 * 0.04 / (0.04 + on.noise + float64(scale)*half*half)
		if on.outcome != nearEdgeFix || math.Abs(float64(on.sb.state[1])-want) > 1e-6 {
			t.Fatalf("scale %v: y %v, want %v toward the medoid", scale, on.sb.state[1], want)
		}
		// The fixture's position carries no covariance with velocity or
		// across axes, so the loose term moves nothing but y here; the
		// oblique test covers the correlated case.
		if on.sb.state[0] != off.sb.state[0] || on.sb.state[2] != 0 || on.sb.state[3] != 0 {
			t.Fatalf("scale %v: T5 moved along the face or velocity: %v", scale, on.sb.state)
		}
		if on.m.Rank != 1 || on.m.NIS != off.m.NIS || on.m.Source != MeasurementNearEdgeCandidateV1 {
			t.Fatalf("scale %v: the fix's record changed: %+v against %+v", scale, on.m, off.m)
		}
		// The recorded measurement is the medoid across the body and the
		// face's implied centre along it.
		if math.Abs(float64(on.applied.measuredY)-0.5) > 1e-6 || on.applied.measuredX != 0 {
			t.Fatalf("scale %v: recorded measurement (%v, %v), want (0, 0.5)", scale, on.applied.measuredX, on.applied.measuredY)
		}
		if on.sb.p[1*4+1] >= 0.04 {
			t.Fatalf("scale %v: the across-body variance %v did not shrink", scale, on.sb.p[1*4+1])
		}
	}
	plain := stepRankOne(t, 1, []EdgeMeasurement{rankOneRear}, cluster, nil)
	tight := stepRankOne(t, 0.25, []EdgeMeasurement{rankOneRear}, cluster, nil)
	if !(tight.sb.state[1] > plain.sb.state[1]) {
		t.Fatalf("the tight setting (%v) did not trust the medoid more than the plain one (%v)",
			tight.sb.state[1], plain.sb.state[1])
	}
}

func TestASideFaceLeavesTheLengthToThePrediction(t *testing.T) {
	// A side face alone leaves the length open. The medoid slides along a
	// passing body as its aspect changes, and that slide would reach the
	// speed through the position-velocity covariance, so T5 leaves it be.
	cluster := WorldCluster{CentroidX: 1.0, CentroidY: -0.9}
	off := stepRankOne(t, 0, []EdgeMeasurement{rankOneRight}, cluster, nil)
	on := stepRankOne(t, 1, []EdgeMeasurement{rankOneRight}, cluster, nil)
	if on.outcome != nearEdgeFix || on.sb.state != off.sb.state || on.sb.p != off.sb.p || on.applied != off.applied {
		t.Fatalf("T5 changed a side-face fix: %v against %v", on.sb.state, off.sb.state)
	}
}

func TestAFoundSideFaceStopsTheMedoid(t *testing.T) {
	// A side face the frame found pulls the medoid toward it, so while one
	// is visible T5 stays off, even if the fix does not use the face: here
	// its half-width is only the class prior, which Section 8.1 excludes.
	cluster := WorldCluster{CentroidX: -2.0, CentroidY: 0.5}
	prior := rankOneRight
	prior.HalfExtentProvenance = ProvenanceClassPrior
	off := stepRankOne(t, 0, []EdgeMeasurement{rankOneRear, prior}, cluster, nil)
	on := stepRankOne(t, 1, []EdgeMeasurement{rankOneRear, prior}, cluster, nil)
	if on.m.Rank != 1 || on.sb.state != off.sb.state || on.applied != off.applied {
		t.Fatalf("T5 applied beside a found side face: rank %d, %v against %v", on.m.Rank, on.sb.state, off.sb.state)
	}
	// Hysteresis withholding a side face is the same case: found, not used.
	tracker := NewTracker(rankOneTrackingConfig(1))
	f := nearEdgeFrame{edges: EdgeMeasurementSet{Rank: 2, Edges: []EdgeMeasurement{rankOneRear, rankOneRight}},
		width: DimensionBelief{Metres: 1.8}}
	if _, ok := tracker.rankOneMedoidTerm([]EdgeMeasurement{rankOneRear}, f, cluster); ok {
		t.Fatal("T5 applied while a side face was withheld")
	}
}

func TestAnInvalidScaleIsOff(t *testing.T) {
	cluster := WorldCluster{CentroidX: -2.0, CentroidY: 0.5}
	for _, scale := range []float32{-1, float32(math.NaN())} {
		if got := stepRankOne(t, scale, []EdgeMeasurement{rankOneRear}, cluster, nil); got.sb.state[1] != 0 {
			t.Errorf("scale %v moved the body across its face to %v", scale, got.sb.state[1])
		}
	}
}

func TestAnObliqueFixIsTheJointUpdate(t *testing.T) {
	// Off the axes, with correlated position and velocity and a face that
	// disagrees with the prediction, the face and the loose term applied in
	// turn must be exactly the joint two-dimensional update, and the fix's
	// NIS the face's own.
	psi := 30 * math.Pi / 180
	nx, ny := -math.Cos(psi), -math.Sin(psi) // the rear face's outward normal
	face := EdgeMeasurement{Face: FaceRear, NormalX: float32(nx), NormalY: float32(ny),
		PlaneOffsetMetres: 2.25 + 0.3, HalfExtentMetres: 2.25, HalfExtentProvenance: ProvenanceAccumulated}
	cluster := WorldCluster{CentroidX: -1.6, CentroidY: -0.2}
	p := [16]float32{
		0.20, 0.05, 0.10, 0.02,
		0.05, 0.12, 0.01, 0.06,
		0.10, 0.01, 1.00, 0.10,
		0.02, 0.06, 0.10, 0.80,
	}
	edit := func(track *TrackedObject, _ *TrackerConfig, f *nearEdgeFrame) {
		track.X, track.Y, track.VX, track.VY = 0.1, -0.1, 10.4, 6.0
		track.P = p
		f.axis = float32(psi)
	}
	got := stepRankOne(t, 1, []EdgeMeasurement{face}, cluster, edit)

	// The joint update in float64: H's rows are the normal and the tangent
	// over position, with R and R plus the half-width squared.
	tx, ty := -ny, nx
	r := got.noise
	half := 0.9
	x := [4]float64{0.1, -0.1, 10.4, 6.0}
	var P [4][4]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			P[i][j] = float64(p[i*4+j])
		}
	}
	h := [2][2]float64{{nx, ny}, {tx, ty}}
	z := [2]float64{float64(face.ImpliedCentreOffset()), tx*float64(cluster.CentroidX) + ty*float64(cluster.CentroidY)}
	var pht [4][2]float64
	for i := 0; i < 4; i++ {
		for k := 0; k < 2; k++ {
			pht[i][k] = P[i][0]*h[k][0] + P[i][1]*h[k][1]
		}
	}
	var S [2][2]float64
	for a := 0; a < 2; a++ {
		for b := 0; b < 2; b++ {
			S[a][b] = h[a][0]*pht[0][b] + h[a][1]*pht[1][b]
		}
	}
	S[0][0] += r
	S[1][1] += r + half*half
	det := S[0][0]*S[1][1] - S[0][1]*S[1][0]
	inv := [2][2]float64{{S[1][1] / det, -S[0][1] / det}, {-S[1][0] / det, S[0][0] / det}}
	nu := [2]float64{z[0] - (nx*x[0] + ny*x[1]), z[1] - (tx*x[0] + ty*x[1])}
	for i := 0; i < 4; i++ {
		var k [2]float64
		for b := 0; b < 2; b++ {
			k[b] = pht[i][0]*inv[0][b] + pht[i][1]*inv[1][b]
		}
		want := x[i] + k[0]*nu[0] + k[1]*nu[1]
		if math.Abs(float64(got.sb.state[i])-want) > 1e-4 {
			t.Fatalf("state[%d] %v, want the joint update's %v", i, got.sb.state[i], want)
		}
	}
	faceNIS := nu[0] * nu[0] / (S[0][0])
	if math.Abs(float64(got.m.NIS)-faceNIS) > 1e-4 || got.m.Rank != 1 {
		t.Fatalf("fix NIS %v rank %d, want the face's own %v at rank one", got.m.NIS, got.m.Rank, faceNIS)
	}
}

func TestARankTwoFixIgnoresTheMedoid(t *testing.T) {
	cluster := WorldCluster{CentroidX: -2.0, CentroidY: 0.5}
	faces := []EdgeMeasurement{rankOneRear, rankOneRight}
	off := stepRankOne(t, 0, faces, cluster, nil)
	on := stepRankOne(t, 1, faces, cluster, nil)
	if on.sb.state != off.sb.state || on.sb.p != off.sb.p || on.applied != off.applied {
		t.Fatalf("T5 changed a rank-two fix: %v against %v", on.sb.state, off.sb.state)
	}
}

func TestADegenerateLooseTermLeavesTheFaceFixAlone(t *testing.T) {
	// With no noise, no width and no uncertainty across the body, the loose
	// term's innovation variance is zero; it is skipped, and the face's own
	// update, which is well posed, stands.
	zero := func(track *TrackedObject, cfg *TrackerConfig, f *nearEdgeFrame) {
		cfg.MeasurementNoise = 0
		track.P[1*4+1] = 0
		f.width = DimensionBelief{}
	}
	cluster := WorldCluster{CentroidX: -2.0, CentroidY: 0.5}
	got := stepRankOne(t, 1, []EdgeMeasurement{rankOneRear}, cluster, zero)
	if got.outcome != nearEdgeFix || got.sb.state[1] != 0 || got.applied.measuredY != 0 {
		t.Fatalf("a degenerate loose term moved the body: outcome %v, state %v, measured y %v",
			got.outcome, got.sb.state, got.applied.measuredY)
	}
}

func rankOneTrackingConfig(scale float32) TrackerConfig {
	cfg := nearEdgeTrackingConfig()
	cfg.SolidBody.RankOneMedoidScale = scale
	return cfg
}

// rankOneCorpusConfig is the tracked arm the corpus runs T5 on: T1 and T3
// with it. Seen end-on, a rear face alone reads as a box across the lane, so
// the tracked heading is wrong there and only the course (T3) orients the
// face.
func rankOneCorpusConfig(scale float32) TrackerConfig {
	cfg := rankOneTrackingConfig(scale)
	cfg.SolidBody.FaceHysteresis = true
	cfg.SolidBody.CourseAlignedFaces = true
	return cfg
}

// endOnPass is a vehicle driving straight away from the sensor along its
// line of sight, so it mostly shows its rear face alone: the rank-one case,
// with the lateral direction open.
func endOnPass() l4perception.SyntheticPass {
	pass := l4perception.DefaultSyntheticPass()
	pass.Occluder = l4perception.SyntheticOccluder{}
	pass.Vehicle.StartXMetres = 8
	pass.Vehicle.LateralOffsetMetres = 0
	return pass
}

func TestTheTrackedRankOneMedoidBoundsTheLateralDrift(t *testing.T) {
	// The plan's rank-one drift, synthetic: a vehicle seen from behind
	// changes lane by 2 m. With only its rear face fixing it, nothing
	// measures the lateral move until a side face appears, so the tracked
	// state rides on its prediction and falls behind. The medoid across the
	// body bounds that: the worst lateral error must fall to 60 % or less of
	// what it is without T5, and the mean must fall.
	pass := endOnPass()
	pass.Vehicle.LaneChangeMetres = 2
	pass.Vehicle.LaneChangeStartSecs = 1
	pass.Vehicle.LaneChangeDurationSecs = 2
	frames := syntheticPassFrames(t, pass)
	type drift struct{ mean, worst, speed float64 }
	got := map[float32]drift{}
	for _, scale := range []float32{0, 1, 0.25} {
		tracker := NewTracker(rankOneCorpusConfig(scale))
		var errs, speed []float64
		worst := 0.0
		for _, f := range frames {
			tracker.Update(f.clusters, f.at)
			if len(tracker.GetActiveTracks()) == 0 {
				continue
			}
			track := mainTrack(t, tracker)
			if track.PositionReference() != ReferenceBodyCentre {
				continue
			}
			e := math.Abs(float64(track.Y) - f.truthY)
			errs = append(errs, e)
			worst = math.Max(worst, e)
			speed = append(speed, math.Abs(float64(track.VX)-12))
		}
		if len(errs) < 20 {
			t.Fatalf("scale %v: only %d body-centre frames", scale, len(errs))
		}
		got[scale] = drift{meanOf(errs), worst, meanOf(speed)}
		t.Logf("scale %v: lateral error mean %.3f m, worst %.3f m, along-track speed error %.3f m/s over %d body-centre frames",
			scale, got[scale].mean, worst, got[scale].speed, len(errs))
	}
	for _, scale := range []float32{1, 0.25} {
		if got[scale].worst > 0.6*got[0].worst || !(got[scale].mean < got[0].mean) {
			t.Errorf("scale %v: lateral error mean %.3f, worst %.3f m, against %.3f and %.3f m without T5",
				scale, got[scale].mean, got[scale].worst, got[0].mean, got[0].worst)
		}
		// Pulling across the body must not cost the speed along it.
		if got[scale].speed > got[0].speed+0.05 {
			t.Errorf("scale %v: along-track speed error %.3f m/s against %.3f without T5", scale, got[scale].speed, got[0].speed)
		}
	}
}

func TestTheTrackedRankOneMedoidLeavesASideOnPassAlone(t *testing.T) {
	// The default pass is seen side-on and at a corner, so T5 has almost
	// nothing to do there: the settled lateral anchor, the along-track
	// position and the speed must be what they are without it.
	type pass struct{ lateral, along, speed float64 }
	run := func(scale float32) pass {
		tracker := NewTracker(rankOneCorpusConfig(scale))
		var lateral, along, speed []float64
		for _, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
			tracker.Update(f.clusters, f.at)
			track := mainTrack(t, tracker)
			reading, _ := track.SolidBody()
			if reading.Measurement.Source != MeasurementNearEdgeCandidateV1 || f.occluded {
				continue
			}
			lateral = append(lateral, math.Abs(float64(track.Y)-f.truthY))
			along = append(along, math.Abs(float64(track.X)-f.truthX))
			speed = append(speed, math.Abs(math.Hypot(float64(track.VX), float64(track.VY))-12))
		}
		n := len(lateral) / 2
		return pass{meanOf(lateral[n:]), meanOf(along[n:]), meanOf(speed[n:])}
	}
	off, on := run(0), run(1)
	t.Logf("settled errors without T5: lateral %.3f m, along %.3f m, speed %.3f m/s; with T5: %.3f m, %.3f m, %.3f m/s",
		off.lateral, off.along, off.speed, on.lateral, on.along, on.speed)
	if on.lateral > 0.15 || math.Abs(on.lateral-off.lateral) > 0.005 {
		t.Errorf("settled lateral error %.3f m with T5 against %.3f m without", on.lateral, off.lateral)
	}
	if on.along > off.along+0.02 || on.speed > off.speed+0.05 {
		t.Errorf("T5 worsened the along-track position (%.3f against %.3f m) or the speed (%.3f against %.3f m/s)",
			on.along, off.along, on.speed, off.speed)
	}
}

func TestARankOneMedoidLaneChangeKeepsItsMagnitude(t *testing.T) {
	// G-GEO-1 row 4 again, with T5: pulling the open direction toward the
	// medoid must not flatten a real manoeuvre.
	pass := l4perception.DefaultSyntheticPass()
	pass.Occluder = l4perception.SyntheticOccluder{}
	pass.Frames = 70
	pass.Vehicle.StartXMetres = -36
	pass.Vehicle.LaneChangeMetres = 3.5
	pass.Vehicle.LaneChangeStartSecs = 2.5
	pass.Vehicle.LaneChangeDurationSecs = 2.5
	frames := syntheticPassFrames(t, pass)
	for _, scale := range []float32{1, 0.25} {
		tracker := NewTracker(rankOneTrackingConfig(scale))
		var before, after []float64
		for i, f := range frames {
			tracker.Update(f.clusters, f.at)
			if len(tracker.GetActiveTracks()) == 0 {
				continue
			}
			track := mainTrack(t, tracker)
			if track.PositionReference() != ReferenceBodyCentre {
				continue
			}
			at := float64(i) * pass.Interval.Seconds()
			switch {
			case at >= 1.5 && at < 2.5:
				before = append(before, float64(track.Y))
			case at >= 5.5:
				after = append(after, float64(track.Y))
			}
		}
		if len(before) < 5 || len(after) < 5 {
			t.Fatalf("scale %v: too few body-centre frames either side of the manoeuvre: %d and %d", scale, len(before), len(after))
		}
		moved := meanOf(after) - meanOf(before)
		t.Logf("scale %v: tracked lateral displacement %.3f m of 3.5 m", scale, moved)
		if moved < 0.9*3.5 || moved > 1.1*3.5 {
			t.Errorf("scale %v: tracked lateral displacement %.3f m, want within 10%% of 3.5 m", scale, moved)
		}
	}
}

func TestTheShadowTakesTheRankOneMedoidToo(t *testing.T) {
	// T5 is the state machine's, so the shadow runs it as well, and it
	// still never changes the tracks.
	base := solidBodyConfig()
	base.SolidBody.FaceHysteresis, base.SolidBody.CourseAlignedFaces = true, true
	cfg := base
	cfg.SolidBody.RankOneMedoidScale = 1
	shadow, plain := NewTracker(cfg), NewTracker(base)
	moved := false
	for _, f := range syntheticPassFrames(t, endOnPass()) {
		shadow.Update(f.clusters, f.at)
		plain.Update(f.clusters, f.at)
		a, b := mainTrack(t, shadow), mainTrack(t, plain)
		if a.X != b.X || a.Y != b.Y || a.P != b.P {
			t.Fatal("the shadow's T5 changed the tracked state")
		}
		ra, _ := a.SolidBody()
		rb, _ := b.SolidBody()
		if ra.Estimate.Y != rb.Estimate.Y {
			moved = true
		}
	}
	if !moved {
		t.Fatal("the shadow's T5 never moved the body along its open direction")
	}
}
