package serialmux

import (
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
)

const radarFixture = `{"classifier" : "object_outbound", "end_time" : "1750719826.467", "start_time" : "1750719826.031", "delta_time_msec" : 736, "max_speed_mps" : 13.39, "min_speed_mps" : 11.33, "max_magnitude" : 55, "avg_magnitude" : 36, "total_frames" : 7, "frames_per_mps" : 0.5228, "length_m" : 9.86, "speed_change" : 2.799}`

func TestClassifyPayload(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`{"classifier":"x","end_time":123}`, EventTypeRadarObject},
		{`{"magnitude":1.2,"speed":3.4}`, EventTypeRawData},
		{`{"foo":"bar"}`, EventTypeConfig},
		{`plain text line`, EventTypeUnknown},
	}

	for _, c := range cases {
		got := ClassifyPayload(c.in)
		if got != c.want {
			t.Fatalf("ClassifyPayload(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestHandleConfigResponse_ValidAndInvalid(t *testing.T) {
	// reset state
	CurrentState = nil

	if err := HandleConfigResponse(`{"alpha":123,"beta":"x"}`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if CurrentState == nil {
		t.Fatalf("expected CurrentState to be initialized")
	}
	if v, ok := CurrentState["alpha"]; !ok || v == nil {
		t.Fatalf("expected alpha in CurrentState")
	}

	// invalid JSON should return an error and not panic
	if err := HandleConfigResponse("not-json"); err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

func TestHandleEvent_RadarObjectAndRawData(t *testing.T) {
	tmp := t.TempDir()
	d, err := db.NewDB(tmp + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	// Radar object
	if err := HandleEvent(d, radarFixture); err != nil {
		t.Fatalf("HandleEvent radar fixture failed: %v", err)
	}
	objs, err := d.RadarObjects()
	if err != nil {
		t.Fatalf("failed to read radar objects: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 radar object, got %d", len(objs))
	}

	// Raw data event
	raw := `{"uptime": 1.23, "magnitude": 5.6, "speed": 7.8}`
	if err := HandleEvent(d, raw); err != nil {
		t.Fatalf("HandleEvent raw failed: %v", err)
	}
	events, err := d.Events()
	if err != nil {
		t.Fatalf("failed to read events: %v", err)
	}
	if len(events) < 1 {
		t.Fatalf("expected at least 1 event, got %d", len(events))
	}

	// quick sanity on timestamps handling in radar object (not strict match)
	if objs[0].StartTime.IsZero() || objs[0].EndTime.IsZero() {
		t.Fatalf("unexpected zero timestamps: start=%v end=%v", objs[0].StartTime, objs[0].EndTime)
	}

	// ensure DB operations persisted quickly
	time.Sleep(5 * time.Millisecond)
}

func TestHandleEvent_ConfigEvent(t *testing.T) {
	tmp := t.TempDir()
	d, err := db.NewDB(tmp + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	// Reset state
	CurrentState = nil

	// Config event
	config := `{"config_key": "config_value", "number": 42}`
	if err := HandleEvent(d, config); err != nil {
		t.Fatalf("HandleEvent config failed: %v", err)
	}

	// Check that config was stored
	if CurrentState == nil {
		t.Fatal("CurrentState should be initialized after config event")
	}
	if v, ok := CurrentState["config_key"]; !ok || v != "config_value" {
		t.Errorf("Expected config_key to be 'config_value', got %v", v)
	}
}

func TestHandleEvent_UnknownEvent(t *testing.T) {
	tmp := t.TempDir()
	d, err := db.NewDB(tmp + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	// Unknown event type should not return error (just log)
	unknown := "plain text that matches no pattern"
	if err := HandleEvent(d, unknown); err != nil {
		t.Fatalf("HandleEvent unknown should not fail: %v", err)
	}
}

func TestHandleRadarObject(t *testing.T) {
	tmp := t.TempDir()
	d, err := db.NewDB(tmp + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	if err := HandleRadarObject(d, radarFixture); err != nil {
		t.Fatalf("HandleRadarObject failed: %v", err)
	}

	objs, err := d.RadarObjects()
	if err != nil {
		t.Fatalf("failed to read radar objects: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 radar object, got %d", len(objs))
	}
}

func TestHandleRawData(t *testing.T) {
	tmp := t.TempDir()
	d, err := db.NewDB(tmp + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	raw := `{"magnitude": 5.6, "speed": 7.8}`
	if err := HandleRawData(d, raw); err != nil {
		t.Fatalf("HandleRawData failed: %v", err)
	}

	events, err := d.Events()
	if err != nil {
		t.Fatalf("failed to read events: %v", err)
	}
	if len(events) < 1 {
		t.Fatalf("expected at least 1 event, got %d", len(events))
	}
}

func TestClassifyPayload_EdgeCases(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"radar with end_time", `{"end_time": 123}`, EventTypeRadarObject},
		{"radar with classifier", `{"classifier": "x"}`, EventTypeRadarObject},
		{"raw with magnitude only", `{"magnitude": 1.2}`, EventTypeRawData},
		{"raw with speed only", `{"speed": 3.4}`, EventTypeRawData},
		{"config JSON object", `{"key": "value"}`, EventTypeConfig},
		{"empty string", ``, EventTypeUnknown},
		{"not JSON", `hello world`, EventTypeUnknown},
		{"array JSON", `[1, 2, 3]`, EventTypeUnknown},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyPayload(c.in)
			if got != c.want {
				t.Errorf("ClassifyPayload(%q) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}

// TestHandleEvent_RadarObjectError tests error handling when radar object
// processing fails.
func TestHandleEvent_RadarObjectError(t *testing.T) {
	tmp := t.TempDir()
	d, err := db.NewDB(tmp + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	// Invalid JSON that looks like a radar object (has end_time) but will fail parsing
	invalidRadar := `{"end_time": "not-a-number", "classifier": "object_outbound"}`
	err = HandleEvent(d, invalidRadar)
	if err == nil {
		t.Error("Expected error for invalid radar object payload")
	}
	if err != nil && !strings.Contains(err.Error(), "RadarObject") {
		t.Errorf("Expected error message to mention RadarObject, got: %v", err)
	}
}

// TestHandleEvent_RawDataError tests that raw data with invalid JSON values
// can still be stored (the schema allows NULL for extracted columns).
func TestHandleEvent_RawDataError(t *testing.T) {
	tmp := t.TempDir()
	d, err := db.NewDB(tmp + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	// Raw data with string values instead of numbers - should succeed
	// because the radar_data table allows NULL for extracted columns
	invalidRaw := `{"magnitude": "not-a-number", "speed": "also-not-a-number"}`
	err = HandleEvent(d, invalidRaw)
	// Should NOT error - invalid values result in NULL but are still stored
	if err != nil {
		t.Errorf("Unexpected error for raw data with invalid values: %v", err)
	}
}

// TestHandleEvent_ConfigError tests error handling when config response
// processing fails.
func TestHandleEvent_ConfigError(t *testing.T) {
	tmp := t.TempDir()
	d, err := db.NewDB(tmp + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	// Malformed JSON that starts with { (so it's classified as config) but is invalid
	invalidConfig := `{invalid json here`
	err = HandleEvent(d, invalidConfig)
	if err == nil {
		t.Error("Expected error for invalid config payload")
	}
	if err != nil && !strings.Contains(err.Error(), "config response") {
		t.Errorf("Expected error message to mention config response, got: %v", err)
	}
}

// TestHandleRadarObject_InvalidJSON tests error handling for invalid radar JSON.
func TestHandleRadarObject_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	d, err := db.NewDB(tmp + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	// Completely invalid JSON
	err = HandleRadarObject(d, "not json at all")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestHandleRawData_InvalidJSON tests error handling for invalid raw data JSON.
func TestHandleRawData_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	d, err := db.NewDB(tmp + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	// Completely invalid JSON
	err = HandleRawData(d, "not json at all")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestHandleConfigResponse_UpdatesExistingState tests that config responses
// update existing state rather than replacing it.
func TestHandleConfigResponse_UpdatesExistingState(t *testing.T) {
	// Reset state
	CurrentState = nil

	// Set initial state
	if err := HandleConfigResponse(`{"key1": "value1"}`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Update with new key
	if err := HandleConfigResponse(`{"key2": "value2"}`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both keys should be present
	if CurrentState["key1"] != "value1" {
		t.Errorf("Expected key1 to be preserved, got %v", CurrentState["key1"])
	}
	if CurrentState["key2"] != "value2" {
		t.Errorf("Expected key2 to be added, got %v", CurrentState["key2"])
	}

	// Update existing key
	if err := HandleConfigResponse(`{"key1": "updated"}`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if CurrentState["key1"] != "updated" {
		t.Errorf("Expected key1 to be updated, got %v", CurrentState["key1"])
	}
}
