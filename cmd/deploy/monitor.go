package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Health check configuration constants
const (
	// maxAcceptableLogErrors is the threshold for errors in recent logs before marking unhealthy
	maxAcceptableLogErrors = 5
	// apiHealthCheckTimeout is the timeout for API health check requests
	apiHealthCheckTimeout = 5 * time.Second
)

// Monitor handles status checking and health monitoring
type Monitor struct {
	Target        string
	SSHUser       string
	SSHKey        string
	IdentityAgent string
	APIPort       int
}

// HealthStatus represents the health check result
type HealthStatus struct {
	Healthy bool
	Message string
	Details string
}

// SystemStatus represents comprehensive system status
type SystemStatus struct {
	Hostname      string
	Uptime        string
	LoadAverage   string
	MemoryUsage   string
	MemoryPercent float64
	DiskUsage     string
	DiskPercent   float64
	ServiceStatus string
	ServiceActive bool
	ServiceUptime string
	APIPort       int
	APIResponding bool
	ProcessCount  int
	CPUCount      int
	DatabaseSize  string
	DatabaseOK    bool
	LogErrors     int
	LogOK         bool
}

// Spinner provides a simple loading animation
type Spinner struct {
	frames  []string
	current int
	message string
}

func NewSpinner(message string) *Spinner {
	return &Spinner{
		frames:  []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		current: 0,
		message: message,
	}
}

func (s *Spinner) Next() string {
	frame := s.frames[s.current]
	s.current = (s.current + 1) % len(s.frames)
	return fmt.Sprintf("\r%s %s", frame, s.message)
}

// GetStatus returns comprehensive system and service status
func (m *Monitor) GetStatus(ctx context.Context) (*SystemStatus, error) {
	exec := NewExecutor(m.Target, m.SSHUser, m.SSHKey, m.IdentityAgent, false)
	status := &SystemStatus{
		APIPort: m.APIPort,
	}

	// Channel for results
	type result struct {
		name string
		data string
		err  error
	}
	results := make(chan result, 12)

	// Run commands concurrently with timeout
	commands := map[string]string{
		"hostname":       "hostname",
		"uptime":         "uptime -p",
		"load":           "uptime | awk -F'load average:' '{print $2}'",
		"memory":         "free -h | grep Mem",
		"disk":           "df -h / | tail -1",
		"service_status": "systemctl is-active velocity-report.service 2>/dev/null || echo 'inactive'",
		"service_uptime": "systemctl show velocity-report.service --property=ActiveEnterTimestamp --value 2>/dev/null || echo 'N/A'",
		"process_count":  "ps aux | grep velocity-report | grep -v grep | wc -l",
		"cpu_count":      "nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo '1'",
		"database":       "test -f /var/lib/velocity-report/sensor_data.db && du -h /var/lib/velocity-report/sensor_data.db | cut -f1 || echo 'missing'",
		"log_errors":     "journalctl -u velocity-report.service -n 20 --no-pager 2>/dev/null | grep -i error | wc -l || echo '0'",
	}

	// Launch all commands
	for name, cmd := range commands {
		go func(n, c string) {
			cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			// Create a channel for the command result
			done := make(chan result, 1)
			go func() {
				output, err := exec.Run(c)
				done <- result{name: n, data: strings.TrimSpace(output), err: err}
			}()

			select {
			case res := <-done:
				results <- res
			case <-cmdCtx.Done():
				results <- result{name: n, data: "", err: fmt.Errorf("timeout")}
			}
		}(name, cmd)
	}

	// Collect results
	resultMap := make(map[string]string)
	for i := 0; i < len(commands); i++ {
		res := <-results
		if res.err == nil {
			resultMap[res.name] = res.data
		}
	}

	// Parse results
	status.Hostname = resultMap["hostname"]
	status.Uptime = resultMap["uptime"]
	status.LoadAverage = resultMap["load"]
	status.ServiceStatus = resultMap["service_status"]
	status.ServiceActive = strings.TrimSpace(resultMap["service_status"]) == "active"
	status.ServiceUptime = resultMap["service_uptime"]

	// Parse process count
	if pc, err := strconv.Atoi(resultMap["process_count"]); err == nil {
		status.ProcessCount = pc
	}

	// Parse CPU count
	if cc, err := strconv.Atoi(resultMap["cpu_count"]); err == nil {
		status.CPUCount = cc
	}

	// Parse database
	if dbSize := resultMap["database"]; dbSize != "" && dbSize != "missing" {
		status.DatabaseSize = dbSize
		status.DatabaseOK = true
	}

	// Parse log errors
	if logErrors := resultMap["log_errors"]; logErrors != "" {
		if count, err := strconv.Atoi(logErrors); err == nil {
			status.LogErrors = count
			status.LogOK = count <= 5
		}
	}

	// Parse memory (format: Mem: total used free shared buff/cache available)
	if memLine := resultMap["memory"]; memLine != "" {
		parts := strings.Fields(memLine)
		if len(parts) >= 3 {
			// parts[0] = "Mem:", parts[1] = total, parts[2] = used
			total := parts[1]
			used := parts[2]
			status.MemoryUsage = fmt.Sprintf("%s / %s", used, total)
			// Calculate percentage
			if usedBytes, err := parseSize(used); err == nil {
				if totalBytes, err := parseSize(total); err == nil && totalBytes > 0 {
					status.MemoryPercent = float64(usedBytes) / float64(totalBytes) * 100
				}
			}
		}
	}

	// Parse disk
	if diskLine := resultMap["disk"]; diskLine != "" {
		parts := strings.Fields(diskLine)
		if len(parts) >= 6 {
			status.DiskUsage = fmt.Sprintf("%s / %s", parts[2], parts[1])
			// Parse percentage (e.g., "45%")
			if pct := strings.TrimSuffix(parts[4], "%"); pct != "" {
				if val, err := strconv.ParseFloat(pct, 64); err == nil {
					status.DiskPercent = val
				}
			}
		}
	}

	// Check API if service is active
	if status.ServiceActive && m.APIPort > 0 {
		apiURL := fmt.Sprintf("http://%s:%d/health", m.Target, m.APIPort)
		client := &http.Client{Timeout: 2 * time.Second}
		if resp, err := client.Get(apiURL); err == nil {
			status.APIResponding = resp.StatusCode == 200
			resp.Body.Close()
		}
	}

	return status, nil
}

