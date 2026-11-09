package l5tracks

import (
	"math"
	"testing"
)

// Tests for BoxModelConfig.BoxExtentMeasuredOnly. What carries the weight: a
// car seen from one side whose extent state has settled on a width the far
// side never showed (2.5 m against 1.8 m, as kirk0's car 8 read 2.45 m
// against 1.73 m) is drawn by #984's option at that width; with the
// sub-option the box keeps its own width belief and draws the shipped width,
// frame for frame, while a length the state has measured is still taken.
// A state measured on both dimensions draws #984's box exactly.

// measuredCarRun drives one box estimator over a 4.5 by 1.8 m car passing
// 5 m from the sensor at 8 m/s, reading body on every frame, and returns the
// boxes it drew and its counters.
func measuredCarRun(t *testing.T, cfg BoxModelConfig, body BoxBodyExtent) ([]BoxState, BoxStats) {
	t.Helper()
	e := NewBoxEstimator(cfg)
	var out []BoxState
	for i := 0; i < 30; i++ {
		x := -12 + 0.8*float64(i)
		pts := visibleFaces(x, 5, 0, 4.5, 1.8, 20)
		in := boxInput(int64(i)*1e8, pts, 8, 0)
		in.Body = body
		b := e.Observe(in)
		assertContains(t, i, b, pts)
		out = append(out, b)
	}
	return out, e.Stats
}

func TestBoxExtentMeasuredOnlyRefusesAOneSidedWidth(t *testing.T) {
	// Settled on both dimensions (sigma 0.3 m against the 0.75 m the 1.5 m
	// prior allows), with face evidence, along the box's axis.
	oneSided := BoxBodyExtent{Known: true, AxisRad: 0, Length: 4.6, Width: 2.5,
		LengthSigma: 0.3, WidthSigma: 0.3, LengthEvidence: true, WidthEvidence: true, PriorSigma: 1.5}
	shipped, _ := measuredCarRun(t, enabledBoxConfig(), oneSided)
	from := enabledBoxConfig()
	from.BoxExtentFromSolidBody = true
	all, allStats := measuredCarRun(t, from, oneSided)
	measured := from
	measured.BoxExtentMeasuredOnly = true

	// #984's option draws the state's width, wider than the car.
	for i, b := range all {
		if math.Abs(b.Width-2.5) > 1e-9 || b.Width <= shipped[i].Width {
			t.Fatalf("frame %d: from the solid body, width %.3f m against the shipped %.3f m; want the state's 2.5 m", i, b.Width, shipped[i].Width)
		}
	}
	if allStats.BodyExtentWidthFrames != len(all) || allStats.BodyExtentOneSided != 0 {
		t.Errorf("from the solid body: stats %+v, want every frame's width from the state and none one-sided", allStats)
	}

	// Measured only, with nothing measured from both sides: the shipped box.
	got, s := measuredCarRun(t, measured, oneSided)
	for i := range got {
		if got[i] != shipped[i] {
			t.Fatalf("frame %d: one-sided state drew %+v, want the shipped %+v", i, got[i], shipped[i])
		}
	}
	if s.BodyExtentOneSided != len(got) || s.BodyExtentFrames != 0 || s.BodyExtentUnsettled != 0 || s.BodyExtentWidthFrames != 0 {
		t.Errorf("one-sided state: stats %+v, want every frame counted one-sided and none taken", s)
	}

	// A span bound has moved hL: the length is taken, the width is still
	// the box's own, and so is the placement across the body.
	bound := oneSided
	bound.LengthBound = true
	got, s = measuredCarRun(t, measured, bound)
	across := func(b BoxState) float64 { return -b.X*math.Sin(b.HeadingRad) + b.Y*math.Cos(b.HeadingRad) }
	for i, b := range got {
		if b.Width != shipped[i].Width || b.WidthSigma != 0 || b.HeadingRad != shipped[i].HeadingRad || math.Abs(across(b)-across(shipped[i])) > 1e-9 {
			t.Fatalf("frame %d: with a length bound drew %+v; want the shipped width, heading and placement across (%+v)", i, b, shipped[i])
		}
		if b.Length < 4.6 || b.LengthSigma != 0.3 {
			t.Fatalf("frame %d: with a length bound, length %.3f ± %.3f m; want the state's 4.6 ± 0.3 m at least", i, b.Length, b.LengthSigma)
		}
	}
	if s.BodyExtentFrames != len(got) || s.BodyExtentOneSided != len(got) || s.BodyExtentWidthFrames != 0 {
		t.Errorf("length bound: stats %+v, want every frame taking the length and refusing the width", s)
	}

	// Both dimensions measured from both sides: #984's box, exactly.
	both := oneSided
	both.LengthPair, both.WidthPair = true, true
	got, s = measuredCarRun(t, measured, both)
	for i := range got {
		if got[i] != all[i] {
			t.Fatalf("frame %d: measured on both sides drew %+v, want #984's %+v", i, got[i], all[i])
		}
	}
	if s != allStats {
		t.Errorf("measured on both sides: stats %+v, want #984's %+v", s, allStats)
	}

	// Not settled: counted unsettled, not one-sided, as without the option.
	loose := oneSided
	loose.LengthSigma, loose.WidthSigma = 1.2, 1.2
	if _, s = measuredCarRun(t, measured, loose); s.BodyExtentUnsettled != 30 || s.BodyExtentOneSided != 0 {
		t.Errorf("unsettled state: stats %+v, want every frame unsettled", s)
	}
}

