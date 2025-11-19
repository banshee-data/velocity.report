package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/network"
)

func TestNewWebServer(t *testing.T) {
	stats := NewPacketStats()

	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		ForwardingEnabled: true,
		ForwardAddr:       "localhost",
		ForwardPort:       2368,
		ParsingEnabled:    true,
		UDPPort:           2369,
		DB:                nil,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}

	server := NewWebServer(config)

	if server == nil {
		t.Fatal("NewWebServer returned nil")
	}

	if server.stats != stats {
		t.Error("WebServer stats not set correctly")
	}

	if server.parsingEnabled != true {
		t.Error("WebServer parsingEnabled not set correctly")
	}

	if server.udpPort != 2369 {
		t.Error("WebServer udpPort not set correctly")
	}
}

func TestWebServer_StatusHandler(t *testing.T) {
	stats := NewPacketStats()

	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		ForwardingEnabled: false,
		ParsingEnabled:    true,
		UDPPort:           2369,
		DB:                nil,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}

	server := NewWebServer(config)

	// Add some stats data
	stats.AddPacket(1262)
	stats.AddPoints(400)
	stats.LogStats(true)

	// Create a request to the status endpoint
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()

	// Call the handler through the mux
	mux := server.setupRoutes()
	mux.ServeHTTP(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Status handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check that the response contains expected content
	body := rr.Body.String()

	if !strings.Contains(body, "LiDAR Monitor") {
		t.Error("Response should contain 'LiDAR Monitor'")
	}

	if !strings.Contains(body, "2369") {
		t.Error("Response should contain the UDP port")
	}
}

func TestWebServer_HealthHandler(t *testing.T) {
	stats := NewPacketStats()

	config := WebServerConfig{
		Address:        ":0",
		Stats:          stats,
		ParsingEnabled: false,
		UDPPort:        2369,
		UDPListenerConfig: network.UDPListenerConfig{
			Address: ":0",
		},
	}

	server := NewWebServer(config)

	// Create a request to the health endpoint
	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()

	// Call the handler through the mux
	mux := server.setupRoutes()
	mux.ServeHTTP(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Health handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check the content type
	expected := "application/json"
	if ctype := rr.Header().Get("Content-Type"); ctype != expected {
		t.Errorf("Health handler returned wrong content type: got %v want %v",
			ctype, expected)
	}

	// Check that the response contains JSON
	body := rr.Body.String()

	if !strings.Contains(body, `"status": "ok"`) {
		t.Error("Response should contain status: ok (with spaces)")
	}

	if !strings.Contains(body, `"service": "lidar"`) {
		t.Error("Response should contain service: lidar (with spaces)")
	}
}

func TestWebServer_StartStop(t *testing.T) {
	stats := NewPacketStats()

	config := WebServerConfig{
		Address:        ":0", // Use port 0 to get an available port
		Stats:          stats,
		ParsingEnabled: true,
		UDPPort:        2369,
		DB:             nil,
		UDPListenerConfig: network.UDPListenerConfig{
			Address: ":0",
		},
	}

	server := NewWebServer(config)

	// Start server with context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startedChan := make(chan struct{})
	errChan := make(chan error, 1)

	// Enable debug logging only when ACTIONS_STEP_DEBUG is set (GitHub Actions debug mode)
	debugLog := func(format string, args ...interface{}) {
		if os.Getenv("ACTIONS_STEP_DEBUG") == "true" {
			t.Logf(format, args...)
		}
	}

	go func() {
		debugLog("Starting server in goroutine...")
		// Signal that we've started attempting to start the server
		close(startedChan)

		err := server.Start(ctx)
		debugLog("Server.Start() returned with error: %v", err)

		// Only report errors that aren't expected shutdown errors
		if err != nil && err != http.ErrServerClosed && !strings.Contains(err.Error(), "context canceled") {
			debugLog("Sending unexpected error to errChan: %v", err)
			errChan <- err
		} else {
			debugLog("Server stopped cleanly (err=%v)", err)
		}
	}()

	// Wait for the goroutine to start
	<-startedChan
	debugLog("Server goroutine started")

	// Give the server more time to fully initialize
	// The UDP listener and HTTP server need time to bind to ports
	time.Sleep(200 * time.Millisecond)
	debugLog("Waited for server initialization")

	// Cancel the context to stop the server
	debugLog("Cancelling context to stop server")
	cancel()

	// Wait for the server to stop
	time.Sleep(200 * time.Millisecond)
	debugLog("Waited for server shutdown")

	// Check if there were any startup errors
	select {
	case err := <-errChan:
		t.Fatalf("Server start failed: %v", err)
	default:
		debugLog("No unexpected errors - test passed")
		// No error, which is what we expect
	}
}

func TestWebServer_ForwardingConfig(t *testing.T) {
	stats := NewPacketStats()

	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		ForwardingEnabled: true,
		ForwardAddr:       "192.168.1.100",
		ForwardPort:       2370,
		ParsingEnabled:    false,
		UDPPort:           3000,
		DB:                nil,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}

	server := NewWebServer(config)

	if !server.forwardingEnabled {
		t.Error("WebServer forwardingEnabled not set correctly")
	}

	if server.forwardAddr != "192.168.1.100" {
		t.Errorf("WebServer forwardAddr not set correctly: got %s, want 192.168.1.100", server.forwardAddr)
	}

	if server.forwardPort != 2370 {
		t.Errorf("WebServer forwardPort not set correctly: got %d, want 2370", server.forwardPort)
	}

	// Create a request to the status endpoint
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()

	// Call the handler
	mux := server.setupRoutes()
	mux.ServeHTTP(rr, req)

	// Check that the response contains forwarding info
	body := rr.Body.String()

	if !strings.Contains(body, "3000") {
		t.Error("Response should contain the correct UDP port")
	}
}

