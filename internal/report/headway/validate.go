package headway

import (
	"errors"
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

// Validate checks a report against its contract. Build always returns a
// valid report; the renderer calls Validate again so a report that arrives by
// decoding, or was edited after building, cannot print a label it has not
// earned or a number where a suppression belongs.
func (r Report) Validate() error {
	if r.Contract != ContractID {
		return fmt.Errorf("report contract is %q, want %q", r.Contract, ContractID)
	}
	if r.Status == StatusPromoted {
		return ErrPromotionGated
	}
	if !r.Status.Valid() {
		return fmt.Errorf("report status is %s", r.Status)
	}
	if r.StatusLabel != r.Status.Label() || r.StatusNote != r.Status.Note() {
		return errors.New("report status label and note must be the status's own")
	}
	if r.Title != Title {
		return fmt.Errorf("report title is %q, want %q", r.Title, Title)
	}
	for _, e := range r.Encounters {
		if err := e.validate(); err != nil {
			return fmt.Errorf("encounter %s: %w", e.ID, err)
		}
	}
	for _, a := range r.Aggregates {
		if err := a.validate(); err != nil {
			return fmt.Errorf("aggregate %s: %w", a.ID, err)
		}
	}
	return nil
}

func (e Encounter) validate() error {
	want := ValueBlockProvisional
	if e.Stage == l8behaviour.StageFinal {
		want = ValueBlockMeasurements
	}
	if !e.Stage.Valid() || e.ValueBlock != want {
		return fmt.Errorf("a %s encounter reads %s, not %s", e.Stage, want, e.ValueBlock)
	}
	ids := l8behaviour.EncounterMetrics()
	if len(e.Measurements) != len(ids) {
		return fmt.Errorf("%d measurements, want %d", len(e.Measurements), len(ids))
	}
	for i, v := range e.Measurements {
		if v.Metric != ids[i] {
			return fmt.Errorf("measurement %d is %s, want %s", i, v.Metric, ids[i])
		}
		if err := v.validate(); err != nil {
			return err
		}
	}
	for _, p := range e.Predicted {
		if p.Metric != l8behaviour.MetricFollowingPredictedGap || p.Visibility != l8behaviour.VisibilityReviewOnly {
			return errors.New("a predicted interval must be the review-only predicted gap")
		}
	}
	return nil
}

// validate holds a row to its registry definition and to the display rule:
// a value prints as itself, a suppression as its reason and nothing else.
func (v Value) validate() error {
	def, ok := l8behaviour.LookupMetric(v.Metric)
	if !ok {
		return fmt.Errorf("metric %q is not registered", v.Metric)
	}
	if v.Unit != def.Unit || v.Estimator != def.Estimator || v.Visibility != def.Visibility || v.Benchmark != def.Benchmark {
		return fmt.Errorf("%s disagrees with its registry row", v.Metric)
	}
	if v.Suppressed {
		if v.Value != nil || v.Uncertainty != nil || v.OpportunitySeconds != nil || !v.Reason.Valid() {
			return fmt.Errorf("%s is suppressed and must carry a reason and nothing else", v.Metric)
		}
		if v.Display != suppressedDisplay(v.Reason) || v.UncertaintyDisplay != "" || v.OpportunityDisplay != "" {
			return fmt.Errorf("%s is suppressed and must print as its reason", v.Metric)
		}
		return nil
	}
	if v.Value == nil || v.Reason != l8behaviour.ReasonUnspecified || v.Uncertainty == nil {
		return fmt.Errorf("%s must carry a value and its uncertainty, and no reason", v.Metric)
	}
	if err := v.Uncertainty.Validate(); err != nil {
		return fmt.Errorf("%s: %w", v.Metric, err)
	}
	if v.Display != formatWithUnit(*v.Value, v.Unit) || v.UncertaintyDisplay != formatUncertainty(v.Uncertainty, v.Unit) {
		return fmt.Errorf("%s does not print its own value", v.Metric)
	}
	return nil
}

func (a Aggregate) validate() error {
	for _, b := range a.Bands {
		if (b.RateValue == nil) == (b.RateReason == l8behaviour.ReasonUnspecified) {
			return fmt.Errorf("band %s must carry a rate or a reason, not both or neither", b.Display)
		}
		if b.RateValue == nil && b.RateDisplay != suppressedDisplay(b.RateReason) {
			return fmt.Errorf("band %s is suppressed and must print as its reason", b.Display)
		}
	}
	for _, d := range a.Metrics {
		if (d.Supported == 0) != (d.Min == nil) {
			return fmt.Errorf("distribution %s must have values exactly when it has supported encounters", d.Metric)
		}
	}
	var sum int64
	for _, b := range a.Histogram.Bins {
		sum += b.Nanos
	}
	for _, x := range a.Histogram.Excluded {
		if !x.Reason.Valid() {
			return errors.New("excluded time requires a reason")
		}
		sum += x.Nanos
	}
	if sum != a.Histogram.DenominatorNanos {
		return fmt.Errorf("histogram accounts %d ns of %d ns", sum, a.Histogram.DenominatorNanos)
	}
	return nil
}
