package vrlog

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// Options configure Open.
type Options struct {
	// Require names the evidence capabilities the caller depends on. Open
	// returns a *l4bobserve.MissingCapabilitiesError naming every one the
	// container's profile lacks, so an experiment needing foreground-complete
	// evidence cannot run on a reduced source.
	Require []l4bobserve.Capability
	// RebuildIndexes rebuilds a missing or damaged chunk index in memory from
	// its chunk. The rebuild validates every record and never overrides a
	// chunk whose digest fails. Nothing on disk is rewritten.
	RebuildIndexes bool
}

// ChunkInfo describes one sealed chunk.
type ChunkInfo struct {
	Ordinal                    uint64
	Bytes                      uint64
	BodySHA256                 l4bobserve.Digest
	SemanticSHA256             l4bobserve.Digest
	Records, Frames, Gaps      uint64
	FirstSequence, EndSequence uint64
	// HaveTime is false when no record in the chunk has a known time.
	HaveTime                                bool
	FirstStartUnixNanos, LastStartUnixNanos int64
	// IndexRebuilt reports that the index was rebuilt from the chunk.
	IndexRebuilt bool
	// Generation is the commit generation that committed the chunk, and
	// FirstRecord the stream position of its first record.
	Generation  uint64
	FirstRecord uint64

	// startCeiling is the latest known record start in this chunk or any
	// before it: monotonic, so time seeks can binary-search chunks.
	startCeiling     int64
	haveStartCeiling bool
	// indexSHA is the index digest its generation committed.
	indexSHA [sha256.Size]byte
}

// Record is one record read from a container, with its location.
type Record struct {
	Kind   RecordKind
	Frame  l4bobserve.FrameRecord // Kind == RecordFrame
	Gap    l4bobserve.GapRecord   // Kind == RecordGap
	Chunk  uint64
	Offset uint64
	Length uint32
}

// Status summarises what Open found: the committed prefix and what lies
// beyond it.
type Status struct {
	// Closed means a close generation committed a closing summary that
	// matches the committed chunks.
	Closed bool
	// State is derived from the chain's terminal generation, if any.
	State CaptureState
	// Generation is the last committed generation the reader validated.
	Generation uint64
	// PointerFallback is set when the current pointer could not be used
	// (missing, torn or foreign): the committed prefix is then the longest
	// validated chain, and this says why.
	PointerFallback string
	// Unpromoted lists complete, validated generations beyond the pointer:
	// published but never acknowledged. They are not read; Recover promotes
	// them with a recovery record.
	Unpromoted []uint64
	// UncommittedTail names objects no committed generation references
	// (an open batch, a chunk sealed but never committed, a torn temporary
	// object). They are never read as evidence.
	UncommittedTail []string
	// Failure is the failure marker a failed writer left, and Recovery the
	// last recovery record, when present.
	Failure  *FailureMarker
	Recovery *RecoveryMarker
	// TailExtentUnknown: the capture did not close and no failure marker
	// says how much was accepted after the committed end.
	TailExtentUnknown bool
	Chunks            int
	Records           uint64
	Frames            uint64
	Gaps              uint64
	RejectedFrames    uint64
	ShedFrames        uint64
	// EndSequence is one past the last source sequence covered.
	EndSequence uint64
	ChunkBytes  uint64
}

// FailureMarker is a failure generation's record.
type FailureMarker struct {
	Generation          uint64
	Cause               FailureCause
	Detail              string
	AcceptedEndSequence uint64
	AcceptedRecords     uint64
	FailedUnixNanos     int64
}

// RecoveryMarker is a recovery generation's record.
type RecoveryMarker struct {
	Generation         uint64
	PointerValid       bool
	PointerGeneration  uint64
	PointerDetail      string
	Promoted           []uint64
	Quarantined        []string
	QuarantinedCount   uint64
	SessionIncomplete  bool
	TailExtentUnknown  bool
	RecoveredUnixNanos int64
	RecoveredBy        string
}

// Reader reads a container's sealed records in source order. It holds chunk
// descriptors for the whole container but at most one chunk's bytes and a
// few indexes at a time. It is not safe for concurrent use.
type Reader struct {
	dir            string
	manifest       Manifest
	manifestDigest [sha256.Size]byte
	tag            [8]byte
	opts           Options
	chunks         []ChunkInfo
	summary        *Summary
	status         Status
	// chain is the last committed generation catalogued.
	chain chainState

	chunkPos, entryPos int
	loaded             *loadedChunk
	indexes            map[int]*chunkIndex
	rebuilt            map[int]*chunkIndex
}

type loadedChunk struct {
	position int
	data     []byte
	index    *chunkIndex
}

type chunkIndex struct {
	ordinal    uint64
	chunkBytes uint64
	bodySHA    [sha256.Size]byte
	sealOffset uint64
	sealLength uint32
	entries    []indexEntry
}

type indexEntry struct {
	offset uint64
	length uint32
	entry
	epoch uint32
}

// covers returns the sequences the record covers, at least one wide so a
// zero-width gap still locates its damage.
func (e indexEntry) covers() (uint64, uint64) { return e.first, e.first + max(e.count, 1) }

// Open is the offline, read-only entry: it validates the manifest and the
// commit chain up to the generation the current pointer names, with every
// committed chunk's index and seal, and the closing summary if a close
// generation committed one. Damage inside that committed prefix fails Open,
// located to its object and sequences. A torn or missing pointer falls back
// to the longest chain that validates. Generations and objects beyond the
// pointer are reported in Status and never read; Recover acts on them.
//
// Open does not read chunk bodies: each chunk's digest is verified when it
// is first read, before any of its records is returned. Verify reads
// everything. Open takes no lock and writes nothing, so it is safe beside a
// live writer, but a filesystem-only reader of a live capture sees at best a
// published generation whose final directory sync may not be complete: a
// live consumer follows the writer's announced frontier instead (Follow).
func Open(dir string, opts Options) (*Reader, error) {
	r, err := openManifest(dir, opts)
	if err != nil {
		return nil, err
	}
	if err := r.catalogueFromPointer(); err != nil {
		return nil, err
	}
	r.probeBeyond()
	return r, nil
}

// openThrough opens dir with exactly generations 0 to generation committed:
// a follower's entry, which trusts the writer's announcement rather than
// the pointer.
func openThrough(dir string, opts Options, generation uint64) (*Reader, error) {
	r, err := openManifest(dir, opts)
	if err != nil {
		return nil, err
	}
	if err := r.extendTo(generation); err != nil {
		return nil, err
	}
	return r, nil
}

