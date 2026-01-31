// Package deploy provides command execution utilities for local and remote deployments.
package deploy

import (
	"bytes"
	"os/exec"
)

// CommandExecutor defines an interface for executing shell commands.
// This abstraction enables unit testing without real shell execution.
type CommandExecutor interface {
	// Run executes the command and returns the combined output (stdout+stderr).
	Run() ([]byte, error)

	// SetStdin sets the stdin for the command.
	SetStdin(stdin []byte)
}

// CommandBuilder defines an interface for building shell commands.
// This abstraction enables unit testing of SSH/SCP command construction.
type CommandBuilder interface {
	// BuildCommand creates a CommandExecutor for running local commands.
	BuildCommand(name string, args ...string) CommandExecutor

	// BuildShellCommand creates a CommandExecutor for running shell commands via sh -c.
	BuildShellCommand(command string) CommandExecutor
}

// RealCommandExecutor wraps exec.Cmd to implement CommandExecutor.
type RealCommandExecutor struct {
	cmd *exec.Cmd
}

// Run executes the command and returns combined output.
func (r *RealCommandExecutor) Run() ([]byte, error) {
	return r.cmd.CombinedOutput()
}

// SetStdin sets stdin for the command.
func (r *RealCommandExecutor) SetStdin(stdin []byte) {
	r.cmd.Stdin = bytes.NewReader(stdin)
}

// RealCommandBuilder implements CommandBuilder using exec.Command.
type RealCommandBuilder struct{}

// NewRealCommandBuilder creates a new RealCommandBuilder.
func NewRealCommandBuilder() *RealCommandBuilder {
	return &RealCommandBuilder{}
}

// BuildCommand creates a CommandExecutor for the given command and arguments.
func (b *RealCommandBuilder) BuildCommand(name string, args ...string) CommandExecutor {
	return &RealCommandExecutor{cmd: exec.Command(name, args...)}
}

// BuildShellCommand creates a CommandExecutor for shell commands.
func (b *RealCommandBuilder) BuildShellCommand(command string) CommandExecutor {
	return &RealCommandExecutor{cmd: exec.Command("sh", "-c", command)}
}

// MockCommandExecutor implements CommandExecutor for testing.
type MockCommandExecutor struct {
	// Output is the output to return from Run.
	Output []byte
	// Err is the error to return from Run.
	Err error
	// Stdin holds the stdin data that was set.
	Stdin []byte
	// RunCalled indicates whether Run was called.
	RunCalled bool
}

// Run returns the configured output and error.
func (m *MockCommandExecutor) Run() ([]byte, error) {
	m.RunCalled = true
	return m.Output, m.Err
}

// SetStdin records the stdin data.
func (m *MockCommandExecutor) SetStdin(stdin []byte) {
	m.Stdin = stdin
}

// MockCommandBuilder implements CommandBuilder for testing.
type MockCommandBuilder struct {
	// Commands records all commands that were built.
	Commands []MockBuiltCommand
	// NextExecutor is the next executor to return. If nil, creates a default MockCommandExecutor.
	NextExecutor *MockCommandExecutor
	// ExecutorFactory allows creating executors dynamically based on command.
	ExecutorFactory func(name string, args []string) *MockCommandExecutor
}

// MockBuiltCommand records details of a built command.
type MockBuiltCommand struct {
	Name    string
	Args    []string
	IsShell bool
}

// NewMockCommandBuilder creates a new MockCommandBuilder.
func NewMockCommandBuilder() *MockCommandBuilder {
	return &MockCommandBuilder{}
}

// BuildCommand creates a MockCommandExecutor and records the command details.
func (b *MockCommandBuilder) BuildCommand(name string, args ...string) CommandExecutor {
	b.Commands = append(b.Commands, MockBuiltCommand{
		Name:    name,
		Args:    args,
		IsShell: false,
	})
	return b.getExecutor(name, args)
}

// BuildShellCommand creates a MockCommandExecutor for shell commands.
func (b *MockCommandBuilder) BuildShellCommand(command string) CommandExecutor {
	b.Commands = append(b.Commands, MockBuiltCommand{
		Name:    "sh",
		Args:    []string{"-c", command},
		IsShell: true,
	})
	return b.getExecutor("sh", []string{"-c", command})
}

// getExecutor returns the appropriate executor for the command.
func (b *MockCommandBuilder) getExecutor(name string, args []string) *MockCommandExecutor {
	if b.ExecutorFactory != nil {
		return b.ExecutorFactory(name, args)
	}
	if b.NextExecutor != nil {
		executor := b.NextExecutor
		b.NextExecutor = nil
		return executor
	}
	return &MockCommandExecutor{}
}

// SetNextExecutor sets the executor to return for the next Build* call.
func (b *MockCommandBuilder) SetNextExecutor(executor *MockCommandExecutor) {
	b.NextExecutor = executor
}

// LastCommand returns the most recently built command, or nil if none.
func (b *MockCommandBuilder) LastCommand() *MockBuiltCommand {
	if len(b.Commands) == 0 {
		return nil
	}
	return &b.Commands[len(b.Commands)-1]
}

// Reset clears all recorded commands.
func (b *MockCommandBuilder) Reset() {
	b.Commands = nil
	b.NextExecutor = nil
}
