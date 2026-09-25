package l8behaviour

// Result contracts, per Sections 7.4, 9.3, 9.4 and 10.2 of
// docs/plans/lidar-behaviour-analytics-plan.md.
//
// Two rules shape every type here. A numeric result is either a meaningful
// value or a suppression with a closed-vocabulary reason, never a zero that
// could be read as a measurement. And an optional quantity is absent when
// unknown rather than zero, which is why optional numbers are pointers: a
// lower bound of zero and "no lower bound" are different statements.

import (
	"fmt"
	"math"
	"regexp"
)

// Uncertainty is a metric's uncertainty in the representation that suits its
// distribution (Section 9.3). Which fields are required depends on Kind:
//
//   - none: nothing else may be set;
//   - sigma: Sigma is required;
//   - interval: Lower, Upper and Coverage are required;
//   - bounds: at least one of Lower and Upper.
//
// Method is required for every kind but none, and a Monte Carlo propagation
// records its sample count so the interval's own precision is known.
type Uncertainty struct {
	Kind     UncertaintyKind    `json:"kind"`
	Sigma    *float64           `json:"sigma,omitempty"`
	Lower    *float64           `json:"lower,omitempty"`
	Upper    *float64           `json:"upper,omitempty"`
	Coverage *float64           `json:"coverage,omitempty"`
	Method   PropagationMethod  `json:"method,omitempty"`
	Samples  int                `json:"samples,omitempty"`
	Scopes   *UncertaintyScopes `json:"scopes,omitempty"`
}

// UncertaintyScopes carries a sigma's contributions separately by source, per
// Section 9.4: sensor pose largely cancels in a separation and does not cancel
// in a time difference, so a total sigma alone makes that cancellation a guess.
// An absent scope was not assessed, which is not the same as zero.
type UncertaintyScopes struct {
	// TrackLocal is per-track estimation and extent uncertainty.
	TrackLocal *float64 `json:"track_local,omitempty"`
	// SharedSensor is sensor pose, site calibration and timestamp alignment.
	SharedSensor *float64 `json:"shared_sensor,omitempty"`
	// SharedGeometry is road-surface, path and map geometry.
	SharedGeometry *float64 `json:"shared_geometry,omitempty"`
}

// Combined returns the total one-sigma of the assessed scopes, summing their
// variances. That is the first implementation Section 9.4 permits: common-mode
// cancellation between two parties' shared terms is not applied yet, which
// overstates a separation's uncertainty rather than understating it.
func (s UncertaintyScopes) Combined() float64 {
	var v float64
	for _, p := range []*float64{s.TrackLocal, s.SharedSensor, s.SharedGeometry} {
		if p != nil {
			v += *p * *p
		}
	}
	return math.Sqrt(v)
}

func (s UncertaintyScopes) validate() error {
	assessed := 0
	for _, f := range []namedValue{
		{"track_local", s.TrackLocal}, {"shared_sensor", s.SharedSensor}, {"shared_geometry", s.SharedGeometry},
	} {
		if f.value == nil {
			continue
		}
		assessed++
		if !finiteNonNegative(*f.value) {
			return fmt.Errorf("scope %s must be finite and non-negative", f.name)
		}
	}
	if assessed == 0 {
		return fmt.Errorf("scopes present but none assessed; omit them instead")
	}
	return nil
}

// SigmaUncertainty is a symmetric one-sigma whose whole contribution is
// track-local, the only scope the first implementation assesses.
func SigmaUncertainty(sigma float64, method PropagationMethod) Uncertainty {
	return Uncertainty{
		Kind: UncertaintySigma, Sigma: ptr(sigma), Method: method,
		Scopes: &UncertaintyScopes{TrackLocal: ptr(sigma)},
	}
}

// NoUncertainty states explicitly that no uncertainty is available. It is a
// declaration, so a reader can tell it from an uncertainty nobody computed.
func NoUncertainty() Uncertainty { return Uncertainty{Kind: UncertaintyNone} }

// scopeTolerance is the relative agreement required between a sigma and the
// quadrature sum of its scopes. It absorbs rounding only.
const scopeTolerance = 1e-9