// parseSize converts human-readable sizes (e.g., "1.5G", "512M", "4.0Gi", "586Mi") to bytes
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0, fmt.Errorf("empty size")
	}

	// Handle both decimal (K, M, G, T) and binary (Ki, Mi, Gi, Ti) units
	multipliers := map[string]int64{
		"Ki": 1024,
		"Mi": 1024 * 1024,
		"Gi": 1024 * 1024 * 1024,
		"Ti": 1024 * 1024 * 1024 * 1024,
		"K":  1024,
		"M":  1024 * 1024,
		"G":  1024 * 1024 * 1024,
		"T":  1024 * 1024 * 1024 * 1024,
	}

	// Try binary units first (Ki, Mi, Gi, Ti)
	for suffix, mult := range multipliers {
		if strings.HasSuffix(s, suffix) {
			numStr := s[:len(s)-len(suffix)]
			if val, err := strconv.ParseFloat(numStr, 64); err == nil {
				return int64(val * float64(mult)), nil
			}
		}
	}

	// Try parsing as plain number
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return val, nil
	}

	return 0, fmt.Errorf("invalid size format: %s", s)
}

// FormatStatus returns a nicely formatted status display
func (s *SystemStatus) FormatStatus() string {
	var b strings.Builder

	// System Info - compact line format (no header)
	if s.Uptime != "" {
		b.WriteString(fmt.Sprintf("⏱  Uptime       %s\n", s.Uptime))
	}
	if s.LoadAverage != "" {
		b.WriteString(fmt.Sprintf("📈 Load         %s\n", s.LoadAverage))
	}
	if s.CPUCount > 0 {
		b.WriteString(fmt.Sprintf("🔧 CPU          %d cores\n", s.CPUCount))
	}

	// Memory - compact with inline bar (aligned)
	if s.MemoryUsage != "" {
		statusIcon := "✅"
		if s.MemoryPercent > 90 {
			statusIcon = "❌"
		} else if s.MemoryPercent > 75 {
			statusIcon = "⚠️ "
		}
		bar := progressBar(s.MemoryPercent, 20)
		b.WriteString(fmt.Sprintf("💾 Memory       %s %-17s (%5.1f%%) %s\n",
			statusIcon, s.MemoryUsage, s.MemoryPercent, bar))
	}

	// Disk - compact with inline bar (aligned)
	if s.DiskUsage != "" {
		statusIcon := "✅"
		if s.DiskPercent > 90 {
			statusIcon = "❌"
		} else if s.DiskPercent > 75 {
			statusIcon = "⚠️ "
		}
		bar := progressBar(s.DiskPercent, 20)
		b.WriteString(fmt.Sprintf("💿 Disk         %s %-17s (%5.1f%%) %s\n",
			statusIcon, s.DiskUsage, s.DiskPercent, bar))
	}

	// Database
	if s.DatabaseOK {
		b.WriteString(fmt.Sprintf("🛢 Database      ✅ %s\n", s.DatabaseSize))
	} else {
		b.WriteString("🛢 Database      ❌ missing\n")
	}

	// Logs
	if s.LogOK {
		b.WriteString(fmt.Sprintf("🪵 Logs          ✅ %d errors\n", s.LogErrors))
	} else {
		b.WriteString(fmt.Sprintf("🪵 Logs          ⚠️  %d errors\n", s.LogErrors))
	}

	// Service Status - compact
	serviceIcon := "❌"
	serviceText := "inactive"
	if s.ServiceActive {
		serviceIcon = "✅"
		serviceText = "active"
	}
	b.WriteString(fmt.Sprintf("🚀 Service      %s %s", serviceIcon, serviceText))
	if s.ServiceActive && s.ServiceUptime != "" && s.ServiceUptime != "N/A" {
		b.WriteString(fmt.Sprintf(" (since %s)", truncate(s.ServiceUptime, 30)))
	}
	b.WriteString("\n")

	if s.ProcessCount > 0 {
		b.WriteString(fmt.Sprintf("🔄 Processes    %d\n", s.ProcessCount))
	}

	// API Status - compact
	if s.APIPort > 0 {
		apiIcon := "❌"
		apiText := "not responding"
		if s.APIResponding {
			apiIcon = "✅"
			apiText = "responding"
		}
		b.WriteString(fmt.Sprintf("🌐 API :%d    %s %s\n", s.APIPort, apiIcon, apiText))
	}

	b.WriteString("\n")
	return b.String()
}

