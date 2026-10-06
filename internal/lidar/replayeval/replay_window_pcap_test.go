//go:build pcap

package replayeval

import (
	"bytes"
	"encoding/json"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/analysis"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"io"
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
	cfg.L4.DbscanXyV1.MaxSamplePoints = 16
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tuning := filepath.Join(t.TempDir(), "tuning.json")
	if err := os.WriteFile(tuning, data, 0600); err != nil {
		t.Fatal(err)
	}
	runCfg := Config{PCAPFile: p, OutDir: out, TuningFile: tuning, UDPPort: 2369, StartSeconds: 20, WarmupSeconds: 20, DurationSeconds: 10, RequireSettled: true, IncludeDebug: true}
	r, err := Run(runCfg)
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
	reader, err := recorder.NewReplayer(out)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	debugCount, sampleCount := 0, 0
	for {
		frame, err := reader.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if frame.Debug != nil {
			debugCount++
			if frame.Debug.FrameID != frame.FrameID {
				t.Fatal("debug frame identity mismatch")
			}
		}
		if frame.Clusters != nil {
			for _, c := range frame.Clusters.Clusters {
				if len(c.SamplePoints) > 0 {
					sampleCount++
				}
			}
		}
	}
	if debugCount == 0 || sampleCount == 0 {
		t.Fatalf("missing recorded evidence: debug=%d samples=%d", debugCount, sampleCount)
	}
	baseline, err := os.ReadFile(filepath.Join(out, "tracking_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	var measured TrackingBaseline
	if err := json.Unmarshal(baseline, &measured); err != nil {
		t.Fatal(err)
	}
	if measured.SchemaVersion != 2 || measured.Population == "" || measured.NISSelection != "accepted_associations_only" || measured.AssociationPopulation == "" || len(measured.Residuals) == 0 {
		t.Fatalf("invalid scoring baseline: %+v", measured)
	}
	runCfg.OutDir = filepath.Join(t.TempDir(), "repeat")
	if _, err := Run(runCfg); err != nil {
		t.Fatal(err)
	}
	repeat, err := os.ReadFile(filepath.Join(runCfg.OutDir, "tracking_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(baseline, repeat) {
		t.Fatalf("repeat baseline changed:\n%s\n%s", baseline, repeat)
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
