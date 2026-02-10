package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient(nil, "http://localhost:8080", "sensor1")

	if c.HTTPClient == nil {
		t.Error("HTTPClient should not be nil")
	}
	if c.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL mismatch: got %s", c.BaseURL)
	}
	if c.SensorID != "sensor1" {
		t.Errorf("SensorID mismatch: got %s", c.SensorID)
	}
}

func TestNewClient_WithHTTPClient(t *testing.T) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	c := NewClient(httpClient, "http://localhost:8080", "sensor1")

	if c.HTTPClient != httpClient {
		t.Error("HTTPClient should be the provided client")
	}
}

func TestDefaultBuckets(t *testing.T) {
	buckets := DefaultBuckets()

	if len(buckets) != 11 {
		t.Errorf("Expected 11 default buckets, got %d", len(buckets))
	}
	if buckets[0] != "1" {
		t.Errorf("First bucket should be '1', got %s", buckets[0])
	}
	if buckets[10] != "200" {
		t.Errorf("Last bucket should be '200', got %s", buckets[10])
	}
}

func TestClient_FetchBuckets_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"BucketsMeters": []interface{}{1.0, 2.0, 4.0, 8.0},
		})
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	buckets := c.FetchBuckets()

	if len(buckets) != 4 {
		t.Errorf("Expected 4 buckets, got %d", len(buckets))
	}
	if buckets[0] != "1" {
		t.Errorf("First bucket should be '1', got %s", buckets[0])
	}
}

func TestClient_FetchBuckets_WithFractionalValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"BucketsMeters": []interface{}{1.5, 2.5},
		})
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	buckets := c.FetchBuckets()

	if len(buckets) != 2 {
		t.Errorf("Expected 2 buckets, got %d", len(buckets))
	}
	if buckets[0] != "1.500000" {
		t.Errorf("First bucket should be '1.500000', got %s", buckets[0])
	}
}

func TestClient_FetchBuckets_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	buckets := c.FetchBuckets()

	// Should return defaults
	if len(buckets) != 11 {
		t.Errorf("Expected 11 default buckets, got %d", len(buckets))
	}
}

func TestClient_FetchBuckets_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	buckets := c.FetchBuckets()

	// Should return defaults on error
	if len(buckets) != 11 {
		t.Errorf("Expected 11 default buckets, got %d", len(buckets))
	}
}

func TestClient_FetchBuckets_ExcessiveBuckets(t *testing.T) {
	// Test that excessive bucket counts are rejected to prevent DoS
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate 150 buckets (exceeds max of 100)
		buckets := make([]interface{}, 150)
		for i := range buckets {
			buckets[i] = float64(i + 1)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"BucketsMeters": buckets,
		})
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	buckets := c.FetchBuckets()

	// Should return defaults when bucket count exceeds maximum
	if len(buckets) != 11 {
		t.Errorf("Expected 11 default buckets (DoS protection), got %d", len(buckets))
	}
}

func TestClient_ResetGrid_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/lidar/grid_reset" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	err := c.ResetGrid()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestClient_ResetGrid_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	err := c.ResetGrid()

	if err == nil {
		t.Error("Expected error")
	}
}

func TestClient_SetParams_Success(t *testing.T) {
	var receivedParams BackgroundParams

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected application/json content type")
		}
		json.NewDecoder(r.Body).Decode(&receivedParams)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	params := BackgroundParams{
		NoiseRelative:              0.1,
		ClosenessMultiplier:        2.0,
		NeighbourConfirmationCount: 3,
		SeedFromFirst:              true,
	}
	err := c.SetParams(params)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if receivedParams.NoiseRelative != 0.1 {
		t.Errorf("NoiseRelative mismatch: got %f", receivedParams.NoiseRelative)
	}
	if receivedParams.NeighbourConfirmationCount != 3 {
		t.Errorf("NeighbourConfirmationCount mismatch: got %d", receivedParams.NeighbourConfirmationCount)
	}
}

