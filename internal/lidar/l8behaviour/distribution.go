package l8behaviour

// A headway distribution over stored following interactions, per Sections 6,
// 9.3 and 10.4 of docs/plans/lidar-behaviour-analytics-plan.md: one version
// group's encounters pooled into a time-weighted spatial gap and net time gap
// distribution with named-band exposure, and every other second of accounted
// encounter time beside it under its reason, so a thin bar is never mistaken
// for a quiet road.
//
// Method following_distribution_v1:
//
//   - Pooling. Only interactions of one InteractionVersion are pooled:
//     estimate stage, estimator, observation model, method with its parameter
//     hash, and estimator parameter hash. Geometry is per follower by
//     construction and never splits a group.
//   - Value block. An encounter's values are read from its production
//     measurements when its stage is final and from its review-only
//     provisional block otherwise, exactly as stored.
//   - Exposure support. An encounter enters the distribution when every band
//     duration in its value block is supported. Its valid instants are binned
//     by the values stored on them, each weighted by the time it stands for.
//     An encounter whose band exposure is suppressed (below the minimum
//     opportunity, say) contributes its valid time under that measurement's
//     reason instead, never as a zero.
//   - Accounted time. The share denominator is every encounter's valid time
//     plus its suppressed time; record gaps stand for nothing and are
//     reported apart. Suppressed time appears under its instant's reason
//     with the predicted-only part of it, so the bins and the excluded time
//     add up to the denominator exactly. Valid time is read from the
//     observed exposure windows, the Section 10.3 denominator, and must equal
//     the pooled accounting.
//   - Bins. Fixed width and half open, [lower, upper), with an open last
//     bin, in exact thousandths of the unit. Every band threshold is a bin
//     edge, so the time left of a band's rule is exactly the pooled time
//     below it.
//   - Uncertainty. Nothing is re-estimated. Each bin carries two bounds from
//     the one-sigma stored on every instant: the time whose closed interval,
//     value plus or minus one sigma, lies wholly inside the half-open bin,
//     and the time whose interval reaches the bin at all. They bracket the
//     bin's time and are not a coverage interval. The minimum and median of each encounter keep their stored
//     Monte Carlo intervals (EncounterSummary); no pooled minimum or median
//     is derived, because the per-encounter draws do not give its interval.
//   - Band exposure. For each band, over the exposure-supported encounters:
//     the valid time and instants below it, how many encounters had any, and
//     the pooled rate, time below over their valid time, keyed by the band's
//     rate id and declaring no uncertainty.

import (
	"errors"
	"fmt"
	"math"
)

// DistributionSchema versions the distribution: its shape, the rules above
// and the bins. Change it when any of them changes.
const DistributionSchema = "following_distribution_v1"

// histogramSpec is one distributed metric: fixed bins of binMilli thousandths
// of its unit, closed up to maxMilli, then one open bin.
type histogramSpec struct {
	id                 MetricID
	binMilli, maxMilli int64
}

// distributionHistograms are the distributed metrics: net time gap in 0.25 s
// bins to 3 s, and spatial gap in 2 m bins to 40 m.
var distributionHistograms = []histogramSpec{
	{MetricFollowingNetTimeGap, 250, 3000},
	{MetricFollowingSpatialGap, 2000, 40000},
}

// DistributedMetrics are the metrics a FollowingDistribution bins, net time
// gap first.
func DistributedMetrics() []MetricID {
	out := make([]MetricID, len(distributionHistograms))
	for i, h := range distributionHistograms {
		out[i] = h.id
	}
	return out
}

func (h histogramSpec) edge(k int) float64 { return float64(int64(k)*h.binMilli) / 1000 }

func (h histogramSpec) bins() int { return int(h.maxMilli/h.binMilli) + 1 }

