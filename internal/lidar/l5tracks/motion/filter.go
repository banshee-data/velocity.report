package motion

import (
	"errors"
	"fmt"
	"math"
)

// CoastRule is what a filter does while a track is unobserved (maths review
// 2026-11-08, Section 4.4). A prediction is unobserved when its interval
// exceeds FilterConfig.CoastAfterSecs. Each part is configurable; the CA
// defaults switch all of them on.
type CoastRule struct {
	// DecayTau, when positive, replaces a CA model's transition by Singer's
	// with this time constant for unobserved predictions, so an acceleration
	// nobody is measuring relaxes toward zero (e^{−t/τ}) instead of
	// compounding quadratically into position. It has no effect on CV.
	DecayTau float64
	// NoReversal forbids an unobserved prediction from carrying the velocity
	// through zero along the course held at the gap's start. When the
	// velocity's projection on that course would cross zero, the state is
	// conditioned on that velocity component and the acceleration along it
	// being zero, through its regression on them (constrainLinear): the mean
	// and P move together, so the correlated components move with it. It is
	// not a clamp of the state alone.
	NoReversal bool
	// MinCourseSpeed is the speed below which no course is recorded at the
	// gap's start, so the no-reversal constraint has no direction to hold. A
	// course read off a near-zero velocity is noise.
	MinCourseSpeed float64
}

// InitConfig sets the covariance a track starts from. Position variance is
// the first observation's R; velocity and acceleration start at zero with
// these variances (acceleration is always zero for CV).
type InitConfig struct {
	VelVar float64
	AccVar float64
}

// FilterConfig configures one single-model filter.
type FilterConfig struct {
	Model ProcessModel
	Coast CoastRule
	// MaxStepSecs is the longest single prediction step. A longer interval is
	// predicted in equal sub-steps no longer than this, never clamped to it.
	// The linear part of every model composes exactly (F(a)F(b) = F(a+b), and
	// Q likewise), so sub-steps change no mean or covariance by themselves;
	// they set the grid on which the coast rule's crossing is looked for and
	// the covariance cap applied.
	MaxStepSecs float64
	// CoastAfterSecs is the interval above which a prediction counts as
	// unobserved and the coast rule applies. At 10 Hz one frame is 0.1 s, so
	// the default 0.15 s treats any dropped frame as a coast.
	CoastAfterSecs float64
	// MaxVariance caps every variance with CapCovariance (P ← D P D); zero
	// disables the cap. The default matches the live tracker's
	// max_covariance_diag of 100.
	MaxVariance float64
	Init        InitConfig
}

// Offline defaults. These are prototype values, not tuning: G-EST-1 is the
// process that selects them, and none of them is read by the live tracker.
const (
	// DefaultCVQ is the live tracker's process_noise_vel, read as the
	// continuous white-noise-acceleration density (m²/s³).
	DefaultCVQ = 0.2
	// DefaultCVPosVar is the live tracker's process_noise_pos.
	DefaultCVPosVar = 0.05
	// DefaultCAQ is the white-noise-jerk density (m²/s⁵) the maths review's
	// simulations used, inside its 0.5 to 3 range.
	DefaultCAQ = 1.0
	// DefaultCoastTau is the Singer decay time constant while unobserved;
	// the review names 1 to 2 s as the range for G-EST-1 to select from.
	DefaultCoastTau       = 1.5
	DefaultMinCourseSpeed = 0.3
	DefaultMaxStepSecs    = 0.1
	DefaultCoastAfterSecs = 0.15
	DefaultMaxVariance    = 100
	// DefaultMeasurementVariance is the live tracker's measurement_noise.
	DefaultMeasurementVariance = 0.05
	DefaultInitVelVar          = 25
	DefaultInitAccVar          = 4
)

// DefaultCVConfig is CV at the live tracker's noise levels with the coupled
// continuous Q, plus the no-reversal constraint (inert for CV, whose velocity
// does not change while unobserved).
func DefaultCVConfig() FilterConfig {
	return FilterConfig{
		Model:          ProcessModel{Kind: ModelCV, Q: DefaultCVQ, PosVar: DefaultCVPosVar},
		Coast:          CoastRule{NoReversal: true, MinCourseSpeed: DefaultMinCourseSpeed},
		MaxStepSecs:    DefaultMaxStepSecs,
		CoastAfterSecs: DefaultCoastAfterSecs,
		MaxVariance:    DefaultMaxVariance,
		Init:           InitConfig{VelVar: DefaultInitVelVar},
	}
}

