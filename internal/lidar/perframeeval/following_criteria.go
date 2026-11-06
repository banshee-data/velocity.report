package perframeeval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// The following-criteria record (experiment l8fol0004).
//
// The following limits are predeclared on tuning data before any held-out
// window is read, as the geometry levels are in
// docs/lidar/operations/geometry-convergence-criteria.md (W0). That record is
// prose; this one is a versioned JSON file the scorer reads, so a score can
// cite it by path and content digest and a later change to a limit is
// visible as a different digest. The schema is
// config/criteria/following-limits.schema.json, and the unpinned template is
// config/criteria/following-limits.draft.json.
//
// A record is a draft or pinned. A draft may leave any value null: the scorer
// reads it on a tuning split, reports every interval, and calls a stratum
// insufficient where a limit it needs is absent ("missing truth or a missing
// criterion is insufficient evidence", the physical-reference review plan). A
// pinned record names the decision that pinned it (a D-NN), the date, the
// tuning evidence it was drafted on, and every value. Only a pinned record
// admits a held-out split. A pinned file is never edited: an amendment is a
// new file under a new decision, and the old digest stays cited by the
// scores read under it.

// FollowingCriteriaSchema names the following-criteria record.
const FollowingCriteriaSchema = "velocity.report/following-criteria"

// FollowingCriteriaSchemaVersion is its layout version.
const FollowingCriteriaSchemaVersion = 1

// A record's status.
const (
	CriteriaDraft  = "draft"
	CriteriaPinned = "pinned"
)

// CorrectionBonferroni divides the family's error rate equally over every
// interval test of one arm's following acceptance.
const CorrectionBonferroni = "bonferroni"

// FollowingLimits are the acceptance limits the decide step sets. A nil
// value is not yet chosen.
type FollowingLimits struct {
	// GapErrorLimitM bounds a gap stratum's 95th percentile of |gap error|
	// (nearest rank), metres.
	GapErrorLimitM *float64 `json:"gap_error_limit_m"`
	// ReferencePrecisionM is the widest reference gap half-width that may
	// count toward a pass. A gap whose reference is wider is reported and
	// counted as imprecise, never scored toward a pass.
	ReferencePrecisionM *float64 `json:"reference_precision_m"`
	// NominalCoverage is the coverage the predicted gap's interval claims:
	// a gap is covered when |error| <= z sigma, z the two-sided normal
	// quantile of this coverage.
	NominalCoverage *float64 `json:"nominal_coverage"`
	// MinCoverage and MaxCoverage bound the empirical coverage: too low is
	// overconfidence, too high an interval too wide to inform.
	MinCoverage *float64 `json:"min_coverage"`
	MaxCoverage *float64 `json:"max_coverage"`
	// MinIndependentEncounters is the fewest independent encounters a
	// stratum's verdict may rest on: the Kish effective count of its body
	// pairs (or follower bodies, for a no_leader or ambiguous record).
	MinIndependentEncounters *int `json:"min_independent_encounters_per_stratum"`
	// MaxMissingOpportunity and MaxSuppressedOpportunity bound the shares of
	// a gap stratum's opportunities that the estimator missed (no match, no
	// prediction, no leader where the record names one) or suppressed (a
	// refused leader choice, an unprojectable body, a gap the behaviour
	// layer would not publish).
	MaxMissingOpportunity    *float64 `json:"max_missing_opportunity_share"`
	MaxSuppressedOpportunity *float64 `json:"max_suppressed_opportunity_share"`
	// WrongLeaderTolerance bounds a leader-choice stratum's share of scored
	// instants whose leader is wrong.
	WrongLeaderTolerance *float64 `json:"wrong_leader_tolerance"`
}

// ValidFollowingDefinition is the valid-following definition the maths
// decision review fixed (E-01), with its values chosen in the decide step and
// the ceiling chosen by E-09's rule. Method following_acceptance_v1 records
// and validates it; it does not yet apply it to the scored instants.
type ValidFollowingDefinition struct {
	// MeasurementFloorMps is the speed below which a time gap is not
	// measured.
	MeasurementFloorMps *float64 `json:"measurement_floor_mps"`
	// OpportunityFloorMph is the speed below which an instant is not a
	// following opportunity.
	OpportunityFloorMph *float64 `json:"opportunity_floor_mph"`
	// SpatialOnlyBelowMph is the band edge below which only the spatial gap
	// is reported.
	SpatialOnlyBelowMph *float64 `json:"spatial_only_below_mph"`
	// NetCeilingS is the net time-gap ceiling, 4 or 6 s by E-09's rule, and
	// CeilingProvisional marks a 6 s ceiling chosen because 6 s kept fewer
	// than ten leader records.
	NetCeilingS        *float64 `json:"net_ceiling_s"`
	CeilingProvisional *bool    `json:"ceiling_provisional"`
	HysteresisMarginS  *float64 `json:"hysteresis_margin_s"`
	MinDwellS          *float64 `json:"min_dwell_s"`
	// SensitivityCeilingsS are the ceilings the report prints beside the
	// chosen one.
	SensitivityCeilingsS []float64 `json:"sensitivity_ceilings_s"`
}

