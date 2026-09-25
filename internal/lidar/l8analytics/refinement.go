package l8analytics

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// Label-free comparison of the online estimate with retrospectively refined
// ones (state-estimation plan §10, G-SMO-1), computed on one replay so every
// arm sees exactly the same filter record and association.
//
// None of these figures is accuracy. Without an independent reference they
// measure how far a refinement moved the estimate, how consistent it is with
// the measurements and with its own model, how smooth it is, and how long it
// takes to become final. A lower residual or a smaller speed step is not by
// itself better: a smoother that flattened every manoeuvre would win both
// (plan §17.1). The acceptance criteria that do bear on accuracy, identity
// and manoeuvre preservation are predeclared in
// docs/lidar/operations/retrospective-refinement-criteria.md and need the
// reviewed reference set; these numbers are the paired, label-free half.
//
// Population: states of tracks confirmed at that frame, at or after the
// scoring start. Residual figures use observed states only; coasted states
// count towards revision, speed-step and latency figures.

// RevisionFlagMetres is G-SMO-1 criterion 4's per-state revision bound: a
// larger revision is permitted but must be flagged.
const RevisionFlagMetres = 0.3

// headingMinSpeedMps is the speed below which an estimate's velocity is too
// uncertain to define lateral and longitudinal axes.
const headingMinSpeedMps = 1.0

