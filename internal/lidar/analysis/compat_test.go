package analysis

import (
	"bytes"
	"encoding/json"
	"testing"
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

func TestTrackDetailUnmarshalUsesMaxSpeedMps(t *testing.T) {
	var td TrackDetail
	if err := json.Unmarshal([]byte(`{
		"track_id": "track-1",
		"avg_speed_mps": 8.4,
		"max_speed_mps": 9.1
	}`), &td); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if td.MaxSpeedMps != 9.1 {
		t.Errorf("MaxSpeedMps = %v, want 9.1", td.MaxSpeedMps)
	}
}
