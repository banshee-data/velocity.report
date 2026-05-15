package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/api"
	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/serialmux"
	"github.com/google/go-cmp/cmp"
)

const fixture string = `{"classifier" : "object_outbound", "end_time" : "1750719826.467", "start_time" : "1750719826.031", "delta_time_msec" : 736, "max_speed_mps" : 13.39, "min_speed_mps" : 11.33, "max_magnitude" : 55, "avg_magnitude" : 36, "total_frames" : 7, "frames_per_mps" : 0.5228, "length_m" : 9.86, "speed_change" : 2.799}`

func TestRadarEndToEnd(t *testing.T) {
	testingDir := t.TempDir()

	// Print out the testing directory for debugging purposes
	t.Logf("Testing directory: %s", testingDir)

	// Initialise the database
	d, err := db.NewDB(testingDir + "/test_sensor_data.db")
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	defer func() {
		if err := d.Close(); err != nil {
			t.Errorf("Failed to close test database: %v", err)
		}
	}()

	// handle the fixture as an event with serialmux.HandleEvent
	if err := serialmux.HandleEvent(d, fixture); err != nil {
		t.Fatalf("Failed to handle event: %v", err)
	}

	// Retrieve the events from the database using db.RadarObjects
	events, err := d.RadarObjects()
	if err != nil {
		t.Fatalf("Failed to retrieve events from database: %v", err)
	}
	if len(events) != 1 {
		t.Fatal("Expected only one event in the database")
	}
	// set expectations on the event
	expectedEvent := db.RadarObject{
		Classifier:   "object_outbound",
		StartTime:    time.Date(2025, time.June, 23, 23, 03, 46, 31000000, time.UTC),
		EndTime:      time.Date(2025, time.June, 23, 23, 03, 46, 467000000, time.UTC),
		DeltaTimeMs:  736,
		MaxSpeed:     13.39,
		MinSpeed:     11.33,
		SpeedChange:  2.799,
		MaxMagnitude: 55,
		AvgMagnitude: 36,
		TotalFrames:  7,
		FramesPerMps: 0.5228,
		Length:       9.86,
	}

	// Check if the event matches the expected event
	if diff := cmp.Diff(expectedEvent, events[0]); diff != "" {
		t.Errorf("Event mismatch (-got +want):\n%s", diff)
	}
}

func TestNewRuntimeSerialManager_EnablesActiveSerialTestPath(t *testing.T) {
	disabledMux := serialmux.NewDisabledSerialMux()
	manager := newRuntimeSerialManager(nil, disabledMux, "/dev/ttySC1", true, false)
	defer manager.Close()

	server := api.NewServer(disabledMux, nil, "mph", "UTC")
	server.SetSerialManager(manager)

	body, err := json.Marshal(map[string]any{
		"port_path":       "/dev/ttySC1",
		"baud_rate":       19200,
		"data_bits":       8,
		"stop_bits":       1,
		"parity":          "None",
		"timeout_seconds": 5,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/serial/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeMux().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success response, got %+v", resp)
	}
	if !strings.Contains(resp.Message, "already active") {
		t.Fatalf("expected active-port shortcut message, got %q", resp.Message)
	}
}
