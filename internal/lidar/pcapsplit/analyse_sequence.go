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
// Joining matters more than it looks. The background model needs tens of
// seconds to settle, and a per-file analysis restarts it at every boundary, so
// the head of each file reports as motion whether or not the sensor moved. Over
// a rolling capture that manufactures a motion period every five minutes and
// cuts genuinely static stretches into pieces. Replaying the files as one
// stream carries the model across the joins and the artefacts go away.
func replayForAnalysis(cfg SplitConfig, parser network.Parser,
	frameBuilder network.FrameBuilder, stats network.PacketStatsInterface) error {

	files := cfg.PCAPFiles
	if len(files) <= 1 {
		if err := network.ReadPCAPFile(
			context.Background(), cfg.PCAPFile, cfg.UDPPort,
			parser, frameBuilder, stats, nil,
			cfg.StartSeconds, cfg.DurationSeconds, 0, 0, nil,
		); err != nil {
			return fmt.Errorf("pcap replay: %w", err)
		}
		return nil
	}

	seq, err := BuildSequence(files, cfg.UDPPort)
	if err != nil {
		return err
	}
	steps, err := seq.Plan(cfg.StartSeconds, normaliseReplayDuration(cfg.DurationSeconds))
	if err != nil {
		return fmt.Errorf("planning the replay window: %w", err)
	}

	if _, err := network.ReadPCAPSequence(context.Background(), steps,
		network.SequenceReplayConfig{
			UDPPort:      cfg.UDPPort,
			Parser:       parser,
			FrameBuilder: frameBuilder,
			Stats:        stats,
		}); err != nil {
		return fmt.Errorf("pcap sequence replay: %w", err)
	}
	return nil
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
