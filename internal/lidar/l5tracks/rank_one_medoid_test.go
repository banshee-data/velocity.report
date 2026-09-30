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

func TestASideFaceTakesTheMedoidAlongTheBody(t *testing.T) {
	// A side face constrains across the body and leaves its length open, so
	// the medoid's along-body offset is loosened by half the length.
	cluster := WorldCluster{CentroidX: 1.0, CentroidY: -0.9}
	on := stepRankOne(t, 1, []EdgeMeasurement{rankOneRight}, cluster, nil)
	want := 1.0 * 0.04 / (0.04 + on.noise + 2.25*2.25)
	if math.Abs(float64(on.sb.state[0])-want) > 1e-6 || on.sb.state[1] != 0 {
		t.Fatalf("state %v, want x %v and nothing across the face", on.sb.state, want)
	}
	if math.Abs(float64(on.applied.measuredX)-1) > 1e-6 {
		t.Fatalf("recorded x %v, want the medoid's 1", on.applied.measuredX)
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

func TestTheTrackedRankOneMedoidRecordsTheMedoidAcrossTheBody(t *testing.T) {
	// Seen from behind on a straight pass, the course is the x axis, so a
	// rank-one fix's open direction is y: with T5 it records the medoid's y
	// as what it measured across the body, and without it the prediction's.
	frames := syntheticPassFrames(t, endOnPass())
	for _, scale := range []float32{0, 1} {
		tracker := NewTracker(rankOneCorpusConfig(scale))
		rankOne := 0
		for _, f := range frames {
			tracker.Update(f.clusters, f.at)
			if len(tracker.GetActiveTracks()) == 0 {
				continue
			}
			track := mainTrack(t, tracker)
			reading, _ := track.SolidBody()
			if reading.Measurement.Source != MeasurementNearEdgeCandidateV1 || reading.Measurement.Rank != 1 {
				continue
			}
			rankOne++
			res := track.LastResidual
			want := res.PredictedY
			if scale > 0 {
				want = nearestCentroidY(f.clusters, res.Measurement.Y)
			}
			if math.Abs(float64(res.Measurement.Y-want)) > 0.02 {
				t.Fatalf("scale %v: a rank-one fix recorded y %v across the body, want %v", scale, res.Measurement.Y, want)
			}
		}
		if rankOne < 3 {
			t.Fatalf("scale %v: %d rank-one fixes; the end-on pass is not exercising rank one", scale, rankOne)
		}
	}
}

func nearestCentroidY(clusters []WorldCluster, y float32) float32 {
	best := clusters[0].CentroidY
	for _, c := range clusters[1:] {
		if math.Abs(float64(c.CentroidY-y)) < math.Abs(float64(best-y)) {
			best = c.CentroidY
		}
	}
	return best
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
	type drift struct{ mean, worst float64 }
	got := map[float32]drift{}
	for _, scale := range []float32{0, 1, 0.25} {
		tracker := NewTracker(rankOneCorpusConfig(scale))
		var errs []float64
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
		}
		if len(errs) < 20 {
			t.Fatalf("scale %v: only %d body-centre frames", scale, len(errs))
		}
		got[scale] = drift{meanOf(errs), worst}
		t.Logf("scale %v: lateral error mean %.3f m, worst %.3f m over %d body-centre frames", scale, got[scale].mean, worst, len(errs))
	}
	for _, scale := range []float32{1, 0.25} {
		if got[scale].worst > 0.6*got[0].worst || !(got[scale].mean < got[0].mean) {
			t.Errorf("scale %v: lateral error mean %.3f, worst %.3f m, against %.3f and %.3f m without T5",
				scale, got[scale].mean, got[scale].worst, got[0].mean, got[0].worst)
		}
	}
}

func TestTheTrackedRankOneMedoidLeavesASideOnPassAnchored(t *testing.T) {
	// The default pass is seen side-on and at a corner; T5 must not move its
	// settled lateral anchor off the body centre.
	tracker := NewTracker(rankOneTrackingConfig(1))
	var errs []float64
	for _, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
		tracker.Update(f.clusters, f.at)
		track := mainTrack(t, tracker)
		reading, _ := track.SolidBody()
		if reading.Measurement.Source != MeasurementNearEdgeCandidateV1 || f.occluded {
			continue
		}
		errs = append(errs, math.Abs(float64(track.Y)-f.truthY))
	}
	settled := meanOf(errs[len(errs)/2:])
	t.Logf("settled tracked lateral error %.3f m over %d fixes", settled, len(errs))
	if settled > 0.15 {
		t.Errorf("tracked lateral error %.3f m with T5, want under 0.15 m once settled", settled)
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
	cfg := solidBodyConfig()
	cfg.SolidBody.RankOneMedoidScale = 1
	shadow, plain := NewTracker(cfg), NewTracker(solidBodyConfig())
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
