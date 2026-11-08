package l8analytics

// G-EST-1 on two measurements (maths review 2026-11-08, Section 4.3). The
// tracked filter measures the cluster medoid, whose bias slides along a long
// body as it enters and leaves view; a CV filter reads that slide as
// acceleration it cannot follow, so its NIS correlates with acceleration for
// the wrong reason. The shadow solid body measures faces, and under the
// face-state options carries the half-extents in its state. GEST1Collector
// records both from one replay, per track and frame:
//
//   - the tracked filter, from the filter-step record (ObserveFilterFrame):
//     the live innovation against the prior, S = P⁻ + R·I, and the live NIS
//     with two degrees of freedom;
//   - the solid body, from the tracker after each update (ObserveTap): each
//     near-edge fix's joint face innovations (l5tracks.FaceInnovation),
//     rank-aware, and its NIS with Rank degrees of freedom. Medoid updates,
//     lapses and coasts of the body are not face-referenced and are not
//     recorded.
//
// Point count, which neither record carries, is joined from the tracker's
// input: the points of the cluster associated with the track that frame.
//
// BuildGEST1Report then reads each source as one arm of criteria 2 to 5. The
// smoothed acceleration both read is the same estimator over each source's
// own observations (smoothedAcceleration), so the arms differ in the
// measurement and nothing else.

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks/motion"
	"github.com/banshee-data/velocity.report/internal/stats"
)

// GEST1 residual sources: the two measurements a G-EST-1 arm can read.
const (
	GEST1SourceTracked   = "tracked_filter"
	GEST1SourceSolidBody = "solid_body_faces"
)

// GEST1Residual is one observation's live residual. Along is the innovation's
// component along the prior course, positive ahead of the prediction, and
// Across along the course's left normal; AlongSD and AcrossSD are each
// component's predicted standard deviation, √(uᵀSu). A two-dimensional
// residual has both. A rank-one face fix measures only along its normal and
// has the one whose axis the normal lies nearer (|n·u| ≥ √½ is along), signed
// by the normal's projection on it. Without a course (prior speed below the
// collector's minimum) it has neither.
type GEST1Residual struct {
	UnixNanos      int64 `json:"unix_nanos"`
	FrameUnixNanos int64 `json:"frame_unix_nanos"`
	// Dim is the NIS's degrees of freedom: 2 for the tracked filter, the
	// fix's rank for a face fix.
	Dim int     `json:"dim"`
	NIS float64 `json:"nis"`

	Along     float64 `json:"along_m"`
	AlongSD   float64 `json:"along_sd_m"`
	Across    float64 `json:"across_m"`
	AcrossSD  float64 `json:"across_sd_m"`
	HasAlong  bool    `json:"has_along"`
	HasAcross bool    `json:"has_across"`
	// Points is the associated cluster's point count, zero when the tap did
	// not see the frame.
	Points int `json:"points"`
	// PriorSecs is the time since the filter's previous update, which its
	// prior was predicted across: for the tracked filter since the chain's
	// previous applied observation, for a face fix since the solid body's
	// previous update of any kind; NaN when unknown. It is not the filter
	// step's PredictedSecs, which counts one frame however many frames went
	// unobserved before it. G-EST-1's criteria 2 to 4 are one-step tests,
	// which this lets a read restrict to.
	PriorSecs float64 `json:"prior_s"`
	// ReferenceChange marks a face fix that re-referenced a
	// medoid-referenced body to its centre: its prior described a point on
	// the near surface, so its innovation carries the medoid's offset and is
	// not a face-referenced residual.
	ReferenceChange bool `json:"reference_change"`
}

// GEST1Chain is one track chain's record from one source: its live residuals
// and the observation sequence the offline estimators read, each observation
// with the frame it came from.
type GEST1Chain struct {
	TrackID          string
	CreationSequence int64
	Confirmed        bool
	Residuals        []GEST1Residual
	Observations     []motion.Observation
	ObservationFrame []int64
	// LeadingRankOne counts face fixes dropped before the chain's first
	// two-dimensional one: an offline filter cannot be founded on a line.
	LeadingRankOne int
}

// GEST1CollectorConfig sets how residuals are read.
type GEST1CollectorConfig struct {
	// MeasurementVariance is the tracked filter's scalar R (measurement_noise);
	// zero means motion.DefaultMeasurementVariance.
	MeasurementVariance float64
	// MinCourseSpeed is the prior speed below which a residual has no course
	// to decompose along; zero means 0.3 m/s.
	MinCourseSpeed float64
}

// GEST1Collector gathers both sources during a replay. Attach it as the
// tracker's filter-step observer and call ObserveTap after every update.
type GEST1Collector struct {
	cfg    GEST1CollectorConfig
	chains map[string]map[string]*GEST1Chain // source → chain key
	points map[pointKey]int
	scored map[int64]bool
	// lastFix is the capture time of each chain's latest recorded face fix:
	// a reading persists between updates, so a fix is new only once.
	lastFix map[string]int64
	// lastUpdate is each chain's solid-body update time as last read, of any
	// source, so a fix's prior interval is the time since the one before.
	lastUpdate map[string]int64
	// nisRecompute is the largest relative difference between a tracked
	// residual's live NIS and yᵀS⁻¹y from its record, and solidRecompute
	// the same for a face fix's joint NIS: the checks that the record is
	// the filter's.
	nisRecompute, solidRecompute float64
}

type pointKey struct {
	trackID string
	frame   int64
}

// NewGEST1Collector returns an empty collector.
func NewGEST1Collector(cfg GEST1CollectorConfig) *GEST1Collector {
	if cfg.MeasurementVariance <= 0 {
		cfg.MeasurementVariance = motion.DefaultMeasurementVariance
	}
	if cfg.MinCourseSpeed <= 0 {
		cfg.MinCourseSpeed = 0.3
	}
	return &GEST1Collector{
		cfg: cfg, chains: map[string]map[string]*GEST1Chain{GEST1SourceTracked: {}, GEST1SourceSolidBody: {}},
		points: map[pointKey]int{}, scored: map[int64]bool{}, lastFix: map[string]int64{},
		lastUpdate: map[string]int64{},
	}
}

func gest1ChainKey(id string, seq int64) string { return id + "\x00" + strconv.FormatInt(seq, 10) }

