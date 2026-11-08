package motion

import (
	"fmt"
	"math"
)

// Interacting multiple model over {CV, CA} (Blom & Bar-Shalom 1988).
//
// Each cycle, across a capture interval Δt:
//
//  1. Transition. Π(Δt) = expm(Λ Δt) from the switching-rate generator Λ,
//     formed from the interval itself, so a dropped frame or a capture gap
//     switches with the probability its length implies (ModeRates).
//  2. Mixing. Predicted mode probabilities c_j = Σ_i Π_ij μ_i and mixing
//     weights μ_{i|j} = Π_ij μ_i / c_j; each mode starts from
//     x0_j = Σ_i μ_{i|j} x_i and P0_j = Σ_i μ_{i|j} (P_i + (x_i − x0_j)(…)ᵀ).
//     Both modes live in the six-state space, so the sums are like with like.
//     CV's transition maps the mixed acceleration to zero.
//  3. Mode-matched filtering. Each mode predicts its mixed state (CA with its
//     coast rule, holding the course of the combined estimate at the gap's
//     start) and applies the Joseph update.
//  4. Mode probabilities. μ_j ∝ c_j L_j, with L_j the configured likelihood
//     of mode j's innovation, in the log domain.
//  5. Combination. x = Σ μ_j x_j, P = Σ μ_j (P_j + (x_j − x)(…)ᵀ).
//
// Mode probabilities are output on every step (IMMStep), so "CA carried 0.82
// of the weight during braking" is a line in the record, not an inference.

// Mode indices.
const (
	ModeCV = 0
	ModeCA = 1
	nModes = 2
)

// ModeRates is the generator of the two-mode switching process, in s⁻¹:
//
//	Λ = | −λ12   λ12 |    λ12 = CVToCA, λ21 = CAToCV
//	    |  λ21  −λ21 |
//
// A rate is the reciprocal of the mean dwell time in the mode it leaves.
type ModeRates struct {
	CVToCA float64
	CAToCV float64
}

// RatesFromDwell returns the generator for mean dwell times in seconds.
func RatesFromDwell(dwellCV, dwellCA float64) (ModeRates, error) {
	if !(dwellCV > 0) || !(dwellCA > 0) || math.IsInf(dwellCV, 0) || math.IsInf(dwellCA, 0) {
		return ModeRates{}, fmt.Errorf("motion: dwell times must be positive and finite, got %v and %v", dwellCV, dwellCA)
	}
	return ModeRates{CVToCA: 1 / dwellCV, CAToCV: 1 / dwellCA}, nil
}

// Transition returns Π(Δt) = expm(Λ Δt) in closed form. With λ = λ12 + λ21
// and E = e^{−λΔt}:
//
//	Π = | (λ21 + λ12 E)/λ   λ12 (1 − E)/λ  |
//	    | λ21 (1 − E)/λ     (λ12 + λ21 E)/λ |
//
// The off-diagonal terms are computed with expm1, and each diagonal as one
// minus its row's off-diagonal, so rows sum to one to rounding. Π(0) = I and
// Π(a + b) = Π(a) Π(b).
func (r ModeRates) Transition(dtSecs float64) [nModes][nModes]float64 {
	lambda := r.CVToCA + r.CAToCV
	if dtSecs <= 0 || lambda <= 0 {
		return [nModes][nModes]float64{{1, 0}, {0, 1}}
	}
	oneMinusE := -math.Expm1(-lambda * dtSecs)
	p12 := r.CVToCA * oneMinusE / lambda
	p21 := r.CAToCV * oneMinusE / lambda
	return [nModes][nModes]float64{{1 - p12, p12}, {p21, 1 - p21}}
}

// Stationary returns the switching process's stationary distribution,
// (λ21, λ12)/λ: the share of time spent in each mode, dwell-weighted.
func (r ModeRates) Stationary() [nModes]float64 {
	lambda := r.CVToCA + r.CAToCV
	if lambda <= 0 {
		return [nModes]float64{0.5, 0.5}
	}
	return [nModes]float64{r.CAToCV / lambda, r.CVToCA / lambda}
}

// IMMConfig configures the IMM. Both mode filters run in the common space;
// their own MaxStepSecs, CoastAfterSecs and coast rule apply.
type IMMConfig struct {
	CV, CA FilterConfig
	// DwellCVSecs and DwellCASecs are the mean dwell times defining the
	// switching rates.
	DwellCVSecs, DwellCASecs float64
	// Likelihood scores each mode's innovation for the probability update.
	Likelihood Likelihood
	// InitialProb is the mode distribution at founding; the zero value means
	// the switching process's stationary distribution.
	InitialProb [nModes]float64
}

