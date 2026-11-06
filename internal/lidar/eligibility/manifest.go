// Package eligibility decides whether a capture may be used for a
// development purpose (tuning, sweeps, development evaluation, developer
// diagnostics) before any of its samples load, and records each decision.
//
// It sits above the annotation contracts rather than replacing them. A
// frozen split (annotation.FrozenSplit) partitions objects within packs and
// binds a run given the split; this package holds the protection that does
// not depend on the caller supplying one: the site/capture cohort manifest of
// docs/plans/lidar-qualification-cohort-isolation-plan.md, every frozen
// split's held-out packs and cases beneath the annotation root whatever
// lineage they belong to, and the measure-specific record of what each
// capture has already been used for.
//
// Four things are kept apart and never folded into one value: the cohort a
// site is assigned to (development, validation, qualification), the role a
// frozen split gives an annotation (tuning, held_out, screen), the history of
// what a capture was used for, and how far its labelling and review have got.
//
// A capture is identified by the SHA-256 of its bytes. A basename is only
// ever a way to notice a collision: renaming or copying a capture keeps its
// hash, so it keeps its restrictions, and a new run ID changes nothing.
// Unknown, conflicting, unassigned and hash-mismatched inputs fail closed for
// every development purpose. Production processing is not gated.
package eligibility

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ManifestSchema names the cohort manifest format.
const ManifestSchema = "velocity.report/cohort-manifest"

// ManifestSchemaVersion is the only version this package reads.
const ManifestSchemaVersion = 1

// maxManifestBytes bounds the manifest read: it lists sites and captures,
// never samples.
const maxManifestBytes = 8 << 20

// Cohort is a site's population role. It is not an annotation split role.
type Cohort string

const (
	CohortDevelopment   Cohort = "development"
	CohortValidation    Cohort = "validation"
	CohortQualification Cohort = "qualification"
	// CohortUnassigned is what a capture the manifest does not list, or any
	// capture when no manifest is configured, has. It is never safe for
	// development use.
	CohortUnassigned Cohort = "unassigned"
)

func (c Cohort) assignable() bool {
	return c == CohortDevelopment || c == CohortValidation || c == CohortQualification
}

// Manifest is the versioned cohort assignment: which site every capture is
// from and which cohort every site belongs to. Sessions from one site share
// the site's cohort by construction, so a site cannot be split across
// development and qualification.
type Manifest struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schema_version"`
	// Revision increases with every change to the assignments.
	Revision int `json:"revision"`
	// FrozenUTC is when this revision of the assignments was frozen.
	FrozenUTC string `json:"frozen_utc"`
	// ProtocolDigest names the preregistered qualification protocol the
	// assignments were made under. Empty means the protocol is still TBD.
	ProtocolDigest string         `json:"protocol_digest,omitempty"`
	Sites          []ManifestSite `json:"sites"`
	Captures       []CaptureEntry `json:"captures"`
	// LiveSources assigns a live sensor to a site, so a live sweep has a
	// cohort too. A sensor that is not listed is unassigned.
	LiveSources []LiveSource `json:"live_sources,omitempty"`
	// Access states which access controls exist for protected data. A
	// manifest alone is not an access-control mechanism.
	Access AccessControls `json:"access"`
}

// ManifestSite is one canonical site and its cohort.
type ManifestSite struct {
	SiteID    string `json:"site_id"`
	S2L10     string `json:"s2_l10,omitempty"`
	S2L13     string `json:"s2_l13,omitempty"`
	Cohort    Cohort `json:"cohort"`
	Rationale string `json:"rationale"`
}

