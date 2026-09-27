package l5tracks

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// A body north of the sensor heading east is broadside to it, with the sensor
// on its right; the same body heading north has the sensor dead astern.
func TestMeasurementGeometryForAspect(t *testing.T) {
	broadside := MeasurementGeometryFor(0, 20, 0, 0, 0, 50)
	if !broadside.Valid || !approx(float64(broadside.RangeMetres), 20, 1e-5) {
		t.Fatalf("geometry %+v", broadside)
	}
	if !approx(float64(broadside.RadialX), 0, 1e-6) || !approx(float64(broadside.RadialY), 1, 1e-6) {
		t.Errorf("line of sight (%v, %v), want (0, 1)", broadside.RadialX, broadside.RadialY)
	}
	if !approx(float64(broadside.AspectRad), 3*math.Pi/2, 1e-5) {
		t.Errorf("aspect %v, want 3π/2 (sensor on the body's right)", broadside.AspectRad)
	}
	if !approx(float64(broadside.FoldedAspectRad()), math.Pi/2, 1e-5) || broadside.AspectOctant() != 6 {
		t.Errorf("folded %v octant %d, want π/2 and 6", broadside.FoldedAspectRad(), broadside.AspectOctant())
	}

	// Heading just west of north: the sensor is astern, a little to port of
	// the octant boundary a float32 π would otherwise straddle.
	astern := MeasurementGeometryFor(0, 20, 0, 0, math.Pi/2-0.1, 50)
	if !approx(float64(astern.AspectRad), math.Pi+0.1, 1e-5) || !approx(float64(astern.FoldedAspectRad()), 0.1, 1e-5) || astern.AspectOctant() != 4 {
		t.Errorf("astern aspect %v folded %v octant %d, want π+0.1, 0.1 and 4", astern.AspectRad, astern.FoldedAspectRad(), astern.AspectOctant())
	}
	// A sensor origin away from zero moves the line of sight with it.
	offset := MeasurementGeometryFor(10, 20, 10, 0, 0, 50)
	if !approx(float64(offset.RangeMetres), 20, 1e-5) || !approx(float64(offset.RadialY), 1, 1e-6) {
		t.Errorf("offset sensor geometry %+v", offset)
	}
}

func TestMeasurementGeometryRefusesWithoutLineOfSight(t *testing.T) {
	if g := MeasurementGeometryFor(0.1, 0.1, 0, 0, 0, 10); g.Valid {
		t.Errorf("a measurement at the sensor has a line of sight: %+v", g)
	}
	if g := MeasurementGeometryFor(5, 5, 0, 0, float32(math.NaN()), 10); g.Valid {
		t.Errorf("a NaN heading produced an aspect: %+v", g)
	}
	if r, tan := SensorPhysicsNoise(MeasurementGeometry{}); r != 0 || tan != 0 {
		t.Errorf("invalid geometry physics (%v, %v), want zero", r, tan)
	}
}

// The Section 8.1 terms, by hand: range accuracy plus grazing spread along the
// line of sight, quantisation plus averaging across it.
func TestSensorPhysicsNoiseMatchesSection81(t *testing.T) {
	footprint := 30 * PandarAzimuthStepRad

	broadside := MeasurementGeometryFor(0, 30, 0, 0, 0, 10)
	r, tan := SensorPhysicsNoise(broadside)
	if !approx(float64(r), PandarRangeSigmaMetres*PandarRangeSigmaMetres, 1e-9) {
		t.Errorf("broadside radial %v: the face is square on, so only range accuracy remains", r)
	}
	if want := footprint*footprint/12 + footprint*footprint/10; !approx(float64(tan), want, 1e-9) {
		t.Errorf("tangential %v, want %v", tan, want)
	}

	// At 45 degrees both faces are at grazing incidence and the spread peaks.
	oblique := MeasurementGeometryFor(0, 30, 0, 0, math.Pi/4, 10)
	r45, _ := SensorPhysicsNoise(oblique)
	if want := PandarRangeSigmaMetres*PandarRangeSigmaMetres + footprint*footprint; !approx(float64(r45), want, 1e-7) {
		t.Errorf("oblique radial %v, want %v", r45, want)
	}

	// Farther is noisier across; more support is quieter across.
	_, near := SensorPhysicsNoise(MeasurementGeometryFor(0, 10, 0, 0, 0, 10))
	_, sparse := SensorPhysicsNoise(MeasurementGeometryFor(0, 30, 0, 0, 0, 2))
	if !(near < tan) || !(sparse > tan) {
		t.Errorf("tangential does not grow with range (%v < %v) and shrink with support (%v > %v)", near, tan, sparse, tan)
	}
}

