package headway

// The headway report contract, headway_report_v1. It is the report's
// data.json and the whole input of its charts and Typst template, so a
// reviewer holding the source ZIP holds everything the PDF shows.
//
// Conventions shared by every type here:
//
//   - Times are int64 nanoseconds. A *_nanos field inside an encounter is an
//     offset from the encounter's first instant; *_unix_nanos is absolute.
//     Absolute times are also given as UTC ISO 8601 with a trailing Z.
//   - A number that may be suppressed is a *float64 marshalled without
//     omitempty, so a suppression reads "value": null next to its reason,
//     never a zero.
//   - Every *_display string is the exact text the PDF prints. The template
//     formats nothing itself, so the golden data.json shows the report's
//     words and numbers as a reader sees them.

import "github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"

// ContractID versions this contract: its shape, the value-block rule, the
// aggregation rules and the display formats. Change it when any changes.
const ContractID = "headway_report_v1"

// Title is the report's name, from the plan's naming rule (Section 8.3): an
// observed following exposure for encounters at one site.
const Title = "Observed following exposure"

// Histogram bins, in milliseconds so the edges are exact. Every band
// threshold must fall on an edge, which Build checks, so the share of valid
// time left of a band's rule is exactly the pooled rate below it.
const (
	HistogramBinMillis = 250
	HistogramMaxMillis = 3000
)

// ValueBlock names which of an l8behaviour encounter's measurement lists a
// row's values were read from, using the encounter's own JSON field names.
type ValueBlock string

const (
	// ValueBlockMeasurements is the production list of a final encounter.
	ValueBlockMeasurements ValueBlock = "measurements"
	// ValueBlockProvisional is the review-labelled list of an encounter over
	// non-final estimates; its production list is suppressed with
	// estimate_not_final.
	ValueBlockProvisional ValueBlock = "provisional"
)

// Report is the whole headway report.
type Report struct {
	Contract    string `json:"contract"`
	Status      Status `json:"status"`
	StatusLabel string `json:"status_label"`
	StatusNote  string `json:"status_note"`
	Title       string `json:"title"`
	// Paper is the Typst paper name; set by the renderer.
	Paper string `json:"paper"`
	// Statements are the report's fixed scope statements, printed verbatim.
	Statements []string    `json:"statements"`
	Methods    Methods     `json:"methods"`
	Bands      []Band      `json:"bands"`
	Captures   []Capture   `json:"captures"`
	Encounters []Encounter `json:"encounters"`
	Aggregates []Aggregate `json:"aggregates"`
}

// Methods are the versions of every part of the following method.
type Methods struct {
	Encounter string `json:"encounter"`
	LocalPath string `json:"local_path"`
	Pairing   string `json:"pairing"`
	Sync      string `json:"sync"`
	Pointwise string `json:"pointwise"`
	Exposure  string `json:"exposure"`
}

// Band is one named net-time-gap band and the registry ids reported
// against it.
type Band struct {
	Seconds           float64                   `json:"seconds"`
	Display           string                    `json:"display"`
	Duration          l8behaviour.MetricID      `json:"duration_metric"`
	DurationBenchmark l8behaviour.BenchmarkKind `json:"duration_benchmark"`
	Rate              l8behaviour.MetricID      `json:"rate_metric"`
	RateBenchmark     l8behaviour.BenchmarkKind `json:"rate_benchmark"`
}

// Capture is one analysed capture: its source, the analysis version and
// parameters, and every track's time as a follower.
type Capture struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	// Source locates the capture's trajectories; a track's locator is the
	// source followed by its track id.
	Source         string                              `json:"source"`
	Estimate       l8behaviour.EstimateIdentity        `json:"estimate"`
	MethodID       string                              `json:"method_id"`
	ParamsHash     string                              `json:"params_hash"`
	Params         l8behaviour.FollowingAnalysisParams `json:"params"`
	FirstUnixNanos int64                               `json:"first_unix_nanos"`
	LastUnixNanos  int64                               `json:"last_unix_nanos"`
	FirstUTC       string                              `json:"first_utc"`
	LastUTC        string                              `json:"last_utc"`
	Followers      []Follower                          `json:"followers"`
}