func TestClient_SetParams_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid params"))
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	err := c.SetParams(BackgroundParams{})

	if err == nil {
		t.Error("Expected error")
	}
}

func TestClient_ResetAcceptance_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/lidar/acceptance/reset" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	err := c.ResetAcceptance()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestClient_WaitForGridSettle_ImmediateSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"background_count": 100.0,
		})
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")

	start := time.Now()
	c.WaitForGridSettle(5 * time.Second)
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("Should have returned immediately, took %v", elapsed)
	}
}

func TestClient_WaitForGridSettle_ZeroTimeout(t *testing.T) {
	c := NewClient(nil, "http://localhost:8080", "sensor1")

	start := time.Now()
	c.WaitForGridSettle(0)
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("Should have returned immediately for zero timeout, took %v", elapsed)
	}
}

func TestClient_WaitForGridSettle_NegativeTimeout(t *testing.T) {
	c := NewClient(nil, "http://localhost:8080", "sensor1")

	start := time.Now()
	c.WaitForGridSettle(-1 * time.Second)
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("Should have returned immediately for negative timeout, took %v", elapsed)
	}
}

func TestClient_FetchAcceptanceMetrics_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"AcceptCounts": []interface{}{100.0, 200.0},
			"RejectCounts": []interface{}{10.0, 20.0},
		})
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	metrics, err := c.FetchAcceptanceMetrics()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if metrics == nil {
		t.Error("Expected metrics")
	}
	if metrics["AcceptCounts"] == nil {
		t.Error("Expected AcceptCounts")
	}
}

func TestClient_FetchAcceptanceMetrics_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	_, err := c.FetchAcceptanceMetrics()

	if err == nil {
		t.Error("Expected error")
	}
}

func TestClient_FetchGridStatus_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"background_count": 150.0,
			"total_cells":      1000.0,
		})
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	status, err := c.FetchGridStatus()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if status == nil {
		t.Error("Expected status")
	}
	if status["background_count"].(float64) != 150.0 {
		t.Errorf("Unexpected background_count: %v", status["background_count"])
	}
}

func TestClient_StartPCAPReplay_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected application/json content type")
		}
		var payload map[string]string
		json.NewDecoder(r.Body).Decode(&payload)
		// The path should be passed through as-is (relative to server's PCAP safe directory)
		if payload["pcap_file"] != "captures/test.pcap" {
			t.Errorf("Unexpected pcap_file: %s", payload["pcap_file"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	err := c.StartPCAPReplay("captures/test.pcap", 1)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestClient_StartPCAPReplay_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	err := c.StartPCAPReplay("captures/test.pcap", 1)

	if err == nil {
		t.Error("Expected error")
	}
}

func TestClient_StartPCAPReplay_Timeout(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("conflict"))
	}))
	defer server.Close()

	// Use a very short timeout client
	httpClient := &http.Client{Timeout: 100 * time.Millisecond}
	c := NewClient(httpClient, server.URL, "sensor1")

	// Set maxRetries to 1 to speed up the test
	err := c.StartPCAPReplay("captures/test.pcap", 1)

	if err == nil {
		t.Error("Expected timeout error")
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestBackgroundParams_JSONEncoding(t *testing.T) {
	params := BackgroundParams{
		NoiseRelative:              0.15,
		ClosenessMultiplier:        2.5,
		NeighbourConfirmationCount: 4,
		SeedFromFirst:              true,
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded BackgroundParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.NoiseRelative != params.NoiseRelative {
		t.Errorf("NoiseRelative mismatch")
	}
	if decoded.ClosenessMultiplier != params.ClosenessMultiplier {
		t.Errorf("ClosenessMultiplier mismatch")
	}
	if decoded.NeighbourConfirmationCount != params.NeighbourConfirmationCount {
		t.Errorf("NeighbourConfirmationCount mismatch")
	}
	if decoded.SeedFromFirst != params.SeedFromFirst {
		t.Errorf("SeedFromFirst mismatch")
	}
}

// ====== FetchTrackingMetrics tests ======

func TestClient_FetchTrackingMetrics_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/api/lidar/tracks/metrics") {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"total_tracks":           10,
			"mean_velocity_residual": 0.15,
			"alignment_score":        0.92,
		})
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	metrics, err := c.FetchTrackingMetrics()

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if metrics == nil {
		t.Fatal("Expected metrics, got nil")
	}
	if metrics["total_tracks"].(float64) != 10 {
		t.Errorf("Expected total_tracks=10, got %v", metrics["total_tracks"])
	}
}

