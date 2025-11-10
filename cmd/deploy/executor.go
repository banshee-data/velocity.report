package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Executor handles command execution locally or via SSH
type Executor struct {
	Target  string
	SSHUser string
	SSHKey  string
	DryRun  bool
}

// NewExecutor creates a new executor
func NewExecutor(target, sshUser, sshKey string, dryRun bool) *Executor {
	return &Executor{
		Target:  target,
		SSHUser: sshUser,
		SSHKey:  sshKey,
		DryRun:  dryRun,
	}
}

// IsLocal returns true if target is localhost
func (e *Executor) IsLocal() bool {
	return e.Target == "localhost" || e.Target == "127.0.0.1" || e.Target == ""
}

// Run executes a command
func (e *Executor) Run(command string) (string, error) {
	if e.DryRun {
		fmt.Printf("[DRY-RUN] Would execute: %s\n", command)
		return "", nil
	}

	if e.IsLocal() {
		return e.runLocal(command)
	}
	return e.runSSH(command, false)
}

// RunSudo executes a command with sudo
func (e *Executor) RunSudo(command string) (string, error) {
	if e.DryRun {
		fmt.Printf("[DRY-RUN] Would execute (sudo): %s\n", command)
		return "", nil
	}

	sudoCmd := fmt.Sprintf("sudo %s", command)

	if e.IsLocal() {
		return e.runLocal(sudoCmd)
	}
	return e.runSSH(sudoCmd, true)
}

// CopyFile copies a file to the target
func (e *Executor) CopyFile(src, dst string) error {
	if e.DryRun {
		fmt.Printf("[DRY-RUN] Would copy: %s -> %s\n", src, dst)
		return nil
	}

	if e.IsLocal() {
		return e.copyLocal(src, dst)
	}
	return e.copySSH(src, dst)
}

// WriteFile writes content to a file on the target
func (e *Executor) WriteFile(path, content string) error {
	if e.DryRun {
		fmt.Printf("[DRY-RUN] Would write to: %s\n", path)
		return nil
	}

	if e.IsLocal() {
		return os.WriteFile(path, []byte(content), 0644)
	}

	// For remote, use SSH to write file
	cmd := fmt.Sprintf("cat > %s", path)
	sshCmd := e.buildSSHCommand(cmd, false)
	sshCmd.Stdin = strings.NewReader(content)

	var stderr bytes.Buffer
	sshCmd.Stderr = &stderr

	if err := sshCmd.Run(); err != nil {
		return fmt.Errorf("ssh write failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

func (e *Executor) runLocal(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (e *Executor) runSSH(command string, useSudo bool) (string, error) {
	cmd := e.buildSSHCommand(command, useSudo)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (e *Executor) buildSSHCommand(command string, useSudo bool) *exec.Cmd {
	args := []string{}

	if e.SSHKey != "" {
		args = append(args, "-i", e.SSHKey)
	}

	// Disable strict host key checking for easier automation
	args = append(args, "-o", "StrictHostKeyChecking=no")
	args = append(args, "-o", "UserKnownHostsFile=/dev/null")

	target := e.Target
	if e.SSHUser != "" && !strings.Contains(target, "@") {
		target = fmt.Sprintf("%s@%s", e.SSHUser, target)
	}

	args = append(args, target, command)

	return exec.Command("ssh", args...)
}

func (e *Executor) copyLocal(src, dst string) error {
	// For local, we need to use sudo to copy to system directories
	if strings.HasPrefix(dst, "/usr") || strings.HasPrefix(dst, "/etc") || strings.HasPrefix(dst, "/var") {
		cmd := exec.Command("sudo", "cp", src, dst)
		return cmd.Run()
	}

	// Regular copy for non-privileged paths
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func (e *Executor) copySSH(src, dst string) error {
	// Use scp for remote copy
	args := []string{}

	if e.SSHKey != "" {
		args = append(args, "-i", e.SSHKey)
	}

	args = append(args, "-o", "StrictHostKeyChecking=no")
	args = append(args, "-o", "UserKnownHostsFile=/dev/null")

	target := e.Target
	if e.SSHUser != "" && !strings.Contains(target, "@") {
		target = fmt.Sprintf("%s@%s", e.SSHUser, target)
	}

	// First copy to temp directory
	tempPath := filepath.Join("/tmp", filepath.Base(dst))
	args = append(args, src, fmt.Sprintf("%s:%s", target, tempPath))

	cmd := exec.Command("scp", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp failed: %w", err)
	}

	// Then move to final destination with sudo
	if strings.HasPrefix(dst, "/usr") || strings.HasPrefix(dst, "/etc") || strings.HasPrefix(dst, "/var") {
		_, err := e.RunSudo(fmt.Sprintf("mv %s %s", tempPath, dst))
		return err
	}

	// Move to user directory without sudo
	_, err := e.Run(fmt.Sprintf("mv %s %s", tempPath, dst))
	return err
}
