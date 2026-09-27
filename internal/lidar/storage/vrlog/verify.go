package vrlog

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
)

// VerifyReport is what a full verification found.
type VerifyReport struct {
	Status
	// Semantic is the l4bobserve stream digest recomputed from the decoded
	// records. Two containers with equal Semantic hold the same evidence,
	// however they were chunked.
	Semantic l4bobserve.Digest
	// AncillarySkipped counts records of ancillary kinds this reader does
	// not know.
	AncillarySkipped uint64
	// IndexesRebuilt lists chunks whose index was rebuilt at Open.
	IndexesRebuilt []uint64
}

// Verify reads every sealed record and checks everything a reader can: each
// chunk's body digest; that its index equals one rebuilt from its bytes;
// every record's CRC, decoding, profile and the stream contract from sequence
// zero; each chunk's semantic digest against the one its writer computed from
// the values it was given, which is the codec's fidelity check; and, when the
// writer closed, the whole stream's semantic digest against the summary. The
// first failure is returned with its location. The cursor is left where it
// was.
func (r *Reader) Verify() (VerifyReport, error) {
	saved := cursor{r.chunkPos, r.entryPos}
	defer func() { r.chunkPos, r.entryPos = saved.chunk, saved.entry }()
	report := VerifyReport{Status: r.Status()}
	stream := l4bobserve.NewStreamValidator(r.manifest.Profile)
	semantic := l4bobserve.NewStreamDigest()
	for position, info := range r.chunks {
		if info.IndexRebuilt {
			report.IndexesRebuilt = append(report.IndexesRebuilt, info.Ordinal)
		}
		c, err := r.loadChunk(position)
		if err != nil {
			return report, err
		}
		rebuilt, err := r.rebuildIndex(info.Ordinal, c.data)
		if err != nil {
			return report, err
		}
		if i := firstIndexDifference(rebuilt, c.index); i >= 0 {
			return report, &CorruptionError{Kind: CorruptDisagreement, Object: indexObject(info.Ordinal), Chunk: int64(info.Ordinal),
				Length: -1, HaveSequences: true, FirstSequence: info.FirstSequence, EndSequence: info.EndSequence,
				Detail: fmt.Sprintf("the stored index differs from one rebuilt from the chunk at entry %d", i)}
		}
		chunkSemantic := l4bobserve.NewStreamDigest()
		for i := range c.index.entries {
			rec, known, err := r.readEntry(c, i)
			if err != nil {
				return report, err
			}
			if !known {
				report.AncillarySkipped++
				continue
			}
			var streamErr error
			switch rec.Kind {
			case RecordFrame:
				streamErr = stream.AddFrame(rec.Frame)
				d := l4bobserve.FrameDigest(rec.Frame)
				chunkSemantic.AddFrame(d)
				semantic.AddFrame(d)
			case RecordGap:
				streamErr = stream.AddGap(rec.Gap)
				d := l4bobserve.GapDigest(rec.Gap)
				chunkSemantic.AddGap(d)
				semantic.AddGap(d)
			}
			if streamErr != nil {
				first, end := c.index.entries[i].covers()
				return report, &CorruptionError{Kind: CorruptSequence, Object: chunkObject(info.Ordinal), Chunk: int64(info.Ordinal),
					Offset: int64(rec.Offset), Length: int64(rec.Length), HaveSequences: true, FirstSequence: first, EndSequence: end,
					Detail: streamErr.Error()}
			}
		}
		if chunkSemantic.Sum() != info.SemanticSHA256 {
			return report, &CorruptionError{Kind: CorruptSemantic, Object: chunkObject(info.Ordinal), Chunk: int64(info.Ordinal),
				Length: -1, HaveSequences: true, FirstSequence: info.FirstSequence, EndSequence: info.EndSequence,
				Detail: "decoded records do not reproduce the semantic digest their writer sealed"}
		}
	}
	report.Semantic = semantic.Sum()
	if r.summary != nil && report.Semantic != r.summary.Semantic {
		return report, &CorruptionError{Kind: CorruptSummary, Object: summaryName, Chunk: -1, Length: -1,
			Detail: "decoded records do not reproduce the stream's semantic digest"}
	}
	return report, nil
}

// firstIndexDifference returns the first entry at which two indexes of the
// same chunk differ, 0 for a header difference, or -1 when they are equal.
func firstIndexDifference(a, b *chunkIndex) int {
	if a.ordinal != b.ordinal || a.chunkBytes != b.chunkBytes || a.bodySHA != b.bodySHA ||
		a.sealOffset != b.sealOffset || a.sealLength != b.sealLength {
		return 0
	}
	for i := range max(len(a.entries), len(b.entries)) {
		if i >= len(a.entries) || i >= len(b.entries) || a.entries[i] != b.entries[i] {
			return i
		}
	}
	return -1
}
