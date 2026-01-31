package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testLogger struct {
	logs []string
}

func (l *testLogger) Debugf(format string, args ...interface{}) {
	l.logs = append(l.logs, format)
}

func TestNewExecutor(t *testing.T) {
	e := NewExecutor("host.example.com", "user", "/path/to/key", "/path/to/agent", false)

	if e.Target != "host.example.com" {
		t.Errorf("Expected target host.example.com, got %s", e.Target)
	}
	if e.SSHUser != "user" {
		t.Errorf("Expected user, got %s", e.SSHUser)
	}
	if e.SSHKey != "/path/to/key" {
		t.Errorf("Expected /path/to/key, got %s", e.SSHKey)
	}
	if e.IdentityAgent != "/path/to/agent" {
		t.Errorf("Expected /path/to/agent, got %s", e.IdentityAgent)
	}
	if e.DryRun {
		t.Error("Expected DryRun false")
	}
}

func TestExecutor_IsLocal(t *testing.T) {
	tests := []struct {
		target   string
		expected bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"", true},
		{"remote.example.com", false},
		{"192.168.1.100", false},
	}

	for _, tc := range tests {
		t.Run(tc.target, func(t *testing.T) {
			e := NewExecutor(tc.target, "", "", "", false)
			if e.IsLocal() != tc.expected {
				t.Errorf("IsLocal(%s) = %v, want %v", tc.target, e.IsLocal(), tc.expected)
			}
		})
	}
}

func TestExecutor_SetLogger(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", false)
	logger := &testLogger{}
	e.SetLogger(logger)

	// Verify logger is set (by running a command)
	e.DryRun = true
	e.Run("echo test")

	// SetLogger with nil should not panic
	e.SetLogger(nil)
}

func TestExecutor_Run_DryRun(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", true)
	output, err := e.Run("echo hello")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !strings.Contains(output, "[DRY-RUN]") {
		t.Errorf("Expected dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "echo hello") {
		t.Errorf("Expected command in output, got: %s", output)
	}
}

func TestExecutor_Run_Local(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", false)
	output, err := e.Run("echo hello")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if strings.TrimSpace(output) != "hello" {
		t.Errorf("Expected 'hello', got: %s", output)
	}
}

func TestExecutor_Run_LocalError(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", false)
	_, err := e.Run("exit 1")

	if err == nil {
		t.Error("Expected error for failed command")
	}
}

func TestExecutor_RunSudo_Local(t *testing.T) {
	// Test that RunSudo prepends sudo (use a command that doesn't actually require sudo)
	e := NewExecutor("localhost", "", "", "", false)
	logger := &testLogger{}
	e.SetLogger(logger)

	// This will fail because 'sudo echo test' requires sudo, but we can check the command was constructed
	// We test with DryRun to verify the command format
	e.DryRun = true
	output, _ := e.RunSudo("echo test")

	if !strings.Contains(output, "sudo") {
		t.Errorf("Expected sudo in command, got: %s", output)
	}
}

func TestExecutor_RunSudo_DryRun(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", true)
	output, err := e.RunSudo("cat /etc/passwd")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !strings.Contains(output, "[DRY-RUN]") {
		t.Errorf("Expected dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "sudo") {
		t.Errorf("Expected sudo in output, got: %s", output)
	}
}

func TestExecutor_CopyFile_DryRun(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", true)
	err := e.CopyFile("/source/file", "/dest/file")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestExecutor_CopyFile_Local(t *testing.T) {
	// Create temp files
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	if err := os.WriteFile(srcPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	e := NewExecutor("localhost", "", "", "", false)
	err := e.CopyFile(srcPath, dstPath)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify content was copied
	content, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("Expected 'test content', got: %s", string(content))
	}
}

func TestExecutor_CopyFile_LocalMissingSrc(t *testing.T) {
	tmpDir := t.TempDir()
	e := NewExecutor("localhost", "", "", "", false)
	err := e.CopyFile(filepath.Join(tmpDir, "nonexistent.txt"), filepath.Join(tmpDir, "dest.txt"))

	if err == nil {
		t.Error("Expected error for missing source file")
	}
}

func TestExecutor_WriteFile_DryRun(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", true)
	err := e.WriteFile("/tmp/test.txt", "content")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestExecutor_WriteFile_Local(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	e := NewExecutor("localhost", "", "", "", false)
	err := e.WriteFile(filePath, "test content")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("Expected 'test content', got: %s", string(content))
	}
}

func TestExecutor_buildSSHCommand(t *testing.T) {
	e := NewExecutor("remote.example.com", "testuser", "/path/to/key", "/path/to/agent", false)
	cmd := e.buildSSHCommand("echo hello", false)

	args := cmd.Args
	if args[0] != "ssh" {
		t.Errorf("Expected ssh command, got: %s", args[0])
	}

	// Check for key argument
	keyFound := false
	for i, arg := range args {
		if arg == "-i" && i+1 < len(args) && args[i+1] == "/path/to/key" {
			keyFound = true
			break
		}
	}
	if !keyFound {
		t.Errorf("Expected -i /path/to/key in args: %v", args)
	}

	// Check for IdentityAgent
	agentFound := false
	for _, arg := range args {
		if strings.Contains(arg, "IdentityAgent=/path/to/agent") {
			agentFound = true
			break
		}
	}
	if !agentFound {
		t.Errorf("Expected IdentityAgent=/path/to/agent in args: %v", args)
	}

	// Check for target with user
	targetFound := false
	for _, arg := range args {
		if arg == "testuser@remote.example.com" {
			targetFound = true
			break
		}
	}
	if !targetFound {
		t.Errorf("Expected testuser@remote.example.com in args: %v", args)
	}
}

func TestExecutor_buildSSHCommand_NoUser(t *testing.T) {
	e := NewExecutor("remote.example.com", "", "", "", false)
	cmd := e.buildSSHCommand("echo hello", false)

	args := cmd.Args
	// Should use target without @ since no user
	targetFound := false
	for _, arg := range args {
		if arg == "remote.example.com" {
			targetFound = true
			break
		}
	}
	if !targetFound {
		t.Errorf("Expected remote.example.com in args: %v", args)
	}
}

func TestExecutor_buildSSHCommand_TargetWithAt(t *testing.T) {
	// If target already contains @, don't add user prefix
	e := NewExecutor("existing@remote.example.com", "ignored", "", "", false)
	cmd := e.buildSSHCommand("echo hello", false)

	args := cmd.Args
	// Should use target as-is since it already contains @
	targetFound := false
	for _, arg := range args {
		if arg == "existing@remote.example.com" {
			targetFound = true
			break
		}
	}
	if !targetFound {
		t.Errorf("Expected existing@remote.example.com in args: %v", args)
	}
}

func TestLogger_NopLogger(t *testing.T) {
	// Test that nopLogger doesn't panic
	logger := nopLogger{}
	logger.Debugf("test %s", "message")
}
