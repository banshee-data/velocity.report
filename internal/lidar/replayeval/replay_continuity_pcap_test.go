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

// kirk0Coverage is kirk0's declared coverage: the sensor at the frame's
// origin, the full circle, out to the measured detection envelope. On the
// default replay the online estimates reach 91.7 m (p90 42.8 m).
var kirk0Coverage = ContinuityCoverage{
	Source:         "measured: kirk0 online track-range envelope, max 91.7 m, default replay",
	MaxRangeMetres: 92, AzimuthHalfWidthDeg: 180,
}

// The occlusion-continuity harness on kirk0: the in-repo smoke run of what a
// local agent points at the S2 corpus. Arms over the short moving window,
// kept few for the race build, and the refusals:
//
//   - default: the manifest carries a continuity block whose unobserved
//     instants are all coasted, since nothing classifies them;
//   - coverage_free: capture-time coast inflation and the reacquisition
//     guard, which need no coverage, must reach the tracker, be named in the
//     manifest and move the parameter hash;
//   - coast_support, under kirk0's declared coverage: absence explanation
//     alone is diagnostic, so the tracks and the schema-2 baseline are
//     byte-identical to the default's and the same unobserved instants are
//     merely split by explanation;
//   - occlusion_continuity, under the same coverage: every option, which must
//     reach the tracker, move the parameter hash, and replace the
//     frame-count expiry with capture-time bounds;
//   - without a coverage declaration, or with one that bounds nothing, the
//     experiments that classify an absence are refused before any replay.
//
// Like the time-domain smoke run it asserts no direction for the estimate:
// one capture is not evidence. The label-free comparison is logged.
func TestOcclusionContinuityExperimentsOnKirk0(t *testing.T) {
	dir := t.TempDir()
	type arm struct {
		name        string
		experiments []string
		coverage    *ContinuityCoverage
	}
	coverage := kirk0Coverage
	arms := []arm{
		{"default", nil, nil},
		{"coverage_free", []string{ExperimentCoastTimeInflation, ExperimentReacquisitionGuard}, nil},
		{"coast_support", []string{ExperimentCoastSupport}, &coverage},
		{"occlusion_continuity", []string{ExperimentOcclusionContinuity}, &coverage},
	}
	unbounded := kirk0Coverage
	unbounded.MaxRangeMetres = 0
	for _, needsCoverage := range []string{ExperimentCoastSupport, ExperimentClassCoastBounds, ExperimentOcclusionContinuity} {
		cfg := kirk0MovingWindow(t, filepath.Join(dir, "refused-"+needsCoverage))
		cfg.Experiments = []string{needsCoverage}
		if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "require explicit sensor coverage") {
			t.Fatalf("%s without coverage: err %v, want a refusal", needsCoverage, err)
		}
		cfg = kirk0MovingWindow(t, filepath.Join(dir, "unbounded-"+needsCoverage))
		cfg.Experiments, cfg.ContinuityCoverage = []string{needsCoverage}, &unbounded
		if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "max_range_m must be positive") {
			t.Fatalf("%s with an unbounded coverage: err %v, want a refusal", needsCoverage, err)
		}
	}
	baselines := map[string][]byte{}
	fingerprints := map[string]string{}
	hashes := map[string]string{}
	stats := map[string]l5tracks.ContinuityStats{}
	for _, a := range arms {
		cfg := kirk0MovingWindow(t, filepath.Join(dir, a.name))
		cfg.Experiments, cfg.ContinuityCoverage = a.experiments, a.coverage
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

	// Under a declared coverage, absence explanation is diagnostic only.
	support, all := stats["coast_support"], stats["occlusion_continuity"]
	if !bytes.Equal(baselines["coast_support"], baselines["default"]) {
		t.Fatalf("coast_support changed the tracking baseline; explanation must be diagnostic:\n%s\n%s",
			baselines["default"], baselines["coast_support"])
	}
	requireSameDecisions(t, "coast_support",
		strings.Split(fingerprints["default"], "\n"), strings.Split(fingerprints["coast_support"], "\n"))
	sp := support.SupportInstants
	if sp.Coasted != 0 || sp.Observed != unexplained.Observed ||
		sp.OccludedInferred+sp.MissedUnknown+sp.OutOfFOV != unexplained.Coasted {
		t.Fatalf("coast_support should split the default's %d coasted instants by explanation: %+v", unexplained.Coasted, sp)
	}
	if support.ExpiredByReason != def.ExpiredByReason {
		t.Fatalf("coast_support changed how tracks ended: %+v vs %+v", support.ExpiredByReason, def.ExpiredByReason)
	}
	if hashes["coast_support"] == hashes["default"] {
		t.Error("coast_support: parameter hash equals the default's; the coverage and option are not identifiable")
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

	var withCoverage struct {
		Coverage ContinuityCoverage `json:"continuity_coverage"`
		ID       string             `json:"continuity_coverage_id"`
		Applied  bool               `json:"continuity_coverage_applied"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "coast_support", "replay_manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &withCoverage); err != nil {
		t.Fatal(err)
	}
	if withCoverage.Coverage != kirk0Coverage || withCoverage.ID != kirk0Coverage.ID() || !withCoverage.Applied {
		t.Fatalf("manifest coverage = %+v, want kirk0's declaration, its id and applied", withCoverage)
	}
}
