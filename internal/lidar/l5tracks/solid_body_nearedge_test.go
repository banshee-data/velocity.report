package l5tracks

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Tests for populating the solid-body estimate from the near-edge measurement
// model. Two properties carry the weight: the solid body never changes what the
// tracker concludes, and on the synthetic pass it anchors the body centre where
// the tracked medoid is biased toward the sensor.

// syntheticPassFrame is one frame of the synthetic pass as the tracker sees it.
type syntheticPassFrame struct {
	at       time.Time
	clusters []WorldCluster
	truthX   float64
	truthY   float64
	occluded bool
}

// syntheticPassBase moves the pass off the Unix epoch: a zero capture time is
// the tracker's "no previous frame" sentinel.
var syntheticPassBase = time.Unix(1_700_000_000, 0).Sub(time.Unix(0, 0))

// syntheticPassFrames clusters the default pass with retained points, which is
// what the near-edge model reads.
func syntheticPassFrames(t *testing.T, pass l4perception.SyntheticPass) []syntheticPassFrame {
	t.Helper()
	params := l4perception.DefaultDBSCANParams()
	params.Eps = 0.6
	params.MinPts = 5
	params.MaxSamplePoints = 512
	var out []syntheticPassFrame
	for _, f := range l4perception.GenerateSyntheticPass(pass) {
		points := make([]l4perception.WorldPoint, len(f.Points))
		for i, p := range f.Points {
			p.Timestamp = p.Timestamp.Add(syntheticPassBase)
			points[i] = p
		}
		out = append(out, syntheticPassFrame{
			at:       f.Timestamp.Add(syntheticPassBase),
			clusters: l4perception.DBSCAN(points, params),
			truthX:   f.TrueCentreX,
			truthY:   f.TrueCentreY,
			occluded: f.Occluded,
		})
	}
	if len(out) == 0 {
		t.Fatal("the synthetic pass produced no frames")
	}
	return out
}

// syntheticOrigin declares the synthetic pass's sensor origin: the pass is
// generated in the sensor frame.
const syntheticOrigin = "test: synthetic pass in the sensor frame"

func solidBodyConfig() TrackerConfig {
	cfg := DefaultTrackerConfig()
	cfg.SolidBody = SolidBodyOptions{Enabled: true, OriginSource: syntheticOrigin}
	return cfg
}

// mainTrack returns the live track with the most observations: the vehicle's.
// The synthetic pass has one vehicle, but the tracked filter may drop and
// reacquire it early and seed short tracks on roof fragments near the
// occluder; which it does is its business, not the solid body's.
func mainTrack(t *testing.T, tracker *Tracker) *TrackedObject {
	t.Helper()
	var best *TrackedObject
	for _, tr := range tracker.GetActiveTracks() {
		if best == nil || tr.ObservationCount > best.ObservationCount ||
			(tr.ObservationCount == best.ObservationCount && tr.CreationSequence < best.CreationSequence) {
			best = tr
		}
	}
	if best == nil {
		t.Fatal("no live track on the synthetic pass")
	}
	return best
}

// associationsBySequence rewrites the last association of each cluster from
// the random track ID to the deterministic creation sequence.
func associationsBySequence(tracker *Tracker) []int64 {
	var out []int64
	for _, id := range tracker.GetLastAssociations() {
		if id == "" {
			out = append(out, 0)
			continue
		}
		out = append(out, tracker.GetTrack(id).CreationSequence)
	}
	return out
}

func TestSolidBodyIsOffByDefault(t *testing.T) {
	if DefaultTrackerConfig().SolidBody.Enabled {
		t.Fatal("the solid-body estimator must be opt-in")
	}
	tracker := NewTracker(DefaultTrackerConfig())
	for _, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass())[:6] {
		tracker.Update(f.clusters, f.at)
	}
	if _, ok := mainTrack(t, tracker).SolidBody(); ok {
		t.Fatal("a track reports a solid body with the option off")
	}
}

// comparableTrack is a track with the fields that differ by construction
// removed: the random UUID, and the solid body itself.
func comparableTrack(track *TrackedObject) TrackedObject {
	c := *track
	c.TrackID = ""
	c.solidBody = solidBodyTrack{}
	return c
}

