package eligibility

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// The protection registry is what every frozen split beneath the annotation
// root holds out, and what every lineage has tuned on, gathered without the
// caller naming a split. A frozen split binds only a run given the split;
// the registry is how a run given none is still refused what a split holds
// out, and how a new lineage still sees what an older one tuned on.

// splitFilePattern is a split file name the registry reads, as the freeze
// API writes them.
var splitFilePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.json$`)

// maxSplitFiles bounds the registry scan.
const maxSplitFiles = 2000

// CaptureSpan is a stretch of one capture: by the SHA-256 of its bytes where
// anything verified them, and by basename always. FirstNs and LastNs are
// sample times; GuardSeconds widens them both ways.
type CaptureSpan struct {
	Basename     string   `json:"basename,omitempty"`
	SHA256s      []string `json:"sha256s,omitempty"`
	FirstNs      int64    `json:"first_ns,string"`
	LastNs       int64    `json:"last_ns,string"`
	GuardSeconds float64  `json:"guard_seconds"`
	// Whole marks a span that covers the entire capture: a corpus case.
	Whole bool `json:"whole,omitempty"`
}

// ProtectedSpan is a stretch a frozen split holds out, or one a lineage
// tuned on.
type ProtectedSpan struct {
	CaptureSpan
	SplitName   string `json:"split_name"`
	SplitDigest string `json:"split_digest"`
	PackDigest  string `json:"pack_digest,omitempty"`
	CaseID      string `json:"case_id,omitempty"`
	Objects     int    `json:"objects,omitempty"`
}

// Registry is the protection read from the splits directory.
type Registry struct {
	// HeldOut are the stretches some frozen split holds out.
	HeldOut []ProtectedSpan `json:"held_out"`
	// Tuned are the stretches any lineage's Tuned ledger records.
	Tuned []ProtectedSpan `json:"tuned"`
	// Splits is how many split files were read.
	Splits int `json:"splits"`
	// Unreadable lists split files that could not be read. Something they
	// protect cannot be known, so while any is listed every development
	// request fails closed.
	Unreadable []string `json:"unreadable,omitempty"`
	// WeakIdentity counts held-out stretches known only by basename: a
	// renamed copy of their capture would not be recognised. Binding the
	// label set to its capture (velocity lidar labels bind) records the hash.
	WeakIdentity int `json:"weak_identity"`
	// Digest is over the split files read, by name and content digest.
	Digest string `json:"digest"`
}

// ReadRegistry reads every frozen split in dir. extra adds capture hashes
// verified after a pack was frozen, by pack digest: a label set bound to its
// capture later still protects renamed copies. A missing directory is an
// empty registry.
func ReadRegistry(dir string, extra map[string][]string) (*Registry, error) {
	reg := &Registry{HeldOut: []ProtectedSpan{}, Tuned: []ProtectedSpan{}}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		reg.Digest = digestBytes(nil)
		return reg, nil
	}
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	read := 0
	for _, e := range entries {
		if e.IsDir() || !splitFilePattern.MatchString(e.Name()) {
			continue
		}
		read++
		if read > maxSplitFiles {
			reg.Unreadable = append(reg.Unreadable, fmt.Sprintf("more than %d split files: the rest were not read", maxSplitFiles))
			break
		}
		f, err := annotation.LoadFrozenSplit(filepath.Join(dir, e.Name()))
		if err != nil {
			reg.Unreadable = append(reg.Unreadable, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		reg.Splits++
		fmt.Fprintf(h, "%s\n%s\n", e.Name(), f.SplitDigest)
		reg.addSplit(e.Name(), f, extra)
	}
	for _, s := range reg.HeldOut {
		if len(s.SHA256s) == 0 {
			reg.WeakIdentity++
		}
	}
	reg.Digest = "sha256:" + hex.EncodeToString(h.Sum(nil))
	return reg, nil
}

func (reg *Registry) addSplit(name string, f *annotation.FrozenSplit, extra map[string][]string) {
	roles := map[string]annotation.SplitRole{}
	for _, p := range f.Partitions {
		roles[p.Name] = p.Role
	}
	for _, p := range f.Packs {
		held := 0
		for _, o := range p.Objects {
			if roles[o.Partition] == annotation.SplitRoleHeldOut {
				held++
			}
		}
		if held == 0 {
			continue
		}
		reg.HeldOut = append(reg.HeldOut, ProtectedSpan{
			CaptureSpan: CaptureSpan{
				Basename: p.Source.PCAPBasename, SHA256s: hashes(p.Captures, extra[p.PackDigest]),
				FirstNs: p.Source.FirstSampleNs, LastNs: p.Source.LastSampleNs, GuardSeconds: f.GuardSeconds,
			},
			SplitName: name, SplitDigest: f.SplitDigest, PackDigest: p.PackDigest, CaseID: p.CaseID, Objects: held,
		})
	}
	for _, c := range f.Cases {
		if c.Role != annotation.SplitRoleHeldOut {
			continue
		}
		for _, cp := range c.Captures {
			reg.HeldOut = append(reg.HeldOut, ProtectedSpan{
				CaptureSpan: CaptureSpan{Basename: cp.Basename, SHA256s: hashes([]annotation.CaseCapture{cp}, nil), Whole: true},
				SplitName:   name, SplitDigest: f.SplitDigest, CaseID: c.CaseID,
			})
		}
	}
	for _, s := range f.Tuned.Spans {
		basename := strings.TrimPrefix(s.Source, "pcap:")
		if basename == s.Source {
			basename = ""
		}
		reg.Tuned = append(reg.Tuned, ProtectedSpan{
			CaptureSpan: CaptureSpan{
				Basename: basename, SHA256s: extra[s.PackDigest],
				FirstNs: s.FirstSampleNs, LastNs: s.LastSampleNs, GuardSeconds: s.GuardSeconds,
			},
			SplitName: name, SplitDigest: f.SplitDigest, PackDigest: s.PackDigest, Objects: len(s.ObjectIDs),
		})
	}
	for _, c := range f.Tuned.Cases {
		for _, cp := range c.Captures {
			reg.Tuned = append(reg.Tuned, ProtectedSpan{
				CaptureSpan: CaptureSpan{Basename: cp.Basename, SHA256s: hashes([]annotation.CaseCapture{cp}, nil), Whole: true},
				SplitName:   name, SplitDigest: f.SplitDigest, CaseID: c.CaseID,
			})
		}
	}
}

func hashes(captures []annotation.CaseCapture, extra []string) []string {
	set := map[string]bool{}
	for _, c := range captures {
		if c.SHA256 != "" {
			set[c.SHA256] = true
		}
	}
	for _, s := range extra {
		if s != "" {
			set[s] = true
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// spanMatch is how an input relates to a protected stretch's capture.
type spanMatch int

const (
	matchNone spanMatch = iota
	// matchSame: the input is the stretch's capture, by hash, or by name
	// where no hash was ever verified.
	matchSame
	// matchConflict: the input shares the capture's name but not any of
	// its verified bytes.
	matchConflict
)

func (s CaptureSpan) match(in Identity) spanMatch {
	if in.SHA256 != "" {
		for _, h := range s.SHA256s {
			if h == in.SHA256 {
				return matchSame
			}
		}
	}
	if s.Basename == "" || s.Basename != in.Basename {
		return matchNone
	}
	if len(s.SHA256s) == 0 {
		return matchSame
	}
	return matchConflict
}

// overlaps reports whether [start, end] meets the span widened by its guard.
// A zero interval is the whole capture.
func (s CaptureSpan) overlaps(start, end int64) bool {
	if s.Whole || (start == 0 && end == 0) {
		return true
	}
	guard := int64(s.GuardSeconds * 1e9)
	return start <= s.LastNs+guard && end >= s.FirstNs-guard
}