func (c *GEST1Collector) chain(source, id string, seq int64) *GEST1Chain {
	key := gest1ChainKey(id, seq)
	ch := c.chains[source][key]
	if ch == nil {
		ch = &GEST1Chain{TrackID: id, CreationSequence: seq}
		c.chains[source][key] = ch
	}
	return ch
}

// ObserveFilterFrame implements l5tracks.FilterStepObserver: the tracked
// filter's applied steps become observations, and those with an innovation
// residuals.
func (c *GEST1Collector) ObserveFilterFrame(frame l5tracks.FilterFrame) {
	adapter := motion.AdapterConfig{MeasurementVariance: c.cfg.MeasurementVariance}
	for _, step := range frame.Steps {
		if !step.Observed || step.UpdateRefused || !step.Observation.Applied() {
			continue
		}
		ch := c.chain(GEST1SourceTracked, step.TrackID, step.CreationSequence)
		ch.Confirmed = ch.Confirmed || step.Confirmed
		obs := adapter.Observation(step)
		prior := math.NaN()
		if n := len(ch.Observations); n > 0 {
			if obs.UnixNanos <= ch.Observations[n-1].UnixNanos {
				continue
			}
			prior = float64(obs.UnixNanos-ch.Observations[n-1].UnixNanos) / 1e9
		}
		ch.Observations = append(ch.Observations, obs)
		ch.ObservationFrame = append(ch.ObservationFrame, step.FrameUnixNanos)
		if !step.Observation.HasInnovation {
			continue
		}
		o, pr := step.Observation, step.Prior
		y := [2]float64{float64(o.InnovationX), float64(o.InnovationY)}
		r := c.cfg.MeasurementVariance
		s := [2][2]float64{
			{float64(pr.P[0]) + r, float64(pr.P[1])},
			{float64(pr.P[4]), float64(pr.P[5]) + r},
		}
		res := GEST1Residual{UnixNanos: obs.UnixNanos, FrameUnixNanos: step.FrameUnixNanos, Dim: 2, NIS: float64(o.NIS),
			PriorSecs: prior}
		c.decomposeTwo(&res, y, s, float64(pr.VX), float64(pr.VY))
		if nis, ok := quadForm2(y, s); ok {
			c.nisRecompute = math.Max(c.nisRecompute, relDiff(nis, res.NIS))
		}
		ch.Residuals = append(ch.Residuals, res)
	}
}

// ObserveTap reads the tracker after one update: the points of each
// associated cluster, whether the frame is scored, and every track's new
// near-edge fix. associations is indexed like clusters and names the track
// each updated; tracks are the active tracks after the update.
func (c *GEST1Collector) ObserveTap(frameNanos int64, scored bool, clusters []l5tracks.WorldCluster, associations []string, tracks []*l5tracks.TrackedObject) {
	c.scored[frameNanos] = scored
	for i, id := range associations {
		if id != "" && i < len(clusters) {
			c.points[pointKey{id, frameNanos}] = clusters[i].PointsCount
		}
	}
	for _, tr := range tracks {
		reading, ok := tr.SolidBody()
		if !ok {
			continue
		}
		key := gest1ChainKey(tr.TrackID, tr.CreationSequence)
		at := reading.Estimate.LastObservedUnixNanos
		prior := math.NaN()
		if prev, seen := c.lastUpdate[key]; seen && at > prev {
			prior = float64(at-prev) / 1e9
		}
		c.lastUpdate[key] = at
		c.ObserveSolidBodyFix(tr.TrackID, tr.CreationSequence, tr.TrackState == l5tracks.TrackConfirmed,
			reading.Measurement, at, frameNanos, prior)
	}
}

// ObserveSolidBodyFix records one near-edge fix of a track's solid body,
// captured at unixNanos and read in the update of frameNanos, its prior
// predicted across priorSecs (NaN when unknown). A reading
// persists between updates, so a fix no later than the chain's last recorded
// one is the same fix and is ignored, as is a measurement with no face
// innovations.
func (c *GEST1Collector) ObserveSolidBodyFix(trackID string, seq int64, confirmed bool, m l5tracks.SolidBodyMeasurement, unixNanos, frameNanos int64, priorSecs float64) {
	if m.Source != l5tracks.MeasurementNearEdgeCandidateV1 || m.FaceInnovationCount == 0 {
		return
	}
	key := gest1ChainKey(trackID, seq)
	if last, seen := c.lastFix[key]; seen && unixNanos <= last {
		return
	}
	c.lastFix[key] = unixNanos
	ch := c.chain(GEST1SourceSolidBody, trackID, seq)
	ch.Confirmed = ch.Confirmed || confirmed
	c.addFaceFix(ch, m, unixNanos, frameNanos, priorSecs)
}

