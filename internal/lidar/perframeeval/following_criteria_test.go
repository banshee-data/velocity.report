package perframeeval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	draftCriteriaPath  = "../../../config/criteria/following-limits.draft.json"
	criteriaSchemaPath = "../../../config/criteria/following-limits.schema.json"
)

func f64(v float64) *float64 { return &v }

// pinnedTestCriteria is a complete, pinned record with synthetic values. The
// values are fixtures for the tests, not limits anyone has chosen.
func pinnedTestCriteria() FollowingCriteria {
	draws, seed, enc, ceilingProvisional := 400, int64(7), 3, false
	return FollowingCriteria{
		Schema: FollowingCriteriaSchema, SchemaVersion: FollowingCriteriaSchemaVersion,
		RecordID: "following-limits-test", Status: CriteriaPinned, Decision: "D-99", PinnedOn: "2026-11-05",
		DraftedOn: FollowingCriteriaProvenance{Split: "tuning", SplitDigest: "sha256:split", PhysicalContentDigest: "sha256:content",
			Report: "docs/lidar/operations/following-limits-2026-10.md"},
		Limits: FollowingLimits{
			GapErrorLimitM: f64(0.5), ReferencePrecisionM: f64(0.3), NominalCoverage: f64(0.9),
			MinCoverage: f64(0.5), MaxCoverage: f64(1), MinIndependentEncounters: &enc,
			MaxMissingOpportunity: f64(0.5), MaxSuppressedOpportunity: f64(0.5), WrongLeaderTolerance: f64(0.2),
		},
		ValidFollowing: ValidFollowingDefinition{
			MeasurementFloorMps: f64(0.4), OpportunityFloorMph: f64(2), SpatialOnlyBelowMph: f64(10), NetCeilingS: f64(4),
			CeilingProvisional: &ceilingProvisional, HysteresisMarginS: f64(0.5), MinDwellS: f64(1),
			SensitivityCeilingsS: []float64{3, 4, 6},
		},
		Bootstrap: FollowingBootstrapPlan{Draws: &draws, Seed: &seed, FamilyConfidence: f64(0.95), Correction: CorrectionBonferroni},
	}
}

