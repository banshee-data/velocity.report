package l5tracks

import "testing"

// Gap S2/S3 in data/maths/paper-implementation-gap-analysis.md: tentative
// tracks compete with confirmed tracks in one joint assignment, so a tentative
// track created a frame ago can take the cluster a confirmed track coasting
// through one miss should have received. The campaign saw what a population
// of such pairings does: hits_to_confirm=1 took kirk0 from 7 to 0 of 16
// labelled tracks while producing 141 candidates in place of 81.

// cascadeScene is the S2 test as the gap analysis specifies it: a confirmed
// track coasting (one miss), a new tentative track beside it, and one cluster
// that sits nearer the tentative track's prediction.
func cascadeScene(cascade bool) (*Tracker, []WorldCluster) {
	c := DefaultTrackerConfig()
	c.CascadedAssociation = cascade
	tk := NewTracker(c)

	confirmed := &TrackedObject{TrackID: "confirmed", CreationSequence: 1,
		TrackMeasurement: TrackMeasurement{TrackState: TrackConfirmed}}
	confirmed.X, confirmed.Y = 0, 0
	confirmed.Hits, confirmed.Misses, confirmed.ObservationCount = 12, 1, 12
	confirmed.BoundingBoxLengthAvg, confirmed.BoundingBoxWidthAvg = 4.5, 1.9

	tentative := &TrackedObject{TrackID: "tentative", CreationSequence: 2,
		TrackMeasurement: TrackMeasurement{TrackState: TrackTentative}}
	tentative.X, tentative.Y = 0.30, 0
	tentative.Hits, tentative.ObservationCount = 1, 1
	tentative.BoundingBoxLengthAvg, tentative.BoundingBoxWidthAvg = 4.5, 1.9

	tk.Tracks["confirmed"] = confirmed
	tk.Tracks["tentative"] = tentative

	cluster := elongatedCluster(0.20, 0, 0) // 0.20 m from confirmed, 0.10 m from tentative
	return tk, []WorldCluster{cluster}
}

func TestJointAssignmentLetsATentativeTrackOutbidACoastingConfirmedOne(t *testing.T) {
	tk, clusters := cascadeScene(false)
	got := tk.associate(clusters, 0.1)
	if got[0] != "tentative" {
		t.Fatalf("joint assignment gave the cluster to %q; this test expects the shipped "+
			"behaviour (nearest prediction wins) so the cascade's change is measured against it", got[0])
	}
}

func TestCascadedAssociationGivesConfirmedTracksFirstChoice(t *testing.T) {
	tk, clusters := cascadeScene(true)
	got := tk.associate(clusters, 0.1)
	if got[0] != "confirmed" {
		t.Fatalf("cascade gave the cluster to %q, want the coasting confirmed track", got[0])
	}
}

// What confirmed tracks leave is still offered to tentative ones: the cascade
// must not starve new tracks of clusters no confirmed track wanted.
func TestCascadedAssociationStillFeedsTentativeTracks(t *testing.T) {
	tk, clusters := cascadeScene(true)
	far := elongatedCluster(0.35, 0, 0)  // 0.05 m from tentative, 0.35 m from confirmed
	near := elongatedCluster(0.05, 0, 0) // 0.05 m from confirmed
	got := tk.associate(append(clusters[:0:0], near, far), 0.1)
	if got[0] != "confirmed" || got[1] != "tentative" {
		t.Fatalf("associations = %v, want [confirmed tentative]", got)
	}
}

// A cluster no confirmed track can take (outside its gate) must reach the
// tentative stage rather than being dropped with the first assignment.
func TestCascadedAssociationPassesUngatedClustersDown(t *testing.T) {
	tk, _ := cascadeScene(true)
	// Beyond MaxPositionJumpMetres from the confirmed track at the origin,
	// within reach of a tentative track placed beside it.
	tk.Tracks["tentative"].X = 20
	only := elongatedCluster(20.1, 0, 0)
	got := tk.associate([]WorldCluster{only}, 0.1)
	if got[0] != "tentative" {
		t.Fatalf("cluster outside every confirmed gate went to %q, want the tentative track", got[0])
	}
}

func TestCascadedAssociationOffByDefault(t *testing.T) {
	if DefaultTrackerConfig().CascadedAssociation {
		t.Fatal("CascadedAssociation defaults on; it must stay measurable against the shipped assignment first")
	}
}
