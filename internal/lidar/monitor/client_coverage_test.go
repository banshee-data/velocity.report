package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/sweep"
)

// --- GetLastAnalysisRunID ---

func TestGetLastAnalysisRunID_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"last_run_id": "run-123"})
	}))
	defer srv.Close()

	c := NewClient(srv.Client(), srv.URL, "sensor1")
	runID := c.GetLastAnalysisRunID()
	if runID != "run-123" {
		t.Errorf("expected run-123, got %s", runID)
	}
}

func TestGetLastAnalysisRunID_ServerError(t *testing.T) {
	c := NewClient(nil, "http://127.0.0.1:1", "sensor1")
	runID := c.GetLastAnalysisRunID()
	if runID != "" {
		t.Errorf("expected empty string on error, got %s", runID)
	}
}

func TestGetLastAnalysisRunID_NoRunID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"other": "value"})
	}))
	defer srv.Close()

	c := NewClient(srv.Client(), srv.URL, "sensor1")
	runID := c.GetLastAnalysisRunID()
	if runID != "" {
		t.Errorf("expected empty string, got %s", runID)
	}
}

func TestGetLastAnalysisRunID_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewClient(srv.Client(), srv.URL, "sensor1")
	runID := c.GetLastAnalysisRunID()
	if runID != "" {
		t.Errorf("expected empty string for invalid JSON, got %s", runID)
	}
}

// --- GetSensorID ---

func TestGetSensorID(t *testing.T) {
	c := NewClient(nil, "http://localhost", "sensor-42")
	if c.GetSensorID() != "sensor-42" {
		t.Errorf("expected sensor-42, got %s", c.GetSensorID())
	}
}

// --- ClientBackend ---

func TestClientBackend_New(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.Client(), srv.URL, "sensor-1")
	cb := NewClientBackend(c)
	if cb.SensorID() != "sensor-1" {
		t.Errorf("expected sensor-1, got %s", cb.SensorID())
	}
}

func TestClientBackend_StartPCAPReplayWithConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.Client(), srv.URL, "sensor-1")
	cb := NewClientBackend(c)
	err := cb.StartPCAPReplayWithConfig(sweep.PCAPReplayConfig{
		PCAPFile:   "test.pcap",
		MaxRetries: 1,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- StartPCAPReplayWithSweepConfig ---

func TestStartPCAPReplayWithSweepConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.Client(), srv.URL, "sensor-1")
	err := c.StartPCAPReplayWithSweepConfig(sweep.PCAPReplayConfig{
		PCAPFile:   "test.pcap",
		MaxRetries: 1,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- StartPCAPReplayWithConfig all options ---

func TestStartPCAPReplayWithConfig_AllOptions(t *testing.T) {
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.Client(), srv.URL, "sensor-1")
	err := c.StartPCAPReplayWithConfig(PCAPReplayConfig{
		PCAPFile:        "test.pcap",
		StartSeconds:    5.0,
		DurationSeconds: 30.0,
		AnalysisMode:    true,
		SpeedMode:       "scaled",
		SpeedRatio:      2.0,
		MaxRetries:      1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received["analysis_mode"] != true {
		t.Errorf("expected analysis_mode=true, got %v", received["analysis_mode"])
	}
	if received["speed_mode"] != "scaled" {
		t.Errorf("expected speed_mode=scaled, got %v", received["speed_mode"])
	}
	if received["speed_ratio"].(float64) != 2.0 {
		t.Errorf("expected speed_ratio=2.0, got %v", received["speed_ratio"])
	}
	if received["duration_seconds"].(float64) != 30.0 {
		t.Errorf("expected duration_seconds=30.0, got %v", received["duration_seconds"])
	}
}

// --- SetTrackerConfig error path ---

func TestSetTrackerConfig_ConnectionError(t *testing.T) {
	c := NewClient(nil, "http://127.0.0.1:1", "sensor1")
	gating := 25.0
	err := c.SetTrackerConfig(TrackingParams{GatingDistanceSquared: &gating})
	if err == nil {
		t.Error("expected connection error")
	}
}

// --- FetchAcceptanceMetrics error path ---

func TestFetchAcceptanceMetrics_ConnectionError(t *testing.T) {
	c := NewClient(nil, "http://127.0.0.1:1", "sensor1")
	_, err := c.FetchAcceptanceMetrics()
	if err == nil {
		t.Error("expected connection error")
	}
}

// --- FetchGridStatus error path ---

func TestFetchGridStatus_ConnectionError(t *testing.T) {
	c := NewClient(nil, "http://127.0.0.1:1", "sensor1")
	_, err := c.FetchGridStatus()
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestFetchGridStatus_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewClient(srv.Client(), srv.URL, "sensor1")
	_, err := c.FetchGridStatus()
	if err == nil {
		t.Error("expected decode error")
	}
}

// --- SetTuningParams connection error ---

func TestSetTuningParams_ConnectionError(t *testing.T) {
	c := NewClient(nil, "http://127.0.0.1:1", "sensor1")
	err := c.SetTuningParams(map[string]interface{}{"noise_relative": 0.1})
	if err == nil {
		t.Error("expected connection error")
	}
}

// --- SetTrackerConfig request creation error ---

func TestSetTrackerConfig_RequestCreationError(t *testing.T) {
	c := NewClient(nil, "://bad-url", "sensor1")
	gating := 25.0
	err := c.SetTrackerConfig(TrackingParams{GatingDistanceSquared: &gating})
	if err == nil {
		t.Error("expected request creation error")
	}
}

// --- SetTuningParams request creation error ---

func TestSetTuningParams_RequestCreationError(t *testing.T) {
	c := NewClient(nil, "://bad-url", "sensor1")
	err := c.SetTuningParams(map[string]interface{}{"noise_relative": 0.1})
	if err == nil {
		t.Error("expected request creation error")
	}
}

// --- StartPCAPReplayWithConfig conflict and timeout ---

func TestStartPCAPReplayWithConfig_ConflictTimeout(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("conflict"))
	}))
	defer srv.Close()

	c := NewClient(srv.Client(), srv.URL, "sensor-1")
	err := c.StartPCAPReplayWithConfig(PCAPReplayConfig{
		PCAPFile:   "test.pcap",
		MaxRetries: 1,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestStartPCAPReplayWithConfig_RequestCreationError(t *testing.T) {
	c := NewClient(nil, "://bad-url", "sensor1")
	err := c.StartPCAPReplayWithConfig(PCAPReplayConfig{
		PCAPFile:   "test.pcap",
		MaxRetries: 1,
	})
	if err == nil {
		t.Fatal("expected request creation error")
	}
}

// --- FetchAcceptanceMetrics decode error ---

func TestFetchAcceptanceMetrics_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewClient(srv.Client(), srv.URL, "sensor1")
	_, err := c.FetchAcceptanceMetrics()
	if err == nil {
		t.Error("expected decode error")
	}
}
