// Package capseq sequences LiDAR capture files into a single continuous replay
// stream without writing an intermediate unioned file.
//
// Field capture writes a rolling series of short PCAP files (typically five
// minutes each). A useful static period is bounded by when the sensor stopped
// and started moving, not by when the capture tool rolled its output, so a
// replay case routinely spans several files. This package answers the two
// questions that makes possible:
//
//   - Do these files actually abut? Every join between adjacent files is graded
//     against packet-time tolerances, so a silent gap is reported rather than
//     replayed as if it were continuous.
//   - Which byte ranges does a given window touch? Plan converts a
//     sequence-relative window into per-file read steps, each of which the L1
//     reader can execute against one shared parser and frame builder.
//
// The package is deliberately free of any libpcap dependency: it operates on
// packet-time extents that the caller has already probed (see
// network.CountPCAPPackets), so it builds and tests under the default
// `go test ./...` with no build tag.
package capseq

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// SeamGrade classifies the join between two adjacent capture files.
type SeamGrade string

const (
	// SeamSeamless means the files abut within the seamless tolerance: the
	// capture tool rolled its output without losing a packet. The revolution in
	// flight across the join is genuinely contiguous and is kept.
	SeamSeamless SeamGrade = "seamless"

	// SeamAcceptable means a real but tolerable loss. Replay continues across
	// the join, but the revolution in flight is discarded: its two halves are
	// far enough apart in time that assembling them into one frame would
	// present L3 with a plausible-looking frame that never existed.
	SeamAcceptable SeamGrade = "acceptable"

	// SeamBroken means too much data is missing to call the files one stream.
	SeamBroken SeamGrade = "broken"

	// SeamOverlap means the later file starts before the earlier one ends by
	// more than the overlap tolerance, so replaying both would duplicate
	// packets. Usually a mis-selection rather than a capture fault.
	SeamOverlap SeamGrade = "overlap"
)

// severity orders grades from best to worst so a sequence can report its
// weakest join.
func (g SeamGrade) severity() int {
	switch g {
	case SeamSeamless:
		return 0
	case SeamAcceptable:
		return 1
	case SeamBroken:
		return 2
	case SeamOverlap:
		return 3
	default:
		return 4
	}
}

// Replayable reports whether a join of this grade can be crossed during replay.
func (g SeamGrade) Replayable() bool {
	return g == SeamSeamless || g == SeamAcceptable
}

// Tolerances bound what counts as continuous. The defaults come from the field
// requirement: less than a second of data lost across a join, and ideally less
// than ten milliseconds.
type Tolerances struct {
	// Seamless is the largest gap still considered a clean roll-over. At a 20 Hz
	// spin rate ten milliseconds is a fifth of a revolution, within the sensor's
	// own inter-packet jitter.
	Seamless time.Duration

	// MaxGap is the largest gap that may still be crossed during replay.
	MaxGap time.Duration

	// MaxOverlap is how far the later file may start before the earlier one ends
	// before the join is called an overlap. Capture tools occasionally repeat a
	// packet at a roll-over; a whole duplicated revolution is a different matter.
	MaxOverlap time.Duration
}

// Default tolerances for capture sequencing.
const (
	// DefaultSeamlessTolerance is the "ideally less than" bound: a join inside
	// it keeps the revolution that straddles it.
	DefaultSeamlessTolerance = 10 * time.Millisecond
	// DefaultMaxGap is the hard bound on data lost at a join.
	DefaultMaxGap = 1 * time.Second
	// DefaultMaxOverlap tolerates a repeated packet or two at a roll-over.
	DefaultMaxOverlap = 10 * time.Millisecond
)

// DefaultTolerances returns the field-workflow tolerances.
func DefaultTolerances() Tolerances {
	return Tolerances{
		Seamless:   DefaultSeamlessTolerance,
		MaxGap:     DefaultMaxGap,
		MaxOverlap: DefaultMaxOverlap,
	}
}

// withDefaults fills unset fields so a zero Tolerances behaves like the default
// rather than grading every join as broken.
func (t Tolerances) withDefaults() Tolerances {
	if t.Seamless <= 0 {
		t.Seamless = DefaultSeamlessTolerance
	}
	if t.MaxGap <= 0 {
		t.MaxGap = DefaultMaxGap
	}
	if t.MaxOverlap <= 0 {
		t.MaxOverlap = DefaultMaxOverlap
	}
	return t
}

// GradeGap classifies a single join from the signed gap between the earlier
// file's last packet and the later file's first packet. A negative gap is an
// overlap.
func GradeGap(gap time.Duration, tol Tolerances) SeamGrade {
	tol = tol.withDefaults()
	switch {
	case gap < -tol.MaxOverlap:
		return SeamOverlap
	case gap <= tol.Seamless:
		// Includes small negative gaps inside the overlap tolerance: a repeated
		// packet at a roll-over is still a clean join.
		return SeamSeamless
	case gap <= tol.MaxGap:
		return SeamAcceptable
	default:
		return SeamBroken
	}
}

