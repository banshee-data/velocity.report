package replayeval

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func testCoverage() ContinuityCoverage {
	return ContinuityCoverage{Source: "test: synthetic", SensorXMetres: 1, SensorYMetres: -2,
		MinRangeMetres: 1, MaxRangeMetres: 60, AzimuthCentreDeg: 90, AzimuthHalfWidthDeg: 180}
}

func TestContinuityCoverageValidate(t *testing.T) {
	if err := testCoverage().Validate(); err != nil {
		t.Fatalf("a valid declaration was refused: %v", err)
	}
	for name, c := range map[string]struct {
		mutate func(*ContinuityCoverage)
		want   string
	}{
		"no source":            {func(c *ContinuityCoverage) { c.Source = "  " }, "no source"},
		"unbounded range":      {func(c *ContinuityCoverage) { c.MaxRangeMetres = 0 }, "max_range_m must be positive"},
		"negative min range":   {func(c *ContinuityCoverage) { c.MinRangeMetres = -1 }, "min_range_m"},
		"min above max":        {func(c *ContinuityCoverage) { c.MinRangeMetres = 70 }, "min_range_m"},
		"zero sector":          {func(c *ContinuityCoverage) { c.AzimuthHalfWidthDeg = 0 }, "write 180 for the full circle"},
		"sector beyond circle": {func(c *ContinuityCoverage) { c.AzimuthHalfWidthDeg = 181 }, "azimuth_half_width_deg"},
		"centre out of range":  {func(c *ContinuityCoverage) { c.AzimuthCentreDeg = 400 }, "azimuth_centre_deg"},
		"non-finite origin":    {func(c *ContinuityCoverage) { c.SensorXMetres = float32(math.NaN()) }, "sensor_x_m is not finite"},
		"infinite range":       {func(c *ContinuityCoverage) { c.MaxRangeMetres = float32(math.Inf(1)) }, "max_range_m is not finite"},
	} {
		t.Run(name, func(t *testing.T) {
			cov := testCoverage()
			c.mutate(&cov)
			if err := cov.Validate(); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestLoadContinuityCoverage(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("good.json", `{"source": "survey", "max_range_m": 50, "azimuth_half_width_deg": 180}`)
	c, err := LoadContinuityCoverage(good)
	if err != nil || c.MaxRangeMetres != 50 || c.Source != "survey" {
		t.Fatalf("load = %+v, %v", c, err)
	}
	// A misspelt bound must not silently become zero.
	if _, err := LoadContinuityCoverage(write("typo.json", `{"source": "survey", "max_range": 50, "azimuth_half_width_deg": 180}`)); err == nil {
		t.Fatal("an unknown field was accepted")
	}
	if _, err := LoadContinuityCoverage(write("unbounded.json", `{"source": "survey", "azimuth_half_width_deg": 180}`)); err == nil {
		t.Fatal("an unbounded declaration was accepted")
	}
	if _, err := LoadContinuityCoverage(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("a missing file was accepted")
	}
	// A second object is not a second chance to read the first.
	if _, err := LoadContinuityCoverage(write("twice.json",
		`{"source": "survey", "max_range_m": 50, "azimuth_half_width_deg": 180} {"source": "other"}`)); err == nil ||
		!strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("trailing data: err %v", err)
	}
}

func TestContinuityCoverageIDIsContentAddressed(t *testing.T) {
	a, b := testCoverage(), testCoverage()
	if a.ID() != b.ID() || !strings.HasPrefix(a.ID(), "sha256:") {
		t.Fatalf("equal declarations have ids %s and %s", a.ID(), b.ID())
	}
	b.MaxRangeMetres = 61
	if a.ID() == b.ID() {
		t.Fatal("different declarations share an id")
	}
	// Declarations Validate refuses still have distinct ids.
	nan, inf := testCoverage(), testCoverage()
	nan.MaxRangeMetres, inf.MaxRangeMetres = float32(math.NaN()), float32(math.Inf(1))
	if nan.ID() == inf.ID() || nan.ID() == a.ID() || nan.ID() != nan.ID() {
		t.Fatalf("non-finite declarations: ids %s and %s", nan.ID(), inf.ID())
	}
}

// With a declaration the absence-classifying experiments run, at the
// declared origin and coverage; without one they are refused; and the
// options that need no coverage are unchanged by one.
func TestCoverageUnlocksOnlyTheAbsenceExperiments(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	cov := testCoverage()
	for _, name := range []string{ExperimentCoastSupport, ExperimentClassCoastBounds, ExperimentOcclusionContinuity} {
		if _, err := trackerConfigFor(l5, "", []string{name}, nil); err == nil ||
			!strings.Contains(err.Error(), "--continuity-coverage") {
			t.Fatalf("%s without coverage: err %v, want a refusal naming the flag", name, err)
		}
		got, err := trackerConfigFor(l5, "", []string{name}, &cov)
		if err != nil {
			t.Fatalf("%s with coverage: %v", name, err)
		}
		oc := got.OcclusionContinuity
		wantCoverage := l5tracks.SensorCoverage{MinRangeMetres: 1, MaxRangeMetres: 60, AzimuthCentreDeg: 90, AzimuthHalfWidthDeg: 180}
		if oc.SensorX != 1 || oc.SensorY != -2 || oc.Coverage != wantCoverage {
			t.Fatalf("%s: origin (%g, %g) coverage %+v, want the declaration", name, oc.SensorX, oc.SensorY, oc.Coverage)
		}
	}
	bad := cov
	bad.MaxRangeMetres = 0
	if _, err := trackerConfigFor(l5, "", []string{ExperimentCoastSupport}, &bad); err == nil {
		t.Fatal("an invalid declaration unlocked coast_support")
	}

	plain, err := trackerConfigFor(l5, "", []string{ExperimentCoastTimeInflation}, nil)
	if err != nil {
		t.Fatal(err)
	}
	withCoverage, err := trackerConfigFor(l5, "", []string{ExperimentCoastTimeInflation}, &cov)
	if err != nil {
		t.Fatal(err)
	}
	if plain != withCoverage {
		t.Fatal("a coverage declaration changed an experiment that does not classify absences")
	}
	if needsCoverage([]string{ExperimentCoastTimeInflation, ExperimentReacquisitionGuard}) ||
		!needsCoverage([]string{ExperimentCoastTimeInflation, ExperimentCoastSupport}) {
		t.Fatal("needsCoverage disagrees with the experiments that classify absences")
	}
}

func TestCoverageHashesOnlyWhenApplied(t *testing.T) {
	cov := testCoverage()
	if coverageHashSuffix(nil, true) != nil || coverageHashSuffix(&cov, false) != nil {
		t.Fatal("an absent or unapplied declaration changed the parameter hash")
	}
	if got := string(coverageHashSuffix(&cov, true)); got != "\ncontinuity_coverage:"+cov.ID() {
		t.Fatalf("suffix = %q", got)
	}
}

func TestLoadContinuityCoverageSet(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	set, err := LoadContinuityCoverageSet(write("set.json", `{
		"columbus-broadway": {"source": "survey", "max_range_m": 60, "azimuth_half_width_deg": 180},
		"marina-webster-beach": {"source": "survey", "max_range_m": 45, "azimuth_centre_deg": 90, "azimuth_half_width_deg": 120}}`))
	if err != nil || len(set) != 2 || set["marina-webster-beach"].AzimuthHalfWidthDeg != 120 {
		t.Fatalf("set = %+v, %v", set, err)
	}
	for name, body := range map[string]string{
		"empty.json":   `{}`,
		"invalid.json": `{"columbus-broadway": {"source": "survey", "azimuth_half_width_deg": 180}}`,
		"typo.json":    `{"columbus-broadway": {"source": "survey", "max_range": 60, "azimuth_half_width_deg": 180}}`,
		"twice.json": `{"columbus-broadway": {"source": "survey", "max_range_m": 60, "azimuth_half_width_deg": 180}}
			{"marina-webster-beach": {"source": "survey"}}`,
	} {
		if _, err := LoadContinuityCoverageSet(write(name, body)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A declaration is also the solid body's sensor origin, and is then applied:
// hashed, as the continuity experiments' is. Without one the origin is derived
// from the identity tracking transform and named as such.
func TestCoverageIsTheSolidBodysDeclaredOrigin(t *testing.T) {
	l5 := config.MustLoadDefaultConfig().L5.CvKfV1
	cov := testCoverage()
	got, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody}, &cov)
	if err != nil {
		t.Fatal(err)
	}
	sb := got.SolidBody
	if !sb.Enabled || sb.SensorX != 1 || sb.SensorY != -2 || sb.OriginSource != "continuity_coverage:"+cov.ID() {
		t.Fatalf("solid body %+v, want the declaration's origin and id", sb)
	}
	derived, err := trackerConfigFor(l5, "", []string{ExperimentSolidBody}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if o := derived.SolidBody; o.SensorX != 0 || o.SensorY != 0 || o.OriginSource != OriginTrackingTransformIdentity {
		t.Fatalf("solid body %+v without a declaration, want the identity transform's origin", o)
	}
	if needsCoverage([]string{ExperimentSolidBody}) {
		t.Fatal("solid_body alone must not require coverage; it derives its origin without one")
	}
}
