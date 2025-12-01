package main

import (
	"strings"
	"testing"
)

func TestMonitor_GetStatus(t *testing.T) {
	t.Skip("Skipping test that requires sudo and service installation")

	m := &Monitor{
		Target:  "localhost",
		SSHUser: "",
		SSHKey:  "",
		APIPort: 8080,
	}

	// This will fail if systemd service doesn't exist, which is expected in tests
	// We're just testing the function doesn't panic
	_, err := m.GetStatus()
	if err == nil {
		t.Log("Service status retrieved successfully")
	} else {
		t.Log("Service status failed (expected in test environment):", err)
	}
}

func TestMonitor_CheckHealth(t *testing.T) {
	t.Skip("Skipping test that requires sudo and service installation")

	m := &Monitor{
		Target:  "localhost",
		SSHUser: "",
		SSHKey:  "",
		APIPort: 8080,
	}

	// This will fail if service doesn't exist, which is expected in tests
	health, err := m.CheckHealth()
	if err != nil {
		t.Log("Health check returned error (expected in test environment):", err)
	}

	if health != nil {
		t.Logf("Health check result: healthy=%v, message=%s", health.Healthy, health.Message)
		// Health should be false in test environment without actual service
		if health.Healthy {
			t.Log("Note: Service appears healthy (unexpected in test environment)")
		}
	}
}

func TestHealthStatus_Structure(t *testing.T) {
	health := &HealthStatus{
		Healthy: true,
		Message: "All checks passed",
		Details: "Service is running normally",
	}

	if !health.Healthy {
		t.Error("Expected Healthy to be true")
	}
	if health.Message != "All checks passed" {
		t.Errorf("Message = %s, want 'All checks passed'", health.Message)
	}
	if health.Details != "Service is running normally" {
		t.Errorf("Details = %s, want 'Service is running normally'", health.Details)
	}
}

func TestMonitor_CheckHealth_Fields(t *testing.T) {
	m := &Monitor{
		Target:  "192.168.1.100",
		SSHUser: "testuser",
		SSHKey:  "/test/key",
		APIPort: 9090,
	}

	if m.Target != "192.168.1.100" {
		t.Errorf("Target = %s, want 192.168.1.100", m.Target)
	}
	if m.SSHUser != "testuser" {
		t.Errorf("SSHUser = %s, want testuser", m.SSHUser)
	}
	if m.SSHKey != "/test/key" {
		t.Errorf("SSHKey = %s, want /test/key", m.SSHKey)
	}
	if m.APIPort != 9090 {
		t.Errorf("APIPort = %d, want 9090", m.APIPort)
	}
}

func TestMonitor_APIEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		port     int
		wantHost string
	}{
		{"localhost", "localhost", 8080, "localhost"},
		{"remote IP", "192.168.1.100", 8080, "192.168.1.100"},
		{"remote hostname", "pi.local", 9090, "pi.local"},
		{"custom port", "server.com", 3000, "server.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Monitor{
				Target:  tt.target,
				APIPort: tt.port,
			}

			// The monitor should use these values when constructing API URLs
			if m.Target != tt.wantHost {
				t.Errorf("Target = %s, want %s", m.Target, tt.wantHost)
			}
			if m.APIPort != tt.port {
				t.Errorf("APIPort = %d, want %d", m.APIPort, tt.port)
			}
		})
	}
}

func TestHealthStatus_EmptyMessage(t *testing.T) {
	health := &HealthStatus{
		Healthy: false,
		Message: "",
		Details: "",
	}

	if health.Healthy {
		t.Error("Expected Healthy to be false")
	}
	if health.Message != "" {
		t.Error("Expected empty message")
	}
}

func TestMonitor_GetStatus_ErrorHandling(t *testing.T) {
	t.Skip("Skipping test that requires sudo and service installation")

	m := &Monitor{
		Target:  "nonexistent.invalid.host.12345",
		SSHUser: "nobody",
		SSHKey:  "/nonexistent/key",
		APIPort: 8080,
	}

	// Should handle errors gracefully
	status, err := m.GetStatus()
	if err != nil {
		// Error is expected for invalid host
		if !strings.Contains(err.Error(), "failed to get service status") {
			t.Errorf("Expected 'failed to get service status' error, got: %v", err)
		}
	} else {
		t.Logf("Unexpected success with status: %s", status)
	}
}

func TestMonitor_CheckHealth_UnhealthyScenario(t *testing.T) {
	// Test that we can create an unhealthy status
	health := &HealthStatus{
		Healthy: false,
		Message: "Service is not running",
		Details: "✗ Service: NOT RUNNING\n✗ Logs: 10 errors found",
	}

	if health.Healthy {
		t.Error("Expected Healthy to be false for unhealthy status")
	}

	if !strings.Contains(health.Message, "not running") {
		t.Error("Expected message to contain 'not running'")
	}

	if !strings.Contains(health.Details, "NOT RUNNING") {
		t.Error("Expected details to contain status check results")
	}
}
