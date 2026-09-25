package vrlog

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrNotContainer: the path is not a VRLOG 1.x container.
	ErrNotContainer = errors.New("not a VRLOG observation container")
	// ErrLegacyRecording: the path is a VRLOG 0.5 FrameBundle recording. It
	// matches ErrNotContainer too.
	ErrLegacyRecording = errors.New("VRLOG 0.5 FrameBundle recording")
	// ErrCorrupt is matched by every *CorruptionError.
	ErrCorrupt = errors.New("VRLOG container is corrupt")
	// ErrUnsupported is matched by every *UnsupportedError.
	ErrUnsupported = errors.New("VRLOG container needs a newer reader")
	// ErrFrameTooLarge is matched by every *FrameTooLargeError.
	ErrFrameTooLarge = errors.New("frame exceeds the container's limits")
	// ErrSequenceNotFound: a seek named a sequence the stream does not cover.
	ErrSequenceNotFound = errors.New("sequence not in the stream")
	// ErrNotCommitted: a seek named evidence beyond the committed frontier.
	// It matches ErrSequenceNotFound too.
	ErrNotCommitted = errors.New("not yet committed")
	// ErrCaptureFailed is matched by every *CaptureFailedError.
	ErrCaptureFailed = errors.New("observation capture failed")
	// ErrFrameShed is matched by every *FrameShedError.
	ErrFrameShed = errors.New("frame shed under writer backlog")
	// ErrStaleCursor: a cursor does not describe this container's committed
	// prefix, because it belongs to another capture or the prefix it was
	// taken from is no longer committed here.
	ErrStaleCursor = errors.New("stale observation cursor")
	// ErrContainerBusy: a writer or a recovery holds the container's lock.
	ErrContainerBusy = errors.New("observation container is in use")
	// ErrPreGenerationLayout: a container 1.0 written before commit
	// generations. Its sealed chunks carry no commit record, so nothing says
	// which of them a crash left uncommitted.
	ErrPreGenerationLayout = errors.New("VRLOG 1.0 observation container without commit generations")

	// errWriterKilled is returned after a test kill hook stopped the writer.
	errWriterKilled = errors.New("observation writer killed")

	errNotContainer = fmt.Errorf("%w: missing VRLOG root magic", ErrNotContainer)
)

type legacyError struct{ dir string }

func (e *legacyError) Error() string {
	return fmt.Sprintf("%s is a VRLOG 0.5 FrameBundle recording (header.json, index.bin), not an observation container: "+
		"replay it with the FrameBundle replayer, or re-extract observations from its PCAP", e.dir)
}

func (e *legacyError) Is(target error) bool {
	return target == ErrLegacyRecording || target == ErrNotContainer
}

// UnsupportedError names a construct this reader does not know and must not
// guess at: a newer major version, a required feature, a critical record
// kind, a codec or an evidence profile.
type UnsupportedError struct{ What string }

func (e *UnsupportedError) Error() string {
	return "VRLOG container needs a newer reader: unknown " + e.What
}

// Is reports ErrUnsupported.
func (e *UnsupportedError) Is(target error) bool { return target == ErrUnsupported }

func isUnsupported(err error) bool { return errors.Is(err, ErrUnsupported) }

type notCommittedError struct{ detail string }

func (e *notCommittedError) Error() string { return "not yet committed: " + e.detail }
func (e *notCommittedError) Is(target error) bool {
	return target == ErrNotCommitted || target == ErrSequenceNotFound
}

// FailureCause classifies a capture failure. Values are stable strings,
// recorded in the failure marker.
type FailureCause string

const (
	// FailureDiskFull: a write or sync reported no space or quota.
	FailureDiskFull FailureCause = "disk-full"
	// FailureIO: any other write, sync or rename error.
	FailureIO FailureCause = "io-error"
	// FailureStall: an accepted record stayed undurable beyond the declared
	// bound, the batch age plus the commit deadline.
	FailureStall FailureCause = "commit-stall"
	// FailureStopped: the capture's owner ended it abnormally (Writer.Fail).
	FailureStopped FailureCause = "stopped"
)

// CaptureFailedError is the writer's capture-failure state. Evidence up to
// CommittedEndSequence (and CommittedRecords) is durable; records accepted
// after it, up to AcceptedEndSequence, are at risk or lost; nothing after is
// admitted. MarkerWritten reports whether a failure generation recording
// this reached the container.
type CaptureFailedError struct {
	Cause                FailureCause
	Err                  error
	At                   time.Time
	CommittedEndSequence uint64
	CommittedRecords     uint64
	AcceptedEndSequence  uint64
	AcceptedRecords      uint64
	MarkerWritten        bool
}

