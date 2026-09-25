package l8behaviour

// Frozen analytic following fixtures, per Section 8.3 of
// docs/plans/lidar-behaviour-analytics-plan.md: known body lengths, offset
// anchors, an oblique heading, partial front/rear views, standstill, an
// occlusion, an ambiguous pair and a lane-adjacent distractor.
//
// Each fixture is a pair of final-stage trajectories with its answers. The
// expected values were computed outside this package, by brute force over the
// footprint corners for the endpoints and from the stated linearised formula
// for the sigmas, and are written here as literals. They are frozen: a change
// to an expected value is a change to the method, and must move
// FollowingMethodID with it.
//
// Most geometry is chosen so the arithmetic is exact in binary (quarter-metre
// positions, power-of-two variances), which is why most gaps and sigmas are
// short decimals. The oblique fixture is deliberately not exact, so it is the
// one that exercises trigonometry and the covariance cross term.
//
// The ambiguous and distractor fixtures also carry pairing expectations. This
// package does not choose leaders; those expectations are the oracle the
// pairing step must reproduce.

import "math"

// Fixture timing and identity. Every fixture uses the same estimator identity
// so any fixture pair may be evaluated together.
const (
	FixtureBaseUnixNanos    int64 = 1_750_000_000_000_000_000
	FixtureFramePeriodNanos int64 = 100_000_000
	fixtureSiteID                 = "fixture_site"
	fixtureSensorID               = "fixture_sensor"
)

// FixtureEstimate is the estimator identity every fixture trajectory carries.
func FixtureEstimate() EstimateIdentity {
	return EstimateIdentity{
		EstimatorID: "fixture_analytic_v1",
		ObsModelID:  "fixture_exact_v1",
		ParamHash:   "fixture/following_v1",
	}
}

// FixtureParams are the bounds every fixture is evaluated with. The speed
// floor sits above Section 8.11's roughly 0.3 m/s noise floor; the corridor is
// half a 3.5 m lane. They are fixture values, not calibrated defaults.
func FixtureParams() FollowingParams {
	return FollowingParams{SpeedFloorMps: 0.5, CorridorHalfWidthM: 1.75, MaxRelativeHeadingRad: 0.35}
}

// ExpectedValue is a frozen value with its one-sigma.
type ExpectedValue struct {
	Value float64 `json:"value"`
	Sigma float64 `json:"sigma"`
}

// ExpectedFollowing is the answer for one pair at one instant.
type ExpectedFollowing struct {
	LeaderTrackID    string `json:"leader_track_id"`
	FollowerTrackID  string `json:"follower_track_id"`
	CaptureUnixNanos int64  `json:"capture_unix_nanos"`
	// Gap is the arithmetic whenever both bodies project, published or not.
	Gap              *ExpectedValue    `json:"gap,omitempty"`
	SpatialGapReason SuppressionReason `json:"spatial_gap_reason,omitempty"`
	// NetTimeGap is nil exactly when the net time gap is suppressed.
	NetTimeGap       *ExpectedValue    `json:"net_time_gap,omitempty"`
	NetTimeGapReason SuppressionReason `json:"net_time_gap_reason,omitempty"`
	// PredictedGap is set when a review-only predicted value is expected.
	PredictedGap         *ExpectedValue `json:"predicted_gap,omitempty"`
	LeaderSource         EndpointSource `json:"leader_source,omitempty"`
	FollowerSource       EndpointSource `json:"follower_source,omitempty"`
	SupportedOpportunity bool           `json:"supported_opportunity"`
	CoastAgeNanos        int64          `json:"coast_age_nanos,omitempty"`
}

// ExpectedPairing is what the pairing step must decide for one follower: a
// leader, or a suppression reason and no leader.
type ExpectedPairing struct {
	FollowerTrackID   string            `json:"follower_track_id"`
	CaptureUnixNanos  int64             `json:"capture_unix_nanos"`
	CandidateTrackIDs []string          `json:"candidate_track_ids"`
	LeaderTrackID     string            `json:"leader_track_id,omitempty"`
	Reason            SuppressionReason `json:"reason,omitempty"`
}