// index is the bin holding v: the last k with edge(k) <= v, capped at the
// open bin. The float estimate is corrected against the edges themselves, so
// a value on an edge lands where a direct comparison puts it.
func (h histogramSpec) index(v float64) int {
	last := h.bins() - 1
	k := min(max(int(math.Floor(v*1000/float64(h.binMilli))), 0), last)
	for k > 0 && v < h.edge(k) {
		k--
	}
	for k < last && v >= h.edge(k+1) {
		k++
	}
	return k
}

func (h histogramSpec) empty() Histogram {
	def, _ := LookupMetric(h.id)
	out := Histogram{Unit: def.Unit, Bins: make([]HistogramBin, h.bins())}
	for k := range out.Bins {
		out.Bins[k].Lower = h.edge(k)
		if k < len(out.Bins)-1 {
			out.Bins[k].Upper = ptr(h.edge(k + 1))
		}
	}
	return out
}

// add bins one valid instant's value, with its one-sigma reach.
func (h histogramSpec) add(hist *Histogram, v SeriesValue, nanos int64) error {
	if v.Value < 0 {
		return fmt.Errorf("%s value %.6g is below the first bin", h.id, v.Value)
	}
	k := h.index(v.Value)
	lo, hi := h.index(max(v.Value-v.Sigma, 0)), h.index(v.Value+v.Sigma)
	hist.Bins[k].Instants++
	hist.Bins[k].Nanos += nanos
	if lo == hi && v.Value-v.Sigma >= h.edge(k) {
		hist.Bins[k].SigmaInsideNanos += nanos
	}
	for j := lo; j <= hi; j++ {
		hist.Bins[j].SigmaOverlapNanos += nanos
	}
	return nil
}

// checkBandEdges requires every band threshold on a net time gap bin edge.
func checkBandEdges() error {
	h := distributionHistograms[0]
	for _, b := range FollowingBands() {
		if h.edge(h.index(b.Seconds)) != b.Seconds {
			return fmt.Errorf("band %g s is not an edge of the %d ms bins", b.Seconds, h.binMilli)
		}
	}
	return nil
}

// SurfaceStatus says what a served following result is. Nothing is promoted
// until the field promotion gates (G-GEO-1, G-UNC-1, G-SMO-1 and the metric
// gate) can be asserted, so no promoted status exists to claim.
type SurfaceStatus string

const (
	// StatusSyntheticOracle is output from the analytic fixture estimator:
	// known bumpers, gap and time gap, no sensor data.
	StatusSyntheticOracle SurfaceStatus = "synthetic_oracle"
	// StatusProvisional is estimator output that has not passed the field
	// promotion gates, at every estimate stage, final included.
	StatusProvisional SurfaceStatus = "provisional"
)

// StatusOf is the status of results at a version: a synthetic oracle when
// its estimator is the analytic fixture estimator, provisional otherwise.
func StatusOf(v InteractionVersion) SurfaceStatus {
	f := FixtureEstimate()
	if v.EstimatorID == f.EstimatorID && v.ObsModelID == f.ObsModelID && v.ParamHash == f.ParamHash {
		return StatusSyntheticOracle
	}
	return StatusProvisional
}

// Label is the status as every chart draws it. Both begin PROVISIONAL, so a
// synthetic oracle is never read as a field result.
func (s SurfaceStatus) Label() string {
	if s == StatusSyntheticOracle {
		return "PROVISIONAL · SYNTHETIC ORACLE"
	}
	return "PROVISIONAL"
}

// HistogramBin is one bin, [Lower, Upper) in the metric's unit; Upper is
// absent on the open last bin. Nanos is the valid time whose value falls in
// it. SigmaInsideNanos and SigmaOverlapNanos bracket it: the time whose
// closed interval, value plus or minus its stored one-sigma, lies wholly
// inside the bin, and the time whose interval reaches the bin at all.
type HistogramBin struct {
	Lower             float64  `json:"lower"`
	Upper             *float64 `json:"upper,omitempty"`
	Instants          int      `json:"instants"`
	Nanos             int64    `json:"nanos"`
	SigmaInsideNanos  int64    `json:"sigma_inside_nanos"`
	SigmaOverlapNanos int64    `json:"sigma_overlap_nanos"`
}

