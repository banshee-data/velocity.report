package l5tracks

import (
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

// A sole bidder keeps its cluster: the rule chooses between claimants and
// never leaves a cluster unassigned that a track would have taken.
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