// IMM defaults. These are prototype values for G-EST-1 to select, not
// tuning.
//
// Inside an IMM the modes divide the work the single CV filter's process
// noise does alone. The shipped process_noise_vel of 0.2 is sized to absorb
// manoeuvres CV cannot represent; with the CA mode carrying those, the CV
// mode is the quiet one, so it gets a smaller q and no independent position
// term, and the CA mode the upper half of the review's 0.5 to 3 m²/s⁵ range.
// If the two modes are about as wide, as they are with both at their
// single-model defaults (prior position variance 0.030 against 0.031 m²),
// nothing separates them on a steady track and the mode probabilities sit at
// the switching process's stationary distribution whatever the motion.
//
// Steady driving is the common case and a speed change takes a few seconds,
// so CV is given the longer dwell.
const (
	DefaultIMMCVQ      = 0.05
	DefaultIMMCAQ      = 2.0
	DefaultDwellCVSecs = 6.0
	DefaultDwellCASecs = 3.0
)

// DefaultIMMConfig is the IMM over a quiet CV mode and a CA mode with the full
// coast rule, the default dwell times, and the Student-t likelihood with
// ν = 4. The robust likelihood is the default because the live residual
// record (K9) puts 7 to 17 % of moving innovations beyond the 95 % bound, and
// under a Gaussian each one would move most of the probability to the wider
// mode.
func DefaultIMMConfig() IMMConfig {
	cv := DefaultCVConfig()
	cv.Model.Q, cv.Model.PosVar = DefaultIMMCVQ, 0
	ca := DefaultCAConfig()
	ca.Model.Q = DefaultIMMCAQ
	return IMMConfig{
		CV: cv, CA: ca,
		DwellCVSecs: DefaultDwellCVSecs, DwellCASecs: DefaultDwellCASecs,
		Likelihood: Likelihood{Kind: LikelihoodStudentT, Nu: DefaultStudentTNu},
	}
}

// IMMStep is one IMM cycle, every intermediate it used.
type IMMStep struct {
	UnixNanos int64
	Founded   bool
	DtSecs    float64
	// Transition is Π(DtSecs); PredictedProb the c_j; Mixing[i][j] = μ_{i|j}.
	Transition    [nModes][nModes]float64
	PredictedProb [nModes]float64
	Mixing        [nModes][nModes]float64
	// LogLikelihood[j] is the configured likelihood of mode j's innovation;
	// -Inf for a mode whose innovation covariance was singular.
	LogLikelihood [nModes]float64
	// ModeProb is μ after this step's update.
	ModeProb [nModes]float64
	// Modes are the mode-matched steps (each mode's prior is its mixed,
	// predicted state).
	Modes [nModes]Step
	// Prior is the combined prediction, weighted by PredictedProb; Innovation
	// is the observation against it. Posterior is the combined estimate.
	Prior      Estimate
	Innovation Innovation
	Posterior  Estimate
}

// IMM is the two-mode interacting multiple model filter.
type IMM struct {
	cfg   IMMConfig
	rates ModeRates
	modes [nModes]*Filter
	mu    [nModes]float64
}

// NewIMM validates cfg and returns an uninitialised IMM.
func NewIMM(cfg IMMConfig) (*IMM, error) {
	if cfg.CV.Model.Kind != ModelCV || cfg.CA.Model.Kind != ModelCA {
		return nil, fmt.Errorf("motion: IMM modes must be CV and CA, got %q and %q", cfg.CV.Model.Kind, cfg.CA.Model.Kind)
	}
	rates, err := RatesFromDwell(cfg.DwellCVSecs, cfg.DwellCASecs)
	if err != nil {
		return nil, err
	}
	cv, err := NewFilter(cfg.CV)
	if err != nil {
		return nil, err
	}
	ca, err := NewFilter(cfg.CA)
	if err != nil {
		return nil, err
	}
	switch cfg.Likelihood.Kind {
	case LikelihoodGaussian, LikelihoodStudentT:
	default:
		return nil, fmt.Errorf("motion: unknown likelihood %q", cfg.Likelihood.Kind)
	}
	if cfg.Likelihood.Nu < 0 {
		return nil, fmt.Errorf("motion: Student-t degrees of freedom must be positive")
	}
	mu := cfg.InitialProb
	if mu == ([nModes]float64{}) {
		mu = rates.Stationary()
	}
	if !(mu[0] >= 0) || !(mu[1] >= 0) || math.Abs(mu[0]+mu[1]-1) > 1e-9 {
		return nil, fmt.Errorf("motion: initial mode probabilities must be a distribution, got %v", mu)
	}
	return &IMM{cfg: cfg, rates: rates, modes: [nModes]*Filter{cv, ca}, mu: mu}, nil
}

