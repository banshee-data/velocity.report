package perframeeval

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func TestBoxIoU(t *testing.T) {
	unit := annotation.BoxBound{LengthM: 2, WidthM: 2}
	for name, c := range map[string]struct {
		b    annotation.BoxBound
		want float64
	}{
		"identical":        {unit, 1},
		"quarter turn":     {annotation.BoxBound{YawRad: math.Pi / 2, LengthM: 2, WidthM: 2}, 1},
		"half overlap":     {annotation.BoxBound{CentreXM: 1, LengthM: 2, WidthM: 2}, 1.0 / 3},
		"disjoint":         {annotation.BoxBound{CentreXM: 5, LengthM: 2, WidthM: 2}, 0},
		"eighth turn":      {annotation.BoxBound{YawRad: math.Pi / 4, LengthM: 2, WidthM: 2}, (8 * (math.Sqrt2 - 1)) / (8 - 8*(math.Sqrt2-1))},
		"degenerate union": {annotation.BoxBound{}, 0},
	} {
		a := unit
		if name == "degenerate union" {
			a = annotation.BoxBound{}
		}
		if got := boxIoU(a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: IoU %v, want %v", name, got, c.want)
		}
	}
}

func TestPhysicalSmallPieces(t *testing.T) {
	if got := describeUnscored(ComponentAccounting{Expected: 1, Scored: 1}); got != "" {
		t.Errorf("nothing unscored described as %q", got)
	}
	if _, ok := (PhysicalResult{}).Instant("e", "o", 0); ok {
		t.Error("an instant was found in an empty result")
	}
	// Opposite headings are the same axis; three quarters of a half turn is
	// a quarter the other way.
	if a, b := wrapHalfTurn(math.Pi), wrapHalfTurn(3*math.Pi/4); math.Abs(a) > 1e-12 || math.Abs(b+math.Pi/4) > 1e-12 {
		t.Errorf("axis errors %v and %v", a, b)
	}
	d := compareDimension(PredictedExtent{Metres: 5}, annotation.LinearBound{LowerM: 4, UpperM: 4.5, ValueM: 4.2})
	if math.Abs(d.OutsideBoundM-0.5) > 1e-12 || math.Abs(d.ErrorM-0.8) > 1e-12 {
		t.Errorf("above the interval: %+v", d)
	}
}

