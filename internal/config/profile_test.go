package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoFile resolves a repo-relative path from inside the package directory.
func repoFile(t *testing.T, rel string) string {
	t.Helper()
	for _, prefix := range []string{"", "../../", "../../../"} {
		p := filepath.Join(prefix, rel)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatalf("could not locate %s from the package directory", rel)
	return ""
}

func TestParseProfile(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Profile
		wantErr bool
	}{
		{"empty defaults to full", "", ProfileFull, false},
		{"whitespace defaults to full", "   ", ProfileFull, false},
		{"l3-only", "l3-only", ProfileL3Only, false},
		{"detect", "detect", ProfileDetect, false},
		{"track", "track", ProfileTrack, false},
		{"full", "full", ProfileFull, false},
		{"surrounding space is trimmed", " detect ", ProfileDetect, false},
		{"unknown is rejected", "everything", "", true},
		{"case is significant", "Full", "", true},
		{"a layer name is not a profile", "l4", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseProfile(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseProfile(%q) = %q, want an error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseProfile(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseProfile(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestParseProfileErrorNamesTheAlternatives checks the failure is actionable.
// A rejected profile is almost always a typo, and the fix is in the message.
func TestParseProfileErrorNamesTheAlternatives(t *testing.T) {
	_, err := ParseProfile("detekt")
	if err == nil {
		t.Fatal("expected an error for an unknown profile")
	}
	for _, want := range []string{"l3-only", "detect", "track", "full", "detekt"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

// TestRunsLayerDepthOrdering pins the depth semantics every stage boundary
// depends on. A profile that silently ran one layer too far would produce
// numbers that look like a regression and are actually a different workload —
// which is the exact failure this whole mechanism exists to prevent.
func TestRunsLayerDepthOrdering(t *testing.T) {
	tests := []struct {
		profile Profile
		runs    map[int]bool
	}{
		{ProfileL3Only, map[int]bool{3: true, 4: false, 5: false, 6: false}},
		{ProfileDetect, map[int]bool{3: true, 4: true, 5: false, 6: false}},
		{ProfileTrack, map[int]bool{3: true, 4: true, 5: true, 6: false}},
		{ProfileFull, map[int]bool{3: true, 4: true, 5: true, 6: true}},
	}

	for _, tc := range tests {
		t.Run(string(tc.profile), func(t *testing.T) {
			for layer, want := range tc.runs {
				if got := tc.profile.RunsLayer(layer); got != want {
					t.Errorf("%s.RunsLayer(%d) = %v, want %v", tc.profile, layer, got, want)
				}
			}
		})
	}
}

// TestRunsLayerUnknownProfileFallsBackToFull covers the arm that keeps an
// unvalidated Profile value from silently disabling the pipeline. Validation
// rejects unknown names at load, so this is the belt to that braces.
func TestRunsLayerUnknownProfileFallsBackToFull(t *testing.T) {
	var p Profile = "nonsense"
	for layer := 3; layer <= 6; layer++ {
		if !p.RunsLayer(layer) {
			t.Errorf("an unknown profile disabled layer %d; it must fall back to full", layer)
		}
	}
}

func TestKnownProfilesIsOrderedAndCopied(t *testing.T) {
	got := KnownProfiles()
	want := []Profile{ProfileL3Only, ProfileDetect, ProfileTrack, ProfileFull}
	if len(got) != len(want) {
		t.Fatalf("KnownProfiles() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("KnownProfiles() = %v, want %v (depth order)", got, want)
		}
	}
	got[0] = "mutated"
	if KnownProfiles()[0] != ProfileL3Only {
		t.Error("KnownProfiles returned the package slice; callers can corrupt the enum")
	}
}

func TestGetProfileAndFrameBudget(t *testing.T) {
	tests := []struct {
		name       string
		profile    string
		budget     float64
		wantP      Profile
		wantBudget float64
	}{
		{"unset uses defaults", "", 0, ProfileFull, DefaultFrameBudgetMs},
		{"configured values are honoured", "detect", 42.5, ProfileDetect, 42.5},
		{"negative budget falls back", "l3-only", -1, ProfileL3Only, DefaultFrameBudgetMs},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &TuningConfig{Pipeline: PipelineConfig{Profile: tc.profile, FrameBudgetMs: tc.budget}}
			if got := cfg.GetProfile(); got != tc.wantP {
				t.Errorf("GetProfile() = %q, want %q", got, tc.wantP)
			}
			if got := cfg.GetFrameBudgetMs(); got != tc.wantBudget {
				t.Errorf("GetFrameBudgetMs() = %v, want %v", got, tc.wantBudget)
			}
		})
	}
}

// TestGetProfileFallsBackOnInvalidValue covers the defensive arm. Validation
// makes this unreachable through a loaded config, but GetProfile is also
// reachable from a struct built in code.
func TestGetProfileFallsBackOnInvalidValue(t *testing.T) {
	cfg := &TuningConfig{Pipeline: PipelineConfig{Profile: "not-a-profile"}}
	if got := cfg.GetProfile(); got != DefaultProfile {
		t.Errorf("GetProfile() = %q, want the %q fallback", got, DefaultProfile)
	}
}

// TestFingerprintDiscriminates is the property the baseline comparator relies
// on: same config, same fingerprint; any tuning difference, different one.
func TestFingerprintDiscriminates(t *testing.T) {
	base := MustLoadDefaultConfig()

	same := MustLoadDefaultConfig()
	if base.Fingerprint() != same.Fingerprint() {
		t.Error("two loads of the same config produced different fingerprints")
	}

	tweaked := MustLoadDefaultConfig()
	tweaked.Pipeline.MinFramePoints++
	if base.Fingerprint() == tweaked.Fingerprint() {
		t.Error("a changed tuning value did not change the fingerprint")
	}

	reprofiled := MustLoadDefaultConfig()
	reprofiled.Pipeline.Profile = string(ProfileDetect)
	if base.Fingerprint() == reprofiled.Fingerprint() {
		t.Error("a changed profile did not change the fingerprint")
	}

	if got := len(base.Fingerprint()); got != 12 {
		t.Errorf("fingerprint length = %d, want 12", got)
	}
}

// TestFingerprintCoversEveryLayer guards against the fingerprint being taken
// over a subset of the config. A tuning change the fingerprint cannot see is a
// workload change the baseline comparator cannot refuse.
func TestFingerprintCoversEveryLayer(t *testing.T) {
	mutations := map[string]func(*TuningConfig){
		"l1": func(c *TuningConfig) { c.L1.Sensor += "-x" },
		"l3": func(c *TuningConfig) { c.L3.EmaBaselineV1.WarmupMinFrames++ },
		"l4": func(c *TuningConfig) { c.L4.DbscanXyV1.ForegroundDBSCANEps += 0.1 },
		"l5": func(c *TuningConfig) { c.L5.CvKfV1.MaxTracks++ },
		"pipeline": func(c *TuningConfig) {
			c.Pipeline.FrameBudgetMs += 1
		},
	}

	base := MustLoadDefaultConfig().Fingerprint()
	for layer, mutate := range mutations {
		t.Run(layer, func(t *testing.T) {
			cfg := MustLoadDefaultConfig()
			mutate(cfg)
			if cfg.Fingerprint() == base {
				t.Errorf("a change to %s left the fingerprint unchanged", layer)
			}
		})
	}
}

// TestProfileConfigsDifferOnlyInProfile is the anti-drift guard. The shipped
// profile configs exist so an operator (and CI) can select a profile by
// pointing at a file, and they are only meaningful if the profile is the sole
// difference — otherwise a benchmark comparison between two profiles would be
// measuring tuning changes nobody intended.
func TestProfileConfigsDifferOnlyInProfile(t *testing.T) {
	defaults := readConfigJSON(t, repoFile(t, DefaultConfigPath))

	for _, p := range KnownProfiles() {
		if p == DefaultProfile {
			continue
		}
		t.Run(string(p), func(t *testing.T) {
			path := repoFile(t, ProfileConfigPath(p))
			got := readConfigJSON(t, path)

			pipeline, ok := got["pipeline"].(map[string]interface{})
			if !ok {
				t.Fatalf("%s has no pipeline block", path)
			}
			if pipeline["profile"] != string(p) {
				t.Errorf("%s sets pipeline.profile = %v, want %q", path, pipeline["profile"], p)
			}

			// Normalise the one key that is meant to differ, then require
			// byte-for-byte equality of the rest.
			pipeline["profile"] = "full"
			if diff := jsonDiff(defaults, got); diff != "" {
				t.Errorf("%s differs from %s outside pipeline.profile: %s",
					path, DefaultConfigPath, diff)
			}
		})
	}
}

// TestProfileConfigsLoadAndValidate checks the shipped files are not just
// textually right but actually loadable, since they are what CI points at.
func TestProfileConfigsLoadAndValidate(t *testing.T) {
	for _, p := range KnownProfiles() {
		t.Run(string(p), func(t *testing.T) {
			cfg, err := LoadTuningConfig(repoFile(t, ProfileConfigPath(p)))
			if err != nil {
				t.Fatalf("loading the %s profile config: %v", p, err)
			}
			if got := cfg.GetProfile(); got != p {
				t.Errorf("%s config resolved to profile %q", p, got)
			}
		})
	}
}

func readConfigJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return out
}

func jsonDiff(want, got map[string]interface{}) string {
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(wantJSON) == string(gotJSON) {
		return ""
	}
	return "want " + string(wantJSON) + "\n got " + string(gotJSON)
}

// TestPipelineValidateRejectsBadProfile checks the load-time gate, which is
// what makes GetProfile's fallback unreachable in practice.
func TestPipelineValidateRejectsBadProfile(t *testing.T) {
	cfg := PipelineConfig{
		Profile:        "l7-only",
		BufferTimeout:  "500ms",
		MinFramePoints: 1000,
		FlushInterval:  "60s",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected an unknown profile to be rejected at validation")
	}
	if !strings.Contains(err.Error(), "profile") {
		t.Errorf("error %q should name the offending field", err)
	}
}

// TestPipelineValidateRejectsNegativeBudget covers the other new validation
// arm. A negative budget would mark every frame over budget and turn the alarm
// into noise.
func TestPipelineValidateRejectsNegativeBudget(t *testing.T) {
	cfg := PipelineConfig{
		BufferTimeout:  "500ms",
		MinFramePoints: 1000,
		FlushInterval:  "60s",
		FrameBudgetMs:  -1,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a negative frame_budget_ms to be rejected")
	}
}
