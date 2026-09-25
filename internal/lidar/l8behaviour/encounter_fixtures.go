package l8behaviour

// Frozen multi-frame scenarios for the local path, leader choice and encounter
// aggregation, per Sections 8.3, 9.2 and 10.4 of
// docs/plans/lidar-behaviour-analytics-plan.md: steady following across the
// named bands, an occlusion, a standstill queue, a lane-adjacent distractor,
// an ambiguous pair that resolves, a partial view, a fork, a reversal, a
// crossing and sparse support.
//
// Every scenario runs at 10 Hz along the sensor's +x axis with the analytic
// fixtures' bodies (leader 4.5 by 2.0 m, follower 4.0 by 1.75 m, front face
// observed), so the expected values follow from the stated positions by hand:
//
//	gap = (x_L - 2.25) - (x_F + 2.0),  THW = gap / v_F
//
// and each follower instant stands for one 0.1 s frame, except the follower's
// last, which stands for nothing. Positions are quarter metres and speeds are
// exact binary fractions per frame, so gaps, band boundaries and accounting
// are exact; a boundary instant (a time gap of exactly 2.0, 1.5 or 1.0 s) is
// placed deliberately to pin strict band membership. Interval bounds come
// from a seeded Monte Carlo draw and are not frozen as literals here; the
// tests check them against their analytic limits instead.

import "math"

// EncounterScenarioParams are the bounds every scenario is analysed with,
// unless the scenario states otherwise. They are fixture values, not
// calibrated defaults: a 2 m knot, a 1.5 m grouping bound under a 1.75 m
// corridor, and a fully common-mode interval model, whose intervals have a
// closed form for a constant sigma.
func EncounterScenarioParams() FollowingAnalysisParams {
	return FollowingAnalysisParams{
		Path: LocalPathParams{
			KnotSpacingM: 2, GroupLateralM: 1.5, MaxTangentRad: 0.35, MinSpeedMps: 0.5,
			MinTrackEvidence: 5, MinOverlapKnots: 2, MinSamplesPerKnot: 1, MaxBridgeKnots: 2, MinExtentM: 10,
		},
		Following: FixtureParams(),
		Pairing:   PairingParams{MaxLeaderRangeM: 60, SeparationSigmas: 2, UnresolvedSeparationM: 12},
		Exposure: ExposureParams{
			MinOpportunitySeconds: 1, MaxIntervalNanos: FixtureFramePeriodNanos * 3 / 2,
			IntervalCoverage: 0.9, MonteCarloSamples: 2000, CommonModeFraction: 1,
		},
	}
}

// ExpectedPath is the frozen path outcome for one follower.
type ExpectedPath struct {
	FollowerTrackID string          `json:"follower_track_id"`
	MemberTrackIDs  []string        `json:"member_track_ids,omitempty"`
	Conditions      []PathCondition `json:"conditions,omitempty"`
}

// ExpectedEncounter is the frozen outcome of one encounter. A nil statistic is
// one whose series is empty, so its measurement is suppressed.
type ExpectedEncounter struct {
	LeaderTrackID   string        `json:"leader_track_id"`
	FollowerTrackID string        `json:"follower_track_id"`
	Instants        int           `json:"instants"`
	ValidNanos      int64         `json:"valid_nanos"`
	BandNanos       []int64       `json:"band_nanos"`
	UnobservedNanos int64         `json:"unobserved_nanos"`
	Suppressions    []ReasonTally `json:"suppressions,omitempty"`
	SpatialGapMin   *float64      `json:"spatial_gap_min,omitempty"`
	SpatialGapP50   *float64      `json:"spatial_gap_p50,omitempty"`
	NetTimeGapMin   *float64      `json:"net_time_gap_min,omitempty"`
	NetTimeGapP50   *float64      `json:"net_time_gap_p50,omitempty"`
	// GapSigma is the constant one-sigma of every supported gap, which makes
	// a fully common-mode interval analytic: value -/+ z * sigma.
	GapSigma       float64 `json:"gap_sigma"`
	PredictedGaps  int     `json:"predicted_gaps"`
	SpatialGapSize int     `json:"spatial_gap_size"`
	NetTimeGapSize int     `json:"net_time_gap_size"`
}