// The fixture fails loudly when it cannot write what it promises.
func TestPhysicalFixtureRefusesToOverwrite(t *testing.T) {
	for _, occupied := range []string{"pack", "split.json", "evidence.db"} {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, occupied), 0o700); err != nil {
			t.Fatal(err)
		}
		if occupied == "pack" {
			if err := os.WriteFile(filepath.Join(dir, occupied, "x"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := evalfixture.WritePhysical(dir); err == nil {
			t.Errorf("wrote over an occupied %s", occupied)
		}
	}
}

// An episode scores only its own objects' following references.
func TestPhysicalEpisodeScoresOnlyItsFollowers(t *testing.T) {
	f := physFixture(t)
	m := f.Manifest()
	m.Episodes[0].ObjectIDs = []string{evalfixture.Leader}
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	r := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	for _, g := range r.Following {
		if g.FollowerObjectID != evalfixture.Leader {
			t.Fatalf("an unscored follower's gap was expected: %+v", g)
		}
	}
	if r.Reference.ExpectedInstants != evalfixture.PhysSamples {
		t.Fatalf("expected instants %d", r.Reference.ExpectedInstants)
	}
}

func TestAlignPredictionsKeepsTheNearest(t *testing.T) {
	pr := &PhysicalReference{
		samples:  []annotation.Sample{{TimestampNs: 0}, {TimestampNs: 100}},
		Identity: PhysicalReferenceIdentity{FrameToleranceNanos: 10},
	}
	got := pr.alignPredictions([]PredictedBody{
		{TrackKey: "a", TimestampNs: 5}, {TrackKey: "a", TimestampNs: 3}, {TrackKey: "a", TimestampNs: 4},
		{TrackKey: "b", TimestampNs: 50}, {TrackKey: "c", TimestampNs: 98}, {TrackKey: "d", TimestampNs: 120},
	})
	if len(got[0]) != 1 || got[0][0].offset != 3 || len(got[1]) != 1 || got[1][0].body.TrackKey != "c" || got[1][0].offset != -2 {
		t.Fatalf("aligned: %+v", got)
	}
}

// One prediction between two objects goes to the nearer; the other is told
// its candidate was taken. A proposal or a keyframe with no position takes
// no part.
func TestMatchSampleIsOneToOneNearestFirst(t *testing.T) {
	truth := func(x float64) annotation.PhysicalGeometry {
		return annotation.PhysicalGeometry{Truth: true, AnchorPoint: &annotation.PlanarBound{XM: x}}
	}
	pr := &PhysicalReference{
		Identity: PhysicalReferenceIdentity{GateMetres: 1},
		geometry: map[string]map[int]annotation.PhysicalGeometry{
			"far": {0: truth(0)}, "near": {0: truth(1.5)},
			"proposal": {0: {AnchorPoint: &annotation.PlanarBound{XM: 1.4}}}, "nowhere": {0: {Truth: true}},
		},
	}
	m := pr.matchSample(0, []alignedPrediction{{body: PredictedBody{TrackKey: "t", XM: 1.4}}})
	if i, ok := m.matched["near"]; !ok || i != 0 || m.candidates["far"] != 0 {
		t.Fatalf("matches %+v candidates %+v", m.matched, m.candidates)
	}
	// 0.7 m from one and 0.8 m from the other: the nearer pair wins, and the
	// other object is left with a candidate it did not get.
	m = pr.matchSample(0, []alignedPrediction{{body: PredictedBody{TrackKey: "t", XM: 0.7}}})
	if _, ok := m.matched["near"]; ok || m.matched["far"] != 0 || m.candidates["far"] != 1 || m.candidates["near"] != 1 {
		t.Fatalf("matches %+v candidates %+v", m.matched, m.candidates)
	}
}

func TestPredictionAndReferenceReasons(t *testing.T) {
	heading := &PredictedHeading{Resolved: true}
	length := &PredictedExtent{Metres: 4}
	for want, p := range map[string]PredictedBody{
		ReasonPredictionNotOnBody:     {},
		ReasonPredictionNoHeading:     {Physical: true},
		ReasonPredictionAxisAmbiguous: {Physical: true, Heading: &PredictedHeading{}},
		ReasonPredictionNoExtent:      {Physical: true, Heading: heading},
		"":                            {Physical: true, Heading: heading, Length: length},
	} {
		if got := predictionEndsReason(&p, true); got != want {
			t.Errorf("signed ends of %+v: %q, want %q", p, got, want)
		}
	}
	if got := predictionEndsReason(&PredictedBody{Physical: true, Heading: &PredictedHeading{}, Length: length}, false); got != "" {
		t.Errorf("unsigned ends under an ambiguous heading: %q", got)
	}
	centre := &annotation.PlanarBound{}
	for want, g := range map[string]annotation.PhysicalGeometry{
		annotation.UnavailableCentre:      {CentreUnavailable: annotation.UnavailableAnchorOffsetUnknown},
		annotation.UnavailableAxisUnknown: {Centre: centre, YawUnavailable: annotation.UnavailableAxisUnknown},
		annotation.UnavailableNoBody:      {Centre: centre, LengthUnavailable: annotation.UnavailableNoBody},
		"":                                {Ends: []annotation.PlanarBound{{}, {}}},
	} {
		if got := endsUnavailable(g); got != want {
			t.Errorf("ends of %+v: %q, want %q", g, got, want)
		}
	}
	for want, g := range map[string]annotation.PhysicalGeometry{
		ReasonReferenceTrackerAssisted: {Origin: annotation.OriginTrackerAssisted},
		ReasonReferenceUnreviewed:      {Origin: annotation.OriginIndependent, ReviewStatus: annotation.StatusProposed},
		"":                             {Origin: annotation.OriginIndependent, ReviewStatus: annotation.StatusReviewed},
	} {
		if got := referenceReason(g, true); got != want {
			t.Errorf("reference %+v: %q, want %q", g, got, want)
		}
	}
}

// A partial span's lower bound is checked for every dimension, and not at
// all when the prediction has no belief about it.
func TestLowerBoundChecks(t *testing.T) {
	lower := annotation.DimensionBound{Status: annotation.EvidenceObserved, Span: annotation.SpanPartial, LowerM: func() *float64 { v := 3.0; return &v }()}
	s := physicalScorer{pr: &PhysicalReference{bodies: map[string]annotation.BodyGeometry{
		"o": {Length: lower, Width: lower, Height: lower},
	}}}
	in := PhysicalInstant{ObjectID: "o", Prediction: &PredictedBody{
		Length: &PredictedExtent{Metres: 2}, Width: &PredictedExtent{Metres: 3.5}, Height: &PredictedExtent{Metres: 3},
	}}
	for _, c := range []PhysicalComponent{ComponentLength, ComponentWidth, ComponentHeight} {
		s.lowerBound(&in, c)
	}
	lb := in.Comparison.LowerBounds
	if lb[ComponentLength].ShortfallM != 1 || lb[ComponentWidth].ShortfallM != 0 || lb[ComponentHeight].ShortfallM != 0 {
		t.Fatalf("lower bounds: %+v", lb)
	}
	bare := PhysicalInstant{ObjectID: "o", Prediction: &PredictedBody{}}
	s.lowerBound(&bare, ComponentWidth)
	if bare.Comparison.LowerBounds != nil {
		t.Fatal("a prediction with no width was checked against one")
	}
}

// A near-face estimate places its centre by its declared body-frame offset.
func TestPredictedBodyOffsetsANearFaceAnchor(t *testing.T) {
	s := l8behaviour.TrajectorySample{
		Reference: l8behaviour.ReferenceNearFaceCentre, X: 10, Y: 0,
		AnchorToCentre: l8behaviour.BodyOffset{LongitudinalM: -2, LateralM: 1},
		Heading:        l8behaviour.HeadingBelief{Rad: math.Pi / 2, Provenance: l8behaviour.ProvenanceObserved},
	}
	b := predictedBody("k", s)
	if !b.Physical || math.Abs(b.CentreXM-9) > 1e-9 || math.Abs(b.CentreYM+2) > 1e-9 || !b.Heading.Resolved {
		t.Fatalf("near-face centre: %+v", b)
	}
}

// Every way a following instant can go unscored, from the fixture with one
// thing changed.
func TestPhysicalFollowingReasons(t *testing.T) {
	f := physFixture(t)
	load := func(t *testing.T) (*PhysicalReference, PhysicalArm) {
		t.Helper()
		pr, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions())
		if err != nil {
			t.Fatal(err)
		}
		arm, err := LoadPhysicalArm(physArm(f, "arm", evalfixture.ParamsExact))
		if err != nil {
			t.Fatal(err)
		}
		return pr, arm
	}
	gapAt := func(t *testing.T, r PhysicalResult, sample int) PhysicalFollowingInstant {
		t.Helper()
		for _, g := range r.Following {
			if g.FollowingID == "follow-lead" && g.SampleID == sample {
				return g
			}
		}
		t.Fatalf("no gap at %d", sample)
		return PhysicalFollowingInstant{}
	}
	withBodies := func(arm PhysicalArm, keep func(PredictedBody) (PredictedBody, bool)) PhysicalArm {
		var out []PredictedBody
		for _, b := range arm.Bodies {
			if nb, ok := keep(b); ok {
				out = append(out, nb)
			}
		}
		arm.Bodies = out
		return arm
	}
	at := func(sample int, track string) func(PredictedBody) bool {
		return func(b PredictedBody) bool {
			return b.TrackKey == track && b.TimestampNs == evalfixture.SampleTime(sample)+evalfixture.EstimateOffsetNs
		}
	}
	cases := []struct {
		name     string
		sample   int
		mutate   func(*PhysicalReference, PhysicalArm) PhysicalArm
		category string
		reason   string
	}{
		{"tracker-assisted decision", 0, func(pr *PhysicalReference, a PhysicalArm) PhysicalArm {
			for i := range pr.following {
				pr.following[i].ref.Review.Origin = annotation.OriginTrackerAssisted
			}
			return a
		}, OutcomeUnknownGeometry, ReasonReferenceTrackerAssisted},
		{"unreviewed decision", 0, func(pr *PhysicalReference, a PhysicalArm) PhysicalArm {
			for i := range pr.following {
				pr.following[i].ref.Review.Status = annotation.StatusProposed
			}
			return a
		}, OutcomeUnknownGeometry, ReasonReferenceUnreviewed},
		{"unknown gap", 0, func(pr *PhysicalReference, a PhysicalArm) PhysicalArm {
			for i := range pr.following {
				gaps := append([]annotation.FollowingGap(nil), pr.following[i].ref.Gaps...)
				for g := range gaps {
					gaps[g].Status = annotation.EvidenceUnknown
				}
				pr.following[i].ref.Gaps = gaps
			}
			return a
		}, OutcomeUnknownGeometry, "unknown"},
		{"nothing predicted", 0, func(_ *PhysicalReference, a PhysicalArm) PhysicalArm {
			return withBodies(a, func(b PredictedBody) (PredictedBody, bool) { return b, b.TimestampNs > evalfixture.SampleTime(1) })
		}, OutcomeMissingPrediction, ReasonNoPredictionAtInstant},
		{"follower unmatched", 0, func(_ *PhysicalReference, a PhysicalArm) PhysicalArm {
			return withBodies(a, func(b PredictedBody) (PredictedBody, bool) { return b, !at(0, "seq-000001")(b) })
		}, OutcomeUnmatched, ReasonFollowerUnmatched},
		{"leader unmatched", 0, func(_ *PhysicalReference, a PhysicalArm) PhysicalArm {
			return withBodies(a, func(b PredictedBody) (PredictedBody, bool) { return b, !at(0, "seq-000002")(b) })
		}, OutcomeUnmatched, ReasonLeaderUnmatched},
		{"orientation unresolved", 5, func(_ *PhysicalReference, a PhysicalArm) PhysicalArm {
			return withBodies(a, func(b PredictedBody) (PredictedBody, bool) {
				if at(5, "seq-000001")(b) {
					h := *b.Heading
					h.Resolved = false
					b.Heading = &h
				}
				return b, true
			})
		}, OutcomeMissingPrediction, ReasonOrientationUnresolved},
		{"instants differ", 0, func(_ *PhysicalReference, a PhysicalArm) PhysicalArm {
			return withBodies(a, func(b PredictedBody) (PredictedBody, bool) {
				if at(0, "seq-000002")(b) {
					b.TimestampNs++
				}
				return b, true
			})
		}, OutcomeMissingPrediction, "prediction_instants_differ"},
		{"leader not on the body", 0, func(_ *PhysicalReference, a PhysicalArm) PhysicalArm {
			return withBodies(a, func(b PredictedBody) (PredictedBody, bool) {
				if at(0, "seq-000002")(b) {
					b.sample.Reference = l8behaviour.ReferenceClusterMedoid
				}
				return b, true
			})
		}, OutcomeMissingPrediction, "leader_insufficient_observation"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pr, arm := load(t)
			arm = c.mutate(pr, arm)
			r, err := ScorePhysical(pr, arm)
			if err != nil {
				t.Fatal(err)
			}
			if g := gapAt(t, r, c.sample); g.Outcome.Category != c.category || g.Outcome.Reason != c.reason {
				t.Fatalf("outcome %+v, want %s/%s", g.Outcome, c.category, c.reason)
			}
			checkComplete(t, r)
		})
	}

	// A leader placed behind its follower gives a gap the behaviour layer
	// would suppress: it is still compared, with the reason beside it.
	pr, arm := load(t)
	arm = withBodies(arm, func(b PredictedBody) (PredictedBody, bool) {
		if at(0, "seq-000002")(b) {
			b.sample.X = evalfixture.FollowerX(0) + 1
			b.XM, b.CentreXM = b.sample.X, b.sample.X
		}
		return b, true
	})
	pr.Identity.GateMetres = 20
	r, err := ScorePhysical(pr, arm)
	if err != nil {
		t.Fatal(err)
	}
	if g := gapAt(t, r, 0); g.Outcome.Category != OutcomeScored || g.PredictedGap.PredictionReason != "non_positive_gap" || *g.OutsideBoundM >= 0 {
		t.Fatalf("overlapping pair: %+v", g)
	}
	// A leader a metre further ahead: the gap is over the reference bound.
	pr, arm = load(t)
	arm = withBodies(arm, func(b PredictedBody) (PredictedBody, bool) {
		if at(0, "seq-000002")(b) {
			b.sample.X++
		}
		return b, true
	})
	if r, err = ScorePhysical(pr, arm); err != nil {
		t.Fatal(err)
	}
	if g := gapAt(t, r, 0); math.Abs(*g.OutsideBoundM-0.8) > 1e-5 {
		t.Fatalf("a long gap: %+v", g)
	}
	// Two predictions that share a frame but not a capture time are not one
	// instant: the behaviour layer refuses to difference them.
	pr, arm = load(t)
	arm = withBodies(arm, func(b PredictedBody) (PredictedBody, bool) {
		if at(0, "seq-000002")(b) {
			b.sample.CaptureUnixNanos++
			b.sample.LastObservedUnixNanos++
		}
		return b, true
	})
	if _, err := ScorePhysical(pr, arm); err == nil || !strings.Contains(err.Error(), "not synchronised") {
		t.Fatalf("unsynchronised endpoints: %v", err)
	}

	// A prediction that is not a valid sample is a defect, not a reason.
	for _, track := range []string{"seq-000001", "seq-000002"} {
		pr, arm = load(t)
		arm = withBodies(arm, func(b PredictedBody) (PredictedBody, bool) {
			if at(0, track)(b) {
				b.sample.StateModel = ""
			}
			return b, true
		})
		if _, err := ScorePhysical(pr, arm); err == nil || !strings.Contains(err.Error(), "state model") {
			t.Fatalf("an invalid %s sample was scored: %v", track, err)
		}
	}
}

