package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Installer handles installation of velocity.report service
type Installer struct {
	Target        string
	SSHUser       string
	SSHKey        string
	IdentityAgent string
	BinaryPath    string
	DBPath        string
	DryRun        bool
}

const (
	serviceName    = "velocity-report"
	installPath    = "/usr/local/bin/velocity-report"
	dataDir        = "/var/lib/velocity-report"
	serviceFile    = "velocity-report.service"
	serviceUser    = "velocity"
	serviceContent = `[Unit]
Description=Velocity.report radar monitor service
After=network.target

[Service]
User=velocity
Group=velocity
Type=simple
ExecStart=/usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db
WorkingDirectory=/var/lib/velocity-report
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=velocity-report

[Install]
WantedBy=multi-user.target
`
)

// Install performs the installation
func (i *Installer) Install() error {
	exec := NewExecutor(i.Target, i.SSHUser, i.SSHKey, i.IdentityAgent, i.DryRun)

	fmt.Println("Starting installation of velocity.report...")

	// Step 1: Validate binary exists
	if err := i.validateBinary(); err != nil {
		return fmt.Errorf("binary validation failed: %w", err)
	}

	// Step 2: Check if already installed
	if installed, err := i.checkExisting(exec); err != nil {
		return fmt.Errorf("failed to check existing installation: %w", err)
	} else if installed {
		fmt.Println("velocity.report is already installed. Use 'upgrade' command to update.")
		return nil
	}

	// Step 3: Create service user
	if err := i.createServiceUser(exec); err != nil {
		return fmt.Errorf("failed to create service user: %w", err)
	}

	// Step 4: Create data directory
	if err := i.createDataDirectory(exec); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Step 5: Install binary
	if err := i.installBinary(exec); err != nil {
		return fmt.Errorf("failed to install binary: %w", err)
	}

	// Step 6: Install systemd service
	if err := i.installService(exec); err != nil {
		return fmt.Errorf("failed to install service: %w", err)
	}

	// Step 7: Migrate database if provided
	if i.DBPath != "" {
		if err := i.migrateDatabase(exec); err != nil {
			return fmt.Errorf("failed to migrate database: %w", err)
		}
	}

	// Step 8: Start service
	if err := i.startService(exec); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	fmt.Println("\n✓ Installation completed successfully!")
	fmt.Println("\nUseful commands:")
	fmt.Println("  Check status:  velocity-deploy status")
	fmt.Println("  Health check:  velocity-deploy health")
	fmt.Println("  View logs:     sudo journalctl -u velocity-report.service -f")

	return nil
}

func (i *Installer) validateBinary() error {
	fmt.Printf("Validating binary: %s\n", i.BinaryPath)

	if _, err := os.Stat(i.BinaryPath); os.IsNotExist(err) {
		return fmt.Errorf("binary not found: %s", i.BinaryPath)
	}

	// Check if binary is executable
	info, err := os.Stat(i.BinaryPath)
	if err != nil {
		return err
	}

	if info.Mode()&0111 == 0 {
		return fmt.Errorf("binary is not executable: %s", i.BinaryPath)
	}

	fmt.Println("  ✓ Binary validated")
	return nil
}

func (i *Installer) checkExisting(exec *Executor) (bool, error) {
	fmt.Println("Checking for existing installation...")

	// Check if service file exists
	output, err := exec.Run("test -f /etc/systemd/system/velocity-report.service && echo 'exists' || echo 'not found'")
	if err != nil {
		return false, err
	}

	if strings.TrimSpace(output) == "exists" {
		return true, nil
	}

	fmt.Println("  ✓ No existing installation found")
	return false, nil
}

func (i *Installer) createServiceUser(exec *Executor) error {
	fmt.Printf("Creating service user '%s'...\n", serviceUser)

	// Check if user exists
	output, err := exec.Run(fmt.Sprintf("id %s 2>/dev/null && echo 'exists' || echo 'not found'", serviceUser))
	if err != nil {
		return err
	}

	if strings.TrimSpace(output) == "exists" {
		fmt.Printf("  ✓ User '%s' already exists\n", serviceUser)
		return nil
	}

	// Create user
	_, err = exec.RunSudo(fmt.Sprintf("useradd --system --no-create-home --shell /usr/sbin/nologin %s", serviceUser))
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	fmt.Printf("  ✓ User '%s' created\n", serviceUser)
	return nil
}

func (i *Installer) createDataDirectory(exec *Executor) error {
	fmt.Printf("Creating data directory: %s\n", dataDir)

	_, err := exec.RunSudo(fmt.Sprintf("mkdir -p %s && chown %s:%s %s", dataDir, serviceUser, serviceUser, dataDir))
	if err != nil {
		return err
	}

	fmt.Printf("  ✓ Data directory created\n")
	return nil
}

func (i *Installer) installBinary(exec *Executor) error {
	fmt.Printf("Installing binary to %s...\n", installPath)

	// Copy binary to remote host if needed
	if err := exec.CopyFile(i.BinaryPath, installPath); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Set permissions
	_, err := exec.RunSudo(fmt.Sprintf("chown root:root %s && chmod 0755 %s", installPath, installPath))
	if err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	fmt.Println("  ✓ Binary installed")
	return nil
}

func (i *Installer) installService(exec *Executor) error {
	fmt.Println("Installing systemd service...")

	// Write service file to temp location
	tempFile := "/tmp/velocity-report.service"
	if err := exec.WriteFile(tempFile, serviceContent); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Move to systemd directory
	_, err := exec.RunSudo(fmt.Sprintf("mv %s /etc/systemd/system/%s", tempFile, serviceFile))
	if err != nil {
		return fmt.Errorf("failed to install service file: %w", err)
	}

	// Reload systemd
	_, err = exec.RunSudo("systemctl daemon-reload")
	if err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable service
	_, err = exec.RunSudo(fmt.Sprintf("systemctl enable %s", serviceName))
	if err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	fmt.Println("  ✓ Service installed and enabled")
	return nil
}

func (i *Installer) migrateDatabase(exec *Executor) error {
	fmt.Printf("Migrating database from: %s\n", i.DBPath)

	dbDest := filepath.Join(dataDir, "sensor_data.db")

	// Copy database
	if err := exec.CopyFile(i.DBPath, dbDest); err != nil {
		return fmt.Errorf("failed to copy database: %w", err)
	}

	// Set ownership
	_, err := exec.RunSudo(fmt.Sprintf("chown %s:%s %s", serviceUser, serviceUser, dbDest))
	if err != nil {
		return fmt.Errorf("failed to set database ownership: %w", err)
	}

	fmt.Println("  ✓ Database migrated")
	return nil
}

func (i *Installer) startService(exec *Executor) error {
	fmt.Printf("Starting %s service...\n", serviceName)

	_, err := exec.RunSudo(fmt.Sprintf("systemctl start %s", serviceName))
	if err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Wait a moment for service to start
	exec.Run("sleep 2")

	// Check if service is running
	output, err := exec.RunSudo(fmt.Sprintf("systemctl is-active %s", serviceName))
	if err != nil || strings.TrimSpace(output) != "active" {
		return fmt.Errorf("service failed to start properly")
	}

	fmt.Println("  ✓ Service started successfully")
	return nil
}
