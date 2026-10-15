// Package experiments reads the experiment specs under data/experiments: one
// Markdown file per experiment, its header a YAML block that says what the
// experiment asks, what it depends on, what it runs, and how long that should
// take. The body is the proposal and, once run, the pointer to its report.
//
// A spec is intent, kept in Git. Where an experiment is in its execution
// (queued, running, waiting on a person) is the hub's state and never written
// back here; raw outputs stay off the repository as AGENTS.md requires. This
// package validates specs, orders them by dependency, totals their estimates
// and renders the dependency graph; expanding a step into jobs is the job
// runner's business (internal/lidar/jobs).
package experiments

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Status is where a spec is in its authoring life. Execution state is not a
// status: it lives on the hub.
type Status string

const (
	// StatusProposed is a draft: written from a question, not yet agreed.
	StatusProposed Status = "proposed"
	// StatusApproved has the owner's agreement and may be scheduled once
	// its dependencies are answered.
	StatusApproved Status = "approved"
	// StatusAnswered has a merged report; the spec names it.
	StatusAnswered Status = "answered"
	// StatusRetired will not run: superseded, or the question went away.
	// The spec says why.
	StatusRetired Status = "retired"
)

func (s Status) valid() bool {
	switch s {
	case StatusProposed, StatusApproved, StatusAnswered, StatusRetired:
		return true
	}
	return false
}

// ReclaimPolicy says who may pick up a step whose worker stopped renewing
// its lease.
type ReclaimPolicy string

const (
	// ReclaimAny lets any worker configured to reclaim orphans, and holding
	// the step's capabilities, take the work as a new attempt. The default.
	ReclaimAny ReclaimPolicy = "any"
	// ReclaimOrigin waits for the worker that started it: for work tied to
	// one machine's disks or state.
	ReclaimOrigin ReclaimPolicy = "origin"
	// ReclaimNever leaves an orphan for the operator to resolve.
	ReclaimNever ReclaimPolicy = "never"
)

func (p ReclaimPolicy) valid() bool {
	switch p {
	case "", ReclaimAny, ReclaimOrigin, ReclaimNever:
		return true
	}
	return false
}

// Spec is the header of one experiment file.
type Spec struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Status  Status `yaml:"status"`
	Owner   string `yaml:"owner"`
	Created string `yaml:"created"`
	// Asks names the questions the experiment answers: a repository path to
	// the Markdown that poses each (a briefing, a plan, a report), with an
	// optional #anchor, or a decision id such as D-29.
	Asks []string `yaml:"asks"`
	// DependsOn names experiments whose answers this one needs first.
	DependsOn []string `yaml:"depends_on"`
	// Report is the repository path of the report that answers it; required
	// once answered, and the file must exist.
	Report string `yaml:"report"`
	// Reason says why a retired experiment will not run.
	Reason   string        `yaml:"reason"`
	Requires Requires      `yaml:"requires"`
	Reclaim  ReclaimPolicy `yaml:"reclaim"`
	Arms     []Arm         `yaml:"arms"`
	Cases    *Cases        `yaml:"cases"`
	Steps    []Step        `yaml:"steps"`
	// Actual records what an answered experiment took, for calibrating the
	// estimates of the next one. Optional.
	Actual *Actual `yaml:"actual"`

	// Path is the file the spec was read from, relative to the repository
	// root. Not part of the header.
	Path string `yaml:"-"`
}

// Requires is what a worker must offer to run the experiment's job steps.
type Requires struct {
	Capabilities []string `yaml:"capabilities"`
	// DiskGB is the free space a worker needs on its scratch disk before it
	// starts a step: the transient evidence a step writes before it moves it.
	DiskGB float64 `yaml:"disk_gb"`
	Cores  int     `yaml:"cores"`
}

// Arm is one configuration an experiment compares: a checked-in tuning
// config and any replay options switched on with it.
type Arm struct {
	Name string `yaml:"name"`
	// Tuning is config/tuning.defaults.json or a parameter set under
	// config/experiments/, as AGENTS.md requires of every arm.
	Tuning string `yaml:"tuning"`
	// Replay names default-off replay options (the -experiment names).
	Replay []string `yaml:"replay"`
}

