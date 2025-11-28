package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Backup handles backing up the installation
type Backup struct {
	Target    string
	SSHUser   string
	SSHKey    string
	OutputDir string
}

// Execute performs the backup
func (b *Backup) Execute() error {
	exec := NewExecutor(b.Target, b.SSHUser, b.SSHKey, false)

	fmt.Println("Starting backup of velocity.report...")

	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("velocity-report-backup-%s", timestamp)

	// Step 1: Create local backup directory
	localBackupDir := filepath.Join(b.OutputDir, backupName)
	if _, err := exec.Run(fmt.Sprintf("mkdir -p %s", localBackupDir)); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	fmt.Printf("Backup directory: %s\n", localBackupDir)

	// Step 2: Backup binary
	if err := b.backupBinary(exec, localBackupDir); err != nil {
		return fmt.Errorf("failed to backup binary: %w", err)
	}

	// Step 3: Backup database
	if err := b.backupDatabase(exec, localBackupDir); err != nil {
		fmt.Printf("Warning: could not backup database: %v\n", err)
	}

	// Step 4: Backup service file
	if err := b.backupServiceFile(exec, localBackupDir); err != nil {
		fmt.Printf("Warning: could not backup service file: %v\n", err)
	}

	// Step 5: Create metadata file
	if err := b.createMetadata(exec, localBackupDir, timestamp); err != nil {
		fmt.Printf("Warning: could not create metadata: %v\n", err)
	}

	fmt.Printf("\n✓ Backup completed successfully!\n")
	fmt.Printf("Backup saved to: %s\n", localBackupDir)

	return nil
}

func (b *Backup) backupBinary(exec *Executor, backupDir string) error {
	fmt.Println("Backing up binary...")

	binaryDest := filepath.Join(backupDir, "velocity-report")

	if exec.IsLocal() {
		_, err := exec.RunSudo(fmt.Sprintf("cp /usr/local/bin/velocity-report %s", binaryDest))
		if err != nil {
			return err
		}
		// Make it readable by current user
		_, err = exec.RunSudo(fmt.Sprintf("chmod 644 %s", binaryDest))
		if err != nil {
			return err
		}
	} else {
		// For remote, copy via scp
		scpArgs := []string{}
		if b.SSHKey != "" {
			scpArgs = append(scpArgs, "-i", b.SSHKey)
		}

		target := b.Target
		if b.SSHUser != "" && !strings.Contains(target, "@") {
			target = fmt.Sprintf("%s@%s", b.SSHUser, target)
		}

		// First ensure we can read the binary
		_, err := exec.RunSudo("cp /usr/local/bin/velocity-report /tmp/velocity-report-backup && chmod 644 /tmp/velocity-report-backup")
		if err != nil {
			return err
		}

		// Then scp it
		args := append(scpArgs, fmt.Sprintf("%s:/tmp/velocity-report-backup", target), binaryDest)
		if _, err := exec.Run(fmt.Sprintf("scp %s", strings.Join(args, " "))); err != nil {
			return err
		}

		// Clean up temp file
		exec.Run("rm /tmp/velocity-report-backup")
	}

	fmt.Println("  ✓ Binary backed up")
	return nil
}

