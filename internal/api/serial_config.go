package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/security"
	"github.com/banshee-data/velocity.report/internal/serialmux"
)

// normaliseSerialConfigRequest applies serialmux.PortOptions defaults and
// validation to the wire fields, then writes them back to the request so
// downstream persistence sees the same canonicalised values that the live
// serial layer would. Returns a 400-suitable error on invalid input.
func normaliseSerialConfigRequest(req *SerialConfigRequest) error {
	opts := serialmux.PortOptions{
		BaudRate: req.BaudRate,
		DataBits: req.DataBits,
		StopBits: req.StopBits,
		Parity:   req.Parity,
	}
	normalised, err := opts.Normalise()
	if err != nil {
		return err
	}
	req.BaudRate = normalised.BaudRate
	req.DataBits = normalised.DataBits
	req.StopBits = normalised.StopBits
	req.Parity = normalised.Parity
	return nil
}

// SerialConfigRequest represents the request body for creating/updating serial configs
type SerialConfigRequest struct {
	PortPath    string `json:"port_path"`
	BaudRate    int    `json:"baud_rate"`
	DataBits    int    `json:"data_bits"`
	StopBits    int    `json:"stop_bits"`
	Parity      string `json:"parity"`
	Enabled     bool   `json:"enabled"`
	SensorModel string `json:"sensor_model"`
}

