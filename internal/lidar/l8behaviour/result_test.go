package l8behaviour

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func testProvenance() Provenance {
	return Provenance{
		Version: VersionProvenance{
			EstimateStage: StageFinal, EstimatorID: "est", ObsModelID: "obs",
			MethodID: FollowingMethodID, GeometryID: "path", ParamHash: "hash",
		},
		Input: InputProvenance{
			ContributingTrackIDs: []string{"trk_a", "trk_b"},
			FirstUnixNanos:       10, LastUnixNanos: 20, ObservedFrames: 2, PlanarFallback: true,
		},
	}
}

func TestUncertaintyValidation(t *testing.T) {
	valid := []Uncertainty{
		NoUncertainty(),
		SigmaUncertainty(0.3, MethodLinearised),
		{Kind: UncertaintySigma, Sigma: ptr(0.0), Method: MethodAnalytic},
		{Kind: UncertaintyInterval, Lower: ptr(0.8), Upper: ptr(1.9), Coverage: ptr(0.9), Method: MethodMonteCarlo, Samples: 4000},
		{Kind: UncertaintyBounds, Lower: ptr(0.0), Method: MethodAnalytic},
		{Kind: UncertaintyBounds, Upper: ptr(3.0), Method: MethodSigmaPoint, Samples: 9},
		{
			Kind: UncertaintySigma, Sigma: ptr(0.5), Method: MethodLinearised,
			Scopes: &UncertaintyScopes{TrackLocal: ptr(0.3), SharedSensor: ptr(0.4)},
		},
	}
	for i, u := range valid {
		if err := u.Validate(); err != nil {
			t.Errorf("valid[%d]: %v", i, err)
		}
	}
	invalid := map[string]Uncertainty{
		"unspecified kind":        {},
		"none with a sigma":       {Kind: UncertaintyNone, Sigma: ptr(1.0)},
		"none with a method":      {Kind: UncertaintyNone, Method: MethodAnalytic},
		"sigma without method":    {Kind: UncertaintySigma, Sigma: ptr(1.0)},
		"sigma absent":            {Kind: UncertaintySigma, Method: MethodAnalytic},
		"negative sigma":          {Kind: UncertaintySigma, Sigma: ptr(-1.0), Method: MethodAnalytic},
		"NaN sigma":               {Kind: UncertaintySigma, Sigma: ptr(math.NaN()), Method: MethodAnalytic},
		"sigma with bounds":       {Kind: UncertaintySigma, Sigma: ptr(1.0), Lower: ptr(0.0), Method: MethodAnalytic},
		"interval sans coverage":  {Kind: UncertaintyInterval, Lower: ptr(0.0), Upper: ptr(1.0), Method: MethodAnalytic},
		"interval coverage one":   {Kind: UncertaintyInterval, Lower: ptr(0.0), Upper: ptr(1.0), Coverage: ptr(1.0), Method: MethodAnalytic},
		"interval reversed":       {Kind: UncertaintyInterval, Lower: ptr(2.0), Upper: ptr(1.0), Coverage: ptr(0.9), Method: MethodAnalytic},
		"interval with sigma":     {Kind: UncertaintyInterval, Sigma: ptr(1.0), Lower: ptr(0.0), Upper: ptr(1.0), Coverage: ptr(0.9), Method: MethodAnalytic},
		"bounds empty":            {Kind: UncertaintyBounds, Method: MethodAnalytic},
		"bounds with coverage":    {Kind: UncertaintyBounds, Lower: ptr(0.0), Coverage: ptr(0.9), Method: MethodAnalytic},
		"monte carlo sans count":  {Kind: UncertaintySigma, Sigma: ptr(1.0), Method: MethodMonteCarlo},
		"analytic with samples":   {Kind: UncertaintySigma, Sigma: ptr(1.0), Method: MethodAnalytic, Samples: 5},
		"negative samples":        {Kind: UncertaintySigma, Sigma: ptr(1.0), Method: MethodSigmaPoint, Samples: -1},
		"scopes disagree":         {Kind: UncertaintySigma, Sigma: ptr(1.0), Method: MethodAnalytic, Scopes: &UncertaintyScopes{TrackLocal: ptr(0.5)}},
		"scopes empty":            {Kind: UncertaintySigma, Sigma: ptr(1.0), Method: MethodAnalytic, Scopes: &UncertaintyScopes{}},
		"scope negative":          {Kind: UncertaintySigma, Sigma: ptr(1.0), Method: MethodAnalytic, Scopes: &UncertaintyScopes{SharedGeometry: ptr(-1.0)}},
		"scopes on an interval":   {Kind: UncertaintyInterval, Lower: ptr(0.0), Upper: ptr(1.0), Coverage: ptr(0.9), Method: MethodAnalytic, Scopes: &UncertaintyScopes{TrackLocal: ptr(1.0)}},
		"unregistered method":     {Kind: UncertaintySigma, Sigma: ptr(1.0), Method: PropagationMethod(99)},
		"unregistered kind value": {Kind: UncertaintyKind(99)},
	}
	for name, u := range invalid {
		if err := u.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestUncertaintyScopesCombineInQuadrature(t *testing.T) {
	s := UncertaintyScopes{TrackLocal: ptr(0.3), SharedSensor: ptr(0.4)}
	if got := s.Combined(); math.Abs(got-0.5) > 1e-15 {
		t.Fatalf("combined %v, want 0.5", got)
	}
	// An unassessed scope is absent, not zero, and does not serialise.
	raw, _ := json.Marshal(s)
	if strings.Contains(string(raw), "shared_geometry") {
		t.Fatalf("unassessed scope serialised: %s", raw)
	}
}

func TestBenchmarkValidation(t *testing.T) {
	valid := []Benchmark{
		{Kind: BenchmarkNoEstablishedThreshold, Threshold: ptr(1.5)},
		{Kind: BenchmarkLegal, Jurisdiction: "DE", EffectiveFromUnixNanos: 1},
		{Kind: BenchmarkResearchThreshold, Citation: "FHWA-HRT-08-049"},
		{Kind: BenchmarkExternalDistribution, Citation: "highD"},
		{Kind: BenchmarkLocalDistribution, Stratification: "site,direction,hour"},
	}
	for _, b := range valid {
		if err := b.Validate(); err != nil {
			t.Errorf("%s: %v", b.Kind, err)
		}
	}
	for name, b := range map[string]Benchmark{
		"unspecified":            {},
		"legal sans date":        {Kind: BenchmarkLegal, Jurisdiction: "DE"},
		"research sans citation": {Kind: BenchmarkResearchThreshold},
		"external sans citation": {Kind: BenchmarkExternalDistribution},
		"local sans strata":      {Kind: BenchmarkLocalDistribution},
		"infinite threshold":     {Kind: BenchmarkNoEstablishedThreshold, Threshold: ptr(math.Inf(1))},
		"no threshold, cited":    {Kind: BenchmarkNoEstablishedThreshold, Citation: "Dingus 2006"},
		"no threshold, legal":    {Kind: BenchmarkNoEstablishedThreshold, Jurisdiction: "DE", EffectiveFromUnixNanos: 1},
		"no threshold, strata":   {Kind: BenchmarkNoEstablishedThreshold, Stratification: "site"},
	} {
		if err := b.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestProvenanceValidation(t *testing.T) {
	if err := testProvenance().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, fn := range map[string]func(*Provenance){
		"no stage":          func(p *Provenance) { p.Version.EstimateStage = StageUnspecified },
		"no estimator":      func(p *Provenance) { p.Version.EstimatorID = "" },
		"no method":         func(p *Provenance) { p.Version.MethodID = "" },
		"no param hash":     func(p *Provenance) { p.Version.ParamHash = "" },
		"no tracks":         func(p *Provenance) { p.Input.ContributingTrackIDs = nil },
		"empty track":       func(p *Provenance) { p.Input.ContributingTrackIDs = []string{""} },
		"duplicate track":   func(p *Provenance) { p.Input.ContributingTrackIDs = []string{"a", "a"} },
		"reversed interval": func(p *Provenance) { p.Input.FirstUnixNanos = 30 },
		"negative frames":   func(p *Provenance) { p.Input.CoastedFrames = -1 },
	} {
		p := testProvenance()
		fn(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	// Geometry is optional: a metric may use none.
	p := testProvenance()
	p.Version.GeometryID = ""
	if err := p.Validate(); err != nil {
		t.Fatalf("geometry is optional: %v", err)
	}
}

func TestMeasurementIsValueXorSuppression(t *testing.T) {
	m, err := NewMeasurement(MetricFollowingSpatialGap, 5.75, SigmaUncertainty(0.25, MethodLinearised), testProvenance())
	if err != nil || m.Unit != "m" || m.Benchmark != nil {
		t.Fatalf("measurement %+v err %v", m, err)
	}
	s, err := NewSuppressedMeasurement(MetricFollowingNetTimeGap, ReasonBelowSpeedFloor, testProvenance())
	if err != nil || s.Unit != "s" || s.Value != nil || !s.Suppressed {
		t.Fatalf("suppression %+v err %v", s, err)
	}
	// A no_established_threshold band carries its band as the threshold.
	band, err := NewMeasurement(MetricFollowingTimeBelow1500ms, 0.4, NoUncertainty(), testProvenance())
	if err != nil || band.Benchmark == nil || band.Benchmark.Kind != BenchmarkNoEstablishedThreshold ||
		band.Benchmark.Threshold == nil || *band.Benchmark.Threshold != 1.5 {
		t.Fatalf("band measurement %+v err %v", band, err)
	}

	// The registered kind may be attached with its provenance, and a band
	// may not be relabelled as a research threshold.
	cited := m
	cited.Benchmark = &Benchmark{Kind: BenchmarkExternalDistribution, Citation: "highD"}
	if err := cited.Validate(); err != nil {
		t.Fatalf("registered benchmark kind rejected: %v", err)
	}
	relabelled := band
	relabelled.Benchmark = &Benchmark{Kind: BenchmarkResearchThreshold, Citation: "driver education", Threshold: ptr(1.5)}
	if err := relabelled.Validate(); err == nil {
		t.Fatal("a no_established_threshold band validated as a research threshold")
	}

	for name, fn := range map[string]func(*Measurement){
		"value and suppressed": func(m *Measurement) { m.Suppressed, m.Reason = true, ReasonNotObserved },
		"value and reason":     func(m *Measurement) { m.Reason = ReasonNotObserved },
		"neither":              func(m *Measurement) { m.Value = nil },
		"no uncertainty":       func(m *Measurement) { m.Uncertainty = nil },
		"bad uncertainty":      func(m *Measurement) { m.Uncertainty = &Uncertainty{} },
		"NaN value":            func(m *Measurement) { m.Value = ptr(math.NaN()) },
		"unregistered name":    func(m *Measurement) { m.Name = "track.gap_m" },
		"wrong unit":           func(m *Measurement) { m.Unit = "ft" },
		"percentile over 100":  func(m *Measurement) { m.Percentile = ptr(101.0) },
		"negative opportunity": func(m *Measurement) { m.OpportunitySeconds = ptr(-1.0) },
		"bad benchmark":        func(m *Measurement) { m.Benchmark = &Benchmark{Kind: BenchmarkLegal} },
		"relabelled benchmark": func(m *Measurement) {
			m.Benchmark = &Benchmark{Kind: BenchmarkResearchThreshold, Citation: "FHWA-HRT-08-049"}
		},
		"bad provenance":         func(m *Measurement) { m.Provenance = Provenance{} },
		"suppressed sans reason": func(m *Measurement) { m.Value, m.Uncertainty, m.Suppressed = nil, nil, true },
		"suppressed with a percentile": func(m *Measurement) {
			m.Value, m.Uncertainty, m.Suppressed, m.Reason, m.Percentile = nil, nil, true, ReasonNotObserved, ptr(50.0)
		},
		"suppressed with a value": func(m *Measurement) {
			m.Uncertainty, m.Suppressed, m.Reason = nil, true, ReasonNotObserved
		},
	} {
		c := m
		fn(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := NewMeasurement("unregistered.metric_m", 1, NoUncertainty(), testProvenance()); err == nil {
		t.Error("unregistered name accepted")
	}
	if _, err := NewSuppressedMeasurement("unregistered.metric_m", ReasonNotObserved, testProvenance()); err == nil {
		t.Error("unregistered name accepted for a suppression")
	}
	if _, err := NewSuppressedMeasurement(MetricFollowingSpatialGap, ReasonUnspecified, testProvenance()); err == nil {
		t.Error("suppression without a reason accepted")
	}
}

func TestMeasurementJSONContract(t *testing.T) {
	m, _ := NewMeasurement(MetricFollowingNetTimeGap, 1.15, SigmaUncertainty(0.076, MethodLinearised), testProvenance())
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"name":"interaction.following_net_time_gap_s"`, `"unit":"s"`, `"value":1.15`,
		`"kind":"sigma"`, `"method":"linearised"`, `"track_local":0.076`, `"suppressed":false`,
		`"benchmark":{"kind":"no_established_threshold"}`, `"estimate_stage":"final"`,
		`"contributing_track_ids":["trk_a","trk_b"]`, `"planar_fallback":true`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("encoding lacks %s: %s", want, raw)
		}
	}
	var back Measurement
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, m) {
		t.Fatalf("round trip changed the measurement:\n%+v\n%+v", back, m)
	}
	// An unregistered reason token is refused on the way in.
	bad := `{"name":"interaction.following_spatial_gap_m","unit":"m","suppressed":true,"reason":"too_close"}`
	if err := json.Unmarshal([]byte(bad), &back); err == nil {
		t.Fatal("unregistered reason decoded")
	}
	// And an unset required token refuses to serialise at all.
	m.Provenance.Version.EstimateStage = StageUnspecified
	if _, err := json.Marshal(m); err == nil {
		t.Fatal("unspecified stage serialised")
	}
}

func TestOutcomeKeepsItsEvidence(t *testing.T) {
	good := Outcome{
		Name: "following_band", Value: "below_1500ms", Confidence: ptr(0.9),
		DerivedFrom:  []MeasurementRef{{Name: MetricFollowingNetTimeGap, FirstUnixNanos: 1, LastUnixNanos: 2}},
		ThresholdSet: "following_bands_v1", Provenance: testProvenance(),
	}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	suppressed := Outcome{Name: "following_band", Suppressed: true, Reason: ReasonNotObserved, Provenance: testProvenance()}
	if err := suppressed.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, fn := range map[string]func(*Outcome){
		"no evidence":           func(o *Outcome) { o.DerivedFrom = nil },
		"no threshold set":      func(o *Outcome) { o.ThresholdSet = "" },
		"unregistered evidence": func(o *Outcome) { o.DerivedFrom[0].Name = "track.nothing_m" },
		"reversed evidence":     func(o *Outcome) { o.DerivedFrom[0].FirstUnixNanos = 3 },
		"confidence over one":   func(o *Outcome) { o.Confidence = ptr(1.5) },
		"camel-case name":       func(o *Outcome) { o.Name = "followingBand" },
		"blank value":           func(o *Outcome) { o.Value = "" },
		"value and reason":      func(o *Outcome) { o.Reason = ReasonNotObserved },
		"suppressed with value": func(o *Outcome) { o.Suppressed, o.Reason = true, ReasonNotObserved },
		"suppressed sans reason": func(o *Outcome) {
			o.Suppressed, o.Value, o.Confidence = true, "", nil
		},
		"bad provenance": func(o *Outcome) { o.Provenance = Provenance{} },
	} {
		o := good
		o.DerivedFrom = append([]MeasurementRef(nil), good.DerivedFrom...)
		fn(&o)
		if err := o.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
