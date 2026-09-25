package vrlog

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// Summary is the closing account of a container: what a clean Close wrote,
// or what a reader found.
type Summary struct {
	Chunks uint64
	// ChunkChain folds every chunk's body digest in ordinal order; see
	// CaptureSummary in recording.proto.
	ChunkChain     l4bobserve.Digest
	Records        uint64
	Frames         uint64
	Gaps           uint64
	EndSequence    uint64
	RejectedFrames uint64
	ShedFrames     uint64
	ChunkBytes     uint64
	// Semantic is the l4bobserve stream digest of every record in order.
	Semantic l4bobserve.Digest
	// Commits counts the generations published before the close, and
	// CommitLatency is their measured publication latency.
	Commits       uint64
	CommitLatency Latency
}

// Writer is the one ordered writer of a capture. The L4 callback appends
// each frame synchronously: the record is checked, encoded and written into
// the open batch, and the append returns. That is acceptance, not
// durability. A committer goroutine closes the batch at the policy's age or
// byte threshold (or at once in strict mode) and publishes it as a commit
// generation by the four-step protocol, after which the durable frontier
// advances and is announced. Nothing downstream of capture, tracking
// included, is ever waited on.
//
// At most two batches are undurable at once: the one committing and the open
// one. An append that finds the open batch full waits for the committer,
// which backpressures a paused PCAP reader; with ShedAfter set it instead
// records the frame as an explicit gap. An accepted record not durable within
// the declared stall bound, a write or sync error, or a full disk put the
// capture into failure: admission stops, and a failure generation is written
// if the disk still allows it.
//
// Append, Flush, Close, Fail and Abandon are serialised; Frontier,
// WaitFrontier and Stats may be called from any goroutine.
type Writer struct {
	dir            string
	manifest       Manifest
	manifestDigest [sha256.Size]byte
	tag            [8]byte
	limits         Limits
	policy         CommitPolicy
	lock           *os.File
	// hooks are fault injectors, nil in production. Tests swap them while
	// the committer runs, hence the atomic pointer.
	hooks atomic.Pointer[writerHooks]

	appendMu sync.Mutex // serialises the caller-facing operations

	mu sync.Mutex // guards everything below
	// Admission.
	stream      l4bobserve.StreamValidator
	semantic    *l4bobserve.StreamDigest
	scratch     []byte
	open        *openChunk
	nextOrdinal uint64
	accepted    struct{ records, frames, gaps, payload uint64 }
	// Commitment. published is the chain tip on disk; durable is the tip
	// whose publication completed every step, which is what is announced.
	committing *openChunk
	published  chainState
	durable    chainState
	failure    *CaptureFailedError
	// Lifecycle. settled: the committer has exited, so no further
	// generation can be published.
	closing, closeDone, stopping, markerTried, finished, dead, settled bool
	summary                                                            Summary
	progress                                                           chan struct{}
	wake                                                               chan struct{}
	exited                                                             chan struct{}
	ageTimer, watchdog                                                 *time.Timer
	// Measurements.
	publishLatency, syncLatency, batchAge latencyHistogram
	objectBytes                           [objectKinds]atomic.Uint64
	files, syncs                          atomic.Uint64
}

// openChunk is a batch: one chunk file being filled, then sealed and
// committed by exactly one generation.
type openChunk struct {
	ordinal       uint64
	file          *os.File
	openRel       string // container-relative name while open
	hash          hash.Hash
	semantic      *l4bobserve.StreamDigest
	offset        uint64
	index         pb.ChunkIndex
	seal          pb.ChunkSeal
	firstAccepted time.Time
	// full: closed to new frames (the byte threshold, the chunk bound or the
	// record bound was reached). forced: close at once, for this trigger.
	full    bool
	forced  pb.CommitTrigger
	trigger pb.CommitTrigger
	// rejected and shed are frames replaced by gaps in this batch.
	rejected, shed uint64
	// Set when sealed: the published index and the generation's entry.
	indexObject []byte
	committed   *pb.CommittedChunk
}

// commitStep names the points of the publication protocol at which a test
// can kill the writer. Steps 1 to 4 are those of VRLOG_FORMAT.md.
type commitStep int

const (
	stepSealed              commitStep = iota + 1 // 1: seal written, nothing synchronised
	stepChunkSynced                               // 2: chunk synchronised, not renamed
	stepChunkPublished                            // 2: chunk renamed
	stepIndexPublished                            // 2: index written, synchronised, renamed
	stepObjectsDurable                            // 2: directories synchronised
	stepGenerationWritten                         // 3: generation written and synchronised, not renamed
	stepGenerationPublished                       // 3: generation renamed, directory not synchronised
	stepGenerationDurable                         // 3: generation directory synchronised
	stepPointerWritten                            // 4: pointer written and synchronised, not renamed
	stepPointerPublished                          // 4: pointer renamed, directory not synchronised
	stepPointerDurable                            // 4: complete, not yet announced
)

// writerHooks inject faults in tests; production leaves them nil.
type writerHooks struct {
	// kill, returning true, stops the writer dead at step of generation n:
	// no further write, sync, rename or announcement, as when the process is
	// killed. Only a power loss would additionally lose unsynchronised data.
	kill func(step commitStep, generation uint64) bool
	// sync and write run before each fsync and write of the named
	// container-relative object; an error replaces the call's result, and a
	// hook may sleep to simulate a stalled device.
	sync  func(name string) error
	write func(name string) error
}

// Object kinds for byte accounting.
const (
	bytesChunk = iota
	bytesIndex
	bytesGeneration
	bytesCurrent
	bytesSummary
	bytesManifest
	bytesReserve
	objectKinds
)

var objectKindNames = [objectKinds]string{"chunk", "index", "generation", "current", "summary", "manifest", "reserve"}

