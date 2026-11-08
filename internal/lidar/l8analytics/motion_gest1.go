package l8analytics

// G-EST-1 analyses (state estimation plan, Section 7.3): the offline tests
// that decide whether the tracker should move from constant velocity to
// constant acceleration. They read observation sequences and the offline
// estimators in l5tracks/motion; nothing here runs in the live tracker.
//
//   - Criterion 2, PartialCorrelation: is CV's NIS over-dispersion correlated
//     with acceleration once range, aspect and point count are controlled for?
//   - Criterion 3, RunLengthSignature: what share of tracks show at least
//     three consecutive same-sign longitudinal residuals beyond one sigma?
//   - Criterion 5, ScoreOcclusion: across a synthetic occlusion, is each
//     model's reacquisition error no worse than CV's, overall and for gaps
//     that begin while the track decelerates?
//
// Definitions and their limits are in data/maths/motion-models-maths.md.

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks/motion"
	"github.com/banshee-data/velocity.report/internal/stats"
)

// ---------------------------------------------------------------------------
// Criterion 2: partial correlation of NIS with acceleration.
// ---------------------------------------------------------------------------

// PartialCorrelationResult is a partial correlation with its Fisher-z
// interval.
type PartialCorrelationResult struct {
	// R is the Pearson correlation of the two OLS residuals.
	R float64
	// N is the number of samples; Covariates the number of covariate
	// columns that remained linearly independent and were controlled for.
	N, Covariates int
	// Lo and Hi bound R at Level by the Fisher z transform, with standard
	// error 1/√(N − Covariates − 3).
	Lo, Hi, Level float64
	// DroppedCovariates counts columns removed as collinear with the
	// intercept and the earlier columns.
	DroppedCovariates int
}

// ErrTooFewSamples is returned when N − covariates − 3 leaves no degrees of
// freedom for the Fisher interval.
var ErrTooFewSamples = errors.New("l8analytics: too few samples for a partial correlation")

// PartialCorrelation returns the partial correlation of y and x controlling
// for the covariate columns (each covariates[k] has one value per sample).
// Both y and x are residualised by ordinary least squares on an intercept and
// the covariates (modified Gram–Schmidt, so a near-collinear column is
// detected and dropped rather than amplified); R is the Pearson correlation
// of the residuals. level is the interval's coverage, 0.95 if zero.
func PartialCorrelation(y, x []float64, covariates [][]float64, level float64) (PartialCorrelationResult, error) {
	n := len(y)
	if len(x) != n {
		return PartialCorrelationResult{}, fmt.Errorf("l8analytics: %d responses against %d predictors", n, len(x))
	}
	for k, c := range covariates {
		if len(c) != n {
			return PartialCorrelationResult{}, fmt.Errorf("l8analytics: covariate %d has %d values, want %d", k, len(c), n)
		}
	}
	if level == 0 {
		level = 0.95
	}
	if !(level > 0 && level < 1) {
		return PartialCorrelationResult{}, fmt.Errorf("l8analytics: interval level %v outside (0, 1)", level)
	}
	for i := 0; i < n; i++ {
		if !finite(y[i]) || !finite(x[i]) {
			return PartialCorrelationResult{}, fmt.Errorf("l8analytics: sample %d is not finite", i)
		}
		for k := range covariates {
			if !finite(covariates[k][i]) {
				return PartialCorrelationResult{}, fmt.Errorf("l8analytics: covariate %d sample %d is not finite", k, i)
			}
		}
	}

	// Orthonormal basis of [1, c1, …, ck].
	basis := make([][]float64, 0, len(covariates)+1)
	ones := make([]float64, n)
	for i := range ones {
		ones[i] = 1
	}
	dropped := 0
	for k, col := range append([][]float64{ones}, covariates...) {
		v := append([]float64(nil), col...)
		norm0 := l2(v)
		for _, q := range basis {
			project(v, q)
		}
		norm := l2(v)
		if norm0 == 0 || norm <= 1e-10*norm0 {
			if k == 0 {
				return PartialCorrelationResult{}, ErrTooFewSamples
			}
			dropped++
			continue
		}
		for i := range v {
			v[i] /= norm
		}
		basis = append(basis, v)
	}
	k := len(basis) - 1
	res := PartialCorrelationResult{N: n, Covariates: k, Level: level, DroppedCovariates: dropped}
	if n-k-3 <= 0 {
		return res, ErrTooFewSamples
	}
	ry := append([]float64(nil), y...)
	rx := append([]float64(nil), x...)
	for _, q := range basis {
		project(ry, q)
		project(rx, q)
	}
	sy, sx := l2(ry), l2(rx)
	if sy == 0 || sx == 0 {
		return res, fmt.Errorf("l8analytics: a residual is identically zero; the covariates explain it exactly")
	}
	var sxy float64
	for i := range ry {
		sxy += ry[i] * rx[i]
	}
	res.R = math.Max(-1, math.Min(1, sxy/(sy*sx)))
	zc := math.Sqrt2 * math.Erfinv(level)
	z := math.Atanh(math.Max(-1+1e-15, math.Min(1-1e-15, res.R)))
	se := 1 / math.Sqrt(float64(n-k-3))
	res.Lo, res.Hi = math.Tanh(z-zc*se), math.Tanh(z+zc*se)
	return res, nil
}

