package l5tracks

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// A small deterministic scene generator for the uncertainty work.
//
// Each body follows a scripted path from the sensor's point of view: constant
// velocity, then from manoeuvreFrame a constant longitudinal acceleration and
// yaw rate until manoeuvreEnd. Its cluster is placed at the true centre plus
// noise drawn along and across the sensor's line of sight with known sigmas,
// from a seeded source, so every run of a scene is the same run. A frame can
// hide a body (occlusion) or replace its cluster with a displaced one (a
// spurious measurement, such as a merge or a reflection standing in for it).

type uncBody struct {
	x, y, headingRad, speedMps float64
	manoeuvreFrame             int
	manoeuvreEnd               int
	accelMps2, yawRateRadps    float64
	lengthM, widthM            float64
	points                     int
	hidden                     func(frame int) bool
	// spurious replaces the body's cluster on the given frames with one
	// displaced by the given offset along and across the body's heading.
	spurious map[int][2]float64
}

type uncNoise struct {
	radialSigma, tangentialSigma float64
}

type uncScene struct {
	frames int
	period time.Duration
	bodies []uncBody
	noise  uncNoise
	seed   int64
}

type uncPose struct {
	x, y, heading float64
	visible       bool
}

type uncFrame struct {
	at       time.Time
	clusters []WorldCluster
	truth    []uncPose
}

func (s uncScene) render() []uncFrame {
	rng := rand.New(rand.NewSource(s.seed))
	start := time.Unix(1_700_000_000, 0)
	dt := s.period.Seconds()
	type state struct{ x, y, h, v float64 }
	states := make([]state, len(s.bodies))
	for i, b := range s.bodies {
		states[i] = state{b.x, b.y, b.headingRad, b.speedMps}
	}
	out := make([]uncFrame, 0, s.frames)
	for f := 0; f < s.frames; f++ {
		frame := uncFrame{at: start.Add(time.Duration(f) * s.period)}
		for i, b := range s.bodies {
			st := states[i]
			pose := uncPose{x: st.x, y: st.y, heading: st.h}
			// Noise is drawn for every body on every frame, visible or not,
			// so hiding one body does not shift another's noise.
			nr, nt := rng.NormFloat64()*s.noise.radialSigma, rng.NormFloat64()*s.noise.tangentialSigma
			hidden := b.hidden != nil && b.hidden(f)
			if !hidden {
				pose.visible = true
				r := math.Hypot(st.x, st.y)
				ux, uy := st.x/r, st.y/r
				mx := st.x + nr*ux - nt*uy
				my := st.y + nr*uy + nt*ux
				if off, ok := b.spurious[f]; ok {
					ch, sh := math.Cos(st.h), math.Sin(st.h)
					mx = st.x + off[0]*ch - off[1]*sh
					my = st.y + off[0]*sh + off[1]*ch
				}
				frame.clusters = append(frame.clusters, uncCluster(mx, my, st.h, b))
			}
			frame.truth = append(frame.truth, pose)

			// Advance to the next frame.
			if b.manoeuvreFrame > 0 && f >= b.manoeuvreFrame && (b.manoeuvreEnd <= 0 || f < b.manoeuvreEnd) {
				st.v = math.Max(0, st.v+b.accelMps2*dt)
				st.h += b.yawRateRadps * dt
			}
			st.x += st.v * math.Cos(st.h) * dt
			st.y += st.v * math.Sin(st.h) * dt
			states[i] = st
		}
		out = append(out, frame)
	}
	return out
}

func uncCluster(x, y, heading float64, b uncBody) WorldCluster {
	fx, fy := float32(x), float32(y)
	return WorldCluster{
		CentroidX: fx, CentroidY: fy,
		BoundingBoxLength: float32(b.lengthM), BoundingBoxWidth: float32(b.widthM), BoundingBoxHeight: 1.5,
		PointsCount: b.points,
		OBB: &l4perception.OrientedBoundingBox{
			CenterX: fx, CenterY: fy, Length: float32(b.lengthM), Width: float32(b.widthM), Height: 1.5,
			HeadingRad: float32(heading),
		},
	}
}

// uncCar is a vehicle-sized body travelling east.
func uncCar(x, y, speed float64) uncBody {
	return uncBody{x: x, y: y, speedMps: speed, lengthM: 4.5, widthM: 1.8, points: 120}
}

