package l5tracks

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Gap S3 in data/maths/paper-implementation-gap-analysis.md: the assignment
// cost is the bare squared Mahalanobis distance, so when two tracks compete for
// one cluster the track with the larger innovation covariance pays less for the
// same miss. DeepSORT names this as the reason for its matching cascade. Here
// it is amplified on purpose: every missed frame adds OcclusionCovInflation to
// a coasting track's position variance, which widens its gate and, by the same
// arithmetic, discounts its bid.
//
// This is not the S2 case that CascadedAssociation addresses. S2 is a tentative
// track outbidding a confirmed one. S3 is between two confirmed tracks, so a
// confirmed-first cascade puts both bidders in the same stage and changes
// nothing; the tests below pin that too. It is also why the cascade cannot
// explain the campaign's hits_to_confirm=1 result: at 1 every track is
// confirmed on its first hit and the cascade has a single stage.
//
// These tests pin the shipped behaviour so the suite stays green. When the
// cost is fixed, the *ShippedCost* assertions flip; that flip is the evidence.

const s3FramePeriod = 100 * time.Millisecond

// s3Cluster is a pedestrian-sized cluster. Small on purpose: the fragment
// guard only protects tracks that believe they are metre-scale, and it must
// stay out of this measurement.
func s3Cluster(x, y float32) WorldCluster {
	return WorldCluster{
		CentroidX: x, CentroidY: y,
		BoundingBoxLength: 0.6, BoundingBoxWidth: 0.5, BoundingBoxHeight: 1.7,
		PointsCount: 40,
		OBB: &l4perception.OrientedBoundingBox{
			CenterX: x, CenterY: y, Length: 0.6, Width: 0.5, Height: 1.7,
		},
	}
}

// s3Scene runs the real Update path: two stationary objects `separation`
// metres apart are tracked to steady state, then the second goes unobserved
// for `misses` frames while the first keeps being updated. It returns the
// tracker with both tracks predicted into the next frame, which is the state
// associate() sees, and the two track IDs.
func s3Scene(t *testing.T, cfg TrackerConfig, separation float32, misses int) (tk *Tracker, freshID, coastingID string) {
	t.Helper()
	tk = NewTracker(cfg)
	now := time.Unix(1_700_000_000, 0)

	fresh, coasting := s3Cluster(10, 0), s3Cluster(10, separation)
	for i := 0; i < 60; i++ {
		tk.Update([]WorldCluster{fresh, coasting}, now)
		now = now.Add(s3FramePeriod)
	}
	for i := 0; i < misses; i++ {
		tk.Update([]WorldCluster{fresh}, now)
		now = now.Add(s3FramePeriod)
	}

	for id, track := range tk.Tracks {
		if track.TrackState != TrackConfirmed {
			continue
		}
		switch {
		case track.Misses == 0:
			freshID = id
		case track.Misses == misses:
			coastingID = id
		}
	}
	if freshID == "" || coastingID == "" {
		t.Fatalf("scene did not produce one fresh and one coasting confirmed track (misses=%d): %d tracks", misses, len(tk.Tracks))
	}

	// Step 1 of Update for the contested frame, so P is the predicted P.
	tk.LastUpdateNanos = now.UnixNano()
	for _, track := range tk.Tracks {
		if track.TrackState != TrackDeleted {
			tk.predict(track, float32(s3FramePeriod.Seconds()))
		}
	}
	return tk, freshID, coastingID
}

// s3Costs returns, for one track and cluster, the shipped cost d² and the
// log-determinant of the innovation covariance S = HPHᵀ + R that a likelihood
// cost would add to it.
func s3Costs(tk *Tracker, id string, c WorldCluster) (d2, lnDetS float64) {
	track := tk.Tracks[id]
	d2 = float64(tk.mahalanobisDistanceSquared(track, c, float32(s3FramePeriod.Seconds())))
	r := float64(tk.Config.MeasurementNoise)
	s00, s11 := float64(track.P[0*4+0])+r, float64(track.P[1*4+1])+r
	s01 := float64(track.P[0*4+1])
	return d2, math.Log(s00*s11 - s01*s01)
}

