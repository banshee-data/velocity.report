package l5tracks

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Tests for the near-edge measurement model against the synthetic pass that
// Section 3 of the state-estimation plan drew its evidence from.
//
// The comparison is the point: the OBB centre and the near-edge measurement are
// run over the same frames, from the same production clustering, so the
// difference is attributable to the measurement definition and nothing else.

// syntheticEvaluation runs a pass and returns per-frame errors for both
// measurement definitions.
type syntheticEvaluation struct {
	obbLateral  []float64
	nearLateral []float64
	nearAlong   []float64
	ranks       []int
}

// evaluateSyntheticPass drives both measurements over the default pass.
// halfWidthError is added to the width prior to model a dimension belief that
// is wrong by a known amount.
func evaluateSyntheticPass(t *testing.T, halfWidthError float32) syntheticEvaluation {
	t.Helper()
	pass := l4perception.DefaultSyntheticPass()
	frames := l4perception.GenerateSyntheticPass(pass)
	clusterer := l4perception.NewDBSCANClusterer(0.6, 5)

	var eval syntheticEvaluation
	for _, f := range frames {
		cluster, points, ok := largestSyntheticCluster(clusterer.Cluster(f.Points, pass.SensorID, f.Timestamp), f.Points)
		if !ok || cluster.OBB == nil {
			continue
		}

		set := MeasureNearEdge(NearEdgeInput{
			Cluster: cluster,
			Points:  points,
			SensorX: 0, SensorY: 0,
			// The vehicle travels along +X, so this is its true heading. The
			// model needs a resolved orientation and this isolates the
			// measurement definition from heading error.
			HeadingRad:       0,
			HalfLength:       float32(pass.Vehicle.Length / 2),
			HalfWidth:        float32(pass.Vehicle.Width/2) + halfWidthError,
			LengthProvenance: ProvenanceClassPrior,
			WidthProvenance:  ProvenanceClassPrior,
			PredictedX:       float32(f.TrueCentreX),
			PredictedY:       float32(f.TrueCentreY),
		})

		eval.obbLateral = append(eval.obbLateral, math.Abs(float64(cluster.OBB.CenterY)-f.TrueCentreY))
		eval.ranks = append(eval.ranks, set.Rank)
		if set.Rank == 0 {
			continue
		}
		lateral := math.Abs(float64(set.CentreY) - f.TrueCentreY)
		along := math.Abs(float64(set.CentreX) - f.TrueCentreX)
		eval.nearLateral = append(eval.nearLateral, lateral)
		eval.nearAlong = append(eval.nearAlong, along)
	}
	return eval
}

func largestSyntheticCluster(clusters []l4perception.WorldCluster, framePoints []l4perception.WorldPoint) (l4perception.WorldCluster, []l4perception.WorldPoint, bool) {
	best, found := l4perception.WorldCluster{}, false
	for _, c := range clusters {
		if !found || c.PointsCount > best.PointsCount {
			best, found = c, true
		}
	}
	if !found {
		return best, nil, false
	}
	// The synthetic clusterer does not retain member points, so recover them by
	// proximity to the cluster. Generous bounds: the aim is the vehicle's own
	// returns, not a tight refit.
	var points []l4perception.WorldPoint
	for _, p := range framePoints {
		if math.Abs(p.X-float64(best.CentroidX)) < 4 && math.Abs(p.Y-float64(best.CentroidY)) < 3 {
			points = append(points, p)
		}
	}
	return best, points, true
}

