package main

import (
	"fmt"
	"strings"
)

// Rollback handles rolling back to a previous version
type Rollback struct {
	Target  string
	SSHUser string
	SSHKey  string
	DryRun  bool
}

// Execute performs the rollback
func (r *Rollback) Execute() error {
	exec := NewExecutor(r.Target, r.SSHUser, r.SSHKey, r.DryRun)

	fmt.Println("Starting rollback to previous version...")

	// Step 1: Find most recent backup
	backupDir, err := r.findLatestBackup(exec)
	if err != nil {
		return fmt.Errorf("failed to find backup: %w", err)
	}

	fmt.Printf("Found backup: %s\n", backupDir)

	// Step 2: Confirm rollback
	if !r.DryRun {
		fmt.Print("Are you sure you want to rollback? This will stop the service and restore the backup. [y/N]: ")
		var confirm string
		fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "y" {
			fmt.Println("Rollback cancelled")
			return nil
		}
	}

	// Step 3: Stop service
	if err := r.stopService(exec); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	// Step 4: Restore binary
	if err := r.restoreBinary(exec, backupDir); err != nil {
		return fmt.Errorf("failed to restore binary: %w", err)
	}

	// Step 5: Optionally restore database
	if err := r.restoreDatabase(exec, backupDir); err != nil {
		fmt.Printf("Warning: could not restore database: %v\n", err)
	}

	// Step 6: Start service
	if err := r.startService(exec); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Step 7: Verify service is healthy
	if err := r.verifyHealth(exec); err != nil {
		return fmt.Errorf("health check failed after rollback: %w", err)
	}

	fmt.Println("\n✓ Rollback completed successfully!")
	return nil
}

func (r *Rollback) findLatestBackup(exec *Executor) (string, error) {
	fmt.Println("Looking for backups...")

	// List backup directories sorted by name (which includes timestamp)
	output, err := exec.RunSudo("ls -1t /var/lib/velocity-report/backups/ 2>/dev/null | head -1")
	if err != nil {
		return "", fmt.Errorf("no backups found")
	}

	backupName := strings.TrimSpace(output)
	if backupName == "" {
		return "", fmt.Errorf("no backups found in /var/lib/velocity-report/backups/")
	}

	backupDir := fmt.Sprintf("/var/lib/velocity-report/backups/%s", backupName)

	// Verify backup contains binary
	checkOutput, err := exec.RunSudo(fmt.Sprintf("test -f %s/velocity-report && echo 'exists' || echo 'missing'", backupDir))
	if err != nil || strings.TrimSpace(checkOutput) != "exists" {
		return "", fmt.Errorf("backup directory exists but binary not found: %s", backupDir)
	}

	return backupDir, nil
}

func (r *Rollback) stopService(exec *Executor) error {
	fmt.Println("Stopping service...")

	_, err := exec.RunSudo("systemctl stop velocity-report.service")
	if err != nil {
		return err
	}

	exec.Run("sleep 2")
	fmt.Println("  ✓ Service stopped")
	return nil
}

func (r *Rollback) restoreBinary(exec *Executor, backupDir string) error {
	fmt.Printf("Restoring binary from: %s\n", backupDir)

	binaryPath := fmt.Sprintf("%s/velocity-report", backupDir)

	_, err := exec.RunSudo(fmt.Sprintf("cp %s /usr/local/bin/velocity-report", binaryPath))
	if err != nil {
		return fmt.Errorf("failed to restore binary: %w", err)
	}

	_, err = exec.RunSudo("chown root:root /usr/local/bin/velocity-report && chmod 0755 /usr/local/bin/velocity-report")
	if err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	fmt.Println("  ✓ Binary restored")
	return nil
}

func (r *Rollback) restoreDatabase(exec *Executor, backupDir string) error {
	dbBackup := fmt.Sprintf("%s/sensor_data.db", backupDir)

	// Check if database backup exists
	checkOutput, err := exec.RunSudo(fmt.Sprintf("test -f %s && echo 'exists' || echo 'missing'", dbBackup))
	if err != nil || strings.TrimSpace(checkOutput) != "exists" {
		fmt.Println("  ⊘ No database backup found, keeping current database")
		return nil
	}

	fmt.Print("Database backup found. Restore it? This will replace current data. [y/N]: ")
	var confirm string
	if !r.DryRun {
		fmt.Scanln(&confirm)
	} else {
		confirm = "n"
	}

	if strings.ToLower(confirm) != "y" {
		fmt.Println("  ⊘ Keeping current database")
		return nil
	}

	fmt.Println("  Restoring database...")

	_, err = exec.RunSudo(fmt.Sprintf("cp %s /var/lib/velocity-report/sensor_data.db", dbBackup))
	if err != nil {
		return err
	}

	_, err = exec.RunSudo("chown velocity:velocity /var/lib/velocity-report/sensor_data.db")
	if err != nil {
		return err
	}

	fmt.Println("  ✓ Database restored")
	return nil
}

func (r *Rollback) startService(exec *Executor) error {
	fmt.Println("Starting service...")

	_, err := exec.RunSudo("systemctl start velocity-report.service")
	if err != nil {
		return err
	}

	exec.Run("sleep 3")
	fmt.Println("  ✓ Service started")
	return nil
}

func (r *Rollback) verifyHealth(exec *Executor) error {
	fmt.Println("Verifying service health...")

	output, err := exec.RunSudo("systemctl is-active velocity-report.service")
	if err != nil || strings.TrimSpace(output) != "active" {
		return fmt.Errorf("service is not active")
	}

	fmt.Println("  ✓ Service is running")
	return nil
}