func openManifest(dir string, opts Options) (*Reader, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: %w: not a directory", dir, ErrNotContainer)
	}
	r := &Reader{dir: dir, opts: opts, indexes: map[int]*chunkIndex{}, rebuilt: map[int]*chunkIndex{}}
	if err := r.readManifest(); err != nil {
		return nil, err
	}
	if !r.manifest.hasFeature(featureCommitGenerations) {
		return nil, fmt.Errorf("%s: %w: written before commit generations (container 1.0), so nothing records which "+
			"sealed chunks were committed; re-extract it from its source", dir, ErrPreGenerationLayout)
	}
	if err := r.manifest.Profile.Require(opts.Require...); err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	return r, nil
}

// Manifest returns the container's manifest.
func (r *Reader) Manifest() Manifest { return r.manifest }

// Profile is the evidence profile every record satisfies.
func (r *Reader) Profile() l4bobserve.Profile { return r.manifest.Profile }

// Chunks returns the sealed chunks in ordinal order.
func (r *Reader) Chunks() []ChunkInfo { return append([]ChunkInfo(nil), r.chunks...) }

// Summary returns the closing summary, if the writer closed cleanly.
func (r *Reader) Summary() (Summary, bool) {
	if r.summary == nil {
		return Summary{}, false
	}
	return *r.summary, true
}

// Status summarises the container.
func (r *Reader) Status() Status {
	s := r.status
	s.UncommittedTail = append([]string(nil), s.UncommittedTail...)
	s.Unpromoted = append([]uint64(nil), s.Unpromoted...)
	return s
}

// Close releases the cached chunk.
func (r *Reader) Close() error {
	r.loaded = nil
	r.indexes = nil
	r.rebuilt = nil
	return nil
}

func (r *Reader) readManifest() error {
	path := filepath.Join(r.dir, manifestName)
	data, err := readBounded(path, preambleSize+envelopeSize+maxManifestBytes)
	if errors.Is(err, fs.ErrNotExist) {
		for _, legacy := range []string{"header.json", "index.bin"} {
			if _, statErr := os.Stat(filepath.Join(r.dir, legacy)); statErr == nil {
				return &legacyError{dir: r.dir}
			}
		}
		return fmt.Errorf("%s: %w: no %s object", r.dir, ErrNotContainer, manifestName)
	}
	if err != nil {
		return err
	}
	if err := checkPreamble(data, objectManifest); err != nil {
		if errors.Is(err, ErrNotContainer) || errors.Is(err, ErrUnsupported) {
			return fmt.Errorf("%s: %w", r.dir, err)
		}
		return r.corruptObject(CorruptStructure, manifestName, 0, err)
	}
	payload, err := singleRecord(data, RecordManifest, [8]byte{}, maxManifestBytes)
	if err != nil {
		return r.objectError(manifestName, err)
	}
	var m pb.RecordingManifest
	if err := unmarshalOptions.Unmarshal(payload, &m); err != nil {
		return r.corruptObject(CorruptStructure, manifestName, preambleSize+envelopeSize, err)
	}
	manifest, err := manifestFromProto(&m)
	if err == nil {
		err = manifest.validate()
	}
	if err != nil {
		if errors.Is(err, ErrUnsupported) {
			return fmt.Errorf("%s: %w", r.dir, err)
		}
		return fmt.Errorf("%s: invalid manifest: %w", r.dir, err)
	}
	r.manifest = manifest
	r.manifestDigest = sha256.Sum256(data)
	copy(r.tag[:], r.manifestDigest[:8])
	return nil
}

// objectFault carries an error found at a byte offset of a single-record
// object, so the caller can name the object.
type objectFault struct {
	kind   CorruptionKind
	offset int64
	err    error
}

func (f *objectFault) Error() string { return f.err.Error() }
func (f *objectFault) Unwrap() error { return f.err }

func (r *Reader) objectError(object string, err error) error {
	var fault *objectFault
	if errors.As(err, &fault) {
		return r.corruptObject(fault.kind, object, fault.offset, fault.err)
	}
	if errors.Is(err, ErrUnsupported) {
		return fmt.Errorf("%s/%s: %w", r.dir, object, err)
	}
	return r.corruptObject(CorruptStructure, object, 0, err)
}

func (r *Reader) corruptObject(kind CorruptionKind, object string, offset int64, err error) error {
	return &CorruptionError{Kind: kind, Object: object, Chunk: -1, Offset: offset, Length: -1, Detail: err.Error()}
}

// singleRecord returns the payload of an object that holds exactly one
// record of kind after its preamble, checked in order: envelope, kind, tag,
// length bound, exact size and CRC.
func singleRecord(data []byte, kind RecordKind, tag [8]byte, maxPayload int) ([]byte, error) {
	body := data[preambleSize:]
	e, err := parseEnvelope(body)
	if err != nil {
		return nil, &objectFault{CorruptTruncated, preambleSize, err}
	}
	switch {
	case e.kind != kind:
		return nil, &objectFault{CorruptStructure, preambleSize, fmt.Errorf("holds a %s record, not a %s", e.kind, kind)}
	case e.tag != tag:
		return nil, &objectFault{CorruptDisagreement, preambleSize, fmt.Errorf("record belongs to another container (tag %x)", e.tag)}
	case int64(e.stored) > int64(maxPayload):
		return nil, &objectFault{CorruptLength, preambleSize, fmt.Errorf("payload length %d exceeds %d", e.stored, maxPayload)}
	case envelopeSize+int(e.stored) != len(body):
		kind := CorruptLength
		if envelopeSize+int(e.stored) > len(body) {
			kind = CorruptTruncated
		}
		return nil, &objectFault{kind, preambleSize, fmt.Errorf("object holds %d bytes after its preamble, the record declares %d", len(body), envelopeSize+int(e.stored))}
	}
	payload := body[envelopeSize:]
	if err := e.checkBody(body, payload); err != nil {
		if errors.Is(err, ErrUnsupported) {
			return nil, err
		}
		return nil, &objectFault{CorruptChecksum, preambleSize, err}
	}
	return payload, nil
}

// readBounded reads a whole file after checking its size against limit, so
// a hostile or damaged object cannot make the reader allocate beyond it.
func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() > limit {
		return nil, &CorruptionError{Kind: CorruptLength, Object: filepath.Base(path), Chunk: -1, Offset: limit, Length: -1,
			Detail: fmt.Sprintf("object of %d bytes exceeds the %d-byte bound", info.Size(), limit)}
	}
	data := make([]byte, info.Size())
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, err
	}
	return data, nil
}

