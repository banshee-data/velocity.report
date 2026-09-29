package perframeeval

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

func physFixture(t *testing.T) *evalfixture.PhysicalFixture {
	t.Helper()
	f, err := evalfixture.WritePhysical(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func physRefOpts(f *evalfixture.PhysicalFixture) ReferenceOptions {
	return ReferenceOptions{PackDir: f.PackDir, SplitManifestPath: f.SplitManifestPath, Split: evalfixture.PhysSplit,
		AllowTuningSplit: true, Policy: annotation.DefaultReferencePolicy()}
}

func physArm(f *evalfixture.PhysicalFixture, label, params string) ArmSpec {
	return ArmSpec{Label: label, DBPath: f.DBPath, ParamHash: params, Stage: "online", DeclaredBaseline: true, SolidBodies: true}
}

func scorePhys(t *testing.T, f *evalfixture.PhysicalFixture, opts PhysicalOptions, params string) PhysicalResult {
	t.Helper()
	pr, err := LoadPhysicalReference(physRefOpts(f), opts)
	if err != nil {
		t.Fatal(err)
	}
	arm, err := LoadPhysicalArm(physArm(f, "arm", params))
	if err != nil {
		t.Fatal(err)
	}
	res, err := ScorePhysical(pr, arm)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func instantOf(t *testing.T, r PhysicalResult, object string, sample int) PhysicalInstant {
	t.Helper()
	in, ok := r.Instant(evalfixture.PhysEpisode, object, sample)
	if !ok {
		t.Fatalf("no instant for %s at %d", object, sample)
	}
	return in
}

func wantOutcome(t *testing.T, in PhysicalInstant, c PhysicalComponent, category, reason string) {
	t.Helper()
	if got := in.Outcomes[c]; got.Category != category || got.Reason != reason {
		t.Fatalf("%s at sample %d, %s: %s/%s, want %s/%s", in.ObjectID, in.SampleID, c, got.Category, got.Reason, category, reason)
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-5 }

// checkComplete holds every component to complete accounting: each expected
// instant is scored or counted once, by category and reason.
func checkComplete(t *testing.T, r PhysicalResult) {
	t.Helper()
	for _, c := range PhysicalComponents() {
		a := r.Accounting.Components[c]
		if a.Expected != r.Reference.ExpectedInstants || !a.Complete() {
			t.Fatalf("%s accounting incomplete: %+v of %d", c, a, r.Reference.ExpectedInstants)
		}
	}
	if f := r.Accounting.Following; f.Expected != len(r.Following) || !f.Complete() {
		t.Fatalf("following accounting incomplete: %+v", f)
	}
}

// Completion evidence for P2: a saved reference produces the expected centre,
// axis and endpoint comparison against a named estimate version; missing
// geometry is unavailable and every expected instant is accounted for.
func TestPhysicalScoringOfAStraightPass(t *testing.T) {
	f := physFixture(t)
	r := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	checkComplete(t, r)
	if r.Schema != PhysicalScoreSchema || r.Reference.ExpectedInstants != 20 || r.Arm.ParamHash != evalfixture.ParamsExact ||
		r.Arm.Table != solidBodyTable || r.Reference.PhysicalRevision != 1 || r.Reference.Digest == "" {
		t.Fatalf("identity: %+v %+v", r.Reference, r.Arm)
	}

	// The body centre, yaw, bumpers and gap of an exact estimate are exact.
	in := instantOf(t, r, evalfixture.Follower, 0)
	for _, c := range []PhysicalComponent{ComponentCentre, ComponentYaw, ComponentLength, ComponentWidth,
		ComponentFront, ComponentRear, ComponentEnds, ComponentBox} {
		wantOutcome(t, in, c, OutcomeScored, "")
	}
	cmp := in.Comparison
	if !approx(cmp.Centre.ErrorM, 0) || cmp.Centre.LongitudinalM == nil || !approx(cmp.Yaw.ErrorRad, 0) || cmp.Yaw.AxisOnly ||
		!approx(cmp.Front.DistanceM, 0) || !approx(cmp.Rear.DistanceM, 0) || !approx(cmp.Box.IoU, 1) || !approx(cmp.Length.ErrorM, 0) {
		t.Fatalf("exact comparison at sample 0: %+v", cmp)
	}
	if in.Estimate != "source/v1/physical-fixture/cv_kf_v1/near_edge_candidate_v1/params/exact/online" || in.ReferenceRevision != 1 {
		t.Fatalf("instant does not name its version: %q revision %d", in.Estimate, in.ReferenceRevision)
	}
	// The reference layer and the prediction layer are separate records.
	if in.Reference == nil || in.Prediction == nil || in.Reference.Centre.XM != evalfixture.FollowerX(0) ||
		in.Prediction.TrackKey != "seq-000001" || in.Match.TrackKey != "seq-000001" {
		t.Fatalf("layers: %+v / %+v", in.Reference, in.Prediction)
	}

	// A front-face anchor with its offset gives the same centre.
	in = instantOf(t, r, evalfixture.Follower, 2)
	wantOutcome(t, in, ComponentCentre, OutcomeScored, "")
	if !approx(in.Comparison.Centre.ErrorM, 0) || !approx(in.Comparison.Front.DistanceM, 0) {
		t.Fatalf("front-face keyframe: %+v", in.Comparison)
	}

	// Frames between keyframes are counted, never filled in.
	for _, s := range []int{1, 4, 6, 9} {
		in := instantOf(t, r, evalfixture.Follower, s)
		if in.Reference != nil || in.Prediction != nil {
			t.Fatalf("sample %d between keyframes acquired a layer", s)
		}
		wantOutcome(t, in, ComponentCentre, OutcomeUnknownGeometry, ReasonNoKeyframe)
	}
	if n := r.Accounting.Components[ComponentCentre].Unscored[OutcomeUnknownGeometry][ReasonNoKeyframe]; n != 11 {
		t.Fatalf("frames without a keyframe = %d, want 11", n)
	}

	// Following: the reviewed gap at three instants, a no-leader decision
	// counted as an answer, and the rest counted as unreferenced.
	fa := r.Accounting.Following
	if fa.Scored != 2 || fa.Unscored[OutcomeNotFollowing]["no_leader"] != 10 || fa.Unscored[OutcomeUnknownGeometry][ReasonNoGapReference] != 7 ||
		fa.Unscored[OutcomeUnknownGeometry]["unknown"] != 1 {
		t.Fatalf("following accounting: %+v", fa)
	}
	for _, g := range r.Following {
		if g.Outcome.Category == OutcomeScored && (!approx(*g.ErrorM, 0) || !approx(g.PredictedGap.ValueM, evalfixture.TrueGapM) || *g.OutsideBoundM != 0) {
			t.Fatalf("gap at %d: %+v", g.SampleID, g.PredictedGap)
		}
	}
	s := r.Summary
	if s.Components[ComponentCentre].Scored != 6 || s.Components[ComponentCentre].WithinReferenceBound != 6 ||
		s.BoxScored != 6 || !approx(s.MeanBoxIoU, 1) || s.Following.Scored != 2 || s.CentreByPredictionReference["body_centre"].Scored != 6 {
		t.Fatalf("summary: %+v", s)
	}
}

// Acceptance: unresolved axis, partial and sparse support, occlusion and a
// tracker-assisted proposal each produce the documented unavailability.
func TestPhysicalScoringOfUnresolvedPartialAndOccludedInstants(t *testing.T) {
	f := physFixture(t)
	r := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)

	// An unresolved axis: an axis error and unsigned ends, no signed bumper.
	in := instantOf(t, r, evalfixture.Follower, 5)
	wantOutcome(t, in, ComponentYaw, OutcomeScored, "")
	wantOutcome(t, in, ComponentFront, OutcomeUnknownGeometry, annotation.UnavailableAxisAmbiguous)
	wantOutcome(t, in, ComponentRear, OutcomeUnknownGeometry, annotation.UnavailableAxisAmbiguous)
	wantOutcome(t, in, ComponentEnds, OutcomeScored, "")
	if !in.Comparison.Yaw.AxisOnly || in.Comparison.Centre.LongitudinalM != nil || in.Comparison.Centre.AlongAxisAbsM == nil ||
		!approx(in.Comparison.Ends.MeanDistanceM, 0) {
		t.Fatalf("ambiguous-axis comparison: %+v", in.Comparison)
	}
	// The follower's front has no name there, so its gap is unknown, not
	// measured along the prediction's own heading.
	for _, g := range r.Following {
		if g.FollowingID == "follow-lead" && g.SampleID == 5 &&
			(g.Outcome != Outcome{Category: OutcomeUnknownGeometry, Reason: "unknown"} || g.PredictedGap != nil) {
			t.Fatalf("gap at the unresolved axis: %+v", g)
		}
	}

	// Partial support: a rear face with no offset scores its bumper and
	// nothing that needs the centre; a partial height is only a lower bound.
	in = instantOf(t, r, evalfixture.Follower, 7)
	wantOutcome(t, in, ComponentCentre, OutcomeUnknownGeometry, annotation.UnavailableAnchorOffsetUnknown)
	wantOutcome(t, in, ComponentRear, OutcomeScored, "")
	wantOutcome(t, in, ComponentBox, OutcomeUnknownGeometry, annotation.UnavailableIncompleteBox)
	wantOutcome(t, in, ComponentHeight, OutcomeUnknownGeometry, annotation.UnavailableLowerBoundOnly)
	if lb := in.Comparison.LowerBounds[ComponentHeight]; lb.LowerM != 1.1 || !approx(lb.PredictedM, 1.5) || lb.ShortfallM != 0 {
		t.Fatalf("lower-bound check: %+v", lb)
	}

	// Occluded: the keyframe stands, the estimate has nothing there.
	in = instantOf(t, r, evalfixture.Follower, evalfixture.OccludedFollowerSample)
	if in.Reference == nil || in.Prediction != nil {
		t.Fatalf("occluded instant layers: %+v %+v", in.Reference, in.Prediction)
	}
	wantOutcome(t, in, ComponentCentre, OutcomeUnmatched, ReasonNoPredictionWithinGate)

	// A tracker-assisted proposal is shown and never scored.
	in = instantOf(t, r, evalfixture.Follower, 8)
	if in.Reference == nil || in.Reference.Truth || in.Prediction != nil {
		t.Fatalf("proposal instant: %+v", in)
	}
	wantOutcome(t, in, ComponentCentre, OutcomeUnknownGeometry, ReasonReferenceTrackerAssisted)
}

// Acceptance: a straight pass with changing visible faces. An estimate that
// leans towards the visible face shows a step at the face transition that
// the reference does not, and its bumpers and gap carry the lean.
func TestPhysicalScoringOfChangingVisibleFaces(t *testing.T) {
	f := physFixture(t)
	r := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsFaceBias)
	checkComplete(t, r)
	in := instantOf(t, r, evalfixture.Follower, 0)
	if c := in.Comparison.Centre; !approx(c.ErrorM, evalfixture.FaceBiasM) || !approx(*c.LongitudinalM, evalfixture.FaceBiasM) || !approx(*c.LateralM, 0) {
		t.Fatalf("front-lean at sample 0: %+v", c)
	}
	in = instantOf(t, r, evalfixture.Follower, 2)
	if st := in.Comparison.Centre.Step; st == nil || st.FromSampleID != 0 || !approx(st.ErrorM, 0) || !st.SameTrack {
		t.Fatalf("true motion 0 to 2: %+v", st)
	}
	in = instantOf(t, r, evalfixture.Follower, 5)
	st := in.Comparison.Centre.Step
	if st == nil || st.FromSampleID != 2 || !approx(st.ReferenceMoveM, 3) || !approx(st.PredictionMoveM, 3-evalfixture.FaceBiasM) ||
		!approx(st.ErrorM, evalfixture.FaceBiasM) {
		t.Fatalf("apparent step at the face transition: %+v", st)
	}
	// The rear-face keyframe is matched to the new track, which leans back.
	in = instantOf(t, r, evalfixture.Follower, 7)
	if in.Match.TrackKey != "seq-000003" || !approx(in.Comparison.Rear.LongitudinalErrorM, -evalfixture.FaceBiasM) {
		t.Fatalf("rear after the transition: %+v %+v", in.Match, in.Comparison.Rear)
	}
	for _, g := range r.Following {
		if g.FollowingID == "follow-lead" && g.SampleID == 0 {
			if !approx(*g.ErrorM, -evalfixture.FaceBiasM) || !approx(*g.OutsideBoundM, -0.1) {
				t.Fatalf("gap with a leaning follower: %+v", g)
			}
		}
	}
	if s := r.Summary.Components[ComponentCentre]; !approx(s.MaxAbsError, evalfixture.FaceBiasM) {
		t.Fatalf("centre summary: %+v", s)
	}
}

// A medoid is not a body centre: its centre is a missing prediction, its
// distance from the body centre is reported by prediction reference only, and
// nothing that needs a place on the body is scored from it.
func TestPhysicalScoringOfANonPhysicalPoint(t *testing.T) {
	f := physFixture(t)
	r := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsMedoid)
	checkComplete(t, r)
	in := instantOf(t, r, evalfixture.Follower, 0)
	if c := in.Comparison.Centre; c.PredictionReference != "cluster_medoid" || !approx(c.ErrorM, evalfixture.MedoidOffsetM) {
		t.Fatalf("medoid centre: %+v", c)
	}
	wantOutcome(t, in, ComponentCentre, OutcomeMissingPrediction, ReasonPredictionNotOnBody)
	wantOutcome(t, in, ComponentFront, OutcomeMissingPrediction, ReasonPredictionNotOnBody)
	wantOutcome(t, in, ComponentEnds, OutcomeMissingPrediction, ReasonPredictionNotOnBody)
	if s := r.Summary.Components[ComponentCentre]; s.Scored != 0 {
		t.Fatalf("medoids entered the body-centre score: %+v", s)
	}
	wantOutcome(t, in, ComponentBox, OutcomeMissingPrediction, ReasonPredictionIncompleteBox)
	wantOutcome(t, in, ComponentLength, OutcomeScored, "")
	if !approx(in.Comparison.Length.ErrorM, 4.0-evalfixture.FollowerLength) || !approx(in.Comparison.Length.OutsideBoundM, 4.0-4.5) {
		t.Fatalf("medoid length: %+v", in.Comparison.Length)
	}
	if r.Accounting.Following.Unscored[OutcomeMissingPrediction]["follower_insufficient_observation"] != 2 {
		t.Fatalf("gaps from medoids: %+v", r.Accounting.Following)
	}
	if s := r.Summary.CentreByPredictionReference["cluster_medoid"]; s.Scored != 6 || !approx(s.MeanAbsError, evalfixture.MedoidOffsetM) {
		t.Fatalf("centre by prediction reference: %+v", r.Summary.CentreByPredictionReference)
	}
	found := false
	for _, c := range r.Caveats {
		found = found || strings.Contains(c, "6 at cluster_medoid")
	}
	if !found {
		t.Fatalf("caveats do not say the points were medoids: %v", r.Caveats)
	}
}

// Point estimates carry a visible-box point and no body: nothing is scored,
// and the point's distance from the body centre is kept by prediction
// reference.
func TestPhysicalScoringOfPointEstimates(t *testing.T) {
	f := physFixture(t)
	pr, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	arm, err := LoadPhysicalArm(ArmSpec{Label: "points", DBPath: f.DBPath, ParamHash: evalfixture.ParamsExact})
	if err != nil {
		t.Fatal(err)
	}
	r, err := ScorePhysical(pr, arm)
	if err != nil {
		t.Fatal(err)
	}
	checkComplete(t, r)
	in := instantOf(t, r, evalfixture.Follower, 0)
	wantOutcome(t, in, ComponentCentre, OutcomeMissingPrediction, ReasonPredictionNotOnBody)
	if in.Comparison.Centre == nil || r.Summary.Components[ComponentCentre].Scored != 0 ||
		r.Summary.CentreByPredictionReference["visible_obb_centre"].Scored == 0 {
		t.Fatalf("visible-box centres: %+v %+v", in.Comparison.Centre, r.Summary)
	}
	wantOutcome(t, in, ComponentYaw, OutcomeMissingPrediction, ReasonPredictionNoHeading)
	wantOutcome(t, in, ComponentLength, OutcomeMissingPrediction, ReasonPredictionNoExtent)
	wantOutcome(t, in, ComponentBox, OutcomeMissingPrediction, ReasonPredictionIncompleteBox)
	if in.Prediction.Reference != "visible_obb_centre" || r.Arm.Stage != StageFinal || r.Arm.Table != "" {
		t.Fatalf("point arm: %+v %+v", in.Prediction, r.Arm)
	}
	for _, c := range r.Caveats {
		if strings.Contains(c, "declared baseline") {
			t.Fatalf("a final arm was called a baseline: %v", r.Caveats)
		}
	}
}

// Acceptance: source and version changes. An estimate under another
// calibration, or from another sensor, is refused; a reference revised after
// scoring is a new revision with a new result, and the old one reproduces.
func TestPhysicalSourceAndRevisionChanges(t *testing.T) {
	f := physFixture(t)
	pr, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	other, err := LoadPhysicalArm(physArm(f, "other", evalfixture.ParamsOtherCalibration))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ScorePhysical(pr, other); err == nil || !strings.Contains(err.Error(), "calibration") {
		t.Fatalf("another calibration was scored: %v", err)
	}
	exact, err := LoadPhysicalArm(physArm(f, "arm", evalfixture.ParamsExact))
	if err != nil {
		t.Fatal(err)
	}
	moved := *pr
	moved.Source.SensorID = "another-sensor"
	if _, err := ScorePhysical(&moved, exact); err == nil || !strings.Contains(err.Error(), "sensor") {
		t.Fatalf("another sensor was scored: %v", err)
	}
	unpinned := *pr
	unpinned.Source.CalibrationID = ""
	if _, err := ScorePhysical(&unpinned, other); err != nil {
		t.Fatalf("a reference that names no calibration refused one: %v", err)
	}

	first, err := ScorePhysical(pr, exact)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	// Reload: the same inputs give the same bytes.
	again := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	if againJSON, _ := json.Marshal(again); !bytes.Equal(firstJSON, againJSON) {
		t.Fatal("a reload scored differently")
	}

	// Revise the follower's first keyframe after scoring.
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := annotation.LoadPhysicalReferences(pack)
	if err != nil {
		t.Fatal(err)
	}
	for i := range doc.Objects {
		if doc.Objects[i].ObjectID == evalfixture.Follower {
			*doc.Objects[i].Keyframes[0].Position.XM += 0.1
		}
	}
	doc.Change = annotation.Provenance{Author: "fixture", Operation: "revise"}
	if err := annotation.SavePhysicalReferences(pack, doc); err != nil {
		t.Fatal(err)
	}
	revised := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	if revised.Reference.PhysicalRevision != 2 || revised.Reference.PhysicalContentDigest == first.Reference.PhysicalContentDigest ||
		revised.Reference.Digest == first.Reference.Digest {
		t.Fatalf("revision after scoring: %+v", revised.Reference)
	}
	if in := instantOf(t, revised, evalfixture.Follower, 0); !approx(in.Comparison.Centre.ErrorM, 0.1) || in.ReferenceRevision != 2 {
		t.Fatalf("revised centre: %+v", in.Comparison.Centre)
	}
	pinned := DefaultPhysicalOptions()
	pinned.Revision = 1
	old := scorePhys(t, f, pinned, evalfixture.ParamsExact)
	if oldJSON, _ := json.Marshal(old); !bytes.Equal(firstJSON, oldJSON) {
		t.Fatal("the pinned revision no longer reproduces its result")
	}
}

// Acceptance: a contaminated mask moves the mask position the identity
// comparison uses; it does not move the physical reference, which never
// reads a mask.
func TestPhysicalScoringIgnoresAContaminatedMask(t *testing.T) {
	f := physFixture(t)
	ref, err := LoadReference(physRefOpts(f))
	if err != nil {
		t.Fatal(err)
	}
	var maskX float64
	for _, s := range ref.Episodes[0].Series {
		if s.ID == evalfixture.Follower {
			maskX = float64(s.Points[0].X)
		}
	}
	truth := evalfixture.FollowerX(evalfixture.ContaminatedSample)
	if math.Abs(maskX-truth) < 1 {
		t.Fatalf("the fixture's stray return did not move the mask position (%v)", maskX)
	}
	r := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	if in := instantOf(t, r, evalfixture.Follower, evalfixture.ContaminatedSample); !approx(in.Comparison.Centre.ErrorM, 0) {
		t.Fatalf("the contaminated mask reached the physical score: %+v", in.Comparison.Centre)
	}
}

func TestPhysicalReferenceRefusals(t *testing.T) {
	f := physFixture(t)
	if _, err := LoadPhysicalReference(physRefOpts(f), PhysicalOptions{GateMetres: 0}); err == nil {
		t.Fatal("a zero gate was accepted")
	}
	for _, o := range []PhysicalOptions{{Revision: -1, GateMetres: 1}, {GateMetres: math.Inf(1)}, {GateMetres: 1, FrameToleranceNanos: -1}} {
		if err := o.Validate(); err == nil {
			t.Fatalf("options %+v validated", o)
		}
	}
	opts := physRefOpts(f)
	opts.Split = ""
	if _, err := LoadPhysicalReference(opts, DefaultPhysicalOptions()); err == nil {
		t.Fatal("no split named")
	}
	opts = physRefOpts(f)
	opts.PackDir += ".missing"
	if _, err := LoadPhysicalReference(opts, DefaultPhysicalOptions()); err == nil {
		t.Fatal("a missing pack loaded")
	}
	opts = physRefOpts(f)
	opts.SplitManifestPath += ".missing"
	if _, err := LoadPhysicalReference(opts, DefaultPhysicalOptions()); err == nil {
		t.Fatal("a missing manifest loaded")
	}
	// Held-out scoring is asked for without -allow-tuning-split.
	opts = physRefOpts(f)
	opts.AllowTuningSplit = false
	if _, err := LoadPhysicalReference(opts, DefaultPhysicalOptions()); !errors.Is(err, annotation.ErrNotHeldOut) {
		t.Fatalf("a tuning split scored as held out: %v", err)
	}
	pinned := DefaultPhysicalOptions()
	pinned.Revision = 7
	if _, err := LoadPhysicalReference(physRefOpts(f), pinned); err == nil {
		t.Fatal("a missing revision loaded")
	}

	// A held-out split is refused outright.
	m := f.Manifest()
	m.Splits[0].Role = annotation.SplitRoleHeldOut
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions()); !errors.Is(err, ErrPhysicalHeldOut) {
		t.Fatalf("a held-out split was physically scored: %v", err)
	}
	// A pinned annotation revision that does not exist, and one that does
	// but no longer holds the objects the manifest lists.
	m = f.Manifest()
	m.SidecarRevision = 9
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions()); err == nil {
		t.Fatal("a missing annotation revision loaded")
	}
	m.SidecarRevision = 0
	m.Splits[0].ObjectIDs = append(m.Splits[0].ObjectIDs, "obj_ghost")
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions()); err == nil {
		t.Fatal("a manifest listing an unknown object loaded")
	}
	m = f.Manifest()
	m.SidecarRevision = 0
	m.Episodes[0].ObjectIDs = []string{"x"}
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions()); err == nil {
		t.Fatal("an invalid manifest loaded")
	}
}

