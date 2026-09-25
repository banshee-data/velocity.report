package vrlog

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
)

// Cursor is a durable position in a capture's committed stream: the number
// of records before it, and a generation, committed when the cursor was
// taken, that covers them. Records, not sequences, because a gap between
// received frames covers no sequence and would make a sequence position
// ambiguous. A consumer checkpoints a cursor and resumes from it; a cursor
// that no longer describes this container's committed prefix is stale.
type Cursor struct {
	CaptureUUID string
	// ContainerTag is the hex container tag (the manifest digest's first
	// eight bytes), which tells two containers apart even if a capture UUID
	// were reused.
	ContainerTag string
	Record       uint64
	Generation   uint64
}

// Cursor returns the reader's position.
func (r *Reader) Cursor() Cursor {
	return Cursor{CaptureUUID: r.manifest.Capture.UUID, ContainerTag: hex.EncodeToString(r.tag[:]),
		Record: r.position(), Generation: r.chain.generation}
}

// position is the number of records before the cursor.
func (r *Reader) position() uint64 {
	if r.chunkPos < len(r.chunks) {
		return r.chunks[r.chunkPos].FirstRecord + uint64(r.entryPos)
	}
	return r.chain.records
}

// recordsThrough is how many records generations 0 to g commit.
func (r *Reader) recordsThrough(g uint64) uint64 {
	i := sort.Search(len(r.chunks), func(i int) bool { return r.chunks[i].Generation > g })
	if i == 0 {
		return 0
	}
	last := r.chunks[i-1]
	return last.FirstRecord + last.Records
}

// SeekCursor positions the reader at c. It refuses, with ErrStaleCursor, a
// cursor from another container, one naming a generation this container
// has not committed, and one beyond what its generation commits: each means
// the cursor does not describe this committed prefix, and resuming from it
// would skip or repeat evidence.
func (r *Reader) SeekCursor(c Cursor) error {
	if c.CaptureUUID != r.manifest.Capture.UUID || c.ContainerTag != hex.EncodeToString(r.tag[:]) {
		return &staleCursorError{detail: fmt.Sprintf("the cursor belongs to capture %s (container %s), not %s (%x)",
			c.CaptureUUID, c.ContainerTag, r.manifest.Capture.UUID, r.tag)}
	}
	if !r.chain.valid || c.Generation > r.chain.generation {
		return &staleCursorError{detail: fmt.Sprintf("generation %d is not committed here; the last committed generation is %d: "+
			"the container lost committed generations, or the cursor was taken from an unacknowledged one", c.Generation, r.chain.generation)}
	}
	if through := r.recordsThrough(c.Generation); c.Record > through {
		return &staleCursorError{detail: fmt.Sprintf("the cursor is at record %d, but generation %d commits %d records", c.Record, c.Generation, through)}
	}
	r.seekRecord(c.Record)
	return nil
}

// seekRecord positions the cursor before record n, or at the end.
func (r *Reader) seekRecord(n uint64) {
	i := sort.Search(len(r.chunks), func(i int) bool { return r.chunks[i].FirstRecord+r.chunks[i].Records > n })
	if i == len(r.chunks) {
		r.chunkPos, r.entryPos = len(r.chunks), 0
		return
	}
	r.chunkPos, r.entryPos = i, int(n-r.chunks[i].FirstRecord)
}

// Follower reads a live capture's committed records as the writer commits
// them. It advances only through generations its FrontierSource has
// announced, so it never sees an uncommitted or partial frame, and never
// trusts a pointer or a directory listing. Records come in stream order,
// each exactly once. The writer never waits for a follower: a slow or paused
// one simply finds more committed generations when it resumes.
//
// A follower holds one chunk's bytes, a few indexes, and a descriptor per
// committed chunk. It is not safe for concurrent use; give each consumer
// its own.
type Follower struct {
	r        *Reader
	src      FrontierSource
	frontier Frontier
}