// addFaceFix records one fix's residual and its observation for the offline
// estimators. The observation is the body centre the faces imply at the
// predicted half-extents, along each face's normal: z_k = n_k·c⁻ + y_k. Its
// variance along n_k is the face's noise plus the predicted half-extent's
// variance (zero without the extent state), so the offline filters, which
// carry no extents, are told how far the centre the face implies can be from
// it. The position–extent covariance is not carried over; the offline
// filters are labels and comparisons, not the solid body.
func (c *GEST1Collector) addFaceFix(ch *GEST1Chain, m l5tracks.SolidBodyMeasurement, at, frame int64, priorSecs float64) {
	res := GEST1Residual{UnixNanos: at, FrameUnixNanos: frame, Dim: m.FaceInnovationCount, NIS: float64(m.NIS),
		PriorSecs: priorSecs, ReferenceChange: m.ReferenceChange == l5tracks.ReferenceToBodyCentre}
	if joint, ok := m.JointFaceNIS(); ok {
		c.solidRecompute = math.Max(c.solidRecompute, relDiff(joint, res.NIS))
	}
	px, py := float64(m.PriorX), float64(m.PriorY)
	vx, vy := float64(m.PriorVX), float64(m.PriorVY)
	obs := motion.Observation{UnixNanos: at, LiveNIS: res.NIS, HasLiveNIS: true}
	f0 := m.FaceInnovations[0]
	n0 := [2]float64{float64(f0.NormalX), float64(f0.NormalY)}
	v0 := float64(f0.NoiseVariance) + float64(f0.ExtentVariance)
	if m.FaceInnovationCount == 2 {
		f1 := m.FaceInnovations[1]
		n1 := [2]float64{float64(f1.NormalX), float64(f1.NormalY)}
		y0, y1 := float64(f0.Innovation), float64(f1.Innovation)
		// The two normals are perpendicular (one longitudinal face, one
		// lateral), so N = [n0; n1] is orthonormal and the residual vector
		// and its covariance are Nᵀy and Nᵀ S N.
		y := [2]float64{n0[0]*y0 + n1[0]*y1, n0[1]*y0 + n1[1]*y1}
		s00, s11, s01 := float64(f0.Variance), float64(f1.Variance), float64(m.FaceInnovationCross)
		var s [2][2]float64
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				s[i][j] = s00*n0[i]*n0[j] + s01*(n0[i]*n1[j]+n1[i]*n0[j]) + s11*n1[i]*n1[j]
			}
		}
		c.decomposeTwo(&res, y, s, vx, vy)
		v1 := float64(f1.NoiseVariance) + float64(f1.ExtentVariance)
		obs.X, obs.Y = px+y[0], py+y[1]
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				obs.R[i][j] = v0*n0[i]*n0[j] + v1*n1[i]*n1[j]
			}
		}
	} else {
		y0 := float64(f0.Innovation)
		c.decomposeOne(&res, y0, float64(f0.Variance), n0, vx, vy)
		obs.X, obs.Y = px+y0*n0[0], py+y0*n0[1]
		obs.RankOne, obs.NX, obs.NY = true, n0[0], n0[1]
		obs.R = motion.IsotropicR(v0)
	}
	ch.Residuals = append(ch.Residuals, res)
	if obs.RankOne && len(ch.Observations) == 0 {
		ch.LeadingRankOne++
		return
	}
	ch.Observations = append(ch.Observations, obs)
	ch.ObservationFrame = append(ch.ObservationFrame, frame)
}

func (c *GEST1Collector) course(vx, vy float64) (ux, uy float64, ok bool) {
	speed := math.Hypot(vx, vy)
	if !(speed > 0) || speed < c.cfg.MinCourseSpeed {
		return 0, 0, false
	}
	return vx / speed, vy / speed, true
}

// decomposeTwo projects a two-dimensional residual on the course and its
// left normal.
func (c *GEST1Collector) decomposeTwo(res *GEST1Residual, y [2]float64, s [2][2]float64, vx, vy float64) {
	ux, uy, ok := c.course(vx, vy)
	if !ok {
		return
	}
	lx, ly := -uy, ux
	res.Along, res.AlongSD = ux*y[0]+uy*y[1], math.Sqrt(math.Max(0, quad2(s, ux, uy)))
	res.Across, res.AcrossSD = lx*y[0]+ly*y[1], math.Sqrt(math.Max(0, quad2(s, lx, ly)))
	res.HasAlong, res.HasAcross = res.AlongSD > 0, res.AcrossSD > 0
}

// decomposeOne attributes a rank-one residual to the axis its normal lies
// nearer.
func (c *GEST1Collector) decomposeOne(res *GEST1Residual, y, s float64, n [2]float64, vx, vy float64) {
	ux, uy, ok := c.course(vx, vy)
	if !ok || !(s > 0) {
		return
	}
	along := n[0]*ux + n[1]*uy
	across := n[0]*(-uy) + n[1]*ux
	sd := math.Sqrt(s)
	if math.Abs(along) >= math.Sqrt2/2 {
		res.Along, res.AlongSD, res.HasAlong = y*math.Copysign(1, along), sd, true
	} else {
		res.Across, res.AcrossSD, res.HasAcross = y*math.Copysign(1, across), sd, true
	}
}

func quad2(s [2][2]float64, a, b float64) float64 {
	return a*a*s[0][0] + a*b*(s[0][1]+s[1][0]) + b*b*s[1][1]
}

func quadForm2(y [2]float64, s [2][2]float64) (float64, bool) {
	det := s[0][0]*s[1][1] - s[0][1]*s[1][0]
	if !(s[0][0] > 0) || !(det > 0) {
		return 0, false
	}
	return (s[1][1]*y[0]*y[0] - (s[0][1]+s[1][0])*y[0]*y[1] + s[0][0]*y[1]*y[1]) / det, true
}

func relDiff(a, b float64) float64 {
	return math.Abs(a-b) / math.Max(1, math.Abs(b))
}

// Chains returns a source's chains, by creation sequence then ID, with each
// residual's point count joined from the tap.
func (c *GEST1Collector) Chains(source string) []GEST1Chain {
	out := make([]GEST1Chain, 0, len(c.chains[source]))
	for _, ch := range c.chains[source] {
		cp := *ch
		cp.Residuals = append([]GEST1Residual(nil), ch.Residuals...)
		for i := range cp.Residuals {
			cp.Residuals[i].Points = c.points[pointKey{ch.TrackID, cp.Residuals[i].FrameUnixNanos}]
		}
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreationSequence != out[j].CreationSequence {
			return out[i].CreationSequence < out[j].CreationSequence
		}
		return out[i].TrackID < out[j].TrackID
	})
	return out
}

// Scored reports whether the tap marked the frame as in the scoring window.
func (c *GEST1Collector) Scored(frameNanos int64) bool { return c.scored[frameNanos] }

var _ l5tracks.FilterStepObserver = (*GEST1Collector)(nil)

// ---------------------------------------------------------------------------
// The report.
// ---------------------------------------------------------------------------