func TestNoiseStratumBins(t *testing.T) {
	cases := []struct {
		x, y    float32
		heading float32
		support int
		source  MeasurementSource
		want    NoiseStratum
	}{
		{0, 5, 0, 5, MeasurementMedoidV0, NoiseStratum{0, 0, 0, 2}},
		{0, 25, math.Pi / 2, 40, MeasurementOBBCentreV1, NoiseStratum{1, 2, 2, 0}},
		{0, 75, math.Pi / 4, 500, MeasurementMedoidFallbackV1, NoiseStratum{2, 4, 3, 1}},
		{0, 10, 0, 10, MeasurementMedoidV0, NoiseStratum{0, 1, 1, 2}},
	}
	for _, c := range cases {
		got, ok := NoiseStratumFor(c.source, MeasurementGeometryFor(c.x, c.y, 0, 0, c.heading, c.support))
		if !ok || got != c.want {
			t.Errorf("(%v, %v) heading %v support %d %s: got %+v ok=%v, want %+v", c.x, c.y, c.heading, c.support, c.source, got, ok, c.want)
		}
	}
	if _, ok := NoiseStratumFor(MeasurementNearEdgeCandidateV1, MeasurementGeometryFor(0, 5, 0, 0, 0, 5)); ok {
		t.Error("the offline near-edge source has no online row but was placed in the table")
	}
}

// The rotation preserves the two variances as eigenvalues and puts the
// correlation where the line of sight says.
func TestRotateSensorNoise(t *testing.T) {
	along := rotateSensorNoise(0.01, 0.5, 1, 0)
	if along.XX != 0.01 || along.YY != 0.5 || along.XY != 0 {
		t.Errorf("line of sight along x: %+v", along)
	}
	s := float32(math.Sqrt2 / 2)
	diag := rotateSensorNoise(0.01, 0.5, s, s)
	if !approx(float64(diag.XY), (0.01-0.5)/2, 1e-6) || !approx(float64(diag.XX), 0.255, 1e-6) {
		t.Errorf("line of sight at 45 degrees: %+v", diag)
	}
	trace := float64(diag.XX + diag.YY)
	det := float64(diag.XX*diag.YY - diag.XY*diag.XY)
	if !approx(trace, 0.51, 1e-6) || !approx(det, 0.005, 1e-6) {
		t.Errorf("rotation changed the eigenvalues: trace %v det %v", trace, det)
	}
}

func TestNoiseCalibrationValidate(t *testing.T) {
	if err := UniformNoiseCalibration(0.05).Validate(); err != nil {
		t.Fatalf("uniform table refused: %v", err)
	}
	var nilCal *NoiseCalibration
	if err := nilCal.Validate(); err == nil {
		t.Error("nil calibration accepted")
	}
	for _, bad := range []float32{0, -0.1, float32(math.NaN()), float32(math.Inf(1))} {
		c := UniformNoiseCalibration(0.05)
		c.Coefficients[1][2][3][1][NoiseAxisTangential] = bad
		if err := c.Validate(); err == nil {
			t.Errorf("coefficient %v accepted", bad)
		}
	}
}

func adaptiveTestTrack(heading float32) *TrackedObject {
	return &TrackedObject{
		TrackID: "t", X: 0, Y: 20, OBBHeadingRad: heading,
		P: [16]float32{
			0.02, 0, 0, 0,
			0, 0.02, 0, 0,
			0, 0, 0.1, 0,
			0, 0, 0, 0.1,
		},
	}
}

