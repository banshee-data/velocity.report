package egotrack

import (
	"errors"
	"fmt"
	"math"
)

// Options sets the estimator. Every field has a documented default applied by
// withDefaults; the summary records the values used.
type Options struct {
	// HalfWindowS is the half-width of the local polynomial fit, in seconds.
	// Wider windows lower derivative noise and attenuate faster motion: a
	// local cubic over ±h passes content well below 1/(2h) Hz. Default 0.5.
	HalfWindowS float64 `json:"half_window_s"`
	// Degree of the local polynomial. Jerk needs 3. Default 3.
	Degree int `json:"degree"`
	// DefaultSigmaPosM is the per-axis 1σ position noise assumed for samples
	// whose quality states none. Default 0.02 m.
	DefaultSigmaPosM float64 `json:"default_sigma_pos_m"`
	// SigmaYawRad is the 1σ yaw noise per sample used for yaw rate. Default
	// 0.002 rad (0.11°).
	SigmaYawRad float64 `json:"sigma_yaw_rad"`
	// GapFactor splits the trajectory where an interval exceeds this many
	// median intervals. Default 3.
	GapFactor float64 `json:"gap_factor"`
	// MinHeadingSpeedMS is the speed below which the direction of travel is
	// too uncertain to split acceleration and jerk into longitudinal and
	// lateral parts. Default 1.0 m/s.
	MinHeadingSpeedMS float64 `json:"min_heading_speed_ms"`
	// StopSpeedMS and StopMinS define a stop: speed below StopSpeedMS for at
	// least StopMinS. Defaults 0.3 m/s and 2 s.
	StopSpeedMS float64 `json:"stop_speed_ms"`
	StopMinS    float64 `json:"stop_min_s"`
	// SpectrumSegmentS is the Welch segment length in seconds, so frequency
	// resolution (1/SpectrumSegmentS) does not change with the rotation
	// rate: 64 samples at 10 Hz, 128 at 20 Hz. Default 6.4 s.
	SpectrumSegmentS float64 `json:"spectrum_segment_s"`
	// AnchorWindowS is how much of each end is averaged for an anchor check.
	// Default 5 s.
	AnchorWindowS float64 `json:"anchor_window_s"`
}

func (o Options) withDefaults() Options {
	if o.HalfWindowS <= 0 {
		o.HalfWindowS = 0.5
	}
	if o.Degree <= 0 {
		o.Degree = 3
	}
	if o.DefaultSigmaPosM <= 0 {
		o.DefaultSigmaPosM = 0.02
	}
	if o.SigmaYawRad <= 0 {
		o.SigmaYawRad = 0.002
	}
	if o.GapFactor <= 0 {
		o.GapFactor = 3
	}
	if o.MinHeadingSpeedMS <= 0 {
		o.MinHeadingSpeedMS = 1.0
	}
	if o.StopSpeedMS <= 0 {
		o.StopSpeedMS = 0.3
	}
	if o.StopMinS <= 0 {
		o.StopMinS = 2
	}
	if o.SpectrumSegmentS <= 0 {
		o.SpectrumSegmentS = 6.4
	}
	if o.AnchorWindowS <= 0 {
		o.AnchorWindowS = 5
	}
	return o
}

// Kinematics is the derived motion at one sample. Sigma fields are 1σ under
// the noise model in Derive's comment. Longitudinal and lateral parts and the
// course are meaningful only when HeadingValid.
type Kinematics struct {
	Frame  int     `json:"frame"`
	T      float64 `json:"t_s"`
	Valid  bool    `json:"valid"`
	Reason string  `json:"reason,omitempty"`

	Vel        [3]float64 `json:"vel_ms"`
	Speed      float64    `json:"speed_ms"`
	SpeedSigma float64    `json:"speed_sigma_ms"`

	HeadingValid bool    `json:"heading_valid"`
	CourseRad    float64 `json:"course_rad"` // direction of travel in the frame, from +x towards +y

	AccelNorm  float64 `json:"accel_norm_ms2"`
	AccelLong  float64 `json:"accel_long_ms2"`
	AccelLat   float64 `json:"accel_lat_ms2"` // positive to the left of travel
	AccelSigma float64 `json:"accel_sigma_ms2"`

	JerkNorm  float64 `json:"jerk_norm_ms3"`
	JerkLong  float64 `json:"jerk_long_ms3"`
	JerkSigma float64 `json:"jerk_sigma_ms3"`

	YawRate      float64 `json:"yaw_rate_rads"`
	YawRateSigma float64 `json:"yaw_rate_sigma_rads"`

	// Residuals of z, pitch and roll about their local fit: the vibration
	// the smooth path does not explain, used by the jitter spectrum.
	ResidZ     float64 `json:"resid_z_m"`
	ResidPitch float64 `json:"resid_pitch_rad"`
	ResidRoll  float64 `json:"resid_roll_rad"`
}

