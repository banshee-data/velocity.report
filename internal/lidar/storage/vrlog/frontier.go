package vrlog

import (
	"context"
	"math"
	"time"
)

// CaptureState is where a capture stands. The writer reports it live; a
// reader derives it from the terminal generation the chain ends with.
type CaptureState int

const (
	// CaptureOpen: no terminal generation. From a writer, it is capturing;
	// from a reader, it is being written or was interrupted, and until
	// recovery marks it, how much was accepted after the committed end is
	// unknown.
	CaptureOpen CaptureState = iota
	// CaptureClosed: a close generation committed the closing summary.
	CaptureClosed
	// CaptureFailed: admission stopped on a capture failure.
	CaptureFailed
	// CaptureIncomplete: recovery ended an interrupted capture.
	CaptureIncomplete
)

func (s CaptureState) String() string {
	switch s {
	case CaptureOpen:
		return "open"
	case CaptureClosed:
		return "closed"
	case CaptureFailed:
		return "failed"
	case CaptureIncomplete:
		return "incomplete"
	}
	return "unknown"
}

// Frontier is the writer's durable frontier: the last generation whose
// publication completed every step, including the final directory sync, and
// what it commits. It is announced only after that. Accepted fields count
// what the writer has admitted, durable or not; the difference is at risk.
type Frontier struct {
	Generation  uint64
	Chunks      uint64
	Records     uint64
	Frames      uint64
	Gaps        uint64
	EndSequence uint64
	ChunkBytes  uint64
	State       CaptureState
	// Failure is set when State is CaptureFailed, possibly before the
	// failure marker is durable, or when the marker could not be written.
	Failure             *CaptureFailedError
	AcceptedRecords     uint64
	AcceptedEndSequence uint64
	// Final: the writer will announce no further generation. A failed
	// writer is final only once its committer has stopped, since a commit
	// in flight when the failure was declared may still complete.
	Final bool
}

// FrontierSource announces a durable frontier: a live Writer, or anything
// relaying one (a capture service's local API). A Follower advances only
// through generations a source has announced.
type FrontierSource interface {
	Frontier() Frontier
	// WaitFrontier blocks until the announced generation exceeds after, the
	// frontier is final, or ctx ends. It never blocks the writer.
	WaitFrontier(ctx context.Context, after uint64) (Frontier, error)
}

// latencyHistogram records durations in logarithmic buckets eight to a
// doubling, so adjacent bucket bounds are within 10% of each other, over 1 ns
// to about 36 hours: bounded memory however long the capture, and quantiles
// accurate to a bucket.
type latencyHistogram struct {
	buckets [latencyBuckets]uint64
	count   uint64
	max     time.Duration
}

const latencyBuckets = 8 * 48

func latencyBucket(d time.Duration) int {
	if d <= 1 {
		return 0
	}
	return min(int(math.Log2(float64(d))*8), latencyBuckets-1)
}

func (h *latencyHistogram) add(d time.Duration) {
	h.buckets[latencyBucket(d)]++
	h.count++
	h.max = max(h.max, d)
}

// quantile returns the upper bound of the bucket holding rank q, capped at
// the exact maximum.
func (h *latencyHistogram) quantile(q float64) time.Duration {
	if h.count == 0 {
		return 0
	}
	rank := uint64(math.Ceil(q * float64(h.count)))
	var seen uint64
	for i, n := range h.buckets {
		if seen += n; seen >= rank && n > 0 {
			return min(time.Duration(math.Exp2(float64(i+1)/8)), h.max)
		}
	}
	return h.max
}

// Latency summarises a latency distribution.
type Latency struct {
	Count              uint64
	P50, P95, P99, Max time.Duration
}

func (h *latencyHistogram) summary() Latency {
	return Latency{Count: h.count, P50: h.quantile(0.50), P95: h.quantile(0.95), P99: h.quantile(0.99), Max: h.max}
}

// WriterStats is what the writer measured. Bytes count everything it wrote,
// so ObjectBytes/PayloadBytes is its logical write amplification; filesystem
// block rounding and journalling come on top.
type WriterStats struct {
	// Publish is steps 2 to 4 of each generation's publication; Sync is
	// step 2 alone (the chunk, index and object synchronisation).
	Publish, Sync Latency
	// BatchAge is the age of each batch's first record when it closed.
	BatchAge Latency
	// PayloadBytes are the frame and gap payloads admitted.
	PayloadBytes uint64
	// ObjectBytes are every byte written, by object kind: chunk, index,
	// generation, current, summary, manifest, reserve.
	ObjectBytes map[string]uint64
	// Files is the number of objects created (temporary names included once).
	Files uint64
	Syncs uint64
}
