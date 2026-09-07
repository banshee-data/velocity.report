package l5tracks

import "math"

// Filter consistency instrumentation (Phase 0).
//
// The tracker carries a single fixed scalar measurement noise for every
// observation, at every range, on every aspect. Phase 3 of the state
// estimation plan replaces that with an observation-conditioned model, and
// Phase 0's job is to produce the evidence that the fixed scalar is actually
// wrong rather than to assume it.
//
// Two quantities do that, and neither needs a human label, which is why they
// can be measured now while reference annotation is still being built.
//
// NIS, the normalised innovation squared, is the filter grading its own
// uncertainty. For a two-dimensional position measurement a consistent filter
// produces NIS values following a chi-squared distribution with two degrees of
// freedom, so their mean sits at 2. A mean well above 2 means the filter is
// overconfident: reality surprises it more often than its covariance predicts,
// which for a fixed scalar noise is what range and partial visibility would
// do. A mean well below 2 means it is underconfident and discarding
// information. Either way the number is diagnostic rather than a verdict: NIS
// also rises when the motion model is wrong, so a manoeuvring vehicle inflates
// it without the noise model being at fault.
//
// The lateral residual splits the innovation into the part along the track's
// direction of travel and the part across it. They fail differently. A car
// seen end-on has its along-track extent guessed and its across-track extent
// measured, so the centroid slides longitudinally as visibility changes while
// staying laterally stable. Pooling the two hides that, and the plan asks for
// the lateral distribution specifically because it is the one a lane-keeping
// or road-position claim rests on.
//
// Both are banded by speed, because the failure is not uniform: a stationary
// track's innovation is dominated by clustering jitter, while a fast one's is
// dominated by the constant-velocity model's inability to turn.

// ResidualSpeedBands are the lower edges, in metres per second, of the bands
// residuals are accumulated into. The last band is open-ended.
var ResidualSpeedBands = [...]float32{0, 2, 5, 10, 15}

// ResidualBandCount is the number of speed bands.
const ResidualBandCount = len(ResidualSpeedBands)

// residualBand returns the band index for a speed.
func residualBand(speed float32) int {
	band := 0
	for i, edge := range ResidualSpeedBands {
		if speed >= edge {
			band = i
		}
	}
	return band
}

// ResidualAccumulator holds running sums for one speed band. Sums rather than
// samples: the cost has to stay constant per track on a Pi, and the mean and
// RMS are what the acceptance gate compares.
type ResidualAccumulator struct {
	Count int `json:"count"`
	// LateralSumSq and LongitudinalSumSq accumulate squared metres, so their
	// roots are RMS residuals.
	LateralSumSq      float64 `json:"lateral_sum_sq"`
	LongitudinalSumSq float64 `json:"longitudinal_sum_sq"`
	// LateralSum keeps the signed mean, which separates a biased estimator
	// from a merely noisy one. A centroid that consistently sits to one side
	// of the truth shows up here and nowhere else.
	LateralSum      float64 `json:"lateral_sum"`
	LongitudinalSum float64 `json:"longitudinal_sum"`
	// NISSum accumulates the normalised innovation squared.
	NISSum float64 `json:"nis_sum"`
	// NISOverThreshold counts observations above the 95th-percentile
	// chi-squared bound for two degrees of freedom. A consistent filter puts
	// about 5% of observations here.
	NISOverThreshold int `json:"nis_over_threshold"`
	// Decomposed counts observations fast enough to split into lateral and
	// longitudinal parts. Below that speed the direction of travel is noise,
	// so the lateral figure is not zero error — it is no measurement at all,
	// and the two must not be confused.
	Decomposed int `json:"decomposed"`
}

// ChiSquared2DOF95 is the 95th percentile of the chi-squared distribution with
// two degrees of freedom.
const ChiSquared2DOF95 = 5.991

// ResidualBands is the per-track set of banded accumulators.
type ResidualBands [ResidualBandCount]ResidualAccumulator

// Observe records one innovation against its predicted covariance.
//
// The innovation (yX, yY) is the measurement minus the prediction, in metres,
// in the world frame. (vx, vy) is the predicted velocity, which defines the
// along-track direction. invS is the inverse innovation covariance, already
// computed for the Kalman gain.
func (b *ResidualBands) Observe(yX, yY, vx, vy float32, invS00, invS01, invS10, invS11 float32) {
	speed := float32(math.Hypot(float64(vx), float64(vy)))
	acc := &b[residualBand(speed)]

	// Below a usable speed the direction of travel is estimator noise, so the
	// lateral/longitudinal split would be a rotation by a random angle. Record
	// the magnitude in the longitudinal slot and leave lateral alone rather
	// than manufacturing a decomposition.
	lat, lon := 0.0, math.Hypot(float64(yX), float64(yY))
	if speed > CourseAlignmentMinSpeedMps {
		ux, uy := float64(vx)/float64(speed), float64(vy)/float64(speed)
		lon = ux*float64(yX) + uy*float64(yY)
		lat = -uy*float64(yX) + ux*float64(yY)
	}

	nis := float64(yX)*float64(invS00*yX+invS01*yY) + float64(yY)*float64(invS10*yX+invS11*yY)
	if math.IsNaN(nis) || math.IsInf(nis, 0) {
		return
	}

	acc.Count++
	if speed > CourseAlignmentMinSpeedMps {
		acc.Decomposed++
	}
	acc.LateralSum += lat
	acc.LongitudinalSum += lon
	acc.LateralSumSq += lat * lat
	acc.LongitudinalSumSq += lon * lon
	acc.NISSum += nis
	if nis > ChiSquared2DOF95 {
		acc.NISOverThreshold++
	}
}

