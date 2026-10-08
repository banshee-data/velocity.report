//go:build pcap
// +build pcap

package pcapsplit

import (
	"context"
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
)

// replayForAnalysis feeds the analysis pass, from one capture or from several
// joined into a continuous stream.
//
// Joining matters more than it looks. A per-file analysis restarts the model at
// every capture boundary and can manufacture a transition while it settles.
// Replaying a sequence keeps its motion timeline continuous across those joins.
// MotionClassifier refreshes its baseline on capture time, independently of the
// file boundaries, so it can recognise a stop after a long drive.
func replayForAnalysis(ctx context.Context, cfg SplitConfig, parser network.Parser,
	frameBuilder network.FrameBuilder, stats network.PacketStatsInterface) error {

	files := cfg.PCAPFiles
	if len(files) <= 1 {
		var total uint64
		if extents, ok := knownExtents(cfg, []string{cfg.PCAPFile}); ok {
			total = extents[0].PacketCount
		}
		if err := network.ReadPCAPFile(
			ctx, cfg.PCAPFile, cfg.UDPPort,
			parser, frameBuilder, stats, nil,
			cfg.StartSeconds, cfg.DurationSeconds, 0, total, cfg.OnProgress,
		); err != nil {
			return fmt.Errorf("pcap replay: %w", err)
		}
		return nil
	}

	seq, err := sequenceForAnalysis(cfg, files)
	if err != nil {
		return err
	}
	steps, err := seq.Plan(cfg.StartSeconds, normaliseReplayDuration(cfg.DurationSeconds))
	if err != nil {
		return fmt.Errorf("planning the replay window: %w", err)
	}

	if _, err := network.ReadPCAPSequence(ctx, steps,
		network.SequenceReplayConfig{
			UDPPort:      cfg.UDPPort,
			Parser:       parser,
			FrameBuilder: frameBuilder,
			Stats:        stats,
			OnProgress:   cfg.OnProgress,
		}); err != nil {
		return fmt.Errorf("pcap sequence replay: %w", err)
	}
	return nil
}

// sequenceForAnalysis joins the captures from the extents the caller gave, or
// counts every capture to find them when it gave none that fit.
func sequenceForAnalysis(cfg SplitConfig, files []string) (*capseq.Sequence, error) {
	if extents, ok := knownExtents(cfg, files); ok {
		return sequenceOf(extents)
	}
	return BuildSequence(files, cfg.UDPPort)
}

// knownExtents returns cfg.Extents when they describe files, in order, each
// with a packet count and both packet times; otherwise it reports false, and
// the files are counted instead. A partial or reordered set is never trusted.
func knownExtents(cfg SplitConfig, files []string) ([]capseq.Segment, bool) {
	if len(cfg.Extents) != len(files) || len(files) == 0 {
		return nil, false
	}
	for i, e := range cfg.Extents {
		if e.Path != files[i] || e.PacketCount == 0 || e.FirstPacket.IsZero() || e.LastPacket.IsZero() {
			return nil, false
		}
	}
	return cfg.Extents, true
}

// BuildSequence probes each capture's packet extent and grades the joins
// between them, refusing a set that is not one continuous recording.
func BuildSequence(files []string, udpPort int) (*capseq.Sequence, error) {
	segments := make([]capseq.Segment, 0, len(files))
	for _, path := range files {
		count, err := network.CountPCAPPackets(path, udpPort)
		if err != nil {
			return nil, fmt.Errorf("probing %s: %w", path, err)
		}
		if count.Count == 0 {
			return nil, fmt.Errorf("%s contains no packets on UDP port %d", path, udpPort)
		}
		segments = append(segments, capseq.Segment{
			Path:        path,
			FirstPacket: time.Unix(0, count.FirstTimestampNs),
			LastPacket:  time.Unix(0, count.LastTimestampNs),
			PacketCount: count.Count,
		})
	}

	return sequenceOf(segments)
}

// sequenceOf grades the joins between captures and refuses a set that is not
// one continuous recording.
func sequenceOf(segments []capseq.Segment) (*capseq.Sequence, error) {
	seq, err := capseq.Build(segments, capseq.DefaultTolerances())
	if err != nil {
		return nil, fmt.Errorf("sequencing captures: %w", err)
	}
	if !seq.Continuous() {
		broken := seq.BrokenSeams()[0]
		return nil, fmt.Errorf(
			"captures do not form one continuous stream: the join %s → %s is %s (gap %v)",
			broken.Before, broken.After, broken.Grade, broken.Gap)
	}
	return seq, nil
}