func TestShippedCostGivesAnEquidistantClusterToTheCoastingTrack(t *testing.T) {
	const separation = 1.2
	for _, misses := range []int{1, 3, 5} {
		tk, freshID, coastingID := s3Scene(t, DefaultTrackerConfig(), separation, misses)
		contested := s3Cluster(10, separation/2) // 0.6 m from each prediction

		freshD2, _ := s3Costs(tk, freshID, contested)
		coastD2, _ := s3Costs(tk, coastingID, contested)
		if coastD2 >= freshD2 {
			t.Fatalf("misses=%d: coasting d²=%.3f is not below fresh d²=%.3f; the covariance discount S3 describes is gone",
				misses, coastD2, freshD2)
		}

		got := tk.associate([]WorldCluster{contested}, float32(s3FramePeriod.Seconds()))
		if got[0] != coastingID {
			t.Fatalf("misses=%d: equidistant cluster went to the fresh track (fresh d²=%.3f, coasting d²=%.3f). "+
				"This test pins the shipped S3 defect; if the cost was fixed, flip it", misses, freshD2, coastD2)
		}
		t.Logf("misses=%d: fresh d²=%.3f, coasting d²=%.3f (%.1fx cheaper for the same 0.6 m miss)",
			misses, freshD2, coastD2, freshD2/coastD2)
	}
}

// The sharper form: the cluster is twice as far from the coasting track as
// from the fresh one, and after a single missed frame the coasting track
// still takes it.
func TestShippedCostLetsACoastingTrackWinFromTwiceAsFarAway(t *testing.T) {
	const separation = 1.2
	tk, freshID, coastingID := s3Scene(t, DefaultTrackerConfig(), separation, 1)
	contested := s3Cluster(10, 0.4) // 0.4 m from fresh, 0.8 m from coasting

	got := tk.associate([]WorldCluster{contested}, float32(s3FramePeriod.Seconds()))
	if got[0] != coastingID {
		freshD2, _ := s3Costs(tk, freshID, contested)
		coastD2, _ := s3Costs(tk, coastingID, contested)
		t.Fatalf("cluster 0.4 m from the fresh track and 0.8 m from the coasting one went to the fresh track "+
			"(fresh d²=%.3f, coasting d²=%.3f). This test pins the shipped S3 defect; if the cost was fixed, flip it",
			freshD2, coastD2)
	}
}

// CascadedAssociation orders by confirmation state. Both S3 bidders are
// confirmed, so it must not change the outcome, and an evaluation of the
// cascade must not be read as an evaluation of S3.
func TestConfirmedFirstCascadeDoesNotAddressTheCovarianceBias(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.CascadedAssociation = true
	const separation = 1.2
	for _, misses := range []int{1, 3, 5} {
		tk, _, coastingID := s3Scene(t, cfg, separation, misses)
		contested := s3Cluster(10, separation/2)
		got := tk.associate([]WorldCluster{contested}, float32(s3FramePeriod.Seconds()))
		if got[0] != coastingID {
			t.Fatalf("misses=%d: with CascadedAssociation on the equidistant cluster went to the fresh track; "+
				"the cascade now orders by something other than confirmation state, so update S3's tests and row", misses)
		}
	}
}

// The remedy's arithmetic, checked against the real covariances rather than
// the gap analysis's hand computation: a likelihood cost d² + ln|S| charges a
// vague track for its vagueness, and that is enough to reverse both cases
// above. Nothing in production computes this yet.
func TestLikelihoodCostWouldReverseTheCovarianceBias(t *testing.T) {
	const separation = 1.2
	cases := []struct {
		name   string
		y      float32
		misses int
	}{
		{"equidistant, 1 miss", 0.6, 1},
		{"equidistant, 3 misses", 0.6, 3},
		{"equidistant, 5 misses", 0.6, 5},
		{"twice as far from the coasting track, 1 miss", 0.4, 1},
	}
	for _, tc := range cases {
		tk, freshID, coastingID := s3Scene(t, DefaultTrackerConfig(), separation, tc.misses)
		contested := s3Cluster(10, tc.y)
		freshD2, freshLn := s3Costs(tk, freshID, contested)
		coastD2, coastLn := s3Costs(tk, coastingID, contested)

		if freshD2+freshLn >= coastD2+coastLn {
			t.Errorf("%s: likelihood cost still favours the coasting track: fresh %.3f%+.3f=%.3f, coasting %.3f%+.3f=%.3f",
				tc.name, freshD2, freshLn, freshD2+freshLn, coastD2, coastLn, coastD2+coastLn)
		}
		t.Logf("%s: d² fresh %.3f vs coasting %.3f; with ln|S| fresh %.3f vs coasting %.3f",
			tc.name, freshD2, coastD2, freshD2+freshLn, coastD2+coastLn)
	}
}