// Follower is one track's passage taken as a follower: whether its own path
// could be fitted, and where its time went. Free flow is time with no body
// ahead, which is neither following nor a suppression.
type Follower struct {
	TrackID    string `json:"track_id"`
	GeometryID string `json:"geometry_id,omitempty"`
	// PathConditions is why the follower's path was refused; empty when it
	// was fitted.
	PathConditions   []l8behaviour.PathCondition `json:"path_conditions,omitempty"`
	Instants         int                         `json:"instants"`
	LeaderNanos      int64                       `json:"leader_nanos"`
	LeaderDisplay    string                      `json:"leader_display"`
	FreeFlowNanos    int64                       `json:"free_flow_nanos"`
	FreeFlowDisplay  string                      `json:"free_flow_display"`
	Suppressed       []ReasonTime                `json:"suppressed"`
	RecordGapNanos   int64                       `json:"record_gap_nanos"`
	RecordGapDisplay string                      `json:"record_gap_display"`
}

// ReasonTime is time, and the instants behind it, under one reason (and,
// for no_common_path, its path condition).
type ReasonTime struct {
	Reason    l8behaviour.SuppressionReason `json:"reason"`
	Condition l8behaviour.PathCondition     `json:"condition,omitempty"`
	Instants  int                           `json:"instants"`
	Nanos     int64                         `json:"nanos"`
	Display   string                        `json:"display"`
}

// Encounter is one leader/follower encounter row.
type Encounter struct {
	ID        string `json:"id"`
	CaptureID string `json:"capture_id"`
	Leader    Track  `json:"leader"`
	Follower  Track  `json:"follower"`
	Path      Path   `json:"path"`

	FirstUnixNanos  int64  `json:"first_unix_nanos"`
	LastUnixNanos   int64  `json:"last_unix_nanos"`
	FirstUTC        string `json:"first_utc"`
	LastUTC         string `json:"last_utc"`
	DurationNanos   int64  `json:"duration_nanos"`
	DurationDisplay string `json:"duration_display"`

	Stage      l8behaviour.EstimateStage `json:"estimate_stage"`
	ValueBlock ValueBlock                `json:"value_block"`
	// Version and Input are the provenance every measurement of the
	// encounter carries.
	Version      l8behaviour.VersionProvenance `json:"version"`
	Input        l8behaviour.InputProvenance   `json:"input"`
	Measurements []Value                       `json:"measurements"`
	Accounting   Accounting                    `json:"accounting"`
	Endpoints    Endpoints                     `json:"endpoints"`
	// Unsupported is every run of instants that is not valid following time.
	Unsupported []Interval `json:"unsupported"`
	// Predicted is every run of review-only predicted gaps. Nothing in an
	// aggregate reads it.
	Predicted []PredictedInterval `json:"predicted"`
	Series    Series              `json:"series"`
	// Chart is the root-relative path of the encounter's chart; set by the
	// renderer.
	Chart string `json:"chart"`
}

// Track identifies one party and how to find its trajectory.
type Track struct {
	TrackID     string                       `json:"track_id"`
	Locator     string                       `json:"locator"`
	SiteID      string                       `json:"site_id,omitempty"`
	SensorID    string                       `json:"sensor_id"`
	MotionClass l8behaviour.MotionClass      `json:"motion_class"`
	ClassLabel  string                       `json:"class_label,omitempty"`
	Estimate    l8behaviour.EstimateIdentity `json:"estimate"`
	Samples     int                          `json:"samples"`
	FirstUTC    string                       `json:"first_utc"`
	LastUTC     string                       `json:"last_utc"`
}

// Path is the follower's directed local path the pair was measured along.
type Path struct {
	GeometryID     string                    `json:"geometry_id"`
	MethodID       string                    `json:"method_id"`
	Stage          l8behaviour.EstimateStage `json:"estimate_stage"`
	LengthM        float64                   `json:"length_m"`
	LengthDisplay  string                    `json:"length_display"`
	Knots          int                       `json:"knots"`
	BridgedKnots   int                       `json:"bridged_knots"`
	MemberTrackIDs []string                  `json:"member_track_ids"`
}

// Value is one registered measurement as the report shows it.
type Value struct {
	Metric     l8behaviour.MetricID      `json:"metric"`
	Unit       string                    `json:"unit"`
	Estimator  string                    `json:"estimator"`
	Visibility l8behaviour.Visibility    `json:"visibility"`
	Benchmark  l8behaviour.BenchmarkKind `json:"benchmark,omitempty"`
	// Value is null exactly when the measurement is suppressed.
	Value              *float64                      `json:"value"`
	Suppressed         bool                          `json:"suppressed"`
	Reason             l8behaviour.SuppressionReason `json:"reason,omitempty"`
	Uncertainty        *l8behaviour.Uncertainty      `json:"uncertainty,omitempty"`
	OpportunitySeconds *float64                      `json:"opportunity_seconds,omitempty"`
	Display            string                        `json:"display"`
	UncertaintyDisplay string                        `json:"uncertainty_display"`
	OpportunityDisplay string                        `json:"opportunity_display,omitempty"`
}

