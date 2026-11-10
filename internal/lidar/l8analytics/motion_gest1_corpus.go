package l8analytics

// G-EST-1 restated and read across cases (l5est0007). l5est0005's screen
// found the CV filter under-dispersed on kirk0, so its criterion 2, a
// correlation off zero, could pass on a filter that never fails. The restated
// criterion asks first whether there is over-dispersion along the track at
// all, then whether acceleration explains it:
//
//   - dispersion: the one-step along-track z = along / its predicted sd, its
//     RMS and the share of |z| beyond 1.96, per track and pooled, with a
//     whole-track bootstrap; across-track the same;
//   - the partial correlation of z_along² with the smoothed |longitudinal
//     acceleration|, controlling for range, aspect and point count, with the
//     bootstrap 5th percentile;
//   - the along-track z RMS by stratum (speed, range, aspect, point count,
//     track age under and over 1 s);
//   - Punzo et al. 2011's label-free trajectory checks on each estimator's
//     output: the share of jerk samples beyond 15 m/s³ and the jerk sign
//     changes a second;
//   - the share of track-frames turning faster than stated yaw rates.
//
// Every one of these is a sum over tracks, so a case writes its moving
// tracks' records (GEST1CorpusTrack) and ReadGEST1Corpus reads any union of
// them: one case, or every case of a corpus. Definitions are in
// data/maths/motion-models-maths.md, Section 10.

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks/motion"
	"github.com/banshee-data/velocity.report/internal/stats"
)

// GEST1OfflineModels are the offline estimators a track record carries
// one-step innovations and jerk counts for, in this order. The live filter
// (the tracked filter or the solid body, by source) is read beside them as
// GEST1EstimatorLive.
var GEST1OfflineModels = []string{"cv", "ca_coast", "imm"}

// GEST1EstimatorLive names the estimator that ran in the replay: the tracked
// filter for the tracked source, the solid body's own filter for the faces.
const GEST1EstimatorLive = "live"

// gest1Z196 is the two-sided 95 % point of the standard normal.
const gest1Z196 = 1.959963984540054

// GEST1CorpusSample is one scored residual of a moving track. JSON names are
// short because a corpus holds millions; a value whose flag is false is zero
// and means nothing.
type GEST1CorpusSample struct {
	// OneStep: the prior was predicted across at most one frame
	// (gest1OneStepSecs) and the residual changed no reference.
	OneStep bool `json:"o,omitempty"`
	Dim     int  `json:"d"`
	// ZAlong and ZAcross are the components normalised by their predicted sd.
	HasAlong  bool    `json:"ha,omitempty"`
	ZAlong    float64 `json:"za,omitempty"`
	HasAcross bool    `json:"hc,omitempty"`
	ZAcross   float64 `json:"zc,omitempty"`
	// HasState: the smoothed state exists at the residual (not a leading
	// rank-one fix); Speed (m/s) and Range (m) are read from it.
	HasState bool    `json:"hs,omitempty"`
	Speed    float64 `json:"v,omitempty"`
	Range    float64 `json:"r,omitempty"`
	// Aspect (rad, 0 end-on to π/2 broadside) needs a course.
	HasAspect bool    `json:"hp,omitempty"`
	Aspect    float64 `json:"p,omitempty"`
	// Accel is the smoothed |longitudinal acceleration| (m/s²).
	HasAccel bool    `json:"hx,omitempty"`
	Accel    float64 `json:"x,omitempty"`
	Points   int     `json:"n,omitempty"`
	// AgeSecs is the time since the track's first tracked-filter
	// observation (the source's own first observation when the tracked
	// filter did not record the chain).
	AgeSecs float64 `json:"t"`
}

// complete reports whether the sample has every covariate the partial
// correlation controls for.
func (s GEST1CorpusSample) complete() bool {
	return s.HasState && s.HasAspect && s.HasAccel && s.Points > 0
}

// GEST1JerkCount is Punzo et al. 2011's two jerk checks on one estimator's
// output over one track: N jerk samples, Over of them beyond the threshold in
// magnitude, and SignChanges between consecutive jerk samples over Secs
// seconds between them.
type GEST1JerkCount struct {
	Estimator   string  `json:"estimator"`
	N           int     `json:"n"`
	Over        int     `json:"over"`
	SignChanges int     `json:"sign_changes"`
	Secs        float64 `json:"secs"`
}

// GEST1OcclusionRecord is one criterion 5 case: the horizon, its split and
// each model's error (DefaultOcclusionModels' order), so cases pool.
type GEST1OcclusionRecord struct {
	HorizonSecs  float64   `json:"h"`
	Decelerating bool      `json:"decel,omitempty"`
	Errors       []float64 `json:"e"`
}