// GEST1Config sets how BuildGEST1Report reads the collected sources.
type GEST1Config struct {
	// SensorX and SensorY are the sensor origin the range and aspect are
	// measured from, in the tracker's frame.
	SensorX float64 `json:"sensor_x_m"`
	SensorY float64 `json:"sensor_y_m"`
	// MovingSpeed is the smoothed speed a confirmed chain must reach to be a
	// moving track, MinObservations the observations it needs.
	MovingSpeed     float64 `json:"moving_speed_mps"`
	MinObservations int     `json:"min_observations"`
	// MinCourseSpeed is the smoothed speed below which a sample has no
	// course, so no longitudinal acceleration and no aspect.
	MinCourseSpeed float64 `json:"min_course_speed_mps"`
	// AccelHalfWindowSecs is the centred difference's half-width:
	// a(t) = (v(t + h) − v(t − h)) / 2h over the smoothed velocity, at the
	// nearest observations at or beyond ±h; AccelMaxSpanSecs the widest
	// those may be apart.
	AccelHalfWindowSecs float64 `json:"accel_half_window_s"`
	AccelMaxSpanSecs    float64 `json:"accel_max_span_s"`
	// Level is the Fisher interval's coverage.
	Level float64 `json:"level"`
	// InteriorMarginSecs is the distance from a chain's ends that the
	// interior read of criterion 2 keeps its samples beyond.
	InteriorMarginSecs float64 `json:"interior_margin_s"`
	// CrossReadToleranceSecs is how far apart a tracked residual and the
	// solid body's observation of the same track may be for the cross-read.
	CrossReadToleranceSecs float64 `json:"cross_read_tolerance_s"`
	// BootstrapDraws is the number of whole-track resamples behind criterion
	// 2's primary and cross-read bootstrap intervals; zero skips them.
	// BootstrapSeed seeds them.
	BootstrapDraws int   `json:"bootstrap_draws"`
	BootstrapSeed  int64 `json:"bootstrap_seed"`
	// Occlusion is criterion 5's configuration.
	Occlusion OcclusionConfig `json:"occlusion"`
}

// DefaultGEST1Config is the configuration l5est0005's rule names.
func DefaultGEST1Config(maxCoastSecs float64) GEST1Config {
	return GEST1Config{
		MovingSpeed: 2.0, MinObservations: 10, MinCourseSpeed: 0.5,
		AccelHalfWindowSecs: 0.5, AccelMaxSpanSecs: 1.5, Level: 0.95, CrossReadToleranceSecs: 0.05,
		InteriorMarginSecs: 1.5, BootstrapDraws: 2000, BootstrapSeed: 20261108,
		Occlusion: DefaultOcclusionConfig(maxCoastSecs),
	}
}

// GEST1DimSummary is a source's NIS at one dimension: how many, its mean,
// and the share above the χ² 95 % point (3.841 at one, 5.991 at two).
type GEST1DimSummary struct {
	Dim         int     `json:"dim"`
	N           int     `json:"n"`
	MeanNIS     float64 `json:"mean_nis"`
	Above95     float64 `json:"share_above_chi2_95"`
	MeanPerDim  float64 `json:"mean_nis_per_dim"`
	MedianNIS   float64 `json:"median_nis"`
	P95NIS      float64 `json:"p95_nis"`
	MeanAbsAccl float64 `json:"mean_abs_longitudinal_accel_mps2"`
}

// GEST1Correlation is criterion 2 over one sample set: the partial
// correlation of NIS/dim with |longitudinal acceleration|, controlling for
// range, aspect, point count and the dimension (dropped when constant).
type GEST1Correlation struct {
	Label string                   `json:"label"`
	N     int                      `json:"n"`
	Err   string                   `json:"error,omitempty"`
	R     PartialCorrelationResult `json:"partial_correlation"`
	// Excludes zero: the interval lies wholly above or below it.
	ExcludesZero bool `json:"interval_excludes_zero"`
	// Tracks is how many chains the samples come from. Samples within a
	// track are not independent, so the Fisher interval, which assumes they
	// are, is too narrow when tracks are few. BootLo and BootHi are the
	// percentile interval at the same level over BootDraws resamples of whole
	// tracks with replacement (seeded, so a rerun reproduces it), and
	// BootExcludesZero says whether it too lies off zero; zero draws when not
	// computed.
	Tracks           int     `json:"tracks"`
	BootDraws        int     `json:"bootstrap_draws,omitempty"`
	BootLo           float64 `json:"bootstrap_lo"`
	BootHi           float64 `json:"bootstrap_hi"`
	BootExcludesZero bool    `json:"bootstrap_interval_excludes_zero"`
	BootFailed       int     `json:"bootstrap_failed_draws,omitempty"`
}

// GEST1ResidualTails is criterion 4: offline CV and CA (with the coast
// rule) over the same observations, p99 of |along| and |across| one-step
// innovations in metres, and the ratios CA/CV.
type GEST1ResidualTails struct {
	NAlong      int     `json:"n_along"`
	NAcross     int     `json:"n_across"`
	CVAlongP99  float64 `json:"cv_along_p99_m"`
	CAAlongP99  float64 `json:"ca_along_p99_m"`
	CVAcrossP99 float64 `json:"cv_across_p99_m"`
	CAAcrossP99 float64 `json:"ca_across_p99_m"`
	AlongRatio  float64 `json:"along_ratio"`
	AcrossRatio float64 `json:"across_ratio"`
	// Pass is criterion 4: AlongRatio at most 0.8 and AcrossRatio at most
	// 1.05.
	Pass bool   `json:"pass"`
	Note string `json:"note,omitempty"`
}

// GEST1SourceReport is one source read as one arm.
type GEST1SourceReport struct {
	Source       string `json:"source"`
	Chains       int    `json:"chains"`
	MovingTracks int    `json:"moving_tracks"`
	// Residuals are the moving tracks' scored residuals; Samples those with
	// every covariate, which criterion 2 reads. The exclusions say why the
	// rest were not.
	Residuals          int     `json:"residuals"`
	Samples            int     `json:"samples"`
	NoAcceleration     int     `json:"excluded_no_acceleration"`
	NoCourse           int     `json:"excluded_no_course"`
	NoState            int     `json:"excluded_no_smoothed_state"`
	NoPoints           int     `json:"excluded_no_points"`
	LeadingRankOne     int     `json:"leading_rank_one_dropped"`
	FilterErrors       int     `json:"offline_filter_errors"`
	NISRecomputeMaxRel float64 `json:"nis_recompute_max_rel"`

	ByDim      []GEST1DimSummary `json:"nis_by_dim"`
	Criterion2 GEST1Correlation  `json:"criterion2"`
	// Criterion2By are the declared secondary reads: each dimension alone
	// (where both occur), the interior (samples at least
	// InteriorMarginSecs from both ends of their chain, where the smoothed
	// acceleration is not attenuated by the smoother's ends) and ranks (the
	// same partial correlation on rank-transformed columns, which one large
	// NIS cannot carry).
	Criterion2By []GEST1Correlation `json:"criterion2_secondary,omitempty"`
	Criterion3   RunLengthResult    `json:"criterion3"`
	Criterion4   GEST1ResidualTails `json:"criterion4"`
	Criterion5   []OcclusionSummary `json:"criterion5"`
	// Criterion5Violations are NoWorseThan's for each model against CV.
	Criterion5Violations map[string][]string `json:"criterion5_violations"`
	Criterion5Cases      int                 `json:"criterion5_cases"`
	// PostHoc are reads added after l5est0005's first replay, not part of
	// its rule: see GEST1PostHoc.
	PostHoc GEST1PostHoc `json:"post_hoc"`
}