// DefaultCAConfig is white-noise-jerk CA with the full coast rule.
func DefaultCAConfig() FilterConfig {
	return FilterConfig{
		Model:          ProcessModel{Kind: ModelCA, Q: DefaultCAQ},
		Coast:          CoastRule{DecayTau: DefaultCoastTau, NoReversal: true, MinCourseSpeed: DefaultMinCourseSpeed},
		MaxStepSecs:    DefaultMaxStepSecs,
		CoastAfterSecs: DefaultCoastAfterSecs,
		MaxVariance:    DefaultMaxVariance,
		Init:           InitConfig{VelVar: DefaultInitVelVar, AccVar: DefaultInitAccVar},
	}
}

// Validate reports a configuration the filter cannot run.
func (c FilterConfig) Validate() error {
	if err := c.Model.Validate(); err != nil {
		return err
	}
	if !(c.MaxStepSecs > 0) || math.IsInf(c.MaxStepSecs, 0) {
		return fmt.Errorf("motion: max step must be positive and finite, got %v", c.MaxStepSecs)
	}
	if !(c.CoastAfterSecs >= 0) || math.IsInf(c.CoastAfterSecs, 0) {
		return fmt.Errorf("motion: coast threshold must be finite and non-negative, got %v", c.CoastAfterSecs)
	}
	if !(c.Coast.DecayTau >= 0) || !(c.Coast.MinCourseSpeed >= 0) || !(c.MaxVariance >= 0) {
		return fmt.Errorf("motion: coast decay, course speed and variance cap must be non-negative")
	}
	if !(c.Init.VelVar >= 0) || !(c.Init.AccVar >= 0) {
		return fmt.Errorf("motion: initial variances must be non-negative")
	}
	return nil
}

// Estimate is a state and its covariance at a capture time.
type Estimate struct {
	UnixNanos int64
	X         Vec
	P         Mat
}

// Step is one filter cycle, as the filter computed it.
type Step struct {
	UnixNanos int64
	// Founded marks the observation that initialised the track: there is no
	// prior or innovation.
	Founded bool
	// DtSecs is the interval predicted across; SubSteps how many steps it took.
	DtSecs   float64
	SubSteps int
	// Coasted means the prediction was unobserved (DtSecs > CoastAfterSecs).
	Coasted bool
	// Constrained means the no-reversal constraint fired during the
	// prediction, at ConstrainedAfterSecs into it.
	Constrained          bool
	ConstrainedAfterSecs float64
	Prior                Estimate
	Posterior            Estimate
	Innovation           Innovation
	// LogLikelihood is the Gaussian log likelihood of the innovation.
	LogLikelihood float64
}

// Filter is a single-model Kalman filter in the six-state space.
type Filter struct {
	cfg         FilterConfig
	est         Estimate
	initialised bool
}

// NewFilter validates cfg and returns an uninitialised filter.
func NewFilter(cfg FilterConfig) (*Filter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Filter{cfg: cfg}, nil
}

// Config returns the filter's configuration.
func (f *Filter) Config() FilterConfig { return f.cfg }

// Initialised reports whether the filter holds a state.
func (f *Filter) Initialised() bool { return f.initialised }

// Current returns the filter's latest posterior.
func (f *Filter) Current() Estimate { return f.est }

// SetState replaces the filter's state. For CV the acceleration and its
// covariance are dropped, which is the embedding.
func (f *Filter) SetState(est Estimate) {
	if f.cfg.Model.Kind == ModelCV {
		est.X, est.P = zeroAcceleration(est.X, est.P)
	}
	f.est = est
	f.initialised = true
}

// Clone returns an independent copy.
func (f *Filter) Clone() *Filter {
	c := *f
	return &c
}

func (f *Filter) initialEstimate(obs Observation) Estimate {
	est := Estimate{UnixNanos: obs.UnixNanos}
	est.X[IdxX], est.X[IdxY] = obs.X, obs.Y
	est.P[IdxX][IdxX], est.P[IdxY][IdxY] = obs.R[0][0], obs.R[1][1]
	xy := 0.5 * (obs.R[0][1] + obs.R[1][0])
	est.P[IdxX][IdxY], est.P[IdxY][IdxX] = xy, xy
	est.P[IdxVX][IdxVX], est.P[IdxVY][IdxVY] = f.cfg.Init.VelVar, f.cfg.Init.VelVar
	if f.cfg.Model.Kind == ModelCA {
		est.P[IdxAX][IdxAX], est.P[IdxAY][IdxAY] = f.cfg.Init.AccVar, f.cfg.Init.AccVar
	}
	return est
}

