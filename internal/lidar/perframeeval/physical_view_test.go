package perframeeval

import (
	"math"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// measurementShiftM and measurementAcrossM are how far the fixture's
// measurements sit from the box version's estimates: toward the sensor along
// the course, and to its left.
const (
	measurementShiftM  = 0.4
	measurementAcrossM = 0.2
)

// measurementFixture is the box fixture with each box estimate's residual
// given a measurement measurementShiftM nearer the sensor than the estimate
// and measurementAcrossM to its left. The fixture's bodies drive along +x at
// y = 0 from x = 10 m, with the sensor at the origin, so nearer the sensor is
// −x and every pose is end on.
func measurementFixture(t *testing.T) *evalfixture.PhysicalFixture {
	t.Helper()
	f := boxFixture(t)
	database, err := db.NewDB(f.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`UPDATE lidar_track_residuals
		   SET measurement_x = (SELECT e.x FROM lidar_track_estimates e WHERE e.estimate_id = lidar_track_residuals.estimate_id) - ?,
		       measurement_y = ?
		 WHERE estimate_id IN (SELECT estimate_id FROM lidar_track_estimates WHERE param_hash = ?)`,
		measurementShiftM, measurementAcrossM, evalfixture.ParamsBox); err != nil {
		t.Fatal(err)
	}
	return f
}

func measurementArm(f *evalfixture.PhysicalFixture) ArmSpec {
	return ArmSpec{Label: "measured", DBPath: f.DBPath, ParamHash: evalfixture.ParamsBox, Stage: StageOnline,
		DeclaredBaseline: true, Measurements: true}
}

// TestPhysicalScoringOfMeasurements: a measurement arm is scored at the
// measurement each update consumed, not the filtered state beside it, keeps
// the estimate's reference point and names the residual table; its centre is
// a visible-box point, compared and grouped by reference but never scored as
// a body centre.
func TestPhysicalScoringOfMeasurements(t *testing.T) {
	f := measurementFixture(t)
	pr, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	arm, err := LoadPhysicalArm(measurementArm(f))
	if err != nil {
		t.Fatal(err)
	}
	if arm.Identity.Table != measurementTable || arm.Identity.ParamHash != evalfixture.ParamsBox || arm.Identity.Points != 19 {
		t.Fatalf("identity %+v", arm.Identity)
	}
	r, err := ScorePhysical(pr, arm)
	if err != nil {
		t.Fatal(err)
	}
	in := instantOf(t, r, evalfixture.Follower, 0)
	wantOutcome(t, in, ComponentCentre, OutcomeMissingPrediction, ReasonPredictionNotOnBody)
	wantX := evalfixture.FollowerX(0) - evalfixture.BoxEstimateOffsetM - measurementShiftM
	if in.Prediction.Reference != "visible_obb_centre" || !approx(in.Prediction.XM, wantX) || !approx(in.Prediction.YM, measurementAcrossM) {
		t.Fatalf("prediction %+v, want the measurement at (%g, %g)", in.Prediction, wantX, measurementAcrossM)
	}
	c := in.Comparison.Centre
	if !approx(c.DXM, -(evalfixture.BoxEstimateOffsetM+measurementShiftM)) || !approx(c.DYM, measurementAcrossM) {
		t.Fatalf("centre comparison %+v", c)
	}
}

// TestLoadArmReadsTheMeasurements: the per-frame arm of a measurement version
// tracks the measurements, keyed by creation sequence.
func TestLoadArmReadsTheMeasurements(t *testing.T) {
	f := measurementFixture(t)
	h, err := LoadArm(measurementArm(f))
	if err != nil {
		t.Fatal(err)
	}
	if h.Identity.Table != measurementTable || h.Identity.Points != 19 || len(h.Series) != 2 || h.Series[0].ID != "seq-000001" {
		t.Fatalf("identity %+v, %d series", h.Identity, len(h.Series))
	}
	for _, p := range h.Series[1].Points {
		i := int((p.TimestampNanos - evalfixture.EstimateOffsetNs - evalfixture.SampleTime(0)) / (evalfixture.SampleTime(1) - evalfixture.SampleTime(0)))
		want := evalfixture.LeaderX(i) - evalfixture.BoxEstimateOffsetM - measurementShiftM
		if !approx(float64(p.X), want) || !approx(float64(p.Y), measurementAcrossM) {
			t.Fatalf("leader at sample %d: (%g, %g), want the measurement (%g, %g)", i, p.X, p.Y, want, measurementAcrossM)
		}
	}
	cv := strings.Join(caveats(Comparison{A: ArmResult{Arm: h.Identity}, B: ArmResult{Arm: h.Identity}}), "\n")
	if !strings.Contains(cv, "filter's measurements, not its state") {
		t.Errorf("caveats lack the measurement arm: %s", cv)
	}
}

// TestMeasurementArmRefusals: a measurement arm names one table, and names
// an estimate version rather than a run.
func TestMeasurementArmRefusals(t *testing.T) {
	f := measurementFixture(t)
	withBoxes, withBodies, withRun := measurementArm(f), measurementArm(f), measurementArm(f)
	withBoxes.Boxes, withBodies.SolidBodies = true, true
	withRun.Stage, withRun.ParamHash, withRun.RunID = "", "", "run"
	for name, spec := range map[string]ArmSpec{"boxes": withBoxes, "solid bodies": withBodies} {
		if _, err := LoadPhysicalArm(spec); err == nil || !strings.Contains(err.Error(), "not more than one") {
			t.Errorf("physical, measurements with %s: %v", name, err)
		}
		if _, err := LoadArm(spec); err == nil || !strings.Contains(err.Error(), "not more than one") {
			t.Errorf("per-frame, measurements with %s: %v", name, err)
		}
	}
	if _, err := LoadArm(withRun); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("a measurement arm naming a run: %v", err)
	}
}

