package l5tracks

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Under solid_body_rectangle_fit every associated frame's row carries the
// frame's rectangle fit, and on the end-on truck the fit's axis is within a
// few degrees of the truck's course for nearly every frame it does not
// abstain on, whatever the tracked heading does. Nothing else changes: the
// estimates are those of the same arm without the fit.
func TestRectangleFitIsRecordedOnTheRowAndReadsTheAxis(t *testing.T) {
	run := func(fit bool) (axisErrs []float64, abstained, known int, xs []float32) {
		cfg := solidBodyConfig()
		cfg.SolidBody.CourseAlignedFaces = true
		cfg.SolidBody.RectangleFit = fit
		tracker := NewTracker(cfg)
		var prevX, prevY float64
		var havePrev bool
		for _, f := range syntheticPassFrames(t, endOnTruckPass()) {
			tracker.Update(f.clusters, f.at)
			sb, ok := mainTrack(t, tracker).SolidBody()
			if !ok {
				continue
			}
			xs = append(xs, sb.Estimate.X, sb.Estimate.Y, sb.Estimate.Orientation.PsiRad, sb.Estimate.Length.Metres)
			m := sb.Measurement
			if m.RectangleKnown {
				known++
				if m.RectangleAbstain != "" {
					abstained++
				} else if havePrev {
					course := math.Atan2(f.truthY-prevY, f.truthX-prevX)
					axisErrs = append(axisErrs, l4perception.FoldAxisRad(float64(m.RectangleAxisRad), course)*180/math.Pi)
				}
			}
			prevX, prevY, havePrev = f.truthX, f.truthY, true
		}
		return
	}
	errs, abstained, known, with := run(true)
	_, _, knownWithout, without := run(false)
	if knownWithout != 0 {
		t.Fatalf("%d rows carry a rectangle fit without the experiment", knownWithout)
	}
	if known == 0 || len(errs) == 0 {
		t.Fatalf("no row carried a rectangle fit (%d known, %d errors)", known, len(errs))
	}
	if len(with) != len(without) {
		t.Fatalf("the fit changed the number of readings: %d against %d", len(with), len(without))
	}
	for i := range with {
		if with[i] != without[i] {
			t.Fatalf("the fit changed an estimate at reading %d: %v against %v", i/4, with[i], without[i])
		}
	}
	over5 := 0
	for _, e := range errs {
		if e > 5 {
			over5++
		}
	}
	t.Logf("%d fits, %d abstained, %d scored against the course: over 5 degrees %d", known, abstained, len(errs), over5)
	if float64(over5) > 0.1*float64(len(errs)) {
		t.Fatalf("the fit's axis is over 5 degrees off the course on %d of %d frames", over5, len(errs))
	}
}
