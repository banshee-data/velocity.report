package perframeeval

import (
	"math"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// centreExtentInstant is one scored pose built by hand: a reference body at
// (cx, cy) heading yaw with length refLength, and a prediction at (px, py)
// with the same heading and length predLength, compared through the
// scorer's own centre and length comparisons.
func centreExtentInstant(t *testing.T, obj string, sample int, cx, cy, yaw, refLength, px, py, predLength float64) PhysicalInstant {
	t.Helper()
	g := annotation.PhysicalGeometry{
		ObjectID: obj, SampleID: sample, Truth: true,
		Centre: &annotation.PlanarBound{XM: cx, YM: cy, BoundM: 0.3},
		Yaw:    &annotation.AngleBound{Rad: yaw, BoundRad: 0.05, Axis: annotation.AxisResolved},
		Length: &annotation.LinearBound{LowerM: refLength - 0.2, UpperM: refLength + 0.2, ValueM: refLength, HalfWidthM: 0.2},
	}
	p := PredictedBody{TrackKey: "seq-000001", Reference: "body_centre", XM: px, YM: py, Physical: true, CentreXM: px, CentreYM: py,
		Heading: &PredictedHeading{Rad: yaw, Resolved: true}, Length: &PredictedExtent{Metres: predLength}}
	in := PhysicalInstant{EpisodeID: "ep", ObjectID: obj, SampleID: sample, Reference: &g, Prediction: &p,
		Outcomes: map[PhysicalComponent]Outcome{}}
	for _, c := range []PhysicalComponent{ComponentCentre, ComponentLength} {
		if reason := compare(&in, g, c); reason != "" {
			t.Fatalf("%s not compared: %s", c, reason)
		}
		in.Outcomes[c] = Outcome{Category: OutcomeScored}
	}
	return in
}

// The four cases the decomposition must get right. The sensor is at the
// origin and the reference centre at x = -10 heading +x, so the front faces
// the sensor and is the seen end.
func TestCentreExtentDecomposesEachPose(t *testing.T) {
	for name, c := range map[string]struct {
		px, py, predLength                  float64
		along, across, explained, remainder float64
		share                               float64 // NaN: no along error, no share
	}{
		"exactly right": {px: -10, py: 0, predLength: 10, share: math.NaN()},
		// Front exact at x = -5, length 8: centre at -9, 1 m toward the
		// seen front, all of it the length's.
		"short by 2 m, seen front exact": {px: -9, py: 0, predLength: 8, along: 1, explained: 1, share: 1},
		"pure centre shift":              {px: -9.5, py: 0, predLength: 10, along: 0.5, remainder: 0.5, share: 0},
		"across only":                    {px: -10, py: 0.3, predLength: 10, across: 0.3, share: math.NaN()},
	} {
		in := centreExtentInstant(t, "obj_a", 1, -10, 0, 0, 10, c.px, c.py, c.predLength)
		d := decomposeCentreExtent([]PhysicalInstant{in}, map[string]string{"obj_a": "truck"},
			map[string]map[int]referenceEnds{"obj_a": {1: {front: annotation.EvidenceObserved, rear: annotation.EvidenceInferred}}}, true)
		if len(d.Poses) != 1 {
			t.Fatalf("%s: %d poses", name, len(d.Poses))
		}
		p := d.Poses[0]
		if p.SeenEnd != "front" || p.SeenEndSign != 1 || p.SeenEndReferenceEvidence != "observed" || p.Label != "truck" {
			t.Errorf("%s: seen end %+v", name, p)
		}
		if !near(p.AlongM, c.along) || !near(p.AcrossM, c.across) || !near(p.LengthExplainedM, c.explained) || !near(p.RemainderM, c.remainder) {
			t.Errorf("%s: along %v across %v explained %v remainder %v", name, p.AlongM, p.AcrossM, p.LengthExplainedM, p.RemainderM)
		}
		if !near(p.AlongM, p.LengthExplainedM+p.RemainderM) {
			t.Errorf("%s: the parts do not sum to the along error", name)
		}
		s := d.Pooled
		switch {
		case math.IsNaN(c.share) && s.ExplainedShare != nil:
			t.Errorf("%s: share %v with no along error", name, *s.ExplainedShare)
		case !math.IsNaN(c.share) && (s.ExplainedShare == nil || !near(*s.ExplainedShare, c.share)):
			t.Errorf("%s: share %v, want %v", name, s.ExplainedShare, c.share)
		}
		if s.Poses != 1 || s.CentreScored != 1 || s.SeenFront != 1 || s.SeenEndObserved != 1 || !near(s.AcrossMeanAbsM, math.Abs(c.across)) {
			t.Errorf("%s: pooled %+v", name, s)
		}
	}
}

// The rule reads the reference, not the arm: a body past the sensor shows
// its rear, and a short box on its rear has its centre displaced rearward,
// negative along a heading the frame turns a quarter.
func TestCentreExtentSeenRearInATurnedFrame(t *testing.T) {
	// Heading +y, centre at y = 10: the rear (y = 5) faces the sensor.
	in := centreExtentInstant(t, "obj_a", 1, 0, 10, math.Pi/2, 10, 0, 9, 8)
	d := decomposeCentreExtent([]PhysicalInstant{in}, nil, nil, true)
	p := d.Poses[0]
	if p.SeenEnd != "rear" || p.SeenEndSign != -1 || p.Label != "unlabelled" || p.SeenEndReferenceEvidence != "unknown" {
		t.Fatalf("seen end %+v", p)
	}
	if !near(p.AlongM, -1) || !near(p.LengthExplainedM, -1) || !near(p.RemainderM, 0) || !near(p.AcrossM, 0) {
		t.Fatalf("decomposition %+v", p)
	}
	// A long box on the seen rear pushes the centre forward, away from it.
	in = centreExtentInstant(t, "obj_a", 1, 0, 10, math.Pi/2, 10, 0, 11, 12)
	if p := decomposeCentreExtent([]PhysicalInstant{in}, nil, nil, true).Poses[0]; !near(p.LengthExplainedM, 1) || !near(p.RemainderM, 0) {
		t.Fatalf("long box %+v", p)
	}
}

// Vehicles are kept apart, pooled by label and in all; a pose whose length
// was not scored is counted and not decomposed; the share is over sums.
func TestCentreExtentGroupsAndExcludes(t *testing.T) {
	ends := map[string]map[int]referenceEnds{}
	instants := []PhysicalInstant{
		// Truck 1: short by 2 m on the seen front, then shifted 1 m too.
		centreExtentInstant(t, "obj_t1", 1, -10, 0, 0, 10, -9, 0, 8),
		centreExtentInstant(t, "obj_t1", 2, -10, 0, 0, 10, -8, 0, 8),
		// Truck 2: a pure shift of 0.5 m back.
		centreExtentInstant(t, "obj_t2", 1, -20, 0, 0, 10, -20.5, 0, 10),
		// Car: length unscored.
		centreExtentInstant(t, "obj_c", 1, -15, 0, 0, 4, -15, 0, 4),
	}
	instants[3].Outcomes[ComponentLength] = Outcome{Category: OutcomeMissingPrediction, Reason: ReasonPredictionNoExtent}
	// Not centre-scored: not counted at all.
	unscored := centreExtentInstant(t, "obj_c", 2, -15, 0, 0, 4, -15, 0, 4)
	unscored.Outcomes[ComponentCentre] = Outcome{Category: OutcomeUnmatched, Reason: ReasonNoPredictionWithinGate}
	instants = append(instants, unscored)

	labels := map[string]string{"obj_t1": "truck", "obj_t2": "truck", "obj_c": "car"}
	d := decomposeCentreExtent(instants, labels, ends, false)
	if len(d.Poses) != 3 || len(d.Vehicles) != 3 || len(d.ByLabel) != 2 || len(d.Caveats) != 1 {
		t.Fatalf("decomposition %+v", d)
	}
	byObj := map[string]CentreExtentSummary{}
	for _, v := range d.Vehicles {
		byObj[v.ObjectID] = v
	}
	car := byObj["obj_c"]
	if car.Label != "car" || car.CentreScored != 1 || car.Poses != 0 || car.Excluded["length:missing_prediction/prediction_extent_absent"] != 1 || car.ExplainedShare != nil {
		t.Errorf("car %+v", car)
	}
	t1 := byObj["obj_t1"]
	// Along 1 and 2, explained 1 and 1, remainder 0 and 1: share 2/3.
	if t1.Poses != 2 || !near(t1.AlongMeanM, 1.5) || !near(t1.AlongMedianM, 1.5) || !near(t1.ExplainedMeanM, 1) ||
		!near(t1.RemainderMeanM, 0.5) || !near(t1.LengthErrorMeanM, -2) || !near(*t1.ExplainedShare, 2.0/3) {
		t.Errorf("truck 1 %+v", t1)
	}
	t2 := byObj["obj_t2"]
	if t2.Poses != 1 || !near(t2.AlongMeanM, -0.5) || !near(t2.RemainderMeanM, -0.5) || !near(*t2.ExplainedShare, 0) {
		t.Errorf("truck 2 %+v", t2)
	}
	// Trucks pooled: along 1, 2, -0.5; |explained| 1, 1, 0; share 2/3.5.
	var trucks CentreExtentSummary
	for _, l := range d.ByLabel {
		if l.Label == "truck" {
			trucks = l
		}
	}
	if trucks.Poses != 3 || trucks.CentreScored != 3 || !near(trucks.AlongMedianM, 1) || !near(*trucks.ExplainedShare, 2/3.5) {
		t.Errorf("trucks %+v", trucks)
	}
	if p := d.Pooled; p.Label != "all" || p.Poses != 3 || p.CentreScored != 4 || p.Excluded["length:missing_prediction/prediction_extent_absent"] != 1 {
		t.Errorf("pooled %+v", p)
	}
}

// The section is written with every physical score and rendered beside it.
func TestCentreExtentInTheRunAndMarkdown(t *testing.T) {
	f := physFixture(t)
	c, err := Run(physConfig(f))
	if err != nil {
		t.Fatal(err)
	}
	for _, arm := range []PhysicalResult{c.Physical.A, c.Physical.B} {
		d := arm.CentreExtent
		if d == nil || d.SeenEndRule != CentreExtentSeenEndRule || d.Pooled.CentreScored != arm.Summary.Components[ComponentCentre].Scored {
			t.Fatalf("arm %s: section %+v", arm.Arm.Label, d)
		}
		for _, p := range d.Poses {
			if !near(p.AlongM, p.LengthExplainedM+p.RemainderM) {
				t.Errorf("arm %s: pose %+v does not sum", arm.Arm.Label, p)
			}
		}
	}
	md := RenderMarkdown(*c)
	for _, want := range []string{"### Centre against extent", "| Arm | Vehicle | Label | Poses (centre scored) |", "| exact | all |",
		"Seen end: reference_end_nearer_sensor", "Share: explained_share"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
}