func TestSolidBodyNeverFeedsBackIntoTheTrackedState(t *testing.T) {
	// The whole attribution argument for G-GEO-1 rests on this: switching the
	// solid body on must not move association, the tracked state or any
	// recorded field, so a difference between the two estimates is the
	// observation model's and nothing else's. Checked with the shipped
	// options and with every continuity option on, since those change how an
	// unobserved frame is charged, and with the face-transition remedies on,
	// since those keep state of their own.
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	continuity := DefaultTrackerConfig()
	continuity.OcclusionContinuity = DefaultOcclusionContinuity()
	for name, c := range map[string]struct {
		base TrackerConfig
		body SolidBodyOptions
	}{
		"shipped":    {DefaultTrackerConfig(), SolidBodyOptions{Enabled: true, OriginSource: syntheticOrigin}},
		"continuity": {continuity, SolidBodyOptions{Enabled: true, OriginSource: syntheticOrigin}},
		"remedies": {DefaultTrackerConfig(), SolidBodyOptions{Enabled: true, OriginSource: syntheticOrigin,
			FaceHysteresis: true, FaceEntryConsider: true, CourseAlignedFaces: true}},
		"undeclared": {DefaultTrackerConfig(), SolidBodyOptions{Enabled: true}},
	} {
		base := c.base
		withBody := base
		withBody.SolidBody = c.body
		off, on := NewTracker(base), NewTracker(withBody)
		for i, f := range frames {
			off.Update(f.clusters, f.at)
			on.Update(f.clusters, f.at)
			a, b := off.GetAllTracks(), on.GetAllTracks()
			if len(a) != len(b) {
				t.Fatalf("%s frame %d: %d tracks with the option off, %d with it on", name, i, len(a), len(b))
			}
			byID := map[int64]*TrackedObject{}
			for _, tr := range b {
				byID[tr.CreationSequence] = tr
			}
			for _, tr := range a {
				other, ok := byID[tr.CreationSequence]
				if !ok {
					t.Fatalf("%s frame %d: track %d exists only with the option off", name, i, tr.CreationSequence)
				}
				if !reflect.DeepEqual(comparableTrack(tr), comparableTrack(other)) {
					t.Fatalf("%s frame %d: track %d differs with the solid body on:\noff %+v\n on %+v",
						name, i, tr.CreationSequence, comparableTrack(tr), comparableTrack(other))
				}
			}
			if !reflect.DeepEqual(associationsBySequence(off), associationsBySequence(on)) {
				t.Fatalf("%s frame %d: associations differ with the solid body on", name, i)
			}
		}
		if _, ok := mainTrack(t, on).SolidBody(); !ok {
			t.Fatalf("%s: the option was on but no solid body was populated", name)
		}
	}
}

func TestSolidBodyWaitsOutTheInitialisationWindowOnTheMedoid(t *testing.T) {
	// Phase 2's mitigation for the heading/face circularity: for the first
	// HitsToConfirm observations the heading that would select a face is
	// itself unconverged, so the medoid updates the body and the body stays
	// referenced to it.
	cfg := solidBodyConfig()
	tracker := NewTracker(cfg)
	checked := 0
	for i, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
		tracker.Update(f.clusters, f.at)
		for _, track := range tracker.GetActiveTracks() {
			if track.Misses != 0 || track.ObservationCount > cfg.HitsToConfirm {
				continue
			}
			reading, ok := track.SolidBody()
			if !ok {
				t.Fatalf("frame %d: track %d has no solid body", i, track.CreationSequence)
			}
			if reading.Estimate.Reference != ReferenceClusterMedoid {
				t.Fatalf("frame %d: reference %s inside the initialisation window", i, reading.Estimate.Reference)
			}
			m := reading.Measurement
			if m.Source != MeasurementMedoidV0 || m.Rank != 2 || m.FallbackReason != "initialisation_window" {
				t.Fatalf("frame %d: measurement %+v, want the medoid with the initialisation reason", i, m)
			}
			if reading.Estimate.Width.Provenance != ProvenanceClassPrior {
				t.Fatalf("frame %d: width %s before any face was measured", i, reading.Estimate.Width.Provenance)
			}
			checked++
		}
	}
	if checked < cfg.HitsToConfirm {
		t.Fatalf("only %d observed frames inside an initialisation window", checked)
	}
}

func TestSolidBodyAnchorsTheBodyCentreWhereTheMedoidIsBiased(t *testing.T) {
	// Section 3's defect and its correction, through the tracker: the tracked
	// medoid sits on the sensor-facing side of the vehicle, while the solid
	// body reconstructs the centre from the near face and the believed width.
	tracker := NewTracker(solidBodyConfig())
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())

	var solidLateral, trackedLateral []float64
	sawBodyCentre := false
	for _, f := range frames {
		tracker.Update(f.clusters, f.at)
		track := mainTrack(t, tracker)
		reading, ok := track.SolidBody()
		if !ok {
			t.Fatal("no solid body")
		}
		if reading.Estimate.Reference != ReferenceBodyCentre {
			if sawBodyCentre {
				t.Fatalf("the solid body went back to %s after a body-centre fix", reading.Estimate.Reference)
			}
			continue
		}
		sawBodyCentre = true
		if f.occluded || reading.Measurement.Source != MeasurementNearEdgeCandidateV1 {
			continue
		}
		solidLateral = append(solidLateral, math.Abs(float64(reading.Estimate.Y)-f.truthY))
		trackedLateral = append(trackedLateral, math.Abs(float64(track.Y)-f.truthY))
	}
	if len(solidLateral) < 20 {
		t.Fatalf("only %d near-edge frames on a forty-frame pass", len(solidLateral))
	}
	// Settled: the second half of the near-edge frames.
	solid := meanOf(solidLateral[len(solidLateral)/2:])
	tracked := meanOf(trackedLateral[len(trackedLateral)/2:])
	t.Logf("settled mean lateral error: solid body %.3f m, tracked medoid %.3f m", solid, tracked)
	if solid > 0.15 {
		t.Errorf("solid-body lateral error %.3f m, want under 0.15 m once settled", solid)
	}
	if solid >= tracked/2 {
		t.Errorf("solid-body lateral error %.3f m does not halve the tracked medoid's %.3f m", solid, tracked)
	}
}