// Histogram is one metric's time-weighted distribution over the
// exposure-supported encounters' valid time.
type Histogram struct {
	Unit string         `json:"unit"`
	Bins []HistogramBin `json:"bins"`
}

// Nanos is the histogram's total time.
func (h Histogram) Nanos() int64 {
	var n int64
	for _, b := range h.Bins {
		n += b.Nanos
	}
	return n
}

// ExcludedTime is accounted time outside the distribution under one reason:
// suppressed instants, and the valid time of encounters whose band exposure
// is suppressed. PredictedOnlyNanos is the part of it at which either party
// was not observed.
type ExcludedTime struct {
	Instants           int   `json:"instants"`
	Nanos              int64 `json:"nanos"`
	PredictedOnlyNanos int64 `json:"predicted_only_nanos"`
}

// BandExposure is one named band pooled over the exposure-supported
// encounters. Name is the band's duration metric id and Threshold its net
// time gap in seconds; Rate is keyed by the band's rate id.
type BandExposure struct {
	Name      MetricID         `json:"name"`
	Threshold float64          `json:"threshold"`
	Events    int              `json:"events"`
	Instants  int              `json:"instants"`
	Nanos     int64            `json:"nanos"`
	Rate      MeasurementValue `json:"rate"`
}

// FollowingDistribution is one version group's following encounters,
// pooled.
type FollowingDistribution struct {
	Schema  string             `json:"schema"`
	Version InteractionVersion `json:"version"`
	// Events counts every encounter pooled; ExposureEvents those whose band
	// exposure is supported, which alone fill the bins and bands.
	Events         int `json:"events"`
	ExposureEvents int `json:"exposure_events"`
	// Accounting is the sum of every encounter's stored accounting:
	// bookkeeping over all of them, including those outside the bins.
	Accounting InteractionAccounting `json:"accounting"`
	// AccountedNanos is valid plus suppressed time, the share denominator;
	// the bins' time and every excluded reason's time add up to it.
	AccountedNanos int64                              `json:"accounted_nanos"`
	Histograms     map[MetricID]Histogram             `json:"histograms"`
	Excluded       map[SuppressionReason]ExcludedTime `json:"excluded"`
	Bands          []BandExposure                     `json:"bands"`
}

// ExcludedReasons returns the reasons with excluded time, in precedence
// order.
func (d FollowingDistribution) ExcludedReasons() []SuppressionReason {
	var out []SuppressionReason
	for _, r := range SuppressionReasons() {
		if _, ok := d.Excluded[r]; ok {
			out = append(out, r)
		}
	}
	return out
}

// ErrNothingToAggregate reports an empty input: there is no version to name
// the distribution by, so the caller says why there is nothing instead.
var ErrNothingToAggregate = errors.New("no following interactions to aggregate")

// valueBlock is the measurement block an encounter's values are read from.
func (ev InteractionEvent) valueBlock() map[MetricID]Measurement {
	if ev.Version.EstimateStage == StageFinal {
		return ev.Measurements
	}
	return ev.Provisional
}

// exposureSupport reports whether every band duration in a value block is
// supported, and otherwise the widest suppressed band's reason.
func exposureSupport(block map[MetricID]Measurement) (bool, SuppressionReason) {
	for _, band := range FollowingBands() {
		if m := block[band.Duration]; m.Suppressed {
			return false, m.Reason
		}
	}
	return true, ReasonUnspecified
}

