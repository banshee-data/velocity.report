package main

import (
	"testing"
)

// Additional tests to increase executor.go coverage

func TestExecutor_CopyFile_DryRun(t *testing.T) {
	exec := NewExecutor("localhost", "", "", "", true)

	// Dry-run should not error
	err := exec.CopyFile("/tmp/source", "/tmp/dest")
	if err != nil {
		t.Errorf("CopyFile() in dry-run mode should not error: %v", err)
	}
}

func TestExecutor_CopyFile_Remote(t *testing.T) {
	exec := NewExecutor("testhost", "testuser", "/test/key", "", true)

	// Dry-run remote copy should not error
	err := exec.CopyFile("/tmp/source", "/tmp/dest")
	if err != nil {
		t.Errorf("CopyFile() remote in dry-run mode should not error: %v", err)
	}
}

func TestExecutor_WriteFile_Remote(t *testing.T) {
	exec := NewExecutor("testhost", "testuser", "/test/key", "", true)

	// Dry-run remote write should not error
	err := exec.WriteFile("/tmp/test.txt", "test content")
	if err != nil {
		t.Errorf("WriteFile() remote in dry-run mode should not error: %v", err)
	}
}

func TestExecutor_RunSudo_Remote_DryRun(t *testing.T) {
	exec := NewExecutor("testhost", "testuser", "/test/key", "", true)

	// Remote sudo in dry-run should not error
	output, err := exec.RunSudo("systemctl status test")
	if err != nil {
		t.Errorf("RunSudo() remote in dry-run mode should not error: %v", err)
	}

	if output != "" {
		t.Errorf("RunSudo() in dry-run should return empty output, got: %s", output)
	}
}

func TestExecutor_Run_Remote_DryRun(t *testing.T) {
	exec := NewExecutor("testhost", "testuser", "/test/key", "", true)

	// Remote run in dry-run should not error
	output, err := exec.Run("echo test")
	if err != nil {
		t.Errorf("Run() remote in dry-run mode should not error: %v", err)
	}

	if output != "" {
		t.Errorf("Run() in dry-run should return empty output, got: %s", output)
	}
}

func TestExecutor_buildSSHCommand(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		sshUser  string
		sshKey   string
		command  string
		wantArgs []string
	}{
		{
			name:     "simple command with key",
			target:   "testhost",
			sshUser:  "testuser",
			sshKey:   "/test/key",
			command:  "echo test",
			wantArgs: []string{"-i", "/test/key", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "LogLevel=ERROR", "testuser@testhost", "echo test"},
		},
		{
			name:     "no SSH key",
			target:   "testhost",
			sshUser:  "testuser",
			sshKey:   "",
			command:  "ls",
			wantArgs: []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "LogLevel=ERROR", "testuser@testhost", "ls"},
		},
		{
			name:     "target with user@host",
			target:   "user@testhost",
			sshUser:  "",
			sshKey:   "",
			command:  "pwd",
			wantArgs: []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "LogLevel=ERROR", "user@testhost", "pwd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewExecutor(tt.target, tt.sshUser, tt.sshKey, "", false)
			cmd := exec.buildSSHCommand(tt.command, false)

			if cmd.Path != "/usr/bin/ssh" && cmd.Path != "ssh" {
				t.Errorf("buildSSHCommand() command path = %s, want ssh", cmd.Path)
			}

			// Check that args contain expected elements
			args := cmd.Args[1:] // Skip the command name itself
			if len(args) != len(tt.wantArgs) {
				t.Errorf("buildSSHCommand() args length = %d, want %d", len(args), len(tt.wantArgs))
				t.Logf("Got args: %v", args)
				t.Logf("Want args: %v", tt.wantArgs)
			}
		})
	}
}