// ErrTimeOrder is returned for an observation earlier than the state.
var ErrTimeOrder = errors.New("motion: observation precedes the filter state")

// Step assimilates one observation: initialise on the first, otherwise
// predict to its capture time and apply the Joseph update. On an error the
// filter is unchanged.
func (f *Filter) Step(obs Observation) (Step, error) {
	if !f.initialised {
		if obs.RankOne {
			return Step{}, fmt.Errorf("motion: a track cannot be founded on a rank-one observation")
		}
		if _, _, _, _, err := measurementRows(obs); err != nil {
			return Step{}, err
		}
		f.est = f.initialEstimate(obs)
		f.initialised = true
		return Step{UnixNanos: obs.UnixNanos, Founded: true, Prior: f.est, Posterior: f.est}, nil
	}
	if obs.UnixNanos < f.est.UnixNanos {
		return Step{}, ErrTimeOrder
	}
	prior, info := f.predict(f.est, obs.UnixNanos, nil)
	x, p, in, err := josephUpdate(prior.X, prior.P, obs)
	if err != nil {
		return Step{}, err
	}
	post := Estimate{UnixNanos: obs.UnixNanos, X: x, P: p}
	if f.cfg.Model.Kind == ModelCV {
		post.X, post.P = zeroAcceleration(post.X, post.P)
	}
	f.est = post
	return Step{
		UnixNanos: obs.UnixNanos, DtSecs: info.dtSecs, SubSteps: info.subSteps, Coasted: info.coasted,
		Constrained: info.constrained, ConstrainedAfterSecs: info.constrainedAfter,
		Prior: prior, Posterior: post, Innovation: in, LogLikelihood: GaussianLogLikelihood(in),
	}, nil
}

// Predict returns the prior at unixNanos from the current posterior without
// changing the filter: the prediction a track would carry into an
// observation at that time, coast rule included.
func (f *Filter) Predict(unixNanos int64) (Estimate, error) {
	if !f.initialised {
		return Estimate{}, fmt.Errorf("motion: filter is not initialised")
	}
	if unixNanos < f.est.UnixNanos {
		return Estimate{}, ErrTimeOrder
	}
	prior, _ := f.predict(f.est, unixNanos, nil)
	return prior, nil
}

type predictInfo struct {
	dtSecs           float64
	subSteps         int
	coasted          bool
	constrained      bool
	constrainedAfter float64
}

// course is a unit direction in the plane.
type course struct{ ux, uy float64 }

// courseOf returns the direction of the estimate's velocity, or false below
// the configured minimum speed.
func (f *Filter) courseOf(x Vec) (course, bool) {
	speed := math.Hypot(x[IdxVX], x[IdxVY])
	if !(speed > 0) || speed < f.cfg.Coast.MinCourseSpeed {
		return course{}, false
	}
	return course{x[IdxVX] / speed, x[IdxVY] / speed}, true
}

// predict carries est to unixNanos in sub-steps. gapCourse, when non-nil,
// overrides the course the no-reversal rule holds (the IMM passes the
// combined estimate's, so both modes hold the same one).
func (f *Filter) predict(est Estimate, unixNanos int64, gapCourse *course) (Estimate, predictInfo) {
	dt := float64(unixNanos-est.UnixNanos) / 1e9
	info := predictInfo{dtSecs: dt}
	if dt <= 0 {
		est.UnixNanos = unixNanos
		return est, info
	}
	info.coasted = dt > f.cfg.CoastAfterSecs
	model := f.cfg.Model
	if info.coasted && model.Kind == ModelCA && f.cfg.Coast.DecayTau > 0 {
		model.SingerTau = f.cfg.Coast.DecayTau
	}
	var u course
	hold := false
	if info.coasted && f.cfg.Coast.NoReversal {
		if gapCourse != nil {
			u, hold = *gapCourse, true
		} else {
			u, hold = f.courseOf(est.X)
		}
	}
	n := int(math.Ceil(dt/f.cfg.MaxStepSecs - 1e-9))
	if n < 1 {
		n = 1
	}
	info.subSteps = n
	h := dt / float64(n)
	x, p := est.X, est.P
	for i := 0; i < n; i++ {
		x1, p1 := f.propagate(model, x, p, h)
		if hold && u.along(x1) < 0 {
			if !info.constrained {
				info.constrained = true
				info.constrainedAfter = float64(i) * h
			}
			if before := u.along(x); before > 0 {
				// Stop at the crossing, constrain there, and carry on.
				tc := crossingTime(model, x, h, u)
				info.constrainedAfter = float64(i)*h + tc
				x, p = f.propagate(model, x, p, tc)
				x, p = u.hold(x, p)
				x1, p1 = f.propagate(model, x, p, h-tc)
				if u.along(x1) < 0 {
					x1, p1 = u.hold(x1, p1)
				}
			} else {
				x1, p1 = u.hold(x1, p1)
			}
		}
		x, p = x1, p1
	}
	return Estimate{UnixNanos: unixNanos, X: x, P: p}, info
}