func TestClient_FetchTrackingMetrics_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	_, err := c.FetchTrackingMetrics()

	if err == nil {
		t.Error("Expected error for server error response")
	}
}

func TestClient_FetchTrackingMetrics_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	_, err := c.FetchTrackingMetrics()

	if err == nil {
		t.Error("Expected error for invalid JSON response")
	}
}

// ====== SetTrackerConfig tests ======

func TestClient_SetTrackerConfig_Success(t *testing.T) {
	var receivedParams map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected application/json content type, got %s", r.Header.Get("Content-Type"))
		}
		if !strings.Contains(r.URL.Path, "/api/lidar/params") {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		// Verify sensor_id in query params
		if r.URL.Query().Get("sensor_id") != "sensor1" {
			t.Errorf("Expected sensor_id=sensor1, got %s", r.URL.Query().Get("sensor_id"))
		}
		json.NewDecoder(r.Body).Decode(&receivedParams)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	gatingDist := 25.0
	processNoisePos := 0.5
	params := TrackingParams{
		GatingDistanceSquared: &gatingDist,
		ProcessNoisePos:       &processNoisePos,
	}
	err := c.SetTrackerConfig(params)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if receivedParams["gating_distance_squared"].(float64) != 25.0 {
		t.Errorf("Expected gating_distance_squared=25.0, got %v", receivedParams["gating_distance_squared"])
	}
}

func TestClient_SetTrackerConfig_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid params"))
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	gatingDist := 25.0
	params := TrackingParams{
		GatingDistanceSquared: &gatingDist,
	}
	err := c.SetTrackerConfig(params)

	if err == nil {
		t.Error("Expected error for server error response")
	}
}

func TestClient_SetTrackerConfig_AllParams(t *testing.T) {
	var receivedParams map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedParams)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	gatingDist := 25.0
	processNoisePos := 0.5
	processNoiseVel := 1.0
	measurementNoise := 0.1
	params := TrackingParams{
		GatingDistanceSquared: &gatingDist,
		ProcessNoisePos:       &processNoisePos,
		ProcessNoiseVel:       &processNoiseVel,
		MeasurementNoise:      &measurementNoise,
	}
	err := c.SetTrackerConfig(params)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if receivedParams["process_noise_vel"].(float64) != 1.0 {
		t.Errorf("Expected process_noise_vel=1.0, got %v", receivedParams["process_noise_vel"])
	}
	if receivedParams["measurement_noise"].(float64) != 0.1 {
		t.Errorf("Expected measurement_noise=0.1, got %v", receivedParams["measurement_noise"])
	}
}

func TestTrackingParams_JSONOmitEmpty(t *testing.T) {
	// Test that nil fields are omitted
	params := TrackingParams{}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("Expected empty JSON object, got %s", string(data))
	}

	// Test partial params
	gatingDist := 25.0
	params2 := TrackingParams{GatingDistanceSquared: &gatingDist}
	data2, err := json.Marshal(params2)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !strings.Contains(string(data2), "gating_distance_squared") {
		t.Errorf("Expected gating_distance_squared in JSON, got %s", string(data2))
	}
}

