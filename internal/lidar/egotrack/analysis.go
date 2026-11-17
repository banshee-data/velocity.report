package egotrack

import (
	"fmt"
	"math"
	"sort"
)

// Stop is an interval at near-zero speed. Displacement is how far the raw
// poses moved across it: at a real standstill it is odometry drift plus pose
// noise, so it is a free check on the trajectory, not a vehicle quantity.
type Stop struct {
	StartT        float64 `json:"start_t_s"`
	EndT          float64 `json:"end_t_s"`
	DurationS     float64 `json:"duration_s"`
	Samples       int     `json:"samples"`
	MaxSpeedMS    float64 `json:"max_speed_ms"`
	DisplacementM float64 `json:"displacement_m"`
}

// Stops finds runs of valid samples below the stop speed lasting at least
// the minimum duration.
func Stops(t *Trajectory, k []Kinematics, opt Options) []Stop {
	opt = opt.withDefaults()
	var out []Stop
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		a, b := t.Samples[start], t.Samples[end]
		if b.T-a.T >= opt.StopMinS {
			st := Stop{StartT: a.T, EndT: b.T, DurationS: b.T - a.T, Samples: end - start + 1}
			for i := start; i <= end; i++ {
				st.MaxSpeedMS = math.Max(st.MaxSpeedMS, k[i].Speed)
			}
			st.DisplacementM = math.Sqrt(sq(b.P[0]-a.P[0]) + sq(b.P[1]-a.P[1]) + sq(b.P[2]-a.P[2]))
			out = append(out, st)
		}
		start = -1
	}
	for i := range k {
		still := k[i].Valid && k[i].Speed < opt.StopSpeedMS
		if i > 0 && t.Samples[i].T-t.Samples[i-1].T > opt.GapFactor*t.MedianInterval() {
			flush(i - 1)
		}
		switch {
		case still && start < 0:
			start = i
		case !still && start >= 0:
			flush(i - 1)
		}
	}
	flush(len(k) - 1)
	return out
}

func sq(v float64) float64 { return v * v }

// Spectrum is a Welch estimate of the vibration left after the local fit.
// It can only see motion up to Nyquist (half the pose rate): vibration above
// that is aliased into the band, not removed, so a peak is a lower bound on
// what the mount does, not a measurement of its full spectrum.
type Spectrum struct {
	SampleRateHz float64 `json:"sample_rate_hz"`
	NyquistHz    float64 `json:"nyquist_hz"`
	Segments     int     `json:"segments"`
	// SegmentSamples is the Welch segment length used at this pose rate.
	SegmentSamples int       `json:"segment_samples"`
	FreqHz         []float64 `json:"freq_hz"`
	PSDZ           []float64 `json:"psd_z_m2_per_hz"`
	PSDPitch       []float64 `json:"psd_pitch_rad2_per_hz"`
	PSDRoll        []float64 `json:"psd_roll_rad2_per_hz"`
	RMSZM          float64   `json:"rms_z_m"`
	RMSPitchDeg    float64   `json:"rms_pitch_deg"`
	RMSRollDeg     float64   `json:"rms_roll_deg"`
	PeakPitchHz    float64   `json:"peak_pitch_hz"`
	PeakRollHz     float64   `json:"peak_roll_hz"`
	PeakZHz        float64   `json:"peak_z_hz"`
	// ResidualGain is |1 - H(f)| of the local fit at each frequency: the
	// residual is the motion seen through this filter, so it under-reads
	// slow motion (the fit follows it) and over-reads where the fit's
	// response goes negative. The RMS fields are of the residual as seen;
	// the peak amplitudes divide the gain out at the peak.
	ResidualGain    []float64 `json:"residual_gain"`
	PeakZAmpM       float64   `json:"peak_z_amp_m"`
	PeakPitchAmpDeg float64   `json:"peak_pitch_amp_deg"`
	PeakRollAmpDeg  float64   `json:"peak_roll_amp_deg"`
	Skipped         []string  `json:"skipped,omitempty"`
}

// residualGain is |1 - H(f)| for the centre weights of a uniform-sample
// local polynomial fit; the interior of an evenly sampled trajectory sees
// exactly this filter.
func residualGain(freq []float64, dt float64, opt Options) []float64 {
	m := int(opt.HalfWindowS/dt + 1e-9)
	var tt, ones []float64
	for j := -m; j <= m; j++ {
		tt = append(tt, float64(j)*dt)
		ones = append(ones, 1)
	}
	out := make([]float64, len(freq))
	// The centre weight of sample j is the fitted c0 when the input is the
	// unit impulse at j.
	w := make([]float64, len(tt))
	for j := range tt {
		v := make([]float64, len(tt))
		v[j] = 1
		f, err := fitLocal(tt, v, ones, 0, opt.HalfWindowS, opt.Degree)
		if err != nil {
			return nil
		}
		w[j] = f.c[0]
	}
	for b, fr := range freq {
		h := 0.0
		for j := range tt {
			h += w[j] * math.Cos(2*math.Pi*fr*tt[j])
		}
		out[b] = math.Abs(1 - h)
	}
	return out
}

