package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRationale_NoSamples(t *testing.T) {
	th := SettlingThresholds{MinCoverage: 0.8, MaxSpreadDelta: 0.001, MinRegionStability: 0.95, MinConfidence: 10}
	r := buildRationale(nil, -1, th)
	want := "No frames processed; the PCAP file may be empty or contain no valid LiDAR packets."
	if r != want {
		t.Fatalf("got %q", r)
	}
}

func TestBuildRationale_NotConverged(t *testing.T) {
	history := []SettlingMetrics{
		{CoverageRate: 0.5, SpreadDeltaRate: 0.01, RegionStability: 0.8, MeanConfidence: 5, FrameNumber: 100},
	}
	th := SettlingThresholds{MinCoverage: 0.8, MaxSpreadDelta: 0.001, MinRegionStability: 0.95, MinConfidence: 10}
	r := buildRationale(history, -1, th)
	if len(r) < 50 {
		t.Fatalf("rationale too short: %s", r)
	}
}

func TestBuildRationale_Converged(t *testing.T) {
	history := []SettlingMetrics{
		{CoverageRate: 0.9, SpreadDeltaRate: 0.0001, RegionStability: 0.99, MeanConfidence: 20, FrameNumber: 80},
	}
	th := SettlingThresholds{MinCoverage: 0.8, MaxSpreadDelta: 0.001, MinRegionStability: 0.95, MinConfidence: 10}
	r := buildRationale(history, 80, th)
	if len(r) < 20 {
		t.Fatalf("rationale too short: %s", r)
	}
}

func TestWriteReport_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")

	eval := &SettlingEvaluation{
		PCAPFile:            "test.pcap",
		SensorID:            "test-sensor",
		TotalSamples:        2,
		TotalFrames:         10,
		MetricsHistory:      []SettlingMetrics{{FrameNumber: 1}, {FrameNumber: 2}},
		RecommendedFrame:    5,
		RecommendedDuration: "0.5s (at 10 Hz)",
		Thresholds:          SettlingThresholds{MinCoverage: 0.8, MaxSpreadDelta: 0.001},
		Rationale:           "test rationale",
		WallDuration:        "500ms",
	}

	writeReport(eval, path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}

	var got SettlingEvaluation
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if got.PCAPFile != "test.pcap" {
		t.Errorf("pcap_file = %q, want test.pcap", got.PCAPFile)
	}
	if got.TotalFrames != 10 {
		t.Errorf("total_frames = %d, want 10", got.TotalFrames)
	}
	if got.RecommendedFrame != 5 {
		t.Errorf("recommended_frame = %d, want 5", got.RecommendedFrame)
	}
}

func TestSettlingEvaluation_JSONFields(t *testing.T) {
	eval := SettlingEvaluation{
		PCAPFile: "test.pcap",
		SensorID: "s1",
	}
	data, _ := json.Marshal(eval)
	var raw map[string]interface{}
	_ = json.Unmarshal(data, &raw)
	if _, ok := raw["pcap_file"]; !ok {
		t.Error("expected pcap_file in JSON")
	}
	if _, ok := raw["sensor_id"]; !ok {
		t.Error("expected sensor_id in JSON")
	}
}
