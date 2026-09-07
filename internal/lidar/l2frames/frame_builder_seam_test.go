package l2frames

import (
	"sync"
	"testing"
)

// seamRecorder collects finalised frames so a test can assert which
// revolutions survived a capture-file join.
type seamRecorder struct {
	mu     sync.Mutex
	frames []*LiDARFrame
}

func (r *seamRecorder) record(f *LiDARFrame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, f)
}

func (r *seamRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.frames)
}

func (r *seamRecorder) pointCounts() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]int, len(r.frames))
	for i, f := range r.frames {
		out[i] = f.PointCount
	}
	return out
}

// newSeamBuilder returns a builder wired for deterministic offline replay:
// blocking sends so no frame is dropped for queue pressure, and a low minimum
// point count so a synthetic revolution counts as complete.
func newSeamBuilder(t *testing.T, rec *seamRecorder) *FrameBuilder {
	t.Helper()
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:       "seam-test",
		FrameCallback:  rec.record,
		MinFramePoints: 4,
	})
	fb.SetBlockOnFrameChannel(true)
	t.Cleanup(fb.Close)
	return fb
}

// sweepSteps is the azimuth resolution of a synthetic revolution. At 10 degrees
// per step a sweep spans 0-350 degrees, clearing the builder's
// MinAzimuthCoverage gate so the wrap into the next revolution is accepted.
const sweepSteps = 36

// revolution emits points sweeping azimuth from 0 towards 360 in `steps`
// increments. Feeding two revolutions back to back makes the builder detect the
// wrap and finalise the first.
func revolution(steps int, tsBase int64) []PointPolar {
	pts := make([]PointPolar, 0, steps)
	for i := range steps {
		pts = append(pts, PointPolar{
			Channel:   1,
			Azimuth:   float64(i) * (360.0 / float64(steps)),
			Distance:  10.0,
			Timestamp: tsBase + int64(i),
		})
	}
	return pts
}

// halfRevolution emits the first or second half of a sweep, standing in for the
// tail of one capture file and the head of the next.
func halfRevolution(steps int, tsBase int64, secondHalf bool) []PointPolar {
	pts := make([]PointPolar, 0, steps/2)
	offset := 0
	if secondHalf {
		offset = steps / 2
	}
	for i := offset; i < offset+steps/2; i++ {
		pts = append(pts, PointPolar{
			Channel:   1,
			Azimuth:   float64(i) * (360.0 / float64(steps)),
			Distance:  10.0,
			Timestamp: tsBase + int64(i),
		})
	}
	return pts
}

func TestDropNextFrameReportsWhetherAFrameWasInFlight(t *testing.T) {
	rec := &seamRecorder{}
	fb := newSeamBuilder(t, rec)

	// Nothing has arrived yet, so there is no straddling revolution to drop.
	// Arming the flag here would discard the first real frame instead.
	if fb.DropNextFrame() {
		t.Error("DropNextFrame reported a frame in flight before any points arrived")
	}
	fb.mu.Lock()
	armed := fb.dropStraddling
	fb.mu.Unlock()
	if armed {
		t.Error("DropNextFrame armed the drop with no frame in flight")
	}

	fb.AddPointsPolar(revolution(sweepSteps, 0))
	if !fb.DropNextFrame() {
		t.Error("DropNextFrame reported no frame in flight after points arrived")
	}
}

func TestDropNextFrameDiscardsTheStraddlingRevolution(t *testing.T) {
	rec := &seamRecorder{}
	fb := newSeamBuilder(t, rec)

	const steps = sweepSteps

	// File A: one complete revolution, then the head of a second.
	fb.AddPointsPolar(revolution(steps, 0))
	fb.AddPointsPolar(halfRevolution(steps, 100, false))

	// The join. The revolution in flight spans it.
	if !fb.DropNextFrame() {
		t.Fatal("no frame in flight at the join")
	}

	// File B: the tail of that same revolution, then two complete ones.
	fb.AddPointsPolar(halfRevolution(steps, 200, true))
	fb.AddPointsPolar(revolution(steps, 300))
	fb.AddPointsPolar(revolution(steps, 400))

	fb.FlushPendingFrames()
	fb.WaitForCallbacks()

	if got := fb.StraddlingFramesDropped(); got != 1 {
		t.Errorf("StraddlingFramesDropped = %d, want 1", got)
	}

	// Revolution 1 from A survives; the straddling one is discarded; the two
	// complete revolutions from B survive.
	if got := rec.count(); got != 3 {
		t.Errorf("finalised %d frames %v, want 3 (the straddling revolution should be gone)",
			got, rec.pointCounts())
	}

	// None of the surviving frames may contain the half-and-half mixture: each
	// is a whole sweep of `steps` points.
	for i, n := range rec.pointCounts() {
		if n != steps {
			t.Errorf("frame %d has %d points, want %d (a full revolution)", i, n, steps)
		}
	}
}

