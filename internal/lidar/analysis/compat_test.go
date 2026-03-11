package analysis

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/recorder"
)

func TestTrackDetailMarshalUsesMaxSpeedMps(t *testing.T) {
	td := TrackDetail{
		TrackID:      "track-1",
		AvgSpeedMps:  8.4,
		MaxSpeedMps:  9.1,
		DurationSecs: 4.2,
	}

	data, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if bytes.Contains(data, []byte(`"peak_speed_mps"`)) {
		t.Fatalf("marshal emitted legacy key: %s", data)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal(map) error = %v", err)
	}
	if _, ok := decoded["max_speed_mps"]; !ok {
		t.Fatalf("marshal omitted max_speed_mps: %s", data)
	}
}

func TestTrackDetailUnmarshalAcceptsLegacySpeedKey(t *testing.T) {
	var td TrackDetail
	if err := json.Unmarshal([]byte(`{
		"track_id": "legacy-track",
		"avg_speed_mps": 8.4,
		"peak_speed_mps": 9.1
	}`), &td); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if td.MaxSpeedMps != 9.1 {
		t.Errorf("MaxSpeedMps = %v, want 9.1", td.MaxSpeedMps)
	}
}

func TestTrackDetailUnmarshalPrefersExplicitMaxSpeedMps(t *testing.T) {
	var td TrackDetail
	if err := json.Unmarshal([]byte(`{
		"track_id": "mixed-track",
		"max_speed_mps": 0,
		"peak_speed_mps": 9.1
	}`), &td); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if td.MaxSpeedMps != 0 {
		t.Errorf("MaxSpeedMps = %v, want 0", td.MaxSpeedMps)
	}
}

func TestGenerateReportFallsBackToFrameSpeedWhenMaxMissing(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "legacy-max-missing.vrlog")

	rec, err := recorder.NewRecorder(basePath, "legacy-sensor")
	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	baseTime := int64(1_000_000_000_000)
	speeds := []float32{1.5, 4.0, 3.2}
	for i, speed := range speeds {
		ts := baseTime + int64(i)*100_000_000
		frame := &visualiser.FrameBundle{
			TimestampNanos: ts,
			Tracks: &visualiser.TrackSet{
				TimestampNanos: ts,
				Tracks: []visualiser.Track{
					{
						TrackID:          "legacy-track",
						State:            visualiser.TrackStateConfirmed,
						SpeedMps:         speed,
						MaxSpeedMps:      0,
						ObservationCount: i + 1,
						Hits:             i + 1,
						FirstSeenNanos:   baseTime,
						LastSeenNanos:    ts,
					},
				},
			},
		}
		if err := rec.Record(frame); err != nil {
			t.Fatalf("Record frame %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	report, _, err := GenerateReport(basePath)
	if err != nil {
		t.Fatalf("GenerateReport() error = %v", err)
	}
	if len(report.Tracks) != 1 {
		t.Fatalf("len(report.Tracks) = %d, want 1", len(report.Tracks))
	}
	if got := report.Tracks[0].MaxSpeedMps; got != 4.0 {
		t.Errorf("MaxSpeedMps = %v, want 4.0", got)
	}
}
