package pcapsplit

import (
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// gridRings and gridAzBins are the fixed Pandar40P background-grid dimensions,
// matching the production pipeline.
const (
	gridRings  = 40
	gridAzBins = 1800
)

// MotionClassifierConfig controls the per-frame stable-scene decision before
// timeline hysteresis is applied.
type MotionClassifierConfig struct {
	SettledThreshold      uint32
	MaxForegroundFraction float64
	MinSettledFraction    float64
	MaxNoiseDeviation     float64
	NoiseBoundsThreshold  float64
}

// DefaultMotionClassifierConfig matches the PCAP split design's stable-scene
// rule. The classifier deliberately disables wall-clock warmup and consumes
// capture timestamps so replay speed cannot change its result.
func DefaultMotionClassifierConfig() MotionClassifierConfig {
	return MotionClassifierConfig{
		SettledThreshold:      DefaultSettledThreshold,
		MaxForegroundFraction: 0.05,
		MinSettledFraction:    0.70,
		MaxNoiseDeviation:     2.0,
		NoiseBoundsThreshold:  2.0,
	}
}

// MotionEvidence is the complete per-frame decision record. Moving is the raw
// classification consumed by BuildTimeline; Stable is its inverse.
type MotionEvidence struct {
	T                  time.Time
	TotalPoints        int
	ForegroundPoints   int
	ForegroundFraction float64
	NonzeroCells       int
	SettledCells       int
	PercentSettled     float64
	DeviationFromNoise float64
	WithinNoiseBounds  bool
	Stable             bool
	Moving             bool
}

// MotionClassifier owns the offline background model used by both
// pcap-analyse --motion and pcap-split. Keeping it here prevents their preview
// and written segments from drifting apart as tuning changes.
type MotionClassifier struct {
	bg  *l3grid.BackgroundManager
	cfg MotionClassifierConfig

	// The grid-wide settling/noise scans are expensive (72,000 cells) but their
	// values do not need sub-second precision: BuildTimeline applies 5 s / 60 s
	// hysteresis. Foreground remains evaluated for every complete frame.
	metricsAt        time.Time
	metrics          l3grid.FrameSettlingMetrics
	noiseDeviation   float64
	withinNoiseBound bool
}

// NewMotionClassifier builds a replay-specific background model from the
// active default tuning. It must not share the tracking pipeline's historical
// hard-coded parameters or wall-clock warmup behaviour.
func NewMotionClassifier(sensorID, sourcePath string, cfg MotionClassifierConfig) (*MotionClassifier, error) {
	if sensorID == "" {
		return nil, fmt.Errorf("sensor ID is required")
	}
	if cfg.SettledThreshold == 0 {
		cfg.SettledThreshold = DefaultSettledThreshold
	}
	if cfg.MaxForegroundFraction <= 0 || cfg.MaxForegroundFraction >= 1 ||
		cfg.MinSettledFraction <= 0 || cfg.MinSettledFraction > 1 ||
		cfg.MaxNoiseDeviation <= 0 || cfg.NoiseBoundsThreshold <= 0 {
		return nil, fmt.Errorf("invalid motion classifier thresholds")
	}

	params := motionBackgroundParams()
	bg := l3grid.NewBackgroundManagerDI(sensorID, gridRings, gridAzBins, params, nil)
	bg.SetSourcePath(sourcePath)
	return &MotionClassifier{bg: bg, cfg: cfg}, nil
}

// SetRingElevations configures the model with the parser's sensor geometry.
func (c *MotionClassifier) SetRingElevations(elevations []float64) error {
	if c == nil || c.bg == nil {
		return fmt.Errorf("motion classifier is not initialized")
	}
	return c.bg.SetRingElevations(elevations)
}

// Observe classifies one complete frame using PCAP time for every
// time-dependent background-model operation.
func (c *MotionClassifier) Observe(t time.Time, points []l3grid.PointPolar) (MotionEvidence, error) {
	if c == nil || c.bg == nil {
		return MotionEvidence{}, fmt.Errorf("motion classifier is not initialized")
	}
	mask, _ := c.bg.ProcessFramePolarWithMaskAt(points, t)
	fg := 0
	for _, isForeground := range mask {
		if isForeground {
			fg++
		}
	}
	fraction := 0.0
	if len(mask) > 0 {
		fraction = float64(fg) / float64(len(mask))
	}
	metrics, deviation, withinBounds := c.settlingEvidence(t)
	stable := isStable(c.cfg, fraction, metrics.PercentSettled, deviation, withinBounds)

	return MotionEvidence{
		T:                  t,
		TotalPoints:        len(points),
		ForegroundPoints:   fg,
		ForegroundFraction: fraction,
		NonzeroCells:       metrics.NonzeroCells,
		SettledCells:       metrics.SettledCells,
		PercentSettled:     metrics.PercentSettled,
		DeviationFromNoise: deviation,
		WithinNoiseBounds:  withinBounds,
		Stable:             stable,
		Moving:             !stable,
	}, nil
}

func (c *MotionClassifier) settlingEvidence(t time.Time) (l3grid.FrameSettlingMetrics, float64, bool) {
	if c.metricsAt.IsZero() || t.Before(c.metricsAt) || t.Sub(c.metricsAt) >= time.Second {
		c.metrics = c.bg.GetFrameSettlingMetricsAt(c.cfg.SettledThreshold, t)
		c.noiseDeviation = c.bg.GetNoiseBoundsDeviation()
		c.withinNoiseBound = c.bg.IsWithinNoiseBounds(c.cfg.NoiseBoundsThreshold)
		c.metricsAt = t
	}
	return c.metrics, c.noiseDeviation, c.withinNoiseBound
}

func isStable(cfg MotionClassifierConfig, foregroundFraction, settledFraction, noiseDeviation float64, withinNoiseBounds bool) bool {
	return foregroundFraction < cfg.MaxForegroundFraction &&
		settledFraction >= cfg.MinSettledFraction &&
		noiseDeviation < cfg.MaxNoiseDeviation &&
		withinNoiseBounds
}

func motionBackgroundParams() l3grid.BackgroundParams {
	tuningCfg := config.MustLoadDefaultConfig()
	bgConfig := l3grid.BackgroundConfigFromTuning(tuningCfg.L3.EmaBaselineV1, tuningCfg.L4.DbscanXyV1)
	// Offline classification has no wall clock: capture timestamps supplied to
	// Observe advance freeze state, and warmup is represented by the explicit
	// stable-scene rule plus the timeline's 60-second hysteresis.
	bgConfig.WarmupMinFrames = 0
	bgConfig.WarmupDuration = 0
	bgConfig.SettlingPeriod = 24 * time.Hour
	// The config loader validates the selected active engine before returning it.
	return bgConfig.ToBackgroundParams()
}