// progressBar creates a visual progress bar
func progressBar(percent float64, width int) string {
	filled := int(float64(width) * percent / 100.0)
	if filled > width {
		filled = width
	}
	empty := width - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return bar
}

// truncate truncates a string to a maximum length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// CheckHealth performs a comprehensive health check
func (m *Monitor) CheckHealth() (*HealthStatus, error) {
	exec := NewExecutor(m.Target, m.SSHUser, m.SSHKey, m.IdentityAgent, false)

	health := &HealthStatus{
		Healthy: true,
		Details: "",
	}

	var checks []string

	// Check 1: Service is running
	output, err := exec.RunSudo("systemctl is-active velocity-report.service")
	if err != nil || strings.TrimSpace(output) != "active" {
		health.Healthy = false
		health.Message = "Service is not running"
		checks = append(checks, "❌ Service: NOT RUNNING")
	} else {
		checks = append(checks, "✅ Service: RUNNING")
	}

	// Check 2: Service has been up for some time (not crash-looping)
	uptimeOutput, err := exec.RunSudo("systemctl show velocity-report.service --property=ActiveEnterTimestamp --value")
	if err == nil {
		checks = append(checks, fmt.Sprintf("✅ Started: %s", strings.TrimSpace(uptimeOutput)))
	}

	// Check 3: Check for recent errors in logs
	logsOutput, err := exec.RunSudo("journalctl -u velocity-report.service -n 20 --no-pager")
	if err != nil {
		// Log retrieval failed - mark as degraded rather than giving false positive
		checks = append(checks, "⚠ Logs: COULD NOT CHECK (log retrieval failed)")
	} else {
		errorCount := strings.Count(strings.ToLower(logsOutput), "error")
		if errorCount > maxAcceptableLogErrors {
			health.Healthy = false
			health.Message = fmt.Sprintf("Too many errors in logs (%d)", errorCount)
			checks = append(checks, fmt.Sprintf("❌ Logs: %d errors found", errorCount))
		} else {
			checks = append(checks, fmt.Sprintf("✅ Logs: %d errors (acceptable)", errorCount))
		}
	}

	// Check 4: API endpoint is responding
	apiHost := m.Target
	if apiHost == "localhost" || apiHost == "" {
		apiHost = "localhost"
	} else {
		// Extract hostname from user@host format
		parts := strings.Split(apiHost, "@")
		if len(parts) > 1 {
			apiHost = parts[1]
		}
	}

	apiPort := m.APIPort
	if apiPort == 0 {
		apiPort = 8080
	}

	apiURL := fmt.Sprintf("http://%s:%d/api/config", apiHost, apiPort)
	client := &http.Client{Timeout: apiHealthCheckTimeout}

	resp, err := client.Get(apiURL)
	if err != nil {
		health.Healthy = false
		if health.Message == "" {
			health.Message = "API endpoint not responding"
		}
		checks = append(checks, "❌ API: NOT RESPONDING")
	} else {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			checks = append(checks, "✅ API: RESPONDING")

			// Try to parse config response
			var config map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&config); err == nil {
				if units, ok := config["units"].(string); ok {
					checks = append(checks, fmt.Sprintf("  Units: %s", units))
				}
				if tz, ok := config["timezone"].(string); ok {
					checks = append(checks, fmt.Sprintf("  Timezone: %s", tz))
				}
			}
		} else {
			health.Healthy = false
			if health.Message == "" {
				health.Message = fmt.Sprintf("API returned status %d", resp.StatusCode)
			}
			checks = append(checks, fmt.Sprintf("❌ API: Status %d", resp.StatusCode))
		}
	}

	// Check 5: Database file exists
	dbPath := "/var/lib/velocity-report/sensor_data.db"
	dbCheck, err := exec.RunSudo(fmt.Sprintf("test -f %s && echo 'exists' || echo 'missing'", dbPath))
	if err == nil && strings.TrimSpace(dbCheck) == "exists" {
		// Get database size
		sizeOutput, err := exec.RunSudo(fmt.Sprintf("du -h %s | cut -f1", dbPath))
		if err == nil {
			checks = append(checks, fmt.Sprintf("✅ Database: %s", strings.TrimSpace(sizeOutput)))
		} else {
			checks = append(checks, "✅ Database: EXISTS")
		}
	} else {
		health.Healthy = false
		if health.Message == "" {
			health.Message = "Database file not found"
		}
		checks = append(checks, "❌ Database: MISSING")
	}

	health.Details = strings.Join(checks, "\n")

	if health.Healthy {
		health.Message = "All checks passed"
	}

	return health, nil
}