// Accounting is where an encounter's time went. Accounted time is valid time
// plus suppressed time; record gaps stand for nothing and are shown apart,
// and unobserved time overlaps the suppressed time.
type Accounting struct {
	ValidNanos        int64        `json:"valid_nanos"`
	ValidDisplay      string       `json:"valid_display"`
	AccountedNanos    int64        `json:"accounted_nanos"`
	AccountedDisplay  string       `json:"accounted_display"`
	Suppressions      []ReasonTime `json:"suppressions"`
	UnobservedNanos   int64        `json:"unobserved_nanos"`
	UnobservedDisplay string       `json:"unobserved_display"`
	RecordGapNanos    int64        `json:"record_gap_nanos"`
	RecordGapDisplay  string       `json:"record_gap_display"`
}

// Endpoints records the evidence behind the gaps: the source of each party's
// endpoint at every instant whose gap could be computed, and the two
// endpoint estimates at the minimum supported gap.
type Endpoints struct {
	LeaderTrailing  []SourceCount `json:"leader_trailing_sources"`
	FollowerLeading []SourceCount `json:"follower_leading_sources"`
	// AtMinimumGap is absent when no gap was supported.
	AtMinimumGap *EndpointPair `json:"at_minimum_gap,omitempty"`
}

// SourceCount is how many instants rested on one endpoint source.
type SourceCount struct {
	Source   l8behaviour.EndpointSource `json:"source"`
	Instants int                        `json:"instants"`
}

// EndpointPair is the leader's trailing and the follower's leading endpoint
// at one instant, and the gap between them.
type EndpointPair struct {
	OffsetNanos   int64            `json:"offset_nanos"`
	OffsetDisplay string           `json:"offset_display"`
	Leader        EndpointEstimate `json:"leader"`
	Follower      EndpointEstimate `json:"follower"`
	GapDisplay    string           `json:"gap_display"`
}

// EndpointEstimate is one projected body endpoint on the shared path.
type EndpointEstimate struct {
	TrackID   string                     `json:"track_id"`
	Extremity l8behaviour.PathExtremity  `json:"extremity"`
	ArcM      float64                    `json:"arc_m"`
	SigmaM    float64                    `json:"sigma_m"`
	Source    l8behaviour.EndpointSource `json:"source"`
	Support   l8behaviour.SupportState   `json:"support"`
	Display   string                     `json:"display"`
}

// Interval is a run of consecutive encounter instants that are not valid
// following time, under one reason, condition and leader disposition.
type Interval struct {
	StartNanos      int64                            `json:"start_nanos"`
	EndNanos        int64                            `json:"end_nanos"`
	RangeDisplay    string                           `json:"range_display"`
	Instants        int                              `json:"instants"`
	Nanos           int64                            `json:"nanos"`
	DurationDisplay string                           `json:"duration_display"`
	Reason          l8behaviour.SuppressionReason    `json:"reason"`
	Condition       l8behaviour.PathCondition        `json:"condition,omitempty"`
	Role            l8behaviour.CandidateDisposition `json:"role"`
}

// PredictedInterval is a run of consecutive review-only predicted gaps.
type PredictedInterval struct {
	Metric           l8behaviour.MetricID   `json:"metric"`
	Visibility       l8behaviour.Visibility `json:"visibility"`
	StartNanos       int64                  `json:"start_nanos"`
	EndNanos         int64                  `json:"end_nanos"`
	RangeDisplay     string                 `json:"range_display"`
	Instants         int                    `json:"instants"`
	CoastAgeMinNanos int64                  `json:"coast_age_min_nanos"`
	CoastAgeMaxNanos int64                  `json:"coast_age_max_nanos"`
	CoastAgeDisplay  string                 `json:"coast_age_display"`
	GapDisplay       string                 `json:"gap_display"`
	SigmaMaxDisplay  string                 `json:"sigma_max_display"`
}

// Series holds the encounter's series for its chart. Gap and NetTimeGap are
// the supported instants only; Predicted is review-only.
type Series struct {
	Gap        []SeriesPoint    `json:"gap"`
	NetTimeGap []SeriesPoint    `json:"net_time_gap"`
	Predicted  []PredictedPoint `json:"predicted"`
}

// SeriesPoint is one supported value with its one-sigma, at an offset.
type SeriesPoint struct {
	OffsetNanos int64   `json:"offset_nanos"`
	Value       float64 `json:"value"`
	Sigma       float64 `json:"sigma"`
}

