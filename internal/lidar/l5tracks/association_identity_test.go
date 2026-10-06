package l5tracks

import "testing"

// Gap K4 in data/maths/paper-implementation-gap-analysis.md: association is
// position-first, and the fragment guard covers one direction only (a scrap
// under 0.5 m against a metre-scale belief). A pedestrian-sized cluster beside
// a car-sized track, or the reverse, is decided by position alone. This is
// the soft extent-compatibility cost's first evaluation on that scenario,
// run at the shipped weight (0) and at 1.

// unlikeNeighbours builds a car track at the origin and a pedestrian track
// 1 m along +X, then offers a car-sized cluster 0.6 m along +X and a
// pedestrian-sized cluster 0.4 m along +X. On position alone the cheaper
// pairing is the swap: each cluster is nearer the other object's track.
func unlikeNeighbours(weight float32) (*Tracker, []WorldCluster) {
	tk := extentTracker(weight)

	car := believingTrack(4.5)
	car.TrackID, car.CreationSequence = "car", 1
	car.TrackState = TrackConfirmed
	ped := believingTrack(0.6)
	ped.TrackID, ped.CreationSequence = "ped", 2
	ped.TrackState = TrackConfirmed
	ped.X = 1.0
	tk.Tracks["car"], tk.Tracks["ped"] = car, ped

	carCluster := clusterOfExtent(4.5, 1.9)
	carCluster.CentroidX, carCluster.OBB.CenterX = 0.6, 0.6
	pedCluster := clusterOfExtent(0.6, 0.3)
	pedCluster.CentroidX, pedCluster.OBB.CenterX = 0.4, 0.4
	return tk, []WorldCluster{carCluster, pedCluster}
}

func TestUnlikeNeighboursSwapIdentityOnPositionAlone(t *testing.T) {
	tk, clusters := unlikeNeighbours(0)
	if tk.isFragmentFor(tk.Tracks["car"], clusters[1]) {
		t.Fatal("the 0.6 m cluster trips the fragment guard; this scenario is meant to sit outside it")
	}
	got := tk.associate(clusters, 0.1)
	if got[0] != "ped" || got[1] != "car" {
		t.Fatalf("associations = %v; the shipped position-only cost was expected to swap "+
			"the identities here, which is the defect K4 records", got)
	}
}

func TestExtentCostKeepsUnlikeNeighboursApart(t *testing.T) {
	tk, clusters := unlikeNeighbours(1)
	got := tk.associate(clusters, 0.1)
	if got[0] != "car" || got[1] != "ped" {
		t.Fatalf("associations = %v with the extent cost at weight 1, want [car ped]", got)
	}
}
