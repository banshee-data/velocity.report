//go:build pcap && !race

package replayeval

// Excluded from race builds for CI time, not for correctness: three replays
// cost about three minutes under the race detector, and the continuity code
// adds no goroutine or shared state beyond the tracker's own lock, which the
// other kirk0 replays already exercise under race. It passes with -race.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// The occlusion-continuity harness on kirk0: the in-repo smoke run of what a
// local agent points at the S2 corpus. Three arms over the short moving
// window, to keep the race build lean:
//
//   - default: the manifest carries a continuity block whose unobserved
//     instants are all coasted, since nothing classifies them;
//   - coast_support: absence explanation alone must be diagnostic. The
//     tracks and the schema-2 baseline are byte-identical to the default's,
//     and the same unobserved instants are merely split by explanation;
//   - occlusion_continuity: every option, which must reach the tracker, be
//     named in the manifest, move the parameter hash, and replace the
//     frame-count expiry with capture-time bounds.
//
// Like the time-domain smoke run it asserts no direction for the estimate:
// one capture is not evidence. The label-free comparison is logged.
func TestOcclusionContinuityExperimentsOnKirk0(t *testing.T) {
	dir := t.TempDir()
	type arm struct {
		name        string
		experiments []string
	}
	arms := []arm{
		{"default", nil},
		{"coast_support", []string{ExperimentCoastSupport}},
		{"occlusion_continuity", []string{ExperimentOcclusionContinuity}},
	}
	baselines := map[string][]byte{}
	fingerprints := map[string]string{}
	hashes := map[string]string{}
	stats := map[string]l5tracks.ContinuityStats{}
	for _, a := range arms {
		cfg := kirk0MovingWindow(t, filepath.Join(dir, a.name))
		cfg.Experiments = a.experiments
		res, err := Run(cfg)
		if err != nil {
			t.Fatalf("%s: %v", a.name, err)
		}
		baselines[a.name] = readBaseline(t, cfg.OutDir)
		frames, _ := trackFingerprint(t, cfg.OutDir)
		fingerprints[a.name] = strings.Join(frames, "\n")

		var manifest struct {
			Params     string                    `json:"params_sha256"`
			Continuity *l5tracks.ContinuityStats `json:"continuity"`
		}
		b, err := os.ReadFile(filepath.Join(cfg.OutDir, "replay_manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Continuity == nil || *manifest.Continuity != res.Continuity {
			t.Fatalf("%s: the manifest's continuity block does not match the result", a.name)
		}
		hashes[a.name], stats[a.name] = manifest.Params, res.Continuity
		summary, _ := json.Marshal(res.Continuity)
		t.Logf("%s: continuity %s", a.name, summary)
	}

	def, support, all := stats["default"], stats["coast_support"], stats["occlusion_continuity"]
	unexplained := def.SupportInstants
	if unexplained.Coasted == 0 || def.TracksBorn == 0 {
		t.Fatalf("default: the window has no coasting to explain: %+v", def)
	}
	if unexplained.OccludedInferred+unexplained.MissedUnknown+unexplained.OutOfFOV != 0 {
		t.Fatalf("default classified an absence with explanation off: %+v", unexplained)
	}

	if !bytes.Equal(baselines["coast_support"], baselines["default"]) {
		t.Fatalf("coast_support changed the tracking baseline; explanation must be diagnostic:\n%s\n%s",
			baselines["default"], baselines["coast_support"])
	}
	requireSameDecisions(t, "coast_support",
		strings.Split(fingerprints["default"], "\n"), strings.Split(fingerprints["coast_support"], "\n"))
	s := support.SupportInstants
	if s.Coasted != 0 || s.Observed != unexplained.Observed ||
		s.OccludedInferred+s.MissedUnknown+s.OutOfFOV != unexplained.Coasted {
		t.Fatalf("coast_support should split the default's %d coasted instants by explanation: %+v", unexplained.Coasted, s)
	}
	if support.ExpiredByReason != def.ExpiredByReason {
		t.Fatalf("coast_support changed how tracks ended: %+v vs %+v", support.ExpiredByReason, def.ExpiredByReason)
	}

	if hashes["occlusion_continuity"] == hashes["default"] {
		t.Error("occlusion_continuity: parameter hash equals the default's; the arm is not identifiable")
	}
	if bytes.Equal(baselines["occlusion_continuity"], baselines["default"]) && fingerprints["occlusion_continuity"] == fingerprints["default"] {
		t.Error("occlusion_continuity: the recording is identical to the default; the options did not reach the tracker")
	}
	if all.ExpiredByReason.Misses != 0 {
		t.Errorf("occlusion_continuity: %d tracks expired by miss count; capture-time bounds replace it", all.ExpiredByReason.Misses)
	}
}
