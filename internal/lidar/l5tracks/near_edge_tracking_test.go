package l5tracks

import (
	"math"
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Tests for the tracked near-edge update (S2.2). What carries the weight: the
// option changes nothing unless the solid body is on; under it the tracked
// state is the body centre, a re-reference is a translation that leaves
// velocity alone, a lapse returns to the medoid, a lane change keeps its
// magnitude, and association compares a body-centre prediction with a
// measurement of the same point.

func nearEdgeTrackingConfig() TrackerConfig {
	cfg := solidBodyConfig()
	cfg.NearEdgeTracking = true
	return cfg
}

func TestNearEdgeTrackingIsOffByDefault(t *testing.T) {
	if DefaultTrackerConfig().NearEdgeTracking {
		t.Fatal("near-edge tracking is on by default")
	}
}

func TestNearEdgeTrackingWithoutTheSolidBodyChangesNothing(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.NearEdgeTracking = true
	on, off := NewTracker(cfg), NewTracker(DefaultTrackerConfig())
	for _, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
		on.Update(f.clusters, f.at)
		off.Update(f.clusters, f.at)
		if !reflect.DeepEqual(associationsBySequence(on), associationsBySequence(off)) {
			t.Fatal("associations differ with near-edge tracking on and no solid body")
		}
	}
	a, b := mainTrack(t, on), mainTrack(t, off)
	if a.X != b.X || a.Y != b.Y || a.VX != b.VX || a.VY != b.VY || a.P != b.P {
		t.Fatalf("tracked state differs: (%v, %v) against (%v, %v)", a.X, a.Y, b.X, b.Y)
	}
	if _, ok := a.SolidBody(); ok {
		t.Fatal("a solid body appeared without the option that makes one")
	}
}

func TestNearEdgeTrackingPutsTheTrackedStateOnTheBodyCentre(t *testing.T) {
	// The shadow's anchor result, now in the tracked state itself: once the
	// first fix re-references it, the track's own position is the body
	// centre, the solid body reads exactly that state, and the track's
	// reference says so.
	tracker := NewTracker(nearEdgeTrackingConfig())
	var errs []float64
	fixes := 0
	for _, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
		tracker.Update(f.clusters, f.at)
		track := mainTrack(t, tracker)
		reading, ok := track.SolidBody()
		if !ok {
			t.Fatal("no solid body under near-edge tracking")
		}
		if reading.Estimate.X != track.X || reading.Estimate.Y != track.Y || reading.VX != track.VX ||
			reading.Covariance != track.P {
			t.Fatalf("the solid body (%v, %v) is not the tracked state (%v, %v)",
				reading.Estimate.X, reading.Estimate.Y, track.X, track.Y)
		}
		if got := track.PositionReference(); got != reading.Estimate.Reference {
			t.Fatalf("track reference %s, solid body %s", got, reading.Estimate.Reference)
		}
		if reading.Measurement.Source != MeasurementNearEdgeCandidateV1 || f.occluded {
			continue
		}
		fixes++
		if track.LastMeasurementSource != MeasurementNearEdgeCandidateV1 {
			t.Fatalf("a fix recorded %s as the geometry that entered the filter", track.LastMeasurementSource)
		}
		errs = append(errs, math.Abs(float64(track.Y)-f.truthY))
	}
	if fixes < 20 {
		t.Fatalf("only %d tracked fixes on a forty-frame pass", fixes)
	}
	settled := meanOf(errs[len(errs)/2:])
	t.Logf("settled tracked lateral error %.3f m over %d fixes", settled, fixes)
	if settled > 0.15 {
		t.Errorf("tracked lateral error %.3f m, want under 0.15 m once settled", settled)
	}
	if got := tracker.SolidBodyReferenceChanges(); got.ToBodyCentre < 1 {
		t.Errorf("reference changes %+v: the first fix was not counted", got)
	}
}

