//go:build pcap
// +build pcap

package pcapsplit

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

type analysisMotionClassifier interface {
	SetRingElevations([]float64) error
	Observe(time.Time, []l2frames.PointPolar) (MotionEvidence, error)
}

var (
	loadAnalysisPandarConfig = parse.LoadPandar40PConfig
	newAnalysisClassifier    = func(sensorID, sourcePath string, tuning *config.TuningConfig) (analysisMotionClassifier, error) {
		return NewMotionClassifier(sensorID, sourcePath, tuning)
	}
)

// Analysis is the result of the read/classify pass over a PCAP. Frames are the
// per-frame records for CSV export; Samples feed BuildTimeline; Capture is the
// scan-level health summary (frame rate, RPM, per-10s buckets).
type Analysis struct {
	Frames       []FrameMetrics
	Samples      []FrameSample
	TotalPackets int
	TotalFrames  int
	FirstTime    time.Time
	LastTime     time.Time
	Capture      CaptureStats
}

// Analyse replays the PCAP through a BackgroundManager and records, per frame,
// the raw motion signal and settling metrics. It performs no file writing; the
// timeline and segment writing are separate steps. This is pass 1 of the
// two-pass split (pass 2 is WriteSegments).
func Analyse(cfg SplitConfig) (*Analysis, error) {
	cfg.DurationSeconds = normaliseReplayDuration(cfg.DurationSeconds)
	parserCfg, err := loadAnalysisPandarConfig()
	if err != nil {
		return nil, fmt.Errorf("load parser config: %w", err)
	}
	parser := parse.NewPandar40PParser(*parserCfg)
	elevations := parse.ElevationsFromConfig(parserCfg)

	// Resolve the tuning once (nil = embedded defaults) without mutating a
	// caller-owned configuration.
	tuningCfg := tuningOrEmbedded(cfg.Tuning)
	classifier, err := newAnalysisClassifier(cfg.SensorID, cfg.PCAPFile, tuningCfg)
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
			mu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
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
			DriftRatio:         evidence.DriftRatio,
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
	// Wrap the frame builder so the scan pass captures motor RPM (reported per
	// packet via SetMotorSpeed); point assembly still flows to the real builder.
	sb := &statsBuilder{inner: fb}
	if err := network.ReadPCAPFile(
		context.Background(), cfg.PCAPFile, cfg.UDPPort,
		parser, sb, reader, nil,
		cfg.StartSeconds, cfg.DurationSeconds, 0, 0, nil,
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
	frameTimes := make([]time.Time, len(frames))
	var totalPoints, foregroundPoints int
	for i, f := range frames {
		frameTimes[i] = f.T
		totalPoints += f.TotalPoints
		foregroundPoints += f.ForegroundPoints
	}
	a.Capture = computeCaptureStats(cfg.PCAPFile, frameTimes, stats.count(), totalPoints, foregroundPoints, sb.snapshot())
	return a, nil
}

// normaliseReplayDuration preserves the historical zero-value full-capture
// behaviour while retaining -1 as the explicit full-capture setting.
func normaliseReplayDuration(durationSeconds float64) float64 {
	if durationSeconds == 0 {
		return -1
	}
	return durationSeconds
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

// statsBuilder wraps the frame builder to capture motor RPM, which the network
// layer reports per packet via SetMotorSpeed. Point assembly passes straight
// through; only RPM is recorded, for the capture-stats summary.
type statsBuilder struct {
	inner network.FrameBuilder
	mu    sync.Mutex
	rpm   rpmAccumulator
}

func (b *statsBuilder) AddPointsPolar(points []l2frames.PointPolar) {
	b.inner.AddPointsPolar(points)
}

func (b *statsBuilder) SetMotorSpeed(rpm uint16) {
	b.inner.SetMotorSpeed(rpm)
	b.mu.Lock()
	b.rpm.observe(rpm)
	b.mu.Unlock()
}

func (b *statsBuilder) snapshot() rpmAccumulator {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.rpm
}
