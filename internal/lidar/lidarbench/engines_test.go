//go:build pcap

package lidarbench

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
)

// A config naming an engine the pipeline does not run is refused with the
// server's text before any replay, rather than timing the EMA baseline and
// DBSCAN under that engine's name.
func TestBenchmarkRefusesUnimplementedEngines(t *testing.T) {
	// Run stats the capture before anything else; the engine check must fail
	// it before the empty file is ever read.
	capture := filepath.Join(t.TempDir(), "empty.pcapng")
	if err := os.WriteFile(capture, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, set := range []func(*config.TuningConfig){
		func(c *config.TuningConfig) { c.L3.Engine = "ema_track_assist_v2" },
		func(c *config.TuningConfig) { c.L4.Engine = "two_stage_mahalanobis_v2" },
	} {
		cfg := config.MustLoadDefaultConfig()
		set(cfg)
		want := cfg.ValidateImplementedEngines()
		if want == nil {
			t.Fatalf("shared check accepted l3/l4 = %s/%s", cfg.L3.Engine, cfg.L4.Engine)
		}

		bench := Config{PCAPFile: capture, SensorID: "test-sensor", UDPPort: fixturePort, Tuning: cfg}
		if _, _, _, err := runRepeated(bench); err == nil || err.Error() != want.Error() {
			t.Errorf("l3/l4 = %s/%s: got %v, want %v", cfg.L3.Engine, cfg.L4.Engine, err, want)
		}
		if code := Run(bench); code != 1 {
			t.Errorf("l3/l4 = %s/%s: Run exited %d, want 1", cfg.L3.Engine, cfg.L4.Engine, code)
		}
	}
}