func TestTheTrackedReReferenceIsATranslationThatLeavesVelocityAlone(t *testing.T) {
	// Without the translation the first fix absorbs half a body as an
	// innovation, and the position-velocity correlation carries some of it
	// into velocity. The translation moves the position by the measured
	// offset first, so the fix's innovation is only the prediction's error.
	jump := func(cfg TrackerConfig) (dv float64, change ReferenceChange) {
		tracker := NewTracker(cfg)
		var before [2]float32
		for _, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
			tracker.Update(f.clusters, f.at)
			track := mainTrack(t, tracker)
			reading, _ := track.SolidBody()
			if reading.Measurement.ReferenceChange == ReferenceToBodyCentre {
				return math.Hypot(float64(reading.VX-before[0]), float64(reading.VY-before[1])), ReferenceToBodyCentre
			}
			before = [2]float32{reading.VX, reading.VY}
		}
		return 0, ReferenceUnchanged
	}
	shadow := solidBodyConfig()
	plain, change := jump(shadow)
	if change != ReferenceToBodyCentre {
		t.Fatal("the shadow never re-referenced")
	}
	shadow.SolidBody.ReferenceTranslation = true
	translated, _ := jump(shadow)
	tracked, _ := jump(nearEdgeTrackingConfig())
	t.Logf("velocity change at the re-reference: shadow %.3f m/s, translated shadow %.3f m/s, tracked %.3f m/s",
		plain, translated, tracked)
	if translated >= plain {
		t.Errorf("the translation did not reduce the velocity change at the re-reference: %.3f against %.3f m/s", translated, plain)
	}
	// The tracked filter's velocity is the medoid-updated one until the fix,
	// so it is not the shadow's to compare with; at 12 m/s, a change of
	// under 0.1 m/s at the re-reference is no kick.
	if tracked >= 0.1 {
		t.Errorf("the tracked re-reference moved velocity by %.3f m/s", tracked)
	}
}

func TestTranslateToFacesMovesAlongTheNormalsOnly(t *testing.T) {
	cluster := WorldCluster{CentroidX: 10, CentroidY: 4}
	state := [4]float32{9.5, 4.2, 12, -1}
	lateral := EdgeMeasurement{Face: FaceRight, NormalX: 0, NormalY: -1, PlaneOffsetMetres: -4.1, HalfExtentMetres: 0.9}
	got := translateToFaces(state, []EdgeMeasurement{lateral}, cluster)
	// The face implies the centre at y = 4.1 + 0.9 = 5.0 along -Y; the
	// medoid projects to y = 4, so the state moves 1 m in +Y and not at all
	// in X, and velocity is untouched.
	if got[0] != state[0] || got[2] != state[2] || got[3] != state[3] {
		t.Fatalf("translation moved x or velocity: %v -> %v", state, got)
	}
	if math.Abs(float64(got[1]-state[1])-1) > 1e-6 {
		t.Fatalf("translated y by %v, want 1 m toward the implied centre", got[1]-state[1])
	}
	longitudinal := EdgeMeasurement{Face: FaceRear, NormalX: -1, NormalY: 0, PlaneOffsetMetres: -8, HalfExtentMetres: 2.25}
	got = translateToFaces(state, []EdgeMeasurement{lateral, longitudinal}, cluster)
	if math.Abs(float64(got[0]-state[0])-0.25) > 1e-6 || math.Abs(float64(got[1]-state[1])-1) > 1e-6 {
		t.Fatalf("rank-two translation %v -> %v, want (+0.25, +1)", state, got)
	}
}

