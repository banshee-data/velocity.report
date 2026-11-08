//go:build pcap

package replayeval

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The initialisation and covariance hygiene options (tracking-maths §2.3) on
// kirk0's moving window, each the default replay with one option on. Like the
// time-domain smoke run, it asserts the wiring end to end and no direction:
// each arm is named in the manifest and moves the parameter hash, and the
// two-point initialisation, which acts on every track's second frame, changes
// the recording. The covariance cap is latent at the shipped 100 m² and the
// speed clamp binds only above 30 m/s, so whether those two change this
// window is logged rather than asserted.
func TestHygieneExperimentsOnKirk0(t *testing.T) {
	dir := t.TempDir()
	arms := []struct {
		name        string
		experiments []string
	}{
		{"default", nil},
		{ExperimentTwoPointVelocityInit, []string{ExperimentTwoPointVelocityInit}},
		{ExperimentConsistentCovarianceCap, []string{ExperimentConsistentCovarianceCap}},
		{ExperimentConsistentSpeedClamp, []string{ExperimentConsistentSpeedClamp}},
	}
	baselines := map[string][]byte{}
	fingerprints := map[string]string{}
	hashes := map[string]string{}
	for _, a := range arms {
		cfg := kirk0MovingWindow(t, filepath.Join(dir, a.name))
		cfg.Experiments = a.experiments
		if _, err := Run(cfg); err != nil {
			t.Fatalf("%s: %v", a.name, err)
		}
		baselines[a.name] = readBaseline(t, cfg.OutDir)
		frames, _ := trackFingerprint(t, cfg.OutDir)
		fingerprints[a.name] = strings.Join(frames, "\n")

		var manifest struct {
			Params      string   `json:"params_sha256"`
			Experiments []string `json:"experiments"`
		}
		b, err := os.ReadFile(filepath.Join(cfg.OutDir, "replay_manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &manifest); err != nil {
			t.Fatal(err)
		}
		if strings.Join(manifest.Experiments, ",") != strings.Join(a.experiments, ",") {
			t.Fatalf("%s: manifest records experiments %v", a.name, manifest.Experiments)
		}
		hashes[a.name] = manifest.Params
		t.Logf("%s: tracking baseline\n%s", a.name, baselines[a.name])
	}
	for _, a := range arms[1:] {
		if hashes[a.name] == hashes["default"] {
			t.Errorf("%s: parameter hash equals the default's; the arm is not identifiable", a.name)
		}
		same := bytes.Equal(baselines[a.name], baselines["default"]) && fingerprints[a.name] == fingerprints["default"]
		if a.name == ExperimentTwoPointVelocityInit && same {
			t.Errorf("%s: the recording is identical to the default; the option did not reach the tracker", a.name)
		}
		t.Logf("%s: recording identical to the default: %v", a.name, same)
	}
}