// GEST1PostHoc are diagnostic reads added after l5est0005's declared
// replay showed three things its rule did not anticipate, kept apart from
// the declared criteria so neither can be mistaken for the other:
//
//   - the NIS is under-dispersed in total, so the along- and across-track
//     normalised residuals are given separately (RMS, share beyond 1.96);
//   - G-EST-1's criteria 2 to 4 are one-step tests, and residuals after a
//     gap (a miss, or frames without a usable face) are not one-step;
//   - a face fix that re-references a medoid-referenced body carries the
//     medoid's offset in its innovation.
//
// The one-step reads keep residuals whose prior interval is at most
// OneStepMaxSecs and that changed no reference, and criterion 4's offline
// steps whose interval is at most the same.
type GEST1PostHoc struct {
	OneStepMaxSecs float64 `json:"one_step_max_s"`
	// The moving tracks' scored residuals by what the one-step reads drop.
	MultiStep       int `json:"multi_step"`
	ReferenceChange int `json:"reference_change"`
	OneStep         int `json:"one_step"`
	// Normalised along and across residuals, over every scored residual of
	// the moving tracks and over the one-step ones.
	Along        GEST1ZSummary      `json:"along"`
	Across       GEST1ZSummary      `json:"across"`
	AlongOneStep GEST1ZSummary      `json:"along_one_step"`
	AcrossOne    GEST1ZSummary      `json:"across_one_step"`
	ByDimOneStep []GEST1DimSummary  `json:"nis_by_dim_one_step"`
	Criterion2   GEST1Correlation   `json:"criterion2_one_step"`
	Criterion3   RunLengthResult    `json:"criterion3_one_step"`
	Criterion4   GEST1ResidualTails `json:"criterion4_one_step"`
}

// GEST1ZSummary describes normalised residuals z = component / its sd: a
// consistent filter has RMS 1 and 5 % beyond 1.96.
type GEST1ZSummary struct {
	N        int     `json:"n"`
	RMS      float64 `json:"rms"`
	Mean     float64 `json:"mean"`
	Beyond   float64 `json:"share_beyond_1_96"`
	MedianAb float64 `json:"median_abs"`
}

func zSummary(z []float64) GEST1ZSummary {
	out := GEST1ZSummary{N: len(z)}
	if len(z) == 0 {
		return out
	}
	var ss, sum float64
	beyond := 0
	abs := make([]float64, len(z))
	for i, v := range z {
		ss += v * v
		sum += v
		if math.Abs(v) > 1.959963984540054 {
			beyond++
		}
		abs[i] = math.Abs(v)
	}
	sort.Float64s(abs)
	n := float64(len(z))
	out.RMS, out.Mean, out.Beyond = math.Sqrt(ss/n), sum/n, float64(beyond)/n
	out.MedianAb = stats.NearestRank(abs, 0.5)
	return out
}

// gest1OneStepSecs is the post-hoc reads' one-step bound: one 10 Hz frame
// with the same margin the offline filters' coast threshold allows.
const gest1OneStepSecs = 0.15

// GEST1Report is criteria 2 to 5 for every source the replay recorded, and
// the cross-read when both were: the tracked filter's NIS against the solid
// body's acceleration at the same track and time, which separates "the NIS
// follows acceleration" from "the medoid's slide drives both".
type GEST1Report struct {
	Config    GEST1Config         `json:"config"`
	Sources   []GEST1SourceReport `json:"sources"`
	CrossRead *GEST1Correlation   `json:"cross_read,omitempty"`
}

// gest1Sample is one residual with its covariates.
type gest1Sample struct {
	res                 GEST1Residual
	accel, rng, aspect  float64
	chainKey            string
	unixNanos           int64
	hasAccel, hasAspect bool
	interior            bool
}

// chainKinematics is a chain's smoothed motion at each observation.
type chainKinematics struct {
	times        []int64
	x, y, vx, vy []float64
	accelLong    []float64 // NaN without a window or a course
	moving       bool
	err          error
}

// BuildGEST1Report reads every source the collector holds.
func BuildGEST1Report(c *GEST1Collector, cfg GEST1Config) (GEST1Report, error) {
	if cfg.Level == 0 {
		cfg.Level = 0.95
	}
	rep := GEST1Report{Config: cfg}
	kin := map[string]map[string]chainKinematics{}
	samples := map[string][]gest1Sample{}
	for _, source := range []string{GEST1SourceTracked, GEST1SourceSolidBody} {
		chains := c.Chains(source)
		if len(chains) == 0 {
			continue
		}
		sr, k, s, err := buildGEST1Source(c, source, chains, cfg)
		if err != nil {
			return GEST1Report{}, fmt.Errorf("l8analytics: %s: %w", source, err)
		}
		sr.NISRecomputeMaxRel = c.nisRecompute
		if source == GEST1SourceSolidBody {
			sr.NISRecomputeMaxRel = c.solidRecompute
		}
		rep.Sources = append(rep.Sources, sr)
		kin[source], samples[source] = k, s
	}
	if len(kin[GEST1SourceTracked]) > 0 && len(kin[GEST1SourceSolidBody]) > 0 {
		cross := crossRead(samples[GEST1SourceTracked], kin[GEST1SourceSolidBody], cfg)
		rep.CrossRead = &cross
	}
	return rep, nil
}