// EncounterScenario is one frozen multi-frame scenario.
type EncounterScenario struct {
	Name         string                  `json:"name"`
	Description  string                  `json:"description"`
	Params       FollowingAnalysisParams `json:"params"`
	Trajectories []Trajectory            `json:"trajectories"`
	// Paths pins the path outcome of every track in the scenario.
	Paths []ExpectedPath `json:"paths"`
	// Encounters is every encounter the scenario yields, in output order.
	Encounters []ExpectedEncounter `json:"encounters,omitempty"`
	// Decisions pins selected pairing decisions.
	Decisions []ExpectedPairing `json:"decisions,omitempty"`
}

// EncounterScenarios returns every frozen scenario, freshly built, in a
// stable order.
func EncounterScenarios() []EncounterScenario {
	return []EncounterScenario{
		ScenarioSteadyApproach(),
		ScenarioOcclusion(),
		ScenarioStandstillQueue(),
		ScenarioLaneAdjacentDistractor(),
		ScenarioAmbiguousThenResolved(),
		ScenarioPartialView(),
		ScenarioFork(),
		ScenarioReversal(),
		ScenarioCrossing(),
		ScenarioSparseSupport(),
	}
}

// frames builds n samples of one body from a per-frame spec.
func frames(n int64, spec func(k int64, t int64) bodySpec) []bodySpec {
	out := make([]bodySpec, n)
	for k := int64(0); k < n; k++ {
		out[k] = spec(k, fixtureAt(k))
	}
	return out
}

func nanosOf(frames int64) int64 { return frames * FixtureFramePeriodNanos }

// alignedGapSigma is every aligned fixture pair's gap sigma: Section 9.1 with
// position variance 1/64 m^2 and length sigma 0.25 m on each body.
const alignedGapSigma = 0.25

// ScenarioSteadyApproach is a follower at 10 m/s closing on a leader at
// 7.5 m/s for 50 frames. The gap falls from 20 m by 0.25 m a frame, so the
// time gap falls from exactly 2.0 s by 0.025 s a frame and crosses every band:
// frame 0 sits exactly on the 2.0 s band, frame 20 on 1.5 s and frame 40 on
// 1.0 s, and none of them is below its band.
//
//	valid     frames 0-48          4.9 s
//	< 2.0 s   frames 1-48          4.8 s
//	< 1.5 s   frames 21-48         2.8 s
//	< 1.0 s   frames 41-48         0.8 s
//	gap       min 7.75 m (frame 49), median of 7.75 + 0.25 j at j = 24, 25: 13.875 m
//	THW       min 0.775 s, median 1.3875 s
func ScenarioSteadyApproach() EncounterScenario {
	const fID, lID = "trk_s_steady_follower", "trk_s_steady_leader"
	follower := frames(50, func(k, t int64) bodySpec { return fixtureCar(t, 20+float64(k), 0, 10).followerBody() })
	leader := frames(50, func(k, t int64) bodySpec { return fixtureCar(t, 44.25+0.75*float64(k), 0, 7.5) })
	return EncounterScenario{
		Name:        "steady_approach",
		Description: "A closing pair whose time gap crosses 2.0, 1.5 and 1.0 s, landing exactly on each band once.",
		Params:      EncounterScenarioParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower...),
			fixtureTrajectory(lID, leader...),
		},
		Paths: []ExpectedPath{
			{FollowerTrackID: fID, MemberTrackIDs: []string{fID, lID}},
			{FollowerTrackID: lID, MemberTrackIDs: []string{fID, lID}},
		},
		Encounters: []ExpectedEncounter{{
			LeaderTrackID: lID, FollowerTrackID: fID, Instants: 50,
			ValidNanos: nanosOf(49), BandNanos: []int64{nanosOf(48), nanosOf(28), nanosOf(8)},
			SpatialGapMin: ptr(7.75), SpatialGapP50: ptr(13.875),
			NetTimeGapMin: ptr(0.775), NetTimeGapP50: ptr(1.3875),
			GapSigma: alignedGapSigma, SpatialGapSize: 50, NetTimeGapSize: 50,
		}},
		Decisions: []ExpectedPairing{{
			FollowerTrackID: fID, CaptureUnixNanos: fixtureAt(0),
			CandidateTrackIDs: []string{lID}, LeaderTrackID: lID,
		}},
	}
}