// Validate enforces the per-kind field rules.
func (u Uncertainty) Validate() error {
	if !u.Kind.Valid() {
		return fmt.Errorf("uncertainty kind is %s", u.Kind)
	}
	if u.Kind == UncertaintyNone {
		if u.Sigma != nil || u.Lower != nil || u.Upper != nil || u.Coverage != nil ||
			u.Method != MethodUnspecified || u.Samples != 0 || u.Scopes != nil {
			return fmt.Errorf("uncertainty kind none carries no other fields")
		}
		return nil
	}
	if !u.Method.Valid() {
		return fmt.Errorf("uncertainty kind %s requires a propagation method", u.Kind)
	}
	if u.Samples < 0 {
		return fmt.Errorf("uncertainty samples must not be negative")
	}
	if u.Method == MethodMonteCarlo && u.Samples == 0 {
		return fmt.Errorf("monte_carlo propagation must record its sample count")
	}
	if (u.Method == MethodAnalytic || u.Method == MethodLinearised) && u.Samples != 0 {
		return fmt.Errorf("%s propagation draws no samples", u.Method)
	}
	for _, f := range []namedValue{{"sigma", u.Sigma}, {"lower", u.Lower}, {"upper", u.Upper}, {"coverage", u.Coverage}} {
		if f.value != nil && (math.IsNaN(*f.value) || math.IsInf(*f.value, 0)) {
			return fmt.Errorf("uncertainty %s must be finite", f.name)
		}
	}
	if u.Lower != nil && u.Upper != nil && *u.Lower > *u.Upper {
		return fmt.Errorf("uncertainty lower %.6g exceeds upper %.6g", *u.Lower, *u.Upper)
	}
	switch u.Kind {
	case UncertaintySigma:
		if u.Sigma == nil || *u.Sigma < 0 {
			return fmt.Errorf("sigma uncertainty requires a non-negative sigma")
		}
		if u.Lower != nil || u.Upper != nil || u.Coverage != nil {
			return fmt.Errorf("sigma uncertainty carries no interval or bounds")
		}
		if u.Scopes != nil {
			if err := u.Scopes.validate(); err != nil {
				return err
			}
			combined := u.Scopes.Combined()
			if math.Abs(combined-*u.Sigma) > scopeTolerance*math.Max(1, *u.Sigma) {
				return fmt.Errorf("sigma %.9g disagrees with its scopes' combined %.9g", *u.Sigma, combined)
			}
		}
	case UncertaintyInterval:
		if u.Lower == nil || u.Upper == nil || u.Coverage == nil {
			return fmt.Errorf("interval uncertainty requires lower, upper and coverage")
		}
		if *u.Coverage <= 0 || *u.Coverage >= 1 {
			return fmt.Errorf("interval coverage must be in (0, 1)")
		}
		if u.Sigma != nil || u.Scopes != nil {
			return fmt.Errorf("interval uncertainty carries no sigma or scopes")
		}
	case UncertaintyBounds:
		if u.Lower == nil && u.Upper == nil {
			return fmt.Errorf("bounds uncertainty requires a lower or an upper bound")
		}
		if u.Sigma != nil || u.Coverage != nil || u.Scopes != nil {
			return fmt.Errorf("bounds uncertainty carries no sigma, coverage or scopes")
		}
	}
	return nil
}

// Benchmark is what a value is compared against, with the provenance its kind
// needs (Section 10.2): a legal benchmark needs a jurisdiction and an effective
// date, a research threshold or external distribution a citation, a local
// distribution its stratification.
type Benchmark struct {
	Kind                   BenchmarkKind `json:"kind"`
	Threshold              *float64      `json:"threshold,omitempty"`
	Citation               string        `json:"citation,omitempty"`
	Jurisdiction           string        `json:"jurisdiction,omitempty"`
	EffectiveFromUnixNanos int64         `json:"effective_from_unix_nanos,omitempty"`
	Stratification         string        `json:"stratification,omitempty"`
}