func TestNearEdgeTrackingLapsesToTheMedoidAndComesBack(t *testing.T) {
	cfg := nearEdgeTrackingConfig()
	tracker := NewTracker(cfg)
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	const faceless = 14
	for _, f := range frames[:faceless] {
		tracker.Update(f.clusters, f.at)
	}
	if got := mainTrack(t, tracker).PositionReference(); got != ReferenceBodyCentre {
		t.Fatalf("setup: tracked reference %s before the faceless run", got)
	}
	var velocity [2]float32
	for i, f := range frames[faceless : faceless+cfg.MaxMisses] {
		for c := range f.clusters {
			f.clusters[c].RetainedPoints = nil
		}
		track := mainTrack(t, tracker)
		velocity = [2]float32{track.VX, track.VY}
		tracker.Update(f.clusters, f.at)
		track = mainTrack(t, tracker)
		r, _ := track.SolidBody()
		if i < cfg.MaxMisses-1 {
			if r.Estimate.Reference != ReferenceBodyCentre || r.Estimate.Support.Instant != SupportCoasted {
				t.Fatalf("faceless frame %d: %s, support %+v; want a body-centre coast", i, r.Estimate.Reference, r.Estimate.Support)
			}
			continue
		}
		if r.Measurement.ReferenceChange != ReferenceToMedoid || track.PositionReference() != ReferenceClusterMedoid {
			t.Fatalf("after %d faceless frames: %+v, reference %s; want the lapse", cfg.MaxMisses, r.Measurement, track.PositionReference())
		}
		if track.X != f.clusters[0].CentroidX || track.Y != f.clusters[0].CentroidY {
			t.Fatalf("tracked state lapsed to (%v, %v), not the medoid", track.X, track.Y)
		}
		if track.VX != velocity[0] || track.VY != velocity[1] {
			t.Fatalf("the lapse changed the tracked velocity (%v, %v) -> (%v, %v)", velocity[0], velocity[1], track.VX, track.VY)
		}
		if track.LastMeasurementSource != MeasurementMedoidV0 {
			t.Fatalf("a lapse recorded %s as the geometry that entered the filter", track.LastMeasurementSource)
		}
	}
	if got := tracker.SolidBodyReferenceChanges(); got.ToMedoid != 1 {
		t.Fatalf("reference changes %+v, want one lapse", got)
	}
	tracker.Update(frames[faceless+cfg.MaxMisses].clusters, frames[faceless+cfg.MaxMisses].at)
	if got := mainTrack(t, tracker).PositionReference(); got != ReferenceBodyCentre {
		t.Fatalf("faces returned but the tracked reference stayed %s", got)
	}
}

func TestNearEdgeTrackingCoastsWithTheTrackedFilter(t *testing.T) {
	tracker := NewTracker(nearEdgeTrackingConfig())
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	for _, f := range frames[:12] {
		tracker.Update(f.clusters, f.at)
	}
	tracker.Update(nil, frames[12].at)
	track := mainTrack(t, tracker)
	r, ok := track.SolidBody()
	if !ok || r.Measurement.FallbackReason != "no_association" {
		t.Fatalf("missed frame: %+v", r.Measurement)
	}
	if r.Estimate.X != track.X || r.Covariance != track.P {
		t.Fatal("the coasting solid body is not the tracked state")
	}
}

func TestATrackedLaneChangeKeepsItsMagnitude(t *testing.T) {
	// G-GEO-1 row 4, synthetic: a 3.5 m lane change away from the sensor at
	// 12 m/s must keep at least 90 % of its lateral magnitude in the tracked
	// state. A smoother that bought steadiness by flattening the manoeuvre
	// would fail here.
	pass := l4perception.DefaultSyntheticPass()
	pass.Occluder = l4perception.SyntheticOccluder{}
	pass.Frames = 70
	pass.Vehicle.StartXMetres = -36
	pass.Vehicle.LaneChangeMetres = 3.5
	pass.Vehicle.LaneChangeStartSecs = 2.5
	pass.Vehicle.LaneChangeDurationSecs = 2.5
	frames := syntheticPassFrames(t, pass)
	tracker := NewTracker(nearEdgeTrackingConfig())
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
		t.Fatalf("too few body-centre frames either side of the manoeuvre: %d and %d", len(before), len(after))
	}
	moved := meanOf(after) - meanOf(before)
	t.Logf("tracked lateral displacement %.3f m of 3.5 m", moved)
	if moved < 0.9*3.5 || moved > 1.1*3.5 {
		t.Errorf("tracked lateral displacement %.3f m, want within 10%% of 3.5 m", moved)
	}
}

