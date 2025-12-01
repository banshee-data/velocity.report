package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Service management timing constants
const (
	// serviceStopGracePeriod is the time to wait after stopping the service
	// to allow systemd to fully terminate the process
	serviceStopGracePeriod = 2 * time.Second
	// serviceStartGracePeriod is the time to wait after starting the service
	// to allow it to initialize and be ready for health checks
	serviceStartGracePeriod = 3 * time.Second
)

// Upgrader handles upgrading velocity.report to a new version
type Upgrader struct {
	Target        string
	SSHUser       string
	SSHKey        string
	IdentityAgent string
	BinaryPath    string
	DryRun        bool
	NoBackup      bool
}

// Upgrade performs the upgrade
func (u *Upgrader) Upgrade() error {
	exec := NewExecutor(u.Target, u.SSHUser, u.SSHKey, u.IdentityAgent, u.DryRun)

	fmt.Println("Starting upgrade of velocity.report...")

	// Step 1: Check if service is installed
	if installed, err := u.checkInstalled(exec); err != nil {
		return fmt.Errorf("failed to check installation: %w", err)
	} else if !installed {
		return fmt.Errorf("velocity.report is not installed. Use 'install' command first")
	}

	// Step 2: Get current version info
	currentVersion, err := u.getCurrentVersion(exec)
	if err != nil {
		fmt.Printf("Warning: could not determine current version: %v\n", err)
		currentVersion = "unknown"
	}
	fmt.Printf("Current version: %s\n", currentVersion)

	// Step 3: Backup current installation
	if !u.NoBackup {
		if err := u.backupCurrent(exec); err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}
	} else {
		fmt.Println("Skipping backup (--no-backup flag set)")
	}

	// Step 4: Stop service
	if err := u.stopService(exec); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	// Step 5: Install new binary
	if err := u.installNewBinary(exec); err != nil {
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Step 6: Start service
	if err := u.startService(exec); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Step 7: Verify service is healthy
	if err := u.verifyHealth(exec); err != nil {
		fmt.Println("\n⚠ Warning: Service health check failed!")
		fmt.Println("You may want to rollback using: velocity-deploy rollback")
		return fmt.Errorf("health check failed: %w", err)
	}

	fmt.Println("\n✓ Upgrade completed successfully!")
	return nil
}

func (u *Upgrader) checkInstalled(exec *Executor) (bool, error) {
	output, err := exec.Run("test -f /etc/systemd/system/velocity-report.service && echo 'exists' || echo 'not found'")
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(output) == "exists", nil
}

func (u *Upgrader) getCurrentVersion(exec *Executor) (string, error) {
	// Try to get version from binary
	output, err := exec.Run("/usr/local/bin/velocity-report --version 2>&1 || echo 'unknown'")
	if err != nil {
		return "unknown", err
	}

	version := strings.TrimSpace(output)
	if version == "" || strings.Contains(version, "unknown") {
		// Try to get from file modification time
		timeOutput, err := exec.Run("stat -c %Y /usr/local/bin/velocity-report 2>/dev/null || echo '0'")
		if err == nil && strings.TrimSpace(timeOutput) != "0" {
			return fmt.Sprintf("installed-%s", strings.TrimSpace(timeOutput)), nil
		}
		return "unknown", nil
	}

	return version, nil
}

func (u *Upgrader) backupCurrent(exec *Executor) error {
	fmt.Println("Backing up current installation...")

	timestamp := time.Now().Format("20060102-150405")
	backupDir := fmt.Sprintf("/var/lib/velocity-report/backups/%s", timestamp)

	// Get the service user for proper ownership
	serviceUserOutput, err := exec.Run("systemctl show velocity-report.service -p User --value 2>/dev/null || echo 'velocity'")
	if err != nil {
		return fmt.Errorf("failed to get service user: %w", err)
	}
	serviceUser := strings.TrimSpace(serviceUserOutput)
	if serviceUser == "" {
		serviceUser = "velocity" // fallback to default
	}

	// Create backup directory - run as the service user to avoid permission issues
	_, err = exec.Run(fmt.Sprintf("mkdir -p %s", backupDir))
	if err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Backup binary
	debugLog("Backing up binary from /usr/local/bin/velocity-report to %s/velocity-report", backupDir)
	output, err := exec.RunSudo(fmt.Sprintf("cp /usr/local/bin/velocity-report %s/velocity-report", backupDir))
	if err != nil {
		return fmt.Errorf("failed to backup binary to %s: %w (output: %s)", backupDir, err, output)
	}
	debugLog("Binary backup successful")

	// Backup database
	dbPath := "/var/lib/velocity-report/sensor_data.db"
	debugLog("Checking for database at %s", dbPath)
	output, err = exec.RunSudo(fmt.Sprintf("test -f %s && cp %s %s/sensor_data.db || true", dbPath, dbPath, backupDir))
	if err != nil {
		fmt.Printf("Warning: could not backup database: %v (output: %s)\n", err, output)
	}
	debugLog("Database backup complete (or skipped if not found)")

	// Save version info
	versionInfo := fmt.Sprintf("Backup created: %s\nBinary: /usr/local/bin/velocity-report\n", timestamp)
	versionFile := filepath.Join(backupDir, "version.txt")
	if err := exec.WriteFile(versionFile, versionInfo); err != nil {
		fmt.Printf("Warning: could not write version info: %v\n", err)
	}

	fmt.Printf("  ✓ Backup saved to: %s\n", backupDir)
	return nil
}

func (u *Upgrader) stopService(exec *Executor) error {
	fmt.Println("Stopping service...")

	_, err := exec.RunSudo("systemctl stop velocity-report.service")
	if err != nil {
		return err
	}

	// Wait for service to stop
	exec.Run(fmt.Sprintf("sleep %d", int(serviceStopGracePeriod.Seconds())))

	fmt.Println("  ✓ Service stopped")
	return nil
}

func (u *Upgrader) installNewBinary(exec *Executor) error {
	fmt.Printf("Installing new binary from: %s\n", u.BinaryPath)

	// Copy binary to remote host
	tempPath := "/tmp/velocity-report-new"
	if err := exec.CopyFile(u.BinaryPath, tempPath); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Move to install path
	_, err := exec.RunSudo(fmt.Sprintf("mv %s /usr/local/bin/velocity-report", tempPath))
	if err != nil {
		return fmt.Errorf("failed to move binary: %w", err)
	}

	// Set ownership
	_, err = exec.RunSudo("chown root:root /usr/local/bin/velocity-report")
	if err != nil {
		return fmt.Errorf("failed to set ownership: %w", err)
	}

	// Set permissions
	_, err = exec.RunSudo("chmod 0755 /usr/local/bin/velocity-report")
	if err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	fmt.Println("  ✓ New binary installed")
	return nil
}

func (u *Upgrader) startService(exec *Executor) error {
	fmt.Println("Starting service...")

	_, err := exec.RunSudo("systemctl start velocity-report.service")
	if err != nil {
		return err
	}

	// Wait for service to start
	exec.Run(fmt.Sprintf("sleep %d", int(serviceStartGracePeriod.Seconds())))

	fmt.Println("  ✓ Service started")
	return nil
}

func (u *Upgrader) verifyHealth(exec *Executor) error {
	fmt.Println("Verifying service health...")

	// Check if service is active
	output, err := exec.RunSudo("systemctl is-active velocity-report.service")
	if err != nil || strings.TrimSpace(output) != "active" {
		return fmt.Errorf("service is not active")
	}

	fmt.Println("  ✓ Service is running")
	return nil
}
