//go:build pcap
// +build pcap

package server

import (
	"context"
	"fmt"
	"time"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/pcapsplit"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// sessionMotionPass classifies a session's captures, joined into one stream,
// and returns its motion/static timeline.
//
// It is the same engine as `velocity lidar pcap-split --dry-run`: one
// classification, one set of thresholds, one answer whether an operator asks
// from the command line or the web UI.
//
// extents are the captures as the index probed them, or nil. Given, the pass
// joins the captures without reading each one to count it first, which halves
// what it reads, and it can say which capture it has reached. It reports as it
// reads and stops when ctx ends.
func sessionMotionPass(ctx context.Context, paths []string, extents []capseq.Segment, udpPort int,
	tuning *cfgpkg.TuningConfig, report func(current, total int64, detail string)) ([]sqlite.MotionPeriod, error) {

	if len(paths) == 0 {
		return nil, fmt.Errorf("no captures to classify")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cfg := pcapsplit.DefaultSplitConfig()
	cfg.PCAPFile = paths[0]
	if len(paths) > 1 {
		cfg.PCAPFiles = paths
	}
	cfg.Extents = extents
	cfg.UDPPort = udpPort
	cfg.Tuning = tuning
	if tuning != nil && cfg.SensorID == "" {
		cfg.SensorID = tuning.GetSensor()
	}
	cfg.DryRun = true

	if report != nil {
		report(0, int64(totalPackets(extents)), fmt.Sprintf("classifying %d captures", len(paths)))
		send, stop := newestProgress(func(p capjobs.Progress) { report(p.Current, p.Total, p.Detail) })
		defer stop()
		progress := newMotionProgress(extents, len(paths), motionProgressInterval, time.Now)
		cfg.OnProgress = func(current, total uint64) {
			if p, ok := progress.observe(current, total); ok {
				send(p)
			}
		}
	}

	analysis, err := pcapsplit.AnalyseContext(ctx, cfg)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("motion pass: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	periods := pcapsplit.BuildTimeline(analysis.Samples, cfg.TimelineConfig())
	return toStorePeriods(periods), nil
}

// toStorePeriods converts the classifier's periods into stored rows, labelling
// each the way pcap-split names its segment files so an operator reading the
// UI and an operator reading the CLI are looking at the same thing.
func toStorePeriods(periods []pcapsplit.MotionPeriod) []sqlite.MotionPeriod {
	out := make([]sqlite.MotionPeriod, 0, len(periods))
	counts := map[string]int{}
	for i, p := range periods {
		label := fmt.Sprintf("%s-%d", p.Type, counts[p.Type])
		counts[p.Type]++

		startFrame, endFrame := p.StartFrame, p.EndFrame
		out = append(out, sqlite.MotionPeriod{
			Ordinal:    i,
			Type:       p.Type,
			Label:      label,
			StartNs:    p.StartTime.UnixNano(),
			EndNs:      p.EndTime.UnixNano(),
			DurationNs: p.EndTime.Sub(p.StartTime).Nanoseconds(),
			StartSecs:  p.StartSecs,
			EndSecs:    p.EndSecs,
			StartFrame: &startFrame,
			EndFrame:   &endFrame,
		})
	}
	return out
}