func buildGEST1Source(c *GEST1Collector, source string, chains []GEST1Chain, cfg GEST1Config) (GEST1SourceReport, map[string]chainKinematics, []gest1Sample, error) {
	sr := GEST1SourceReport{Source: source, Chains: len(chains), Criterion5Violations: map[string][]string{}}
	kin := map[string]chainKinematics{}
	var samples []gest1Sample
	var runs [][]float64
	var scoredTracks []motion.TrackObservations
	var along [2][]float64
	var across [2][]float64
	var alongOne, acrossOne [2][]float64
	var oneStepSamples []gest1Sample
	var oneStepRuns [][]float64
	var zAlong, zAcross, zAlongOne, zAcrossOne []float64
	ph := GEST1PostHoc{OneStepMaxSecs: gest1OneStepSecs}
	for _, ch := range chains {
		sr.LeadingRankOne += ch.LeadingRankOne
		key := gest1ChainKey(ch.TrackID, ch.CreationSequence)
		k := smoothedKinematics(ch, cfg)
		kin[key] = k
		if k.err != nil {
			sr.FilterErrors++
			continue
		}
		if !ch.Confirmed || !k.moving || len(ch.Observations) < cfg.MinObservations {
			continue
		}
		sr.MovingTracks++
		var run, runOne []float64
		for _, r := range ch.Residuals {
			if !c.Scored(r.FrameUnixNanos) {
				continue
			}
			sr.Residuals++
			oneStep := r.PriorSecs <= gest1OneStepSecs && !r.ReferenceChange
			switch {
			case r.ReferenceChange:
				ph.ReferenceChange++
			case !(r.PriorSecs <= gest1OneStepSecs):
				ph.MultiStep++
			default:
				ph.OneStep++
			}
			if r.HasAlong {
				run = append(run, r.Along/r.AlongSD)
				zAlong = append(zAlong, r.Along/r.AlongSD)
				if oneStep {
					runOne = append(runOne, r.Along/r.AlongSD)
					zAlongOne = append(zAlongOne, r.Along/r.AlongSD)
				}
			}
			if r.HasAcross {
				zAcross = append(zAcross, r.Across/r.AcrossSD)
				if oneStep {
					zAcrossOne = append(zAcrossOne, r.Across/r.AcrossSD)
				}
			}
			s := gest1Sample{res: r, chainKey: key, unixNanos: r.UnixNanos}
			margin := int64(cfg.InteriorMarginSecs * 1e9)
			s.interior = r.UnixNanos-k.times[0] >= margin && k.times[len(k.times)-1]-r.UnixNanos >= margin
			i := k.index(r.UnixNanos)
			if i >= 0 {
				s.accel = k.accelLong[i]
				s.hasAccel = !math.IsNaN(s.accel)
				rng, asp := RangeAndAspect(k.x[i], k.y[i], k.vx[i], k.vy[i], cfg.SensorX, cfg.SensorY)
				s.rng, s.aspect = rng, asp
				s.hasAspect = !math.IsNaN(asp) && math.Hypot(k.vx[i], k.vy[i]) >= cfg.MinCourseSpeed
			}
			switch {
			case i < 0:
				// A leading rank-one fix: no observation founded the
				// offline filter yet, so there is no smoothed state.
				sr.NoState++
			case !s.hasAspect:
				sr.NoCourse++
			case !s.hasAccel:
				sr.NoAcceleration++
			case r.Points <= 0:
				sr.NoPoints++
			default:
				samples = append(samples, s)
				if oneStep {
					oneStepSamples = append(oneStepSamples, s)
				}
			}
		}
		if len(run) > 0 {
			runs = append(runs, run)
		}
		if len(runOne) > 0 {
			oneStepRuns = append(oneStepRuns, runOne)
		}
		scored := motion.TrackObservations{TrackID: ch.TrackID, CreationSequence: ch.CreationSequence}
		for i, o := range ch.Observations {
			if c.Scored(ch.ObservationFrame[i]) {
				scored.Observations = append(scored.Observations, o)
			}
		}
		for len(scored.Observations) > 0 && scored.Observations[0].RankOne {
			scored.Observations = scored.Observations[1:]
		}
		if len(scored.Observations) >= 2 {
			scoredTracks = append(scoredTracks, scored)
			tails, err := oneStepTails(scored.Observations, cfg.MinCourseSpeed)
			if err != nil {
				sr.FilterErrors++
			} else {
				for m := 0; m < 2; m++ {
					for _, v := range tails[m] {
						if v.hasAlong {
							along[m] = append(along[m], v.along)
							if v.dt <= gest1OneStepSecs {
								alongOne[m] = append(alongOne[m], v.along)
							}
						}
						if v.hasAcross {
							across[m] = append(across[m], v.across)
							if v.dt <= gest1OneStepSecs {
								acrossOne[m] = append(acrossOne[m], v.across)
							}
						}
					}
				}
			}
		}
	}
	sr.Samples = len(samples)
	sr.ByDim = dimSummaries(samples)
	sr.Criterion2 = correlate("all", samples, cfg.Level)
	bootstrapTracks(&sr.Criterion2, samples, cfg)
	for _, d := range []int{1, 2} {
		var sub []gest1Sample
		for _, s := range samples {
			if s.res.Dim == d {
				sub = append(sub, s)
			}
		}
		if len(sub) > 0 && len(sub) < len(samples) {
			sr.Criterion2By = append(sr.Criterion2By, correlate("dim"+strconv.Itoa(d), sub, cfg.Level))
		}
	}
	var interior []gest1Sample
	for _, s := range samples {
		if s.interior {
			interior = append(interior, s)
		}
	}
	sr.Criterion2By = append(sr.Criterion2By, correlate("interior", interior, cfg.Level))
	ranks := correlate("ranks", samples, cfg.Level)
	ranks.R, ranks.Err, ranks.ExcludesZero = rankPartialCorrelation(samples, cfg.Level)
	sr.Criterion2By = append(sr.Criterion2By, ranks)
	sr.Criterion3 = RunLengthSignature(runs, 3, 1)
	sr.Criterion4 = residualTails(along, across)
	ph.Along, ph.Across = zSummary(zAlong), zSummary(zAcross)
	ph.AlongOneStep, ph.AcrossOne = zSummary(zAlongOne), zSummary(zAcrossOne)
	ph.ByDimOneStep = dimSummaries(oneStepSamples)
	ph.Criterion2 = correlate("one_step", oneStepSamples, cfg.Level)
	bootstrapTracks(&ph.Criterion2, oneStepSamples, cfg)
	ph.Criterion3 = RunLengthSignature(oneStepRuns, 3, 1)
	ph.Criterion4 = residualTails(alongOne, acrossOne)
	sr.PostHoc = ph
	if len(scoredTracks) > 0 {
		occ, err := ScoreOcclusion(scoredTracks, DefaultOcclusionModels(), cfg.Occlusion)
		if err != nil {
			return sr, nil, nil, err
		}
		sr.Criterion5, sr.Criterion5Cases = occ.Summary, len(occ.Cases)
		for _, m := range occ.Models {
			if m != "cv" {
				sr.Criterion5Violations[m] = occ.NoWorseThan(m, "cv")
			}
		}
	}
	return sr, kin, samples, nil
}

