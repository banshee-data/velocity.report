package vrlog

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// The commit chain. Generation 0 is written by Create and commits nothing;
// every later generation commits whole sealed chunks (or, when terminal, a
// closing summary, a failure marker or a recovery record) and names its
// predecessor's SHA-256. The current pointer names the latest generation the
// writer published. Evidence is exactly what the validated chain commits: a
// sealed chunk no generation names is an uncommitted tail, however intact.

// chainState is the chain after one generation: which generation, its
// object digest, and the cumulative account every generation carries.
type chainState struct {
	valid      bool // false before generation 0 is known
	generation uint64
	digest     [sha256.Size]byte
	kind       pb.GenerationKind
	// closed, failed and recovered record the terminal kinds seen so far.
	closed, failed, recovered bool
	chunks                    uint64
	chain                     l4bobserve.Digest
	records, frames, gaps     uint64
	endSequence, chunkBytes   uint64
	rejected, shed            uint64
}

// next is the number the following generation must carry.
func (c chainState) next() uint64 {
	if !c.valid {
		return 0
	}
	return c.generation + 1
}

// terminal reports whether the chain has ended: after a close, failure or
// recovery generation only recovery may follow. A recovered session is
// incomplete and is never resumed; a later capture is a new container.
func (c chainState) terminal() bool { return c.closed || c.failed || c.recovered }

// batchDelta is what one generation adds to the chain.
type batchDelta struct {
	kind    pb.GenerationKind
	trigger pb.CommitTrigger
	chunk   *pb.CommittedChunk // nil when the generation commits no chunk
	frames  uint64             // frame and gap records in chunk
	gaps    uint64
	// rejected and shed are the frames replaced by gaps in chunk.
	rejected, shed uint64
	objects        []*pb.CommittedObject
	failure        *pb.CaptureFailure
	recovery       *pb.RecoveryRecord
	batchAgeNanos  int64
	syncNanos      int64
	committedNanos int64
}

// apply returns the generation message for d and the chain after it.
func (c chainState) apply(manifestDigest [sha256.Size]byte, d batchDelta) (*pb.CommitGeneration, chainState) {
	n := c
	n.valid, n.generation, n.kind = true, c.next(), d.kind
	if d.chunk != nil {
		n.chunks++
		n.chain = chainDigest(c.chain, d.chunk.BodySha256)
		n.records += d.chunk.RecordCount
		n.frames += d.frames
		n.gaps += d.gaps
		n.endSequence = d.chunk.EndSequence
		n.chunkBytes += d.chunk.ChunkBytes
	}
	n.rejected += d.rejected
	n.shed += d.shed
	switch d.kind {
	case pb.GenerationKind_GENERATION_KIND_CLOSE:
		n.closed = true
	case pb.GenerationKind_GENERATION_KIND_FAILURE:
		n.failed = true
	case pb.GenerationKind_GENERATION_KIND_RECOVERY:
		n.recovered = true
	}
	g := &pb.CommitGeneration{
		Generation: n.generation, ManifestSha256: manifestDigest[:], Kind: d.kind, Trigger: d.trigger,
		Objects: d.objects, ChunkCount: n.chunks, ChunkChainSha256: n.chain[:], RecordCount: n.records,
		FrameCount: n.frames, GapCount: n.gaps, EndSequence: n.endSequence, TotalChunkBytes: n.chunkBytes,
		RejectedFrames: n.rejected, ShedFrames: n.shed, CommittedUnixNanos: d.committedNanos,
		BatchAgeNanos: d.batchAgeNanos, SyncNanos: d.syncNanos, Failure: d.failure, Recovery: d.recovery,
	}
	if c.valid {
		g.PreviousSha256 = c.digest[:]
	}
	if d.chunk != nil {
		g.Chunks = []*pb.CommittedChunk{d.chunk}
	}
	return g, n
}

