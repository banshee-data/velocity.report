package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		SeedFromFirstFrame:         true,
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
		if payload["pcap_file"] != "test.pcap" {
			t.Errorf("Unexpected pcap_file: %s", payload["pcap_file"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.Client(), server.URL, "sensor1")
	err := c.StartPCAPReplay("/path/to/test.pcap", 1)

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
	err := c.StartPCAPReplay("/path/to/test.pcap", 1)

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
	err := c.StartPCAPReplay("/path/to/test.pcap", 1)

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
		SeedFromFirstFrame:         true,
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
	if decoded.SeedFromFirstFrame != params.SeedFromFirstFrame {
		t.Errorf("SeedFromFirstFrame mismatch")
	}
}