// project removes v's component along the unit vector q, in place.
func project(v, q []float64) {
	var d float64
	for i := range v {
		d += v[i] * q[i]
	}
	for i := range v {
		v[i] -= d * q[i]
	}
}

func l2(v []float64) float64 {
	var s float64
	for _, x := range v {
		s += x * x
	}
	return math.Sqrt(s)
}

// NISAccelerationSample is one observation's entry in criterion 2.
type NISAccelerationSample struct {
	// NIS is the CV filter's normalised innovation squared.
	NIS float64
	// Acceleration is a smoothed acceleration estimate at the observation,
	// for example |longitudinal| from SmoothedKinematics.
	Acceleration float64
	// Range (m), Aspect (rad, 0 end-on to π/2 broadside) and Points are the
	// geometry covariates criterion 2 controls for.
	Range, Aspect float64
	Points        int
}

// NISAccelerationCorrelation is criterion 2: the partial correlation of NIS
// with acceleration, controlling for range, aspect and point count.
func NISAccelerationCorrelation(samples []NISAccelerationSample, level float64) (PartialCorrelationResult, error) {
	n := len(samples)
	y, x := make([]float64, n), make([]float64, n)
	cov := [][]float64{make([]float64, n), make([]float64, n), make([]float64, n)}
	for i, s := range samples {
		y[i], x[i] = s.NIS, s.Acceleration
		cov[0][i], cov[1][i], cov[2][i] = s.Range, s.Aspect, float64(s.Points)
	}
	return PartialCorrelation(y, x, cov, level)
}

// RangeAndAspect returns the range from the sensor and the aspect angle, the
// angle between the course and the sensor's line of sight folded into
// [0, π/2]: 0 when the track moves along the ray (end-on), π/2 broadside.
// The aspect is NaN when the speed is zero.
func RangeAndAspect(x, y, vx, vy, sensorX, sensorY float64) (rangeM, aspect float64) {
	dx, dy := x-sensorX, y-sensorY
	rangeM = math.Hypot(dx, dy)
	speed := math.Hypot(vx, vy)
	if speed == 0 || rangeM == 0 {
		return rangeM, math.NaN()
	}
	c := math.Abs(dx*vx+dy*vy) / (rangeM * speed)
	return rangeM, math.Acos(math.Min(1, c))
}

// Kinematics is a local polynomial estimate of a track's motion.
type Kinematics struct {
	VX, VY, AX, AY float64
	// N is the number of observations the fit used.
	N int
}

// Longitudinal returns the acceleration along the velocity, or NaN when the
// speed is below minSpeed (no course to project on).
func (k Kinematics) Longitudinal(minSpeed float64) float64 {
	speed := math.Hypot(k.VX, k.VY)
	if speed == 0 || speed < minSpeed {
		return math.NaN()
	}
	return (k.AX*k.VX + k.AY*k.VY) / speed
}