func TestWebServer_InvalidHTTPMethod(t *testing.T) {
	stats := NewPacketStats()

	config := WebServerConfig{
		Address:        ":0",
		Stats:          stats,
		ParsingEnabled: true,
		UDPPort:        2369,
		UDPListenerConfig: network.UDPListenerConfig{
			Address: ":0",
		},
	}

	server := NewWebServer(config)

	// Test POST request to status endpoint (should still work as it just shows the page)
	req, err := http.NewRequest("POST", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	mux := server.setupRoutes()
	mux.ServeHTTP(rr, req)

	// Should still return OK (the handler doesn't restrict methods)
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("POST to status handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}
}

func TestWebServer_DataSourceStatus(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:        ":0",
		Stats:          stats,
		ParsingEnabled: true,
		UDPPort:        2369,
		UDPListenerConfig: network.UDPListenerConfig{
			Address: ":0",
		},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodGet, "/api/lidar/data_source", nil)
	rr := httptest.NewRecorder()

	server.handleDataSource(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["data_source"] != string(DataSourceLive) {
		t.Errorf("expected data_source=live, got %v", resp["data_source"])
	}
}

func TestWebServer_DataSourceStatus_MethodNotAllowed(t *testing.T) {
	stats := NewPacketStats()
	config := WebServerConfig{
		Address:        ":0",
		Stats:          stats,
		ParsingEnabled: true,
		UDPPort:        2369,
		UDPListenerConfig: network.UDPListenerConfig{
			Address: ":0",
		},
	}
	server := NewWebServer(config)

	req := httptest.NewRequest(http.MethodPost, "/api/lidar/data_source", nil)
	rr := httptest.NewRecorder()

	server.handleDataSource(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 MethodNotAllowed, got %d", rr.Code)
	}
}

func BenchmarkWebServer_StatusHandler(b *testing.B) {
	stats := NewPacketStats()

	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		ParsingEnabled:    true,
		UDPPort:           2369,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}

	server := NewWebServer(config)

	// Add some stats data
	stats.AddPacket(1262)
	stats.AddPoints(400)
	stats.LogStats(true)

	// Create a request
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		b.Fatal(err)
	}

	mux := server.setupRoutes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
	}
}

func BenchmarkWebServer_HealthHandler(b *testing.B) {
	stats := NewPacketStats()

	config := WebServerConfig{
		Address:           ":0",
		Stats:             stats,
		ParsingEnabled:    true,
		UDPPort:           2369,
		UDPListenerConfig: network.UDPListenerConfig{Address: ":0"},
	}

	server := NewWebServer(config)

	// Create a request
	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		b.Fatal(err)
	}

	mux := server.setupRoutes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
	}
}