// ScenarioOcclusion is a pair at 10 m/s, 15.75 m apart (1.575 s), for 40
// frames, with the follower hidden for frames 15 to 19: established while it
// coasts for three frames, temporarily degraded for two. The coasted frames
// are not evidence, so the path has no evidence at x 36 to 40 and bridges
// those two knots. They carry review-only predicted gaps, are counted as
// unobserved time and by reason, and never enter valid following time.
//
//	valid            frames 0-14 and 20-38     3.4 s
//	not_observed     frames 15-17              0.3 s
//	model_degraded   frames 18-19              0.2 s
func ScenarioOcclusion() EncounterScenario {
	const fID, lID = "trk_s_occlusion_follower", "trk_s_occlusion_leader"
	follower := frames(40, func(k, t int64) bodySpec {
		b := fixtureCar(t, 20+float64(k), 0, 10).followerBody()
		if k >= 15 && k <= 19 {
			b.support, b.faces, b.lastObserved = SupportCoasted, FaceVisibility{}, fixtureAt(14)
			b.posVar = [4]float64{0.015625 * float64(k-13), 0, 0, 0.015625 * float64(k-13)}
			if k >= 18 {
				b.estimation = EstimationTemporarilyDegraded
			}
		}
		return b
	})
	leader := frames(40, func(k, t int64) bodySpec { return fixtureCar(t, 40+float64(k), 0, 10) })
	return EncounterScenario{
		Name:        "occlusion",
		Description: "Follower hidden for five frames: coasted time is review-only and excluded from valid following time.",
		Params:      EncounterScenarioParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower...),
			fixtureTrajectory(lID, leader...),
		},
		Paths: []ExpectedPath{
			{FollowerTrackID: fID, MemberTrackIDs: []string{fID, lID}},
			{FollowerTrackID: lID, MemberTrackIDs: []string{fID, lID}},
		},
		Encounters: []ExpectedEncounter{{
			LeaderTrackID: lID, FollowerTrackID: fID, Instants: 40,
			ValidNanos: nanosOf(34), BandNanos: []int64{nanosOf(34), 0, 0}, UnobservedNanos: nanosOf(5),
			Suppressions: []ReasonTally{
				{Reason: ReasonModelDegraded, Instants: 2, Nanos: nanosOf(2)},
				{Reason: ReasonNotObserved, Instants: 3, Nanos: nanosOf(3)},
			},
			SpatialGapMin: ptr(15.75), SpatialGapP50: ptr(15.75),
			NetTimeGapMin: ptr(1.575), NetTimeGapP50: ptr(1.575),
			GapSigma: alignedGapSigma, PredictedGaps: 5, SpatialGapSize: 35, NetTimeGapSize: 35,
		}},
	}
}

// standstillOffset is the queue's distance travelled by frame k: 0.5 m a
// frame to frame 10, stopped from frame 10 to 19, then 0.5 m a frame again.
func standstillOffset(k int64) (offset, speed float64) {
	switch {
	case k < 10:
		return 0.5 * float64(k), 5
	case k <= 19:
		return 5, 0
	}
	return 5 + 0.5*float64(k-19), 5
}

