package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"go.bug.st/serial"
)

// SerialTestRequest represents the request body for testing serial port
type SerialTestRequest struct {
	PortPath        string `json:"port_path"`
	BaudRate        int    `json:"baud_rate"`
	DataBits        int    `json:"data_bits"`
	StopBits        int    `json:"stop_bits"`
	Parity          string `json:"parity"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	AutoCorrectBaud bool   `json:"auto_correct_baud"`
}

// SerialTestResponse represents the response from testing serial port
type SerialTestResponse struct {
	Success         bool                   `json:"success"`
	PortPath        string                 `json:"port_path"`
	BaudRate        int                    `json:"baud_rate"`
	TestDurationMS  int64                  `json:"test_duration_ms"`
	BytesReceived   int                    `json:"bytes_received,omitempty"`
	SampleData      string                 `json:"sample_data,omitempty"`
	RawResponses    []SerialCommandResult  `json:"raw_responses,omitempty"`
	Error           string                 `json:"error,omitempty"`
	Message         string                 `json:"message"`
	Suggestion      string                 `json:"suggestion,omitempty"`
}

// SerialCommandResult represents a single command/response pair
type SerialCommandResult struct {
	Command  string `json:"command"`
	Response string `json:"response"`
	IsJSON   bool   `json:"is_json"`
}

// SerialDeviceInfo represents information about a discovered serial device
type SerialDeviceInfo struct {
	PortPath     string `json:"port_path"`
	FriendlyName string `json:"friendly_name"`
	VendorID     string `json:"vendor_id,omitempty"`
	ProductID    string `json:"product_id,omitempty"`
	LastSeen     int64  `json:"last_seen"`
}

// handleSerialTest handles POST /api/serial/test
func (s *Server) handleSerialTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SerialTestRequest
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
		http.Error(w, "Invalid port path. Must start with /dev/tty or /dev/serial", http.StatusBadRequest)
		return
	}

	// Set defaults
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
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 5
	}

	// Perform the serial port test
	result := testSerialPort(req)

	w.Header().Set("Content-Type", "application/json")
	if result.Success {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusOK) // Still return 200 for test failure (not an API error)
	}
	json.NewEncoder(w).Encode(result)
}

// testSerialPort tests a serial port with the given configuration
func testSerialPort(req SerialTestRequest) SerialTestResponse {
	startTime := time.Now()

	// Build serial port mode
	mode := &serial.Mode{
		BaudRate: req.BaudRate,
		DataBits: req.DataBits,
		StopBits: serial.StopBits(req.StopBits),
	}

	// Set parity
	switch req.Parity {
	case "N":
		mode.Parity = serial.NoParity
	case "E":
		mode.Parity = serial.EvenParity
	case "O":
		mode.Parity = serial.OddParity
	default:
		return SerialTestResponse{
			Success:        false,
			PortPath:       req.PortPath,
			BaudRate:       req.BaudRate,
			TestDurationMS: time.Since(startTime).Milliseconds(),
			Error:          fmt.Sprintf("Invalid parity: %s", req.Parity),
			Message:        "Serial port test failed",
			Suggestion:     "Parity must be one of: N (None), E (Even), O (Odd)",
		}
	}

	// Try to open the serial port
	port, err := serial.Open(req.PortPath, mode)
	if err != nil {
		suggestion := getSuggestionForError(err)
		return SerialTestResponse{
			Success:        false,
			PortPath:       req.PortPath,
			BaudRate:       req.BaudRate,
			TestDurationMS: time.Since(startTime).Milliseconds(),
			Error:          fmt.Sprintf("Failed to open port: %v", err),
			Message:        "Serial port test failed",
			Suggestion:     suggestion,
		}
	}
	defer port.Close()

	// Set read timeout
	if err := port.SetReadTimeout(time.Duration(req.TimeoutSeconds) * time.Second); err != nil {
		log.Printf("Warning: Failed to set read timeout: %v", err)
	}

	var rawResponses []SerialCommandResult
	var totalBytesRead int

	// Send test commands
	testCommands := []string{"??", "I?"} // Query firmware info and baud rate

	for _, cmd := range testCommands {
		// Send command
		_, err := port.Write([]byte(cmd + "\r"))
		if err != nil {
			log.Printf("Warning: Failed to write command %s: %v", cmd, err)
			continue
		}

		// Read response
		buf := make([]byte, 512)
		n, err := port.Read(buf)
		if err != nil {
			log.Printf("Warning: Failed to read response for %s: %v", cmd, err)
			continue
		}

		if n > 0 {
			totalBytesRead += n
			response := strings.TrimSpace(string(buf[:n]))

			// Check if response is JSON
			isJSON := json.Valid([]byte(response))

			rawResponses = append(rawResponses, SerialCommandResult{
				Command:  cmd,
				Response: response,
				IsJSON:   isJSON,
			})
		}
	}

	testDuration := time.Since(startTime).Milliseconds()

	// If no data received, report failure
	if totalBytesRead == 0 {
		return SerialTestResponse{
			Success:        false,
			PortPath:       req.PortPath,
			BaudRate:       req.BaudRate,
			TestDurationMS: testDuration,
			BytesReceived:  0,
			Error:          "No response from device",
			Message:        "Serial port test failed",
			Suggestion:     "Device may be at wrong baud rate. Try 9600, 115200, or other common rates. Ensure device is powered on.",
		}
	}

	// Auto-correct baud rate if requested
	detectedBaudRate := req.BaudRate
	if req.AutoCorrectBaud {
		// Look for baud rate response in I? command
		for _, resp := range rawResponses {
			if resp.Command == "I?" && !resp.IsJSON {
				// Try to parse the baud rate
				baudStr := strings.TrimSpace(resp.Response)
				var reportedBaud int
				_, err := fmt.Sscanf(baudStr, "%d", &reportedBaud)
				if err == nil && reportedBaud != req.BaudRate {
					detectedBaudRate = reportedBaud
					log.Printf("Auto-detected baud rate: %d (requested: %d)", detectedBaudRate, req.BaudRate)
				}
			}
		}
	}

	// Prepare sample data from first response
	sampleData := ""
	if len(rawResponses) > 0 {
		sampleData = rawResponses[0].Response
		if len(sampleData) > 100 {
			sampleData = sampleData[:100] + "..."
		}
	}

	message := "Serial port communication successful"
	if detectedBaudRate != req.BaudRate {
		message = fmt.Sprintf("Device reports different baud rate (%d). Configuration updated automatically.", detectedBaudRate)
	}

	return SerialTestResponse{
		Success:        true,
		PortPath:       req.PortPath,
		BaudRate:       detectedBaudRate,
		TestDurationMS: testDuration,
		BytesReceived:  totalBytesRead,
		SampleData:     sampleData,
		RawResponses:   rawResponses,
		Message:        message,
	}
}

// getSuggestionForError provides helpful suggestions based on error type
func getSuggestionForError(err error) string {
	errStr := err.Error()

	if strings.Contains(errStr, "no such file") || strings.Contains(errStr, "not found") {
		return "Check that the device is connected and appears in /dev/"
	}

	if strings.Contains(errStr, "permission denied") {
		return "Run: sudo usermod -a -G dialout $USER && sudo reboot"
	}

	if strings.Contains(errStr, "resource busy") || strings.Contains(errStr, "device busy") {
		return "Another process may be using the port. Stop other applications using this serial port."
	}

	return "Check device connection and permissions"
}

// handleSerialDevices handles GET /api/serial/devices - List available serial devices
func (s *Server) handleSerialDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get all existing configs to filter them out
	existingConfigs, err := s.db.GetSerialConfigs()
	if err != nil {
		log.Printf("Error fetching existing configs: %v", err)
		http.Error(w, "Failed to fetch existing configurations", http.StatusInternalServerError)
		return
	}

	// Build a set of already-configured port paths
	configuredPorts := make(map[string]bool)
	for _, config := range existingConfigs {
		configuredPorts[config.PortPath] = true
	}

	// Enumerate available serial ports
	ports, err := serial.GetPortsList()
	if err != nil {
		log.Printf("Error enumerating serial ports: %v", err)
		http.Error(w, "Failed to enumerate serial ports", http.StatusInternalServerError)
		return
	}

	// Filter out already-configured ports and build response
	var devices []SerialDeviceInfo
	now := time.Now().Unix()

	for _, portPath := range ports {
		// Skip if already configured
		if configuredPorts[portPath] {
			continue
		}

		// Create device info
		// TODO: Add USB metadata (vendor/product IDs) via udev/sysfs if needed
		friendlyName := getFriendlyName(portPath)

		devices = append(devices, SerialDeviceInfo{
			PortPath:     portPath,
			FriendlyName: friendlyName,
			LastSeen:     now,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

// getFriendlyName generates a user-friendly name for a serial port
func getFriendlyName(portPath string) string {
	// Extract the device name from the path
	parts := strings.Split(portPath, "/")
	if len(parts) > 0 {
		deviceName := parts[len(parts)-1]

		// Provide friendly names for common device types
		switch {
		case strings.HasPrefix(deviceName, "ttyUSB"):
			return fmt.Sprintf("USB Serial Adapter (%s)", deviceName)
		case strings.HasPrefix(deviceName, "ttyACM"):
			return fmt.Sprintf("USB CDC Device (%s)", deviceName)
		case strings.HasPrefix(deviceName, "ttySC"):
			return fmt.Sprintf("SC16IS762 HAT (%s)", deviceName)
		case strings.HasPrefix(deviceName, "ttyAMA"):
			return fmt.Sprintf("Raspberry Pi Serial (%s)", deviceName)
		default:
			return deviceName
		}
	}

	return portPath
}
