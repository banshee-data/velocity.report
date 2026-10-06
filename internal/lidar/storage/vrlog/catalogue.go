package vrlog

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// The reader's catalogue is the committed chain: generations are read in
// order, each validated against its predecessor, and each chunk a generation
// commits is checked (index digest, index structure, seal) before it joins
// the catalogue. The directory is never listed to discover evidence.

// staged is a validated generation not yet applied to the reader, so a walk
// can stop at the first bad generation without half-applying it.
type staged struct {
	g       *pb.CommitGeneration
	state   chainState
	chunks  []ChunkInfo
	rebuilt map[int]*chunkIndex
}

// lastChunk returns the last catalogued chunk, or nil.
func (r *Reader) lastChunk() *ChunkInfo {
	if n := len(r.chunks); n > 0 {
		return &r.chunks[n-1]
	}
	return nil
}

// stage reads generation n and validates it after prev, whose last chunk is
// last, without changing the reader.
func (r *Reader) stage(prev chainState, last *ChunkInfo, n uint64) (*staged, error) {
	object := generationObject(n)
	g, digest, err := readGeneration(r.dir, n, r.tag)
	if err != nil {
		return nil, err
	}
	if err := checkGeneration(prev, g, r.manifestDigest); err != nil {
		if isUnsupported(err) {
			return nil, fmt.Errorf("%s: %w", object, err)
		}
		return nil, &CorruptionError{Kind: CorruptSequence, Object: object, Chunk: -1, Length: -1,
			HaveSequences: true, FirstSequence: prev.endSequence, EndSequence: max(g.GetEndSequence(), prev.endSequence+1),
			Detail: err.Error()}
	}
	st := &staged{g: g, state: chainAfter(prev, g, digest), rebuilt: map[int]*chunkIndex{}}
	var frames, gaps uint64
	firstRecord := prev.records
	for _, c := range g.GetChunks() {
		position := int(c.GetOrdinal())
		ix, rebuilt, err := r.openIndex(position, [sha256.Size]byte(c.GetIndexSha256()))
		if err != nil {
			return nil, err
		}
		if rebuilt {
			st.rebuilt[position] = ix
		}
		seal, err := r.readSeal(position, ix)
		if err != nil {
			return nil, err
		}
		info := ChunkInfo{Ordinal: c.GetOrdinal(), Bytes: ix.chunkBytes, BodySHA256: ix.bodySHA, IndexRebuilt: rebuilt,
			Generation: n, FirstRecord: firstRecord, indexSHA: [sha256.Size]byte(c.GetIndexSha256())}
		copy(info.SemanticSHA256[:], seal.GetSemanticSha256())
		info.Records, info.Frames, info.Gaps = seal.RecordCount, seal.FrameCount, seal.GapCount
		info.FirstSequence, info.EndSequence = seal.FirstSequence, seal.EndSequence
		if seal.FirstStartUnixNanos != nil {
			info.HaveTime, info.FirstStartUnixNanos, info.LastStartUnixNanos = true, *seal.FirstStartUnixNanos, *seal.LastStartUnixNanos
		}
		if ix.chunkBytes != c.GetChunkBytes() || [sha256.Size]byte(c.GetBodySha256()) != ix.bodySHA ||
			info.FirstSequence != c.GetFirstSequence() || info.EndSequence != c.GetEndSequence() || info.Records != c.GetRecordCount() {
			return nil, &CorruptionError{Kind: CorruptDisagreement, Object: chunkObject(c.GetOrdinal()), Chunk: int64(c.GetOrdinal()), Length: -1,
				HaveSequences: true, FirstSequence: c.GetFirstSequence(), EndSequence: max(c.GetEndSequence(), c.GetFirstSequence()+1),
				Detail: fmt.Sprintf("the chunk is not the one generation %d committed", n)}
		}
		if err := joinChunk(last, &info); err != nil {
			return nil, err
		}
		st.chunks = append(st.chunks, info)
		last = &st.chunks[len(st.chunks)-1]
		frames += info.Frames
		gaps += info.Gaps
		firstRecord += info.Records
	}
	if g.GetFrameCount() != prev.frames+frames || g.GetGapCount() != prev.gaps+gaps {
		return nil, &CorruptionError{Kind: CorruptDisagreement, Object: object, Chunk: -1, Length: -1,
			Detail: fmt.Sprintf("generation %d counts %d frames and %d gaps; its chunks' seals give %d and %d",
				n, g.GetFrameCount(), g.GetGapCount(), prev.frames+frames, prev.gaps+gaps)}
	}
	return st, nil
}