// peakAmplitude converts the PSD around the peak into the amplitude of the
// sine that would produce it, with the fit's gain divided out.
func peakAmplitude(f, p, gain []float64, peakHz float64) float64 {
	if len(f) < 2 || len(gain) != len(p) {
		return 0
	}
	df := f[1] - f[0]
	b := int(math.Round(peakHz / df))
	power := 0.0
	for j := b - 2; j <= b+2; j++ {
		if j > 0 && j < len(p) && gain[j] > 0.1 {
			power += p[j] * df / (gain[j] * gain[j])
		}
	}
	// A Hann window spreads a sine over its main lobe; summing ±2 bins
	// recovers its power, the sine's variance A²/2.
	return math.Sqrt(2 * power)
}

// Jitter estimates the residual vibration spectrum over every evenly sampled
// stretch long enough for one Welch segment.
func Jitter(t *Trajectory, k []Kinematics, opt Options) Spectrum {
	opt = opt.withDefaults()
	dt := t.MedianInterval()
	n := int(math.Round(opt.SpectrumSegmentS / dt))
	if n < 16 {
		n = 16
	}
	sp := Spectrum{SampleRateHz: 1 / dt, NyquistHz: 0.5 / dt, SegmentSamples: n}
	win := make([]float64, n)
	u := 0.0
	for i := range win {
		win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
		u += win[i] * win[i]
	}
	bins := n/2 + 1
	sp.FreqHz = make([]float64, bins)
	for b := range sp.FreqHz {
		sp.FreqHz[b] = float64(b) * sp.SampleRateHz / float64(n)
	}
	acc := [3][]float64{make([]float64, bins), make([]float64, bins), make([]float64, bins)}
	var sumSq [3]float64
	var count int
	for _, seg := range t.segments(opt.GapFactor) {
		// Use only the valid, evenly spaced interior of the segment.
		run := []int{}
		closeRun := func() {
			if len(run) < n {
				if len(run) > 0 {
					sp.Skipped = append(sp.Skipped, fmt.Sprintf("%d samples from t=%.2f s: shorter than one segment", len(run), t.Samples[run[0]].T))
				}
				run = run[:0]
				return
			}
			for start := 0; start+n <= len(run); start += n / 2 {
				series := [3][]float64{make([]float64, n), make([]float64, n), make([]float64, n)}
				for j := 0; j < n; j++ {
					kk := k[run[start+j]]
					series[0][j], series[1][j], series[2][j] = kk.ResidZ, kk.ResidPitch, kk.ResidRoll
				}
				for c := 0; c < 3; c++ {
					detrend(series[c])
					p := periodogram(series[c], win, u, sp.SampleRateHz)
					for b := range p {
						acc[c][b] += p[b]
					}
				}
				sp.Segments++
			}
			for _, i := range run {
				sumSq[0] += sq(k[i].ResidZ)
				sumSq[1] += sq(k[i].ResidPitch)
				sumSq[2] += sq(k[i].ResidRoll)
				count++
			}
			run = run[:0]
		}
		for i := seg[0]; i < seg[1]; i++ {
			even := i == seg[0] || math.Abs((t.Samples[i].T-t.Samples[i-1].T)-dt) <= 0.2*dt
			if !k[i].Valid || !even {
				closeRun()
				if !k[i].Valid {
					continue
				}
			}
			run = append(run, i)
		}
		closeRun()
	}
	if sp.Segments == 0 {
		sp.FreqHz = nil
		return sp
	}
	for c := 0; c < 3; c++ {
		for b := range acc[c] {
			acc[c][b] /= float64(sp.Segments)
		}
	}
	sp.PSDZ, sp.PSDPitch, sp.PSDRoll = acc[0], acc[1], acc[2]
	sp.RMSZM = math.Sqrt(sumSq[0] / float64(count))
	sp.RMSPitchDeg = math.Sqrt(sumSq[1]/float64(count)) * 180 / math.Pi
	sp.RMSRollDeg = math.Sqrt(sumSq[2]/float64(count)) * 180 / math.Pi
	sp.PeakZHz, sp.PeakPitchHz, sp.PeakRollHz = peak(sp.FreqHz, acc[0]), peak(sp.FreqHz, acc[1]), peak(sp.FreqHz, acc[2])
	sp.ResidualGain = residualGain(sp.FreqHz, dt, opt)
	sp.PeakZAmpM = peakAmplitude(sp.FreqHz, acc[0], sp.ResidualGain, sp.PeakZHz)
	sp.PeakPitchAmpDeg = peakAmplitude(sp.FreqHz, acc[1], sp.ResidualGain, sp.PeakPitchHz) * 180 / math.Pi
	sp.PeakRollAmpDeg = peakAmplitude(sp.FreqHz, acc[2], sp.ResidualGain, sp.PeakRollHz) * 180 / math.Pi
	return sp
}

