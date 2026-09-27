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
// local agent points at the S2 corpus. Two arms over the short moving window,
// to keep the race build lean, and the experiments that may not run yet:
//
//   - default: the manifest carries a continuity block whose unobserved
//     instants are all coasted, since nothing classifies them;
//   - coverage_free: the options that need no sensor coverage, capture-time
//     coast inflation and the reacquisition guard, which must reach the
//     tracker, be named in the manifest and move the parameter hash;
//   - coast_support, class_coast_bounds and occlusion_continuity classify an
//     absence, or bound a coast by one, and so need explicit sensor coverage
//     to tell a field-of-view exit from an occlusion. Until it is wired in,
//     the replay refuses them rather than mislabel an exit.
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
		{"coverage_free", []string{ExperimentCoastTimeInflation, ExperimentReacquisitionGuard}},
	}
	for _, needsCoverage := range []string{ExperimentCoastSupport, ExperimentClassCoastBounds, ExperimentOcclusionContinuity} {
		cfg := kirk0MovingWindow(t, filepath.Join(dir, needsCoverage))
		cfg.Experiments = []string{needsCoverage}
		if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "require explicit sensor coverage") {
			t.Fatalf("%s: err %v, want a refusal until sensor coverage is wired in", needsCoverage, err)
		}
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

	def, free := stats["default"], stats["coverage_free"]
	unexplained := def.SupportInstants
	if unexplained.Coasted == 0 || def.TracksBorn == 0 {
		t.Fatalf("default: the window has no coasting to explain: %+v", def)
	}
	if unexplained.OccludedInferred+unexplained.MissedUnknown+unexplained.OutOfFOV != 0 {
		t.Fatalf("default classified an absence with explanation off: %+v", unexplained)
	}
	if f := free.SupportInstants; f.OccludedInferred+f.MissedUnknown+f.OutOfFOV != 0 {
		t.Fatalf("coverage_free classified an absence without absence explanation: %+v", f)
	}

	if hashes["coverage_free"] == hashes["default"] {
		t.Error("coverage_free: parameter hash equals the default's; the arm is not identifiable")
	}
	if bytes.Equal(baselines["coverage_free"], baselines["default"]) && fingerprints["coverage_free"] == fingerprints["default"] {
		t.Error("coverage_free: the recording is identical to the default; the options did not reach the tracker")
	}
}
