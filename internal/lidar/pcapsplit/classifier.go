package pcapsplit

import (
	"fmt"
	"time"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// gridRings and gridAzBins are the fixed Pandar40P background-grid dimensions,
// matching the production pipeline.
const (
	gridRings  = 40
	gridAzBins = 1800
)

// MotionClassifierConfig controls the per-frame motion decision before
// timeline hysteresis is applied.
type MotionClassifierConfig struct {
	// SettledThreshold is the per-cell observation count above which a grid cell
	// is reported as "settled". It feeds the reported evidence only.
	SettledThreshold uint32
	// MovementForegroundThreshold is the foreground fraction at or above which a
	// frame is classified as sensor motion. It catches the *onset* of motion:
	// when the platform first moves, the scene shifts and foreground spikes.
	MovementForegroundThreshold float64
	// MovementDeviationThreshold is the noise deviation (mean RangeSpreadMeters
	// over the noise floor) at or above which a frame is classified as motion. It
	// catches *sustained* motion: as the platform keeps driving, every cell's
	// range churns, so the per-cell spread balloons and the foreground gate
	// widens until foreground collapses — but the spread (deviation) stays high.
	// Foreground alone goes blind to long drives; deviation does not.
	MovementDeviationThreshold float64
	// NoiseBoundsThreshold parameterises the reported WithinNoiseBounds metric.
	NoiseBoundsThreshold float64
}

// DefaultMotionClassifierConfig derives the classifier thresholds from the
// embedded tuning defaults, so they match the live pipeline's tuning.
func DefaultMotionClassifierConfig() MotionClassifierConfig {
	return MotionClassifierConfigFromTuning(tuningOrEmbedded(nil))
}

// MotionClassifierConfigFromTuning reads the classifier thresholds from the
// active L3 tuning. Motion is classified by sensor ego-motion, via the
// foreground fraction (onset) or the noise deviation (sustained drives); the
// classifier consumes capture timestamps so replay speed cannot change it.
func MotionClassifierConfigFromTuning(t *config.TuningConfig) MotionClassifierConfig {
	l3 := t.L3.ActiveCommon()
	return MotionClassifierConfig{
		SettledThreshold:            uint32(l3.SettledThreshold),
		MovementForegroundThreshold: l3.SensorMovementForegroundThreshold,
		MovementDeviationThreshold:  l3.MovementDeviationThreshold,
		NoiseBoundsThreshold:        l3.NoiseBoundsThreshold,
	}
}

// tuningOrEmbedded returns t when non-nil, else the tuning loaded from the
// default path falling back to the binary-embedded defaults. The embedded copy
// is validated at build time, so a load failure is a build invariant.
func tuningOrEmbedded(t *config.TuningConfig) *config.TuningConfig {
	if t != nil {
		return t
	}
	cfg, err := config.LoadTuningConfigOrEmbedded(config.DefaultConfigPath, radarassets.TuningDefaults)
	if err != nil {
		panic(fmt.Sprintf("pcapsplit: load embedded tuning: %v", err))
	}
	return cfg
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

// NewMotionClassifier builds a replay-specific background model and thresholds
// from the given tuning (nil = embedded defaults), the same tuning the live
// pipeline uses. Warmup gating is disabled for offline replay so the model
// settles from frame 0; the thresholds come straight from the L3 config.
func NewMotionClassifier(sensorID, sourcePath string, tuningCfg *config.TuningConfig) (*MotionClassifier, error) {
	if sensorID == "" {
		return nil, fmt.Errorf("sensor ID is required")
	}
	tuningCfg = tuningOrEmbedded(tuningCfg)
	cfg := MotionClassifierConfigFromTuning(tuningCfg)
	if cfg.SettledThreshold == 0 {
		cfg.SettledThreshold = DefaultSettledThreshold
	}

	bg := l3grid.NewBackgroundManagerDI(sensorID, gridRings, gridAzBins, motionBackgroundParams(tuningCfg), nil)
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
	// Motion is sensor ego-motion, signalled by a foreground spike (onset) or a
	// high noise deviation (sustained drive — see classifyMoving). Settled-cell %
	// and noise bounds are recorded as evidence but do not gate the decision, or
	// a capture that begins parked has its cold-start period (high foreground
	// while the background settles) mislabelled as motion.
	moving := classifyMoving(c.cfg, fraction, deviation)

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
		Stable:             !moving,
		Moving:             moving,
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

// classifyMoving reports sensor ego-motion when either signal fires: the
// foreground fraction (motion onset, before the background spread catches up)
// or the noise deviation (sustained motion, after foreground has collapsed into
// the widened gate). A parked sensor stays below both, even while settling.
func classifyMoving(cfg MotionClassifierConfig, foregroundFraction, deviation float64) bool {
	return foregroundFraction >= cfg.MovementForegroundThreshold ||
		deviation >= cfg.MovementDeviationThreshold
}

// motionBackgroundParams builds the offline background-model params from the
// tuning, applying the one intentional offline-replay divergence from live:
// warmup gating is disabled (and the settling period stretched) so the model
// settles from frame 0 on a fixed capture. Everything else matches live.
func motionBackgroundParams(tuningCfg *config.TuningConfig) l3grid.BackgroundParams {
	bgConfig := l3grid.BackgroundConfigFromTuning(tuningCfg.L3.EmaBaselineV1, tuningCfg.L4.DbscanXyV1)
	bgConfig.WarmupMinFrames = 0
	bgConfig.WarmupDuration = 0
	bgConfig.SettlingPeriod = 24 * time.Hour
	return bgConfig.ToBackgroundParams()
}