// AggregateFollowing pools one version group's stored following
// interactions under method following_distribution_v1. Every interaction is
// validated first; interactions of two versions, a repeated event or an
// empty input are errors, never a partial distribution.
func AggregateFollowing(interactions []FollowingInteraction) (FollowingDistribution, error) {
	if len(interactions) == 0 {
		return FollowingDistribution{}, ErrNothingToAggregate
	}
	if err := checkBandEdges(); err != nil {
		return FollowingDistribution{}, err
	}
	bands := FollowingBands()
	d := FollowingDistribution{
		Schema: DistributionSchema, Version: interactions[0].Event.Version.InteractionVersion(),
		Accounting: InteractionAccounting{BandNanos: map[MetricID]int64{}},
		Histograms: map[MetricID]Histogram{}, Excluded: map[SuppressionReason]ExcludedTime{},
	}
	for _, band := range bands {
		d.Accounting.BandNanos[band.Duration] = 0
		d.Bands = append(d.Bands, BandExposure{Name: band.Duration, Threshold: band.Seconds})
	}
	for _, h := range distributionHistograms {
		d.Histograms[h.id] = h.empty()
	}
	exclude := func(r SuppressionReason, nanos int64, predicted bool) {
		x := d.Excluded[r]
		x.Instants++
		x.Nanos += nanos
		if predicted {
			x.PredictedOnlyNanos += nanos
		}
		d.Excluded[r] = x
	}

	seen := map[string]bool{}
	var windowValid, exposureValid int64
	for _, fi := range interactions {
		if err := fi.Validate(); err != nil {
			return FollowingDistribution{}, err
		}
		ev := fi.Event
		if ev.Type != InteractionFollowing {
			return FollowingDistribution{}, fmt.Errorf("event %s is %s, not following", ev.EventID, ev.Type)
		}
		if v := ev.Version.InteractionVersion(); v != d.Version {
			return FollowingDistribution{}, fmt.Errorf("event %s is at version %+v; a distribution pools one version, %+v",
				ev.EventID, v, d.Version)
		}
		if seen[ev.EventID] {
			return FollowingDistribution{}, fmt.Errorf("event %s appears twice", ev.EventID)
		}
		seen[ev.EventID] = true
		d.Events++
		addAccounting(&d.Accounting, ev.Accounting)
		windowValid += OpportunityNanos(fi.Windows)

		exposed, reason := exposureSupport(ev.valueBlock())
		if exposed {
			d.ExposureEvents++
			exposureValid += ev.Accounting.ValidNanos
		}
		below := make([]bool, len(bands))
		for _, in := range fi.Instants {
			counted := in.IntervalNanos
			if in.RecordGap {
				counted = 0
			}
			switch {
			case !in.Valid:
				exclude(in.Reason, counted, in.Basis == BasisPredictedOnly)
				continue
			case !exposed:
				exclude(reason, counted, false)
				continue
			}
			for _, h := range distributionHistograms {
				hist := d.Histograms[h.id]
				if err := h.add(&hist, in.Values[h.id], counted); err != nil {
					return FollowingDistribution{}, fmt.Errorf("event %s instant %d: %w", ev.EventID, in.CaptureUnixNanos, err)
				}
				d.Histograms[h.id] = hist
			}
			thw := in.Values[MetricFollowingNetTimeGap].Value
			for b, band := range bands {
				if band.Contains(thw) {
					d.Bands[b].Instants++
					d.Bands[b].Nanos += counted
					below[b] = below[b] || counted > 0
				}
			}
		}
		for b := range bands {
			if below[b] {
				d.Bands[b].Events++
			}
		}
	}

	if windowValid != d.Accounting.ValidNanos {
		return FollowingDistribution{}, fmt.Errorf("observed windows hold %d ns of valid time, the accounting %d ns",
			windowValid, d.Accounting.ValidNanos)
	}
	d.AccountedNanos = d.Accounting.ValidNanos
	for _, c := range d.Accounting.Suppressions {
		d.AccountedNanos += c.Nanos
	}
	var excluded int64
	for _, x := range d.Excluded {
		excluded += x.Nanos
	}
	for id, h := range d.Histograms {
		if got := h.Nanos(); got != exposureValid || got+excluded != d.AccountedNanos {
			return FollowingDistribution{}, fmt.Errorf("%s bins hold %d ns and %d ns is excluded, of %d ns accounted",
				id, got, excluded, d.AccountedNanos)
		}
	}
	for b, band := range bands {
		rate, err := pooledRate(band.Rate, d.Bands[b].Nanos, exposureValid)
		if err != nil {
			return FollowingDistribution{}, err
		}
		d.Bands[b].Rate = rate
	}
	return d, nil
}