func TestSolidBodyNamesTheFacesAndAspectItUsed(t *testing.T) {
	tracker := NewTracker(solidBodyConfig())
	var reading SolidBodyReading
	for _, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
		tracker.Update(f.clusters, f.at)
		r, _ := mainTrack(t, tracker).SolidBody()
		if r.Measurement.Source == MeasurementNearEdgeCandidateV1 {
			reading = r
		}
	}
	m := reading.Measurement
	if m.Source != MeasurementNearEdgeCandidateV1 {
		t.Fatal("no near-edge fix on the synthetic pass")
	}
	// The vehicle passes 5 m to the +Y side of the sensor heading +X, so the
	// face that looks at the sensor is its right side.
	if !m.Faces.Has(FaceRight) || m.Faces.Has(FaceLeft) {
		t.Errorf("faces %q: want the right side and never the far left side", m.Faces)
	}
	if m.Rank != len(faceList(m.Faces)) {
		t.Errorf("rank %d disagrees with faces %q", m.Rank, m.Faces)
	}
	if !m.AspectKnown || m.AspectRad < 0 || m.AspectRad > math.Pi/2 {
		t.Errorf("aspect %v (known %v) outside [0, pi/2]", m.AspectRad, m.AspectKnown)
	}
	if m.NIS < 0 || math.IsNaN(float64(m.NIS)) {
		t.Errorf("NIS %v", m.NIS)
	}
}

func faceList(v VisibleFaces) []BodyFace {
	var out []BodyFace
	for _, f := range allBodyFaces {
		if v.Has(f) {
			out = append(out, f)
		}
	}
	return out
}

func TestSolidBodyExtentsAccumulateAsLowerBoundsAndConverge(t *testing.T) {
	// A classified vehicle, so the class prior's sigma is the vehicle one and
	// the lifecycle can reach established inside a forty-frame pass.
	tracker := NewTracker(solidBodyConfig())
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	established := false
	for _, f := range frames {
		tracker.Update(f.clusters, f.at)
		track := mainTrack(t, tracker)
		tracker.UpdateClassification(track.TrackID, "car", 0.9, "test")
		r, _ := mainTrack(t, tracker).SolidBody()
		if r.Estimate.Estimation == EstimationEstablished {
			established = true
		}
	}
	r, _ := mainTrack(t, tracker).SolidBody()
	width := r.Estimate.Width
	if width.Provenance != ProvenanceAccumulated || width.AdmissibleFrames == 0 {
		t.Fatalf("width %+v: want accumulated evidence", width)
	}
	// Lower-bound evidence, quantised to the belief's bins: never more than a
	// bin above the true width, and within a bin of it on a clean pass.
	const trueWidth = 1.8
	if d := float64(width.Metres) - trueWidth; d > extentBeliefBinMetres || d < -extentBeliefBinMetres {
		t.Errorf("width %.3f m against a true %.1f m", width.Metres, trueWidth)
	}
	if r.Estimate.Height.Provenance != ProvenanceClassPrior {
		t.Errorf("height provenance %s: vertical extent is not admitted", r.Estimate.Height.Provenance)
	}
	if !established {
		t.Errorf("the lifecycle never reached established on a clean pass; last %s, length %+v, width %+v, orientation %+v",
			r.Estimate.Estimation, r.Estimate.Length, r.Estimate.Width, r.Estimate.Orientation)
	}
}

func TestSolidBodyWithoutRetainedPointsStaysOnTheMedoidAndSaysWhy(t *testing.T) {
	// No retained points is not a reason to guess a face. The body keeps its
	// medoid reference, and the reason is recorded on every frame.
	tracker := NewTracker(solidBodyConfig())
	for _, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
		for i := range f.clusters {
			f.clusters[i].RetainedPoints = nil
		}
		tracker.Update(f.clusters, f.at)
	}
	r, _ := mainTrack(t, tracker).SolidBody()
	if r.Estimate.Reference != ReferenceClusterMedoid {
		t.Fatalf("reference %s without any face evidence", r.Estimate.Reference)
	}
	if r.Measurement.Source != MeasurementMedoidV0 || r.Measurement.FallbackReason != "no_cluster_points" {
		t.Fatalf("measurement %+v: want the medoid, and the reason no face was claimed", r.Measurement)
	}
	if r.Estimate.Width.Provenance != ProvenanceClassPrior {
		t.Errorf("width provenance %s with no face ever measured", r.Estimate.Width.Provenance)
	}
}

