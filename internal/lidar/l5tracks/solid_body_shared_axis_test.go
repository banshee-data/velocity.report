package l5tracks

import (
	"math"
	"testing"
)

// Tests for what SolidBodyOptions.SharedOrientation does to the published
// box (shared-orientation-duplicates-2026-11). What carries the weight:
// alone the option leaves every published box and every tracked state
// bit-identical, so neither the split of the box's Observe nor the coast
// order reaches the box; beside BoxExtentFromSolidBody the box's agreement
// test reads the body's own axis unless BodyExtentSharedAxis says to read
// the shared one, against which it cannot refuse.

// boxFrame is one track's published box and tracked state on one frame.
type boxFrame struct {
	seq                                       int64
	x, y, vx, vy, bx, by, heading, length, wd float32
}

func publishedBoxes(cfg TrackerConfig, frames []syntheticPassFrame) [][]boxFrame {
	tracker := NewTracker(cfg)
	out := make([][]boxFrame, 0, len(frames))
	for _, f := range frames {
		tracker.Update(f.clusters, f.at)
		var row []boxFrame
		for _, tr := range tracker.GetAllTracks() {
			row = append(row, boxFrame{tr.CreationSequence, tr.X, tr.Y, tr.VX, tr.VY,
				tr.BoxX, tr.BoxY, tr.BoxHeadingRad, tr.BoxLength, tr.BoxWidth})
		}
		out = append(out, row)
	}
	return out
}

// Alone the option is a shadow on the box as well as on the tracks: the
// heading half runs ahead of the solid body and the placement half after
// it, and a coasting box coasts first, but the box reads nothing of the
// solid body without BoxExtentFromSolidBody, so every frame's box is the
// one the shipped order draws.
func TestSharedOrientationAloneLeavesThePublishedBox(t *testing.T) {
	frames := syntheticPassFrames(t, throughTruckPass())
	plain := faceStateConfig()
	shared := faceStateConfig()
	shared.SolidBody.SharedOrientation = true
	a, b := publishedBoxes(plain, frames), publishedBoxes(shared, frames)
	drawn := 0
	for i := range a {
		if len(a[i]) != len(b[i]) {
			t.Fatalf("frame %d: %d tracks without the option, %d with", i, len(a[i]), len(b[i]))
		}
		index := map[int64]boxFrame{}
		for _, r := range a[i] {
			index[r.seq] = r
		}
		for _, r := range b[i] {
			if index[r.seq] != r {
				t.Fatalf("frame %d track %d: box or state %+v with the option, %+v without", i, r.seq, r, index[r.seq])
			}
			if r.length > 0 {
				drawn++
			}
		}
	}
	if drawn == 0 {
		t.Fatal("no box was drawn: the comparison read nothing")
	}
}

// boxBodyExtent's agreement axis on a shared frame is the body's own unless
// BodyExtentSharedAxis is set; with no own axis there is nothing to test it
// against, and the box keeps its own belief.
func TestBoxBodyExtentReadsTheOwnAxisOnASharedFrame(t *testing.T) {
	track := boxPassBody(0, 5, 7)
	sb := &track.solidBody
	sb.seeded, sb.ext.on, sb.reference = true, true, ReferenceBodyCentre
	sb.orientation = OrientationBelief{PsiRad: 0.1, VarianceRad2: 0.01, Provenance: ProvenanceObserved}
	sb.own = OrientationBelief{PsiRad: 1.2, VarianceRad2: 0.05, Provenance: ProvenanceObserved}

	if got := boxBodyExtent(track, false); !got.Known || got.AxisRad != float64(sb.orientation.PsiRad) {
		t.Fatalf("an unshared frame: %+v, want the body's orientation", got)
	}
	sb.shared = true
	if got := boxBodyExtent(track, false); !got.Known || got.AxisRad != float64(sb.own.PsiRad) {
		t.Fatalf("a shared frame: %+v, want the body's own axis", got)
	}
	if got := boxBodyExtent(track, true); !got.Known || got.AxisRad != float64(sb.orientation.PsiRad) {
		t.Fatalf("a shared frame under BodyExtentSharedAxis: %+v, want the shared heading", got)
	}
	sb.own = OrientationBelief{}
	if got := boxBodyExtent(track, false); got.Known {
		t.Fatalf("a shared frame with no own axis: %+v, want nothing to read", got)
	}
	if got := boxBodyExtent(track, true); !got.Known {
		t.Fatal("BodyExtentSharedAxis does not need the own axis")
	}
}

// boxExtentTotals runs the tracker and sums every track's body-extent counts.
func boxExtentTotals(cfg TrackerConfig, frames []syntheticPassFrame) (BoxStats, int) {
	tracker := NewTracker(cfg)
	stats := map[int64]BoxStats{}
	sharedFrames := 0
	for _, f := range frames {
		tracker.Update(f.clusters, f.at)
		for _, tr := range tracker.GetAllTracks() {
			if st, ok := tr.BoxModelStats(); ok {
				stats[tr.CreationSequence] = st
			}
			if tr.solidBody.shared {
				sharedFrames++
			}
		}
	}
	var sum BoxStats
	for _, st := range stats {
		sum.BodyExtentFrames += st.BodyExtentFrames
		sum.BodyExtentUnknown += st.BodyExtentUnknown
		sum.BodyExtentAxisRefusals += st.BodyExtentAxisRefusals
		sum.BodyExtentUnsettled += st.BodyExtentUnsettled
	}
	return sum, sharedFrames
}