// ScenarioStandstillQueue is a queue at 5 m/s, 6 m apart (1.2 s), that stops
// for frames 10 to 19 and moves off again, over 50 frames. The spatial gap
// stays supported throughout; the time gap is suppressed below the speed
// floor while stopped, which is constrained time, not following opportunity.
// Stopped samples name no direction and are not path evidence.
//
//	valid               frames 0-9 and 20-48     3.9 s
//	below_speed_floor   frames 10-19             1.0 s
func ScenarioStandstillQueue() EncounterScenario {
	const fID, lID = "trk_s_queue_follower", "trk_s_queue_leader"
	follower := frames(50, func(k, t int64) bodySpec {
		o, v := standstillOffset(k)
		return fixtureCar(t, 20+o, 0, v).followerBody()
	})
	leader := frames(50, func(k, t int64) bodySpec {
		o, v := standstillOffset(k)
		return fixtureCar(t, 30.25+o, 0, v)
	})
	return EncounterScenario{
		Name:        "standstill_queue",
		Description: "A queue that stops and moves off: spatial gap throughout, time gap only while moving.",
		Params:      EncounterScenarioParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower...),
			fixtureTrajectory(lID, leader...),
		},
		Paths: []ExpectedPath{
			{FollowerTrackID: fID, MemberTrackIDs: []string{fID, lID}},
			{FollowerTrackID: lID, MemberTrackIDs: []string{fID, lID}},
		},
		Encounters: []ExpectedEncounter{{
			LeaderTrackID: lID, FollowerTrackID: fID, Instants: 50,
			ValidNanos: nanosOf(39), BandNanos: []int64{nanosOf(39), nanosOf(39), 0},
			Suppressions:  []ReasonTally{{Reason: ReasonBelowSpeedFloor, Instants: 10, Nanos: nanosOf(10)}},
			SpatialGapMin: ptr(6.0), SpatialGapP50: ptr(6.0),
			NetTimeGapMin: ptr(1.2), NetTimeGapP50: ptr(1.2),
			GapSigma: alignedGapSigma, SpatialGapSize: 50, NetTimeGapSize: 40,
		}},
	}
}

// ScenarioLaneAdjacentDistractor has a car in the adjacent lane (3.5 m to
// the left) nearer than the in-lane leader, all at 10 m/s for 30 frames. The
// distractor overlaps both lane tracks along the path but is always 3.5 m
// apart, so it is laterally incompatible, never grouped and never in the
// corridor. The in-lane gap is 13.75 m (1.375 s).
func ScenarioLaneAdjacentDistractor() EncounterScenario {
	const fID, lID, dID = "trk_s_lane_follower", "trk_s_lane_leader", "trk_s_lane_adjacent"
	follower := frames(30, func(k, t int64) bodySpec { return fixtureCar(t, 20+float64(k), 0, 10).followerBody() })
	leader := frames(30, func(k, t int64) bodySpec { return fixtureCar(t, 38+float64(k), 0, 10) })
	adjacent := frames(30, func(k, t int64) bodySpec { return fixtureCar(t, 27+float64(k), 3.5, 10) })
	return EncounterScenario{
		Name:        "lane_adjacent_distractor",
		Description: "A nearer car in the adjacent lane is never grouped or chosen; the in-lane car is the leader.",
		Params:      EncounterScenarioParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower...),
			fixtureTrajectory(lID, leader...),
			fixtureTrajectory(dID, adjacent...),
		},
		Paths: []ExpectedPath{
			{FollowerTrackID: dID, MemberTrackIDs: []string{dID}},
			{FollowerTrackID: fID, MemberTrackIDs: []string{fID, lID}},
			{FollowerTrackID: lID, MemberTrackIDs: []string{fID, lID}},
		},
		Encounters: []ExpectedEncounter{{
			LeaderTrackID: lID, FollowerTrackID: fID, Instants: 30,
			ValidNanos: nanosOf(29), BandNanos: []int64{nanosOf(29), nanosOf(29), 0},
			SpatialGapMin: ptr(13.75), SpatialGapP50: ptr(13.75),
			NetTimeGapMin: ptr(1.375), NetTimeGapP50: ptr(1.375),
			GapSigma: alignedGapSigma, SpatialGapSize: 30, NetTimeGapSize: 30,
		}},
		Decisions: []ExpectedPairing{{
			FollowerTrackID: fID, CaptureUnixNanos: fixtureAt(0),
			CandidateTrackIDs: []string{dID, lID}, LeaderTrackID: lID,
		}},
	}
}