func TestABodyCentreWithoutFacesLapsesToTheMedoidAndRecovers(t *testing.T) {
	// Observed every frame but with nothing the near-edge model can use, a
	// body-centre solid body coasts for fewer than MaxMisses frames, then its
	// claim lapses to the medoid; when faces return it is re-referenced.
	cfg := solidBodyConfig()
	tracker := NewTracker(cfg)
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	const faceless = 14
	for _, f := range frames[:faceless] {
		tracker.Update(f.clusters, f.at)
	}
	before, _ := mainTrack(t, tracker).SolidBody()
	if before.Estimate.Reference != ReferenceBodyCentre {
		t.Fatalf("setup: reference %s before the faceless run", before.Estimate.Reference)
	}
	limit := cfg.MaxMisses
	for i, f := range frames[faceless : faceless+limit] {
		for c := range f.clusters {
			f.clusters[c].RetainedPoints = nil
		}
		tracker.Update(f.clusters, f.at)
		r, _ := mainTrack(t, tracker).SolidBody()
		if i < limit-1 {
			if r.Estimate.Reference != ReferenceBodyCentre || r.Estimate.Support.CoastedFrames != i+1 {
				t.Fatalf("faceless frame %d: reference %s, coasted %d; want a body-centre coast",
					i, r.Estimate.Reference, r.Estimate.Support.CoastedFrames)
			}
			continue
		}
		if r.Estimate.Reference != ReferenceClusterMedoid || r.Measurement.FallbackReason != "body_centre_lapsed" ||
			r.Measurement.Source != MeasurementMedoidV0 {
			t.Fatalf("after %d faceless frames: reference %s, measurement %+v; want the lapse to the medoid",
				limit, r.Estimate.Reference, r.Measurement)
		}
		cluster := f.clusters[0]
		if r.Estimate.X != cluster.CentroidX || r.Estimate.Y != cluster.CentroidY {
			t.Fatalf("lapsed to (%v, %v), not the medoid (%v, %v)", r.Estimate.X, r.Estimate.Y, cluster.CentroidX, cluster.CentroidY)
		}
		if r.Covariance[2] != 0 || r.Covariance[8] != 0 {
			t.Fatalf("lapse kept a position-velocity correlation from before it")
		}
		if r.Estimate.Width.Provenance != ProvenanceAccumulated || r.VX == 0 {
			t.Fatalf("lapse discarded beliefs about the body: width %+v, vx %v", r.Estimate.Width, r.VX)
		}
	}
	tracker.Update(frames[faceless+limit].clusters, frames[faceless+limit].at)
	after, _ := mainTrack(t, tracker).SolidBody()
	if after.Estimate.Reference != ReferenceBodyCentre || after.Measurement.Source != MeasurementNearEdgeCandidateV1 {
		t.Fatalf("faces returned but the body stayed %s via %+v", after.Estimate.Reference, after.Measurement)
	}
}

func TestSolidBodyCoastsWhenTheTrackIsUnassociated(t *testing.T) {
	tracker := NewTracker(solidBodyConfig())
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	for _, f := range frames[:12] {
		tracker.Update(f.clusters, f.at)
	}
	before, _ := mainTrack(t, tracker).SolidBody()
	tracker.Update(nil, frames[12].at)
	after, ok := mainTrack(t, tracker).SolidBody()
	if !ok {
		t.Fatal("the solid body was lost on a missed frame")
	}
	if after.Estimate.Support.CoastedFrames != 1 || after.Estimate.Support.IsObserved() {
		t.Errorf("support %+v after one missed frame", after.Estimate.Support)
	}
	if after.Measurement.Source != "" || after.Measurement.FallbackReason != "no_association" {
		t.Errorf("measurement %+v on a missed frame", after.Measurement)
	}
	if after.Estimate.LastObservedUnixNanos != before.Estimate.LastObservedUnixNanos {
		t.Errorf("last observed moved on a frame with no observation")
	}
	if after.Covariance[0] <= before.Covariance[0] || after.Covariance[5] <= before.Covariance[5] {
		t.Errorf("position variance did not grow across a missed frame: %v, %v -> %v, %v",
			before.Covariance[0], before.Covariance[5], after.Covariance[0], after.Covariance[5])
	}
	// Predicted forward at its own velocity.
	if after.Estimate.X <= before.Estimate.X {
		t.Errorf("x %v -> %v: the body was not predicted forward", before.Estimate.X, after.Estimate.X)
	}
}

func TestSolidBodyReadingAgreesWithItself(t *testing.T) {
	// l8behaviour refuses a sample whose covariance block disagrees with the
	// estimate's own, because the two would describe different frames.
	tracker := NewTracker(solidBodyConfig())
	for _, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
		tracker.Update(f.clusters, f.at)
		r, _ := mainTrack(t, tracker).SolidBody()
		block := [4]float32{r.Covariance[0], r.Covariance[1], r.Covariance[4], r.Covariance[5]}
		if block != r.Estimate.PositionCovariance {
			t.Fatalf("covariance block %v disagrees with the estimate's %v", block, r.Estimate.PositionCovariance)
		}
		if r.Estimate.StateModel != StateModelCVCartesianV1 {
			t.Fatalf("state model %q", r.Estimate.StateModel)
		}
		if asym := covarianceAsymmetry(r.Covariance); asym > 1e-5 {
			t.Fatalf("solid-body covariance asymmetric by %v", asym)
		}
	}
}

func TestSolidBodySnapshotIsIndependentOfLaterUpdates(t *testing.T) {
	// Held by value, so a published snapshot cannot be changed underneath its
	// reader by the next frame.
	tracker := NewTracker(solidBodyConfig())
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	for _, f := range frames[:8] {
		tracker.Update(f.clusters, f.at)
	}
	snapshot := mainTrack(t, tracker)
	held, _ := snapshot.SolidBody()
	for _, f := range frames[8:12] {
		tracker.Update(f.clusters, f.at)
	}
	again, _ := snapshot.SolidBody()
	if !reflect.DeepEqual(held, again) {
		t.Fatal("a snapshot's solid body changed after later updates")
	}
}