// Fixture is one frozen scenario.
type Fixture struct {
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Path         StraightPath        `json:"path"`
	Params       FollowingParams     `json:"params"`
	Trajectories []Trajectory        `json:"trajectories"`
	Expected     []ExpectedFollowing `json:"expected"`
	Pairing      []ExpectedPairing   `json:"pairing,omitempty"`
	// SupportSeconds is the expected per-track support accounting, at
	// SupportMaxIntervalNanos, where the fixture pins it.
	SupportSeconds          map[string]SupportSeconds `json:"support_seconds,omitempty"`
	SupportMaxIntervalNanos int64                     `json:"support_max_interval_nanos,omitempty"`
}

// Trajectory returns the fixture trajectory for a track id.
func (f Fixture) Trajectory(trackID string) (Trajectory, bool) {
	for _, t := range f.Trajectories {
		if t.Passage.TrackID == trackID {
			return t, true
		}
	}
	return Trajectory{}, false
}

// Fixtures returns every frozen fixture, freshly built, in a stable order.
func Fixtures() []Fixture {
	return []Fixture{
		FixtureAlignedPair(),
		FixtureOffsetAnchors(),
		FixtureObliqueHeading(),
		FixturePartialViews(),
		FixtureStandstill(),
		FixtureOcclusion(),
		FixtureAmbiguousLeader(),
		FixtureLaneAdjacentDistractor(),
	}
}

// bodySpec is one fixture sample before it is frozen into the contract type.
type bodySpec struct {
	x, y, psi, headingVar float64
	vx, vy                float64
	length, width         ExtentBelief
	posVar                [4]float64 // xx, xy, yx, yy
	velVar                [4]float64
	reference             ReferencePoint
	offset                BodyOffset
	faces                 FaceVisibility
	support               SupportState
	estimation            EstimationState
	capture, lastObserved int64
}

func converged(metres, sigma float64) ExtentBelief {
	return ExtentBelief{
		Metres: metres, SigmaMetres: sigma, AdmissibleFrames: 8,
		Provenance: ProvenanceAccumulated, Converged: true,
	}
}

// fixtureCar is an established, observed, final car travelling along +x at
// speed, centred at (x, y), with the leader's default body (4.5 by 2.0 m).
func fixtureCar(capture int64, x, y, speed float64) bodySpec {
	return bodySpec{
		x: x, y: y, vx: speed,
		length: converged(4.5, 0.25), width: converged(2.0, 0.125),
		posVar:    [4]float64{0.015625, 0, 0, 0.015625},
		velVar:    [4]float64{0.0625, 0, 0, 0.0625},
		reference: ReferenceBodyCentre, support: SupportObserved,
		estimation: EstimationEstablished, capture: capture, lastObserved: capture,
	}
}

// followerBody switches a fixture car to the follower's body (4.0 by 1.75 m)
// with its front face observed.
func (b bodySpec) followerBody() bodySpec {
	b.length, b.width = converged(4.0, 0.25), converged(1.75, 0.125)
	b.faces = FaceVisibility{FrontObserved: true}
	return b
}

func (b bodySpec) sample() TrajectorySample {
	return TrajectorySample{
		CaptureUnixNanos: b.capture,
		StateModel:       StateModelCVCartesianV1,
		Reference:        b.reference,
		X:                b.x, Y: b.y, VX: b.vx, VY: b.vy,
		Covariance: [16]float64{
			b.posVar[0], b.posVar[1], 0, 0,
			b.posVar[2], b.posVar[3], 0, 0,
			0, 0, b.velVar[0], b.velVar[1],
			0, 0, b.velVar[2], b.velVar[3],
		},
		AnchorToCentre: b.offset,
		Heading: HeadingBelief{
			Rad: b.psi, VarianceRad2: b.headingVar, Provenance: ProvenanceObserved,
		},
		Length: b.length, Width: b.width, Faces: b.faces,
		Support: b.support, Stage: StageFinal, Estimation: b.estimation,
		LastObservedUnixNanos: b.lastObserved,
	}
}

func fixtureTrajectory(trackID string, specs ...bodySpec) Trajectory {
	samples := make([]TrajectorySample, len(specs))
	for i, s := range specs {
		samples[i] = s.sample()
	}
	return Trajectory{
		Passage: Passage{
			TrackID: trackID, SiteID: fixtureSiteID, SensorID: fixtureSensorID,
			MotionClass: MotionRigidVehicle, ClassLabel: "car",
		},
		Estimate: FixtureEstimate(),
		Samples:  samples,
	}
}

func fixturePathX() StraightPath {
	return StraightPath{ID: "fixture/straight_x_v1", LengthM: 120}
}

func fixtureAt(k int64) int64 { return FixtureBaseUnixNanos + k*FixtureFramePeriodNanos }

