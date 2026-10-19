package experiments

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Dir is where experiment specs live, relative to the repository root.
const Dir = "data/experiments"

// slugPattern is an area of four to six characters (a letter, then letters
// or digits) and a four-digit number. The number is the last four
// characters, so an area ending in a digit is unambiguous.
var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9]{3,5}[0-9]{4}$`)

var decisionPattern = regexp.MustCompile(`^D-[0-9]+$`)

// namePattern is an arm name, a step id or a site: each is part of a unit's
// id and of its file under the state directory.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// AreaOf returns a slug's area: everything but its last four characters.
func AreaOf(slug string) string {
	if len(slug) < 4 {
		return ""
	}
	return slug[:len(slug)-4]
}

// Area is one subject area, a directory under Dir whose README.md header
// registers it. The directory is the registry: a slug's area must be one.
type Area struct {
	Area   string `yaml:"area"`
	Title  string `yaml:"title"`
	Layers string `yaml:"layers"`
	Owner  string `yaml:"owner"`
}

// Set is every experiment under Dir.
type Set struct {
	Areas map[string]Area
	Specs []*Spec // sorted by ID
	byID  map[string]*Spec
}

// Get returns the spec with the given slug, or nil.
func (s *Set) Get(id string) *Spec { return s.byID[id] }

// Load reads every area and spec under root/Dir. A directory without a
// README.md header, and the legacy try/ folder, are skipped.
func Load(root string) (*Set, error) {
	base := filepath.Join(root, Dir)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	set := &Set{Areas: map[string]Area{}, byID: map[string]*Spec{}}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "try" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		areaDir := filepath.Join(base, e.Name())
		readme := filepath.Join(areaDir, "README.md")
		if !hasFrontMatter(readme) {
			continue // not an area: no README, or a README without a header
		}
		area, err := readArea(readme)
		if err != nil {
			return nil, err
		}
		if area.Area != e.Name() {
			return nil, fmt.Errorf("%s: area %q does not match its directory %q", readme, area.Area, e.Name())
		}
		set.Areas[area.Area] = *area
		files, err := os.ReadDir(areaDir)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") || f.Name() == "README.md" {
				continue
			}
			path := filepath.Join(areaDir, f.Name())
			spec, err := ReadFile(path)
			if err != nil {
				return nil, err
			}
			rel, _ := filepath.Rel(root, path)
			spec.Path = filepath.ToSlash(rel)
			set.Specs = append(set.Specs, spec)
		}
	}
	sort.Slice(set.Specs, func(i, j int) bool { return set.Specs[i].ID < set.Specs[j].ID })
	for _, s := range set.Specs {
		if _, dup := set.byID[s.ID]; !dup {
			set.byID[s.ID] = s
		}
	}
	return set, nil
}

func hasFrontMatter(path string) bool {
	raw, err := os.ReadFile(path)
	return err == nil && strings.HasPrefix(strings.ReplaceAll(string(raw), "\r\n", "\n"), "---\n")
}

func readArea(path string) (*Area, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	header, err := frontMatter(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(header))
	dec.KnownFields(true)
	var a Area
	if err := dec.Decode(&a); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &a, nil
}

// Validate checks every spec against the rules in the experiment queue plan
// and returns each problem found, in file order. root is the repository
// root, against which every path a spec names must resolve.
func (s *Set) Validate(root string) []error {
	var errs []error
	seen := map[string]string{}
	for _, spec := range s.Specs {
		for _, e := range spec.validate(root, s) {
			errs = append(errs, fmt.Errorf("%s: %w", spec.Path, e))
		}
		if prev, dup := seen[spec.ID]; dup {
			errs = append(errs, fmt.Errorf("%s: id %s is also used by %s", spec.Path, spec.ID, prev))
		}
		seen[spec.ID] = spec.Path
	}
	if cycle := s.dependencyCycle(); cycle != nil {
		errs = append(errs, fmt.Errorf("dependency cycle: %s", strings.Join(cycle, " -> ")))
	}
	return errs
}

func (spec *Spec) validate(root string, set *Set) []error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	// Identity.
	if !slugPattern.MatchString(spec.ID) {
		fail("id %q is not a slug: an area of 4 to 6 lower-case letters or digits, starting with a letter, then four digits (e.g. l3bg0004)", spec.ID)
	}
	area := AreaOf(spec.ID)
	if _, ok := set.Areas[area]; !ok {
		fail("area %q of %s is not a registered area (a directory under %s with a README.md header)", area, spec.ID, Dir)
	}
	if want := Dir + "/" + area + "/" + spec.ID + ".md"; spec.Path != want {
		fail("file must be %s for id %s", want, spec.ID)
	}
	if strings.TrimSpace(spec.Title) == "" {
		fail("title is empty")
	}
	if strings.TrimSpace(spec.Owner) == "" {
		fail("owner is empty")
	}
	if _, err := time.Parse("2006-01-02", spec.Created); err != nil {
		fail("created %q is not a date (YYYY-MM-DD)", spec.Created)
	}

	// Lifecycle.
	if !spec.Status.valid() {
		fail("status %q: want proposed, approved, answered or retired", spec.Status)
	}
	if spec.Report != "" && (!strings.HasPrefix(spec.Report, "docs/") || !strings.HasSuffix(spec.Report, ".md")) {
		fail("report %q must be a Markdown file under docs/", spec.Report)
	}
	if spec.Status == StatusAnswered {
		// A proposed experiment may name the report it intends; an
		// answered one must point at the report that exists.
		if spec.Report == "" {
			fail("answered without a report")
		} else if !exists(root, spec.Report) {
			fail("report %s does not exist", spec.Report)
		}
	}
	if spec.Status == StatusRetired && strings.TrimSpace(spec.Reason) == "" {
		fail("retired without a reason")
	}

	// Questions and dependencies.
	if len(spec.Asks) == 0 {
		fail("asks names no question: an experiment answers something")
	}
	for _, q := range spec.Asks {
		if decisionPattern.MatchString(q) {
			continue
		}
		path, _, _ := strings.Cut(q, "#")
		if !strings.HasSuffix(path, ".md") || !exists(root, path) {
			fail("asks %q: want a decision id (D-NN) or the repository path of an existing Markdown file, with an optional #anchor", q)
		}
	}
	for _, d := range spec.DependsOn {
		if d == spec.ID {
			fail("depends on itself")
		} else if set.Get(d) == nil {
			fail("depends on %s, which does not exist", d)
		}
	}

	// Execution.
	if !spec.Reclaim.valid() {
		fail("reclaim %q: want any, origin or never", spec.Reclaim)
	}
	for _, c := range spec.Requires.Capabilities {
		if _, ok := Capabilities[c]; !ok {
			fail("capability %q is not one of %s", c, sortedKeys(Capabilities))
		}
	}
	if spec.Requires.DiskGB < 0 || spec.Requires.Cores < 0 {
		fail("requires must not be negative")
	}
	armNames := map[string]bool{}
	for _, a := range spec.Arms {
		if a.Name == "" {
			fail("an arm has no name")
		} else if !namePattern.MatchString(a.Name) {
			fail("arm name %q: want lower-case letters, digits, '.', '_' or '-'", a.Name)
		} else if armNames[a.Name] {
			fail("arm %q is named twice", a.Name)
		}
		armNames[a.Name] = true
		if a.Tuning != "config/tuning.defaults.json" && !strings.HasPrefix(a.Tuning, "config/experiments/") {
			fail("arm %q: tuning %q must be config/tuning.defaults.json or a parameter set under config/experiments/", a.Name, a.Tuning)
		} else if !exists(root, a.Tuning) {
			fail("arm %q: tuning %s does not exist", a.Name, a.Tuning)
		}
		for _, r := range a.Replay {
			if strings.TrimSpace(r) == "" || strings.ContainsAny(r, " ,") {
				fail("arm %q: replay option %q must be one -experiment name", a.Name, r)
			}
		}
		for _, e := range a.Export {
			if _, ok := ExportOptions[e]; !ok {
				fail("arm %q: export option %q is not one of %s", a.Name, e, sortedKeys(ExportOptions))
			}
		}
	}
	if c := spec.Cases; c != nil {
		corpus, ok := Corpora[c.Corpus]
		if !ok {
			fail("corpus %q is not one of %s", c.Corpus, sortedKeys(Corpora))
		}
		if c.Count <= 0 {
			fail("cases.count must be positive")
		} else if ok && c.Count > corpus.Cases {
			fail("cases.count %d exceeds corpus %s's %d cases", c.Count, c.Corpus, corpus.Cases)
		}
		if len(c.Sites) > 0 && len(c.Sites) != c.Count {
			fail("cases.sites lists %d sites but count is %d", len(c.Sites), c.Count)
		}
		seen := map[string]bool{}
		for _, site := range c.Sites {
			if !namePattern.MatchString(site) {
				fail("site %q: want lower-case letters, digits, '.', '_' or '-'", site)
			} else if seen[site] {
				fail("site %q is listed twice", site)
			}
			seen[site] = true
		}
	}
	if len(spec.Steps) == 0 {
		fail("no steps")
	}
	stepIDs := map[string]int{}
	for i, st := range spec.Steps {
		if st.ID == "" {
			fail("step %d has no id", i+1)
			continue
		}
		if !namePattern.MatchString(st.ID) {
			fail("step id %q: want lower-case letters, digits, '.', '_' or '-'", st.ID)
		}
		if _, dup := stepIDs[st.ID]; dup {
			fail("step %q is defined twice", st.ID)
		}
		stepIDs[st.ID] = i
	}
	for i, st := range spec.Steps {
		errs = append(errs, st.validate(i, spec, stepIDs)...)
	}
	if spec.Actual != nil && spec.Actual.Worker != "" {
		if _, ok := WorkerClasses[spec.Actual.Worker]; !ok {
			fail("actual.worker %q is not one of %s", spec.Actual.Worker, sortedKeys(WorkerClasses))
		}
	}
	return errs
}

func (st Step) validate(index int, spec *Spec, stepIDs map[string]int) []error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("step %q: "+format, append([]any{st.ID}, args...)...))
	}
	class := ClassifyKind(st.Kind)
	if class == KindUnknown {
		fail("kind %q is not one of %s", st.Kind, strings.Join(KnownKinds(), ", "))
	}
	for _, a := range st.After {
		if a == "-" {
			continue
		}
		j, ok := stepIDs[a]
		if !ok {
			fail("after %q: no such step", a)
		} else if j >= index {
			fail("after %q: a step may wait only for steps listed before it", a)
		}
	}
	for _, o := range st.Over {
		switch o {
		case "arms":
			if len(spec.Arms) == 0 {
				fail("fans out over arms, but the experiment has none")
			}
		case "cases":
			if spec.Cases == nil {
				fail("fans out over cases, but the experiment has none")
			}
		default:
			fail("over %q: want arms or cases", o)
		}
	}
	e := st.Estimate
	if e.Worker != "" {
		if _, ok := WorkerClasses[e.Worker]; !ok {
			fail("estimate.worker %q is not one of %s", e.Worker, sortedKeys(WorkerClasses))
		}
	}
	switch class {
	case KindManual:
		if e.Human <= 0 {
			fail("a manual step needs estimate.human")
		}
		if e.Each > 0 || e.Total > 0 || len(st.Over) > 0 {
			fail("a manual step takes a person's time (estimate.human), not machine time or a fan-out")
		}
	case KindJob, KindPlanned:
		if (e.Each > 0) == (e.Total > 0) {
			fail("a job step needs exactly one of estimate.each and estimate.total")
		}
		if e.Each > 0 && len(st.Over) == 0 {
			fail("estimate.each is per fan-out unit, so the step must fan out (over: arms, cases)")
		}
		if len(st.Gate) > 0 {
			fail("a gate belongs on a manual step")
		}
	}
	return errs
}

// dependencyCycle returns one cycle among depends_on edges, or nil.
func (s *Set) dependencyCycle() []string {
	const (
		white = iota
		grey
		black
	)
	colour := map[string]int{}
	var stack []string
	var found []string
	var visit func(id string) bool
	visit = func(id string) bool {
		colour[id] = grey
		stack = append(stack, id)
		spec := s.Get(id)
		if spec != nil {
			for _, d := range spec.DependsOn {
				if s.Get(d) == nil {
					continue
				}
				switch colour[d] {
				case grey:
					for i, v := range stack {
						if v == d {
							found = append(append([]string{}, stack[i:]...), d)
							return true
						}
					}
				case white:
					if visit(d) {
						return true
					}
				}
			}
		}
		stack = stack[:len(stack)-1]
		colour[id] = black
		return false
	}
	for _, spec := range s.Specs {
		if colour[spec.ID] == white && visit(spec.ID) {
			return found
		}
	}
	return nil
}

func exists(root, rel string) bool {
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") {
		return false
	}
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}