func detrend(x []float64) {
	m := 0.0
	for _, v := range x {
		m += v
	}
	m /= float64(len(x))
	for i := range x {
		x[i] -= m
	}
}

// periodogram is the one-sided, window-normalised power spectral density of
// x by direct DFT; segments are short, so O(n²) is cheap and dependency-free.
func periodogram(x, win []float64, u, fs float64) []float64 {
	n := len(x)
	out := make([]float64, n/2+1)
	for b := range out {
		var re, im float64
		for j := 0; j < n; j++ {
			a := -2 * math.Pi * float64(b*j) / float64(n)
			re += win[j] * x[j] * math.Cos(a)
			im += win[j] * x[j] * math.Sin(a)
		}
		p := (re*re + im*im) / (fs * u)
		if b != 0 && !(n%2 == 0 && b == n/2) {
			p *= 2
		}
		out[b] = p
	}
	return out
}

func peak(f, p []float64) float64 {
	best := 1
	for b := 2; b < len(p); b++ {
		if p[b] > p[best] {
			best = b
		}
	}
	if best >= len(f) {
		return 0
	}
	return f[best]
}

// Anchor is an independently known pose at one end of the trajectory, in
// the trajectory's frame: for a drive between two registered static sites,
// the accepted sensor pose of each site.
type Anchor struct {
	Name string     `json:"name"`
	At   string     `json:"at"` // "start" or "end"
	P    [3]float64 `json:"p_m"`
	// YawDeg is the anchor's yaw in the frame (from +x towards +y), if known.
	YawDeg *float64 `json:"yaw_deg,omitempty"`
	// SigmaHM is the anchor's stated 1σ horizontal uncertainty; zero means
	// not stated (an operator placement usually carries none).
	SigmaHM float64 `json:"sigma_h_m,omitempty"`
	// HeldOut marks an anchor the trajectory was solved without; only a
	// held-out anchor is an independent check.
	HeldOut bool `json:"held_out"`
}

// AnchorResult compares an end of the trajectory with its anchor.
type AnchorResult struct {
	Anchor
	Samples   int      `json:"samples"`
	HorizErrM float64  `json:"horiz_err_m"`
	VertErrM  float64  `json:"vert_err_m"`
	YawErrDeg *float64 `json:"yaw_err_deg,omitempty"`
	// SpreadM is the horizontal standard deviation of the averaged poses; a
	// large spread means the end was not static and the comparison is weak.
	SpreadM float64 `json:"spread_m"`
	// Within2Sigma is set only when the anchor states its σ.
	Within2Sigma *bool `json:"within_2sigma,omitempty"`
}

// CheckAnchors averages the first or last AnchorWindowS of poses and
// compares them with each anchor.
func CheckAnchors(t *Trajectory, anchors []Anchor, opt Options) ([]AnchorResult, error) {
	opt = opt.withDefaults()
	s := t.Samples
	var out []AnchorResult
	for _, a := range anchors {
		var idx []int
		switch a.At {
		case "start":
			for i := 0; i < len(s) && s[i].T-s[0].T <= opt.AnchorWindowS; i++ {
				idx = append(idx, i)
			}
		case "end":
			for i := len(s) - 1; i >= 0 && s[len(s)-1].T-s[i].T <= opt.AnchorWindowS; i-- {
				idx = append(idx, i)
			}
		default:
			return nil, fmt.Errorf("anchor %q: at must be start or end, got %q", a.Name, a.At)
		}
		var m [3]float64
		var sx, sy float64
		for _, i := range idx {
			for c := 0; c < 3; c++ {
				m[c] += s[i].P[c]
			}
			y, _, _ := EulerZYX(s[i].Q)
			sx += math.Cos(y)
			sy += math.Sin(y)
		}
		for c := range m {
			m[c] /= float64(len(idx))
		}
		spread := 0.0
		for _, i := range idx {
			spread += sq(s[i].P[0]-m[0]) + sq(s[i].P[1]-m[1])
		}
		r := AnchorResult{Anchor: a, Samples: len(idx), SpreadM: math.Sqrt(spread / float64(len(idx)))}
		r.HorizErrM = math.Hypot(m[0]-a.P[0], m[1]-a.P[1])
		r.VertErrM = m[2] - a.P[2]
		if a.YawDeg != nil {
			e := math.Atan2(sy, sx)*180/math.Pi - *a.YawDeg
			e = math.Mod(e+540, 360) - 180
			r.YawErrDeg = &e
		}
		if a.SigmaHM > 0 {
			ok := r.HorizErrM <= 2*a.SigmaHM
			r.Within2Sigma = &ok
		}
		out = append(out, r)
	}
	return out, nil
}

