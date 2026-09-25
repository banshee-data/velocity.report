package l4bobserve

import "fmt"

// StreamValidator checks an extraction's frame and gap records in order,
// holding only the last boundary, so a long capture can be checked as it is
// produced or read.
//
// Sequence coverage is strict: frames and sequence-range gaps must cover
// 0, 1, 2, ... exactly once, so a missing record is an error rather than a
// silent hole. Capture time is checked for order, not disjointness: a
// rotation seam can split one packet across two frames, so neighbouring
// capture intervals legitimately overlap by up to one packet's firing time
// (51 µs on the kirk0 reference capture), and so can a gap's bounds and the
// frames beside it. The time rule is therefore on starts: no frame or bounded
// gap may start before the record preceding it. Clock resets need explicit
// epoch records, which this contract does not have yet; until then a
// backwards step is refused rather than accepted as order.
type StreamValidator struct {
	profile      Profile
	next         uint64
	frames       uint64
	gaps         uint64
	haveBoundary bool
	boundary     int64 // latest frame or bounded-gap start seen so far
}

// NewStreamValidator applies profile's per-record guarantees to every frame.
func NewStreamValidator(profile Profile) *StreamValidator {
	return &StreamValidator{profile: profile}
}

// AddFrame accepts the next frame record.
func (v *StreamValidator) AddFrame(f FrameRecord) error {
	if f.Sequence != v.next {
		return fmt.Errorf("frame sequence %d where %d was expected: a missing frame must be an explicit gap", f.Sequence, v.next)
	}
	if err := f.ValidateFor(v.profile); err != nil {
		return err
	}
	if v.haveBoundary && f.CaptureStartUnixNanos < v.boundary {
		return fmt.Errorf("frame %d starts %d ns before the preceding record", f.Sequence, v.boundary-f.CaptureStartUnixNanos)
	}
	v.next++
	v.frames++
	v.haveBoundary = true
	v.boundary = f.CaptureStartUnixNanos
	return nil
}

// AddGap accepts the next gap record.
func (v *StreamValidator) AddGap(g GapRecord) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if g.HasSequenceRange {
		if g.FirstSequence != v.next {
			return fmt.Errorf("gap covers sequences from %d where %d was expected", g.FirstSequence, v.next)
		}
	}
	if g.Time == GapTimeBounded {
		if v.haveBoundary && g.StartUnixNanos < v.boundary {
			return fmt.Errorf("gap starts %d ns before the preceding record", v.boundary-g.StartUnixNanos)
		}
		v.haveBoundary = true
		v.boundary = g.StartUnixNanos
	}
	if g.HasSequenceRange {
		v.next = g.LastSequence + 1
	}
	v.gaps++
	return nil
}

// Frames and Gaps report how many records were accepted.
func (v *StreamValidator) Frames() uint64 { return v.frames }
func (v *StreamValidator) Gaps() uint64   { return v.gaps }

// NextSequence is the sequence the next frame must carry.
func (v *StreamValidator) NextSequence() uint64 { return v.next }