// ScenarioAmbiguousThenResolved is a wide approach (grouping bound 2.5 m): two
// cars 1.1 m either side of the follower's line, the one on the right
// starting 0.125 m ahead and pulling away at 12.5 m/s against 10 m/s. The
// right car's rear clears the left car's front by 0.25 k - 4.375 m, against a
// separability threshold of 2 x 0.25 m, so frames 0 to 19 are ambiguous and
// from frame 20 the left car is the nearest separable leader, 15.75 m ahead.
// Its encounter then has 0.9 s of valid time, under the 1.0 s minimum
// opportunity, so its band durations and rates are suppressed. The two cars
// are never each other's leader: 2.2 m apart, each is outside the other's
// corridor.
func ScenarioAmbiguousThenResolved() EncounterScenario {
	const fID, aID, bID = "trk_s_wide_follower", "trk_s_wide_a", "trk_s_wide_b"
	params := EncounterScenarioParams()
	params.Path.GroupLateralM = 2.5
	follower := frames(30, func(k, t int64) bodySpec { return fixtureCar(t, 20+float64(k), 0, 10).followerBody() })
	a := frames(30, func(k, t int64) bodySpec { return fixtureCar(t, 40+float64(k), -1.1, 10) })
	b := frames(30, func(k, t int64) bodySpec { return fixtureCar(t, 40.125+1.25*float64(k), 1.1, 12.5) })
	ambiguous := []ReasonTally{{Reason: ReasonAmbiguousLeader, Instants: 20, Nanos: nanosOf(20)}}
	return EncounterScenario{
		Name:        "ambiguous_then_resolved",
		Description: "Two cars side by side ahead: ambiguous until one clears the other, then the nearer is the leader.",
		Params:      params,
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower...),
			fixtureTrajectory(aID, a...),
			fixtureTrajectory(bID, b...),
		},
		Paths: []ExpectedPath{
			{FollowerTrackID: aID, MemberTrackIDs: []string{aID, bID, fID}},
			{FollowerTrackID: bID, MemberTrackIDs: []string{aID, bID, fID}},
			{FollowerTrackID: fID, MemberTrackIDs: []string{aID, bID, fID}},
		},
		Encounters: []ExpectedEncounter{
			{
				LeaderTrackID: aID, FollowerTrackID: fID, Instants: 30,
				ValidNanos: nanosOf(9), BandNanos: []int64{nanosOf(9), 0, 0}, Suppressions: ambiguous,
				SpatialGapMin: ptr(15.75), SpatialGapP50: ptr(15.75),
				NetTimeGapMin: ptr(1.575), NetTimeGapP50: ptr(1.575),
				GapSigma: alignedGapSigma, SpatialGapSize: 10, NetTimeGapSize: 10,
			},
			{
				LeaderTrackID: bID, FollowerTrackID: fID, Instants: 20,
				BandNanos: []int64{0, 0, 0}, Suppressions: ambiguous,
			},
		},
		Decisions: []ExpectedPairing{
			{
				FollowerTrackID: fID, CaptureUnixNanos: fixtureAt(19),
				CandidateTrackIDs: []string{aID, bID}, Reason: ReasonAmbiguousLeader,
			},
			{
				FollowerTrackID: fID, CaptureUnixNanos: fixtureAt(20),
				CandidateTrackIDs: []string{aID, bID}, LeaderTrackID: aID,
			},
			{FollowerTrackID: aID, CaptureUnixNanos: fixtureAt(20), CandidateTrackIDs: []string{bID, fID}},
		},
	}
}