func addAccounting(sum *InteractionAccounting, a InteractionAccounting) {
	sum.Instants += a.Instants
	sum.ValidNanos += a.ValidNanos
	sum.PredictedOnlyNanos += a.PredictedOnlyNanos
	sum.RecordGapNanos += a.RecordGapNanos
	for id, n := range a.BandNanos {
		sum.BandNanos[id] += n
	}
	for r, c := range a.Suppressions {
		if sum.Suppressions == nil {
			sum.Suppressions = map[SuppressionReason]SuppressionCount{}
		}
		s := sum.Suppressions[r]
		s.Instants += c.Instants
		s.Nanos += c.Nanos
		sum.Suppressions[r] = s
	}
}

// pooledRate is time below a band over the exposure-supported valid time, or
// insufficient_observation when there is none. It declares no uncertainty:
// the per-encounter intervals do not combine into one.
func pooledRate(id MetricID, belowNanos, validNanos int64) (MeasurementValue, error) {
	def, ok := LookupMetric(id)
	if !ok {
		return MeasurementValue{}, fmt.Errorf("rate %s is not registered", id)
	}
	m := MeasurementValue{Name: id, Unit: def.Unit}
	if validNanos <= 0 {
		m.Suppressed, m.Reason = true, ReasonInsufficientObservation
	} else {
		m.Value = ptr(float64(belowNanos) / float64(validNanos))
		m.Uncertainty = ptr(NoUncertainty())
		m.OpportunitySeconds = ptr(float64(validNanos) / 1e9)
	}
	return m, m.Validate()
}

// EncounterSummary is one stored encounter as a surface lists it: the pair,
// the interval, the path, the worst support, the evidence counts, the
// accounting and every measurement, without the provenance each measurement
// repeats. Measurements is the production block, which a non-final
// encounter suppresses; Provisional is its review-only values, present
// exactly when the stage is not final.
type EncounterSummary struct {
	EventID          string                        `json:"event_id"`
	PrimaryTrackID   string                        `json:"primary_track_id"`
	SecondaryTrackID string                        `json:"secondary_track_id"`
	StartUnixNanos   int64                         `json:"start_unix_nanos"`
	EndUnixNanos     int64                         `json:"end_unix_nanos"`
	GeometryID       string                        `json:"geometry_id"`
	WorstSupport     SupportState                  `json:"worst_support"`
	Input            InputProvenance               `json:"input"`
	Accounting       InteractionAccounting         `json:"accounting"`
	Measurements     map[MetricID]MeasurementValue `json:"measurements"`
	Provisional      map[MetricID]MeasurementValue `json:"provisional,omitempty"`
}

// Summary returns the event as a surface lists it.
func (ev InteractionEvent) Summary() EncounterSummary {
	strip := func(ms map[MetricID]Measurement) map[MetricID]MeasurementValue {
		if len(ms) == 0 {
			return nil
		}
		out := make(map[MetricID]MeasurementValue, len(ms))
		for id, m := range ms {
			out[id] = m.WithoutProvenance()
		}
		return out
	}
	return EncounterSummary{
		EventID: ev.EventID, PrimaryTrackID: ev.PrimaryTrackID, SecondaryTrackID: ev.SecondaryTrackID,
		StartUnixNanos: ev.StartUnixNanos, EndUnixNanos: ev.EndUnixNanos, GeometryID: ev.Version.GeometryID,
		WorstSupport: ev.WorstSupport, Input: ev.Input, Accounting: ev.Accounting,
		Measurements: strip(ev.Measurements), Provisional: strip(ev.Provisional),
	}
}