// runUncScene drives a tracker through a scene, opening the scoring and
// calibration windows together after warmup frames, as a replay does.
func runUncScene(t *testing.T, cfg TrackerConfig, frames []uncFrame, warmup int, window bool) *Tracker {
	t.Helper()
	tk := NewTracker(cfg)
	for i, f := range frames {
		if window && i == warmup {
			tk.BeginTrackingBaseline()
			tk.BeginUncertaintyCalibration()
		}
		tk.Update(f.clusters, f.at)
	}
	return tk
}

func uncPreGateTotals(w UncertaintyWindow) (total PreGateBandSummary) {
	for _, b := range w.PreGate {
		total.Candidates += b.Candidates
		total.Eligible += b.Eligible
		total.Rejections += b.Rejections
		total.RejectedUnassigned += b.RejectedUnassigned
		total.RejectionPersistent += b.RejectionPersistent
		total.RejectionTransient += b.RejectionTransient
		total.RejectionUnresolved += b.RejectionUnresolved
		total.RejectionOpen += b.RejectionOpen
	}
	return total
}

func TestSceneRenderIsDeterministicAndCarriesTheInjectedNoise(t *testing.T) {
	scene := uncScene{frames: 400, period: 100 * time.Millisecond, seed: 7,
		bodies: []uncBody{{x: 0, y: 30, lengthM: 4.5, widthM: 1.8, points: 50}},
		noise:  uncNoise{radialSigma: 0.05, tangentialSigma: 0.4}}
	a, b := scene.render(), scene.render()
	var sumR, sumT float64
	for i := range a {
		if a[i].clusters[0].CentroidX != b[i].clusters[0].CentroidX || a[i].clusters[0].CentroidY != b[i].clusters[0].CentroidY {
			t.Fatalf("frame %d differs between renders", i)
		}
		// Line of sight is +y for a body at (0, 30).
		sumR += math.Pow(float64(a[i].clusters[0].CentroidY)-30, 2)
		sumT += math.Pow(float64(a[i].clusters[0].CentroidX), 2)
	}
	sr, st := math.Sqrt(sumR/400), math.Sqrt(sumT/400)
	if math.Abs(sr-0.05) > 0.006 || math.Abs(st-0.4) > 0.04 {
		t.Errorf("injected sigmas (%.3f, %.3f), want (0.05, 0.4)", sr, st)
	}
}

// Opening the window must not change a single estimate, and a closed window
// must record nothing.
func TestCalibrationWindowIsReadOnly(t *testing.T) {
	scene := uncScene{frames: 80, period: 100 * time.Millisecond, seed: 3,
		bodies: []uncBody{uncCar(-30, 15, 12), uncCar(-10, -25, 8)},
		noise:  uncNoise{radialSigma: 0.05, tangentialSigma: 0.15}}
	frames := scene.render()
	for _, adaptive := range []bool{false, true} {
		cfg := DefaultTrackerConfig()
		cfg.AdaptiveMeasurementNoise = adaptive
		closed := runUncScene(t, cfg, frames, 10, false)
		open := runUncScene(t, cfg, frames, 10, true)
		if w := closed.UncertaintyWindow(); len(w.Samples) != 0 || w.PreGate != nil {
			t.Fatalf("adaptive=%v: a closed window recorded %+v", adaptive, w)
		}
		if closed.GetWindowBaseline().PreGate != nil {
			t.Fatalf("adaptive=%v: pre-gate bands reported with no window open", adaptive)
		}
		if len(open.UncertaintyWindow().Samples) == 0 || open.GetWindowBaseline().PreGate == nil {
			t.Fatalf("adaptive=%v: the open window recorded nothing", adaptive)
		}
		for _, a := range closed.Tracks {
			found := false
			for _, b := range open.Tracks {
				if a.CreationSequence == b.CreationSequence {
					found = true
					if a.X != b.X || a.Y != b.Y || a.VX != b.VX || a.VY != b.VY || a.P != b.P || a.TrackState != b.TrackState {
						t.Fatalf("adaptive=%v: track %d differs with the window open", adaptive, a.CreationSequence)
					}
				}
			}
			if !found {
				t.Fatalf("adaptive=%v: track %d missing with the window open", adaptive, a.CreationSequence)
			}
		}
	}
}