func TestPhysicalReferenceNeedsReferences(t *testing.T) {
	f, err := evalfixture.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	opts := ReferenceOptions{PackDir: f.PackDir, SplitManifestPath: f.SplitManifestPath, Split: evalfixture.SplitTuning, AllowTuningSplit: true}
	if _, err := LoadPhysicalReference(opts, DefaultPhysicalOptions()); err == nil || !strings.Contains(err.Error(), "no physical references") {
		t.Fatalf("a pack without physical references: %v", err)
	}
}

// The physical references are checked against the annotation revision the
// split pins: an object that revision rejects cannot be scored.
func TestPhysicalReferenceRefusesStaleLinks(t *testing.T) {
	f := physFixture(t)
	pack, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := annotation.LoadSidecar(pack)
	if err != nil {
		t.Fatal(err)
	}
	s.Objects = append(s.Objects, annotation.Object{ObjectID: "obj_other", Class: "car", Status: annotation.StatusReviewed})
	s.Change.Operation = "add"
	if err := annotation.SaveSidecar(pack, s); err != nil {
		t.Fatal(err)
	}
	doc, err := annotation.LoadPhysicalReferences(pack)
	if err != nil {
		t.Fatal(err)
	}
	doc.Objects = append(doc.Objects, annotation.PhysicalObject{ObjectID: "obj_other", Keyframes: []annotation.PhysicalKeyframe{}})
	if err := annotation.SavePhysicalReferences(pack, doc); err != nil {
		t.Fatal(err)
	}
	// The manifest pins revision 1, which predates obj_other.
	if _, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions()); err == nil || !strings.Contains(err.Error(), "obj_other") {
		t.Fatalf("references to an object the pinned annotation lacks: %v", err)
	}
}

