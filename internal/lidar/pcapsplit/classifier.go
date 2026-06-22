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
	Stable             bool
	Moving             bool
}

// MotionClassifier owns the offline background model used by both
// pcap-analyse --motion and pcap-split. Keeping it here prevents their preview
// and written segments from drifting apart as tuning changes.
type MotionClassifier struct {
	bg *l3grid.BackgroundManager

	// The grid-wide settling/noise scans are expensive (72,000 cells) but their
	// values do not need sub-second precision: BuildTimeline applies 5 s / 60 s
	// hysteresis. Foreground remains evaluated for every complete frame.
	metricsAt time.Time
	metrics   l3grid.FrameSettlingMetrics
}

// NewMotionClassifier builds the common L3 background model from the given
// tuning (nil = embedded defaults). Replay mode only exposes foreground during
// warmup; every L3 model parameter remains identical to the live pipeline.
func NewMotionClassifier(sensorID, sourcePath string, tuningCfg *config.TuningConfig) (*MotionClassifier, error) {
	if sensorID == "" {
		return nil, fmt.Errorf("sensor ID is required")
	}
	tuningCfg = tuningOrEmbedded(tuningCfg)
	bgConfig := l3grid.BackgroundConfigFromActiveTuning(tuningCfg)
	bg := l3grid.NewBackgroundManagerDI(sensorID, gridRings, gridAzBins, bgConfig.ToBackgroundParams(), nil)
	bg.SetReplayMode(true)
	bg.SetSourcePath(sourcePath)
	return &MotionClassifier{bg: bg}, nil
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
	metrics := c.settlingEvidence(t)
	// Motion is evaluated by the common L3 engine from the foreground onset and
	// sustained-motion deviation signals.
	motion := c.bg.EvaluateSensorMotion(mask)

	return MotionEvidence{
		T:                  t,
		TotalPoints:        len(points),
		ForegroundPoints:   fg,
		ForegroundFraction: motion.ForegroundFraction,
		NonzeroCells:       metrics.NonzeroCells,
		SettledCells:       metrics.SettledCells,
		PercentSettled:     metrics.PercentSettled,
		DeviationFromNoise: motion.NoiseDeviation,
		Stable:             !motion.Moving,
		Moving:             motion.Moving,
	}, nil
}

func (c *MotionClassifier) settlingEvidence(t time.Time) l3grid.FrameSettlingMetrics {
	if c.metricsAt.IsZero() || t.Before(c.metricsAt) || t.Sub(c.metricsAt) >= time.Second {
		c.metrics = c.bg.GetFrameSettlingMetricsAt(c.bg.GetParams().LockedBaselineThreshold, t)
		c.metricsAt = t
	}
	return c.metrics
}
