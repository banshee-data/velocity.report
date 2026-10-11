package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestExperimentConfigsDeclareEveryChangeFromTheDefaults is the anti-drift
// guard for config/experiments/. Each shipped parameter set loads under the
// strict loader, differs from the defaults, and has a row in the
// directory's README naming every key it changes. A set copied from an
// older defaults file then fails here, listing the keys it carries that its
// row does not explain, rather than running quietly as something other
// than the arm it is named for.
func TestExperimentConfigsDeclareEveryChangeFromTheDefaults(t *testing.T) {
	dir := repoFile(t, "config/experiments")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("config/experiments/README.md: %v", err)
	}
	defaults := MustLoadDefaultConfig()
	base := flatConfig(t, defaults)
	var seen int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		seen++
		t.Run(e.Name(), func(t *testing.T) {
			path := filepath.Join(dir, e.Name())
			cfg, err := LoadTuningConfig(path)
			if err != nil {
				t.Fatalf("loading %s: %v", path, err)
			}
			changed := changedConfigPaths(base, flatConfig(t, cfg))
			if len(changed) == 0 {
				t.Fatalf("%s is the defaults; a parameter set changes something", path)
			}
			row := readmeRow(string(readme), e.Name())
			if row == "" {
				t.Fatalf("config/experiments/README.md has no row for %s; add one naming what it changes and the report it serves", e.Name())
			}
			var undeclared []string
			for _, p := range changed {
				if !strings.Contains(row, p[strings.LastIndex(p, ".")+1:]) {
					undeclared = append(undeclared, p)
				}
			}
			if len(undeclared) > 0 {
				t.Errorf("%s differs from the defaults in %s, which its README row does not name.\nRegenerate it from config/tuning.defaults.json, or name the change in the row if it is intended.",
					path, strings.Join(undeclared, ", "))
			}
		})
	}
	if seen == 0 {
		t.Fatal("config/experiments/ holds no parameter sets")
	}
}

// The shipped solid-body sets change the block and nothing else; the
// general rule above lets a set change any key, so this pins the four
// whose reports compare them against the defaults on the block alone.
func TestSolidBodyExperimentConfigsChangeOnlyTheBlock(t *testing.T) {
	dir := repoFile(t, "config/experiments")
	base := flatConfig(t, MustLoadDefaultConfig())
	for _, name := range []string{
		"solid-body-shadow.json", "solid-body-a2.json",
		"alignment-candidate.json", "geometry-convergence-candidate.json",
	} {
		cfg, err := LoadTuningConfig(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if sb := cfg.L5.CvKfV1.SolidBody; sb == nil || !sb.Enabled {
			t.Errorf("%s does not switch the solid body on", name)
		}
		if got := changedConfigPaths(base, flatConfig(t, cfg)); !reflect.DeepEqual(got, []string{"l5.cv_kf_v1.solid_body"}) {
			t.Errorf("%s changes %v; it should change only l5.cv_kf_v1.solid_body", name, got)
		}
	}
}

// flatConfig is a config as dotted leaf paths to JSON values.
func flatConfig(t *testing.T, cfg *TuningConfig) map[string]interface{} {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var nested map[string]interface{}
	if err := json.Unmarshal(data, &nested); err != nil {
		t.Fatal(err)
	}
	flat := map[string]interface{}{}
	var walk func(prefix string, v interface{})
	walk = func(prefix string, v interface{}) {
		obj, ok := v.(map[string]interface{})
		if !ok {
			flat[prefix] = v
			return
		}
		for k, child := range obj {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			walk(p, child)
		}
	}
	walk("", nested)
	return flat
}

// changedConfigPaths lists the leaf paths whose values differ, folding an
// optional block present on one side only into the block's own path.
func changedConfigPaths(base, cfg map[string]interface{}) []string {
	set := map[string]struct{}{}
	note := func(p string, other map[string]interface{}) {
		// A path under a block the other side lacks names the block.
		for i := strings.LastIndex(p, "."); i > 0; i = strings.LastIndex(p[:i], ".") {
			parent := p[:i]
			if !hasPrefixPath(other, parent) {
				p = parent
			} else {
				break
			}
		}
		set[p] = struct{}{}
	}
	for p, v := range cfg {
		if bv, ok := base[p]; !ok {
			note(p, base)
		} else if !reflect.DeepEqual(v, bv) {
			set[p] = struct{}{}
		}
	}
	for p := range base {
		if _, ok := cfg[p]; !ok {
			note(p, cfg)
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func hasPrefixPath(flat map[string]interface{}, prefix string) bool {
	for p := range flat {
		if strings.HasPrefix(p, prefix+".") {
			return true
		}
	}
	return false
}

// readmeRow is the README table row that names file, or "".
func readmeRow(readme, file string) string {
	for _, line := range strings.Split(readme, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "|") && strings.Contains(line, file) {
			return line
		}
	}
	return ""
}