// Create makes dir, which must not exist, takes its lock, and durably
// publishes the manifest and generation 0 before any record is admitted. An
// empty profile is foreground-complete, the only record family this writer
// produces; zero limits and zero policy fields take the provisional
// defaults; an empty capture UUID and creation time are allocated here.
func Create(dir string, m Manifest) (*Writer, error) {
	declared := l4bobserve.ForegroundComplete()
	switch {
	case m.Profile.Name == "" && len(m.Profile.Capabilities.List()) == 0:
		m.Profile = declared
	case m.Profile.Name != declared.Name:
		return nil, fmt.Errorf("the observation writer records profile %q, not %q", declared.Name, m.Profile.Name)
	}
	// Required features are the writer's to declare; a manifest read back
	// from a container, which already names them, may be reused.
	for _, f := range m.RequiredFeatures {
		if f != featureCommitGenerations {
			return nil, fmt.Errorf("this writer implements no required feature %q", f)
		}
	}
	m.RequiredFeatures = []string{featureCommitGenerations}
	if m.Limits == (Limits{}) {
		m.Limits = DefaultLimits()
	}
	m.Commit = m.Commit.withDefaults(m.Limits)
	if m.Capture.UUID == "" {
		m.Capture.UUID = uuid.NewString()
	}
	if m.Capture.CreatedUnixNanos == 0 {
		m.Capture.CreatedUnixNanos = time.Now().UnixNano()
	}
	m.Extraction.CaptureFiles = slices.Clone(m.Extraction.CaptureFiles)
	m.Metadata = slices.Clone(m.Metadata)
	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	payload, err := marshalOptions.Marshal(m.toProto())
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	if len(payload) > maxManifestBytes {
		return nil, fmt.Errorf("manifest of %d bytes exceeds %d", len(payload), maxManifestBytes)
	}
	object := appendRecord(appendPreamble(nil, objectManifest), envelope{kind: RecordManifest}, payload)

	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return nil, err
	}
	// Mkdir, not MkdirAll: evidence is never written into an existing
	// directory, where it could mix with or replace another capture.
	if err := os.Mkdir(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}
	for _, sub := range []string{chunksDir, generationsDir} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	lock, err := acquireLock(dir)
	if err != nil {
		return nil, err
	}
	w := &Writer{
		dir: dir, manifest: m, manifestDigest: sha256.Sum256(object), limits: m.Limits, policy: m.Commit, lock: lock,
		stream: *l4bobserve.NewStreamValidator(m.Profile), semantic: l4bobserve.NewStreamDigest(),
		progress: make(chan struct{}), wake: make(chan struct{}, 1), exited: make(chan struct{}),
	}
	copy(w.tag[:], w.manifestDigest[:8])
	if err := w.create(object); err != nil {
		w.releaseLocked()
		return nil, err
	}
	go w.commitLoop()
	return w, nil
}

// create writes the manifest, the failure reserve and generation 0.
func (w *Writer) create(manifest []byte) error {
	if _, err := w.writeObject(manifestName, manifest, bytesManifest); err != nil {
		return err
	}
	reserve, err := os.OpenFile(filepath.Join(w.dir, reserveName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	err = w.writeFile(reserve, reserveName, make([]byte, w.policy.FailureReserveBytes), bytesReserve)
	if err == nil {
		err = w.syncFile(reserve, reserveName)
	}
	if closeErr := reserve.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := w.syncDirectory("."); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(w.dir)); err != nil {
		return err
	}
	res := w.publish(&commitJob{delta: batchDelta{kind: pb.GenerationKind_GENERATION_KIND_OPEN}})
	if res.err != nil {
		return res.err
	}
	w.published, w.durable = res.state, res.state
	return nil
}

func acquireLock(dir string) (*os.File, error) {
	lock, err := os.OpenFile(filepath.Join(dir, lockName), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open container lock: %w", err)
	}
	// The kernel releases a flock when the holder exits, killed or not, so a
	// crashed writer never leaves the container locked.
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("%s: %w: %v", dir, ErrContainerBusy, err)
	}
	return lock, nil
}

// Dir is the container directory.
func (w *Writer) Dir() string { return w.dir }

// Manifest is the manifest as written, with the allocated identity and the
// effective commit policy.
func (w *Writer) Manifest() Manifest { return w.manifest }

// Policy is the effective commit policy.
func (w *Writer) Policy() CommitPolicy { return w.policy }

// stopErrLocked says why the capture has stopped, if it has: the writer
// was killed or the capture failed.
func (w *Writer) stopErrLocked() error {
	switch {
	case w.dead:
		return errWriterKilled
	case w.failure != nil:
		f := *w.failure
		return &f
	}
	return nil
}

// admitErrLocked says why nothing more may be admitted, if so.
func (w *Writer) admitErrLocked() error {
	if err := w.stopErrLocked(); err != nil {
		return err
	}
	if w.finished || w.closing {
		return errors.New("observation writer is closed")
	}
	return nil
}

// ObserveFrame appends f; it lets the writer serve directly as the
// pipeline's observation frame sink.
func (w *Writer) ObserveFrame(f l4bobserve.FrameRecord) error { return w.AppendFrame(f) }

// AppendFrame admits the next frame. A record that breaks the stream
// contract is refused and nothing is written. A valid frame over a declared
// limit is never truncated: a gap over its sequence is recorded and a
// *FrameTooLargeError returned. A frame that could not be admitted within
// the policy's shed wait is likewise a gap, with a *FrameShedError. After a
// capture failure it returns a *CaptureFailedError. A nil return means
// accepted, and durable too in strict mode.
func (w *Writer) AppendFrame(f l4bobserve.FrameRecord) error {
	w.appendMu.Lock()
	defer w.appendMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.admitErrLocked(); err != nil {
		return err
	}
	// Validate against a copy, so a refused or oversized frame leaves the
	// stream where it was.
	probe := w.stream
	if err := probe.AddFrame(f); err != nil {
		return fmt.Errorf("frame %d refused: %w", f.Sequence, err)
	}
	switch l := w.limits; {
	case uint64(f.Points.Len()) > uint64(l.MaxPointsPerFrame):
		return w.replaceFrameLocked(f, fmt.Sprintf("%d retained points exceed the limit of %d", f.Points.Len(), l.MaxPointsPerFrame), 0)
	case uint64(len(f.Clusters)) > uint64(l.MaxClustersPerFrame):
		return w.replaceFrameLocked(f, fmt.Sprintf("%d clusters exceed the limit of %d", len(f.Clusters), l.MaxClustersPerFrame), 0)
	case uint64(len(f.Stages)) > uint64(l.MaxStagesPerFrame):
		return w.replaceFrameLocked(f, fmt.Sprintf("%d stage counts exceed the limit of %d", len(f.Stages), l.MaxStagesPerFrame), 0)
	}
	record, err := w.encode(RecordFrame, f.Sequence, f.CaptureStartUnixNanos, frameToProto(f))
	if err != nil {
		return err
	}
	if payload := len(record) - envelopeSize; payload > int(w.limits.MaxRecordBytes) {
		return w.replaceFrameLocked(f, fmt.Sprintf("frame record of %d bytes exceeds the %d-byte limit", payload, w.limits.MaxRecordBytes), 0)
	}
	if waited, err := w.awaitRoomLocked(len(record), w.policy.ShedAfter); err != nil {
		if errors.Is(err, errShed) {
			return w.replaceFrameLocked(f, "", waited)
		}
		return err
	}
	e := entry{kind: RecordFrame, first: f.Sequence, count: 1, timeKnown: true, start: f.CaptureStartUnixNanos, end: f.CaptureEndUnixNanos}
	ordinal, err := w.writeRecordLocked(record, e, l4bobserve.FrameDigest(f), nil)
	if err != nil {
		return err
	}
	w.stream = probe
	return w.strictWaitLocked(ordinal)
}

