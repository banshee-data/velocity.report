//go:build pcap
// +build pcap

package pcapsplit

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
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
	parserCfg, err := parse.LoadPandar40PConfig()
	if err != nil {
		return nil, fmt.Errorf("load parser config: %w", err)
	}
	parser := parse.NewPandar40PParser(*parserCfg)
	elevations := parse.ElevationsFromConfig(parserCfg)

	classifierCfg := DefaultMotionClassifierConfig()
	classifierCfg.SettledThreshold = cfg.SettledThreshold
	classifier, err := NewMotionClassifier(cfg.SensorID, cfg.PCAPFile, classifierCfg)
	if err != nil {
		return nil, err
	}
	if err := classifier.SetRingElevations(elevations); err != nil {
		return nil, fmt.Errorf("set ring elevations: %w", err)
	}

	var (
		mu       sync.Mutex
		frames   []FrameMetrics
		samples  []FrameSample
		firstErr error
	)

	frameCallback := func(frame *l2frames.LiDARFrame) {
		if frame == nil || len(frame.PolarPoints) == 0 {
			return
		}
		evidence, err := classifier.Observe(frame.StartTimestamp, frame.PolarPoints)
		if err != nil {
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
			return
		}
		mu.Lock()
		frames = append(frames, FrameMetrics{
			FrameID:            len(frames),
			T:                  evidence.T,
			TotalPoints:        evidence.TotalPoints,
			ForegroundPoints:   evidence.ForegroundPoints,
			ForegroundFraction: evidence.ForegroundFraction,
			NonzeroCells:       evidence.NonzeroCells,
			SettledCells:       evidence.SettledCells,
			PercentSettled:     evidence.PercentSettled,
			DeviationFromNoise: evidence.DeviationFromNoise,
			WithinNoiseBounds:  evidence.WithinNoiseBounds,
			Stable:             evidence.Stable,
			Moving:             evidence.Moving,
		})
		samples = append(samples, FrameSample{T: evidence.T, Moving: evidence.Moving})
		mu.Unlock()
	}

	fb := l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{
		SensorID:        cfg.SensorID,
		FrameCallback:   frameCallback,
		FrameChCapacity: 32,
	})
	// Analysis must not drop completed rotations when the callback is slower
	// than PCAP decoding; Close flushes the final partial/buffered frames.
	fb.SetBlockOnFrameChannel(true)
	closed := false
	defer func() {
		if !closed {
			fb.Close()
		}
	}()

	stats := &countingStats{}
	reader := wrapProgress(cfg, stats, "scan")
	if err := network.ReadPCAPFile(
		context.Background(), cfg.PCAPFile, cfg.UDPPort,
		parser, fb, reader, nil,
		0, -1, 0, 0, nil,
	); err != nil {
		return nil, fmt.Errorf("pcap replay: %w", err)
	}

	// Drain the FrameBuilder so every frame callback completes.
	fb.Close()
	closed = true

	mu.Lock()
	defer mu.Unlock()
	if firstErr != nil {
		return nil, fmt.Errorf("frame processing failed: %w", firstErr)
	}
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

// wrapProgress decorates inner with a periodic progress reporter when
// cfg.ProgressSecs > 0; otherwise it returns inner unchanged. The reporter
// forwards every call, so inner still accumulates.
func wrapProgress(cfg SplitConfig, inner network.PacketStatsInterface, label string) network.PacketStatsInterface {
	if cfg.ProgressSecs <= 0 {
		return inner
	}
	var size int64
	if fi, err := os.Stat(cfg.PCAPFile); err == nil {
		size = fi.Size()
	}
	interval := time.Duration(cfg.ProgressSecs * float64(time.Second))
	return network.NewProgressStats(inner, size, interval, label, os.Stderr)
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
