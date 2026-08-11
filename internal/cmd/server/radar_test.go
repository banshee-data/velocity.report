package server

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
	snapshot, err := runtimeSerialSnapshot(nil, "/dev/ttySC1", true, false)
	if err != nil {
		t.Fatalf("runtime serial snapshot: %v", err)
	}
	manager := newRuntimeSerialManager(nil, disabledMux, snapshot, false)
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

func TestRuntimeSerialSnapshotPrefersEnabledDatabaseConfig(t *testing.T) {
	database := newTestDB(t)
	disableAllSerialConfigs(t, database)

	id, err := database.CreateSerialConfig(&db.SerialConfig{
		PortPath:    "/dev/ttyUSB0",
		BaudRate:    115200,
		DataBits:    7,
		StopBits:    2,
		Parity:      "Even",
		Enabled:     true,
		SensorModel: "OPS243-A",
	})
	if err != nil {
		t.Fatalf("create serial config: %v", err)
	}

	snapshot, err := runtimeSerialSnapshot(database, "/dev/ttySC1", true, true)
	if err != nil {
		t.Fatalf("runtime serial snapshot: %v", err)
	}

	want := api.SerialConfigSnapshot{
		ConfigID: int(id),
		PortPath: "/dev/ttyUSB0",
		Source:   "database",
		Options: serialmux.PortOptions{
			BaudRate: 115200,
			DataBits: 7,
			StopBits: 2,
			Parity:   "E",
		},
	}
	if diff := cmp.Diff(want, snapshot); diff != "" {
		t.Fatalf("snapshot mismatch (-want +got):\n%s", diff)
	}
}

func TestRuntimeSerialSnapshotFallsBackToCLIWhenNoEnabledDatabaseConfig(t *testing.T) {
	database := newTestDB(t)
	disableAllSerialConfigs(t, database)

	if _, err := database.CreateSerialConfig(&db.SerialConfig{
		PortPath:    "/dev/ttyUSB0",
		BaudRate:    115200,
		DataBits:    8,
		StopBits:    1,
		Parity:      "N",
		Enabled:     false,
		SensorModel: "OPS243-A",
	}); err != nil {
		t.Fatalf("create disabled serial config: %v", err)
	}

	snapshot, err := runtimeSerialSnapshot(database, " /dev/ttySC1 ", true, true)
	if err != nil {
		t.Fatalf("runtime serial snapshot: %v", err)
	}

	want := api.SerialConfigSnapshot{
		PortPath: "/dev/ttySC1",
		Source:   "cli",
		Options:  defaultRuntimeSerialOptions(),
	}
	if diff := cmp.Diff(want, snapshot); diff != "" {
		t.Fatalf("snapshot mismatch (-want +got):\n%s", diff)
	}
}

func TestRuntimeSerialSnapshotDisabledRadarHasNoActiveConfig(t *testing.T) {
	snapshot, err := runtimeSerialSnapshot(nil, "", false, false)
	if err != nil {
		t.Fatalf("runtime serial snapshot: %v", err)
	}

	if diff := cmp.Diff(api.SerialConfigSnapshot{}, snapshot); diff != "" {
		t.Fatalf("snapshot mismatch (-want +got):\n%s", diff)
	}
}

func TestRuntimeSerialSnapshotRejectsInvalidEnabledDatabaseConfig(t *testing.T) {
	database := newTestDB(t)
	disableAllSerialConfigs(t, database)

	if _, err := database.CreateSerialConfig(&db.SerialConfig{
		PortPath:    "/dev/ttyUSB0",
		BaudRate:    12345,
		DataBits:    8,
		StopBits:    1,
		Parity:      "N",
		Enabled:     true,
		SensorModel: "OPS243-A",
	}); err != nil {
		t.Fatalf("create invalid serial config: %v", err)
	}

	_, err := runtimeSerialSnapshot(database, "/dev/ttySC1", true, true)
	if err == nil {
		t.Fatal("expected invalid enabled database config error")
	}
	if !strings.Contains(err.Error(), "enabled serial configuration") {
		t.Fatalf("expected enabled config context in error, got %v", err)
	}
}

func newTestDB(t *testing.T) *db.DB {
	t.Helper()

	database, err := db.NewDB(t.TempDir() + "/sensor_data.db")
	if err != nil {
		t.Fatalf("new test database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	return database
}

func disableAllSerialConfigs(t *testing.T, database *db.DB) {
	t.Helper()

	if _, err := database.Exec(`UPDATE radar_serial_config SET enabled = 0`); err != nil {
		t.Fatalf("disable seeded serial configs: %v", err)
	}
}