// Without BoxExtentFromSolidBody the sub-option does nothing.
func TestBoxExtentMeasuredOnlyAloneIsTheShippedBox(t *testing.T) {
	body := BoxBodyExtent{Known: true, Length: 4.6, Width: 2.5, LengthSigma: 0.3, WidthSigma: 0.3,
		LengthEvidence: true, WidthEvidence: true, LengthBound: true, PriorSigma: 1.5}
	shipped, _ := measuredCarRun(t, enabledBoxConfig(), body)
	cfg := enabledBoxConfig()
	cfg.BoxExtentMeasuredOnly = true
	got, s := measuredCarRun(t, cfg, body)
	for i := range got {
		if got[i] != shipped[i] {
			t.Fatalf("frame %d: %+v, want the shipped %+v", i, got[i], shipped[i])
		}
	}
	if s != (BoxStats{}) {
		t.Errorf("stats %+v, want none", s)
	}
}

// A dimension is measured from both sides when both of its faces have
// updated it and their last site-frame normals are opposed. A heading that
// turned by a half turn renames the face the sensor keeps seeing, and its
// normal, still toward the sensor, does not oppose the first.
func TestExtentMeasuredPairNeedsOpposedFaces(t *testing.T) {
	face := func(f BodyFace, nx, ny float32) EdgeMeasurement {
		return EdgeMeasurement{Face: f, NormalX: nx, NormalY: ny}
	}
	var e solidBodyExtent
	e.recordFace(face(FaceFront, -1, 0))
	if e.measuredPair(0) || e.measuredPair(1) {
		t.Fatal("one end face is a pair")
	}
	e.recordFace(face(FaceRight, 0, -1))
	e.recordFace(face(FaceRear, 1, 0))
	if !e.measuredPair(0) {
		t.Error("front and rear seen from opposite sides are not a pair")
	}
	if e.measuredPair(1) {
		t.Error("one side face is a pair")
	}
	if e.faces != [4]int{1, 1, 0, 1} {
		t.Errorf("face counts %v", e.faces)
	}

	// The near end seen throughout, renamed by a half turn of the heading.
	var flipped solidBodyExtent
	flipped.recordFace(face(FaceFront, -1, 0))
	flipped.recordFace(face(FaceRear, -0.98, 0.2))
	if flipped.measuredPair(0) {
		t.Error("one end renamed by a heading flip is a pair")
	}
	// A side face named while the axis ran across the body (an end face),
	// then the near side: perpendicular normals are not a pair.
	var across solidBodyExtent
	across.recordFace(face(FaceLeft, -1, 0))
	across.recordFace(face(FaceRight, 0, -1))
	if across.measuredPair(1) {
		t.Error("perpendicular side faces are a pair")
	}
	across.recordFace(face(FaceLeft, 0, 1))
	if !across.measuredPair(1) {
		t.Error("the last normals are opposed and the pair is refused")
	}
}

// Through the tracker on the through-truck pass (both ends seen, one side):
// the stepNearEdge bookkeeping records both end faces and the bounds, the
// box reads the length as measured from both ends on some frames, and the
// published box takes the length from the state and never the width. (By
// the pass's end the face-state body's axis has run off the box's, the
// extent-state defect l5id0019 ranks third, so the read is counted per
// frame, not taken from the last.)
func TestTrackerBoxExtentMeasuredOnlyReadsTheFaces(t *testing.T) {
	cfg := faceStateConfig()
	cfg.BoxModel.BoxExtentFromSolidBody = true
	cfg.BoxModel.BoxExtentMeasuredOnly = true
	tracker := NewTracker(cfg)
	lengthPairs, widthPairs, lengthBounds := 0, 0, 0
	for _, f := range syntheticPassFrames(t, throughTruckPass()) {
		tracker.Update(f.clusters, f.at)
		for _, tr := range tracker.GetActiveTracks() {
			b := boxBodyExtent(tr, false)
			if !b.Known {
				continue
			}
			if b.LengthPair {
				lengthPairs++
			}
			if b.WidthPair {
				widthPairs++
			}
			if b.LengthBound != (tr.solidBody.ext.bounds[0] > 0) {
				t.Fatalf("LengthBound %v with %d length bounds", b.LengthBound, tr.solidBody.ext.bounds[0])
			}
			if b.LengthBound {
				lengthBounds++
			}
		}
	}
	tr := mainTrack(t, tracker)
	ext := tr.solidBody.ext
	if ext.faces[FaceFront] == 0 || ext.faces[FaceRear] == 0 {
		t.Fatalf("end face counts %v: the pass shows both ends", ext.faces)
	}
	if lengthPairs == 0 {
		t.Error("no frame read the length as measured from both ends")
	}
	s, ok := tr.BoxModelStats()
	if !ok {
		t.Fatal("the truck has no box")
	}
	if s.BodyExtentFrames == 0 || s.BodyExtentWidthFrames != 0 {
		t.Errorf("stats %+v: want frames that took the length and none that took the width", s)
	}
	t.Logf("through-truck pass, measured only: faces %v, bounds %v; body-centre reads with the length paired %d, "+
		"the width paired %d, a length bound %d; %+v; box %.2f × %.2f m",
		ext.faces, ext.bounds, lengthPairs, widthPairs, lengthBounds, s, tr.BoxLength, tr.BoxWidth)
}