// errShed is awaitRoomLocked's signal that the shed wait elapsed.
var errShed = errors.New("shed")

// replaceFrameLocked records a gap in place of a frame: one the limits
// refuse (detail set), or one shed after waiting.
func (w *Writer) replaceFrameLocked(f l4bobserve.FrameRecord, detail string, waited time.Duration) error {
	cause := "writer-limit: " + detail
	if detail == "" {
		cause = fmt.Sprintf("writer-backlog: frame not admitted within %s while a commit was pending", w.policy.ShedAfter)
	}
	gap := l4bobserve.GapRecord{
		HasSequenceRange: true, FirstSequence: f.Sequence, LastSequence: f.Sequence,
		MissingFrames: l4bobserve.KnownCount(1),
		Time:          l4bobserve.GapTimeBounded, StartUnixNanos: f.CaptureStartUnixNanos, EndUnixNanos: f.CaptureEndUnixNanos,
		Cause: cause,
	}
	mark := func(c *openChunk) { c.rejected++ }
	if detail == "" {
		mark = func(c *openChunk) { c.shed++ }
	}
	// A shed gap may exceed the open batch's byte threshold (never the chunk
	// bound): refusing it too would make the loss silent.
	if err := w.appendGapLocked(gap, detail == "", mark); err != nil {
		return fmt.Errorf("frame %d not admitted, and its gap could not be recorded: %w", f.Sequence, err)
	}
	if detail == "" {
		return &FrameShedError{Sequence: f.Sequence, Waited: waited}
	}
	return &FrameTooLargeError{Sequence: f.Sequence, Detail: detail}
}

// AppendGap admits an explicit gap at the current position. A gap is never
// shed; with the open batch full it waits for the committer.
func (w *Writer) AppendGap(g l4bobserve.GapRecord) error {
	w.appendMu.Lock()
	defer w.appendMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.appendGapLocked(g, false, nil)
}

func (w *Writer) appendGapLocked(g l4bobserve.GapRecord, relaxed bool, mark func(*openChunk)) error {
	if err := w.admitErrLocked(); err != nil {
		return err
	}
	probe := w.stream
	if err := probe.AddGap(g); err != nil {
		return fmt.Errorf("gap refused: %w", err)
	}
	e := gapEntry(g, w.stream.NextSequence())
	record, err := w.encode(RecordGap, e.first, e.envelopeTime(), gapToProto(g))
	if err != nil {
		return err
	}
	if payload := len(record) - envelopeSize; payload > int(w.limits.MaxRecordBytes) {
		return fmt.Errorf("gap record of %d bytes exceeds the %d-byte limit", payload, w.limits.MaxRecordBytes)
	}
	if relaxed {
		err = w.awaitLocked(func() bool { return w.fitsLocked(len(record), true) }, 0)
	} else {
		_, err = w.awaitRoomLocked(len(record), 0)
	}
	if err != nil {
		return err
	}
	ordinal, err := w.writeRecordLocked(record, e, l4bobserve.GapDigest(g), mark)
	if err != nil {
		return err
	}
	w.stream = probe
	return w.strictWaitLocked(ordinal)
}

// entry is one index row before it is packed into the parallel arrays.
type entry struct {
	kind       RecordKind
	first      uint64
	count      uint64
	timeKnown  bool
	start, end int64
}

// envelopeTime is the capture time the envelope carries: zero when unknown.
func (e entry) envelopeTime() int64 {
	if !e.timeKnown {
		return 0
	}
	return e.start
}

// gapEntry places a gap: a gap over assigned sequences covers them; a gap
// between received frames sits at the next sequence and covers none.
func gapEntry(g l4bobserve.GapRecord, next uint64) entry {
	e := entry{kind: RecordGap, first: next}
	if g.HasSequenceRange {
		e.first, e.count = g.FirstSequence, g.LastSequence-g.FirstSequence+1
	}
	if g.Time == l4bobserve.GapTimeBounded {
		e.timeKnown, e.start, e.end = true, g.StartUnixNanos, g.EndUnixNanos
	}
	return e
}

// encode marshals the payload directly after space for its envelope, so a
// large frame is not copied again to be framed.
func (w *Writer) encode(kind RecordKind, sequence uint64, captureTime int64, m proto.Message) ([]byte, error) {
	buf := append(w.scratch[:0], make([]byte, envelopeSize)...)
	buf, err := marshalOptions.MarshalAppend(buf, m)
	if err != nil {
		return nil, fmt.Errorf("encode %s %d: %w", kind, sequence, err)
	}
	w.scratch = buf
	putEnvelope(buf, envelope{kind: kind, tag: w.tag, sequence: sequence, time: captureTime})
	return buf, nil
}

// fitsLocked reports whether a record of size bytes may join the open batch
// now. A batch at its byte threshold takes no more frames; relaxed lets a
// small gap in up to the chunk bound.
func (w *Writer) fitsLocked(size int, relaxed bool) bool {
	c := w.open
	if c == nil {
		return true
	}
	if c.offset+uint64(size)+envelopeSize+maxSealBytes > w.limits.MaxChunkBytes || c.seal.RecordCount >= uint64(w.limits.MaxRecordsPerChunk) {
		return false
	}
	return relaxed || !c.full
}

// awaitRoomLocked waits until the open batch can take a record of size
// bytes. A full batch is handed to the committer; while another batch is
// committing, the append waits, up to shedAfter when that is set. The wait
// ends early on a capture failure, which the stall watchdog declares if the
// committer does not progress.
func (w *Writer) awaitRoomLocked(size int, shedAfter time.Duration) (time.Duration, error) {
	start := time.Now()
	if !w.fitsLocked(size, false) && w.open != nil && !w.open.full {
		// The chunk or record bound, not the byte threshold, is what stops it.
		w.open.full = true
		w.kickLocked()
	}
	err := w.awaitLocked(func() bool { return w.fitsLocked(size, false) }, shedAfter)
	return time.Since(start), err
}

