package vrlog

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/google/uuid"
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
	ChunkBytes     uint64
	// Semantic is the l4bobserve stream digest of every record in order.
	Semantic l4bobserve.Digest
}

// Writer appends one extraction's frame and gap records to a new container.
// It is the phase-1 sealed-chunk writer: records are checked against the
// stream contract, encoded, written into the open chunk and sealed into
// immutable chunk and index objects at the target size. It does not publish
// commit generations or a durable frontier; a reader sees sealed chunks only,
// and a crash leaves at most one unsealed tail, which readers never treat as
// evidence. It is not safe for concurrent use.
type Writer struct {
	dir            string
	manifest       Manifest
	manifestDigest [sha256.Size]byte
	tag            [8]byte
	limits         Limits

	stream   l4bobserve.StreamValidator
	semantic *l4bobserve.StreamDigest
	chunk    *openChunk
	summary  Summary
	scratch  []byte

	// err is sticky: after a failed write the open chunk's contents are
	// uncertain, so nothing more is appended and it is never sealed.
	err    error
	closed bool
}

type openChunk struct {
	ordinal  uint64
	file     *os.File
	openPath string
	hash     hash.Hash
	semantic *l4bobserve.StreamDigest
	offset   uint64
	index    pb.ChunkIndex
	seal     pb.ChunkSeal
}

// Create makes dir, which must not exist, and durably writes its manifest
// before any record is admitted. An empty profile is foreground-complete, the
// only record family this writer produces; zero limits are the defaults; an
// empty capture UUID and creation time are allocated here.
func Create(dir string, m Manifest) (*Writer, error) {
	declared := l4bobserve.ForegroundComplete()
	switch {
	case m.Profile.Name == "" && len(m.Profile.Capabilities.List()) == 0:
		m.Profile = declared
	case m.Profile.Name != declared.Name:
		return nil, fmt.Errorf("the observation writer records profile %q, not %q", declared.Name, m.Profile.Name)
	}
	if len(m.RequiredFeatures) != 0 {
		return nil, fmt.Errorf("this writer implements no optional required features, got %v", m.RequiredFeatures)
	}
	if m.Limits == (Limits{}) {
		m.Limits = DefaultLimits()
	}
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
	if err := os.Mkdir(filepath.Join(dir, chunksDir), 0o755); err != nil {
		return nil, err
	}
	if err := writeObject(filepath.Join(dir, manifestName), object); err != nil {
		return nil, err
	}
	if err := syncDir(dir); err != nil {
		return nil, err
	}
	w := &Writer{
		dir:            dir,
		manifest:       m,
		manifestDigest: sha256.Sum256(object),
		limits:         m.Limits,
		stream:         *l4bobserve.NewStreamValidator(m.Profile),
		semantic:       l4bobserve.NewStreamDigest(),
	}
	copy(w.tag[:], w.manifestDigest[:8])
	return w, nil
}

// Dir is the container directory.
func (w *Writer) Dir() string { return w.dir }

// Manifest is the manifest as written, with the allocated identity.
func (w *Writer) Manifest() Manifest { return w.manifest }

func (w *Writer) usable() error {
	switch {
	case w.closed:
		return errors.New("observation writer is closed")
	case w.err != nil:
		return fmt.Errorf("observation writer failed earlier: %w", w.err)
	}
	return nil
}

// AppendFrame admits the next frame. A record that breaks the stream
// contract is refused and nothing is written. A valid frame over a declared
// limit is never truncated: the writer records a gap over its sequence and
// returns a *FrameTooLargeError, and the stream continues.
func (w *Writer) AppendFrame(f l4bobserve.FrameRecord) error {
	if err := w.usable(); err != nil {
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
		return w.reject(f, fmt.Sprintf("%d retained points exceed the limit of %d", f.Points.Len(), l.MaxPointsPerFrame))
	case uint64(len(f.Clusters)) > uint64(l.MaxClustersPerFrame):
		return w.reject(f, fmt.Sprintf("%d clusters exceed the limit of %d", len(f.Clusters), l.MaxClustersPerFrame))
	case uint64(len(f.Stages)) > uint64(l.MaxStagesPerFrame):
		return w.reject(f, fmt.Sprintf("%d stage counts exceed the limit of %d", len(f.Stages), l.MaxStagesPerFrame))
	}
	record, err := w.encode(RecordFrame, f.Sequence, f.CaptureStartUnixNanos, frameToProto(f))
	if err != nil {
		return err
	}
	if payload := len(record) - envelopeSize; payload > int(w.limits.MaxRecordBytes) {
		return w.reject(f, fmt.Sprintf("frame record of %d bytes exceeds the %d-byte limit", payload, w.limits.MaxRecordBytes))
	}
	w.stream = probe
	w.summary.Frames++
	return w.append(record, entry{kind: RecordFrame, first: f.Sequence, count: 1, timeKnown: true,
		start: f.CaptureStartUnixNanos, end: f.CaptureEndUnixNanos}, l4bobserve.FrameDigest(f))
}