// Validate enforces the per-kind provenance rules.
func (b Benchmark) Validate() error {
	if !b.Kind.Valid() {
		return fmt.Errorf("benchmark kind is %s", b.Kind)
	}
	if b.Threshold != nil && (math.IsNaN(*b.Threshold) || math.IsInf(*b.Threshold, 0)) {
		return fmt.Errorf("benchmark threshold must be finite")
	}
	switch b.Kind {
	case BenchmarkLegal:
		if b.Jurisdiction == "" || b.EffectiveFromUnixNanos == 0 {
			return fmt.Errorf("legal benchmark requires a jurisdiction and an effective date")
		}
	case BenchmarkResearchThreshold, BenchmarkExternalDistribution:
		if b.Citation == "" {
			return fmt.Errorf("%s benchmark requires a citation", b.Kind)
		}
	case BenchmarkLocalDistribution:
		if b.Stratification == "" {
			return fmt.Errorf("local_distribution benchmark requires its stratification")
		}
	}
	return nil
}

// VersionProvenance names the four independent versions a result depends on
// (Section 10.2): estimator, observation model, behaviour method and geometry.
// A change to any of them invalidates the derived rows rather than silently
// mixing versions.
type VersionProvenance struct {
	EstimateStage EstimateStage `json:"estimate_stage"`
	EstimatorID   string        `json:"estimator_id"`
	ObsModelID    string        `json:"obs_model_id"`
	MethodID      string        `json:"method_id"`
	// GeometryID names the path or road geometry used; empty when the metric
	// uses none.
	GeometryID string `json:"geometry_id,omitempty"`
	ParamHash  string `json:"param_hash"`
}

// Validate requires every version axis a result always has.
func (v VersionProvenance) Validate() error {
	if !v.EstimateStage.Valid() {
		return fmt.Errorf("provenance estimate stage is %s", v.EstimateStage)
	}
	if v.EstimatorID == "" || v.ObsModelID == "" || v.MethodID == "" || v.ParamHash == "" {
		return fmt.Errorf("provenance requires estimator, observation model, method and parameter hash")
	}
	return nil
}

// InputProvenance records exactly which evidence contributed (Section 10.2).
// For a pointwise pair result the frame counts are contributing samples, one
// per track at that instant.
type InputProvenance struct {
	ContributingTrackIDs []string `json:"contributing_track_ids"`
	FirstUnixNanos       int64    `json:"first_unix_nanos"`
	LastUnixNanos        int64    `json:"last_unix_nanos"`
	ObservedFrames       int      `json:"observed_frames"`
	CoastedFrames        int      `json:"coasted_frames"`
	// PlanarFallback is true when the result was computed in the planar
	// sensor frame without a road-surface model. It is true for every result
	// until a surface model exists, and saying so is the point.
	PlanarFallback bool `json:"planar_fallback"`
}

// Validate requires at least one distinct contributing track and an ordered
// interval.
func (p InputProvenance) Validate() error {
	if len(p.ContributingTrackIDs) == 0 {
		return fmt.Errorf("input provenance requires at least one contributing track")
	}
	seen := make(map[string]bool, len(p.ContributingTrackIDs))
	for _, id := range p.ContributingTrackIDs {
		if id == "" {
			return fmt.Errorf("input provenance has an empty track id")
		}
		if seen[id] {
			return fmt.Errorf("input provenance lists track %s twice", id)
		}
		seen[id] = true
	}
	if p.FirstUnixNanos > p.LastUnixNanos {
		return fmt.Errorf("input provenance interval is reversed")
	}
	if p.ObservedFrames < 0 || p.CoastedFrames < 0 {
		return fmt.Errorf("input provenance frame counts must not be negative")
	}
	return nil
}

// Provenance is the explainability record of Section 7.6: which versions and
// which evidence produced a result.
type Provenance struct {
	Version VersionProvenance `json:"version"`
	Input   InputProvenance   `json:"input"`
}

// Validate checks both halves.
func (p Provenance) Validate() error {
	if err := p.Version.Validate(); err != nil {
		return err
	}
	return p.Input.Validate()
}