// SmoothedKinematics fits a quadratic in time to each coordinate of the
// observations within ±halfWindowSecs of observation i, by unweighted least
// squares, and returns the fitted velocity and acceleration at its time. It
// needs at least five observations in the window. The fit uses observations
// on both sides of i: it is a smoothed (non-causal) estimate, which is what
// a label for "this track was decelerating here" should be, and why it is
// independent of every model being scored.
func SmoothedKinematics(obs []motion.Observation, i int, halfWindowSecs float64) (Kinematics, bool) {
	if i < 0 || i >= len(obs) || !(halfWindowSecs > 0) {
		return Kinematics{}, false
	}
	t0 := obs[i].UnixNanos
	half := int64(halfWindowSecs * 1e9)
	// Normal equations of [1, τ, τ²/2] in τ = t − t0, per axis.
	var a [3][3]float64
	var bx, by [3]float64
	n := 0
	for j := i; j >= 0 && t0-obs[j].UnixNanos <= half; j-- {
		accumulate(&a, &bx, &by, float64(obs[j].UnixNanos-t0)/1e9, obs[j].X, obs[j].Y)
		n++
	}
	for j := i + 1; j < len(obs) && obs[j].UnixNanos-t0 <= half; j++ {
		accumulate(&a, &bx, &by, float64(obs[j].UnixNanos-t0)/1e9, obs[j].X, obs[j].Y)
		n++
	}
	if n < 5 {
		return Kinematics{}, false
	}
	cx, okx := solve3(a, bx)
	cy, oky := solve3(a, by)
	if !okx || !oky {
		return Kinematics{}, false
	}
	return Kinematics{VX: cx[1], VY: cy[1], AX: cx[2], AY: cy[2], N: n}, true
}

func accumulate(a *[3][3]float64, bx, by *[3]float64, tau, x, y float64) {
	row := [3]float64{1, tau, tau * tau / 2}
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			a[r][c] += row[r] * row[c]
		}
		bx[r] += row[r] * x
		by[r] += row[r] * y
	}
}

// solve3 solves a 3×3 system by Gaussian elimination with partial pivoting.
func solve3(a [3][3]float64, b [3]float64) ([3]float64, bool) {
	scale := math.Max(math.Abs(a[0][0]), math.Max(math.Abs(a[1][1]), math.Abs(a[2][2])))
	for c := 0; c < 3; c++ {
		p := c
		for r := c + 1; r < 3; r++ {
			if math.Abs(a[r][c]) > math.Abs(a[p][c]) {
				p = r
			}
		}
		if math.Abs(a[p][c]) <= 1e-12*scale {
			return b, false
		}
		a[c], a[p] = a[p], a[c]
		b[c], b[p] = b[p], b[c]
		for r := c + 1; r < 3; r++ {
			f := a[r][c] / a[c][c]
			for k := c; k < 3; k++ {
				a[r][k] -= f * a[c][k]
			}
			b[r] -= f * b[c]
		}
	}
	var out [3]float64
	for r := 2; r >= 0; r-- {
		s := b[r]
		for k := r + 1; k < 3; k++ {
			s -= a[r][k] * out[k]
		}
		out[r] = s / a[r][r]
	}
	return out, true
}

// ---------------------------------------------------------------------------
// Criterion 3: run-length signature of longitudinal residuals.
// ---------------------------------------------------------------------------

// LongitudinalResiduals returns each step's innovation along the prior
// velocity's direction, normalised by its own predicted sd: uᵀy / √(uᵀSu).
// Founding steps, rank-one innovations and steps whose prior speed is below
// minSpeed (no course) are skipped, so the sequence is the observations that
// have a longitudinal axis, in order.
func LongitudinalResiduals(steps []motion.Step, minSpeed float64) []float64 {
	out := make([]float64, 0, len(steps))
	for _, s := range steps {
		if s.Founded || s.Innovation.Dim != 2 {
			continue
		}
		vx, vy := s.Prior.X[motion.IdxVX], s.Prior.X[motion.IdxVY]
		speed := math.Hypot(vx, vy)
		if speed == 0 || speed < minSpeed {
			continue
		}
		ux, uy := vx/speed, vy/speed
		in := s.Innovation
		v := ux*ux*in.S[0][0] + ux*uy*(in.S[0][1]+in.S[1][0]) + uy*uy*in.S[1][1]
		if !(v > 0) {
			continue
		}
		out = append(out, (ux*in.Y[0]+uy*in.Y[1])/math.Sqrt(v))
	}
	return out
}

// RunLengthResult is criterion 3 over a set of tracks.
type RunLengthResult struct {
	// Tracks is the number of non-empty residual sequences offered; Eligible
	// those long enough to hold a run of MinRun; Flagged those holding one.
	Tracks, Eligible, Flagged int
	// Share is Flagged / Tracks: the criterion's "share of moving tracks".
	Share     float64
	MinRun    int
	Threshold float64
}