// Two bodies inside each other's plausibility radius are ambiguous and give
// no calibration sample; an isolated one gives one per confirmed frame. At
// 10 Hz the implied-speed rule (30 m/s) makes the effective radius 3 m, so the
// pair runs 2 m apart.
func TestPreGateEligibilityExcludesAmbiguity(t *testing.T) {
	isolated := uncScene{frames: 40, period: 100 * time.Millisecond, seed: 1,
		bodies: []uncBody{uncCar(-20, 20, 10)}, noise: uncNoise{0.03, 0.05}}
	tk := runUncScene(t, DefaultTrackerConfig(), isolated.render(), 0, true)
	w := tk.UncertaintyWindow()
	// The track confirms on its fourth hit, and is first eligible on the
	// frame after.
	if want := 40 - DefaultTrackerConfig().HitsToConfirm; len(w.Samples) != want {
		t.Errorf("isolated body: %d samples, want %d", len(w.Samples), want)
	}
	frames := isolated.render()
	for i, s := range w.Samples {
		if s.Rank != 2 || s.Source != MeasurementMedoidV0 || !s.Assigned || s.Gated || s.Misses != 0 {
			t.Fatalf("unexpected sample %+v", s)
		}
		// The join keys: the frame's capture time and the measured position.
		f := frames[DefaultTrackerConfig().HitsToConfirm+i]
		if s.FrameUnixNanos != f.at.UnixNano() || s.X != f.clusters[0].CentroidX || s.Y != f.clusters[0].CentroidY {
			t.Fatalf("sample %d join keys (%d, %v, %v), frame (%d, %v, %v)", i, s.FrameUnixNanos, s.X, s.Y,
				f.at.UnixNano(), f.clusters[0].CentroidX, f.clusters[0].CentroidY)
		}
	}

	pair := uncScene{frames: 40, period: 100 * time.Millisecond, seed: 1,
		bodies: []uncBody{uncCar(-20, 20, 10), uncCar(-20, 22, 10)}, noise: uncNoise{0.03, 0.05}}
	tk = runUncScene(t, DefaultTrackerConfig(), pair.render(), 0, true)
	w = tk.UncertaintyWindow()
	totals := uncPreGateTotals(w)
	if len(w.Samples) != 0 || totals.Eligible != 0 || totals.Candidates == 0 {
		t.Errorf("ambiguous pair: %d samples, %d eligible of %d candidates; want none eligible", len(w.Samples), totals.Eligible, totals.Candidates)
	}
}

// A spurious cluster the gate rejects is still recorded before the gate, and
// only there: the accepted-only bands never see it.
func TestPreGateRecordsWhatTheGateRejects(t *testing.T) {
	body := uncCar(-25, 18, 12)
	body.spurious = map[int][2]float64{30: {0, 3}}
	scene := uncScene{frames: 45, period: 100 * time.Millisecond, seed: 5,
		bodies: []uncBody{body}, noise: uncNoise{0.03, 0.05}}
	tk := runUncScene(t, DefaultTrackerConfig(), scene.render(), 10, true)
	w := tk.UncertaintyWindow()
	gated := 0
	for _, s := range w.Samples {
		if s.Gated {
			gated++
			if s.Assigned || float64(s.NIS) <= float64(DefaultTrackerConfig().GatingDistanceSquared) {
				t.Errorf("gated sample %+v", s)
			}
		}
	}
	if gated != 1 {
		t.Fatalf("%d gated samples, want the one spurious frame", gated)
	}
	accepted := 0
	for _, b := range tk.GetWindowBaseline().Residuals {
		accepted += b.Count
	}
	if accepted != len(w.Samples)-gated {
		t.Errorf("accepted-only count %d, want %d", accepted, len(w.Samples)-gated)
	}
}

func uncRejectionOutcomes(t *testing.T, cfg TrackerConfig, body uncBody, frames int) UncertaintyWindow {
	t.Helper()
	scene := uncScene{frames: frames, period: 100 * time.Millisecond, seed: 11,
		bodies: []uncBody{body}, noise: uncNoise{0.02, 0.03}}
	return runUncScene(t, cfg, scene.render(), 8, true).UncertaintyWindow()
}

