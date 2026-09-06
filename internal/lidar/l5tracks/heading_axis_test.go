package l5tracks

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"math"
	"testing"
	"time"
)

func axisTestTracker() *Tracker {
	c := DefaultTrackerConfig()
	c.OBBAxisCoherenceEnabled = true
	c.OBBHeadingSmoothingAlpha = 1
	return NewTracker(c)
}

func TestAxisModeToggleClearsObservedReference(t *testing.T) {
	tr := NewTracker(DefaultTrackerConfig())
	obj := &TrackedObject{axisReferenceL: 4, axisReferenceW: 2, AxisScoreGap: 8}
	tr.Tracks["car"] = obj
	tr.UpdateConfig(func(c *TrackerConfig) { c.OBBAxisCoherenceEnabled = true })
	if obj.axisReferenceL != 0 || obj.axisReferenceW != 0 || obj.AxisScoreGap != 0 {
		t.Fatal("stale reference survived mode switch")
	}
}

func TestAxisEquivalentRectangleAndReverse(t *testing.T) {
	tk := axisTestTracker()
	c := elongatedCluster(0, 0, 0)
	tr := tk.initTrack(c, 1e9)
	tr.VX = -10 // Travel backwards must not relabel the body's front.
	c.OBB.Length, c.OBB.Width = c.OBB.Width, c.OBB.Length
	c.OBB.HeadingRad = math.Pi / 2
	tk.update(tr, c, 2e9)
	if tr.HeadingSource != HeadingSourceAxis || math.Abs(axisResidual(float64(tr.OBBHeadingRad), 0)) > 1e-6 {
		t.Fatalf("swap not resolved: source=%s heading=%f", tr.HeadingSource, tr.OBBHeadingRad)
	}
	if tr.AxisScoreGap < axisMinimumGap {
		t.Fatal("accepted without separation")
	}
	if math.Abs(float64(tr.OBBLength-4.5)) > .001 || math.Abs(float64(tr.OBBWidth-1.9)) > .001 {
		t.Fatal("rectangle geometry changed")
	}
}

func TestAxisAbstainsAndPreservesReference(t *testing.T) {
	for _, kind := range []string{"square", "fragment", "few_points", "missing", "invalid"} {
		t.Run(kind, func(t *testing.T) {
			tk := axisTestTracker()
			tr := tk.initTrack(elongatedCluster(0, 0, 0), 1e9)
			c := elongatedCluster(0, 0, 80)
			source := HeadingSourceAmbiguous
			switch kind {
			case "square":
				c.OBB.Length, c.OBB.Width = 2, 2
			case "fragment":
				c.OBB.Length, c.OBB.Width = .11, .08
			case "few_points":
				c.PointsCount = 1
				source = HeadingSourceInsufficient
			case "missing":
				c.OBB = nil
				source = HeadingSourceInsufficient
			case "invalid":
				c.OBB.Length = float32(math.NaN())
				source = HeadingSourceInsufficient
			}
			for i := 0; i < 20; i++ {
				tk.update(tr, c, int64(i+2)*1e9)
			}
			if tr.HeadingSource != source || tr.OBBHeadingRad != 0 {
				t.Fatalf("invented heading: %+v", tr)
			}
			if tr.axisReferenceL != 4.5 || tr.axisReferenceW != 1.9 {
				t.Fatal("partial views changed reference")
			}
			if tr.HeadingEpisodes.Outcome() != "unrecovered" {
				t.Fatal("ambiguity missing from episode telemetry")
			}
		})
	}
}

func TestAxisSmoothTurnAndWrap(t *testing.T) {
	tk := axisTestTracker()
	tr := tk.initTrack(elongatedCluster(0, 0, 170), 1e9)
	for i := 1; i <= 80; i++ {
		degrees := 170 + float64(i)*2
		c := elongatedCluster(0, 0, degrees)
		tk.update(tr, c, int64(i+1)*1000000000)
		if tr.HeadingSource != HeadingSourceAxis || math.Abs(axisResidual(float64(tr.OBBHeadingRad), degrees*math.Pi/180)) > 1e-5 {
			t.Fatalf("turn rejected at %v", degrees)
		}
	}
	if tr.HeadingLockReleases != 0 {
		t.Fatal("axis path invoked forced-release guard")
	}
	if got := axisResidual(179*math.Pi/180, -179*math.Pi/180); math.Abs(got+2*math.Pi/180) > 1e-9 {
		t.Fatal(got)
	}
}

func TestAxisTurnWithDefaultSmootherDoesNotRatchet(t *testing.T) {
	tk := axisTestTracker()
	tk.Config.OBBHeadingSmoothingAlpha = DefaultTrackerConfig().OBBHeadingSmoothingAlpha
	tr := tk.initTrack(elongatedCluster(0, 0, 0), 1e9)
	for i := 1; i <= 80; i++ {
		c := elongatedCluster(0, 0, float64(i)*2)
		tk.update(tr, c, int64(i+1)*1000000000)
		if tr.HeadingSource != HeadingSourceAxis {
			t.Fatalf("turn locked at step %d", i)
		}
	}
}

