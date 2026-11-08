package l5tracks

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks/likelihood"
)

// RobustLikelihoodAssociationCost: the Student-t negative log-likelihood the
// multiple-model filter weighs its modes with, as the assignment cost. The S3
// properties it shares with LikelihoodAssociationCost are pinned beside that
// option's in association_covariance_bias_test.go; the off-path digest is in
// association_likelihood_off_test.go. These tests cover what is its own: the
// floor, the heavy tail, the S it reads and how it composes.

// costRecorder keeps the last cost association recorded for each pairing.
type costRecorder struct {
	costs map[costKey]float32
}

type costKey struct {
	cluster int64
	track   string
}

func newCostRecorder() *costRecorder { return &costRecorder{costs: map[costKey]float32{}} }

func (c *costRecorder) IsEnabled() bool { return true }
func (c *costRecorder) RecordAssociation(clusterID int64, trackID string, cost float32, _ bool) {
	c.costs[costKey{clusterID, trackID}] = cost
}
func (c *costRecorder) RecordGatingRegion(string, float32, float32, float32, float32, float32) {}
func (c *costRecorder) RecordInnovation(string, float32, float32, float32, float32, float32)   {}
func (c *costRecorder) RecordPrediction(string, float32, float32, float32, float32)            {}

func TestRobustLikelihoodCostIsZeroAtItsFloorAndRisesFromIt(t *testing.T) {
	for _, nu := range []float64{0, 1, 4, 30} {
		floor := robustLikelihoodFloor(nu)
		lnMin := math.Log(robustLikelihoodMinDet)
		if got := robustLikelihoodCost(0, lnMin, nu, floor); got != 0 {
			t.Errorf("ν=%v: cost %v at the floor, want 0", nu, got)
		}
		// The medoid gate's own float32 threshold is admitted too.
		if got := robustLikelihoodCost(0, math.Log(float64(float32(MinDeterminantThreshold))), nu, floor); got < 0 {
			t.Errorf("ν=%v: cost %v at the float32 determinant threshold", nu, got)
		}
		prev := float32(-1)
		for _, d2 := range []float32{0, 0.5, 4, 9.21, 36} {
			got := robustLikelihoodCost(d2, 0, nu, floor)
			if !(got > prev) {
				t.Errorf("ν=%v: cost %v at d²=%v does not rise from %v", nu, got, d2, prev)
			}
			prev = got
		}
	}
	if robustLikelihoodFloor(0) != robustLikelihoodFloor(likelihood.DefaultStudentTNu) {
		t.Error("ν=0 does not mean the default")
	}
}

// The cost is twice the shared Student-t NLL, less the floor: the density the
// IMM uses, on the d² scale the other cost terms are stated in.
func TestRobustLikelihoodCostIsTheSharedStudentT(t *testing.T) {
	const nu = 4
	floor := robustLikelihoodFloor(nu)
	for _, c := range []struct{ d2, logDet float64 }{{0.3, -2}, {5, 0.4}, {30, -5}} {
		want := 2 * (likelihood.StudentTNegLogLikelihood(2, c.d2, c.logDet, nu) - floor)
		if got := robustLikelihoodCost(float32(c.d2), c.logDet, nu, floor); math.Abs(float64(got)-want) > 1e-5*math.Max(1, want) {
			t.Errorf("d²=%v ln|S|=%v: cost %v, want %v", c.d2, c.logDet, got, want)
		}
	}
}