// Genuine manoeuvres and spurious measurements, under the shipped gate. The
// manoeuvres are the severe end of what a road user does, because that is
// what the shipped gate of 36 rejects at all: a stop at 20 m/s² (collision
// severity, beyond tyre grip) and a turn at 0.8 rad/s from 12 m/s (about 1 g
// lateral). Those are exactly the events Section 12 says must be preserved,
// and every rejection they provoke must close as persistent. One displaced
// cluster on a straight path, across or along the body, is spurious and must
// close as transient. Both the shipped and the uncalibrated adaptive model
// are checked.
func TestGateRejectionEvidenceSeparatesManoeuvreFromSpurious(t *testing.T) {
	brake := uncCar(-30, 15, 12)
	brake.manoeuvreFrame, brake.accelMps2 = 20, -20
	turn := uncCar(-30, 15, 12)
	turn.manoeuvreFrame, turn.manoeuvreEnd, turn.yawRateRadps = 20, 39, 0.8
	across := uncCar(-30, 15, 12)
	across.spurious = map[int][2]float64{24: {0, 2.5}}
	along := uncCar(-30, 15, 12)
	along.spurious = map[int][2]float64{24: {-2.5, 0}}

	for _, c := range []struct {
		name           string
		body           uncBody
		wantPersistent bool
	}{
		{"collision stop", brake, true},
		{"limit turn", turn, true},
		{"spurious across", across, false},
		{"spurious along", along, false},
	} {
		for _, adaptive := range []bool{false, true} {
			cfg := DefaultTrackerConfig()
			cfg.AdaptiveMeasurementNoise = adaptive
			w := uncRejectionOutcomes(t, cfg, c.body, 50)
			got := uncPreGateTotals(w)
			if got.Rejections == 0 {
				t.Errorf("%s adaptive=%v: no rejection; the scene does not exercise the gate", c.name, adaptive)
				continue
			}
			closed := got.RejectionPersistent + got.RejectionTransient + got.RejectionUnresolved
			if closed != got.Rejections || got.RejectionOpen != 0 {
				t.Errorf("%s adaptive=%v: %d events, %d closed, %d open", c.name, adaptive, got.Rejections, closed, got.RejectionOpen)
			}
			if c.wantPersistent && (got.RejectionPersistent != got.Rejections || got.RejectionTransient != 0) {
				t.Errorf("%s adaptive=%v: a genuine manoeuvre read as %+v", c.name, adaptive, got)
			}
			if !c.wantPersistent && (got.RejectionTransient != got.Rejections || got.RejectionPersistent != 0) {
				t.Errorf("%s adaptive=%v: a spurious measurement read as %+v", c.name, adaptive, got)
			}
			if got.RejectedUnassigned != got.Rejections {
				t.Errorf("%s adaptive=%v: %d of %d rejected clusters left free to seed a track", c.name, adaptive, got.RejectedUnassigned, got.Rejections)
			}
			gated := 0
			for _, s := range w.Samples {
				if s.Gated {
					gated++
				}
			}
			if gated < got.Rejections {
				t.Errorf("%s adaptive=%v: %d gated samples for %d events", c.name, adaptive, gated, got.Rejections)
			}
		}
	}
}

// A sample taken after a missed frame says so: its prediction carries the
// occlusion inflation.
func TestUncertaintySampleRecordsCoasting(t *testing.T) {
	body := uncCar(-25, 18, 8)
	body.hidden = func(f int) bool { return f == 20 || f == 21 }
	scene := uncScene{frames: 30, period: 100 * time.Millisecond, seed: 3,
		bodies: []uncBody{body}, noise: uncNoise{0.03, 0.05}}
	w := runUncScene(t, DefaultTrackerConfig(), scene.render(), 10, true).UncertaintyWindow()
	coasted := 0
	for _, s := range w.Samples {
		if s.Misses > 0 {
			coasted++
			if s.Misses != 2 || s.PredRadial < 1 {
				t.Errorf("coasting sample %+v: want two misses and an inflated prediction", s)
			}
		}
	}
	if coasted != 1 {
		t.Errorf("%d coasting samples, want the one frame after the gap", coasted)
	}
}

