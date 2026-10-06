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

// TestProfileIsDerivedFromEngines is the property the whole design rests on:
// the engine selectors are the configuration and the profile is read off them.
// There is no stored profile that could disagree with what actually runs.
func TestProfileIsDerivedFromEngines(t *testing.T) {
	tests := []struct {
		name     string
		l4Engine string
		l5Engine string
		want     Profile
	}{
		{"both layers live", "dbscan_xy_v1", "cv_kf_v1", ProfileFull},
		{"tracking disabled", "dbscan_xy_v1", EngineNone, ProfileDetect},
		{"clustering disabled", EngineNone, EngineNone, ProfileL3Only},
		{"a different L4 engine is still full depth", "hdbscan_adaptive_v1", "cv_kf_v1", ProfileFull},
		{"a different L5 engine is still full depth", "dbscan_xy_v1", "imm_cv_ca_v2", ProfileFull},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &TuningConfig{
				L4: L4Config{Engine: tc.l4Engine},
				L5: L5Config{Engine: tc.l5Engine},
			}
			if got := cfg.Profile(); got != tc.want {
				t.Errorf("Profile() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestProfileIsOrthogonalToEngineChoice states the separation explicitly: the
// profile says which layers run, the engine says which algorithm runs at a
// layer, and changing one must not move the other.
func TestProfileIsOrthogonalToEngineChoice(t *testing.T) {
	dbscan := &TuningConfig{
		L4: L4Config{Engine: "dbscan_xy_v1"},
		L5: L5Config{Engine: EngineNone},
	}
	hdbscan := &TuningConfig{
		L4: L4Config{Engine: "hdbscan_adaptive_v1"},
		L5: L5Config{Engine: EngineNone},
	}

	if dbscan.Profile() != ProfileDetect || hdbscan.Profile() != ProfileDetect {
		t.Errorf("swapping the L4 engine changed the profile: %q vs %q",
			dbscan.Profile(), hdbscan.Profile())
	}
}

func TestParseProfile(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Profile
		wantErr bool
	}{
		{"l3-only", "l3-only", ProfileL3Only, false},
		{"detect", "detect", ProfileDetect, false},
		{"full", "full", ProfileFull, false},
		{"surrounding space is trimmed", " detect ", ProfileDetect, false},
		{"empty is not a label", "", "", true},
		{"unknown is rejected", "everything", "", true},
		{"case is significant", "Full", "", true},
		{"a layer name is not a profile", "l4", "", true},
		{"track was dropped", "track", "", true},
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
	for _, want := range []string{"l3-only", "detect", "full", "detekt"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

// TestRunsLayerDepthOrdering pins the depth semantics every stage boundary
// depends on. A profile that silently ran one layer too far would produce
// numbers that look like a regression and are actually a different workload —
// which is the exact failure this mechanism exists to prevent.
func TestRunsLayerDepthOrdering(t *testing.T) {
	tests := []struct {
		profile Profile
		runs    map[int]bool
	}{
		{ProfileL3Only, map[int]bool{3: true, 4: false, 5: false, 6: false}},
		{ProfileDetect, map[int]bool{3: true, 4: true, 5: false, 6: false}},
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

// TestRunsLayerUnknownProfileRunsEverything covers the arm that keeps a
// misread label from silently switching the pipeline off.
func TestRunsLayerUnknownProfileRunsEverything(t *testing.T) {
	var p Profile = "nonsense"
	for layer := 3; layer <= 6; layer++ {
		if !p.RunsLayer(layer) {
			t.Errorf("an unknown profile disabled layer %d; it must fall back to full depth", layer)
		}
	}
}

func TestKnownProfilesIsOrderedAndCopied(t *testing.T) {
	got := KnownProfiles()
	want := []Profile{ProfileL3Only, ProfileDetect, ProfileFull}
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

// TestApplyProfileDisablesLayers checks the reduction path clears both the
// selector and the parameter block. A leftover block would keep tuning for a
// layer that does not run, which is exactly the confusion `none` removes — and
// it would still move the fingerprint.
func TestApplyProfileDisablesLayers(t *testing.T) {
	tests := []struct {
		target     Profile
		wantL4None bool
		wantL5None bool
	}{
		{ProfileFull, false, false},
		{ProfileDetect, false, true},
		{ProfileL3Only, true, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.target), func(t *testing.T) {
			cfg := MustLoadDefaultConfig()
			if err := cfg.ApplyProfile(tc.target); err != nil {
				t.Fatalf("ApplyProfile(%s): %v", tc.target, err)
			}

			if got := cfg.L4.Engine == EngineNone; got != tc.wantL4None {
				t.Errorf("l4 disabled = %v, want %v (engine %q)", got, tc.wantL4None, cfg.L4.Engine)
			}
			if got := cfg.L5.Engine == EngineNone; got != tc.wantL5None {
				t.Errorf("l5 disabled = %v, want %v (engine %q)", got, tc.wantL5None, cfg.L5.Engine)
			}
			if tc.wantL4None && cfg.L4.DbscanXyV1 != nil {
				t.Error("a disabled L4 kept its parameter block")
			}
			if tc.wantL5None && cfg.L5.CvKfV1 != nil {
				t.Error("a disabled L5 kept its parameter block")
			}
			if got := cfg.Profile(); got != tc.target {
				t.Errorf("after ApplyProfile(%s) the derived profile is %q", tc.target, got)
			}
			if err := cfg.Validate(); err != nil {
				t.Errorf("ApplyProfile produced a config that fails validation: %v", err)
			}
		})
	}
}

// TestApplyProfileRefusesToRaiseDepth covers the direction that cannot work.
// Raising depth needs parameter blocks the config does not carry, and
// inventing defaults would produce a run whose tuning nobody chose.
func TestApplyProfileRefusesToRaiseDepth(t *testing.T) {
	cfg := MustLoadDefaultConfig()
	if err := cfg.ApplyProfile(ProfileL3Only); err != nil {
		t.Fatalf("reducing to l3-only: %v", err)
	}

	err := cfg.ApplyProfile(ProfileFull)
	if err == nil {
		t.Fatal("expected raising the profile to be refused")
	}
	for _, want := range []string{"l3-only", "full", "engine block"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestApplyProfileRejectsUnknownTarget(t *testing.T) {
	cfg := MustLoadDefaultConfig()
	if err := cfg.ApplyProfile("track"); err == nil {
		t.Error("expected an unknown profile to be rejected")
	}
}

// TestApplyProfileToSameDepthIsANoOp covers the equal case, which sits between
// the two guarded directions.
func TestApplyProfileToSameDepthIsANoOp(t *testing.T) {
	cfg := MustLoadDefaultConfig()
	before := cfg.Fingerprint()

	if err := cfg.ApplyProfile(ProfileFull); err != nil {
		t.Fatalf("ApplyProfile(full) on a full config: %v", err)
	}
	if after := cfg.Fingerprint(); after != before {
		t.Errorf("applying the current profile changed the config: %s -> %s", before, after)
	}
}

// TestDisabledLayerAccessorsReturnZeroes covers the nil-safety that lets
// roughly thirty accessors keep dereferencing ActiveCommon() directly. A
// disabled layer has no meaningful parameter values and the stage that would
// read them never runs, so zeroes are the honest answer — but a nil would
// panic the process at startup.
func TestDisabledLayerAccessorsReturnZeroes(t *testing.T) {
	cfg := MustLoadDefaultConfig()
	if err := cfg.ApplyProfile(ProfileL3Only); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}

	// Each of these would panic on a nil block.
	if got := cfg.GetForegroundDBSCANEps(); got != 0 {
		t.Errorf("GetForegroundDBSCANEps() = %v on a disabled layer, want 0", got)
	}
	if got := cfg.GetForegroundMinClusterPoints(); got != 0 {
		t.Errorf("GetForegroundMinClusterPoints() = %v on a disabled layer, want 0", got)
	}
	if got := cfg.GetMaxTracks(); got != 0 {
		t.Errorf("GetMaxTracks() = %v on a disabled layer, want 0", got)
	}
	if got := cfg.GetGatingDistanceSquared(); got != 0 {
		t.Errorf("GetGatingDistanceSquared() = %v on a disabled layer, want 0", got)
	}
	if cfg.L4.ActiveConfig() != nil {
		t.Error("a disabled L4 reported an active engine block")
	}
}

// TestValidateRejectsALayerAboveADisabledOne is the rule that keeps the depth
// a closed set without enumerating the legal combinations anywhere. Tracking
// cannot consume clusters that were never produced.
func TestValidateRejectsALayerAboveADisabledOne(t *testing.T) {
	cfg := MustLoadDefaultConfig()
	cfg.L4.Engine = EngineNone
	cfg.L4.DbscanXyV1 = nil
	// L5 deliberately left running.

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected a live L5 above a disabled L4 to be rejected")
	}
	for _, want := range []string{"l5", "l4", "none"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestValidateAcceptsEveryReachableDepth(t *testing.T) {
	for _, p := range KnownProfiles() {
		t.Run(string(p), func(t *testing.T) {
			cfg := MustLoadDefaultConfig()
			if err := cfg.ApplyProfile(p); err != nil {
				t.Fatalf("ApplyProfile(%s): %v", p, err)
			}
			if err := cfg.Validate(); err != nil {
				t.Errorf("depth %s failed validation: %v", p, err)
			}
		})
	}
}

// TestDisabledLayerRejectsParameterBlocks checks the codec refuses tuning for
// a layer that is switched off. Such a block would be read by nobody and would
// still change the fingerprint, which is the confusion `none` exists to remove.
func TestDisabledLayerRejectsParameterBlocks(t *testing.T) {
	raw := `{
		"engine": "none",
		"dbscan_xy_v1": {"foreground_dbscan_eps": 0.8}
	}`

	var cfg L4Config
	err := json.Unmarshal([]byte(raw), &cfg)
	if err == nil {
		t.Fatal("expected a parameter block alongside engine \"none\" to be rejected")
	}
	if !strings.Contains(err.Error(), "dbscan_xy_v1") {
		t.Errorf("error %q should name the offending block", err)
	}
}

func TestDisabledLayerDecodes(t *testing.T) {
	var cfg L4Config
	if err := json.Unmarshal([]byte(`{"engine": "none"}`), &cfg); err != nil {
		t.Fatalf("a bare disabled layer should decode: %v", err)
	}
	if cfg.Engine != EngineNone {
		t.Errorf("engine = %q, want %q", cfg.Engine, EngineNone)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("a disabled layer should validate: %v", err)
	}
}

// TestL3CannotBeDisabled pins the floor. L3 is the layer every profile runs;
// disabling it would leave a pipeline that decodes packets and does nothing.
func TestL3CannotBeDisabled(t *testing.T) {
	var cfg L3Config
	if err := json.Unmarshal([]byte(`{"engine": "none"}`), &cfg); err == nil {
		t.Error("expected l3 to reject engine \"none\"")
	}
}

func TestGetFrameBudgetMs(t *testing.T) {
	tests := []struct {
		name   string
		budget float64
		want   float64
	}{
		{"unset uses the default", 0, DefaultFrameBudgetMs},
		{"configured value is honoured", 42.5, 42.5},
		{"negative falls back", -1, DefaultFrameBudgetMs},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &TuningConfig{Pipeline: PipelineConfig{FrameBudgetMs: tc.budget}}
			if got := cfg.GetFrameBudgetMs(); got != tc.want {
				t.Errorf("GetFrameBudgetMs() = %v, want %v", got, tc.want)
			}
		})
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

	shallower := MustLoadDefaultConfig()
	if err := shallower.ApplyProfile(ProfileDetect); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if base.Fingerprint() == shallower.Fingerprint() {
		t.Error("a changed depth did not change the fingerprint")
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
		"l1":       func(c *TuningConfig) { c.L1.Sensor += "-x" },
		"l3":       func(c *TuningConfig) { c.L3.EmaBaselineV1.WarmupMinFrames++ },
		"l4":       func(c *TuningConfig) { c.L4.DbscanXyV1.ForegroundDBSCANEps += 0.1 },
		"l5":       func(c *TuningConfig) { c.L5.CvKfV1.MaxTracks++ },
		"pipeline": func(c *TuningConfig) { c.Pipeline.FrameBudgetMs++ },
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

// TestProfileConfigsMatchAppliedDefaults is the anti-drift guard. Each shipped
// profile config must be exactly what applying that profile to the defaults
// produces — otherwise a benchmark comparison between two profiles would be
// measuring tuning changes nobody intended.
func TestProfileConfigsMatchAppliedDefaults(t *testing.T) {
	for _, p := range KnownProfiles() {
		if p == ProfileFull {
			continue
		}
		t.Run(string(p), func(t *testing.T) {
			want := MustLoadDefaultConfig()
			if err := want.ApplyProfile(p); err != nil {
				t.Fatalf("ApplyProfile(%s): %v", p, err)
			}

			path := repoFile(t, ProfileConfigPath(p))
			got, err := LoadTuningConfig(path)
			if err != nil {
				t.Fatalf("loading %s: %v", path, err)
			}

			if got.Fingerprint() != want.Fingerprint() {
				t.Errorf("%s is not the defaults at depth %s.\nRegenerate it; the only intended difference is which layers are disabled.\n got %s\nwant %s",
					path, p, mustJSON(t, got), mustJSON(t, want))
			}
			if got.Profile() != p {
				t.Errorf("%s derives profile %q, want %q", path, got.Profile(), p)
			}
		})
	}
}

// TestDefaultConfigIsFullDepth pins the shipped default: the binary runs
// everything unless a config says otherwise.
func TestDefaultConfigIsFullDepth(t *testing.T) {
	if got := MustLoadDefaultConfig().Profile(); got != ProfileFull {
		t.Errorf("the default config derives profile %q, want %q", got, ProfileFull)
	}
}

func mustJSON(t *testing.T, cfg *TuningConfig) string {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshalling config: %v", err)
	}
	return string(data)
}

// TestPipelineValidateRejectsNegativeBudget covers the validation arm for the
// one setting that stayed in the pipeline block. A negative budget would mark
// every frame over budget and turn the alarm into noise.
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