// A likelihood cost must not overcorrect: when the fresh track is clearly the
// wrong owner, the coasting track has to be able to get its object back. The
// object reappears exactly where the coasting track predicted it, 1.2 m from
// the fresh track.
func TestLikelihoodCostStillLetsACoastingTrackReacquireItsOwnObject(t *testing.T) {
	const separation = 1.2
	for _, misses := range []int{1, 3, 5} {
		tk, freshID, coastingID := s3Scene(t, DefaultTrackerConfig(), separation, misses)
		reappeared := s3Cluster(10, separation)
		freshD2, freshLn := s3Costs(tk, freshID, reappeared)
		coastD2, coastLn := s3Costs(tk, coastingID, reappeared)
		if coastD2+coastLn >= freshD2+freshLn {
			t.Errorf("misses=%d: likelihood cost would hand a reappearing object to the wrong track: "+
				"coasting %.3f, fresh %.3f", misses, coastD2+coastLn, freshD2+freshLn)
		}
	}
}

// With LikelihoodAssociationCost on, the production assignment (not just the
// arithmetic above) gives every pinned contested cluster to the fresh track.
func TestLikelihoodAssociationCostGivesContestedClustersToTheFreshTrack(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.LikelihoodAssociationCost = true
	const separation = 1.2
	cases := []struct {
		name   string
		y      float32
		misses int
	}{
		{"equidistant, 1 miss", 0.6, 1},
		{"equidistant, 3 misses", 0.6, 3},
		{"equidistant, 5 misses", 0.6, 5},
		{"twice as far from the coasting track, 1 miss", 0.4, 1},
	}
	for _, tc := range cases {
		tk, freshID, _ := s3Scene(t, cfg, separation, tc.misses)
		got := tk.associate([]WorldCluster{s3Cluster(10, tc.y)}, float32(s3FramePeriod.Seconds()))
		if got[0] != freshID {
			t.Errorf("%s: contested cluster went to the coasting track with the likelihood cost on", tc.name)
		}
	}
}

// The option must not cost a coasting track its own object: a cluster that
// reappears where the coasting track predicted it, 1.2 m from the fresh track,
// goes back to the coasting track, and each track keeps its own cluster when
// both are present.
func TestLikelihoodAssociationCostStillReacquires(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.LikelihoodAssociationCost = true
	const separation = 1.2
	dt := float32(s3FramePeriod.Seconds())
	for _, misses := range []int{1, 3, 5} {
		tk, freshID, coastingID := s3Scene(t, cfg, separation, misses)
		if got := tk.associate([]WorldCluster{s3Cluster(10, separation)}, dt); got[0] != coastingID {
			t.Errorf("misses=%d: a reappearing object went to %q, want the coasting track that predicted it", misses, got[0])
		}
		both := []WorldCluster{s3Cluster(10, 0), s3Cluster(10, separation)}
		if got := tk.associate(both, dt); got[0] != freshID || got[1] != coastingID {
			t.Errorf("misses=%d: with both objects present got %v, want [fresh coasting]", misses, got)
		}
	}
}

// The gate is d² with or without the option: the likelihood term may reorder
// admitted pairings, never admit or forbid one.
func TestLikelihoodAssociationCostLeavesTheGateAlone(t *testing.T) {
	const separation = 1.2
	dt := float32(s3FramePeriod.Seconds())
	for _, on := range []bool{false, true} {
		cfg := DefaultTrackerConfig()
		cfg.LikelihoodAssociationCost = on
		tk, freshID, coastingID := s3Scene(t, cfg, separation, 1)
		// 3.2 m from the fresh track is outside its gate (about 1.7 m); 2 m
		// from the coasting track is inside its inflated one (about 4.6 m)
		// and inside the implied-speed guard, which at a 100 ms frame period
		// binds at 3 m, before MaxPositionJumpMetres does. Admission is
		// decided by d² and the guards alone, so the answer cannot depend on
		// the option.
		got := tk.associate([]WorldCluster{s3Cluster(10, separation+2)}, dt)
		if got[0] == freshID {
			t.Errorf("option=%v: a cluster 3.2 m from the fresh track passed its d² gate", on)
		}
		if got[0] != coastingID {
			t.Errorf("option=%v: a cluster inside only the coasting track's gate went to %q", on, got[0])
		}
	}
}

func TestLikelihoodAssociationCostOffByDefault(t *testing.T) {
	if DefaultTrackerConfig().LikelihoodAssociationCost {
		t.Fatal("LikelihoodAssociationCost defaults on; it must be measured against the shipped cost first")
	}
}
