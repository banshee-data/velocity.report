package l8behaviour

import (
	"strings"
	"testing"
)

func TestFollowingMetricDefinitions(t *testing.T) {
	defs := FollowingMetrics()
	if len(defs) != 11 {
		t.Fatalf("%d following metrics, want 11", len(defs))
	}
	seen := map[MetricID]bool{}
	aliases := map[string]MetricID{}
	for _, d := range defs {
		if err := d.Validate(); err != nil {
			t.Error(err)
		}
		if seen[d.ID] {
			t.Errorf("duplicate id %s", d.ID)
		}
		seen[d.ID] = true
		if d.Family != "following" || d.Level != "interaction" {
			t.Errorf("%s: family %s level %s", d.ID, d.Family, d.Level)
		}
		if got, ok := LookupMetric(d.ID); !ok || got.ID != d.ID {
			t.Errorf("lookup %s failed", d.ID)
		}
		for _, a := range d.Aliases {
			// An alias is never itself a valid name, and names one metric.
			if _, ok := LookupMetric(MetricID(a)); ok {
				t.Errorf("%s alias %s is a registered id", d.ID, a)
			}
			if prev, ok := aliases[a]; ok && a != "following_close_rate" {
				t.Errorf("alias %s names both %s and %s", a, prev, d.ID)
			}
			aliases[a] = d.ID
		}
		// No composite score and no verdict: the vocabulary of Section 1.
		for _, forbidden := range []string{"score", "tailgat", "aggress", "driver", "safe"} {
			if strings.Contains(string(d.ID), forbidden) {
				t.Errorf("%s names a verdict", d.ID)
			}
		}
	}
	if _, ok := LookupMetric("interaction.nothing_m"); ok {
		t.Fatal("unregistered id found")
	}
	// Returned definitions are copies: editing one cannot edit the registry.
	defs[0].Aliases[0] = "edited"
	d, _ := LookupMetric(defs[0].ID)
	d.Aliases[0] = "edited too"
	if again, _ := LookupMetric(defs[0].ID); again.Aliases[0] != "distance_headway" {
		t.Fatalf("registry alias was edited through a copy: %v", again.Aliases)
	}
	// Review-only metrics never enter a published distribution.
	for _, id := range []MetricID{MetricFollowingObservedSurfaceGap, MetricFollowingPredictedGap} {
		if d, _ := LookupMetric(id); d.Visibility != VisibilityReviewOnly {
			t.Errorf("%s visibility %s", id, d.Visibility)
		}
	}
}

func TestMetricDefinitionValidateRejectsDrift(t *testing.T) {
	good, _ := LookupMetric(MetricFollowingTimeBelow1500ms)
	for name, fn := range map[string]func(*MetricDefinition){
		"level prefix":     func(d *MetricDefinition) { d.Level = "track" },
		"unit suffix":      func(d *MetricDefinition) { d.Unit = "ms" },
		"no estimator":     func(d *MetricDefinition) { d.Estimator = "" },
		"no visibility":    func(d *MetricDefinition) { d.Visibility = VisibilityUnspecified },
		"bad benchmark":    func(d *MetricDefinition) { d.Benchmark = BenchmarkKind(99) },
		"negative band":    func(d *MetricDefinition) { d.BandSeconds = -1 },
		"band not in name": func(d *MetricDefinition) { d.BandSeconds = 2.0 },
	} {
		d := good
		fn(&d)
		if err := d.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestFollowingBands(t *testing.T) {
	bands := FollowingBands()
	want := []float64{2.0, 1.5, 1.0}
	if len(bands) != len(want) {
		t.Fatalf("%d bands", len(bands))
	}
	for i, b := range bands {
		if b.Seconds != want[i] {
			t.Fatalf("band %d is %v s, want %v", i, b.Seconds, want[i])
		}
		for _, id := range []MetricID{b.Duration, b.Rate} {
			d, ok := LookupMetric(id)
			if !ok || d.BandSeconds != b.Seconds {
				t.Fatalf("%s band %v, want %v", id, d.BandSeconds, b.Seconds)
			}
		}
		// The band itself has no established threshold; its rate is
		// compared against the local distribution (Section 8.3).
		if d, _ := LookupMetric(b.Duration); d.Benchmark != BenchmarkNoEstablishedThreshold {
			t.Errorf("%s benchmark %s", b.Duration, d.Benchmark)
		}
		if d, _ := LookupMetric(b.Rate); d.Benchmark != BenchmarkLocalDistribution {
			t.Errorf("%s benchmark %s", b.Rate, d.Benchmark)
		}
		// Strictly below: time(THW < X).
		if b.Contains(b.Seconds) || !b.Contains(b.Seconds-1e-9) {
			t.Errorf("band %v is not strict", b.Seconds)
		}
	}
}

func TestClassApplicability(t *testing.T) {
	for _, c := range []struct {
		subject, counterpart MotionClass
		want                 SuppressionReason
	}{
		{MotionRigidVehicle, MotionRigidVehicle, ReasonUnspecified},
		{MotionTwoWheeler, MotionRigidVehicle, ReasonClassNotSupported},
		{MotionRigidVehicle, MotionPedestrian, ReasonClassNotSupported},
		{MotionPedestrian, MotionPedestrian, ReasonClassNotSupported},
		{MotionUnknown, MotionRigidVehicle, ReasonClassNotSupported},
		{MotionRigidVehicle, MotionUnknown, ReasonClassNotSupported},
	} {
		for _, d := range FollowingMetrics() {
			got, err := ClassApplicability(d.ID, c.subject, c.counterpart)
			if err != nil || got != c.want {
				t.Errorf("%s %s after %s: %s, %v; want %s", d.ID, c.subject, c.counterpart, got, err, c.want)
			}
		}
	}
	for name, args := range map[string]struct {
		id                   MetricID
		subject, counterpart MotionClass
	}{
		"unregistered metric":     {"interaction.nothing_m", MotionRigidVehicle, MotionRigidVehicle},
		"unspecified subject":     {MetricFollowingSpatialGap, MotionClassUnspecified, MotionRigidVehicle},
		"unspecified counterpart": {MetricFollowingSpatialGap, MotionRigidVehicle, MotionClassUnspecified},
	} {
		if _, err := ClassApplicability(args.id, args.subject, args.counterpart); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestClassRuleSingleTrack(t *testing.T) {
	// No single-track metric is registered yet; the rule is exercised
	// directly so the first one inherits a tested path.
	rule := ClassRule{Subject: []MotionClass{MotionRigidVehicle, MotionTwoWheeler}}
	for _, c := range []struct {
		subject MotionClass
		want    SuppressionReason
	}{
		{MotionTwoWheeler, ReasonUnspecified},
		{MotionPedestrian, ReasonClassNotSupported},
		{MotionUnknown, ReasonClassNotSupported},
	} {
		if got, err := rule.Check(c.subject, MotionClassUnspecified); err != nil || got != c.want {
			t.Errorf("%s: %s, %v; want %s", c.subject, got, err, c.want)
		}
	}
	if _, err := rule.Check(MotionRigidVehicle, MotionRigidVehicle); err == nil {
		t.Fatal("a counterpart supplied to a single-track rule was accepted")
	}
	if _, ok := classApplicability("track.nothing_m"); ok {
		t.Fatal("unregistered metric has a rule")
	}
}