// Segment is one capture file's packet-time extent, as probed at index time.
type Segment struct {
	// Path identifies the file. It is opaque to this package and is passed
	// through to the read steps unchanged.
	Path string
	// FirstPacket and LastPacket are the capture timestamps of the first and
	// last matching packets in the file.
	FirstPacket time.Time
	LastPacket  time.Time
	// PacketCount is the number of matching packets, used to aggregate replay
	// progress across a multi-file plan. Zero is permitted.
	PacketCount uint64
}

// Duration is the packet-time extent of the segment.
func (s Segment) Duration() time.Duration {
	return s.LastPacket.Sub(s.FirstPacket)
}

// Seam is the graded join between Segments[Index] and Segments[Index+1].
type Seam struct {
	// Index is the position of the earlier segment in Sequence.Segments.
	Index int
	// Before and After are the joined segments' paths, carried so a caller can
	// report a seam without indexing back into the sequence.
	Before string
	After  string
	// Gap is After's first packet minus Before's last packet. Negative is an
	// overlap.
	Gap   time.Duration
	Grade SeamGrade
}

// Sequence is an ordered run of capture files with every join graded.
type Sequence struct {
	// Segments are ordered by first-packet time.
	Segments []Segment
	// Seams has one entry per join, so len(Seams) == len(Segments)-1.
	Seams      []Seam
	Tolerances Tolerances

	// Start and End are the first and last packet times across the whole run.
	Start time.Time
	End   time.Time
	// Span is End-Start: the wall-clock extent the sequence claims to cover.
	Span time.Duration
	// Covered is the sum of the segments' own extents, so Span-Covered is the
	// time unaccounted for at the joins.
	Covered time.Duration
	// Lost is the sum of the positive gaps at the joins.
	Lost time.Duration
	// Worst is the weakest grade across all joins; SeamSeamless for a
	// single-file sequence, which has no joins.
	Worst SeamGrade
	// PacketCount is the sum of the segments' packet counts.
	PacketCount uint64
}

// Errors returned when the caller's segments cannot form a sequence at all.
var (
	// ErrNoSegments is returned by Build for an empty segment list.
	ErrNoSegments = errors.New("capseq: no segments")
	// ErrNotContinuous is returned by Plan when a join cannot be crossed.
	ErrNotContinuous = errors.New("capseq: sequence is not continuous")
	// ErrEmptyWindow is returned by Plan when the requested window selects no
	// packets.
	ErrEmptyWindow = errors.New("capseq: window selects no packets")
)

// Build orders the segments by first-packet time and grades every join.
//
// Grading is data, not failure: a run containing a broken join still builds, so
// a caller can show the operator which join broke and why. Build returns an
// error only when the input is malformed — no segments, a zero or inverted
// extent, or a repeated path.
func Build(segments []Segment, tol Tolerances) (*Sequence, error) {
	if len(segments) == 0 {
		return nil, ErrNoSegments
	}
	tol = tol.withDefaults()

	ordered := make([]Segment, len(segments))
	copy(ordered, segments)

	seen := make(map[string]struct{}, len(ordered))
	for _, s := range ordered {
		if s.Path == "" {
			return nil, fmt.Errorf("capseq: segment with empty path")
		}
		if _, dup := seen[s.Path]; dup {
			return nil, fmt.Errorf("capseq: duplicate segment path %q", s.Path)
		}
		seen[s.Path] = struct{}{}
		if s.FirstPacket.IsZero() || s.LastPacket.IsZero() {
			return nil, fmt.Errorf("capseq: segment %q has an unset packet time", s.Path)
		}
		if s.LastPacket.Before(s.FirstPacket) {
			return nil, fmt.Errorf("capseq: segment %q ends before it starts", s.Path)
		}
	}

	// Sort by first packet, tie-breaking on path so the order is deterministic
	// for files that share a start time.
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].FirstPacket.Equal(ordered[j].FirstPacket) {
			return ordered[i].Path < ordered[j].Path
		}
		return ordered[i].FirstPacket.Before(ordered[j].FirstPacket)
	})

	seq := &Sequence{
		Segments:   ordered,
		Tolerances: tol,
		Start:      ordered[0].FirstPacket,
		End:        ordered[0].LastPacket,
		Worst:      SeamSeamless,
	}

	for i, s := range ordered {
		seq.Covered += s.Duration()
		seq.PacketCount += s.PacketCount
		if s.LastPacket.After(seq.End) {
			seq.End = s.LastPacket
		}
		if i == 0 {
			continue
		}
		prev := ordered[i-1]
		gap := s.FirstPacket.Sub(prev.LastPacket)
		grade := GradeGap(gap, tol)
		seq.Seams = append(seq.Seams, Seam{
			Index:  i - 1,
			Before: prev.Path,
			After:  s.Path,
			Gap:    gap,
			Grade:  grade,
		})
		if gap > 0 {
			seq.Lost += gap
		}
		if grade.severity() > seq.Worst.severity() {
			seq.Worst = grade
		}
	}

	seq.Span = seq.End.Sub(seq.Start)
	return seq, nil
}

