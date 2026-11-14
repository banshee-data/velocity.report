// Package harness runs the mechanical part of approved experiments offline.
package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Host is machine configuration, kept outside Git. No command comes from it.
type Host struct {
	FetchOnTick   bool                 `json:"fetch_on_tick"`
	Publish       *Publication         `json:"publish,omitempty"`
	Worker        string               `json:"worker"`
	Class         string               `json:"class"`
	Cores         int                  `json:"cores"`
	Capabilities  []string             `json:"capabilities"`
	EvidenceRoot  string               `json:"evidence_root"`
	StageDir      string               `json:"stage_dir"`
	BuildRoot     string               `json:"build_root,omitempty"`
	StageAttempts bool                 `json:"stage_attempts,omitempty"`
	CaptureRoot   string               `json:"capture_root"`
	CaptureSubdir string               `json:"capture_subdir"`
	DiskFloorGB   float64              `json:"disk_floor_gb"`
	DailyHours    float64              `json:"daily_hours"`
	Reclaim       bool                 `json:"reclaim"`
	Corpora       map[string]Corpus    `json:"corpora"`
	References    map[string]Reference `json:"references"`
}

type Corpus struct {
	Corpus      string `json:"corpus"`
	Index       string `json:"index"`
	CaptureRoot string `json:"capture_root,omitempty"`
}

type Reference struct {
	Pack     string `json:"pack"`
	Manifest string `json:"manifest"`
	Split    string `json:"split"`
	ADB      string `json:"a_db"`
	BDB      string `json:"b_db"`
}

func ReadHost(path string) (Host, error) {
	var h Host
	raw, err := os.ReadFile(path)
	if err != nil {
		return h, err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&h); err != nil {
		return h, err
	}
	if h.Worker == "" || h.Class != "arrow" && h.Class != "m1pro" && h.Class != "swan" {
		return h, fmt.Errorf("host needs a worker and class arrow, m1pro or swan")
	}
	if h.Cores < 1 || h.DiskFloorGB < 0 || h.DailyHours <= 0 || h.DailyHours > 24 {
		return h, fmt.Errorf("host needs cores >= 1, nonnegative disk floor and daily_hours in (0,24]")
	}
	for _, p := range []string{h.EvidenceRoot, h.StageDir, h.CaptureRoot} {
		if !filepath.IsAbs(p) {
			return h, fmt.Errorf("host paths must be absolute: %q", p)
		}
	}
	if h.BuildRoot != "" && !filepath.IsAbs(h.BuildRoot) {
		return h, fmt.Errorf("host paths must be absolute: %q", h.BuildRoot)
	}
	for name, c := range h.Corpora {
		if c.CaptureRoot != "" && !filepath.IsAbs(c.CaptureRoot) {
			return h, fmt.Errorf("corpus %s capture_root must be absolute", name)
		}
	}
	if e := validatePublication(h.Publish); e != nil {
		return h, e
	}
	return h, nil
}

// BuildBase is the directory that holds builds/<commit>. Builds are a
// rebuildable cache that needs exclusive file creation, so it can sit on local
// disk while the evidence root is on a share that refuses O_EXCL.
func (h Host) BuildBase() string {
	if h.BuildRoot != "" {
		return h.BuildRoot
	}
	return h.EvidenceRoot
}

// captureHost applies a corpus-specific directory containing indexed captures.
// An override replaces both the legacy root and its subdirectory.
func captureHost(h Host, c Corpus) Host {
	if c.CaptureRoot != "" {
		h.CaptureRoot = c.CaptureRoot
		h.CaptureSubdir = ""
	}
	return h
}

// scoringReference uses the same arm override and step fallback for selection
// and execution. Capability declarations alone do not supply scoring inputs.
func (h Host) scoringReference(slug, step, arm string) (Reference, error) {
	key := slug + "/" + step + "/" + arm
	ref, ok := h.References[key]
	if !ok {
		ref, ok = h.References[slug+"/"+step]
	}
	if !ok {
		return Reference{}, fmt.Errorf("no frozen reference mapping for %s", key)
	}
	return ref, nil
}

// checkScoringInputs is a cheap preflight before building or replaying. The
// evaluator remains responsible for frozen pins, reviewed truth and DB versions.
func (ref Reference) checkScoringInputs() error {
	if strings.TrimSpace(ref.Split) == "" {
		return fmt.Errorf("reference split must be named")
	}
	for _, input := range []struct {
		name, path string
		directory  bool
	}{
		{"pack", ref.Pack, true}, {"manifest", ref.Manifest, false},
		{"a_db", ref.ADB, false}, {"b_db", ref.BDB, false},
	} {
		if !filepath.IsAbs(input.path) {
			return fmt.Errorf("reference %s path must be absolute", input.name)
		}
		info, err := os.Stat(input.path)
		if err != nil {
			return fmt.Errorf("reference %s: %w", input.name, err)
		}
		if input.directory && !info.IsDir() || !input.directory && !info.Mode().IsRegular() {
			return fmt.Errorf("reference %s has wrong file type", input.name)
		}
		f, err := os.Open(input.path)
		if err != nil {
			return fmt.Errorf("reference %s: %w", input.name, err)
		}
		info, err = f.Stat()
		f.Close()
		if err != nil {
			return fmt.Errorf("reference %s: %w", input.name, err)
		}
		if input.directory && !info.IsDir() || !input.directory && !info.Mode().IsRegular() {
			return fmt.Errorf("reference %s has wrong file type", input.name)
		}
		if !input.directory && info.Size() == 0 {
			return fmt.Errorf("reference %s is empty", input.name)
		}
	}
	return nil
}