// Rates returns the switching generator in use.
func (m *IMM) Rates() ModeRates { return m.rates }

// ModeProb returns the current mode probabilities.
func (m *IMM) ModeProb() [nModes]float64 { return m.mu }

// Initialised reports whether the IMM holds a state.
func (m *IMM) Initialised() bool { return m.modes[ModeCV].Initialised() }

// Current returns the combined posterior.
func (m *IMM) Current() Estimate {
	return combine(m.mu, [nModes]Estimate{m.modes[0].Current(), m.modes[1].Current()})
}

// Clone returns an independent copy.
func (m *IMM) Clone() *IMM {
	c := *m
	for j := range c.modes {
		c.modes[j] = m.modes[j].Clone()
	}
	return &c
}

// mix computes the transition, predicted probabilities, mixing weights and
// mixed estimates for an interval.
func (m *IMM) mix(dtSecs float64) (pi [nModes][nModes]float64, c [nModes]float64, w [nModes][nModes]float64, mixed [nModes]Estimate) {
	pi = m.rates.Transition(dtSecs)
	ests := [nModes]Estimate{m.modes[0].Current(), m.modes[1].Current()}
	for j := 0; j < nModes; j++ {
		for i := 0; i < nModes; i++ {
			c[j] += pi[i][j] * m.mu[i]
		}
	}
	for j := 0; j < nModes; j++ {
		for i := 0; i < nModes; i++ {
			if c[j] > 0 {
				w[i][j] = pi[i][j] * m.mu[i] / c[j]
			}
		}
		var weights [nModes]float64
		for i := 0; i < nModes; i++ {
			weights[i] = w[i][j]
		}
		mixed[j] = combine(weights, ests)
	}
	return pi, c, w, mixed
}

// combine is the moment-matched mixture Σ w_i N(x_i, P_i).
func combine(w [nModes]float64, ests [nModes]Estimate) Estimate {
	out := Estimate{UnixNanos: ests[0].UnixNanos}
	for i := 0; i < nModes; i++ {
		for s := 0; s < StateDim; s++ {
			out.X[s] += w[i] * ests[i].X[s]
		}
	}
	for i := 0; i < nModes; i++ {
		if w[i] == 0 {
			continue
		}
		d := ests[i].X.sub(out.X)
		out.P = out.P.add(ests[i].P.add(outer(d, d)).scale(w[i]))
	}
	out.P = Symmetrise(out.P)
	return out
}

// gapCourse is the course both modes hold across an unobserved interval:
// the combined estimate's at the gap's start.
func (m *IMM) gapCourse() *course {
	u, ok := m.modes[ModeCA].courseOf(m.Current().X)
	if !ok {
		return nil
	}
	return &u
}