func TestChangingTheSolidBodyOptionsDiscardsExistingBodies(t *testing.T) {
	tracker := NewTracker(solidBodyConfig())
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	for _, f := range frames[:6] {
		tracker.Update(f.clusters, f.at)
	}
	tracker.UpdateConfig(func(c *TrackerConfig) { c.SolidBody.Enabled = false })
	if _, ok := mainTrack(t, tracker).SolidBody(); ok {
		t.Fatal("a solid body survived the option being switched off")
	}
	tracker.UpdateConfig(func(c *TrackerConfig) { c.SolidBody.Enabled = true })
	tracker.Update(frames[6].clusters, frames[6].at)
	r, ok := mainTrack(t, tracker).SolidBody()
	if !ok || r.Estimate.Reference != ReferenceClusterMedoid {
		t.Fatalf("switched back on, the body did not reseed from the next observation: %+v", r.Estimate)
	}
}

func TestScalarUpdateAlongOneNormalLeavesTheOtherDirectionAlone(t *testing.T) {
	// A single face constrains its own normal. Updating along +Y must not move
	// X at all, however uncertain X is.
	x := [4]float32{0, 0, 1, 0}
	p := [16]float32{
		4, 0, 0, 0,
		0, 4, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
	nis, ok := scalarPositionUpdate(&x, &p, 0, 1, 1, 0.05)
	if !ok {
		t.Fatal("update refused")
	}
	if x[0] != 0 || x[2] != 1 {
		t.Errorf("x %v, vx %v moved under a Y-only measurement", x[0], x[2])
	}
	if x[1] <= 0.9 || x[1] >= 1 {
		t.Errorf("y %v: want it pulled most of the way to 1 by a confident face", x[1])
	}
	if p[0] != 4 {
		t.Errorf("x variance %v changed under a Y-only measurement", p[0])
	}
	if p[5] >= 4 {
		t.Errorf("y variance %v did not shrink", p[5])
	}
	if want := 1.0 / 4.05; math.Abs(nis-want) > 1e-6 {
		t.Errorf("NIS %v, want %v", nis, want)
	}
}

func TestSequentialScalarUpdatesEqualTheJointUpdate(t *testing.T) {
	// Two perpendicular faces with independent noise, applied one after the
	// other, must give exactly the two-dimensional Kalman update, including a
	// covariance with cross terms.
	x0 := [4]float32{1, 2, 3, -1}
	p0 := [16]float32{
		2, 0.3, 0.1, 0,
		0.3, 1, 0, 0.2,
		0.1, 0, 0.5, 0,
		0, 0.2, 0, 0.5,
	}
	const r = 0.05
	zx, zy := 1.4, 1.7

	x, p := x0, p0
	n1, ok1 := scalarPositionUpdate(&x, &p, 1, 0, zx, r)
	n2, ok2 := scalarPositionUpdate(&x, &p, 0, 1, zy, r)
	if !ok1 || !ok2 {
		t.Fatal("update refused")
	}

	// The joint update, written out.
	s00, s01, s11 := float64(p0[0])+r, float64(p0[1]), float64(p0[5])+r
	det := s00*s11 - s01*s01
	i00, i01, i11 := s11/det, -s01/det, s00/det
	yx, yy := zx-float64(x0[0]), zy-float64(x0[1])
	for i := 0; i < 4; i++ {
		k0 := float64(p0[i*4+0])*i00 + float64(p0[i*4+1])*i01
		k1 := float64(p0[i*4+0])*i01 + float64(p0[i*4+1])*i11
		want := float64(x0[i]) + k0*yx + k1*yy
		if math.Abs(float64(x[i])-want) > 1e-5 {
			t.Errorf("state[%d] = %v, joint update gives %v", i, x[i], want)
		}
		for j := 0; j < 4; j++ {
			kj0 := float64(p0[j*4+0])*i00 + float64(p0[j*4+1])*i01
			kj1 := float64(p0[j*4+0])*i01 + float64(p0[j*4+1])*i11
			// P' = P - K S Kᵀ
			ks := k0*(s00*kj0+s01*kj1) + k1*(s01*kj0+s11*kj1)
			want := float64(p0[i*4+j]) - ks
			if math.Abs(float64(p[i*4+j])-want) > 1e-5 {
				t.Errorf("P[%d][%d] = %v, joint update gives %v", i, j, p[i*4+j], want)
			}
		}
	}
	jointNIS := yx*(i00*yx+i01*yy) + yy*(i01*yx+i11*yy)
	if math.Abs(n1+n2-jointNIS) > 1e-6 {
		t.Errorf("sequential NIS %v, joint %v", n1+n2, jointNIS)
	}
}

func TestScalarUpdateRefusesANonPositiveInnovationVariance(t *testing.T) {
	x := [4]float32{1, 2, 0, 0}
	p := [16]float32{}
	before := x
	if _, ok := scalarPositionUpdate(&x, &p, 1, 0, 5, 0); ok {
		t.Fatal("a zero innovation variance was accepted")
	}
	if x != before {
		t.Fatal("a refused update changed the state")
	}
}

func TestVisibleFacesRoundTrip(t *testing.T) {
	for _, v := range []VisibleFaces{0, faceBit(FaceFront), faceBit(FaceRight) | faceBit(FaceRear), 0xF} {
		got, err := ParseVisibleFaces(v.String())
		if err != nil || got != v {
			t.Errorf("%q round-tripped to %q, %v", v, got, err)
		}
	}
	if s := (faceBit(FaceRight) | faceBit(FaceFront)).String(); s != "front,right" {
		t.Errorf("faces named %q, want the fixed order front,right", s)
	}
	if _, err := ParseVisibleFaces("front,roof"); err == nil {
		t.Error("an unknown face name was accepted")
	}
}

// rectangleOutline samples the outline of a length x width rectangle centred
// at the origin with its length along +X.
func rectangleOutline(length, width float64, perSide int) []l4perception.WorldPoint {
	var points []l4perception.WorldPoint
	for i := 0; i <= perSide; i++ {
		f := float64(i) / float64(perSide)
		x := -length/2 + f*length
		y := -width/2 + f*width
		points = append(points,
			l4perception.WorldPoint{X: x, Y: -width / 2}, l4perception.WorldPoint{X: x, Y: width / 2},
			l4perception.WorldPoint{X: -length / 2, Y: y}, l4perception.WorldPoint{X: length / 2, Y: y})
	}
	return points
}

func TestExtentsAreRefusedAlongAnAxisTheCourseContradicts(t *testing.T) {
	// A 4.5 x 1.8 m body moving along +X. Along a heading 45 degrees off its
	// course the width span would read the length; that evidence is refused.
	// Along a heading on its course it is admitted.
	tracker := NewTracker(solidBodyConfig())
	cluster := WorldCluster{RetainedPoints: rectangleOutline(4.5, 1.8, 200)}
	set := EdgeMeasurementSet{Edges: []EdgeMeasurement{{Face: FaceRight}}}
	for _, c := range []struct {
		headingRad float32
		admitted   bool
	}{{math.Pi / 4, false}, {0.05, true}, {math.Pi + 0.05, true}} {
		track := &TrackedObject{}
		track.solidBody = solidBodyTrack{
			seeded:      true,
			state:       [4]float32{0, 0, 10, 0},
			estimation:  EstimationGeometryConverging,
			orientation: OrientationBelief{PsiRad: c.headingRad, Provenance: ProvenanceObserved},
		}
		tracker.admitSolidBodyExtents(track, cluster, set, tracker.faceAxis(&track.solidBody))
		got := track.solidBody.widthBelief.Support > 0
		if got != c.admitted {
			t.Errorf("heading %.2f rad on a +X course: admitted %v, want %v", c.headingRad, got, c.admitted)
		}
		if got {
			if w := track.solidBody.widthBelief.Estimate(); w > 2 {
				t.Errorf("heading %.2f rad: width %v overstates a 1.8 m body", c.headingRad, w)
			}
		}
	}

	// With course-aligned faces the spans are taken along the course, so the
	// same lagging heading no longer costs the evidence, and the width is
	// still the width.
	cfg := solidBodyConfig()
	cfg.SolidBody.CourseAlignedFaces = true
	course := NewTracker(cfg)
	track := &TrackedObject{}
	track.solidBody = solidBodyTrack{
		seeded:      true,
		state:       [4]float32{0, 0, 10, 0},
		estimation:  EstimationGeometryConverging,
		orientation: OrientationBelief{PsiRad: math.Pi / 4, Provenance: ProvenanceObserved},
	}
	course.admitSolidBodyExtents(track, cluster, set, course.faceAxis(&track.solidBody))
	if track.solidBody.widthBelief.Support == 0 {
		t.Fatal("course-aligned spans refused a body moving along its length")
	}
	if w := track.solidBody.widthBelief.Estimate(); w > 2 {
		t.Errorf("course-aligned width %v overstates a 1.8 m body", w)
	}
}

func TestMinimumAxisSpanIsATrimmedLowerBound(t *testing.T) {
	var points []l4perception.WorldPoint
	for i := 0; i <= 100; i++ {
		points = append(points, l4perception.WorldPoint{X: float64(i) / 100 * 4, Y: 0})
	}
	// One stray a long way out must not define the span.
	points = append(points, l4perception.WorldPoint{X: 30})
	span, ok := minimumAxisSpan(points, 0)
	if !ok {
		t.Fatal("no span")
	}
	if span > 4 || span < 3.8 {
		t.Errorf("span %v over a 4 m side with one stray", span)
	}
	if _, ok := minimumAxisSpan(points[:3], 0); ok {
		t.Error("a span from three points was accepted")
	}
}

func TestMinimumAxisSpanDoesNotOverstateAcrossAHeadingError(t *testing.T) {
	// A 4.5 x 1.8 m body read along an axis seven degrees off. Measured along
	// that axis its width would be over 2.3 m; the minimum within the window
	// recovers the true width, and never exceeds it.
	points := rectangleOutline(4.5, 1.8, 200)
	const errorRad = 7 * math.Pi / 180
	naive := func(axis float64) float64 {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, p := range points {
			v := p.X*math.Cos(axis) + p.Y*math.Sin(axis)
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
		return hi - lo
	}
	if w := naive(errorRad + math.Pi/2); w < 2.3 {
		t.Fatalf("test geometry: the naive width %v should be inflated", w)
	}
	width, ok := minimumAxisSpan(points, float32(errorRad+math.Pi/2))
	if !ok {
		t.Fatal("no width")
	}
	if width > 1.8+1e-6 || width < 1.7 {
		t.Errorf("width %v across a 7 degree heading error, want at most and near 1.8", width)
	}
	length, ok := minimumAxisSpan(points, float32(errorRad))
	if !ok || length > 4.5+1e-6 || length < 4.4 {
		t.Errorf("length %v (ok %v), want at most and near 4.5", length, ok)
	}
}

func TestFaceHysteresisAdmitsAFaceAfterTwoFramesAndDropsItAfterTwoWithout(t *testing.T) {
	right := EdgeMeasurement{Face: FaceRight}
	front := EdgeMeasurement{Face: FaceFront}
	var sb solidBodyTrack
	steps := []struct {
		name   string
		usable []EdgeMeasurement
		want   VisibleFaces
	}{
		{"first sighting waits", []EdgeMeasurement{right}, 0},
		{"second consecutive admits", []EdgeMeasurement{right}, faceBit(FaceRight)},
		{"one frame without keeps admission", nil, 0},
		{"back after one frame is used at once", []EdgeMeasurement{right}, faceBit(FaceRight)},
		{"a new face waits beside an admitted one", []EdgeMeasurement{right, front}, faceBit(FaceRight)},
		{"and is admitted on its second frame", []EdgeMeasurement{right, front}, faceBit(FaceRight) | faceBit(FaceFront)},
		{"first frame without", []EdgeMeasurement{front}, faceBit(FaceFront)},
		{"second frame without drops it", []EdgeMeasurement{front}, faceBit(FaceFront)},
		{"so it must qualify again", []EdgeMeasurement{right, front}, faceBit(FaceFront)},
		{"and does on its second frame", []EdgeMeasurement{right, front}, faceBit(FaceRight) | faceBit(FaceFront)},
	}
	for _, step := range steps {
		var got VisibleFaces
		for _, e := range sb.admitFaces(step.usable) {
			got |= faceBit(e.Face)
		}
		if got != step.want {
			t.Fatalf("%s: faces %q, want %q", step.name, got, step.want)
		}
	}
}

// firstFix replays the synthetic pass and returns the main track's reading on
// its first near-edge fix and the frame index it came on.
func firstFix(t *testing.T, cfg TrackerConfig, frames []syntheticPassFrame) (int, SolidBodyReading, []SolidBodyReading) {
	t.Helper()
	tracker := NewTracker(cfg)
	var readings []SolidBodyReading
	fix := -1
	var at SolidBodyReading
	for i, f := range frames {
		tracker.Update(f.clusters, f.at)
		r, _ := mainTrack(t, tracker).SolidBody()
		readings = append(readings, r)
		if fix < 0 && r.Measurement.Source == MeasurementNearEdgeCandidateV1 {
			fix, at = i, r
		}
	}
	if fix < 0 {
		t.Fatal("no near-edge fix on the synthetic pass")
	}
	return fix, at, readings
}

func TestFaceHysteresisHoldsTheFirstFixForOneFrame(t *testing.T) {
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	plain, _, _ := firstFix(t, solidBodyConfig(), frames)
	cfg := solidBodyConfig()
	cfg.SolidBody.FaceHysteresis = true
	held, _, readings := firstFix(t, cfg, frames)
	if held != plain+1 {
		t.Fatalf("first fix on frame %d with hysteresis, %d without; want exactly one frame later", held, plain)
	}
	if got := readings[plain].Measurement.FallbackReason; got != "face_hysteresis" {
		t.Fatalf("the held frame says %q, want face_hysteresis", got)
	}
}

func TestFaceEntryConsiderWeightsOnlyAFaceThatWasNotInThePreviousFix(t *testing.T) {
	sb := solidBodyTrack{lastFixFaces: faceBit(FaceRight)}
	length := DimensionBelief{Metres: 4.5, SigmaMetres: 0.4}
	width := DimensionBelief{Metres: 1.8, SigmaMetres: 0.2}
	got := sb.entryConsider([]EdgeMeasurement{{Face: FaceRight}, {Face: FaceFront}, {Face: FaceLeft}}, length, width)
	want := []float64{0, 0.4 * 0.4 / 4, 0.2 * 0.2 / 4}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-7 {
			t.Fatalf("consider %v, want %v: a face already in the fix carries none, and a new one a quarter of its dimension's variance", got, want)
		}
	}
}

func TestFaceEntryConsiderSoftensTheEntryUpdateAndNothingBefore(t *testing.T) {
	// The first fix enters every face, so with the consider term its
	// innovation variance is larger and its NIS smaller. Everything before it
	// is the medoid, which the option does not touch.
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	plainAt, plain, plainReadings := firstFix(t, solidBodyConfig(), frames)
	cfg := solidBodyConfig()
	cfg.SolidBody.FaceEntryConsider = true
	consideredAt, considered, consideredReadings := firstFix(t, cfg, frames)
	if consideredAt != plainAt {
		t.Fatalf("first fix moved from frame %d to %d; the consider term must not delay it", plainAt, consideredAt)
	}
	for i := 0; i < plainAt; i++ {
		if !reflect.DeepEqual(plainReadings[i], consideredReadings[i]) {
			t.Fatalf("frame %d, before any fix, differs with the consider term on", i)
		}
	}
	if !(considered.Measurement.NIS < plain.Measurement.NIS) {
		t.Fatalf("entry NIS %v with the consider term, %v without; want it smaller", considered.Measurement.NIS, plain.Measurement.NIS)
	}
}

func TestSolidBodyMeasuresFromMembersWhenNoSampleIsRetained(t *testing.T) {
	// The live pipeline retains no evidence sample. Handed the same points as
	// members instead, the solid body must measure exactly as it does from
	// the sample, and prefer the members when it has both.
	frames := syntheticPassFrames(t, l4perception.DefaultSyntheticPass())
	membersOnly := make([]syntheticPassFrame, len(frames))
	both := make([]syntheticPassFrame, len(frames))
	for i, f := range frames {
		membersOnly[i], both[i] = f, f
		membersOnly[i].clusters = make([]WorldCluster, len(f.clusters))
		both[i].clusters = make([]WorldCluster, len(f.clusters))
		for c, cl := range f.clusters {
			m := cl
			m.Members, m.RetainedPoints = cl.RetainedPoints, nil
			membersOnly[i].clusters[c] = m
			b := cl
			b.Members = cl.RetainedPoints
			b.RetainedPoints = cl.RetainedPoints[:1]
			both[i].clusters[c] = b
		}
	}
	_, _, sample := firstFix(t, solidBodyConfig(), frames)
	_, _, members := firstFix(t, solidBodyConfig(), membersOnly)
	_, _, preferred := firstFix(t, solidBodyConfig(), both)
	if !reflect.DeepEqual(sample, members) {
		t.Fatal("the solid body measured differently from the same points handed over as members")
	}
	if !reflect.DeepEqual(sample, preferred) {
		t.Fatal("with members and a one-point sample the solid body did not measure from the members")
	}
}

func TestSolidBodyWithoutADeclaredOriginMakesNoFixAndSaysWhy(t *testing.T) {
	// (0, 0) is where the synthetic sensor is, but nothing said so: the
	// solid body must stay on the medoid and name the missing declaration on
	// every row, rather than assume the zero value is the sensor.
	cfg := solidBodyConfig()
	cfg.SolidBody.OriginSource = ""
	tracker := NewTracker(cfg)
	for i, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
		tracker.Update(f.clusters, f.at)
		r, _ := mainTrack(t, tracker).SolidBody()
		if i == 0 {
			continue // the seed row
		}
		if r.Measurement.Source == MeasurementNearEdgeCandidateV1 || r.Estimate.Reference != ReferenceClusterMedoid {
			t.Fatalf("frame %d: near-edge fix or body-centre reference without a declared origin: %+v", i, r.Measurement)
		}
		if r.Measurement.FallbackReason != "missing_calibrated_sensor_origin" && r.Measurement.FallbackReason != "no_association" {
			t.Fatalf("frame %d: fallback %q, want missing_calibrated_sensor_origin", i, r.Measurement.FallbackReason)
		}
	}
}