// propagate is one exact step of the model: x ← F x, P ← F P Fᵀ + Q, then
// the PSD-preserving cap.
func (f *Filter) propagate(model ProcessModel, x Vec, p Mat, dt float64) (Vec, Mat) {
	if dt <= 0 {
		return x, p
	}
	fm, q := model.Discretise(dt)
	x = fm.apply(x)
	p = quadratic(fm, p).add(q)
	return x, CapCovariance(p, f.cfg.MaxVariance)
}

// crossingTime finds, by bisection on the mean, when within (0, h] the
// velocity along u reaches zero. Only the mean is propagated; the transition
// is linear, so the crossing is a property of the mean alone.
func crossingTime(model ProcessModel, x Vec, h float64, u course) float64 {
	lo, hi := 0.0, h
	for k := 0; k < 60 && hi-lo > 1e-9; k++ {
		mid := 0.5 * (lo + hi)
		fm, _ := model.Discretise(mid)
		if u.along(fm.apply(x)) > 0 {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi
}

func (u course) along(x Vec) float64 { return u.ux*x[IdxVX] + u.uy*x[IdxVY] }

// hold conditions the state on zero velocity and zero acceleration along u.
func (u course) hold(x Vec, p Mat) (Vec, Mat) {
	var hv, ha Vec
	hv[IdxVX], hv[IdxVY] = u.ux, u.uy
	ha[IdxAX], ha[IdxAY] = u.ux, u.uy
	x, p = constrainLinear(x, p, hv, 0)
	return constrainLinear(x, p, ha, 0)
}

// constrainLinear conditions (x, P) on the linear equality h·x = target, h a
// unit vector, as a noise-free measurement:
//
//	k = P hᵀ / (h P hᵀ),  x ← x + k (target − h x),  P ← P − k h P
//
// Every component correlated with h·x moves with it, by its regression on
// it, and P loses exactly the variance along h. When h P hᵀ is already zero
// the component is known exactly and only the mean is moved, along h.
func constrainLinear(x Vec, p Mat, h Vec, target float64) (Vec, Mat) {
	ph := p.apply(h)
	s := dot(h, ph)
	r := target - dot(h, x)
	scale := 0.0
	for i := 0; i < StateDim; i++ {
		scale = math.Max(scale, p[i][i])
	}
	if !(s > 1e-15*math.Max(scale, 1e-300)) {
		for i := range x {
			x[i] += h[i] * r
		}
		return x, p
	}
	for i := range x {
		x[i] += ph[i] / s * r
	}
	p = p.add(outer(ph, ph).scale(-1 / s))
	return x, Symmetrise(p)
}

// zeroAcceleration is the CV embedding applied to an arbitrary estimate: the
// acceleration mean and every acceleration row and column of P set to zero.
// It is what CV's transition does in one step; it is applied directly where
// a CV estimate is set or updated, so a CV posterior never carries one.
func zeroAcceleration(x Vec, p Mat) (Vec, Mat) {
	for _, i := range []int{IdxAX, IdxAY} {
		x[i] = 0
		for j := 0; j < StateDim; j++ {
			p[i][j], p[j][i] = 0, 0
		}
	}
	return x, p
}

// RunFilter runs a fresh filter over a track's observations in order and
// returns every step.
func RunFilter(cfg FilterConfig, observations []Observation) ([]Step, error) {
	f, err := NewFilter(cfg)
	if err != nil {
		return nil, err
	}
	steps := make([]Step, 0, len(observations))
	for i, obs := range observations {
		step, err := f.Step(obs)
		if err != nil {
			return steps, fmt.Errorf("motion: observation %d: %w", i, err)
		}
		steps = append(steps, step)
	}
	return steps, nil
}