// ScenarioPartialView is a pair at 10 m/s, 15.75 m apart, for 30 frames,
// whose follower starts with its front unseen and its extent a class prior
// (4.5 by 1.9 m, geometry converging) for frames 0 to 4. Those frames are
// path evidence (the pose is believed) but their gap, 15.5 m, is
// prior-dominated and suppressed as extent_not_converged. The leader's rear
// is never seen, so its endpoint is temporally inferred throughout.
func ScenarioPartialView() EncounterScenario {
	const fID, lID = "trk_s_partial_follower", "trk_s_partial_leader"
	follower := frames(30, func(k, t int64) bodySpec {
		if k >= 5 {
			return fixtureCar(t, 20+float64(k), 0, 10).followerBody()
		}
		b := fixtureCar(t, 20+float64(k), 0, 10)
		b.estimation = EstimationGeometryConverging
		b.length = ExtentBelief{Metres: 4.5, SigmaMetres: 1.5, Provenance: ProvenanceClassPrior}
		b.width = ExtentBelief{Metres: 1.9, SigmaMetres: 1.5, Provenance: ProvenanceClassPrior}
		return b
	})
	leader := frames(30, func(k, t int64) bodySpec { return fixtureCar(t, 40+float64(k), 0, 10) })
	return EncounterScenario{
		Name:        "partial_view",
		Description: "Follower extent a class prior for five frames: those gaps are review-only until the extent converges.",
		Params:      EncounterScenarioParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower...),
			fixtureTrajectory(lID, leader...),
		},
		Paths: []ExpectedPath{
			{FollowerTrackID: fID, MemberTrackIDs: []string{fID, lID}},
			{FollowerTrackID: lID, MemberTrackIDs: []string{fID, lID}},
		},
		Encounters: []ExpectedEncounter{{
			LeaderTrackID: lID, FollowerTrackID: fID, Instants: 30,
			ValidNanos: nanosOf(24), BandNanos: []int64{nanosOf(24), 0, 0},
			Suppressions:  []ReasonTally{{Reason: ReasonExtentNotConverged, Instants: 5, Nanos: nanosOf(5)}},
			SpatialGapMin: ptr(15.75), SpatialGapP50: ptr(15.75),
			NetTimeGapMin: ptr(1.575), NetTimeGapP50: ptr(1.575),
			GapSigma: alignedGapSigma, SpatialGapSize: 25, NetTimeGapSize: 25,
		}},
	}
}

// forkAngle is the branch's heading: a slope of 1 in 4, inside the 0.35 rad
// tangent bound, so the branch diverges as a fork rather than a crossing.
var forkAngle = math.Atan(0.25)

