package l5tracks

import "testing"

// vehicleTrack is a confirmed track that believes it is looking at a car.
func vehicleTrack(id string) *TrackedObject {
	tr := &TrackedObject{
		TrackID:          id,
		TrackMeasurement: TrackMeasurement{TrackState: TrackConfirmed},
		P:                [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
	}
	tr.ObservationCount = 20
	tr.BoundingBoxLengthAvg = 4.3
	tr.BoundingBoxWidthAvg = 1.9
	return tr
}

func sizedCluster(length, width float32) WorldCluster {
	return WorldCluster{
		BoundingBoxLength: length,
		BoundingBoxWidth:  width,
		BoundingBoxHeight: 1.4,
		PointsCount:       50,
	}
}

// The case from run baf20f02: a 0.11 by 0.08 metre scrap capturing a track
// carrying a 4.33 metre car.
func TestFragmentGuardRejectsScrapForVehicleTrack(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	if !tk.isFragmentFor(vehicleTrack("car"), sizedCluster(0.11, 0.08)) {
		t.Fatal("a 0.11 x 0.08 m cluster was accepted for a 4.3 m track")
	}
}

func TestFragmentGuardAcceptsPlausibleCluster(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	// Partly occluded, but still clearly the object.
	if tk.isFragmentFor(vehicleTrack("car"), sizedCluster(2.6, 1.7)) {
		t.Fatal("a 2.6 m cluster was rejected for a 4.3 m track")
	}
}

// The guard must not touch genuinely small road users. A pedestrian track's
// clusters vary by a similar amount to their own size, so a small-cluster rule
// applied there would reject ordinary observations.
func TestFragmentGuardIgnoresSmallTracks(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	ped := &TrackedObject{TrackID: "ped", TrackMeasurement: TrackMeasurement{TrackState: TrackConfirmed}}
	ped.ObservationCount = 20
	ped.BoundingBoxLengthAvg = 0.7
	ped.BoundingBoxWidthAvg = 0.5

	if tk.isFragmentFor(ped, sizedCluster(0.3, 0.25)) {
		t.Fatal("guard fired on a pedestrian-scale track, which it must never do")
	}
}

// A track with too little history has no belief worth defending.
func TestFragmentGuardIgnoresYoungTracks(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	young := vehicleTrack("young")
	young.ObservationCount = 2

	if tk.isFragmentFor(young, sizedCluster(0.11, 0.08)) {
		t.Fatal("guard fired on a track with 2 observations")
	}
}

// A cluster with no reported extent carries no evidence either way. Rejecting
// it here would invent a decision the data does not support; leave it to the
// distance gate.
func TestFragmentGuardIgnoresExtentlessCluster(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	if tk.isFragmentFor(vehicleTrack("car"), sizedCluster(0, 0)) {
		t.Fatal("guard fired on a cluster reporting no extent")
	}
}

func TestFragmentGuardDisabledByZeroConfig(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.MinAssociableExtentMetres = 0
	tk := NewTracker(cfg)

	if tk.isFragmentFor(vehicleTrack("car"), sizedCluster(0.11, 0.08)) {
		t.Fatal("guard fired with MinAssociableExtentMetres = 0")
	}
}

// The guard uses the running average rather than the latest frame, because the
// latest frame is exactly the value a fragment would already have corrupted.
func TestFragmentGuardUsesRunningAverageNotLatestFrame(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	tr := vehicleTrack("car")
	// A fragment got through on a previous frame and wrecked the latest dims.
	tr.OBBLength = 0.11
	tr.OBBWidth = 0.08

	if !tk.isFragmentFor(tr, sizedCluster(0.12, 0.09)) {
		t.Fatal("guard consulted the corrupted latest frame instead of the running average")
	}
}

// End to end through associate(): the fragment must be left unassociated so it
// can seed its own track, rather than consumed by the vehicle.
func TestAssociateLeavesFragmentUnassociated(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	tr := vehicleTrack("car")
	tr.X, tr.Y = 10, 0
	tk.Tracks["car"] = tr

	fragment := sizedCluster(0.11, 0.08)
	fragment.CentroidX, fragment.CentroidY = 10.2, 0.1 // well inside the 6 m gate

	got := tk.associate([]WorldCluster{fragment}, 0.1)
	if got[0] != "" {
		t.Fatalf("fragment associated to %q, want it left free to seed its own track", got[0])
	}
	if tr.FragmentPairingsRejected == 0 {
		t.Fatal("rejection was not recorded")
	}
}

// The same cluster at a plausible size must still associate, or the guard has
// broken ordinary tracking.
func TestAssociateStillAcceptsPlausibleCluster(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	tr := vehicleTrack("car")
	tr.X, tr.Y = 10, 0
	tk.Tracks["car"] = tr

	c := sizedCluster(4.2, 1.8)
	c.CentroidX, c.CentroidY = 10.2, 0.1

	got := tk.associate([]WorldCluster{c}, 0.1)
	if got[0] != "car" {
		t.Fatalf("plausible cluster associated to %q, want \"car\"", got[0])
	}
	if tr.FragmentPairingsRejected != 0 {
		t.Fatal("guard fired on a plausible cluster")
	}
}

func TestDefaultConfigArmsFragmentGuard(t *testing.T) {
	cfg := DefaultTrackerConfig()
	if cfg.MinAssociableExtentMetres <= 0 {
		t.Fatalf("MinAssociableExtentMetres = %v in the shipped defaults, guard is off",
			cfg.MinAssociableExtentMetres)
	}
}
