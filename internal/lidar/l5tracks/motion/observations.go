package motion

import (
	"math"
	"sort"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// Estimator is what an offline evaluation needs from any model here: take
// observations in order, predict a position at a later capture time without
// changing state, and copy itself at a gap's start.
type Estimator interface {
	Observe(Observation) error
	PredictPosition(unixNanos int64) (x, y float64, err error)
	Current() Estimate
	CloneEstimator() Estimator
}

// Observe implements Estimator.
func (f *Filter) Observe(obs Observation) error {
	_, err := f.Step(obs)
	return err
}

// PredictPosition implements Estimator.
func (f *Filter) PredictPosition(unixNanos int64) (float64, float64, error) {
	est, err := f.Predict(unixNanos)
	return est.X[IdxX], est.X[IdxY], err
}

// CloneEstimator implements Estimator.
func (f *Filter) CloneEstimator() Estimator { return f.Clone() }

// Observe implements Estimator.
func (m *IMM) Observe(obs Observation) error {
	_, err := m.Step(obs)
	return err
}

// PredictPosition implements Estimator.
func (m *IMM) PredictPosition(unixNanos int64) (float64, float64, error) {
	est, _, err := m.Predict(unixNanos)
	return est.X[IdxX], est.X[IdxY], err
}

// CloneEstimator implements Estimator.
func (m *IMM) CloneEstimator() Estimator { return m.Clone() }

// TrackObservations is one track's observation sequence, in capture order.
type TrackObservations struct {
	TrackID          string
	CreationSequence int64
	Observations     []Observation
	// Skipped counts observations dropped because their capture time did not
	// advance on the previous one.
	Skipped int
}

// AdapterConfig says how filter-step observations become Observations.
type AdapterConfig struct {
	// MeasurementVariance is the scalar R the live filter's gain used
	// (measurement_noise); zero means DefaultMeasurementVariance.
	MeasurementVariance float64
	// UseGeometryCovariance takes R from the cluster-shape covariance the
	// residual retains, where it is a valid covariance, instead of the
	// scalar.
	UseGeometryCovariance bool
}

// ObservationCollector turns the live tracker's filter-step record into
// per-track observation sequences. It implements l5tracks.FilterStepObserver,
// so it can be attached to a tracker under replay with
// SetFilterStepObserver; it only reads the frames it is given.
//
// An observation is taken from every step whose update the filter applied
// (Observed, not refused, Observation.Applied) and from each founding step.
// Coasted steps carry no evidence and are not observations; they appear as
// the gaps between capture times.
type ObservationCollector struct {
	cfg    AdapterConfig
	tracks map[string]*TrackObservations
}

// NewObservationCollector returns an empty collector.
func NewObservationCollector(cfg AdapterConfig) *ObservationCollector {
	if cfg.MeasurementVariance <= 0 {
		cfg.MeasurementVariance = DefaultMeasurementVariance
	}
	return &ObservationCollector{cfg: cfg, tracks: map[string]*TrackObservations{}}
}

// chainKey separates two chains that reuse a track ID.
func chainKey(id string, seq int64) string {
	return id + "\x00" + strconv.FormatInt(seq, 10)
}

// ObserveFilterFrame implements l5tracks.FilterStepObserver.
func (c *ObservationCollector) ObserveFilterFrame(frame l5tracks.FilterFrame) {
	for _, step := range frame.Steps {
		if !step.Observed || step.UpdateRefused || !step.Observation.Applied() {
			continue
		}
		key := chainKey(step.TrackID, step.CreationSequence)
		track := c.tracks[key]
		if track == nil {
			track = &TrackObservations{TrackID: step.TrackID, CreationSequence: step.CreationSequence}
			c.tracks[key] = track
		}
		obs := c.cfg.Observation(step)
		if n := len(track.Observations); n > 0 && obs.UnixNanos <= track.Observations[n-1].UnixNanos {
			track.Skipped++
			continue
		}
		track.Observations = append(track.Observations, obs)
	}
}

// Observation is the observation a filter step's evidence becomes: its
// measured position at its measurement time (the state time, then the frame
// time, when the step carries none), with the scalar R, or the cluster-shape
// covariance under UseGeometryCovariance where it is a valid covariance, and
// the live NIS when the step has an innovation. It does not decide whether
// the step is evidence; ObservationCollector does.
func (cfg AdapterConfig) Observation(step l5tracks.FilterStep) Observation {
	if cfg.MeasurementVariance <= 0 {
		cfg.MeasurementVariance = DefaultMeasurementVariance
	}
	o := step.Observation
	t := o.MeasurementUnixNanos
	if t <= 0 {
		t = step.StateUnixNanos
	}
	if t <= 0 {
		t = step.FrameUnixNanos
	}
	obs := Observation{
		UnixNanos: t, X: float64(o.X), Y: float64(o.Y),
		R:         IsotropicR(cfg.MeasurementVariance),
		ClusterID: o.ClusterID,
	}
	if o.HasInnovation {
		obs.LiveNIS, obs.HasLiveNIS = float64(o.NIS), true
	}
	if cfg.UseGeometryCovariance {
		g := o.GeometryCovariance
		xx, xy, yy := float64(g.XX), float64(g.XY), float64(g.YY)
		if xx > 0 && yy > 0 && xx*yy-xy*xy > 0 && !math.IsInf(xx+yy, 0) {
			obs.R = [2][2]float64{{xx, xy}, {xy, yy}}
		}
	}
	return obs
}

// Tracks returns the collected sequences, by creation sequence then ID.
func (c *ObservationCollector) Tracks() []TrackObservations {
	out := make([]TrackObservations, 0, len(c.tracks))
	for _, t := range c.tracks {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreationSequence != out[j].CreationSequence {
			return out[i].CreationSequence < out[j].CreationSequence
		}
		return out[i].TrackID < out[j].TrackID
	})
	return out
}

// ObservationsFromFilterFrames is the collector over a stored record.
func ObservationsFromFilterFrames(frames []l5tracks.FilterFrame, cfg AdapterConfig) []TrackObservations {
	c := NewObservationCollector(cfg)
	for _, f := range frames {
		c.ObserveFilterFrame(f)
	}
	return c.Tracks()
}

var _ l5tracks.FilterStepObserver = (*ObservationCollector)(nil)
