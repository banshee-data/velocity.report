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

// MotionEvidence is the per-frame decision record. Moving is the raw
// classification consumed by BuildTimeline; Stable is its inverse.
type MotionEvidence struct {
	T                  time.Time
	TotalPoints        int
	ForegroundPoints   int
	ForegroundFraction float64
	NonzeroCells       int
	SettledCells       int
	PercentSettled     float64
	DriftRatio         float64
	Stable             bool
	Moving             bool
}

// MotionClassifier owns the offline background model used by pcap-split for both
// its motion/static timeline and the segments it writes. Keeping it here prevents
// the previewed timeline and the written segments from drifting apart as tuning
// changes. It refreshes the model during long continuous captures so the locked
// baseline describes the current location rather than a junction left behind.
type MotionClassifier struct {
	bg          *l3grid.BackgroundManager
	params      l3grid.BackgroundParams
	sensorID    string
	sourcePath  string
	elevations  []float64
	windowStart time.Time

	// The grid-wide settling/noise scans are expensive (72,000 cells) but their
	// values do not need sub-second precision: BuildTimeline applies 7 s / 60 s
	// hysteresis. Foreground remains evaluated for every complete frame.
	metricsAt time.Time
	metrics   l3grid.FrameSettlingMetrics
}

// A locked baseline is useful for distinguishing a moving sensor from passing
// traffic, but it remembers the previous junction after the sensor stops. A
// short, capture-time window gives the drift test a recent reference. The
// timeline's 60-second settling hysteresis absorbs a brief refresh dip during
// driving; its 7-second motion trigger absorbs startup spikes while parked.
// The 60-second window also lets L3 finish its 30-second warmup.
const motionWindow = 60 * time.Second

// NewMotionClassifier builds the common L3 background model from the given
// tuning (nil = embedded defaults). Replay mode only exposes foreground during
// warmup; every L3 model parameter remains identical to the live pipeline.
func NewMotionClassifier(sensorID, sourcePath string, tuningCfg *config.TuningConfig) (*MotionClassifier, error) {
	if sensorID == "" {
		return nil, fmt.Errorf("sensor ID is required")
	}
	tuningCfg = tuningOrEmbedded(tuningCfg)
	bgConfig := l3grid.BackgroundConfigFromActiveTuning(tuningCfg)
	params := bgConfig.ToBackgroundParams()
	bg := l3grid.NewBackgroundManagerDI(sensorID, gridRings, gridAzBins, params, nil)
	bg.SetReplayMode(true)
	bg.SetSourcePath(sourcePath)
	return &MotionClassifier{bg: bg, params: params, sensorID: sensorID, sourcePath: sourcePath}, nil
}

// SetRingElevations configures the model with the parser's sensor geometry.
func (c *MotionClassifier) SetRingElevations(elevations []float64) error {
	if c == nil || c.bg == nil {
		return fmt.Errorf("motion classifier is not initialized")
	}
	if err := c.bg.SetRingElevations(elevations); err != nil {
		return err
	}
	c.elevations = append(c.elevations[:0], elevations...)
	return nil
}

// Observe classifies one complete frame using PCAP time for every
// time-dependent background-model operation and model refresh.
func (c *MotionClassifier) Observe(t time.Time, points []l3grid.PointPolar) (MotionEvidence, error) {
	if c == nil || c.bg == nil {
		return MotionEvidence{}, fmt.Errorf("motion classifier is not initialized")
	}
	if c.windowStart.IsZero() {
		c.windowStart = t
	} else if t.Before(c.windowStart) || t.Sub(c.windowStart) >= motionWindow {
		if err := c.refresh(t); err != nil {
			return MotionEvidence{}, err
		}
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
		DriftRatio:         motion.DriftRatio,
		Stable:             !motion.Moving,
		Moving:             motion.Moving,
	}, nil
}

func (c *MotionClassifier) refresh(t time.Time) error {
	bg := l3grid.NewBackgroundManagerDI(c.sensorID, gridRings, gridAzBins, c.params, nil)
	bg.SetReplayMode(true)
	bg.SetSourcePath(c.sourcePath)
	if len(c.elevations) > 0 {
		if err := bg.SetRingElevations(c.elevations); err != nil {
			return fmt.Errorf("refresh ring elevations: %w", err)
		}
	}
	c.bg = bg
	c.windowStart = t
	c.metricsAt = time.Time{}
	return nil
}

func (c *MotionClassifier) settlingEvidence(t time.Time) l3grid.FrameSettlingMetrics {
	if c.metricsAt.IsZero() || t.Before(c.metricsAt) || t.Sub(c.metricsAt) >= time.Second {
		c.metrics = c.bg.GetFrameSettlingMetricsAt(c.bg.GetParams().LockedBaselineThreshold, t)
		c.metricsAt = t
	}
	return c.metrics
}