// Summary is the run-level report the CLI writes.
type Summary struct {
	Options          Options        `json:"options"`
	Frame            string         `json:"frame"`
	TimeScale        string         `json:"time_scale"`
	Samples          int            `json:"samples"`
	ValidSamples     int            `json:"valid_samples"`
	Segments         int            `json:"segments"`
	DurationS        float64        `json:"duration_s"`
	DistanceM        float64        `json:"distance_m"`
	MaxSpeedMS       float64        `json:"max_speed_ms"`
	MeanMovingMS     float64        `json:"mean_moving_speed_ms"`
	MaxAbsAccelLong  float64        `json:"max_abs_accel_long_ms2"`
	MaxAbsAccelLat   float64        `json:"max_abs_accel_lat_ms2"`
	MaxAbsJerkLong   float64        `json:"max_abs_jerk_long_ms3"`
	MedianSpeedSigma float64        `json:"median_speed_sigma_ms"`
	MedianAccelSigma float64        `json:"median_accel_sigma_ms2"`
	MedianJerkSigma  float64        `json:"median_jerk_sigma_ms3"`
	Stops            []Stop         `json:"stops"`
	Jitter           Spectrum       `json:"jitter"`
	Anchors          []AnchorResult `json:"anchors,omitempty"`
	Notes            []string       `json:"notes"`
}

// Analyse runs every step and assembles the summary.
func Analyse(t *Trajectory, anchors []Anchor, opt Options) (Summary, []Kinematics, error) {
	opt = opt.withDefaults()
	k := Derive(t, opt)
	sum := Summary{Options: opt, Frame: t.Frame, TimeScale: t.TimeScale, Samples: len(k), Segments: len(t.segments(opt.GapFactor))}
	sum.DurationS = t.Samples[len(t.Samples)-1].T - t.Samples[0].T
	var ss, as, js []float64
	var movingT, movingD float64
	for i := range k {
		if !k[i].Valid {
			continue
		}
		sum.ValidSamples++
		ss, as, js = append(ss, k[i].SpeedSigma), append(as, k[i].AccelSigma), append(js, k[i].JerkSigma)
		sum.MaxSpeedMS = math.Max(sum.MaxSpeedMS, k[i].Speed)
		if k[i].HeadingValid {
			sum.MaxAbsAccelLong = math.Max(sum.MaxAbsAccelLong, math.Abs(k[i].AccelLong))
			sum.MaxAbsAccelLat = math.Max(sum.MaxAbsAccelLat, math.Abs(k[i].AccelLat))
			sum.MaxAbsJerkLong = math.Max(sum.MaxAbsJerkLong, math.Abs(k[i].JerkLong))
		}
		if i > 0 && k[i-1].Valid {
			dt := k[i].T - k[i-1].T
			if dt <= opt.GapFactor*t.MedianInterval() {
				d := 0.5 * (k[i].Speed + k[i-1].Speed) * dt
				sum.DistanceM += d
				if k[i].Speed >= opt.StopSpeedMS {
					movingT += dt
					movingD += d
				}
			}
		}
	}
	if movingT > 0 {
		sum.MeanMovingMS = movingD / movingT
	}
	sum.MedianSpeedSigma, sum.MedianAccelSigma, sum.MedianJerkSigma = median(ss), median(as), median(js)
	sum.Stops = Stops(t, k, opt)
	sum.Jitter = Jitter(t, k, opt)
	if len(anchors) > 0 {
		r, err := CheckAnchors(t, anchors, opt)
		if err != nil {
			return sum, k, err
		}
		sum.Anchors = r
	}
	sum.Notes = []string{
		fmt.Sprintf("derivatives are local degree-%d fits over ±%.2f s; motion faster than about %.1f Hz is attenuated in speed, acceleration and jerk", opt.Degree, opt.HalfWindowS, 1/(2*opt.HalfWindowS)),
		"sigma fields assume independent per-pose noise; correlated odometry drift biases values without widening sigma and is checked only by held-out anchors and stop displacement",
		fmt.Sprintf("the jitter spectrum is limited to %.2f Hz (Nyquist of the pose rate); vibration above it aliases into the band", sum.Jitter.NyquistHz),
		"distance is the integral of estimated speed over valid samples and inherits its noise floor near standstill",
	}
	return sum, k, nil
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	return c[len(c)/2]
}
