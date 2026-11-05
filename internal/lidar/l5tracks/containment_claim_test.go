package l5tracks

import (
	"math"
	"math/rand"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The containment claim (W5, l5id0011) is tested in S3's own scene
// (association_covariance_bias_test.go): two confirmed tracks 1.2 m apart,
// the second coasting, so the shipped cost gives a contested cluster to the
// coasting track. Each track is given a body: an oriented box referenced to
// its centre at its predicted position. The contested cluster's returns lie
// on the fresh track's body, where its box holds them and the coasting
// track's does not.

func claimConfig() TrackerConfig {
	cfg := DefaultTrackerConfig()
	cfg.SolidBody = SolidBodyOptions{Enabled: true, OriginSource: syntheticOrigin, Containment: true}
	cfg.ContainmentClaim = true
	return cfg
}

// giveBody places a track's solid body on its predicted position, along +X,
// referenced to the body centre, as an established body would be.
func giveBody(tk *Tracker, id string) {
	track := tk.Tracks[id]
	sb := &track.solidBody
	sb.state[0], sb.state[1] = track.X, track.Y
	sb.orientation = OrientationBelief{PsiRad: 0, Provenance: ProvenanceAccumulated}
	sb.reference = ReferenceBodyCentre
}

// returnsAt is a tight cloud of 40 returns about (x, y): any box centred
// there holds it, and any box whose edge is a metre away holds none of it.
func returnsAt(x, y float64) []l4perception.WorldPoint {
	var points []l4perception.WorldPoint
	for i := 0; i < 40; i++ {
		points = append(points, l4perception.WorldPoint{X: x - 0.08 + 0.16*float64(i%8)/7, Y: y - 0.08 + 0.16*float64(i/8)/4})
	}
	return points
}

// claimScene is s3Scene with both tracks given bodies, and a cluster contested
// equidistantly whose returns lie at (px, py).
func claimScene(t *testing.T, cfg TrackerConfig, misses int, px, py float64) (*Tracker, string, string, WorldCluster) {
	t.Helper()
	const separation = 1.2
	tk, freshID, coastingID := s3Scene(t, cfg, separation, misses)
	giveBody(tk, freshID)
	giveBody(tk, coastingID)
	contested := s3Cluster(10, separation/2)
	contested.RetainedPoints = returnsAt(px, py)
	return tk, freshID, coastingID, contested
}

func associateOne(tk *Tracker, c WorldCluster) string {
	return tk.associate([]WorldCluster{c}, float32(s3FramePeriod.Seconds()))[0]
}

// The case the rule exists for: the coasting track bids cheaper, but its box
// cannot hold the returns and the fresh track's can, so the fresh track takes
// them. Without the option the same scene goes to the coasting track (S3).
func TestContainmentClaimGivesTheClusterToTheBoxThatHoldsIt(t *testing.T) {
	for _, misses := range []int{1, 3, 5} {
		off := claimConfig()
		off.ContainmentClaim = false
		tk, _, coastingID, contested := claimScene(t, off, misses, 10, 0)
		if got := associateOne(tk, contested); got != coastingID {
			t.Fatalf("misses=%d: without the claim the contested cluster went to %q, not the coasting track; the scene no longer shows S3", misses, got)
		}

		tk, freshID, _, contested := claimScene(t, claimConfig(), misses, 10, 0)
		if got := associateOne(tk, contested); got != freshID {
			t.Fatalf("misses=%d: with the claim the cluster went to %q, want the fresh track %q whose box holds it", misses, got, freshID)
		}
		if n := tk.ContinuityStats().ContainmentClaimRefusals; n != 1 {
			t.Fatalf("misses=%d: %d refusals, want the coasting track's one", misses, n)
		}
	}
}

// A sole bidder keeps its cluster. This local guard does not guarantee the
// cardinality of a multi-cluster assignment, as the following test shows.
func TestContainmentClaimLeavesASoleBidderItsCluster(t *testing.T) {
	tk, freshID, coastingID, contested := claimScene(t, claimConfig(), 1, 10, 0)
	tk.Tracks[freshID].TrackState = TrackDeleted
	if got := associateOne(tk, contested); got != coastingID {
		t.Fatalf("the only bidder lost the cluster to %q", got)
	}
	if n := tk.ContinuityStats().ContainmentClaimRefusals; n != 0 {
		t.Fatalf("%d refusals with one bidder", n)
	}
}

// A row retaining a valid bidder is insufficient to preserve the global
// matching: the fresh track is also the second cluster's only bidder. This
// counterexample documents the disabled experiment's known limitation; it
// does not assert that a particular replay's new births have this cause.
func TestContainmentClaimLocalBidderCanLoseGlobalAssignment(t *testing.T) {
	tk, freshID, coastingID, contested := claimScene(t, claimConfig(), 1, 10, 0)
	clusters := []WorldCluster{contested, contested}
	clusters[1].ClusterID++
	costs := [][]float32{{.1, 1}, {float32(hungarianlnf), .2}}
	count := func(assignment []int) int {
		n := 0
		for _, col := range assignment {
			if col >= 0 {
				n++
			}
		}
		return n
	}
	if count(HungarianAssign(costs)) != 2 {
		t.Fatal("counterexample must start with a complete matching")
	}
	tk.applyContainmentClaim(costs, clusters, []int{0, 1}, []string{coastingID, freshID})
	for _, row := range costs {
		if row[1] >= float32(hungarianlnf) {
			t.Fatal("each row must retain the fresh bidder")
		}
	}
	if got := count(HungarianAssign(costs)); got != 1 {
		t.Fatalf("local refusal left %d assignments, want the demonstrated global shortfall of one", got)
	}
}

// Where no bidder's box holds the returns, nothing is refused and the shipped
// cost decides, as it does with the option off.
func TestContainmentClaimRefusesNothingWhenNoBoxHolds(t *testing.T) {
	tk, _, coastingID, contested := claimScene(t, claimConfig(), 1, 10, 6)
	if got := associateOne(tk, contested); got != coastingID {
		t.Fatalf("with no box holding the returns the cluster went to %q, not where the shipped cost sends it", got)
	}
	if n := tk.ContinuityStats().ContainmentClaimRefusals; n != 0 {
		t.Fatalf("%d refusals where no box holds the returns", n)
	}
}

// The rule reads the contained box: without Containment, a body referenced
// to its centre, or enough returns to judge, it refuses nothing.
func TestContainmentClaimNeedsAContainedBodyCentredBoxAndReturns(t *testing.T) {
	noContainment := claimConfig()
	noContainment.SolidBody.Containment = false
	tk, _, coastingID, contested := claimScene(t, noContainment, 1, 10, 0)
	if got := associateOne(tk, contested); got != coastingID {
		t.Fatalf("without Containment the claim changed the assignment to %q", got)
	}

	tk, _, coastingID, contested = claimScene(t, claimConfig(), 1, 10, 0)
	tk.Tracks[coastingID].solidBody.reference = ReferenceClusterMedoid
	if got := associateOne(tk, contested); got != coastingID {
		t.Fatalf("a medoid-referenced box was judged, and the cluster went to %q", got)
	}

	tk, _, coastingID, contested = claimScene(t, claimConfig(), 1, 10, 0)
	contested.RetainedPoints = contested.RetainedPoints[:DefaultMinFaceSupport-1]
	if got := associateOne(tk, contested); got != coastingID {
		t.Fatalf("too few returns to judge, yet the cluster went to %q", got)
	}
	if n := tk.ContinuityStats().ContainmentClaimRefusals; n != 0 {
		t.Fatalf("%d refusals with too few returns", n)
	}
}

// A track the gate has already refused is not a rival: with the fresh track
// moved out of reach, the coasting track is the only admitted bidder and keeps
// the cluster although its box cannot hold the returns.
func TestContainmentClaimCountsOnlyAdmittedBidders(t *testing.T) {
	tk, freshID, coastingID, contested := claimScene(t, claimConfig(), 1, 10, 0)
	fresh := tk.Tracks[freshID]
	fresh.X, fresh.Y = 100, 100
	giveBody(tk, freshID)
	if got := associateOne(tk, contested); got != coastingID {
		t.Fatalf("with the fresh track gated out the cluster went to %q, not the only admitted bidder", got)
	}
	if n := tk.ContinuityStats().ContainmentClaimRefusals; n != 0 {
		t.Fatalf("%d refusals counted against a gated-out rival", n)
	}
}

func TestContainmentClaimCardinalityGuardRestoresGlobalCounterexample(t *testing.T) {
	cfg := claimConfig()
	cfg.ContainmentClaimCardinalityGuard = true
	tk, fresh, coast, cluster := claimScene(t, cfg, 1, 10, 0)
	costs := [][]float32{{.1, 1}, {forbid, .2}}
	tk.applyContainmentClaim(costs, []WorldCluster{cluster, cluster}, []int{0, 1}, []string{coast, fresh})
	if pairs, _ := bruteAssignment(costs); pairs != 2 || costs[0][0] != .1 {
		t.Fatalf("guard lost original matching: %v", costs)
	}
	stats := tk.ContinuityStats()
	if stats.ContainmentClaimRefusals != 0 || stats.ContainmentClaimGuardFallbacks != 1 {
		t.Fatalf("restored edges counted as refusals: %+v", stats)
	}
}

func TestContainmentClaimCardinalityGuardKeepsUsefulRefusals(t *testing.T) {
	cfg := claimConfig()
	cfg.ContainmentClaimCardinalityGuard = true
	tk, fresh, coast, cluster := claimScene(t, cfg, 1, 10, 0)
	costs := [][]float32{{.1, 1}}
	tk.applyContainmentClaim(costs, []WorldCluster{cluster}, []int{0}, []string{coast, fresh})
	if costs[0][0] != forbid || costs[0][1] != 1 {
		t.Fatalf("useful refusal not retained: %v", costs)
	}
	stats := tk.ContinuityStats()
	if stats.ContainmentClaimRefusals != 1 || stats.ContainmentClaimGuardFallbacks != 0 {
		t.Fatalf("incorrect accepted refusal counters: %+v", stats)
	}
}

// Enumerate all sparse four-by-two graphs, with two distinct admitted costs.
// An independent brute-force assignment oracle checks maximum cardinality,
// including graphs with no complete matching and extra clusters (birth risk).
func TestContainmentClaimCardinalityGuardSparseGraphs(t *testing.T) {
	cfg := claimConfig()
	cfg.ContainmentClaimCardinalityGuard = true
	tk, fresh, coast, cluster := claimScene(t, cfg, 1, 10, 0)
	values := []float32{forbid, .1, 1}
	for n := 1; n <= 4; n++ {
		clusters := make([]WorldCluster, n)
		indices := make([]int, n)
		for i := range clusters {
			clusters[i] = cluster
			indices[i] = i
		}
		cases := 1
		for i := 0; i < 2*n; i++ {
			cases *= len(values)
		}
		for code := 0; code < cases; code++ {
			original := make([][]float32, n)
			costs := make([][]float32, n)
			remaining := code
			for i := range costs {
				original[i] = make([]float32, 2)
				for j := range original[i] {
					original[i][j] = values[remaining%len(values)]
					remaining /= len(values)
				}
				costs[i] = append([]float32(nil), original[i]...)
			}
			before, _ := bruteAssignment(original)
			tk.applyContainmentClaim(costs, clusters, indices, []string{coast, fresh})
			after, _ := bruteAssignment(costs)
			if after != before {
				t.Fatalf("cardinality %d -> %d: %v -> %v", before, after, original, costs)
			}
			for i := range costs {
				for j, v := range costs[i] {
					if v != original[i][j] && v != forbid {
						t.Fatalf("original cost changed: %v -> %v", original, costs)
					}
				}
			}
		}
	}
}

func TestContainmentClaimCardinalityGuardRectangularNonFiniteAndRepeat(t *testing.T) {
	cfg := claimConfig()
	cfg.ContainmentClaimCardinalityGuard = true
	tk, fresh, coast, cluster := claimScene(t, cfg, 1, 10, 0)
	rng := rand.New(rand.NewSource(180018))
	for trial := 0; trial < 1000; trial++ {
		n := 1 + rng.Intn(4)
		original := randomCostMatrix(rng, n, 3, trial%4)
		clusters := make([]WorldCluster, n)
		indices := make([]int, n)
		for i := range clusters {
			clusters[i] = cluster
			indices[i] = i
		}
		before, _ := bruteAssignment(original)
		var first [][]float32
		for repeat := 0; repeat < 2; repeat++ {
			costs := make([][]float32, n)
			for i := range costs {
				costs[i] = append([]float32(nil), original[i]...)
			}
			tk.applyContainmentClaim(costs, clusters, indices, []string{coast, fresh, "unconfirmed-bidder"})
			after, _ := bruteAssignment(costs)
			if before != after {
				t.Fatalf("trial %d: matching size %d -> %d", trial, before, after)
			}
			for i := range costs {
				for j, v := range costs[i] {
					if math.Float32bits(v) != math.Float32bits(original[i][j]) && v != forbid {
						t.Fatal("modified admitted cost or original forbidden edge")
					}
					if repeat == 1 && math.Float32bits(v) != math.Float32bits(first[i][j]) {
						t.Fatal("guard was not repeatable")
					}
				}
			}
			first = costs
		}
	}
	tk.applyContainmentClaim(nil, nil, nil, nil)
}