// handleSerialConfigsOrCreate handles GET and POST to /api/serial/configs
func (s *Server) handleSerialConfigsOrCreate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleSerialConfigs(w, r)
	case http.MethodPost:
		s.handleCreateSerialConfig(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSerialConfigs handles GET /api/serial/configs - List all serial configurations
func (s *Server) handleSerialConfigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	configs, err := s.db.GetSerialConfigs()
	if err != nil {
		log.Printf("Error fetching serial configs: %v", err)
		http.Error(w, "Failed to fetch serial configurations", http.StatusInternalServerError)
		return
	}

	// A nil slice marshals as JSON `null`, which crashes frontend callers that
	// iterate the list (e.g. rebuildPortPathOptions' .forEach). Force `[]` for
	// an empty config set so the wire shape is always an array.
	if configs == nil {
		configs = []db.SerialConfig{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(configs); err != nil {
		log.Printf("Error encoding serial configs response: %v", err)
	}
}

// handleSerialConfigByID handles GET/PUT/DELETE /api/serial/configs/:id
func (s *Server) handleSerialConfigByID(w http.ResponseWriter, r *http.Request) {
	// Extract ID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/serial/configs/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		http.Error(w, "Missing config ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(pathParts[0])
	if err != nil {
		http.Error(w, "Invalid config ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetSerialConfig(w, r, id)
	case http.MethodPut:
		s.handleUpdateSerialConfig(w, r, id)
	case http.MethodDelete:
		s.handleDeleteSerialConfig(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetSerialConfig handles GET /api/serial/configs/:id
func (s *Server) handleGetSerialConfig(w http.ResponseWriter, _ *http.Request, id int) {
	config, err := s.db.GetSerialConfig(id)
	if err != nil {
		log.Printf("Error fetching serial config %d: %v", id, err)
		http.Error(w, "Failed to fetch serial configuration", http.StatusInternalServerError)
		return
	}

	if config == nil {
		http.Error(w, "Configuration not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(config); err != nil {
		log.Printf("Error encoding serial config %d response: %v", id, err)
	}
}

// handleCreateSerialConfig handles POST /api/serial/configs
func (s *Server) handleCreateSerialConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SerialConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.PortPath == "" {
		http.Error(w, "Port path is required", http.StatusBadRequest)
		return
	}

	// Validate port path format
	if !isValidPortPath(req.PortPath) {
		http.Error(w, "Invalid port path. Must be a serial device under /dev/ (e.g. /dev/ttyUSB0, /dev/serial0, /dev/cu.usbserial-*)", http.StatusBadRequest)
		return
	}

	// Validate sensor model
	if _, ok := GetSensorModel(req.SensorModel); !ok {
		http.Error(w, fmt.Sprintf("Unsupported sensor model: %s", req.SensorModel), http.StatusBadRequest)
		return
	}

	// Apply defaults and validate against the same rules the live serial
	// path uses. Without this, invalid baud/data/stop/parity values land
	// in SQLite and only fail later when the runtime tries to open the
	// port — surfacing as a confusing reload failure rather than a clear
	// API rejection.
	if err := normaliseSerialConfigRequest(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid serial parameters: %v", err), http.StatusBadRequest)
		return
	}

	config := &db.SerialConfig{
		PortPath:    req.PortPath,
		BaudRate:    req.BaudRate,
		DataBits:    req.DataBits,
		StopBits:    req.StopBits,
		Parity:      req.Parity,
		Enabled:     req.Enabled,
		SensorModel: req.SensorModel,
	}

	id, err := s.db.CreateSerialConfig(config)
	if err != nil {
		log.Printf("Error creating serial config: %v", err)
		if db.IsSerialConfigPortPathConflict(err) {
			http.Error(w, "Port path is already configured", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to create serial configuration", http.StatusInternalServerError)
		return
	}

	// Fetch the created config to return it
	created, err := s.db.GetSerialConfig(int(id))
	if err != nil {
		log.Printf("Error fetching created config: %v", err)
		http.Error(w, "Configuration created but failed to fetch", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(created); err != nil {
		log.Printf("Error encoding created serial config response: %v", err)
	}
}

// handleUpdateSerialConfig handles PUT /api/serial/configs/:id
func (s *Server) handleUpdateSerialConfig(w http.ResponseWriter, r *http.Request, id int) {
	var req SerialConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.PortPath == "" {
		http.Error(w, "Port path is required", http.StatusBadRequest)
		return
	}

	// Validate port path format
	if !isValidPortPath(req.PortPath) {
		http.Error(w, "Invalid port path. Must be a serial device under /dev/ (e.g. /dev/ttyUSB0, /dev/serial0, /dev/cu.usbserial-*)", http.StatusBadRequest)
		return
	}

	// Validate sensor model
	if _, ok := GetSensorModel(req.SensorModel); !ok {
		http.Error(w, fmt.Sprintf("Unsupported sensor model: %s", req.SensorModel), http.StatusBadRequest)
		return
	}

	// Same normalisation as the create handler — the update path previously
	// didn't even apply defaults, so a stop_bits=0 or parity="" update
	// would persist garbage that the runtime later rejected.
	if err := normaliseSerialConfigRequest(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid serial parameters: %v", err), http.StatusBadRequest)
		return
	}

	config := &db.SerialConfig{
		ID:          id,
		PortPath:    req.PortPath,
		BaudRate:    req.BaudRate,
		DataBits:    req.DataBits,
		StopBits:    req.StopBits,
		Parity:      req.Parity,
		Enabled:     req.Enabled,
		SensorModel: req.SensorModel,
	}

	err := s.db.UpdateSerialConfig(config)
	if err != nil {
		log.Printf("Error updating serial config %d: %v", id, err)
		if db.IsSerialConfigPortPathConflict(err) {
			http.Error(w, "Port path is already configured", http.StatusConflict)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Configuration not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to update serial configuration", http.StatusInternalServerError)
		return
	}

	// Fetch the updated config to return it
	updated, err := s.db.GetSerialConfig(id)
	if err != nil {
		log.Printf("Error fetching updated config: %v", err)
		http.Error(w, "Configuration updated but failed to fetch", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(updated); err != nil {
		log.Printf("Error encoding updated serial config response: %v", err)
	}
}

// handleDeleteSerialConfig handles DELETE /api/serial/configs/:id
func (s *Server) handleDeleteSerialConfig(w http.ResponseWriter, _ *http.Request, id int) {
	err := s.db.DeleteSerialConfig(id)
	if err != nil {
		log.Printf("Error deleting serial config %d: %v", id, err)
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Configuration not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to delete serial configuration", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleSensorModels handles GET /api/serial/models - List all sensor models
func (s *Server) handleSensorModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	models := GetAllSensorModels()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(models); err != nil {
		log.Printf("Error encoding sensor models response: %v", err)
	}
}

// isValidPortPath validates that a port path is a serial device under /dev/
// and is not a symlink-escape attack. The decisive security check is that the
// resolved path stays within /dev/; the prefix allowlist keeps the surface
// tight so an unauthenticated caller cannot point the tester at block or
// memory devices (/dev/sda, /dev/mem).
//
// Allowed prefixes cover every device class the enumeration can list:
//   - /dev/tty…     Linux UARTs and USB serial (ttyS, ttyUSB, ttyACM, ttyAMA,
//     ttySC) plus macOS dial-in nodes (/dev/tty.*)
//   - /dev/serial…  Raspberry Pi symlinks (serial0/serial1, by-id, by-path)
//   - /dev/cu.…     macOS call-out devices (/dev/cu.usbserial-*, cu.usbmodem*)
func isValidPortPath(path string) bool {
	if path == "" {
		return false
	}

	validPrefix := strings.HasPrefix(path, "/dev/tty") ||
		strings.HasPrefix(path, "/dev/serial") ||
		strings.HasPrefix(path, "/dev/cu.")
	if !validPrefix {
		return false
	}

	// Validate the path doesn't escape /dev/ via path traversal, symlinks, or other attacks
	return security.ValidatePathWithinDirectory(path, "/dev") == nil
}
