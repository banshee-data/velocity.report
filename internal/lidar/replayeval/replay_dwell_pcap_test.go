//go:build pcap

package replayeval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Each dwell policy setting reaches a replay of kirk0 and the replay's
// result and manifest echo the T and the stationary drift rule it ran with
// (the harness summary's dwell_policy block is the result's report). Without
// one the result carries no report and the manifest no dwell_policy section,
// as before the policy existed. Two seconds of capture is enough to build the
// policy and write the report; it is far short of any T, so nothing is
// absorbed.
func TestRunEchoesTheDwellPolicyT(t *testing.T) {
	pcapPath := requireKirk0(t)
	dir := t.TempDir()
	for _, c := range []struct {
		experiment string
		after      float64
		drift      float64
		anchor     string
		window     int
	}{
		{"", 0, 0, "", 0},
		{ExperimentDwellPolicy, 20, 1, "first_estimate", 0},
		{ExperimentDwellPolicyT40, 40, 1, "first_estimate", 0},
		{ExperimentDwellPolicyT60, 60, 1, "first_estimate", 0},
		{ExperimentDwellPolicyT40Drift2m, 40, 2, "first_estimate", 0},
		{ExperimentDwellPolicyT40Median, 40, 1, "running_median", 400},
	} {
		label := c.experiment
		if label == "" {
			label = "off"
		}
		cfg := Config{
			PCAPFile: pcapPath, OutDir: filepath.Join(dir, label), SensorID: "dwell-t",
			UDPPort: 2369, DurationSeconds: 2,
		}
		if c.experiment != "" {
			cfg.Experiments = []string{c.experiment}
		}
		result, err := Run(cfg)
		if err != nil {
			t.Fatalf("%s: Run: %v", label, err)
		}
		b, err := os.ReadFile(filepath.Join(cfg.OutDir, "replay_manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		var manifest map[string]json.RawMessage
		if err := json.Unmarshal(b, &manifest); err != nil {
			t.Fatal(err)
		}
		section, present := manifest["dwell_policy"]
		if c.experiment == "" {
			if result.Dwell != nil || present {
				t.Errorf("off: result dwell %+v, manifest section %s; want neither", result.Dwell, section)
			}
			continue
		}
		if d := result.Dwell; d == nil || d.AbsorbAfterSeconds != c.after || d.StationaryDriftMetres != c.drift ||
			d.StationaryAnchor != c.anchor || d.StationaryMedianWindowFrames != c.window {
			t.Errorf("%s: result dwell %+v, want T %g s, %g m from %s (window %d)", label, d, c.after, c.drift, c.anchor, c.window)
		}
		var echoed struct {
			AbsorbAfterSeconds float64 `json:"absorb_after_seconds"`
			DriftMetres        float64 `json:"stationary_drift_metres"`
			Anchor             string  `json:"stationary_anchor"`
			WindowFrames       int     `json:"stationary_median_window_frames"`
		}
		if !present || json.Unmarshal(section, &echoed) != nil || echoed.AbsorbAfterSeconds != c.after ||
			echoed.DriftMetres != c.drift || echoed.Anchor != c.anchor || echoed.WindowFrames != c.window {
			t.Errorf("%s: manifest dwell_policy %s, want absorb_after_seconds %g, stationary_drift_metres %g, stationary_anchor %s, window %d",
				label, section, c.after, c.drift, c.anchor, c.window)
		}
	}
}