// Continuous reports whether every join can be crossed during replay.
func (s *Sequence) Continuous() bool {
	return s.Worst.Replayable()
}

// OffsetOf is the sequence-relative offset of segment i's first packet.
func (s *Sequence) OffsetOf(i int) time.Duration {
	if i < 0 || i >= len(s.Segments) {
		return 0
	}
	return s.Segments[i].FirstPacket.Sub(s.Start)
}

// BrokenSeams returns the joins that cannot be crossed, for reporting.
func (s *Sequence) BrokenSeams() []Seam {
	var out []Seam
	for _, seam := range s.Seams {
		if !seam.Grade.Replayable() {
			out = append(out, seam)
		}
	}
	return out
}

// ReadStep is one file's contribution to a replayed window. The L1 reader
// executes the steps in order against a single shared parser and frame builder,
// which is what makes the union unnecessary: pipeline state carries across the
// join while the packet source changes underneath.
type ReadStep struct {
	// Path is the capture file to open.
	Path string
	// StartSecs is the offset into this file at which to begin, relative to its
	// own first packet. Zero means start at the beginning of the file.
	StartSecs float64
	// DurationSecs is how much of this file to read from StartSecs. It is -1
	// when the step runs to the end of the file, which lets the reader skip its
	// end threshold entirely rather than rely on a float comparison against the
	// last packet's timestamp.
	DurationSecs float64
	// DropFrameAtStart tells the reader to discard the revolution in flight when
	// this step begins. It is set when the preceding join was merely acceptable
	// rather than seamless: the two halves of that revolution are far enough
	// apart that combining them would fabricate a frame.
	DropFrameAtStart bool
	// PacketCount is the indexed packet count for this file, carried so a caller
	// can aggregate replay progress across the plan. It is the whole file's
	// count, not the step's, and is zero when unknown.
	PacketCount uint64
}

// Plan converts a sequence-relative window into per-file read steps.
//
// startSecs is measured from Sequence.Start. A negative or zero startSecs means
// the beginning of the sequence. A negative durationSecs means "to the end".
//
// Plan refuses a sequence with a join it cannot cross, because a plan that
// silently skipped a broken join would replay two unrelated stretches as one
// continuous capture.
//
// A step that begins part-way into a file will present the pipeline with one
// partial revolution, exactly as a single-file replay with a start offset does
// today; only revolutions straddling a join are dropped.
func (s *Sequence) Plan(startSecs, durationSecs float64) ([]ReadStep, error) {
	if len(s.Segments) == 0 {
		return nil, ErrNoSegments
	}
	if !s.Continuous() {
		broken := s.BrokenSeams()
		return nil, fmt.Errorf("%w: %d unusable join(s), first is %s → %s (%s, gap %s)",
			ErrNotContinuous, len(broken),
			broken[0].Before, broken[0].After, broken[0].Grade, broken[0].Gap)
	}

	if startSecs < 0 {
		startSecs = 0
	}
	windowStart := s.Start.Add(time.Duration(startSecs * float64(time.Second)))
	windowEnd := s.End
	if durationSecs >= 0 {
		windowEnd = windowStart.Add(time.Duration(durationSecs * float64(time.Second)))
		if windowEnd.After(s.End) {
			windowEnd = s.End
		}
	}
	if !windowEnd.After(windowStart) {
		return nil, ErrEmptyWindow
	}

	var steps []ReadStep
	for i, seg := range s.Segments {
		// Skip segments entirely outside the window. A segment touching the
		// window at exactly one instant contributes no span and is skipped too.
		if !seg.LastPacket.After(windowStart) || !windowEnd.After(seg.FirstPacket) {
			continue
		}

		stepStart := seg.FirstPacket
		if windowStart.After(stepStart) {
			stepStart = windowStart
		}
		stepEnd := seg.LastPacket
		runsToFileEnd := true
		if windowEnd.Before(stepEnd) {
			stepEnd = windowEnd
			runsToFileEnd = false
		}

		duration := -1.0
		if !runsToFileEnd {
			duration = stepEnd.Sub(stepStart).Seconds()
		}

		// The revolution in flight is dropped when this step continues from a
		// previous one across a join that was not seamless. The first step of a
		// plan has no in-flight revolution to drop.
		drop := false
		if len(steps) > 0 && i > 0 {
			drop = s.Seams[i-1].Grade != SeamSeamless
		}

		steps = append(steps, ReadStep{
			Path:             seg.Path,
			StartSecs:        stepStart.Sub(seg.FirstPacket).Seconds(),
			DurationSecs:     duration,
			DropFrameAtStart: drop,
			PacketCount:      seg.PacketCount,
		})
	}

	if len(steps) == 0 {
		return nil, ErrEmptyWindow
	}
	return steps, nil
}