// PredictedPoint is one review-only predicted gap with its coast age.
type PredictedPoint struct {
	OffsetNanos   int64   `json:"offset_nanos"`
	ValueM        float64 `json:"value_m"`
	SigmaM        float64 `json:"sigma_m"`
	CoastAgeNanos int64   `json:"coast_age_nanos"`
}

// VersionKey is what encounters must share to be pooled: every version axis
// but geometry, which is per follower by construction, and the estimate
// stage, so final and non-final values never share a distribution.
type VersionKey struct {
	EstimateStage l8behaviour.EstimateStage `json:"estimate_stage"`
	EstimatorID   string                    `json:"estimator_id"`
	ObsModelID    string                    `json:"obs_model_id"`
	ParamHash     string                    `json:"param_hash"`
	MethodID      string                    `json:"method_id"`
}

// Aggregate pools one version group's supported encounter values.
type Aggregate struct {
	ID           string         `json:"id"`
	Version      VersionKey     `json:"version"`
	ValueBlock   ValueBlock     `json:"value_block"`
	EncounterIDs []string       `json:"encounter_ids"`
	Metrics      []Distribution `json:"metrics"`
	Bands        []BandExposure `json:"bands"`
	Histogram    Histogram      `json:"histogram"`
	Chart        string         `json:"chart"`
}

// Distribution summarises one encounter metric over the group: the
// supported values' count and range, and the suppressed count by reason.
// Uncertainty is not propagated to this level and is declared none.
type Distribution struct {
	Metric     l8behaviour.MetricID `json:"metric"`
	Unit       string               `json:"unit"`
	Supported  int                  `json:"supported"`
	Suppressed []ReasonCount        `json:"suppressed"`
	Min        *float64             `json:"min"`
	P50        *float64             `json:"p50"`
	Max        *float64             `json:"max"`
	MinDisplay string               `json:"min_display"`
	P50Display string               `json:"p50_display"`
	MaxDisplay string               `json:"max_display"`
}

// ReasonCount is how many encounters one reason suppressed.
type ReasonCount struct {
	Reason     l8behaviour.SuppressionReason `json:"reason"`
	Encounters int                           `json:"encounters"`
}

// BandExposure pools one band over the encounters whose band duration is
// supported: time below the band over their valid following time.
type BandExposure struct {
	Seconds      float64                       `json:"seconds"`
	Display      string                        `json:"display"`
	Duration     l8behaviour.MetricID          `json:"duration_metric"`
	Rate         l8behaviour.MetricID          `json:"rate_metric"`
	Encounters   int                           `json:"encounters"`
	Excluded     []ReasonCount                 `json:"excluded"`
	BelowNanos   int64                         `json:"below_nanos"`
	BelowDisplay string                        `json:"below_display"`
	ValidNanos   int64                         `json:"valid_nanos"`
	ValidDisplay string                        `json:"valid_display"`
	RateValue    *float64                      `json:"rate_value"`
	RateReason   l8behaviour.SuppressionReason `json:"rate_reason,omitempty"`
	RateDisplay  string                        `json:"rate_display"`
}

// Histogram is the time-weighted distribution of valid net time gap over
// the group's exposure-supported encounters, with every other second of the
// group's accounted time beside it under its reason. Bins and excluded time
// together are the denominator, exactly.
type Histogram struct {
	Metric             l8behaviour.MetricID `json:"metric"`
	Unit               string               `json:"unit"`
	BinMillis          int                  `json:"bin_millis"`
	MaxMillis          int                  `json:"max_millis"`
	DenominatorNanos   int64                `json:"denominator_nanos"`
	DenominatorDisplay string               `json:"denominator_display"`
	Encounters         int                  `json:"encounters"`
	Bins               []HistogramBin       `json:"bins"`
	Excluded           []ExcludedTime       `json:"excluded"`
}

// HistogramBin is [LowerMillis, UpperMillis) of valid time; the last bin has
// no upper edge.
type HistogramBin struct {
	LowerMillis  int     `json:"lower_millis"`
	UpperMillis  *int    `json:"upper_millis"`
	Label        string  `json:"label"`
	Nanos        int64   `json:"nanos"`
	Share        float64 `json:"share"`
	ShareDisplay string  `json:"share_display"`
}

// ExcludedTime is accounted time that is not in the distribution, by reason.
type ExcludedTime struct {
	Reason       l8behaviour.SuppressionReason `json:"reason"`
	Nanos        int64                         `json:"nanos"`
	Display      string                        `json:"display"`
	Share        float64                       `json:"share"`
	ShareDisplay string                        `json:"share_display"`
}