func (b *Backup) backupDatabase(exec *Executor, backupDir string) error {
	fmt.Println("Backing up database...")

	dbPath := "/var/lib/velocity-report/sensor_data.db"

	// Check if database exists
	checkOutput, err := exec.RunSudo(fmt.Sprintf("test -f %s && echo 'exists' || echo 'missing'", dbPath))
	if err != nil || strings.TrimSpace(checkOutput) != "exists" {
		fmt.Println("  ⊘ No database found")
		return nil
	}

	dbDest := filepath.Join(backupDir, "sensor_data.db")

	if exec.IsLocal() {
		_, err := exec.RunSudo(fmt.Sprintf("cp %s %s", dbPath, dbDest))
		if err != nil {
			return err
		}
		_, err = exec.RunSudo(fmt.Sprintf("chmod 644 %s", dbDest))
		if err != nil {
			return err
		}
	} else {
		// For remote, copy to temp first
		_, err := exec.RunSudo("cp /var/lib/velocity-report/sensor_data.db /tmp/sensor_data.db && chmod 644 /tmp/sensor_data.db")
		if err != nil {
			return err
		}

		// Then scp it
		scpArgs := []string{}
		if b.SSHKey != "" {
			scpArgs = append(scpArgs, "-i", b.SSHKey)
		}

		target := b.Target
		if b.SSHUser != "" && !strings.Contains(target, "@") {
			target = fmt.Sprintf("%s@%s", b.SSHUser, target)
		}

		args := append(scpArgs, fmt.Sprintf("%s:/tmp/sensor_data.db", target), dbDest)
		if _, err := exec.Run(fmt.Sprintf("scp %s", strings.Join(args, " "))); err != nil {
			return err
		}

		// Clean up
		exec.Run("rm /tmp/sensor_data.db")
	}

	// Get database size
	sizeOutput, _ := exec.Run(fmt.Sprintf("du -h %s | cut -f1", dbDest))
	fmt.Printf("  ✓ Database backed up (%s)\n", strings.TrimSpace(sizeOutput))

	return nil
}

func (b *Backup) backupServiceFile(exec *Executor, backupDir string) error {
	fmt.Println("Backing up service file...")

	serviceDest := filepath.Join(backupDir, "velocity-report.service")

	if exec.IsLocal() {
		_, err := exec.RunSudo(fmt.Sprintf("cp /etc/systemd/system/velocity-report.service %s", serviceDest))
		if err != nil {
			return err
		}
		_, err = exec.RunSudo(fmt.Sprintf("chmod 644 %s", serviceDest))
		if err != nil {
			return err
		}
	} else {
		// For remote
		_, err := exec.RunSudo("cp /etc/systemd/system/velocity-report.service /tmp/velocity-report.service && chmod 644 /tmp/velocity-report.service")
		if err != nil {
			return err
		}

		scpArgs := []string{}
		if b.SSHKey != "" {
			scpArgs = append(scpArgs, "-i", b.SSHKey)
		}

		target := b.Target
		if b.SSHUser != "" && !strings.Contains(target, "@") {
			target = fmt.Sprintf("%s@%s", b.SSHUser, target)
		}

		args := append(scpArgs, fmt.Sprintf("%s:/tmp/velocity-report.service", target), serviceDest)
		if _, err := exec.Run(fmt.Sprintf("scp %s", strings.Join(args, " "))); err != nil {
			return err
		}

		exec.Run("rm /tmp/velocity-report.service")
	}

	fmt.Println("  ✓ Service file backed up")
	return nil
}

func (b *Backup) createMetadata(exec *Executor, backupDir, timestamp string) error {
	fmt.Println("Creating backup metadata...")

	// Get version info if possible
	versionOutput, _ := exec.Run("/usr/local/bin/velocity-report --version 2>&1 || echo 'unknown'")

	// Get service status
	statusOutput, _ := exec.RunSudo("systemctl is-active velocity-report.service 2>&1 || echo 'unknown'")

	metadata := fmt.Sprintf(`Velocity.report Backup
=====================
Timestamp: %s
Target: %s
Binary Version: %s
Service Status: %s

Files included:
- velocity-report (binary)
- sensor_data.db (database)
- velocity-report.service (systemd service file)

To restore this backup:
1. Stop the service: sudo systemctl stop velocity-report.service
2. Restore binary: sudo cp velocity-report /usr/local/bin/velocity-report
3. Restore database: sudo cp sensor_data.db /var/lib/velocity-report/sensor_data.db
4. Restore service: sudo cp velocity-report.service /etc/systemd/system/
5. Reload systemd: sudo systemctl daemon-reload
6. Start service: sudo systemctl start velocity-report.service
`, timestamp, b.Target, strings.TrimSpace(versionOutput), strings.TrimSpace(statusOutput))

	metadataFile := filepath.Join(backupDir, "README.txt")
	if _, err := exec.Run(fmt.Sprintf("cat > %s << 'EOF'\n%s\nEOF", metadataFile, metadata)); err != nil {
		return err
	}

	fmt.Println("  ✓ Metadata created")
	return nil
}
