package l5tracks

import "testing"

func extentTracker(weight float32) *Tracker {
	c := DefaultTrackerConfig()
	c.AssociationExtentCostWeight = weight
	return NewTracker(c)
}

// A track with a belief, so the compatibility term has something to compare to.
func believingTrack(longExtent float32) *TrackedObject {
	tr := &TrackedObject{}
	tr.BoundingBoxLengthAvg = longExtent
	tr.BoundingBoxWidthAvg = longExtent / 2
	tr.ObservationCount = 10
	return tr
}

func clusterOfExtent(l, w float32) WorldCluster {
	c := elongatedCluster(0, 0, 0)
	c.BoundingBoxLength, c.BoundingBoxWidth = l, w
	c.OBB.Length, c.OBB.Width = l, w
	return c
}

// Occlusion produces a short cluster on every pass, so a partial view must not
// be treated as harshly as a cluster that is too big to explain.
func TestExtentCostIsAsymmetric(t *testing.T) {
	tk := extentTracker(1)
	tr := believingTrack(4.5)

	half := tk.extentCompatibilityCost(tr, clusterOfExtent(2.25, 1.0))
	double := tk.extentCompatibilityCost(tr, clusterOfExtent(9.0, 4.0))

	if half <= 0 {
		t.Fatal("a partial view carried no cost at all; a scrap would then outbid nothing")
	}
	if half >= double {
		t.Fatalf("shortfall %.3f is not cheaper than the same excess %.3f", half, double)
	}
}

// Position must remain the dominant term: the shape cost may nudge an
// assignment, never decide it on its own.
func TestExtentCostStaysBelowTheGate(t *testing.T) {
	tk := extentTracker(1000) // absurd weight on purpose
	tr := believingTrack(4.5)

	for _, c := range []WorldCluster{clusterOfExtent(0.05, 0.04), clusterOfExtent(15, 8)} {
		got := tk.extentCompatibilityCost(tr, c)
		if limit := tk.Config.GatingDistanceSquared * associationExtentCostCap; float64(got) > float64(limit)+1e-6 {
			t.Fatalf("cost %.3f exceeded the cap %.3f even at an absurd weight", got, limit)
		}
	}
}

func TestExtentCostOffByDefault(t *testing.T) {
	if w := DefaultTrackerConfig().AssociationExtentCostWeight; w != 0 {
		t.Fatalf("default weight = %v, want 0 so association changes stay measurable separately", w)
	}
	tk := extentTracker(0)
	if got := tk.extentCompatibilityCost(believingTrack(4.5), clusterOfExtent(0.1, 0.1)); got != 0 {
		t.Fatalf("cost = %v with the term disabled, want 0", got)
	}
}

// Nothing to compare against is not evidence of incompatibility.
func TestExtentCostNeedsBothSides(t *testing.T) {
	tk := extentTracker(1)
	if got := tk.extentCompatibilityCost(&TrackedObject{}, clusterOfExtent(4.5, 1.9)); got != 0 {
		t.Fatalf("a track with no belief was charged %v", got)
	}
	if got := tk.extentCompatibilityCost(believingTrack(4.5), clusterOfExtent(0, 0)); got != 0 {
		t.Fatalf("a cluster reporting no extent was charged %v", got)
	}
}

// The extent belief supersedes the running average once it has one, because
// the average is the quantity a fragment corrupts.
func TestExtentCostPrefersTheBelief(t *testing.T) {
	tr := believingTrack(1.0) // running average says small
	for i := 0; i < 5; i++ {
		tr.lengthBelief.Observe(4.5)
		tr.widthBelief.Observe(1.9)
	}
	if got := tr.believedLongExtent(); !nearBelief(got, 4.5) {
		t.Fatalf("believed extent = %.2f, want the belief (4.5) not the average (1.0)", got)
	}
}

// The point of the change: a scrap that the hard guard would have refused can
// still be associated, so it does not go off and seed a second track on the
// same vehicle.
func TestSoftCostAdmitsTheScrapTheHardGuardRefused(t *testing.T) {
	scrap := clusterOfExtent(0.11, 0.08)

	hard := extentTracker(0)
	tr := believingTrack(4.5)
	tr.TrackID = "car"
	hard.Tracks["car"] = tr
	if !hard.isFragmentFor(tr, scrap) {
		t.Fatal("the hard guard no longer recognises the scrap; this test is not measuring what it claims")
	}
	if got := hard.associate([]WorldCluster{scrap}, .1); got[0] != "" {
		t.Fatalf("hard guard associated the scrap to %q", got[0])
	}

	soft := extentTracker(1)
	tr2 := believingTrack(4.5)
	tr2.TrackID = "car"
	soft.Tracks["car"] = tr2
	if got := soft.associate([]WorldCluster{scrap}, .1); got[0] != "car" {
		t.Fatalf("soft cost refused the scrap (%q); it will now seed a duplicate track", got[0])
	}
}

// ...but a complete cluster still outbids a scrap for the same track.
func TestCompleteClusterOutbidsAScrap(t *testing.T) {
	tk := extentTracker(1)
	tr := believingTrack(4.5)
	scrapCost := tk.extentCompatibilityCost(tr, clusterOfExtent(0.11, 0.08))
	fullCost := tk.extentCompatibilityCost(tr, clusterOfExtent(4.4, 1.9))
	if fullCost >= scrapCost {
		t.Fatalf("full cluster cost %.3f is not cheaper than the scrap's %.3f", fullCost, scrapCost)
	}
}