// FixtureAlignedPair is two aligned, centred cars at 5 m/s. The gap sigma is
// exactly Section 9.1's sqrt(sigma_sL^2 + sigma_sF^2 + (sigma_LL/2)^2 +
// (sigma_LF/2)^2) with zero heading variance: 0.25 m.
func FixtureAlignedPair() Fixture {
	t := fixtureAt(0)
	leader := fixtureCar(t, 30, 0, 5)
	follower := fixtureCar(t, 20, 0, 5).followerBody()
	return Fixture{
		Name:        "aligned_pair",
		Description: "Centred, aligned cars of known length; the gap is bumper to bumper, not centre to centre (10 m).",
		Path:        fixturePathX(), Params: FixtureParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory("trk_fixture_aligned_leader", leader),
			fixtureTrajectory("trk_fixture_aligned_follower", follower),
		},
		Expected: []ExpectedFollowing{{
			LeaderTrackID: "trk_fixture_aligned_leader", FollowerTrackID: "trk_fixture_aligned_follower",
			CaptureUnixNanos: t,
			Gap:              &ExpectedValue{Value: 5.75, Sigma: 0.25},
			NetTimeGap:       &ExpectedValue{Value: 1.15, Sigma: 0.07619875327064084},
			LeaderSource:     EndpointTemporallyInferred, FollowerSource: EndpointDirectlyObserved,
			SupportedOpportunity: true,
		}},
	}
}

// FixtureOffsetAnchors is the aligned pair again, but neither state is at the
// body centre: the follower's reference is its front-face centre and the
// leader's its right-side face centre. The physical gap is unchanged at 5.75
// m; treating either anchor as the centre would not be. The heading variance
// swings each centre about its anchor, which widens the sigma.
func FixtureOffsetAnchors() Fixture {
	t := fixtureAt(0)
	leader := fixtureCar(t, 30, -1.0, 5)
	leader.reference, leader.offset, leader.headingVar = ReferenceNearFaceCentre, BodyOffset{LateralM: 1.0}, 0.0025
	follower := fixtureCar(t, 22, 0, 5).followerBody()
	follower.reference, follower.offset, follower.headingVar = ReferenceNearFaceCentre, BodyOffset{LongitudinalM: -2.0}, 0.0025
	return Fixture{
		Name:        "offset_anchors",
		Description: "Reference points on the front face and the side face; endpoints follow the body, not the anchor.",
		Path:        fixturePathX(), Params: FixtureParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory("trk_fixture_offset_leader", leader),
			fixtureTrajectory("trk_fixture_offset_follower", follower),
		},
		Expected: []ExpectedFollowing{{
			LeaderTrackID: "trk_fixture_offset_leader", FollowerTrackID: "trk_fixture_offset_follower",
			CaptureUnixNanos: t,
			Gap:              &ExpectedValue{Value: 5.75, Sigma: 0.27278941053493994},
			NetTimeGap:       &ExpectedValue{Value: 1.15, Sigma: 0.07926419431243845},
			LeaderSource:     EndpointTemporallyInferred, FollowerSource: EndpointDirectlyObserved,
			SupportedOpportunity: true,
		}},
	}
}

// FixtureObliqueHeading places the path at 30 degrees to the sensor axes, with
// an anisotropic, correlated position covariance, and yaws the follower 0.2
// rad off the path tangent. Its leading extreme is then a front corner:
// h = 2.0 cos 0.2 + 0.875 sin 0.2 past its centre.
func FixtureObliqueHeading() Fixture {
	t := fixtureAt(0)
	path := StraightPath{ID: "fixture/oblique_v1", OriginX: 5, OriginY: -3, HeadingRad: math.Pi / 6, LengthM: 120}
	lx, ly := path.PointAt(32, 0)
	fx, fy := path.PointAt(20, 0.3)
	psiL, psiF := path.HeadingRad, path.HeadingRad+0.2
	pos := [4]float64{0.02, 0.005, 0.005, 0.01}
	vel := [4]float64{0.0625, 0.01, 0.01, 0.04}

	leader := fixtureCar(t, lx, ly, 0)
	leader.psi, leader.headingVar, leader.posVar, leader.velVar = psiL, 0.0025, pos, vel
	leader.vx, leader.vy = 12*math.Cos(psiL), 12*math.Sin(psiL)
	follower := fixtureCar(t, fx, fy, 0).followerBody()
	follower.psi, follower.headingVar, follower.posVar, follower.velVar = psiF, 0.0025, pos, vel
	follower.vx, follower.vy = 12*math.Cos(psiF), 12*math.Sin(psiF)
	return Fixture{
		Name:        "oblique_heading",
		Description: "Path oblique to the sensor frame; follower yawed 0.2 rad, so its leading extreme is a corner.",
		Path:        path, Params: FixtureParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory("trk_fixture_oblique_leader", leader),
			fixtureTrajectory("trk_fixture_oblique_follower", follower),
		},
		Expected: []ExpectedFollowing{{
			LeaderTrackID: "trk_fixture_oblique_leader", FollowerTrackID: "trk_fixture_oblique_follower",
			CaptureUnixNanos: t,
			Gap:              &ExpectedValue{Value: 7.616031179871843, Sigma: 0.27834731180017186},
			NetTimeGap:       &ExpectedValue{Value: 0.6475777047588789, Sigma: 0.02754703429680678},
			LeaderSource:     EndpointTemporallyInferred, FollowerSource: EndpointDirectlyObserved,
			SupportedOpportunity: true,
		}},
	}
}