// TestCentreViewOfMeasurements: every pose is end on and its range is the
// reference centre's distance from the origin; the measurements sit nearer
// the sensor by the box offset and the shift, and the occluded sample is
// counted unmatched rather than dropped.
func TestCentreViewOfMeasurements(t *testing.T) {
	f := measurementFixture(t)
	pr, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	arm, err := LoadPhysicalArm(measurementArm(f))
	if err != nil {
		t.Fatal(err)
	}
	r, err := ScorePhysical(pr, arm)
	if err != nil {
		t.Fatal(err)
	}
	v := r.CentreView
	if v == nil || len(v.Poses) == 0 || v.ReferencePoses != len(v.Poses)+v.Unmatched {
		t.Fatalf("view %+v", v)
	}
	toward := evalfixture.BoxEstimateOffsetM + measurementShiftM
	for _, p := range v.Poses {
		in := instantOf(t, r, p.ObjectID, p.SampleID)
		wantRange := math.Hypot(in.Reference.Centre.XM, in.Reference.Centre.YM)
		if !approx(p.RangeM, wantRange) || p.AspectDeg == nil || !approx(*p.AspectDeg, 0) || p.AspectBin != "aspect_0_30deg" ||
			!approx(p.TowardSensorM, toward) || !approx(*p.AlongAbsM, toward) || !approx(*p.AcrossAbsM, measurementAcrossM) ||
			p.PredictionReference != "visible_obb_centre" || p.RangeBand != rangeBand(wantRange, CentreViewRangeBandEdgesM) {
			t.Fatalf("pose %+v", p)
		}
	}
	var all *CentreViewCell
	for i, c := range v.Cells {
		if c.Group == "all" {
			all = &v.Cells[i]
		}
	}
	if all == nil || all.Poses != len(v.Poses) || !approx(all.TowardSensorMedianM, toward) ||
		!approx(all.ErrorMedianM, math.Hypot(toward, measurementAcrossM)) {
		t.Fatalf("pooled cell %+v", all)
	}
}