// awaitLocked releases mu until ready holds, the capture stops, or limit
// (when positive) elapses, which returns errShed.
func (w *Writer) awaitLocked(ready func() bool, limit time.Duration) error {
	var deadline <-chan time.Time
	if limit > 0 {
		t := time.NewTimer(limit)
		defer t.Stop()
		deadline = t.C
	}
	for !ready() {
		if err := w.stopErrLocked(); err != nil {
			return err
		}
		w.kickLocked()
		ch := w.progress
		w.mu.Unlock()
		select {
		case <-ch:
			w.mu.Lock()
		case <-deadline:
			w.mu.Lock()
			if ready() {
				return nil
			}
			return errShed
		}
	}
	return w.stopErrLocked()
}

// strictWaitLocked waits, in strict mode, until record ordinal is durable.
func (w *Writer) strictWaitLocked(ordinal uint64) error {
	if !w.policy.Strict {
		return nil
	}
	if c := w.open; c != nil && c.forced == pb.CommitTrigger_COMMIT_TRIGGER_UNSPECIFIED {
		c.forced = pb.CommitTrigger_COMMIT_TRIGGER_STRICT
	}
	w.kickLocked()
	return w.awaitLocked(func() bool { return w.durable.records > ordinal }, 0)
}

// writeRecordLocked writes one framed record into the open batch, opening
// one if needed, and returns its ordinal in the stream. A failed write is a
// capture failure: the batch's last record may be partial, so it is never
// sealed.
func (w *Writer) writeRecordLocked(record []byte, e entry, digest l4bobserve.Digest, mark func(*openChunk)) (uint64, error) {
	if w.open == nil {
		if err := w.openBatchLocked(); err != nil {
			return 0, err
		}
	}
	c := w.open
	size := uint64(len(record))
	if err := w.writeFile(c.file, c.openRel, record, bytesChunk); err != nil {
		return 0, w.failLocked(classify(err), fmt.Errorf("write %s: %w", c.openRel, err))
	}
	c.hash.Write(record)
	ix := &c.index
	ix.Offset = append(ix.Offset, c.offset)
	ix.Length = append(ix.Length, uint32(size))
	ix.Kind = append(ix.Kind, uint32(e.kind))
	ix.SequenceFirst = append(ix.SequenceFirst, e.first)
	ix.SequenceCount = append(ix.SequenceCount, e.count)
	ix.TimeStart = append(ix.TimeStart, e.start)
	ix.TimeEnd = append(ix.TimeEnd, e.end)
	var flags uint32
	if e.timeKnown {
		flags = indexTimeKnown
	}
	ix.Flags = append(ix.Flags, flags)
	ix.Epoch = append(ix.Epoch, 0)
	c.offset += size

	s := &c.seal
	if s.RecordCount == 0 {
		s.FirstSequence = e.first
		c.firstAccepted = time.Now()
	}
	s.RecordCount++
	s.EndSequence = e.first + e.count
	switch e.kind {
	case RecordFrame:
		s.FrameCount++
		c.semantic.AddFrame(digest)
		w.semantic.AddFrame(digest)
		w.accepted.frames++
	case RecordGap:
		s.GapCount++
		c.semantic.AddGap(digest)
		w.semantic.AddGap(digest)
		w.accepted.gaps++
	}
	if e.timeKnown {
		if s.FirstStartUnixNanos == nil {
			s.FirstStartUnixNanos = proto.Int64(e.start)
		}
		s.LastStartUnixNanos = proto.Int64(e.start)
	}
	if mark != nil {
		mark(c)
	}
	ordinal := w.accepted.records
	w.accepted.records++
	w.accepted.payload += size - envelopeSize
	if s.RecordCount == 1 {
		w.armAgeLocked(c)
		w.armWatchdogLocked()
	}
	if c.offset >= w.policy.MaxBatchBytes {
		c.full = true
		w.kickLocked()
	}
	return ordinal, nil
}

const indexTimeKnown uint32 = 1

func (w *Writer) openBatchLocked() error {
	ordinal := w.nextOrdinal
	rel := chunksDir + "/" + chunkBase(ordinal) + chunkSuffix + openSuffix
	file, err := os.OpenFile(filepath.Join(w.dir, rel), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return w.failLocked(classify(err), err)
	}
	w.files.Add(1)
	header, err := marshalOptions.Marshal(&pb.ChunkHeader{Ordinal: ordinal, ManifestSha256: w.manifestDigest[:]})
	if err != nil {
		file.Close()
		return w.failLocked(FailureIO, err)
	}
	prefix := appendRecord(appendPreamble(nil, objectChunk), envelope{kind: RecordChunkHeader, tag: w.tag, sequence: ordinal}, header)
	c := &openChunk{ordinal: ordinal, file: file, openRel: rel, hash: sha256.New(), semantic: l4bobserve.NewStreamDigest()}
	w.open = c
	w.nextOrdinal++
	if err := w.writeFile(file, rel, prefix, bytesChunk); err != nil {
		return w.failLocked(classify(err), fmt.Errorf("write %s: %w", rel, err))
	}
	c.hash.Write(prefix)
	c.offset = uint64(len(prefix))
	return nil
}

// sealLocked closes the open batch (step 1): it appends the seal and builds
// the index object and the generation's chunk entry. The batch moves to
// committing.
func (w *Writer) sealLocked(trigger pb.CommitTrigger) error {
	c := w.open
	s := &c.seal
	s.Ordinal = c.ordinal
	s.BodyBytes = c.offset
	s.BodySha256 = c.hash.Sum(nil)
	semantic := c.semantic.Sum()
	s.SemanticSha256 = semantic[:]
	payload, err := marshalOptions.Marshal(s)
	if err != nil {
		return w.failLocked(FailureIO, err)
	}
	seal := appendRecord(nil, envelope{kind: RecordChunkSeal, tag: w.tag, sequence: c.ordinal}, payload)
	if err := w.writeFile(c.file, c.openRel, seal, bytesChunk); err != nil {
		return w.failLocked(classify(err), fmt.Errorf("write %s: %w", c.openRel, err))
	}
	ix := &c.index
	ix.Ordinal = c.ordinal
	ix.ChunkBytes = c.offset + uint64(len(seal))
	ix.ChunkBodySha256 = s.BodySha256
	ix.SealOffset = c.offset
	ix.SealLength = uint32(len(seal))
	payload, err = marshalOptions.Marshal(ix)
	if err != nil {
		return w.failLocked(FailureIO, err)
	}
	if len(payload) > maxIndexBytes {
		return w.failLocked(FailureIO, fmt.Errorf("chunk %d index of %d bytes exceeds %d", c.ordinal, len(payload), maxIndexBytes))
	}
	c.indexObject = appendRecord(appendPreamble(nil, objectIndex), envelope{kind: RecordChunkIndex, tag: w.tag, sequence: c.ordinal}, payload)
	indexSHA := sha256.Sum256(c.indexObject)
	c.committed = &pb.CommittedChunk{Ordinal: c.ordinal, ChunkBytes: ix.ChunkBytes, BodySha256: s.BodySha256, IndexSha256: indexSHA[:],
		FirstSequence: s.FirstSequence, EndSequence: s.EndSequence, RecordCount: s.RecordCount}
	c.trigger = trigger
	w.committing, w.open = c, nil
	return nil
}