// Add folds another set of bands into this one, for run-level rollups.
func (b *ResidualBands) Add(other ResidualBands) {
	for i := range b {
		b[i].Count += other[i].Count
		b[i].LateralSum += other[i].LateralSum
		b[i].LongitudinalSum += other[i].LongitudinalSum
		b[i].LateralSumSq += other[i].LateralSumSq
		b[i].LongitudinalSumSq += other[i].LongitudinalSumSq
		b[i].NISSum += other[i].NISSum
		b[i].NISOverThreshold += other[i].NISOverThreshold
		b[i].Decomposed += other[i].Decomposed
	}
}

// ResidualBandSummary is one band reduced to the figures a baseline compares.
type ResidualBandSummary struct {
	SpeedFloorMps float32 `json:"speed_floor_mps"`
	Count         int     `json:"count"`
	// Decomposed is how many of Count were fast enough to separate lateral
	// from longitudinal. Zero means the lateral figures below are absent
	// rather than zero.
	Decomposed int `json:"decomposed"`
	// LateralRMSMetres and LongitudinalRMSMetres are spread; the Bias fields
	// are the signed means. A large RMS with near-zero bias is noise; a large
	// bias is a systematic offset, and the two call for different fixes.
	LateralRMSMetres      float64 `json:"lateral_rms_metres"`
	LongitudinalRMSMetres float64 `json:"longitudinal_rms_metres"`
	LateralBiasMetres     float64 `json:"lateral_bias_metres"`
	LongitudinalBias      float64 `json:"longitudinal_bias_metres"`
	// MeanNIS sits at about 2 for a consistent two-dimensional filter.
	MeanNIS float64 `json:"mean_nis"`
	// NISExceedanceRatio is the share above the 95% chi-squared bound, which a
	// consistent filter holds near 0.05.
	NISExceedanceRatio float64 `json:"nis_exceedance_ratio"`
}

// Summarise reduces the bands. Empty bands are omitted: a band with no
// observations has no residual, and reporting zero would read as a perfect
// score for a speed nothing travelled at.
func (b ResidualBands) Summarise() []ResidualBandSummary {
	out := make([]ResidualBandSummary, 0, ResidualBandCount)
	for i, acc := range b {
		if acc.Count == 0 {
			continue
		}
		n := float64(acc.Count)
		out = append(out, ResidualBandSummary{
			SpeedFloorMps:         ResidualSpeedBands[i],
			Count:                 acc.Count,
			Decomposed:            acc.Decomposed,
			LateralRMSMetres:      math.Sqrt(acc.LateralSumSq / math.Max(1, float64(acc.Decomposed))),
			LongitudinalRMSMetres: math.Sqrt(acc.LongitudinalSumSq / n),
			LateralBiasMetres:     acc.LateralSum / math.Max(1, float64(acc.Decomposed)),
			LongitudinalBias:      acc.LongitudinalSum / n,
			MeanNIS:               acc.NISSum / n,
			NISExceedanceRatio:    float64(acc.NISOverThreshold) / n,
		})
	}
	return out
}

// AssociationBands counts, per speed band, how often a track was matched to a
// cluster and how often it went without.
//
// A miss is not necessarily a failure — a vehicle can leave the field of view
// or be genuinely occluded — which is why this is a rate to be read next to
// the residuals rather than an error count. The plan asks for it by speed band
// because the constant-velocity model's prediction degrades with speed, and a
// prediction that lands outside the gate produces exactly this.
type AssociationBands struct {
	Matched [ResidualBandCount]int `json:"matched"`
	Missed  [ResidualBandCount]int `json:"missed"`
}

// Observe records one frame's outcome for a track at the given speed.
func (a *AssociationBands) Observe(speed float32, matched bool) {
	band := residualBand(speed)
	if matched {
		a.Matched[band]++
		return
	}
	a.Missed[band]++
}

// Add folds another set of counts into this one.
func (a *AssociationBands) Add(other AssociationBands) {
	for i := range a.Matched {
		a.Matched[i] += other.Matched[i]
		a.Missed[i] += other.Missed[i]
	}
}

// AssociationBandSummary is one band's association rate.
type AssociationBandSummary struct {
	SpeedFloorMps float32 `json:"speed_floor_mps"`
	Matched       int     `json:"matched"`
	Missed        int     `json:"missed"`
	Rate          float64 `json:"rate"`
}

// Summarise reduces the counts, omitting bands nothing travelled at.
func (a AssociationBands) Summarise() []AssociationBandSummary {
	out := make([]AssociationBandSummary, 0, ResidualBandCount)
	for i := range a.Matched {
		total := a.Matched[i] + a.Missed[i]
		if total == 0 {
			continue
		}
		out = append(out, AssociationBandSummary{
			SpeedFloorMps: ResidualSpeedBands[i],
			Matched:       a.Matched[i],
			Missed:        a.Missed[i],
			Rate:          float64(a.Matched[i]) / float64(total),
		})
	}
	return out
}