func (e *CaptureFailedError) Error() string {
	return fmt.Sprintf("observation capture failed (%s): committed to sequence %d, accepted to %d: %v",
		e.Cause, e.CommittedEndSequence, e.AcceptedEndSequence, e.Err)
}

// Is reports ErrCaptureFailed.
func (e *CaptureFailedError) Is(target error) bool { return target == ErrCaptureFailed }

// Unwrap returns the underlying I/O or stall error.
func (e *CaptureFailedError) Unwrap() error { return e.Err }

// FrameShedError reports a frame the writer could not admit within the
// policy's shed wait. It was recorded as a gap over its sequence; the
// capture continues, incomplete.
type FrameShedError struct {
	Sequence uint64
	Waited   time.Duration
}

func (e *FrameShedError) Error() string {
	return fmt.Sprintf("frame %d shed after waiting %s for the committer; recorded as a gap", e.Sequence, e.Waited)
}

// Is reports ErrFrameShed.
func (e *FrameShedError) Is(target error) bool { return target == ErrFrameShed }

// staleCursorError explains why a cursor does not fit.
type staleCursorError struct{ detail string }

func (e *staleCursorError) Error() string   { return "stale observation cursor: " + e.detail }
func (e *staleCursorError) Is(t error) bool { return t == ErrStaleCursor }

// CorruptionKind classifies a CorruptionError. Values are stable strings.
type CorruptionKind string

const (
	CorruptChecksum     CorruptionKind = "checksum-mismatch"
	CorruptDigest       CorruptionKind = "chunk-digest-mismatch"
	CorruptSemantic     CorruptionKind = "semantic-digest-mismatch"
	CorruptTruncated    CorruptionKind = "truncated"
	CorruptLength       CorruptionKind = "bad-length"
	CorruptStructure    CorruptionKind = "bad-structure"
	CorruptDisagreement CorruptionKind = "index-chunk-disagreement"
	CorruptSequence     CorruptionKind = "sequence-break"
	CorruptMissing      CorruptionKind = "missing-object"
	CorruptRecord       CorruptionKind = "invalid-record"
	CorruptSummary      CorruptionKind = "summary-mismatch"
)

// CorruptionError locates damage: which object, which bytes and which source
// sequences are affected, so an operator can tell a lost frame from a lost
// hour. Ranges are half-open; a negative Chunk or unknown range is left at
// its sentinel (-1, or HaveSequences false).
type CorruptionError struct {
	Kind CorruptionKind
	// Object is the container-relative object name, e.g. chunks/00000003.chunk.
	Object string
	// Chunk is the chunk ordinal, or -1 when the damage is not in a chunk.
	Chunk int64
	// Offset and Length are the affected bytes within Object; Length is -1
	// when the damage extends to the end of the object.
	Offset, Length int64
	// Sequences [FirstSequence, EndSequence) are the source sequences whose
	// evidence is affected, when the index can say.
	HaveSequences              bool
	FirstSequence, EndSequence uint64
	Detail                     string
}

func (e *CorruptionError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "corrupt VRLOG container: %s in %s", e.Kind, e.Object)
	if e.Length >= 0 {
		fmt.Fprintf(&b, " bytes [%d, %d)", e.Offset, e.Offset+e.Length)
	} else {
		fmt.Fprintf(&b, " from byte %d", e.Offset)
	}
	if e.HaveSequences {
		fmt.Fprintf(&b, ", sequences [%d, %d)", e.FirstSequence, e.EndSequence)
	}
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(e.Detail)
	}
	return b.String()
}

// Is reports ErrCorrupt.
func (e *CorruptionError) Is(target error) bool { return target == ErrCorrupt }

// FrameTooLargeError reports a frame the writer refused because it exceeds a
// declared limit. The frame is never truncated: the writer records a gap over
// its sequence instead, and the capture summary counts it as rejected.
type FrameTooLargeError struct {
	Sequence uint64
	Detail   string
}

func (e *FrameTooLargeError) Error() string {
	return fmt.Sprintf("frame %d refused: %s; recorded as a gap", e.Sequence, e.Detail)
}

// Is reports ErrFrameTooLarge.
func (e *FrameTooLargeError) Is(target error) bool { return target == ErrFrameTooLarge }