// commitJob is one generation to publish.
type commitJob struct {
	batch    *openChunk
	delta    batchDelta
	prev     chainState
	summary  []byte
	value    Summary
	firstAge time.Duration
	// releaseReserve frees the failure reserve before anything is written.
	releaseReserve bool
	// err, when set, fails the job before anything is published.
	err error
}

// commitLoop is the committer goroutine: it takes due batches and publishes
// them one generation at a time, then the failure marker if the capture
// fails, and exits when the writer stops.
func (w *Writer) commitLoop() {
	defer func() {
		w.mu.Lock()
		w.settled = true
		w.broadcastLocked()
		w.mu.Unlock()
		close(w.exited)
	}()
	for {
		w.mu.Lock()
		job, exit := w.nextJobLocked(time.Now())
		w.mu.Unlock()
		if exit {
			return
		}
		if job == nil {
			<-w.wake
			continue
		}
		res := w.publish(job)
		w.mu.Lock()
		w.finishLocked(job, res)
		w.mu.Unlock()
	}
}

// nextJobLocked returns the next generation to publish, nil to wait for a
// wake-up, or exit.
func (w *Writer) nextJobLocked(now time.Time) (*commitJob, bool) {
	if w.dead {
		return nil, true
	}
	if w.failure != nil {
		if w.markerTried {
			return nil, true
		}
		w.markerTried = true
		// The open batch was accepted, not committed; the failure record
		// states its extent. Its file stays on disk for recovery to
		// quarantine, never to read.
		if c := w.open; c != nil {
			c.file.Close()
			w.open = nil
		}
		f := w.failure
		return &commitJob{prev: w.published, releaseReserve: true, delta: batchDelta{
			kind: pb.GenerationKind_GENERATION_KIND_FAILURE,
			failure: &pb.CaptureFailure{Cause: string(f.Cause), Detail: f.Err.Error(), AcceptedEndSequence: f.AcceptedEndSequence,
				AcceptedRecordCount: f.AcceptedRecords, FailedUnixNanos: f.At.UnixNano()},
		}}, false
	}
	if w.committing != nil {
		return nil, false
	}
	if c := w.open; c != nil {
		trigger := c.forced
		switch {
		case w.closing:
			trigger = pb.CommitTrigger_COMMIT_TRIGGER_CLOSE
		case trigger != pb.CommitTrigger_COMMIT_TRIGGER_UNSPECIFIED:
		case w.stopping:
			trigger = pb.CommitTrigger_COMMIT_TRIGGER_FLUSH
		case c.full:
			trigger = pb.CommitTrigger_COMMIT_TRIGGER_BATCH_BYTES
		case !w.policy.Strict && now.Sub(c.firstAccepted) >= w.policy.MaxBatchAge:
			trigger = pb.CommitTrigger_COMMIT_TRIGGER_BATCH_AGE
		default:
			w.armAgeLocked(c)
			return nil, false
		}
		if err := w.sealLocked(trigger); err != nil {
			return w.nextJobLocked(now)
		}
		// An append waiting for room can open the next batch now, while
		// this one is published.
		w.broadcastLocked()
		job := &commitJob{batch: w.committing, prev: w.published, firstAge: now.Sub(c.firstAccepted), delta: batchDelta{
			kind: pb.GenerationKind_GENERATION_KIND_COMMIT, trigger: trigger, chunk: c.committed,
			frames: c.seal.FrameCount, gaps: c.seal.GapCount, rejected: c.rejected, shed: c.shed,
			batchAgeNanos: int64(now.Sub(c.firstAccepted)),
		}}
		if w.closing {
			w.closeJobLocked(job)
		}
		return job, false
	}
	if w.closing && !w.closeDone {
		job := &commitJob{prev: w.published, delta: batchDelta{trigger: pb.CommitTrigger_COMMIT_TRIGGER_CLOSE}}
		w.closeJobLocked(job)
		return job, false
	}
	return nil, w.stopping || w.closeDone
}

// closeJobLocked makes job the close generation: it publishes the closing
// summary, computed from the chain the job completes.
func (w *Writer) closeJobLocked(job *commitJob) {
	job.delta.kind = pb.GenerationKind_GENERATION_KIND_CLOSE
	_, after := job.prev.apply(w.manifestDigest, job.delta)
	publish := w.publishLatency.summary()
	s := Summary{Chunks: after.chunks, ChunkChain: after.chain, Records: after.records, Frames: after.frames, Gaps: after.gaps,
		EndSequence: after.endSequence, RejectedFrames: after.rejected, ShedFrames: after.shed, ChunkBytes: after.chunkBytes,
		Semantic: w.semantic.Sum(), Commits: publish.Count, CommitLatency: publish}
	payload, err := marshalOptions.Marshal(&pb.CaptureSummary{
		ChunkCount: s.Chunks, ChunkChainSha256: s.ChunkChain[:], RecordCount: s.Records,
		FrameCount: s.Frames, GapCount: s.Gaps, EndSequence: s.EndSequence,
		RejectedFrames: s.RejectedFrames, SemanticSha256: s.Semantic[:], TotalChunkBytes: s.ChunkBytes, ShedFrames: s.ShedFrames,
		CommitCount: s.Commits, CommitP50Nanos: int64(publish.P50), CommitP95Nanos: int64(publish.P95),
		CommitP99Nanos: int64(publish.P99), CommitMaxNanos: int64(publish.Max),
	})
	if err != nil {
		// Not expected for fixed-size fields; if it happens, publish fails
		// the job and the capture, rather than committing an empty summary.
		job.err = fmt.Errorf("encode closing summary: %w", err)
		return
	}
	job.summary = appendRecord(appendPreamble(nil, objectSummary), envelope{kind: RecordSummary, tag: w.tag}, payload)
	sum := sha256.Sum256(job.summary)
	job.delta.objects = []*pb.CommittedObject{{Name: summaryName, Bytes: uint64(len(job.summary)), Sha256: sum[:]}}
	job.value = s
}