// outlierTracks builds two confirmed tracks with the same isotropic S = σ²I,
// σ = 0.25 m: A at the origin and B 5σ along +X. Cluster c1 sits on A; c2 sits
// 5σ from A and 10σ from B, on the far side of A. Pairing A–c1 and B–c2 fits
// one cluster exactly and leaves one 10-sigma outlier; the swap misses both by
// 5σ. Under any quadratic cost the swap is cheaper (25 + 25 < 0 + 100), so the
// one outlier moves both identities; under the Student-t it is not
// (3 ln 26 < 6 ln 7.25, in NLL).
func outlierTracks(set func(*TrackerConfig)) (*Tracker, []WorldCluster) {
	cfg := DefaultTrackerConfig()
	// The shipped gate (d² ≤ 36) refuses a 10-sigma pair outright; widen it
	// so the pairing reaches the cost, which is what this test is about.
	cfg.GatingDistanceSquared = 121
	cfg.AssociationExtentCostWeight = 0
	set(&cfg)
	tk := NewTracker(cfg)
	sigma := float32(0.25)
	pos := sigma*sigma - cfg.MeasurementNoise
	mk := func(id string, seq int64, x float32) *TrackedObject {
		tr := &TrackedObject{TrackID: id, CreationSequence: seq, TrackState: TrackConfirmed, ObservationCount: 10, X: x}
		tr.P = [16]float32{pos, 0, 0, 0, 0, pos, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
		return tr
	}
	tk.Tracks["a"], tk.Tracks["b"] = mk("a", 1, 0), mk("b", 2, 5*sigma)
	c1, c2 := s3Cluster(0, 0), s3Cluster(-5*sigma, 0)
	c1.ClusterID, c2.ClusterID = 1, 2
	return tk, []WorldCluster{c1, c2}
}

func TestATenSigmaOutlierDoesNotDominateTheRobustAssignment(t *testing.T) {
	arms := []struct {
		name     string
		set      func(*TrackerConfig)
		wantSwap bool
	}{
		{"shipped d²", func(*TrackerConfig) {}, true},
		{"likelihood_cost", func(c *TrackerConfig) { c.LikelihoodAssociationCost = true }, true},
		{"robust_likelihood_cost", func(c *TrackerConfig) { c.RobustLikelihoodAssociationCost = true }, false},
	}
	for _, arm := range arms {
		tk, clusters := outlierTracks(arm.set)
		rec := newCostRecorder()
		tk.DebugCollector = rec
		got := tk.associate(clusters, 0.1)
		inlier, outlier := rec.costs[costKey{1, "b"}], rec.costs[costKey{2, "b"}]
		exact := rec.costs[costKey{1, "a"}]
		t.Logf("%s: assignment %v; B's 5σ cost − A's exact %.3f, B's 10σ cost − A's exact %.3f",
			arm.name, got, inlier-exact, outlier-exact)
		swapped := got[0] == "b" && got[1] == "a"
		kept := got[0] == "a" && got[1] == "b"
		if arm.wantSwap && !swapped {
			t.Errorf("%s: got %v, want the swap [b a] a quadratic cost makes", arm.name, got)
		}
		if !arm.wantSwap && !kept {
			t.Errorf("%s: got %v, want [a b]: one outlier must not move both identities", arm.name, got)
		}
	}

	// Relative to an inlier, the 10-sigma pair is charged far less under the
	// Student-t than under the Gaussian: same S, so the difference is the
	// miss term alone.
	g, _ := outlierTracks(func(c *TrackerConfig) { c.LikelihoodAssociationCost = true })
	r, _ := outlierTracks(func(c *TrackerConfig) { c.RobustLikelihoodAssociationCost = true })
	for _, tk := range []*Tracker{g, r} {
		tk.DebugCollector = newCostRecorder()
	}
	_, clusters := outlierTracks(func(*TrackerConfig) {})
	g.associate(clusters, 0.1)
	r.associate(clusters, 0.1)
	gc, rc := g.DebugCollector.(*costRecorder).costs, r.DebugCollector.(*costRecorder).costs
	gaussExcess := gc[costKey{2, "b"}] - gc[costKey{1, "b"}]
	robustExcess := rc[costKey{2, "b"}] - rc[costKey{1, "b"}]
	if !(robustExcess > 0) || !(robustExcess < gaussExcess/3) {
		t.Fatalf("10σ over 5σ excess: Student-t %.3f, Gaussian %.3f; want positive and under a third", robustExcess, gaussExcess)
	}
}

// The cost reads the S the medoid gate read, isotropic or adaptive.
func TestRobustLikelihoodCostReadsTheMedoidGatesS(t *testing.T) {
	for _, adaptive := range []bool{false, true} {
		tk, clusters := outlierTracks(func(c *TrackerConfig) {
			c.RobustLikelihoodAssociationCost = true
			c.AdaptiveMeasurementNoise = adaptive
		})
		rec := newCostRecorder()
		tk.DebugCollector = rec
		track, cluster := tk.Tracks["a"], clusters[0]
		tk.assignClusters(clusters, []int{0}, []string{"a"}, 0.1, tk.Config.GatingDistanceSquared)

		d2, lnDet := tk.gateInnovation(track, clusters, 0, 0.1)
		// The S the gate used, recomputed independently of it.
		var s00, s01, s10, s11 float64
		if adaptive {
			r := tk.evaluateNoise(track, cluster, tk.measurementForCluster(cluster, tk.LastUpdateNanos)).SiteCovariance
			a00, a01, a10, a11 := adaptiveInnovationCovariance(&track.P, r)
			s00, s01, s10, s11 = float64(a00), float64(a01), float64(a10), float64(a11)
		} else {
			r := float64(tk.Config.MeasurementNoise)
			s00, s11 = float64(track.P[0])+r, float64(track.P[5])+r
		}
		if want := math.Log(s00*s11 - s01*s10); math.Abs(lnDet-want) > 1e-5 {
			t.Errorf("adaptive=%v: gate ln det S %v, independent %v", adaptive, lnDet, want)
		}
		want := robustLikelihoodCost(d2, lnDet, 0, robustLikelihoodFloor(0))
		if got := rec.costs[costKey{1, "a"}]; got != want {
			t.Errorf("adaptive=%v: cost %v, want %v from the gate's d² and S", adaptive, got, want)
		}
	}
}

// Under NearEdgeTracking a body-centre track is gated on A2's face residual,
// whose S carries the loose medoid direction's half-extent variance. The cost
// reads that S, not the medoid's P + R, so the face-referenced innovation is
// scored as the gate judged it.
func TestRobustLikelihoodCostReadsTheA2GatesS(t *testing.T) {
	cfg := nearEdgeTrackingConfig()
	cfg.RobustLikelihoodAssociationCost = true
	tk := NewTracker(cfg)
	rec := newCostRecorder()
	tk.DebugCollector = rec
	track := a2Track()
	track.TrackID = "body"
	tk.Tracks["body"] = track
	clusters := []WorldCluster{{ClusterID: 7, CentroidX: 0.5, CentroidY: -0.9}}
	tk.assignClusters(clusters, []int{0}, []string{"body"}, 0.1, cfg.GatingDistanceSquared)

	frame, ok := tk.nearEdgePairs[nearEdgePairKey{track, 0}]
	if !ok {
		t.Fatal("the gate did not measure the pair under A2")
	}
	a2D2, a2Det := tk.faceResidualInnovationAt(trackedPrediction{x: track.X, y: track.Y, p: track.P},
		&track.solidBody, nearEdgePriorFor(track), clusters[0], frame)
	if a2D2 >= SingularDistanceRejection || a2D2 > cfg.GatingDistanceSquared {
		t.Fatalf("setup: A2 refuses the pair (d² %v)", a2D2)
	}
	floor := robustLikelihoodFloor(0)
	want := robustLikelihoodCost(a2D2, math.Log(a2Det), 0, floor)
	got := rec.costs[costKey{7, "body"}]
	if got != want {
		t.Fatalf("cost %v, want %v from A2's d² %v and det S %v", got, want, a2D2, a2Det)
	}
	r := float64(cfg.MeasurementNoise)
	medoidLnDet := math.Log((float64(track.P[0]) + r) * (float64(track.P[5]) + r))
	if math.Abs(math.Log(a2Det)-medoidLnDet) < 0.1 {
		t.Fatalf("setup: A2's ln det S %v is the medoid's %v, so the test cannot tell them apart", math.Log(a2Det), medoidLnDet)
	}
	if wrong := robustLikelihoodCost(a2D2, medoidLnDet, 0, floor); got == wrong {
		t.Fatal("the cost used the medoid's S, not the gate's")
	}
	t.Logf("A2 d² %.3f, ln det S %.3f (medoid P+R would be %.3f); cost %.3f", a2D2, math.Log(a2Det), medoidLnDet, got)
}

// The robust cost composes with the other association options because it
// changes only the value of a pairing the gate admitted. For every option
// whose decisions do not read cost values (the staged assignments, the extent
// term, the overlap end and birth gate, the adaptive noise model), on the same
// tracker state each composite admits exactly the pairings it admits with the
// cost off, and every admitted cost is finite and non-negative. The
// reacquisition guard's ambiguity rules and ContestedRejoin do read cost
// differences, against reacquisitionAmbiguityMargin, so their refusals follow
// whichever cost is in force; that case is pinned separately below.
func TestRobustLikelihoodCostComposesWithTheOtherAssociationOptions(t *testing.T) {
	composites := map[string]func(*TrackerConfig){
		"tentative_priority": func(c *TrackerConfig) { c.TentativePriority = true },
		"cascade":            func(c *TrackerConfig) { c.CascadedAssociation = true },
		"extent_cost":        func(c *TrackerConfig) { c.AssociationExtentCostWeight = 1 },
		"overlap_end_birth_gate": func(c *TrackerConfig) {
			c.OverlapEndFrames, c.OverlapEndOnBody, c.BirthGateOnBody = 1, true, true
		},
		"adaptive_noise": func(c *TrackerConfig) { c.AdaptiveMeasurementNoise = true },
	}
	dt := float32(s3FramePeriod.Seconds())
	offers := [][]WorldCluster{
		{s3Cluster(10, 0.6)},
		{s3Cluster(10, 0.4), s3Cluster(10, 1.2)},
		{s3Cluster(10, 0), s3Cluster(10, 0.6), s3Cluster(10, 3.2)},
	}
	for name, set := range composites {
		cfg := DefaultTrackerConfig()
		set(&cfg)
		for _, misses := range []int{1, 3} {
			tk, _, _ := s3Scene(t, cfg, 1.2, misses)
			for oi, offer := range offers {
				for i := range offer {
					offer[i].ClusterID = int64(i + 1)
				}
				off, on := newCostRecorder(), newCostRecorder()
				tk.Config.RobustLikelihoodAssociationCost = false
				tk.DebugCollector = off
				tk.associate(offer, dt)
				tk.Config.RobustLikelihoodAssociationCost = true
				tk.DebugCollector = on
				tk.associate(offer, dt)
				if len(on.costs) != len(off.costs) {
					t.Fatalf("%s misses=%d offer %d: %d pairings costed, %d without the option", name, misses, oi, len(on.costs), len(off.costs))
				}
				for k, c := range off.costs {
					rc, ok := on.costs[k]
					if !ok || hungarianForbidden(c) != hungarianForbidden(rc) {
						t.Errorf("%s misses=%d offer %d: pairing %v forbidden=%v off, %v on", name, misses, oi, k, hungarianForbidden(c), hungarianForbidden(rc))
					}
					if !hungarianForbidden(rc) && !(rc >= 0) {
						t.Errorf("%s misses=%d offer %d: pairing %v costs %v", name, misses, oi, k, rc)
					}
				}
			}
		}
	}
}

// ContestedRejoin refuses a reacquiring track's bid that a live track's beats
// by less than the margin, reading the matrix's costs. Under d² the coasting
// track's inflated S makes its bid on S3's equidistant cluster the cheaper by
// far, so nothing is contested and the coasting track takes it; under the
// robust cost the live track's is cheaper, the guard contests the rejoin, and
// the cluster stays with the live track. The two remedies agree. Admitted
// costs stay finite and non-negative throughout.
func TestRobustLikelihoodCostAndContestedRejoinAgreeOnS3(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.OcclusionContinuity = DefaultOcclusionContinuity()
	cfg.OcclusionContinuity.ContestedRejoin = true
	dt := float32(s3FramePeriod.Seconds())
	for _, misses := range []int{1, 3} {
		tk, freshID, coastingID := s3Scene(t, cfg, 1.2, misses)
		contested := []WorldCluster{s3Cluster(10, 0.6)}
		contested[0].ClusterID = 1
		before := tk.continuity.ReacquisitionsContested
		off := tk.associate(contested, dt)
		if tk.continuity.ReacquisitionsContested != before {
			t.Errorf("misses=%d: d² contested the rejoin; the coasting bid should be the cheaper", misses)
		}
		if off[0] != coastingID {
			t.Errorf("misses=%d: d² gave the cluster to %q, want the coasting track (S3)", misses, off[0])
		}
		tk.Config.RobustLikelihoodAssociationCost = true
		rec := newCostRecorder()
		tk.DebugCollector = rec
		before = tk.continuity.ReacquisitionsContested
		on := tk.associate(contested, dt)
		if tk.continuity.ReacquisitionsContested != before+1 {
			t.Errorf("misses=%d: the robust cost did not leave the rejoin contested", misses)
		}
		if on[0] != freshID {
			t.Errorf("misses=%d: robust cost with contested rejoin gave the cluster to %q, want the fresh track", misses, on[0])
		}
		for k, c := range rec.costs {
			if !hungarianForbidden(c) && !(c >= 0) {
				t.Errorf("misses=%d: pairing %v costs %v", misses, k, c)
			}
		}
	}
}

// Set together in Go, the robust term replaces the Gaussian one rather than
// adding to it, so the cost never counts ln det S twice. The replay refuses
// the pair; this is the defined behaviour for a caller that does not.
func TestRobustLikelihoodCostReplacesTheGaussianTerm(t *testing.T) {
	both, clusters := outlierTracks(func(c *TrackerConfig) {
		c.LikelihoodAssociationCost = true
		c.RobustLikelihoodAssociationCost = true
	})
	robust, _ := outlierTracks(func(c *TrackerConfig) { c.RobustLikelihoodAssociationCost = true })
	a, b := newCostRecorder(), newCostRecorder()
	both.DebugCollector, robust.DebugCollector = a, b
	both.associate(clusters, 0.1)
	robust.associate(clusters, 0.1)
	for k, c := range b.costs {
		if a.costs[k] != c {
			t.Errorf("pairing %v costs %v with both options, %v with the robust one alone", k, a.costs[k], c)
		}
	}
}

// ν reaches the cost: a lighter tail charges the outlier more.
func TestRobustLikelihoodNuReachesTheCost(t *testing.T) {
	cost := func(nu float64) float32 {
		tk, clusters := outlierTracks(func(c *TrackerConfig) {
			c.RobustLikelihoodAssociationCost = true
			c.RobustLikelihoodNu = nu
		})
		rec := newCostRecorder()
		tk.DebugCollector = rec
		tk.associate(clusters, 0.1)
		return rec.costs[costKey{2, "b"}] - rec.costs[costKey{1, "a"}]
	}
	if cost(0) != cost(likelihood.DefaultStudentTNu) {
		t.Error("RobustLikelihoodNu zero is not the default")
	}
	if !(cost(30) > cost(4)) || !(cost(4) > cost(1)) {
		t.Errorf("the 10σ pair's excess does not fall as ν falls: ν=30 %v, ν=4 %v, ν=1 %v", cost(30), cost(4), cost(1))
	}
}
