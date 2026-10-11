package config

import (
	"path/filepath"
	"testing"
)

// Every config the repository ships must name engines the pipeline runs, or
// whatever starts from it would be refused.
func TestValidateImplementedEnginesAcceptsShippedConfigs(t *testing.T) {
	var paths []string
	for _, p := range KnownProfiles() {
		paths = append(paths, repoFile(t, ProfileConfigPath(p)))
	}
	experiments, err := filepath.Glob(filepath.Join(repoFile(t, "config/experiments"), "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, experiments...)

	for _, path := range paths {
		cfg, err := LoadTuningConfig(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		if err := cfg.ValidateImplementedEngines(); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}

// Every engine the registry describes but the pipeline does not run is
// refused, so an engine added to the schema is refused until code lands for it.
func TestValidateImplementedEnginesRefusesEveryUnimplementedEngine(t *testing.T) {
	implemented := map[string]string{"l3": ImplementedL3Engine, "l4": ImplementedL4Engine, "l5": ImplementedL5Engine}
	refused := 0
	for name, spec := range engineRegistry {
		if name == implemented[spec.Layer] {
			continue
		}
		cfg := MustLoadDefaultConfig()
		switch spec.Layer {
		case "l3":
			cfg.L3.Engine = name
		case "l4":
			cfg.L4.Engine = name
		case "l5":
			cfg.L5.Engine = name
		default:
			t.Fatalf("engine %q has unknown layer %q", name, spec.Layer)
		}
		want := UnsupportedEngineError(spec.Layer, name, implemented[spec.Layer]).Error()
		err := cfg.ValidateImplementedEngines()
		if err == nil || err.Error() != want {
			t.Errorf("%s.engine=%q: got %v, want %q", spec.Layer, name, err, want)
		}
		refused++
	}
	if refused == 0 {
		t.Fatal("the registry lists no unimplemented engine; the test checked nothing")
	}
}

func TestValidateImplementedEnginesAcceptsDisabledLayers(t *testing.T) {
	for _, p := range []Profile{ProfileL3Only, ProfileDetect} {
		cfg := MustLoadDefaultConfig()
		if err := cfg.ApplyProfile(p); err != nil {
			t.Fatal(err)
		}
		if err := cfg.ValidateImplementedEngines(); err != nil {
			t.Errorf("profile %s: %v", p, err)
		}
	}
}

// "none" is a depth at L4 and L5 only; L3 always runs.
func TestValidateImplementedEnginesRefusesNoneAtL3(t *testing.T) {
	cfg := MustLoadDefaultConfig()
	cfg.L3.Engine = EngineNone
	if err := cfg.ValidateImplementedEngines(); err == nil {
		t.Fatal(`l3.engine "none" was accepted`)
	}
}