// An event whose track ends inside the look-ahead closes as unresolved.
func TestGateRejectionOfAVanishingTrackIsUnresolved(t *testing.T) {
	body := uncCar(-30, 15, 12)
	body.spurious = map[int][2]float64{24: {0, 2.5}}
	body.hidden = func(f int) bool { return f > 24 }
	got := uncPreGateTotals(uncRejectionOutcomes(t, DefaultTrackerConfig(), body, 60))
	if got.Rejections != 1 || got.RejectionUnresolved != 1 {
		t.Errorf("vanishing track: %+v, want one unresolved event", got)
	}
}

// G-UNC-1's recovery row, on synthetic data: after five frames of occlusion,
// the track must be back within half a metre of the truth in under three
// frames, under the shipped model, the uncalibrated adaptive model and a
// calibrated one.
func TestRecoveryAfterFiveFrameOcclusionIsUnderThreeFrames(t *testing.T) {
	const occludedFrom, occludedTo = 25, 30 // five frames
	body := uncCar(-30, 15, 8)
	body.hidden = func(f int) bool { return f >= occludedFrom && f < occludedTo }
	scene := uncScene{frames: 45, period: 100 * time.Millisecond, seed: 13,
		bodies: []uncBody{body}, noise: uncNoise{0.03, 0.08}}
	frames := scene.render()

	cal := UniformNoiseCalibration(0.02)
	for ri := range cal.Coefficients[0] {
		for ni := range cal.Coefficients[0][ri] {
			for ai := range cal.Coefficients[0][ri][ni] {
				cal.Coefficients[0][ri][ni][ai] = [2]float32{0.01, 0.03}
			}
		}
	}
	shipped := DefaultTrackerConfig()
	prior := shipped
	prior.AdaptiveMeasurementNoise = true
	calibrated := prior
	calibrated.MeasurementNoiseCalibration = cal

	for _, c := range []struct {
		name string
		cfg  TrackerConfig
	}{{"shipped", shipped}, {"adaptive prior", prior}, {"adaptive calibrated", calibrated}} {
		tk := NewTracker(c.cfg)
		var trackSeq int64
		recovered := -1
		for i, f := range frames {
			tk.Update(f.clusters, f.at)
			if i == occludedFrom-1 {
				for _, tr := range tk.Tracks {
					if tr.TrackState == TrackConfirmed {
						trackSeq = tr.CreationSequence
					}
				}
				if trackSeq == 0 {
					t.Fatalf("%s: no confirmed track before the occlusion", c.name)
				}
			}
			if i < occludedTo || recovered >= 0 {
				continue
			}
			for _, tr := range tk.Tracks {
				if tr.CreationSequence != trackSeq || tr.Misses != 0 {
					continue
				}
				truth := f.truth[0]
				if math.Hypot(float64(tr.X)-truth.x, float64(tr.Y)-truth.y) <= 0.5 {
					recovered = i - occludedTo + 1
				}
			}
		}
		if recovered < 0 || recovered >= 3 {
			t.Errorf("%s: recovered after %d frames, want 1 or 2", c.name, recovered)
		}
	}
}

// Past the cap, samples are counted rather than kept.
func TestUncertaintySampleCapCountsDrops(t *testing.T) {
	scene := uncScene{frames: 30, period: 100 * time.Millisecond, seed: 2,
		bodies: []uncBody{uncCar(-20, 20, 10)}, noise: uncNoise{0.03, 0.05}}
	frames := scene.render()
	tk := NewTracker(DefaultTrackerConfig())
	tk.BeginUncertaintyCalibration()
	tk.uncertaintySampleCap = 5
	for _, f := range frames {
		tk.Update(f.clusters, f.at)
	}
	w := tk.UncertaintyWindow()
	if len(w.Samples) != 5 || w.SamplesDropped != 30-DefaultTrackerConfig().HitsToConfirm-5 {
		t.Errorf("kept %d, dropped %d", len(w.Samples), w.SamplesDropped)
	}
	tk.Reset()
	if w := tk.UncertaintyWindow(); len(w.Samples) != 0 || w.SamplesDropped != 0 {
		t.Errorf("Reset left a window: %+v", w)
	}
}

