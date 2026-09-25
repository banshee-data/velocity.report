package vrlog

import (
	"fmt"
	"math"
	"time"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// Provisional commit-policy values (VRLOG plan §4.2). The group-commit
// interval and strict-mode default are an open choice to be settled from
// target-hardware latency, energy and loss measurements; these numbers only
// make the policy concrete and testable. None is a power-loss guarantee.
const (
	DefaultMaxBatchAge    = 100 * time.Millisecond
	DefaultCommitDeadline = 2 * time.Second
	// DefaultMaxFrameRateHz is a Pandar40P's fastest rotation.
	DefaultMaxFrameRateHz      = 20
	DefaultFailureReserveBytes = 64 << 10
	// maxCommitDeadline bounds a declared stall bound: a longer one would
	// make the crash-loss interval meaningless on any hardware.
	maxCommitDeadline = 10 * time.Minute
	maxReserveBytes   = 16 << 20
)

// CommitPolicy is how the writer turns accepted evidence into durable
// evidence. An append that returns nil means accepted; the durable frontier
// says what is committed. The policy is declared in the root manifest with
// the crash-loss bound it implies, before any record is admitted.
type CommitPolicy struct {
	// MaxBatchAge closes the open batch once its first record is this old.
	MaxBatchAge time.Duration
	// MaxBatchBytes closes it once it holds this many bytes; zero is the
	// target chunk size, the byte threshold of plan §4.1. A batch is a chunk:
	// every generation seals and commits whole chunks.
	MaxBatchBytes uint64
	// Strict makes every append wait until its record is durable.
	Strict bool
	// CommitDeadline is the stall bound: an accepted record not durable
	// within MaxBatchAge + CommitDeadline (CommitDeadline alone when strict)
	// puts the capture into failure, and admission stops.
	CommitDeadline time.Duration
	// UpstreamQueueAge is the caller's bound on how long a frame waits before
	// it reaches the writer. It is declared, not enforced here: the bounded
	// ingress queue belongs to the caller.
	UpstreamQueueAge time.Duration
	// ShedAfter is how long an append may wait for room in the open batch
	// before the frame is recorded as an explicit gap instead. Zero never
	// sheds: the append waits, which backpressures a paused PCAP reader, and
	// fails at the commit deadline. A live source that cannot be paused sets
	// it so a stalled disk costs visible gaps rather than a blocked callback.
	ShedAfter time.Duration
	// FailureReserveBytes are preallocated at Create and released to write a
	// failure marker. Zero is the default; there is always a reserve.
	FailureReserveBytes uint64
	// MaxFrameRateHz states the frame bound of the crash-loss interval.
	MaxFrameRateHz float64
}

// DefaultCommitPolicy returns the provisional group-commit policy.
func DefaultCommitPolicy() CommitPolicy {
	return CommitPolicy{
		MaxBatchAge:         DefaultMaxBatchAge,
		CommitDeadline:      DefaultCommitDeadline,
		MaxFrameRateHz:      DefaultMaxFrameRateHz,
		FailureReserveBytes: DefaultFailureReserveBytes,
	}
}

// withDefaults fills zero fields from the defaults and the limits.
func (p CommitPolicy) withDefaults(l Limits) CommitPolicy {
	d := DefaultCommitPolicy()
	if p.MaxBatchAge == 0 && !p.Strict {
		p.MaxBatchAge = d.MaxBatchAge
	}
	if p.MaxBatchBytes == 0 {
		p.MaxBatchBytes = l.TargetChunkBytes
	}
	if p.CommitDeadline == 0 {
		p.CommitDeadline = d.CommitDeadline
	}
	if p.MaxFrameRateHz == 0 {
		p.MaxFrameRateHz = d.MaxFrameRateHz
	}
	if p.FailureReserveBytes == 0 {
		p.FailureReserveBytes = d.FailureReserveBytes
	}
	return p
}

func (p CommitPolicy) validate(l Limits) error {
	switch {
	case p.MaxBatchAge < 0 || (p.MaxBatchAge == 0 && !p.Strict):
		return fmt.Errorf("batch age %s must be positive unless strict", p.MaxBatchAge)
	case p.MaxBatchBytes == 0 || p.MaxBatchBytes > l.TargetChunkBytes:
		return fmt.Errorf("batch bytes %d outside (0, target chunk %d]", p.MaxBatchBytes, l.TargetChunkBytes)
	case p.CommitDeadline <= 0 || p.CommitDeadline > maxCommitDeadline:
		return fmt.Errorf("commit deadline %s outside (0, %s]", p.CommitDeadline, maxCommitDeadline)
	case p.UpstreamQueueAge < 0 || p.ShedAfter < 0:
		return fmt.Errorf("negative upstream queue age or shed wait")
	case p.ShedAfter >= p.CommitDeadline:
		return fmt.Errorf("shed wait %s must be shorter than the commit deadline %s", p.ShedAfter, p.CommitDeadline)
	case p.FailureReserveBytes == 0 || p.FailureReserveBytes > maxReserveBytes:
		return fmt.Errorf("failure reserve %d outside (0, %d]", p.FailureReserveBytes, maxReserveBytes)
	case !(p.MaxFrameRateHz > 0) || p.MaxFrameRateHz > 10_000 || math.IsInf(p.MaxFrameRateHz, 0):
		return fmt.Errorf("frame rate %g outside (0, 10000]", p.MaxFrameRateHz)
	}
	return nil
}

// batchAge is the age at which a batch closes: zero in strict mode, where
// every record closes its own batch.
func (p CommitPolicy) batchAge() time.Duration {
	if p.Strict {
		return 0
	}
	return p.MaxBatchAge
}

// stallBound is the longest an accepted record may stay undurable before the
// capture fails.
func (p CommitPolicy) stallBound() time.Duration { return p.batchAge() + p.CommitDeadline }

// CrashLoss is the declared crash-loss bound: evidence accepted within
// Interval of a process crash may be lost; at most Bytes of chunk data and
// about Frames frames. It covers a killed process. What a power loss loses
// also depends on whether the filesystem and device honour fsync, which
// only target-hardware tests can show.
type CrashLoss struct {
	Interval time.Duration
	Bytes    uint64
	Frames   uint64
}

// CrashLoss derives the bound from the policy and limits. The writer holds
// at most two undurable batches, the one committing and the open one, each
// closed at MaxBatchBytes plus the record that crossed it.
func (p CommitPolicy) CrashLoss(l Limits) CrashLoss {
	interval := p.UpstreamQueueAge + p.stallBound()
	return CrashLoss{
		Interval: interval,
		Bytes:    2 * (p.MaxBatchBytes + uint64(l.MaxRecordBytes) + envelopeSize),
		Frames:   uint64(math.Ceil(interval.Seconds()*p.MaxFrameRateHz)) + 1,
	}
}

func (p CommitPolicy) toProto(l Limits) *pb.CommitPolicy {
	loss := p.CrashLoss(l)
	return &pb.CommitPolicy{
		MaxBatchAgeNanos: int64(p.MaxBatchAge), MaxBatchBytes: p.MaxBatchBytes, Strict: p.Strict,
		CommitDeadlineNanos: int64(p.CommitDeadline), UpstreamQueueAgeNanos: int64(p.UpstreamQueueAge),
		ShedAfterNanos: int64(p.ShedAfter), FailureReserveBytes: p.FailureReserveBytes, MaxFrameRateHz: p.MaxFrameRateHz,
		CrashLossIntervalNanos: int64(loss.Interval), CrashLossBytes: loss.Bytes, CrashLossFrames: loss.Frames,
	}
}

// commitPolicyFromProto maps a stored policy and requires its derived bound
// to be the one its fields imply: a manifest cannot claim a tighter bound
// than its policy gives.
func commitPolicyFromProto(m *pb.CommitPolicy, l Limits) (CommitPolicy, error) {
	if m == nil {
		return CommitPolicy{}, fmt.Errorf("the manifest requires commit generations but declares no commit policy")
	}
	p := CommitPolicy{
		MaxBatchAge: time.Duration(m.GetMaxBatchAgeNanos()), MaxBatchBytes: m.GetMaxBatchBytes(), Strict: m.GetStrict(),
		CommitDeadline: time.Duration(m.GetCommitDeadlineNanos()), UpstreamQueueAge: time.Duration(m.GetUpstreamQueueAgeNanos()),
		ShedAfter: time.Duration(m.GetShedAfterNanos()), FailureReserveBytes: m.GetFailureReserveBytes(), MaxFrameRateHz: m.GetMaxFrameRateHz(),
	}
	if err := p.validate(l); err != nil {
		return CommitPolicy{}, fmt.Errorf("commit policy: %w", err)
	}
	loss := p.CrashLoss(l)
	if time.Duration(m.GetCrashLossIntervalNanos()) != loss.Interval || m.GetCrashLossBytes() != loss.Bytes || m.GetCrashLossFrames() != loss.Frames {
		return CommitPolicy{}, fmt.Errorf("commit policy declares a crash-loss bound (%s, %d bytes, %d frames) its fields do not give (%s, %d, %d)",
			time.Duration(m.GetCrashLossIntervalNanos()), m.GetCrashLossBytes(), m.GetCrashLossFrames(), loss.Interval, loss.Bytes, loss.Frames)
	}
	return p, nil
}