func TestAxisCostMarginAndReferenceUpdate(t *testing.T) {
	tk := axisTestTracker()
	tr := tk.initTrack(elongatedCluster(0, 0, 0), 1e9)
	c := elongatedCluster(0, 0, 0)
	c.OBB.Length = 4.6
	c.OBB.Width = 1.95
	tk.update(tr, c, 2e9)
	if tr.axisReferenceL <= 4.5 {
		t.Fatal("comparable support was not revisable")
	}
	tr.axisReferenceL, tr.axisReferenceW = 2.1, 1.9
	c.OBB.Length, c.OBB.Width = 2.2, 1.8
	tk.update(tr, c, 3e9)
	if tr.HeadingSource != HeadingSourceAmbiguous {
		t.Fatal("near-tied alternatives forced a winner")
	}
	tr.axisReferenceL = 0
	tk.update(tr, elongatedCluster(0, 0, 20), 4e9)
	if tr.HeadingSource != HeadingSourceAxis {
		t.Fatal("missing reference did not reseed")
	}
}

func TestObservedEnvelopeContainsFreshCornersWithoutAccumulation(t *testing.T) {
	for i := 0; i < 50; i++ {
		tr := TrackedObject{X: -.6, Y: .3, OBBHeadingRad: float32(i) * .17}
		b := &l4perception.OrientedBoundingBox{CenterX: 1, CenterY: -.5, CenterZ: 2, Length: 4.5, Width: 1.9, Height: 1.5, HeadingRad: float32(i) * .31}
		projectObservedEnvelope(&tr, b)
		length, width := tr.OBBLength, tr.OBBWidth
		for _, x := range []float64{-1, 1} {
			for _, y := range []float64{-1, 1} {
				c, s := math.Cos(float64(b.HeadingRad)), math.Sin(float64(b.HeadingRad))
				wx := float64(b.CenterX) + c*x*float64(b.Length)/2 - s*y*float64(b.Width)/2 - float64(tr.X)
				wy := float64(b.CenterY) + s*x*float64(b.Length)/2 + c*y*float64(b.Width)/2 - float64(tr.Y)
				h := float64(tr.OBBHeadingRad)
				if math.Abs(math.Cos(h)*wx+math.Sin(h)*wy) > float64(length)/2+1e-5 || math.Abs(-math.Sin(h)*wx+math.Cos(h)*wy) > float64(width)/2+1e-5 {
					t.Fatal("corner outside envelope")
				}
			}
		}
		projectObservedEnvelope(&tr, b)
		if tr.OBBLength != length || tr.OBBWidth != width || tr.LatestZ != b.CenterZ {
			t.Fatal("recursive inflation or centre mismatch")
		}
	}
}

func TestFiniteOBBAndInitialInsufficientSupport(t *testing.T) {
	if finiteOBB(nil) {
		t.Fatal("nil valid")
	}
	for _, v := range []float32{float32(math.NaN()), float32(math.Inf(1))} {
		b := *elongatedCluster(0, 0, 0).OBB
		b.CenterX = v
		if finiteOBB(&b) {
			t.Fatal("nonfinite valid")
		}
	}
	b := *elongatedCluster(0, 0, 0).OBB
	b.Length = 0
	if finiteOBB(&b) {
		t.Fatal("zero length valid")
	}
	tk := axisTestTracker()
	c := elongatedCluster(0, 0, 0)
	c.PointsCount = 0
	tr := tk.initTrack(c, time.Now().UnixNano())
	if tr.HeadingSource != HeadingSourceInsufficient || tr.axisReferenceL != 0 {
		t.Fatal("invalid seed promoted")
	}
	if !HeadingSourceAmbiguous.IsLocked() || !HeadingSourceInsufficient.IsLocked() || HeadingSourceAxis.IsLocked() {
		t.Fatal("source semantics")
	}
	for _, s := range []HeadingSource{HeadingSourceAxis, HeadingSourceAmbiguous, HeadingSourceInsufficient} {
		if s.String() == "unknown" {
			t.Fatal("missing source name")
		}
	}
}

func TestAssociationUsesCreationSequenceInsteadOfUUID(t *testing.T) {
	assign := func(a, b string) int64 {
		tk := axisTestTracker()
		c := elongatedCluster(0, 0, 0)
		x := tk.initTrack(c, 1e9)
		y := tk.initTrack(c, 1e9)
		delete(tk.Tracks, x.TrackID)
		delete(tk.Tracks, y.TrackID)
		x.TrackID, y.TrackID = a, b
		tk.Tracks[a] = x
		tk.Tracks[b] = y
		ids := tk.associate([]WorldCluster{c}, .1)
		return tk.Tracks[ids[0]].CreationSequence
	}
	if assign("a", "z") != assign("z", "a") {
		t.Fatal("tied assignment depends on UUID")
	}
}
