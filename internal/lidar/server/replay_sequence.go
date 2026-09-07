package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
)

// replayPlan is a validated multi-file replay: the per-file read steps plus the
// aggregate figures the progress and timeline surfaces report.
type replayPlan struct {
	// Steps are executed in order against one shared parser and frame builder.
	Steps []capseq.ReadStep
	// TotalPackets is the sum across every file, so one progress bar spans the
	// whole replay rather than restarting per file.
	TotalPackets uint64
	// FirstTimestampNs and LastTimestampNs bound the sequence, standing in for
	// the single-file pre-count's extents.
	FirstTimestampNs int64
	LastTimestampNs  int64
	// JoinsCrossed is the number of file joins the replay traverses, and
	// FrameDrops how many of those are non-seamless and therefore cost a
	// revolution.
	JoinsCrossed int
	FrameDrops   int
	// Lost is the total packet-time unaccounted for at the joins.
	Lost time.Duration
}

// buildReplaySequence resolves an ordered list of capture files, probes each
// one's packet extent, grades the joins between them, and plans the read steps
// for the requested window.
//
// The window is relative to the start of the whole sequence, not to any one
// file: a replay case describes a stretch of a site visit, and where the
// capture tool happened to roll its output is not part of that description.
//
// Planning happens before replay starts so a join too wide to cross is
// reported to the caller as a 400 rather than discovered mid-replay, and so no
// unioned intermediate file has to be written to find out.
func (ws *Server) buildReplaySequence(files []string, startSecs, durationSecs float64) (*replayPlan, error) {
	if len(files) == 0 {
		return nil, &switchError{status: http.StatusBadRequest,
			err: fmt.Errorf("no capture files given for replay")}
	}

	segments := make([]capseq.Segment, 0, len(files))
	for _, file := range files {
		resolved, err := ws.resolvePCAPPath(file)
		if err != nil {
			return nil, err
		}
		count, err := countPCAPPackets(resolved, ws.udpPort)
		if err != nil {
			return nil, &switchError{status: http.StatusBadRequest,
				err: fmt.Errorf("probing %s: %w", file, err)}
		}
		// A file with nothing on the LiDAR port has no extent to sequence, and
		// capseq would reject it with a message about unset packet times that
		// says nothing about the actual problem.
		if count.Count == 0 {
			return nil, &switchError{status: http.StatusBadRequest, err: fmt.Errorf(
				"%s contains no packets on UDP port %d", file, ws.udpPort)}
		}
		segments = append(segments, capseq.Segment{
			Path:        resolved,
			FirstPacket: time.Unix(0, count.FirstTimestampNs),
			LastPacket:  time.Unix(0, count.LastTimestampNs),
			PacketCount: count.Count,
		})
	}

	seq, err := capseq.Build(segments, capseq.DefaultTolerances())
	if err != nil {
		return nil, &switchError{status: http.StatusBadRequest,
			err: fmt.Errorf("sequencing capture files: %w", err)}
	}
	if !seq.Continuous() {
		broken := seq.BrokenSeams()[0]
		return nil, &switchError{status: http.StatusBadRequest, err: fmt.Errorf(
			"capture files do not form one continuous stream: the join %s → %s is %s (gap %v); "+
				"%d of %d joins are unusable",
			broken.Before, broken.After, broken.Grade, broken.Gap,
			len(seq.BrokenSeams()), len(seq.Seams))}
	}

	steps, err := seq.Plan(startSecs, durationSecs)
	if err != nil {
		return nil, &switchError{status: http.StatusBadRequest,
			err: fmt.Errorf("planning replay window: %w", err)}
	}

	plan := &replayPlan{
		Steps:            steps,
		FirstTimestampNs: seq.Start.UnixNano(),
		LastTimestampNs:  seq.End.UnixNano(),
		JoinsCrossed:     len(steps) - 1,
		Lost:             seq.Lost,
	}
	for _, step := range steps {
		plan.TotalPackets += step.PacketCount
		if step.DropFrameAtStart {
			plan.FrameDrops++
		}
	}
	return plan, nil
}
