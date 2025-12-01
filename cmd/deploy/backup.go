package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Backup handles backing up the installation
type Backup struct {
	Target        string
	SSHUser       string
	SSHKey        string
	IdentityAgent string
	OutputDir     string
}

// Execute performs the backup
func (b *Backup) Execute() error {
	remoteExec := NewExecutor(b.Target, b.SSHUser, b.SSHKey, b.IdentityAgent, false)

	fmt.Println("Starting backup of velocity.report...")

	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("velocity-report-backup-%s", timestamp)

	// Step 1: Create local backup directory (always on local machine)
	localBackupDir := filepath.Join(b.OutputDir, backupName)
	if err := os.MkdirAll(localBackupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	fmt.Printf("Backup directory: %s\n", localBackupDir)

	// Step 2: Backup binary
	if err := b.backupBinary(remoteExec, localBackupDir); err != nil {
		return fmt.Errorf("failed to backup binary: %w", err)
	}

	// Step 3: Backup database
	if err := b.backupDatabase(remoteExec, localBackupDir); err != nil {
		fmt.Printf("Warning: could not backup database: %v\n", err)
	}

	// Step 4: Backup service file
	if err := b.backupServiceFile(remoteExec, localBackupDir); err != nil {
		fmt.Printf("Warning: could not backup service file: %v\n", err)
	}

	// Step 5: Create metadata file (locally)
	if err := b.createMetadata(remoteExec, localBackupDir, timestamp); err != nil {
		fmt.Printf("Warning: could not create metadata: %v\n", err)
	}

	fmt.Printf("\n✓ Backup completed successfully!\n")
	fmt.Printf("Backup saved to: %s\n", localBackupDir)

	return nil
}

// isLocal returns true if target is localhost
func (b *Backup) isLocal() bool {
	return b.Target == "localhost" || b.Target == "127.0.0.1" || b.Target == ""
}

// scpFromRemote copies a file from remote host to local destination
func (b *Backup) scpFromRemote(remotePath, localDest string) error {
	args := []string{}

	if b.SSHKey != "" {
		args = append(args, "-i", b.SSHKey)
	}

	// Disable strict host key checking for automation
	args = append(args, "-o", "StrictHostKeyChecking=no")
	args = append(args, "-o", "UserKnownHostsFile=/dev/null")

	target := b.Target
	if b.SSHUser != "" && !strings.Contains(target, "@") {
		target = fmt.Sprintf("%s@%s", b.SSHUser, target)
	}

	// SCP from remote to local
	args = append(args, fmt.Sprintf("%s:%s", target, remotePath), localDest)

	debugLog("SCP (pull): scp %v", args)
	cmd := exec.Command("scp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scp failed: %w, output: %s", err, string(output))
	}
	return nil
}

func (b *Backup) backupBinary(remoteExec *Executor, backupDir string) error {
	fmt.Println("Backing up binary...")

	binaryDest := filepath.Join(backupDir, "velocity-report")

	if b.isLocal() {
		// For local, just copy with sudo
		cmd := exec.Command("sudo", "cp", "/usr/local/bin/velocity-report", binaryDest)
		if err := cmd.Run(); err != nil {
			return err
		}
		// Make it readable by current user
		cmd = exec.Command("sudo", "chmod", "644", binaryDest)
		if err := cmd.Run(); err != nil {
			return err
		}
	} else {
		// For remote:
		// 1. Copy binary to temp location on remote (readable)
		_, err := remoteExec.RunSudo("cp /usr/local/bin/velocity-report /tmp/velocity-report-backup && chmod 644 /tmp/velocity-report-backup")
		if err != nil {
			return err
		}

		// 2. SCP from remote to local
		if err := b.scpFromRemote("/tmp/velocity-report-backup", binaryDest); err != nil {
			return err
		}

		// 3. Clean up temp file on remote
		remoteExec.Run("rm -f /tmp/velocity-report-backup")
	}

	fmt.Println("  ✓ Binary backed up")
	return nil
}

