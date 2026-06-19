//go:build pcap
// +build pcap

package pcapsplit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// gridRings and gridAzBins are the fixed Pandar40P background-grid dimensions,
// matching settling-eval and the rest of the pipeline.
const (
	gridRings  = 40
	gridAzBins = 1800
)

// Analysis is the result of the read/classify pass over a PCAP. Frames are the
// per-frame records for CSV export; Samples feed BuildTimeline.
type Analysis struct {
	Frames       []FrameMetrics
	Samples      []FrameSample
	TotalPackets int
	TotalFrames  int
	FirstTime    time.Time
	LastTime     time.Time
}

// Analyse replays the PCAP through a BackgroundManager and records, per frame,
// the raw motion signal and settling metrics. It performs no file writing; the
// timeline and segment writing are separate steps. This is pass 1 of the
// two-pass split (pass 2 is WriteSegments).
func Analyse(cfg SplitConfig) (*Analysis, error) {
	params, err := backgroundParams()
	if err != nil {
		return nil, err
	}

	parserCfg, err := parse.LoadPandar40PConfig()
	if err != nil {
		return nil, fmt.Errorf("load parser config: %w", err)
	}
	parser := parse.NewPandar40PParser(*parserCfg)
	elevations := parse.ElevationsFromConfig(parserCfg)

	bgMgr := l3grid.NewBackgroundManagerDI(cfg.SensorID, gridRings, gridAzBins, params, nil)
	if bgMgr == nil {
		return nil, fmt.Errorf("failed to create BackgroundManager")
	}
	if err := bgMgr.SetRingElevations(elevations); err != nil {
		return nil, fmt.Errorf("set ring elevations: %w", err)
	}
	bgMgr.SetSourcePath(cfg.PCAPFile)

	settledThreshold := cfg.SettledThreshold
	if settledThreshold == 0 {
		settledThreshold = DefaultSettledThreshold
	}

	var (
		mu      sync.Mutex
		frames  []FrameMetrics
		samples []FrameSample
	)

	frameCallback := func(frame *l2frames.LiDARFrame) {
		if frame == nil || len(frame.PolarPoints) == 0 {
			return
		}
		mask, err := bgMgr.ProcessFramePolarWithMask(frame.PolarPoints)
		if err != nil || mask == nil {
			return
		}
		fg := 0
		for _, isFg := range mask {
			if isFg {
				fg++
			}
		}
		moving := bgMgr.CheckForSensorMovement(mask)
		fsm := bgMgr.GetFrameSettlingMetrics(settledThreshold)
		dev := bgMgr.GetNoiseBoundsDeviation()
		t := frame.StartTimestamp

		mu.Lock()
		frames = append(frames, FrameMetrics{
			FrameID:            len(frames),
			T:                  t,
			TotalPoints:        len(frame.PolarPoints),
			ForegroundPoints:   fg,
			NonzeroCells:       fsm.NonzeroCells,
			SettledCells:       fsm.SettledCells,
			PercentSettled:     fsm.PercentSettled,
			DeviationFromNoise: dev,
			Moving:             moving,
		})
		samples = append(samples, FrameSample{T: t, Moving: moving})
		mu.Unlock()
	}

	fb := l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{
		SensorID:        cfg.SensorID,
		FrameCallback:   frameCallback,
		FrameChCapacity: 32,
	})
	closed := false
	defer func() {
		if !closed {
			fb.Close()
		}
	}()

	stats := &countingStats{}
	if err := network.ReadPCAPFile(
		context.Background(), cfg.PCAPFile, cfg.UDPPort,
		parser, fb, stats, nil,
		0, -1, 0, 0, nil,
	); err != nil {
		return nil, fmt.Errorf("pcap replay: %w", err)
	}

	// Drain the FrameBuilder so every frame callback completes.
	fb.Close()
	closed = true

	mu.Lock()
	defer mu.Unlock()
	a := &Analysis{
		Frames:       frames,
		Samples:      samples,
		TotalPackets: stats.count(),
		TotalFrames:  len(frames),
	}
	if len(samples) > 0 {
		a.FirstTime = samples[0].T
		a.LastTime = samples[len(samples)-1].T
	}
	return a, nil
}

// backgroundParams builds settling-friendly background parameters from the
// default tuning config, mirroring settling-eval: warmup gating is disabled so
// the model can settle from frame 0 during offline replay.
func backgroundParams() (l3grid.BackgroundParams, error) {
	tuningCfg := config.MustLoadDefaultConfig()
	bgConfig := l3grid.BackgroundConfigFromTuning(tuningCfg.L3.EmaBaselineV1, tuningCfg.L4.DbscanXyV1)
	bgConfig.WarmupMinFrames = 0
	bgConfig.WarmupDuration = 0
	bgConfig.SettlingPeriod = 24 * time.Hour
	if err := bgConfig.Validate(); err != nil {
		return l3grid.BackgroundParams{}, fmt.Errorf("invalid background config: %w", err)
	}
	return bgConfig.ToBackgroundParams(), nil
}

// countingStats is a minimal network.PacketStatsInterface that counts packets.
type countingStats struct {
	mu      sync.Mutex
	packets int
}

func (s *countingStats) AddPacket(int) {
	s.mu.Lock()
	s.packets++
	s.mu.Unlock()
}
func (s *countingStats) AddDropped()   {}
func (s *countingStats) AddPoints(int) {}
func (s *countingStats) LogStats(bool) {}
func (s *countingStats) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.packets
}
