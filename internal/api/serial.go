package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/serialmux"
	"go.bug.st/serial"
)

// supplementalSerialDevicePatterns is the set of device-name regexes we scan
// /dev with as a fallback to go.bug.st/serial.GetPortsList(). The primary
// enumeration silently drops several device classes on certain Pi/HAT
// combinations (notably ttySC* from the SC16IS762 HAT), so we cast a wider
// net here. Duplicates are deduped in buildSerialDeviceList — adding a
// pattern that the primary path already returns does not produce a doubled
// entry.
var supplementalSerialDevicePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^ttySC[0-9]{1,3}$`),  // SC16IS762 HAT
	regexp.MustCompile(`^ttyS[0-9]{1,3}$`),   // legacy 16550 UART
	regexp.MustCompile(`^ttyAMA[0-9]{1,3}$`), // Pi GPIO UART
	regexp.MustCompile(`^ttyACM[0-9]{1,3}$`), // USB CDC (Arduino-style)
	regexp.MustCompile(`^ttyUSB[0-9]{1,3}$`), // USB-to-serial (FTDI/CH340/etc.)
}

// supplementalSerialSymlinkDirs are directories whose entries we treat as
// serial port symlinks regardless of name. /dev/serial holds serial0/serial1
// (the Pi's primary UART aliases); by-id and by-path are udev-managed.
var supplementalSerialSymlinkDirs = []string{
	"/dev/serial",
	"/dev/serial/by-id",
	"/dev/serial/by-path",
}

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
	Success        bool                  `json:"success"`
	PortPath       string                `json:"port_path"`
	BaudRate       int                   `json:"baud_rate"`
	TestDurationMS int64                 `json:"test_duration_ms"`
	BytesReceived  int                   `json:"bytes_received,omitempty"`
	SampleData     string                `json:"sample_data,omitempty"`
	RawResponses   []SerialCommandResult `json:"raw_responses,omitempty"`
	Error          string                `json:"error,omitempty"`
	Message        string                `json:"message"`
	Suggestion     string                `json:"suggestion,omitempty"`
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

// SerialDevicesDiagnostic exposes the raw enumeration breakdown — primary
// scan vs supplemental scan vs configured ports vs raw /dev/ contents —
// so an operator (or the serial-harness CLI) can tell exactly why a given
// port did or did not appear in the merged list. Populated only when the
// request URL includes ?diagnostic=true.
type SerialDevicesDiagnostic struct {
	EnumerationSource  string   `json:"enumeration_source"`
	EnumeratedPorts    []string `json:"enumerated_ports"`
	EnumerationError   string   `json:"enumeration_error,omitempty"`
	SupplementalPorts  []string `json:"supplemental_ports"`
	SupplementalErrors []string `json:"supplemental_errors,omitempty"`
	ConfiguredPorts    []string `json:"configured_ports"`
	DevDirListing      []string `json:"dev_dir_listing"`
}

// SerialDevicesResponse is the envelope returned by /api/serial/devices when
// ?diagnostic=true is set. The default response (no query param) is the
// plain []SerialDeviceInfo array so the existing UI contract is preserved.
type SerialDevicesResponse struct {
	Devices    []SerialDeviceInfo       `json:"devices"`
	Diagnostic *SerialDevicesDiagnostic `json:"diagnostic,omitempty"`
}

func applySerialTestDefaults(req *SerialTestRequest) {
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
}

func normaliseSerialTestOptions(req SerialTestRequest) (serialmux.PortOptions, error) {
	opts := serialmux.PortOptions{
		BaudRate: req.BaudRate,
		DataBits: req.DataBits,
		StopBits: req.StopBits,
		Parity:   req.Parity,
	}

	return opts.Normalise()
}

func serialOptionsEqual(a, b serialmux.PortOptions) bool {
	return a.BaudRate == b.BaudRate &&
		a.DataBits == b.DataBits &&
		a.StopBits == b.StopBits &&
		a.Parity == b.Parity
}

func serialTestCommands(autoCorrectBaud bool) []string {
	commands := []string{"??"}
	if autoCorrectBaud {
		commands = append(commands, "I?")
	}

	return commands
}

func (s *Server) activeSerialTestResult(req SerialTestRequest) (SerialTestResponse, bool) {
	if s.serialManager == nil {
		return SerialTestResponse{}, false
	}

	snap := s.serialManager.Snapshot()
	if snap.PortPath == "" || snap.PortPath != req.PortPath {
		return SerialTestResponse{}, false
	}

	normalisedReq, err := normaliseSerialTestOptions(req)
	if err != nil {
		return SerialTestResponse{
			Success:        false,
			PortPath:       req.PortPath,
			BaudRate:       req.BaudRate,
			TestDurationMS: 0,
			Error:          fmt.Sprintf("Invalid serial configuration: %v", err),
			Message:        "Serial port test failed",
			Suggestion:     "Check baud rate, data bits, stop bits, and parity settings",
		}, true
	}

	normalisedActive, err := snap.Options.Normalise()
	if err != nil {
		normalisedActive = snap.Options
	}

	if !serialOptionsEqual(normalisedReq, normalisedActive) {
		return SerialTestResponse{
			Success:        false,
			PortPath:       req.PortPath,
			BaudRate:       normalisedReq.BaudRate,
			TestDurationMS: 0,
			Error:          "Port is currently in use by the active serial connection",
			Message:        "Serial port test failed",
			Suggestion: fmt.Sprintf(
				"The live radar connection is already using this port at %d baud, %d data bits, %d stop bit(s), parity %s. Save/apply the new settings or stop the active connection before testing this port separately.",
				normalisedActive.BaudRate,
				normalisedActive.DataBits,
				normalisedActive.StopBits,
				normalisedActive.Parity,
			),
		}, true
	}

	if s.serialManager.CurrentMux() == nil {
		return SerialTestResponse{
			Success:        false,
			PortPath:       req.PortPath,
			BaudRate:       normalisedReq.BaudRate,
			TestDurationMS: 0,
			Error:          "Port is configured as active but no live serial connection is available",
			Message:        "Serial port test failed",
			Suggestion:     "Check service status and serial connection logs before retrying.",
		}, true
	}

	return SerialTestResponse{
		Success:        true,
		PortPath:       req.PortPath,
		BaudRate:       normalisedReq.BaudRate,
		TestDurationMS: 0,
		Message:        "Serial port is already active and owned by the live radar connection",
		Suggestion:     "The service is already using this port with the requested settings, so no separate test open was attempted.",
	}, true
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

	applySerialTestDefaults(&req)

	if result, handled := s.activeSerialTestResult(req); handled {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(result); err != nil {
			log.Printf("Error encoding serial test response for active port %q: %v", req.PortPath, err)
		}
		return
	}

	// Perform the serial port test
	result := testSerialPort(req)

	w.Header().Set("Content-Type", "application/json")
	// Always return 200 OK, even for test failure (not an API error)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("Error encoding serial test response for port %q: %v", req.PortPath, err)
	}
}

// testSerialPort tests a serial port with the given configuration.
// It uses serialmux.PortOptions for validation and mode construction
// to stay consistent with the production serial connection path.
func testSerialPort(req SerialTestRequest) SerialTestResponse {
	startTime := time.Now()

	// Build and validate options using the shared PortOptions type
	normalised, err := normaliseSerialTestOptions(req)
	if err != nil {
		return SerialTestResponse{
			Success:        false,
			PortPath:       req.PortPath,
			BaudRate:       req.BaudRate,
			TestDurationMS: time.Since(startTime).Milliseconds(),
			Error:          fmt.Sprintf("Invalid serial configuration: %v", err),
			Message:        "Serial port test failed",
			Suggestion:     "Check baud rate, data bits, stop bits, and parity settings",
		}
	}

	mode, err := normalised.SerialMode()
	if err != nil {
		return SerialTestResponse{
			Success:        false,
			PortPath:       req.PortPath,
			BaudRate:       normalised.BaudRate,
			TestDurationMS: time.Since(startTime).Milliseconds(),
			Error:          fmt.Sprintf("Failed to build serial mode: %v", err),
			Message:        "Serial port test failed",
		}
	}

	// Try to open the serial port
	port, err := serial.Open(req.PortPath, mode)
	if err != nil {
		suggestion := getSuggestionForError(err)
		return SerialTestResponse{
			Success:        false,
			PortPath:       req.PortPath,
			BaudRate:       normalised.BaudRate,
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

	// Send read-only test commands. "??" queries firmware info; "I?" is only
	// needed when the caller explicitly wants baud auto-detection.
	testCommands := serialTestCommands(req.AutoCorrectBaud)

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

// handleSerialDevices handles GET /api/serial/devices - List available serial devices.
//
// Pass ?diagnostic=true to receive a SerialDevicesResponse envelope that
// includes the raw enumeration breakdown alongside the merged device list.
// The plain []SerialDeviceInfo response is preserved when the query param
// is absent so the existing frontend keeps working without modification.
func (s *Server) handleSerialDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	diagnostic := r.URL.Query().Get("diagnostic") == "true"

	// Get all existing configs to filter them out
	existingConfigs, err := s.db.GetSerialConfigs()
	if err != nil {
		log.Printf("Error fetching existing configs: %v", err)
		http.Error(w, "Failed to fetch existing configurations", http.StatusInternalServerError)
		return
	}

	// Build a set of already-configured port paths
	configuredPorts := make(map[string]bool)
	configuredPortsList := make([]string, 0, len(existingConfigs))
	for _, config := range existingConfigs {
		configuredPorts[config.PortPath] = true
		configuredPortsList = append(configuredPortsList, config.PortPath)
	}
	sort.Strings(configuredPortsList)

	// Enumerate available serial ports
	var enumerationErrMsg string
	ports, err := serial.GetPortsList()
	if err != nil {
		if !diagnostic {
			log.Printf("Error enumerating serial ports: %v", err)
			http.Error(w, "Failed to enumerate serial ports", http.StatusInternalServerError)
			return
		}
		// In diagnostic mode, surface the error in the envelope rather
		// than failing the whole request — operators care about the
		// supplemental scan and configured-ports lists even when the
		// primary enumeration fails.
		log.Printf("serial: primary enumeration failed (returning empty list to diagnostic caller): %v", err)
		enumerationErrMsg = err.Error()
		ports = nil
	}

	scan := scanSupplementalSerialPorts(os.ReadDir)
	devices := buildSerialDeviceList(configuredPorts, ports, scan.Ports, time.Now().Unix())

	w.Header().Set("Content-Type", "application/json")

	if diagnostic {
		// Force every []string to render as [] rather than null so the
		// Python harness (and any other consumer) can iterate without
		// nil-handling. Go's json package distinguishes nil slices from
		// empty ones — we don't want that surfaced over the wire.
		nonNil := func(s []string) []string {
			if s == nil {
				return []string{}
			}
			return s
		}
		resp := SerialDevicesResponse{
			Devices: devices,
			Diagnostic: &SerialDevicesDiagnostic{
				EnumerationSource:  "go.bug.st/serial",
				EnumeratedPorts:    nonNil(ports),
				EnumerationError:   enumerationErrMsg,
				SupplementalPorts:  nonNil(scan.Ports),
				SupplementalErrors: nonNil(scan.ScanError),
				ConfiguredPorts:    nonNil(configuredPortsList),
				DevDirListing:      nonNil(scan.DevTTY),
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("Error encoding serial devices response: %v", err)
		}
		return
	}

	if err := json.NewEncoder(w).Encode(devices); err != nil {
		log.Printf("Error encoding serial devices response: %v", err)
	}
}

func buildSerialDeviceList(
	configuredPorts map[string]bool,
	enumeratedPorts []string,
	supplementalPorts []string,
	now int64,
) []SerialDeviceInfo {
	seen := make(map[string]bool)
	devices := make([]SerialDeviceInfo, 0, len(enumeratedPorts)+len(supplementalPorts))
	addPort := func(portPath string) {
		if portPath == "" || configuredPorts[portPath] || seen[portPath] {
			return
		}
		seen[portPath] = true
		devices = append(devices, SerialDeviceInfo{
			PortPath:     portPath,
			FriendlyName: getFriendlyName(portPath),
			LastSeen:     now,
		})
	}

	for _, portPath := range enumeratedPorts {
		addPort(portPath)
	}

	for _, portPath := range supplementalPorts {
		addPort(portPath)
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].PortPath < devices[j].PortPath
	})

	return devices
}

// supplementalSerialScan captures everything the supplemental scan saw,
// so the /api/serial/devices diagnostic mode can show operators exactly
// which ports came from where, what was in /dev/ raw, and which directories
// failed to read. Only the Ports field is needed for the normal hot path.
type supplementalSerialScan struct {
	Ports     []string // discovered ports (deduped, sorted)
	DevTTY    []string // raw /dev entries matching tty*/serial* — diagnostic only
	ScanError []string // human-readable errors per directory, excluding ENOENT
}

// scanSupplementalSerialPorts walks /dev and the configured symlink
// directories and reports every port it found, plus diagnostic context.
// ENOENT on the symlink directories is expected on systems without those
// trees (e.g. macOS dev hosts) and is intentionally not flagged as an error.
func scanSupplementalSerialPorts(readDir func(string) ([]os.DirEntry, error)) supplementalSerialScan {
	var result supplementalSerialScan
	if readDir == nil {
		return result
	}

	seen := make(map[string]bool)
	addPort := func(portPath string) {
		if portPath == "" || seen[portPath] {
			return
		}
		seen[portPath] = true
		result.Ports = append(result.Ports, portPath)
	}

	if entries, err := readDir("/dev"); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			// Capture every tty*/serial* name for diagnostic visibility,
			// independent of whether any pattern matches. This is what
			// lets the harness say "you have ttyXYZ0 in /dev/ but the
			// scan ignored it — consider broadening the regex."
			if strings.HasPrefix(name, "tty") || strings.HasPrefix(name, "serial") {
				result.DevTTY = append(result.DevTTY, name)
			}
			if entry.IsDir() {
				continue
			}
			for _, pat := range supplementalSerialDevicePatterns {
				if pat.MatchString(name) {
					addPort(filepath.Join("/dev", name))
					break
				}
			}
		}
	} else {
		log.Printf("serial: /dev scan failed: %v", err)
		result.ScanError = append(result.ScanError, fmt.Sprintf("/dev: %v", err))
	}

	for _, dir := range supplementalSerialSymlinkDirs {
		entries, err := readDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Printf("serial: %s scan failed: %v", dir, err)
				result.ScanError = append(result.ScanError, fmt.Sprintf("%s: %v", dir, err))
			}
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			addPort(filepath.Join(dir, entry.Name()))
		}
	}

	sort.Strings(result.Ports)
	sort.Strings(result.DevTTY)
	return result
}

// listSupplementalSerialPorts is a thin wrapper kept for existing callers
// and tests that only need the port list. New callers wanting diagnostic
// data should use scanSupplementalSerialPorts directly.
func listSupplementalSerialPorts(readDir func(string) ([]os.DirEntry, error)) []string {
	return scanSupplementalSerialPorts(readDir).Ports
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
