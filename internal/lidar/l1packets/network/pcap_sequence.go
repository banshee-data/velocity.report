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
		if cfg.OnProgress != nil {
			base := completed
			onProgress = func(current, _ uint64) {
				cfg.OnProgress(base+current, total)
			}
		}

		if err := stepReader(ctx, step.Path, cfg.UDPPort, cfg.Parser, cfg.FrameBuilder,
			cfg.Stats, cfg.Forwarder, step.StartSecs, step.DurationSecs, 0,
			step.PacketCount, onProgress); err != nil {
			return result, fmt.Errorf("network: replaying step %d of %d (%s): %w",
				i+1, len(steps), step.Path, err)
		}

		completed += step.PacketCount
		result.StepsCompleted++
	}

	diagf("PCAP sequence complete: %d files, %d straddling revolution(s) dropped",
		result.StepsCompleted, result.FramesDropped)
	return result, nil
}