func meanOf(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func maxOf(v []float64) float64 {
	worst := 0.0
	for _, x := range v {
		if x > worst {
			worst = x
		}
	}
	return worst
}

func TestNearEdgeBeatsTheOBBCentreOnTheSyntheticPass(t *testing.T) {
	// Section 3.3's result, as a bound rather than a table. With a correct
	// dimension prior the measurement is exact where the near face is visible,
	// because the centroid of a sampled plane is on that plane and the rest is
	// arithmetic.
	eval := evaluateSyntheticPass(t, 0)

	if len(eval.nearLateral) < 30 {
		t.Fatalf("only %d frames measured, too few to bound", len(eval.nearLateral))
	}

	obbMean := meanOf(eval.obbLateral)
	nearMean := meanOf(eval.nearLateral)
	t.Logf("lateral error over %d frames: OBB centre mean %.4f m (max %.4f), near edge mean %.4f m (max %.4f)",
		len(eval.nearLateral), obbMean, maxOf(eval.obbLateral), nearMean, maxOf(eval.nearLateral))

	if nearMean >= obbMean {
		t.Errorf("near-edge lateral error %.4f is no better than the OBB centre's %.4f", nearMean, obbMean)
	}
	// The bound. Section 3.3 reports 0.035 m with an estimated dimension; with
	// the true dimension the lateral reconstruction is exact, so this is tight
	// on purpose: it would catch a sign error or an off-by-half that a loose
	// bound would let through.
	if nearMean > 0.01 {
		t.Errorf("near-edge lateral mean %.4f m exceeds the bound with a correct dimension prior", nearMean)
	}
	if worst := maxOf(eval.nearLateral); worst > 0.05 {
		t.Errorf("worst near-edge lateral error %.4f m: the measurement should be exact where the near face is visible", worst)
	}
}

func TestNearEdgeErrorIsExactlyTheDimensionPriorError(t *testing.T) {
	// The property that makes the dimension belief's uncertainty and provenance
	// load-bearing rather than decorative. The centre is reconstructed as the
	// measured face plus a believed half-extent, so an error in that belief
	// passes through to the position one-for-one.
	//
	// This is a far better failure mode than the OBB centre's, which hops
	// unpredictably as the visible faces change. But it is not free: it
	// converts a geometry error into a dimension-belief error, which is why the
	// belief carries a sigma and says where it came from.
	for _, priorError := range []float32{0.1, -0.1, 0.25} {
		eval := evaluateSyntheticPass(t, priorError)
		if len(eval.nearLateral) == 0 {
			t.Fatal("no frames measured")
		}
		got := meanOf(eval.nearLateral)
		want := math.Abs(float64(priorError))
		if math.Abs(got-want) > 0.01 {
			t.Errorf("half-width prior off by %+.2f m gave a mean lateral error of %.4f m, want %.4f m one-for-one",
				priorError, got, want)
		}
	}
}

func TestNearEdgeDegradesWhereItsFaceGoesObliqueAndSaysSo(t *testing.T) {
	// Section 3.3 attributes the measurement's error to the frames the
	// occluder touches. Measured here, the cause is geometric rather than the
	// occluder: the along-track error appears exactly in the closest-approach
	// window, where the end face is seen at a grazing angle, and it is zero
	// outside it whether or not the occluder is active. The occluder in the
	// default pass happens to sit inside that window, which is why the two
	// explanations are easy to confuse.
	//
	// The valuable part is that the measurement announces the degradation: the
	// end face's own support count halves in the same window, so an
	// uncertainty model can key on the support rather than having to infer
	// that something went wrong from the residual alone.
	pass := l4perception.DefaultSyntheticPass()
	frames := l4perception.GenerateSyntheticPass(pass)
	clusterer := l4perception.NewDBSCANClusterer(0.6, 5)

	// Abeam: the end face's normal is nearly perpendicular to the line of
	// sight, so it is barely observable.
	const abeamMetres = 3.0

	var abeamErr, obliqueSupport []float64
	var squareErr, squareSupport []float64

	for _, f := range frames {
		cluster, points, ok := largestSyntheticCluster(clusterer.Cluster(f.Points, pass.SensorID, f.Timestamp), f.Points)
		if !ok {
			continue
		}
		set := MeasureNearEdge(NearEdgeInput{
			Cluster: cluster, Points: points,
			SensorX: 0, SensorY: 0, HeadingRad: 0,
			HalfLength: float32(pass.Vehicle.Length / 2), HalfWidth: float32(pass.Vehicle.Width / 2),
			LengthProvenance: ProvenanceClassPrior, WidthProvenance: ProvenanceClassPrior,
			PredictedX: float32(f.TrueCentreX), PredictedY: float32(f.TrueCentreY),
		})
		if set.Rank == 0 {
			continue
		}

		along := math.Abs(float64(set.CentreX) - f.TrueCentreX)
		var support float64
		for _, e := range set.Edges {
			if e.Face.IsLongitudinal() {
				support = float64(e.SupportPoints)
			}
		}
		// The lateral face never goes oblique on this pass, so its
		// reconstruction must stay exact throughout — including in the window
		// where the along-track one fails. A measurement model that let one
		// bad axis contaminate the other would be no better than the OBB.
		if lateral := math.Abs(float64(set.CentreY) - f.TrueCentreY); lateral > 0.01 {
			t.Errorf("frame %d: lateral error %.4f m while the end face was the compromised one",
				f.Index, lateral)
		}

		if math.Abs(f.TrueCentreX) <= abeamMetres {
			abeamErr = append(abeamErr, along)
			obliqueSupport = append(obliqueSupport, support)
		} else {
			squareErr = append(squareErr, along)
			squareSupport = append(squareSupport, support)
		}
	}

	if len(abeamErr) == 0 || len(squareErr) == 0 {
		t.Fatalf("need frames on both sides of the abeam window, got %d and %d", len(abeamErr), len(squareErr))
	}

	t.Logf("along-track error: abeam (|x| <= %.0f m) mean %.4f m, elsewhere mean %.4f m",
		abeamMetres, meanOf(abeamErr), meanOf(squareErr))
	t.Logf("end-face support: abeam mean %.0f points, elsewhere mean %.0f points",
		meanOf(obliqueSupport), meanOf(squareSupport))

	// Away from closest approach the end face is well seen and the
	// reconstruction is exact.
	if got := maxOf(squareErr); got > 0.01 {
		t.Errorf("along-track error away from closest approach reaches %.4f m, want exact", got)
	}
	// Within it the error is real, and must not be silently small: this is the
	// failure the residual record and the uncertainty model exist for.
	if got := meanOf(abeamErr); got < 0.05 {
		t.Errorf("abeam along-track error is only %.4f m, so this test is no longer exercising the failure", got)
	}
	// And the support count drops with it, which is what makes the degradation
	// detectable from the measurement itself.
	if meanOf(obliqueSupport) >= meanOf(squareSupport) {
		t.Errorf("end-face support did not fall abeam: %.0f against %.0f elsewhere",
			meanOf(obliqueSupport), meanOf(squareSupport))
	}
	// Even at its worst it stays well inside the OBB centre's worst lateral
	// error on the same pass, which is the comparison that matters.
	if got := maxOf(abeamErr); got > 0.4 {
		t.Errorf("worst abeam along-track error %.4f m is no longer clearly better than the OBB centre", got)
	}
}

func TestNearEdgeReportsRankTwoWhenTwoFacesAreVisible(t *testing.T) {
	eval := evaluateSyntheticPass(t, 0)
	for i, rank := range eval.ranks {
		if rank != 2 {
			t.Errorf("frame %d reported rank %d; both an end face and a lateral face are visible on this pass", i, rank)
		}
	}
}

// --- Rank, the property that makes this robust to occlusion ----------------

// singleFacePoints samples one plane: a lateral face at y, spanning x.
func singleFacePoints(y float64, x0, x1 float64, n int) []l4perception.WorldPoint {
	var points []l4perception.WorldPoint
	for i := 0; i < n; i++ {
		frac := float64(i) / float64(n-1)
		points = append(points, l4perception.WorldPoint{
			X: x0 + frac*(x1-x0), Y: y, Z: 0.75,
		})
	}
	return points
}

func TestOneVisibleFaceProducesARankOneMeasurement(t *testing.T) {
	// The property Section 9.1 identifies as the one the current pipeline
	// lacks: "A frame in which only the near lateral face is visible constrains
	// lateral position and nothing else, and the measurement model should say so
	// by producing a one-dimensional measurement rather than a two-dimensional
	// one with a fudged covariance."
	const trueY, halfWidth = 5.0, 0.9
	nearFaceY := trueY - halfWidth

	set := MeasureNearEdge(NearEdgeInput{
		Points:  singleFacePoints(nearFaceY, -2.25, 2.25, 60),
		SensorX: 0, SensorY: 0,
		HeadingRad: 0,
		// Only a width belief, so only the lateral axis can contribute.
		HalfWidth:       halfWidth,
		WidthProvenance: ProvenanceAccumulated,
		PredictedX:      3.0, // deliberately wrong along track
		PredictedY:      4.0, // and wrong across it
	})

	if set.Rank != 1 {
		t.Fatalf("rank = %d with one visible face, want 1 (%s)", set.Rank, set.FallbackReason)
	}
	if len(set.Edges) != 1 {
		t.Fatalf("got %d edges, want 1", len(set.Edges))
	}
	if set.Edges[0].Face.IsLongitudinal() {
		t.Errorf("attributed the lateral face to %v", set.Edges[0].Face)
	}

	// Across track it is corrected to the truth.
	if math.Abs(float64(set.CentreY)-trueY) > 1e-4 {
		t.Errorf("lateral centre = %v, want %v", set.CentreY, trueY)
	}
	// Along track it must keep the prediction untouched: the face says nothing
	// about it, and inventing a position there is the defect.
	if math.Abs(float64(set.CentreX)-3.0) > 1e-4 {
		t.Errorf("along-track centre = %v, want the prediction 3.0 left alone", set.CentreX)
	}
	// And the constrained direction is the face normal.
	if math.Abs(math.Abs(float64(set.ConstrainedY))-1) > 1e-4 || math.Abs(float64(set.ConstrainedX)) > 1e-4 {
		t.Errorf("constrained direction = (%v, %v), want the lateral axis", set.ConstrainedX, set.ConstrainedY)
	}
}

func TestRankOneCovarianceIsUnboundedAcrossTheUnmeasuredDirection(t *testing.T) {
	// The covariance is how the filter is told to keep its prediction where
	// nothing was measured. An isotropic covariance here would quietly invent
	// information, which is what the rank exists to prevent.
	set := EdgeMeasurementSet{Rank: 1, ConstrainedX: 0, ConstrainedY: 1}
	cov := set.Covariance(0.01, 1000)

	// Along Y, the constrained direction: tight.
	if math.Abs(float64(cov.YY)-0.01) > 1e-6 {
		t.Errorf("variance along the constrained direction = %v, want 0.01", cov.YY)
	}
	// Along X, unmeasured: effectively unbounded.
	if cov.XX < 999 {
		t.Errorf("variance across the unmeasured direction = %v, want ~1000", cov.XX)
	}
	if math.Abs(float64(cov.XY)) > 1e-6 {
		t.Errorf("cross term = %v, want 0 for an axis-aligned constraint", cov.XY)
	}

	// A rank-two measurement is tight in both.
	full := EdgeMeasurementSet{Rank: 2}.Covariance(0.01, 1000)
	if full.XX > 0.011 || full.YY > 0.011 {
		t.Errorf("rank-two covariance is not tight in both directions: %+v", full)
	}

	// No measurement at all is unbounded in both, so the filter coasts.
	none := EdgeMeasurementSet{}.Covariance(0.01, 1000)
	if none.XX < 999 || none.YY < 999 {
		t.Errorf("rank-zero covariance is not unbounded: %+v", none)
	}
}

func TestRankOneCovarianceRotatesWithTheConstraint(t *testing.T) {
	// A diagonal constraint must produce a cross term, or the filter would be
	// told the measurement is axis-aligned when it is not.
	s := float32(math.Sqrt2 / 2)
	cov := EdgeMeasurementSet{Rank: 1, ConstrainedX: s, ConstrainedY: s}.Covariance(0.01, 1000)
	if cov.XY >= -1e-6 {
		t.Errorf("cross term = %v, want a negative one for a +45 degree constraint", cov.XY)
	}
	// Trace is preserved under rotation.
	if trace := cov.XX + cov.YY; math.Abs(float64(trace)-1000.01) > 0.1 {
		t.Errorf("trace = %v, want 1000.01 preserved under rotation", trace)
	}
}

// --- Refusals --------------------------------------------------------------

func TestMeasureNearEdgeRefusesWithoutTheGeometryItNeeds(t *testing.T) {
	good := NearEdgeInput{
		Points:  singleFacePoints(4.1, -2, 2, 40),
		SensorX: 0, SensorY: 0,
		HeadingRad:      0,
		HalfWidth:       0.9,
		WidthProvenance: ProvenanceAccumulated,
	}
	if got := MeasureNearEdge(good); got.Rank == 0 {
		t.Fatalf("the control case did not measure: %s", got.FallbackReason)
	}

	for _, tc := range []struct {
		name       string
		mutate     func(*NearEdgeInput)
		wantReason string
	}{
		{"no sensor origin", func(in *NearEdgeInput) {
			in.SensorX = float32(math.NaN())
		}, "missing_calibrated_sensor_origin"},
		{"no heading", func(in *NearEdgeInput) {
			in.HeadingRad = float32(math.NaN())
		}, "missing_heading"},
		{"no points", func(in *NearEdgeInput) { in.Points = nil }, "no_cluster_points"},
		{"no believed dimension", func(in *NearEdgeInput) {
			in.HalfWidth, in.WidthProvenance = 0, ProvenanceNone
		}, "no_face_reached_minimum_support"},
		{"too few points to define a face", func(in *NearEdgeInput) {
			in.Points = singleFacePoints(4.1, -2, 2, 3)
		}, "no_face_reached_minimum_support"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := good
			tc.mutate(&in)
			got := MeasureNearEdge(in)
			if got.Rank != 0 {
				t.Errorf("measured at rank %d when %s", got.Rank, tc.name)
			}
			if got.FallbackReason != tc.wantReason {
				t.Errorf("fallback reason = %q, want %q", got.FallbackReason, tc.wantReason)
			}
		})
	}
}