// Rows the behaviour adapters refuse are refused here too, with the arm
// named: an estimate whose measurement names no reference point, a solid
// body with no stored observation, one whose reference the adapter cannot
// place, and two tracks under one creation sequence.
func TestLoadPhysicalArmRefusesUnreadableRows(t *testing.T) {
	f := physFixture(t)
	insertUnreadableRows(t, f.DBPath)
	for params, want := range map[string]string{
		"params/bad-source": "names no reference point",
		"params/orphan":     "not stored",
		"params/near-face":  "no declared offset",
		"params/two-runs":   "names two tracks",
	} {
		spec := ArmSpec{Label: "x", DBPath: f.DBPath, ParamHash: params, Stage: "online", DeclaredBaseline: true, SolidBodies: true}
		if params == "params/bad-source" {
			spec.Stage, spec.SolidBodies = "", false
		}
		if _, err := LoadPhysicalArm(spec); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v (want %q)", params, err, want)
		}
	}
}

// insertUnreadableRows adds versions whose rows the behaviour adapters, or
// the version reader, refuse.
func insertUnreadableRows(t *testing.T, path string) {
	t.Helper()
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewStateEstimateStore(database.DB)
	observe := func(id string, frame int64) {
		t.Helper()
		if _, err := database.Exec(`INSERT INTO lidar_observations
			(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos,
			 cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
			VALUES (?, 1, 's', 'c', 'fixture-sensor', 'f', ?, ?, 1, '{}', 1)`, id, frame, frame); err != nil {
			t.Fatal(err)
		}
	}
	frame := evalfixture.SampleTime(0)
	e := sqlite.TrackEstimate{
		EstimateID: "e/bad", TrackID: "t", ObservationID: "o/bad", SourceID: "s", CalibrationID: "c",
		FrameUnixNanos: frame, MeasurementUnixNanos: frame, EstimatorID: evalfixture.EstimatorID, ObservationModelID: "m",
		ParamHash: "params/bad-source", Stage: "final", MeasurementSource: "mystery_v1",
	}
	observe(e.ObservationID, frame)
	if err := store.Insert(e, sqlite.TrackResidual{EstimateID: e.EstimateID, ObservationID: e.ObservationID, Disposition: "accepted", Reason: "t"}); err != nil {
		t.Fatal(err)
	}
	solid := func(params, id string, seq int64, ref l5tracks.ReferencePoint, withObservation bool) {
		t.Helper()
		sb := sqlite.TrackSolidBody{
			EstimateID: "sb/" + params + id, TrackID: "track-" + id, ObservationID: "o/" + params + id, SourceID: "s", CalibrationID: "c",
			FrameUnixNanos: frame, MeasurementUnixNanos: frame, EstimatorID: evalfixture.EstimatorID, ObservationModelID: "m",
			ParamHash: params, Stage: "online", CreationSequence: seq,
			Reading: l5tracks.SolidBodyReading{Estimate: l5tracks.SolidBodyEstimate{
				StateModel: l5tracks.StateModelCVCartesianV1, Reference: ref, Stage: l5tracks.StageLive,
				Estimation: l5tracks.EstimationInitialising, LastObservedUnixNanos: frame,
			}},
		}
		if withObservation {
			observe(sb.ObservationID, frame)
		}
		if err := store.InsertSolidBody(sb); err != nil {
			t.Fatal(err)
		}
	}
	solid("params/orphan", "a", 1, l5tracks.ReferenceClusterMedoid, false)
	solid("params/near-face", "a", 1, l5tracks.ReferenceNearFaceCentre, true)
	solid("params/two-runs", "a", 1, l5tracks.ReferenceClusterMedoid, true)
	solid("params/two-runs", "b", 1, l5tracks.ReferenceClusterMedoid, true)
	database.Close()
}