func TestTheTrackedStepReusesThePairsMeasurement(t *testing.T) {
	tracker := NewTracker(nearEdgeTrackingConfig())
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	for _, f := range frames[:15] {
		tracker.Update(f.clusters, f.at)
	}
	track := mainTrack(t, tracker)
	if track.solidBody.reference != ReferenceBodyCentre {
		t.Fatal("setup: not on the body centre")
	}
	// A frame the gate measured is taken, not measured again.
	tracker.nearEdgePairs = map[nearEdgePairKey]nearEdgeFrame{
		{track, 0}: {fallback: "measured_by_the_gate"},
	}
	tracker.attachNearEdgePair(track, 0)
	tracker.stepTrackedNearEdge(track, frames[15].clusters[0])
	if got := track.solidBody.pending.m.FallbackReason; got != "measured_by_the_gate" {
		t.Fatalf("the step measured the pair again (fallback %q)", got)
	}
	if track.solidBody.hasPair {
		t.Fatal("the pair outlived its frame")
	}
	// A pairing the gate never measured is measured by the step.
	tracker.stepTrackedNearEdge(track, frames[15].clusters[0])
	if got := track.solidBody.pending.m.FallbackReason; got == "measured_by_the_gate" {
		t.Fatal("an unattached step took a stale pair")
	}
	// An untracked body takes no pair.
	shadow := NewTracker(solidBodyConfig())
	shadowTrack := &TrackedObject{}
	shadow.attachNearEdgePair(shadowTrack, 3)
	if shadowTrack.solidBody.hasPair {
		t.Fatal("a shadow body took a pair")
	}
}

func TestCompletingATrackedFrameWithoutAStepSaysSo(t *testing.T) {
	tracker := NewTracker(nearEdgeTrackingConfig())
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	for _, f := range frames[:3] {
		tracker.Update(f.clusters, f.at)
	}
	track := mainTrack(t, tracker)
	tracker.completeTrackedSolidBody(track, frames[3].clusters[0])
	r, _ := track.SolidBody()
	if r.Measurement.FallbackReason != "no_tracked_step" || r.Estimate.Support.Instant != SupportCoasted {
		t.Fatalf("completion without a step: %+v, support %+v", r.Measurement, r.Estimate.Support)
	}
	// A medoid step whose tracked update was refused is not an observation.
	track.solidBody.pending = nearEdgePending{valid: true, outcome: nearEdgeMedoid}
	track.LastResidual.Valid = false
	tracker.completeTrackedSolidBody(track, frames[3].clusters[0])
	if r, _ := track.SolidBody(); r.Measurement.Source != "" || r.Estimate.Support.Instant != SupportCoasted {
		t.Fatalf("a refused medoid update read as measured: %+v", r.Measurement)
	}
}

func TestNearEdgeTrackingDeletesATrackWhoseStepIsNotFinite(t *testing.T) {
	tracker := NewTracker(nearEdgeTrackingConfig())
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	for _, f := range frames[:15] {
		tracker.Update(f.clusters, f.at)
	}
	track := mainTrack(t, tracker)
	track.P[0] = float32(math.NaN())
	tracker.nearEdgePairs = map[nearEdgePairKey]nearEdgeFrame{{track, 0}: {fallback: "faceless"}}
	track.solidBody.support.CoastedFrames = 0
	tracker.attachNearEdgePair(track, 0)
	tracker.update(track, frames[15].clusters[0], frames[15].at.UnixNano())
	if track.TrackState != TrackDeleted {
		t.Fatalf("a non-finite tracked step left the track %s", track.TrackState)
	}
}

func TestSwitchingNearEdgeTrackingDiscardsExistingBodies(t *testing.T) {
	tracker := NewTracker(solidBodyConfig())
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	for _, f := range frames[:5] {
		tracker.Update(f.clusters, f.at)
	}
	tracker.UpdateConfig(func(c *TrackerConfig) { c.NearEdgeTracking = true })
	if _, ok := mainTrack(t, tracker).SolidBody(); ok {
		t.Fatal("a shadow body survived the switch to near-edge tracking")
	}
	tracker.Update(frames[5].clusters, frames[5].at)
	if !mainTrack(t, tracker).solidBody.tracked {
		t.Fatal("the reseeded body is not the tracked one")
	}
	tracker.Reset()
	if got := tracker.SolidBodyReferenceChanges(); got != (SolidBodyReferenceChanges{}) {
		t.Fatalf("reset kept reference changes %+v", got)
	}
}

