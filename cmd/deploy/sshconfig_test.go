package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSSHConfig_NoConfigFile(t *testing.T) {
	// Use a non-existent directory to ensure no config file exists
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	config, err := ParseSSHConfig("testhost")
	if err != nil {
		t.Errorf("ParseSSHConfig() should not error when config doesn't exist: %v", err)
	}
	if config != nil {
		t.Errorf("ParseSSHConfig() should return nil when config doesn't exist, got: %+v", config)
	}
}

func TestParseSSHConfig_HostFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create .ssh directory
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	// Create SSH config file
	configPath := filepath.Join(sshDir, "config")
	configContent := `# SSH Config
Host mypi
    HostName 192.168.1.100
    User pi
    IdentityFile ~/.ssh/id_rsa
    Port 22

Host otherhost
    HostName 10.0.0.1
    User admin
`

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write SSH config: %v", err)
	}

	config, err := ParseSSHConfig("mypi")
	if err != nil {
		t.Fatalf("ParseSSHConfig() error: %v", err)
	}

	if config == nil {
		t.Fatal("ParseSSHConfig() returned nil for existing host")
	}

	if config.Host != "mypi" {
		t.Errorf("Host = %s, want mypi", config.Host)
	}
	if config.HostName != "192.168.1.100" {
		t.Errorf("HostName = %s, want 192.168.1.100", config.HostName)
	}
	if config.User != "pi" {
		t.Errorf("User = %s, want pi", config.User)
	}
	if config.Port != "22" {
		t.Errorf("Port = %s, want 22", config.Port)
	}

	// Verify IdentityFile path expansion
	expectedKeyPath := filepath.Join(tmpDir, ".ssh", "id_rsa")
	if config.IdentityFile != expectedKeyPath {
		t.Errorf("IdentityFile = %s, want %s", config.IdentityFile, expectedKeyPath)
	}
}

func TestParseSSHConfig_HostNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	configPath := filepath.Join(sshDir, "config")
	configContent := `Host otherhost
    HostName 10.0.0.1
`

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write SSH config: %v", err)
	}

	config, err := ParseSSHConfig("nonexistent")
	if err != nil {
		t.Fatalf("ParseSSHConfig() error: %v", err)
	}

	if config != nil {
		t.Errorf("ParseSSHConfig() should return nil for non-existent host, got: %+v", config)
	}
}

func TestParseSSHConfig_WithComments(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	configPath := filepath.Join(sshDir, "config")
	configContent := `# Main config
# Comment line

Host testhost
    # This is a hostname
    HostName test.example.com
    User testuser
`

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write SSH config: %v", err)
	}

	config, err := ParseSSHConfig("testhost")
	if err != nil {
		t.Fatalf("ParseSSHConfig() error: %v", err)
	}

	if config == nil {
		t.Fatal("ParseSSHConfig() returned nil")
	}

	if config.HostName != "test.example.com" {
		t.Errorf("HostName = %s, want test.example.com", config.HostName)
	}
	if config.User != "testuser" {
		t.Errorf("User = %s, want testuser", config.User)
	}
}

func TestMatchHost(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		pattern string
		want    bool
	}{
		{"exact match", "myhost", "myhost", true},
		{"no match", "myhost", "otherhost", false},
		{"empty target", "", "host", false},
		{"empty pattern", "host", "", false},
		{"both empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchHost(tt.target, tt.pattern)
			if got != tt.want {
				t.Errorf("matchHost(%q, %q) = %v, want %v", tt.target, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestResolveSSHTarget_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// No SSH config file
	host, user, key, _, err := ResolveSSHTarget("192.168.1.100", "myuser", "/path/to/key")
	if err != nil {
		t.Fatalf("ResolveSSHTarget() error: %v", err)
	}

	if host != "192.168.1.100" {
		t.Errorf("host = %s, want 192.168.1.100", host)
	}
	if user != "myuser" {
		t.Errorf("user = %s, want myuser", user)
	}
	if key != "/path/to/key" {
		t.Errorf("key = %s, want /path/to/key", key)
	}
}

func TestResolveSSHTarget_WithUserInTarget(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	host, user, key, _, err := ResolveSSHTarget("admin@server.com", "", "/key")
	if err != nil {
		t.Fatalf("ResolveSSHTarget() error: %v", err)
	}

	if host != "server.com" {
		t.Errorf("host = %s, want server.com", host)
	}
	if user != "admin" {
		t.Errorf("user = %s, want admin", user)
	}
	if key != "/key" {
		t.Errorf("key = %s, want /key", key)
	}
}

func TestResolveSSHTarget_WithConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	configPath := filepath.Join(sshDir, "config")
	configContent := `Host myserver
    HostName 192.168.1.50
    User serveruser
    IdentityFile ~/.ssh/server_key
`

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write SSH config: %v", err)
	}

	// Test with config values
	host, user, key, _, err := ResolveSSHTarget("myserver", "", "")
	if err != nil {
		t.Fatalf("ResolveSSHTarget() error: %v", err)
	}

	if host != "192.168.1.50" {
		t.Errorf("host = %s, want 192.168.1.50", host)
	}
	if user != "serveruser" {
		t.Errorf("user = %s, want serveruser", user)
	}

	expectedKey := filepath.Join(tmpDir, ".ssh", "server_key")
	if key != expectedKey {
		t.Errorf("key = %s, want %s", key, expectedKey)
	}
}

func TestResolveSSHTarget_OverrideConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	configPath := filepath.Join(sshDir, "config")
	configContent := `Host myserver
    HostName 192.168.1.50
    User serveruser
    IdentityFile ~/.ssh/server_key
`

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write SSH config: %v", err)
	}

	// Test with overrides
	host, user, key, _, err := ResolveSSHTarget("myserver", "overrideuser", "/override/key")
	if err != nil {
		t.Fatalf("ResolveSSHTarget() error: %v", err)
	}

	if host != "192.168.1.50" {
		t.Errorf("host = %s, want 192.168.1.50 (from config)", host)
	}
	if user != "overrideuser" {
		t.Errorf("user = %s, want overrideuser (override)", user)
	}
	if key != "/override/key" {
		t.Errorf("key = %s, want /override/key (override)", key)
	}
}

func TestResolveSSHTarget_PartialConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	configPath := filepath.Join(sshDir, "config")
	configContent := `Host partial
    HostName 10.0.0.5
`

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write SSH config: %v", err)
	}

	// Config has HostName but not User or IdentityFile
	host, user, key, _, err := ResolveSSHTarget("partial", "cmdlineuser", "/cmd/key")
	if err != nil {
		t.Fatalf("ResolveSSHTarget() error: %v", err)
	}

	if host != "10.0.0.5" {
		t.Errorf("host = %s, want 10.0.0.5 (from config)", host)
	}
	if user != "cmdlineuser" {
		t.Errorf("user = %s, want cmdlineuser (from args)", user)
	}
	if key != "/cmd/key" {
		t.Errorf("key = %s, want /cmd/key (from args)", key)
	}
}
