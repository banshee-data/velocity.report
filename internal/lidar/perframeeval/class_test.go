package perframeeval

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

const (
	classFrame  = int64(100_000_000)
	classOffset = int64(1_000_000) // hypotheses stamped 1 ms late, as a replay's are
)

var (
	beliefUnknown    = l5tracks.MotionClassBelief{Class: l5tracks.MotionUnknown}
	beliefRigid      = l5tracks.MotionClassBelief{Class: l5tracks.MotionRigidVehicle, Posterior: 0.9}
	beliefPedestrian = l5tracks.MotionClassBelief{Class: l5tracks.MotionPedestrian, Posterior: 0.8}
)

// classReference is one episode of ten frames with a car, a truck, a
// pedestrian, all moving along +x at a metre a frame on their own lanes, and
// a noise object every point of which is ignored.
func classReference() *Reference {
	ep := EpisodeReference{EpisodeID: "ep", spans: []timeSpan{{0, 9 * classFrame}}}
	lanes := []struct {
		id     string
		y      float32
		ignore bool
	}{{"car", 0, false}, {"truck", 20, false}, {"ped", 40, false}, {"noise", 60, true}}
	for i := 0; i < 10; i++ {
		ep.Frames = append(ep.Frames, int64(i)*classFrame)
	}
	for _, l := range lanes {
		s := l8analytics.TrackSeries{ID: l.id}
		for i := 0; i < 10; i++ {
			s.Points = append(s.Points, l8analytics.SeriesPoint{TimestampNanos: int64(i) * classFrame, X: float32(i), Y: l.y, Ignore: l.ignore})
		}
		ep.Series = append(ep.Series, s)
	}
	return &Reference{
		Episodes:      []EpisodeReference{ep},
		ObjectClasses: map[string]string{"car": "car", "truck": "truck", "ped": "pedestrian", "noise": "noise"},
	}
}

// classTrack is a hypothesis on lane y over frames [from, to], stamped late,
// with class rows from classAt (nil for no row) at the same late stamps.
func classTrack(id string, y float32, from, to int, classAt func(int) *l5tracks.MotionClassBelief) (l8analytics.TrackSeries, []ClassRow) {
	s := l8analytics.TrackSeries{ID: id}
	var rows []ClassRow
	for i := from; i <= to; i++ {
		ts := int64(i)*classFrame + classOffset
		s.Points = append(s.Points, l8analytics.SeriesPoint{TimestampNanos: ts, X: float32(i), Y: y})
		if b := classAt(i); b != nil {
			rows = append(rows, ClassRow{FrameUnixNanos: ts, Belief: *b})
		}
	}
	return s, rows
}

func when(rule func(int) l5tracks.MotionClassBelief) func(int) *l5tracks.MotionClassBelief {
	return func(i int) *l5tracks.MotionClassBelief { b := rule(i); return &b }
}

// classScene, counted by hand:
//
//	car    seq-000001 frames 0-4: unknown at 0-2, rigid at 3-4 (ends rigid).
//	       Unmatched at frame 5. seq-000002 frames 6-10: rigid at 6-8,
//	       pedestrian at 9, unknown at 10, which lies outside the episode, so
//	       the end track ends unknown although its last matched frame was
//	       pedestrian. Rigid at 5 of 10 reviewed frames; first rigid 0.3 s in.
//	truck  seq-000003 frames 0-9: unknown at 0-1, rigid from 2 (age 0.2 s).
//	       seq-000004 is a duplicate 0.3 m off, pedestrian throughout; the
//	       matching keeps seq-000003, so it never counts.
//	ped    seq-000005 frames 0-9: pedestrian, no row at 2, rigid at 9.
//	noise  every point ignored: not a reviewed road user, not reported.
func classScene() (Hypothesis, ClassRows) {
	hyp := Hypothesis{Identity: ArmIdentity{Label: "arm", Kind: ArmEstimates, TrackKey: "creation_sequence"}}
	rows := ClassRows{Source: ClassSource{Table: solidBodyTable}, Tracks: map[string][]ClassRow{}}
	add := func(s l8analytics.TrackSeries, r []ClassRow) {
		hyp.Series = append(hyp.Series, s)
		rows.Tracks[s.ID] = r
	}
	add(classTrack("seq-000001", 0, 0, 4, when(func(i int) l5tracks.MotionClassBelief {
		if i < 3 {
			return beliefUnknown
		}
		return beliefRigid
	})))
	add(classTrack("seq-000002", 0, 6, 10, when(func(i int) l5tracks.MotionClassBelief {
		switch {
		case i <= 8:
			return beliefRigid
		case i == 9:
			return beliefPedestrian
		}
		return beliefUnknown
	})))
	add(classTrack("seq-000003", 20, 0, 9, when(func(i int) l5tracks.MotionClassBelief {
		if i < 2 {
			return beliefUnknown
		}
		return beliefRigid
	})))
	add(classTrack("seq-000004", 20.3, 0, 9, when(func(int) l5tracks.MotionClassBelief { return beliefPedestrian })))
	add(classTrack("seq-000005", 40, 0, 9, func(i int) *l5tracks.MotionClassBelief {
		switch i {
		case 2:
			return nil
		case 9:
			return &beliefRigid
		}
		return &beliefPedestrian
	}))
	return hyp, rows
}