// smoothedKinematics runs the offline CV filter over a chain's observations,
// smooths it over the whole chain (motion.SmoothCV), and takes the centred
// difference of the smoothed velocity. The smoother is non-causal, so the
// estimate has no lag in the sense a filter does: it is centred on each
// observation and averages over about 2h plus the smoother's own span. The
// same estimator runs on each source's own observations.
func smoothedKinematics(ch GEST1Chain, cfg GEST1Config) chainKinematics {
	var k chainKinematics
	if len(ch.Observations) == 0 {
		return k
	}
	steps, err := motion.RunFilter(motion.DefaultCVConfig(), ch.Observations)
	if err != nil {
		k.err = err
		return k
	}
	sm, err := motion.SmoothCV(steps)
	if err != nil {
		k.err = err
		return k
	}
	n := len(sm)
	k.times = make([]int64, n)
	k.x, k.y, k.vx, k.vy = make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n)
	k.accelLong = make([]float64, n)
	for i, e := range sm {
		k.times[i] = e.UnixNanos
		k.x[i], k.y[i], k.vx[i], k.vy[i] = e.X[motion.IdxX], e.X[motion.IdxY], e.X[motion.IdxVX], e.X[motion.IdxVY]
		if math.Hypot(k.vx[i], k.vy[i]) >= cfg.MovingSpeed {
			k.moving = true
		}
	}
	h := int64(cfg.AccelHalfWindowSecs * 1e9)
	span := int64(cfg.AccelMaxSpanSecs * 1e9)
	for i := range sm {
		k.accelLong[i] = math.NaN()
		lo := sort.Search(n, func(j int) bool { return k.times[j] > k.times[i]-h }) - 1
		hi := sort.Search(n, func(j int) bool { return k.times[j] >= k.times[i]+h })
		if lo < 0 || hi >= n || k.times[hi]-k.times[lo] > span {
			continue
		}
		dt := float64(k.times[hi]-k.times[lo]) / 1e9
		ax, ay := (k.vx[hi]-k.vx[lo])/dt, (k.vy[hi]-k.vy[lo])/dt
		kn := Kinematics{VX: k.vx[i], VY: k.vy[i], AX: ax, AY: ay}
		k.accelLong[i] = kn.Longitudinal(cfg.MinCourseSpeed)
	}
	return k
}

// index is the observation at exactly unixNanos, or −1.
func (k chainKinematics) index(unixNanos int64) int {
	i := sort.Search(len(k.times), func(j int) bool { return k.times[j] >= unixNanos })
	if i < len(k.times) && k.times[i] == unixNanos {
		return i
	}
	return -1
}

// nearest is the observation nearest unixNanos within tol, or −1.
func (k chainKinematics) nearest(unixNanos, tol int64) int {
	i := sort.Search(len(k.times), func(j int) bool { return k.times[j] >= unixNanos })
	best, bestD := -1, tol+1
	for _, j := range []int{i - 1, i} {
		if j < 0 || j >= len(k.times) {
			continue
		}
		d := k.times[j] - unixNanos
		if d < 0 {
			d = -d
		}
		if d < bestD {
			best, bestD = j, d
		}
	}
	return best
}

func trackCount(samples []gest1Sample) int {
	seen := map[string]bool{}
	for _, s := range samples {
		seen[s.chainKey] = true
	}
	return len(seen)
}

// gest1Columns are criterion 2's columns: NIS per degree of freedom, the
// absolute longitudinal acceleration, and the covariates range, aspect,
// point count and dimension.
func gest1Columns(samples []gest1Sample) (y, x []float64, cov [][]float64) {
	n := len(samples)
	y, x = make([]float64, n), make([]float64, n)
	cov = [][]float64{make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n)}
	for i, s := range samples {
		y[i] = s.res.NIS / float64(s.res.Dim)
		x[i] = math.Abs(s.accel)
		cov[0][i], cov[1][i], cov[2][i], cov[3][i] = s.rng, s.aspect, float64(s.res.Points), float64(s.res.Dim)
	}
	return y, x, cov
}

// rankPartialCorrelation is criterion 2 on rank-transformed columns (average
// ranks for ties): a partial Spearman correlation, whose Fisher interval is
// the same approximation's.
func rankPartialCorrelation(samples []gest1Sample, level float64) (PartialCorrelationResult, string, bool) {
	y, x, cov := gest1Columns(samples)
	y, x = averageRanks(y), averageRanks(x)
	for k := range cov {
		cov[k] = averageRanks(cov[k])
	}
	r, err := PartialCorrelation(y, x, cov, level)
	if err != nil {
		return r, err.Error(), false
	}
	return r, "", r.Lo > 0 || r.Hi < 0
}

// averageRanks returns each value's rank from 1, ties sharing their mean.
func averageRanks(v []float64) []float64 {
	idx := make([]int, len(v))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return v[idx[a]] < v[idx[b]] })
	out := make([]float64, len(v))
	for i := 0; i < len(idx); {
		j := i
		for j+1 < len(idx) && v[idx[j+1]] == v[idx[i]] {
			j++
		}
		r := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			out[idx[k]] = r
		}
		i = j + 1
	}
	return out
}

