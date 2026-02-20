package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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

	// Test GET /api/serial/configs - should return configs (may be empty for fresh DB)
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

// ── Additional serial_config.go tests for comprehensive coverage ──

func TestSerialConfigsOrCreate_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("DELETE", "/api/serial/configs", nil)
	w := httptest.NewRecorder()
	server.handleSerialConfigsOrCreate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

func TestSerialConfigs_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("POST", "/api/serial/configs", nil)
	w := httptest.NewRecorder()
	server.handleSerialConfigs(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

func TestCreateSerialConfig_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("GET", "/api/serial/configs", nil)
	w := httptest.NewRecorder()
	server.handleCreateSerialConfig(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

func TestCreateSerialConfig_InvalidBody(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("POST", "/api/serial/configs", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	server.handleCreateSerialConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestCreateSerialConfig_EmptyName(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body, _ := json.Marshal(SerialConfigRequest{PortPath: "/dev/ttyUSB0", SensorModel: "ops243-a"})
	req := httptest.NewRequest("POST", "/api/serial/configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleCreateSerialConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestCreateSerialConfig_EmptyPortPath(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body, _ := json.Marshal(SerialConfigRequest{Name: "test", SensorModel: "ops243-a"})
	req := httptest.NewRequest("POST", "/api/serial/configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleCreateSerialConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestCreateSerialConfig_Defaults(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)
	mux := server.ServeMux()

	body, _ := json.Marshal(SerialConfigRequest{
		Name:        "DefaultsTest",
		PortPath:    "/dev/ttyUSB0",
		Enabled:     true,
		SensorModel: "ops243-a",
	})
	req := httptest.NewRequest("POST", "/api/serial/configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var cfg db.SerialConfig
	json.NewDecoder(w.Body).Decode(&cfg)
	if cfg.BaudRate != 19200 {
		t.Errorf("Default baud = %d, want 19200", cfg.BaudRate)
	}
	if cfg.DataBits != 8 {
		t.Errorf("Default data bits = %d, want 8", cfg.DataBits)
	}
	if cfg.StopBits != 1 {
		t.Errorf("Default stop bits = %d, want 1", cfg.StopBits)
	}
	if cfg.Parity != "N" {
		t.Errorf("Default parity = %q, want N", cfg.Parity)
	}
}

func TestCreateSerialConfig_DuplicateName(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)
	mux := server.ServeMux()

	body, _ := json.Marshal(SerialConfigRequest{
		Name: "Dup", PortPath: "/dev/ttyUSB0", SensorModel: "ops243-a", Enabled: true,
	})

	// First create
	req := httptest.NewRequest("POST", "/api/serial/configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("First create failed: %d %s", w.Code, w.Body.String())
	}

	// Second create with same name
	body, _ = json.Marshal(SerialConfigRequest{
		Name: "Dup", PortPath: "/dev/ttyUSB1", SensorModel: "ops243-a", Enabled: true,
	})
	req = httptest.NewRequest("POST", "/api/serial/configs", bytes.NewReader(body))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("Expected 409 for duplicate name, got %d", w.Code)
	}
}

func TestSerialConfigByID_MissingID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("GET", "/api/serial/configs/", nil)
	w := httptest.NewRecorder()
	server.handleSerialConfigByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestSerialConfigByID_InvalidID(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("GET", "/api/serial/configs/abc", nil)
	w := httptest.NewRecorder()
	server.handleSerialConfigByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestSerialConfigByID_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("PATCH", "/api/serial/configs/1", nil)
	w := httptest.NewRecorder()
	server.handleSerialConfigByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

func TestGetSerialConfig_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("GET", "/api/serial/configs/99999", nil)
	w := httptest.NewRecorder()
	server.handleGetSerialConfig(w, req, 99999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestUpdateSerialConfig_InvalidBody(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("PUT", "/api/serial/configs/1", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	server.handleUpdateSerialConfig(w, req, 1)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestUpdateSerialConfig_EmptyName(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body, _ := json.Marshal(SerialConfigRequest{PortPath: "/dev/ttyUSB0", SensorModel: "ops243-a"})
	req := httptest.NewRequest("PUT", "/api/serial/configs/1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleUpdateSerialConfig(w, req, 1)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestUpdateSerialConfig_EmptyPortPath(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body, _ := json.Marshal(SerialConfigRequest{Name: "X", SensorModel: "ops243-a"})
	req := httptest.NewRequest("PUT", "/api/serial/configs/1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleUpdateSerialConfig(w, req, 1)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestUpdateSerialConfig_InvalidPortPath(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body, _ := json.Marshal(SerialConfigRequest{Name: "X", PortPath: "/bad/path", SensorModel: "ops243-a"})
	req := httptest.NewRequest("PUT", "/api/serial/configs/1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleUpdateSerialConfig(w, req, 1)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestUpdateSerialConfig_InvalidSensorModel(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body, _ := json.Marshal(SerialConfigRequest{Name: "X", PortPath: "/dev/ttyUSB0", SensorModel: "bad"})
	req := httptest.NewRequest("PUT", "/api/serial/configs/1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleUpdateSerialConfig(w, req, 1)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestUpdateSerialConfig_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	body, _ := json.Marshal(SerialConfigRequest{
		Name: "X", PortPath: "/dev/ttyUSB0", SensorModel: "ops243-a",
		BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N",
	})
	req := httptest.NewRequest("PUT", "/api/serial/configs/99999", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleUpdateSerialConfig(w, req, 99999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteSerialConfig_NotFound(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("DELETE", "/api/serial/configs/99999", nil)
	w := httptest.NewRecorder()
	server.handleDeleteSerialConfig(w, req, 99999)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestSensorModels_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("POST", "/api/serial/models", nil)
	w := httptest.NewRecorder()
	server.handleSensorModels(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

func TestIsValidPortPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/dev/ttyUSB0", true},
		{"/dev/ttyACM0", true},
		{"/dev/ttySC1", true},
		{"/dev/serial/by-id/usb-foo", true},
		{"/dev/ttyAMA0", true},
		{"", false},
		{"/invalid/path", false},
		{"/dev/sda1", false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := isValidPortPath(tc.path)
			if got != tc.want {
				t.Errorf("isValidPortPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// ── serial_reload.go comprehensive tests ──

// mockSerialMuxForReload is a simple mock that satisfies SerialMuxInterface
// for reload testing without side effects.
type mockSerialMuxForReload struct {
	initErr  error
	closeErr error
	closed   bool
}

func (m *mockSerialMuxForReload) Subscribe() (string, chan string) {
	ch := make(chan string, 1)
	return "mock-sub", ch
}
func (m *mockSerialMuxForReload) Unsubscribe(string)                   {}
func (m *mockSerialMuxForReload) SendCommand(string) error             { return nil }
func (m *mockSerialMuxForReload) Monitor(ctx context.Context) error    { <-ctx.Done(); return ctx.Err() }
func (m *mockSerialMuxForReload) Close() error                         { m.closed = true; return m.closeErr }
func (m *mockSerialMuxForReload) Initialise() error                    { return m.initErr }
func (m *mockSerialMuxForReload) AttachAdminRoutes(mux *http.ServeMux) {}

func TestSerialPortManager_Initialise(t *testing.T) {
	mockMux := &mockSerialMuxForReload{}
	mgr := NewSerialPortManager(nil, mockMux, SerialConfigSnapshot{PortPath: "/dev/test"}, nil)
	defer mgr.Close()

	if err := mgr.Initialise(); err != nil {
		t.Errorf("Initialise() = %v, want nil", err)
	}
}

func TestSerialPortManager_Initialise_NilMux(t *testing.T) {
	mgr := NewSerialPortManager(nil, nil, SerialConfigSnapshot{}, nil)
	defer mgr.Close()

	if err := mgr.Initialise(); err == nil {
		t.Error("Initialise() with nil mux should return error")
	}
}

func TestSerialPortManager_Initialise_Closed(t *testing.T) {
	mockMux := &mockSerialMuxForReload{}
	mgr := NewSerialPortManager(nil, mockMux, SerialConfigSnapshot{PortPath: "/dev/test"}, nil)
	mgr.Close()
	time.Sleep(50 * time.Millisecond)

	if err := mgr.Initialise(); err == nil {
		t.Error("Initialise() after Close() should return error")
	}
}

func TestSerialPortManager_SendCommand_NilMux(t *testing.T) {
	mgr := NewSerialPortManager(nil, nil, SerialConfigSnapshot{}, nil)
	defer mgr.Close()

	if err := mgr.SendCommand("??"); err == nil {
		t.Error("SendCommand() with nil mux should return error")
	}
}

func TestSerialPortManager_AttachAdminRoutes(t *testing.T) {
	mockMux := &mockSerialMuxForReload{}
	mgr := NewSerialPortManager(nil, mockMux, SerialConfigSnapshot{PortPath: "/dev/test"}, nil)
	defer mgr.Close()

	httpMux := http.NewServeMux()
	// Should not panic
	mgr.AttachAdminRoutes(httpMux)
}

func TestSerialPortManager_AttachAdminRoutes_NilMux(t *testing.T) {
	mgr := NewSerialPortManager(nil, nil, SerialConfigSnapshot{}, nil)
	defer mgr.Close()

	httpMux := http.NewServeMux()
	// Should not panic with nil mux
	mgr.AttachAdminRoutes(httpMux)
}

func TestSerialPortManager_CurrentMux(t *testing.T) {
	mockMux := &mockSerialMuxForReload{}
	mgr := NewSerialPortManager(nil, mockMux, SerialConfigSnapshot{PortPath: "/dev/test"}, nil)
	defer mgr.Close()

	got := mgr.CurrentMux()
	if got != mockMux {
		t.Errorf("CurrentMux() returned wrong mux")
	}
}

func TestSerialPortManager_ReloadConfig_NoFactory(t *testing.T) {
	mgr := NewSerialPortManager(nil, nil, SerialConfigSnapshot{}, nil)
	defer mgr.Close()

	_, err := mgr.ReloadConfig(context.Background())
	if err == nil {
		t.Error("ReloadConfig() without factory should return error")
	}
	if !strings.Contains(err.Error(), "factory not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSerialPortManager_ReloadConfig_NoDB(t *testing.T) {
	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{}, nil
	}
	mgr := NewSerialPortManager(nil, nil, SerialConfigSnapshot{}, factory)
	defer mgr.Close()

	_, err := mgr.ReloadConfig(context.Background())
	if err == nil {
		t.Error("ReloadConfig() without db should return error")
	}
	if !strings.Contains(err.Error(), "database not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSerialPortManager_ReloadConfig_CancelledContext(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_cancelled_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()
	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{}, nil
	}
	mgr := NewSerialPortManager(database, nil, SerialConfigSnapshot{}, factory)
	defer mgr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = mgr.ReloadConfig(ctx)
	if err == nil {
		t.Error("ReloadConfig() with cancelled context should return error")
	}
}

func TestSerialPortManager_ReloadConfig_NoEnabledConfigs(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_no_configs_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()
	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{}, nil
	}
	mgr := NewSerialPortManager(database, nil, SerialConfigSnapshot{}, factory)
	defer mgr.Close()

	_, err = mgr.ReloadConfig(context.Background())
	if err == nil {
		t.Error("ReloadConfig() with no enabled configs should return error")
	}
	if !strings.Contains(err.Error(), "no enabled serial configurations") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSerialPortManager_ReloadConfig_AlreadyActive(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_active_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()
	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Insert an enabled config
	_, err = database.CreateSerialConfig(&db.SerialConfig{
		Name: "Active", PortPath: "/dev/ttyUSB0", BaudRate: 19200,
		DataBits: 8, StopBits: 1, Parity: "N", Enabled: true, SensorModel: "ops243-a",
	})
	if err != nil {
		t.Fatal(err)
	}

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{}, nil
	}

	// Pre-set the snapshot to match the config we just created
	snap := SerialConfigSnapshot{
		PortPath: "/dev/ttyUSB0",
		Options:  serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"},
	}
	mgr := NewSerialPortManager(database, &mockSerialMuxForReload{}, snap, factory)
	defer mgr.Close()

	result, err := mgr.ReloadConfig(context.Background())
	if err != nil {
		t.Fatalf("ReloadConfig() error = %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if !strings.Contains(result.Message, "already active") {
		t.Errorf("expected 'already active' message, got %q", result.Message)
	}
}

func TestSerialPortManager_ReloadConfig_DifferentPort(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_diff_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()
	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Insert an enabled config
	_, err = database.CreateSerialConfig(&db.SerialConfig{
		Name: "NewPort", PortPath: "/dev/ttyUSB1", BaudRate: 9600,
		DataBits: 8, StopBits: 1, Parity: "N", Enabled: true, SensorModel: "ops243-a",
	})
	if err != nil {
		t.Fatal(err)
	}

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{}, nil
	}

	// Current config is on a different port
	snap := SerialConfigSnapshot{
		PortPath: "/dev/ttyUSB0",
		Options:  serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"},
	}
	oldMux := &mockSerialMuxForReload{}
	mgr := NewSerialPortManager(database, oldMux, snap, factory)
	defer mgr.Close()

	result, err := mgr.ReloadConfig(context.Background())
	if err != nil {
		t.Fatalf("ReloadConfig() error = %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if !strings.Contains(result.Message, "Reloaded") {
		t.Errorf("expected 'Reloaded' message, got %q", result.Message)
	}
	if !oldMux.closed {
		t.Error("old mux should have been closed")
	}
}

func TestSerialPortManager_ReloadConfig_SamePort(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_same_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()
	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Insert an enabled config with different baud rate
	_, err = database.CreateSerialConfig(&db.SerialConfig{
		Name: "SamePort", PortPath: "/dev/ttyUSB0", BaudRate: 9600,
		DataBits: 8, StopBits: 1, Parity: "N", Enabled: true, SensorModel: "ops243-a",
	})
	if err != nil {
		t.Fatal(err)
	}

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{}, nil
	}

	// Current config is on the same port but different baud rate
	snap := SerialConfigSnapshot{
		PortPath: "/dev/ttyUSB0",
		Options:  serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"},
	}
	oldMux := &mockSerialMuxForReload{}
	mgr := NewSerialPortManager(database, oldMux, snap, factory)
	defer mgr.Close()

	result, err := mgr.ReloadConfig(context.Background())
	if err != nil {
		t.Fatalf("ReloadConfig() error = %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if !oldMux.closed {
		t.Error("old mux should have been closed for same-port reload")
	}
}

func TestSerialPortManager_ReloadConfig_FactoryError(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_factory_err_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()
	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	_, err = database.CreateSerialConfig(&db.SerialConfig{
		Name: "FailPort", PortPath: "/dev/ttyUSB0", BaudRate: 19200,
		DataBits: 8, StopBits: 1, Parity: "N", Enabled: true, SensorModel: "ops243-a",
	})
	if err != nil {
		t.Fatal(err)
	}

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return nil, errors.New("factory error: port not found")
	}

	mgr := NewSerialPortManager(database, nil, SerialConfigSnapshot{}, factory)
	defer mgr.Close()

	_, err = mgr.ReloadConfig(context.Background())
	if err == nil {
		t.Error("expected error from factory")
	}
	if !strings.Contains(err.Error(), "failed to open serial port") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSerialPortManager_ReloadConfig_SamePortFactoryError(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_same_err_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()
	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	_, err = database.CreateSerialConfig(&db.SerialConfig{
		Name: "FailSame", PortPath: "/dev/ttyUSB0", BaudRate: 9600,
		DataBits: 8, StopBits: 1, Parity: "N", Enabled: true, SensorModel: "ops243-a",
	})
	if err != nil {
		t.Fatal(err)
	}

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return nil, errors.New("factory error: port busy")
	}

	snap := SerialConfigSnapshot{
		PortPath: "/dev/ttyUSB0",
		Options:  serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"},
	}
	mgr := NewSerialPortManager(database, &mockSerialMuxForReload{}, snap, factory)
	defer mgr.Close()

	_, err = mgr.ReloadConfig(context.Background())
	if err == nil {
		t.Error("expected error from factory")
	}
}

func TestSerialPortManager_ReloadConfig_InitialiseError(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_init_err_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()
	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	_, err = database.CreateSerialConfig(&db.SerialConfig{
		Name: "InitFail", PortPath: "/dev/ttyUSB1", BaudRate: 19200,
		DataBits: 8, StopBits: 1, Parity: "N", Enabled: true, SensorModel: "ops243-a",
	})
	if err != nil {
		t.Fatal(err)
	}

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{initErr: errors.New("init failed")}, nil
	}

	mgr := NewSerialPortManager(database, nil, SerialConfigSnapshot{}, factory)
	defer mgr.Close()

	_, err = mgr.ReloadConfig(context.Background())
	if err == nil {
		t.Error("expected error from Initialise")
	}
	if !strings.Contains(err.Error(), "failed to initialise") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSerialPortManager_ReloadConfig_SamePortInitialiseError(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_same_init_err_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()
	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	_, err = database.CreateSerialConfig(&db.SerialConfig{
		Name: "SameInitFail", PortPath: "/dev/ttyUSB0", BaudRate: 9600,
		DataBits: 8, StopBits: 1, Parity: "N", Enabled: true, SensorModel: "ops243-a",
	})
	if err != nil {
		t.Fatal(err)
	}

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{initErr: errors.New("init failed on same port")}, nil
	}

	snap := SerialConfigSnapshot{
		PortPath: "/dev/ttyUSB0",
		Options:  serialmux.PortOptions{BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N"},
	}
	mgr := NewSerialPortManager(database, &mockSerialMuxForReload{}, snap, factory)
	defer mgr.Close()

	_, err = mgr.ReloadConfig(context.Background())
	if err == nil {
		t.Error("expected error from Initialise")
	}
	if !strings.Contains(err.Error(), "failed to initialise") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSerialPortManager_Monitor_CancelledContext(t *testing.T) {
	mockMux := &mockSerialMuxForReload{}
	mgr := NewSerialPortManager(nil, mockMux, SerialConfigSnapshot{PortPath: "/dev/test"}, nil)
	defer mgr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.Monitor(ctx)
	if err != context.Canceled {
		t.Errorf("Monitor() = %v, want context.Canceled", err)
	}
}

func TestSerialPortManager_Monitor_NilMuxCancelled(t *testing.T) {
	mgr := NewSerialPortManager(nil, nil, SerialConfigSnapshot{}, nil)
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := mgr.Monitor(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Monitor() = %v, want context.DeadlineExceeded", err)
	}
}

func TestSerialPortManager_Close_WithMuxCloseError(t *testing.T) {
	mockMux := &mockSerialMuxForReload{closeErr: errors.New("close failed")}
	mgr := NewSerialPortManager(nil, mockMux, SerialConfigSnapshot{PortPath: "/dev/test"}, nil)

	// Close should not return error (it only logs warnings)
	err := mgr.Close()
	if err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestSerialPortManager_Unsubscribe_UnknownID(t *testing.T) {
	mgr := NewSerialPortManager(nil, nil, SerialConfigSnapshot{}, nil)
	defer mgr.Close()

	// Should not panic
	mgr.Unsubscribe("nonexistent-id")
}

// ── server.go serial handler tests ──

func TestHandleSerialReload_MethodNotAllowed(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("GET", "/api/serial/reload", nil)
	w := httptest.NewRecorder()
	server.handleSerialReload(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", w.Code)
	}
}

func TestHandleSerialReload_NoManager(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	req := httptest.NewRequest("POST", "/api/serial/reload", nil)
	w := httptest.NewRecorder()
	server.handleSerialReload(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503, got %d", w.Code)
	}
}

func TestSetSerialManager_And_CurrentSerialMux(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Without manager, currentSerialMux returns the default mux
	mux1 := server.currentSerialMux()
	if mux1 == nil {
		t.Fatal("expected non-nil default mux")
	}

	// Set a manager
	mockMux := &mockSerialMuxForReload{}
	mgr := NewSerialPortManager(nil, mockMux, SerialConfigSnapshot{PortPath: "/dev/test"}, nil)
	defer mgr.Close()

	server.SetSerialManager(mgr)

	// Now currentSerialMux should return the manager's mux
	mux2 := server.currentSerialMux()
	if mux2 != mockMux {
		t.Error("expected currentSerialMux to return manager's mux")
	}
}

func TestCurrentSerialMux_FallbackWhenManagerMuxNil(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)

	// Manager with nil underlying mux
	mgr := NewSerialPortManager(nil, nil, SerialConfigSnapshot{}, nil)
	defer mgr.Close()

	server.SetSerialManager(mgr)

	// Should fall back to server's default mux
	mux := server.currentSerialMux()
	if mux == nil {
		t.Fatal("expected non-nil fallback mux")
	}
}

func TestHandleSerialReload_WithManager(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_handler_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Insert an enabled config
	_, err = database.CreateSerialConfig(&db.SerialConfig{
		Name: "Handler", PortPath: "/dev/ttyUSB0", BaudRate: 19200,
		DataBits: 8, StopBits: 1, Parity: "N", Enabled: true, SensorModel: "ops243-a",
	})
	if err != nil {
		t.Fatal(err)
	}

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{}, nil
	}

	disabledMux := serialmux.NewDisabledSerialMux()
	server := NewServer(disabledMux, database, "mph", "UTC")
	mgr := NewSerialPortManager(database, nil, SerialConfigSnapshot{}, factory)
	defer mgr.Close()
	server.SetSerialManager(mgr)

	req := httptest.NewRequest("POST", "/api/serial/reload", nil)
	w := httptest.NewRecorder()
	server.handleSerialReload(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result SerialReloadResult
	json.NewDecoder(w.Body).Decode(&result)
	if !result.Success {
		t.Errorf("Expected success, got: %+v", result)
	}
}

func TestHandleSerialReload_FailedReload(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_handler_fail_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()

	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// No enabled configs — will cause error
	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{}, nil
	}

	disabledMux := serialmux.NewDisabledSerialMux()
	server := NewServer(disabledMux, database, "mph", "UTC")
	mgr := NewSerialPortManager(database, nil, SerialConfigSnapshot{}, factory)
	defer mgr.Close()
	server.SetSerialManager(mgr)

	req := httptest.NewRequest("POST", "/api/serial/reload", nil)
	w := httptest.NewRecorder()
	server.handleSerialReload(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}
}

// ── DB-error path tests for serial_config.go ──

func TestSerialConfigs_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()
	defer cleanupClosedDB(t, fname)

	req := httptest.NewRequest("GET", "/api/serial/configs", nil)
	w := httptest.NewRecorder()
	server.handleSerialConfigs(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}
}

func TestGetSerialConfig_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()
	defer cleanupClosedDB(t, fname)

	req := httptest.NewRequest("GET", "/api/serial/configs/1", nil)
	w := httptest.NewRecorder()
	server.handleGetSerialConfig(w, req, 1)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}
}

func TestCreateSerialConfig_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()
	defer cleanupClosedDB(t, fname)

	body, _ := json.Marshal(SerialConfigRequest{
		Name: "X", PortPath: "/dev/ttyUSB0", SensorModel: "ops243-a",
		BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N",
	})
	req := httptest.NewRequest("POST", "/api/serial/configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleCreateSerialConfig(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}
}

func TestUpdateSerialConfig_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()
	defer cleanupClosedDB(t, fname)

	body, _ := json.Marshal(SerialConfigRequest{
		Name: "X", PortPath: "/dev/ttyUSB0", SensorModel: "ops243-a",
		BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N",
	})
	req := httptest.NewRequest("PUT", "/api/serial/configs/1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleUpdateSerialConfig(w, req, 1)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}
}

func TestDeleteSerialConfig_DBError(t *testing.T) {
	server, dbInst := setupTestServer(t)
	fname := t.Name() + ".db"
	dbInst.Close()
	defer cleanupClosedDB(t, fname)

	req := httptest.NewRequest("DELETE", "/api/serial/configs/1", nil)
	w := httptest.NewRecorder()
	server.handleDeleteSerialConfig(w, req, 1)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}
}

func TestUpdateSerialConfig_DuplicateNameConflict(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)
	mux := server.ServeMux()

	// Create two configs
	for _, name := range []string{"First", "Second"} {
		body, _ := json.Marshal(SerialConfigRequest{
			Name: name, PortPath: "/dev/ttyUSB0", SensorModel: "ops243-a",
			BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N", Enabled: true,
		})
		req := httptest.NewRequest("POST", "/api/serial/configs", bytes.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("Failed to create config %s: %d %s", name, w.Code, w.Body.String())
		}
	}

	// Try to rename Second to First — should get 409 Conflict
	body, _ := json.Marshal(SerialConfigRequest{
		Name: "First", PortPath: "/dev/ttyUSB1", SensorModel: "ops243-a",
		BaudRate: 19200, DataBits: 8, StopBits: 1, Parity: "N",
	})
	req := httptest.NewRequest("PUT", "/api/serial/configs/2", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

// ── Event fanout tests for serial_reload.go ──

func TestSerialPortManager_EventFanout_ForwardsEvents(t *testing.T) {
	// Create a controllable mock mux that allows sending events
	eventCh := make(chan string, 10)
	mockMux := &eventableMock{eventCh: eventCh}
	snap := SerialConfigSnapshot{PortPath: "/dev/test"}

	mgr := NewSerialPortManager(nil, mockMux, snap, nil)
	defer mgr.Close()

	// Subscribe to the manager
	subID, subCh := mgr.Subscribe()
	defer mgr.Unsubscribe(subID)

	// Give the fanout loop time to subscribe to the mux
	time.Sleep(100 * time.Millisecond)

	// Send an event through the mock mux's channel
	eventCh <- `{"speed": 42}`

	// Subscriber should receive the forwarded event
	select {
	case payload := <-subCh:
		if payload != `{"speed": 42}` {
			t.Errorf("Expected forwarded event, got %q", payload)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for forwarded event")
	}
}

func TestSerialPortManager_EventFanout_FullChannel(t *testing.T) {
	eventCh := make(chan string, 10)
	mockMux := &eventableMock{eventCh: eventCh}
	snap := SerialConfigSnapshot{PortPath: "/dev/test"}

	mgr := NewSerialPortManager(nil, mockMux, snap, nil)
	defer mgr.Close()

	// Subscribe, but don't read — channel should fill up
	subID, _ := mgr.Subscribe()
	defer mgr.Unsubscribe(subID)

	time.Sleep(100 * time.Millisecond)

	// Flood events — fanout should drop when subscriber is full
	for i := 0; i < 20; i++ {
		eventCh <- fmt.Sprintf("event-%d", i)
	}

	// Just ensure no panic/deadlock — give time for processing
	time.Sleep(200 * time.Millisecond)
}

func TestSerialPortManager_EventFanout_CloseWithActiveSubscription(t *testing.T) {
	eventCh := make(chan string, 10)
	mockMux := &eventableMock{eventCh: eventCh}
	snap := SerialConfigSnapshot{PortPath: "/dev/test"}

	mgr := NewSerialPortManager(nil, mockMux, snap, nil)

	// Subscribe
	_, subCh := mgr.Subscribe()

	time.Sleep(100 * time.Millisecond)

	// Close the manager — should clean up fanout
	mgr.Close()
	time.Sleep(100 * time.Millisecond)

	// Subscriber channel should be closed
	select {
	case _, ok := <-subCh:
		if ok {
			t.Error("Expected subscriber channel to be closed")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Timed out waiting for subscriber channel close")
	}
}

// eventableMock is a SerialMuxInterface mock with a controllable event channel
type eventableMock struct {
	eventCh chan string
	closeCh chan struct{}
}

func (m *eventableMock) Subscribe() (string, chan string) {
	return "mock-event-sub", m.eventCh
}
func (m *eventableMock) Unsubscribe(string)                   {}
func (m *eventableMock) SendCommand(string) error             { return nil }
func (m *eventableMock) Monitor(ctx context.Context) error    { <-ctx.Done(); return ctx.Err() }
func (m *eventableMock) Close() error                         { return nil }
func (m *eventableMock) Initialise() error                    { return nil }
func (m *eventableMock) AttachAdminRoutes(mux *http.ServeMux) {}

// ── Monitor error path tests ──

type errorMonitorMock struct {
	monitorErr error
	callCount  int
}

func (m *errorMonitorMock) Subscribe() (string, chan string) { return "m", make(chan string) }
func (m *errorMonitorMock) Unsubscribe(string)               {}
func (m *errorMonitorMock) SendCommand(string) error         { return nil }
func (m *errorMonitorMock) Monitor(ctx context.Context) error {
	m.callCount++
	if m.monitorErr != nil {
		return m.monitorErr
	}
	return nil
}
func (m *errorMonitorMock) Close() error                         { return nil }
func (m *errorMonitorMock) Initialise() error                    { return nil }
func (m *errorMonitorMock) AttachAdminRoutes(mux *http.ServeMux) {}

func TestSerialPortManager_Monitor_ErrorRetry(t *testing.T) {
	mock := &errorMonitorMock{monitorErr: errors.New("port disconnected")}
	mgr := NewSerialPortManager(nil, mock, SerialConfigSnapshot{PortPath: "/dev/test"}, nil)
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	err := mgr.Monitor(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Monitor() = %v, want DeadlineExceeded", err)
	}
	// Should have retried at least once
	if mock.callCount < 1 {
		t.Errorf("Expected at least 1 Monitor call, got %d", mock.callCount)
	}
}

func TestSerialPortManager_Monitor_CleanExit(t *testing.T) {
	mock := &errorMonitorMock{monitorErr: nil} // Clean exit (no error)
	mgr := NewSerialPortManager(nil, mock, SerialConfigSnapshot{PortPath: "/dev/test"}, nil)
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := mgr.Monitor(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Monitor() = %v, want DeadlineExceeded", err)
	}
}

func TestSerialPortManager_ReloadConfig_DBLoadError(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_db_err_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()
	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{}, nil
	}

	mgr := NewSerialPortManager(database, nil, SerialConfigSnapshot{}, factory)
	defer mgr.Close()

	// Close the database to force an error
	database.Close()

	_, err = mgr.ReloadConfig(context.Background())
	if err == nil {
		t.Error("Expected DB error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to load serial configurations") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSerialPortManager_ReloadConfig_InvalidNormalise(t *testing.T) {
	tmpDB, err := os.CreateTemp("", "test_reload_invalid_norm_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpDB.Name())
	tmpDB.Close()
	database, err := db.NewDB(tmpDB.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Insert config with invalid baud rate that passes DB CHECK but fails PortOptions.Normalise
	_, err = database.Exec(`INSERT INTO radar_serial_config (name, port_path, baud_rate, data_bits, stop_bits, parity, enabled, description, sensor_model)
		VALUES ('Bad', '/dev/ttyUSB0', 12345, 8, 1, 'N', 1, '', 'ops243-a')`)
	if err != nil {
		t.Fatal(err)
	}

	factory := func(path string, opts serialmux.PortOptions) (serialmux.SerialMuxInterface, error) {
		return &mockSerialMuxForReload{}, nil
	}

	mgr := NewSerialPortManager(database, nil, SerialConfigSnapshot{}, factory)
	defer mgr.Close()

	_, err = mgr.ReloadConfig(context.Background())
	if err == nil {
		t.Error("Expected normalise error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid serial configuration") {
		t.Errorf("unexpected error: %v", err)
	}
}
