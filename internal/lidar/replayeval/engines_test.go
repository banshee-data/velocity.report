package replayeval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
)

// unimplementedEngineConfigs returns the defaults with L3, then L4, switched
// to an engine the schema describes and no code runs. Each carries a complete
// block, so it passes the loader and only the engine check can refuse it.
func unimplementedEngineConfigs() []*config.TuningConfig {
	l3 := config.MustLoadDefaultConfig()
	l3.L3.Engine = "ema_track_assist_v2"
	l3.L3.EmaTrackAssistV2 = &config.L3EmaTrackAssistV2{L3Common: l3.L3.EmaBaselineV1.L3Common}
	l3.L3.EmaBaselineV1 = nil

	l4 := config.MustLoadDefaultConfig()
	l4.L4.Engine = "two_stage_mahalanobis_v2"
	l4.L4.TwoStageMahalanobisV2 = &config.L4TwoStageMahalanobisV2{L4Common: l4.L4.DbscanXyV1.L4Common, VelocityCoherenceGate: 1}
	l4.L4.DbscanXyV1 = nil
	return []*config.TuningConfig{l3, l4}
}

// A tuning file naming an engine the pipeline does not run is refused with
// the server's text before any capture is read, rather than replaying the EMA
// baseline and DBSCAN under that engine's name.
func TestRunRefusesUnimplementedEngines(t *testing.T) {
	for _, cfg := range unimplementedEngineConfigs() {
		want := cfg.ValidateImplementedEngines()
		if want == nil {
			t.Fatalf("shared check accepted l3/l4 = %s/%s", cfg.L3.Engine, cfg.L4.Engine)
		}
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "tuning.json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := config.LoadTuningConfig(path); err != nil {
			t.Fatalf("fixture does not load, so the test would pass for the wrong reason: %v", err)
		}

		_, err = Run(Config{PCAPFile: "missing.pcap", OutDir: filepath.Join(t.TempDir(), "out"), TuningFile: path, UDPPort: 2369})
		if err == nil || !strings.Contains(err.Error(), want.Error()) {
			t.Errorf("l3/l4 = %s/%s: got %v, want %v", cfg.L3.Engine, cfg.L4.Engine, err, want)
		}
	}
}
