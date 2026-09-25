package vrlog

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// requireCorruption asserts a located corruption of the given kind.
func requireCorruption(t *testing.T, err error, kind CorruptionKind, chunk int64) *CorruptionError {
	t.Helper()
	var c *CorruptionError
	if !errors.As(err, &c) || !errors.Is(err, ErrCorrupt) {
		t.Fatalf("error = %v, want a %s corruption", err, kind)
	}
	if c.Kind != kind || c.Chunk != chunk {
		t.Fatalf("corruption = %s in chunk %d (%v), want %s in chunk %d", c.Kind, c.Chunk, err, kind, chunk)
	}
	return c
}

// damaged returns a fresh closed container with several chunks.
func damaged(t *testing.T) (string, []ChunkInfo) {
	t.Helper()
	dir, _, _ := writeTestContainer(t, smallChunks())
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	chunks := r.Chunks()
	if len(chunks) < 3 {
		t.Fatalf("need three chunks, have %d", len(chunks))
	}
	return dir, chunks
}

// readUntilError reads until an error, returning the records read before it.
func readUntilError(t *testing.T, r *Reader) (int, error) {
	t.Helper()
	n := 0
	for {
		_, err := r.Next()
		if err != nil {
			return n, err
		}
		n++
	}
}

// A flipped payload bit fails the chunk digest before any of the chunk's
// records is returned, names the chunk's sequence range, and keeps failing:
// only an explicit seek past it continues.
func TestBitFlipFailsTheWholeChunk(t *testing.T) {
	dir, chunks := damaged(t)
	offset, length := RecordSpan(t, dir, 1, 0)
	FlipBit(t, ChunkPath(dir, 1), offset+uint64(length)-3, 2)

	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatalf("the seal and index are intact, so Open should succeed: %v", err)
	}
	n, err := readUntilError(t, r)
	c := requireCorruption(t, err, CorruptDigest, 1)
	if uint64(n) != chunks[0].Records || !c.HaveSequences || c.FirstSequence != chunks[1].FirstSequence || c.EndSequence != chunks[1].EndSequence {
		t.Fatalf("read %d records before %+v", n, c)
	}
	if _, again := r.Next(); !errors.Is(again, ErrCorrupt) {
		t.Fatalf("the cursor moved past a corrupt chunk: %v", again)
	}
	if err := r.SeekSequence(chunks[2].FirstSequence); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Next(); err != nil {
		t.Fatalf("an explicit seek past the damage failed: %v", err)
	}
	if _, err := r.Verify(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Verify passed a corrupt chunk: %v", err)
	}
}

// Truncation is caught at Open from the size the index records, and a
// rebuild from the short chunk cannot find its seal.
func TestTruncationIsLocated(t *testing.T) {
	dir, chunks := damaged(t)
	Truncate(t, ChunkPath(dir, 2), int64(chunks[2].Bytes)-10)
	_, err := Open(dir, Options{})
	requireCorruption(t, err, CorruptTruncated, 2)

	os.Remove(IndexPath(dir, 2))
	_, err = Open(dir, Options{RebuildIndexes: true})
	requireCorruption(t, err, CorruptTruncated, 2)
}

// An index is derived data: without it Open refuses unless asked to
// rebuild, a rebuilt index reads identically, and no rebuild can make a
// chunk with a failed digest readable.
func TestIndexRebuildCannotOverrideADigest(t *testing.T) {
	dir, _ := damaged(t)
	want := readAll(t, dir, Options{})
	os.Remove(IndexPath(dir, 1))
	_, err := Open(dir, Options{})
	requireCorruption(t, err, CorruptMissing, 1)

	r, err := Open(dir, Options{RebuildIndexes: true})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Chunks()[1].IndexRebuilt {
		t.Fatal("the rebuilt index is not reported")
	}
	report, err := r.Verify()
	if err != nil || len(report.IndexesRebuilt) != 1 {
		t.Fatalf("verify = %+v, %v", report, err)
	}
	assertRecords(t, want, readAll(t, dir, Options{RebuildIndexes: true}))

	offset, _ := RecordSpan(t, dir, 0, 1)
	os.Remove(IndexPath(dir, 0))
	FlipBit(t, ChunkPath(dir, 0), offset+envelopeSize+2, 0)
	_, err = Open(dir, Options{RebuildIndexes: true})
	requireCorruption(t, err, CorruptDigest, 0)
}

// A damaged index is refused by its own CRC; rebuilding replaces it.
func TestDamagedIndexIsRefusedAndRebuilt(t *testing.T) {
	dir, _ := damaged(t)
	FlipBit(t, IndexPath(dir, 0), preambleSize+envelopeSize+5, 1)
	_, err := Open(dir, Options{})
	requireCorruption(t, err, CorruptChecksum, 0)
	if _, err := Open(dir, Options{RebuildIndexes: true}); err != nil {
		t.Fatal(err)
	}
}