// joinChunk checks a chunk continues the stream after prev (nil for the
// first chunk): its first sequence is the previous chunk's end, and no
// record starts before one already seen.
func joinChunk(prev *ChunkInfo, info *ChunkInfo) error {
	var wantFirst uint64
	var ceiling int64
	haveCeiling := false
	if prev != nil {
		wantFirst, ceiling, haveCeiling = prev.EndSequence, prev.startCeiling, prev.haveStartCeiling
	}
	if info.FirstSequence != wantFirst {
		lo, hi := min(wantFirst, info.FirstSequence), max(wantFirst, info.FirstSequence)
		return &CorruptionError{Kind: CorruptSequence, Object: chunkObject(info.Ordinal), Chunk: int64(info.Ordinal), Length: -1,
			HaveSequences: true, FirstSequence: lo, EndSequence: hi,
			Detail: fmt.Sprintf("chunk starts at sequence %d where %d was expected: a chunk is missing, reordered or from another capture", info.FirstSequence, wantFirst)}
	}
	if info.HaveTime && haveCeiling && info.FirstStartUnixNanos < ceiling {
		return &CorruptionError{Kind: CorruptSequence, Object: chunkObject(info.Ordinal), Chunk: int64(info.Ordinal), Length: -1,
			HaveSequences: true, FirstSequence: info.FirstSequence, EndSequence: info.EndSequence,
			Detail: "chunk starts before the preceding chunk's last record"}
	}
	info.startCeiling, info.haveStartCeiling = ceiling, haveCeiling
	if info.HaveTime {
		info.startCeiling, info.haveStartCeiling = info.LastStartUnixNanos, true
	}
	return nil
}

func parseOrdinal(base string) (uint64, bool) {
	if len(base) < 8 {
		return 0, false
	}
	v, err := strconv.ParseUint(base, 10, 64)
	return v, err == nil && chunkBase(v) == base
}

func chunkObject(ordinal uint64) string { return chunksDir + "/" + chunkBase(ordinal) + chunkSuffix }
func indexObject(ordinal uint64) string { return chunksDir + "/" + chunkBase(ordinal) + indexSuffix }

// openIndex returns chunk position's validated index: the stored one, which
// must be the object its generation committed, or, when that is missing or
// damaged and rebuilding is allowed, one rebuilt from the chunk's bytes.
func (r *Reader) openIndex(position int, committed [sha256.Size]byte) (*chunkIndex, bool, error) {
	ordinal := uint64(position)
	ix, digest, err := r.readIndex(position)
	if err == nil && digest != committed {
		err = &CorruptionError{Kind: CorruptDisagreement, Object: indexObject(ordinal), Chunk: int64(ordinal), Length: -1,
			Detail: "the index is not the object its generation committed"}
	}
	if err == nil {
		return ix, false, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		err = &CorruptionError{Kind: CorruptMissing, Object: indexObject(ordinal), Chunk: int64(ordinal), Length: -1,
			Detail: "the committed chunk has no index; open with index rebuilding to derive it from the chunk"}
	}
	if !r.opts.RebuildIndexes || errors.Is(err, ErrUnsupported) {
		return nil, false, err
	}
	data, loadErr := r.readChunkBytes(ordinal, 0)
	if loadErr != nil {
		return nil, false, loadErr
	}
	ix, rebuildErr := r.rebuildIndex(ordinal, data)
	if rebuildErr != nil {
		return nil, false, rebuildErr
	}
	return ix, true, nil
}

// readIndex reads and checks a stored index, returning it with the SHA-256
// of its object.
func (r *Reader) readIndex(position int) (*chunkIndex, [sha256.Size]byte, error) {
	ordinal := uint64(position)
	object := indexObject(ordinal)
	var none [sha256.Size]byte
	data, err := readBounded(filepath.Join(r.dir, object), preambleSize+envelopeSize+maxIndexBytes)
	if err != nil {
		var corrupt *CorruptionError
		if errors.As(err, &corrupt) {
			corrupt.Object, corrupt.Chunk = object, int64(ordinal)
			return nil, none, corrupt
		}
		return nil, none, err
	}
	if err := checkPreamble(data, objectIndex); err != nil {
		if errors.Is(err, ErrUnsupported) {
			return nil, none, err
		}
		return nil, none, &CorruptionError{Kind: CorruptStructure, Object: object, Chunk: int64(ordinal), Length: preambleSize, Detail: err.Error()}
	}
	payload, err := singleRecord(data, RecordChunkIndex, r.tag, maxIndexBytes)
	if err != nil {
		e := r.objectError(object, err)
		var corrupt *CorruptionError
		if errors.As(e, &corrupt) {
			corrupt.Chunk = int64(ordinal)
		}
		return nil, none, e
	}
	var m pb.ChunkIndex
	if err := unmarshalOptions.Unmarshal(payload, &m); err != nil {
		return nil, none, &CorruptionError{Kind: CorruptStructure, Object: object, Chunk: int64(ordinal), Offset: preambleSize + envelopeSize, Length: -1, Detail: err.Error()}
	}
	ix, err := r.indexFromProto(&m)
	if err == nil {
		err = r.checkIndex(ix, ordinal)
	}
	if errors.Is(err, ErrUnsupported) {
		// A newer writer's record kind is not damage; say which is needed.
		return nil, none, fmt.Errorf("%s: %w", object, err)
	}
	if err != nil {
		return nil, none, &CorruptionError{Kind: CorruptDisagreement, Object: object, Chunk: int64(ordinal), Length: -1, Detail: err.Error()}
	}
	return ix, sha256.Sum256(data), nil
}