// localFit is a weighted least-squares polynomial in τ = (t - t0)/h.
type localFit struct {
	c   []float64   // coefficients in τ
	cov [][]float64 // covariance of c under the stated weights
}

// fitLocal fits value(t) over the given samples. sigma[j] is the 1σ noise of
// values[j]. It returns an error when the window cannot support the degree.
func fitLocal(t, values, sigma []float64, t0, h float64, degree int) (localFit, error) {
	n := degree + 1
	if len(t) < n+1 {
		return localFit{}, fmt.Errorf("window has %d samples, need at least %d", len(t), n+1)
	}
	normal := make([][]float64, n)
	for i := range normal {
		normal[i] = make([]float64, n)
	}
	rhs := make([]float64, n)
	pow := make([]float64, n)
	for j := range t {
		tau := (t[j] - t0) / h
		w := 1 / (sigma[j] * sigma[j])
		pow[0] = 1
		for k := 1; k < n; k++ {
			pow[k] = pow[k-1] * tau
		}
		for a := 0; a < n; a++ {
			rhs[a] += w * pow[a] * values[j]
			for b := 0; b < n; b++ {
				normal[a][b] += w * pow[a] * pow[b]
			}
		}
	}
	cov, err := invert(normal)
	if err != nil {
		return localFit{}, err
	}
	c := make([]float64, n)
	for a := 0; a < n; a++ {
		for b := 0; b < n; b++ {
			c[a] += cov[a][b] * rhs[b]
		}
	}
	return localFit{c: c, cov: cov}, nil
}

// derivative returns the k-th time derivative at τ = 0 and its 1σ.
func (f localFit) derivative(k int, h float64) (float64, float64) {
	if k >= len(f.c) {
		return math.NaN(), math.NaN()
	}
	scale := factorial(k) / math.Pow(h, float64(k))
	return scale * f.c[k], scale * math.Sqrt(f.cov[k][k])
}

func factorial(k int) float64 {
	f := 1.0
	for i := 2; i <= k; i++ {
		f *= float64(i)
	}
	return f
}

// invert inverts a small symmetric positive-definite matrix by Gauss-Jordan
// elimination with partial pivoting.
func invert(m [][]float64) ([][]float64, error) {
	n := len(m)
	a := make([][]float64, n)
	for i := range a {
		a[i] = make([]float64, 2*n)
		copy(a[i], m[i])
		a[i][n+i] = 1
	}
	for col := 0; col < n; col++ {
		p := col
		for r := col + 1; r < n; r++ {
			if math.Abs(a[r][col]) > math.Abs(a[p][col]) {
				p = r
			}
		}
		if math.Abs(a[p][col]) < 1e-12 {
			return nil, errors.New("singular normal matrix")
		}
		a[col], a[p] = a[p], a[col]
		pv := a[col][col]
		for k := range a[col] {
			a[col][k] /= pv
		}
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := a[r][col]
			for k := range a[r] {
				a[r][k] -= f * a[col][k]
			}
		}
	}
	out := make([][]float64, n)
	for i := range out {
		out[i] = append([]float64(nil), a[i][n:]...)
	}
	return out, nil
}

// Derive estimates kinematics at every sample.
//
// Noise model: each pose's position has independent, isotropic per-axis
// noise of SigmaPosM (or DefaultSigmaPosM), and yaw independent noise of
// SigmaYawRad. Under that model the per-axis variance of a derivative does
// not depend on the axis, so the σ of speed, of acceleration (norm or any
// component) and of jerk is the per-axis σ. Odometry error that is
// correlated across the window (drift, a slow yaw bias) is not noise under
// this model: it biases the estimate without widening σ, and is what the
// anchor and stop checks exist to expose. Near zero speed the norm is
// biased upward by noise (a Rayleigh floor of about 1.25·SpeedSigma).
func Derive(t *Trajectory, opt Options) []Kinematics {
	opt = opt.withDefaults()
	s := t.Samples
	out := make([]Kinematics, len(s))
	yaw, pitch, roll := make([]float64, len(s)), make([]float64, len(s)), make([]float64, len(s))
	for i := range s {
		yaw[i], pitch[i], roll[i] = EulerZYX(s[i].Q)
		out[i] = Kinematics{Frame: s[i].Frame, T: s[i].T}
	}
	for _, seg := range t.segments(opt.GapFactor) {
		unwrap(yaw[seg[0]:seg[1]])
		for i := seg[0]; i < seg[1]; i++ {
			out[i] = deriveAt(s, yaw, pitch, roll, seg, i, opt)
		}
	}
	return out
}