// FollowingBootstrapPlan is the bootstrap the verdicts are read from.
type FollowingBootstrapPlan struct {
	Draws *int   `json:"draws"`
	Seed  *int64 `json:"seed"`
	// FamilyConfidence is the confidence of all of one arm's interval tests
	// together; Correction says how it is shared among them.
	FamilyConfidence *float64 `json:"family_confidence"`
	Correction       string   `json:"correction"`
}

// FollowingCriteriaProvenance is the tuning evidence a record was drafted
// on.
type FollowingCriteriaProvenance struct {
	Split                 string `json:"split"`
	SplitDigest           string `json:"split_digest"`
	PhysicalContentDigest string `json:"physical_content_digest"`
	// Report is the repository path of the report that sets out the
	// limits' rationale.
	Report string `json:"report"`
}

// FollowingCriteria is one following-criteria record.
type FollowingCriteria struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schema_version"`
	RecordID      string `json:"record_id"`
	Status        string `json:"status"`
	// Decision is the D-NN that pinned the record; PinnedOn its date,
	// YYYY-MM-DD. Both are empty in a draft.
	Decision       string                      `json:"decision"`
	PinnedOn       string                      `json:"pinned_on"`
	DraftedOn      FollowingCriteriaProvenance `json:"drafted_on"`
	Limits         FollowingLimits             `json:"limits"`
	ValidFollowing ValidFollowingDefinition    `json:"valid_following"`
	Bootstrap      FollowingBootstrapPlan      `json:"bootstrap"`
	Notes          []string                    `json:"notes,omitempty"`
}

var decisionID = regexp.MustCompile(`^D-[0-9]+$`)

// Validate checks every value present against its domain, and that a
// pinned record is complete and names its decision.
func (c FollowingCriteria) Validate() error {
	if c.Schema != FollowingCriteriaSchema || c.SchemaVersion != FollowingCriteriaSchemaVersion {
		return fmt.Errorf("following criteria are %q version %d, want %q version %d",
			c.Schema, c.SchemaVersion, FollowingCriteriaSchema, FollowingCriteriaSchemaVersion)
	}
	if c.RecordID == "" {
		return fmt.Errorf("following criteria need a record_id")
	}
	switch c.Status {
	case CriteriaDraft:
		if c.Decision != "" || c.PinnedOn != "" {
			return fmt.Errorf("following criteria %s are a draft but name decision %q of %q: pin them, or leave both empty",
				c.RecordID, c.Decision, c.PinnedOn)
		}
	case CriteriaPinned:
		if !decisionID.MatchString(c.Decision) {
			return fmt.Errorf("pinned following criteria %s need the decision that pinned them (D-NN), not %q", c.RecordID, c.Decision)
		}
		if _, err := time.Parse("2006-01-02", c.PinnedOn); err != nil {
			return fmt.Errorf("pinned following criteria %s need pinned_on as YYYY-MM-DD, not %q", c.RecordID, c.PinnedOn)
		}
		if c.DraftedOn.SplitDigest == "" || c.DraftedOn.Report == "" {
			return fmt.Errorf("pinned following criteria %s need the tuning split digest and the report they were drafted on", c.RecordID)
		}
		if missing := c.Missing(true); len(missing) > 0 {
			return fmt.Errorf("pinned following criteria %s leave %v unset: a pinned record states every value", c.RecordID, missing)
		}
	default:
		return fmt.Errorf("following criteria %s have status %q (want %q or %q)", c.RecordID, c.Status, CriteriaDraft, CriteriaPinned)
	}

	l := c.Limits
	for _, v := range []struct {
		name string
		v    *float64
	}{{"gap_error_limit_m", l.GapErrorLimitM}, {"reference_precision_m", l.ReferencePrecisionM}} {
		if v.v != nil && (!(*v.v > 0) || math.IsInf(*v.v, 0)) {
			return fmt.Errorf("following criteria %s: %s %v must be positive and finite", c.RecordID, v.name, *v.v)
		}
	}
	if l.NominalCoverage != nil && !(*l.NominalCoverage > 0 && *l.NominalCoverage < 1) {
		return fmt.Errorf("following criteria %s: nominal_coverage %v must be in (0, 1)", c.RecordID, *l.NominalCoverage)
	}
	for _, v := range []struct {
		name string
		v    *float64
	}{
		{"min_coverage", l.MinCoverage}, {"max_coverage", l.MaxCoverage},
		{"max_missing_opportunity_share", l.MaxMissingOpportunity},
		{"max_suppressed_opportunity_share", l.MaxSuppressedOpportunity},
		{"wrong_leader_tolerance", l.WrongLeaderTolerance},
	} {
		if v.v != nil && !(*v.v >= 0 && *v.v <= 1) {
			return fmt.Errorf("following criteria %s: %s %v must be in [0, 1]", c.RecordID, v.name, *v.v)
		}
	}
	if l.MinCoverage != nil && l.MaxCoverage != nil && *l.MinCoverage > *l.MaxCoverage {
		return fmt.Errorf("following criteria %s: min_coverage %v exceeds max_coverage %v", c.RecordID, *l.MinCoverage, *l.MaxCoverage)
	}
	if l.MinIndependentEncounters != nil && *l.MinIndependentEncounters < 1 {
		return fmt.Errorf("following criteria %s: min_independent_encounters_per_stratum must be at least 1", c.RecordID)
	}

	if err := c.ValidFollowing.validate(); err != nil {
		return fmt.Errorf("following criteria %s: %w", c.RecordID, err)
	}

	b := c.Bootstrap
	if b.Draws != nil && *b.Draws < 1 {
		return fmt.Errorf("following criteria %s: bootstrap draws must be at least 1", c.RecordID)
	}
	if b.FamilyConfidence != nil && !(*b.FamilyConfidence > 0 && *b.FamilyConfidence < 1) {
		return fmt.Errorf("following criteria %s: bootstrap family_confidence %v must be in (0, 1)", c.RecordID, *b.FamilyConfidence)
	}
	if b.Correction != "" && b.Correction != CorrectionBonferroni {
		return fmt.Errorf("following criteria %s: bootstrap correction %q (want %q)", c.RecordID, b.Correction, CorrectionBonferroni)
	}
	return nil
}

