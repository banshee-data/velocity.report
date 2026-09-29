package perframeeval

import (
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// reviseReferences edits the fixture's stored physical references and saves
// the edit through the store, so every change here is one the validation
// accepts.
func reviseReferences(t *testing.T, f *evalfixture.PhysicalFixture, edit func(*annotation.PhysicalReferenceSet)) {
	t.Helper()
	p, err := annotation.OpenPack(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := annotation.LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	edit(doc)
	if err := annotation.SavePhysicalReferences(p, doc); err != nil {
		t.Fatalf("save: %v", err)
	}
}

func keyframeAt(doc *annotation.PhysicalReferenceSet, object string, sample int) *annotation.PhysicalKeyframe {
	for i := range doc.Objects {
		if doc.Objects[i].ObjectID != object {
			continue
		}
		for k := range doc.Objects[i].Keyframes {
			if doc.Objects[i].Keyframes[k].SampleID == sample {
				return &doc.Objects[i].Keyframes[k]
			}
		}
	}
	return nil
}

func followingByID(doc *annotation.PhysicalReferenceSet, id string) *annotation.FollowingReference {
	for i := range doc.Following {
		if doc.Following[i].FollowingID == id {
			return &doc.Following[i]
		}
	}
	return nil
}

// A front-face keyframe whose yaw is only a prior places its front bumper,
// but a signed bumper is scored along the axis: without a scorable yaw it is
// unknown geometry, never a nil dereference.
func TestPhysicalSignedBumperNeedsAScorableYaw(t *testing.T) {
	f := physFixture(t)
	reviseReferences(t, f, func(doc *annotation.PhysicalReferenceSet) {
		k := keyframeAt(doc, evalfixture.Follower, 2)
		k.Yaw.Status, k.Yaw.Support = annotation.EvidencePriorOnly, annotation.EvidenceSupport{External: "lane direction"}
	})
	r := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	checkComplete(t, r)
	in := instantOf(t, r, evalfixture.Follower, 2)
	wantOutcome(t, in, ComponentFront, OutcomeUnknownGeometry, annotation.UnavailableYaw)
	wantOutcome(t, in, ComponentRear, OutcomeUnknownGeometry, annotation.UnavailableYaw)

	// The scorer holds to that whatever geometry it is handed.
	placed := annotation.PhysicalGeometry{Front: &annotation.PlanarBound{}}
	if got := signedEndUnavailable(placed, ""); got != annotation.UnavailableYaw {
		t.Fatalf("a placed bumper with no yaw: %q", got)
	}
	if got := signedEndUnavailable(placed, "unknown"); got != "unknown" {
		t.Fatalf("the geometry's own reason was replaced: %q", got)
	}
	if got := signedEndUnavailable(annotation.PhysicalGeometry{Yaw: &annotation.AngleBound{}}, ""); got != "" {
		t.Fatalf("a bumper with a yaw: %q", got)
	}
}

// A follower in the scored split and its leader in another: the leader's
// references are not used, neither for the gap nor in matching.
func TestPhysicalGapToALeaderInAnotherSplit(t *testing.T) {
	f := physFixture(t)
	m := f.Manifest()
	m.Splits = []annotation.Split{
		{Name: evalfixture.PhysSplit, Role: annotation.SplitRoleTuning, ObjectIDs: []string{evalfixture.Follower}},
		{Name: "held", Role: annotation.SplitRoleHeldOut, ObjectIDs: []string{evalfixture.Leader}},
	}
	m.Episodes = []annotation.Episode{
		{EpisodeID: evalfixture.PhysEpisode, Split: evalfixture.PhysSplit, ObjectIDs: []string{evalfixture.Follower},
			FrameIntervals: []annotation.FrameInterval{{FirstSample: 0, LastSample: evalfixture.PhysSamples - 1}}},
		{EpisodeID: "held-episode", Split: "held", ObjectIDs: []string{evalfixture.Leader},
			FrameIntervals: []annotation.FrameInterval{{FirstSample: 0, LastSample: evalfixture.PhysSamples - 1}}},
	}
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	r := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	checkComplete(t, r)
	for _, g := range r.Following {
		if g.Outcome.Category == OutcomeScored {
			t.Fatalf("a gap to a leader in another split was scored: %+v", g)
		}
	}
	if r.Accounting.Following.Unscored[OutcomeUnknownGeometry][ReasonLeaderOutsideEpisode] == 0 || r.Summary.Following.Scored != 0 {
		t.Fatalf("following accounting: %+v", r.Accounting.Following)
	}

	pr, err := LoadPhysicalReference(physRefOpts(f), DefaultPhysicalOptions())
	if err != nil {
		t.Fatal(err)
	}
	arm, err := LoadPhysicalArm(ArmSpec{Label: "exact", DBPath: f.DBPath, ParamHash: evalfixture.ParamsExact, Stage: "online",
		DeclaredBaseline: true, SolidBodies: true})
	if err != nil {
		t.Fatal(err)
	}
	aligned := pr.alignPredictions(arm.Bodies)
	match := pr.matchSample(0, aligned[0])
	if _, ok := match.matched[evalfixture.Leader]; ok || match.candidates[evalfixture.Leader] != 0 {
		t.Fatalf("an object in another split competed for predictions: %+v", match)
	}
	if _, ok := match.matched[evalfixture.Follower]; !ok {
		t.Fatalf("the follower was not matched: %+v", match)
	}
}

// Two reviewed records that agree about one follower, and a proposal over
// them, still give each of its instants one expected item, spoken for by the
// reviewed record with a gap there. An episode that ends before the records
// do expects nothing after it.
func TestPhysicalFollowingCountsEachFollowerInstantOnce(t *testing.T) {
	f := physFixture(t)
	m := f.Manifest()
	m.Episodes[0].FrameIntervals = []annotation.FrameInterval{{FirstSample: 0, LastSample: evalfixture.PhysSamples - 2}}
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	before := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	reviseReferences(t, f, func(doc *annotation.PhysicalReferenceSet) {
		dup := *followingByID(doc, "follow-lead")
		dup.FollowingID, dup.Gaps, dup.GapDefinition = "follow-lead-again", nil, ""
		proposal := dup
		proposal.FollowingID, proposal.Review.Status = "follow-lead-proposed", annotation.StatusProposed
		doc.Following = append(doc.Following, proposal, dup)
	})
	r := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	checkComplete(t, r)
	if r.Accounting.Following.Expected != before.Accounting.Following.Expected ||
		r.Accounting.Following.Scored != before.Accounting.Following.Scored {
		t.Fatalf("following accounting %+v, before %+v", r.Accounting.Following, before.Accounting.Following)
	}
	type key struct {
		follower string
		sample   int
	}
	seen := map[key]bool{}
	for _, g := range r.Following {
		k := key{g.FollowerObjectID, g.SampleID}
		if seen[k] {
			t.Fatalf("follower %s at sample %d counted twice", g.FollowerObjectID, g.SampleID)
		}
		seen[k] = true
		if g.FollowerObjectID == evalfixture.Follower && g.SampleID == 0 && g.FollowingID != "follow-lead" {
			t.Fatalf("sample 0 spoken for by %s, not the record with its gap", g.FollowingID)
		}
		if g.FollowingID == "follow-lead-proposed" || g.SampleID == evalfixture.PhysSamples-1 {
			t.Fatalf("a proposal or a sample outside the episode was expected: %+v", g)
		}
	}
}

// A gap that the leader's references cannot place, because the leader has no
// keyframe there, is unknown geometry, not an unmatched prediction.
func TestPhysicalGapWithoutALeaderKeyframe(t *testing.T) {
	f := physFixture(t)
	reviseReferences(t, f, func(doc *annotation.PhysicalReferenceSet) {
		fl := followingByID(doc, "follow-lead")
		external := annotation.EvidenceSupport{External: "timed from the video"}
		keyframeAt(doc, evalfixture.Follower, 7).Front = annotation.EndpointEvidence{Status: annotation.EvidenceInferred, Support: external}
		lo, hi := 5.0, 6.0
		fl.Gaps = append(fl.Gaps, annotation.FollowingGap{
			SampleID: 7, TimestampNs: evalfixture.SampleTime(7), Status: annotation.EvidenceInferred, LowerM: &lo, UpperM: &hi,
			FollowerFront: annotation.EndpointEvidence{Status: annotation.EvidenceInferred, Support: external},
			LeaderRear:    annotation.EndpointEvidence{Status: annotation.EvidenceInferred, Support: external},
			Support:       external,
		})
	})
	r := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	checkComplete(t, r)
	for _, g := range r.Following {
		if g.FollowerObjectID == evalfixture.Follower && g.SampleID == 7 {
			if g.Outcome != (Outcome{Category: OutcomeUnknownGeometry, Reason: ReasonNoLeaderKeyframe}) {
				t.Fatalf("gap at 7: %+v", g.Outcome)
			}
			return
		}
	}
	t.Fatal("no following instant at sample 7")
}

// An arm whose rows were made under two calibrations cannot be scored
// against one reference; a scored arm records its calibration.
func TestPhysicalArmCalibrations(t *testing.T) {
	arm := PhysicalArm{Identity: ArmIdentity{Label: "mixed"}, SensorIDs: []string{"s"}, CalibrationIDs: []string{"a", "b"}}
	if err := checkSource(annotation.PhysicalSource{SensorID: "s"}, arm); err == nil || !strings.Contains(err.Error(), "mixes calibrations") {
		t.Fatalf("a mixed arm: %v", err)
	}
	f := physFixture(t)
	r := scorePhys(t, f, DefaultPhysicalOptions(), evalfixture.ParamsExact)
	if len(r.ArmCalibrations) != 1 || r.ArmCalibrations[0] != evalfixture.PhysCalibration {
		t.Fatalf("arm calibrations: %v", r.ArmCalibrations)
	}
}

// Accounting that files an instant under no category is not complete, and a
// gap's bound is the farther end from the quoted value, as a dimension's is.
func TestPhysicalAccountingAndGapBound(t *testing.T) {
	a := ComponentAccounting{Expected: 1, Unscored: map[string]map[string]int{"": {"x": 1}}}
	if a.Complete() {
		t.Fatal("an unnamed category counted as complete")
	}
	lo, hi, value, errM, outside := 1.0, 3.0, 1.5, 0.1, 0.0
	gap := &annotation.FollowingGap{LowerM: &lo, UpperM: &hi, ValueM: &value}
	s := summarisePhysical(PhysicalResult{Following: []PhysicalFollowingInstant{{ReferenceGap: gap, ErrorM: &errM, OutsideBoundM: &outside}}})
	if !approx(s.Following.MeanReferenceBound, 1.5) {
		t.Fatalf("gap bound %v, want 1.5", s.Following.MeanReferenceBound)
	}
	if got := referenceGapValue(annotation.FollowingGap{LowerM: &lo, UpperM: &hi}); !approx(got, 2) {
		t.Fatalf("an interval's value %v, want its middle", got)
	}
}