func unwrap(a []float64) {
	for i := 1; i < len(a); i++ {
		for a[i]-a[i-1] > math.Pi {
			a[i] -= 2 * math.Pi
		}
		for a[i]-a[i-1] < -math.Pi {
			a[i] += 2 * math.Pi
		}
	}
}

func deriveAt(s []Sample, yaw, pitch, roll []float64, seg [2]int, i int, opt Options) Kinematics {
	k := Kinematics{Frame: s[i].Frame, T: s[i].T}
	h := opt.HalfWindowS
	lo, hi := i, i
	for lo > seg[0] && s[i].T-s[lo-1].T <= h {
		lo--
	}
	for hi+1 < seg[1] && s[hi+1].T-s[i].T <= h {
		hi++
	}
	if lo == i || hi == i {
		k.Reason = "window is one-sided at a trajectory or gap edge"
		return k
	}
	n := hi - lo + 1
	tt := make([]float64, n)
	sig := make([]float64, n)
	sigYaw := make([]float64, n)
	for j := 0; j < n; j++ {
		tt[j] = s[lo+j].T
		sig[j] = opt.DefaultSigmaPosM
		if q := s[lo+j].Quality; q != nil && q.SigmaPosM > 0 {
			sig[j] = q.SigmaPosM
		}
		sigYaw[j] = opt.SigmaYawRad
	}
	var d1, d2, d3 [3]float64
	var s1, s2, s3 float64
	for axis := 0; axis < 3; axis++ {
		v := make([]float64, n)
		for j := 0; j < n; j++ {
			v[j] = s[lo+j].P[axis]
		}
		f, err := fitLocal(tt, v, sig, s[i].T, h, opt.Degree)
		if err != nil {
			k.Reason = err.Error()
			return k
		}
		d1[axis], s1 = f.derivative(1, h)
		d2[axis], s2 = f.derivative(2, h)
		d3[axis], s3 = f.derivative(3, h)
		if axis == 2 {
			k.ResidZ = s[i].P[2] - f.c[0]
		}
	}
	k.Vel = d1
	k.Speed = norm(d1)
	k.SpeedSigma = s1
	k.AccelNorm, k.AccelSigma = norm(d2), s2
	k.JerkNorm, k.JerkSigma = norm(d3), s3

	horiz := math.Hypot(d1[0], d1[1])
	if horiz >= opt.MinHeadingSpeedMS && horiz >= 3*s1 {
		k.HeadingValid = true
		ux, uy := d1[0]/horiz, d1[1]/horiz
		k.CourseRad = math.Atan2(uy, ux)
		k.AccelLong = d2[0]*ux + d2[1]*uy
		k.AccelLat = -d2[0]*uy + d2[1]*ux
		k.JerkLong = d3[0]*ux + d3[1]*uy
	}

	for _, ch := range []struct {
		series []float64
		sigma  []float64
		rate   *float64
		rateS  *float64
		resid  *float64
	}{
		{yaw, sigYaw, &k.YawRate, &k.YawRateSigma, nil},
		{pitch, sigYaw, nil, nil, &k.ResidPitch},
		{roll, sigYaw, nil, nil, &k.ResidRoll},
	} {
		f, err := fitLocal(tt, ch.series[lo:hi+1], ch.sigma, s[i].T, h, opt.Degree)
		if err != nil {
			k.Reason = err.Error()
			return k
		}
		if ch.rate != nil {
			*ch.rate, *ch.rateS = f.derivative(1, h)
		}
		if ch.resid != nil {
			*ch.resid = ch.series[i] - f.c[0]
		}
	}
	k.Valid = true
	return k
}

func norm(v [3]float64) float64 { return math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2]) }
