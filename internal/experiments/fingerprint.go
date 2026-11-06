package experiments

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/banshee-data/velocity.report/internal/config"
)

// FingerprintUnknown is the fingerprint an answered arm records when it ran
// before arms were pinned and its report names nothing to compute one from.
// Nothing is invented: the arm says it does not know.
const FingerprintUnknown = "unknown"

// PinnedSince is the date arms began carrying fingerprints. An answered spec
// created on or after it must carry one on every arm, so only the specs that
// predate pinning may leave the field out.
const PinnedSince = "2026-11-06"

// fingerprintPattern is what config.TuningConfig.Fingerprint returns: twelve
// lower-case hex digits.
var fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)

// Open reports whether a spec can still run: proposed or approved.
func (s *Spec) Open() bool { return s.Status == StatusProposed || s.Status == StatusApproved }

// TuningFingerprint is the tuning fingerprint of a checked-in tuning file:
// the value a server loading it reports, and the value an arm pins.
func TuningFingerprint(root, rel string) (string, error) {
	if !exists(root, rel) {
		return "", fmt.Errorf("tuning %s does not exist", rel)
	}
	cfg, err := config.LoadTuningConfig(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("tuning %s: %w", rel, err)
	}
	return cfg.Fingerprint(), nil
}

// fingerprints caches TuningFingerprint per file for one validation pass.
type fingerprints struct {
	root  string
	cache map[string]fpResult
}

type fpResult struct {
	fp  string
	err error
}

func (f *fingerprints) of(rel string) (string, error) {
	if r, ok := f.cache[rel]; ok {
		return r.fp, r.err
	}
	fp, err := TuningFingerprint(f.root, rel)
	f.cache[rel] = fpResult{fp, err}
	return fp, err
}

// validateFingerprints holds the two rules of the experiment queue plan's
// section 12. An open spec pins every arm to its file's current fingerprint
// and is refused when the file has changed under it. An answered spec keeps
// the fingerprints it ran under, and its report must carry each one.
func (spec *Spec) validateFingerprints(root string, fps *fingerprints) []error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	var report string
	reportRead := false
	for _, a := range spec.Arms {
		if a.Fingerprint != "" && a.Fingerprint != FingerprintUnknown && !fingerprintPattern.MatchString(a.Fingerprint) {
			fail("arm %q: fingerprint %q is not twelve lower-case hex digits (or %q)", a.Name, a.Fingerprint, FingerprintUnknown)
			continue
		}
		switch {
		case spec.Open():
			if !exists(root, a.Tuning) {
				continue // reported by the arm's own check
			}
			current, err := fps.of(a.Tuning)
			if err != nil {
				fail("arm %q: %v", a.Name, err)
				continue
			}
			switch a.Fingerprint {
			case "":
				fail("arm %q is not pinned: run `go run ./cmd/tools/experiments pin %s` to record %s's fingerprint (%s)", a.Name, spec.ID, a.Tuning, current)
			case FingerprintUnknown:
				fail("arm %q: an open spec pins what it will run; %q is only for answered arms that ran before pinning", a.Name, FingerprintUnknown)
			case current:
			default:
				fail("arm %q: %s changed under the spec (pinned %s, now %s): restate the spec against the new values and re-pin it (experiments pin %s), or move the arm to a parameter set that holds the old ones", a.Name, a.Tuning, a.Fingerprint, current, spec.ID)
			}
		case spec.Status == StatusAnswered:
			switch a.Fingerprint {
			case "":
				if spec.Created >= PinnedSince {
					fail("arm %q: an answered spec created on or after %s carries the fingerprint it ran under", a.Name, PinnedSince)
				}
			case FingerprintUnknown:
			default:
				if !reportRead {
					reportRead = true
					if exists(root, spec.Report) {
						raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(spec.Report)))
						if err == nil {
							report = string(raw)
						}
					}
				}
				if spec.Report != "" && exists(root, spec.Report) && !strings.Contains(report, a.Fingerprint) {
					fail("arm %q: report %s does not carry the arm's fingerprint %s", a.Name, spec.Report, a.Fingerprint)
				}
			}
		}
	}
	return errs
}

// armsBlock matches the header's arms list and the keys pinning edits.
var (
	armNameLine   = regexp.MustCompile(`^(\s*)- name:\s*(.+?)\s*$`)
	armTuningLine = regexp.MustCompile(`^(\s+)tuning:\s*(.+?)\s*$`)
	armFPLine     = regexp.MustCompile(`^\s+fingerprint:`)
)

// Pin records each arm's current tuning fingerprint in an open spec's file,
// on the line after the arm's tuning, replacing any fingerprint it had. It
// refuses an answered or retired spec: those keep what they ran under. It
// returns whether the file changed.
func Pin(root string, spec *Spec) (bool, error) {
	if !spec.Open() {
		return false, fmt.Errorf("%s is %s: only an open spec is pinned; an answered one keeps the fingerprints it ran under", spec.ID, spec.Status)
	}
	want := map[string]string{}
	for _, a := range spec.Arms {
		fp, err := TuningFingerprint(root, a.Tuning)
		if err != nil {
			return false, fmt.Errorf("%s arm %q: %w", spec.ID, a.Name, err)
		}
		want[a.Name] = fp
	}
	path := filepath.Join(root, filepath.FromSlash(spec.Path))
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return false, fmt.Errorf("%s: no YAML header", spec.Path)
	}
	out := make([]string, 0, len(lines)+len(spec.Arms))
	out = append(out, lines[0])
	inHeader, inArms := true, false
	arm := ""
	pinned := map[string]bool{}
	for _, line := range lines[1:] {
		if inHeader && line == "---" {
			inHeader, inArms = false, false
			out = append(out, line)
			continue
		}
		if !inHeader {
			out = append(out, line)
			continue
		}
		if line == "arms:" {
			inArms = true
			out = append(out, line)
			continue
		}
		if inArms && line != "" && !strings.HasPrefix(line, " ") {
			inArms = false
		}
		if !inArms {
			out = append(out, line)
			continue
		}
		if m := armNameLine.FindStringSubmatch(line); m != nil {
			arm = unquote(m[2])
			out = append(out, line)
			continue
		}
		if armFPLine.MatchString(line) {
			continue // re-emitted after the arm's tuning line
		}
		out = append(out, line)
		if m := armTuningLine.FindStringSubmatch(line); m != nil && arm != "" {
			fp, ok := want[arm]
			if !ok {
				return false, fmt.Errorf("%s: arm %q in the file is not in the parsed spec", spec.Path, arm)
			}
			out = append(out, m[1]+"fingerprint: "+fp)
			pinned[arm] = true
		}
	}
	for name := range want {
		if !pinned[name] {
			return false, fmt.Errorf("%s: could not find arm %q's tuning line to pin (arms must be written as `- name:` then `tuning:`)", spec.Path, name)
		}
	}
	next := strings.Join(out, "\n")
	if next == text {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return false, err
	}
	reread, err := ReadFile(path)
	if err != nil {
		return true, fmt.Errorf("%s after pinning: %w", spec.Path, err)
	}
	for _, a := range reread.Arms {
		if a.Fingerprint != want[a.Name] {
			return true, fmt.Errorf("%s after pinning: arm %q reads %q, want %q", spec.Path, a.Name, a.Fingerprint, want[a.Name])
		}
	}
	return true, nil
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