// encodeGeneration returns a generation object's bytes.
func encodeGeneration(tag [8]byte, g *pb.CommitGeneration) ([]byte, error) {
	payload, err := marshalOptions.Marshal(g)
	if err != nil {
		return nil, fmt.Errorf("encode generation %d: %w", g.Generation, err)
	}
	if len(payload) > maxGenerationBytes {
		return nil, fmt.Errorf("generation %d of %d bytes exceeds %d", g.Generation, len(payload), maxGenerationBytes)
	}
	return appendRecord(appendPreamble(nil, objectGeneration), envelope{kind: RecordGeneration, tag: tag, sequence: g.Generation}, payload), nil
}

// encodeCurrent returns the pointer object naming c.
func encodeCurrent(tag [8]byte, c chainState) ([]byte, error) {
	payload, err := marshalOptions.Marshal(&pb.CurrentGeneration{Generation: c.generation, GenerationSha256: c.digest[:],
		EndSequence: c.endSequence, RecordCount: c.records})
	if err != nil {
		return nil, err
	}
	return appendRecord(appendPreamble(nil, objectCurrent), envelope{kind: RecordCurrent, tag: tag, sequence: c.generation}, payload), nil
}

// decodeSingle checks an object's preamble and single record and returns
// the payload, locating damage to the object.
func decodeSingle(data []byte, kind objectKind, record RecordKind, tag [8]byte, maxPayload int, object string) ([]byte, error) {
	if err := checkPreamble(data, kind); err != nil {
		if isUnsupported(err) {
			return nil, err
		}
		return nil, &CorruptionError{Kind: CorruptStructure, Object: object, Chunk: -1, Length: preambleSize, Detail: err.Error()}
	}
	payload, err := singleRecord(data, record, tag, maxPayload)
	if err != nil {
		if f, ok := err.(*objectFault); ok {
			return nil, &CorruptionError{Kind: f.kind, Object: object, Chunk: -1, Offset: f.offset, Length: -1, Detail: f.err.Error()}
		}
		if isUnsupported(err) {
			return nil, err
		}
		return nil, &CorruptionError{Kind: CorruptStructure, Object: object, Chunk: -1, Length: -1, Detail: err.Error()}
	}
	return payload, nil
}

// readGeneration reads, bounds and decodes generation n, returning it with
// the SHA-256 of its object.
func readGeneration(dir string, n uint64, tag [8]byte) (*pb.CommitGeneration, [sha256.Size]byte, error) {
	object := generationObject(n)
	data, err := readBounded(filepath.Join(dir, object), preambleSize+envelopeSize+maxGenerationBytes)
	if err != nil {
		return nil, [sha256.Size]byte{}, locateObject(err, object)
	}
	payload, err := decodeSingle(data, objectGeneration, RecordGeneration, tag, maxGenerationBytes, object)
	if err != nil {
		return nil, [sha256.Size]byte{}, err
	}
	var g pb.CommitGeneration
	if err := unmarshalOptions.Unmarshal(payload, &g); err != nil {
		return nil, [sha256.Size]byte{}, &CorruptionError{Kind: CorruptStructure, Object: object, Chunk: -1,
			Offset: preambleSize + envelopeSize, Length: -1, Detail: err.Error()}
	}
	return &g, sha256.Sum256(data), nil
}

// readCurrent reads the pointer object.
func readCurrent(dir string, tag [8]byte) (*pb.CurrentGeneration, error) {
	data, err := readBounded(filepath.Join(dir, currentName), preambleSize+envelopeSize+maxCurrentBytes)
	if err != nil {
		return nil, locateObject(err, currentName)
	}
	payload, err := decodeSingle(data, objectCurrent, RecordCurrent, tag, maxCurrentBytes, currentName)
	if err != nil {
		return nil, err
	}
	var c pb.CurrentGeneration
	if err := unmarshalOptions.Unmarshal(payload, &c); err != nil {
		return nil, &CorruptionError{Kind: CorruptStructure, Object: currentName, Chunk: -1, Offset: preambleSize + envelopeSize, Length: -1, Detail: err.Error()}
	}
	if len(c.GenerationSha256) != sha256.Size {
		return nil, &CorruptionError{Kind: CorruptStructure, Object: currentName, Chunk: -1, Length: -1, Detail: "pointer digest is malformed"}
	}
	return &c, nil
}