func (r *Reader) indexFromProto(m *pb.ChunkIndex) (*chunkIndex, error) {
	n := len(m.Offset)
	for name, length := range map[string]int{"length": len(m.Length), "kind": len(m.Kind), "sequence": len(m.SequenceFirst),
		"sequence count": len(m.SequenceCount), "start": len(m.TimeStart), "end": len(m.TimeEnd), "flags": len(m.Flags), "epoch": len(m.Epoch)} {
		if length != n {
			return nil, fmt.Errorf("index column %s has %d entries, offsets have %d", name, length, n)
		}
	}
	if n > int(r.manifest.Limits.MaxRecordsPerChunk) {
		return nil, fmt.Errorf("index lists %d records, over the limit of %d", n, r.manifest.Limits.MaxRecordsPerChunk)
	}
	if len(m.ChunkBodySha256) != sha256.Size {
		return nil, fmt.Errorf("index chunk digest has %d bytes", len(m.ChunkBodySha256))
	}
	ix := &chunkIndex{ordinal: m.Ordinal, chunkBytes: m.ChunkBytes, sealOffset: m.SealOffset, sealLength: m.SealLength,
		bodySHA: [sha256.Size]byte(m.ChunkBodySha256), entries: make([]indexEntry, n)}
	for i := range n {
		if m.Kind[i] > 0xffff {
			return nil, fmt.Errorf("index entry %d has kind %d", i, m.Kind[i])
		}
		ix.entries[i] = indexEntry{offset: m.Offset[i], length: m.Length[i], epoch: m.Epoch[i], entry: entry{
			kind: RecordKind(m.Kind[i]), first: m.SequenceFirst[i], count: m.SequenceCount[i],
			timeKnown: m.Flags[i]&indexTimeKnown != 0, start: m.TimeStart[i], end: m.TimeEnd[i]}}
		if m.Flags[i]&^indexTimeKnown != 0 {
			return nil, fmt.Errorf("index entry %d has unknown flags %#x", i, m.Flags[i])
		}
	}
	return ix, nil
}

// checkIndex verifies an index's own structure before it is trusted to
// locate anything: it names this chunk, its entries tile the chunk from the
// header to the seal without overlap, record lengths respect the limits, and
// within the chunk sequences run on and starts never go back.
func (r *Reader) checkIndex(ix *chunkIndex, ordinal uint64) error {
	limits := r.manifest.Limits
	switch {
	case ix.ordinal != ordinal:
		return fmt.Errorf("index names chunk %d", ix.ordinal)
	case ix.chunkBytes > limits.MaxChunkBytes:
		return fmt.Errorf("chunk of %d bytes exceeds the %d-byte bound", ix.chunkBytes, limits.MaxChunkBytes)
	case ix.sealLength < envelopeSize || ix.sealLength > envelopeSize+maxSealBytes:
		return fmt.Errorf("seal length %d is out of range", ix.sealLength)
	case ix.sealOffset+uint64(ix.sealLength) != ix.chunkBytes:
		return fmt.Errorf("seal [%d, +%d) does not end the %d-byte chunk", ix.sealOffset, ix.sealLength, ix.chunkBytes)
	case len(ix.entries) == 0:
		return fmt.Errorf("sealed chunk holds no records")
	}
	next := ix.entries[0].offset
	if next < preambleSize+envelopeSize {
		return fmt.Errorf("first record at byte %d overlaps the chunk header", next)
	}
	var lastStart int64
	haveStart := false
	sequence := ix.entries[0].first
	for i, e := range ix.entries {
		switch {
		case e.offset != next:
			return fmt.Errorf("entry %d at byte %d, where the previous record ended at %d", i, e.offset, next)
		case e.length < envelopeSize || e.length-envelopeSize > limits.MaxRecordBytes:
			return fmt.Errorf("entry %d has length %d, outside [%d, %d]", i, e.length, envelopeSize, uint64(limits.MaxRecordBytes)+envelopeSize)
		case e.epoch != 0:
			return fmt.Errorf("entry %d is in epoch %d; this container declares only epoch 0", i, e.epoch)
		case e.first != sequence:
			return fmt.Errorf("entry %d starts at sequence %d where %d was expected", i, e.first, sequence)
		}
		switch e.kind {
		case RecordFrame:
			if e.count != 1 || !e.timeKnown {
				return fmt.Errorf("frame entry %d covers %d sequences (time known: %v)", i, e.count, e.timeKnown)
			}
		case RecordGap:
		default:
			if !e.kind.Ancillary() {
				return &UnsupportedError{What: fmt.Sprintf("critical record kind %#04x in chunk %d", uint16(e.kind), ordinal)}
			}
			if e.count != 0 {
				return fmt.Errorf("ancillary entry %d covers sequences", i)
			}
		}
		if e.timeKnown {
			if haveStart && e.start < lastStart {
				return fmt.Errorf("entry %d starts %d ns before its predecessor", i, lastStart-e.start)
			}
			lastStart, haveStart = e.start, true
		}
		next = e.offset + uint64(e.length)
		sequence = e.first + e.count
	}
	if next != ix.sealOffset {
		return fmt.Errorf("records end at byte %d, the seal is at %d", next, ix.sealOffset)
	}
	return nil
}