// Quantiles is a distribution summary.
type Quantiles struct {
	N   int     `json:"n"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

func quantilesOf(values []float64) Quantiles {
	if len(values) == 0 {
		return Quantiles{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	at := func(p float64) float64 {
		// Nearest rank: the smallest value with at least p of the sample at
		// or below it. Reproducible, and never interpolates past the data.
		rank := int(math.Ceil(p*float64(len(sorted)))) - 1
		if rank < 0 {
			rank = 0
		}
		return sorted[rank]
	}
	return Quantiles{N: len(sorted), P50: at(0.50), P95: at(0.95), P99: at(0.99), Max: sorted[len(sorted)-1]}
}

func rms(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(values)))
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// RefinementArmMetrics are one arm's label-free figures.
type RefinementArmMetrics struct {
	// Label names the arm: online, or the refinement stage and lag.
	Label string `json:"label"`
	Stage string `json:"stage"`
	Lag   string `json:"lag,omitempty"`

	// EstimatorID and ParamHash key the arm's persisted rows, when the replay
	// wrote any, for the per-frame evaluator. Filled in by the caller.
	EstimatorID string `json:"estimator_id,omitempty"`
	ParamHash   string `json:"param_hash,omitempty"`

	States         int `json:"states"`
	ObservedStates int `json:"observed_states"`
	Tracks         int `json:"tracks"`

	// Revision is refined minus online, per observed state: the persisted
	// population G-SMO-1 criterion 4 bounds.
	RevisionPositionM   Quantiles `json:"revision_position_m"`
	RevisionVelocityMps Quantiles `json:"revision_velocity_mps"`
	// RevisionsFlagged counts observed states moved by more than
	// RevisionFlagMetres.
	RevisionsFlagged int `json:"revisions_flagged"`
	// PerFrameMaxRevisionP99M is G-SMO-1 criterion 4 as written: the p99 over
	// frames of the largest observed-state revision in each frame.
	PerFrameMaxRevisionP99M float64 `json:"per_frame_max_revision_p99_m"`
	// CoastedRevisionPositionM is the same for coasted states. A prediction
	// through an occlusion is corrected by the observation that ends it, and
	// that is where the large revisions are: evidence-driven, but kept apart
	// so they neither hide in nor inflate the persisted figure.
	CoastedRevisionPositionM Quantiles `json:"coasted_revision_position_m"`
	CoastedRevisionsFlagged  int       `json:"coasted_revisions_flagged"`
	// RevisionsWithoutEvidence must be zero: a moved state with no new
	// observation in its window is a defect, not a smoothing choice.
	RevisionsWithoutEvidence int `json:"revisions_without_evidence"`

	// Residuals are measurement minus estimate for observed states, resolved
	// along and across the arm's own velocity when it exceeds 1 m/s.
	ResidualM             Quantiles `json:"residual_m"`
	ResidualLateralM      Quantiles `json:"residual_lateral_m"`
	ResidualLongitudinalM Quantiles `json:"residual_longitudinal_m"`

	// NormalisedResidualMean is the mean of rᵀ(R - H P Hᵀ)⁻¹r, the residual
	// normalised by its own covariance under the model. For the online
	// posterior and for a smoothed estimate alike its expectation is the
	// measurement dimension, 2, when the model is consistent; states where
	// R - H P Hᵀ is not positive definite are counted, not scored.
	NormalisedResidualMean    float64 `json:"normalised_residual_mean"`
	NormalisedResidualSkipped int     `json:"normalised_residual_skipped"`

	// InnovationNISMean is the filter's own innovation NIS over the same
	// observed states. Fixed assignment leaves it the same in every arm; it
	// is here so a table shows the evidence every arm was computed under.
	InnovationNISMean float64 `json:"innovation_nis_mean"`

	// CVPredictionErrorM predicts each observed state's estimate at constant
	// velocity to the track's next observed state and compares with that
	// measurement. It measures self-consistency, not accuracy (plan §21.1
	// D5): a refined state has already seen the measurement it predicts.
	CVPredictionErrorM        Quantiles `json:"cv_prediction_error_m"`
	CVPredictionErrorLateralM Quantiles `json:"cv_prediction_error_lateral_m"`

	// SpeedStepRMSMps2 is the RMS of speed change per second between
	// consecutive states of a track, the D5 table's "speed step".
	SpeedStepRMSMps2 float64 `json:"speed_step_rms_mps2"`

	// FinalityDelaySecs is capture time from a state to the frame at which
	// its estimate was released: zero online. It includes states released
	// when their track ended, which wait for the tracker to delete it.
	FinalityDelaySecs Quantiles `json:"finality_delay_secs"`
	// LagReleaseDelaySecs is the same over states released because their lag
	// was reached: the smoother's own latency, which the criteria bound.
	LagReleaseDelaySecs Quantiles `json:"lag_release_delay_secs"`
	// LookaheadTruncated counts states released with less look-ahead than
	// the arm asks for (the track or the input ended first).
	LookaheadTruncated int `json:"lookahead_truncated"`
	// ClampedPrediction counts states whose look-ahead crossed a transition
	// the filter predicted across less time than elapsed.
	ClampedPrediction int `json:"clamped_prediction"`
	// CovarianceFallbacks counts smoothed covariances replaced by the
	// filtered one after failing their positive-definiteness check.
	CovarianceFallbacks int `json:"covariance_fallbacks"`
}

// RefinementAccumulator gathers one arm's figures from the states a smoother
// releases, in release order.
type RefinementAccumulator struct {
	label, stage, lag string
	online            bool
	measurementNoise  float64
	scoreFromNanos    int64

	states, observed int
	tracks           map[int64]*trackCursor
	frameMax         map[int64]float64

	revPos, revVel                    []float64
	flagged, withoutEvidence          int
	coastedRev                        []float64
	coastedFlagged                    int
	residual, residualLat, residualLo []float64
	normalised                        []float64
	normalisedSkipped                 int
	nis                               []float64
	cvError, cvLateral                []float64
	speedSteps                        []float64
	finality, lagFinality             []float64
	truncated, clamped, fallbacks     int
}

type trackCursor struct {
	haveLast     bool
	last         l5tracks.FilterMoments
	lastNanos    int64
	haveObserved bool
	observed     l5tracks.FilterMoments
	observedAt   int64
}

// NewRefinementAccumulator scores refined states. measurementNoise is the
// filter's R (variance, m²); states before scoreFromNanos are ignored.
func NewRefinementAccumulator(label, stage, lag string, measurementNoise float64, scoreFromNanos int64) *RefinementAccumulator {
	return &RefinementAccumulator{label: label, stage: stage, lag: lag, measurementNoise: measurementNoise,
		scoreFromNanos: scoreFromNanos, tracks: map[int64]*trackCursor{}, frameMax: map[int64]float64{}}
}

// NewOnlineAccumulator scores the online estimate carried inside refined
// states. Feed it one refined arm's full release stream: every arm releases
// every state exactly once, so the population is the same.
func NewOnlineAccumulator(measurementNoise float64, scoreFromNanos int64) *RefinementAccumulator {
	a := NewRefinementAccumulator("online", string(l5tracks.RefinementOnline), "", measurementNoise, scoreFromNanos)
	a.online = true
	return a
}

// Add scores one released state.
func (a *RefinementAccumulator) Add(s l5tracks.SmoothedState) {
	if !s.Confirmed || s.StateUnixNanos < a.scoreFromNanos {
		return
	}
	estimate := s.Smoothed
	if a.online {
		estimate = s.Online
	}
	a.states++
	cursor := a.tracks[s.CreationSequence]
	if cursor == nil {
		cursor = &trackCursor{}
		a.tracks[s.CreationSequence] = cursor
	}

	if !a.online {
		switch {
		case s.Observed:
			a.revPos = append(a.revPos, s.Revision.PositionMetres)
			a.revVel = append(a.revVel, s.Revision.VelocityMps)
			if s.Revision.PositionMetres > RevisionFlagMetres {
				a.flagged++
			}
			if worst, ok := a.frameMax[s.FrameUnixNanos]; !ok || s.Revision.PositionMetres > worst {
				a.frameMax[s.FrameUnixNanos] = s.Revision.PositionMetres
			}
		default:
			a.coastedRev = append(a.coastedRev, s.Revision.PositionMetres)
			if s.Revision.PositionMetres > RevisionFlagMetres {
				a.coastedFlagged++
			}
		}
		if s.RevisionWithoutEvidence {
			a.withoutEvidence++
		}
		a.finality = append(a.finality, s.FinalityDelaySecs())
		if s.Release == l5tracks.ReleaseLag {
			a.lagFinality = append(a.lagFinality, s.FinalityDelaySecs())
		}
		if s.LookaheadTruncated {
			a.truncated++
		}
		if s.ClampedPrediction {
			a.clamped++
		}
		if s.CovarianceFallback {
			a.fallbacks++
		}
	} else {
		a.finality = append(a.finality, 0)
		if _, ok := a.frameMax[s.FrameUnixNanos]; !ok && s.Observed {
			a.frameMax[s.FrameUnixNanos] = 0
		}
	}

	if cursor.haveLast {
		if dt := float64(s.StateUnixNanos-cursor.lastNanos) / 1e9; dt > 0 {
			step := (speedOf(estimate) - speedOf(cursor.last)) / dt
			a.speedSteps = append(a.speedSteps, step)
		}
	}
	cursor.haveLast, cursor.last, cursor.lastNanos = true, estimate, s.StateUnixNanos

	if !s.Observed {
		return
	}
	a.observed++
	z := s.Observation
	rx, ry := float64(z.X)-float64(estimate.X), float64(z.Y)-float64(estimate.Y)
	a.residual = append(a.residual, math.Hypot(rx, ry))
	if lat, lon, ok := resolveAlong(rx, ry, estimate); ok {
		a.residualLat = append(a.residualLat, math.Abs(lat))
		a.residualLo = append(a.residualLo, math.Abs(lon))
	}
	if q, ok := normalisedResidual(rx, ry, estimate.P, a.measurementNoise); ok {
		a.normalised = append(a.normalised, q)
	} else {
		a.normalisedSkipped++
	}
	if z.HasInnovation {
		a.nis = append(a.nis, float64(z.NIS))
	}
	if cursor.haveObserved {
		if dt := float64(s.StateUnixNanos-cursor.observedAt) / 1e9; dt > 0 {
			prev := cursor.observed
			px := float64(prev.X) + float64(prev.VX)*dt
			py := float64(prev.Y) + float64(prev.VY)*dt
			ex, ey := float64(z.X)-px, float64(z.Y)-py
			a.cvError = append(a.cvError, math.Hypot(ex, ey))
			if lat, _, ok := resolveAlong(ex, ey, prev); ok {
				a.cvLateral = append(a.cvLateral, math.Abs(lat))
			}
		}
	}
	cursor.haveObserved, cursor.observed, cursor.observedAt = true, estimate, s.StateUnixNanos
}

func speedOf(m l5tracks.FilterMoments) float64 { return math.Hypot(float64(m.VX), float64(m.VY)) }

// resolveAlong splits (dx, dy) into the components across and along the
// estimate's velocity.
func resolveAlong(dx, dy float64, m l5tracks.FilterMoments) (lateral, longitudinal float64, ok bool) {
	speed := speedOf(m)
	if speed < headingMinSpeedMps {
		return 0, 0, false
	}
	ux, uy := float64(m.VX)/speed, float64(m.VY)/speed
	return -uy*dx + ux*dy, ux*dx + uy*dy, true
}

// normalisedResidual is rᵀ(R·I - P_pos)⁻¹r for the 2×2 position block, or
// false when that covariance is not positive definite.
func normalisedResidual(rx, ry float64, p [16]float32, r float64) (float64, bool) {
	a := r - float64(p[0])
	b := -0.5 * (float64(p[1]) + float64(p[4]))
	d := r - float64(p[5])
	det := a*d - b*b
	if !(a > 0) || !(det > 1e-12*r*r) {
		return 0, false
	}
	return (d*rx*rx - 2*b*rx*ry + a*ry*ry) / det, true
}

// Metrics summarises what has been added.
func (a *RefinementAccumulator) Metrics() RefinementArmMetrics {
	frameMax := make([]float64, 0, len(a.frameMax))
	for _, v := range a.frameMax {
		frameMax = append(frameMax, v)
	}
	speedAbs := make([]float64, len(a.speedSteps))
	for i, v := range a.speedSteps {
		speedAbs[i] = math.Abs(v)
	}
	return RefinementArmMetrics{
		Label: a.label, Stage: a.stage, Lag: a.lag,
		States: a.states, ObservedStates: a.observed, Tracks: len(a.tracks),
		RevisionPositionM: quantilesOf(a.revPos), RevisionVelocityMps: quantilesOf(a.revVel),
		RevisionsFlagged: a.flagged, PerFrameMaxRevisionP99M: quantilesOf(frameMax).P99,
		CoastedRevisionPositionM: quantilesOf(a.coastedRev), CoastedRevisionsFlagged: a.coastedFlagged,
		RevisionsWithoutEvidence: a.withoutEvidence,
		ResidualM:                quantilesOf(a.residual), ResidualLateralM: quantilesOf(a.residualLat),
		ResidualLongitudinalM:  quantilesOf(a.residualLo),
		NormalisedResidualMean: mean(a.normalised), NormalisedResidualSkipped: a.normalisedSkipped,
		InnovationNISMean:  mean(a.nis),
		CVPredictionErrorM: quantilesOf(a.cvError), CVPredictionErrorLateralM: quantilesOf(a.cvLateral),
		SpeedStepRMSMps2:  rms(speedAbs),
		FinalityDelaySecs: quantilesOf(a.finality), LagReleaseDelaySecs: quantilesOf(a.lagFinality),
		LookaheadTruncated: a.truncated,
		ClampedPrediction:  a.clamped, CovarianceFallbacks: a.fallbacks,
	}
}

// RefinementComparisonMarkdown renders arms as the per-horizon table the
// criteria document and a replay report quote.
func RefinementComparisonMarkdown(arms []RefinementArmMetrics) string {
	var b strings.Builder
	b.WriteString("| Arm | Observed / coasted | Revision p50 / p95 / p99 (m) | >0.3 m | Frame-max p99 (m) | Velocity revision p95 (m/s) | Coasted revision p95 (m), >0.3 m | Lateral residual p95 (m) | Normalised residual mean | CV prediction error p95 (m) | Speed step RMS (m/s²) | Finality delay p50 / p95 (s) |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, m := range arms {
		fmt.Fprintf(&b, "| %s | %d / %d | %.3f / %.3f / %.3f | %d | %.3f | %.3f | %.3f, %d | %.3f | %.2f | %.3f | %.2f | %.2f / %.2f |\n",
			m.Label, m.ObservedStates, m.States-m.ObservedStates,
			m.RevisionPositionM.P50, m.RevisionPositionM.P95, m.RevisionPositionM.P99,
			m.RevisionsFlagged, m.PerFrameMaxRevisionP99M, m.RevisionVelocityMps.P95,
			m.CoastedRevisionPositionM.P95, m.CoastedRevisionsFlagged, m.ResidualLateralM.P95,
			m.NormalisedResidualMean, m.CVPredictionErrorM.P95, m.SpeedStepRMSMps2,
			m.FinalityDelaySecs.P50, m.FinalityDelaySecs.P95)
	}
	return b.String()
}
