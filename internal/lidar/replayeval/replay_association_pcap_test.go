//go:build pcap

package replayeval

import (
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// identitySummary is a label-free reading of a recording's identities: the
// distinct tracks it published, how many of them were ever confirmed, and the
// pairs of confirmed tracks whose centres stood within colocatedMetres of each
// other on at least colocatedFrames frames. The last is a crude proxy for a
// duplicate, not the duplicate-pair rule of the l5id reports (which judges
// box overlap on the published boxes, in lidar-box-eval); it is here so a
// wiring run says which way an arm moved, not by how much.
type identitySummary struct {
	Tracks, Confirmed, ColocatedPairs int
}

const (
	colocatedMetres = 2.0
	colocatedFrames = 5
)

func summariseIdentities(t *testing.T, dir string) identitySummary {
	t.Helper()
	reader, err := recorder.NewReplayer(dir)
	if err != nil {
		t.Fatalf("open recording %s: %v", dir, err)
	}
	defer reader.Close()
	seen, confirmed := map[string]bool{}, map[string]bool{}
	together := map[[2]string]int{}
	for {
		frame, err := reader.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read recording %s: %v", dir, err)
		}
		if frame.Tracks == nil {
			continue
		}
		var live []l9endpoints.Track
		for _, tr := range frame.Tracks.Tracks {
			seen[tr.TrackID] = true
			if tr.State == l9endpoints.TrackStateConfirmed {
				confirmed[tr.TrackID] = true
				live = append(live, tr)
			}
		}
		sort.Slice(live, func(i, j int) bool { return live[i].TrackID < live[j].TrackID })
		for i := range live {
			for j := i + 1; j < len(live); j++ {
				if math.Hypot(float64(live[i].X-live[j].X), float64(live[i].Y-live[j].Y)) <= colocatedMetres {
					together[[2]string{live[i].TrackID, live[j].TrackID}]++
				}
			}
		}
	}
	s := identitySummary{Tracks: len(seen), Confirmed: len(confirmed)}
	for _, n := range together {
		if n >= colocatedFrames {
			s.ColocatedPairs++
		}
	}
	return s
}

// The two likelihood costs on kirk0's moving window, from l5id0011's a0
// parameter set (solid body, full members, containment; Gaussian likelihood
// cost off). Like the hygiene and time-domain smoke runs it asserts the
// wiring end to end and no direction: each arm is named in the manifest,
// moves the parameter hash and changes the recording, and the Gaussian and
// Student-t costs are refused together. The identity counts are logged so
// the size of the effect on this window is on record; one six-second window
// is not evidence, and l5id0011's held-out run is where the cost is judged.
func TestLikelihoodCostsOnKirk0(t *testing.T) {
	dir := t.TempDir()
	const a0 = "../../../config/experiments/association-containment-control.json"
	arms := []struct {
		name        string
		experiments []string
	}{
		{"a0", nil},
		{ExperimentLikelihoodCost, []string{ExperimentLikelihoodCost}},
		{ExperimentRobustLikelihoodCost, []string{ExperimentRobustLikelihoodCost}},
		// The robust cost reads A2's S, so it runs on the face-referenced
		// tracked state, which likelihood_cost refuses.
		{"near_edge_track", []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack}},
		{"near_edge_track+" + ExperimentRobustLikelihoodCost, []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers, ExperimentNearEdgeTrack, ExperimentRobustLikelihoodCost}},
	}
	fingerprints := map[string]string{}
	hashes := map[string]string{}
	for _, a := range arms {
		cfg := kirk0MovingWindow(t, filepath.Join(dir, a.name))
		cfg.TuningFile = a0
		cfg.Experiments = a.experiments
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
		want := append([]string(nil), a.experiments...)
		sort.Strings(want)
		if strings.Join(manifest.Experiments, ",") != strings.Join(want, ",") {
			t.Fatalf("%s: manifest records experiments %v", a.name, manifest.Experiments)
		}
		hashes[a.name] = manifest.Params
		ids := summariseIdentities(t, cfg.OutDir)
		c := res.Continuity
		t.Logf("%s: tracks %d (confirmed %d), co-located confirmed pairs %d, track-frames %d; "+
			"window births %d, confirmations %d, reacquisitions %d",
			a.name, ids.Tracks, ids.Confirmed, ids.ColocatedPairs, trackFrames,
			c.TracksBorn, c.TracksConfirmed, c.Reacquisitions)
	}
	for _, pair := range [][2]string{
		{ExperimentLikelihoodCost, "a0"},
		{ExperimentRobustLikelihoodCost, "a0"},
		{ExperimentRobustLikelihoodCost, ExperimentLikelihoodCost},
		{"near_edge_track+" + ExperimentRobustLikelihoodCost, "near_edge_track"},
	} {
		if hashes[pair[0]] == hashes[pair[1]] {
			t.Errorf("%s: parameter hash equals %s's; the arm is not identifiable", pair[0], pair[1])
		}
		same := fingerprints[pair[0]] == fingerprints[pair[1]]
		// The two likelihood costs share ln det S and differ only in the
		// miss term, so on a window without a contested heavy-tailed pairing
		// they can make every decision alike; that is logged, not refused.
		if pair[1] == ExperimentLikelihoodCost {
			t.Logf("%s: recording identical to %s's: %v", pair[0], pair[1], same)
			continue
		}
		if same {
			t.Errorf("%s: the recording is identical to %s's; the option did not reach the tracker", pair[0], pair[1])
		}
	}

	// Two likelihood costs, from names or from the tuning file, are refused
	// before the capture is read.
	for name, cfg := range map[string]Config{
		"both named": func() Config {
			c := kirk0MovingWindow(t, filepath.Join(dir, "refused-named"))
			c.TuningFile = a0
			c.Experiments = []string{ExperimentLikelihoodCost, ExperimentRobustLikelihoodCost}
			return c
		}(),
		"over the tuning file's": func() Config {
			c := kirk0MovingWindow(t, filepath.Join(dir, "refused-tuning"))
			c.TuningFile = "../../../config/experiments/association-likelihood.json"
			c.Experiments = []string{ExperimentRobustLikelihoodCost}
			return c
		}(),
	} {
		if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), ExperimentRobustLikelihoodCost) {
			t.Errorf("%s: robust_likelihood_cost with likelihood_cost gave %v, want the refusal", name, err)
		}
	}
}
