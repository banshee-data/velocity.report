//go:build pcap
// +build pcap

package pcapsplit

import (
	"context"
	"fmt"
	"sync"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// SettledBackground replays cfg's window (StartSeconds and DurationSeconds,
// measured from the first packet of the first capture) into a fresh L3 model
// and returns the cells that settled, as sensor-frame points.
//
// It is the background an operator aligns against a map: the static scene of
// one place, without the vehicles that passed through it. It writes nothing.
func SettledBackground(ctx context.Context, cfg SplitConfig) (*Background, error) {
	cfg.DurationSeconds = normaliseReplayDuration(cfg.DurationSeconds)
	parserCfg, err := loadAnalysisPandarConfig()
	if err != nil {
		return nil, fmt.Errorf("load parser config: %w", err)
	}
	parser := parse.NewPandar40PParser(*parserCfg)
	elevations := parse.ElevationsFromConfig(parserCfg)
	bg, threshold, err := newBackgroundModel(cfg.SensorID, cfg.PCAPFile, cfg.Tuning, elevations)
	if err != nil {
		return nil, err
	}

	out := &Background{SettledThreshold: threshold}
	var mu sync.Mutex
	fb := l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{
		SensorID: cfg.SensorID,
		FrameCallback: func(frame *l2frames.LiDARFrame) {
			if frame == nil || len(frame.PolarPoints) == 0 {
				return
			}
			bg.ProcessFramePolarWithMaskAt(frame.PolarPoints, frame.StartTimestamp)
			mu.Lock()
			if out.Frames == 0 {
				out.FirstTime = frame.StartTimestamp
			}
			out.LastTime = frame.StartTimestamp
			out.Frames++
			mu.Unlock()
		},
		FrameChCapacity: 32,
	})
	fb.SetBlockOnFrameChannel(true)
	if err := replayForAnalysis(ctx, cfg, parser, fb, &countingStats{}); err != nil {
		fb.Close()
		return nil, err
	}
	fb.Close()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	mu.Lock()
	defer mu.Unlock()
	if out.Frames == 0 {
		return nil, fmt.Errorf("no complete rotation in the window")
	}
	out.Points = settledPoints(bg, elevations, threshold)
	return out, nil
}