func TestEdgeMeasurementRecordsWhetherItsExtentWasInferred(t *testing.T) {
	// The far face is never observed, so some prior is unavoidable. What
	// matters is that the consumer is told when the reconstruction leaned on an
	// assumed size rather than a measured one.
	in := NearEdgeInput{
		Points:  singleFacePoints(4.1, -2, 2, 40),
		SensorX: 0, SensorY: 0,
		HeadingRad: 0,
		HalfWidth:  0.9,
	}

	in.WidthProvenance = ProvenanceClassPrior
	if got := MeasureNearEdge(in); !got.UsesInferredExtent() {
		t.Error("a class-prior half-extent was not reported as inferred")
	}
	in.WidthProvenance = ProvenanceAccumulated
	if got := MeasureNearEdge(in); got.UsesInferredExtent() {
		t.Error("an accumulated half-extent was reported as inferred")
	}
}

func TestImpliedCentreOffsetPutsTheCentreInboard(t *testing.T) {
	// The centre is inboard of the face it was measured from. A sign error here
	// would place it a whole body-width outside, which is the kind of mistake
	// the synthetic bound above would also catch but which is worth pinning
	// directly.
	e := EdgeMeasurement{PlaneOffsetMetres: -4.1, HalfExtentMetres: 0.9}
	if got := e.ImpliedCentreOffset(); math.Abs(float64(got)-(-5.0)) > 1e-6 {
		t.Errorf("implied centre offset = %v, want -5.0", got)
	}
}