// bootstrapTracks sets c's track-cluster bootstrap interval: draw as many
// tracks as there are, with replacement, pool their samples, and take the
// partial correlation; the interval is the draws' percentiles at the level.
// A draw whose correlation is undefined (too few samples, a constant
// column) is counted and left out.
func bootstrapTracks(c *GEST1Correlation, samples []gest1Sample, cfg GEST1Config) {
	if cfg.BootstrapDraws <= 0 || c.Err != "" {
		return
	}
	var keys []string
	byTrack := map[string][]gest1Sample{}
	for _, s := range samples {
		if _, ok := byTrack[s.chainKey]; !ok {
			keys = append(keys, s.chainKey)
		}
		byTrack[s.chainKey] = append(byTrack[s.chainKey], s)
	}
	if len(keys) < 2 {
		return
	}
	rng := rand.New(rand.NewSource(cfg.BootstrapSeed))
	var rs []float64
	draw := make([]gest1Sample, 0, len(samples))
	for d := 0; d < cfg.BootstrapDraws; d++ {
		draw = draw[:0]
		for range keys {
			draw = append(draw, byTrack[keys[rng.Intn(len(keys))]]...)
		}
		y, x, cov := gest1Columns(draw)
		r, err := PartialCorrelation(y, x, cov, cfg.Level)
		if err != nil {
			c.BootFailed++
			continue
		}
		rs = append(rs, r.R)
	}
	c.BootDraws = cfg.BootstrapDraws
	if len(rs) == 0 {
		return
	}
	sort.Float64s(rs)
	tail := (1 - cfg.Level) / 2
	c.BootLo, c.BootHi = stats.NearestRank(rs, tail), stats.NearestRank(rs, 1-tail)
	c.BootExcludesZero = c.BootLo > 0 || c.BootHi < 0
}

func correlate(label string, samples []gest1Sample, level float64) GEST1Correlation {
	out := GEST1Correlation{Label: label, N: len(samples), Tracks: trackCount(samples)}
	y, x, cov := gest1Columns(samples)
	r, err := PartialCorrelation(y, x, cov, level)
	out.R = r
	if err != nil {
		out.Err = err.Error()
		return out
	}
	out.ExcludesZero = r.Lo > 0 || r.Hi < 0
	return out
}

func dimSummaries(samples []gest1Sample) []GEST1DimSummary {
	var out []GEST1DimSummary
	for _, d := range []int{1, 2} {
		var nis []float64
		var acc float64
		for _, s := range samples {
			if s.res.Dim == d {
				nis = append(nis, s.res.NIS)
				acc += math.Abs(s.accel)
			}
		}
		if len(nis) == 0 {
			continue
		}
		crit := 3.841458820694124
		if d == 2 {
			crit = 5.991464547107979
		}
		var sum float64
		above := 0
		for _, v := range nis {
			sum += v
			if v > crit {
				above++
			}
		}
		sort.Float64s(nis)
		n := float64(len(nis))
		out = append(out, GEST1DimSummary{
			Dim: d, N: len(nis), MeanNIS: sum / n, MeanPerDim: sum / n / float64(d), Above95: float64(above) / n,
			MedianNIS: stats.NearestRank(nis, 0.5), P95NIS: stats.NearestRank(nis, 0.95), MeanAbsAccl: acc / n,
		})
	}
	return out
}

// offlineStep is one offline innovation's absolute components and the
// interval its prior was predicted across.
type offlineStep struct {
	along, across       float64
	hasAlong, hasAcross bool
	dt                  float64
}

// oneStepTails runs offline CV and CA with the coast rule over the
// observations and returns each model's |along| and |across| innovations,
// against its own prior course, rank-aware as the live residuals are, with
// each step's prediction interval. The declared criterion 4 reads them all;
// the post-hoc read keeps the one-step ones.
func oneStepTails(obs []motion.Observation, minCourse float64) (out [2][]offlineStep, err error) {
	for m, cfg := range []motion.FilterConfig{motion.DefaultCVConfig(), motion.DefaultCAConfig()} {
		steps, err := motion.RunFilter(cfg, obs)
		if err != nil {
			return out, err
		}
		dec := &GEST1Collector{cfg: GEST1CollectorConfig{MinCourseSpeed: minCourse}}
		for i, s := range steps {
			if s.Founded {
				continue
			}
			var r GEST1Residual
			vx, vy := s.Prior.X[motion.IdxVX], s.Prior.X[motion.IdxVY]
			in := s.Innovation
			if in.Dim == 2 {
				dec.decomposeTwo(&r, in.Y, in.S, vx, vy)
			} else {
				nx, ny := obs[i].NX, obs[i].NY
				norm := math.Hypot(nx, ny)
				dec.decomposeOne(&r, in.Y[0], in.S[0][0], [2]float64{nx / norm, ny / norm}, vx, vy)
			}
			if r.HasAlong || r.HasAcross {
				out[m] = append(out[m], offlineStep{along: math.Abs(r.Along), across: math.Abs(r.Across),
					hasAlong: r.HasAlong, hasAcross: r.HasAcross, dt: s.DtSecs})
			}
		}
	}
	return out, nil
}

func residualTails(along, across [2][]float64) GEST1ResidualTails {
	var t GEST1ResidualTails
	t.NAlong, t.NAcross = len(along[0]), len(across[0])
	if len(along[0]) == 0 || len(along[1]) == 0 || len(across[0]) == 0 || len(across[1]) == 0 {
		t.Note = "no residuals on one axis"
		return t
	}
	p99 := func(v []float64) float64 {
		s := append([]float64(nil), v...)
		sort.Float64s(s)
		return stats.NearestRank(s, 0.99)
	}
	t.CVAlongP99, t.CAAlongP99 = p99(along[0]), p99(along[1])
	t.CVAcrossP99, t.CAAcrossP99 = p99(across[0]), p99(across[1])
	if t.CVAlongP99 > 0 {
		t.AlongRatio = t.CAAlongP99 / t.CVAlongP99
	}
	if t.CVAcrossP99 > 0 {
		t.AcrossRatio = t.CAAcrossP99 / t.CVAcrossP99
	}
	t.Pass = t.AlongRatio > 0 && t.AlongRatio <= 0.8 && t.AcrossRatio <= 1.05
	return t
}

// crossRead correlates the tracked filter's NIS with the solid body's
// acceleration for the same track at the same time, keeping the tracked
// sample's own geometry covariates.
func crossRead(tracked []gest1Sample, solid map[string]chainKinematics, cfg GEST1Config) GEST1Correlation {
	tol := int64(cfg.CrossReadToleranceSecs * 1e9)
	var joined []gest1Sample
	for _, s := range tracked {
		k, ok := solid[s.chainKey]
		if !ok || k.err != nil {
			continue
		}
		i := k.nearest(s.unixNanos, tol)
		if i < 0 || math.IsNaN(k.accelLong[i]) {
			continue
		}
		s.accel = k.accelLong[i]
		joined = append(joined, s)
	}
	out := correlate("tracked_nis_against_solid_body_acceleration", joined, cfg.Level)
	bootstrapTracks(&out, joined, cfg)
	return out
}