// apply adds a staged generation to the catalogue.
func (r *Reader) apply(st *staged) error {
	for position, ix := range st.rebuilt {
		r.rebuilt[position] = ix
	}
	r.chunks = append(r.chunks, st.chunks...)
	r.chain = st.state
	g := st.g
	switch g.GetKind() {
	case pb.GenerationKind_GENERATION_KIND_CLOSE:
		r.status = r.accountStatus()
		if err := r.readSummary(g.GetObjects()[0]); err != nil {
			return err
		}
	case pb.GenerationKind_GENERATION_KIND_FAILURE:
		f := g.GetFailure()
		r.status.Failure = &FailureMarker{Generation: g.GetGeneration(), Cause: FailureCause(f.GetCause()), Detail: f.GetDetail(),
			AcceptedEndSequence: f.GetAcceptedEndSequence(), AcceptedRecords: f.GetAcceptedRecordCount(), FailedUnixNanos: f.GetFailedUnixNanos()}
	case pb.GenerationKind_GENERATION_KIND_RECOVERY:
		v := g.GetRecovery()
		r.status.Recovery = &RecoveryMarker{Generation: g.GetGeneration(), PointerValid: v.GetPointerValid(),
			PointerGeneration: v.GetPointerGeneration(), PointerDetail: v.GetPointerDetail(), Promoted: v.GetPromotedGenerations(),
			Quarantined: v.GetQuarantined(), QuarantinedCount: v.GetQuarantinedCount(), SessionIncomplete: v.GetSessionIncomplete(),
			TailExtentUnknown: v.GetTailExtentUnknown(), RecoveredUnixNanos: v.GetRecoveredUnixNanos(), RecoveredBy: v.GetRecoveredBy()}
	}
	r.status = r.accountStatus()
	return nil
}

// accountStatus derives the committed account and state from the chain.
func (r *Reader) accountStatus() Status {
	s := r.status
	c := r.chain
	s.Generation, s.Chunks, s.Records, s.Frames, s.Gaps = c.generation, len(r.chunks), c.records, c.frames, c.gaps
	s.EndSequence, s.ChunkBytes, s.RejectedFrames, s.ShedFrames = c.endSequence, c.chunkBytes, c.rejected, c.shed
	s.Closed = c.closed && r.summary != nil
	switch {
	case c.closed:
		s.State = CaptureClosed
	case c.failed:
		s.State = CaptureFailed
	case c.recovered:
		s.State = CaptureIncomplete
	default:
		s.State = CaptureOpen
	}
	s.TailExtentUnknown = !c.closed && !c.failed
	return s
}

// extendTo catalogues generations up to and including target.
func (r *Reader) extendTo(target uint64) error {
	for r.chain.next() <= target || !r.chain.valid {
		st, err := r.stage(r.chain, r.lastChunk(), r.chain.next())
		if err != nil {
			return err
		}
		if err := r.apply(st); err != nil {
			return err
		}
		if r.chain.generation >= target {
			break
		}
	}
	return nil
}

// catalogueFromPointer validates the chain up to the generation the pointer
// names. That prefix was acknowledged, so damage inside it fails the read,
// located to the committed range. A pointer that cannot be read is torn: the
// longest chain that validates stands in for it.
func (r *Reader) catalogueFromPointer() error {
	ptr, err := readCurrent(r.dir, r.tag)
	if err != nil {
		if isUnsupported(err) {
			return fmt.Errorf("%s: %w", r.dir, err)
		}
		r.status.PointerFallback = err.Error()
		if stop := r.walk(); stop != nil {
			r.status.PointerFallback += "; the chain ends at generation " + fmt.Sprint(r.chain.next()) + ": " + stop.Error()
		}
		r.status = r.accountStatus()
		return nil
	}
	if err := r.extendTo(ptr.GetGeneration()); err != nil {
		return r.committedDamage(err, ptr)
	}
	if r.chain.digest != [sha256.Size]byte(ptr.GetGenerationSha256()) || r.chain.endSequence != ptr.GetEndSequence() ||
		r.chain.records != ptr.GetRecordCount() {
		return &CorruptionError{Kind: CorruptDisagreement, Object: generationObject(ptr.GetGeneration()), Chunk: -1, Length: -1,
			HaveSequences: true, FirstSequence: min(r.chain.endSequence, ptr.GetEndSequence()),
			EndSequence: max(r.chain.endSequence, ptr.GetEndSequence(), min(r.chain.endSequence, ptr.GetEndSequence())+1),
			Detail:      "the current pointer names a generation with a different digest or account: committed evidence was replaced"}
	}
	r.status = r.accountStatus()
	return nil
}