// GEST1CorpusTrack is one moving track's record from one source.
type GEST1CorpusTrack struct {
	Case             string              `json:"case,omitempty"`
	TrackID          string              `json:"track_id"`
	CreationSequence int64               `json:"creation_sequence"`
	Samples          []GEST1CorpusSample `json:"samples"`
	// RunTracked says the track has one-step along residuals, and
	// RunFlagged that they hold criterion 3's run (three same-sign beyond
	// one sigma).
	RunTracked bool `json:"criterion3_one_step_tracked,omitempty"`
	RunFlagged bool `json:"criterion3_one_step_flagged,omitempty"`
	// OfflineAlong[m] and OfflineAcross[m] are model m's
	// (GEST1OfflineModels) one-step |innovation| components in metres over
	// the scored observations: criterion 4's inputs.
	OfflineAlong  [][]float64 `json:"offline_one_step_along_m"`
	OfflineAcross [][]float64 `json:"offline_one_step_across_m"`
	// OfflineAlongAge[m] and OfflineAcrossAge[m] are the track ages (s,
	// as GEST1CorpusSample.AgeSecs) of OfflineAlong[m] and OfflineAcross[m],
	// value for value, so criterion 4 splits at GEST1AgeSplitSecs (l5est0008).
	// Records written before them carry none.
	OfflineAlongAge  [][]float64 `json:"offline_one_step_along_age_s,omitempty"`
	OfflineAcrossAge [][]float64 `json:"offline_one_step_across_age_s,omitempty"`
	// Jerk holds the live estimator's count and each offline model's.
	Jerk []GEST1JerkCount `json:"jerk"`
	// TurnFrames are the observations with a yaw rate, TurnOver[k] those
	// whose |yaw rate| exceeds the config's TurnRateThresholds[k].
	TurnFrames int                    `json:"turn_frames"`
	TurnOver   []int                  `json:"turn_over"`
	Occlusion  []GEST1OcclusionRecord `json:"occlusion,omitempty"`
}

// GEST1CorpusSource is every moving track of one source in one case.
type GEST1CorpusSource struct {
	Source string             `json:"source"`
	Tracks []GEST1CorpusTrack `json:"tracks"`
}

// GEST1CorpusRecord is a case's track records, the per-case file a corpus
// read pools.
type GEST1CorpusRecord struct {
	Case    string              `json:"case"`
	Config  GEST1Config         `json:"config"`
	Sources []GEST1CorpusSource `json:"sources"`
}