// locateObject names the object in a read error. A missing object stays a
// missing-object corruption the caller can recognise.
func locateObject(err error, object string) error {
	return classifyReadError(err, object, -1)
}

// classifyReadError sorts an object read's error. Corruption is located on
// the object; only an object that does not exist is a missing-object
// corruption, which a chain walk may take as its end. Any other failure (a
// permission or I/O error) is operational and returned as such, so a reader
// fails rather than accepting a truncated view of the evidence.
func classifyReadError(err error, object string, chunk int64) error {
	var corrupt *CorruptionError
	if errors.As(err, &corrupt) {
		corrupt.Object, corrupt.Chunk = object, chunk
		return corrupt
	}
	if errors.Is(err, fs.ErrNotExist) {
		return &CorruptionError{Kind: CorruptMissing, Object: object, Chunk: chunk, Length: -1, Detail: err.Error()}
	}
	return fmt.Errorf("read %s: %w", object, err)
}

// checkGeneration verifies everything a generation promises that does not
// need its chunks' objects: its place in the chain, its kind, and that its
// cumulative account is its predecessor's plus its own chunks. The chunks'
// indexes and seals are checked against it by the reader.
func checkGeneration(prev chainState, g *pb.CommitGeneration, manifestDigest [sha256.Size]byte) error {
	want := prev.next()
	kind := g.GetKind()
	switch {
	case g.GetGeneration() != want:
		return fmt.Errorf("generation object %d names itself generation %d", want, g.GetGeneration())
	case !bytes.Equal(g.GetManifestSha256(), manifestDigest[:]):
		return fmt.Errorf("generation %d belongs to another manifest", want)
	case prev.valid && !bytes.Equal(g.GetPreviousSha256(), prev.digest[:]):
		return fmt.Errorf("generation %d does not follow generation %d: previous digest differs", want, prev.generation)
	case !prev.valid && len(g.GetPreviousSha256()) != 0:
		return fmt.Errorf("generation 0 names a predecessor")
	case (want == 0) != (kind == pb.GenerationKind_GENERATION_KIND_OPEN):
		return fmt.Errorf("generation %d is of kind %s; only generation 0 opens the chain", want, kind)
	case kind == pb.GenerationKind_GENERATION_KIND_UNSPECIFIED || kind > pb.GenerationKind_GENERATION_KIND_RECOVERY:
		return &UnsupportedError{What: fmt.Sprintf("generation kind %d", kind)}
	case prev.terminal() && kind != pb.GenerationKind_GENERATION_KIND_RECOVERY:
		return fmt.Errorf("generation %d (%s) follows a terminal generation", want, kind)
	case len(g.GetChunks()) > 0 && kind != pb.GenerationKind_GENERATION_KIND_COMMIT && kind != pb.GenerationKind_GENERATION_KIND_CLOSE:
		return fmt.Errorf("a %s generation commits chunks", kind)
	case len(g.GetChunks()) > 1:
		// The format allows more; this writer seals one chunk per batch, and
		// a reader that met more would need to bound them.
		return &UnsupportedError{What: fmt.Sprintf("generation committing %d chunks", len(g.GetChunks()))}
	case (g.GetFailure() != nil) != (kind == pb.GenerationKind_GENERATION_KIND_FAILURE):
		return fmt.Errorf("generation %d: failure record and kind %s disagree", want, kind)
	case (g.GetRecovery() != nil) != (kind == pb.GenerationKind_GENERATION_KIND_RECOVERY):
		return fmt.Errorf("generation %d: recovery record and kind %s disagree", want, kind)
	case len(g.GetRecovery().GetQuarantined()) > maxListedQuarantine:
		return fmt.Errorf("recovery record lists %d objects", len(g.GetRecovery().GetQuarantined()))
	}
	for _, o := range g.GetObjects() {
		if kind != pb.GenerationKind_GENERATION_KIND_CLOSE || o.GetName() != summaryName || len(o.GetSha256()) != sha256.Size || len(g.GetObjects()) != 1 {
			return fmt.Errorf("generation %d (%s) publishes an unexpected object %q", want, kind, o.GetName())
		}
	}
	if kind == pb.GenerationKind_GENERATION_KIND_CLOSE && len(g.GetObjects()) != 1 {
		return fmt.Errorf("close generation %d does not publish a summary", want)
	}
	end, records, chunks, bytesTotal, chain := prev.endSequence, prev.records, prev.chunks, prev.chunkBytes, prev.chain
	for _, c := range g.GetChunks() {
		switch {
		case c.GetOrdinal() != chunks:
			return fmt.Errorf("generation %d commits chunk %d where chunk %d was next", want, c.GetOrdinal(), chunks)
		case c.GetFirstSequence() != end || c.GetEndSequence() < c.GetFirstSequence():
			return fmt.Errorf("generation %d commits chunk %d over [%d, %d) where sequence %d was next", want, c.GetOrdinal(),
				c.GetFirstSequence(), c.GetEndSequence(), end)
		case len(c.GetBodySha256()) != sha256.Size || len(c.GetIndexSha256()) != sha256.Size || c.GetRecordCount() == 0:
			return fmt.Errorf("generation %d: chunk %d entry is malformed", want, c.GetOrdinal())
		}
		chunks++
		end = c.GetEndSequence()
		records += c.GetRecordCount()
		bytesTotal += c.GetChunkBytes()
		chain = chainDigest(chain, c.GetBodySha256())
	}
	switch {
	case g.GetChunkCount() != chunks || !bytes.Equal(g.GetChunkChainSha256(), chain[:]):
		return fmt.Errorf("generation %d counts %d chunks; its chain has %d or a different digest", want, g.GetChunkCount(), chunks)
	case g.GetRecordCount() != records || g.GetEndSequence() != end || g.GetTotalChunkBytes() != bytesTotal:
		return fmt.Errorf("generation %d account (%d records to sequence %d, %d bytes) is not its predecessor's plus its chunks (%d, %d, %d)",
			want, g.GetRecordCount(), g.GetEndSequence(), g.GetTotalChunkBytes(), records, end, bytesTotal)
	case g.GetFrameCount() < prev.frames || g.GetGapCount() < prev.gaps || g.GetRejectedFrames() < prev.rejected || g.GetShedFrames() < prev.shed:
		return fmt.Errorf("generation %d account goes backwards", want)
	case g.GetFrameCount()+g.GetGapCount() > g.GetRecordCount():
		return fmt.Errorf("generation %d counts more frames and gaps than records", want)
	case len(g.GetChunks()) == 0 && (g.GetFrameCount() != prev.frames || g.GetGapCount() != prev.gaps ||
		g.GetRejectedFrames() != prev.rejected || g.GetShedFrames() != prev.shed):
		return fmt.Errorf("generation %d commits no chunk but changes the record account", want)
	}
	return nil
}

// chainAfter returns the chain state g establishes, once checked.
func chainAfter(prev chainState, g *pb.CommitGeneration, digest [sha256.Size]byte) chainState {
	n := prev
	n.valid, n.generation, n.digest, n.kind = true, g.GetGeneration(), digest, g.GetKind()
	n.chunks, n.records, n.frames, n.gaps = g.GetChunkCount(), g.GetRecordCount(), g.GetFrameCount(), g.GetGapCount()
	n.endSequence, n.chunkBytes, n.rejected, n.shed = g.GetEndSequence(), g.GetTotalChunkBytes(), g.GetRejectedFrames(), g.GetShedFrames()
	copy(n.chain[:], g.GetChunkChainSha256())
	switch g.GetKind() {
	case pb.GenerationKind_GENERATION_KIND_CLOSE:
		n.closed = true
	case pb.GenerationKind_GENERATION_KIND_FAILURE:
		n.failed = true
	case pb.GenerationKind_GENERATION_KIND_RECOVERY:
		n.recovered = true
	}
	return n
}
