package segments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/config"
)

// SelectorsVersion is the version of the selector file this build reads.
const SelectorsVersion = 1

// DefaultSelectorsPath is the selector file in the repository. A binary that
// runs anywhere else carries the same file embedded.
const DefaultSelectorsPath = "config/segment-selectors.defaults.json"

const (
	maxSelectorsFileSize = 1 << 20
	maxSelectors         = 100
	maxLabel             = 80
	maxCategory          = 40
	maxDescription       = 400
)

// Score names the measure that ranks a selector's windows, and which way:
// "descending" puts the largest first, "ascending" the smallest.
type Score struct {
	Measure string `json:"measure"`
	Order   string `json:"order"`
}

// Requirement bounds one measure. A window outside the bounds is not
// offered. Both bounds are inclusive, and a nil bound is no bound.
type Requirement struct {
	Measure string   `json:"measure"`
	Min     *float64 `json:"min"`
	Max     *float64 `json:"max"`
}

// Selector is one way of choosing windows, as the selector file defines it: a
// finder at its parameters, the measure that ranks what the finder returns,
// and the bounds a window must meet. The label, category and description are
// for people; they change nothing about which windows are chosen.
type Selector struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	Category    string        `json:"category"`
	Description string        `json:"description"`
	Finder      string        `json:"finder"`
	Parameters  Params        `json:"parameters"`
	Score       Score         `json:"score"`
	Require     []Requirement `json:"require"`
}

// Catalogue is a parsed selector file: the selectors in the order to show
// them, with the file's digest and where it was read from, a path or
// "embedded".
type Catalogue struct {
	Version   int        `json:"version"`
	Selectors []Selector `json:"selectors"`
	Digest    string     `json:"digest"`
	Source    string     `json:"source"`
}

// finderParameters lists the parameters each finder reads. A selector names
// exactly these. The rest keep their defaults, because every parameter is
// part of a window's identity: one the finder ignores must not be able to
// change which window is which.
var finderParameters = map[string][]string{
	"following":      {"window_seconds", "min_speed", "max_heading_deg", "min_gap", "max_gap", "max_lateral"},
	"leader_changes": {"window_seconds", "min_speed", "max_heading_deg", "min_gap", "max_gap", "max_lateral"},
	"lateral_jump":   {"window_seconds", "jump_threshold", "jump_max_gap_seconds"},
	"split_flags":    {"window_seconds"},
	"exposure":       {"window_seconds", "min_speed"},
	"random":         {"window_seconds", "min_speed", "random_seed"},
}

var selectorID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// HeldOut reports whether the selector may choose held-out windows. Only a
// standard definition may: a finder that does not seek tracker failure, at
// its default parameters, ranked by its own score, largest first, with
// nothing required. Whether a measure is safe depends on all of these at
// once. A min_gap of 0 makes a following pair of the two halves of one split
// vehicle, and ranking following smallest first finds one-frame pairs. The
// file cannot say otherwise; widening this is a change to the code, with its
// reason recorded.
func (s Selector) HeldOut() bool {
	return Allowed(s.Finder, "held_out") && s.standard()
}

// standard reports whether the selector ranks exactly as its finder does on
// its own.
func (s Selector) standard() bool {
	return s.Parameters == DefaultParams() && s.Score == Score{nativeMeasure[s.Finder], "descending"} && len(s.Require) == 0
}

