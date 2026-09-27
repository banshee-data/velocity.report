package l5tracks

import "testing"

// Tests for the Sprint 0.5.2.2 identity options in identity.go and
// continuity.go: TentativePriority (gap analysis S2), ClassIdentity (K4) and
// ContestedRejoin. Each pins the option's decision on a hand-built frame, the
// shipped decision it changes, and that it is off by default.

func TestIdentityOptionsOffByDefault(t *testing.T) {
	c := DefaultTrackerConfig()
	if c.TentativePriority || c.ClassIdentity || c.OcclusionContinuity.ContestedRejoin {
		t.Fatalf("an identity option defaults on: TentativePriority=%v ClassIdentity=%v ContestedRejoin=%v",
			c.TentativePriority, c.ClassIdentity, c.OcclusionContinuity.ContestedRejoin)
	}
	if DefaultOcclusionContinuity().ContestedRejoin {
		t.Fatal("ContestedRejoin is part of DefaultOcclusionContinuity; it is identity work, measured separately")
	}
}

// S2 as the gap analysis specifies it (cascadeScene): with TentativePriority
// the confirmed track coasting through one miss takes the cluster inside its
// 99% ellipse, where the joint assignment gives it to the newborn.
func TestTentativePriorityGivesAConfirmedTrackTheClusterInsideItsEllipse(t *testing.T) {
	tk, clusters := cascadeScene(false)
	tk.Config.TentativePriority = true
	if d2 := tk.mahalanobisDistanceSquared(tk.Tracks["confirmed"], clusters[0], 0.1); d2 > tentativePriorityGate {
		t.Fatalf("scene precondition: confirmed d²=%.2f is outside the 99%% ellipse", d2)
	}
	got := tk.associate(clusters, 0.1)
	if got[0] != "confirmed" {
		t.Fatalf("TentativePriority gave the cluster to %q, want the confirmed track", got[0])
	}
}

// The difference from CascadedAssociation: a confirmed track whose object
// dropped out does not take a newborn's cluster from outside its 99% ellipse.
// The cascade gives confirmed tracks first choice anywhere inside the shipped
// gate, so it does.
func TestTentativePriorityLeavesANewbornItsClusterBeyondTheEllipse(t *testing.T) {
	tk, _ := cascadeScene(false)
	cluster := elongatedCluster(0.30, 0, 0) // at the tentative track, 0.30 m from the confirmed
	confirmedD2 := tk.mahalanobisDistanceSquared(tk.Tracks["confirmed"], cluster, 0.1)
	// Move the tentative track and its cluster out until the confirmed track's
	// d² lies between the 99% ellipse and the shipped gate.
	for x := float32(0.30); confirmedD2 <= tentativePriorityGate && x < 3; x += 0.05 {
		tk.Tracks["tentative"].X = x
		cluster = elongatedCluster(x, 0, 0)
		confirmedD2 = tk.mahalanobisDistanceSquared(tk.Tracks["confirmed"], cluster, 0.1)
	}
	if confirmedD2 <= tentativePriorityGate || confirmedD2 > tk.Config.GatingDistanceSquared {
		t.Fatalf("scene precondition: confirmed d²=%.2f is not between %.2f and the gate %.2f",
			confirmedD2, tentativePriorityGate, tk.Config.GatingDistanceSquared)
	}

	tk.Config.CascadedAssociation = true
	if got := tk.associate([]WorldCluster{cluster}, 0.1); got[0] != "confirmed" {
		t.Fatalf("scene precondition: the cascade gave the cluster to %q, want the confirmed track", got[0])
	}
	// TentativePriority supersedes the cascade when both are set.
	tk.Config.TentativePriority = true
	if got := tk.associate([]WorldCluster{cluster}, 0.1); got[0] != "tentative" {
		t.Fatalf("TentativePriority gave the newborn's cluster (confirmed d²=%.2f) to %q, want the tentative track",
			confirmedD2, got[0])
	}
}

// What the first stage leaves still reaches every unmatched track, confirmed
// ones included, under the shipped gate.
func TestTentativePriorityStillFeedsEveryTrack(t *testing.T) {
	tk, _ := cascadeScene(false)
	tk.Config.TentativePriority = true
	near := elongatedCluster(0.05, 0, 0) // the confirmed track's
	far := elongatedCluster(0.35, 0, 0)  // the tentative track's
	got := tk.associate([]WorldCluster{near, far}, 0.1)
	if got[0] != "confirmed" || got[1] != "tentative" {
		t.Fatalf("associations = %v, want [confirmed tentative]", got)
	}

	tk.Tracks["tentative"].X = 20 // out of the confirmed track's reach
	if got := tk.associate([]WorldCluster{elongatedCluster(20.1, 0, 0)}, 0.1); got[0] != "tentative" {
		t.Fatalf("a cluster outside every confirmed gate went to %q, want the tentative track", got[0])
	}
}