// readSeal reads just the seal record from the chunk file and requires it
// to agree with the index: the cheap check that the two describe the same
// bytes, made without reading the chunk body.
func (r *Reader) readSeal(position int, ix *chunkIndex) (*pb.ChunkSeal, error) {
	ordinal := uint64(position)
	object := chunkObject(ordinal)
	f, err := os.Open(filepath.Join(r.dir, object))
	if err != nil {
		return nil, &CorruptionError{Kind: CorruptMissing, Object: object, Chunk: int64(ordinal), Length: -1, Detail: err.Error()}
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	seqFirst, seqEnd := ix.entries[0].first, ix.entries[len(ix.entries)-1].first+ix.entries[len(ix.entries)-1].count
	fault := func(kind CorruptionKind, offset, length int64, format string, args ...any) error {
		return &CorruptionError{Kind: kind, Object: object, Chunk: int64(ordinal), Offset: offset, Length: length,
			HaveSequences: true, FirstSequence: seqFirst, EndSequence: max(seqEnd, seqFirst+1), Detail: fmt.Sprintf(format, args...)}
	}
	if uint64(info.Size()) != ix.chunkBytes {
		kind := CorruptDisagreement
		if uint64(info.Size()) < ix.chunkBytes {
			kind = CorruptTruncated
		}
		return nil, fault(kind, min(info.Size(), int64(ix.chunkBytes)), -1, "chunk is %d bytes, its index says %d", info.Size(), ix.chunkBytes)
	}
	buf := make([]byte, ix.sealLength)
	if _, err := f.ReadAt(buf, int64(ix.sealOffset)); err != nil {
		return nil, fault(CorruptTruncated, int64(ix.sealOffset), int64(ix.sealLength), "read seal: %v", err)
	}
	e, err := parseEnvelope(buf)
	if err != nil {
		return nil, fault(CorruptDisagreement, int64(ix.sealOffset), int64(ix.sealLength), "no record at the indexed seal offset: %v", err)
	}
	if e.kind != RecordChunkSeal || e.tag != r.tag || envelopeSize+int(e.stored) != len(buf) {
		return nil, fault(CorruptDisagreement, int64(ix.sealOffset), int64(ix.sealLength), "the indexed seal offset holds a %d-byte %s record", e.stored, e.kind)
	}
	if err := e.checkBody(buf, buf[envelopeSize:]); err != nil {
		return nil, fault(CorruptChecksum, int64(ix.sealOffset), int64(ix.sealLength), "seal: %v", err)
	}
	var seal pb.ChunkSeal
	if err := unmarshalOptions.Unmarshal(buf[envelopeSize:], &seal); err != nil {
		return nil, fault(CorruptStructure, int64(ix.sealOffset), int64(ix.sealLength), "seal: %v", err)
	}
	var frames, gaps uint64
	var firstStart, lastStart *int64
	for i := range ix.entries {
		e := &ix.entries[i]
		switch e.kind {
		case RecordFrame:
			frames++
		case RecordGap:
			gaps++
		}
		if e.timeKnown {
			if firstStart == nil {
				firstStart = &e.start
			}
			lastStart = &e.start
		}
	}
	switch {
	case seal.Ordinal != ordinal:
		return nil, fault(CorruptDisagreement, int64(ix.sealOffset), int64(ix.sealLength), "chunk file %s holds sealed chunk %d", chunkBase(ordinal), seal.Ordinal)
	case seal.BodyBytes != ix.sealOffset || !bytes.Equal(seal.BodySha256, ix.bodySHA[:]):
		return nil, fault(CorruptDisagreement, 0, -1, "seal and index disagree on the chunk body's size or digest")
	case seal.RecordCount != uint64(len(ix.entries)) || seal.FrameCount != frames || seal.GapCount != gaps:
		return nil, fault(CorruptDisagreement, 0, -1, "seal counts %d records (%d frames, %d gaps), the index %d (%d, %d)",
			seal.RecordCount, seal.FrameCount, seal.GapCount, len(ix.entries), frames, gaps)
	case seal.FirstSequence != seqFirst || seal.EndSequence != seqEnd:
		return nil, fault(CorruptDisagreement, 0, -1, "seal covers sequences [%d, %d), the index [%d, %d)", seal.FirstSequence, seal.EndSequence, seqFirst, seqEnd)
	case !sameOptionalTime(seal.FirstStartUnixNanos, firstStart) || !sameOptionalTime(seal.LastStartUnixNanos, lastStart):
		return nil, fault(CorruptDisagreement, 0, -1, "seal and index disagree on the chunk's time bounds")
	case len(seal.SemanticSha256) != sha256.Size:
		return nil, fault(CorruptStructure, int64(ix.sealOffset), int64(ix.sealLength), "seal has no semantic digest")
	}
	return &seal, nil
}

func sameOptionalTime(a, b *int64) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

// readChunkBytes reads a whole chunk within the chunk bound. When
// wantBytes is non-zero the file must be exactly that long.
func (r *Reader) readChunkBytes(ordinal, wantBytes uint64) ([]byte, error) {
	object := chunkObject(ordinal)
	data, err := readBounded(filepath.Join(r.dir, object), int64(r.manifest.Limits.MaxChunkBytes))
	if err != nil {
		var corrupt *CorruptionError
		if errors.As(err, &corrupt) {
			corrupt.Object, corrupt.Chunk = object, int64(ordinal)
			return nil, corrupt
		}
		return nil, &CorruptionError{Kind: CorruptMissing, Object: object, Chunk: int64(ordinal), Length: -1, Detail: err.Error()}
	}
	if wantBytes != 0 && uint64(len(data)) != wantBytes {
		return nil, &CorruptionError{Kind: CorruptTruncated, Object: object, Chunk: int64(ordinal), Offset: int64(min(uint64(len(data)), wantBytes)), Length: -1,
			Detail: fmt.Sprintf("chunk is %d bytes, its index says %d", len(data), wantBytes)}
	}
	if err := checkPreamble(data, objectChunk); err != nil {
		if errors.Is(err, ErrUnsupported) {
			return nil, err
		}
		return nil, &CorruptionError{Kind: CorruptStructure, Object: object, Chunk: int64(ordinal), Length: preambleSize, Detail: err.Error()}
	}
	return data, nil
}

// loadChunk makes position the cached chunk, verifying its body digest and
// header before any record in it can be returned.
func (r *Reader) loadChunk(position int) (*loadedChunk, error) {
	if r.loaded != nil && r.loaded.position == position {
		return r.loaded, nil
	}
	r.loaded = nil
	info := r.chunks[position]
	ix, err := r.index(position)
	if err != nil {
		return nil, err
	}
	data, err := r.readChunkBytes(info.Ordinal, ix.chunkBytes)
	if err != nil {
		return nil, err
	}
	if err := r.checkChunkBody(info, ix, data); err != nil {
		return nil, err
	}
	r.loaded = &loadedChunk{position: position, data: data, index: ix}
	return r.loaded, nil
}

// checkChunkBody verifies the body digest, then the chunk header: it names
// this chunk and this manifest, and ends where the first record begins.
func (r *Reader) checkChunkBody(info ChunkInfo, ix *chunkIndex, data []byte) error {
	object := chunkObject(info.Ordinal)
	fault := func(kind CorruptionKind, offset, length int64, format string, args ...any) error {
		return &CorruptionError{Kind: kind, Object: object, Chunk: int64(info.Ordinal), Offset: offset, Length: length,
			HaveSequences: true, FirstSequence: info.FirstSequence, EndSequence: max(info.EndSequence, info.FirstSequence+1),
			Detail: fmt.Sprintf(format, args...)}
	}
	if sum := sha256.Sum256(data[:ix.sealOffset]); sum != ix.bodySHA {
		return fault(CorruptDigest, 0, int64(ix.sealOffset), "SHA-256 of the chunk body does not match its seal: every record in the chunk is untrusted")
	}
	header := data[preambleSize:]
	e, err := parseEnvelope(header)
	if err != nil || e.kind != RecordChunkHeader || e.tag != r.tag || e.stored > maxChunkHeaderBytes {
		return fault(CorruptStructure, preambleSize, envelopeSize, "chunk header record is missing or foreign")
	}
	end := uint64(preambleSize + envelopeSize + int(e.stored))
	if end != ix.entries[0].offset {
		return fault(CorruptDisagreement, preambleSize, int64(end)-preambleSize, "chunk header ends at byte %d, the first indexed record is at %d", end, ix.entries[0].offset)
	}
	payload := header[envelopeSize : envelopeSize+int(e.stored)]
	if err := e.checkBody(header, payload); err != nil {
		return fault(CorruptChecksum, preambleSize, int64(end)-preambleSize, "chunk header: %v", err)
	}
	var h pb.ChunkHeader
	if err := unmarshalOptions.Unmarshal(payload, &h); err != nil {
		return fault(CorruptStructure, preambleSize, int64(end)-preambleSize, "chunk header: %v", err)
	}
	if h.Ordinal != info.Ordinal || !bytes.Equal(h.ManifestSha256, r.manifestDigest[:]) {
		return fault(CorruptDisagreement, preambleSize, int64(end)-preambleSize, "chunk header names chunk %d of another manifest or position", h.Ordinal)
	}
	return nil
}

// index returns a chunk's validated index, keeping a handful in memory. A
// re-read index must still be the object its generation committed.
func (r *Reader) index(position int) (*chunkIndex, error) {
	if ix := r.rebuilt[position]; ix != nil {
		return ix, nil
	}
	if ix := r.indexes[position]; ix != nil {
		return ix, nil
	}
	ix, digest, err := r.readIndex(position)
	if err != nil {
		return nil, err
	}
	if digest != r.chunks[position].indexSHA {
		return nil, &CorruptionError{Kind: CorruptDisagreement, Object: indexObject(uint64(position)), Chunk: int64(position), Length: -1,
			Detail: "the index changed after Open: it is not the object its generation committed"}
	}
	if len(r.indexes) >= 4 {
		clear(r.indexes)
	}
	r.indexes[position] = ix
	return ix, nil
}

// Next returns the next record in stream order, or io.EOF after the last.
// An error does not advance the cursor: a damaged record blocks sequential
// reading until the caller explicitly seeks past it. Ancillary records of
// kinds this reader does not know are skipped.
func (r *Reader) Next() (Record, error) {
	for r.chunkPos < len(r.chunks) {
		c, err := r.loadChunk(r.chunkPos)
		if err != nil {
			return Record{}, err
		}
		if r.entryPos >= len(c.index.entries) {
			r.chunkPos, r.entryPos = r.chunkPos+1, 0
			continue
		}
		rec, known, err := r.readEntry(c, r.entryPos)
		if err != nil {
			return Record{}, err
		}
		r.entryPos++
		if known {
			return rec, nil
		}
	}
	return Record{}, io.EOF
}

// readEntry decodes one indexed record and checks it against its envelope,
// its index entry and the manifest's profile. known is false for an
// ancillary record of a kind this reader does not know.
func (r *Reader) readEntry(c *loadedChunk, i int) (Record, bool, error) {
	info := r.chunks[c.position]
	e := c.index.entries[i]
	object := chunkObject(info.Ordinal)
	first, end := e.covers()
	fault := func(kind CorruptionKind, format string, args ...any) error {
		return &CorruptionError{Kind: kind, Object: object, Chunk: int64(info.Ordinal), Offset: int64(e.offset), Length: int64(e.length),
			HaveSequences: true, FirstSequence: first, EndSequence: end, Detail: fmt.Sprintf(format, args...)}
	}
	record := c.data[e.offset : e.offset+uint64(e.length)]
	env, err := parseEnvelope(record)
	if err != nil {
		return Record{}, false, fault(CorruptStructure, "%v", err)
	}
	switch {
	case uint64(env.stored) > uint64(r.manifest.Limits.MaxRecordBytes):
		return Record{}, false, fault(CorruptLength, "payload length %d exceeds the %d-byte limit", env.stored, r.manifest.Limits.MaxRecordBytes)
	case envelopeSize+uint64(env.stored) != uint64(e.length):
		return Record{}, false, fault(CorruptLength, "envelope declares a %d-byte payload in a %d-byte indexed record", env.stored, e.length)
	case env.kind != e.kind || env.tag != r.tag || env.sequence != e.first || env.time != e.envelopeTime():
		return Record{}, false, fault(CorruptDisagreement, "envelope (%s, sequence %d, time %d) disagrees with its index entry (%s, %d, %d)",
			env.kind, env.sequence, env.time, e.kind, e.first, e.envelopeTime())
	}
	payload := record[envelopeSize:]
	if err := env.checkBody(record, payload); err != nil {
		if errors.Is(err, ErrUnsupported) {
			return Record{}, false, err
		}
		return Record{}, false, fault(CorruptChecksum, "%v", err)
	}
	rec := Record{Kind: e.kind, Chunk: info.Ordinal, Offset: e.offset, Length: e.length}
	switch e.kind {
	case RecordFrame:
		f, err := decodeFrame(payload, r.manifest.Limits)
		if err == nil {
			err = f.ValidateFor(r.manifest.Profile)
		}
		if err != nil {
			return Record{}, false, fault(CorruptRecord, "%v", err)
		}
		if f.Sequence != e.first || f.CaptureStartUnixNanos != e.start || f.CaptureEndUnixNanos != e.end {
			return Record{}, false, fault(CorruptDisagreement, "frame %d [%d, %d] disagrees with its index entry", f.Sequence, f.CaptureStartUnixNanos, f.CaptureEndUnixNanos)
		}
		rec.Frame = f
	case RecordGap:
		g, err := decodeGap(payload)
		if err == nil {
			err = g.Validate()
		}
		if err != nil {
			return Record{}, false, fault(CorruptRecord, "%v", err)
		}
		if gapEntry(g, e.first) != e.entry {
			return Record{}, false, fault(CorruptDisagreement, "gap %+v disagrees with its index entry", g)
		}
		rec.Gap = g
	default:
		return rec, false, nil
	}
	return rec, true, nil
}

// Reset returns the cursor to the first record.
func (r *Reader) Reset() { r.chunkPos, r.entryPos = 0, 0 }

// SeekSequence positions the cursor on the record covering sequence: the
// frame with that sequence, or the gap over assigned sequences containing it.
func (r *Reader) SeekSequence(sequence uint64) error {
	position := sort.Search(len(r.chunks), func(i int) bool { return r.chunks[i].EndSequence > sequence })
	if position == len(r.chunks) {
		return fmt.Errorf("%w: %d is at or beyond the stream's end sequence %d", ErrSequenceNotFound, sequence, r.status.EndSequence)
	}
	ix, err := r.index(position)
	if err != nil {
		return err
	}
	for i, e := range ix.entries {
		if e.count > 0 && e.first <= sequence && sequence < e.first+e.count {
			r.chunkPos, r.entryPos = position, i
			return nil
		}
	}
	return fmt.Errorf("%w: chunk %d covers [%d, %d) but no record holds %d", ErrSequenceNotFound, position,
		r.chunks[position].FirstSequence, r.chunks[position].EndSequence, sequence)
}

// SeekTime positions the cursor for capture time unixNanos in epoch. It
// lands on the first record with a known time that starts at or after the
// target, or on its time-known predecessor when that one's interval contains
// the target, so a seek into a long gap lands on the gap. Gaps of unknown
// time immediately before the landing record are included: the loss may lie
// at the target. With nothing at or after the target the cursor is at the
// end. Container 1.0 has one epoch, 0; clock resets will add epochs, and a
// time alone is ambiguous across them.
func (r *Reader) SeekTime(epoch uint32, unixNanos int64) error {
	if epoch != 0 {
		return fmt.Errorf("epoch %d not in this container, which declares only epoch 0", epoch)
	}
	position := sort.Search(len(r.chunks), func(i int) bool {
		return r.chunks[i].haveStartCeiling && r.chunks[i].startCeiling >= unixNanos
	})
	if position == len(r.chunks) {
		r.chunkPos, r.entryPos = len(r.chunks), 0
		return nil
	}
	ix, err := r.index(position)
	if err != nil {
		return err
	}
	entry := sort.Search(len(ix.entries), func(i int) bool {
		// Carry the last known start forward over unknown-time gaps, which
		// keeps the predicate monotonic.
		for j := i; j < len(ix.entries); j++ {
			if ix.entries[j].timeKnown {
				return ix.entries[j].start >= unixNanos
			}
		}
		return true
	})
	pos := cursor{position, entry}
	if prev, ok, err := r.previousTimed(pos); err != nil {
		return err
	} else if ok {
		e, err := r.entryAt(prev)
		if err != nil {
			return err
		}
		if e.end >= unixNanos {
			pos = prev
		}
	}
	for {
		prev, ok := r.previous(pos)
		if !ok {
			break
		}
		e, err := r.entryAt(prev)
		if err != nil {
			return err
		}
		if e.timeKnown || e.kind != RecordGap {
			break
		}
		pos = prev
	}
	r.chunkPos, r.entryPos = pos.chunk, pos.entry
	return nil
}

type cursor struct{ chunk, entry int }

func (r *Reader) entryAt(c cursor) (indexEntry, error) {
	ix, err := r.index(c.chunk)
	if err != nil {
		return indexEntry{}, err
	}
	return ix.entries[c.entry], nil
}

// previous steps back one record, across chunk boundaries.
func (r *Reader) previous(c cursor) (cursor, bool) {
	if c.entry > 0 {
		return cursor{c.chunk, c.entry - 1}, true
	}
	if c.chunk == 0 {
		return cursor{}, false
	}
	return cursor{c.chunk - 1, int(r.chunks[c.chunk-1].Records) - 1}, true
}

// previousTimed steps back to the nearest record with a known time.
func (r *Reader) previousTimed(c cursor) (cursor, bool, error) {
	for {
		prev, ok := r.previous(c)
		if !ok {
			return cursor{}, false, nil
		}
		e, err := r.entryAt(prev)
		if err != nil {
			return cursor{}, false, err
		}
		if e.timeKnown {
			return prev, true, nil
		}
		c = prev
	}
}

// readSummary reads the closing summary a close generation committed, and
// requires it to be that object and to match the committed chunks exactly.
func (r *Reader) readSummary(committed *pb.CommittedObject) error {
	data, err := readBounded(filepath.Join(r.dir, summaryName), preambleSize+envelopeSize+maxSummaryBytes)
	if err != nil {
		return locateObject(err, summaryName)
	}
	if sum := sha256.Sum256(data); uint64(len(data)) != committed.GetBytes() || !bytes.Equal(sum[:], committed.GetSha256()) {
		return &CorruptionError{Kind: CorruptSummary, Object: summaryName, Chunk: -1, Length: -1,
			Detail: "the summary is not the object its close generation committed"}
	}
	if err := checkPreamble(data, objectSummary); err != nil {
		if errors.Is(err, ErrUnsupported) {
			return err
		}
		return r.corruptObject(CorruptStructure, summaryName, 0, err)
	}
	payload, err := singleRecord(data, RecordSummary, r.tag, maxSummaryBytes)
	if err != nil {
		return r.objectError(summaryName, err)
	}
	var m pb.CaptureSummary
	if err := unmarshalOptions.Unmarshal(payload, &m); err != nil {
		return r.corruptObject(CorruptStructure, summaryName, preambleSize+envelopeSize, err)
	}
	if len(m.ChunkChainSha256) != sha256.Size || len(m.SemanticSha256) != sha256.Size {
		return r.corruptObject(CorruptStructure, summaryName, preambleSize+envelopeSize, errors.New("summary digests are malformed"))
	}
	s := Summary{Chunks: m.ChunkCount, ChunkChain: l4bobserve.Digest(m.ChunkChainSha256), Records: m.RecordCount,
		Frames: m.FrameCount, Gaps: m.GapCount, EndSequence: m.EndSequence, RejectedFrames: m.RejectedFrames,
		ShedFrames: m.ShedFrames, ChunkBytes: m.TotalChunkBytes, Semantic: l4bobserve.Digest(m.SemanticSha256),
		Commits: m.CommitCount, CommitLatency: Latency{Count: m.CommitCount, P50: time.Duration(m.CommitP50Nanos),
			P95: time.Duration(m.CommitP95Nanos), P99: time.Duration(m.CommitP99Nanos), Max: time.Duration(m.CommitMaxNanos)}}
	var chain l4bobserve.Digest
	for _, c := range r.chunks {
		chain = chainDigest(chain, c.BodySHA256[:])
	}
	st := r.status
	mismatch := ""
	switch {
	case s.Chunks != uint64(len(r.chunks)):
		mismatch = fmt.Sprintf("the summary lists %d chunks, %d are sealed", s.Chunks, len(r.chunks))
	case s.ChunkChain != chain:
		mismatch = "the sealed chunks' digests do not chain to the summary's"
	case s.Records != st.Records || s.Frames != st.Frames || s.Gaps != st.Gaps || s.EndSequence != st.EndSequence || s.ChunkBytes != st.ChunkBytes:
		mismatch = fmt.Sprintf("the summary counts %d records to sequence %d, the chunks %d to %d", s.Records, s.EndSequence, st.Records, st.EndSequence)
	case s.RejectedFrames != st.RejectedFrames || s.ShedFrames != st.ShedFrames:
		mismatch = "the summary and the chain disagree on refused and shed frames"
	}
	if mismatch != "" {
		return &CorruptionError{Kind: CorruptSummary, Object: summaryName, Chunk: -1, Length: -1,
			HaveSequences: s.EndSequence != st.EndSequence, FirstSequence: min(s.EndSequence, st.EndSequence), EndSequence: max(s.EndSequence, st.EndSequence),
			Detail: mismatch + ": a chunk is missing, reordered or foreign"}
	}
	r.summary = &s
	return nil
}

// rebuildIndex derives a chunk's index from its bytes. It first finds the
// seal by walking envelope lengths alone and checks the chunk digest, so no
// record of a damaged chunk is decoded or trusted: a rebuild can never make
// damaged evidence readable. It then validates every record it indexes.
func (r *Reader) rebuildIndex(ordinal uint64, data []byte) (*chunkIndex, error) {
	object := chunkObject(ordinal)
	fault := func(kind CorruptionKind, offset, length int64, format string, args ...any) error {
		return &CorruptionError{Kind: kind, Object: object, Chunk: int64(ordinal), Offset: offset, Length: length, Detail: "index rebuild: " + fmt.Sprintf(format, args...)}
	}
	limits := r.manifest.Limits
	// walk bounds one record at offset and returns its envelope and size.
	walk := func(offset uint64) (envelope, uint64, error) {
		env, err := parseEnvelope(data[offset:])
		if err != nil {
			return envelope{}, 0, fault(CorruptTruncated, int64(offset), -1, "%v", err)
		}
		limit := uint64(limits.MaxRecordBytes)
		switch env.kind {
		case RecordChunkHeader:
			limit = maxChunkHeaderBytes
		case RecordChunkSeal:
			limit = maxSealBytes
		}
		if uint64(env.stored) > limit {
			return envelope{}, 0, fault(CorruptLength, int64(offset), envelopeSize, "payload length %d exceeds %d", env.stored, limit)
		}
		size := envelopeSize + uint64(env.stored)
		if offset+size > uint64(len(data)) {
			return envelope{}, 0, fault(CorruptTruncated, int64(offset), -1, "record of %d bytes runs past the chunk's end", size)
		}
		return env, size, nil
	}

	// Pass 1: the seal, and the digest it records.
	sealOffset, sealSize := uint64(0), uint64(0)
	for offset := uint64(preambleSize); offset < uint64(len(data)); {
		env, size, err := walk(offset)
		if err != nil {
			return nil, err
		}
		if env.kind == RecordChunkSeal {
			if offset+size != uint64(len(data)) {
				return nil, fault(CorruptStructure, int64(offset+size), -1, "bytes follow the seal")
			}
			sealOffset, sealSize = offset, size
			break
		}
		offset += size
	}
	if sealSize == 0 {
		return nil, fault(CorruptTruncated, int64(len(data)), -1, "chunk ends without a seal")
	}
	sealRecord := data[sealOffset:]
	env, _ := parseEnvelope(sealRecord)
	if err := env.checkBody(sealRecord, sealRecord[envelopeSize:]); err != nil {
		return nil, fault(CorruptChecksum, int64(sealOffset), int64(sealSize), "seal: %v", err)
	}
	var seal pb.ChunkSeal
	if err := unmarshalOptions.Unmarshal(sealRecord[envelopeSize:], &seal); err != nil {
		return nil, fault(CorruptStructure, int64(sealOffset), int64(sealSize), "seal: %v", err)
	}
	sum := sha256.Sum256(data[:sealOffset])
	if !bytes.Equal(sum[:], seal.BodySha256) || seal.BodyBytes != sealOffset {
		return nil, fault(CorruptDigest, 0, int64(sealOffset), "the chunk body does not match its seal; the index cannot be rebuilt from untrusted records")
	}
	ix := &chunkIndex{ordinal: ordinal, chunkBytes: uint64(len(data)), bodySHA: sum, sealOffset: sealOffset, sealLength: uint32(sealSize)}

	// Pass 2: the header, then every record, each checked and decoded.
	var next uint64
	for offset := uint64(preambleSize); offset < sealOffset; {
		env, size, err := walk(offset)
		if err != nil {
			return nil, err
		}
		record := data[offset : offset+size]
		payload := record[envelopeSize:]
		if err := env.checkBody(record, payload); err != nil {
			if errors.Is(err, ErrUnsupported) {
				return nil, err
			}
			return nil, fault(CorruptChecksum, int64(offset), int64(size), "%v", err)
		}
		if env.tag != r.tag {
			return nil, fault(CorruptDisagreement, int64(offset), int64(size), "record belongs to another container")
		}
		if offset == preambleSize {
			var h pb.ChunkHeader
			if env.kind != RecordChunkHeader || unmarshalOptions.Unmarshal(payload, &h) != nil {
				return nil, fault(CorruptStructure, int64(offset), int64(size), "chunk opens with a %s record, not its header", env.kind)
			}
			if h.Ordinal != ordinal || !bytes.Equal(h.ManifestSha256, r.manifestDigest[:]) {
				return nil, fault(CorruptDisagreement, int64(offset), int64(size), "chunk header names chunk %d of another manifest or position", h.Ordinal)
			}
			offset += size
			continue
		}
		e := indexEntry{offset: offset, length: uint32(size)}
		switch env.kind {
		case RecordFrame:
			f, err := decodeFrame(payload, limits)
			if err == nil {
				err = f.ValidateFor(r.manifest.Profile)
			}
			if err != nil {
				return nil, fault(CorruptRecord, int64(offset), int64(size), "%v", err)
			}
			e.entry = entry{kind: RecordFrame, first: f.Sequence, count: 1, timeKnown: true, start: f.CaptureStartUnixNanos, end: f.CaptureEndUnixNanos}
		case RecordGap:
			g, err := decodeGap(payload)
			if err == nil {
				err = g.Validate()
			}
			if err != nil {
				return nil, fault(CorruptRecord, int64(offset), int64(size), "%v", err)
			}
			e.entry = gapEntry(g, env.sequence)
		default:
			if !env.kind.Ancillary() {
				return nil, &UnsupportedError{What: fmt.Sprintf("critical record kind %#04x in chunk %d", uint16(env.kind), ordinal)}
			}
			e.entry = entry{kind: env.kind, first: env.sequence}
		}
		if len(ix.entries) == 0 {
			next = e.first
		}
		if e.first != next || env.sequence != e.first || env.time != e.envelopeTime() {
			return nil, fault(CorruptDisagreement, int64(offset), int64(size), "record at sequence %d disagrees with its envelope or position %d", e.first, next)
		}
		next = e.first + e.count
		ix.entries = append(ix.entries, e)
		offset += size
	}
	if err := r.checkIndex(ix, ordinal); err != nil {
		if errors.Is(err, ErrUnsupported) {
			return nil, err
		}
		return nil, fault(CorruptStructure, 0, -1, "%v", err)
	}
	return ix, nil
}
