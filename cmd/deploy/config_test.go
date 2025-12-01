package main

import (
	"testing"
)

func TestConfigManager_Structure(t *testing.T) {
	cm := &ConfigManager{
		Target:  "localhost",
		SSHUser: "testuser",
		SSHKey:  "/test/key",
	}

	if cm.Target != "localhost" {
		t.Errorf("Target = %s, want localhost", cm.Target)
	}
	if cm.SSHUser != "testuser" {
		t.Errorf("SSHUser = %s, want testuser", cm.SSHUser)
	}
	if cm.SSHKey != "/test/key" {
		t.Errorf("SSHKey = %s, want /test/key", cm.SSHKey)
	}
}

func TestConfigManager_Show_NoService(t *testing.T) {
	t.Skip("Skipping test that requires sudo and service installation")

	cm := &ConfigManager{
		Target:  "localhost",
		SSHUser: "",
		SSHKey:  "",
	}

	// This will fail because there's no actual service installed
	err := cm.Show()
	if err == nil {
		t.Log("Note: Show succeeded (unexpected in test environment)")
	} else {
		t.Logf("Show failed as expected: %v", err)
	}
}

func TestConfigManager_Edit_NoService(t *testing.T) {
	t.Skip("Skipping test that requires sudo and service installation")

	cm := &ConfigManager{
		Target:  "localhost",
		SSHUser: "",
		SSHKey:  "",
	}

	// This will fail because there's no actual service installed
	err := cm.Edit()
	if err == nil {
		t.Log("Note: Edit succeeded (unexpected in test environment)")
	} else {
		t.Logf("Edit failed as expected: %v", err)
	}
}

func TestConfigManager_RemoteTarget(t *testing.T) {
	cm := &ConfigManager{
		Target:  "192.168.1.100",
		SSHUser: "pi",
		SSHKey:  "/home/user/.ssh/id_rsa",
	}

	if cm.Target != "192.168.1.100" {
		t.Errorf("Target = %s, want 192.168.1.100", cm.Target)
	}
	if cm.SSHUser != "pi" {
		t.Errorf("SSHUser = %s, want pi", cm.SSHUser)
	}
}

func TestConfigManager_EmptyFields(t *testing.T) {
	cm := &ConfigManager{
		Target:  "",
		SSHUser: "",
		SSHKey:  "",
	}

	// Empty fields should be allowed
	if cm.Target != "" {
		t.Errorf("Empty Target should remain empty, got %s", cm.Target)
	}
	if cm.SSHUser != "" {
		t.Errorf("Empty SSHUser should remain empty, got %s", cm.SSHUser)
	}
	if cm.SSHKey != "" {
		t.Errorf("Empty SSHKey should remain empty, got %s", cm.SSHKey)
	}
}

func TestConfigManager_FieldAssignment(t *testing.T) {
	tests := []struct {
		name   string
		target string
		user   string
		key    string
	}{
		{"localhost", "localhost", "", ""},
		{"remote with user", "server.com", "admin", "/root/.ssh/id_rsa"},
		{"IP address", "10.0.0.1", "deploy", "/keys/deploy.pem"},
		{"with port", "host:2222", "user", "/key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &ConfigManager{
				Target:  tt.target,
				SSHUser: tt.user,
				SSHKey:  tt.key,
			}

			if cm.Target != tt.target {
				t.Errorf("Target = %s, want %s", cm.Target, tt.target)
			}
			if cm.SSHUser != tt.user {
				t.Errorf("SSHUser = %s, want %s", cm.SSHUser, tt.user)
			}
			if cm.SSHKey != tt.key {
				t.Errorf("SSHKey = %s, want %s", cm.SSHKey, tt.key)
			}
		})
	}
}