// Cases is the data an experiment's steps fan out over.
type Cases struct {
	// Corpus names the case set: see Corpora.
	Corpus string   `yaml:"corpus"`
	Sites  []string `yaml:"sites"`
	// Count is how many cases a step runs over, for the estimate; when Sites
	// is given it must agree.
	Count int `yaml:"count"`
}

// Step is one unit of an experiment: a job kind a worker runs, a planned
// kind no worker can run yet, or a manual step a person closes.
type Step struct {
	ID   string `yaml:"id"`
	Kind string `yaml:"kind"`
	// After lists the steps this one waits for; empty means the step
	// before it, and "-" means none.
	After []string `yaml:"after"`
	// Over fans the step out: "arms", "cases", or both.
	Over     []string       `yaml:"over"`
	Params   map[string]any `yaml:"params"`
	Estimate Estimate       `yaml:"estimate"`
	// Gate is a manual step's completion check, numeric thresholds the hub
	// tests before it lets the experiment continue (HINT's label gates).
	Gate map[string]float64 `yaml:"gate"`
	Note string             `yaml:"note"`
}

// Estimate is an authored expectation of a step's cost.
type Estimate struct {
	// Each is machine time per fan-out unit (one arm on one case).
	Each Duration `yaml:"each"`
	// Total is machine time for the whole step, for steps that do not fan
	// out or are easier to estimate whole.
	Total Duration `yaml:"total"`
	// Human is a person's time on a manual step.
	Human Duration `yaml:"human"`
	// Worker is the class the estimate was made for; see WorkerClasses.
	Worker string `yaml:"worker"`
}

// Actual is a measured outcome, kept for calibration.
type Actual struct {
	Wall   Duration `yaml:"wall"`
	Worker string   `yaml:"worker"`
	Note   string   `yaml:"note"`
}

// Duration is a time.Duration written as Go writes one: "80s", "2h30m".
// There is no day unit on purpose: a person's day and a machine's are not
// the same length, so estimates are written in hours.
type Duration time.Duration

// UnmarshalYAML reads a duration string.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	v, err := ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// ParseDuration is time.ParseDuration, refusing negative values.
func ParseDuration(s string) (time.Duration, error) {
	v, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil || v < 0 {
		return 0, fmt.Errorf("duration %q: want e.g. 90s, 45m or 2h30m (hours, not days: a person's day is not 24h)", s)
	}
	return v, nil
}

// String writes the duration compactly: 1h20m, 45m, 30s.
func (d Duration) String() string { return FormatDuration(time.Duration(d)) }

// FormatDuration writes a duration in the largest whole units, dropping
// zero parts: 26h, 1h20m, 45m, 30s.
func FormatDuration(v time.Duration) string {
	if v <= 0 {
		return "0"
	}
	v = v.Round(time.Second)
	h := int(v / time.Hour)
	m := int((v % time.Hour) / time.Minute)
	sec := int((v % time.Minute) / time.Second)
	var b strings.Builder
	if h > 0 {
		fmt.Fprintf(&b, "%dh", h)
	}
	if m > 0 {
		fmt.Fprintf(&b, "%dm", m)
	}
	if sec > 0 && h == 0 {
		fmt.Fprintf(&b, "%ds", sec)
	}
	return b.String()
}

// ReadFile parses the header of one experiment file. The header is the YAML
// between a first line of "---" and the next "---" line; unknown keys are an
// error, so a misspelt field never silently becomes its zero value.
func ReadFile(path string) (*Spec, error) {
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
	var s Spec
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &s, nil
}

func frontMatter(raw []byte) ([]byte, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return nil, fmt.Errorf("no YAML header: the file must start with a line of ---")
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			end = len(rest) - len("\n---")
		} else {
			return nil, fmt.Errorf("YAML header not closed by a line of ---")
		}
	}
	return []byte(rest[:end]), nil
}