// classScene is one confirmed track labelled by L6 and a cluster beside its
// prediction. The track's own extents are a pedestrian's.
func classScene(label string, confidence float32) *Tracker {
	tk := NewTracker(DefaultTrackerConfig())
	tr := &TrackedObject{TrackID: "labelled", CreationSequence: 1,
		TrackMeasurement: TrackMeasurement{TrackState: TrackConfirmed}}
	tr.Hits, tr.ObservationCount = 12, 12
	tr.BoundingBoxLengthAvg, tr.BoundingBoxWidthAvg = 0.6, 0.5
	tr.ObjectClass, tr.ObjectConfidence = label, confidence
	tk.Tracks["labelled"] = tr
	return tk
}

func boxCluster(x, y, length, width float32) WorldCluster {
	c := s3Cluster(x, y)
	c.BoundingBoxLength, c.BoundingBoxWidth = length, width
	c.OBB.Length, c.OBB.Width = length, width
	return c
}

// K4: a track L6 has labelled a pedestrian, cyclist or motorcyclist does not
// take a cluster larger than that label's envelope, the merge of the body
// with a car beside it. A vehicle label, a weak label, an in-envelope
// cluster and a cluster reporting no extent are never refused.
func TestClassIdentityRefusesAClusterOutsideTheLabelsEnvelope(t *testing.T) {
	merged := boxCluster(0.1, 0, 5.2, 2.4)
	cases := []struct {
		name       string
		label      string
		confidence float32
		cluster    WorldCluster
		refused    bool
	}{
		{"pedestrian, merged with a car", "pedestrian", 0.8, merged, true},
		{"cyclist, merged with a car", "cyclist", 0.8, merged, true},
		{"motorcyclist, merged with a car", "motorcyclist", 0.8, merged, true},
		{"cyclist, a cyclist-sized cluster", "cyclist", 0.8, boxCluster(0.1, 0, 2.0, 0.7), false},
		{"pedestrian, wide but within slack", "pedestrian", 0.8, boxCluster(0.1, 0, 3.4, 1.9), false},
		{"car label carries no envelope", "car", 0.8, merged, false},
		{"unlabelled", "", 0, merged, false},
		{"weak label", "pedestrian", 0.3, merged, false},
		{"cluster reports no extent", "pedestrian", 0.8, boxCluster(0.1, 0, 0, 0), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			off := classScene(tc.label, tc.confidence)
			if got := off.associate([]WorldCluster{tc.cluster}, 0.1); got[0] != "labelled" {
				t.Fatalf("scene precondition: with ClassIdentity off the cluster went to %q", got[0])
			}

			on := classScene(tc.label, tc.confidence)
			on.Config.ClassIdentity = true
			got := on.associate([]WorldCluster{tc.cluster}, 0.1)
			refusals := on.ContinuityStats().ClassIdentityRefusals
			if tc.refused {
				if got[0] != "" || refusals != 1 {
					t.Fatalf("got %q with %d refusals, want the pairing refused once", got[0], refusals)
				}
				return
			}
			if got[0] != "labelled" || refusals != 0 {
				t.Fatalf("got %q with %d refusals, want the labelled track and none", got[0], refusals)
			}
		})
	}
}

// A refusal is counted only where the gate admitted the pairing, so the count
// is of decisions the option changed.
func TestClassIdentityCountsOnlyPairingsTheGateAdmitted(t *testing.T) {
	tk := classScene("pedestrian", 0.8)
	tk.Config.ClassIdentity = true
	if got := tk.associate([]WorldCluster{boxCluster(40, 0, 5.2, 2.4)}, 0.1); got[0] != "" {
		t.Fatalf("a cluster 40 m away went to %q", got[0])
	}
	if n := tk.ContinuityStats().ClassIdentityRefusals; n != 0 {
		t.Fatalf("ClassIdentityRefusals = %d for a pairing the gate refused", n)
	}
}