// Step runs one IMM cycle on an observation. On an error the IMM is
// unchanged.
func (m *IMM) Step(obs Observation) (IMMStep, error) {
	if !m.Initialised() {
		var out IMMStep
		for j, f := range m.modes {
			s, err := f.Step(obs)
			if err != nil {
				return IMMStep{}, err
			}
			out.Modes[j] = s
		}
		out.UnixNanos, out.Founded, out.ModeProb, out.PredictedProb = obs.UnixNanos, true, m.mu, m.mu
		out.Prior, out.Posterior = m.Current(), m.Current()
		return out, nil
	}
	start := m.modes[0].Current().UnixNanos
	if obs.UnixNanos < start {
		return IMMStep{}, ErrTimeOrder
	}
	dt := float64(obs.UnixNanos-start) / 1e9
	gc := m.gapCourse()
	pi, c, w, mixed := m.mix(dt)
	out := IMMStep{UnixNanos: obs.UnixNanos, DtSecs: dt, Transition: pi, PredictedProb: c, Mixing: w}

	var priors, posts [nModes]Estimate
	updated := [nModes]bool{}
	for j, f := range m.modes {
		trial := f.Clone()
		trial.SetState(mixed[j])
		prior, info := trial.predict(trial.est, obs.UnixNanos, gc)
		if f.cfg.Model.Kind == ModelCV {
			prior.X, prior.P = zeroAcceleration(prior.X, prior.P)
		}
		priors[j] = prior
		step := Step{
			UnixNanos: obs.UnixNanos, DtSecs: info.dtSecs, SubSteps: info.subSteps, Coasted: info.coasted,
			Constrained: info.constrained, ConstrainedAfterSecs: info.constrainedAfter, Prior: prior,
		}
		x, p, in, err := josephUpdate(prior.X, prior.P, obs)
		if err != nil {
			out.LogLikelihood[j] = math.Inf(-1)
			step.Posterior = prior
			posts[j] = prior
		} else {
			post := Estimate{UnixNanos: obs.UnixNanos, X: x, P: p}
			if f.cfg.Model.Kind == ModelCV {
				post.X, post.P = zeroAcceleration(post.X, post.P)
			}
			step.Posterior, step.Innovation = post, in
			step.LogLikelihood = GaussianLogLikelihood(in)
			out.LogLikelihood[j] = m.cfg.Likelihood.LogDensity(in)
			posts[j] = post
			updated[j] = true
		}
		out.Modes[j] = step
	}
	if !updated[0] && !updated[1] {
		return IMMStep{}, ErrSingularInnovation
	}
	mu, err := modeUpdate(c, out.LogLikelihood)
	if err != nil {
		return IMMStep{}, err
	}
	out.ModeProb = mu
	out.Prior = combine(c, priors)
	if in, _, _, _, err := innovationFor(out.Prior.X, out.Prior.P, obs); err == nil {
		out.Innovation = in
	}
	for j, f := range m.modes {
		f.est, f.initialised = posts[j], true
	}
	m.mu = mu
	out.Posterior = m.Current()
	return out, nil
}

// modeUpdate returns μ_j ∝ c_j exp(ℓ_j), normalised in the log domain so a
// large innovation cannot underflow both terms to zero.
func modeUpdate(c [nModes]float64, logL [nModes]float64) ([nModes]float64, error) {
	var terms [nModes]float64
	maxTerm := math.Inf(-1)
	for j := 0; j < nModes; j++ {
		terms[j] = math.Inf(-1)
		if c[j] > 0 && !math.IsInf(logL[j], -1) && !math.IsNaN(logL[j]) {
			terms[j] = math.Log(c[j]) + logL[j]
		}
		maxTerm = math.Max(maxTerm, terms[j])
	}
	if math.IsInf(maxTerm, -1) {
		return [nModes]float64{}, fmt.Errorf("motion: no mode has a defined likelihood")
	}
	var mu [nModes]float64
	sum := 0.0
	for j := 0; j < nModes; j++ {
		mu[j] = math.Exp(terms[j] - maxTerm)
		sum += mu[j]
	}
	for j := range mu {
		mu[j] /= sum
	}
	return mu, nil
}

// Predict returns the combined IMM prior at unixNanos, weighted by the
// predicted mode probabilities, without changing the IMM.
func (m *IMM) Predict(unixNanos int64) (Estimate, [nModes]float64, error) {
	if !m.Initialised() {
		return Estimate{}, [nModes]float64{}, fmt.Errorf("motion: IMM is not initialised")
	}
	start := m.modes[0].Current().UnixNanos
	if unixNanos < start {
		return Estimate{}, [nModes]float64{}, ErrTimeOrder
	}
	gc := m.gapCourse()
	_, c, _, mixed := m.mix(float64(unixNanos-start) / 1e9)
	var priors [nModes]Estimate
	for j, f := range m.modes {
		trial := f.Clone()
		trial.SetState(mixed[j])
		priors[j], _ = trial.predict(trial.est, unixNanos, gc)
		if f.cfg.Model.Kind == ModelCV {
			priors[j].X, priors[j].P = zeroAcceleration(priors[j].X, priors[j].P)
		}
	}
	return combine(c, priors), c, nil
}

// RunIMM runs a fresh IMM over a track's observations in order.
func RunIMM(cfg IMMConfig, observations []Observation) ([]IMMStep, error) {
	m, err := NewIMM(cfg)
	if err != nil {
		return nil, err
	}
	steps := make([]IMMStep, 0, len(observations))
	for i, obs := range observations {
		step, err := m.Step(obs)
		if err != nil {
			return steps, fmt.Errorf("motion: observation %d: %w", i, err)
		}
		steps = append(steps, step)
	}
	return steps, nil
}
