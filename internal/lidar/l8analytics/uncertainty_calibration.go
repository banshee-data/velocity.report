package l8analytics

// Measurement-noise calibration and NIS consistency (state-estimation plan
// Phase 3, Section 8.3, gate G-UNC-1).
//
// Input: the pre-gate samples a replay's calibration window recorded
// (l5tracks.UncertaintySample), each an eligible pairing decomposed along and
// across the sensor's line of sight. Output: an UncertaintyReport holding
//
//   - NIS consistency before the gate, by measurement dimension, with 95%
//     interval coverage and a chi-squared goodness-of-fit test, beside the
//     accepted-only figures the gate censors;
//   - the stratified checks G-UNC-1 names: range deciles, point-count deciles
//     and aspect octants, each against the overall normalised mean;
//   - a per-stratum fit of the adaptive model's coefficients, and the
//     coefficient table (CalibrationTable) a later replay can load.
//
// The fit is innovation-based. For a consistent filter the innovation along
// an axis u, normalised by its own predicted variance, is standard normal:
// (uᵀy)² / (uᵀHPHᵀu + uᵀRu) follows chi-squared with one degree of freedom.
// Per stratum and axis the fit finds the coefficient c for which a statistic
// of
//
//	(uᵀy - centre)² / (uᵀHPHᵀu + phi_u + c)
//
// over the stratum's samples meets its consistent value. The default,
// EstimatorMean, matches the mean about the mean to 1: the statistic
// G-UNC-1 judges, so every eligible observation counts, tail included.
// EstimatorMedian matches the median about the median to the chi-squared(1)
// median: robust, so a few manoeuvres or spurious returns recorded before the
// gate cannot set R for every ordinary observation, but blind to the tail the
// gate is judged on. On kirk0 the two disagree by a factor of several; that
// disagreement is a heavy tail, reported rather than chosen away.
//
// Each sample is normalised by its own predicted covariance, and only samples
// whose track was observed on the previous frame enter. The textbook moment
// estimate, Var(uᵀy) - mean(uᵀHPHᵀu) - mean(phi_u), assumes one S per
// stratum; on a real replay S varies from track to track, the mean of P is set
// by the largest, and the estimate can go negative while the typical sample is
// overconfident. It is still reported, as MomentCoefficient, because that gap
// is itself evidence about the process noise. A coasting track's P carries
// OcclusionCovInflation, an allowance for a missed frame rather than anything
// about the measurement, so R is not fitted to absorb it; coasting samples are
// still in every consistency figure, and split out in CoastSplit.
//
// It is centred. The spread is measured about the stratum's own centre, so a
// biased stratum is not given a wider R to cover its bias; it is flagged
// instead, because the plan requires bias to be corrected by the measurement
// model rather than inflated away.
//
// It is one step of a fixed-point iteration. A smaller R shrinks the
// filter's own P, which moves the next fit, so a table is refitted from a
// replay that used it until every fitted coefficient agrees with the one the
// replay ran with (UncertaintyFit.Converged). It attributes to R whatever the
// predicted covariance does not explain, process-model error included; that
// is the limit of innovation-based estimation, and the reason G-UNC-1 is
// judged on a held-out partition, stratum by stratum, with the gate-rejection
// evidence beside it.
//
// Every function here is pure: no I/O, no clock, no randomness, and the result
// does not depend on the order of the samples.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// UncertaintyReportSchemaVersion is the report's schema version.
const UncertaintyReportSchemaVersion = 1

// G-UNC-1 thresholds, Section 8.3.
const (
	gUNC1MeanLow        = 0.7
	gUNC1MeanHigh       = 1.4
	gUNC1GOFAlpha       = 0.01
	gUNC1StratumLow     = 0.5
	gUNC1StratumHigh    = 2.0
	gofBins             = 10
	gofMinExpectedCount = 5
)

// Gate check statuses.
const (
	CheckWithin              = "within"
	CheckOutside             = "outside"
	CheckInsufficientSamples = "insufficient_samples"
	CheckRequiresLabels      = "requires_labels"
	CheckSyntheticTest       = "synthetic_test"
)

// Coefficient provenance.
const (
	ProvenanceFitted       = "fitted"
	ProvenancePooledSource = "pooled_source"
	ProvenancePrior        = "prior"
)

// UncertaintyFitOptions are the declared choices the fit makes. They are
// recorded in the report so a calibration says how it was made.
type UncertaintyFitOptions struct {
	// BaseVarianceM2 is the prior coefficient for a stratum with too few
	// samples of its own or its source: the shipped MeasurementNoise.
	BaseVarianceM2 float64 `json:"base_variance_m2"`
	// MinStratumSamples is the fewest samples a stratum, a pooled source or a
	// consistency bin needs to be fitted or judged.
	MinStratumSamples int `json:"min_stratum_samples"`
	// CoefficientFloorM2 and CoefficientCeilingM2 bound a fitted coefficient.
	// The floor keeps R positive definite when the predicted covariance
	// already explains the innovation; the ceiling matches the cap the
	// recorded geometry covariance uses.
	CoefficientFloorM2   float64 `json:"coefficient_floor_m2"`
	CoefficientCeilingM2 float64 `json:"coefficient_ceiling_m2"`
	// BiasFlagRatio flags a stratum whose squared median innovation exceeds
	// this share of its median squared innovation.
	BiasFlagRatio float64 `json:"bias_flag_ratio"`
	// Estimator is the statistic a stratum's normalised innovations are
	// matched on: EstimatorMean or EstimatorMedian. See the package comment.
	Estimator string `json:"estimator"`
}

// Fit estimators.
const (
	// EstimatorMean matches the mean of the normalised squared innovation,
	// centred on its mean, to 1: the statistic G-UNC-1 judges, stratum by
	// stratum. Every eligible observation counts, the tail included.
	EstimatorMean = "mean"
	// EstimatorMedian matches the median, centred on the median, to the
	// chi-squared(1) median: robust to the tail, and so blind to it.
	EstimatorMedian = "median"
)

// DefaultUncertaintyFitOptions returns the declared defaults for a base
// variance, normally the tuning file's measurement_noise.
func DefaultUncertaintyFitOptions(baseVarianceM2 float64) UncertaintyFitOptions {
	return UncertaintyFitOptions{
		BaseVarianceM2:       baseVarianceM2,
		MinStratumSamples:    30,
		CoefficientFloorM2:   0.0025,
		CoefficientCeilingM2: 4,
		BiasFlagRatio:        0.25,
		Estimator:            EstimatorMean,
	}
}