func writeCriteria(t *testing.T, dir, name string, c FollowingCriteria) string {
	t.Helper()
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The committed template is an unpinned draft that chooses nothing: every
// limit, definition value and bootstrap choice is unset.
func TestCommittedFollowingCriteriaDraftChoosesNothing(t *testing.T) {
	c, cite, err := LoadFollowingCriteria(draftCriteriaPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != CriteriaDraft || cite.Pinned() || c.Decision != "" || c.PinnedOn != "" {
		t.Fatalf("the template is %s by %q", c.Status, c.Decision)
	}
	if got, want := len(c.Missing(true)), 21; got != want {
		t.Fatalf("the template sets %d values: unset %v", want-got, c.Missing(true))
	}
	if len(cite.Unset) != 13 || !strings.HasPrefix(cite.Digest, "sha256:") || cite.Path != draftCriteriaPath {
		t.Fatalf("citation %+v", cite)
	}
}

// The schema names exactly the fields the reader decodes, at every level, so
// the two cannot drift apart unnoticed.
func TestFollowingCriteriaSchemaMatchesTheReader(t *testing.T) {
	raw, err := os.ReadFile(criteriaSchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	keys := func(typ reflect.Type) []string {
		var out []string
		for i := 0; i < typ.NumField(); i++ {
			name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			out = append(out, name)
		}
		sort.Strings(out)
		return out
	}
	names := func(m map[string]json.RawMessage) []string {
		var out []string
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	top := map[string]json.RawMessage{}
	for k := range schema.Properties {
		top[k] = nil
	}
	if got, want := names(top), keys(reflect.TypeOf(FollowingCriteria{})); !reflect.DeepEqual(got, want) {
		t.Fatalf("schema properties %v, reader fields %v", got, want)
	}
	for field, typ := range map[string]reflect.Type{
		"drafted_on": reflect.TypeOf(FollowingCriteriaProvenance{}), "limits": reflect.TypeOf(FollowingLimits{}),
		"valid_following": reflect.TypeOf(ValidFollowingDefinition{}), "bootstrap": reflect.TypeOf(FollowingBootstrapPlan{}),
	} {
		if got, want := names(schema.Properties[field].Properties), keys(typ); !reflect.DeepEqual(got, want) {
			t.Fatalf("schema %s properties %v, reader fields %v", field, got, want)
		}
	}
	// The template and the test record each satisfy the reader; the schema
	// requires every top-level field but notes.
	if len(schema.Required) != len(keys(reflect.TypeOf(FollowingCriteria{})))-1 {
		t.Fatalf("schema requires %v", schema.Required)
	}
}

func TestFollowingCriteriaValidation(t *testing.T) {
	if err := pinnedTestCriteria().Validate(); err != nil {
		t.Fatalf("a complete pinned record: %v", err)
	}
	for name, mutate := range map[string]func(c *FollowingCriteria){
		"wrong schema":               func(c *FollowingCriteria) { c.Schema = "other" },
		"no record id":               func(c *FollowingCriteria) { c.RecordID = "" },
		"unknown status":             func(c *FollowingCriteria) { c.Status = "final" },
		"pinned without a decision":  func(c *FollowingCriteria) { c.Decision = "" },
		"pinned by a non-decision":   func(c *FollowingCriteria) { c.Decision = "E-09" },
		"pinned without a date":      func(c *FollowingCriteria) { c.PinnedOn = "" },
		"pinned without evidence":    func(c *FollowingCriteria) { c.DraftedOn.Report = "" },
		"pinned with a limit unset":  func(c *FollowingCriteria) { c.Limits.WrongLeaderTolerance = nil },
		"pinned with dwell unset":    func(c *FollowingCriteria) { c.ValidFollowing.MinDwellS = nil },
		"pinned with draws unset":    func(c *FollowingCriteria) { c.Bootstrap.Draws = nil },
		"draft naming a decision":    func(c *FollowingCriteria) { c.Status = CriteriaDraft },
		"negative gap limit":         func(c *FollowingCriteria) { c.Limits.GapErrorLimitM = f64(-1) },
		"coverage of one":            func(c *FollowingCriteria) { c.Limits.NominalCoverage = f64(1) },
		"coverage range reversed":    func(c *FollowingCriteria) { c.Limits.MinCoverage = f64(0.9); c.Limits.MaxCoverage = f64(0.8) },
		"share above one":            func(c *FollowingCriteria) { c.Limits.MaxMissingOpportunity = f64(1.5) },
		"no encounters":              func(c *FollowingCriteria) { n := 0; c.Limits.MinIndependentEncounters = &n },
		"a 5 s ceiling":              func(c *FollowingCriteria) { c.ValidFollowing.NetCeilingS = f64(5) },
		"a provisional 4 s ceiling":  func(c *FollowingCriteria) { p := true; c.ValidFollowing.CeilingProvisional = &p },
		"ceiling not in sensitivity": func(c *FollowingCriteria) { c.ValidFollowing.SensitivityCeilingsS = []float64{3, 6} },
		"sensitivity not ascending":  func(c *FollowingCriteria) { c.ValidFollowing.SensitivityCeilingsS = []float64{4, 3} },
		"band edge below the floor":  func(c *FollowingCriteria) { c.ValidFollowing.SpatialOnlyBelowMph = f64(1) },
		"unknown correction":         func(c *FollowingCriteria) { c.Bootstrap.Correction = "holm" },
		"family confidence of one":   func(c *FollowingCriteria) { c.Bootstrap.FamilyConfidence = f64(1) },
	} {
		c := pinnedTestCriteria()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A draft may leave anything unset, and may hold candidate values.
	draft := pinnedTestCriteria()
	draft.Status, draft.Decision, draft.PinnedOn = CriteriaDraft, "", ""
	draft.Limits.GapErrorLimitM, draft.Bootstrap = nil, FollowingBootstrapPlan{}
	if err := draft.Validate(); err != nil {
		t.Fatalf("a partial draft: %v", err)
	}
}

// A record is read strictly: a misspelt field or trailing data is refused,
// not silently left unset. The citation's digest is the file's bytes, and a
// relative path is read from the directory given but cited as named.
func TestLoadFollowingCriteria(t *testing.T) {
	dir := t.TempDir()
	path := writeCriteria(t, dir, "pinned.json", pinnedTestCriteria())
	raw, _ := os.ReadFile(path)
	sum := sha256.Sum256(raw)
	c, cite, err := LoadFollowingCriteria("pinned.json", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cite.Pinned() || cite.Path != "pinned.json" || cite.Digest != "sha256:"+hex.EncodeToString(sum[:]) ||
		cite.Decision != "D-99" || cite.RecordID != c.RecordID || len(cite.Unset) != 0 {
		t.Fatalf("citation %+v", cite)
	}
	if _, _, err := LoadFollowingCriteria("pinned.json", ""); err == nil {
		t.Fatal("a relative path was read from the working directory, not the one given")
	}

	misspelt := strings.Replace(string(raw), `"wrong_leader_tolerance"`, `"wrong_leader_tolerence"`, 1)
	if _, err := ParseFollowingCriteria([]byte(misspelt)); err == nil {
		t.Fatal("a misspelt limit was accepted")
	}
	if _, err := ParseFollowingCriteria(append(append([]byte{}, raw...), []byte(" {}")...)); err == nil {
		t.Fatal("trailing data was accepted")
	}
	var nilCite *FollowingCriteriaCitation
	if nilCite.Pinned() {
		t.Fatal("no citation is pinned")
	}
}
