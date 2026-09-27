package headway

// Aggregation over encounters, per Section 10.4: "Aggregate only supported
// encounter values, with their sample count and opportunity denominator;
// never substitute zero for suppressed time."
//
// Rules, each a consequence of that sentence:
//
//   - Version groups. Encounters are pooled only when they share estimate
//     stage, estimator, observation model, parameter hash and method (whose
//     id carries the analysis parameter hash). Anything else would mix
//     versions inside one number, which Section 10.3 forbids.
//   - Distributions. Each encounter metric is summarised over the encounters
//     where it is supported: count, minimum, median (the mean of the middle
//     two for an even count, as l8behaviour's p50) and maximum. Suppressed
//     encounters are counted by reason, not read as zero. No uncertainty is
//     propagated to this level; the encounter rows carry it.
//   - Band exposure. For each band, the time below it and the valid
//     following time are summed over the encounters whose band duration is
//     supported, in integer nanoseconds, so the pooled rate is exact. With no
//     such encounter the pooled rate is suppressed with
//     insufficient_observation.
//   - Distribution of time. The valid time of every encounter whose band
//     exposure is supported is binned by its net time gap. Every other second
//     of the group's accounted time appears beside the bins under its
//     reason: a suppressed instant under its own reason, and the valid time
//     of an encounter whose band exposure is suppressed under that
//     suppression's reason. Bins and excluded time sum to the accounted time
//     exactly, which Build checks.
//   - The review-only predicted gap is read by nothing here.