// RunLengthSignature counts the tracks whose normalised longitudinal
// residuals contain at least minRun consecutive values of the same sign each
// beyond threshold in magnitude (G-EST-1: 3 and 1 sigma). A value inside the
// threshold, or a change of sign, ends a run. Zero minRun or threshold take
// the criterion's 3 and 1.
func RunLengthSignature(tracks [][]float64, minRun int, threshold float64) RunLengthResult {
	if minRun <= 0 {
		minRun = 3
	}
	if threshold <= 0 {
		threshold = 1
	}
	res := RunLengthResult{MinRun: minRun, Threshold: threshold}
	for _, z := range tracks {
		if len(z) == 0 {
			continue
		}
		res.Tracks++
		if len(z) >= minRun {
			res.Eligible++
		}
		if longestSameSignRun(z, threshold) >= minRun {
			res.Flagged++
		}
	}
	if res.Tracks > 0 {
		res.Share = float64(res.Flagged) / float64(res.Tracks)
	}
	return res
}

func longestSameSignRun(z []float64, threshold float64) int {
	best, run, sign := 0, 0, 0.0
	for _, v := range z {
		switch {
		case v > threshold:
			if sign > 0 {
				run++
			} else {
				run, sign = 1, 1
			}
		case v < -threshold:
			if sign < 0 {
				run++
			} else {
				run, sign = 1, -1
			}
		default:
			run, sign = 0, 0
		}
		if run > best {
			best = run
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// Criterion 5: synthetic occlusion.
// ---------------------------------------------------------------------------

// OcclusionModel names an estimator and makes a fresh one per track.
type OcclusionModel struct {
	Name string
	New  func() (motion.Estimator, error)
}

// DefaultOcclusionModels are the four arms criterion 5 compares: CV, CA with
// the coast rule, CA without it, and the IMM, each at its offline default.
func DefaultOcclusionModels() []OcclusionModel {
	filter := func(cfg motion.FilterConfig) func() (motion.Estimator, error) {
		return func() (motion.Estimator, error) { return motion.NewFilter(cfg) }
	}
	bare := motion.DefaultCAConfig()
	bare.Coast = motion.CoastRule{}
	return []OcclusionModel{
		{Name: "cv", New: filter(motion.DefaultCVConfig())},
		{Name: "ca_coast", New: filter(motion.DefaultCAConfig())},
		{Name: "ca_no_coast", New: filter(bare)},
		{Name: "imm", New: func() (motion.Estimator, error) { return motion.NewIMM(motion.DefaultIMMConfig()) }},
	}
}

// OcclusionConfig sets where synthetic gaps are cut and how they are split.
type OcclusionConfig struct {
	// HorizonsSecs are the gap lengths; G-EST-1 names 0.5 s, 1.0 s and the
	// longest permitted coast.
	HorizonsSecs []float64
	// WarmupSecs is the observed history a track needs before a gap starts.
	WarmupSecs float64
	// StrideSecs is the least spacing between gap starts in one track, so
	// cases from one track are not near-duplicates.
	StrideSecs float64
	// MaxOvershootSecs bounds how far past the horizon the first observation
	// after the gap may fall; a start without one inside it is skipped.
	MaxOvershootSecs float64
	// DecelerationThreshold (m/s², negative) classes a gap as starting while
	// decelerating when the smoothed longitudinal acceleration at its start
	// is below it.
	DecelerationThreshold float64
	// AccelHalfWindowSecs is SmoothedKinematics' half-window for that label.
	AccelHalfWindowSecs float64
	// MinSpeed is the speed below which a gap start has no course and is
	// not classed as decelerating.
	MinSpeed float64
}

// DefaultOcclusionConfig is criterion 5's 0.5 s, 1.0 s and maxCoastSecs
// horizons with the analysis defaults.
func DefaultOcclusionConfig(maxCoastSecs float64) OcclusionConfig {
	return OcclusionConfig{
		HorizonsSecs:          []float64{0.5, 1.0, maxCoastSecs},
		WarmupSecs:            1.0,
		StrideSecs:            1.0,
		MaxOvershootSecs:      0.25,
		DecelerationThreshold: -0.5,
		AccelHalfWindowSecs:   0.5,
		MinSpeed:              0.3,
	}
}

// OcclusionCase is one synthetic gap and every model's error across it.
type OcclusionCase struct {
	TrackID          string
	CreationSequence int64
	HorizonSecs      float64
	// StartUnixNanos is the last observation before the gap; EndUnixNanos the
	// first after it, the one scored against. GapSecs is their difference.
	StartUnixNanos, EndUnixNanos int64
	GapSecs                      float64
	// LongitudinalAccel is the smoothed acceleration along the course at the
	// gap's start (NaN when unavailable); Decelerating its class.
	LongitudinalAccel float64
	Decelerating      bool
	// Errors[m] is model m's Euclidean distance from the predicted to the
	// observed position; LongitudinalErrors[m] its signed component along
	// the smoothed course at the start (positive: predicted ahead), NaN
	// without a course.
	Errors             []float64
	LongitudinalErrors []float64
}

// Occlusion summary splits.
const (
	SplitAll          = "all"
	SplitDecelerating = "decelerating"
)

// OcclusionSummary is one model's error distribution at one horizon.
type OcclusionSummary struct {
	Model       string
	HorizonSecs float64
	Split       string
	N           int
	// Median and P95 are nearest-rank (stats.NearestRank, decision E-03).
	Median, P95 float64
}

// OcclusionReport is criterion 5's result.
type OcclusionReport struct {
	Models  []string
	Cases   []OcclusionCase
	Summary []OcclusionSummary
}

// ScoreOcclusion cuts synthetic gaps into each track, predicts across each
// gap from the state at its start with every model, and scores the predicted
// position against the first observation after it.
//
// Gap starts are chosen once per track, from the observations alone, so every
// model is scored on the same cases: an observation at least WarmupSecs after
// the track's first and at least StrideSecs after the previous start. For each
// start and horizon H the gap ends at the first observation at or after
// start + H, provided it is within MaxOvershootSecs of that time. Each model
// runs once over the whole track; at a start it is cloned and the clone
// predicts to the gap's end with nothing observed in between, which is
// exactly what withholding the gap's observations would give.
func ScoreOcclusion(tracks []motion.TrackObservations, models []OcclusionModel, cfg OcclusionConfig) (OcclusionReport, error) {
	if len(models) == 0 || len(cfg.HorizonsSecs) == 0 {
		return OcclusionReport{}, fmt.Errorf("l8analytics: occlusion scoring needs models and horizons")
	}
	for _, h := range cfg.HorizonsSecs {
		if !(h > 0) {
			return OcclusionReport{}, fmt.Errorf("l8analytics: horizon %v is not positive", h)
		}
	}
	report := OcclusionReport{}
	for _, m := range models {
		report.Models = append(report.Models, m.Name)
	}
	for _, track := range tracks {
		cases, err := occlusionCases(track, models, cfg)
		if err != nil {
			return OcclusionReport{}, fmt.Errorf("l8analytics: track %s/%d: %w", track.TrackID, track.CreationSequence, err)
		}
		report.Cases = append(report.Cases, cases...)
	}
	report.Summary = summariseOcclusion(report, cfg.HorizonsSecs)
	return report, nil
}

type gapCut struct {
	start, end int
	horizon    float64
}

func occlusionCases(track motion.TrackObservations, models []OcclusionModel, cfg OcclusionConfig) ([]OcclusionCase, error) {
	obs := track.Observations
	if len(obs) < 2 {
		return nil, nil
	}
	warm := int64(cfg.WarmupSecs * 1e9)
	stride := int64(cfg.StrideSecs * 1e9)
	overshoot := int64(cfg.MaxOvershootSecs * 1e9)
	var cuts []gapCut
	starts := map[int]bool{}
	last := int64(math.MinInt64)
	for k := range obs {
		if obs[k].UnixNanos-obs[0].UnixNanos < warm {
			continue
		}
		if last != math.MinInt64 && obs[k].UnixNanos-last < stride {
			continue
		}
		found := false
		for _, h := range cfg.HorizonsSecs {
			target := obs[k].UnixNanos + int64(math.Round(h*1e9))
			j := sort.Search(len(obs), func(i int) bool { return obs[i].UnixNanos >= target })
			if j < len(obs) && obs[j].UnixNanos-target <= overshoot {
				cuts = append(cuts, gapCut{start: k, end: j, horizon: h})
				found = true
			}
		}
		if found {
			starts[k] = true
			last = obs[k].UnixNanos
		}
	}
	if len(cuts) == 0 {
		return nil, nil
	}
	cases := make([]OcclusionCase, len(cuts))
	for c, cut := range cuts {
		s, e := obs[cut.start], obs[cut.end]
		oc := OcclusionCase{
			TrackID: track.TrackID, CreationSequence: track.CreationSequence, HorizonSecs: cut.horizon,
			StartUnixNanos: s.UnixNanos, EndUnixNanos: e.UnixNanos, GapSecs: float64(e.UnixNanos-s.UnixNanos) / 1e9,
			LongitudinalAccel: math.NaN(),
			Errors:            make([]float64, len(models)), LongitudinalErrors: make([]float64, len(models)),
		}
		if k, ok := SmoothedKinematics(obs, cut.start, cfg.AccelHalfWindowSecs); ok {
			oc.LongitudinalAccel = k.Longitudinal(cfg.MinSpeed)
			oc.Decelerating = !math.IsNaN(oc.LongitudinalAccel) && oc.LongitudinalAccel < cfg.DecelerationThreshold
		}
		cases[c] = oc
	}
	for m, model := range models {
		est, err := model.New()
		if err != nil {
			return nil, fmt.Errorf("model %s: %w", model.Name, err)
		}
		for k := range obs {
			if err := est.Observe(obs[k]); err != nil {
				return nil, fmt.Errorf("model %s, observation %d: %w", model.Name, k, err)
			}
			if !starts[k] {
				continue
			}
			ux, uy, hasCourse := courseAt(obs, k, cfg)
			for c, cut := range cuts {
				if cut.start != k {
					continue
				}
				e := obs[cut.end]
				px, py, err := est.CloneEstimator().PredictPosition(e.UnixNanos)
				if err != nil {
					return nil, fmt.Errorf("model %s, gap at %d: %w", model.Name, k, err)
				}
				cases[c].Errors[m] = math.Hypot(px-e.X, py-e.Y)
				cases[c].LongitudinalErrors[m] = math.NaN()
				if hasCourse {
					cases[c].LongitudinalErrors[m] = ux*(px-e.X) + uy*(py-e.Y)
				}
			}
		}
	}
	return cases, nil
}

func courseAt(obs []motion.Observation, k int, cfg OcclusionConfig) (float64, float64, bool) {
	kin, ok := SmoothedKinematics(obs, k, cfg.AccelHalfWindowSecs)
	if !ok {
		return 0, 0, false
	}
	speed := math.Hypot(kin.VX, kin.VY)
	if speed == 0 || speed < cfg.MinSpeed {
		return 0, 0, false
	}
	return kin.VX / speed, kin.VY / speed, true
}

func summariseOcclusion(r OcclusionReport, horizons []float64) []OcclusionSummary {
	var out []OcclusionSummary
	for _, h := range horizons {
		for _, split := range []string{SplitAll, SplitDecelerating} {
			for m, name := range r.Models {
				var errs []float64
				for _, c := range r.Cases {
					if c.HorizonSecs != h || (split == SplitDecelerating && !c.Decelerating) {
						continue
					}
					errs = append(errs, c.Errors[m])
				}
				if len(errs) == 0 {
					continue
				}
				sort.Float64s(errs)
				out = append(out, OcclusionSummary{
					Model: name, HorizonSecs: h, Split: split, N: len(errs),
					Median: stats.NearestRank(errs, 0.5), P95: stats.NearestRank(errs, 0.95),
				})
			}
		}
	}
	return out
}

// NoWorseThan applies criterion 5's comparison: for every horizon and split
// where both models have cases, candidate's median and p95 must not exceed
// baseline's. It returns the violations, empty when the candidate passes.
func (r OcclusionReport) NoWorseThan(candidate, baseline string) []string {
	type key struct {
		h     float64
		split string
	}
	base := map[key]OcclusionSummary{}
	for _, s := range r.Summary {
		if s.Model == baseline {
			base[key{s.HorizonSecs, s.Split}] = s
		}
	}
	var out []string
	for _, s := range r.Summary {
		if s.Model != candidate {
			continue
		}
		b, ok := base[key{s.HorizonSecs, s.Split}]
		if !ok {
			continue
		}
		if s.Median > b.Median {
			out = append(out, fmt.Sprintf("%s %.2f s %s: median %.3f m exceeds %s's %.3f m", candidate, s.HorizonSecs, s.Split, s.Median, baseline, b.Median))
		}
		if s.P95 > b.P95 {
			out = append(out, fmt.Sprintf("%s %.2f s %s: p95 %.3f m exceeds %s's %.3f m", candidate, s.HorizonSecs, s.Split, s.P95, baseline, b.P95))
		}
	}
	return out
}
