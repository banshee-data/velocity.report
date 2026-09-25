package l8behaviour

// Canonical metric identities for behaviour results, mirrored from
// docs/platform/architecture/metrics-registry.md.
//
// The registry document is the canonical definition; this table is the code
// that has to agree with it, and a test fails when a metric here is missing
// from the document or disagrees with it on unit or visibility. It exists in
// code because Measurement validation needs it: a value may only be stored
// under a registered name with its registered unit, which is what stops the
// same word drifting across storage, API and report (Section 10.4).
//
// Identifiers follow the registry's <level>.<leaf>_<unit> shape. Band
// thresholds are written in milliseconds in the leaf (below_1500ms) rather
// than as a decimal, because a "p" decimal marker would read as a percentile
// and a literal "." would collide with the level separator.

import (
	"fmt"
	"strings"
)

// MetricID is a canonical registry identifier.
type MetricID string

// Following-metric identifiers, Section 8.3. The pointwise pair is physical
// spatial gap and net time gap; the encounter-level trio is supported
// opportunity, time below each named band, and the rate over that opportunity.
// observed_surface_gap and the predicted gap are review-only and never enter a
// published distribution or exposure.
const (
	MetricFollowingSpatialGap         MetricID = "interaction.following_spatial_gap_m"
	MetricFollowingNetTimeGap         MetricID = "interaction.following_net_time_gap_s"
	MetricFollowingValidTime          MetricID = "interaction.following_valid_time_s"
	MetricFollowingTimeBelow2000ms    MetricID = "interaction.following_time_below_2000ms_s"
	MetricFollowingTimeBelow1500ms    MetricID = "interaction.following_time_below_1500ms_s"
	MetricFollowingTimeBelow1000ms    MetricID = "interaction.following_time_below_1000ms_s"
	MetricFollowingRateBelow2000ms    MetricID = "interaction.following_rate_below_2000ms_ratio"
	MetricFollowingRateBelow1500ms    MetricID = "interaction.following_rate_below_1500ms_ratio"
	MetricFollowingRateBelow1000ms    MetricID = "interaction.following_rate_below_1000ms_ratio"
	MetricFollowingObservedSurfaceGap MetricID = "interaction.observed_surface_gap_m"
	MetricFollowingPredictedGap       MetricID = "interaction.following_predicted_gap_m"
)

// Registry field tokens used by the following family. The registry document
// defines each; they are unexported because no caller should invent a row.
const (
	followingFamily        = "following"
	interactionLevel       = "interaction"
	estimatorInstantaneous = "instantaneous"
	estimatorDuration      = "duration"
	estimatorRatio         = "ratio"
	unitMetres             = "m"
	unitSeconds            = "s"
	unitRatio              = "ratio"
)

// MetricDefinition is one registry row, as far as code needs it. The document
// additionally carries source modes and tag policy, which no code reads yet.
type MetricDefinition struct {
	ID        MetricID
	Family    string
	Level     string
	Estimator string
	Unit      string
	// Visibility decides whether values may enter a published distribution.
	Visibility Visibility
	// Benchmark is the kind of comparison the metric declares; unspecified
	// means none applies.
	Benchmark BenchmarkKind
	// BandSeconds is the net-time-gap band a band metric reports against, and
	// zero for every other metric.
	BandSeconds float64
	// Aliases are the names the plan and earlier drafts used for the same
	// quantity. They are recorded so a later alias audit can find them; they
	// are never valid measurement names.
	Aliases []string
}

// Validate checks the definition's internal consistency against the registry
// naming rules.
func (d MetricDefinition) Validate() error {
	if !strings.HasPrefix(string(d.ID), d.Level+".") {
		return fmt.Errorf("metric %s: id must start with its level %q", d.ID, d.Level)
	}
	if !strings.HasSuffix(string(d.ID), "_"+d.Unit) {
		return fmt.Errorf("metric %s: id must end with its unit suffix %q", d.ID, d.Unit)
	}
	if d.Family == "" || d.Estimator == "" {
		return fmt.Errorf("metric %s: family and estimator are required", d.ID)
	}
	if !d.Visibility.Valid() {
		return fmt.Errorf("metric %s: visibility is %s", d.ID, d.Visibility)
	}
	if d.Benchmark != BenchmarkUnspecified && !d.Benchmark.Valid() {
		return fmt.Errorf("metric %s: benchmark is %s", d.ID, d.Benchmark)
	}
	if d.BandSeconds < 0 {
		return fmt.Errorf("metric %s: band must not be negative", d.ID)
	}
	if d.BandSeconds > 0 {
		band := fmt.Sprintf("_below_%dms_", int(d.BandSeconds*1000+0.5))
		if !strings.Contains(string(d.ID), band) {
			return fmt.Errorf("metric %s: band %.3f s must appear in the id as %q", d.ID, d.BandSeconds, band)
		}
	}
	return nil
}

