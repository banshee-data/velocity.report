//go:build pcap

package replayeval

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCampaignMetricsPreserveTrackingBaseline(t *testing.T) {
	cfg := Config{PCAPFile: requireKirk0(t), OutDir: filepath.Join(t.TempDir(), "plain"), SensorID: "campaign-check", UDPPort: 2369, StartSeconds: 20, WarmupSeconds: 20, DurationSeconds: 4}
	if _, err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	baseline, err := os.ReadFile(filepath.Join(cfg.OutDir, "tracking_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.OutDir = filepath.Join(t.TempDir(), "measured")
	cfg.CampaignMetrics = true
	if _, err = Run(cfg); err != nil {
		t.Fatal(err)
	}
	measured, err := os.ReadFile(filepath.Join(cfg.OutDir, "tracking_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(baseline, measured) {
		t.Fatal("diagnostics altered tracking baseline")
	}
	data, err := os.ReadFile(filepath.Join(cfg.OutDir, "tracker_timing.json"))
	if err != nil {
		t.Fatal(err)
	}
	var timing struct {
		Samples int     `json:"samples"`
		P99     float64 `json:"p99_seconds"`
	}
	if err = json.Unmarshal(data, &timing); err != nil {
		t.Fatal(err)
	}
	if timing.Samples == 0 || timing.P99 <= 0 {
		t.Fatalf("missing timings: %+v", timing)
	}
	if _, err = os.Stat(filepath.Join(cfg.OutDir, "confirmed_duration.json")); err != nil {
		t.Fatal(err)
	}
}