// contestedConfig is the reacquisition guard, with or without ContestedRejoin.
func contestedConfig(contested bool) TrackerConfig {
	cfg := DefaultTrackerConfig()
	cfg.OcclusionContinuity.ReacquisitionGuard = true
	cfg.OcclusionContinuity.ContestedRejoin = contested
	return cfg
}

// ContestedRejoin on S3's scene: a live track and a track coasting through one
// miss, 1.2 m apart, and a cluster between them. The shipped d² cost
// discounts the coasting track's bid by its inflated covariance, so it wins
// clusters nearer the live track than the midpoint. Where its cost beats the
// live track's by less than the ambiguity margin, ContestedRejoin leaves the
// cluster with the live track, which has evidence from last frame.
func TestContestedRejoinLeavesACloselyContestedClusterWithTheLiveTrack(t *testing.T) {
	const separation = 1.2
	dt := float32(s3FramePeriod.Seconds())

	// Find a cluster the coasting track wins by less than the margin.
	probe, freshID, coastingID := s3Scene(t, contestedConfig(false), separation, 1)
	var contested WorldCluster
	var freshD2, coastD2 float64
	found := false
	for y := float32(separation / 2); y > 0; y -= 0.02 {
		contested = s3Cluster(10, y)
		freshD2, _ = s3Costs(probe, freshID, contested)
		coastD2, _ = s3Costs(probe, coastingID, contested)
		if d := freshD2 - coastD2; d > 0 && d < reacquisitionAmbiguityMargin {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("scene precondition: no cluster between the tracks is won by the coasting track within the margin")
	}
	if got := probe.associate([]WorldCluster{contested}, dt); got[0] != coastingID {
		t.Fatalf("scene precondition: with the guard alone the cluster went to %q, want the coasting track", got[0])
	}

	tk, freshID, _ := s3Scene(t, contestedConfig(true), separation, 1)
	before := tk.ContinuityStats().ReacquisitionsContested
	if got := tk.associate([]WorldCluster{contested}, dt); got[0] != freshID {
		t.Fatalf("ContestedRejoin gave the contested cluster to %q, want the live track (d² live %.2f, coasting %.2f)",
			got[0], freshD2, coastD2)
	}
	if n := tk.ContinuityStats().ReacquisitionsContested - before; n != 1 {
		t.Fatalf("ReacquisitionsContested rose by %d, want 1", n)
	}
}

// It must not overcorrect: an object reappearing where the coasting track
// predicted it is that track's, and the live neighbour does not contest it.
// With both objects present the coasting track is refused only the live
// track's cluster.
func TestContestedRejoinStillReturnsACoastingTracksOwnObject(t *testing.T) {
	const separation = 1.2
	dt := float32(s3FramePeriod.Seconds())
	for _, misses := range []int{1, 3, 5} {
		own := s3Cluster(10, separation)
		tk, freshID, coastingID := s3Scene(t, contestedConfig(true), separation, misses)
		// The coast itself was contested: each missed frame offered the
		// coasting track the live track's cluster.
		before := tk.ContinuityStats().ReacquisitionsContested
		if got := tk.associate([]WorldCluster{own}, dt); got[0] != coastingID {
			t.Fatalf("misses=%d: the coasting track's own object went to %q", misses, got[0])
		}
		if n := tk.ContinuityStats().ReacquisitionsContested - before; n != 0 {
			t.Fatalf("misses=%d: ReacquisitionsContested rose by %d for an uncontested rejoin", misses, n)
		}

		tk, freshID, coastingID = s3Scene(t, contestedConfig(true), separation, misses)
		got := tk.associate([]WorldCluster{s3Cluster(10, 0), own}, dt)
		if got[0] != freshID || got[1] != coastingID {
			t.Fatalf("misses=%d: associations = %v, want [%s %s]", misses, got, freshID, coastingID)
		}
	}
}

// ContestedRejoin extends the guard and does nothing without it.
func TestContestedRejoinNeedsTheReacquisitionGuard(t *testing.T) {
	const separation = 1.2
	cfg := DefaultTrackerConfig()
	cfg.OcclusionContinuity.ContestedRejoin = true
	tk, _, coastingID := s3Scene(t, cfg, separation, 1)
	if got := tk.associate([]WorldCluster{s3Cluster(10, separation/2)}, float32(s3FramePeriod.Seconds())); got[0] != coastingID {
		t.Fatalf("ContestedRejoin without the guard changed the association to %q", got[0])
	}
	if n := tk.ContinuityStats().ReacquisitionsContested; n != 0 {
		t.Fatalf("ReacquisitionsContested = %d without the guard", n)
	}
}
