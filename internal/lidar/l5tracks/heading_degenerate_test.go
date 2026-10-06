package l5tracks

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Gap P1 in data/maths/paper-implementation-gap-analysis.md notes that an
// all-identical-points cluster yields a 0x0 OBB with an arbitrary heading, and
// that no test covered it. The box itself is fine — see
// l4perception/obb_degenerate_test.go — but Guard 2, which exists to reject
// exactly this kind of ambiguous heading, used to skip the check whenever the
// box had no extent, to avoid dividing by zero. That let the maximally
// ambiguous measurement through while rejecting merely near-square ones.

// degenerateCluster is a cluster whose OBB has the given planar extents and a
// heading of zero, which is what EstimateOBBFromCluster's X-axis fallback
// reports when there is no axis of variation to recover.
func degenerateCluster(length, width float32) WorldCluster {
	return WorldCluster{
		CentroidX:         1,
		CentroidY:         0,
		BoundingBoxLength: length,
		BoundingBoxWidth:  width,
		BoundingBoxHeight: 1.5,
		PointsCount:       200,
		OBB: &l4perception.OrientedBoundingBox{
			CenterX:    1,
			Length:     length,
			Width:      width,
			Height:     1.5,
			HeadingRad: 0,
		},
	}
}

func TestGuard2RejectsAZeroExtentOBBHeading(t *testing.T) {
	// The track's heading starts 30 degrees from the cluster's arbitrary 0, a
	// delta small enough that Guard 3's 60-120 degree axis-swap window does not
	// fire. That isolates Guard 2 as the only guard in play.
	tk := lockTestTracker(3)
	tr := lockTestTrack(tk, 30)
	before := tr.OBBHeadingRad

	tk.update(tr, degenerateCluster(0, 0), 100_000_000)

	if !tr.HeadingSource.IsLocked() {
		t.Errorf("a zero-extent OBB was accepted as a heading measurement (source %v); "+
			"the arbitrary PCA fallback must not steer a real heading", tr.HeadingSource)
	}
	if tr.HeadingSource != HeadingSourceInsufficient {
		t.Errorf("heading source = %v, want insufficient so a degenerate cluster stays "+
			"distinguishable from a near-square one in telemetry", tr.HeadingSource)
	}
	if tr.OBBHeadingRad != before {
		t.Errorf("heading moved from %v to %v on a measurement carrying no orientation",
			before, tr.OBBHeadingRad)
	}
}

func TestGuard2TreatsDegenerateAndNearSquareConsistently(t *testing.T) {
	// The defect was an inconsistency, not just a missed case: the ambiguity
	// rejection has to be monotonic in how square the box is. A box with no
	// extent is infinitely square and must be rejected at least as firmly as
	// one that is merely nearly square.
	for _, tc := range []struct {
		name       string
		length     float32
		width      float32
		wantLocked bool
	}{
		{"zero extent", 0, 0, true},
		{"5cm square", 0.05, 0.05, true},
		{"1m square", 1.0, 1.0, true},
		{"car-shaped", 4.5, 1.9, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tk := lockTestTracker(3)
			tr := lockTestTrack(tk, 30)
			before := tr.OBBHeadingRad

			tk.update(tr, degenerateCluster(tc.length, tc.width), 100_000_000)

			if got := tr.HeadingSource.IsLocked(); got != tc.wantLocked {
				t.Errorf("locked = %v, want %v (source %v)", got, tc.wantLocked, tr.HeadingSource)
			}
			moved := math.Abs(float64(tr.OBBHeadingRad - before))
			if tc.wantLocked && moved != 0 {
				t.Errorf("heading moved %v rad despite an ambiguous measurement", moved)
			}
			if !tc.wantLocked && moved == 0 {
				t.Errorf("heading did not move on a well-formed measurement")
			}
		})
	}
}

func TestZeroExtentOBBDoesNotCorruptTrackGeometry(t *testing.T) {
	// Beyond the heading: a degenerate observation must not leave the track
	// carrying non-finite or negative dimensions, since those feed physical
	// endpoint uncertainty downstream.
	tk := lockTestTracker(3)
	tr := lockTestTrack(tk, 30)
	tr.OBBLength = 4.5
	tr.OBBWidth = 1.9

	for i := 0; i < 20; i++ {
		tk.update(tr, degenerateCluster(0, 0), int64(i)*100_000_000)
	}

	for _, f := range []struct {
		name string
		v    float32
	}{
		{"OBBLength", tr.OBBLength},
		{"OBBWidth", tr.OBBWidth},
		{"OBBHeadingRad", tr.OBBHeadingRad},
		{"X", tr.X}, {"Y", tr.Y}, {"VX", tr.VX}, {"VY", tr.VY},
	} {
		if math.IsNaN(float64(f.v)) || math.IsInf(float64(f.v), 0) {
			t.Errorf("%s is not finite after 20 degenerate observations: %v", f.name, f.v)
		}
	}
	if tr.OBBLength < 0 || tr.OBBWidth < 0 {
		t.Errorf("negative track dimensions: %v x %v", tr.OBBLength, tr.OBBWidth)
	}
}