// ScenarioFork has a follower and a car ahead of it on one stem, and a leader
// further ahead that stays on it, while the middle car takes a branch that
// diverges at x = 50 m at a slope of 1 in 4. The middle car shares the stem
// with both others, so all three are one group, but it and the leader are
// near where they share the stem and more than 1.5 m apart beyond it: the
// group forks, and every member's path is refused with fork_or_merge rather
// than fitted through both branches.
func ScenarioFork() EncounterScenario {
	const fID, lID, xID = "trk_s_fork_follower", "trk_s_fork_leader", "trk_s_fork_branch"
	follower := frames(30, func(k, t int64) bodySpec { return fixtureCar(t, 20+float64(k), 0, 10).followerBody() })
	leader := frames(30, func(k, t int64) bodySpec { return fixtureCar(t, 40+float64(k), 0, 10) })
	branch := frames(30, func(k, t int64) bodySpec {
		s := 30 + float64(k)
		if s <= 50 {
			return fixtureCar(t, s, 0, 10)
		}
		c, sn := math.Cos(forkAngle), math.Sin(forkAngle)
		b := fixtureCar(t, 50+(s-50)*c, (s-50)*sn, 0)
		b.psi, b.vx, b.vy = forkAngle, 10*c, 10*sn
		return b
	})
	refused := []PathCondition{PathForkOrMerge}
	members := []string{xID, fID, lID}
	return EncounterScenario{
		Name:        "fork",
		Description: "One track takes a branch the others do not: the group forks and is refused, not fitted.",
		Params:      EncounterScenarioParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower...),
			fixtureTrajectory(lID, leader...),
			fixtureTrajectory(xID, branch...),
		},
		Paths: []ExpectedPath{
			{FollowerTrackID: xID, MemberTrackIDs: members, Conditions: refused},
			{FollowerTrackID: fID, MemberTrackIDs: members, Conditions: refused},
			{FollowerTrackID: lID, MemberTrackIDs: members, Conditions: refused},
		},
		Decisions: []ExpectedPairing{{
			FollowerTrackID: fID, CaptureUnixNanos: fixtureAt(0),
			Reason: ReasonNoCommonPath, Condition: PathForkOrMerge,
		}},
	}
}

// ScenarioReversal has a follower that drives forward at 5 m/s for 20 frames
// and then reverses at 2 m/s for 10, with a leader far ahead. Its evidence
// moves both ways along its own axis, so its path is refused with
// direction_reversal. The leader never overlaps it, fits its own path, and
// has nothing ahead.
func ScenarioReversal() EncounterScenario {
	const fID, lID = "trk_s_reverse_follower", "trk_s_reverse_leader"
	follower := frames(30, func(k, t int64) bodySpec {
		if k < 20 {
			return fixtureCar(t, 20+0.5*float64(k), 0, 5).followerBody()
		}
		b := fixtureCar(t, 29.5-0.2*float64(k-19), 0, -2).followerBody()
		// Reversing, the front faces away from its motion and is not seen.
		b.faces = FaceVisibility{}
		return b
	})
	leader := frames(30, func(k, t int64) bodySpec { return fixtureCar(t, 40+float64(k), 0, 10) })
	return EncounterScenario{
		Name:        "reversal",
		Description: "A follower that reverses along its own path: the directed path is refused.",
		Params:      EncounterScenarioParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower...),
			fixtureTrajectory(lID, leader...),
		},
		Paths: []ExpectedPath{
			{FollowerTrackID: fID, MemberTrackIDs: []string{fID}, Conditions: []PathCondition{PathDirectionReversal}},
			{FollowerTrackID: lID, MemberTrackIDs: []string{lID}},
		},
		Decisions: []ExpectedPairing{{
			FollowerTrackID: fID, CaptureUnixNanos: fixtureAt(25),
			Reason: ReasonNoCommonPath, Condition: PathDirectionReversal,
		}},
	}
}