func TestReferenceChangeNames(t *testing.T) {
	for c, want := range map[ReferenceChange]string{
		ReferenceUnchanged: "", ReferenceToBodyCentre: "medoid_to_body_centre", ReferenceToMedoid: "body_centre_to_medoid",
	} {
		if got := c.String(); got != want {
			t.Errorf("%d names %q, want %q", c, got, want)
		}
	}
}

// a2Track is a confirmed body-centre tracked body at the origin heading +X,
// with an isotropic position variance, believing a 4.5 x 1.8 m car.
func a2Track() *TrackedObject {
	track := &TrackedObject{}
	track.TrackState, track.ObservationCount = TrackConfirmed, 10
	track.P = [16]float32{0.04, 0, 0, 0, 0, 0.04, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	sb := &track.solidBody
	sb.seeded, sb.tracked, sb.reference = true, true, ReferenceBodyCentre
	sb.orientation = OrientationBelief{PsiRad: 0, Provenance: ProvenanceObserved}
	for i := 0; i < 10; i++ {
		sb.lengthBelief.Observe(4.5)
		sb.widthBelief.Observe(1.8)
	}
	return track
}

func TestA2GatesOnTheFaceAlongItsNormalAndLoosensTheMedoidAcross(t *testing.T) {
	tracker := NewTracker(nearEdgeTrackingConfig())
	track := a2Track()
	// The medoid of a body seen side-on sits on the near side, 0.9 m from
	// the centre toward the sensor, and anywhere along the length.
	cluster := WorldCluster{CentroidX: 2.0, CentroidY: -0.9}
	face := EdgeMeasurement{Face: FaceRight, NormalX: 0, NormalY: -1, PlaneOffsetMetres: 0.9, HalfExtentMetres: 0.9,
		HalfExtentProvenance: ProvenanceAccumulated}
	frame := nearEdgeFrame{edges: EdgeMeasurementSet{Rank: 1, Edges: []EdgeMeasurement{face}},
		length: DimensionBelief{Metres: 4.5}, width: DimensionBelief{Metres: 1.8}}
	a2 := tracker.faceResidualDistanceSquared(track, cluster, frame)
	medoid := tracker.mahalanobisDistanceSquared(track, cluster, 0.1)
	t.Logf("d²: A2 %.3f, medoid %.3f", a2, medoid)
	if a2 > 1 {
		t.Errorf("A2 d² %.3f for a pair whose face agrees exactly; the along-body medoid offset should be loose", a2)
	}
	if medoid <= tracker.Config.GatingDistanceSquared {
		t.Errorf("setup: the medoid gate admits the pair (d² %.3f), so the test shows nothing", medoid)
	}
	// Moving the face 1 m from where the track predicts it is not loose.
	face.PlaneOffsetMetres = 1.9
	frame.edges.Edges[0] = face
	if got := tracker.faceResidualDistanceSquared(track, cluster, frame); got < 10 {
		t.Errorf("A2 d² %.3f for a face 1 m from its prediction", got)
	}
	// Rank two: both directions are face residuals with the tracked noise.
	face.PlaneOffsetMetres = 0.9
	rear := EdgeMeasurement{Face: FaceRear, NormalX: -1, NormalY: 0, PlaneOffsetMetres: 2.25, HalfExtentMetres: 2.25,
		HalfExtentProvenance: ProvenanceAccumulated}
	frame.edges = EdgeMeasurementSet{Rank: 2, Edges: []EdgeMeasurement{face, rear}}
	if got := tracker.faceResidualDistanceSquared(track, cluster, frame); got > 1e-6 {
		t.Errorf("rank-two A2 d² %.6f for faces that agree exactly", got)
	}
	// No usable face: the medoid in both directions, loosened by each
	// half-extent, from the beliefs even when the frame measured nothing.
	none := nearEdgeFrame{fallback: "no_face"}
	loose := tracker.faceResidualDistanceSquared(track, cluster, none)
	if loose >= medoid || loose > tracker.Config.GatingDistanceSquared {
		t.Errorf("faceless A2 d² %.3f, medoid %.3f: the loose medoid should admit what the tight one refuses", loose, medoid)
	}
	// A prior-only face constrains nothing.
	faceless := nearEdgeFrame{length: frame.length, width: frame.width}
	prior := face
	prior.HalfExtentProvenance = ProvenanceClassPrior
	frame.edges = EdgeMeasurementSet{Rank: 1, Edges: []EdgeMeasurement{prior}}
	if got, want := tracker.faceResidualDistanceSquared(track, cluster, frame),
		tracker.faceResidualDistanceSquared(track, cluster, faceless); got != want {
		t.Errorf("a prior-only face changed the A2 distance: %.4f against %.4f", got, want)
	}
}

func TestA2RefusesASingularResidualCovariance(t *testing.T) {
	tracker := NewTracker(nearEdgeTrackingConfig())
	tracker.Config.MeasurementNoise = 0
	track := a2Track()
	track.P = [16]float32{}
	face := EdgeMeasurement{Face: FaceRight, NormalX: 0, NormalY: -1, PlaneOffsetMetres: 0.9, HalfExtentProvenance: ProvenanceAccumulated}
	rear := EdgeMeasurement{Face: FaceRear, NormalX: -1, NormalY: 0, PlaneOffsetMetres: 2.25, HalfExtentProvenance: ProvenanceAccumulated}
	frame := nearEdgeFrame{edges: EdgeMeasurementSet{Rank: 2, Edges: []EdgeMeasurement{face, rear}}}
	if got := tracker.faceResidualDistanceSquared(track, WorldCluster{}, frame); got != SingularDistanceRejection {
		t.Fatalf("d² %v for a zero residual covariance, want the rejection", got)
	}
}

func TestTheGateUsesA2OnlyForABodyCentreTrackedBody(t *testing.T) {
	tracker := NewTracker(nearEdgeTrackingConfig())
	track := a2Track()
	far := WorldCluster{CentroidX: 100}
	if got := tracker.gateDistanceSquared(track, []WorldCluster{far}, 0, 0.1); got != SingularDistanceRejection {
		t.Errorf("an implausible jump gave d² %v under A2", got)
	}
	if len(tracker.nearEdgePairs) != 0 {
		t.Error("an implausible pairing was measured")
	}
	near := WorldCluster{CentroidX: 0.5, CentroidY: -0.9}
	first := tracker.gateDistanceSquared(track, []WorldCluster{near}, 0, 0.1)
	if _, ok := tracker.nearEdgePairs[nearEdgePairKey{track, 0}]; !ok {
		t.Fatal("the gate did not keep the pair's measurement")
	}
	if again := tracker.gateDistanceSquared(track, []WorldCluster{near}, 0, 0.1); again != first {
		t.Errorf("the kept measurement gave d² %v, then %v", first, again)
	}
	track.solidBody.reference = ReferenceClusterMedoid
	if got, want := tracker.gateDistanceSquared(track, []WorldCluster{near}, 0, 0.1),
		tracker.mahalanobisDistanceSquared(track, near, 0.1); got != want {
		t.Errorf("a medoid-referenced body gated at %v, the medoid gate says %v", got, want)
	}
}

func TestTheIdentitySceneIsUnchangedWithoutFaces(t *testing.T) {
	// The scripted identity scenes carry no member geometry, so under
	// near-edge tracking every body stays on the medoid and the tracks must
	// be exactly the default's: the option cannot change identity where it
	// has nothing to measure.
	pass := l4perception.DefaultSyntheticPass()
	frames := syntheticPassFrames(t, pass)
	on, off := NewTracker(nearEdgeTrackingConfig()), NewTracker(DefaultTrackerConfig())
	for _, f := range frames {
		stripped := make([]WorldCluster, len(f.clusters))
		for i, c := range f.clusters {
			c.RetainedPoints, c.Members = nil, nil
			stripped[i] = c
		}
		on.Update(stripped, f.at)
		off.Update(stripped, f.at)
		if !reflect.DeepEqual(associationsBySequence(on), associationsBySequence(off)) {
			t.Fatal("associations differ with no geometry to measure")
		}
	}
	a, b := mainTrack(t, on), mainTrack(t, off)
	if a.X != b.X || a.Y != b.Y || a.P != b.P {
		t.Fatalf("tracked state differs without faces: (%v, %v) against (%v, %v)", a.X, a.Y, b.X, b.Y)
	}
}

func TestTheTrackedRecordMatchesWhatTheStepApplied(t *testing.T) {
	// Every associated frame leaves a residual: a fix records the faces'
	// measurement at the position they updated, so the re-reference
	// translation is never an innovation and nothing moves across a face;
	// a faceless frame and a lapse say the observation was not applied.
	cfg := nearEdgeTrackingConfig()
	tracker := NewTracker(cfg)
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	sawReReference := false
	var previousY float32
	for _, f := range frames[:14] {
		tracker.Update(f.clusters, f.at)
		track := mainTrack(t, tracker)
		lastY := previousY
		previousY = track.Y
		r, _ := track.SolidBody()
		res := track.LastResidual
		if track.ObservationCount <= 1 {
			continue // born this frame: seeded, not updated
		}
		if !res.Valid {
			t.Fatalf("an associated frame left no residual (%+v)", r.Measurement)
		}
		if r.Measurement.Source != MeasurementNearEdgeCandidateV1 {
			continue
		}
		if res.Disposition != "" || res.Measurement.Source != MeasurementNearEdgeCandidateV1 || res.NIS != r.Measurement.NIS {
			t.Fatalf("fix residual %+v for measurement %+v", res, r.Measurement)
		}
		// The synthetic body is seen side-on: its one face constrains y.
		if r.Measurement.Rank == 1 && math.Abs(float64(res.InnovationX)) > 1e-5 {
			t.Fatalf("a rank-one fix recorded an innovation of %v across its face", res.InnovationX)
		}
		if r.Measurement.ReferenceChange == ReferenceToBodyCentre {
			sawReReference = true
			// Untranslated, the innovation would be the whole move from the
			// medoid-referenced position to the implied centre; translated,
			// it is only the prediction's error against the medoid.
			whole := res.Measurement.Y - lastY
			t.Logf("re-reference: recorded innovation %.3f m, whole move %.3f m, translation %.3f m",
				res.InnovationY, whole, res.PredictedY-lastY)
			if math.Abs(float64(res.InnovationY)) >= math.Abs(float64(whole)) {
				t.Fatalf("the re-reference recorded %v m of innovation for a %v m move: the translation was counted", res.InnovationY, whole)
			}
		}
	}
	if !sawReReference {
		t.Fatal("no re-reference in the first fourteen frames")
	}
	lengthBefore := mainTrack(t, tracker).TrackLengthMeters
	for i, f := range frames[14 : 14+cfg.MaxMisses] {
		for c := range f.clusters {
			f.clusters[c].RetainedPoints = nil
		}
		tracker.Update(f.clusters, f.at)
		track := mainTrack(t, tracker)
		res := track.LastResidual
		want := ResidualNotApplied
		if i == cfg.MaxMisses-1 {
			want = ResidualReferenceChanged
		}
		if !res.Valid || res.Disposition != want || res.Reason == "" || !(res.NIS > 0) {
			t.Fatalf("faceless frame %d: residual %+v, want disposition %s with a reason and an A2 distance", i, res, want)
		}
	}
	// The lapse translated the trail with the position, so the track did
	// not travel the half-body it moved by.
	track := mainTrack(t, tracker)
	travelled := track.TrackLengthMeters - lengthBefore
	elapsed := float32(cfg.MaxMisses) * 0.1
	if travelled > 12*elapsed*1.2 {
		t.Fatalf("the track travelled %.2f m in %.1f s at 12 m/s: a reference change was counted as motion", travelled, elapsed)
	}
	last := track.History[len(track.History)-1]
	if last.X != track.X || last.Y != track.Y {
		t.Fatalf("the trail ends at (%v, %v), the track is at (%v, %v)", last.X, last.Y, track.X, track.Y)
	}
}

func TestShiftHistoryMovesTheWholeTrail(t *testing.T) {
	track := &TrackedObject{History: []TrackPoint{{X: 1, Y: 2}, {X: 3, Y: 4}}}
	shiftHistory(track, 0, 0)
	if track.History[0].X != 1 {
		t.Fatal("a zero shift moved the trail")
	}
	shiftHistory(track, 0.5, -1)
	if track.History[0].X != 1.5 || track.History[0].Y != 1 || track.History[1].X != 3.5 || track.History[1].Y != 3 {
		t.Fatalf("trail %+v after a (0.5, -1) shift", track.History)
	}
}

func TestApplicationOfRecordsTheTranslationAndTheFacesMeasurement(t *testing.T) {
	face := EdgeMeasurement{Face: FaceRight, NormalX: 0, NormalY: -1, PlaneOffsetMetres: -0.3, HalfExtentMetres: 0.9}
	a := applicationOf([4]float32{3, 0, 12, 0}, [4]float32{3, 1, 12, 0}, []EdgeMeasurement{face})
	// The face implies the centre at y = 1.2 along -Y; from the start at
	// y = 1 that is 0.2 m along the normal and nothing in x.
	if a.shiftX != 0 || a.shiftY != 1 || a.startY != 1 || a.measuredX != 3 || math.Abs(float64(a.measuredY)-1.2) > 1e-6 {
		t.Fatalf("application %+v", a)
	}
}

func TestAFacelessRecordAlwaysGivesAReason(t *testing.T) {
	tracker := NewTracker(nearEdgeTrackingConfig())
	track := a2Track()
	track.solidBody.pending = nearEdgePending{valid: true, outcome: nearEdgeCoast}
	tracker.recordTrackedNearEdgeResidual(track, trackedPrediction{p: track.P}, WorldCluster{CentroidX: 1}, PositionMeasurement{X: 1})
	if r := track.LastResidual; r.Disposition != ResidualNotApplied || r.Reason != "no_usable_face" {
		t.Fatalf("a faceless frame without a fallback reason filed %s/%q", r.Disposition, r.Reason)
	}
}

func TestA2CountsOneFacePerDirection(t *testing.T) {
	// Two faces along the same axis cannot both be near faces; if a
	// measurement ever offered them, the first stands and the other is not
	// a second constraint on the same direction.
	tracker := NewTracker(nearEdgeTrackingConfig())
	track := a2Track()
	right := EdgeMeasurement{Face: FaceRight, NormalX: 0, NormalY: -1, PlaneOffsetMetres: 0.9, HalfExtentMetres: 0.9,
		HalfExtentProvenance: ProvenanceAccumulated}
	left := right
	left.Face, left.NormalY, left.PlaneOffsetMetres = FaceLeft, 1, 5
	one := nearEdgeFrame{edges: EdgeMeasurementSet{Rank: 1, Edges: []EdgeMeasurement{right}},
		length: DimensionBelief{Metres: 4.5}, width: DimensionBelief{Metres: 1.8}}
	both := one
	both.edges = EdgeMeasurementSet{Rank: 1, Edges: []EdgeMeasurement{right, left}}
	cluster := WorldCluster{CentroidX: 2, CentroidY: -0.9}
	if a, b := tracker.faceResidualDistanceSquared(track, cluster, one), tracker.faceResidualDistanceSquared(track, cluster, both); a != b {
		t.Fatalf("a second face across the body changed d² from %v to %v", a, b)
	}
}

func TestTheShadowDropsABodyWhoseUpdateIsNotFinite(t *testing.T) {
	tracker := NewTracker(solidBodyConfig())
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	for _, f := range frames[:15] {
		tracker.Update(f.clusters, f.at)
	}
	track := mainTrack(t, tracker)
	track.solidBody.p[0] = float32(math.NaN())
	tracker.updateSolidBody(track, frames[15].clusters[0])
	if _, ok := track.SolidBody(); ok {
		t.Fatal("a non-finite shadow update kept its solid body")
	}
}

func TestAFrameWithoutAHeadingMeasuresNothing(t *testing.T) {
	tracker := NewTracker(solidBodyConfig())
	sb := &solidBodyTrack{}
	f := tracker.measureNearEdgeFrame(sb, WorldCluster{}, dimensionPriorFor(MotionUnknown), tracker.Config.HitsToConfirm+1)
	if f.fallback != "missing_heading" || len(f.edges.Edges) != 0 {
		t.Fatalf("frame %+v without a heading", f)
	}
}