// Measurement is a numeric behaviour result (BehaviourMeasurement in Section
// 7.4). It carries a value XOR a suppression: Value set, Suppressed false and
// no Reason; or Value absent, Suppressed true and a registered Reason. The
// constructors cannot build anything else, and Validate rejects anything else
// that arrives by decoding.
type Measurement struct {
	Name        MetricID          `json:"name"`
	Unit        string            `json:"unit"`
	Value       *float64          `json:"value,omitempty"`
	Uncertainty *Uncertainty      `json:"uncertainty,omitempty"`
	Suppressed  bool              `json:"suppressed"`
	Reason      SuppressionReason `json:"reason,omitempty"`
	Benchmark   *Benchmark        `json:"benchmark,omitempty"`
	// Percentile is the value's position in the benchmark's population, in
	// [0, 100]; absent until a comparison population exists.
	Percentile *float64 `json:"percentile,omitempty"`
	// OpportunitySeconds is a rate's denominator: observed, supported time
	// only (Section 6).
	OpportunitySeconds *float64   `json:"opportunity_seconds,omitempty"`
	Provenance         Provenance `json:"provenance"`
}

// NewMeasurement builds a supported measurement under a registered metric.
// A benchmark with no further provenance needed (no_established_threshold) is
// attached from the definition; the others need context a pointwise caller
// does not have and are attached by whoever holds it.
func NewMeasurement(id MetricID, value float64, u Uncertainty, prov Provenance) (Measurement, error) {
	def, ok := LookupMetric(id)
	if !ok {
		return Measurement{}, fmt.Errorf("measurement name %q is not a registered metric", id)
	}
	m := Measurement{
		Name: id, Unit: def.Unit, Value: ptr(value), Uncertainty: &u,
		Benchmark: definitionBenchmark(def), Provenance: prov,
	}
	return m, m.Validate()
}

// NewSuppressedMeasurement builds a suppression under a registered metric.
func NewSuppressedMeasurement(id MetricID, reason SuppressionReason, prov Provenance) (Measurement, error) {
	def, ok := LookupMetric(id)
	if !ok {
		return Measurement{}, fmt.Errorf("measurement name %q is not a registered metric", id)
	}
	m := Measurement{
		Name: id, Unit: def.Unit, Suppressed: true, Reason: reason,
		Benchmark: definitionBenchmark(def), Provenance: prov,
	}
	return m, m.Validate()
}

func definitionBenchmark(def MetricDefinition) *Benchmark {
	if def.Benchmark != BenchmarkNoEstablishedThreshold {
		return nil
	}
	b := &Benchmark{Kind: BenchmarkNoEstablishedThreshold}
	if def.BandSeconds > 0 {
		b.Threshold = ptr(def.BandSeconds)
	}
	return b
}

// Validate enforces the value-XOR-suppression rule and the field contracts.
func (m Measurement) Validate() error {
	def, ok := LookupMetric(m.Name)
	if !ok {
		return fmt.Errorf("measurement name %q is not a registered metric", m.Name)
	}
	if m.Unit != def.Unit {
		return fmt.Errorf("measurement %s: unit %q, registered unit %q", m.Name, m.Unit, def.Unit)
	}
	if m.Suppressed {
		if m.Value != nil {
			return fmt.Errorf("measurement %s is suppressed and must not carry a value", m.Name)
		}
		if !m.Reason.Valid() {
			return fmt.Errorf("measurement %s is suppressed without a registered reason", m.Name)
		}
		if m.Uncertainty != nil || m.Percentile != nil {
			return fmt.Errorf("measurement %s is suppressed and must not carry uncertainty or a percentile", m.Name)
		}
	} else {
		if m.Value == nil {
			return fmt.Errorf("measurement %s has neither a value nor a suppression", m.Name)
		}
		if m.Reason != ReasonUnspecified {
			return fmt.Errorf("measurement %s carries a value and a suppression reason", m.Name)
		}
		if math.IsNaN(*m.Value) || math.IsInf(*m.Value, 0) {
			return fmt.Errorf("measurement %s value must be finite", m.Name)
		}
		if m.Uncertainty == nil {
			return fmt.Errorf("measurement %s must state its uncertainty, even as kind none", m.Name)
		}
		if err := m.Uncertainty.Validate(); err != nil {
			return fmt.Errorf("measurement %s: %w", m.Name, err)
		}
	}
	if m.Benchmark != nil {
		if err := m.Benchmark.Validate(); err != nil {
			return fmt.Errorf("measurement %s: %w", m.Name, err)
		}
	}
	if m.Percentile != nil && !(*m.Percentile >= 0 && *m.Percentile <= 100) {
		return fmt.Errorf("measurement %s percentile must be in [0, 100]", m.Name)
	}
	if m.OpportunitySeconds != nil && !finiteNonNegative(*m.OpportunitySeconds) {
		return fmt.Errorf("measurement %s opportunity must be finite and non-negative", m.Name)
	}
	if err := m.Provenance.Validate(); err != nil {
		return fmt.Errorf("measurement %s: %w", m.Name, err)
	}
	return nil
}

