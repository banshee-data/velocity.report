package l5tracks

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

const degToRad = math.Pi / 180

// lockTestTracker builds a tracker whose heading guards are active and whose
// rejection release is set to maxRejections (0 disables the release, which is
// the original ratchet).
func lockTestTracker(maxRejections int) *Tracker {
	cfg := DefaultTrackerConfig()
	cfg.OBBHeadingLockMaxRejections = maxRejections
	return NewTracker(cfg)
}

// lockTestTrack seeds a confirmed track moving along +X at 10 m/s with its
// smoothed OBB heading already parked at obbDeg. That is the state a track
// reaches after Guard 2 locks it early on a near-square cluster.
func lockTestTrack(t *Tracker, obbDeg float64) *TrackedObject {
	tr := &TrackedObject{
		TrackID:          "locked",
		TrackMeasurement: TrackMeasurement{TrackState: TrackConfirmed},
		X:                0,
		Y:                0,
		VX:               10,
		VY:               0,
		OBBHeadingRad:    float32(obbDeg * degToRad),
		P:                [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
	}
	tr.ObservationCount = 5
	t.Tracks[tr.TrackID] = tr
	return tr
}

// elongatedCluster is a clearly non-square cluster whose PCA heading is
// headingDeg, so Guards 1 and 2 both pass and only Guard 3 is in play.
func elongatedCluster(x, y float32, headingDeg float64) WorldCluster {
	return WorldCluster{
		CentroidX:         x,
		CentroidY:         y,
		BoundingBoxLength: 4.5,
		BoundingBoxWidth:  1.9,
		BoundingBoxHeight: 1.5,
		PointsCount:       200,
		OBB: &l4perception.OrientedBoundingBox{
			CenterX:    x,
			CenterY:    y,
			Length:     4.5,
			Width:      1.9,
			Height:     1.5,
			HeadingRad: float32(headingDeg * degToRad),
		},
	}
}

func courseErrorDeg(tr *TrackedObject) float64 {
	course := math.Atan2(float64(tr.VY), float64(tr.VX))
	return FoldAxisAngleDeg(float64(tr.OBBHeadingRad) - course)
}

// The ratchet, reproduced. With the release disabled, a track whose smoothed
// heading has drifted 90 degrees from the truth rejects every correct
// measurement forever and never recovers.
func TestGuard3RatchetTrapsWithoutRelease(t *testing.T) {
	tk := lockTestTracker(0)
	tr := lockTestTrack(tk, 90)

	for i := 0; i < 40; i++ {
		tk.update(tr, elongatedCluster(float32(i)*1.0, 0, 0), int64(i)*100_000_000)
	}

	if got := courseErrorDeg(tr); got < 80 {
		t.Fatalf("course error = %.1f deg, expected the track to stay trapped near 90", got)
	}
	if !tr.HeadingLockTrapped() {
		t.Fatal("track not reported as trapped")
	}
	if tr.HeadingLockReleases != 0 {
		t.Fatalf("release fired %d times with the release disabled", tr.HeadingLockReleases)
	}
}

// The fix. With the release armed, the same track escapes after the configured
// number of consecutive rejections and converges on its direction of travel.
func TestGuard3ReleasesAfterMaxRejections(t *testing.T) {
	const maxRej = 5
	tk := lockTestTracker(maxRej)
	tr := lockTestTrack(tk, 90)

	for i := 0; i < 40; i++ {
		tk.update(tr, elongatedCluster(float32(i)*1.0, 0, 0), int64(i)*100_000_000)
	}

	if got := courseErrorDeg(tr); got > 5 {
		t.Fatalf("course error = %.1f deg after release, want under 5", got)
	}
	if tr.HeadingLockReleases == 0 {
		t.Fatal("release never fired")
	}
	if tr.HeadingLockTrapped() {
		t.Fatal("track still reported as trapped after releasing")
	}
}

// The release must not fire early: the guard's real job is suppressing genuine
// one-off axis swaps, and those must still be rejected.
func TestGuard3StillRejectsBeforeTheLimit(t *testing.T) {
	const maxRej = 5
	tk := lockTestTracker(maxRej)
	tr := lockTestTrack(tk, 0)
	startHeading := tr.OBBHeadingRad

	// Four rejections, one short of the limit.
	for i := 0; i < maxRej-1; i++ {
		tk.update(tr, elongatedCluster(float32(i)*1.0, 0, 90), int64(i)*100_000_000)
	}

	if tr.HeadingLockReleases != 0 {
		t.Fatalf("release fired after %d rejections, limit is %d", maxRej-1, maxRej)
	}
	if tr.OBBHeadingRad != startHeading {
		t.Fatalf("heading moved to %v during rejections, want it held at %v",
			tr.OBBHeadingRad, startHeading)
	}
	if tr.HeadingRejectionRun != maxRej-1 {
		t.Fatalf("rejection run = %d, want %d", tr.HeadingRejectionRun, maxRej-1)
	}
}

// A single good frame between rejections resets the counter, so an isolated
// axis swap every few frames never accumulates into a release.
func TestGuard3RejectionRunResetsOnAcceptedUpdate(t *testing.T) {
	tk := lockTestTracker(5)
	tr := lockTestTrack(tk, 0)

	for i := 0; i < 20; i++ {
		heading := 0.0
		if i%3 == 0 {
			heading = 90 // an occasional axis swap
		}
		tk.update(tr, elongatedCluster(float32(i)*1.0, 0, heading), int64(i)*100_000_000)
	}

	if tr.HeadingLockReleases != 0 {
		t.Fatalf("release fired %d times on isolated swaps, want 0", tr.HeadingLockReleases)
	}
	if got := courseErrorDeg(tr); got > 5 {
		t.Fatalf("course error = %.1f deg, want the box to stay aligned", got)
	}
}

// On release the heading must snap, not ease. The EMA moves 8 per cent of the
// gap per update, so easing across a delta wide enough to be rejected would
// re-trigger the guard on the next frame and never converge.
func TestGuard3ReleaseSnapsRatherThanSmooths(t *testing.T) {
	const maxRej = 3
	tk := lockTestTracker(maxRej)
	tr := lockTestTrack(tk, 90)

	for i := 0; i < maxRej; i++ {
		tk.update(tr, elongatedCluster(float32(i)*1.0, 0, 0), int64(i)*100_000_000)
	}

	if tr.HeadingLockReleases != 1 {
		t.Fatalf("release fired %d times, want exactly 1", tr.HeadingLockReleases)
	}
	// The snap lands on the measurement, not 8 per cent of the way toward it.
	if got := math.Abs(float64(tr.OBBHeadingRad)); got > 1e-5 {
		t.Fatalf("heading after snap = %v rad, want the measured 0", tr.OBBHeadingRad)
	}
}

// Releasing must also unfreeze the dimensions, which are only written when the
// heading update is accepted. A release that left them frozen would keep the
// box the wrong size while fixing its angle.
func TestGuard3ReleaseRestoresDimensionUpdates(t *testing.T) {
	tk := lockTestTracker(3)
	tr := lockTestTrack(tk, 90)
	tr.OBBLength = 0.11
	tr.OBBWidth = 0.08

	for i := 0; i < 10; i++ {
		tk.update(tr, elongatedCluster(float32(i)*1.0, 0, 0), int64(i)*100_000_000)
	}

	if tr.OBBLength < 4.0 || tr.OBBWidth < 1.5 {
		t.Fatalf("dimensions still frozen at %.2f x %.2f after release, want the cluster's 4.5 x 1.9",
			tr.OBBLength, tr.OBBWidth)
	}
}

// Guards 1 and 2 are different in kind from Guard 3: they fire when the
// measurement is genuinely unusable, so they must not drive the release. A
// stream of tiny clusters should stay locked however long it runs.
func TestReleaseDoesNotFireOnLowPointGuard(t *testing.T) {
	tk := lockTestTracker(5)
	tr := lockTestTrack(tk, 90)

	for i := 0; i < 40; i++ {
		c := elongatedCluster(float32(i)*1.0, 0, 0)
		c.PointsCount = 2 // below MinPointsForPCA
		tk.update(tr, c, int64(i)*100_000_000)
	}

	if tr.HeadingLockReleases != 0 {
		t.Fatalf("release fired %d times on Guard 1 rejections, want 0", tr.HeadingLockReleases)
	}
	if got := courseErrorDeg(tr); got < 80 {
		t.Fatalf("course error = %.1f deg, want the heading held at 90 with no usable measurement", got)
	}
}

func TestReleaseDoesNotFireOnNearSquareGuard(t *testing.T) {
	tk := lockTestTracker(5)
	tr := lockTestTrack(tk, 90)

	for i := 0; i < 40; i++ {
		c := elongatedCluster(float32(i)*1.0, 0, 0)
		// Near-square: aspect difference under OBBAspectRatioLockThreshold.
		c.OBB.Length, c.OBB.Width = 2.0, 1.95
		tk.update(tr, c, int64(i)*100_000_000)
	}

	if tr.HeadingLockReleases != 0 {
		t.Fatalf("release fired %d times on Guard 2 rejections, want 0", tr.HeadingLockReleases)
	}
}

// The shipped default must actually arm the release, or none of the above runs
// in production.
func TestDefaultConfigArmsTheRelease(t *testing.T) {
	cfg := DefaultTrackerConfig()
	if cfg.OBBHeadingLockMaxRejections <= 0 {
		t.Fatalf("OBBHeadingLockMaxRejections = %d in the shipped defaults, the ratchet is still in place",
			cfg.OBBHeadingLockMaxRejections)
	}
}