// FixturePartialViews follows a follower whose extent converges during the
// passage. The leader's rear face is never seen, so its trailing endpoint is
// temporally inferred throughout. At the first instant the follower's front is
// not seen and its extent is still the class prior (4.5 +- 1.5 m), so its
// leading endpoint is prior-dominated: the gap arithmetic exists for review,
// with the prior's wide sigma, and is suppressed as extent_not_converged. At
// the second the follower is established with its front face observed.
func FixturePartialViews() Fixture {
	t0, t1 := fixtureAt(0), fixtureAt(1)
	leader0, leader1 := fixtureCar(t0, 40, 0, 10), fixtureCar(t1, 41, 0, 10)
	for _, l := range []*bodySpec{&leader0, &leader1} {
		l.length, l.headingVar = converged(4.5, 0.5), 0.0025
	}
	follower0 := fixtureCar(t0, 30, 0, 10)
	follower0.headingVar = 0.0025
	follower0.estimation = EstimationGeometryConverging
	follower0.length = ExtentBelief{Metres: 4.5, SigmaMetres: 1.5, Provenance: ProvenanceClassPrior}
	follower0.width = ExtentBelief{Metres: 1.9, SigmaMetres: 1.5, Provenance: ProvenanceClassPrior}
	follower1 := fixtureCar(t1, 31, 0, 10).followerBody()
	follower1.headingVar = 0.0025
	return Fixture{
		Name:        "partial_views",
		Description: "Unseen leader rear (temporally inferred); follower front first prior-dominated, then observed.",
		Path:        fixturePathX(), Params: FixtureParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory("trk_fixture_partial_leader", leader0, leader1),
			fixtureTrajectory("trk_fixture_partial_follower", follower0, follower1),
		},
		Expected: []ExpectedFollowing{
			{
				LeaderTrackID: "trk_fixture_partial_leader", FollowerTrackID: "trk_fixture_partial_follower",
				CaptureUnixNanos: t0,
				Gap:              &ExpectedValue{Value: 5.5, Sigma: 0.8130229086563305},
				SpatialGapReason: ReasonExtentNotConverged,
				NetTimeGapReason: ReasonExtentNotConverged,
				LeaderSource:     EndpointTemporallyInferred, FollowerSource: EndpointPriorDominated,
			},
			{
				LeaderTrackID: "trk_fixture_partial_leader", FollowerTrackID: "trk_fixture_partial_follower",
				CaptureUnixNanos: t1,
				Gap:              &ExpectedValue{Value: 5.75, Sigma: 0.33732634421284086},
				NetTimeGap:       &ExpectedValue{Value: 0.575, Sigma: 0.036667850359681564},
				LeaderSource:     EndpointTemporallyInferred, FollowerSource: EndpointDirectlyObserved,
				SupportedOpportunity: true,
			},
		},
	}
}