func TestClient_SetTuningParams_Success(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/lidar/params" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	err := c.SetTuningParams(map[string]interface{}{
		"noise_relative":       0.15,
		"closeness_multiplier": 2.2,
	})
	if err != nil {
		t.Fatalf("SetTuningParams failed: %v", err)
	}

	if received["noise_relative"].(float64) != 0.15 {
		t.Fatalf("expected noise_relative=0.15, got %v", received["noise_relative"])
	}
	if received["closeness_multiplier"].(float64) != 2.2 {
		t.Fatalf("expected closeness_multiplier=2.2, got %v", received["closeness_multiplier"])
	}
}

func TestClient_SetTuningParams_MarshalError(t *testing.T) {
	c := NewClient(nil, "http://localhost:8080", "sensor1")
	err := c.SetTuningParams(map[string]interface{}{
		"invalid": make(chan int),
	})
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestClient_SetTuningParams_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid tuning parameters"))
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	err := c.SetTuningParams(map[string]interface{}{"noise_relative": 0.1})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_StartPCAPReplayWithConfig_Success(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/lidar/pcap/start" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("sensor_id") != "sensor1" {
			t.Errorf("Expected sensor_id=sensor1, got %s", r.URL.Query().Get("sensor_id"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	err := c.StartPCAPReplayWithConfig(PCAPReplayConfig{
		PCAPFile:        "captures/sample.pcapng",
		StartSeconds:    12.5,
		DurationSeconds: 30,
		AnalysisMode:    true,
		MaxRetries:      1,
	})
	if err != nil {
		t.Fatalf("StartPCAPReplayWithConfig failed: %v", err)
	}

	if received["pcap_file"] != "captures/sample.pcapng" {
		t.Fatalf("unexpected pcap_file: %v", received["pcap_file"])
	}
	if received["start_seconds"].(float64) != 12.5 {
		t.Fatalf("unexpected start_seconds: %v", received["start_seconds"])
	}
	if received["duration_seconds"].(float64) != 30 {
		t.Fatalf("unexpected duration_seconds: %v", received["duration_seconds"])
	}
	if received["analysis_mode"].(bool) != true {
		t.Fatalf("unexpected analysis_mode: %v", received["analysis_mode"])
	}
}

func TestClient_StartPCAPReplayWithConfig_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid replay config"))
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	err := c.StartPCAPReplayWithConfig(PCAPReplayConfig{
		PCAPFile:   "captures/sample.pcapng",
		MaxRetries: 1,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_StopPCAPReplay(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("Expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/api/lidar/pcap/stop" {
				t.Errorf("Unexpected path: %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := NewClient(server.Client(), server.URL, "sensor1")
		if err := c.StopPCAPReplay(); err != nil {
			t.Fatalf("StopPCAPReplay failed: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("stop failed"))
		}))
		defer server.Close()

		c := NewClient(server.Client(), server.URL, "sensor1")
		if err := c.StopPCAPReplay(); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestClient_WaitForPCAPComplete(t *testing.T) {
	t.Run("complete immediately", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{"pcap_in_progress": false})
		}))
		defer server.Close()

		c := NewClient(server.Client(), server.URL, "sensor1")
		if err := c.WaitForPCAPComplete(2 * time.Second); err != nil {
			t.Fatalf("WaitForPCAPComplete failed: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{"pcap_in_progress": true})
		}))
		defer server.Close()

		c := NewClient(server.Client(), server.URL, "sensor1")
		err := c.WaitForPCAPComplete(100 * time.Millisecond)
		if err == nil {
			t.Fatal("expected timeout error")
		}
		if !strings.Contains(err.Error(), "timeout") {
			t.Fatalf("expected timeout error, got %v", err)
		}
	})
}

func TestClient_StartPCAPReplay_DefaultRetriesAndRequestError(t *testing.T) {
	// maxRetries <= 0 should use the default retry count without sleeping when
	// the server immediately returns a non-conflict error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	if err := c.StartPCAPReplay("captures/test.pcap", 0); err == nil {
		t.Fatal("expected error for immediate bad-request response")
	}

	// Invalid base URL should fail request creation.
	bad := NewClient(nil, "://bad-url", "sensor1")
	if err := bad.StartPCAPReplay("captures/test.pcap", 1); err == nil {
		t.Fatal("expected request creation error")
	}
}

func TestClient_StartPCAPReplayWithConfig_DefaultsAndOptionalFields(t *testing.T) {
	t.Run("optional fields omitted when zero-values", func(t *testing.T) {
		var received map[string]interface{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := NewClient(server.Client(), server.URL, "sensor1")
		err := c.StartPCAPReplayWithConfig(PCAPReplayConfig{
			PCAPFile:   "captures/sample.pcapng",
			MaxRetries: 1,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if received["pcap_file"] != "captures/sample.pcapng" {
			t.Fatalf("unexpected pcap_file: %v", received["pcap_file"])
		}
		if _, ok := received["start_seconds"]; ok {
			t.Fatalf("start_seconds should be omitted for zero value, got %v", received["start_seconds"])
		}
		if _, ok := received["duration_seconds"]; ok {
			t.Fatalf("duration_seconds should be omitted for zero value, got %v", received["duration_seconds"])
		}
		if _, ok := received["analysis_mode"]; ok {
			t.Fatalf("analysis_mode should be omitted when false, got %v", received["analysis_mode"])
		}
	})

	t.Run("default max-retries branch and request creation error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("bad config"))
		}))
		defer server.Close()

		c := NewClient(server.Client(), server.URL, "sensor1")
		if err := c.StartPCAPReplayWithConfig(PCAPReplayConfig{PCAPFile: "captures/sample.pcapng"}); err == nil {
			t.Fatal("expected error")
		}

		bad := NewClient(nil, "://bad-url", "sensor1")
		if err := bad.StartPCAPReplayWithConfig(PCAPReplayConfig{PCAPFile: "captures/sample.pcapng", MaxRetries: 1}); err == nil {
			t.Fatal("expected request creation error")
		}
	})
}