// Digest identifies what the selector does: its id and everything that
// decides which windows it offers, in which order. The label, category and
// description are left out, so renaming a selector does not change it.
func (s Selector) Digest() string {
	require := s.Require
	if require == nil {
		require = []Requirement{}
	}
	b, _ := json.Marshal(struct {
		ID         string        `json:"id"`
		Finder     string        `json:"finder"`
		Parameters Params        `json:"parameters"`
		Score      Score         `json:"score"`
		Require    []Requirement `json:"require"`
	}{s.ID, s.Finder, s.Parameters, s.Score, require})
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// SelectorProvenance is what a chosen window keeps of the selector that chose
// it: the definition as it ran, which the file may no longer say by the time
// anyone asks. HeldOutEligible is the code's verdict at the time.
type SelectorProvenance struct {
	ID              string        `json:"id"`
	Label           string        `json:"label"`
	Digest          string        `json:"digest"`
	HeldOutEligible bool          `json:"held_out_eligible"`
	Finder          string        `json:"finder"`
	Parameters      Params        `json:"parameters"`
	Score           Score         `json:"score"`
	Require         []Requirement `json:"require"`
}

// Provenance records the selector as it runs.
func (s Selector) Provenance() SelectorProvenance {
	require := s.Require
	if require == nil {
		require = []Requirement{}
	}
	return SelectorProvenance{s.ID, s.Label, s.Digest(), s.HeldOut(), s.Finder, s.Parameters, s.Score, require}
}

// check reads a recorded selector for its form and for agreement with the
// window it chose. It never consults the selector file or today's rules: a
// pack that stops validating is taken for an unfinished attempt and removed,
// so what validates must not change when policy does.
func (p SelectorProvenance) check(finder, role string, params Params) error {
	if !selectorID.MatchString(p.ID) || !strings.HasPrefix(p.Digest, "sha256:") || len(p.Digest) != 71 {
		return fmt.Errorf("invalid selector record")
	}
	if p.Finder != finder || p.Parameters != params {
		return fmt.Errorf("selector %q ran another finder or other parameters", p.ID)
	}
	if role == "held_out" && !p.HeldOutEligible {
		return fmt.Errorf("selector %q could not choose a held_out window", p.ID)
	}
	return nil
}

func (s Selector) validate() error {
	if !selectorID.MatchString(s.ID) {
		return fmt.Errorf("id %q must be lower case letters, digits and underscores, starting with a letter", s.ID)
	}
	for _, text := range []struct {
		name, value string
		limit       int
	}{{"label", s.Label, maxLabel}, {"category", s.Category, maxCategory}, {"description", s.Description, maxDescription}} {
		if strings.TrimSpace(text.value) == "" || len(text.value) > text.limit {
			return fmt.Errorf("%s must be given, in at most %d characters", text.name, text.limit)
		}
	}
	if _, ok := finderParameters[s.Finder]; !ok {
		return fmt.Errorf("unknown finder %q", s.Finder)
	}
	if err := s.Parameters.Validate(); err != nil {
		return err
	}
	if err := s.computes(s.Score.Measure); err != nil {
		return fmt.Errorf("score: %w", err)
	}
	if s.Score.Order != "descending" && s.Score.Order != "ascending" {
		return fmt.Errorf("score: order must be descending or ascending, not %q", s.Score.Order)
	}
	bounded := map[string]bool{}
	for _, r := range s.Require {
		if err := s.computes(r.Measure); err != nil {
			return fmt.Errorf("require: %w", err)
		}
		if bounded[r.Measure] {
			return fmt.Errorf("require: %s is bounded twice", r.Measure)
		}
		bounded[r.Measure] = true
		if r.Min == nil && r.Max == nil {
			return fmt.Errorf("require: %s has neither a min nor a max", r.Measure)
		}
		if (r.Min != nil && !finite(*r.Min)) || (r.Max != nil && !finite(*r.Max)) {
			return fmt.Errorf("require: %s has a bound that is not a finite number", r.Measure)
		}
		if r.Min != nil && r.Max != nil && *r.Min > *r.Max {
			return fmt.Errorf("require: %s has min above max", r.Measure)
		}
	}
	return nil
}

// computes says whether the selector's finder computes a measure.
func (s Selector) computes(name string) error {
	m, ok := measures[name]
	if !ok {
		return fmt.Errorf("unknown measure %q", name)
	}
	for _, finder := range m.finders {
		if finder == s.Finder {
			return nil
		}
	}
	return fmt.Errorf("the %s finder does not compute %s", s.Finder, name)
}

// Selector returns the selector with this id.
func (c *Catalogue) Selector(id string) (Selector, bool) {
	for _, s := range c.Selectors {
		if s.ID == id {
			return s, true
		}
	}
	return Selector{}, false
}

// ForRole returns the selectors that may choose a window for the role, in
// the order the file gives them.
func (c *Catalogue) ForRole(role string) []Selector {
	out := []Selector{}
	for _, s := range c.Selectors {
		if role == "tuning" || (role == "held_out" && s.HeldOut()) {
			out = append(out, s)
		}
	}
	return out
}

// ParseSelectors reads a selector file. It is as strict as the tuning file:
// an unknown key, a missing key and a value of the wrong type are all errors.
// The file must define the six standard selectors, one per finder under the
// finder's own name and with its standard definition; their label, category,
// description and position are free.
func ParseSelectors(data []byte) (*Catalogue, error) {
	var file struct {
		Version   int               `json:"version"`
		Selectors []json.RawMessage `json:"selectors"`
	}
	if err := config.StrictDecode(data, &file, "selectors file"); err != nil {
		return nil, err
	}
	if file.Version != SelectorsVersion {
		return nil, fmt.Errorf("selectors file: version must be %d, not %d", SelectorsVersion, file.Version)
	}
	if len(file.Selectors) == 0 || len(file.Selectors) > maxSelectors {
		return nil, fmt.Errorf("selectors file: must define between 1 and %d selectors, not %d", maxSelectors, len(file.Selectors))
	}
	c := &Catalogue{Version: file.Version, Selectors: make([]Selector, 0, len(file.Selectors))}
	ids, labels := map[string]bool{}, map[string]string{}
	for i, raw := range file.Selectors {
		s, err := parseSelector(raw, fmt.Sprintf("selectors[%d]", i))
		if err != nil {
			return nil, err
		}
		if ids[s.ID] {
			return nil, fmt.Errorf("selectors[%d]: id %q is used twice", i, s.ID)
		}
		label := strings.ToLower(strings.TrimSpace(s.Label))
		if other, used := labels[label]; used {
			return nil, fmt.Errorf("selectors[%d] (%s): label %q is already %s's", i, s.ID, s.Label, other)
		}
		ids[s.ID], labels[label] = true, s.ID
		c.Selectors = append(c.Selectors, s)
	}
	for _, f := range Finders() {
		if !ids[f.Name] {
			return nil, fmt.Errorf("selectors file: the standard selector %q is missing", f.Name)
		}
	}
	sum := sha256.Sum256(data)
	c.Digest = "sha256:" + hex.EncodeToString(sum[:])
	return c, nil
}

func parseSelector(raw json.RawMessage, path string) (Selector, error) {
	var doc struct {
		ID          string            `json:"id"`
		Label       string            `json:"label"`
		Category    string            `json:"category"`
		Description string            `json:"description"`
		Finder      string            `json:"finder"`
		Parameters  json.RawMessage   `json:"parameters"`
		Score       json.RawMessage   `json:"score"`
		Require     []json.RawMessage `json:"require"`
	}
	if err := config.StrictDecode(raw, &doc, path); err != nil {
		return Selector{}, err
	}
	if doc.ID != "" {
		path += " (" + doc.ID + ")"
	}
	keys, ok := finderParameters[doc.Finder]
	if !ok {
		return Selector{}, fmt.Errorf("%s: unknown finder %q", path, doc.Finder)
	}
	s := Selector{ID: doc.ID, Label: doc.Label, Category: doc.Category, Description: doc.Description, Finder: doc.Finder, Require: []Requirement{}}
	params, err := parseParameters(doc.Parameters, keys, path+".parameters")
	if err != nil {
		return Selector{}, err
	}
	s.Parameters = params
	if err := config.StrictDecode(doc.Score, &s.Score, path+".score"); err != nil {
		return Selector{}, err
	}
	if doc.Require == nil {
		return Selector{}, fmt.Errorf("%s.require: expected a list, [] for none", path)
	}
	for i, r := range doc.Require {
		var bound Requirement
		if err := config.StrictDecode(r, &bound, fmt.Sprintf("%s.require[%d]", path, i)); err != nil {
			return Selector{}, err
		}
		s.Require = append(s.Require, bound)
	}
	if err := s.validate(); err != nil {
		return Selector{}, fmt.Errorf("%s: %w", path, err)
	}
	for _, f := range Finders() {
		if s.ID == f.Name && (s.Finder != f.Name || !s.standard()) {
			return Selector{}, fmt.Errorf("%s: %q is a standard selector, so it must keep the standard definition: the %s finder at its default parameters, ranked by %s descending, with nothing required",
				path, s.ID, f.Name, nativeMeasure[f.Name])
		}
	}
	return s, nil
}

// parseParameters reads the parameters a finder uses onto the defaults. The
// keys must be exactly the finder's, and none may be null, which would
// otherwise read as "keep the default" without saying so.
func parseParameters(raw json.RawMessage, keys []string, path string) (Params, error) {
	var given map[string]json.RawMessage
	if err := json.Unmarshal(raw, &given); err != nil || given == nil {
		return Params{}, fmt.Errorf("%s: expected an object", path)
	}
	wanted := map[string]bool{}
	for _, key := range keys {
		wanted[key] = true
	}
	var unknown, missing []string
	for key := range given {
		if !wanted[key] {
			unknown = append(unknown, key)
		}
	}
	for _, key := range keys {
		value, ok := given[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		var v any
		if json.Unmarshal(value, &v) == nil && v == nil {
			return Params{}, fmt.Errorf("%s: %s is null", path, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return Params{}, fmt.Errorf("%s: this finder does not read %s", path, strings.Join(unknown, ", "))
	}
	if len(missing) > 0 {
		return Params{}, fmt.Errorf("%s: missing required keys: %s", path, strings.Join(missing, ", "))
	}
	p := DefaultParams()
	if err := json.Unmarshal(raw, &p); err != nil {
		return Params{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return Params{}, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// LoadSelectors reads and parses a selector file.
func LoadSelectors(path string) (*Catalogue, error) {
	clean := filepath.Clean(path)
	if ext := filepath.Ext(clean); ext != ".json" {
		return nil, fmt.Errorf("selectors file must have the .json extension, not %q", ext)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return nil, fmt.Errorf("selectors file: %w", err)
	}
	if info.Size() > maxSelectorsFileSize {
		return nil, fmt.Errorf("selectors file is %d bytes, more than %d", info.Size(), maxSelectorsFileSize)
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return nil, fmt.Errorf("selectors file: %w", err)
	}
	c, err := ParseSelectors(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", clean, err)
	}
	c.Source = clean
	return c, nil
}

// LoadSelectorsOrEmbedded reads the selector file at path, or the embedded
// copy when there is nothing at path. A file that is there and does not
// parse is an error, never a reason to use the embedded copy instead.
func LoadSelectorsOrEmbedded(path string, embedded []byte) (*Catalogue, error) {
	if _, err := os.Stat(path); err == nil {
		return LoadSelectors(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("selectors file %s: %w", path, err)
	}
	return parseEmbedded(embedded)
}

func parseEmbedded(embedded []byte) (*Catalogue, error) {
	if len(embedded) == 0 {
		return nil, fmt.Errorf("no selectors file was found and none is embedded")
	}
	c, err := ParseSelectors(embedded)
	if err != nil {
		return nil, fmt.Errorf("embedded selectors: %w", err)
	}
	c.Source = "embedded"
	return c, nil
}

var embeddedSelectors []byte

// SetEmbeddedSelectors gives the binary's own copy of the selector file, for
// a binary that runs outside the repository.
func SetEmbeddedSelectors(b []byte) {
	embeddedSelectors = b
}

// DefaultCatalogue reads the repository's selector file when a relative path
// reaches it, as one does from the repository or from a test, and the
// embedded copy otherwise.
func DefaultCatalogue() (*Catalogue, error) {
	paths := []string{}
	for up := ""; len(paths) < 6; up += "../" {
		paths = append(paths, up+DefaultSelectorsPath)
	}
	return catalogueFrom(paths, embeddedSelectors)
}

func catalogueFrom(paths []string, embedded []byte) (*Catalogue, error) {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return LoadSelectors(path)
		}
	}
	return parseEmbedded(embedded)
}