// walk extends the catalogue while generations validate, returning what
// stopped it (nil at a missing generation, the chain's natural end).
func (r *Reader) walk() error {
	for {
		st, err := r.stage(r.chain, r.lastChunk(), r.chain.next())
		if err != nil {
			if isMissing(err) {
				return nil
			}
			return err
		}
		if err := r.apply(st); err != nil {
			return err
		}
	}
}

func isMissing(err error) bool {
	var c *CorruptionError
	return errors.As(err, &c) && c.Kind == CorruptMissing && c.Chunk < 0
}

// committedDamage locates damage inside the acknowledged prefix: everything
// from the last good generation's end to the pointer's end is affected.
func (r *Reader) committedDamage(err error, ptr *pb.CurrentGeneration) error {
	var c *CorruptionError
	if !errors.As(err, &c) {
		return err
	}
	if !c.HaveSequences {
		c.HaveSequences, c.FirstSequence = true, r.chain.endSequence
		c.EndSequence = max(ptr.GetEndSequence(), r.chain.endSequence+1)
	}
	c.Detail = fmt.Sprintf("in the committed prefix (the pointer names generation %d): %s", ptr.GetGeneration(), c.Detail)
	return c
}

// probeBeyond reports, without reading them as evidence, complete
// generations past the committed one and the objects no generation
// references. It probes by name within the writer's bounds (at most one
// batch committing and one open), so a long capture's directories are
// never listed; Recover lists them in full.
func (r *Reader) probeBeyond() {
	prev, last := r.chain, r.lastChunk()
	chunks := uint64(len(r.chunks))
	next := prev.next()
	if r.status.PointerFallback == "" {
		for {
			st, err := r.stage(prev, last, next)
			if err != nil {
				break
			}
			r.status.Unpromoted = append(r.status.Unpromoted, next)
			prev = st.state
			if n := len(st.chunks); n > 0 {
				last = &st.chunks[n-1]
				chunks += uint64(n)
			}
			next++
		}
	}
	exists := func(rel string) bool {
		_, err := os.Lstat(filepath.Join(r.dir, rel))
		return err == nil || !errors.Is(err, fs.ErrNotExist)
	}
	var tail []string
	for ordinal := chunks; ordinal < chunks+3; ordinal++ {
		base := chunksDir + "/" + chunkBase(ordinal)
		for _, suffix := range []string{chunkSuffix, chunkSuffix + openSuffix, indexSuffix, indexSuffix + openSuffix} {
			if exists(base + suffix) {
				tail = append(tail, base+suffix)
			}
		}
	}
	for _, rel := range []string{generationObject(next), generationObject(next) + openSuffix, summaryName + openSuffix} {
		if exists(rel) {
			tail = append(tail, rel)
		}
	}
	if !r.chain.closed && exists(summaryName) {
		tail = append(tail, summaryName)
	}
	r.status.UncommittedTail = tail
}

// Refresh re-reads the pointer and catalogues any newer generations. A
// filesystem notification is only a hint to call it: seeing a renamed
// pointer does not prove the writer's final directory sync completed, which
// is why a live consumer follows the writer's frontier instead. It returns
// how many generations were added.
func (r *Reader) Refresh() (int, error) {
	ptr, err := readCurrent(r.dir, r.tag)
	if err != nil {
		return 0, err
	}
	before := r.chain.generation
	switch {
	case ptr.GetGeneration() < before:
		return 0, &staleCursorError{detail: fmt.Sprintf("the pointer went back from generation %d to %d", before, ptr.GetGeneration())}
	case ptr.GetGeneration() == before:
		return 0, nil
	}
	if err := r.extendTo(ptr.GetGeneration()); err != nil {
		return int(r.chain.generation - before), r.committedDamage(err, ptr)
	}
	return int(r.chain.generation - before), nil
}