func (o UncertaintyFitOptions) validate() error {
	switch {
	case !(o.BaseVarianceM2 > 0) || math.IsInf(o.BaseVarianceM2, 0):
		return fmt.Errorf("base variance %v must be finite and positive", o.BaseVarianceM2)
	case o.MinStratumSamples < 1:
		return fmt.Errorf("min stratum samples %d must be at least 1", o.MinStratumSamples)
	case !(o.CoefficientFloorM2 > 0) || !(o.CoefficientCeilingM2 >= o.CoefficientFloorM2) || math.IsInf(o.CoefficientCeilingM2, 0):
		return fmt.Errorf("coefficient bounds [%v, %v] must be finite, positive and ordered", o.CoefficientFloorM2, o.CoefficientCeilingM2)
	case !(o.BiasFlagRatio > 0) || o.BiasFlagRatio > 1:
		return fmt.Errorf("bias flag ratio %v must lie in (0, 1]", o.BiasFlagRatio)
	case o.Estimator != EstimatorMean && o.Estimator != EstimatorMedian:
		return fmt.Errorf("estimator %q must be %q or %q", o.Estimator, EstimatorMean, EstimatorMedian)
	}
	return nil
}

// UncertaintyInput is what a replay hands the report builder.
type UncertaintyInput struct {
	Samples        []l5tracks.UncertaintySample
	SamplesDropped int
	// NoiseModel names the R the samples were recorded under, for example
	// "isotropic", "adaptive_prior" or "adaptive_calibrated:<id>".
	NoiseModel string
	Options    UncertaintyFitOptions
}

// UncertaintyReport is the calibration and consistency report for one replay
// window, or for several pooled.
type UncertaintyReport struct {
	SchemaVersion int    `json:"schema_version"`
	Population    string `json:"population"`
	Eligibility   string `json:"eligibility"`
	NoiseModel    string `json:"noise_model"`
	Samples       int    `json:"samples"`
	// SamplesDropped were past the tracker's cap; SamplesInvalid had a rank
	// other than 1 or 2 or a non-finite field and were excluded.
	SamplesDropped int `json:"samples_dropped"`
	SamplesInvalid int `json:"samples_invalid"`
	// GateVerdict is never a pass: G-UNC-1 is assessed on the declared
	// decision partition with the labelled manoeuvre set, by a person.
	GateVerdict string                 `json:"gate_verdict"`
	GateChecks  []UncertaintyGateCheck `json:"gate_checks"`
	// PreGate is consistency over every eligible sample; AcceptedOnly over
	// those the assignment accepted, which the gate censors. The gap between
	// them is the gate's selection effect.
	PreGate        []NISConsistency `json:"pre_gate_consistency"`
	AcceptedOnly   []NISConsistency `json:"accepted_only_consistency"`
	RangeDeciles   []NISStratum     `json:"range_deciles"`
	SupportDeciles []NISStratum     `json:"support_deciles"`
	AspectOctants  []NISStratum     `json:"aspect_octants"`
	// CoastSplit compares samples whose track was observed on the previous
	// frame with those whose track was coasting: a test of the coast
	// covariance inflation rather than of R. Supplementary, not a G-UNC-1 row.
	CoastSplit  []NISStratum              `json:"coast_split"`
	ModelStrata []ModelStratumConsistency `json:"model_strata"`
	Fit         UncertaintyFit            `json:"fit"`

	// Filled by the replay, not by the builder.
	PreGateBands []l5tracks.PreGateBandSummary `json:"pre_gate_bands,omitempty"`
	Window       *UncertaintyWindowCounts      `json:"window,omitempty"`
}

// UncertaintyWindowCounts are the replay window's track births and
// confirmations, so fragmentation reads beside the calibration.
type UncertaintyWindowCounts struct {
	TracksCreated      int     `json:"tracks_created"`
	TracksConfirmed    int     `json:"tracks_confirmed"`
	FragmentationRatio float64 `json:"fragmentation_ratio"`
}