func followingMetricTable() []MetricDefinition {
	band := func(seconds float64, duration, rate MetricID, pretty string) []MetricDefinition {
		return []MetricDefinition{
			{
				ID: duration, Family: followingFamily, Level: interactionLevel,
				Estimator: estimatorDuration, Unit: unitSeconds, Visibility: VisibilityPublic,
				Benchmark: BenchmarkNoEstablishedThreshold, BandSeconds: seconds,
				Aliases: []string{"thw_below_" + pretty + "_seconds"},
			},
			{
				ID: rate, Family: followingFamily, Level: interactionLevel,
				Estimator: estimatorRatio, Unit: unitRatio, Visibility: VisibilityPublic,
				Benchmark: BenchmarkLocalDistribution, BandSeconds: seconds,
				Aliases: []string{"thw_below_" + pretty + "_rate", "following_close_rate"},
			},
		}
	}
	defs := []MetricDefinition{
		{
			ID: MetricFollowingSpatialGap, Family: followingFamily, Level: interactionLevel,
			Estimator: estimatorInstantaneous, Unit: unitMetres, Visibility: VisibilityPublic,
			Benchmark: BenchmarkExternalDistribution,
			Aliases:   []string{"distance_headway", "gap"},
		},
		{
			ID: MetricFollowingNetTimeGap, Family: followingFamily, Level: interactionLevel,
			Estimator: estimatorInstantaneous, Unit: unitSeconds, Visibility: VisibilityPublic,
			Benchmark: BenchmarkNoEstablishedThreshold,
			Aliases:   []string{"time_headway", "thw"},
		},
		{
			ID: MetricFollowingValidTime, Family: followingFamily, Level: interactionLevel,
			Estimator: estimatorDuration, Unit: unitSeconds, Visibility: VisibilityPublic,
			Aliases: []string{"following_valid_time", "valid_following_time"},
		},
	}
	defs = append(defs, band(2.0, MetricFollowingTimeBelow2000ms, MetricFollowingRateBelow2000ms, "2.0")...)
	defs = append(defs, band(1.5, MetricFollowingTimeBelow1500ms, MetricFollowingRateBelow1500ms, "1.5")...)
	defs = append(defs, band(1.0, MetricFollowingTimeBelow1000ms, MetricFollowingRateBelow1000ms, "1.0")...)
	defs = append(defs,
		MetricDefinition{
			ID: MetricFollowingObservedSurfaceGap, Family: followingFamily, Level: interactionLevel,
			Estimator: estimatorInstantaneous, Unit: unitMetres, Visibility: VisibilityReviewOnly,
			Aliases: []string{"observed_surface_gap"},
		},
		MetricDefinition{
			ID: MetricFollowingPredictedGap, Family: followingFamily, Level: interactionLevel,
			Estimator: estimatorInstantaneous, Unit: unitMetres, Visibility: VisibilityReviewOnly,
			Aliases: []string{"predicted_gap"},
		},
	)
	return defs
}

// followingMetricDefs is the table, built once; metricIndex locates a row.
// Both are read-only after initialisation, and every exported accessor hands
// out copies, so a caller cannot edit the registry through a returned slice.
var (
	followingMetricDefs = followingMetricTable()
	metricIndex         = func() map[MetricID]int {
		index := make(map[MetricID]int, len(followingMetricDefs))
		for i, d := range followingMetricDefs {
			index[d.ID] = i
		}
		return index
	}()
)

// FollowingMetrics returns a fresh copy of the following-metric definitions.
func FollowingMetrics() []MetricDefinition {
	out := make([]MetricDefinition, len(followingMetricDefs))
	for i, d := range followingMetricDefs {
		out[i] = d.clone()
	}
	return out
}

// LookupMetric returns a copy of the registered definition for id.
func LookupMetric(id MetricID) (MetricDefinition, bool) {
	i, ok := metricIndex[id]
	if !ok {
		return MetricDefinition{}, false
	}
	return followingMetricDefs[i].clone(), true
}

