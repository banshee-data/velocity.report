//go:build pcap

package replayeval

import (
	"encoding/json"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/analysis"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWarmupPrefixAndScoringManifest(t *testing.T) {
	p := requireKirk0(t)
	out := filepath.Join(t.TempDir(), "warm")
	cfg := config.MustLoadDefaultConfig()
	cfg.L3.EmaBaselineV1.WarmupDurationNanos = 100000000
	cfg.L3.EmaBaselineV1.WarmupMinFrames = 1
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tuning := filepath.Join(t.TempDir(), "tuning.json")
	if err := os.WriteFile(tuning, data, 0600); err != nil {
		t.Fatal(err)
	}
	r, err := Run(Config{PCAPFile: p, OutDir: out, TuningFile: tuning, UDPPort: 2369, StartSeconds: 3, WarmupSeconds: 3, DurationSeconds: 2, RequireSettled: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.WarmupFrames == 0 || r.FramesRecorded == 0 || r.FramesRead <= r.FramesRecorded {
		t.Fatalf("prefix not processed: %+v", r)
	}
	var m struct {
		ScoreStart      int64   `json:"scoring_start_ns"`
		ProcessingStart float64 `json:"processing_start_seconds"`
		Settled         bool    `json:"settled_at_boundary"`
		Policy          string  `json:"tracker_boundary_policy"`
		Hash            string  `json:"source_sha256"`
	}
	b, err := os.ReadFile(filepath.Join(out, "replay_manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	rep, _, err := analysis.GenerateReport(out)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Recording.StartNs < m.ScoreStart || m.ProcessingStart != 0 || !m.Settled || m.Policy != "retain" || !strings.HasPrefix(m.Hash, "sha256:") {
		t.Fatalf("bad boundary: %+v", m)
	}
}

func TestRequireSettledFailsClosedAndCaptureBounds(t *testing.T) {
	p := requireKirk0(t)
	if _, err := Run(Config{PCAPFile: p, OutDir: t.TempDir(), UDPPort: 2369, DurationSeconds: 1, RequireSettled: true}); err == nil || !strings.Contains(err.Error(), "not settled") {
		t.Fatal(err)
	}
	if _, err := Run(Config{PCAPFile: p, OutDir: t.TempDir(), UDPPort: 2369, StartSeconds: 1e6}); err == nil {
		t.Fatal("out-of-capture score window accepted")
	}
	if _, err := Run(Config{PCAPFile: p, OutDir: t.TempDir(), UDPPort: 12345}); err == nil {
		t.Fatal("empty port accepted")
	}
}
