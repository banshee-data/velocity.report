//go:build pcap && !race

package replayeval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// robust_birth_gate on the whole of kirk0, against robust_likelihood_cost
// alone, with the shipped defaults (l5id0021's arms). Like the likelihood-cost
// smoke run it asserts the wiring end to end, not a direction: the gate is
// named in the manifest, moves the parameter hash, absorbs at least one
// fragment on this capture and changes the recording. The counts are logged;
// l5id0021's scored replays are where the rule is judged.
//
// Excluded under -race: two whole-capture replays are minutes of work there.
func TestRobustBirthGateOnKirk0(t *testing.T) {
	dir := t.TempDir()
	arms := []struct {
		name        string
		experiments []string
	}{
		{"robust", []string{ExperimentRobustLikelihoodCost}},
		{"robust-birth", []string{ExperimentRobustBirthGate, ExperimentRobustLikelihoodCost}}, // sorted, as the manifest records them
	}
	fingerprints, hashes := map[string]string{}, map[string]string{}
	absorbed := map[string]int64{}
	for _, a := range arms {
		cfg := Config{
			PCAPFile: requireKirk0(t), OutDir: filepath.Join(dir, a.name), SensorID: "hesai-pandar40p", UDPPort: 2369,
			TuningFile:   "../../../config/tuning.defaults.json",
			StartSeconds: 6, WarmupSeconds: 6, RequireSettled: false,
			Experiments: a.experiments,
		}
		res, err := Run(cfg)
		if err != nil {
			t.Fatalf("%s: %v", a.name, err)
		}
		frames, trackFrames := trackFingerprint(t, cfg.OutDir)
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
		if got, want := strings.Join(manifest.Experiments, ","), strings.Join(a.experiments, ","); got != want {
			t.Fatalf("%s: manifest records experiments %q, want %q", a.name, got, want)
		}
		hashes[a.name] = manifest.Params
		c := res.Continuity
		absorbed[a.name] = c.FragmentBirthsAbsorbed
		ids := summariseIdentities(t, cfg.OutDir)
		t.Logf("%s: tracks %d (confirmed %d), co-located confirmed pairs %d, track-frames %d; "+
			"window births %d, confirmations %d, fragment births absorbed %d",
			a.name, ids.Tracks, ids.Confirmed, ids.ColocatedPairs, trackFrames,
			c.TracksBorn, c.TracksConfirmed, c.FragmentBirthsAbsorbed)
	}
	if absorbed["robust"] != 0 {
		t.Errorf("robust alone absorbed %d fragment births; the gate is on without its name", absorbed["robust"])
	}
	if absorbed["robust-birth"] == 0 {
		t.Error("robust_birth_gate absorbed no fragment birth on kirk0; the option did not reach the tracker")
	}
	if hashes["robust"] == hashes["robust-birth"] {
		t.Error("robust_birth_gate did not move the parameter hash; the arm is not identifiable")
	}
	if fingerprints["robust"] == fingerprints["robust-birth"] {
		t.Error("robust_birth_gate left the recording identical to robust's")
	}
}