// Injected anisotropy reaches the samples on the right axes.
func TestUncertaintySamplesSeparateTheAxes(t *testing.T) {
	scene := uncScene{frames: 300, period: 100 * time.Millisecond, seed: 17,
		bodies: []uncBody{{x: 0, y: 25, lengthM: 4.5, widthM: 1.8, points: 60}},
		noise:  uncNoise{radialSigma: 0.03, tangentialSigma: 0.3}}
	tk := runUncScene(t, DefaultTrackerConfig(), scene.render(), 20, true)
	w := tk.UncertaintyWindow()
	var r2, t2 float64
	for _, s := range w.Samples {
		r2 += float64(s.InnovRadial) * float64(s.InnovRadial)
		t2 += float64(s.InnovTangential) * float64(s.InnovTangential)
		if s.PhysRadial <= 0 || s.PhysTangential <= 0 || s.NoiseRadial != DefaultTrackerConfig().MeasurementNoise {
			t.Fatalf("sample %+v lacks its physics or model terms", s)
		}
	}
	if len(w.Samples) < 200 || !(t2 > 20*r2) {
		t.Errorf("%d samples, tangential sum of squares %v against radial %v", len(w.Samples), t2, r2)
	}
}

// The shipped initialiser starts a track at zero velocity with a velocity
// variance of 1 (m/s)² and no position-velocity covariance, so its third
// observation of a fast body lands far outside the prediction. Under the
// shipped R and gate that fragments any body faster than about 14.5 m/s at
// 10 Hz, and a smaller R lowers the ceiling: with a calibrated coefficient of
// 0.01 m² a 12 m/s body is never acquired. This pins the shipped behaviour as
// evidence for G-UNC-1: a fitted R cannot be judged on NIS alone, because the
// gate it tightens also governs birth. When the initialiser is fixed these
// assertions flip.
func TestZeroVelocityInitialiserAcquisitionCeiling(t *testing.T) {
	births := func(cfg TrackerConfig, speed float64) int {
		scene := uncScene{frames: 30, period: 100 * time.Millisecond, seed: 11,
			bodies: []uncBody{uncCar(-30, 15, speed)}, noise: uncNoise{0.02, 0.03}}
		return runUncScene(t, cfg, scene.render(), 0, false).TracksCreated
	}
	shipped := DefaultTrackerConfig()
	if n := births(shipped, 14); n != 1 {
		t.Errorf("shipped at 14 m/s: %d births, want one track", n)
	}
	if n := births(shipped, 15); n < 3 {
		t.Errorf("shipped at 15 m/s: %d births; the acquisition ceiling has moved", n)
	}
	calibrated := shipped
	calibrated.AdaptiveMeasurementNoise = true
	calibrated.MeasurementNoiseCalibration = UniformNoiseCalibration(0.01)
	if n := births(calibrated, 12); n < 10 {
		t.Errorf("calibrated 0.01 m² at 12 m/s: %d births; the ceiling has moved", n)
	}
	if n := births(calibrated, 6); n != 1 {
		t.Errorf("calibrated 0.01 m² at 6 m/s: %d births, want one track", n)
	}
}

func TestPreGateBandsSummarise(t *testing.T) {
	var b PreGateBands
	b[1] = PreGateAccumulator{Candidates: 12, Eligible: 8, NISSum: 20, NISOverThreshold: 2, Gated: 1, Assigned: 7,
		Rejections: 1, RejectedUnassigned: 1, RejectionPersistent: 1}
	var open [ResidualBandCount]int
	open[3] = 2
	got := b.Summarise(open)
	if len(got) != 2 {
		t.Fatalf("got %d bands, want the populated one and the one with open events: %+v", len(got), got)
	}
	s := got[0]
	if s.SpeedFloorMps != ResidualSpeedBands[1] || s.MeanNIS != 2.5 || s.NISExceedanceRatio != 0.25 ||
		s.GatedRatio != 0.125 || s.AssignedRatio != 0.875 || s.RejectionPersistent != 1 {
		t.Errorf("summary %+v", s)
	}
	if got[1].SpeedFloorMps != ResidualSpeedBands[3] || got[1].RejectionOpen != 2 || got[1].MeanNIS != 0 {
		t.Errorf("open-only band %+v", got[1])
	}
	if len(PreGateBands{}.Summarise([ResidualBandCount]int{})) != 0 {
		t.Error("empty bands were reported")
	}
}
