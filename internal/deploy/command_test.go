package deploy

import (
	"errors"
	"strings"
	"testing"
)

func TestRealCommandExecutor_Run(t *testing.T) {
	builder := NewRealCommandBuilder()

	// Test simple echo command
	cmd := builder.BuildShellCommand("echo hello")
	output, err := cmd.Run()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if strings.TrimSpace(string(output)) != "hello" {
		t.Errorf("Expected 'hello', got: %s", output)
	}
}

func TestRealCommandExecutor_Run_Error(t *testing.T) {
	builder := NewRealCommandBuilder()

	// Test command that fails
	cmd := builder.BuildShellCommand("exit 1")
	_, err := cmd.Run()
	if err == nil {
		t.Error("Expected error for failing command")
	}
}

func TestRealCommandExecutor_SetStdin(t *testing.T) {
	builder := NewRealCommandBuilder()

	// Test stdin with cat
	cmd := builder.BuildShellCommand("cat")
	cmd.SetStdin([]byte("test input"))
	output, err := cmd.Run()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if string(output) != "test input" {
		t.Errorf("Expected 'test input', got: %s", output)
	}
}

func TestRealCommandBuilder_BuildCommand(t *testing.T) {
	builder := NewRealCommandBuilder()

	// Test building command with args
	cmd := builder.BuildCommand("echo", "arg1", "arg2")
	output, err := cmd.Run()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	expected := "arg1 arg2"
	if strings.TrimSpace(string(output)) != expected {
		t.Errorf("Expected '%s', got: %s", expected, output)
	}
}

func TestMockCommandExecutor_Run(t *testing.T) {
	mock := &MockCommandExecutor{
		Output: []byte("mock output"),
		Err:    nil,
	}

	output, err := mock.Run()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if string(output) != "mock output" {
		t.Errorf("Expected 'mock output', got: %s", output)
	}
	if !mock.RunCalled {
		t.Error("Expected RunCalled to be true")
	}
}

func TestMockCommandExecutor_Run_Error(t *testing.T) {
	expectedErr := errors.New("mock error")
	mock := &MockCommandExecutor{
		Output: []byte("error output"),
		Err:    expectedErr,
	}

	output, err := mock.Run()
	if err != expectedErr {
		t.Errorf("Expected mock error, got: %v", err)
	}
	if string(output) != "error output" {
		t.Errorf("Expected 'error output', got: %s", output)
	}
}

func TestMockCommandExecutor_SetStdin(t *testing.T) {
	mock := &MockCommandExecutor{}
	mock.SetStdin([]byte("test stdin"))

	if string(mock.Stdin) != "test stdin" {
		t.Errorf("Expected 'test stdin', got: %s", mock.Stdin)
	}
}

func TestMockCommandBuilder_BuildCommand(t *testing.T) {
	builder := NewMockCommandBuilder()

	cmd := builder.BuildCommand("ssh", "-i", "/path/to/key", "user@host", "echo hello")

	// Verify command was recorded
	if len(builder.Commands) != 1 {
		t.Fatalf("Expected 1 command, got %d", len(builder.Commands))
	}

	recorded := builder.Commands[0]
	if recorded.Name != "ssh" {
		t.Errorf("Expected name 'ssh', got: %s", recorded.Name)
	}
	if len(recorded.Args) != 4 {
		t.Errorf("Expected 4 args, got: %d", len(recorded.Args))
	}
	if recorded.IsShell {
		t.Error("Expected IsShell to be false")
	}

	// Verify it returns a MockCommandExecutor
	_, ok := cmd.(*MockCommandExecutor)
	if !ok {
		t.Error("Expected MockCommandExecutor type")
	}
}

func TestMockCommandBuilder_BuildShellCommand(t *testing.T) {
	builder := NewMockCommandBuilder()

	cmd := builder.BuildShellCommand("echo hello && echo world")

	// Verify command was recorded
	if len(builder.Commands) != 1 {
		t.Fatalf("Expected 1 command, got %d", len(builder.Commands))
	}

	recorded := builder.Commands[0]
	if recorded.Name != "sh" {
		t.Errorf("Expected name 'sh', got: %s", recorded.Name)
	}
	if len(recorded.Args) != 2 || recorded.Args[0] != "-c" {
		t.Errorf("Expected args ['-c', 'echo hello && echo world'], got: %v", recorded.Args)
	}
	if !recorded.IsShell {
		t.Error("Expected IsShell to be true")
	}

	// Verify it returns a MockCommandExecutor
	_, ok := cmd.(*MockCommandExecutor)
	if !ok {
		t.Error("Expected MockCommandExecutor type")
	}
}

func TestMockCommandBuilder_SetNextExecutor(t *testing.T) {
	builder := NewMockCommandBuilder()

	customExecutor := &MockCommandExecutor{
		Output: []byte("custom output"),
	}
	builder.SetNextExecutor(customExecutor)

	cmd := builder.BuildCommand("test")
	mockCmd, ok := cmd.(*MockCommandExecutor)
	if !ok {
		t.Fatal("Expected MockCommandExecutor type")
	}

	if string(mockCmd.Output) != "custom output" {
		t.Errorf("Expected 'custom output', got: %s", mockCmd.Output)
	}

	// Next call should use default executor
	cmd2 := builder.BuildCommand("test2")
	mockCmd2, ok := cmd2.(*MockCommandExecutor)
	if !ok {
		t.Fatal("Expected MockCommandExecutor type")
	}

	if mockCmd2.Output != nil {
		t.Errorf("Expected nil output for default executor, got: %s", mockCmd2.Output)
	}
}

func TestMockCommandBuilder_ExecutorFactory(t *testing.T) {
	builder := NewMockCommandBuilder()

	// Set up factory that returns different outputs based on command
	builder.ExecutorFactory = func(name string, args []string) *MockCommandExecutor {
		if name == "ssh" {
			return &MockCommandExecutor{Output: []byte("ssh output")}
		}
		return &MockCommandExecutor{Output: []byte("other output")}
	}

	sshCmd := builder.BuildCommand("ssh", "arg")
	sshOutput, _ := sshCmd.Run()
	if string(sshOutput) != "ssh output" {
		t.Errorf("Expected 'ssh output', got: %s", sshOutput)
	}

	otherCmd := builder.BuildCommand("other", "arg")
	otherOutput, _ := otherCmd.Run()
	if string(otherOutput) != "other output" {
		t.Errorf("Expected 'other output', got: %s", otherOutput)
	}
}

func TestMockCommandBuilder_LastCommand(t *testing.T) {
	builder := NewMockCommandBuilder()

	// No commands yet
	if builder.LastCommand() != nil {
		t.Error("Expected nil when no commands")
	}

	builder.BuildCommand("first")
	builder.BuildCommand("second", "arg1", "arg2")

	last := builder.LastCommand()
	if last == nil {
		t.Fatal("Expected non-nil last command")
	}
	if last.Name != "second" {
		t.Errorf("Expected 'second', got: %s", last.Name)
	}
	if len(last.Args) != 2 {
		t.Errorf("Expected 2 args, got: %d", len(last.Args))
	}
}

func TestMockCommandBuilder_Reset(t *testing.T) {
	builder := NewMockCommandBuilder()
	builder.BuildCommand("cmd1")
	builder.BuildCommand("cmd2")
	builder.SetNextExecutor(&MockCommandExecutor{})

	builder.Reset()

	if len(builder.Commands) != 0 {
		t.Errorf("Expected 0 commands after reset, got: %d", len(builder.Commands))
	}
	if builder.NextExecutor != nil {
		t.Error("Expected nil NextExecutor after reset")
	}
}