// reject records a gap in place of a frame the limits refuse.
func (w *Writer) reject(f l4bobserve.FrameRecord, detail string) error {
	gap := l4bobserve.GapRecord{
		HasSequenceRange: true, FirstSequence: f.Sequence, LastSequence: f.Sequence,
		MissingFrames: l4bobserve.KnownCount(1),
		Time:          l4bobserve.GapTimeBounded, StartUnixNanos: f.CaptureStartUnixNanos, EndUnixNanos: f.CaptureEndUnixNanos,
		Cause: "writer-limit: " + detail,
	}
	if err := w.AppendGap(gap); err != nil {
		return fmt.Errorf("frame %d over a limit (%s), and its gap could not be recorded: %w", f.Sequence, detail, err)
	}
	w.summary.RejectedFrames++
	return &FrameTooLargeError{Sequence: f.Sequence, Detail: detail}
}

// AppendGap admits an explicit gap at the current position.
func (w *Writer) AppendGap(g l4bobserve.GapRecord) error {
	if err := w.usable(); err != nil {
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
	w.stream = probe
	w.summary.Gaps++
	return w.append(record, e, l4bobserve.GapDigest(g))
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

// append writes one framed record, rotating first if it would cross the
// chunk bound and sealing after it once the target size is reached.
func (w *Writer) append(record []byte, e entry, digest l4bobserve.Digest) error {
	size := uint64(len(record))
	if c := w.chunk; c != nil && (c.offset+size+envelopeSize+maxSealBytes > w.limits.MaxChunkBytes ||
		c.seal.RecordCount >= uint64(w.limits.MaxRecordsPerChunk)) {
		if err := w.sealChunk(); err != nil {
			return err
		}
	}
	if w.chunk == nil {
		if err := w.openChunk(); err != nil {
			return err
		}
	}
	c := w.chunk
	if _, err := c.file.Write(record); err != nil {
		return w.fail(fmt.Errorf("write %s: %w", c.openPath, err))
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
	}
	s.RecordCount++
	s.EndSequence = e.first + e.count
	switch e.kind {
	case RecordFrame:
		s.FrameCount++
		c.semantic.AddFrame(digest)
		w.semantic.AddFrame(digest)
	case RecordGap:
		s.GapCount++
		c.semantic.AddGap(digest)
		w.semantic.AddGap(digest)
	}
	if e.timeKnown {
		if s.FirstStartUnixNanos == nil {
			s.FirstStartUnixNanos = proto.Int64(e.start)
		}
		s.LastStartUnixNanos = proto.Int64(e.start)
	}
	w.summary.Records++
	if c.offset >= w.limits.TargetChunkBytes {
		return w.sealChunk()
	}
	return nil
}

const indexTimeKnown uint32 = 1

func (w *Writer) openChunk() error {
	ordinal := w.summary.Chunks
	path := filepath.Join(w.dir, chunksDir, chunkBase(ordinal)+chunkSuffix+openSuffix)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return w.fail(err)
	}
	header, err := marshalOptions.Marshal(&pb.ChunkHeader{Ordinal: ordinal, ManifestSha256: w.manifestDigest[:]})
	if err != nil {
		file.Close()
		return w.fail(err)
	}
	prefix := appendRecord(appendPreamble(nil, objectChunk), envelope{kind: RecordChunkHeader, tag: w.tag, sequence: ordinal}, header)
	c := &openChunk{ordinal: ordinal, file: file, openPath: path, hash: sha256.New(), semantic: l4bobserve.NewStreamDigest()}
	w.chunk = c
	if _, err := file.Write(prefix); err != nil {
		return w.fail(fmt.Errorf("write %s: %w", path, err))
	}
	c.hash.Write(prefix)
	c.offset = uint64(len(prefix))
	return nil
}

// sealChunk appends the seal, makes the chunk and then its index durable
// under their final names, and folds the chunk into the chain.
func (w *Writer) sealChunk() error {
	c := w.chunk
	if c == nil {
		return nil
	}
	s := &c.seal
	s.Ordinal = c.ordinal
	s.BodyBytes = c.offset
	s.BodySha256 = c.hash.Sum(nil)
	semantic := c.semantic.Sum()
	s.SemanticSha256 = semantic[:]
	payload, err := marshalOptions.Marshal(s)
	if err != nil {
		return w.fail(err)
	}
	seal := appendRecord(nil, envelope{kind: RecordChunkSeal, tag: w.tag, sequence: c.ordinal}, payload)
	if _, err := c.file.Write(seal); err != nil {
		return w.fail(fmt.Errorf("write %s: %w", c.openPath, err))
	}
	if err := c.file.Sync(); err != nil {
		return w.fail(fmt.Errorf("sync %s: %w", c.openPath, err))
	}
	if err := c.file.Close(); err != nil {
		return w.fail(err)
	}
	base := filepath.Join(w.dir, chunksDir, chunkBase(c.ordinal))
	if err := os.Rename(c.openPath, base+chunkSuffix); err != nil {
		return w.fail(err)
	}

	ix := &c.index
	ix.Ordinal = c.ordinal
	ix.ChunkBytes = c.offset + uint64(len(seal))
	ix.ChunkBodySha256 = s.BodySha256
	ix.SealOffset = c.offset
	ix.SealLength = uint32(len(seal))
	payload, err = marshalOptions.Marshal(ix)
	if err != nil {
		return w.fail(err)
	}
	if len(payload) > maxIndexBytes {
		return w.fail(fmt.Errorf("chunk %d index of %d bytes exceeds %d", c.ordinal, len(payload), maxIndexBytes))
	}
	object := appendRecord(appendPreamble(nil, objectIndex), envelope{kind: RecordChunkIndex, tag: w.tag, sequence: c.ordinal}, payload)
	if err := writeObject(base+indexSuffix, object); err != nil {
		return w.fail(err)
	}
	if err := syncDir(filepath.Join(w.dir, chunksDir)); err != nil {
		return w.fail(err)
	}
	w.summary.ChunkChain = chainDigest(w.summary.ChunkChain, s.BodySha256)
	w.summary.ChunkBytes += ix.ChunkBytes
	w.summary.Chunks++
	w.chunk = nil
	return nil
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

func (w *Writer) fail(err error) error {
	if w.err == nil {
		w.err = err
	}
	return err
}

// Close seals the open chunk and writes the closing summary. A container
// without a summary is still readable; Close is what lets a reader tell a
// finished capture from an interrupted one, and a missing last chunk from a
// shorter capture.
func (w *Writer) Close() (Summary, error) {
	if w.closed {
		return w.summary, nil
	}
	if w.err != nil {
		w.Abandon()
		return w.summary, fmt.Errorf("observation writer failed earlier: %w", w.err)
	}
	if err := w.sealChunk(); err != nil {
		w.Abandon()
		return w.summary, err
	}
	w.closed = true
	w.summary.EndSequence = w.stream.NextSequence()
	w.summary.Semantic = w.semantic.Sum()
	s := w.summary
	payload, err := marshalOptions.Marshal(&pb.CaptureSummary{
		ChunkCount: s.Chunks, ChunkChainSha256: s.ChunkChain[:], RecordCount: s.Records,
		FrameCount: s.Frames, GapCount: s.Gaps, EndSequence: s.EndSequence,
		RejectedFrames: s.RejectedFrames, SemanticSha256: s.Semantic[:], TotalChunkBytes: s.ChunkBytes,
	})
	if err != nil {
		return s, err
	}
	object := appendRecord(appendPreamble(nil, objectSummary), envelope{kind: RecordSummary, tag: w.tag}, payload)
	if err := writeObject(filepath.Join(w.dir, summaryName), object); err != nil {
		return s, err
	}
	return s, syncDir(w.dir)
}

// Abandon stops writing without a closing summary: the container then reads
// as interrupted. Records already written are sealed if the writer is
// healthy; after a write failure the open chunk is left as an unsealed tail,
// because its last record may be partial.
func (w *Writer) Abandon() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if w.chunk == nil {
		return nil
	}
	if w.err == nil {
		return w.sealChunk()
	}
	return w.chunk.file.Close()
}

// writeObject writes data under name via a temporary name, synchronising
// the file before the rename publishes it. The caller synchronises the
// directory.
func writeObject(name string, data []byte) error {
	tmp := name + openSuffix
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync %s: %w", tmp, err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, name)
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
