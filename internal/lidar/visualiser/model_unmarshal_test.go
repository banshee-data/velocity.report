package visualiser

import (
	"encoding/json"
	"testing"
)

func TestTrack_UnmarshalJSON_MaxSpeedMps(t *testing.T) {
	data := []byte(`{"TrackID":"t1","MaxSpeedMps":12.5}`)
	var track Track
	if err := json.Unmarshal(data, &track); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track.MaxSpeedMps != 12.5 {
		t.Errorf("expected MaxSpeedMps=12.5, got %f", track.MaxSpeedMps)
	}
}

func TestTrack_UnmarshalJSON_MaxSpeedMps_SnakeCase(t *testing.T) {
	data := []byte(`{"TrackID":"t1","max_speed_mps":15.0}`)
	var track Track
	if err := json.Unmarshal(data, &track); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Go's standard json decoder does not match snake_case to PascalCase,
	// but UnmarshalJSON explicitly decodes raw["max_speed_mps"] into MaxSpeedMps.
	if track.MaxSpeedMps != 15.0 {
		t.Errorf("expected MaxSpeedMps=15.0, got %f", track.MaxSpeedMps)
	}
}

func TestTrack_UnmarshalJSON_LegacyPeakSpeedMps(t *testing.T) {
	data := []byte(`{"TrackID":"t1","PeakSpeedMps":20.0}`)
	var track Track
	if err := json.Unmarshal(data, &track); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track.MaxSpeedMps != 20.0 {
		t.Errorf("expected MaxSpeedMps=20.0 from PeakSpeedMps legacy, got %f", track.MaxSpeedMps)
	}
}

func TestTrack_UnmarshalJSON_LegacyPeakSpeedMps_SnakeCase(t *testing.T) {
	data := []byte(`{"TrackID":"t1","peak_speed_mps":25.0}`)
	var track Track
	if err := json.Unmarshal(data, &track); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track.MaxSpeedMps != 25.0 {
		t.Errorf("expected MaxSpeedMps=25.0 from peak_speed_mps legacy, got %f", track.MaxSpeedMps)
	}
}

func TestTrack_UnmarshalJSON_NoSpeedField(t *testing.T) {
	data := []byte(`{"TrackID":"t2","SpeedMps":5.0}`)
	var track Track
	if err := json.Unmarshal(data, &track); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if track.MaxSpeedMps != 0 {
		t.Errorf("expected MaxSpeedMps=0 when no speed field, got %f", track.MaxSpeedMps)
	}
}

func TestTrack_UnmarshalJSON_InvalidJSON(t *testing.T) {
	data := []byte(`{invalid}`)
	var track Track
	// json.Unmarshal validates syntax before calling UnmarshalJSON,
	// so call UnmarshalJSON directly to exercise the inner error branch.
	if err := track.UnmarshalJSON(data); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
