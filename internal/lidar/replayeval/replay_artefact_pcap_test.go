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

// The artefact options (l6cls0005) on kirk0's moving window, each the
// default replay with one option on. Like the hygiene smoke run, it asserts
// the wiring end to end and no direction: each arm is named in the manifest
// and moves the parameter hash, the default writes none of the options'
// counters, and each option's counter shows whether it reached a decision in
// the window, which is logged with whether the recording changed.
func TestArtefactExperimentsOnKirk0(t *testing.T) {
	dir := t.TempDir()
	arms := []struct {
		name        string
		experiments []string
	}{
		{"default", nil},
		{ExperimentArtefactBirthGate, []string{ExperimentArtefactBirthGate}},
		{ExperimentArtefactAbsorb, []string{ExperimentArtefactAbsorb}},
	}
	baselines := map[string][]byte{}
	fingerprints := map[string]string{}
	hashes := map[string]string{}
	for _, a := range arms {
		cfg := kirk0MovingWindow(t, filepath.Join(dir, a.name))
		cfg.Experiments = a.experiments
		res, err := Run(cfg)
		if err != nil {
			t.Fatalf("%s: %v", a.name, err)
		}
		c := res.Continuity
		t.Logf("%s: held %d, released %d, absorbed %d", a.name, c.ArtefactBirthsHeld, c.ArtefactBirthsReleased, c.ArtefactClustersAbsorbed)
		if a.name == "default" && (c.ArtefactBirthsHeld != 0 || c.ArtefactBirthsReleased != 0 || c.ArtefactClustersAbsorbed != 0) {
			t.Errorf("the default replay counted artefact decisions: %+v", c)
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
	}
	for _, a := range arms[1:] {
		if hashes[a.name] == hashes["default"] {
			t.Errorf("%s: parameter hash equals the default's; the arm is not identifiable", a.name)
		}
		same := bytes.Equal(baselines[a.name], baselines["default"]) && fingerprints[a.name] == fingerprints["default"]
		t.Logf("%s: recording identical to the default: %v", a.name, same)
	}
}