func TestClient_RequestCreationErrors(t *testing.T) {
	c := NewClient(nil, "://bad-url", "sensor1")

	if err := c.ResetGrid(); err == nil {
		t.Fatal("expected ResetGrid request creation error")
	}
	if err := c.SetParams(BackgroundParams{}); err == nil {
		t.Fatal("expected SetParams request creation error")
	}
	if err := c.ResetAcceptance(); err == nil {
		t.Fatal("expected ResetAcceptance request creation error")
	}
	if err := c.StopPCAPReplay(); err == nil {
		t.Fatal("expected StopPCAPReplay request creation error")
	}
}

func TestClient_WaitForPCAPComplete_DefaultTimeoutAndDecodeRetry(t *testing.T) {
	t.Run("timeout defaults when <= 0", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{"pcap_in_progress": false})
		}))
		defer server.Close()

		c := NewClient(server.Client(), server.URL, "sensor1")
		if err := c.WaitForPCAPComplete(0); err != nil {
			t.Fatalf("expected immediate success with default timeout, got %v", err)
		}
	})

	t.Run("invalid JSON retries until success", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.Write([]byte("{invalid json"))
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"pcap_in_progress": false})
		}))
		defer server.Close()

		c := NewClient(server.Client(), server.URL, "sensor1")
		if err := c.WaitForPCAPComplete(2 * time.Second); err != nil {
			t.Fatalf("expected eventual success, got %v", err)
		}
		if calls < 2 {
			t.Fatalf("expected at least 2 polls, got %d", calls)
		}
	})

	t.Run("request errors time out", func(t *testing.T) {
		// Unroutable endpoint triggers HTTP request errors and exercises retry path.
		c := NewClient(nil, "http://127.0.0.1:1", "sensor1")
		start := time.Now()
		err := c.WaitForPCAPComplete(1 * time.Millisecond)
		if err == nil {
			t.Fatal("expected timeout error")
		}
		if time.Since(start) < 400*time.Millisecond {
			t.Fatal("expected at least one retry sleep before timeout")
		}
	})
}