// The instant-level ways to go unmatched or unpredicted: nothing predicted
// at all, a truth keyframe with no position, and a prediction won by a
// nearer or earlier object.
func TestPhysicalInstantMatchingReasons(t *testing.T) {
	f := physFixture(t)
	pr, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	arm, err := LoadPhysicalArm(physArm(f, "arm", evalfixture.ParamsExact))
	if err != nil {
		t.Fatal(err)
	}
	first := evalfixture.SampleTime(0) + evalfixture.EstimateOffsetNs
	var rest []PredictedBody
	for _, b := range arm.Bodies {
		if b.TimestampNs != first {
			rest = append(rest, b)
		}
	}
	bare := arm
	bare.Bodies = rest
	r, err := ScorePhysical(pr, bare)
	if err != nil {
		t.Fatal(err)
	}
	wantOutcome(t, instantOf(t, r, evalfixture.Follower, 0), ComponentCentre, OutcomeMissingPrediction, ReasonNoPredictionAtInstant)

	// One prediction halfway between the two cars, inside a wide gate: the
	// tie goes to the first object by ID, and the other is told why.
	between := arm.Bodies[0]
	between.TrackKey, between.TimestampNs = "seq-000009", first
	between.XM, between.CentreXM = 15, 15
	wide := *pr
	wide.Identity.GateMetres = 6
	crowded := arm
	crowded.Bodies = append(append([]PredictedBody(nil), rest...), between)
	if r, err = ScorePhysical(&wide, crowded); err != nil {
		t.Fatal(err)
	}
	wantOutcome(t, instantOf(t, r, evalfixture.Follower, 0), ComponentCentre, OutcomeScored, "")
	wantOutcome(t, instantOf(t, r, evalfixture.Leader, 0), ComponentYaw, OutcomeUnmatched, ReasonPredictionTaken)

	// A truth keyframe that states no position cannot be matched.
	g := pr.geometry[evalfixture.Follower][0]
	g.AnchorPoint, g.Centre, g.CentreUnavailable = nil, nil, annotation.UnavailablePosition
	pr.geometry[evalfixture.Follower][0] = g
	if r, err = ScorePhysical(pr, arm); err != nil {
		t.Fatal(err)
	}
	in := instantOf(t, r, evalfixture.Follower, 0)
	wantOutcome(t, in, ComponentCentre, OutcomeUnknownGeometry, annotation.UnavailablePosition)
	wantOutcome(t, in, ComponentYaw, OutcomeUnmatched, ReasonNoReferencePosition)
}
