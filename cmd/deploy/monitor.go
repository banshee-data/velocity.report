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
			status.LogOK = count <= maxAcceptableLogErrors
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
		b.WriteString(fmt.Sprintf("🛢  Database     ✅ %s\n", s.DatabaseSize))
	} else {
		b.WriteString("🛢  Database     ❌ missing\n")
	}

	// Logs
	if s.LogOK {
		b.WriteString(fmt.Sprintf("🪵  Logs         ✅ %d errors\n", s.LogErrors))
	} else {
		b.WriteString(fmt.Sprintf("🪵  Logs         ⚠️  %d errors\n", s.LogErrors))
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

// DiskItem represents a file or directory with its size
type DiskItem struct {
	Path string
	Size string
	Type string // "file" or "directory"
}

// ScanDiskUsage performs a detailed disk scan to find largest files and directories
func (m *Monitor) ScanDiskUsage(ctx context.Context) (string, error) {
	exec := NewExecutor(m.Target, m.SSHUser, m.SSHKey, m.IdentityAgent, false)

	var output strings.Builder

	// Find top 3 largest directories
	fmt.Print("\n📁 Scanning directories...")
	dirCmd := `find /var/lib/velocity-report /home /opt 2>/dev/null -type d -exec du -sh {} + 2>/dev/null | sort -rh | head -3`
	dirOutput, err := exec.RunSudo(dirCmd)
	fmt.Print("\r\033[K") // Clear line

	output.WriteString("\n📁 Top 3 Largest Directories:\n")
	if err != nil {
		output.WriteString(fmt.Sprintf("  ⚠ Failed to scan directories: %v\n", err))
	} else {
		lines := strings.Split(strings.TrimSpace(dirOutput), "\n")
		for i, line := range lines {
			if line != "" {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					size := parts[0]
					path := strings.Join(parts[1:], " ")
					output.WriteString(fmt.Sprintf("  %d. %s - %s\n", i+1, size, path))
				}
			}
		}
	}
	fmt.Print(output.String())
	output.Reset()

	// Find top 3 largest files (traverse all directories)
	fmt.Print("📄 Scanning files...")
	fileCmd := `find /var/lib/velocity-report /home /opt 2>/dev/null -type f -exec du -h {} + 2>/dev/null | sort -rh | head -3`
	fileOutput, err := exec.RunSudo(fileCmd)
	fmt.Print("\r\033[K") // Clear line

	output.WriteString("\n📄 Top 3 Largest Files:\n")
	if err != nil {
		output.WriteString(fmt.Sprintf("  ⚠ Failed to scan files: %v\n", err))
	} else {
		lines := strings.Split(strings.TrimSpace(fileOutput), "\n")
		for i, line := range lines {
			if line != "" {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					size := parts[0]
					path := strings.Join(parts[1:], " ")
					output.WriteString(fmt.Sprintf("  %d. %s - %s\n", i+1, size, path))
				}
			}
		}
	}
	fmt.Print(output.String())
	output.Reset()

	// Show velocity-report specific data breakdown
	fmt.Print("📊 Scanning data directory...")
	dataCmd := `du -sh /var/lib/velocity-report/* 2>/dev/null | sort -rh`
	dataOutput, err := exec.RunSudo(dataCmd)
	fmt.Print("\r\033[K") // Clear line

	output.WriteString("\n📊 Velocity Report Data Directory:\n")
	if err != nil {
		output.WriteString(fmt.Sprintf("  ⚠ Failed to scan data directory: %v\n", err))
	} else {
		lines := strings.Split(strings.TrimSpace(dataOutput), "\n")
		for _, line := range lines {
			if line != "" {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					size := parts[0]
					path := strings.Join(parts[1:], " ")
					// Extract just the filename
					filename := path
					if idx := strings.LastIndex(path, "/"); idx >= 0 {
						filename = path[idx+1:]
					}
					output.WriteString(fmt.Sprintf("  • %s - %s\n", size, filename))
				}
			}
		}
	}
	fmt.Print(output.String())
	output.Reset()

	// Database file analysis (if database exists)
	fmt.Print("🗄️  Analyzing database...")
	output.WriteString("\n🗄️  Database Statistics:\n")
	dbPath := "/var/lib/velocity-report/sensor_data.db"
	dbCheck, err := exec.RunSudo(fmt.Sprintf("test -f %s && echo 'exists' || echo 'missing'", dbPath))
	if err == nil && strings.TrimSpace(dbCheck) == "exists" {
		// Get database size in MB
		sizeCmd := fmt.Sprintf("du -b %s | cut -f1", dbPath)
		sizeOutput, err := exec.RunSudo(sizeCmd)
		if err == nil {
			bytes := strings.TrimSpace(sizeOutput)
			sizeMBCmd := fmt.Sprintf("echo 'scale=2; %s / 1048576' | bc 2>/dev/null || echo 'N/A'", bytes)
			sizeMB, err := exec.RunSudo(sizeMBCmd)
			if err == nil && strings.TrimSpace(sizeMB) != "N/A" {
				output.WriteString(fmt.Sprintf("  Total Size: %s MB\n", strings.TrimSpace(sizeMB)))
			}
		}

		// Get size per table (requires sqlite3)
		tablesCmd := fmt.Sprintf(`sqlite3 %s "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name" 2>/dev/null`, dbPath)
		tablesOutput, err := exec.RunSudo(tablesCmd)
		if err == nil && tablesOutput != "" {
			tables := strings.Split(strings.TrimSpace(tablesOutput), "\n")
			output.WriteString("  \n  Size per Table (MB):\n")

			for _, table := range tables {
				table = strings.TrimSpace(table)
				if table == "" {
					continue
				}

				// Get row count
				countCmd := fmt.Sprintf(`sqlite3 %s "SELECT COUNT(*) FROM %s" 2>/dev/null`, dbPath, table)
				countOutput, err := exec.RunSudo(countCmd)
				if err == nil {
					count := strings.TrimSpace(countOutput)

					// Estimate size in MB using dbstat
					sizeEstCmd := fmt.Sprintf(`sqlite3 %s "SELECT ROUND(CAST(SUM(payload) / 1048576.0 AS REAL), 2) FROM dbstat WHERE name='%s'" 2>/dev/null || echo '0.00'`, dbPath, table)
					sizeEst, err := exec.RunSudo(sizeEstCmd)
					if err == nil {
						sizeMB := strings.TrimSpace(sizeEst)
						if sizeMB == "" || sizeMB == "0.00" {
							sizeMB = "< 0.01"
						}
						output.WriteString(fmt.Sprintf("    • %-20s %10s rows    %8s MB\n", table, count, sizeMB))
					} else {
						output.WriteString(fmt.Sprintf("    • %-20s %10s rows\n", table, count))
					}
				}
			}
		}

		// Count total records in sensor_data (if table exists)
		countCmd := fmt.Sprintf("sqlite3 %s 'SELECT COUNT(*) FROM sensor_data' 2>/dev/null || echo 'N/A'", dbPath)
		countOutput, err := exec.RunSudo(countCmd)
		if err == nil && strings.TrimSpace(countOutput) != "N/A" {
			output.WriteString(fmt.Sprintf("  \n  Total Records: %s\n", strings.TrimSpace(countOutput)))
		}

		// Get date range (if sqlite3 is available)
		rangeCmd := fmt.Sprintf("sqlite3 %s \"SELECT MIN(timestamp), MAX(timestamp) FROM sensor_data\" 2>/dev/null || echo 'N/A'", dbPath)
		rangeOutput, err := exec.RunSudo(rangeCmd)
		if err == nil && strings.TrimSpace(rangeOutput) != "N/A" {
			parts := strings.Split(strings.TrimSpace(rangeOutput), "|")
			if len(parts) == 2 {
				output.WriteString(fmt.Sprintf("  Date Range: %s to %s\n", parts[0], parts[1]))
			}
		}
	} else {
		output.WriteString("  ⚠ Database file not found\n")
	}
	fmt.Print("\r\033[K") // Clear line
	fmt.Print(output.String())
	output.Reset()

	// Backup analysis
	fmt.Print("💾 Analyzing backups...")
	output.WriteString("\n💾 Backup Statistics:\n")
	backupDir := "/var/lib/velocity-report/backups"

	// Count existing backups
	backupCountCmd := fmt.Sprintf("find %s -maxdepth 1 -type d ! -path %s 2>/dev/null | wc -l", backupDir, backupDir)
	backupCountOutput, err := exec.RunSudo(backupCountCmd)
	if err == nil {
		backupCount := strings.TrimSpace(backupCountOutput)
		output.WriteString(fmt.Sprintf("  Existing Backups: %s\n", backupCount))

		// Get total backup directory size
		backupSizeCmd := fmt.Sprintf("du -sb %s 2>/dev/null | cut -f1", backupDir)
		backupSizeOutput, err := exec.RunSudo(backupSizeCmd)
		if err == nil {
			bytes := strings.TrimSpace(backupSizeOutput)
			// Convert to MB
			sizeMBCmd := fmt.Sprintf("echo 'scale=2; %s / 1048576' | bc 2>/dev/null || echo 'N/A'", bytes)
			sizeMB, err := exec.RunSudo(sizeMBCmd)
			if err == nil && strings.TrimSpace(sizeMB) != "N/A" {
				output.WriteString(fmt.Sprintf("  Total Backup Size: %s MB\n", strings.TrimSpace(sizeMB)))
			}

			// Calculate average backup size
			if backupCount != "0" {
				avgSizeCmd := fmt.Sprintf("echo 'scale=2; %s / %s / 1048576' | bc 2>/dev/null || echo 'N/A'", bytes, backupCount)
				avgSize, err := exec.RunSudo(avgSizeCmd)
				if err == nil && strings.TrimSpace(avgSize) != "N/A" {
					avgSizeMB := strings.TrimSpace(avgSize)
					output.WriteString(fmt.Sprintf("  Avg Backup Size: %s MB\n", avgSizeMB))

					// Calculate available space and potential backups
					dfCmd := "df /var/lib/velocity-report | tail -1 | awk '{print $4}'"
					availOutput, err := exec.RunSudo(dfCmd)
					if err == nil {
						availKB := strings.TrimSpace(availOutput)
						// Convert KB to MB and calculate how many more backups will fit
						moreBackupsCmd := fmt.Sprintf("echo 'scale=0; (%s / 1024) / %s' | bc 2>/dev/null || echo 'N/A'", availKB, avgSizeMB)
						moreBackups, err := exec.RunSudo(moreBackupsCmd)
						if err == nil && strings.TrimSpace(moreBackups) != "N/A" {
							output.WriteString(fmt.Sprintf("  More Backups Possible: ~%s (based on available space)\n", strings.TrimSpace(moreBackups)))
						}
					}
				}
			}
		}

		// List most recent backups
		recentCmd := fmt.Sprintf("ls -t %s 2>/dev/null | head -5", backupDir)
		recentOutput, err := exec.RunSudo(recentCmd)
		if err == nil && recentOutput != "" {
			output.WriteString("  \n  Most Recent Backups:\n")
			recent := strings.Split(strings.TrimSpace(recentOutput), "\n")
			for i, backup := range recent {
				if backup != "" && i < 5 {
					// Get size of this backup
					backupPath := fmt.Sprintf("%s/%s", backupDir, backup)
					sizeCmd := fmt.Sprintf("du -sh %s 2>/dev/null | cut -f1", backupPath)
					size, err := exec.RunSudo(sizeCmd)
					if err == nil {
						output.WriteString(fmt.Sprintf("    • %s (%s)\n", backup, strings.TrimSpace(size)))
					} else {
						output.WriteString(fmt.Sprintf("    • %s\n", backup))
					}
				}
			}
		}
	} else {
		output.WriteString("  ⚠ Backup directory not found\n")
	}
	fmt.Print("\r\033[K") // Clear line
	fmt.Print(output.String())

	return "\n", nil
}