func adaptiveTestCluster(x, y float32, points int) WorldCluster {
	return WorldCluster{CentroidX: x, CentroidY: y, PointsCount: points,
		BoundingBoxLength: 4.5, BoundingBoxWidth: 1.8,
		OBB: &l4perception.OrientedBoundingBox{CenterX: x, CenterY: y, Length: 4.5, Width: 1.8}}
}

func TestEvaluateNoiseReadsTheCalibrationCell(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.AdaptiveMeasurementNoise = true
	tk := NewTracker(cfg)
	track := adaptiveTestTrack(0)
	cluster := adaptiveTestCluster(0, 20, 50)
	m := tk.measurementForCluster(cluster, 0)

	prior := tk.evaluateNoise(track, cluster, m)
	if prior.CalibrationUsed || prior.CoefRadial != cfg.MeasurementNoise || prior.CoefTangential != cfg.MeasurementNoise {
		t.Fatalf("uncalibrated model did not use the shipped variance as its coefficient: %+v", prior)
	}
	if prior.Radial != prior.CoefRadial+prior.PhysRadial || prior.Tangential != prior.CoefTangential+prior.PhysTangential {
		t.Errorf("axis variance is not coefficient plus physics: %+v", prior)
	}

	cal := UniformNoiseCalibration(0.05)
	cal.Coefficients[0][2][2][2] = [2]float32{0.003, 0.7}
	cfg.MeasurementNoiseCalibration = cal
	tk = NewTracker(cfg)
	got := tk.evaluateNoise(track, cluster, m)
	if !got.CalibrationUsed || got.Stratum != (NoiseStratum{0, 2, 2, 2}) || got.CoefRadial != 0.003 || got.CoefTangential != 0.7 {
		t.Fatalf("calibrated evaluation %+v", got)
	}
	// The line of sight is +y here, so radial is YY and tangential XX.
	if !approx(float64(got.SiteCovariance.YY), float64(got.Radial), 1e-7) || !approx(float64(got.SiteCovariance.XX), float64(got.Tangential), 1e-7) {
		t.Errorf("site covariance %+v does not put radial on the line of sight", got.SiteCovariance)
	}

	atSensor := tk.evaluateNoise(track, adaptiveTestCluster(0.1, 0, 50), tk.measurementForCluster(adaptiveTestCluster(0.1, 0, 50), 0))
	if atSensor.FallbackReason != "no_line_of_sight" || atSensor.SiteCovariance != (MeasurementCovariance{XX: cfg.MeasurementNoise, YY: cfg.MeasurementNoise}) {
		t.Errorf("no line of sight did not fall back to the shipped scalar: %+v", atSensor)
	}
}

// The option is the only switch. A calibration and a sensor origin with the
// option off must leave the tracker's output bit for bit as shipped.
func TestAdaptiveNoiseOffIsShippedBehaviour(t *testing.T) {
	shipped := DefaultTrackerConfig()
	shipped.JosephCovarianceUpdate = true
	shipped.LikelihoodAssociationCost = true
	armed := shipped
	cal := UniformNoiseCalibration(0.4)
	armed.MeasurementNoiseCalibration = cal
	armed.NoiseSensorX, armed.NoiseSensorY = 3, -2

	a, b := NewTracker(shipped), NewTracker(armed)
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 40; i++ {
		x := float32(-20 + 1.2*float64(i))
		clusters := []WorldCluster{adaptiveTestCluster(x, 20+0.05*float32(i%3), 60), adaptiveTestCluster(5, 8, 12)}
		a.Update(clusters, now)
		b.Update(clusters, now)
		now = now.Add(100 * time.Millisecond)
	}
	if len(a.Tracks) != len(b.Tracks) {
		t.Fatalf("track counts differ: %d vs %d", len(a.Tracks), len(b.Tracks))
	}
	for _, ta := range a.Tracks {
		var match *TrackedObject
		for _, tb := range b.Tracks {
			if tb.CreationSequence == ta.CreationSequence {
				match = tb
			}
		}
		if match == nil || match.X != ta.X || match.Y != ta.Y || match.VX != ta.VX || match.VY != ta.VY || match.P != ta.P {
			t.Fatalf("track %d differs with the option off", ta.CreationSequence)
		}
	}
}