// Follow opens dir through the frontier src has announced.
func Follow(dir string, src FrontierSource, opts Options) (*Follower, error) {
	f := src.Frontier()
	r, err := openThrough(dir, opts, f.Generation)
	if err != nil {
		return nil, err
	}
	return &Follower{r: r, src: src, frontier: f}, nil
}

// Manifest returns the container's manifest.
func (f *Follower) Manifest() Manifest { return f.r.manifest }

// Frontier is the last frontier the follower caught up with.
func (f *Follower) Frontier() Frontier { return f.frontier }

// Status is the committed prefix the follower has catalogued.
func (f *Follower) Status() Status { return f.r.Status() }

// Cursor returns the follower's position.
func (f *Follower) Cursor() Cursor { return f.r.Cursor() }

// Close releases the follower's cache.
func (f *Follower) Close() error { return f.r.Close() }

// Next returns the next committed record, waiting for the writer to commit
// one if necessary. At the end of the stream it returns io.EOF for a closed
// capture, the capture failure for a failed one, and an error wrapping
// io.ErrUnexpectedEOF when the writer stopped without a terminal generation.
// A ctx error leaves the position unchanged.
func (f *Follower) Next(ctx context.Context) (Record, error) {
	for {
		rec, err := f.r.Next()
		if !errors.Is(err, io.EOF) {
			return rec, err
		}
		if f.r.chain.terminal() {
			return Record{}, f.end()
		}
		fr, err := f.src.WaitFrontier(ctx, f.r.chain.generation)
		if err != nil {
			return Record{}, err
		}
		f.frontier = fr
		if fr.Generation > f.r.chain.generation {
			if err := f.r.extendTo(fr.Generation); err != nil {
				return Record{}, err
			}
			continue
		}
		if fr.Final {
			return Record{}, f.end()
		}
	}
}

// catchUp extends the catalogue to the source's current frontier.
func (f *Follower) catchUp() error {
	fr := f.src.Frontier()
	if fr.Generation > f.r.chain.generation {
		if err := f.r.extendTo(fr.Generation); err != nil {
			return err
		}
	}
	f.frontier = fr
	return nil
}

// Seek positions the follower at a checkpointed cursor, first catching up
// with the announced frontier. A cursor naming a generation beyond it is
// stale: the writer that announced it is not this one, or it was never
// acknowledged.
func (f *Follower) Seek(c Cursor) error {
	if err := f.catchUp(); err != nil {
		return err
	}
	return f.r.SeekCursor(c)
}

// SeekSequence positions the follower on the record covering sequence,
// which must already be committed: ErrNotCommitted otherwise.
func (f *Follower) SeekSequence(sequence uint64) error {
	if err := f.catchUp(); err != nil {
		return err
	}
	if sequence >= f.r.chain.endSequence {
		return &notCommittedError{detail: fmt.Sprintf("sequence %d is at or beyond the committed end %d", sequence, f.r.chain.endSequence)}
	}
	return f.r.SeekSequence(sequence)
}

// SeekTime positions the follower for a capture time within the committed
// prefix, as Reader.SeekTime; past its end, it lands at the end, where Next
// waits for more.
func (f *Follower) SeekTime(epoch uint32, unixNanos int64) error {
	if err := f.catchUp(); err != nil {
		return err
	}
	return f.r.SeekTime(epoch, unixNanos)
}

// end is the result of reading past the last committed record of a stream
// that will not grow.
func (f *Follower) end() error {
	c := f.r.chain
	switch {
	case c.closed:
		return io.EOF
	case c.failed:
		m := f.r.status.Failure
		return &CaptureFailedError{Cause: m.Cause, Err: errors.New(m.Detail), CommittedEndSequence: c.endSequence,
			CommittedRecords: c.records, AcceptedEndSequence: m.AcceptedEndSequence, AcceptedRecords: m.AcceptedRecords, MarkerWritten: true}
	case f.frontier.Failure != nil:
		failure := *f.frontier.Failure
		return &failure
	}
	return fmt.Errorf("%w: the capture ended at generation %d without a terminal generation; recovery will mark it incomplete",
		io.ErrUnexpectedEOF, c.generation)
}
