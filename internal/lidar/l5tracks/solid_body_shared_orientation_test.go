package l5tracks

import (
	"math"
	"math/rand"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Tests for SolidBodyOptions.SharedOrientation (solid_body_shared_orientation.go).
// What carries the weight: on a turning box whose axis and course
// observations are the truth plus noise at exactly the sigma the box filter
// is told, the shared belief's heading error falls with observations, its
// variance is the variance its errors have (normalised estimation error and
// the axis innovation's NIS both average about one), and the same diagnostic
// reads a factor of four when the noise is twice what the filter is told;
// through the tracker the solid body's orientation is the box's posterior,
// faces and all, and an unlabelled box leaves it the shipped orientation.

// sharedCalibrationBand is the band the mean normalised errors must fall in
// for the variance to count as calibrated: within a factor of two of one,
// that is within a factor of the square root of two in sigma. It is stated
// before the run, not fitted to it.
const sharedCalibrationLo, sharedCalibrationHi = 0.5, 2.0

// turningRun is the box filter on one noisy turning pass.
type turningRun struct {
	earlyErr2, lateErr2 []float64 // squared heading errors, rad²
	earlyVar, lateVar   []float64 // the filter's P_psi-psi at the same frames
	nees                []float64 // err²/P_psi-psi after the first second
	nis                 []float64 // accepted axis innovations' r²/S
	gated               int
}

// runTurningBox drives a box estimator, PCA axis source, over a 4.5 by 1.8 m
// car on a 20 m arc at 8 m/s for 6 s (a 137° turn), its OBB heading the true
// heading plus N(0, (noiseScale·σ_axis)²) and its velocity the true tangent
// turned by N(0, (noiseScale·σ_course)²), where σ_axis and σ_course are the
// sigmas the filter weighs the two observations with. Noise-free truth plus
// injected noise of a known sigma: the filter's model is then exact except
// for its yaw-acceleration process noise, which the arc does not use.
func runTurningBox(rng *rand.Rand, noiseScale float64) turningRun {
	cfg := enabledBoxConfig()
	cfg.AxisSource = BoxAxisPCA
	axisSigma := boxPCAAxisSigmaRad * cfg.AxisSigmaScale
	// A velocity covariance small against the sideslip allowance, so the
	// course's variance is the sideslip's.
	const velVar = 1e-4
	const radius, speed, dt = 20.0, 8.0, 0.1
	courseSigma := math.Sqrt(velVar/(speed*speed) + cfg.CourseSideslipRad*cfg.CourseSideslipRad)
	e := NewBoxEstimator(cfg)
	omega := speed / radius
	var r turningRun
	for i := 0; i < 60; i++ {
		phi := -math.Pi/2 + omega*dt*float64(i)
		cx, cy := radius*math.Cos(phi), 25+radius*math.Sin(phi)
		heading := phi + math.Pi/2
		pts := visibleFaces(cx, cy, heading, 4.5, 1.8, 20)
		course := heading + noiseScale*courseSigma*rng.NormFloat64()
		obb := &l4perception.OrientedBoundingBox{CenterX: float32(cx), CenterY: float32(cy), Length: 4.5, Width: 1.8,
			HeadingRad: float32(heading + noiseScale*axisSigma*rng.NormFloat64())}
		mx, my := medoid(pts)
		e.Observe(BoxInput{Nanos: int64(i) * 1e8, X: mx, Y: my, VX: speed * math.Cos(course), VY: speed * math.Sin(course),
			VelCov: [3]float64{velVar, 0, velVar}, Cluster: &WorldCluster{Members: pts, OBB: obb}})
		if a := e.axisInnovation; a.known && i > 0 {
			if a.gated {
				r.gated++
			} else {
				r.nis = append(r.nis, a.nis)
			}
		}
		if !e.labelled {
			continue
		}
		err := wrapPi(e.psi - heading)
		switch {
		case i < 4:
			r.earlyErr2, r.earlyVar = append(r.earlyErr2, err*err), append(r.earlyVar, e.p[0][0])
		case i >= 40:
			r.lateErr2, r.lateVar = append(r.lateErr2, err*err), append(r.lateVar, e.p[0][0])
		}
		if i >= 10 {
			r.nees = append(r.nees, err*err/e.p[0][0])
		}
	}
	return r
}

// turningSummary pools runs.
type turningSummary struct {
	earlyRMS, lateRMS     float64 // degrees
	earlySigma, lateSigma float64 // the filter's, degrees
	nees, nis             float64
	gatedShare            float64
}

func summariseTurning(noiseScale float64, runs int) turningSummary {
	rng := rand.New(rand.NewSource(11))
	var all turningRun
	for k := 0; k < runs; k++ {
		r := runTurningBox(rng, noiseScale)
		all.earlyErr2 = append(all.earlyErr2, r.earlyErr2...)
		all.lateErr2 = append(all.lateErr2, r.lateErr2...)
		all.earlyVar = append(all.earlyVar, r.earlyVar...)
		all.lateVar = append(all.lateVar, r.lateVar...)
		all.nees = append(all.nees, r.nees...)
		all.nis = append(all.nis, r.nis...)
		all.gated += r.gated
	}
	deg := 180 / math.Pi
	return turningSummary{
		earlyRMS: math.Sqrt(meanOf(all.earlyErr2)) * deg, lateRMS: math.Sqrt(meanOf(all.lateErr2)) * deg,
		earlySigma: math.Sqrt(meanOf(all.earlyVar)) * deg, lateSigma: math.Sqrt(meanOf(all.lateVar)) * deg,
		nees: meanOf(all.nees), nis: meanOf(all.nis),
		gatedShare: float64(all.gated) / float64(all.gated+len(all.nis)),
	}
}

func TestSharedOrientationBeliefIsCalibratedOnATurningBox(t *testing.T) {
	calibrated := summariseTurning(1, 40)
	// The first frames' error is about twice the filter's sigma: a labelled
	// seed takes (5°)² whatever the axis observation's sigma, which here is
	// 12°. That is the shipped seed, reported rather than changed; the
	// calibration is read from the second second on.
	t.Logf("noise at the filter's sigmas: heading RMS %.2f° in the first 4 frames, %.2f° after 4 s; filter sigma %.2f° and %.2f°; "+
		"mean NEES %.2f, mean axis NIS %.2f, %.1f %% of axis observations gated",
		calibrated.earlyRMS, calibrated.lateRMS, calibrated.earlySigma, calibrated.lateSigma,
		calibrated.nees, calibrated.nis, 100*calibrated.gatedShare)
	if calibrated.lateRMS >= calibrated.earlyRMS || calibrated.lateSigma >= calibrated.earlySigma {
		t.Errorf("the heading error (%.2f° to %.2f°) and its sigma (%.2f° to %.2f°) must fall with observations",
			calibrated.earlyRMS, calibrated.lateRMS, calibrated.earlySigma, calibrated.lateSigma)
	}
	for name, v := range map[string]float64{"NEES": calibrated.nees, "axis NIS": calibrated.nis} {
		if v < sharedCalibrationLo || v > sharedCalibrationHi {
			t.Errorf("mean %s %.2f outside [%.1f, %.1f]: the shared variance is not the variance its errors have",
				name, v, sharedCalibrationLo, sharedCalibrationHi)
		}
	}
	// The diagnostic must see miscalibration: noise twice the filter's
	// sigma is four times its variance.
	over := summariseTurning(2, 40)
	t.Logf("noise at twice the filter's sigmas: mean NEES %.2f, mean axis NIS %.2f, %.1f %% gated",
		over.nees, over.nis, 100*over.gatedShare)
	if over.nis <= sharedCalibrationHi || over.nees <= sharedCalibrationHi {
		t.Errorf("with noise twice the filter's sigma the mean NIS is %.2f and NEES %.2f; the diagnostic should read above %.1f",
			over.nis, over.nees, sharedCalibrationHi)
	}
}

// Through the tracker on the through-truck pass: the solid body's
// orientation is the box filter's posterior on every frame the box is
// labelled, the faces are found along it, the axis innovation reaches the
// measurement, and the solid body's heading error, which the tracked PCA
// heading leaves up to a quarter turn, is the box's.
func TestSharedOrientationGivesTheSolidBodyTheBoxHeading(t *testing.T) {
	frames := syntheticPassFrames(t, throughTruckPass())
	run := func(shared bool) (sharedFrames, fallback, nisFrames int, worstDeg, meanWidth float64) {
		cfg := faceStateConfig()
		cfg.SolidBody.SharedOrientation = shared
		tracker := NewTracker(cfg)
		var widths []float64
		for _, f := range frames {
			tracker.Update(f.clusters, f.at)
			tracks := tracker.GetActiveTracks()
			if len(tracks) == 0 {
				continue
			}
			tr := mainTrack(t, tracker)
			sb := tr.solidBody
			if !sb.seeded || sb.orientation.Provenance == ProvenanceNone {
				continue
			}
			if shared && sb.shared {
				sharedFrames++
				b := tr.box
				if sb.orientation.PsiRad != float32(b.psi) || sb.orientation.VarianceRad2 != float32(b.p[0][0]) || sb.orientation.AmbiguousModeWeight != 0 {
					t.Fatalf("shared orientation %+v is not the box posterior (%v, %v)", sb.orientation, b.psi, b.p[0][0])
				}
				if sb.reading.Measurement.HeadingNISKnown {
					nisFrames++
				}
			} else if shared {
				fallback++
				if tr.box != nil && tr.box.labelled && tr.box.lastNanos == tracker.LastUpdateNanos && !sb.tracked && sb.support.CoastedFrames == 0 {
					t.Fatalf("a labelled box's heading was not shared")
				}
			}
			if sb.reference == ReferenceBodyCentre && f.truthX > -25 && f.truthX < 25 {
				worstDeg = math.Max(worstDeg, math.Abs(float64(angleDifference(sb.orientation.PsiRad, 0)))*180/math.Pi)
				widths = append(widths, float64(2*sb.ext.h[1]))
			}
		}
		return sharedFrames, fallback, nisFrames, worstDeg, meanOf(widths)
	}
	_, _, _, shippedWorst, shippedWidth := run(false)
	n, fallback, nis, worst, width := run(true)
	t.Logf("shipped orientation: worst heading error %.1f°, mean 2hW %.2f m; shared: %d frames shared, %d fell back, %d with a heading NIS, worst heading error %.1f°, mean 2hW %.2f m (true 2.5 m)",
		shippedWorst, shippedWidth, n, fallback, nis, worst, width)
	if n == 0 || nis == 0 {
		t.Fatalf("%d frames shared the box's heading and %d carried its innovation", n, nis)
	}
	// The span window is the bound the extent admission already believes an
	// axis within.
	if worst > spanSearchHalfWindowDeg || worst >= shippedWorst {
		t.Errorf("worst body-centre heading error %.1f° with the shared belief, %.1f° without; want within %d° and better",
			worst, shippedWorst, spanSearchHalfWindowDeg)
	}
	if math.Abs(width-2.5) >= math.Abs(shippedWidth-2.5) {
		t.Errorf("mean 2hW %.2f m with the shared belief, %.2f m without; faces named by the right axis should measure the width better", width, shippedWidth)
	}
}

// A box that has not resolved which axis is the length cannot state its
// heading as two modes, so the solid body keeps the orientation it would
// have without the option; and with the option off nothing reads the box.
func TestSharedOrientationNeedsALabelledBoxOfThisFrame(t *testing.T) {
	cfg := solidBodyConfig()
	cfg.SolidBody.SharedOrientation = true
	tracker := NewTracker(cfg)
	tracker.LastUpdateNanos = 5e8
	track := boxPassBody(0, 5, 7)
	if _, ok := tracker.sharedOrientation(track); ok {
		t.Fatal("a track with no box shared a heading")
	}
	track.box = NewBoxEstimator(cfg.BoxModel)
	track.box.seeded, track.box.psi, track.box.p[0][0], track.box.lastNanos = true, 0.3, 0.01, 5e8
	if _, ok := tracker.sharedOrientation(track); ok {
		t.Fatal("an unlabelled box shared a heading")
	}
	track.box.labelled = true
	o, ok := tracker.sharedOrientation(track)
	if !ok || o.PsiRad != 0.3 || o.VarianceRad2 != 0.01 || !o.IsResolved() {
		t.Fatalf("a labelled box: %+v %v, want its heading and variance, resolved", o, ok)
	}
	track.box.lastNanos = 4e8
	if _, ok := tracker.sharedOrientation(track); ok {
		t.Fatal("a box not updated this frame shared a stale heading")
	}
	track.box.lastNanos = 5e8
	track.solidBody.tracked = true
	if _, ok := tracker.sharedOrientation(track); ok {
		t.Fatal("the tracked body took the shared heading")
	}
	track.solidBody.tracked = false
	tracker.Config.SolidBody.SharedOrientation = false
	if _, ok := tracker.sharedOrientation(track); ok {
		t.Fatal("the option off shared a heading")
	}
}