// Swapped chunks are refused: the seal and header name their own ordinal,
// and sequences must run on across chunks.
func TestReorderedChunksAreRefused(t *testing.T) {
	dir, _ := damaged(t)
	SwapFiles(t, ChunkPath(dir, 0), ChunkPath(dir, 1))
	_, err := Open(dir, Options{})
	requireCorruption(t, err, CorruptDisagreement, 0)

	dir, _ = damaged(t)
	SwapFiles(t, ChunkPath(dir, 1), ChunkPath(dir, 2))
	SwapFiles(t, IndexPath(dir, 1), IndexPath(dir, 2))
	_, err = Open(dir, Options{})
	requireCorruption(t, err, CorruptDisagreement, 1)
}

// A forged length that every checksum agrees with is still refused by
// bounds, before the reader allocates for it or reads past the record.
func TestOversizedLengthIsRefusedByBounds(t *testing.T) {
	dir, chunks := damaged(t)
	ForgeRecordLength(t, dir, 1, 0, 0xfffffff0)
	// The closing summary chains the chunk digests and would catch the
	// reseal; without it only the length can give the forgery away.
	os.Remove(filepath.Join(dir, summaryName))
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatalf("a consistently resealed chunk should open: %v", err)
	}
	n, err := readUntilError(t, r)
	c := requireCorruption(t, err, CorruptLength, 1)
	if uint64(n) != chunks[0].Records || !c.HaveSequences || c.FirstSequence != chunks[1].FirstSequence || c.Length <= 0 {
		t.Fatalf("after %d records, corruption is not located: %+v", n, c)
	}

	// The same lie in the index is refused at Open.
	dir, _ = damaged(t)
	RewriteIndex(t, dir, 2, func(ix *pb.ChunkIndex) { ix.Length[0] = 0xfffffff0 }, nil)
	_, err = Open(dir, Options{})
	requireCorruption(t, err, CorruptDisagreement, 2)

	// And an object padded past its bound is refused before it is read.
	dir, _ = damaged(t)
	f, err := os.OpenFile(filepath.Join(dir, summaryName), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.Write(make([]byte, maxSummaryBytes+envelopeSize))
	f.Close()
	_, err = Open(dir, Options{})
	requireCorruption(t, err, CorruptLength, -1)
}

// An index that locates records differently from the chunk is refused, and
// Verify compares every stored index with one rebuilt from its chunk.
func TestIndexChunkDisagreement(t *testing.T) {
	dir, _ := damaged(t)
	RewriteIndex(t, dir, 1, func(ix *pb.ChunkIndex) { ix.TimeStart[1]++ }, nil)
	_, err := Open(dir, Options{})
	requireCorruption(t, err, CorruptDisagreement, 1)

	dir, _ = damaged(t)
	RewriteIndex(t, dir, 1, func(ix *pb.ChunkIndex) { ix.TimeEnd[0]-- }, nil)
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Verify()
	requireCorruption(t, err, CorruptDisagreement, 1)
}

// Missing chunks: a hole in the ordinals is refused; a missing final chunk
// is caught by the closing summary, and without one the capture reads as
// unclosed rather than complete.
func TestMissingChunks(t *testing.T) {
	dir, _ := damaged(t)
	os.Remove(ChunkPath(dir, 1))
	os.Remove(IndexPath(dir, 1))
	_, err := Open(dir, Options{})
	requireCorruption(t, err, CorruptMissing, 1)

	dir, chunks := damaged(t)
	last := uint64(len(chunks) - 1)
	os.Remove(ChunkPath(dir, last))
	os.Remove(IndexPath(dir, last))
	_, err = Open(dir, Options{})
	requireCorruption(t, err, CorruptSummary, -1)

	os.Remove(filepath.Join(dir, summaryName))
	r, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st := r.Status(); st.Closed || st.EndSequence != chunks[last].FirstSequence {
		t.Fatalf("status = %+v", st)
	}
}

// A chunk from another capture is refused even at the right ordinal.
func TestForeignChunkIsRefused(t *testing.T) {
	dir, _ := damaged(t)
	other, _ := damaged(t)
	for _, p := range []func(string, uint64) string{ChunkPath, IndexPath} {
		data, err := os.ReadFile(p(other, 1))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p(dir, 1), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := Open(dir, Options{})
	requireCorruption(t, err, CorruptDisagreement, 1)
}

// Damage to the small objects is located to them.
func TestManifestAndSummaryDamage(t *testing.T) {
	dir, _ := damaged(t)
	FlipBit(t, filepath.Join(dir, summaryName), preambleSize+envelopeSize+1, 0)
	_, err := Open(dir, Options{})
	requireCorruption(t, err, CorruptChecksum, -1)

	dir, _ = damaged(t)
	FlipBit(t, filepath.Join(dir, manifestName), preambleSize+envelopeSize+4, 3)
	_, err = Open(dir, Options{})
	if c := requireCorruption(t, err, CorruptChecksum, -1); c.Object != manifestName {
		t.Fatalf("damage located to %s", c.Object)
	}
	if errors.Is(err, ErrNotContainer) {
		t.Fatal("a damaged manifest was reported as not a container")
	}
	if _, err := readUntilError(t, &Reader{}); !errors.Is(err, io.EOF) {
		t.Fatalf("an empty reader returned %v", err)
	}
}