func TestFacePlaneResistsContaminationTheExtremeWouldNotResist(t *testing.T) {
	// The synthetic corpus is noiseless, so it cannot justify using a
	// percentile rather than the extreme: on a perfectly planar face the two
	// agree exactly, and a mutation from one to the other passes every test
	// above. This supplies the missing justification directly.
	//
	// Contamination is the realistic case the choice is for: a few returns from
	// a neighbouring object, a mirror artefact, or background subtraction
	// leaving a fragment attached to the cluster. The extreme is defined by
	// whichever single return is furthest out, so one stray moves the measured
	// face by however far that stray lies; a percentile does not.
	const trueY, halfWidth = 5.0, 0.9
	nearFaceY := trueY - halfWidth

	clean := singleFacePoints(nearFaceY, -2.25, 2.25, 200)
	measure := func(points []l4perception.WorldPoint) EdgeMeasurementSet {
		return MeasureNearEdge(NearEdgeInput{
			Points:  points,
			SensorX: 0, SensorY: 0,
			HeadingRad:      0,
			HalfWidth:       halfWidth,
			WidthProvenance: ProvenanceAccumulated,
			PredictedX:      0,
			PredictedY:      float32(trueY),
		})
	}

	baseline := measure(clean)
	if baseline.Rank != 1 {
		t.Fatalf("clean case did not measure: %s", baseline.FallbackReason)
	}
	if math.Abs(float64(baseline.CentreY)-trueY) > 1e-4 {
		t.Fatalf("clean centre = %v, want %v", baseline.CentreY, trueY)
	}

	// Five strays half a metre nearer the sensor than the real face: 2.5% of
	// the point set, which is inside the 5th percentile's tolerance.
	contaminated := append([]l4perception.WorldPoint{}, clean...)
	for i := 0; i < 5; i++ {
		contaminated = append(contaminated, l4perception.WorldPoint{
			X: float64(i) * 0.1, Y: nearFaceY - 0.5, Z: 0.75,
		})
	}

	got := measure(contaminated)
	if got.Rank != 1 {
		t.Fatalf("contaminated case did not measure: %s", got.FallbackReason)
	}
	shift := math.Abs(float64(got.CentreY - baseline.CentreY))
	t.Logf("five strays 0.5 m off the face moved the reconstructed centre by %.4f m", shift)
	if shift > 0.05 {
		t.Errorf("contamination moved the centre by %.4f m; the percentile is not resisting it", shift)
	}

	// And the comparison that justifies the choice: the extreme of the same
	// point set is captured by the strays outright.
	extreme, ok := l4perception.LateralPercentile(contaminated, 0, -1, 100)
	if !ok {
		t.Fatal("extreme unavailable")
	}
	// Along the -Y normal the face sits at -nearFaceY and the strays at
	// -(nearFaceY - 0.5), which is 0.5 m further along that normal.
	if math.Abs(extreme-(-(nearFaceY - 0.5))) > 1e-6 {
		t.Errorf("the extreme = %v, want it captured by the strays at %v", extreme, -(nearFaceY - 0.5))
	}
}
