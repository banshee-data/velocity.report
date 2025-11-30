package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ConfigManager handles configuration management
type ConfigManager struct {
	Target  string
	SSHUser string
	SSHKey  string
}

// Show displays the current configuration
func (c *ConfigManager) Show() error {
	exec := NewExecutor(c.Target, c.SSHUser, c.SSHKey, false)

	fmt.Println("Current velocity.report configuration:")
	fmt.Println()

	// Show service file
	fmt.Println("=== Service Configuration ===")
	serviceOutput, err := exec.RunSudo("cat /etc/systemd/system/velocity-report.service")
	if err != nil {
		return fmt.Errorf("failed to read service file: %w", err)
	}
	fmt.Println(serviceOutput)

	// Show data directory info
	fmt.Println("\n=== Data Directory ===")
	dataInfo, err := exec.RunSudo("ls -lh /var/lib/velocity-report/")
	if err != nil {
		fmt.Printf("Warning: could not read data directory: %v\n", err)
	} else {
		fmt.Println(dataInfo)
	}

	// Show service status
	fmt.Println("\n=== Service Status ===")
	statusOutput, err := exec.RunSudo("systemctl status velocity-report.service --no-pager")
	if err != nil {
		fmt.Printf("Warning: could not get service status: %v\n", err)
	} else {
		fmt.Println(statusOutput)
	}

	// Show recent logs
	fmt.Println("\n=== Recent Logs (last 10 lines) ===")
	logsOutput, err := exec.RunSudo("journalctl -u velocity-report.service -n 10 --no-pager")
	if err != nil {
		fmt.Printf("Warning: could not read logs: %v\n", err)
	} else {
		fmt.Println(logsOutput)
	}

	return nil
}

// Edit allows editing the service configuration
func (c *ConfigManager) Edit() error {
	exec := NewExecutor(c.Target, c.SSHUser, c.SSHKey, false)

	fmt.Println("Interactive configuration editing")
	fmt.Println("==================================")
	fmt.Println()

	// Get current ExecStart line
	grepOutput, err := exec.RunSudo("grep '^ExecStart=' /etc/systemd/system/velocity-report.service")
	if err != nil {
		return fmt.Errorf("failed to read service file: %w", err)
	}

	currentExecStart := strings.TrimSpace(grepOutput)
	fmt.Printf("Current ExecStart:\n%s\n\n", currentExecStart)

	fmt.Println("Common configuration options:")
	fmt.Println("  --listen :PORT          Change API port (default: 8080)")
	fmt.Println("  --units mph|kph|mps     Change speed units")
	fmt.Println("  --timezone TIMEZONE     Change timezone (e.g., US/Pacific)")
	fmt.Println("  --disable-radar         Disable radar sensor")
	fmt.Println("  --enable-lidar          Enable LIDAR integration")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  ExecStart=/usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db --listen :8080 --units mph")
	fmt.Println()
	fmt.Print("Enter new ExecStart line (or press Enter to keep current): ")

	reader := bufio.NewReader(os.Stdin)
	newExecStart, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	newExecStart = strings.TrimSpace(newExecStart)

	if newExecStart == "" {
		fmt.Println("No changes made")
		return nil
	}

	// Validate that it starts with ExecStart=
	if !strings.HasPrefix(newExecStart, "ExecStart=") {
		newExecStart = "ExecStart=" + newExecStart
	}

	// Validate the ExecStart line doesn't contain dangerous characters
	if strings.ContainsAny(newExecStart, "|;&$`\\\"'") {
		return fmt.Errorf("invalid ExecStart line: contains disallowed characters")
	}

	// Update service file using safe file editing (not sed with user input)
	fmt.Println("\nUpdating service file...")

	// Read the current service file
	serviceFilePath := "/etc/systemd/system/velocity-report.service"
	contents, err := exec.RunSudo(fmt.Sprintf("cat %s", serviceFilePath))
	if err != nil {
		return fmt.Errorf("failed to read service file: %w", err)
	}

	// Replace the ExecStart line
	lines := strings.Split(contents, "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "ExecStart=") {
			lines[i] = newExecStart
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("ExecStart line not found in service file")
	}

	newContents := strings.Join(lines, "\n")

	// Write to a temp file and move it into place
	tmpPath := "/tmp/velocity-report.service.tmp"
	if err := exec.WriteFile(tmpPath, newContents); err != nil {
		return fmt.Errorf("failed to write temporary service file: %w", err)
	}

	_, err = exec.RunSudo(fmt.Sprintf("mv %s %s", tmpPath, serviceFilePath))
	if err != nil {
		return fmt.Errorf("failed to update service file: %w", err)
	}

	// Reload systemd
	fmt.Println("Reloading systemd...")
	_, err = exec.RunSudo("systemctl daemon-reload")
	if err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Ask if user wants to restart service
	fmt.Print("\nRestart service now to apply changes? [y/N]: ")
	var restart string
	fmt.Scanln(&restart)

	if strings.ToLower(restart) == "y" {
		fmt.Println("Restarting service...")
		_, err = exec.RunSudo("systemctl restart velocity-report.service")
		if err != nil {
			return fmt.Errorf("failed to restart service: %w", err)
		}

		// Wait and check status
		exec.Run("sleep 2")

		statusOutput, err := exec.RunSudo("systemctl is-active velocity-report.service")
		if err != nil || strings.TrimSpace(statusOutput) != "active" {
			fmt.Println("⚠ Warning: Service may not have started properly")
			fmt.Println("Check status with: velocity-deploy status")
			return nil
		}

		fmt.Println("  ✓ Service restarted successfully")
	} else {
		fmt.Println("Configuration updated. Restart service to apply changes:")
		fmt.Println("  sudo systemctl restart velocity-report.service")
	}

	return nil
}
