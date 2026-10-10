package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExperimentConfigsAreTheDefaultsPlusASolidBodyBlock is the anti-drift
// guard for config/experiments/: each shipped parameter set is the defaults
// with one solid-body block, so a run from it and a run from the defaults
// differ in the block and nothing else. A set that drifts, or one with the
// estimator off, is not an experiment but a second defaults file.
func TestExperimentConfigsAreTheDefaultsPlusASolidBodyBlock(t *testing.T) {
	dir := repoFile(t, "config/experiments")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defaults := MustLoadDefaultConfig()
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
			sb := cfg.L5.CvKfV1.SolidBody
			if sb == nil || !sb.Enabled {
				t.Fatalf("%s does not switch the solid body on; an experiment set carries a block", path)
			}
			without := *cfg
			l5 := *cfg.L5.CvKfV1
			l5.SolidBody = nil
			without.L5.CvKfV1 = &l5
			if without.Fingerprint() != defaults.Fingerprint() {
				t.Errorf("%s differs from the defaults outside its solid-body block.\nRegenerate it from config/tuning.defaults.json; the block is the only intended difference.\n got %s\nwant %s",
					path, mustJSON(t, &without), mustJSON(t, defaults))
			}
			if cfg.Fingerprint() == defaults.Fingerprint() {
				t.Errorf("%s has the defaults' fingerprint, so its block changes nothing", path)
			}
		})
	}
	if seen == 0 {
		t.Fatal("config/experiments/ holds no parameter sets")
	}
}
