package l3grid

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// BuildRationale
// ---------------------------------------------------------------------------

func TestBuildRationale_NoHistory(t *testing.T) {
	th := DefaultSettlingThresholds()
	got := BuildRationale(nil, -1, th)
	want := "No frames processed; the PCAP file may be empty or contain no valid LiDAR packets."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildRationale_EmptySlice(t *testing.T) {
	th := DefaultSettlingThresholds()
	got := BuildRationale([]SettlingMetrics{}, -1, th)
	want := "No frames processed; the PCAP file may be empty or contain no valid LiDAR packets."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildRationale_NotConverged(t *testing.T) {
	history := []SettlingMetrics{
		{CoverageRate: 0.5, SpreadDeltaRate: 0.01, RegionStability: 0.8, MeanConfidence: 5, FrameNumber: 100},
	}
	th := DefaultSettlingThresholds()
	got := BuildRationale(history, -1, th)

	for _, substr := range []string{
		"Convergence was not reached",
		"coverage=0.50",
		"spread_delta=0.010000",
		"region_stability=0.800",
		"confidence=5.0",
		"WarmupMinFrames",
	} {
		if !strings.Contains(got, substr) {
			t.Errorf("expected %q in rationale, got: %s", substr, got)
		}
	}
}

func TestBuildRationale_Converged(t *testing.T) {
	history := []SettlingMetrics{
		{CoverageRate: 0.9, SpreadDeltaRate: 0.0001, RegionStability: 0.99, MeanConfidence: 20, FrameNumber: 80},
	}
	th := DefaultSettlingThresholds()
	got := BuildRationale(history, 80, th)

	for _, substr := range []string{
		"Convergence reached at frame 80",
		"WarmupMinFrames=80",
		"safety margin: 96",
		"At 10 Hz",
		"at 20 Hz",
	} {
		if !strings.Contains(got, substr) {
			t.Errorf("expected %q in rationale, got: %s", substr, got)
		}
	}
}

func TestBuildRationale_ConvergedFrame1(t *testing.T) {
	history := []SettlingMetrics{{FrameNumber: 1}}
	th := DefaultSettlingThresholds()
	got := BuildRationale(history, 1, th)
	if !strings.Contains(got, "frame 1") {
		t.Errorf("expected 'frame 1' in rationale, got: %s", got)
	}
}

// ---------------------------------------------------------------------------
// FormatRecommendedDuration
// ---------------------------------------------------------------------------

func TestFormatRecommendedDuration_Positive(t *testing.T) {
	tests := []struct {
		frame int
		want  string
	}{
		{100, "10.0s (at 10 Hz)"},
		{50, "5.0s (at 10 Hz)"},
		{1, "0.1s (at 10 Hz)"},
	}
	for _, tt := range tests {
		got := FormatRecommendedDuration(tt.frame)
		if got != tt.want {
			t.Errorf("FormatRecommendedDuration(%d) = %q, want %q", tt.frame, got, tt.want)
		}
	}
}

func TestFormatRecommendedDuration_NonPositive(t *testing.T) {
	for _, frame := range []int{0, -1, -100} {
		got := FormatRecommendedDuration(frame)
		if got != "unknown" {
			t.Errorf("FormatRecommendedDuration(%d) = %q, want 'unknown'", frame, got)
		}
	}
}

// ---------------------------------------------------------------------------
// SettlingReport JSON
// ---------------------------------------------------------------------------

func TestSettlingReport_JSON_RoundTrip(t *testing.T) {
	report := SettlingReport{
		PCAPFile:            "test.pcap",
		TuningFile:          "tuning.json",
		SensorID:            "sensor-01",
		TotalSamples:        3,
		TotalFrames:         100,
		MetricsHistory:      []SettlingMetrics{{FrameNumber: 1}, {FrameNumber: 50}, {FrameNumber: 100}},
		RecommendedFrame:    11,
		RecommendedDuration: "1.1s (at 10 Hz)",
		Thresholds:          DefaultSettlingThresholds(),
		Rationale:           "test rationale",
		WallDuration:        "2.5s",
	}

	data, err := json.Marshal(&report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got SettlingReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.PCAPFile != report.PCAPFile {
		t.Errorf("pcap_file = %q, want %q", got.PCAPFile, report.PCAPFile)
	}
	if got.RecommendedFrame != report.RecommendedFrame {
		t.Errorf("recommended_settling_frame = %d, want %d", got.RecommendedFrame, report.RecommendedFrame)
	}
	if got.TotalFrames != report.TotalFrames {
		t.Errorf("total_frames = %d, want %d", got.TotalFrames, report.TotalFrames)
	}
	if len(got.MetricsHistory) != 3 {
		t.Errorf("metrics_history len = %d, want 3", len(got.MetricsHistory))
	}
}

func TestSettlingReport_JSONKeys(t *testing.T) {
	report := SettlingReport{
		PCAPFile: "test.pcap",
		SensorID: "s1",
	}
	data, _ := json.Marshal(&report)
	var raw map[string]interface{}
	_ = json.Unmarshal(data, &raw)

	for _, key := range []string{
		"pcap_file", "tuning_file", "sensor_id", "total_samples",
		"total_frames", "metrics_history", "recommended_settling_frame",
		"recommended_settling_duration", "thresholds", "rationale", "wall_duration",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected key %q in JSON", key)
		}
	}
}

// ---------------------------------------------------------------------------
// WriteReport
// ---------------------------------------------------------------------------

func TestWriteReport_ToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")

	report := &SettlingReport{
		PCAPFile:         "test.pcap",
		SensorID:         "sensor-01",
		TotalSamples:     2,
		TotalFrames:      10,
		MetricsHistory:   []SettlingMetrics{{FrameNumber: 1}, {FrameNumber: 2}},
		RecommendedFrame: 5,
		Thresholds:       DefaultSettlingThresholds(),
		Rationale:        "test rationale",
		WallDuration:     "500ms",
	}

	if err := WriteReport(report, path); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var got SettlingReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.PCAPFile != "test.pcap" {
		t.Errorf("pcap_file = %q, want test.pcap", got.PCAPFile)
	}
	if got.RecommendedFrame != 5 {
		t.Errorf("recommended_settling_frame = %d, want 5", got.RecommendedFrame)
	}
}

func TestWriteReport_ToStdout(t *testing.T) {
	report := &SettlingReport{
		PCAPFile: "test.pcap",
		SensorID: "s1",
	}
	// Writing to stdout (path="") should not error.
	if err := WriteReport(report, ""); err != nil {
		t.Fatalf("WriteReport to stdout: %v", err)
	}
}

func TestWriteReport_BadPath(t *testing.T) {
	report := &SettlingReport{PCAPFile: "test.pcap"}
	err := WriteReport(report, "/nonexistent/dir/report.json")
	if err == nil {
		t.Fatal("expected error for bad path")
	}
	if !strings.Contains(err.Error(), "write") {
		t.Errorf("expected 'write' in error, got: %v", err)
	}
}

func TestWriteJSON_MarshalError(t *testing.T) {
	// Pass an unmarshalable value (channel) to exercise the JSON encode
	// error path.
	err := writeJSON(make(chan int), "")
	if err == nil {
		t.Fatal("expected error for unmarshalable type")
	}
	if !strings.Contains(err.Error(), "JSON encode") {
		t.Errorf("expected 'JSON encode' in error, got: %v", err)
	}
}