import (
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

func versionKey(v l8behaviour.VersionProvenance) VersionKey {
	return VersionKey{
		EstimateStage: v.EstimateStage, EstimatorID: v.EstimatorID, ObsModelID: v.ObsModelID,
		ParamHash: v.ParamHash, MethodID: v.MethodID,
	}
}

func buildAggregates(rows []Encounter, sources []l8behaviour.Encounter) ([]Aggregate, error) {
	var keys []VersionKey
	members := map[VersionKey][]int{}
	for i, r := range rows {
		k := versionKey(r.Version)
		if _, ok := members[k]; !ok {
			keys = append(keys, k)
		}
		members[k] = append(members[k], i)
	}
	out := []Aggregate{}
	for gi, k := range keys {
		a, err := buildAggregate(fmt.Sprintf("A%d", gi+1), k, rows, sources, members[k])
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func buildAggregate(id string, key VersionKey, rows []Encounter, sources []l8behaviour.Encounter, idx []int) (Aggregate, error) {
	a := Aggregate{ID: id, Version: key, ValueBlock: rows[idx[0]].ValueBlock}
	for _, i := range idx {
		a.EncounterIDs = append(a.EncounterIDs, rows[i].ID)
	}
	metricIndex := map[l8behaviour.MetricID]int{}
	for k, m := range l8behaviour.EncounterMetrics() {
		metricIndex[m] = k
		a.Metrics = append(a.Metrics, distribution(m, k, rows, idx))
	}

	bands := l8behaviour.FollowingBands()
	for b, band := range bands {
		be, err := bandExposure(band, b, metricIndex[band.Duration], rows, sources, idx)
		if err != nil {
			return Aggregate{}, fmt.Errorf("aggregate %s: %w", id, err)
		}
		a.Bands = append(a.Bands, be)
	}

	h, err := histogram(rows, sources, idx, metricIndex)
	if err != nil {
		return Aggregate{}, fmt.Errorf("aggregate %s: %w", id, err)
	}
	a.Histogram = h
	return a, nil
}

func reasonCounts(m map[l8behaviour.SuppressionReason]int) []ReasonCount {
	out := []ReasonCount{}
	for _, r := range l8behaviour.SuppressionReasons() {
		if m[r] > 0 {
			out = append(out, ReasonCount{Reason: r, Encounters: m[r]})
		}
	}
	return out
}

func distribution(metric l8behaviour.MetricID, k int, rows []Encounter, idx []int) Distribution {
	def, _ := l8behaviour.LookupMetric(metric)
	d := Distribution{Metric: metric, Unit: def.Unit}
	var vals []float64
	suppressed := map[l8behaviour.SuppressionReason]int{}
	for _, i := range idx {
		v := rows[i].Measurements[k]
		if v.Suppressed {
			suppressed[v.Reason]++
			continue
		}
		vals = append(vals, *v.Value)
	}
	d.Supported, d.Suppressed = len(vals), reasonCounts(suppressed)
	if len(vals) == 0 {
		none := suppressedDisplay(l8behaviour.ReasonInsufficientObservation)
		d.MinDisplay, d.P50Display, d.MaxDisplay = none, none, none
		return d
	}
	sort.Float64s(vals)
	n := len(vals)
	p50 := vals[n/2]
	if n%2 == 0 {
		p50 = (vals[n/2-1] + vals[n/2]) / 2
	}
	d.Min, d.P50, d.Max = &vals[0], &p50, &vals[n-1]
	d.MinDisplay, d.P50Display, d.MaxDisplay = formatWithUnit(vals[0], d.Unit), formatWithUnit(p50, d.Unit), formatWithUnit(vals[n-1], d.Unit)
	return d
}

func bandExposure(band l8behaviour.FollowingBand, b, k int, rows []Encounter, sources []l8behaviour.Encounter, idx []int) (BandExposure, error) {
	be := BandExposure{
		Seconds: band.Seconds, Display: formatWithUnit(band.Seconds, "s"),
		Duration: band.Duration, Rate: band.Rate,
	}
	excluded := map[l8behaviour.SuppressionReason]int{}
	for _, i := range idx {
		v := rows[i].Measurements[k]
		if v.Suppressed {
			excluded[v.Reason]++
			continue
		}
		acc := sources[i].Accounting
		// The row's value is the accounting sum it was built from; pooling
		// the integer sum keeps the rate exact, and this check keeps the two
		// the same quantity.
		if *v.Value != float64(acc.BandNanos[b])/1e9 {
			return BandExposure{}, fmt.Errorf("encounter %s %s is %v but accounts %d ns", rows[i].ID, band.Duration, *v.Value, acc.BandNanos[b])
		}
		be.Encounters++
		be.BelowNanos += acc.BandNanos[b]
		be.ValidNanos += acc.ValidNanos
	}
	be.Excluded = reasonCounts(excluded)
	be.BelowDisplay, be.ValidDisplay = formatNanos(be.BelowNanos), formatNanos(be.ValidNanos)
	if be.Encounters == 0 || be.ValidNanos == 0 {
		be.RateReason = l8behaviour.ReasonInsufficientObservation
		be.RateDisplay = suppressedDisplay(be.RateReason)
		return be, nil
	}
	rate := float64(be.BelowNanos) / float64(be.ValidNanos)
	be.RateValue = &rate
	be.RateDisplay = formatNumber(rate)
	return be, nil
}

// exposureReason is why an encounter's band exposure is suppressed, the
// first band's reason, or unspecified when every band is supported.
func exposureReason(r Encounter, metricIndex map[l8behaviour.MetricID]int) l8behaviour.SuppressionReason {
	for _, band := range l8behaviour.FollowingBands() {
		if v := r.Measurements[metricIndex[band.Duration]]; v.Suppressed {
			return v.Reason
		}
	}
	return l8behaviour.ReasonUnspecified
}

// histogramBin places a net time gap in its half-open bin; the last index is
// the overflow bin.
func histogramBin(seconds float64) int {
	finiteBins := HistogramMaxMillis / HistogramBinMillis
	edge := func(k int) float64 { return float64(k*HistogramBinMillis) / 1000 }
	k := int(math.Floor(seconds * 1000 / HistogramBinMillis))
	k = min(max(k, 0), finiteBins)
	for k > 0 && seconds < edge(k) {
		k--
	}
	for k < finiteBins && seconds >= edge(k+1) {
		k++
	}
	return k
}

func histogram(rows []Encounter, sources []l8behaviour.Encounter, idx []int, metricIndex map[l8behaviour.MetricID]int) (Histogram, error) {
	finiteBins := HistogramMaxMillis / HistogramBinMillis
	nanos := make([]int64, finiteBins+1)
	excluded := map[l8behaviour.SuppressionReason]int64{}
	h := Histogram{
		Metric: l8behaviour.MetricFollowingNetTimeGap, Unit: "s",
		BinMillis: HistogramBinMillis, MaxMillis: HistogramMaxMillis,
	}
	for _, i := range idx {
		src := sources[i]
		h.DenominatorNanos += rows[i].Accounting.AccountedNanos
		for _, t := range src.Accounting.Suppressions {
			excluded[t.Reason] += t.Nanos
		}
		if r := exposureReason(rows[i], metricIndex); r != l8behaviour.ReasonUnspecified {
			excluded[r] += src.Accounting.ValidNanos
			continue
		}
		h.Encounters++
		for _, inst := range src.Instants {
			if !inst.Valid {
				continue
			}
			if inst.Point == nil || inst.Point.TimeGap == nil || inst.Point.TimeGap.ValueS == nil {
				return Histogram{}, fmt.Errorf("encounter %s has a valid instant without a net time gap", rows[i].ID)
			}
			nanos[histogramBin(*inst.Point.TimeGap.ValueS)] += counted(inst)
		}
	}

	var sum int64
	share := func(n int64) float64 {
		if h.DenominatorNanos == 0 {
			return 0
		}
		return float64(n) / float64(h.DenominatorNanos)
	}
	for k, n := range nanos {
		bin := HistogramBin{LowerMillis: k * HistogramBinMillis, Nanos: n, Share: share(n)}
		if k < finiteBins {
			upper := (k + 1) * HistogramBinMillis
			bin.UpperMillis = &upper
			bin.Label = "[" + formatMillis(bin.LowerMillis) + ", " + formatMillis(upper) + ") s"
		} else {
			bin.Label = formatMillis(bin.LowerMillis) + " s and above"
		}
		bin.ShareDisplay = formatShare(bin.Share)
		h.Bins = append(h.Bins, bin)
		sum += n
	}
	h.Excluded = []ExcludedTime{}
	for _, r := range l8behaviour.SuppressionReasons() {
		if n := excluded[r]; n > 0 {
			h.Excluded = append(h.Excluded, ExcludedTime{
				Reason: r, Nanos: n, Display: formatNanos(n), Share: share(n), ShareDisplay: formatShare(share(n)),
			})
			sum += n
		}
	}
	if sum != h.DenominatorNanos {
		return Histogram{}, fmt.Errorf("histogram accounts %d ns of %d ns", sum, h.DenominatorNanos)
	}
	h.DenominatorDisplay = formatNanos(h.DenominatorNanos)
	return h, nil
}