// publishResult is what publish achieved. published: the generation object
// is in place under its name, whatever happened after; durable: every step
// completed.
type publishResult struct {
	state      chainState
	published  bool
	err        error
	sync, took time.Duration
}

// publish runs steps 2 to 4 of the protocol for job, without the lock:
// synchronise and publish the chunk, its index and any object, then the
// generation, then the pointer, synchronising each directory. Nothing is
// announced here.
func (w *Writer) publish(job *commitJob) publishResult {
	start := time.Now()
	n := job.prev.next()
	var res publishResult
	fail := func(err error) publishResult { res.err = err; return res }
	if job.err != nil {
		return fail(job.err)
	}
	if job.releaseReserve {
		// Freeing the reserve is what lets a full disk take the marker.
		_ = os.Remove(filepath.Join(w.dir, reserveName))
	}
	if b := job.batch; b != nil {
		if w.killAt(stepSealed, n) {
			return fail(errWriterKilled)
		}
		if err := w.syncFile(b.file, b.openRel); err != nil {
			b.file.Close()
			return fail(fmt.Errorf("sync %s: %w", b.openRel, err))
		}
		if w.killAt(stepChunkSynced, n) {
			return fail(errWriterKilled)
		}
		if err := b.file.Close(); err != nil {
			return fail(err)
		}
		base := chunksDir + "/" + chunkBase(b.ordinal)
		if err := os.Rename(filepath.Join(w.dir, b.openRel), filepath.Join(w.dir, base+chunkSuffix)); err != nil {
			return fail(err)
		}
		if w.killAt(stepChunkPublished, n) {
			return fail(errWriterKilled)
		}
		if _, err := w.writeObject(base+indexSuffix, b.indexObject, bytesIndex); err != nil {
			return fail(err)
		}
		if w.killAt(stepIndexPublished, n) {
			return fail(errWriterKilled)
		}
		if err := w.syncDirectory(chunksDir); err != nil {
			return fail(err)
		}
	}
	if job.summary != nil {
		if _, err := w.writeObject(summaryName, job.summary, bytesSummary); err != nil {
			return fail(err)
		}
		if err := w.syncDirectory("."); err != nil {
			return fail(err)
		}
	}
	if w.killAt(stepObjectsDurable, n) {
		return fail(errWriterKilled)
	}
	res.sync = time.Since(start)
	job.delta.syncNanos = int64(res.sync)
	job.delta.committedNanos = time.Now().UnixNano()
	g, next := job.prev.apply(w.manifestDigest, job.delta)
	data, err := encodeGeneration(w.tag, g)
	if err != nil {
		return fail(err)
	}
	next.digest = sha256.Sum256(data)
	rel := generationObject(n)
	if err := w.writeTemp(rel, data, bytesGeneration); err != nil {
		return fail(err)
	}
	if w.killAt(stepGenerationWritten, n) {
		return fail(errWriterKilled)
	}
	if err := os.Rename(filepath.Join(w.dir, rel+openSuffix), filepath.Join(w.dir, rel)); err != nil {
		return fail(err)
	}
	res.state, res.published = next, true
	if w.killAt(stepGenerationPublished, n) {
		return fail(errWriterKilled)
	}
	if err := w.syncDirectory(generationsDir); err != nil {
		return fail(err)
	}
	if w.killAt(stepGenerationDurable, n) {
		return fail(errWriterKilled)
	}
	pointer, err := encodeCurrent(w.tag, next)
	if err != nil {
		return fail(err)
	}
	if err := w.writeTemp(currentName, pointer, bytesCurrent); err != nil {
		return fail(err)
	}
	if w.killAt(stepPointerWritten, n) {
		return fail(errWriterKilled)
	}
	if err := os.Rename(filepath.Join(w.dir, currentName+openSuffix), filepath.Join(w.dir, currentName)); err != nil {
		return fail(err)
	}
	if w.killAt(stepPointerPublished, n) {
		return fail(errWriterKilled)
	}
	if err := w.syncDirectory("."); err != nil {
		return fail(err)
	}
	if w.killAt(stepPointerDurable, n) {
		return fail(errWriterKilled)
	}
	res.took = time.Since(start)
	return res
}

// finishLocked applies a publication's outcome: the new durable frontier and
// its announcement, or a capture failure.
func (w *Writer) finishLocked(job *commitJob, res publishResult) {
	if w.dead {
		return
	}
	if res.published {
		w.published = res.state
	}
	if job.batch != nil {
		w.committing = nil
	}
	if res.err != nil {
		if errors.Is(res.err, errWriterKilled) {
			return
		}
		if job.delta.kind != pb.GenerationKind_GENERATION_KIND_FAILURE {
			w.failLocked(classify(res.err), res.err)
		}
		w.broadcastLocked()
		return
	}
	w.durable = res.state
	w.publishLatency.add(res.took)
	w.syncLatency.add(res.sync)
	if job.batch != nil {
		w.batchAge.add(job.firstAge)
	}
	switch job.delta.kind {
	case pb.GenerationKind_GENERATION_KIND_CLOSE:
		w.closeDone = true
		w.summary = job.value
	case pb.GenerationKind_GENERATION_KIND_FAILURE:
		w.failure.MarkerWritten = true
	}
	if w.failure != nil {
		w.failure.CommittedEndSequence, w.failure.CommittedRecords = w.durable.endSequence, w.durable.records
	}
	w.armWatchdogLocked()
	w.broadcastLocked()
}

// failLocked enters capture failure, once, and returns the failure.
func (w *Writer) failLocked(cause FailureCause, err error) error {
	if w.failure == nil && !w.dead && !w.closeDone {
		w.failure = &CaptureFailedError{Cause: cause, Err: err, At: time.Now(),
			CommittedEndSequence: w.durable.endSequence, CommittedRecords: w.durable.records,
			AcceptedEndSequence: w.stream.NextSequence(), AcceptedRecords: w.accepted.records}
		w.stopTimersLocked()
		w.broadcastLocked()
		w.kickLocked()
	}
	if stop := w.stopErrLocked(); stop != nil {
		return stop
	}
	return err
}