func classOptions() ScoreOptions {
	opts := DefaultScoreOptions()
	opts.Gate = l8analytics.FixedGate(1)
	return opts
}

func approxClass(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil || math.Abs(*got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", name, deref(got), want)
	}
}

func deref(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func TestScoreClassCountsTheHandBuiltScene(t *testing.T) {
	hyp, rows := classScene()
	res, err := ScoreClass(classReference(), hyp, rows, classOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Objects) != 3 {
		t.Fatalf("objects %d, want car, ped and truck only: %+v", len(res.Objects), res.Objects)
	}
	byID := map[string]ClassObject{}
	for _, o := range res.Objects {
		byID[o.ObjectID] = o
	}

	car := byID["car"]
	if car.Frames != 10 || car.MatchedFrames != 9 || car.RigidFrames != 5 || car.ExpectedFrames != 5 {
		t.Errorf("car frames %+v", car)
	}
	if len(car.Tracks) != 2 || car.Tracks[0].Track != "seq-000001" || car.Tracks[1].Track != "seq-000002" {
		t.Fatalf("car tracks %+v", car.Tracks)
	}
	if car.EndTrack != "seq-000002" || car.EndClass != "unknown" || car.EndsRigid || !car.AnyTrackEndsRigid {
		t.Errorf("car end: track %s class %s rigid %v any %v; the fragment that ended rigid counts only as any",
			car.EndTrack, car.EndClass, car.EndsRigid, car.AnyTrackEndsRigid)
	}
	if !car.Tracks[0].EndsRigid || *car.Tracks[0].FirstRigidAgeRows != 3 {
		t.Errorf("first fragment %+v", car.Tracks[0])
	}
	approxClass(t, "car first fragment age", car.Tracks[0].FirstRigidAgeS, 0.3)
	approxClass(t, "car end track age", car.Tracks[1].FirstRigidAgeS, 0)
	approxClass(t, "car time to first rigid", car.TimeToFirstRigidS, 0.3)

	truck := byID["truck"]
	if len(truck.Tracks) != 1 || truck.EndTrack != "seq-000003" || !truck.EndsRigid || truck.RigidFrames != 8 {
		t.Errorf("truck %+v: the duplicate the matching did not choose must not count", truck)
	}
	approxClass(t, "truck end track age", truck.Tracks[0].FirstRigidAgeS, 0.2)
	if p := truck.Tracks[0].EndPosterior; p == nil || math.Abs(*p-0.9) > 1e-6 {
		t.Errorf("truck end posterior %v", deref(p))
	}

	ped := byID["ped"]
	if ped.EndClass != "rigid_vehicle" || !ped.EndsRigid || ped.RigidFrames != 1 || ped.UnclassedFrames != 1 || ped.ExpectedFrames != 8 {
		t.Errorf("ped %+v", ped)
	}

	pooled, _ := json.Marshal(res.ClassSummary)
	if len(res.Episodes) != 1 {
		t.Errorf("episodes %+v", res.Episodes)
	} else if only, _ := json.Marshal(res.Episodes[0].ClassSummary); string(only) != string(pooled) {
		t.Errorf("one episode's summary is not the pooled one:\n%s\n%s", only, pooled)
	}
	v := res.Vehicles
	if v.Objects != 2 || v.MatchedObjects != 2 || v.EndRigid != 1 || v.AnyTrackEndsRigid != 2 ||
		v.Frames != 20 || v.MatchedFrames != 19 || v.RigidFrames != 13 {
		t.Errorf("vehicles %+v", v)
	}
	if !near(v.EndRigidShare, 0.5) || !near(v.RigidFrameShare, 0.65) || !near(v.RigidMatchedFrameShare, 13.0/19) {
		t.Errorf("vehicle shares %+v", v)
	}
	if a := v.TrackAgeAtFirstRigid; a.N != 2 || a.Never != 0 {
		t.Errorf("track age %+v", a)
	}
	approxClass(t, "track age median", v.TrackAgeAtFirstRigid.Median, 0.1)
	approxClass(t, "track age p90", v.TrackAgeAtFirstRigid.P90, 0.18)
	approxClass(t, "object time median", v.ObjectTimeToFirstRigid.Median, 0.25)
	approxClass(t, "object time p90", v.ObjectTimeToFirstRigid.P90, 0.29)

	f := res.FalseApplicability
	if f.Objects != 1 || f.MatchedObjects != 1 || f.EndRigid != 1 || !near(f.EndRigidShare, 1) || f.Frames != 10 || f.RigidFrames != 1 {
		t.Errorf("false applicability %+v", f)
	}

	labels := map[string]LabelClassSummary{}
	for _, l := range res.ByLabel {
		labels[l.Label] = l
	}
	if _, ok := labels["noise"]; ok || len(labels) != 3 {
		t.Errorf("labels %v", res.ByLabel)
	}
	carL := labels["car"]
	if carL.ExpectedMotionClass != "rigid_vehicle" || carL.EndExpected != 0 || !near(carL.FrameRecall, 0.5) ||
		carL.EndClasses["unknown"] != 1 || carL.FrameClasses[ClassUnmatched] != 1 || carL.FrameClasses["unknown"] != 3 ||
		carL.FrameClasses["rigid_vehicle"] != 5 || carL.FrameClasses["pedestrian"] != 1 {
		t.Errorf("car label %+v", carL)
	}
	if l := labels["truck"]; l.EndExpected != 1 || !near(l.EndRecall, 1) || !near(l.FrameRecall, 0.8) {
		t.Errorf("truck label %+v", l)
	}
	if l := labels["pedestrian"]; l.ExpectedMotionClass != "pedestrian" || l.EndExpected != 0 ||
		!near(l.FrameRecall, 0.8) || l.FrameClasses[ClassUnclassed] != 1 {
		t.Errorf("pedestrian label %+v", l)
	}
}

// Overlapping episodes: the pooled summary counts an object once per episode
// that scores it, as the MOT totals do, and each episode's summary once.
func TestScoreClassSummarisesEachEpisode(t *testing.T) {
	ref := classReference()
	second := ref.Episodes[0]
	second.EpisodeID = "ep2"
	ref.Episodes = append(ref.Episodes, second)
	hyp, rows := classScene()
	res, err := ScoreClass(ref, hyp, rows, classOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Episodes) != 2 || res.Episodes[0].EpisodeID != "ep" || res.Episodes[1].EpisodeID != "ep2" {
		t.Fatalf("episodes %+v", res.Episodes)
	}
	for _, ep := range res.Episodes {
		if v := ep.Vehicles; v.Objects != 2 || v.EndRigid != 1 || v.Frames != 20 || v.RigidFrames != 13 {
			t.Errorf("episode %s vehicles %+v", ep.EpisodeID, v)
		}
	}
	if v := res.Vehicles; v.Objects != 4 || v.EndRigid != 2 || v.Frames != 40 || !near(v.EndRigidShare, 0.5) {
		t.Errorf("pooled vehicles %+v", v)
	}
	if len(res.Objects) != 6 {
		t.Errorf("objects %d, want three per episode", len(res.Objects))
	}
}

func TestScoreClassReportsAnUnmatchedVehicle(t *testing.T) {
	hyp, rows := classScene()
	hyp.Series = hyp.Series[2:] // drop both car fragments
	res, err := ScoreClass(classReference(), hyp, rows, classOptions())
	if err != nil {
		t.Fatal(err)
	}
	v := res.Vehicles
	if v.Objects != 2 || v.MatchedObjects != 1 || v.EndRigid != 1 || !near(v.EndRigidShare, 0.5) {
		t.Errorf("an unmatched vehicle must count against the share: %+v", v)
	}
	if v.TrackAgeAtFirstRigid.N != 1 || v.TrackAgeAtFirstRigid.Never != 0 {
		t.Errorf("an unmatched vehicle has no end track to age: %+v", v.TrackAgeAtFirstRigid)
	}
	for _, o := range res.Objects {
		if o.ObjectID == "car" && (o.EndClass != ClassUnmatched || o.EndTrack != "" || o.MatchedFrames != 0 || o.Tracks == nil) {
			t.Errorf("unmatched car %+v", o)
		}
	}
}

func TestScoreClassCountsAVehicleNeverRigid(t *testing.T) {
	hyp, rows := classScene()
	rows.Tracks["seq-000003"] = nil // the truck's track has no class row at all
	res, err := ScoreClass(classReference(), hyp, rows, classOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range res.Objects {
		if o.ObjectID == "truck" && (o.EndClass != ClassUnclassed || o.EndsRigid || o.UnclassedFrames != 10) {
			t.Errorf("unclassed truck %+v", o)
		}
	}
	if a := res.Vehicles.TrackAgeAtFirstRigid; a.N != 1 || a.Never != 1 {
		t.Errorf("track age %+v", a)
	}
}

// The rule is the following gate's, not a list of class names: a belief
// whose posterior is split between rigid and pedestrian runs, and is
// admitted, as the weaker class.
func TestFollowingAdmitsIsTheGatesRule(t *testing.T) {
	split := l5tracks.MotionClassBelief{Class: l5tracks.MotionRigidVehicle, Posterior: 0.5,
		Runner: l5tracks.MotionPedestrian, RunnerPosterior: 0.45}
	for _, c := range []struct {
		name   string
		belief l5tracks.MotionClassBelief
		want   bool
	}{
		{"rigid", beliefRigid, true},
		{"rigid at low confidence", l5tracks.MotionClassBelief{Class: l5tracks.MotionRigidVehicle, Posterior: 0.1}, true},
		{"unknown", beliefUnknown, false},
		{"pedestrian", beliefPedestrian, false},
		{"two-wheeler", l5tracks.MotionClassBelief{Class: l5tracks.MotionTwoWheeler, Posterior: 0.9}, false},
		{"split rigid against pedestrian", split, false},
	} {
		if got := followingAdmits(c.belief); got != c.want {
			t.Errorf("%s: admitted %v, want %v", c.name, got, c.want)
		}
	}
	for label, want := range map[string]string{
		"car": "rigid_vehicle", "truck": "rigid_vehicle", "bus": "rigid_vehicle", "van": "rigid_vehicle",
		"cyclist": "two_wheeler", "motorcycle": "two_wheeler", "pedestrian": "pedestrian", "noise": "unknown",
	} {
		if got := labelMotionClass(label).String(); got != want {
			t.Errorf("label %s expects %s, want %s", label, got, want)
		}
	}
}

// writeClassRows files a solid body with a motion belief beside each of arm
// A's online point estimates: unknown before sample 3, rigid from it.
func writeClassRows(t *testing.T, path string) {
	t.Helper()
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := sqlite.NewStateEstimateStore(database.DB)
	points, err := store.ListEstimatePositions(sqlite.EstimateVersion{
		SourceID: evalfixture.SourceID, EstimatorID: evalfixture.EstimatorID,
		ObservationModelID: evalfixture.ObservationModelID, ParamHash: evalfixture.ParamsA, Stage: StageOnline,
	})
	if err != nil || len(points) == 0 {
		t.Fatalf("fixture point estimates: %d, %v", len(points), err)
	}
	for _, p := range points {
		motion := beliefUnknown
		if p.FrameUnixNanos >= evalfixture.SampleTime(3) {
			motion = beliefRigid
		}
		id := fmt.Sprintf("solid_body/%d/%d", p.CreationSequence, p.FrameUnixNanos)
		if err := store.InsertSolidBody(sqlite.TrackSolidBody{
			EstimateID: id, TrackID: p.TrackID, ObservationID: "observation/" + id,
			SourceID: evalfixture.SourceID, CalibrationID: "calibration/v1/fixture",
			FrameUnixNanos: p.FrameUnixNanos, MeasurementUnixNanos: p.FrameUnixNanos,
			EstimatorID: evalfixture.EstimatorID, ObservationModelID: string(l5tracks.MeasurementNearEdgeCandidateV1),
			ParamHash: evalfixture.ParamsA, Stage: StageOnline, CreationSequence: p.CreationSequence,
			Reading: l5tracks.SolidBodyReading{Estimate: l5tracks.SolidBodyEstimate{
				StateModel: l5tracks.StateModelCVCartesianV1, Reference: l5tracks.ReferenceBodyCentre,
				X: p.X, Y: p.Y, Motion: motion,
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// Through Run and the database: a point-estimate arm reads the solid bodies
// filed beside it, a solid-body arm its own rows, and both agree.
func TestRunReportsTheClassMeasureFromSolidBodies(t *testing.T) {
	f := fixture(t)
	writeClassRows(t, f.DBPath)
	cfg := baseConfig(f)
	cfg.A = ArmSpec{Label: "point", DBPath: f.DBPath, ParamHash: evalfixture.ParamsA, Stage: StageOnline, DeclaredBaseline: true}
	cfg.B = cfg.A
	cfg.B.Label, cfg.B.SolidBodies = "solid", true
	cfg.Class = true

	c, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.Class == nil {
		t.Fatal("no class section")
	}
	for _, arm := range []ClassResult{c.Class.A, c.Class.B} {
		if arm.ClassSource.Table != solidBodyTable || arm.ClassSource.ParamHash != evalfixture.ParamsA || arm.ClassSource.Stage != StageOnline {
			t.Errorf("arm %s read classes from %+v", arm.Arm.Label, arm.ClassSource)
		}
		// obj_car_a is the only reviewed object scored in hold-ep1: nine
		// frames (occluded at 5), rigid from sample 3, so at six of them.
		v := arm.Vehicles
		if v.Objects != 1 || v.EndRigid != 1 || v.Frames != 9 || v.RigidFrames != 6 {
			t.Errorf("arm %s vehicles %+v", arm.Arm.Label, v)
		}
		approxClass(t, arm.Arm.Label+" track age", v.TrackAgeAtFirstRigid.Median, 0.3)
		approxClass(t, arm.Arm.Label+" object time", v.ObjectTimeToFirstRigid.Median, 0.3)
	}
	if md := RenderMarkdown(*c); !strings.Contains(md, "## Class evidence for the following gate") ||
		!strings.Contains(md, "| solid | hold-ep1 | 1 | 1 (1.000) |") {
		t.Errorf("markdown has no class table:\n%s", md)
	}
	if c.Class.Definition.LabelMotionClasses["car"] != "rigid_vehicle" || !strings.Contains(c.Class.Definition.Rigid, "ClassApplicability") {
		t.Errorf("definition %+v", c.Class.Definition)
	}
}

func TestRunWithoutTheClassMeasureWritesNoClassKey(t *testing.T) {
	f := fixture(t)
	c, err := Run(baseConfig(f))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if c.Class != nil || strings.Contains(string(b), `"class":{`) {
		t.Fatal("the class measure ran without being asked for")
	}
}

func TestClassMeasureRefusesAReplayWithoutSolidBodies(t *testing.T) {
	f := fixture(t)
	cfg := baseConfig(f)
	cfg.Class = true
	_, err := Run(cfg)
	if err == nil || !strings.Contains(err.Error(), "solid_body experiment") {
		t.Fatalf("a replay with no class rows was scored: %v", err)
	}
}

func TestClassMeasureRefusesAnalysisRuns(t *testing.T) {
	f := fixture(t)
	cfg := baseConfig(f)
	cfg.A = ArmSpec{Label: "A", DBPath: f.DBPath, RunID: evalfixture.RunA, DeclaredBaseline: true}
	cfg.B = ArmSpec{Label: "B", DBPath: f.DBPath, RunID: evalfixture.RunB, DeclaredBaseline: true}
	cfg.Class = true
	_, err := Run(cfg)
	if err == nil || !strings.Contains(err.Error(), "needs an estimate version") {
		t.Fatalf("an analysis run was given a class measure: %v", err)
	}
}
