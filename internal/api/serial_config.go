package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/db"
)

// SerialConfigRequest represents the request body for creating/updating serial configs
type SerialConfigRequest struct {
	Name        string `json:"name"`
	PortPath    string `json:"port_path"`
	BaudRate    int    `json:"baud_rate"`
	DataBits    int    `json:"data_bits"`
	StopBits    int    `json:"stop_bits"`
	Parity      string `json:"parity"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(configs)
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
func (s *Server) handleGetSerialConfig(w http.ResponseWriter, r *http.Request, id int) {
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
	json.NewEncoder(w).Encode(config)
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
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if req.PortPath == "" {
		http.Error(w, "Port path is required", http.StatusBadRequest)
		return
	}

	// Validate port path format
	if !isValidPortPath(req.PortPath) {
		http.Error(w, "Invalid port path. Must start with /dev/tty or /dev/serial", http.StatusBadRequest)
		return
	}

	// Validate sensor model
	if _, ok := GetSensorModel(req.SensorModel); !ok {
		http.Error(w, fmt.Sprintf("Unsupported sensor model: %s", req.SensorModel), http.StatusBadRequest)
		return
	}

	// Set defaults if not provided
	if req.BaudRate == 0 {
		req.BaudRate = 19200
	}
	if req.DataBits == 0 {
		req.DataBits = 8
	}
	if req.StopBits == 0 {
		req.StopBits = 1
	}
	if req.Parity == "" {
		req.Parity = "N"
	}

	config := &db.SerialConfig{
		Name:        req.Name,
		PortPath:    req.PortPath,
		BaudRate:    req.BaudRate,
		DataBits:    req.DataBits,
		StopBits:    req.StopBits,
		Parity:      req.Parity,
		Enabled:     req.Enabled,
		Description: req.Description,
		SensorModel: req.SensorModel,
	}

	id, err := s.db.CreateSerialConfig(config)
	if err != nil {
		log.Printf("Error creating serial config: %v", err)
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "Configuration with this name already exists", http.StatusConflict)
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
	json.NewEncoder(w).Encode(created)
}

// handleUpdateSerialConfig handles PUT /api/serial/configs/:id
func (s *Server) handleUpdateSerialConfig(w http.ResponseWriter, r *http.Request, id int) {
	var req SerialConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if req.PortPath == "" {
		http.Error(w, "Port path is required", http.StatusBadRequest)
		return
	}

	// Validate port path format
	if !isValidPortPath(req.PortPath) {
		http.Error(w, "Invalid port path. Must start with /dev/tty or /dev/serial", http.StatusBadRequest)
		return
	}

	// Validate sensor model
	if _, ok := GetSensorModel(req.SensorModel); !ok {
		http.Error(w, fmt.Sprintf("Unsupported sensor model: %s", req.SensorModel), http.StatusBadRequest)
		return
	}

	config := &db.SerialConfig{
		ID:          id,
		Name:        req.Name,
		PortPath:    req.PortPath,
		BaudRate:    req.BaudRate,
		DataBits:    req.DataBits,
		StopBits:    req.StopBits,
		Parity:      req.Parity,
		Enabled:     req.Enabled,
		Description: req.Description,
		SensorModel: req.SensorModel,
	}

	err := s.db.UpdateSerialConfig(config)
	if err != nil {
		log.Printf("Error updating serial config %d: %v", id, err)
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Configuration not found", http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "Configuration with this name already exists", http.StatusConflict)
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
	json.NewEncoder(w).Encode(updated)
}

// handleDeleteSerialConfig handles DELETE /api/serial/configs/:id
func (s *Server) handleDeleteSerialConfig(w http.ResponseWriter, r *http.Request, id int) {
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
	json.NewEncoder(w).Encode(models)
}

// isValidPortPath validates that a port path is in an allowed format
func isValidPortPath(path string) bool {
	return strings.HasPrefix(path, "/dev/tty") || strings.HasPrefix(path, "/dev/serial")
}