// TestCentreViewGeometry: the aspect folds to [0, 90] whichever way the body
// faces, the offset toward the sensor has the sensor's sign on either side of
// it, a pose without a heading has no aspect, and a pose with no compared
// centre is unmatched; a reference frame with a site transform is refused.
func TestCentreViewGeometry(t *testing.T) {
	yaw := func(rad float64) *annotation.AngleBound { return &annotation.AngleBound{Rad: rad} }
	centre := func(x, y float64) *annotation.PlanarBound { return &annotation.PlanarBound{XM: x, YM: y, BoundM: 0.5} }
	pose := func(obj string, sample int, g annotation.PhysicalGeometry, dx, dy float64) PhysicalInstant {
		in := PhysicalInstant{EpisodeID: "ep", ObjectID: obj, SampleID: sample, Reference: &g}
		in.Prediction = &PredictedBody{TrackKey: "seq-000001", Reference: "cluster_mean"}
		in.Comparison.Centre = &CentreComparison{ErrorM: math.Hypot(dx, dy), DXM: dx, DYM: dy, ReferenceBoundM: 0.5,
			PredictionReference: "cluster_mean"}
		return in
	}
	instants := []PhysicalInstant{
		// Broadside at 5 m on +y, heading +x; the prediction 0.3 m toward the sensor.
		pose("car", 1, annotation.PhysicalGeometry{Centre: centre(0, 5), Yaw: yaw(0)}, 0, -0.3),
		// Broadside on the other side, heading −x; 0.3 m toward the sensor again.
		pose("car", 2, annotation.PhysicalGeometry{Centre: centre(0, -5), Yaw: yaw(math.Pi)}, 0, 0.3),
		// End on at 25 m heading away; the prediction 1 m beyond the centre.
		pose("truck", 3, annotation.PhysicalGeometry{Centre: centre(25, 0), Yaw: yaw(math.Pi)}, 1, 0),
		// 45° at 15 m, no heading stated.
		pose("truck", 4, annotation.PhysicalGeometry{Centre: centre(15/math.Sqrt2, 15/math.Sqrt2)}, 0, 0),
	}
	unmatched := PhysicalInstant{EpisodeID: "ep", ObjectID: "truck", SampleID: 5,
		Reference: &annotation.PhysicalGeometry{Centre: centre(30, 0), Yaw: yaw(0)}}
	instants = append(instants, unmatched)

	v := summariseCentreView(instants, map[string]string{"car": "car", "truck": "truck"}, true)
	if v.ReferencePoses != 5 || v.Unmatched != 1 || v.UnmatchedByObject["truck"] != 1 || len(v.Poses) != 4 {
		t.Fatalf("accounting %+v", v)
	}
	want := []struct {
		rangeM, aspect, toward float64
		band, bin              string
	}{
		{5, 90, 0.3, "range_0_10m", "aspect_60_90deg"},
		{5, 90, 0.3, "range_0_10m", "aspect_60_90deg"},
		{25, 0, -1, "range_20m_plus", "aspect_0_30deg"},
		{15, -1, 0, "range_10_20m", "aspect_unknown"},
	}
	for i, w := range want {
		p := v.Poses[i]
		if !approx(p.RangeM, w.rangeM) || !approx(p.TowardSensorM, w.toward) || p.RangeBand != w.band || p.AspectBin != w.bin {
			t.Errorf("pose %d: %+v, want %+v", i, p, w)
		}
		if (w.aspect < 0) != (p.AspectDeg == nil) || (p.AspectDeg != nil && !approx(*p.AspectDeg, w.aspect)) {
			t.Errorf("pose %d aspect %v, want %g", i, p.AspectDeg, w.aspect)
		}
	}
	cells := map[string]CentreViewCell{}
	for _, c := range v.Cells {
		cells[c.Group+":"+c.Key] = c
	}
	if c := cells["object:car"]; c.Poses != 2 || !approx(c.TowardSensorMedianM, 0.3) || c.WithinReferenceBound != 2 {
		t.Errorf("car cell %+v", c)
	}
	if c := cells["range_aspect:range_20m_plus/aspect_0_30deg"]; c.Poses != 1 || !approx(c.TowardSensorMeanM, -1) ||
		c.WithinReferenceBound != 0 {
		t.Errorf("far end-on cell %+v", c)
	}
	if c := cells["all:all"]; c.Poses != 4 || c.PredictionReference != "cluster_mean" {
		t.Errorf("pooled cell %+v", c)
	}

	if got := summariseCentreView(instants, nil, false); len(got.Poses) != 0 || len(got.Caveats) != 1 {
		t.Errorf("a transformed frame: %+v", got)
	}
}

func TestAspectBinNames(t *testing.T) {
	for a, want := range map[float64]string{0: "aspect_0_30deg", 29.9: "aspect_0_30deg", 30: "aspect_30_60deg",
		60: "aspect_60_90deg", 90: "aspect_60_90deg"} {
		if got := aspectBin(a, CentreViewAspectBinEdgesDeg); got != want {
			t.Errorf("aspectBin(%g) = %s, want %s", a, got, want)
		}
	}
	for _, c := range []struct{ bearing, yaw, want float64 }{
		{0, 0, 0}, {math.Pi / 2, 0, 90}, {math.Pi, 0, 0}, {math.Pi / 4, -math.Pi / 4, 90}, {0.1, math.Pi, 0.1 * 180 / math.Pi},
	} {
		if got := aspectDeg(c.bearing, c.yaw); !approx(got, c.want) {
			t.Errorf("aspectDeg(%g, %g) = %g, want %g", c.bearing, c.yaw, got, c.want)
		}
	}
}