func TestLoadPhysicalArmRefusals(t *testing.T) {
	f := physFixture(t)
	for name, spec := range map[string]ArmSpec{
		"analysis run":       {Label: "r", DBPath: f.DBPath, RunID: "run"},
		"no label":           {DBPath: f.DBPath},
		"no database":        {Label: "x"},
		"undeclared online":  {Label: "x", DBPath: f.DBPath, Stage: "online", SolidBodies: true},
		"missing database":   {Label: "x", DBPath: f.DBPath + ".missing", Stage: "online", DeclaredBaseline: true},
		"ambiguous version":  {Label: "x", DBPath: f.DBPath, Stage: "online", DeclaredBaseline: true, SolidBodies: true},
		"no such version":    {Label: "x", DBPath: f.DBPath, ParamHash: "params/none"},
		"estimates at stage": {Label: "x", DBPath: f.DBPath, Stage: "online", DeclaredBaseline: true},
	} {
		if _, err := LoadPhysicalArm(spec); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	// Estimates whose observations were never stored have lost their
	// evidence, and their sensor with it.
	old, err := evalfixture.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPhysicalArm(ArmSpec{Label: "x", DBPath: old.DBPath, ParamHash: evalfixture.ParamsA}); err == nil ||
		!strings.Contains(err.Error(), "not stored") {
		t.Fatalf("estimates without observations: %v", err)
	}
}
