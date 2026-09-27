package l8behaviour

import (
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func fixturePairing() PairingParams { return EncounterScenarioParams().Pairing }

// presencesAt returns the follower's presence and every other track's at an
// instant, as the analysis builds them.
func presencesAt(t *testing.T, trs []Trajectory, followerID string, capture int64) (Presence, []Presence) {
	t.Helper()
	var follower Presence
	var others []Presence
	found := false
	for _, tr := range trs {
		p, ok := PresenceAt(tr, capture)
		if !ok {
			continue
		}
		if tr.Passage.TrackID == followerID {
			follower, found = p, true
			continue
		}
		others = append(others, p)
	}
	if !found {
		t.Fatalf("follower %s absent at %d", followerID, capture)
	}
	return follower, others
}

func allEstablished(trs []Trajectory) EstablishedTracks {
	e := EstablishedTracks{}
	for _, tr := range trs {
		e[tr.Passage.TrackID] = true
	}
	return e
}

func dispositions(d PairingDecision) map[string]CandidateDisposition {
	out := map[string]CandidateDisposition{}
	for _, c := range d.Candidates {
		out[c.TrackID] = c.Disposition
	}
	return out
}

// TestDecideLeaderReproducesFixturePairings: the pairing expectations frozen
// with the analytic fixtures, before any leader choice existed.
func TestDecideLeaderReproducesFixturePairings(t *testing.T) {
	wantDispositions := map[string]map[string]CandidateDisposition{
		"ambiguous_leader": {
			"trk_fixture_ambiguous_a": DispositionCompeting, "trk_fixture_ambiguous_b": DispositionCompeting,
		},
		"lane_adjacent_distractor": {
			"trk_fixture_distractor_leader": DispositionLeader, "trk_fixture_distractor_adjacent": DispositionOutsideCorridor,
		},
	}
	pinned := 0
	for _, f := range Fixtures() {
		for _, want := range f.Pairing {
			pinned++
			follower, others := presencesAt(t, f.Trajectories, want.FollowerTrackID, want.CaptureUnixNanos)
			d, err := DecideLeader(f.Path, allEstablished(f.Trajectories), follower, others, f.Params, fixturePairing())
			if err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
			var ids []string
			for _, c := range d.Candidates {
				ids = append(ids, c.TrackID)
			}
			if d.LeaderTrackID != want.LeaderTrackID || d.Reason != want.Reason ||
				!reflect.DeepEqual(ids, sortedIDs(want.CandidateTrackIDs)) {
				t.Errorf("%s: leader %q reason %s candidates %v; want %+v", f.Name, d.LeaderTrackID, d.Reason, ids, want)
			}
			if got := dispositions(d); !reflect.DeepEqual(got, wantDispositions[f.Name]) {
				t.Errorf("%s: dispositions %v", f.Name, got)
			}
		}
	}
	if pinned != 2 {
		t.Fatalf("%d pinned pairings, want the ambiguous and distractor fixtures", pinned)
	}
}

// scene builds single-instant presences of fixture cars on the fixture's
// straight path, from (track id, x, y) triples, followed by the follower.
func scene(bodies map[string][2]float64) []Trajectory {
	ids := make([]string, 0, len(bodies))
	for id := range bodies {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var trs []Trajectory
	for _, id := range ids {
		b := fixtureCar(fixtureAt(0), bodies[id][0], bodies[id][1], 10)
		if id == "f" {
			b = b.followerBody()
		}
		trs = append(trs, fixtureTrajectory(id, b))
	}
	return trs
}

func decide(t *testing.T, path PathFrame, members PathMembership, trs []Trajectory, following FollowingParams, pairing PairingParams) PairingDecision {
	t.Helper()
	follower, others := presencesAt(t, trs, "f", fixtureAt(0))
	d, err := DecideLeader(path, members, follower, others, following, pairing)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	return d
}

func TestDecideLeaderDispositions(t *testing.T) {
	path := fixturePathX() // x 0 to 120 along y = 0
	trs := scene(map[string][2]float64{
		"f":              {20, 1.0},
		"behind":         {12, 1.0},
		"wide_of_path":   {30, 2.5},
		"wide_of_driver": {30, -0.9}, // on the path, 1.9 m from the follower
		"leader":         {35, 0.5},
		"blocked":        {50, 1.0},
		"beyond":         {110, 1.0},
		"off":            {130, 1.0},
	})
	d := decide(t, path, allEstablished(trs), trs, FixtureParams(), fixturePairing())
	want := map[string]CandidateDisposition{
		"behind": DispositionBehind, "wide_of_path": DispositionOutsideCorridor,
		"wide_of_driver": DispositionOutsideCorridor, "leader": DispositionLeader, "blocked": DispositionBlocked,
		"beyond": DispositionBeyondRange, "off": DispositionOffPath,
	}
	if got := dispositions(d); !reflect.DeepEqual(got, want) || d.LeaderTrackID != "leader" || d.Reason != ReasonUnspecified {
		t.Fatalf("decision %+v\ndispositions %v", d, got)
	}
	for _, c := range d.Candidates {
		if (c.CentreArcM == nil) != (c.TrackID == "off") || !c.Synchronised || !c.Established {
			t.Errorf("candidate %+v", c)
		}
	}
	if !reflect.DeepEqual(d.Involved(), []string{"leader"}) {
		t.Fatalf("involved %v", d.Involved())
	}

	// Nothing ahead in the corridor is free flow: neither leader nor reason.
	trs = scene(map[string][2]float64{"f": {20, 0}, "behind": {10, 0}})
	if d := decide(t, path, allEstablished(trs), trs, FixtureParams(), fixturePairing()); d.LeaderTrackID != "" ||
		d.Reason != ReasonUnspecified || d.Involved() != nil {
		t.Fatalf("free flow %+v", d)
	}
}

// TestDecideLeaderSeparabilityThreshold: a second body is blocked only when
// its rear clears the nearest's front by more than k combined sigmas. With
// fixture bodies the combined sigma is 0.25 m, so at k = 2 the threshold is
// 0.5 m: the nearest's front is at 32.25 m, so a rear at 32.74 m competes and
// one at 32.76 m is blocked.
func TestDecideLeaderSeparabilityThreshold(t *testing.T) {
	path := fixturePathX()
	for _, c := range []struct {
		rear float64
		want CandidateDisposition
	}{{32.74, DispositionCompeting}, {32.76, DispositionBlocked}} {
		trs := scene(map[string][2]float64{"f": {20, 0}, "near": {30, 0}, "far": {c.rear + 2.25, 0}})
		d := decide(t, path, allEstablished(trs), trs, FixtureParams(), fixturePairing())
		if got := dispositions(d)["far"]; got != c.want {
			t.Errorf("rear at %v: %s, want %s", c.rear, got, c.want)
		}
		if c.want == DispositionCompeting {
			if d.Reason != ReasonAmbiguousLeader || d.LeaderTrackID != "" ||
				!reflect.DeepEqual(d.Involved(), []string{"far", "near"}) {
				t.Errorf("ambiguous %+v involved %v", d, d.Involved())
			}
		} else if d.LeaderTrackID != "near" {
			t.Errorf("separable: leader %q", d.LeaderTrackID)
		}
	}
}

// TestDecideLeaderUnresolvedFallsBackToCentres: when the nearest body's
// orientation is unresolved its front cannot be projected, so separability
// falls back to centre spacing against UnresolvedSeparationM (12 m).
func TestDecideLeaderUnresolvedFallsBackToCentres(t *testing.T) {
	path := fixturePathX()
	for _, c := range []struct {
		farX float64
		want CandidateDisposition
	}{{41.9, DispositionCompeting}, {42.1, DispositionBlocked}} {
		trs := scene(map[string][2]float64{"f": {20, 0}, "near": {30, 0}, "far": {c.farX, 0}})
		for i := range trs {
			if trs[i].Passage.TrackID == "near" {
				s := &trs[i].Samples[0]
				s.Estimation, s.Heading.AmbiguousModeWeight = EstimationGeometryConverging, 0.5
			}
		}
		d := decide(t, path, allEstablished(trs), trs, FixtureParams(), fixturePairing())
		if got := dispositions(d)["far"]; got != c.want {
			t.Errorf("far at %v: %s, want %s", c.farX, got, c.want)
		}
	}
}

// testMembership states membership and non-membership conditions directly.
type testMembership struct {
	members    map[string]bool
	conditions map[string]PathCondition
}

func (m testMembership) IsMember(id string) bool { return m.members[id] }
func (m testMembership) NotEstablishedCondition(id string) PathCondition {
	if c, ok := m.conditions[id]; ok {
		return c
	}
	return PathUnestablishedBody
}

// TestDecideLeaderUnestablishedNearest: a body that is not on the path and is
// nearest in the corridor can be neither chosen nor ruled out. The instant is
// no_common_path with the body's condition, and it belongs to the encounter
// with the nearest established body behind it.
func TestDecideLeaderUnestablishedNearest(t *testing.T) {
	path := fixturePathX()
	trs := scene(map[string][2]float64{"f": {20, 0}, "oncoming": {28, 0.5}, "leader": {40, 0}, "next": {60, 0}})
	members := testMembership{
		members:    map[string]bool{"f": true, "leader": true, "next": true},
		conditions: map[string]PathCondition{"oncoming": PathDirectionReversal},
	}
	d := decide(t, path, members, trs, FixtureParams(), fixturePairing())
	want := map[string]CandidateDisposition{
		"oncoming": DispositionUnestablished, "leader": DispositionBlocked, "next": DispositionBlocked,
	}
	if got := dispositions(d); !reflect.DeepEqual(got, want) || d.Reason != ReasonNoCommonPath ||
		d.Condition != PathDirectionReversal || d.LeaderTrackID != "" {
		t.Fatalf("decision %+v dispositions %v", d, got)
	}
	if !reflect.DeepEqual(d.Involved(), []string{"leader"}) {
		t.Fatalf("involved %v, want the nearest established body", d.Involved())
	}
	if c, _ := d.Candidate("oncoming"); c.Established {
		t.Fatal("the oncoming body is not established")
	}

	// An unestablished body behind the leader changes nothing.
	trs = scene(map[string][2]float64{"f": {20, 0}, "leader": {30, 0}, "stray": {50, 0}})
	members.members = map[string]bool{"f": true, "leader": true}
	if d := decide(t, path, members, trs, FixtureParams(), fixturePairing()); d.LeaderTrackID != "leader" {
		t.Fatalf("stray beyond the leader: %+v", d)
	}
}

func TestDecideLeaderFollowerOutsideExtent(t *testing.T) {
	trs := scene(map[string][2]float64{"f": {125, 0}, "leader": {110, 0}})
	d := decide(t, fixturePathX(), allEstablished(trs), trs, FixtureParams(), fixturePairing())
	if d.Reason != ReasonNoCommonPath || d.Condition != PathOutsideExtent || d.Candidates != nil || d.Involved() != nil {
		t.Fatalf("decision %+v", d)
	}
}

// TestMissingRowDoesNotPromoteTheNextCar: a leader present on both sides of
// an instant without a row there is placed by interpolation and stays the
// leader; the car behind it is not promoted.
func TestMissingRowDoesNotPromoteTheNextCar(t *testing.T) {
	var f, near, far []bodySpec
	for k := int64(0); k < 3; k++ {
		at := fixtureAt(k)
		f = append(f, fixtureCar(at, 20+float64(k), 0, 10).followerBody())
		near = append(near, fixtureCar(at, 35+float64(k), 0, 10))
		far = append(far, fixtureCar(at, 50+float64(k), 0, 10))
	}
	trs := []Trajectory{
		fixtureTrajectory("f", f...),
		fixtureTrajectory("near", near[0], near[2]), // no row at frame 1
		fixtureTrajectory("far", far...),
	}
	follower, others := presencesAt(t, trs, "f", fixtureAt(1))
	d, err := DecideLeader(fixturePathX(), allEstablished(trs), follower, others, FixtureParams(), fixturePairing())
	if err != nil {
		t.Fatal(err)
	}
	c, _ := d.Candidate("near")
	if d.LeaderTrackID != "near" || c.Synchronised || *c.CentreArcM != 36 || dispositions(d)["far"] != DispositionBlocked {
		t.Fatalf("decision %+v", d)
	}
}

func TestPresenceAt(t *testing.T) {
	tr := fixtureTrajectory("t",
		fixtureCar(fixtureAt(0), 10, 0, 10), fixtureCar(fixtureAt(4), 14, 2, 10))
	if _, ok := PresenceAt(tr, fixtureAt(0)-1); ok {
		t.Fatal("before the first sample")
	}
	if _, ok := PresenceAt(tr, fixtureAt(4)+1); ok {
		t.Fatal("after the last sample")
	}
	p, ok := PresenceAt(tr, fixtureAt(4))
	if !ok || p.Sample == nil || p.X != 14 || p.Y != 2 {
		t.Fatalf("exact %+v", p)
	}
	p, ok = PresenceAt(tr, fixtureAt(1))
	if !ok || p.Sample != nil || p.X != 11 || p.Y != 0.5 || p.Passage.TrackID != "t" {
		t.Fatalf("between %+v", p)
	}
	// A near-face reference is placed at its body centre.
	s := fixtureCar(fixtureAt(0), 10, 0, 10)
	s.reference, s.offset = ReferenceNearFaceCentre, BodyOffset{LongitudinalM: -2}
	if p, _ := PresenceAt(fixtureTrajectory("nf", s), fixtureAt(0)); p.X != 8 || p.Y != 0 {
		t.Fatalf("near face placed at (%v, %v)", p.X, p.Y)
	}
}

func TestDecideLeaderRejectsCallerErrors(t *testing.T) {
	trs := scene(map[string][2]float64{"f": {20, 0}, "leader": {30, 0}})
	follower, others := presencesAt(t, trs, "f", fixtureAt(0))
	path, members := fixturePathX(), allEstablished(trs)
	noSample := follower
	noSample.Sample = nil
	late := others[0]
	lateSample := *late.Sample
	lateSample.CaptureUnixNanos++
	lateSample.LastObservedUnixNanos++
	late.Sample = &lateSample
	for _, c := range []struct {
		name     string
		path     PathFrame
		members  PathMembership
		follower Presence
		others   []Presence
		pairing  PairingParams
		want     string
	}{
		{"nil path", nil, members, follower, others, fixturePairing(), "path"},
		{"nil membership", path, nil, follower, others, fixturePairing(), "membership"},
		{"follower without a sample", path, members, noSample, others, fixturePairing(), "no sample"},
		{"duplicate body", path, members, follower, append(others, others[0]), fixturePairing(), "twice"},
		{"follower among others", path, members, follower, append(others, follower), fixturePairing(), "twice"},
		{"unsynchronised sample", path, members, follower, []Presence{late}, fixturePairing(), "instant"},
		{"invalid pairing", path, members, follower, others, PairingParams{}, "positive"},
	} {
		_, err := DecideLeader(c.path, c.members, c.follower, c.others, FixtureParams(), c.pairing)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want it to mention %q", c.name, err, c.want)
		}
	}
	if _, err := DecideLeader(path, members, follower, others, FollowingParams{}, fixturePairing()); err == nil {
		t.Error("invalid following params: want an error")
	}
	for _, p := range []PairingParams{{1, 2, math.Inf(1)}, {1, math.NaN(), 1}, {-1, 2, 12}} {
		if p.Validate() == nil {
			t.Errorf("%+v: want an error", p)
		}
	}
}
