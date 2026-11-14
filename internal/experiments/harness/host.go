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
	CaptureRoot   string               `json:"capture_root"`
	CaptureSubdir string               `json:"capture_subdir"`
	DiskFloorGB   float64              `json:"disk_floor_gb"`
	DailyHours    float64              `json:"daily_hours"`
	Reclaim       bool                 `json:"reclaim"`
	Corpora       map[string]Corpus    `json:"corpora"`
	References    map[string]Reference `json:"references"`
}

type Corpus struct {
	Corpus string `json:"corpus"`
	Index  string `json:"index"`
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
