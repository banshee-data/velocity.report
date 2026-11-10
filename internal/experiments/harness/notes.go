package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/experiments"
	"go.yaml.in/yaml/v3"
)

const NoteLimit = 64 * 1024

type NoteHeader struct {
	Sources      map[string]string `yaml:"source_digests,omitempty"`
	Schema       int               `yaml:"harness_note"`
	Slug         string            `yaml:"slug"`
	RunID        string            `yaml:"run_id"`
	Commit       string            `yaml:"commit"`
	SpecDigest   string            `yaml:"spec_digest"`
	InputDigest  string            `yaml:"input_digest"`
	BuildVersion string            `yaml:"build_version"`
	Host         string            `yaml:"host"`
	Class        string            `yaml:"class"`
	Started      string            `yaml:"started"`
	Finished     string            `yaml:"updated"`
	State        string            `yaml:"state"`
	Complete     bool              `yaml:"complete"`
	Evidence     string            `yaml:"evidence"`
	Arms         []experiments.Arm `yaml:"arms"`
	Digests      map[string]string `yaml:"summary_digests"`
}

func writeNote(h Host, r *Run, l *experiments.Ledger, sp *experiments.Spec) error {
	recs, e := l.Records(sp.ID)
	if e != nil {
		return e
	}
	head := NoteHeader{Schema: 1, Slug: r.Slug, RunID: r.ID, Commit: r.Commit, SpecDigest: r.SpecDigest, InputDigest: r.InputDigest, BuildVersion: r.BuildVersion, Host: r.Host, Class: r.Class, Started: r.Started.UTC().Format("2006-01-02T15:04:05Z"), Finished: r.Updated.UTC().Format("2006-01-02T15:04:05Z"), State: r.State, Complete: r.State == "complete", Evidence: filepath.Join(h.EvidenceRoot, "runs", r.Slug, r.ID), Arms: r.Arms, Digests: r.SummaryDigests, Sources: r.Sources}
	b, e := yaml.Marshal(head)
	if e != nil {
		return e
	}
	var s strings.Builder
	s.WriteString("---\n")
	s.Write(b)
	s.WriteString("---\n\n# " + r.Slug + " run " + r.ID + "\n\n")
	s.WriteString("| Unit | State | Attempt | Wall seconds | Estimate seconds | Measures or failure |\n| --- | --- | ---: | ---: | ---: | --- |\n")
	for _, st := range sp.Steps {
		for _, u := range sp.Units(st) {
			rec := recs[u.ID()]
			state := string(rec.State)
			if state == "" {
				state = "pending"
			}
			var values []string
			keys := make([]string, 0, len(r.Measures[u.ID()]))
			for k := range r.Measures[u.ID()] {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				values = append(values, fmt.Sprintf("%s=%.6g", k, r.Measures[u.ID()][k]))
			}
			if rec.Note != "" {
				values = append(values, rec.Note)
			}
			fmt.Fprintf(&s, "| %s | %s | %d | %.3f | %.0f | %s |\n", u.ID(), state, rec.Attempt, rec.WallSeconds, sp.UnitEstimate(st).Seconds(), cell(strings.Join(values, "; ")))
		}
	}
	s.WriteString("\nThis note is a measurement record, not a report. It contains no interpretation, adopts nothing, and leaves the experiment's rule and physical validation to the report.\n")
	raw := []byte(s.String())
	if _, e := CheckNote(raw); e != nil {
		return e
	}
	return atomicFile(filepath.Join(h.EvidenceRoot, "notes", r.Slug, r.ID+".md"), raw)
}
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", "/")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

var noteSlug = regexp.MustCompile(`^[a-z][a-z0-9]{3,5}[0-9]{4}$`)
var noteSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)
var noteDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// CheckNote validates the bounded summary before it can be checked into Git.
func CheckNote(raw []byte) (NoteHeader, error) {
	var h NoteHeader
	if len(raw) > NoteLimit {
		return h, fmt.Errorf("run note exceeds %d bytes", NoteLimit)
	}
	parts := strings.SplitN(string(raw), "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		return h, fmt.Errorf("run note needs a YAML header")
	}
	d := yaml.NewDecoder(strings.NewReader(parts[1]))
	d.KnownFields(true)
	if e := d.Decode(&h); e != nil {
		return h, e
	}
	if h.Schema != 1 || !noteSlug.MatchString(h.Slug) || !noteSHA.MatchString(h.Commit) || !noteDigest.MatchString(h.SpecDigest) || !noteDigest.MatchString(h.InputDigest) || h.RunID == "" || strings.ContainsAny(h.RunID, "/\\") || h.BuildVersion == "" || h.Host == "" || h.Evidence == "" {
		return h, fmt.Errorf("invalid run-note provenance")
	}
	started, e := time.Parse("2006-01-02T15:04:05Z", h.Started)
	if e != nil {
		return h, fmt.Errorf("invalid note start timestamp")
	}
	updated, e := time.Parse("2006-01-02T15:04:05Z", h.Finished)
	if e != nil || updated.Before(started) {
		return h, fmt.Errorf("invalid note update timestamp")
	}
	if _, ok := experiments.WorkerClasses[h.Class]; !ok {
		return h, fmt.Errorf("unknown note worker class")
	}
	digests := []map[string]string{h.Digests, h.Sources}
	for _, group := range digests {
		for _, v := range group {
			if !noteDigest.MatchString(v) {
				return h, fmt.Errorf("invalid summary digest")
			}
		}
	}
	if h.Complete && (h.State != "complete" || len(h.Digests) == 0) {
		return h, fmt.Errorf("complete run note needs completed summaries")
	}
	return h, nil
}
func hasRunNote(root, slug string) bool {
	entries, e := os.ReadDir(filepath.Join(root, experiments.Dir, "runs", slug))
	if e != nil {
		return false
	}
	for _, f := range entries {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		p := filepath.Join(root, experiments.Dir, "runs", slug, f.Name())
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		h, e := CheckNote(b)
		if e == nil && h.Slug == slug && h.Complete {
			if _, e = git(context.Background(), root, "ls-files", "--error-unmatch", "--", p); e == nil {
				return true
			}
		}
	}
	return false
}