// FixtureStandstill is a stopped queue. The spatial gap is valid at 2.5 m; the
// net time gap is undefined at rest, not infinite, so it is suppressed, and
// the instant is constrained time rather than following opportunity.
func FixtureStandstill() Fixture {
	t := fixtureAt(0)
	leader := fixtureCar(t, 10, 0, 0)
	follower := fixtureCar(t, 3.25, 0, 0).followerBody()
	return Fixture{
		Name:        "standstill",
		Description: "Stopped queue: spatial gap valid, net time gap suppressed below the speed floor.",
		Path:        fixturePathX(), Params: FixtureParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory("trk_fixture_standstill_leader", leader),
			fixtureTrajectory("trk_fixture_standstill_follower", follower),
		},
		Expected: []ExpectedFollowing{{
			LeaderTrackID: "trk_fixture_standstill_leader", FollowerTrackID: "trk_fixture_standstill_follower",
			CaptureUnixNanos: t,
			Gap:              &ExpectedValue{Value: 2.5, Sigma: 0.25},
			NetTimeGapReason: ReasonBelowSpeedFloor,
			LeaderSource:     EndpointTemporallyInferred, FollowerSource: EndpointDirectlyObserved,
		}},
	}
}

// occlusionSigmas are the frozen gap sigmas at each coasted instant, k = 5 to
// 9: the follower's position variance grows by 0.015625 m^2 per coasted
// frame, so the predicted gap's bound widens monotonically.
var occlusionSigmas = [5]float64{
	0.2795084971874737, 0.30618621784789724, 0.33071891388307384, 0.3535533905932738, 0.375,
}

// FixtureOcclusion is fifteen frames at 10 Hz of a pair at 10 m/s. The
// follower is hidden for frames 5 to 9 and coasts: established for the first
// three coasted frames, temporarily degraded for the last two. Coasted
// instants are not observation. They carry a review-only predicted gap with
// coast age and a widening sigma, contribute nothing to supported opportunity,
// and are accounted as coasted time; reacquisition at frame 10 does not turn
// them back into observation.
func FixtureOcclusion() Fixture {
	const leaderID, followerID = "trk_fixture_occlusion_leader", "trk_fixture_occlusion_follower"
	var leader, follower []bodySpec
	var expected []ExpectedFollowing
	for k := int64(0); k < 15; k++ {
		t := fixtureAt(k)
		leader = append(leader, fixtureCar(t, float64(40+k), 0, 10))
		f := fixtureCar(t, float64(30+k), 0, 10).followerBody()
		e := ExpectedFollowing{
			LeaderTrackID: leaderID, FollowerTrackID: followerID, CaptureUnixNanos: t,
			LeaderSource: EndpointTemporallyInferred, FollowerSource: EndpointDirectlyObserved,
		}
		if k >= 5 && k <= 9 {
			f.support, f.faces, f.lastObserved = SupportCoasted, FaceVisibility{}, fixtureAt(4)
			f.posVar = [4]float64{0.015625 * float64(k-3), 0, 0, 0.015625 * float64(k-3)}
			reason := ReasonNotObserved
			if k >= 8 {
				f.estimation = EstimationTemporarilyDegraded
				reason = ReasonModelDegraded
			}
			sigma := occlusionSigmas[k-5]
			e.Gap = &ExpectedValue{Value: 5.75, Sigma: sigma}
			e.PredictedGap = &ExpectedValue{Value: 5.75, Sigma: sigma}
			e.SpatialGapReason, e.NetTimeGapReason = reason, reason
			e.FollowerSource = EndpointTemporallyInferred
			e.CoastAgeNanos = (k - 4) * FixtureFramePeriodNanos
		} else {
			e.Gap = &ExpectedValue{Value: 5.75, Sigma: 0.25}
			e.NetTimeGap = &ExpectedValue{Value: 0.575, Sigma: 0.028838179987648316}
			e.SupportedOpportunity = true
		}
		follower = append(follower, f)
		expected = append(expected, e)
	}
	return Fixture{
		Name:        "occlusion",
		Description: "Follower hidden for five frames: coasted instants are review-only and excluded from support.",
		Path:        fixturePathX(), Params: FixtureParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(leaderID, leader...),
			fixtureTrajectory(followerID, follower...),
		},
		Expected:                expected,
		SupportMaxIntervalNanos: FixtureFramePeriodNanos * 3 / 2,
		SupportSeconds: map[string]SupportSeconds{
			leaderID:   {SupportObserved: 1.4},
			followerID: {SupportObserved: 0.9, SupportCoasted: 0.5},
		},
	}
}

