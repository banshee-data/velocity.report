package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchHost(t *testing.T) {
	tests := []struct {
		target   string
		pattern  string
		expected bool
	}{
		{"myserver", "myserver", true},
		{"myserver", "otherserver", false},
		{"server1", "server2", false},
		{"", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.target+"_"+tc.pattern, func(t *testing.T) {
			if MatchHost(tc.target, tc.pattern) != tc.expected {
				t.Errorf("MatchHost(%s, %s) = %v, want %v", tc.target, tc.pattern, !tc.expected, tc.expected)
			}
		})
	}
}

func TestParseSSHConfig_NotFound(t *testing.T) {
	// Set HOME to a temp directory without SSH config
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config, err := ParseSSHConfig("myserver")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if config != nil {
		t.Errorf("Expected nil config for missing file, got: %+v", config)
	}
}

func TestParseSSHConfig_HostNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)

	configPath := filepath.Join(sshDir, "config")
	configContent := `Host otherserver
	HostName other.example.com
	User otheruser
`
	os.WriteFile(configPath, []byte(configContent), 0600)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config, err := ParseSSHConfig("myserver")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if config != nil {
		t.Errorf("Expected nil config for non-matching host, got: %+v", config)
	}
}

func TestParseSSHConfig_BasicConfig(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)

	configPath := filepath.Join(sshDir, "config")
	configContent := `Host myserver
	HostName myserver.example.com
	User myuser
	Port 2222
`
	os.WriteFile(configPath, []byte(configContent), 0600)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config, err := ParseSSHConfig("myserver")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if config == nil {
		t.Fatal("Expected config, got nil")
	}
	if config.Host != "myserver" {
		t.Errorf("Expected Host 'myserver', got: %s", config.Host)
	}
	if config.HostName != "myserver.example.com" {
		t.Errorf("Expected HostName 'myserver.example.com', got: %s", config.HostName)
	}
	if config.User != "myuser" {
		t.Errorf("Expected User 'myuser', got: %s", config.User)
	}
	if config.Port != "2222" {
		t.Errorf("Expected Port '2222', got: %s", config.Port)
	}
}

func TestParseSSHConfig_WithIdentityFile(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)

	configPath := filepath.Join(sshDir, "config")
	configContent := `Host myserver
	HostName myserver.example.com
	User myuser
	IdentityFile ~/.ssh/mykey
`
	os.WriteFile(configPath, []byte(configContent), 0600)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config, err := ParseSSHConfig("myserver")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if config == nil {
		t.Fatal("Expected config, got nil")
	}
	expectedKey := filepath.Join(tmpDir, ".ssh", "mykey")
	if config.IdentityFile != expectedKey {
		t.Errorf("Expected IdentityFile '%s', got: %s", expectedKey, config.IdentityFile)
	}
}

func TestParseSSHConfig_WithIdentityAgent(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)

	configPath := filepath.Join(sshDir, "config")
	configContent := `Host myserver
	HostName myserver.example.com
	IdentityAgent "~/Library/agent.sock"
`
	os.WriteFile(configPath, []byte(configContent), 0600)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config, err := ParseSSHConfig("myserver")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if config == nil {
		t.Fatal("Expected config, got nil")
	}
	expectedAgent := filepath.Join(tmpDir, "Library", "agent.sock")
	if config.IdentityAgent != expectedAgent {
		t.Errorf("Expected IdentityAgent '%s', got: %s", expectedAgent, config.IdentityAgent)
	}
}

func TestParseSSHConfig_MultipleHosts(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)

	configPath := filepath.Join(sshDir, "config")
	configContent := `Host server1
	HostName server1.example.com
	User user1

Host server2
	HostName server2.example.com
	User user2

Host server3
	HostName server3.example.com
	User user3
`
	os.WriteFile(configPath, []byte(configContent), 0600)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test first host
	config, _ := ParseSSHConfig("server1")
	if config.HostName != "server1.example.com" {
		t.Errorf("Expected server1.example.com, got: %s", config.HostName)
	}
	if config.User != "user1" {
		t.Errorf("Expected user1, got: %s", config.User)
	}

	// Test middle host
	config, _ = ParseSSHConfig("server2")
	if config.HostName != "server2.example.com" {
		t.Errorf("Expected server2.example.com, got: %s", config.HostName)
	}
	if config.User != "user2" {
		t.Errorf("Expected user2, got: %s", config.User)
	}

	// Test last host
	config, _ = ParseSSHConfig("server3")
	if config.HostName != "server3.example.com" {
		t.Errorf("Expected server3.example.com, got: %s", config.HostName)
	}
	if config.User != "user3" {
		t.Errorf("Expected user3, got: %s", config.User)
	}
}