func TestSeamlessJoinKeepsEveryRevolution(t *testing.T) {
	// The counterpart to the drop: when a join is seamless the caller does not
	// arm a drop, and the revolution spanning it is emitted intact.
	rec := &seamRecorder{}
	fb := newSeamBuilder(t, rec)

	const steps = sweepSteps
	fb.AddPointsPolar(revolution(steps, 0))
	fb.AddPointsPolar(halfRevolution(steps, 100, false))
	// No DropNextFrame call: this join was seamless.
	fb.AddPointsPolar(halfRevolution(steps, 200, true))
	fb.AddPointsPolar(revolution(steps, 300))

	fb.FlushPendingFrames()
	fb.WaitForCallbacks()

	if got := fb.StraddlingFramesDropped(); got != 0 {
		t.Errorf("StraddlingFramesDropped = %d, want 0 across a seamless join", got)
	}
	if got := rec.count(); got != 3 {
		t.Errorf("finalised %d frames %v, want 3 with the straddling revolution kept",
			got, rec.pointCounts())
	}
}

func TestDropNextFrameSurvivesCloseWithoutEmitting(t *testing.T) {
	// A replay that ends while a drop is pending must not emit the straddling
	// revolution during the close flush.
	rec := &seamRecorder{}
	fb := NewFrameBuilder(FrameBuilderConfig{
		SensorID:       "seam-close",
		FrameCallback:  rec.record,
		MinFramePoints: 4,
	})
	fb.SetBlockOnFrameChannel(true)

	const steps = sweepSteps
	fb.AddPointsPolar(revolution(steps, 0))
	fb.AddPointsPolar(halfRevolution(steps, 100, false))
	if !fb.DropNextFrame() {
		t.Fatal("no frame in flight at the join")
	}
	fb.Close()

	if got := fb.StraddlingFramesDropped(); got != 1 {
		t.Errorf("StraddlingFramesDropped = %d, want 1", got)
	}
	// Only the complete revolution from before the join survives.
	if got := rec.count(); got != 1 {
		t.Errorf("finalised %d frames %v, want 1", got, rec.pointCounts())
	}
}

func TestResetClearsAPendingDrop(t *testing.T) {
	// A pending drop belongs to the run being reset. Carrying it into the next
	// run would silently discard that run's first complete revolution.
	rec := &seamRecorder{}
	fb := newSeamBuilder(t, rec)

	const steps = sweepSteps
	fb.AddPointsPolar(revolution(steps, 0))
	if !fb.DropNextFrame() {
		t.Fatal("no frame in flight")
	}
	fb.Reset()

	fb.mu.Lock()
	armed := fb.dropStraddling
	fb.mu.Unlock()
	if armed {
		t.Error("Reset left the join drop armed")
	}

	fb.AddPointsPolar(revolution(steps, 100))
	fb.AddPointsPolar(revolution(steps, 200))
	fb.FlushPendingFrames()
	fb.WaitForCallbacks()

	if got := fb.StraddlingFramesDropped(); got != 0 {
		t.Errorf("StraddlingFramesDropped = %d, want 0 after Reset cleared the flag", got)
	}
	if got := rec.count(); got == 0 {
		t.Error("no frames survived after Reset; the stale drop swallowed one")
	}
}

func TestDropCountAccumulatesAcrossSeveralJoins(t *testing.T) {
	rec := &seamRecorder{}
	fb := newSeamBuilder(t, rec)

	const steps = sweepSteps
	for join := range 3 {
		fb.AddPointsPolar(revolution(steps, int64(join)*1000))
		fb.AddPointsPolar(halfRevolution(steps, int64(join)*1000+100, false))
		if !fb.DropNextFrame() {
			t.Fatalf("join %d: no frame in flight", join)
		}
		fb.AddPointsPolar(halfRevolution(steps, int64(join)*1000+200, true))
	}
	fb.FlushPendingFrames()
	fb.WaitForCallbacks()

	if got := fb.StraddlingFramesDropped(); got != 3 {
		t.Errorf("StraddlingFramesDropped = %d, want 3 across three joins", got)
	}
}