// FixtureAmbiguousLeader has two credible leaders ahead of one follower on a
// wide approach: both inside the corridor, same direction, with gaps of 5.75
// and 5.875 m, a difference well inside one gap sigma (0.25 m). Each pair is
// individually well formed; the pairing step must suppress with
// ambiguous_leader rather than pick the nearer on a 0.125 m margin.
func FixtureAmbiguousLeader() Fixture {
	t := fixtureAt(0)
	follower := fixtureCar(t, 20, 0, 10).followerBody()
	a := fixtureCar(t, 30, -1.0, 10)
	b := fixtureCar(t, 30.125, 1.2, 10)
	b.width = converged(1.8, 0.125)
	const fID, aID, bID = "trk_fixture_ambiguous_follower", "trk_fixture_ambiguous_a", "trk_fixture_ambiguous_b"
	return Fixture{
		Name:        "ambiguous_leader",
		Description: "Two credible leaders 0.125 m apart in gap: the pairing must suppress, not choose.",
		Path:        fixturePathX(), Params: FixtureParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower),
			fixtureTrajectory(aID, a),
			fixtureTrajectory(bID, b),
		},
		Expected: []ExpectedFollowing{
			{
				LeaderTrackID: aID, FollowerTrackID: fID, CaptureUnixNanos: t,
				Gap:          &ExpectedValue{Value: 5.75, Sigma: 0.25},
				NetTimeGap:   &ExpectedValue{Value: 0.575, Sigma: 0.028838179987648316},
				LeaderSource: EndpointTemporallyInferred, FollowerSource: EndpointDirectlyObserved,
				SupportedOpportunity: true,
			},
			{
				LeaderTrackID: bID, FollowerTrackID: fID, CaptureUnixNanos: t,
				Gap:          &ExpectedValue{Value: 5.875, Sigma: 0.25},
				NetTimeGap:   &ExpectedValue{Value: 0.5875, Sigma: 0.02899521781690905},
				LeaderSource: EndpointTemporallyInferred, FollowerSource: EndpointDirectlyObserved,
				SupportedOpportunity: true,
			},
		},
		Pairing: []ExpectedPairing{{
			FollowerTrackID: fID, CaptureUnixNanos: t,
			CandidateTrackIDs: []string{aID, bID},
			Reason:            ReasonAmbiguousLeader,
		}},
	}
}

// FixtureLaneAdjacentDistractor has a nearer car in the adjacent lane (3.5 m
// lateral) and the true leader further ahead in the follower's own lane. The
// distractor pair fails the common-path corridor, so its much shorter gap
// (2.75 m) is never a following measurement; the true pair's is 13.75 m.
func FixtureLaneAdjacentDistractor() Fixture {
	t := fixtureAt(0)
	follower := fixtureCar(t, 20, 0, 10).followerBody()
	leader := fixtureCar(t, 38, 0.25, 10)
	distractor := fixtureCar(t, 27, 3.5, 10)
	const fID, lID, dID = "trk_fixture_distractor_follower", "trk_fixture_distractor_leader", "trk_fixture_distractor_adjacent"
	return Fixture{
		Name:        "lane_adjacent_distractor",
		Description: "A nearer car in the adjacent lane is not a leader; the in-lane car further ahead is.",
		Path:        fixturePathX(), Params: FixtureParams(),
		Trajectories: []Trajectory{
			fixtureTrajectory(fID, follower),
			fixtureTrajectory(lID, leader),
			fixtureTrajectory(dID, distractor),
		},
		Expected: []ExpectedFollowing{
			{
				LeaderTrackID: lID, FollowerTrackID: fID, CaptureUnixNanos: t,
				Gap:          &ExpectedValue{Value: 13.75, Sigma: 0.25},
				NetTimeGap:   &ExpectedValue{Value: 1.375, Sigma: 0.04250459533979826},
				LeaderSource: EndpointTemporallyInferred, FollowerSource: EndpointDirectlyObserved,
				SupportedOpportunity: true,
			},
			{
				LeaderTrackID: dID, FollowerTrackID: fID, CaptureUnixNanos: t,
				Gap:              &ExpectedValue{Value: 2.75, Sigma: 0.25},
				SpatialGapReason: ReasonNoCommonPath,
				NetTimeGapReason: ReasonNoCommonPath,
				LeaderSource:     EndpointTemporallyInferred, FollowerSource: EndpointDirectlyObserved,
			},
		},
		Pairing: []ExpectedPairing{{
			FollowerTrackID: fID, CaptureUnixNanos: t,
			CandidateTrackIDs: []string{lID, dID},
			LeaderTrackID:     lID,
		}},
	}
}