// Through the tracker on the through-truck pass, whose tracked PCA heading
// strays up to a quarter turn from the box's: box_extent_solid_body alone
// refuses the frames whose body axis disagrees with the box. With the shared
// orientation the body's axis is the box's heading, so read against it the
// test refuses nothing on a shared frame; read against the own axis, as it
// is by default, it refuses as box_extent_solid_body alone does.
func TestSharedOrientationKeepsTheBoxExtentAxisTest(t *testing.T) {
	frames := syntheticPassFrames(t, throughTruckPass())
	boxExtent := faceStateConfig()
	boxExtent.BoxModel.BoxExtentFromSolidBody = true
	stack := boxExtent
	stack.SolidBody.SharedOrientation = true
	gated := stack
	gated.BoxModel.BodyExtentSharedAxis = true

	alone, _ := boxExtentTotals(boxExtent, frames)
	fixed, sharedFrames := boxExtentTotals(stack, frames)
	vacuous, _ := boxExtentTotals(gated, frames)
	t.Logf("through-truck pass, axis refusals / frames that took the extent: box extent alone %d / %d; "+
		"with the shared orientation, own axis %d / %d, shared axis %d / %d (%d shared frames)",
		alone.BodyExtentAxisRefusals, alone.BodyExtentFrames, fixed.BodyExtentAxisRefusals, fixed.BodyExtentFrames,
		vacuous.BodyExtentAxisRefusals, vacuous.BodyExtentFrames, sharedFrames)
	if sharedFrames == 0 || alone.BodyExtentAxisRefusals == 0 {
		t.Fatalf("%d shared frames and %d refusals without the option: the pass does not exercise the test",
			sharedFrames, alone.BodyExtentAxisRefusals)
	}
	if vacuous.BodyExtentAxisRefusals >= fixed.BodyExtentAxisRefusals {
		t.Errorf("the shared axis refused %d frames, the own axis %d; want the shared axis to refuse fewer",
			vacuous.BodyExtentAxisRefusals, fixed.BodyExtentAxisRefusals)
	}
	// The tracker alone has no track guard, so the tracks are the same in
	// all three arms and the own axis is the axis box_extent_solid_body
	// alone tests: the refusals must match.
	if fixed.BodyExtentAxisRefusals != alone.BodyExtentAxisRefusals {
		t.Errorf("read against the own axis, the test refused %d frames; box_extent_solid_body alone %d",
			fixed.BodyExtentAxisRefusals, alone.BodyExtentAxisRefusals)
	}
	// What still differs is the extent the box draws, now measured along the
	// shared heading: reported, not bounded.
	t.Logf("box-frames differing from box_extent_solid_body alone: own axis %d, shared axis %d",
		differingBoxes(publishedBoxes(boxExtent, frames), publishedBoxes(stack, frames)),
		differingBoxes(publishedBoxes(boxExtent, frames), publishedBoxes(gated, frames)))
}

// differingBoxes counts the track-frames whose box or state differs.
func differingBoxes(a, b [][]boxFrame) int {
	n := 0
	for i := range a {
		index := map[int64]boxFrame{}
		for _, r := range a[i] {
			index[r.seq] = r
		}
		for _, r := range b[i] {
			if index[r.seq] != r {
				n++
			}
		}
	}
	return n
}

// HeadingNISAtScale at the filter's own scale is HeadingNIS; a smaller scale
// reads the same innovations against a smaller observation variance.
func TestHeadingNISAtScale(t *testing.T) {
	frames := syntheticPassFrames(t, throughTruckPass())
	cfg := faceStateConfig()
	cfg.SolidBody.SharedOrientation = true
	scale := cfg.BoxModel.AxisSigmaScale
	tracker := NewTracker(cfg)
	read := 0
	for _, f := range frames {
		tracker.Update(f.clusters, f.at)
		for _, tr := range tracker.GetActiveTracks() {
			r, ok := tr.SolidBody()
			if !ok || !r.Measurement.HeadingNISKnown {
				continue
			}
			m := r.Measurement
			own, ok := m.HeadingNISAtScale(scale)
			if !ok || math.Abs(own-float64(m.HeadingNIS)) > 1e-4*math.Max(1, float64(m.HeadingNIS)) {
				t.Fatalf("at the filter's scale %.2f: %v %v, HeadingNIS %v", scale, own, ok, m.HeadingNIS)
			}
			if smaller, _ := m.HeadingNISAtScale(scale / 2); m.HeadingInnovationSq > 0 && smaller <= own {
				t.Fatalf("half the scale read %v, the filter's %v; want larger", smaller, own)
			}
			read++
		}
	}
	if read == 0 {
		t.Fatal("no frame carried a heading innovation")
	}
	if _, ok := (SolidBodyMeasurement{}).HeadingNISAtScale(1); ok {
		t.Fatal("a frame without a heading innovation read one")
	}
}
