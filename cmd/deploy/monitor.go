package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Monitor handles status checking and health monitoring
type Monitor struct {
	Target  string
	SSHUser string
	SSHKey  string
	APIPort int
}

// HealthStatus represents the health check result
type HealthStatus struct {
	Healthy bool
	Message string
	Details string
}

// GetStatus returns the current service status
func (m *Monitor) GetStatus() (string, error) {
	exec := NewExecutor(m.Target, m.SSHUser, m.SSHKey, false)

	// Check systemd status
	output, err := exec.RunSudo("systemctl status velocity-report.service --no-pager")
	if err != nil {
		return "", fmt.Errorf("failed to get service status: %w", err)
	}

	return output, nil
}

// CheckHealth performs comprehensive health check
func (m *Monitor) CheckHealth() (*HealthStatus, error) {
	exec := NewExecutor(m.Target, m.SSHUser, m.SSHKey, false)

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
		checks = append(checks, "✗ Service: NOT RUNNING")
	} else {
		checks = append(checks, "✓ Service: RUNNING")
	}

	// Check 2: Service has been up for some time (not crash-looping)
	uptimeOutput, err := exec.RunSudo("systemctl show velocity-report.service --property=ActiveEnterTimestamp --value")
	if err == nil {
		checks = append(checks, fmt.Sprintf("✓ Started: %s", strings.TrimSpace(uptimeOutput)))
	}

	// Check 3: Check for recent errors in logs
	logsOutput, err := exec.RunSudo("journalctl -u velocity-report.service -n 20 --no-pager")
	if err == nil {
		errorCount := strings.Count(strings.ToLower(logsOutput), "error")
		if errorCount > 5 {
			health.Healthy = false
			health.Message = fmt.Sprintf("Too many errors in logs (%d)", errorCount)
			checks = append(checks, fmt.Sprintf("✗ Logs: %d errors found", errorCount))
		} else {
			checks = append(checks, fmt.Sprintf("✓ Logs: %d errors (acceptable)", errorCount))
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
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(apiURL)
	if err != nil {
		health.Healthy = false
		if health.Message == "" {
			health.Message = "API endpoint not responding"
		}
		checks = append(checks, "✗ API: NOT RESPONDING")
	} else {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			checks = append(checks, "✓ API: RESPONDING")

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
			checks = append(checks, fmt.Sprintf("✗ API: Status %d", resp.StatusCode))
		}
	}

	// Check 5: Database file exists
	dbPath := "/var/lib/velocity-report/sensor_data.db"
	dbCheck, err := exec.RunSudo(fmt.Sprintf("test -f %s && echo 'exists' || echo 'missing'", dbPath))
	if err == nil && strings.TrimSpace(dbCheck) == "exists" {
		// Get database size
		sizeOutput, err := exec.RunSudo(fmt.Sprintf("du -h %s | cut -f1", dbPath))
		if err == nil {
			checks = append(checks, fmt.Sprintf("✓ Database: %s", strings.TrimSpace(sizeOutput)))
		} else {
			checks = append(checks, "✓ Database: EXISTS")
		}
	} else {
		health.Healthy = false
		if health.Message == "" {
			health.Message = "Database file not found"
		}
		checks = append(checks, "✗ Database: MISSING")
	}

	health.Details = strings.Join(checks, "\n")

	if health.Healthy {
		health.Message = "All checks passed"
	}

	return health, nil
}