// MeasurementRef points at the evidence an outcome was derived from: a
// registered measurement over a stated interval.
type MeasurementRef struct {
	Name           MetricID `json:"name"`
	FirstUnixNanos int64    `json:"first_unix_nanos"`
	LastUnixNanos  int64    `json:"last_unix_nanos"`
}

// Outcome is a categorical behaviour result (BehaviourOutcome in Section 7.4).
// A categorical outcome never replaces its evidence: an unsuppressed outcome
// must reference the measurements it was derived from and name the threshold
// set that produced it. Confidence is category confidence, not measurement
// uncertainty.
type Outcome struct {
	Name         string            `json:"name"`
	Value        string            `json:"value,omitempty"`
	Confidence   *float64          `json:"confidence,omitempty"`
	DerivedFrom  []MeasurementRef  `json:"derived_from,omitempty"`
	ThresholdSet string            `json:"threshold_set,omitempty"`
	Suppressed   bool              `json:"suppressed"`
	Reason       SuppressionReason `json:"reason,omitempty"`
	Provenance   Provenance        `json:"provenance"`
}

var snakeCase = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Validate enforces value-XOR-suppression and evidence retention.
func (o Outcome) Validate() error {
	if !snakeCase.MatchString(o.Name) {
		return fmt.Errorf("outcome name %q must be snake_case", o.Name)
	}
	if o.Suppressed {
		if o.Value != "" || o.Confidence != nil {
			return fmt.Errorf("outcome %s is suppressed and must not carry a value or confidence", o.Name)
		}
		if !o.Reason.Valid() {
			return fmt.Errorf("outcome %s is suppressed without a registered reason", o.Name)
		}
	} else {
		if !snakeCase.MatchString(o.Value) {
			return fmt.Errorf("outcome %s value %q must be a snake_case label", o.Name, o.Value)
		}
		if o.Reason != ReasonUnspecified {
			return fmt.Errorf("outcome %s carries a value and a suppression reason", o.Name)
		}
		if len(o.DerivedFrom) == 0 {
			return fmt.Errorf("outcome %s must reference the measurements it was derived from", o.Name)
		}
		if o.ThresholdSet == "" {
			return fmt.Errorf("outcome %s must name the threshold set that produced it", o.Name)
		}
	}
	for _, ref := range o.DerivedFrom {
		if _, ok := LookupMetric(ref.Name); !ok {
			return fmt.Errorf("outcome %s derives from unregistered metric %q", o.Name, ref.Name)
		}
		if ref.FirstUnixNanos > ref.LastUnixNanos {
			return fmt.Errorf("outcome %s evidence interval for %s is reversed", o.Name, ref.Name)
		}
	}
	if o.Confidence != nil && !(*o.Confidence >= 0 && *o.Confidence <= 1) {
		return fmt.Errorf("outcome %s confidence must be in [0, 1]", o.Name)
	}
	if err := o.Provenance.Validate(); err != nil {
		return fmt.Errorf("outcome %s: %w", o.Name, err)
	}
	return nil
}

// namedValue pairs an optional field with its wire name, so validation walks
// fields in a fixed order and reports the same error every run.
type namedValue struct {
	name  string
	value *float64
}

func ptr[T any](v T) *T { return &v }

func finiteNonNegative(v float64) bool {
	return v >= 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
}