func (d MetricDefinition) clone() MetricDefinition {
	d.Aliases = append([]string(nil), d.Aliases...)
	return d
}

// FollowingBand is one named net-time-gap reporting band: a descriptive bin,
// never a tailgating verdict or a safety standard (Sections 5 and 10.4). The
// band threshold itself has no established basis; the rate over supported
// opportunity is compared against a local distribution, per Section 8.3.
type FollowingBand struct {
	Seconds  float64
	Duration MetricID
	Rate     MetricID
}

// FollowingBands returns the named bands, widest first: 2.0, 1.5 and 1.0 s.
func FollowingBands() []FollowingBand {
	return []FollowingBand{
		{Seconds: 2.0, Duration: MetricFollowingTimeBelow2000ms, Rate: MetricFollowingRateBelow2000ms},
		{Seconds: 1.5, Duration: MetricFollowingTimeBelow1500ms, Rate: MetricFollowingRateBelow1500ms},
		{Seconds: 1.0, Duration: MetricFollowingTimeBelow1000ms, Rate: MetricFollowingRateBelow1000ms},
	}
}

// Contains reports whether a supported net time gap falls below the band.
// The comparison is strict, matching time(THW < X): a gap exactly at the band
// is not below it.
func (b FollowingBand) Contains(netTimeGapSeconds float64) bool {
	return netTimeGapSeconds < b.Seconds
}

// ClassRule is one row of the class-applicability table: which motion classes
// a metric is defined for. Counterpart is nil for a single-track metric and
// names the other party's allowed classes for a pairwise one.
type ClassRule struct {
	Subject     []MotionClass
	Counterpart []MotionClass
}

// classApplicability is Section 7.1's tier rule made explicit. Following
// metrics are rigid-vehicle metrics for both parties in the first increment:
// Section 10.4 limits the first report to rigid-vehicle pairs, and a cyclist,
// pedestrian or unknown label is suppressed rather than quietly treated as a
// car. Universal trajectory primitives are added here when their metric ids
// are registered.
func classApplicability(id MetricID) (ClassRule, bool) {
	if d, ok := LookupMetric(id); ok && d.Family == followingFamily {
		return ClassRule{
			Subject:     []MotionClass{MotionRigidVehicle},
			Counterpart: []MotionClass{MotionRigidVehicle},
		}, true
	}
	return ClassRule{}, false
}

// ClassApplicability decides whether a metric is defined for the given motion
// classes. For a pairwise metric the subject is the measured party (the
// follower) and the counterpart the other (the leader); for a single-track
// metric the counterpart must be unspecified.
//
// It returns ReasonClassNotSupported rather than false when the metric does
// not apply, so the caller stores why. An unregistered metric or an
// unspecified class is a caller error, not a suppression.
func ClassApplicability(id MetricID, subject, counterpart MotionClass) (SuppressionReason, error) {
	rule, ok := classApplicability(id)
	if !ok {
		return ReasonUnspecified, fmt.Errorf("no class-applicability rule for metric %q", id)
	}
	reason, err := rule.Check(subject, counterpart)
	if err != nil {
		return ReasonUnspecified, fmt.Errorf("metric %s: %w", id, err)
	}
	return reason, nil
}

// Check applies one rule: ReasonClassNotSupported when a class is outside
// it, an error when a class is unspecified or a counterpart is supplied to a
// single-track rule.
func (rule ClassRule) Check(subject, counterpart MotionClass) (SuppressionReason, error) {
	if !subject.Valid() {
		return ReasonUnspecified, fmt.Errorf("subject class is %s", subject)
	}
	if rule.Counterpart == nil {
		if counterpart != MotionClassUnspecified {
			return ReasonUnspecified, fmt.Errorf("single-track rule; counterpart class must be unspecified, got %s", counterpart)
		}
	} else if !counterpart.Valid() {
		return ReasonUnspecified, fmt.Errorf("counterpart class is %s", counterpart)
	}
	if !containsClass(rule.Subject, subject) {
		return ReasonClassNotSupported, nil
	}
	if rule.Counterpart != nil && !containsClass(rule.Counterpart, counterpart) {
		return ReasonClassNotSupported, nil
	}
	return ReasonUnspecified, nil
}

func containsClass(classes []MotionClass, c MotionClass) bool {
	for _, x := range classes {
		if x == c {
			return true
		}
	}
	return false
}