func classify(err error) FailureCause {
	if errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT) {
		return FailureDiskFull
	}
	return FailureIO
}

func (w *Writer) kickLocked() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Writer) kick() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Writer) broadcastLocked() {
	if w.progress == nil {
		return // recovery's publisher has no waiters
	}
	close(w.progress)
	w.progress = make(chan struct{})
}

// armAgeLocked wakes the committer when the open batch reaches its age.
func (w *Writer) armAgeLocked(c *openChunk) {
	if w.policy.Strict || c == nil || c.seal.RecordCount == 0 {
		return
	}
	d := time.Until(c.firstAccepted.Add(w.policy.MaxBatchAge))
	if w.ageTimer == nil {
		w.ageTimer = time.AfterFunc(d, w.kick)
		return
	}
	w.ageTimer.Reset(d)
}

// oldestUndurableLocked is when the oldest accepted, undurable record was
// accepted.
func (w *Writer) oldestUndurableLocked() (time.Time, bool) {
	if c := w.committing; c != nil {
		return c.firstAccepted, true
	}
	if c := w.open; c != nil && c.seal.RecordCount > 0 {
		return c.firstAccepted, true
	}
	return time.Time{}, false
}

// armWatchdogLocked schedules the stall check for the oldest undurable
// record. It does not depend on the committer, which may be the thing
// stalled inside a sync.
func (w *Writer) armWatchdogLocked() {
	oldest, ok := w.oldestUndurableLocked()
	if !ok || w.failure != nil {
		if w.watchdog != nil {
			w.watchdog.Stop()
		}
		return
	}
	d := time.Until(oldest.Add(w.policy.stallBound()))
	if w.watchdog == nil {
		w.watchdog = time.AfterFunc(d, w.checkStall)
		return
	}
	w.watchdog.Reset(d)
}

func (w *Writer) checkStall() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failure != nil || w.dead || w.finished {
		return
	}
	oldest, ok := w.oldestUndurableLocked()
	if !ok {
		return
	}
	if age := time.Since(oldest); age >= w.policy.stallBound() {
		w.failLocked(FailureStall, fmt.Errorf("a record accepted %s ago is not durable; the declared bound is %s (batch age %s + commit deadline %s)",
			age.Round(time.Millisecond), w.policy.stallBound(), w.policy.batchAge(), w.policy.CommitDeadline))
		return
	}
	w.armWatchdogLocked()
}

func (w *Writer) stopTimersLocked() {
	if w.ageTimer != nil {
		w.ageTimer.Stop()
	}
	if w.watchdog != nil {
		w.watchdog.Stop()
	}
}

// Frontier returns the durable frontier and what has been accepted.
func (w *Writer) Frontier() Frontier {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.frontierLocked()
}

func (w *Writer) frontierLocked() Frontier {
	d := w.durable
	f := Frontier{Generation: d.generation, Chunks: d.chunks, Records: d.records, Frames: d.frames, Gaps: d.gaps,
		EndSequence: d.endSequence, ChunkBytes: d.chunkBytes, AcceptedRecords: w.accepted.records,
		AcceptedEndSequence: w.stream.NextSequence(), Final: w.settled || w.dead}
	switch {
	case w.failure != nil || w.dead:
		f.State = CaptureFailed
		if w.failure != nil {
			failure := *w.failure
			f.Failure = &failure
		} else {
			f.Failure = &CaptureFailedError{Cause: FailureIO, Err: errWriterKilled, CommittedEndSequence: d.endSequence,
				CommittedRecords: d.records, AcceptedEndSequence: f.AcceptedEndSequence, AcceptedRecords: f.AcceptedRecords}
		}
	case w.closeDone:
		f.State = CaptureClosed
	case w.finished:
		// Abandoned: stopped without a terminal generation.
		f.State = CaptureIncomplete
	}
	return f
}

// WaitFrontier blocks until the durable generation exceeds after, the
// frontier is final, or ctx ends. The writer never waits for it.
func (w *Writer) WaitFrontier(ctx context.Context, after uint64) (Frontier, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for {
		f := w.frontierLocked()
		if f.Generation > after || f.Final {
			return f, nil
		}
		ch := w.progress
		w.mu.Unlock()
		select {
		case <-ch:
			w.mu.Lock()
		case <-ctx.Done():
			w.mu.Lock()
			return w.frontierLocked(), ctx.Err()
		}
	}
}

// Stats returns what the writer has measured.
func (w *Writer) Stats() WriterStats {
	w.mu.Lock()
	s := WriterStats{Publish: w.publishLatency.summary(), Sync: w.syncLatency.summary(), BatchAge: w.batchAge.summary(),
		PayloadBytes: w.accepted.payload, ObjectBytes: map[string]uint64{}}
	w.mu.Unlock()
	for i := range objectKinds {
		s.ObjectBytes[objectKindNames[i]] = w.objectBytes[i].Load()
	}
	s.Files, s.Syncs = w.files.Load(), w.syncs.Load()
	return s
}

// Flush closes the open batch now and waits until everything accepted is
// durable, or the capture fails.
func (w *Writer) Flush() error {
	w.appendMu.Lock()
	defer w.appendMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked()
}

func (w *Writer) flushLocked() error {
	if err := w.admitErrLocked(); err != nil {
		return err
	}
	if c := w.open; c != nil && c.forced == pb.CommitTrigger_COMMIT_TRIGGER_UNSPECIFIED {
		c.forced = pb.CommitTrigger_COMMIT_TRIGGER_FLUSH
	}
	want := w.accepted.records
	return w.awaitLocked(func() bool { return w.durable.records >= want }, 0)
}