func (v ValidFollowingDefinition) validate() error {
	for _, f := range []struct {
		name string
		v    *float64
	}{
		{"measurement_floor_mps", v.MeasurementFloorMps}, {"opportunity_floor_mph", v.OpportunityFloorMph},
		{"spatial_only_below_mph", v.SpatialOnlyBelowMph}, {"net_ceiling_s", v.NetCeilingS},
		{"hysteresis_margin_s", v.HysteresisMarginS}, {"min_dwell_s", v.MinDwellS},
	} {
		if f.v != nil && (!(*f.v > 0) || math.IsInf(*f.v, 0)) {
			return fmt.Errorf("valid_following %s %v must be positive and finite", f.name, *f.v)
		}
	}
	if v.OpportunityFloorMph != nil && v.SpatialOnlyBelowMph != nil && !(*v.SpatialOnlyBelowMph > *v.OpportunityFloorMph) {
		return fmt.Errorf("valid_following spatial_only_below_mph %v must exceed opportunity_floor_mph %v",
			*v.SpatialOnlyBelowMph, *v.OpportunityFloorMph)
	}
	// E-09: the ceiling is 4 s net, or 6 s; nothing else.
	if v.NetCeilingS != nil && *v.NetCeilingS != 4 && *v.NetCeilingS != 6 {
		return fmt.Errorf("valid_following net_ceiling_s %v: E-09's rule chooses 4 or 6 s", *v.NetCeilingS)
	}
	if v.CeilingProvisional != nil && *v.CeilingProvisional && v.NetCeilingS != nil && *v.NetCeilingS != 6 {
		return fmt.Errorf("valid_following: only a 6 s ceiling is marked provisional under E-09")
	}
	for i, s := range v.SensitivityCeilingsS {
		if !(s > 0) || math.IsInf(s, 0) || (i > 0 && !(s > v.SensitivityCeilingsS[i-1])) {
			return fmt.Errorf("valid_following sensitivity_ceilings_s must ascend strictly and be positive")
		}
	}
	if v.NetCeilingS != nil && len(v.SensitivityCeilingsS) > 0 {
		found := false
		for _, s := range v.SensitivityCeilingsS {
			found = found || s == *v.NetCeilingS
		}
		if !found {
			return fmt.Errorf("valid_following sensitivity_ceilings_s %v must include the chosen ceiling %v",
				v.SensitivityCeilingsS, *v.NetCeilingS)
		}
	}
	return nil
}

