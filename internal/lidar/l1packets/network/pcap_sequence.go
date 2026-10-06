package network

import (
	"context"
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
)

// SeamAwareFrameBuilder is implemented by frame builders that can discard the
// revolution in flight when replay crosses a capture-file join. The L2
// FrameBuilder satisfies it; a caller supplying a simpler builder loses the
// drop but still replays the sequence.
type SeamAwareFrameBuilder interface {
	// DropNextFrame marks the revolution in flight for discard and reports
	// whether there was one to mark.
	DropNextFrame() bool
}

// SequenceReplayConfig carries the state shared across every file in a
// multi-file replay. The parser and frame builder are deliberately shared: that
// is what lets pipeline state — background model, motor speed, azimuth
// continuity — carry across a join while the packet source changes underneath,
// and it is why no unioned intermediate file is needed.
type SequenceReplayConfig struct {
	// UDPPort selects the LiDAR data port; every file in the sequence is
	// filtered on it.
	UDPPort int
	// Parser and FrameBuilder are shared across all steps. Either may be nil,
	// matching ReadPCAPFile's behaviour.
	Parser       Parser
	FrameBuilder FrameBuilder
	// Stats and Forwarder are optional and also shared.
	Stats     PacketStatsInterface
	Forwarder *PacketForwarder
	// OnProgress receives (current, total) aggregated across the whole
	// sequence rather than per file, so a caller can drive one progress bar.
	// total is zero when any step's packet count is unknown.
	OnProgress func(current, total uint64)

	// Paced carries everything a paced replay needs — speed, sensor, the
	// foreground forwarder, background manager and debug ranges — as the
	// template each step is read with.
	//
	// Its SpeedMultiplier decides whether the sequence is paced at all: zero
	// reads as fast as the pipeline accepts packets, which is what an analysis
	// run wants. Above zero, every step is paced against one shared clock so a
	// join does not reset the pacer — see PacingAnchor.
	//
	// The per-step fields are owned by the sequence and overwritten:
	// StartSeconds, DurationSeconds, TotalPackets, OnProgress and PacingAnchor.
	Paced RealtimeReplayConfig
}

// SequenceResult reports what a sequence replay did.
type SequenceResult struct {
	// StepsCompleted is how many files were read to completion.
	StepsCompleted int
	// FramesDropped is the number of revolutions discarded at joins. It should
	// equal the number of steps carrying DropFrameAtStart, and is zero when the
	// frame builder does not implement SeamAwareFrameBuilder.
	FramesDropped int
}

// stepReader reads one file of a sequence. It is indirected through a variable
// so the sequencing logic — join drops, progress aggregation, cancellation,
// error wrapping — can be exercised without libpcap, which the real
// ReadPCAPFile requires. Production code never reassigns it.
var stepReader = ReadPCAPFile

// stepReaderRealtime reads one paced step. Indirected for the same reason as
// stepReader: the sequencing logic is testable without libpcap.
var stepReaderRealtime = ReadPCAPFileRealtime

// ReadPCAPSequence replays an ordered list of capture files as one continuous
// packet stream.
//
// Each step is read by ReadPCAPFile against the shared parser and frame
// builder, so the pipeline sees a single uninterrupted capture. Before a step
// whose DropFrameAtStart is set — that is, one continuing across a join that
// was continuous enough to cross but not seamless — the revolution in flight is
// marked for discard, because assembling one frame from points either side of
// the gap would present L3 with a rotation that never happened.
//
// Steps normally come from capseq.Sequence.Plan, which refuses to produce them
// for a sequence containing a join too wide to cross.
func ReadPCAPSequence(ctx context.Context, steps []capseq.ReadStep, cfg SequenceReplayConfig) (SequenceResult, error) {
	var result SequenceResult
	if len(steps) == 0 {
		return result, fmt.Errorf("network: empty replay sequence")
	}

	// Aggregate the whole sequence into one progress scale. A step whose packet
	// count was never indexed makes the total unknowable, and a zero total is
	// what existing callers already read as "unknown".
	var total uint64
	for _, step := range steps {
		if step.PacketCount == 0 {
			total = 0
			break
		}
		total += step.PacketCount
	}

	seamAware, _ := cfg.FrameBuilder.(SeamAwareFrameBuilder)

	// One anchor for the whole sequence. The first step fixes the origin; the
	// rest measure against it, so elapsed capture time and elapsed wall time
	// both run continuously through every join.
	var anchor *PacingAnchor
	if cfg.Paced.SpeedMultiplier > 0 {
		anchor = &PacingAnchor{}
	}

	var completed uint64
	for i, step := range steps {
		// Honour cancellation between steps as well as inside them, so a stop
		// request between two large files is not held until the next file's
		// first packet.
		if err := ctx.Err(); err != nil {
			return result, err
		}

		if step.DropFrameAtStart {
			if seamAware != nil && seamAware.DropNextFrame() {
				result.FramesDropped++
				diagf("PCAP sequence: dropping the revolution straddling the join into %s", step.Path)
			} else if seamAware == nil {
				opsf("PCAP sequence: frame builder cannot drop the revolution straddling the join into %s; "+
					"that rotation will combine points from either side of the gap", step.Path)
			}
		}

		var onProgress func(current, total uint64)
		var stepProgress uint64
		if cfg.OnProgress != nil {
			base := completed
			onProgress = func(current, _ uint64) {
				stepProgress = current
				cfg.OnProgress(base+current, total)
			}
		}

		var stepErr error
		if anchor != nil {
			// Start from the caller's template so a paced sequence keeps the
			// forwarders, background manager and debug ranges a paced single
			// file would have had, then set what belongs to this step.
			paced := cfg.Paced
			paced.StartSeconds = step.StartSecs
			paced.DurationSeconds = step.DurationSecs
			paced.TotalPackets = step.PacketCount
			paced.OnProgress = onProgress
			paced.PacingAnchor = anchor
			if paced.PacketForwarder == nil {
				paced.PacketForwarder = cfg.Forwarder
			}
			stepErr = stepReaderRealtime(ctx, step.Path, cfg.UDPPort, cfg.Parser,
				cfg.FrameBuilder, cfg.Stats, paced)
		} else {
			stepErr = stepReader(ctx, step.Path, cfg.UDPPort, cfg.Parser, cfg.FrameBuilder,
				cfg.Stats, cfg.Forwarder, step.StartSecs, step.DurationSecs, 0,
				step.PacketCount, onProgress)
		}
		if stepErr != nil {
			return result, fmt.Errorf("network: replaying step %d of %d (%s): %w",
				i+1, len(steps), step.Path, stepErr)
		}

		if total == 0 {
			// Once any step is unindexed, the sequence total is unknown. Carry
			// the progress actually observed in each completed step so later
			// callbacks remain monotonic even when PacketCount is zero.
			completed += stepProgress
		} else {
			completed += step.PacketCount
		}
		result.StepsCompleted++
	}

	diagf("PCAP sequence complete: %d files, %d straddling revolution(s) dropped",
		result.StepsCompleted, result.FramesDropped)
	return result, nil
}