func (b *Backup) backupDatabase(remoteExec *Executor, backupDir string) error {
	fmt.Println("Backing up database...")

	dbPath := "/var/lib/velocity-report/sensor_data.db"

	// Check if database exists (on remote or local target)
	checkOutput, err := remoteExec.RunSudo(fmt.Sprintf("test -f %s && echo 'exists' || echo 'missing'", dbPath))
	if err != nil || strings.TrimSpace(checkOutput) != "exists" {
		fmt.Println("  ⊘ No database found")
		return nil
	}

	dbDest := filepath.Join(backupDir, "sensor_data.db")

	if b.isLocal() {
		// For local, just copy with sudo
		cmd := exec.Command("sudo", "cp", dbPath, dbDest)
		if err := cmd.Run(); err != nil {
			return err
		}
		cmd = exec.Command("sudo", "chmod", "644", dbDest)
		if err := cmd.Run(); err != nil {
			return err
		}
	} else {
		// For remote:
		// 1. Copy database to temp location on remote (readable)
		_, err := remoteExec.RunSudo("cp /var/lib/velocity-report/sensor_data.db /tmp/sensor_data.db && chmod 644 /tmp/sensor_data.db")
		if err != nil {
			return err
		}

		// 2. SCP from remote to local
		if err := b.scpFromRemote("/tmp/sensor_data.db", dbDest); err != nil {
			return err
		}

		// 3. Clean up temp file on remote
		remoteExec.Run("rm -f /tmp/sensor_data.db")
	}

	// Get database size (locally, since the file is now local)
	fi, err := os.Stat(dbDest)
	var sizeStr string
	if err == nil {
		size := fi.Size()
		if size >= 1024*1024 {
			sizeStr = fmt.Sprintf("%.1fM", float64(size)/(1024*1024))
		} else if size >= 1024 {
			sizeStr = fmt.Sprintf("%.1fK", float64(size)/1024)
		} else {
			sizeStr = fmt.Sprintf("%dB", size)
		}
	} else {
		sizeStr = "unknown"
	}
	fmt.Printf("  ✓ Database backed up (%s)\n", sizeStr)

	return nil
}

func (b *Backup) backupServiceFile(remoteExec *Executor, backupDir string) error {
	fmt.Println("Backing up service file...")

	serviceDest := filepath.Join(backupDir, "velocity-report.service")

	if b.isLocal() {
		// For local, just copy with sudo
		cmd := exec.Command("sudo", "cp", "/etc/systemd/system/velocity-report.service", serviceDest)
		if err := cmd.Run(); err != nil {
			return err
		}
		cmd = exec.Command("sudo", "chmod", "644", serviceDest)
		if err := cmd.Run(); err != nil {
			return err
		}
	} else {
		// For remote:
		// 1. Copy service file to temp location on remote (readable)
		_, err := remoteExec.RunSudo("cp /etc/systemd/system/velocity-report.service /tmp/velocity-report.service && chmod 644 /tmp/velocity-report.service")
		if err != nil {
			return err
		}

		// 2. SCP from remote to local
		if err := b.scpFromRemote("/tmp/velocity-report.service", serviceDest); err != nil {
			return err
		}

		// 3. Clean up temp file on remote
		remoteExec.Run("rm -f /tmp/velocity-report.service")
	}

	fmt.Println("  ✓ Service file backed up")
	return nil
}

func (b *Backup) createMetadata(remoteExec *Executor, backupDir, timestamp string) error {
	fmt.Println("Creating backup metadata...")

	// Get version info from target (may be remote)
	versionOutput, _ := remoteExec.Run("/usr/local/bin/velocity-report --version 2>&1 || echo 'unknown'")

	// Get service status from target
	statusOutput, _ := remoteExec.RunSudo("systemctl is-active velocity-report.service 2>&1 || echo 'unknown'")

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

	// Write metadata file locally (not via executor)
	metadataFile := filepath.Join(backupDir, "README.txt")
	if err := os.WriteFile(metadataFile, []byte(metadata), 0644); err != nil {
		return err
	}

	fmt.Println("  ✓ Metadata created")
	return nil
}