// Missing lists the values still unset, by JSON path. Without all, it lists
// only what the acceptance verdicts read: the limits and the bootstrap.
// With all, the valid-following definition too, which a pinned record must
// state.
func (c FollowingCriteria) Missing(all bool) []string {
	var out []string
	l := c.Limits
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"limits.gap_error_limit_m", l.GapErrorLimitM != nil},
		{"limits.reference_precision_m", l.ReferencePrecisionM != nil},
		{"limits.nominal_coverage", l.NominalCoverage != nil},
		{"limits.min_coverage", l.MinCoverage != nil},
		{"limits.max_coverage", l.MaxCoverage != nil},
		{"limits.min_independent_encounters_per_stratum", l.MinIndependentEncounters != nil},
		{"limits.max_missing_opportunity_share", l.MaxMissingOpportunity != nil},
		{"limits.max_suppressed_opportunity_share", l.MaxSuppressedOpportunity != nil},
		{"limits.wrong_leader_tolerance", l.WrongLeaderTolerance != nil},
		{"bootstrap.draws", c.Bootstrap.Draws != nil},
		{"bootstrap.seed", c.Bootstrap.Seed != nil},
		{"bootstrap.family_confidence", c.Bootstrap.FamilyConfidence != nil},
		{"bootstrap.correction", c.Bootstrap.Correction != ""},
	} {
		if !f.set {
			out = append(out, f.name)
		}
	}
	if !all {
		return out
	}
	v := c.ValidFollowing
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"valid_following.measurement_floor_mps", v.MeasurementFloorMps != nil},
		{"valid_following.opportunity_floor_mph", v.OpportunityFloorMph != nil},
		{"valid_following.spatial_only_below_mph", v.SpatialOnlyBelowMph != nil},
		{"valid_following.net_ceiling_s", v.NetCeilingS != nil},
		{"valid_following.ceiling_provisional", v.CeilingProvisional != nil},
		{"valid_following.hysteresis_margin_s", v.HysteresisMarginS != nil},
		{"valid_following.min_dwell_s", v.MinDwellS != nil},
		{"valid_following.sensitivity_ceilings_s", len(v.SensitivityCeilingsS) > 0},
	} {
		if !f.set {
			out = append(out, f.name)
		}
	}
	return out
}

// FollowingCriteriaCitation is how a score cites the record it was read
// under: where it was, what it held, and whether it was pinned.
type FollowingCriteriaCitation struct {
	// Path is the record's path as it was named, which is the repository
	// path when the scorer runs from the checkout.
	Path          string `json:"path"`
	RecordID      string `json:"record_id"`
	Digest        string `json:"digest"`
	SchemaVersion int    `json:"schema_version"`
	Status        string `json:"status"`
	Decision      string `json:"decision,omitempty"`
	// Unset lists the values the verdicts need that the record leaves
	// unset; a stratum that needs one is insufficient.
	Unset []string `json:"unset,omitempty"`
}

// Pinned reports whether the cited record was pinned by a decision.
func (c *FollowingCriteriaCitation) Pinned() bool { return c != nil && c.Status == CriteriaPinned }

// LoadFollowingCriteria reads and validates a record. A relative path is
// read from dir when dir is set; the citation keeps the path as named. An
// unknown field or trailing data is refused, so a misspelt limit cannot be
// silently left unset.
func LoadFollowingCriteria(path, dir string) (*FollowingCriteria, FollowingCriteriaCitation, error) {
	read := path
	if dir != "" && !filepath.IsAbs(path) {
		read = filepath.Join(dir, path)
	}
	raw, err := os.ReadFile(read)
	if err != nil {
		return nil, FollowingCriteriaCitation{}, fmt.Errorf("read following criteria: %w", err)
	}
	c, err := ParseFollowingCriteria(raw)
	if err != nil {
		return nil, FollowingCriteriaCitation{}, fmt.Errorf("following criteria %s: %w", path, err)
	}
	sum := sha256.Sum256(raw)
	return c, FollowingCriteriaCitation{
		Path: path, RecordID: c.RecordID, Digest: "sha256:" + hex.EncodeToString(sum[:]),
		SchemaVersion: c.SchemaVersion, Status: c.Status, Decision: c.Decision, Unset: c.Missing(false),
	}, nil
}

// ParseFollowingCriteria decodes and validates a record's bytes.
func ParseFollowingCriteria(raw []byte) (*FollowingCriteria, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c FollowingCriteria
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("parse: trailing data after the record")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}