// With the line of sight along +y, a calibration that trusts range and
// distrusts bearing must gate a radial miss much harder than a tangential one
// of the same size. The isotropic model cannot tell them apart.
func TestAdaptiveNoiseGatesAlongTheLineOfSight(t *testing.T) {
	cal := UniformNoiseCalibration(0.05)
	for ri := range cal.Coefficients[0] {
		for ni := range cal.Coefficients[0][ri] {
			for ai := range cal.Coefficients[0][ri][ni] {
				cal.Coefficients[0][ri][ni][ai] = [2]float32{0.005, 1.0}
			}
		}
	}
	iso := DefaultTrackerConfig()
	adaptive := iso
	adaptive.AdaptiveMeasurementNoise = true
	adaptive.MeasurementNoiseCalibration = cal

	for _, c := range []struct {
		name string
		cfg  TrackerConfig
	}{{"isotropic", iso}, {"adaptive", adaptive}} {
		tk := NewTracker(c.cfg)
		track := adaptiveTestTrack(0)
		radialMiss := tk.mahalanobisDistanceSquared(track, adaptiveTestCluster(0, 21, 50), 0)
		tangentialMiss := tk.mahalanobisDistanceSquared(track, adaptiveTestCluster(1, 20, 50), 0)
		switch c.name {
		case "isotropic":
			if !approx(float64(radialMiss), float64(tangentialMiss), 1e-4) {
				t.Errorf("isotropic model separated the axes: %v vs %v", radialMiss, tangentialMiss)
			}
		default:
			if !(radialMiss > 20*tangentialMiss) {
				t.Errorf("adaptive model: radial d² %v is not far above tangential d² %v", radialMiss, tangentialMiss)
			}
		}
	}
}

// The update follows the same R: a tangential innovation moves the state less
// than a radial one of the same size when bearing is the distrusted axis.
func TestAdaptiveNoiseUpdateGainFollowsTheAxis(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.AdaptiveMeasurementNoise = true
	cal := UniformNoiseCalibration(0.05)
	cal.Coefficients[0][2][2][2] = [2]float32{0.005, 1.0}
	cfg.MeasurementNoiseCalibration = cal
	for _, joseph := range []bool{false, true} {
		cfg.JosephCovarianceUpdate = joseph
		tk := NewTracker(cfg)

		radial := adaptiveTestTrack(0)
		tk.update(radial, adaptiveTestCluster(0, 21, 50), 0)
		tangential := adaptiveTestTrack(0)
		tk.update(tangential, adaptiveTestCluster(1, 20, 50), 0)

		movedRadially := float64(radial.Y - 20)
		movedTangentially := float64(tangential.X)
		if !(movedRadially > 0.7) || !(movedTangentially < 0.05) {
			t.Errorf("joseph=%v: radial correction %v, tangential %v", joseph, movedRadially, movedTangentially)
		}
		if radial.P[1*4+1] >= 0.02 || !(tangential.P[0*4+0] > 0.019) {
			t.Errorf("joseph=%v: posterior variance did not follow the axis: %v, %v", joseph, radial.P[5], tangential.P[0])
		}
		if asym := covarianceAsymmetry(radial.P); joseph && asym > 1e-7 {
			t.Errorf("joseph form left P asymmetric by %v", asym)
		}
	}
}

// With R = r·I the full Joseph form must agree with the isotropic one.
func TestJosephFullMatchesIsotropic(t *testing.T) {
	p := [16]float32{
		0.3, 0.05, 0.1, 0.01,
		0.05, 0.2, 0.02, 0.08,
		0.1, 0.02, 0.9, 0.03,
		0.01, 0.08, 0.03, 0.7,
	}
	k := [8]float32{0.6, 0.05, 0.04, 0.5, 0.2, 0.01, 0.02, 0.15}
	want := josephCovarianceUpdate(p, k, 0.05)
	got := josephCovarianceUpdateFull(p, k, MeasurementCovariance{XX: 0.05, YY: 0.05})
	for i := range want {
		if !approx(float64(got[i]), float64(want[i]), 1e-6) {
			t.Fatalf("element %d: full %v, isotropic %v", i, got[i], want[i])
		}
	}
}