func TestFaceAxisFollowsTheCourseOnlyWhenAskedAndMoving(t *testing.T) {
	// Heading along +X, moving along +Y: a turn the tracked heading has not
	// caught up with.
	moving := solidBodyTrack{state: [4]float32{0, 0, 0, 5}, orientation: OrientationBelief{PsiRad: 0, Provenance: ProvenanceObserved}}
	slow := moving
	slow.state[3] = CourseAlignmentMinSpeedMps / 2
	cfg := solidBodyConfig()
	plain := NewTracker(cfg)
	cfg.SolidBody.CourseAlignedFaces = true
	course := NewTracker(cfg)
	if got := plain.faceAxis(&moving); got != 0 {
		t.Fatalf("without the option the axis is %v, want the tracked heading", got)
	}
	if got := course.faceAxis(&moving); math.Abs(float64(got)-math.Pi/2) > 1e-6 {
		t.Fatalf("moving at 5 m/s along +Y the axis is %v, want the course, pi/2", got)
	}
	if got := course.faceAxis(&slow); got != 0 {
		t.Fatalf("below %v m/s the axis is %v, want the tracked heading", CourseAlignmentMinSpeedMps, got)
	}
}

func TestCourseAlignedFacesStillAnchorTheStraightPass(t *testing.T) {
	// On a straight pass the course is the heading, so T3 must anchor the
	// body centre as well as the tracked heading does.
	cfg := solidBodyConfig()
	cfg.SolidBody.CourseAlignedFaces = true
	tracker := NewTracker(cfg)
	var lateral []float64
	for _, f := range syntheticPassFrames(t, l4perception.DefaultSyntheticPass()) {
		tracker.Update(f.clusters, f.at)
		r, _ := mainTrack(t, tracker).SolidBody()
		if f.occluded || r.Measurement.Source != MeasurementNearEdgeCandidateV1 {
			continue
		}
		lateral = append(lateral, math.Abs(float64(r.Estimate.Y)-f.truthY))
	}
	if len(lateral) < 20 {
		t.Fatalf("only %d near-edge frames with course-aligned faces", len(lateral))
	}
	if settled := meanOf(lateral[len(lateral)/2:]); settled > 0.15 {
		t.Fatalf("settled lateral error %.3f m with course-aligned faces, want under 0.15 m", settled)
	}
}