// ScenarioCrossing is a pair at 10 m/s, 15.75 m apart, for 40 frames, and a
// car crossing the road at x = 50 m, moving +y at 5 m/s from y = -5 m over
// frames 10 to 30. It is inside the follower's corridor (|y| <= 1.75 m) for
// frames 17 to 23, between follower and leader. The crossing car does not
// shape the path, which is fitted from the pair alone, but at those instants
// it is the nearest body ahead and not established on the path, so the
// instants are suppressed with no_common_path (crossing) and counted in the
// encounter, with the leader blocked behind it. The crossing car's own path
// sees the leader cross it at frames 10 and 11.
//
//	valid            frames 0-16 and 24-38     3.2 s
//	no_common_path   frames 17-23              0.7 s
func ScenarioCrossing() EncounterScenario {
	const fID, lID, cID = "trk_s_cross_follower", "trk_s_cross_leader", "trk_s_cross_car"
	follower := frames(40, func(k, t int64) bodySpec { return fixtureCar(t, 20+float64(k), 0, 10).followerBody() })
	leader := frames(40, func(k, t int64) bodySpec { return fixtureCar(t, 40+float64(k), 0, 10) })
	crossing := make([]bodySpec, 0, 21)
	for k := int64(10); k <= 30; k++ {
		b := fixtureCar(fixtureAt(k), 50, -5+0.5*float64(k-10), 0)
		b.psi, b.vx, b.vy = math.Pi/2, 0, 5
		crossing = append(crossing, b)
	}
	return EncounterScenario{
		Name:        "crossing",
		Description: "A car crossing between follower and leader: those instants are suppressed, not measured through it.",
		Params:      EncounterScenarioParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower...),
			fixtureTrajectory(lID, leader...),
			fixtureTrajectory(cID, crossing...),
		},
		Paths: []ExpectedPath{
			{FollowerTrackID: cID, MemberTrackIDs: []string{cID}},
			{FollowerTrackID: fID, MemberTrackIDs: []string{fID, lID}},
			{FollowerTrackID: lID, MemberTrackIDs: []string{fID, lID}},
		},
		Encounters: []ExpectedEncounter{{
			LeaderTrackID: lID, FollowerTrackID: fID, Instants: 40,
			ValidNanos: nanosOf(32), BandNanos: []int64{nanosOf(32), 0, 0},
			Suppressions:  []ReasonTally{{Reason: ReasonNoCommonPath, Instants: 7, Nanos: nanosOf(7)}},
			SpatialGapMin: ptr(15.75), SpatialGapP50: ptr(15.75),
			NetTimeGapMin: ptr(1.575), NetTimeGapP50: ptr(1.575),
			GapSigma: alignedGapSigma, SpatialGapSize: 33, NetTimeGapSize: 33,
		}},
		Decisions: []ExpectedPairing{
			{
				FollowerTrackID: fID, CaptureUnixNanos: fixtureAt(20),
				CandidateTrackIDs: []string{cID, lID}, Reason: ReasonNoCommonPath, Condition: PathCrossing,
			},
			{
				FollowerTrackID: fID, CaptureUnixNanos: fixtureAt(24),
				CandidateTrackIDs: []string{cID, lID}, LeaderTrackID: lID,
			},
			{
				FollowerTrackID: cID, CaptureUnixNanos: fixtureAt(10),
				CandidateTrackIDs: []string{fID, lID}, Reason: ReasonNoCommonPath, Condition: PathCrossing,
			},
		},
	}
}

// ScenarioSparseSupport is a pair at 10 m/s for 10 frames whose follower is
// observed for only the first four and coasts after, below the five evidence
// samples a track needs. Its path is refused with weak_support and none of
// its instants is ordered. The leader fits its own path, exactly 10 m long.
func ScenarioSparseSupport() EncounterScenario {
	const fID, lID = "trk_s_sparse_follower", "trk_s_sparse_leader"
	follower := frames(10, func(k, t int64) bodySpec {
		b := fixtureCar(t, 20+float64(k), 0, 10).followerBody()
		if k >= 4 {
			b.support, b.faces, b.lastObserved = SupportCoasted, FaceVisibility{}, fixtureAt(3)
		}
		return b
	})
	leader := frames(10, func(k, t int64) bodySpec { return fixtureCar(t, 40+float64(k), 0, 10) })
	return EncounterScenario{
		Name:        "sparse_support",
		Description: "A follower with four observed frames has too little evidence for a path.",
		Params:      EncounterScenarioParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower...),
			fixtureTrajectory(lID, leader...),
		},
		Paths: []ExpectedPath{
			{FollowerTrackID: fID, Conditions: []PathCondition{PathWeakSupport}},
			{FollowerTrackID: lID, MemberTrackIDs: []string{lID}},
		},
		Decisions: []ExpectedPairing{{
			FollowerTrackID: fID, CaptureUnixNanos: fixtureAt(0),
			Reason: ReasonNoCommonPath, Condition: PathWeakSupport,
		}},
	}
}