// CaptureEntry is one capture file, by the SHA-256 of its bytes.
type CaptureEntry struct {
	CaptureID string `json:"capture_id"`
	SiteID    string `json:"site_id"`
	RawSHA256 string `json:"raw_sha256"`
	// Basename is informational and used only to notice a file that shares
	// a listed capture's name but not its bytes.
	Basename  string `json:"basename,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	// Cohort, when given, must equal the site's: it lets a reviewer read
	// the role on the line, and a mismatch is a conflict, not an override.
	Cohort Cohort `json:"cohort,omitempty"`
}

// LiveSource assigns a live sensor to a site.
type LiveSource struct {
	SensorID string `json:"sensor_id"`
	SiteID   string `json:"site_id"`
}

// AccessControls records what protects qualification data beyond the
// manifest. Every field defaults to absent.
type AccessControls struct {
	// QualificationStorageRestricted is true only when qualification
	// annotations, expected answers and residuals are held behind
	// storage-level restrictions this server enforces. Nothing in this
	// repository provides them today.
	QualificationStorageRestricted bool `json:"qualification_storage_restricted"`
	// Custodians are the people authorised to run isolated qualification
	// scoring. Empty means custodians are TBD.
	Custodians []string `json:"custodians,omitempty"`
}

var (
	sha256Pattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	idPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

// ErrManifestInvalid wraps every reason a manifest is refused.
var ErrManifestInvalid = errors.New("cohort manifest is invalid")

// LoadedManifest is a manifest read and validated, with the digest of its
// exact bytes and its lookups.
type LoadedManifest struct {
	Manifest
	Path   string
	Digest string

	sites    map[string]ManifestSite
	bySHA    map[string]CaptureEntry
	byName   map[string][]CaptureEntry
	bySensor map[string]LiveSource
}

// LoadManifest reads and validates the manifest at path.
func LoadManifest(path string) (*LoadedManifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxManifestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxManifestBytes {
		return nil, fmt.Errorf("%w: larger than %d bytes", ErrManifestInvalid, maxManifestBytes)
	}
	m, err := ParseManifest(b)
	if err != nil {
		return nil, err
	}
	m.Path = path
	return m, nil
}

// ParseManifest decodes strictly and validates. Every problem is listed at
// once, so one edit can clear them all.
func ParseManifest(b []byte) (*LoadedManifest, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing data after the manifest", ErrManifestInvalid)
	}
	lm := &LoadedManifest{Manifest: m, Digest: digestBytes(b)}
	if problems := lm.index(); len(problems) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrManifestInvalid, strings.Join(problems, "; "))
	}
	return lm, nil
}

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// index validates the manifest and builds its lookups.
func (lm *LoadedManifest) index() []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	m := &lm.Manifest
	if m.Schema != ManifestSchema {
		add("schema is %q, want %q", m.Schema, ManifestSchema)
	}
	if m.SchemaVersion != ManifestSchemaVersion {
		add("schema_version is %d, this build reads %d", m.SchemaVersion, ManifestSchemaVersion)
	}
	if m.Revision < 1 {
		add("revision must be 1 or more")
	}
	if strings.TrimSpace(m.FrozenUTC) == "" {
		add("frozen_utc is required: an assignment that is not frozen can drift")
	}
	if m.ProtocolDigest != "" && !sha256Pattern.MatchString(m.ProtocolDigest) {
		add("protocol_digest %q is not sha256:<64 hex>", m.ProtocolDigest)
	}
	lm.sites = map[string]ManifestSite{}
	for i, s := range m.Sites {
		switch {
		case !idPattern.MatchString(s.SiteID):
			add("site %d: site_id %q is not a valid identifier", i, s.SiteID)
		case !s.Cohort.assignable():
			add("site %s: cohort %q is not development, validation or qualification", s.SiteID, s.Cohort)
		case strings.TrimSpace(s.Rationale) == "":
			add("site %s: rationale is required", s.SiteID)
		}
		if _, dup := lm.sites[s.SiteID]; dup {
			add("site %s is listed twice", s.SiteID)
		}
		lm.sites[s.SiteID] = s
	}
	lm.bySHA = map[string]CaptureEntry{}
	lm.byName = map[string][]CaptureEntry{}
	ids := map[string]bool{}
	for i, c := range m.Captures {
		label := c.CaptureID
		if label == "" {
			label = fmt.Sprintf("#%d", i)
		}
		if !idPattern.MatchString(c.CaptureID) {
			add("capture %s: capture_id is not a valid identifier", label)
		}
		if ids[c.CaptureID] {
			add("capture %s is listed twice", label)
		}
		ids[c.CaptureID] = true
		if !sha256Pattern.MatchString(c.RawSHA256) {
			add("capture %s: raw_sha256 %q is not sha256:<64 hex>", label, c.RawSHA256)
		}
		site, ok := lm.sites[c.SiteID]
		if !ok {
			add("capture %s: site %q is not listed", label, c.SiteID)
		} else if c.Cohort != "" && c.Cohort != site.Cohort {
			add("capture %s: cohort %q conflicts with site %s's cohort %q", label, c.Cohort, c.SiteID, site.Cohort)
		}
		if prev, dup := lm.bySHA[c.RawSHA256]; dup && c.RawSHA256 != "" {
			add("captures %s and %s have the same bytes (%s): one capture has exactly one entry", prev.CaptureID, label, c.RawSHA256)
		}
		lm.bySHA[c.RawSHA256] = c
		if c.Basename != "" {
			lm.byName[c.Basename] = append(lm.byName[c.Basename], c)
		}
	}
	lm.bySensor = map[string]LiveSource{}
	for i, l := range m.LiveSources {
		if strings.TrimSpace(l.SensorID) == "" {
			add("live source %d: sensor_id is required", i)
		}
		if _, ok := lm.sites[l.SiteID]; !ok {
			add("live source %s: site %q is not listed", l.SensorID, l.SiteID)
		}
		if _, dup := lm.bySensor[l.SensorID]; dup {
			add("live source %s is listed twice", l.SensorID)
		}
		lm.bySensor[l.SensorID] = l
	}
	sort.Strings(problems)
	return problems
}

// Assignment is what the manifest says about one input.
type Assignment struct {
	Cohort    Cohort `json:"cohort"`
	SiteID    string `json:"site_id,omitempty"`
	CaptureID string `json:"capture_id,omitempty"`
	// Conflict says why the input cannot be assigned: a listed basename
	// whose bytes differ.
	Conflict string `json:"conflict,omitempty"`
}

// Capture looks a capture up by its bytes. A file whose basename is listed
// under other bytes is a conflict: it may be an edited or truncated copy, and
// nothing can say which cohort it belongs to.
func (lm *LoadedManifest) Capture(sha, basename string) Assignment {
	if e, ok := lm.bySHA[sha]; ok {
		return Assignment{Cohort: lm.sites[e.SiteID].Cohort, SiteID: e.SiteID, CaptureID: e.CaptureID}
	}
	for _, e := range lm.byName[basename] {
		return Assignment{Cohort: CohortUnassigned, Conflict: fmt.Sprintf(
			"%s shares its name with manifest capture %s (%s) but its bytes are %s", basename, e.CaptureID, e.RawSHA256, sha)}
	}
	return Assignment{Cohort: CohortUnassigned}
}

// Live looks a live sensor up.
func (lm *LoadedManifest) Live(sensorID string) Assignment {
	if l, ok := lm.bySensor[sensorID]; ok {
		return Assignment{Cohort: lm.sites[l.SiteID].Cohort, SiteID: l.SiteID}
	}
	return Assignment{Cohort: CohortUnassigned}
}

// CohortCounts are the sites and captures per cohort: aggregate metadata a
// developer session may see about protected data.
func (lm *LoadedManifest) CohortCounts() map[Cohort]CohortCount {
	out := map[Cohort]CohortCount{}
	for _, s := range lm.Sites {
		c := out[s.Cohort]
		c.Sites++
		out[s.Cohort] = c
	}
	for _, e := range lm.Captures {
		cohort := lm.sites[e.SiteID].Cohort
		c := out[cohort]
		c.Captures++
		out[cohort] = c
	}
	return out
}

// CohortCount is a cohort's size.
type CohortCount struct {
	Sites    int `json:"sites"`
	Captures int `json:"captures"`
}