// Source returns the named source's tracks, nil when absent.
func (r GEST1CorpusRecord) Source(name string) []GEST1CorpusTrack {
	for _, s := range r.Sources {
		if s.Source == name {
			return s.Tracks
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Building a track's record.
// ---------------------------------------------------------------------------

// jerkCount differences an estimator's speed twice over consecutive
// observations: a = Δ|v|/Δt across each interval, j = Δa over the mean of the
// two intervals. An interval longer than maxStep, or with a speed below
// minSpeed (where |v| is rectified noise) or not finite, breaks the sequence.
// A sign change is counted between consecutive jerk samples of opposite sign.
func jerkCount(name string, times []int64, vx, vy []float64, threshold, maxStep, minSpeed float64) GEST1JerkCount {
	out := GEST1JerkCount{Estimator: name}
	prevA, prevJ := math.NaN(), math.NaN()
	for k := 1; k < len(times); k++ {
		dt := float64(times[k]-times[k-1]) / 1e9
		s0, s1 := math.Hypot(vx[k-1], vy[k-1]), math.Hypot(vx[k], vy[k])
		if !(dt > 0) || dt > maxStep || !(s0 >= minSpeed) || !(s1 >= minSpeed) || !finite(s0) || !finite(s1) {
			prevA, prevJ = math.NaN(), math.NaN()
			continue
		}
		a := (s1 - s0) / dt
		if !math.IsNaN(prevA) {
			mid := float64(times[k]-times[k-2]) / 2e9
			j := (a - prevA) / mid
			out.N++
			if math.Abs(j) > threshold {
				out.Over++
			}
			if !math.IsNaN(prevJ) {
				out.Secs += dt
				if (j > 0 && prevJ < 0) || (j < 0 && prevJ > 0) {
					out.SignChanges++
				}
			}
			prevJ = j
		}
		prevA = a
	}
	return out
}

// offlineRun is one offline model's output over a track's scored
// observations: its one-step |innovation| components and its posterior
// velocity at each observation.
type offlineRun struct {
	along, across []float64
	// alongT and acrossT are the observation times of along and across.
	alongT, acrossT []int64
	vx, vy          []float64
}

// runOfflineModels runs offline CV, CA with the coast rule and the IMM over
// the observations (GEST1OfflineModels' order).
func runOfflineModels(obs []motion.Observation, minCourse float64) ([]offlineRun, error) {
	dec := &GEST1Collector{cfg: GEST1CollectorConfig{MinCourseSpeed: minCourse}}
	out := make([]offlineRun, len(GEST1OfflineModels))
	add := func(run *offlineRun, i int, founded bool, dt float64, prior motion.Estimate, in motion.Innovation, post motion.Estimate) {
		run.vx[i], run.vy[i] = post.X[motion.IdxVX], post.X[motion.IdxVY]
		if founded || dt > gest1OneStepSecs {
			return
		}
		var r GEST1Residual
		vx, vy := prior.X[motion.IdxVX], prior.X[motion.IdxVY]
		if in.Dim == 2 {
			dec.decomposeTwo(&r, in.Y, in.S, vx, vy)
		} else {
			nx, ny := obs[i].NX, obs[i].NY
			norm := math.Hypot(nx, ny)
			dec.decomposeOne(&r, in.Y[0], in.S[0][0], [2]float64{nx / norm, ny / norm}, vx, vy)
		}
		if r.HasAlong {
			run.along = append(run.along, math.Abs(r.Along))
			run.alongT = append(run.alongT, obs[i].UnixNanos)
		}
		if r.HasAcross {
			run.across = append(run.across, math.Abs(r.Across))
			run.acrossT = append(run.acrossT, obs[i].UnixNanos)
		}
	}
	for m := range out {
		out[m].vx, out[m].vy = make([]float64, len(obs)), make([]float64, len(obs))
		out[m].along, out[m].across = []float64{}, []float64{}
	}
	for m, cfg := range []motion.FilterConfig{motion.DefaultCVConfig(), motion.DefaultCAConfig()} {
		steps, err := motion.RunFilter(cfg, obs)
		if err != nil {
			return nil, err
		}
		for i, s := range steps {
			add(&out[m], i, s.Founded, s.DtSecs, s.Prior, s.Innovation, s.Posterior)
		}
	}
	steps, err := motion.RunIMM(motion.DefaultIMMConfig(), obs)
	if err != nil {
		return nil, err
	}
	for i, s := range steps {
		add(&out[2], i, s.Founded, s.DtSecs, s.Prior, s.Innovation, s.Posterior)
	}
	return out, nil
}

// yawRate is the smoothed heading's centred rate at observation i, over the
// same window as the acceleration, where the smoothed speed at both ends and
// at i reaches minSpeed; NaN otherwise.
func (k chainKinematics) yawRate(i int, cfg GEST1Config) float64 {
	n := len(k.times)
	h := int64(cfg.AccelHalfWindowSecs * 1e9)
	span := int64(cfg.AccelMaxSpanSecs * 1e9)
	lo := sort.Search(n, func(j int) bool { return k.times[j] > k.times[i]-h }) - 1
	hi := sort.Search(n, func(j int) bool { return k.times[j] >= k.times[i]+h })
	if lo < 0 || hi >= n || k.times[hi]-k.times[lo] > span {
		return math.NaN()
	}
	for _, j := range []int{lo, i, hi} {
		if math.Hypot(k.vx[j], k.vy[j]) < cfg.TurnMinSpeed {
			return math.NaN()
		}
	}
	d := math.Atan2(k.vy[hi], k.vx[hi]) - math.Atan2(k.vy[lo], k.vx[lo])
	d = math.Remainder(d, 2*math.Pi)
	return d / (float64(k.times[hi]-k.times[lo]) / 1e9)
}

// ---------------------------------------------------------------------------
// Reading track records.
// ---------------------------------------------------------------------------

// GEST1CorpusConfig sets how ReadGEST1Corpus reads.
type GEST1CorpusConfig struct {
	// BootstrapDraws whole-track resamples (seeded) behind every interval;
	// zero skips them. Level is the two-sided interval's coverage.
	BootstrapDraws int     `json:"bootstrap_draws"`
	BootstrapSeed  int64   `json:"bootstrap_seed"`
	Level          float64 `json:"level"`
	// MinTrackSamples is the least number of one-step residuals a track
	// needs for its own RMS to enter the per-track distribution.
	MinTrackSamples int `json:"min_track_samples"`
}

// DefaultGEST1CorpusConfig is l5est0007's read.
func DefaultGEST1CorpusConfig() GEST1CorpusConfig {
	return GEST1CorpusConfig{BootstrapDraws: 2000, BootstrapSeed: 20261110, Level: 0.95, MinTrackSamples: 5}
}

// GEST1Dispersion is the restated criterion 2's dispersion half over one
// component: pooled RMS and share beyond 1.96 with whole-track bootstrap
// intervals, and the per-track distribution.
type GEST1Dispersion struct {
	N        int     `json:"n"`
	Tracks   int     `json:"tracks"`
	RMS      float64 `json:"rms"`
	Mean     float64 `json:"mean"`
	Beyond   float64 `json:"share_beyond_1_96"`
	RMSLo    float64 `json:"rms_bootstrap_lo"`
	RMSHi    float64 `json:"rms_bootstrap_hi"`
	BeyondLo float64 `json:"beyond_bootstrap_lo"`
	BeyondHi float64 `json:"beyond_bootstrap_hi"`
	// TrackRMS* describe the per-track RMS over tracks with at least
	// MinTrackSamples values; TrackShareAbove1 is the share of them whose
	// RMS exceeds 1, TrackBeyondMedian the median per-track share beyond.
	TrackRMSTracks    int     `json:"per_track_tracks"`
	TrackRMSMedian    float64 `json:"per_track_rms_median"`
	TrackRMSP10       float64 `json:"per_track_rms_p10"`
	TrackRMSP90       float64 `json:"per_track_rms_p90"`
	TrackShareAbove1  float64 `json:"per_track_share_rms_above_1"`
	TrackBeyondMedian float64 `json:"per_track_beyond_median"`
	// OverDispersed is the restated criterion's first half: RMS above 1 or
	// more than 5 % beyond 1.96.
	OverDispersed bool `json:"over_dispersed"`
}

// GEST1ZCorrelation is the restated criterion 2's second half: the partial
// correlation of z_along² with |longitudinal acceleration|, controlling for
// range, aspect and point count, with its whole-track bootstrap.
type GEST1ZCorrelation struct {
	N      int                      `json:"n"`
	Tracks int                      `json:"tracks"`
	Err    string                   `json:"error,omitempty"`
	R      PartialCorrelationResult `json:"partial_correlation"`
	// BootP05 is the bootstrap distribution's 5th percentile, BootLo and
	// BootHi its two-sided interval at the level.
	BootDraws  int     `json:"bootstrap_draws,omitempty"`
	BootFailed int     `json:"bootstrap_failed_draws,omitempty"`
	BootP05    float64 `json:"bootstrap_p05"`
	BootLo     float64 `json:"bootstrap_lo"`
	BootHi     float64 `json:"bootstrap_hi"`
}

// GEST1Stratum is the one-step along-track z in one stratum.
type GEST1Stratum struct {
	Factor      string  `json:"factor"`
	Bin         string  `json:"bin"`
	N           int     `json:"n"`
	Tracks      int     `json:"tracks"`
	RMS         float64 `json:"rms"`
	Beyond      float64 `json:"share_beyond_1_96"`
	BeyondCount int     `json:"beyond_count"`
}

// GEST1JerkSummary pools one estimator's jerk counts.
type GEST1JerkSummary struct {
	Estimator         string  `json:"estimator"`
	Tracks            int     `json:"tracks"`
	N                 int     `json:"n"`
	ShareOver         float64 `json:"share_over_threshold"`
	SignChangesPerSec float64 `json:"sign_changes_per_s"`
}

// GEST1TurnSummary pools the turning shares.
type GEST1TurnSummary struct {
	Frames     int       `json:"frames"`
	Thresholds []float64 `json:"thresholds_rad_s"`
	Shares     []float64 `json:"shares"`
}

// GEST1OfflineTails is criterion 4's one-step tails pooled, with the IMM:
// p99 |along| and |across| per model and each model's ratio to CV's.
type GEST1OfflineTails struct {
	Models      []string  `json:"models"`
	NAlong      int       `json:"n_along"`
	NAcross     int       `json:"n_across"`
	AlongP99    []float64 `json:"along_p99_m"`
	AcrossP99   []float64 `json:"across_p99_m"`
	AlongRatio  []float64 `json:"along_ratio_to_cv"`
	AcrossRatio []float64 `json:"across_ratio_to_cv"`
}

// GEST1CorpusRead is ReadGEST1Corpus's result over one source's tracks.
type GEST1CorpusRead struct {
	Config  GEST1CorpusConfig `json:"config"`
	Tracks  int               `json:"tracks"`
	Samples int               `json:"samples"`
	// One-step along and across dispersion (the declared reads), and over
	// every scored residual (context).
	Along        GEST1Dispersion   `json:"along_one_step"`
	Across       GEST1Dispersion   `json:"across_one_step"`
	AlongAll     GEST1Dispersion   `json:"along_all"`
	AcrossAll    GEST1Dispersion   `json:"across_all"`
	Correlation  GEST1ZCorrelation `json:"z_along_sq_accel_one_step"`
	CorrelationA GEST1ZCorrelation `json:"z_along_sq_accel_all"`
	Strata       []GEST1Stratum    `json:"strata"`
	// YoungTailShare is the share of the one-step along values beyond 1.96
	// that come from residuals under 1 s into their track.
	YoungTailShare float64 `json:"young_share_of_tail"`
	YoungTracks    int     `json:"young_tracks"`
	// AgeSplit is l5est0008's declared split at GEST1AgeSplitSecs.
	AgeSplit    GEST1AgeSplit      `json:"age_split"`
	Jerk        []GEST1JerkSummary `json:"jerk"`
	Turning     GEST1TurnSummary   `json:"turning"`
	Criterion3  RunLengthResult    `json:"criterion3_one_step"`
	Criterion4  GEST1OfflineTails  `json:"criterion4_one_step"`
	Criterion5  []OcclusionSummary `json:"criterion5"`
	Criterion5N int                `json:"criterion5_cases"`
}

// GEST1AgeSplitSecs is l5est0008's track-age boundary: a residual at an age
// of at most this is young, one after it mature.
const GEST1AgeSplitSecs = 1.0

// GEST1AgeHalf is the restated criterion 2 and criterion 4 over the residuals
// on one side of the age boundary, each interval a whole-track bootstrap over
// the tracks with a value on that side.
type GEST1AgeHalf struct {
	Along       GEST1Dispersion   `json:"along_one_step"`
	Across      GEST1Dispersion   `json:"across_one_step"`
	Correlation GEST1ZCorrelation `json:"z_along_sq_accel_one_step"`
	// Criterion4 reads only the offline values whose age the record
	// carries; NAlong is zero over records written without ages.
	Criterion4 GEST1OfflineTails `json:"criterion4_one_step"`
}

// GEST1AgeSplit is the one-step read split at BoundarySecs of track age:
// Young at most the boundary, Mature after it.
type GEST1AgeSplit struct {
	BoundarySecs float64      `json:"boundary_s"`
	Young        GEST1AgeHalf `json:"young"`
	Mature       GEST1AgeHalf `json:"mature"`
}

// ageSplit reads both halves with its own seeded generator, so adding it
// leaves every other figure of the read as it was.
func ageSplit(tracks []GEST1CorpusTrack, cfg GEST1CorpusConfig) GEST1AgeSplit {
	out := GEST1AgeSplit{BoundarySecs: GEST1AgeSplitSecs}
	rng := rand.New(rand.NewSource(cfg.BootstrapSeed))
	for _, young := range []bool{true, false} {
		side := func(age float64) bool { return (age <= GEST1AgeSplitSecs) == young }
		along, across := make([]trackZ, len(tracks)), make([]trackZ, len(tracks))
		offAlong := make([][]float64, len(GEST1OfflineModels))
		offAcross := make([][]float64, len(GEST1OfflineModels))
		for ti, t := range tracks {
			for _, s := range t.Samples {
				if !s.OneStep || !side(s.AgeSecs) {
					continue
				}
				if s.HasAlong {
					along[ti].add(s.ZAlong)
				}
				if s.HasAcross {
					across[ti].add(s.ZAcross)
				}
			}
			for m := range GEST1OfflineModels {
				offAlong[m] = appendAged(offAlong[m], t.OfflineAlong, t.OfflineAlongAge, m, side)
				offAcross[m] = appendAged(offAcross[m], t.OfflineAcross, t.OfflineAcrossAge, m, side)
			}
		}
		h := GEST1AgeHalf{Along: dispersion(along, cfg, rng), Across: dispersion(across, cfg, rng)}
		h.Correlation = zCorrelation(tracks, func(s GEST1CorpusSample) bool { return s.OneStep && side(s.AgeSecs) }, cfg, rng)
		if len(offAlong[0]) > 0 {
			h.Criterion4 = offlineTails(offAlong, offAcross)
		}
		if young {
			out.Young = h
		} else {
			out.Mature = h
		}
	}
	return out
}

// appendAged appends model m's values whose age is on the wanted side; a
// model without ages of the same length contributes nothing.
func appendAged(dst []float64, vals, ages [][]float64, m int, side func(float64) bool) []float64 {
	if m >= len(vals) || m >= len(ages) || len(vals[m]) != len(ages[m]) {
		return dst
	}
	for i, v := range vals[m] {
		if side(ages[m][i]) {
			dst = append(dst, v)
		}
	}
	return dst
}

// gest1Strata are the stratum table's factors and bins: lower bounds, the
// last bin open above.
var gest1Strata = []struct {
	factor string
	edges  []float64
	labels []string
}{
	{"speed_mps", []float64{0, 5, 10, 15}, []string{"0-5", "5-10", "10-15", "15+"}},
	{"range_m", []float64{0, 10, 20, 40}, []string{"0-10", "10-20", "20-40", "40+"}},
	{"aspect_deg", []float64{0, 30, 60}, []string{"0-30 end-on", "30-60", "60-90 broadside"}},
	{"points", []float64{0, 50, 200, 500}, []string{"1-49", "50-199", "200-499", "500+"}},
	{"age_s", []float64{0, 1}, []string{"under 1", "1+"}},
}

func gest1Bin(edges []float64, v float64) int {
	b := -1
	for i, e := range edges {
		if v >= e {
			b = i
		}
	}
	return b
}

// trackZ is one track's sufficient statistics over one component.
type trackZ struct {
	n, beyond int
	sum, ss   float64
}

func (t *trackZ) add(z float64) {
	t.n++
	t.sum += z
	t.ss += z * z
	if math.Abs(z) > gest1Z196 {
		t.beyond++
	}
}

func dispersion(per []trackZ, cfg GEST1CorpusConfig, rng *rand.Rand) GEST1Dispersion {
	var out GEST1Dispersion
	var tot trackZ
	var rmsByTrack, beyondByTrack []float64
	var withData []trackZ
	for _, t := range per {
		if t.n == 0 {
			continue
		}
		withData = append(withData, t)
		tot.n += t.n
		tot.beyond += t.beyond
		tot.sum += t.sum
		tot.ss += t.ss
		if t.n >= cfg.MinTrackSamples {
			rmsByTrack = append(rmsByTrack, math.Sqrt(t.ss/float64(t.n)))
			beyondByTrack = append(beyondByTrack, float64(t.beyond)/float64(t.n))
		}
	}
	out.N, out.Tracks = tot.n, len(withData)
	if tot.n == 0 {
		return out
	}
	n := float64(tot.n)
	out.RMS, out.Mean, out.Beyond = math.Sqrt(tot.ss/n), tot.sum/n, float64(tot.beyond)/n
	out.OverDispersed = out.RMS > 1 || out.Beyond > 0.05
	if len(rmsByTrack) > 0 {
		sort.Float64s(rmsByTrack)
		sort.Float64s(beyondByTrack)
		out.TrackRMSTracks = len(rmsByTrack)
		out.TrackRMSMedian = stats.NearestRank(rmsByTrack, 0.5)
		out.TrackRMSP10, out.TrackRMSP90 = stats.NearestRank(rmsByTrack, 0.1), stats.NearestRank(rmsByTrack, 0.9)
		above := 0
		for _, v := range rmsByTrack {
			if v > 1 {
				above++
			}
		}
		out.TrackShareAbove1 = float64(above) / float64(len(rmsByTrack))
		out.TrackBeyondMedian = stats.NearestRank(beyondByTrack, 0.5)
	}
	if cfg.BootstrapDraws > 0 && len(withData) > 1 {
		rms, bey := make([]float64, 0, cfg.BootstrapDraws), make([]float64, 0, cfg.BootstrapDraws)
		for d := 0; d < cfg.BootstrapDraws; d++ {
			var nn, bb int
			var ss float64
			for range withData {
				t := withData[rng.Intn(len(withData))]
				nn += t.n
				bb += t.beyond
				ss += t.ss
			}
			rms = append(rms, math.Sqrt(ss/float64(nn)))
			bey = append(bey, float64(bb)/float64(nn))
		}
		sort.Float64s(rms)
		sort.Float64s(bey)
		tail := (1 - cfg.Level) / 2
		out.RMSLo, out.RMSHi = stats.NearestRank(rms, tail), stats.NearestRank(rms, 1-tail)
		out.BeyondLo, out.BeyondHi = stats.NearestRank(bey, tail), stats.NearestRank(bey, 1-tail)
	}
	return out
}

// pcVars is the number of columns behind the z_along² partial correlation:
// y = z_along², x = |accel|, then range, aspect and point count.
const pcVars = 5

// pcMoments are one track's centred sums over the partial correlation's
// columns: n, Σv and Σvvᵀ (upper triangle used).
type pcMoments struct {
	n  int
	s  [pcVars]float64
	ss [pcVars][pcVars]float64
}

func (m *pcMoments) add(v [pcVars]float64) {
	m.n++
	for i := 0; i < pcVars; i++ {
		m.s[i] += v[i]
		for j := i; j < pcVars; j++ {
			m.ss[i][j] += v[i] * v[j]
		}
	}
}

func (m *pcMoments) merge(o *pcMoments) {
	m.n += o.n
	for i := 0; i < pcVars; i++ {
		m.s[i] += o.s[i]
		for j := i; j < pcVars; j++ {
			m.ss[i][j] += o.ss[i][j]
		}
	}
}

// partialFromMoments is the partial correlation of columns 0 and 1 given the
// rest, from the precision matrix of the sample covariance: r = −P01/√(P00
// P11). It reports false when the covariance is singular.
func partialFromMoments(m *pcMoments) (float64, bool) {
	if m.n <= pcVars+1 {
		return 0, false
	}
	n := float64(m.n)
	var c [pcVars][2 * pcVars]float64
	for i := 0; i < pcVars; i++ {
		for j := 0; j < pcVars; j++ {
			a, b := i, j
			if a > b {
				a, b = b, a
			}
			c[i][j] = (m.ss[a][b] - m.s[a]*m.s[b]/n) / n
		}
		c[i][pcVars+i] = 1
	}
	// Scale to correlations so the pivot test is relative.
	var sd [pcVars]float64
	for i := 0; i < pcVars; i++ {
		if !(c[i][i] > 0) {
			return 0, false
		}
		sd[i] = math.Sqrt(c[i][i])
	}
	for i := 0; i < pcVars; i++ {
		for j := 0; j < pcVars; j++ {
			c[i][j] /= sd[i] * sd[j]
		}
	}
	for col := 0; col < pcVars; col++ {
		p := col
		for r := col + 1; r < pcVars; r++ {
			if math.Abs(c[r][col]) > math.Abs(c[p][col]) {
				p = r
			}
		}
		if math.Abs(c[p][col]) < 1e-10 {
			return 0, false
		}
		c[col], c[p] = c[p], c[col]
		pv := c[col][col]
		for k := range c[col] {
			c[col][k] /= pv
		}
		for r := 0; r < pcVars; r++ {
			if r == col {
				continue
			}
			f := c[r][col]
			if f == 0 {
				continue
			}
			for k := range c[r] {
				c[r][k] -= f * c[col][k]
			}
		}
	}
	p00, p11, p01 := c[0][pcVars], c[1][pcVars+1], c[0][pcVars+1]
	if !(p00 > 0) || !(p11 > 0) {
		return 0, false
	}
	r := -p01 / math.Sqrt(p00*p11)
	return math.Max(-1, math.Min(1, r)), finite(r)
}

func zCorrelation(tracks []GEST1CorpusTrack, keep func(GEST1CorpusSample) bool, cfg GEST1CorpusConfig, rng *rand.Rand) GEST1ZCorrelation {
	var y, x []float64
	cov := [][]float64{nil, nil, nil}
	owner := []int{}
	for ti, t := range tracks {
		for _, s := range t.Samples {
			if !s.HasAlong || !s.complete() || !keep(s) {
				continue
			}
			y = append(y, s.ZAlong*s.ZAlong)
			x = append(x, s.Accel)
			cov[0] = append(cov[0], s.Range)
			cov[1] = append(cov[1], s.Aspect)
			cov[2] = append(cov[2], float64(s.Points))
			owner = append(owner, ti)
		}
	}
	out := GEST1ZCorrelation{N: len(y)}
	seen := map[int]bool{}
	for _, o := range owner {
		seen[o] = true
	}
	out.Tracks = len(seen)
	r, err := PartialCorrelation(y, x, cov, cfg.Level)
	out.R = r
	if err != nil {
		out.Err = err.Error()
		return out
	}
	if cfg.BootstrapDraws <= 0 || out.Tracks < 2 {
		return out
	}
	// Centre every column on its pooled mean, then keep each track's sums.
	var mean [pcVars]float64
	cols := [pcVars][]float64{y, x, cov[0], cov[1], cov[2]}
	for k := range cols {
		for _, v := range cols[k] {
			mean[k] += v
		}
		mean[k] /= float64(len(y))
	}
	per := map[int]*pcMoments{}
	var keys []int
	for i, o := range owner {
		m := per[o]
		if m == nil {
			m = &pcMoments{}
			per[o] = m
			keys = append(keys, o)
		}
		var v [pcVars]float64
		for k := range cols {
			v[k] = cols[k][i] - mean[k]
		}
		m.add(v)
	}
	ms := make([]*pcMoments, len(keys))
	for i, k := range keys {
		ms[i] = per[k]
	}
	rs := make([]float64, 0, cfg.BootstrapDraws)
	for d := 0; d < cfg.BootstrapDraws; d++ {
		var sum pcMoments
		for range ms {
			sum.merge(ms[rng.Intn(len(ms))])
		}
		r, ok := partialFromMoments(&sum)
		if !ok {
			out.BootFailed++
			continue
		}
		rs = append(rs, r)
	}
	out.BootDraws = cfg.BootstrapDraws
	if len(rs) == 0 {
		return out
	}
	sort.Float64s(rs)
	tail := (1 - cfg.Level) / 2
	out.BootP05 = stats.NearestRank(rs, 0.05)
	out.BootLo, out.BootHi = stats.NearestRank(rs, tail), stats.NearestRank(rs, 1-tail)
	return out
}

// ReadGEST1Corpus reads the restated criterion 2, the strata, the jerk and
// turning checks and criteria 3 to 5 over a union of one source's track
// records. turnThresholds label the records' TurnOver counts.
func ReadGEST1Corpus(tracks []GEST1CorpusTrack, turnThresholds []float64, cfg GEST1CorpusConfig) GEST1CorpusRead {
	if cfg.Level == 0 {
		cfg.Level = 0.95
	}
	rng := rand.New(rand.NewSource(cfg.BootstrapSeed))
	out := GEST1CorpusRead{Config: cfg, Tracks: len(tracks)}
	along, across := make([]trackZ, len(tracks)), make([]trackZ, len(tracks))
	alongAll, acrossAll := make([]trackZ, len(tracks)), make([]trackZ, len(tracks))
	type sk struct{ f, b int }
	strata := map[sk]*trackZ{}
	strataTracks := map[sk]map[int]bool{}
	addStratum := func(f, b, ti int, z float64) {
		k := sk{f, b}
		if strata[k] == nil {
			strata[k], strataTracks[k] = &trackZ{}, map[int]bool{}
		}
		strata[k].add(z)
		strataTracks[k][ti] = true
	}
	young := map[int]bool{}
	youngBeyond, beyondTotal := 0, 0
	jerk := map[string]*GEST1JerkCount{}
	jerkTracks := map[string]int{}
	var jerkOrder []string
	out.Turning.Thresholds = append([]float64(nil), turnThresholds...)
	turnOver := make([]int, len(turnThresholds))
	runs := RunLengthResult{MinRun: 3, Threshold: 1}
	offAlong := make([][]float64, len(GEST1OfflineModels))
	offAcross := make([][]float64, len(GEST1OfflineModels))
	occ := OcclusionReport{}
	for _, m := range DefaultOcclusionModels() {
		occ.Models = append(occ.Models, m.Name)
	}
	horizons := map[float64]bool{}
	for ti, t := range tracks {
		out.Samples += len(t.Samples)
		for _, s := range t.Samples {
			if s.HasAlong {
				alongAll[ti].add(s.ZAlong)
			}
			if s.HasAcross {
				acrossAll[ti].add(s.ZAcross)
			}
			if !s.OneStep {
				continue
			}
			if s.HasAcross {
				across[ti].add(s.ZAcross)
			}
			if !s.HasAlong {
				continue
			}
			z := s.ZAlong
			along[ti].add(z)
			beyond := math.Abs(z) > gest1Z196
			if beyond {
				beyondTotal++
			}
			if s.AgeSecs < 1 {
				young[ti] = true
				if beyond {
					youngBeyond++
				}
			}
			if s.HasState {
				addStratum(0, gest1Bin(gest1Strata[0].edges, s.Speed), ti, z)
				addStratum(1, gest1Bin(gest1Strata[1].edges, s.Range), ti, z)
			}
			if s.HasAspect {
				addStratum(2, gest1Bin(gest1Strata[2].edges, s.Aspect*180/math.Pi), ti, z)
			}
			if s.Points > 0 {
				addStratum(3, gest1Bin(gest1Strata[3].edges, float64(s.Points)), ti, z)
			}
			addStratum(4, gest1Bin(gest1Strata[4].edges, s.AgeSecs), ti, z)
		}
		for _, j := range t.Jerk {
			acc := jerk[j.Estimator]
			if acc == nil {
				acc = &GEST1JerkCount{Estimator: j.Estimator}
				jerk[j.Estimator] = acc
				jerkOrder = append(jerkOrder, j.Estimator)
			}
			acc.N += j.N
			acc.Over += j.Over
			acc.SignChanges += j.SignChanges
			acc.Secs += j.Secs
			if j.N > 0 {
				jerkTracks[j.Estimator]++
			}
		}
		out.Turning.Frames += t.TurnFrames
		for k := range turnOver {
			if k < len(t.TurnOver) {
				turnOver[k] += t.TurnOver[k]
			}
		}
		if t.RunTracked {
			runs.Tracks++
			runs.Eligible++
			if t.RunFlagged {
				runs.Flagged++
			}
		}
		for m := range GEST1OfflineModels {
			if m < len(t.OfflineAlong) {
				offAlong[m] = append(offAlong[m], t.OfflineAlong[m]...)
			}
			if m < len(t.OfflineAcross) {
				offAcross[m] = append(offAcross[m], t.OfflineAcross[m]...)
			}
		}
		for _, o := range t.Occlusion {
			horizons[o.HorizonSecs] = true
			occ.Cases = append(occ.Cases, OcclusionCase{HorizonSecs: o.HorizonSecs, Decelerating: o.Decelerating, Errors: o.Errors})
		}
	}
	out.Along = dispersion(along, cfg, rng)
	out.Across = dispersion(across, cfg, rng)
	out.AlongAll = dispersion(alongAll, cfg, rng)
	out.AcrossAll = dispersion(acrossAll, cfg, rng)
	out.Correlation = zCorrelation(tracks, func(s GEST1CorpusSample) bool { return s.OneStep }, cfg, rng)
	out.CorrelationA = zCorrelation(tracks, func(GEST1CorpusSample) bool { return true }, cfg, rng)
	out.AgeSplit = ageSplit(tracks, cfg)
	for f, st := range gest1Strata {
		for b, label := range st.labels {
			z := strata[sk{f, b}]
			if z == nil || z.n == 0 {
				continue
			}
			out.Strata = append(out.Strata, GEST1Stratum{Factor: st.factor, Bin: label, N: z.n,
				Tracks: len(strataTracks[sk{f, b}]), RMS: math.Sqrt(z.ss / float64(z.n)),
				Beyond: float64(z.beyond) / float64(z.n), BeyondCount: z.beyond})
		}
	}
	out.YoungTracks = len(young)
	if beyondTotal > 0 {
		out.YoungTailShare = float64(youngBeyond) / float64(beyondTotal)
	}
	for _, name := range jerkOrder {
		j := jerk[name]
		s := GEST1JerkSummary{Estimator: name, Tracks: jerkTracks[name], N: j.N}
		if j.N > 0 {
			s.ShareOver = float64(j.Over) / float64(j.N)
		}
		if j.Secs > 0 {
			s.SignChangesPerSec = float64(j.SignChanges) / j.Secs
		}
		out.Jerk = append(out.Jerk, s)
	}
	out.Turning.Shares = make([]float64, len(turnOver))
	for k, v := range turnOver {
		if out.Turning.Frames > 0 {
			out.Turning.Shares[k] = float64(v) / float64(out.Turning.Frames)
		}
	}
	if runs.Tracks > 0 {
		runs.Share = float64(runs.Flagged) / float64(runs.Tracks)
	}
	out.Criterion3 = runs
	out.Criterion4 = offlineTails(offAlong, offAcross)
	if len(occ.Cases) > 0 {
		var hs []float64
		for h := range horizons {
			hs = append(hs, h)
		}
		sort.Float64s(hs)
		out.Criterion5, out.Criterion5N = summariseOcclusion(occ, hs), len(occ.Cases)
	}
	return out
}

func offlineTails(along, across [][]float64) GEST1OfflineTails {
	t := GEST1OfflineTails{Models: append([]string(nil), GEST1OfflineModels...)}
	p99 := func(v []float64) float64 {
		if len(v) == 0 {
			return math.NaN()
		}
		s := append([]float64(nil), v...)
		sort.Float64s(s)
		return stats.NearestRank(s, 0.99)
	}
	t.NAlong, t.NAcross = len(along[0]), len(across[0])
	for m := range GEST1OfflineModels {
		t.AlongP99 = append(t.AlongP99, p99(along[m]))
		t.AcrossP99 = append(t.AcrossP99, p99(across[m]))
	}
	for m := range GEST1OfflineModels {
		t.AlongRatio = append(t.AlongRatio, t.AlongP99[m]/t.AlongP99[0])
		t.AcrossRatio = append(t.AcrossRatio, t.AcrossP99[m]/t.AcrossP99[0])
	}
	// NaN is not JSON: an empty model reads zero.
	for _, s := range [][]float64{t.AlongP99, t.AcrossP99, t.AlongRatio, t.AcrossRatio} {
		for i := range s {
			if !finite(s[i]) {
				s[i] = 0
			}
		}
	}
	return t
}

// String is a one-line summary of the restated read.
func (r GEST1CorpusRead) String() string {
	return fmt.Sprintf("tracks=%d along_z_rms=%.3f beyond=%.4f across_z_rms=%.3f r=%.3f p05=%.3f",
		r.Tracks, r.Along.RMS, r.Along.Beyond, r.Across.RMS, r.Correlation.R.R, r.Correlation.BootP05)
}

// WriteGEST1CorpusRecord writes a case's track records as JSON, gzipped when
// the path ends in .gz.
func WriteGEST1CorpusRecord(path string, rec GEST1CorpusRecord) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	var w io.Writer = f
	var zw *gzip.Writer
	if strings.HasSuffix(path, ".gz") {
		zw = gzip.NewWriter(f)
		w = zw
	}
	if err := json.NewEncoder(w).Encode(rec); err != nil {
		f.Close()
		return err
	}
	if zw != nil {
		if err := zw.Close(); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}

// LoadGEST1CorpusRecord reads what WriteGEST1CorpusRecord wrote.
func LoadGEST1CorpusRecord(path string) (GEST1CorpusRecord, error) {
	var rec GEST1CorpusRecord
	f, err := os.Open(path)
	if err != nil {
		return rec, err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return rec, fmt.Errorf("%s: %w", path, err)
		}
		defer zr.Close()
		r = zr
	}
	if err := json.NewDecoder(r).Decode(&rec); err != nil {
		return rec, fmt.Errorf("%s: %w", path, err)
	}
	return rec, nil
}
