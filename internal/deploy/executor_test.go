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

func TestExecutor_Run_WithLogger(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", false)
	logger := &testLogger{}
	e.SetLogger(logger)

	// Run a command that will succeed
	_, err := e.Run("echo test")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify logger was called
	if len(logger.logs) == 0 {
		t.Error("Expected logger to be called")
	}
}

func TestExecutor_Run_LocalError_WithLogger(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", false)
	logger := &testLogger{}
	e.SetLogger(logger)

	// Run a command that will fail
	_, err := e.Run("exit 1")
	if err == nil {
		t.Error("Expected error for failing command")
	}

	// Verify logger was called for the error
	if len(logger.logs) < 2 {
		t.Error("Expected logger to be called for error")
	}
}

func TestExecutor_RunSudo_LocalWithLogger(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", true) // Use DryRun to avoid actual sudo
	logger := &testLogger{}
	e.SetLogger(logger)

	// Run a sudo command in dry-run mode
	output, err := e.RunSudo("echo test")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !strings.Contains(output, "[DRY-RUN]") {
		t.Errorf("Expected dry-run output, got: %s", output)
	}
}

func TestExecutor_CopyFile_LocalWithLogger(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	if err := os.WriteFile(srcPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	e := NewExecutor("localhost", "", "", "", false)
	logger := &testLogger{}
	e.SetLogger(logger)

	err := e.CopyFile(srcPath, dstPath)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify logger was called
	if len(logger.logs) == 0 {
		t.Error("Expected logger to be called")
	}
}

func TestExecutor_CopyFile_LocalErrorWithLogger(t *testing.T) {
	tmpDir := t.TempDir()
	e := NewExecutor("localhost", "", "", "", false)
	logger := &testLogger{}
	e.SetLogger(logger)

	err := e.CopyFile(filepath.Join(tmpDir, "nonexistent.txt"), filepath.Join(tmpDir, "dest.txt"))
	if err == nil {
		t.Error("Expected error for missing source file")
	}

	// Verify logger was called for the error
	if len(logger.logs) < 2 {
		t.Error("Expected logger to be called for error")
	}
}

func TestExecutor_CopyFile_LocalToUnwritableDir(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")

	if err := os.WriteFile(srcPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	e := NewExecutor("localhost", "", "", "", false)

	// Try to copy to a path where we can't create the file
	err := e.CopyFile(srcPath, "/nonexistent_dir_12345/dest.txt")
	if err == nil {
		t.Error("Expected error for unwritable destination")
	}
}

func TestExecutor_WriteFile_LocalError(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", false)

	// Try to write to a path where we can't create the file
	err := e.WriteFile("/nonexistent_dir_12345/test.txt", "content")
	if err == nil {
		t.Error("Expected error for unwritable path")
	}
}

func TestExecutor_buildSSHCommand_Sudo(t *testing.T) {
	e := NewExecutor("remote.example.com", "testuser", "/path/to/key", "", false)
	cmd := e.buildSSHCommand("systemctl restart service", true)

	args := cmd.Args
	if args[0] != "ssh" {
		t.Errorf("Expected ssh command, got: %s", args[0])
	}

	// Check that the command is prefixed with sudo
	found := false
	for _, arg := range args {
		if arg == "sudo systemctl restart service" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'sudo systemctl restart service' in args: %v", args)
	}
}

func TestExecutor_SetCommandBuilder(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", false)
	mockBuilder := NewMockCommandBuilder()

	// Set custom command builder
	e.SetCommandBuilder(mockBuilder)

	// Verify the mock builder is used
	mockBuilder.SetNextExecutor(&MockCommandExecutor{
		Output: []byte("mock output"),
	})
	output, err := e.Run("echo test")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if output != "mock output" {
		t.Errorf("Expected 'mock output', got: %s", output)
	}

	// Verify command was recorded
	lastCmd := mockBuilder.LastCommand()
	if lastCmd == nil {
		t.Fatal("Expected command to be recorded")
	}
	if !lastCmd.IsShell {
		t.Error("Expected shell command")
	}
}

func TestExecutor_SetCommandBuilder_Nil(t *testing.T) {
	e := NewExecutor("localhost", "", "", "", false)
	origBuilder := e.CommandBuilder

	// Setting nil should not change the builder
	e.SetCommandBuilder(nil)

	if e.CommandBuilder != origBuilder {
		t.Error("Expected builder to remain unchanged when setting nil")
	}
}

func TestExecutor_Run_Remote_WithMockBuilder(t *testing.T) {
	mockBuilder := NewMockCommandBuilder()
	mockBuilder.SetNextExecutor(&MockCommandExecutor{
		Output: []byte("remote output"),
	})

	e := NewExecutor("remote.example.com", "user", "/path/to/key", "", false)
	e.SetCommandBuilder(mockBuilder)

	output, err := e.Run("uptime")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if output != "remote output" {
		t.Errorf("Expected 'remote output', got: %s", output)
	}

	// Verify SSH command was built correctly
	lastCmd := mockBuilder.LastCommand()
	if lastCmd == nil {
		t.Fatal("Expected command to be recorded")
	}
	if lastCmd.Name != "ssh" {
		t.Errorf("Expected ssh, got: %s", lastCmd.Name)
	}

	// Check for key argument
	hasKey := false
	for i, arg := range lastCmd.Args {
		if arg == "-i" && i+1 < len(lastCmd.Args) && lastCmd.Args[i+1] == "/path/to/key" {
			hasKey = true
			break
		}
	}
	if !hasKey {
		t.Errorf("Expected -i /path/to/key in args: %v", lastCmd.Args)
	}
}

func TestExecutor_RunSudo_Remote_WithMockBuilder(t *testing.T) {
	mockBuilder := NewMockCommandBuilder()
	mockBuilder.SetNextExecutor(&MockCommandExecutor{
		Output: []byte("sudo output"),
	})

	e := NewExecutor("remote.example.com", "user", "", "", false)
	e.SetCommandBuilder(mockBuilder)

	output, err := e.RunSudo("systemctl restart service")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if output != "sudo output" {
		t.Errorf("Expected 'sudo output', got: %s", output)
	}

	// Verify sudo was prepended
	lastCmd := mockBuilder.LastCommand()
	if lastCmd == nil {
		t.Fatal("Expected command to be recorded")
	}

	// Last argument should contain "sudo"
	lastArg := lastCmd.Args[len(lastCmd.Args)-1]
	if !strings.HasPrefix(lastArg, "sudo") {
		t.Errorf("Expected command to start with 'sudo', got: %s", lastArg)
	}
}

func TestExecutor_buildSSHArgs(t *testing.T) {
	tests := []struct {
		name           string
		target         string
		user           string
		key            string
		agent          string
		command        string
		expectTarget   string
		expectHasKey   bool
		expectHasAgent bool
	}{
		{
			name:         "basic remote",
			target:       "server.example.com",
			user:         "admin",
			key:          "",
			agent:        "",
			command:      "ls -la",
			expectTarget: "admin@server.example.com",
		},
		{
			name:         "with ssh key",
			target:       "server.example.com",
			user:         "admin",
			key:          "/home/user/.ssh/id_rsa",
			agent:        "",
			command:      "echo hello",
			expectTarget: "admin@server.example.com",
			expectHasKey: true,
		},
		{
			name:           "with identity agent",
			target:         "server.example.com",
			user:           "admin",
			key:            "",
			agent:          "/tmp/agent.sock",
			command:        "date",
			expectTarget:   "admin@server.example.com",
			expectHasAgent: true,
		},
		{
			name:         "target with @ sign",
			target:       "existinguser@server.example.com",
			user:         "ignored",
			key:          "",
			agent:        "",
			command:      "whoami",
			expectTarget: "existinguser@server.example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewExecutor(tc.target, tc.user, tc.key, tc.agent, false)
			args := e.buildSSHArgs(tc.command)

			// Check target is in args
			found := false
			for _, arg := range args {
				if arg == tc.expectTarget {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected target %s in args: %v", tc.expectTarget, args)
			}

			// Check for key if expected
			if tc.expectHasKey {
				hasKey := false
				for i, arg := range args {
					if arg == "-i" && i+1 < len(args) && args[i+1] == tc.key {
						hasKey = true
						break
					}
				}
				if !hasKey {
					t.Errorf("Expected -i %s in args: %v", tc.key, args)
				}
			}

			// Check for agent if expected
			if tc.expectHasAgent {
				hasAgent := false
				for _, arg := range args {
					if strings.Contains(arg, "IdentityAgent="+tc.agent) {
						hasAgent = true
						break
					}
				}
				if !hasAgent {
					t.Errorf("Expected IdentityAgent=%s in args: %v", tc.agent, args)
				}
			}

			// Command should be last arg
			lastArg := args[len(args)-1]
			if lastArg != tc.command {
				t.Errorf("Expected command %s as last arg, got: %s", tc.command, lastArg)
			}
		})
	}
}

func TestExecutor_buildSCPArgs(t *testing.T) {
	e := NewExecutor("remote.example.com", "user", "/path/to/key", "", false)
	args := e.buildSCPArgs("/local/file.txt", "/remote/file.txt")

	// Check for key
	hasKey := false
	for i, arg := range args {
		if arg == "-i" && i+1 < len(args) && args[i+1] == "/path/to/key" {
			hasKey = true
			break
		}
	}
	if !hasKey {
		t.Errorf("Expected -i /path/to/key in args: %v", args)
	}

	// Check source file
	if args[len(args)-2] != "/local/file.txt" {
		t.Errorf("Expected source file in args: %v", args)
	}

	// Check destination contains target
	lastArg := args[len(args)-1]
	if !strings.HasPrefix(lastArg, "user@remote.example.com:") {
		t.Errorf("Expected destination to start with target, got: %s", lastArg)
	}
}
