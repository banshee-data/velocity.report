package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// debugLog prints debug messages when DebugMode is enabled
func debugLog(format string, args ...interface{}) {
	if DebugMode {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

// Executor handles command execution on local or remote targets
type Executor struct {
	Target        string
	SSHUser       string
	SSHKey        string
	IdentityAgent string
	DryRun        bool
}

// NewExecutor creates a new command executor
func NewExecutor(target, sshUser, sshKey, identityAgent string, dryRun bool) *Executor {
	return &Executor{
		Target:        target,
		SSHUser:       sshUser,
		SSHKey:        sshKey,
		IdentityAgent: identityAgent,
		DryRun:        dryRun,
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

	debugLog("Executing: %s (target=%s, local=%v)", command, e.Target, e.IsLocal())

	if e.IsLocal() {
		output, err := e.runLocal(command)
		if err != nil {
			debugLog("Command failed: %v, output: %s", err, output)
		}
		return output, err
	}
	output, err := e.runSSH(command, false)
	if err != nil {
		debugLog("SSH command failed: %v, output: %s", err, output)
	}
	return output, err
}

// RunSudo executes a command with sudo
func (e *Executor) RunSudo(command string) (string, error) {
	if e.DryRun {
		fmt.Printf("[DRY-RUN] Would execute (sudo): %s\n", command)
		return "", nil
	}

	sudoCmd := fmt.Sprintf("sudo %s", command)
	debugLog("Executing (sudo): %s (target=%s, local=%v)", command, e.Target, e.IsLocal())

	if e.IsLocal() {
		output, err := e.runLocal(sudoCmd)
		if err != nil {
			debugLog("Sudo command failed: %v, output: %s", err, output)
		}
		return output, err
	}
	output, err := e.runSSH(sudoCmd, true)
	if err != nil {
		debugLog("SSH sudo command failed: %v, output: %s", err, output)
	}
	return output, err
}

// CopyFile copies a file to the target
func (e *Executor) CopyFile(src, dst string) error {
	if e.DryRun {
		fmt.Printf("[DRY-RUN] Would copy: %s -> %s\n", src, dst)
		return nil
	}

	debugLog("Copying file: %s -> %s (target=%s, local=%v)", src, dst, e.Target, e.IsLocal())

	var err error
	if e.IsLocal() {
		err = e.copyLocal(src, dst)
	} else {
		err = e.copySSH(src, dst)
	}

	if err != nil {
		debugLog("Copy failed: %v", err)
	}
	return err
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

	if e.IdentityAgent != "" {
		args = append(args, "-o", fmt.Sprintf("IdentityAgent=%s", e.IdentityAgent))
	}

	// Disable strict host key checking for easier automation
	args = append(args, "-o", "StrictHostKeyChecking=no")
	args = append(args, "-o", "UserKnownHostsFile=/dev/null")
	args = append(args, "-o", "LogLevel=ERROR")

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

	// First copy to temp directory (only if dst is not already in /tmp)
	tempPath := fmt.Sprintf("/tmp/velocity-report-copy-%d", time.Now().Unix())
	args = append(args, src, fmt.Sprintf("%s:%s", target, tempPath))

	debugLog("SCP command: scp %v", args)
	cmd := exec.Command("scp", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp failed: %w", err)
	}

	// Then move to final destination with sudo if needed
	if tempPath != dst {
		if strings.HasPrefix(dst, "/usr") || strings.HasPrefix(dst, "/etc") || strings.HasPrefix(dst, "/var") {
			_, err := e.RunSudo(fmt.Sprintf("mv %s %s", tempPath, dst))
			return err
		}

		// Move to user directory without sudo
		_, err := e.Run(fmt.Sprintf("mv %s %s", tempPath, dst))
		return err
	}

	return nil
}