// UncertaintyGateCheck is one G-UNC-1 row as far as a label-free replay can
// take it.
type UncertaintyGateCheck struct {
	ID        string `json:"id"`
	Criterion string `json:"criterion"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
}

// NISConsistency is one dimension's NIS against its chi-squared law.
type NISConsistency struct {
	// Dimension is "joint" (the measurement's full NIS, DOF its rank), or
	// "radial"/"tangential" (one-dimensional marginals along and across the
	// line of sight).
	Dimension string  `json:"dimension"`
	DOF       int     `json:"dof"`
	Count     int     `json:"count"`
	MeanNIS   float64 `json:"mean_nis"`
	// MeanNISOverDOF is 1 for a consistent model; G-UNC-1 asks for [0.7, 1.4].
	MeanNISOverDOF float64 `json:"mean_nis_over_dof"`
	// Coverage95 is the share inside the chi-squared 95% bound: 0.95 when
	// consistent.
	Coverage95 float64    `json:"coverage_95"`
	GOF        *GOFResult `json:"gof,omitempty"`
}

// GOFResult is a Pearson chi-squared goodness-of-fit test over equiprobable
// bins of the reference chi-squared law.
type GOFResult struct {
	Bins             int     `json:"bins"`
	Statistic        float64 `json:"statistic"`
	DegreesOfFreedom int     `json:"degrees_of_freedom"`
	PValue           float64 `json:"p_value"`
}

// NISStratum is one bin of a stratified check.
type NISStratum struct {
	Label string  `json:"label"`
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
	Count int     `json:"count"`
	// MeanNormalisedNIS is the mean of NIS/m over the bin's samples.
	MeanNormalisedNIS float64 `json:"mean_normalised_nis"`
	RatioToOverall    float64 `json:"ratio_to_overall"`
	Coverage95        float64 `json:"coverage_95"`
	// Judged is false when the bin has fewer than MinStratumSamples.
	Judged bool `json:"judged"`
	Within bool `json:"within"`
}

// ModelStratumConsistency is NIS consistency in one cell of the noise model's
// own stratum table.
type ModelStratumConsistency struct {
	Source            string  `json:"source"`
	RangeBin          int     `json:"range_bin"`
	SupportBin        int     `json:"support_bin"`
	AspectBin         int     `json:"aspect_bin"`
	Count             int     `json:"count"`
	JointMeanOverDOF  float64 `json:"joint_mean_over_dof"`
	RadialMeanNIS     float64 `json:"radial_mean_nis"`
	TangentialMeanNIS float64 `json:"tangential_mean_nis"`
	Coverage95        float64 `json:"coverage_95"`
}

// UncertaintyFit is the per-stratum coefficient fit and the table it yields.
type UncertaintyFit struct {
	Method  string                `json:"method"`
	Options UncertaintyFitOptions `json:"options"`
	// Strata lists every stratum and axis with samples; Pooled one entry per
	// source and axis. Strata without samples take the pooled or prior
	// coefficient and appear only in the table.
	Strata []FitStratum `json:"strata"`
	Pooled []FitStratum `json:"pooled"`
	// CoastingExcluded counts samples left out of the fit because their track
	// was coasting; see the package comment.
	CoastingExcluded int `json:"coasting_excluded"`
	// Converged holds when every fitted coefficient lies within
	// ConvergenceTolerance, relative, of the coefficient the replay used.
	// Until it does, replay with this table and refit.
	Converged            bool             `json:"converged"`
	ConvergenceTolerance float64          `json:"convergence_tolerance"`
	UnconvergedStrata    int              `json:"unconverged_strata"`
	Calibration          CalibrationTable `json:"calibration"`
}

// fitConvergenceTolerance is the relative agreement between a fitted and a
// used coefficient that counts as converged.
const fitConvergenceTolerance = 0.1

// FitStratum is one stratum and axis of the fit. RangeBin, SupportBin and
// AspectBin are -1 for a pooled entry.
type FitStratum struct {
	Source     string `json:"source"`
	RangeBin   int    `json:"range_bin"`
	SupportBin int    `json:"support_bin"`
	AspectBin  int    `json:"aspect_bin"`
	Axis       string `json:"axis"`
	Count      int    `json:"count"`
	Gated      int    `json:"gated"`
	// MedianInnovation is the location the spread is measured about and
	// MeanInnovation the signed mean: the bias the fit refuses to fold in.
	MedianInnovation   float64 `json:"median_innovation"`
	MeanInnovation     float64 `json:"mean_innovation"`
	InnovationVariance float64 `json:"innovation_variance"`
	MeanPredicted      float64 `json:"mean_predicted"`
	MeanPhysics        float64 `json:"mean_physics"`
	// MeanModelNoise is the R the replay's filter used along this axis, and
	// MeanNormalisedNIS its one-dimensional NIS: 1 when consistent.
	MeanModelNoise    float64 `json:"mean_model_noise"`
	MeanNormalisedNIS float64 `json:"mean_normalised_nis"`
	// CoefficientUsed is the coefficient the replay ran with, R used less the
	// physics terms, averaged. Convergence compares it with Coefficient.
	CoefficientUsed float64 `json:"coefficient_used"`
	// MomentCoefficient is Var(y) - mean(P) - mean(phi), reported and not
	// used; see the package comment.
	MomentCoefficient float64 `json:"moment_coefficient"`
	Coefficient       float64 `json:"coefficient"`
	Provenance        string  `json:"provenance"`
	BiasDominated     bool    `json:"bias_dominated"`
	// Clamped is "floor" when the predicted covariance alone already explains
	// the spread (no positive R restores consistency: look at the process
	// noise), "ceiling" when even the largest allowed R does not.
	Clamped string `json:"clamped,omitempty"`
}

// CalibrationTable is the coefficient table in portable form: what a replay
// loads with LoadNoiseCalibration. Every cell of the compiled stratum table
// appears exactly once, and the edges it was fitted with are recorded so a
// build with different bins refuses it.
type CalibrationTable struct {
	ID               string            `json:"id"`
	RangeEdgesMetres []float32         `json:"range_edges_metres"`
	SupportEdges     []int             `json:"support_edges"`
	AspectBins       int               `json:"aspect_bins"`
	Sources          []string          `json:"sources"`
	Cells            []CalibrationCell `json:"cells"`
}

// CalibrationCell is one stratum's pair of coefficients, square metres.
type CalibrationCell struct {
	Source               string  `json:"source"`
	RangeBin             int     `json:"range_bin"`
	SupportBin           int     `json:"support_bin"`
	AspectBin            int     `json:"aspect_bin"`
	RadialM2             float32 `json:"radial_m2"`
	TangentialM2         float32 `json:"tangential_m2"`
	RadialProvenance     string  `json:"radial_provenance"`
	TangentialProvenance string  `json:"tangential_provenance"`
}

// BuildUncertaintyReport fits the coefficients and evaluates consistency.
func BuildUncertaintyReport(in UncertaintyInput) (UncertaintyReport, error) {
	if err := in.Options.validate(); err != nil {
		return UncertaintyReport{}, fmt.Errorf("uncertainty fit options: %w", err)
	}
	samples, invalid := canonicalSamples(in.Samples)
	report := UncertaintyReport{
		SchemaVersion: UncertaintyReportSchemaVersion,
		Population:    "scoring_window_eligible_pre_gate_pairings",
		Eligibility: "confirmed track; the cluster is the only one physically plausible for the track " +
			"(max_position_jump_metres, max_reasonable_speed_mps, fragment guard) and the track the only " +
			"active one for which the cluster is plausible; a line of sight exists. Independent of S.",
		NoiseModel:     in.NoiseModel,
		Samples:        len(samples),
		SamplesDropped: in.SamplesDropped,
		SamplesInvalid: invalid,
		GateVerdict: "not_assessed: G-UNC-1 is decided on the declared decision-gate partition " +
			"with the labelled manoeuvre set; this report is evidence toward it, not a verdict",
	}
	report.PreGate = nisConsistency(samples, false)
	report.AcceptedOnly = nisConsistency(samples, true)
	minN := in.Options.MinStratumSamples
	overall := meanNormalisedNIS(samples)
	report.RangeDeciles = decileStrata("range_m", samples, overall, minN, func(s l5tracks.UncertaintySample) float64 {
		return float64(s.RangeMetres)
	})
	report.SupportDeciles = decileStrata("support", samples, overall, minN, func(s l5tracks.UncertaintySample) float64 {
		return float64(s.Support)
	})
	report.AspectOctants = aspectOctants(samples, overall, minN)
	report.CoastSplit = coastSplit(samples, overall, minN)
	report.ModelStrata = modelStrata(samples)
	report.Fit = FitNoiseCoefficients(samples, in.Options)
	report.GateChecks = gateChecks(report, minN)
	return report, nil
}

// ---- sample hygiene ------------------------------------------------------

func finite32(vs ...float32) bool {
	for _, v := range vs {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return true
}

// canonicalSamples drops invalid samples and sorts the rest into a total
// order, so every sum below is independent of the order they arrived in.
func canonicalSamples(in []l5tracks.UncertaintySample) ([]l5tracks.UncertaintySample, int) {
	out := make([]l5tracks.UncertaintySample, 0, len(in))
	invalid := 0
	for _, s := range in {
		if (s.Rank != 1 && s.Rank != 2) || !finite32(s.X, s.Y, s.RangeMetres, s.AspectRad, s.SpeedMps, s.InnovRadial,
			s.InnovTangential, s.PredRadial, s.PredTangential, s.NoiseRadial, s.NoiseTangential, s.PhysRadial,
			s.PhysTangential, s.NIS) || s.NIS < 0 || !(s.PredRadial+s.NoiseRadial > 0) || !(s.PredTangential+s.NoiseTangential > 0) {
			invalid++
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return sampleLess(out[i], out[j]) })
	return out, invalid
}

func sampleLess(a, b l5tracks.UncertaintySample) bool {
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if a.Rank != b.Rank {
		return a.Rank < b.Rank
	}
	if a.Support != b.Support {
		return a.Support < b.Support
	}
	if a.Misses != b.Misses {
		return a.Misses < b.Misses
	}
	if a.FrameUnixNanos != b.FrameUnixNanos {
		return a.FrameUnixNanos < b.FrameUnixNanos
	}
	fa := [...]float32{a.X, a.Y, a.RangeMetres, a.AspectRad, a.SpeedMps, a.InnovRadial, a.InnovTangential, a.PredRadial,
		a.PredTangential, a.NoiseRadial, a.NoiseTangential, a.PhysRadial, a.PhysTangential, a.NIS}
	fb := [...]float32{b.X, b.Y, b.RangeMetres, b.AspectRad, b.SpeedMps, b.InnovRadial, b.InnovTangential, b.PredRadial,
		b.PredTangential, b.NoiseRadial, b.NoiseTangential, b.PhysRadial, b.PhysTangential, b.NIS}
	for i := range fa {
		if fa[i] != fb[i] {
			return fa[i] < fb[i]
		}
	}
	if a.Gated != b.Gated {
		return !a.Gated
	}
	return !a.Assigned && b.Assigned
}

// axisValues returns an axis's innovation, predicted variance, model noise and
// physics term.
func axisValues(s l5tracks.UncertaintySample, axis int) (y, pred, noise, phys float64) {
	if axis == l5tracks.NoiseAxisRadial {
		return float64(s.InnovRadial), float64(s.PredRadial), float64(s.NoiseRadial), float64(s.PhysRadial)
	}
	return float64(s.InnovTangential), float64(s.PredTangential), float64(s.NoiseTangential), float64(s.PhysTangential)
}

// axisNIS is the one-dimensional normalised innovation squared along an axis.
func axisNIS(s l5tracks.UncertaintySample, axis int) float64 {
	y, pred, noise, _ := axisValues(s, axis)
	if v := pred + noise; v > 0 {
		return y * y / v
	}
	return math.Inf(1)
}

// ---- consistency ---------------------------------------------------------

func summariseNIS(dimension string, dof int, values []float64) NISConsistency {
	c := NISConsistency{Dimension: dimension, DOF: dof, Count: len(values)}
	if len(values) == 0 {
		return c
	}
	bound := chiSquaredQuantile(dof, 0.95)
	var sum float64
	inside := 0
	for _, v := range values {
		sum += v
		if v <= bound {
			inside++
		}
	}
	n := float64(len(values))
	mean := sum / n
	c.MeanNIS = r6(mean)
	c.MeanNISOverDOF = r6(mean / float64(dof))
	c.Coverage95 = r6(float64(inside) / n)
	if len(values) >= gofBins*gofMinExpectedCount {
		c.GOF = chiSquaredGOF(values, dof)
	}
	return c
}

// nisConsistency reports joint NIS by measurement rank, then the two
// one-dimensional marginals of the rank-two samples.
func nisConsistency(samples []l5tracks.UncertaintySample, acceptedOnly bool) []NISConsistency {
	joint := map[int][]float64{}
	var radial, tangential []float64
	for _, s := range samples {
		if acceptedOnly && !s.Assigned {
			continue
		}
		joint[s.Rank] = append(joint[s.Rank], float64(s.NIS))
		if s.Rank == 2 {
			radial = append(radial, axisNIS(s, l5tracks.NoiseAxisRadial))
			tangential = append(tangential, axisNIS(s, l5tracks.NoiseAxisTangential))
		}
	}
	out := []NISConsistency{}
	for _, rank := range []int{1, 2} {
		if len(joint[rank]) > 0 {
			out = append(out, summariseNIS("joint", rank, joint[rank]))
		}
	}
	if len(radial) > 0 {
		out = append(out, summariseNIS("radial", 1, radial), summariseNIS("tangential", 1, tangential))
	}
	return out
}

func normalisedNIS(s l5tracks.UncertaintySample) float64 { return float64(s.NIS) / float64(s.Rank) }

func meanNormalisedNIS(samples []l5tracks.UncertaintySample) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		sum += normalisedNIS(s)
	}
	return sum / float64(len(samples))
}

type stratumAccumulator struct {
	count  int
	sum    float64
	inside int
}

func (a *stratumAccumulator) add(s l5tracks.UncertaintySample) {
	a.count++
	a.sum += normalisedNIS(s)
	if float64(s.NIS) <= chiSquaredQuantile(s.Rank, 0.95) {
		a.inside++
	}
}

func (a stratumAccumulator) stratum(label string, lower, upper, overall float64, minN int) NISStratum {
	st := NISStratum{Label: label, Lower: lower, Upper: upper, Count: a.count}
	if a.count == 0 {
		return st
	}
	mean := a.sum / float64(a.count)
	ratio := 0.0
	if overall > 0 {
		ratio = mean / overall
	}
	st.Judged = a.count >= minN && overall > 0
	st.Within = st.Judged && ratio >= gUNC1StratumLow && ratio <= gUNC1StratumHigh
	st.MeanNormalisedNIS = r6(mean)
	st.RatioToOverall = r6(ratio)
	st.Coverage95 = r6(float64(a.inside) / float64(a.count))
	st.Lower, st.Upper = r6(lower), r6(upper)
	return st
}

// decileStrata bins samples by a value's deciles. Tied values never straddle
// a bin edge, so a heavily tied value (point count) can give fewer than ten
// bins; each bin records its own count.
func decileStrata(name string, samples []l5tracks.UncertaintySample, overall float64, minN int,
	value func(l5tracks.UncertaintySample) float64) []NISStratum {
	if len(samples) == 0 {
		return []NISStratum{}
	}
	values := make([]float64, len(samples))
	for i, s := range samples {
		values[i] = value(s)
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	edges := []float64{}
	for i := 1; i < 10; i++ {
		e := sorted[i*len(sorted)/10]
		if (len(edges) == 0 || e > edges[len(edges)-1]) && e > sorted[0] {
			edges = append(edges, e)
		}
	}
	accs := make([]stratumAccumulator, len(edges)+1)
	for i, s := range samples {
		accs[sort.Search(len(edges), func(k int) bool { return edges[k] > values[i] })].add(s)
	}
	out := make([]NISStratum, 0, len(accs))
	for b, acc := range accs {
		lower, upper := sorted[0], sorted[len(sorted)-1]
		if b > 0 {
			lower = edges[b-1]
		}
		if b < len(edges) {
			upper = edges[b]
		}
		out = append(out, acc.stratum(fmt.Sprintf("%s_bin%d", name, b), lower, upper, overall, minN))
	}
	return out
}

// aspectOctants bins by the sensor's 45-degree octant in the body frame.
func aspectOctants(samples []l5tracks.UncertaintySample, overall float64, minN int) []NISStratum {
	var accs [8]stratumAccumulator
	for _, s := range samples {
		accs[l5tracks.MeasurementGeometry{AspectRad: s.AspectRad}.AspectOctant()].add(s)
	}
	out := make([]NISStratum, 0, 8)
	for o, acc := range accs {
		out = append(out, acc.stratum(fmt.Sprintf("octant%d", o), float64(o)*45, float64(o+1)*45, overall, minN))
	}
	return out
}

// coastSplit bins by whether the track was coasting when the sample was
// taken. Lower and Upper are the miss counts each bin covers; the coasting
// bin's upper bound is the largest seen.
func coastSplit(samples []l5tracks.UncertaintySample, overall float64, minN int) []NISStratum {
	var observed, coasting stratumAccumulator
	maxMisses := 1
	for _, s := range samples {
		if s.Misses > 0 {
			coasting.add(s)
			maxMisses = max(maxMisses, s.Misses)
		} else {
			observed.add(s)
		}
	}
	return []NISStratum{
		observed.stratum("observed_last_frame", 0, 0, overall, minN),
		coasting.stratum("coasting", 1, float64(maxMisses), overall, minN),
	}
}

func sampleStratum(s l5tracks.UncertaintySample) (l5tracks.NoiseStratum, bool) {
	return l5tracks.NoiseStratumFor(s.Source, l5tracks.MeasurementGeometry{
		Valid: true, RangeMetres: s.RangeMetres, Support: s.Support, AspectRad: s.AspectRad,
	})
}

func modelStrata(samples []l5tracks.UncertaintySample) []ModelStratumConsistency {
	type acc struct {
		n, inside       int
		joint, rad, tan float64
	}
	cells := map[l5tracks.NoiseStratum]*acc{}
	for _, s := range samples {
		st, ok := sampleStratum(s)
		if !ok || s.Rank != 2 {
			continue
		}
		a := cells[st]
		if a == nil {
			a = &acc{}
			cells[st] = a
		}
		a.n++
		a.joint += normalisedNIS(s)
		a.rad += axisNIS(s, l5tracks.NoiseAxisRadial)
		a.tan += axisNIS(s, l5tracks.NoiseAxisTangential)
		if float64(s.NIS) <= chiSquaredQuantile(2, 0.95) {
			a.inside++
		}
	}
	keys := make([]l5tracks.NoiseStratum, 0, len(cells))
	for k := range cells {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return stratumLess(keys[i], keys[j]) })
	out := make([]ModelStratumConsistency, 0, len(keys))
	for _, k := range keys {
		a := cells[k]
		n := float64(a.n)
		out = append(out, ModelStratumConsistency{
			Source: string(l5tracks.NoiseSources[k.Source]), RangeBin: k.Range, SupportBin: k.Support, AspectBin: k.Aspect,
			Count: a.n, JointMeanOverDOF: r6(a.joint / n), RadialMeanNIS: r6(a.rad / n),
			TangentialMeanNIS: r6(a.tan / n), Coverage95: r6(float64(a.inside) / n),
		})
	}
	return out
}

func stratumLess(a, b l5tracks.NoiseStratum) bool {
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if a.Range != b.Range {
		return a.Range < b.Range
	}
	if a.Support != b.Support {
		return a.Support < b.Support
	}
	return a.Aspect < b.Aspect
}

// ---- coefficient fit -----------------------------------------------------

type axisAccumulator struct {
	n, gated                               int
	ys, bases                              []float64
	sumY, sumY2, sumP, sumPhys, sumR, sumN float64
}

func (a *axisAccumulator) add(s l5tracks.UncertaintySample, axis int) {
	y, pred, noise, phys := axisValues(s, axis)
	a.n++
	if s.Gated {
		a.gated++
	}
	a.ys = append(a.ys, y)
	a.bases = append(a.bases, pred+phys)
	a.sumY += y
	a.sumY2 += y * y
	a.sumP += pred
	a.sumPhys += phys
	a.sumR += noise
	a.sumN += axisNIS(s, axis)
}

func median(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// fit reduces an accumulator to a stratum entry; the provenance and final
// coefficient are decided by the caller.
func (a axisAccumulator) fit(opts UncertaintyFitOptions) FitStratum {
	n := float64(a.n)
	meanY := a.sumY / n
	meanY2 := a.sumY2 / n
	location := median(a.ys)
	st := FitStratum{
		Count: a.n, Gated: a.gated,
		MedianInnovation: location, MeanInnovation: meanY,
		InnovationVariance: math.Max(0, meanY2-meanY*meanY),
		MeanPredicted:      a.sumP / n, MeanPhysics: a.sumPhys / n, MeanModelNoise: a.sumR / n,
		MeanNormalisedNIS: a.sumN / n,
	}
	st.CoefficientUsed = st.MeanModelNoise - st.MeanPhysics
	st.MomentCoefficient = st.InnovationVariance - st.MeanPredicted - st.MeanPhysics
	st.Coefficient, st.Clamped = matchedCoefficient(a.ys, a.bases, opts)
	squares := make([]float64, len(a.ys))
	for i, y := range a.ys {
		squares[i] = y * y
	}
	st.BiasDominated = location*location > opts.BiasFlagRatio*median(squares)
	return st
}

// matchedCoefficient finds c in [floor, ceiling] at which the chosen
// statistic of (y - centre)² / (base + c) meets its target under a
// consistent model: the mean against 1, centred on the mean, or the median
// against the chi-squared(1) median, centred on the median. Either statistic
// is non-increasing in c, so bisection finds it; outside the bracket the bound
// is returned and named.
func matchedCoefficient(ys, bases []float64, opts UncertaintyFitOptions) (float64, string) {
	centre, target, statistic := median(ys), chiSquaredQuantile(1, 0.5), median
	if opts.Estimator == EstimatorMean {
		var sum float64
		for _, y := range ys {
			sum += y
		}
		centre, target, statistic = sum/float64(len(ys)), 1, mean
	}
	normalised := make([]float64, len(ys))
	g := func(c float64) float64 {
		for i, y := range ys {
			d := y - centre
			normalised[i] = d * d / (bases[i] + c)
		}
		return statistic(normalised) - target
	}
	lo, hi := opts.CoefficientFloorM2, opts.CoefficientCeilingM2
	if g(lo) <= 0 {
		return lo, "floor"
	}
	if g(hi) > 0 {
		return hi, "ceiling"
	}
	for i := 0; i < 100 && hi-lo > 1e-9; i++ {
		mid := (lo + hi) / 2
		if g(mid) > 0 {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2, ""
}

// FitNoiseCoefficients fits each stratum's coefficient per axis. A stratum
// with MinStratumSamples of its own is fitted; otherwise it takes its source's
// pooled fit if that has enough; otherwise the prior, BaseVarianceM2. Only
// rank-two samples of tracks observed on the previous frame enter: a rank-one
// measurement constrains one direction, which need not be either axis, and a
// coasting track's covariance carries an inflation R must not absorb. The
// samples need not be sorted.
func FitNoiseCoefficients(samples []l5tracks.UncertaintySample, opts UncertaintyFitOptions) UncertaintyFit {
	samples, _ = canonicalSamples(samples)
	fit := UncertaintyFit{
		Method:               opts.Estimator + "_normalised_squared_innovation_matched",
		Options:              opts,
		Strata:               []FitStratum{},
		Pooled:               []FitStratum{},
		ConvergenceTolerance: fitConvergenceTolerance,
	}
	var cells [l5tracks.NoiseSourceCount][l5tracks.NoiseRangeBins][l5tracks.NoiseSupportBins][l5tracks.NoiseAspectBins][l5tracks.NoiseAxisCount]axisAccumulator
	var pooled [l5tracks.NoiseSourceCount][l5tracks.NoiseAxisCount]axisAccumulator
	for _, s := range samples {
		st, ok := sampleStratum(s)
		if !ok || s.Rank != 2 {
			continue
		}
		if s.Misses > 0 {
			fit.CoastingExcluded++
			continue
		}
		for axis := 0; axis < l5tracks.NoiseAxisCount; axis++ {
			cells[st.Source][st.Range][st.Support][st.Aspect][axis].add(s, axis)
			pooled[st.Source][axis].add(s, axis)
		}
	}

	var pooledFit [l5tracks.NoiseSourceCount][l5tracks.NoiseAxisCount]FitStratum
	for si := range pooled {
		for axis := range pooled[si] {
			acc := pooled[si][axis]
			if acc.n == 0 {
				continue
			}
			st := acc.fit(opts)
			st.Source, st.Axis = string(l5tracks.NoiseSources[si]), l5tracks.NoiseAxisName(axis)
			st.RangeBin, st.SupportBin, st.AspectBin = -1, -1, -1
			st.Provenance = ProvenanceFitted
			if acc.n < opts.MinStratumSamples {
				st.Provenance = ProvenancePrior
				st.Coefficient, st.Clamped = opts.BaseVarianceM2, ""
			}
			st = roundFitStratum(st)
			fit.Pooled = append(fit.Pooled, st)
			pooledFit[si][axis] = st
			fit.noteConvergence(st)
		}
	}

	table := CalibrationTable{
		RangeEdgesMetres: append([]float32(nil), l5tracks.NoiseRangeEdgesMetres[:]...),
		SupportEdges:     append([]int(nil), l5tracks.NoiseSupportEdges[:]...),
		AspectBins:       l5tracks.NoiseAspectBins,
		Cells:            []CalibrationCell{},
	}
	for _, src := range l5tracks.NoiseSources {
		table.Sources = append(table.Sources, string(src))
	}
	for si := range cells {
		for ri := range cells[si] {
			for ni := range cells[si][ri] {
				for ai := range cells[si][ri][ni] {
					cell := CalibrationCell{Source: string(l5tracks.NoiseSources[si]), RangeBin: ri, SupportBin: ni, AspectBin: ai}
					for axis := 0; axis < l5tracks.NoiseAxisCount; axis++ {
						acc := cells[si][ri][ni][ai][axis]
						coefficient, provenance := opts.BaseVarianceM2, ProvenancePrior
						if p := pooledFit[si][axis]; p.Provenance == ProvenanceFitted {
							coefficient, provenance = p.Coefficient, ProvenancePooledSource
						}
						if acc.n > 0 {
							st := acc.fit(opts)
							st.Source, st.Axis = cell.Source, l5tracks.NoiseAxisName(axis)
							st.RangeBin, st.SupportBin, st.AspectBin = ri, ni, ai
							if acc.n >= opts.MinStratumSamples {
								coefficient, provenance = st.Coefficient, ProvenanceFitted
							} else {
								st.Coefficient, st.Clamped = coefficient, ""
							}
							st.Provenance = provenance
							st = roundFitStratum(st)
							fit.Strata = append(fit.Strata, st)
							fit.noteConvergence(st)
						}
						c := float32(r6(coefficient))
						if axis == l5tracks.NoiseAxisRadial {
							cell.RadialM2, cell.RadialProvenance = c, provenance
						} else {
							cell.TangentialM2, cell.TangentialProvenance = c, provenance
						}
					}
					table.Cells = append(table.Cells, cell)
				}
			}
		}
	}
	table.ID = table.digest()
	fit.Calibration = table
	fit.Converged = fit.UnconvergedStrata == 0
	return fit
}

// noteConvergence counts a fitted entry whose coefficient disagrees with the
// one its replay used by more than the tolerance.
func (f *UncertaintyFit) noteConvergence(st FitStratum) {
	if st.Provenance != ProvenanceFitted {
		return
	}
	scale := math.Max(st.CoefficientUsed, f.Options.CoefficientFloorM2)
	if math.Abs(st.Coefficient-st.CoefficientUsed) > f.ConvergenceTolerance*scale {
		f.UnconvergedStrata++
	}
}

func roundFitStratum(s FitStratum) FitStratum {
	s.MeanInnovation = r6(s.MeanInnovation)
	s.InnovationVariance = r6(s.InnovationVariance)
	s.MeanPredicted = r6(s.MeanPredicted)
	s.MeanPhysics = r6(s.MeanPhysics)
	s.MeanModelNoise = r6(s.MeanModelNoise)
	s.MeanNormalisedNIS = r6(s.MeanNormalisedNIS)
	s.MedianInnovation = r6(s.MedianInnovation)
	s.CoefficientUsed = r6(s.CoefficientUsed)
	s.MomentCoefficient = r6(s.MomentCoefficient)
	s.Coefficient = r6(s.Coefficient)
	return s
}

// digest is the table's content address: the SHA-256 of its JSON with the ID
// left empty.
func (t CalibrationTable) digest() string {
	t.ID = ""
	b, err := json.Marshal(t)
	if err != nil {
		// A table of numbers and strings always marshals.
		panic(fmt.Sprintf("marshal calibration table: %v", err))
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// NoiseCalibration converts the table to the tracker's form. It refuses a
// table whose bins differ from this build's, whose cells are missing,
// duplicated or unknown, whose values the filter cannot use, or whose ID does
// not match its content.
func (t CalibrationTable) NoiseCalibration() (*l5tracks.NoiseCalibration, error) {
	if t.ID == "" || t.ID != t.digest() {
		return nil, fmt.Errorf("calibration id %q does not match its content", t.ID)
	}
	if len(t.RangeEdgesMetres) != l5tracks.NoiseRangeBins || len(t.SupportEdges) != l5tracks.NoiseSupportBins ||
		t.AspectBins != l5tracks.NoiseAspectBins || len(t.Sources) != l5tracks.NoiseSourceCount {
		return nil, errors.New("calibration table bins do not match this build's noise model")
	}
	for i, e := range t.RangeEdgesMetres {
		if e != l5tracks.NoiseRangeEdgesMetres[i] {
			return nil, fmt.Errorf("calibration range edge %d is %v, this build uses %v", i, e, l5tracks.NoiseRangeEdgesMetres[i])
		}
	}
	for i, e := range t.SupportEdges {
		if e != l5tracks.NoiseSupportEdges[i] {
			return nil, fmt.Errorf("calibration support edge %d is %v, this build uses %v", i, e, l5tracks.NoiseSupportEdges[i])
		}
	}
	for i, s := range t.Sources {
		if s != string(l5tracks.NoiseSources[i]) {
			return nil, fmt.Errorf("calibration source %d is %q, this build uses %q", i, s, l5tracks.NoiseSources[i])
		}
	}
	cal := &l5tracks.NoiseCalibration{ID: t.ID}
	var seen [l5tracks.NoiseSourceCount][l5tracks.NoiseRangeBins][l5tracks.NoiseSupportBins][l5tracks.NoiseAspectBins]bool
	for _, c := range t.Cells {
		si, ok := l5tracks.NoiseSourceIndex(l5tracks.MeasurementSource(c.Source))
		if !ok || c.RangeBin < 0 || c.RangeBin >= l5tracks.NoiseRangeBins || c.SupportBin < 0 ||
			c.SupportBin >= l5tracks.NoiseSupportBins || c.AspectBin < 0 || c.AspectBin >= l5tracks.NoiseAspectBins {
			return nil, fmt.Errorf("calibration cell %+v is outside the table", c)
		}
		if seen[si][c.RangeBin][c.SupportBin][c.AspectBin] {
			return nil, fmt.Errorf("calibration cell %+v appears twice", c)
		}
		seen[si][c.RangeBin][c.SupportBin][c.AspectBin] = true
		cal.Coefficients[si][c.RangeBin][c.SupportBin][c.AspectBin] = [2]float32{c.RadialM2, c.TangentialM2}
	}
	want := l5tracks.NoiseSourceCount * l5tracks.NoiseRangeBins * l5tracks.NoiseSupportBins * l5tracks.NoiseAspectBins
	if len(t.Cells) != want {
		return nil, fmt.Errorf("calibration table has %d cells, want %d", len(t.Cells), want)
	}
	if err := cal.Validate(); err != nil {
		return nil, err
	}
	return cal, nil
}

// ParseNoiseCalibration reads the calibration from an UncertaintyReport's
// JSON, the file a replay writes, and converts it for the tracker.
func ParseNoiseCalibration(b []byte) (*l5tracks.NoiseCalibration, error) {
	var report struct {
		SchemaVersion int `json:"schema_version"`
		Fit           struct {
			Calibration *CalibrationTable `json:"calibration"`
		} `json:"fit"`
	}
	if err := json.Unmarshal(b, &report); err != nil {
		return nil, fmt.Errorf("decode uncertainty report: %w", err)
	}
	if report.SchemaVersion != UncertaintyReportSchemaVersion {
		return nil, fmt.Errorf("uncertainty report schema %d, want %d", report.SchemaVersion, UncertaintyReportSchemaVersion)
	}
	if report.Fit.Calibration == nil {
		return nil, errors.New("uncertainty report has no fit.calibration")
	}
	return report.Fit.Calibration.NoiseCalibration()
}

// ---- G-UNC-1 checks ------------------------------------------------------

func gateChecks(r UncertaintyReport, minN int) []UncertaintyGateCheck {
	checks := []UncertaintyGateCheck{}
	for _, c := range r.PreGate {
		id := fmt.Sprintf("nis_mean_%s_m%d", c.Dimension, c.DOF)
		criterion := "mean NIS/m over eligible pre-gate observations within [0.7, 1.4]"
		if c.Dimension != "joint" {
			criterion = "supplementary: " + criterion + " for the one-dimensional marginal"
		}
		check := UncertaintyGateCheck{ID: id, Criterion: criterion,
			Detail: fmt.Sprintf("n=%d mean_nis_over_dof=%.4f coverage_95=%.4f", c.Count, c.MeanNISOverDOF, c.Coverage95)}
		switch {
		case c.Count < minN:
			check.Status = CheckInsufficientSamples
		case c.MeanNISOverDOF >= gUNC1MeanLow && c.MeanNISOverDOF <= gUNC1MeanHigh:
			check.Status = CheckWithin
		default:
			check.Status = CheckOutside
		}
		checks = append(checks, check)

		gof := UncertaintyGateCheck{ID: fmt.Sprintf("gof_%s_m%d", c.Dimension, c.DOF),
			Criterion: "chi-squared goodness of fit not rejected at p = 0.01"}
		if c.Dimension != "joint" {
			gof.Criterion = "supplementary: " + gof.Criterion + " for the one-dimensional marginal"
		}
		switch {
		case c.GOF == nil:
			gof.Status = CheckInsufficientSamples
		case c.GOF.PValue >= gUNC1GOFAlpha:
			gof.Status = CheckWithin
		default:
			gof.Status = CheckOutside
		}
		if c.GOF != nil {
			gof.Detail = fmt.Sprintf("X2=%.3f dof=%d p=%.3g over %d equiprobable bins", c.GOF.Statistic, c.GOF.DegreesOfFreedom, c.GOF.PValue, c.GOF.Bins)
		}
		checks = append(checks, gof)
	}
	for _, s := range []struct {
		id     string
		strata []NISStratum
	}{
		{"range_deciles", r.RangeDeciles},
		{"support_deciles", r.SupportDeciles},
		{"aspect_octants", r.AspectOctants},
	} {
		checks = append(checks, stratifiedCheck(s.id, s.strata))
	}
	dropped := UncertaintyGateCheck{ID: "sample_not_truncated",
		Criterion: "every eligible observation in the window was kept", Status: CheckWithin}
	if r.SamplesDropped > 0 || r.SamplesInvalid > 0 {
		dropped.Status = CheckOutside
		dropped.Detail = fmt.Sprintf("dropped=%d invalid=%d", r.SamplesDropped, r.SamplesInvalid)
	}
	checks = append(checks, dropped,
		UncertaintyGateCheck{ID: "manoeuvres_falsely_gated", Criterion: "under 1% on the labelled manoeuvre set",
			Status: CheckRequiresLabels,
			Detail: "label-free gate-rejection outcomes are in pre_gate_bands; the rate needs manoeuvre labels"},
		UncertaintyGateCheck{ID: "occlusion_recovery_5_frames", Criterion: "under 3 frames after a synthetic 5-frame occlusion",
			Status: CheckSyntheticTest,
			Detail: "l5tracks TestRecoveryAfterFiveFrameOcclusionIsUnderThreeFrames"},
	)
	return checks
}

func stratifiedCheck(id string, strata []NISStratum) UncertaintyGateCheck {
	check := UncertaintyGateCheck{ID: id,
		Criterion: "every bin's mean NIS/m within [0.5, 2.0] times the overall normalised mean"}
	judged, outside := 0, 0
	for _, s := range strata {
		if !s.Judged {
			continue
		}
		judged++
		if !s.Within {
			outside++
		}
	}
	switch {
	case judged == 0:
		check.Status = CheckInsufficientSamples
	case outside > 0:
		check.Status = CheckOutside
	default:
		check.Status = CheckWithin
	}
	check.Detail = fmt.Sprintf("%d of %d bins judged, %d outside", judged, len(strata), outside)
	return check
}

// ---- chi-squared helpers -------------------------------------------------

// chiSquaredQuantile is the p-quantile of chi-squared with one or two degrees
// of freedom, in closed form: 2·erfinv(p)² and -2·ln(1-p).
func chiSquaredQuantile(dof int, p float64) float64 {
	if dof == 1 {
		e := math.Erfinv(p)
		return 2 * e * e
	}
	return -2 * math.Log(1-p)
}

// chiSquaredGOF is Pearson's test of NIS values against chi-squared with dof
// degrees of freedom over ten equiprobable bins.
func chiSquaredGOF(values []float64, dof int) *GOFResult {
	edges := make([]float64, gofBins-1)
	for i := range edges {
		edges[i] = chiSquaredQuantile(dof, float64(i+1)/gofBins)
	}
	counts := make([]int, gofBins)
	for _, v := range values {
		counts[sort.SearchFloat64s(edges, v)]++
	}
	expected := float64(len(values)) / gofBins
	var stat float64
	for _, c := range counts {
		d := float64(c) - expected
		stat += d * d / expected
	}
	df := gofBins - 1
	return &GOFResult{Bins: gofBins, Statistic: r6(stat), DegreesOfFreedom: df, PValue: r6(chiSquaredSurvival(stat, float64(df)))}
}

// chiSquaredSurvival is P(X > x) for chi-squared with k degrees of freedom:
// the regularised upper incomplete gamma Q(k/2, x/2).
func chiSquaredSurvival(x, k float64) float64 {
	if x <= 0 {
		return 1
	}
	return regularisedGammaQ(k/2, x/2)
}

// regularisedGammaQ is Q(a, x) = Γ(a, x)/Γ(a), by the series for x < a+1 and
// the Lentz continued fraction otherwise.
func regularisedGammaQ(a, x float64) float64 {
	if x < 0 || a <= 0 {
		return math.NaN()
	}
	if x == 0 {
		return 1
	}
	lg, _ := math.Lgamma(a)
	if x < a+1 {
		sum, term := 1/a, 1/a
		for n := 1; n < 500; n++ {
			term *= x / (a + float64(n))
			sum += term
			if math.Abs(term) < math.Abs(sum)*1e-15 {
				break
			}
		}
		return math.Max(0, 1-sum*math.Exp(-x+a*math.Log(x)-lg))
	}
	const tiny = 1e-300
	b := x + 1 - a
	c := 1 / tiny
	d := 1 / b
	h := d
	for i := 1; i < 500; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = b + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		delta := d * c
		h *= delta
		if math.Abs(delta-1) < 1e-15 {
			break
		}
	}
	return math.Exp(-x+a*math.Log(x)-lg) * h
}

// r6 rounds to six decimal places, the precision the tracking baseline
// publishes at, so a report compares byte for byte across repeat runs.
func r6(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	return math.Round(v*1e6) / 1e6
}