// Close commits everything accepted and ends the chain with a close
// generation publishing the closing summary. Only then is the capture
// closed; a reader tells it from an interrupted one by that generation.
// After a capture failure Close releases the container and returns the
// failure with the committed account.
func (w *Writer) Close() (Summary, error) {
	w.appendMu.Lock()
	defer w.appendMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		switch {
		case w.dead:
			return w.committedSummaryLocked(), errWriterKilled
		case w.failure != nil:
			f := *w.failure
			return w.committedSummaryLocked(), &f
		case !w.closeDone:
			return w.committedSummaryLocked(), errors.New("observation writer was abandoned, not closed")
		}
		return w.summary, nil
	}
	if w.failure == nil && !w.dead {
		w.closing = true
		w.kickLocked()
		// The close generation may commit no records, so the stall watchdog
		// does not cover it; bound the wait here.
		bound := w.policy.stallBound() + w.policy.CommitDeadline
		if err := w.awaitLocked(func() bool { return w.closeDone }, bound); errors.Is(err, errShed) {
			w.failLocked(FailureStall, fmt.Errorf("the close generation was not durable within %s", bound))
		}
	}
	w.shutdownLocked()
	if w.dead {
		return w.committedSummaryLocked(), errWriterKilled
	}
	if w.failure != nil {
		f := *w.failure
		return w.committedSummaryLocked(), &f
	}
	return w.summary, nil
}

// Fail ends the capture abnormally but deliberately, for a reason the
// owner states (a replay that failed, a source that went away): what was
// accepted is committed first, then a failure generation records the
// reason. It returns an error only if the marker could not be written.
func (w *Writer) Fail(reason string) error {
	w.appendMu.Lock()
	defer w.appendMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return nil
	}
	if w.failure == nil && !w.dead {
		_ = w.flushLocked()
		w.failLocked(FailureStopped, errors.New(reason))
	}
	w.shutdownLocked()
	if w.failure != nil && !w.failure.MarkerWritten {
		return fmt.Errorf("failure marker not written: %w", w.failure)
	}
	return nil
}

// Abandon commits what has been accepted, if the writer is healthy, and
// stops without a terminal generation, as a crash would: the container
// reads as open until Recover marks it incomplete. Use Close or Fail when
// the outcome is known.
func (w *Writer) Abandon() error {
	w.appendMu.Lock()
	defer w.appendMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return nil
	}
	err := w.flushLocked()
	if w.failure != nil {
		err = nil // the failure is already recorded, or its marker attempted
	}
	w.shutdownLocked()
	return err
}

// shutdownLocked stops the committer (after the failure marker, if one is
// pending) and releases the container lock and the reserve. It releases mu
// while it waits and holds it again on return. The wait is bounded: a sync
// that never returns cannot be cancelled, and its goroutine is left behind
// rather than blocking the caller.
func (w *Writer) shutdownLocked() {
	w.stopping = true
	w.stopTimersLocked()
	w.kickLocked()
	w.mu.Unlock()
	select {
	case <-w.exited:
	case <-time.After(w.policy.CommitDeadline):
	}
	w.mu.Lock()
	w.finished = true
	if c := w.open; c != nil {
		c.file.Close()
		w.open = nil
	}
	if !w.dead && (w.failure == nil || w.failure.MarkerWritten) {
		_ = os.Remove(filepath.Join(w.dir, reserveName))
	}
	w.releaseLocked()
	w.broadcastLocked()
}

// releaseLocked drops the container lock.
func (w *Writer) releaseLocked() {
	if w.lock != nil {
		w.lock.Close() // closing the descriptor releases the flock
		w.lock = nil
	}
}

// committedSummaryLocked is the committed account as a Summary, for a
// capture that did not close.
func (w *Writer) committedSummaryLocked() Summary {
	d := w.durable
	return Summary{Chunks: d.chunks, ChunkChain: d.chain, Records: d.records, Frames: d.frames, Gaps: d.gaps,
		EndSequence: d.endSequence, RejectedFrames: d.rejected, ShedFrames: d.shed, ChunkBytes: d.chunkBytes,
		Commits: w.publishLatency.count, CommitLatency: w.publishLatency.summary()}
}

// die stops the writer as a killed process stops: descriptors close (the
// kernel would close them) and nothing else happens.
func (w *Writer) die() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.dead {
		return
	}
	w.dead, w.finished = true, true
	w.stopTimersLocked()
	for _, c := range []*openChunk{w.open, w.committing} {
		if c != nil {
			c.file.Close()
		}
	}
	w.releaseLocked()
	w.broadcastLocked()
	w.kickLocked()
}

func (w *Writer) killAt(step commitStep, generation uint64) bool {
	h := w.hooks.Load()
	if h == nil || h.kill == nil || !h.kill(step, generation) {
		return false
	}
	w.die()
	return true
}

// writeFile writes b to f through the fault hook, counting the bytes.
func (w *Writer) writeFile(f *os.File, rel string, b []byte, kind int) error {
	if h := w.hooks.Load(); h != nil && h.write != nil {
		if err := h.write(rel); err != nil {
			return err
		}
	}
	n, err := f.Write(b)
	w.objectBytes[kind].Add(uint64(n))
	return err
}

func (w *Writer) syncFile(f *os.File, rel string) error {
	w.syncs.Add(1)
	if h := w.hooks.Load(); h != nil && h.sync != nil {
		if err := h.sync(rel); err != nil {
			return err
		}
	}
	return f.Sync()
}

func (w *Writer) syncDirectory(rel string) error {
	w.syncs.Add(1)
	if h := w.hooks.Load(); h != nil && h.sync != nil {
		if err := h.sync(rel + "/"); err != nil {
			return err
		}
	}
	return syncDir(filepath.Join(w.dir, rel))
}

// writeTemp writes data under rel's temporary name and synchronises it.
func (w *Writer) writeTemp(rel string, data []byte, kind int) error {
	tmp := rel + openSuffix
	file, err := os.OpenFile(filepath.Join(w.dir, tmp), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	w.files.Add(1)
	if err := w.writeFile(file, tmp, data, kind); err != nil {
		file.Close()
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := w.syncFile(file, tmp); err != nil {
		file.Close()
		return fmt.Errorf("sync %s: %w", tmp, err)
	}
	return file.Close()
}

// writeObject publishes an immutable object: temporary name, sync, rename.
// The caller synchronises the directory.
func (w *Writer) writeObject(rel string, data []byte, kind int) (bool, error) {
	if err := w.writeTemp(rel, data, kind); err != nil {
		return false, err
	}
	if err := os.Rename(filepath.Join(w.dir, rel+openSuffix), filepath.Join(w.dir, rel)); err != nil {
		return false, err
	}
	return true, nil
}

// chainDigest is chain_i = SHA-256(chain_{i-1} || body_i).
func chainDigest(previous l4bobserve.Digest, body []byte) l4bobserve.Digest {
	h := sha256.New()
	h.Write(previous[:])
	h.Write(body)
	var d l4bobserve.Digest
	h.Sum(d[:0])
	return d
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync directory %s: %w", dir, err)
	}
	return nil
}
