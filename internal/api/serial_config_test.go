package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/serialmux"
)

func TestSerialConfigEndpoints(t *testing.T) {
	// Create a temporary database
	tmpDB, err := os.CreateTemp("", "test_api_serial_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp DB: %v", err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer database.Close()

	// Create a mock serial mux
	mockMux := serialmux.NewMockSerialMux([]byte(""))

	// Create the API server
	server := NewServer(mockMux, database, "mph", "UTC")
	mux := server.ServeMux()

	// Test GET /api/serial/configs - should return default config
	t.Run("GET /api/serial/configs", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/serial/configs", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var configs []db.SerialConfig
		if err := json.NewDecoder(w.Body).Decode(&configs); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(configs) != 1 {
			t.Errorf("Expected 1 config, got %d", len(configs))
		}

		if configs[0].Name != "Default HAT" {
			t.Errorf("Expected default config name 'Default HAT', got '%s'", configs[0].Name)
		}
	})

	// Test GET /api/serial/models
	t.Run("GET /api/serial/models", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/serial/models", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var models []SensorModel
		if err := json.NewDecoder(w.Body).Decode(&models); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(models) != 2 {
			t.Errorf("Expected 2 sensor models, got %d", len(models))
		}
	})

	// Test POST /api/serial/configs - create new config
	var createdID int
	t.Run("POST /api/serial/configs", func(t *testing.T) {
		reqBody := SerialConfigRequest{
			Name:        "Test USB Radar",
			PortPath:    "/dev/ttyUSB0",
			BaudRate:    19200,
			DataBits:    8,
			StopBits:    1,
			Parity:      "N",
			Enabled:     true,
			Description: "Test radar sensor",
			SensorModel: "ops243-a",
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/serial/configs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", w.Code)
		}

		var created db.SerialConfig
		if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if created.Name != reqBody.Name {
			t.Errorf("Expected name '%s', got '%s'", reqBody.Name, created.Name)
		}

		createdID = created.ID
	})

	// Test GET /api/serial/configs/:id
	t.Run("GET /api/serial/configs/:id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/serial/configs/"+fmt.Sprintf("%d", createdID), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var config db.SerialConfig
		if err := json.NewDecoder(w.Body).Decode(&config); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if config.ID != createdID {
			t.Errorf("Expected ID %d, got %d", createdID, config.ID)
		}
	})

	// Test PUT /api/serial/configs/:id
	t.Run("PUT /api/serial/configs/:id", func(t *testing.T) {
		updateReq := SerialConfigRequest{
			Name:        "Updated Test Radar",
			PortPath:    "/dev/ttyUSB0",
			BaudRate:    115200,
			DataBits:    8,
			StopBits:    1,
			Parity:      "N",
			Enabled:     false,
			Description: "Updated description",
			SensorModel: "ops243-c",
		}

		body, _ := json.Marshal(updateReq)
		req := httptest.NewRequest("PUT", "/api/serial/configs/"+fmt.Sprintf("%d", createdID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var updated db.SerialConfig
		if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if updated.Name != updateReq.Name {
			t.Errorf("Expected name '%s', got '%s'", updateReq.Name, updated.Name)
		}

		if updated.BaudRate != 115200 {
			t.Errorf("Expected baud rate 115200, got %d", updated.BaudRate)
		}
	})

	// Test DELETE /api/serial/configs/:id
	t.Run("DELETE /api/serial/configs/:id", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/serial/configs/"+fmt.Sprintf("%d", createdID), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("Expected status 204, got %d", w.Code)
		}
	})

	// Test invalid port path
	t.Run("POST /api/serial/configs with invalid port", func(t *testing.T) {
		reqBody := SerialConfigRequest{
			Name:        "Invalid Port",
			PortPath:    "/invalid/path",
			BaudRate:    19200,
			SensorModel: "ops243-a",
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/serial/configs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	// Test invalid sensor model
	t.Run("POST /api/serial/configs with invalid sensor model", func(t *testing.T) {
		reqBody := SerialConfigRequest{
			Name:        "Invalid Sensor",
			PortPath:    "/dev/ttyUSB0",
			BaudRate:    19200,
			SensorModel: "invalid-model",
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/serial/configs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})
}
