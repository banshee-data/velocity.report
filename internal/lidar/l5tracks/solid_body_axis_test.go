package l5tracks

import (
	"math"
	"sort"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func axisFit(deg, sigmaDeg float64) l4perception.RectangleFit {
	return l4perception.RectangleFit{AxisRad: foldQuarterTurn(deg * math.Pi / 180), SigmaRad: sigmaDeg * math.Pi / 180, Points: 100}
}

func axisDeg(a solidBodyAxis) float64 { return a.rad * 180 / math.Pi }

// The axis is filtered on the quarter-turn circle: a fit at 89 degrees and a
// state at 1 are two degrees apart, and the update lands between them across
// the wrap rather than at 45.
func TestAxisFilterUpdatesAcrossTheQuarterTurnWrap(t *testing.T) {
	var a solidBodyAxis
	a.observe(axisFit(1, 3), 0)
	if !a.known || math.Abs(axisDeg(a)-1) > 1e-9 {
		t.Fatalf("first fit gave %+v", a)
	}
	a.observe(axisFit(89, 3), 100e6)
	if e := l4perception.FoldAxisRad(a.rad, 0) * 180 / math.Pi; e > 1.01 {
		t.Fatalf("1 and 89 degrees filtered to %.2f, %.2f off the shared axis", axisDeg(a), e)
	}
	if a.lastSource != "updated" || a.updates != 2 {
		t.Fatalf("the second fit was %q after %d updates", a.lastSource, a.updates)
	}
}

// The variance follows the filter: an abstaining frame predicts only, so the
// variance grows with time and the axis stays; a run of fits narrows it below
// a single fit's.
func TestAxisFilterPredictsThroughAbstentionAndNarrowsWithFits(t *testing.T) {
	var a solidBodyAxis
	nanos := int64(0)
	for i := 0; i < 10; i++ {
		a.observe(axisFit(20, 3), nanos)
		nanos += 100e6
	}
	single := math.Pow(3*math.Pi/180, 2)
	if a.varRad2 >= single {
		t.Fatalf("ten fits left a variance %.5f, no narrower than one fit's %.5f", a.varRad2, single)
	}
	before, at := a.varRad2, a.rad
	// The last fit was at nanos - 100 ms; this frame is half a second after it.
	a.observe(l4perception.RectangleFit{Abstain: "weak_edges"}, nanos-100e6+500e6)
	if a.rad != at || a.lastSource != "abstained" {
		t.Fatalf("an abstaining fit moved the axis or was read: %+v", a)
	}
	if want := before + axisProcessNoiseRad2PerSecond*0.5; math.Abs(a.varRad2-want) > 1e-12 {
		t.Fatalf("variance %.6f after half a second, want %.6f", a.varRad2, want)
	}
}

// A fit far outside the prediction is refused, and three in a row restart
// the axis from the fit, so one wrong first fit cannot hold a track.
func TestAxisFilterRefusesAnOutlierAndRestartsOnARun(t *testing.T) {
	var a solidBodyAxis
	nanos := int64(0)
	for i := 0; i < 10; i++ {
		a.observe(axisFit(10, 3), nanos)
		nanos += 100e6
	}
	a.observe(axisFit(40, 3), nanos)
	if a.lastSource != "refused" || math.Abs(axisDeg(a)-10) > 0.5 {
		t.Fatalf("a 30-degree outlier was %q, axis %.2f", a.lastSource, axisDeg(a))
	}
	a.observe(axisFit(10.5, 3), nanos+100e6)
	if a.lastSource != "updated" || a.refused != 0 {
		t.Fatalf("an agreeing fit after one refusal was %q with %d refused", a.lastSource, a.refused)
	}
	nanos += 200e6
	for i := 0; i < axisReinitialiseAfter; i++ {
		a.observe(axisFit(40, 3), nanos)
		nanos += 100e6
	}
	if a.lastSource != "restarted" || math.Abs(axisDeg(a)-40) > 1e-6 {
		t.Fatalf("after %d refusals the axis is %.2f (%q)", axisReinitialiseAfter, axisDeg(a), a.lastSource)
	}
}

// The label picks the heading among axis + k·π/2 nearest the reference.
func TestLabelAxisPicksTheHeadingNearestTheReference(t *testing.T) {
	for _, c := range []struct{ axis, ref, want float64 }{
		{10, 12, 10}, {10, 185, 190}, {10, 95, 100}, {10, -80, -80}, {80, 0, -10}, {0, 44, 0}, {0, 46, 90},
	} {
		got := labelAxis(c.axis*math.Pi/180, c.ref*math.Pi/180) * 180 / math.Pi
		if d := math.Abs(math.Remainder(got-c.want, 360)); d > 1e-9 {
			t.Errorf("axis %.0f, reference %.0f: got %.2f, want %.0f", c.axis, c.ref, got, c.want)
		}
	}
}

// Through the whole tracker, on kirk0's truck 2 coming straight at the sensor
// end on: the heading the solid body reports is the labelled rectangle axis,
// within a few degrees of the truck's course on nearly every frame once the
// axis is observed, where the tracked PCA heading of the end-face strip runs
// across the truck. Without the option nothing reads the axis.
func TestRectangleHeadingHeadsAnEndOnTruckAlongItsAxis(t *testing.T) {
	run := func(heading bool) (errs []float64, axisKnown int) {
		cfg := solidBodyConfig()
		cfg.SolidBody.CourseAlignedFaces = true
		cfg.SolidBody.RectangleHeading = heading
		tracker := NewTracker(cfg)
		for _, f := range syntheticPassFrames(t, endOnTruckPass()) {
			tracker.Update(f.clusters, f.at)
			track := mainTrack(t, tracker)
			sb, ok := track.SolidBody()
			if !ok || track.ObservationCount <= cfg.HitsToConfirm {
				continue
			}
			if track.solidBody.axis.known {
				axisKnown++
			}
			// The truck drives along +X.
			errs = append(errs, math.Abs(math.Remainder(float64(sb.Estimate.Orientation.PsiRad), 2*math.Pi))*180/math.Pi)
		}
		return
	}
	without, knownWithout := run(false)
	with, known := run(true)
	if knownWithout != 0 {
		t.Fatalf("the axis was observed on %d frames without the option", knownWithout)
	}
	if known == 0 || len(with) == 0 {
		t.Fatalf("the axis was never observed (%d frames)", len(with))
	}
	median := func(v []float64) float64 {
		s := append([]float64(nil), v...)
		sort.Float64s(s)
		return s[len(s)/2]
	}
	over5 := 0
	for _, e := range with {
		if e > 5 {
			over5++
		}
	}
	t.Logf("tracked heading: median %.1f degrees off; rectangle heading: median %.1f, %d of %d frames over 5 degrees, axis known on %d",
		median(without), median(with), over5, len(with), known)
	if median(with) > 3 {
		t.Fatalf("the rectangle heading is a median %.1f degrees off the truck's course", median(with))
	}
	if float64(over5) > 0.1*float64(len(with)) {
		t.Fatalf("the rectangle heading is over 5 degrees off on %d of %d frames", over5, len(with))
	}
}