func TestParseSSHConfig_CommentsAndEmptyLines(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)

	configPath := filepath.Join(sshDir, "config")
	configContent := `# This is a comment
Host myserver
	# Another comment
	HostName myserver.example.com

	User myuser
`
	os.WriteFile(configPath, []byte(configContent), 0600)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	config, err := ParseSSHConfig("myserver")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if config == nil {
		t.Fatal("Expected config, got nil")
	}
	if config.HostName != "myserver.example.com" {
		t.Errorf("Expected HostName 'myserver.example.com', got: %s", config.HostName)
	}
	if config.User != "myuser" {
		t.Errorf("Expected User 'myuser', got: %s", config.User)
	}
}

func TestParseSSHConfigFrom_ExplicitPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "custom_config")
	configContent := `Host myserver
	HostName custom.example.com
	User customuser
`
	os.WriteFile(configPath, []byte(configContent), 0600)

	config, err := ParseSSHConfigFrom("myserver", configPath)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if config == nil {
		t.Fatal("Expected config, got nil")
	}
	if config.HostName != "custom.example.com" {
		t.Errorf("Expected HostName 'custom.example.com', got: %s", config.HostName)
	}
}

func TestResolveSSHTarget_NoConfig(t *testing.T) {
	// Set HOME to a temp directory without SSH config
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	host, user, key, agent, err := ResolveSSHTarget("myserver.example.com", "myuser", "/path/to/key")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if host != "myserver.example.com" {
		t.Errorf("Expected host 'myserver.example.com', got: %s", host)
	}
	if user != "myuser" {
		t.Errorf("Expected user 'myuser', got: %s", user)
	}
	if key != "/path/to/key" {
		t.Errorf("Expected key '/path/to/key', got: %s", key)
	}
	if agent != "" {
		t.Errorf("Expected empty agent, got: %s", agent)
	}
}

func TestResolveSSHTarget_WithAtSign(t *testing.T) {
	// Set HOME to a temp directory without SSH config
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	host, user, _, _, err := ResolveSSHTarget("deployuser@myserver.example.com", "", "")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if host != "myserver.example.com" {
		t.Errorf("Expected host 'myserver.example.com', got: %s", host)
	}
	if user != "deployuser" {
		t.Errorf("Expected user 'deployuser', got: %s", user)
	}
}

func TestResolveSSHTarget_WithConfig(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)

	configPath := filepath.Join(sshDir, "config")
	configContent := `Host myserver
	HostName myserver.example.com
	User configuser
	IdentityFile ~/.ssh/configkey
	IdentityAgent ~/Library/agent.sock
`
	os.WriteFile(configPath, []byte(configContent), 0600)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	host, user, key, agent, err := ResolveSSHTarget("myserver", "", "")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if host != "myserver.example.com" {
		t.Errorf("Expected host 'myserver.example.com', got: %s", host)
	}
	if user != "configuser" {
		t.Errorf("Expected user 'configuser', got: %s", user)
	}
	expectedKey := filepath.Join(tmpDir, ".ssh", "configkey")
	if key != expectedKey {
		t.Errorf("Expected key '%s', got: %s", expectedKey, key)
	}
	expectedAgent := filepath.Join(tmpDir, "Library", "agent.sock")
	if agent != expectedAgent {
		t.Errorf("Expected agent '%s', got: %s", expectedAgent, agent)
	}
}

func TestResolveSSHTarget_CommandLineOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)

	configPath := filepath.Join(sshDir, "config")
	configContent := `Host myserver
	HostName myserver.example.com
	User configuser
	IdentityFile ~/.ssh/configkey
`
	os.WriteFile(configPath, []byte(configContent), 0600)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Command-line user and key should override config
	host, user, key, _, err := ResolveSSHTarget("myserver", "cliuser", "/cli/key")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if host != "myserver.example.com" {
		t.Errorf("Expected host 'myserver.example.com', got: %s", host)
	}
	// User should be CLI value
	if user != "cliuser" {
		t.Errorf("Expected user 'cliuser', got: %s", user)
	}
	// Key should be CLI value
	if key != "/cli/key" {
		t.Errorf("Expected key '/cli/key', got: %s", key)
	}
}
