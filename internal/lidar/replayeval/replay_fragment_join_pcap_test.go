//go:build pcap

package replayeval

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
)

// fragment_join_track_membership on kirk0's truck window, against the
// registered join set alone (l4clus0010). Like the hygiene smoke run it
// asserts the wiring end to end and no direction: the arm is named in the
// manifest and moves the parameter hash, both arms count the join's work, and
// the widened footprints reach the partition, so the joining counters or the
// recording differ. With the defaults, which carry no join, the option is
// refused before the capture is read.
func TestFragmentJoinTrackMembershipOnKirk0(t *testing.T) {
	dir := t.TempDir()
	joinSet := filepath.Join("..", "..", "..", "config", "experiments", "motion-fragment-join.json")
	window := func(name string) Config {
		return Config{
			PCAPFile: requireKirk0(t), OutDir: filepath.Join(dir, name), SensorID: "test-replay", UDPPort: 2369,
			StartSeconds: 40, WarmupSeconds: 20, DurationSeconds: 25, TuningFile: joinSet,
		}
	}
	type arm struct {
		stats     *pipeline.FragmentJoinStats
		baseline  []byte
		frames    string
		paramHash string
	}
	arms := map[string]arm{}
	for _, a := range []struct {
		name        string
		experiments []string
	}{
		{"join", nil},
		{ExperimentFragmentJoinTrackMembership, []string{ExperimentFragmentJoinTrackMembership}},
	} {
		cfg := window(a.name)
		cfg.Experiments = a.experiments
		res, err := Run(cfg)
		if err != nil {
			t.Fatalf("%s: %v", a.name, err)
		}
		if res.FragmentJoin == nil || res.FragmentJoin.Frames == 0 {
			t.Fatalf("%s: the join did not run: %+v", a.name, res.FragmentJoin)
		}
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
		frames, _ := trackFingerprint(t, cfg.OutDir)
		arms[a.name] = arm{stats: res.FragmentJoin, baseline: readBaseline(t, cfg.OutDir), frames: strings.Join(frames, "\n"), paramHash: manifest.Params}
		t.Logf("%s: joining %+v", a.name, *res.FragmentJoin)
	}
	join, membership := arms["join"], arms[ExperimentFragmentJoinTrackMembership]
	if membership.paramHash == join.paramHash {
		t.Error("the parameter hash equals the join's; the arm is not identifiable")
	}
	same := *membership.stats == *join.stats && bytes.Equal(membership.baseline, join.baseline) && membership.frames == join.frames
	if same {
		t.Error("counters and recording equal the join's; the widened footprints did not reach the partition")
	}

	cfg := window("refused")
	cfg.TuningFile = filepath.Join("..", "..", "..", "config", "tuning.defaults.json")
	cfg.Experiments = []string{ExperimentFragmentJoinTrackMembership}
	if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "fragment_join is not enabled") {
		t.Fatalf("the defaults with the option: %v", err)
	}
}
